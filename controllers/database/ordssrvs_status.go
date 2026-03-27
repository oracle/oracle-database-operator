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

func probePoolLanding(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, poolName string) (dbapi.PoolProbeStatus, error) {

	const defaultHTTPSPort int32 = 8443
	httpsPort := defaultHTTPSPort
	if ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort != nil {
		httpsPort = *ordssrvs.Spec.GlobalSettings.StandaloneHTTPSPort
	}

	ctxPath := ordssrvs.Spec.GlobalSettings.StandaloneContextPath
	if ctxPath == "" {
		ctxPath = "/ords"
	}

    svcHost := fmt.Sprintf("%s.%s.svc", ordssrvs.Name, ordssrvs.Namespace)
    url := fmt.Sprintf("https://%s:%d%s/", svcHost, httpsPort, ctxPath)
	if poolName != "default" {
		url = fmt.Sprintf("%s%s/", url, poolName)
	}

    tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // avoid if you can in prod
    }

    client := &http.Client{
        Timeout:   3 * time.Second,
        Transport: tr,
        // Critical: capture the 302 instead of following Location: https://localhost/...
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return dbapi.PoolProbeStatus{}, err
    }

    // Critical: override Host header like curl -H "Host: localhost"
    req.Host = "localhost"

    resp, err := client.Do(req)
    if err != nil {
        return dbapi.PoolProbeStatus{PoolName: poolName, Outcome: "ERROR", Display: url+" (ERROR)", LastChecked: metav1.Now()}, nil
    }
    defer resp.Body.Close()

    code := resp.StatusCode
	var outcome string
    switch {
    case code >= 300 && code <= 399:
        outcome = "OK"
    case code == 404:
        outcome = "POOL_NOT_FOUND"
    case code == 400:
        outcome = "BAD_REQUEST"
    case code >= 500 && code <= 599:
        outcome = "SERVER_ERROR"
    default:
        outcome = "UNEXPECTED"
    }

	display:=fmt.Sprintf("%s|%s|(%d)|%s",poolName,url,code,outcome)
    out := dbapi.PoolProbeStatus{PoolName: poolName, Display: display, Outcome:outcome, LastChecked: metav1.Now()}


    return out, nil
}
 
/************************************************
 * Status
 *************************************************/

func (r *OrdsSrvsReconciler) ComputeWorkloadStatus(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) (string){
	logr := log.FromContext(ctx).WithName("computeWorkloadStatus")

	var readyWorkload int32
	var desiredWorkload int32
	switch ordssrvs.Spec.WorkloadType {
	//nolint:goconst
	case "StatefulSet":
		workload := &appsv1.StatefulSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("StatefulSet not ready")
		}
		readyWorkload = workload.Status.ReadyReplicas
		desiredWorkload = workload.Status.Replicas
	//nolint:goconst
	case "DaemonSet":
		workload := &appsv1.DaemonSet{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("DaemonSet not ready")
		}
		readyWorkload = workload.Status.NumberReady
		desiredWorkload = workload.Status.DesiredNumberScheduled
	default:
		workload := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: ordssrvs.Name, Namespace: ordssrvs.Namespace}, workload); err != nil {
			logr.Info("Deployment not ready")
		}
		readyWorkload = workload.Status.ReadyReplicas
		desiredWorkload = workload.Status.Replicas
	}

	var workloadStatus string
	switch readyWorkload {
	case 0:
		workloadStatus = "Preparing"
	case desiredWorkload:
		workloadStatus = "Healthy"
		ordssrvs.Status.OrdsInstalled = true
	default:
		workloadStatus = "Progressing"
	}

	return workloadStatus
}

func probePools(ctx context.Context, ordssrvs *dbapi.OrdsSrvs, rState *OrdsSrvsReconcileState, pools []*dbapi.PoolSettings) ([]dbapi.PoolProbeStatus, string, string){

	var poolProbes []dbapi.PoolProbeStatus
	poolsTotal:=int32(0)
	poolsOK:=int32(0)


		for i := 0; i < len(pools); i++ {
			poolName := pools[i].PoolName
			poolsTotal++

			pps, error := probePoolLanding(ctx, ordssrvs, poolName) 
			if(error!=nil){
				rState.logger.Error(error, "Error probing pool "+poolName)
			}

			if pps.Outcome == "OK"{
				poolsOK++
			}

			poolProbes = append(poolProbes, pps)
		}
	
	poolsValid := fmt.Sprintf("%d/%d",poolsOK,poolsTotal)

	var poolsHealth string
	if poolsTotal == poolsOK{
		poolsHealth="Healthy"
	} else if poolsOK == 0 && poolsTotal>0 {
		poolsHealth="Unhealthy"
	} else if poolsOK < poolsTotal{
		poolsHealth="Partial"
	} else {
		poolsHealth="Unknown"
	}

		rState.logger.V(1).Info("Pool Probing", "poolsValid", poolsValid, "poolsHealth", poolsHealth)

	return poolProbes, poolsHealth, poolsValid
}

func computePoolsHealthyCondition(poolProbeIntervalSeconds int32, poolsHealth string)(metav1.Condition){

	if(poolProbeIntervalSeconds==0){
		poolsCond := metav1.Condition{
		Type:   "PoolsHealthy",
		Status: metav1.ConditionUnknown,
		Reason: "Disabled",
		Message:"Pool probes are disabled, poolProbeIntervalSeconds=0",
		}
		return poolsCond
	}

	poolsCond := metav1.Condition{
		Type:   "PoolsHealthy",
		Status: metav1.ConditionUnknown,
		Reason: "NotProbed",
		Message:"Pool probes not executed yet",
	}

	switch poolsHealth { // Healthy|Partial|Unhealthy
	case "Healthy":
    	poolsCond.Status = metav1.ConditionTrue
    	poolsCond.Reason = "AllPoolsOK"
    	poolsCond.Message = "All pools reachable"
	case "Partial":
		poolsCond.Status = metav1.ConditionFalse
		poolsCond.Reason = "SomePoolsFailing"
		poolsCond.Message = "Some pools failing probes"
	case "Unhealthy":
		poolsCond.Status = metav1.ConditionFalse
		poolsCond.Reason = "AllPoolsFailing"
		poolsCond.Message = "All pools failing probes"
	}

	return poolsCond
}

// returns the most recent LastChecked across all pool probes in status
func latestLastChecked(ords *dbapi.OrdsSrvs) (time.Time, bool) {
    var latest time.Time
    found := false
    for _, p := range ords.Status.PoolProbes {
        if p.LastChecked.IsZero() {
            continue
        }
        t := p.LastChecked.Time
        if !found || t.After(latest) {
            latest = t
            found = true
        }
    }
    return latest, found
}

// Decide whether to probe now, and when to requeue next.
func probeDue(ordssrvs *dbapi.OrdsSrvs) (due bool, requeueAfter time.Duration) {

	poolProbeInterval := time.Duration(ordssrvs.Spec.PoolProbeIntervalSeconds) * time.Second

	// interval==0 disables probing.
    if poolProbeInterval <= 0 {
        return false, 0
    }

    last, ok := latestLastChecked(ordssrvs)
    if !ok {
        // never probed -> probe immediately
        return true, 0
    }

    next := last.Add(poolProbeInterval)
    now := time.Now()

    if !now.Before(next) {
        return true, 0
    }
    return false, time.Until(next)
}

// Update OrsSrvs status
func (r *OrdsSrvsReconciler) UpdateStatus(
    ctx context.Context,
    req ctrl.Request,
    rState *OrdsSrvsReconcileState,
    workloadStatusCondition metav1.Condition,
    probePoolsEnabled bool,
) error {
    // DEBUG update status during spec change, not during pool probing
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

        // Pools probing
        poolProbes := latest.Status.PoolProbes
        poolsHealth := latest.Status.PoolsHealth
        poolsValid := latest.Status.PoolsValid
        interval := latest.Spec.PoolProbeIntervalSeconds

        ccs:=latest.Spec.GlobalSettings.CentralConfigUrl!=""
        if interval == 0 || ccs {
            probePoolsEnabled = false
            poolsHealth = "Disabled"
            poolsValid = "0/0"
            poolProbes = nil
        }

        if probePoolsEnabled {
            oldHealth := latest.Status.PoolsHealth
            oldValid := latest.Status.PoolsValid

            due, _ := probeDue(latest)
            if due && workloadStatus == "Healthy" {
                poolSetting:=latest.Spec.PoolSettings
                //instanceAPIEnabled:=latest.Spec.GlobalSettings.InstanceAPIEnabled != nil &&
                //  *latest.Spec.GlobalSettings.InstanceAPIEnabled
                //if instanceAPIEnabled {
                    //poolSetting=r.generatePoolSettingsFromPod()
                //} 
                poolProbes, poolsHealth, poolsValid = probePools(ctx, latest, rState, poolSetting)
            }

            if poolsHealth != oldHealth || poolsValid != oldValid {
                rState.logger.Info("Pools health changed", "from", oldHealth, "to", poolsHealth, "poolsValid", poolsValid)
            }

            poolsHealthyCondition := computePoolsHealthyCondition(interval, poolsHealth)
            meta.SetStatusCondition(&latest.Status.Conditions, poolsHealthyCondition)
        }

        // Workload condition
        meta.SetStatusCondition(&latest.Status.Conditions, workloadStatusCondition)

        // Overall status
        overallStatus := workloadStatus
        if probePoolsEnabled && workloadStatus == "Healthy" && poolsHealth != "Healthy" {
            overallStatus = "Partial"
        }

        // Fill status
        latest.Status.Status = overallStatus
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
        latest.Status.PoolProbes = poolProbes
        latest.Status.PoolsHealth = poolsHealth
        latest.Status.PoolsValid = poolsValid
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