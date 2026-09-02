/*
** Copyright (c) 2024 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package controllers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const poolProbeTimeout = 3 * time.Second

// ReconcilePoolProbes probes configured pool aliases without changing workload health.
func (r *OrdsSrvsReconciler) ReconcilePoolProbes(ctx context.Context, req ctrl.Request, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState) (time.Duration, error) {
	intervalSeconds := ordssrvs.Spec.PoolProbeIntervalSeconds
	configuredPools := len(ordssrvs.Spec.PoolSettings)

	if intervalSeconds == 0 || ordssrvs.Spec.GlobalSettings.CentralConfigURL != "" {
		if err := r.updatePoolProbeStatus(ctx, req, nil, "Disabled", ""); err != nil {
			return 0, err
		}
		return 0, nil
	}

	workloadStatus := r.ComputeWorkloadStatus(ctx, ordssrvs)
	if workloadStatus != "Healthy" {
		if len(ordssrvs.Status.PoolProbes) == 0 && ordssrvs.Status.PoolsHealth == "" {
			if err := r.updatePoolProbeStatus(ctx, req, nil, "Unknown", ""); err != nil {
				return 0, err
			}
		}
		return time.Duration(intervalSeconds) * time.Second, nil
	}

	due, requeueAfter := poolProbeDue(ordssrvs)
	if !due {
		return requeueAfter, nil
	}

	poolProbes := make([]dbapi.PoolProbeStatus, 0, configuredPools)
	for _, pool := range ordssrvs.Spec.PoolSettings {
		poolProbes = append(poolProbes, probePoolAlias(ctx, ordssrvs, rState, pool.PoolName))
	}

	poolsHealth, poolsReachable := summarizePoolProbes(poolProbes)
	if err := r.updatePoolProbeStatus(ctx, req, poolProbes, poolsHealth, poolsReachable); err != nil {
		return 0, err
	}

	return time.Duration(intervalSeconds) * time.Second, nil
}

// probePoolAlias uses the documented pool-alias URL. A redirect means that the
// alias is valid; a 404 means that the pool is invalid or does not exist.
func probePoolAlias(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, poolName string) dbapi.PoolProbeStatus {
	scheme := "http"
	port := int32(8080)
	if rState.httpsEnabled {
		scheme = "https"
		port = 8443
		if ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort != nil {
			port = *ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort
		}
	} else if ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort != nil {
		port = *ordssrvs.Spec.GlobalSettings.StandaloneHTTPPort
	}

	contextPath := strings.Trim(ordssrvs.Spec.GlobalSettings.StandaloneContextPath, "/")
	if contextPath == "" {
		contextPath = "ords"
	}
	poolPath := "/" + contextPath
	if poolName != "default" {
		poolPath += "/" + poolName
	}
	poolPath += "/"

	endpoint := fmt.Sprintf("%s://%s.%s.svc:%d%s", scheme, ordssrvs.Name, ordssrvs.Namespace, port, poolPath)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if scheme == "https" {
		// ORDS commonly uses a self-signed certificate for its local Service.
		// The request is limited to this OrdsSrvs Service and uses the same
		// localhost Host header as the Kubernetes lifecycle probes.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- local OrdsSrvs Service probe
	}
	client := &http.Client{
		Timeout:   poolProbeTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	result := dbapi.PoolProbeStatus{PoolName: poolName, LastChecked: metav1.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.Outcome = "ERROR"
		return result
	}
	req.Host = "localhost"

	resp, err := client.Do(req)
	if err != nil {
		result.Outcome = "ERROR"
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.HTTPStatusCode = int32(resp.StatusCode)
	switch {
	case resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest:
		result.Outcome = "OK"
	case resp.StatusCode == http.StatusNotFound:
		result.Outcome = "POOL_NOT_FOUND"
	case resp.StatusCode >= http.StatusInternalServerError:
		result.Outcome = "SERVER_ERROR"
	default:
		result.Outcome = "UNEXPECTED"
	}
	return result
}

func summarizePoolProbes(poolProbes []dbapi.PoolProbeStatus) (string, string) {
	reachable := 0
	for _, poolProbe := range poolProbes {
		if poolProbe.Outcome == "OK" {
			reachable++
		}
	}

	total := len(poolProbes)
	poolsReachable := fmt.Sprintf("%d/%d", reachable, total)
	switch {
	case total == 0:
		return "Unknown", poolsReachable
	case reachable == total:
		return "Healthy", poolsReachable
	case reachable == 0:
		return "Unhealthy", poolsReachable
	default:
		return "Partial", poolsReachable
	}
}

func poolProbeDue(ordssrvs *dbapi.OrdsSrvs) (bool, time.Duration) {
	interval := time.Duration(ordssrvs.Spec.PoolProbeIntervalSeconds) * time.Second
	if interval <= 0 || len(ordssrvs.Status.PoolProbes) == 0 {
		return true, 0
	}

	var lastChecked time.Time
	for _, poolProbe := range ordssrvs.Status.PoolProbes {
		if poolProbe.LastChecked.After(lastChecked) {
			lastChecked = poolProbe.LastChecked.Time
		}
	}
	if lastChecked.IsZero() {
		return true, 0
	}

	nextProbe := lastChecked.Add(interval)
	if time.Now().Before(nextProbe) {
		return false, time.Until(nextProbe)
	}
	return true, 0
}

func (r *OrdsSrvsReconciler) updatePoolProbeStatus(ctx context.Context, req ctrl.Request, poolProbes []dbapi.PoolProbeStatus, poolsHealth, poolsReachable string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &dbapi.OrdsSrvs{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			return err
		}
		if latest.Status.PoolsHealth == poolsHealth &&
			latest.Status.PoolsReachable == poolsReachable &&
			reflect.DeepEqual(latest.Status.PoolProbes, poolProbes) {
			return nil
		}

		base := latest.DeepCopy()
		latest.Status.PoolProbes = poolProbes
		latest.Status.PoolsHealth = poolsHealth
		latest.Status.PoolsReachable = poolsReachable
		return r.Status().Patch(ctx, latest, client.MergeFrom(base))
	})
}

/************************************************
 * Status
 *************************************************/

// ComputeWorkloadStatus computes the workload status for OrdsSrvs.
func (r *OrdsSrvsReconciler) ComputeWorkloadStatus(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) string {
	logr := log.FromContext(ctx).WithName("computeWorkloadStatus")

	var availableWorkload int32
	var desiredWorkload int32
	switch ordssrvs.Spec.WorkloadType {
	case "StatefulSet":
		workload := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("StatefulSet not ready")
		}
		availableWorkload = workload.Status.ReadyReplicas
		desiredWorkload = workload.Status.Replicas
	case "DaemonSet":
		workload := &appsv1.DaemonSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("DaemonSet not ready")
		}
		availableWorkload = workload.Status.NumberReady
		desiredWorkload = workload.Status.DesiredNumberScheduled
	default:
		workload := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("Deployment not ready")
		}
		// readyReplicas: Pods currently passing readiness.
		// availableReplicas: Pods that are ready and have remained ready for the Deployment’s minReadySeconds.
		// availableReplicas is the safer customer-facing signal for OrdsSrvs Healthy and Available=True.
		availableWorkload = workload.Status.AvailableReplicas
		desiredWorkload = workload.Status.Replicas
	}

	// Available is changed to False when the workload degrades, so preserve the
	// prior high-level status to distinguish a first startup from a loss of
	// previously available capacity.
	wasAvailable := ordssrvs.Status.Status == "Healthy" || ordssrvs.Status.Status == "Degraded" || ordssrvs.Status.Status == "Unhealthy" ||
		meta.IsStatusConditionTrue(ordssrvs.Status.Conditions, typeAvailableORDS)

	var workloadStatus string
	switch {
	case availableWorkload == desiredWorkload && desiredWorkload > 0:
		workloadStatus = "Healthy"
		ordssrvs.Status.OrdsInstalled = true
	case wasAvailable && availableWorkload == 0:
		workloadStatus = "Unhealthy"
	case wasAvailable:
		workloadStatus = "Degraded"
	case availableWorkload == 0:
		workloadStatus = "Preparing"
	default:
		workloadStatus = "Progressing"
	}

	return workloadStatus
}

// workloadAvailabilityCondition reports whether the child workload is available.
func (r *OrdsSrvsReconciler) workloadAvailabilityCondition(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) metav1.Condition {
	workloadStatus := r.ComputeWorkloadStatus(ctx, ordssrvs)
	condition := metav1.Condition{Type: typeAvailableORDS, Status: metav1.ConditionFalse}

	switch workloadStatus {
	case "Healthy":
		condition.Status = metav1.ConditionTrue
		condition.Reason = "WorkloadAvailable"
		condition.Message = "Workload is available"
	case "Degraded":
		condition.Reason = "WorkloadDegraded"
		condition.Message = "Workload has lost previously available capacity"
	case "Unhealthy":
		condition.Reason = "WorkloadUnhealthy"
		condition.Message = "Workload has no available capacity"
	case "Progressing":
		condition.Reason = "WorkloadProgressing"
		condition.Message = "Workload is progressing"
	default:
		condition.Reason = "WorkloadPreparing"
		condition.Message = "Workload is preparing"
	}

	return condition
}

// UpdateStatus updates the status of OrdsSrvs.
func (r *OrdsSrvsReconciler) UpdateStatus(
	ctx context.Context,
	req ctrl.Request,
	rState *OrdsSrvsReconcileState,
	workloadStatusCondition metav1.Condition,
) error {
	rState.specDebug.V(1).Info("UpdateStatus", "condition", workloadStatusCondition.Reason, "message", workloadStatusCondition.Message)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &dbapi.OrdsSrvs{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			return err
		}
		base := latest.DeepCopy()

		workloadStatus := r.ComputeWorkloadStatus(ctx, latest)

		// Mongo port safety
		mongoPort := int32(0)
		if latest.Spec.GlobalSettings.MongoEnabled {
			if latest.Spec.GlobalSettings.MongoPort != nil {
				mongoPort = *latest.Spec.GlobalSettings.MongoPort
			} else {
				mongoPort = 27017 // default
			}
		}

		// Workload condition
		meta.SetStatusCondition(&latest.Status.Conditions, workloadStatusCondition)

		// Fill status
		latest.Status.Status = workloadStatus
		latest.Status.WorkloadType = latest.Spec.WorkloadType

		// ORDSVersion extraction (avoid panic if image has no ":tag")
		parts := strings.Split(latest.Spec.Image, ":")
		if len(parts) > 1 {
			latest.Status.ORDSVersion = parts[len(parts)-1]
		} else {
			latest.Status.ORDSVersion = "latest"
		}

		latest.Status.HTTPPort = latest.Spec.GlobalSettings.StandaloneHTTPPort
		latest.Status.HTTPSPort = latest.Spec.GlobalSettings.StandaloneHTTPSPort
		latest.Status.MongoPort = mongoPort
		latest.Status.RestartRequired = rState.RestartPods
		latest.Status.ObservedGeneration = latest.Generation

		// Patch status to reduce conflicts
		if err := r.Status().Patch(ctx, latest, client.MergeFrom(base)); err != nil {
			// Retry only on conflicts
			if apierrors.IsConflict(err) {
				return err
			}
			return err
		}
		return nil
	})
}
