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
	"strings"

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

/************************************************
 * Status
 *************************************************/

// ComputeWorkloadStatus computes the workload status for OrdsSrvs.
func (r *OrdsSrvsReconciler) ComputeWorkloadStatus(ctx context.Context, ordssrvs *dbapi.OrdsSrvs) string {
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


// UpdateStatus updates the status of OrdsSrvs.
func (r *OrdsSrvsReconciler) UpdateStatus(
	ctx context.Context,
	req ctrl.Request,
	rState *OrdsSrvsReconcileState,
	workloadStatusCondition metav1.Condition,
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
