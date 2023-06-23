/*
** Copyright (c) 2022 Oracle and/or its affiliates.
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
	"errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	constants "github.com/oracle/oracle-database-operator/commons/observability"
)

// DatabaseObserverReconciler reconciles a DatabaseObserver object
type DatabaseObserverReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=observability.oracle.com,resources=databaseobservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=observability.oracle.com,resources=databaseobservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=observability.oracle.com,resources=databaseobservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=pods;deployments;services;configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods;deployments;services;secrets;configmaps;events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules;servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DatabaseObserver object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *DatabaseObserverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues(constants.LogReconcile, req.NamespacedName)
	log.Info("Begin Reconcile", "NamespacedName", req.NamespacedName)

	cr := &apiv1.DatabaseObserver{} // Fetch CR
	if err := r.Get(context.TODO(), req.NamespacedName, cr); err != nil {
		log.Error(err, "Failed to fetch Observability CRD")
		return ctrl.Result{}, err
	}

	defer r.validateReadiness(cr, ctx, req)

	if err := r.initialize(ctx, cr, req); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.validateSpecs(cr); err != nil {
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:    constants.PhaseControllerValidation,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonValidationFailure,
			Message: err.Error(),
		})
		r.updateStatusAndRetrieve(cr, ctx, req)
		return ctrl.Result{}, err
	}

	if res, err := r.manageConfigmap(cr, ctx, req); err != nil {
		return res, err
	}

	exporterDeployment := &ObservabilityDeploymentResource{}
	if res, err := r.manageResource(exporterDeployment, cr, ctx, req); err != nil {
		return res, err
	}
	if err := r.compareOrUpdateDeployment(exporterDeployment, cr, ctx, req); err != nil {
		return ctrl.Result{}, err
	}

	exporterService := &ObservabilityServiceResource{}
	if res, err := r.manageResource(exporterService, cr, ctx, req); err != nil {
		return res, err
	}

	exporterServiceMonitor := &ObservabilityServiceMonitorResource{}
	if res, err := r.manageResource(exporterServiceMonitor, cr, ctx, req); err != nil {
		return res, err
	}

	exporterGrafanaOutputCM := &ObservabilityGrafanaJsonResource{}
	if res, err := r.manageResource(exporterGrafanaOutputCM, cr, ctx, req); err != nil {
		return res, err
	}

	if !r.isExporterDeploymentReady(cr, ctx, req) {
		return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *DatabaseObserverReconciler) initialize(ctx context.Context, cr *apiv1.DatabaseObserver, req ctrl.Request) error {

	if cr.Status.Conditions == nil || len(cr.Status.Conditions) == 0 {

		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:    string(constants.StatusObservabilityReady),
			Status:  metav1.ConditionUnknown,
			Reason:  constants.ReasonInitStart,
			Message: "Starting reconciliation",
		})

		cr.Status.Status = string(constants.StatusObservabilityPending)
		cr.Status.ExporterConfig = constants.UnknownValue

		r.updateStatusAndRetrieve(cr, ctx, req)
		r.Recorder.Event(cr, corev1.EventTypeNormal, constants.ReasonInitSuccess, "Initialization of observability resource completed")
	}

	return nil
}

func (r *DatabaseObserverReconciler) validateSpecs(api *apiv1.DatabaseObserver) error {

	// Does DB Password Secret Name have a value
	// if all secretName, vaultOCID and vaultSecretName are empty, then error out
	if api.Spec.Database.DBPassword.SecretName == "" &&
		api.Spec.Database.DBPassword.VaultOCID == "" &&
		api.Spec.Database.DBPassword.VaultSecretName == "" {
		return errors.New("spec validation failed due to required field dbPassword not having a value")
	}

	// Does DB Password Secret Name actually exist if vault fields are not used?
	if api.Spec.Database.DBPassword.VaultOCID == "" || api.Spec.Database.DBPassword.VaultSecretName == "" {
		dbSecret := &corev1.Secret{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: api.Spec.Database.DBPassword.SecretName, Namespace: api.Namespace}, dbSecret); err != nil {
			r.Log.Error(err, "Failed to find secret", "Name", api.Spec.Database.DBPassword.SecretName, "Namespace", api.Namespace)
			return err
		}
	}

	//sn := api.Spec.Database.DBServiceName.SecretName
	//user := api.Spec.Database.DBUser.SecretName
	connStr := api.Spec.Database.DBConnectionString.SecretName

	// Does ConnectionString have a value
	//var snAndUserCannotBeUsed = sn == "" || user == ""

	if connStr == "" {

		return errors.New("spec validation failed due to required fields dbConnectionString or dbServiceName and dbUser not having a value")

	} else {

		// Does DB Connection String Secret Name actually exist
		dbConnectSecret := &corev1.Secret{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: api.Spec.Database.DBConnectionString.SecretName, Namespace: api.Namespace}, dbConnectSecret); err != nil {
			r.Log.Error(err, "Failed to find secret", "Name", api.Spec.Database.DBConnectionString.SecretName, "Namespace", api.Namespace)
			return err
		}

	}

	// Did the user provide a custom configuration configmap
	// If so, does it actually exist
	configurationCMName := api.Spec.Exporter.ExporterConfig.Configmap.Name
	if configurationCMName != "" {

		configurationCM := &corev1.ConfigMap{}
		if err := r.Get(context.TODO(), types.NamespacedName{Name: configurationCMName, Namespace: api.Namespace}, configurationCM); err != nil {
			r.Log.Error(err, "Failed to find configuration config map", "Name", configurationCMName, "Namespace", api.Namespace)
			return err
		}
	}

	meta.RemoveStatusCondition(&api.Status.Conditions, constants.PhaseControllerValidation) // Clear any validation conditions
	r.Log.Info("Completed reconcile validation")
	return nil // valid, did not encounter any errors
}

func (r *DatabaseObserverReconciler) validateReadiness(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) {

	// get latest object
	if err := r.Get(context.TODO(), req.NamespacedName, api); err != nil {
		r.Log.Error(err, "Failed to fetch updated CR")
	}

	// evaluate if there exists conditions present that invalidates readiness
	if api.Status.Conditions != nil && len(api.Status.Conditions) > 1 {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    string(constants.StatusObservabilityReady),
			Status:  metav1.ConditionFalse,
			Reason:  "DeploymentUnsuccessful",
			Message: "Observability exporter deployment failure",
		})
		api.Status.Status = string(constants.StatusObservabilityError)
	} else {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    string(constants.StatusObservabilityReady),
			Status:  metav1.ConditionTrue,
			Reason:  "ResourceReady",
			Message: "Observability exporter deployed successfully",
		})
		api.Status.Status = string(constants.StatusObservabilityReady)
	}

	if err := r.Status().Update(ctx, api); err != nil {
		r.Log.Error(err, "Failed to update resource status")
	}

}

func (r *DatabaseObserverReconciler) updateStatusAndRetrieve(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) {

	// make update
	if err := r.Status().Update(ctx, api); err != nil {
		r.Log.Error(err, "Failed to update resource status")
	}

	// refresh cr before changes
	if err := r.Get(context.TODO(), req.NamespacedName, api); err != nil {
		r.Log.Error(err, "Failed to fetch updated CR")
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseObserverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.DatabaseObserver{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
