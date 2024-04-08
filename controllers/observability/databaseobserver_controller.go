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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

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
//+kubebuilder:rbac:groups=apps,resources=pods;deployments;services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=configmaps,verbs=get;list
//+kubebuilder:rbac:groups="",resources=pods;deployments;services;events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

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

	r.Log.WithName(constants.LogReconcile).Info(constants.LogCRStart, "NamespacedName", req.NamespacedName)

	// fetch databaseObserver
	api := &apiv1.DatabaseObserver{}
	if e := r.Get(context.TODO(), req.NamespacedName, api); e != nil {

		// if CR is not found or does not exist then
		// consider either CR has been deleted
		if apiError.IsNotFound(e) {
			r.Log.WithName(constants.LogReconcile).Info(constants.LogCREnd)
			return ctrl.Result{}, nil
		}

		r.Log.WithName(constants.LogReconcile).Error(e, constants.ErrorCRRetrieve)
		r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonFailedCRRetrieval, constants.EventMessageFailedCRRetrieval)
		return ctrl.Result{}, e

	}

	// evaluate overall custom resource readiness at the end of the stack
	defer r.validateCustomResourceReadiness(ctx, req)

	// initialize databaseObserver custom resource
	if e := r.initialize(ctx, api, req); e != nil {
		return ctrl.Result{}, e
	}

	// validate specs
	if e := r.validateSpecs(api); e != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsExporterDeploymentReady,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonDeploymentSpecValidationFailed,
			Message: constants.MessageExporterDeploymentSpecValidationFailed,
		})
		if e := r.Status().Update(ctx, api); e != nil {
			r.Log.WithName(constants.LogReconcile).Error(e, constants.ErrorStatusUpdate)
		}
		r.Log.WithName(constants.LogExportersDeploy).Error(e, constants.ErrorSpecValidationFailedDueToAnError)
		return ctrl.Result{}, e
	}

	// create resource if they do not exist
	exporterDeployment := &ObservabilityDeploymentResource{}
	if res, e := r.createResourceIfNotExists(exporterDeployment, api, ctx, req); e != nil {
		return res, e
	}

	if res, e := r.checkDeploymentForUpdates(exporterDeployment, api, ctx, req); e != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsExporterDeploymentReady,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonDeploymentUpdateFailed,
			Message: constants.MessageExporterDeploymentUpdateFailed,
		})
		return res, e
	}

	exporterService := &ObservabilityServiceResource{}
	if res, e := r.createResourceIfNotExists(exporterService, api, ctx, req); e != nil {
		return res, e
	}

	exporterServiceMonitor := &ObservabilityServiceMonitorResource{}
	if res, e := r.createResourceIfNotExists(exporterServiceMonitor, api, ctx, req); e != nil {
		return res, e
	}

	// check if deployment pods are ready
	return r.validateDeploymentReadiness(api, ctx, req)
}

// initialize method sets the initial status to PENDING, exporterConfig and sets the base condition
func (r *DatabaseObserverReconciler) initialize(ctx context.Context, api *apiv1.DatabaseObserver, req ctrl.Request) error {

	if api.Status.Conditions == nil || len(api.Status.Conditions) == 0 {

		// set condition
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsCRAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonInitStart,
			Message: constants.MessageCRInitializationStarted,
		})

		api.Status.Status = string(constants.StatusObservabilityPending)
		api.Status.ExporterConfig = constants.UnknownValue
		if e := r.Status().Update(ctx, api); e != nil {
			r.Log.WithName(constants.LogReconcile).Error(e, constants.ErrorStatusUpdate)
			return e
		}

	}

	return nil
}

// validateSpecs method checks the values and secrets passed in the spec
func (r *DatabaseObserverReconciler) validateSpecs(api *apiv1.DatabaseObserver) error {

	// If either Vault Fields are empty, then assume a DBPassword secret is supplied. If the DBPassword secret not found, then error out
	if api.Spec.Database.DBPassword.VaultOCID == "" || api.Spec.Database.DBPassword.VaultSecretName == "" {
		dbSecret := &corev1.Secret{}
		if e := r.Get(context.TODO(), types.NamespacedName{Name: api.Spec.Database.DBPassword.SecretName, Namespace: api.Namespace}, dbSecret); e != nil {
			r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonSpecError, constants.EventMessageSpecErrorDBPasswordSecretMissing)
			return e
		}
	}

	// Does DB Connection String Secret Name actually exist
	dbConnectSecret := &corev1.Secret{}
	if e := r.Get(context.TODO(), types.NamespacedName{Name: api.Spec.Database.DBConnectionString.SecretName, Namespace: api.Namespace}, dbConnectSecret); e != nil {
		r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonSpecError, constants.EventMessageSpecErrorDBConnectionStringSecretMissing)
		return e
	}

	// Does DB User String Secret Name actually exist
	dbUserSecret := &corev1.Secret{}
	if e := r.Get(context.TODO(), types.NamespacedName{Name: api.Spec.Database.DBUser.SecretName, Namespace: api.Namespace}, dbUserSecret); e != nil {
		r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonSpecError, constants.EventMessageSpecErrorDBPUserSecretMissing)
		return e
	}

	// Does a custom configuration configmap actually exist, if provided
	if configurationCMName := api.Spec.Exporter.ExporterConfig.Configmap.Name; configurationCMName != "" {
		configurationCM := &corev1.ConfigMap{}
		if e := r.Get(context.TODO(), types.NamespacedName{Name: configurationCMName, Namespace: api.Namespace}, configurationCM); e != nil {
			r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonSpecError, constants.EventMessageSpecErrorConfigmapMissing)
			return e
		}
	}

	// Does DBWallet actually exist, if provided
	if dbWalletSecretName := api.Spec.Database.DBWallet.SecretName; dbWalletSecretName != "" {
		dbWalletSecret := &corev1.Secret{}
		if e := r.Get(context.TODO(), types.NamespacedName{Name: dbWalletSecretName, Namespace: api.Namespace}, dbWalletSecret); e != nil {
			r.Recorder.Event(api, corev1.EventTypeWarning, constants.EventReasonSpecError, constants.EventMessageSpecErrorDBWalletSecretMissing)
			return e
		}
	}

	return nil // valid, did not encounter any errors
}

// createResourceIfNotExists method creates an ObserverResource if they have not yet been created
func (r *DatabaseObserverReconciler) createResourceIfNotExists(or ObserverResource, api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	conditionType, logger, groupVersionKind := or.identify()

	// update after
	defer r.Status().Update(ctx, api)

	// generate desired object based on api.Spec
	desiredObj, genErr := or.generate(api, r.Scheme)
	if genErr != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGeneralResourceGenerationFailed,
			Message: constants.MessageResourceGenerationFailed,
		})
		return ctrl.Result{}, genErr
	}

	// if resource exists, retrieve the resource
	foundObj := &unstructured.Unstructured{}
	foundObj.SetGroupVersionKind(groupVersionKind)
	getErr := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundObj)

	// if resource not found, create resource then return
	if getErr != nil && apiError.IsNotFound(getErr) {

		if e := r.Create(context.TODO(), desiredObj); e != nil { // create
			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonGeneralResourceCreationFailed,
				Message: constants.MessageResourceCreationFailed,
			})
			r.Log.WithName(logger).Error(e, constants.ErrorResourceCreationFailure, "ResourceName", desiredObj.GetName(), "Kind", groupVersionKind, "Namespace", req.Namespace)
			return ctrl.Result{}, e
		}

		// mark ready if created
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonGeneralResourceCreated,
			Message: constants.MessageResourceCreated,
		})
		r.Log.WithName(logger).Info(constants.LogResourceCreated, "ResourceName", desiredObj.GetName(), "Kind", groupVersionKind, "Namespace", req.Namespace)

	} else if getErr != nil { // if an error occurred
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGeneralResourceValidationFailureDueToError,
			Message: constants.MessageResourceReadinessValidationFailed,
		})
		r.Log.WithName(logger).Error(getErr, constants.ErrorResourceRetrievalFailureDueToAnError, "ResourceName", desiredObj.GetName(), "Kind", groupVersionKind, "Namespace", req.Namespace)
		return ctrl.Result{}, getErr

	} else if getErr == nil && conditionType != constants.IsExporterDeploymentReady { // exclude deployment
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonGeneralResourceValidationCompleted,
			Message: constants.MessageResourceReadinessValidated,
		})
		r.Log.WithName(logger).Info(constants.LogResourceFound, "ResourceName", desiredObj.GetName(), "Kind", groupVersionKind, "Namespace", req.Namespace)

	}

	// if no other error and resource, other than Deployments, have already been created before, end validation and return
	return ctrl.Result{}, nil
}

// checkDeploymentForUpdates method checks the deployment if it needs to be updated
func (r *DatabaseObserverReconciler) checkDeploymentForUpdates(or ObserverResource, api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// declare
	foundDeployment := &appsv1.Deployment{}

	// generate object
	desiredObj, genErr := or.generate(api, r.Scheme)
	if genErr != nil {
		return ctrl.Result{}, genErr
	}

	// convert
	desiredDeployment := &appsv1.Deployment{}
	if e := r.Scheme.Convert(desiredObj, desiredDeployment, nil); e != nil {
		return ctrl.Result{}, e
	}

	// retrieve latest deployment
	if e := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundDeployment); e != nil {
		return ctrl.Result{}, e
	}
	// check for containerImage
	if constants.IsUpdateRequiredForContainerImage(desiredDeployment, foundDeployment) {
		foundDeployment.Spec.Template.Spec.Containers[0].Image = constants.GetExporterImage(api)

		if e := r.updateDeployment(api, ctx, req, foundDeployment, constants.MessageExporterDeploymentImageUpdated, constants.EventMessageUpdatedImageSucceeded); e != nil {
			return ctrl.Result{}, e
		}
	}

	// retrieve latest deployment
	if e := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundDeployment); e != nil {
		return ctrl.Result{}, e
	}
	// check environment variables
	if constants.IsUpdateRequiredForEnvironmentVars(desiredDeployment, foundDeployment) {
		foundDeployment.Spec.Template.Spec.Containers[0].Env = constants.GetExporterEnvs(api)

		if e := r.updateDeployment(api, ctx, req, foundDeployment, constants.MessageExporterDeploymentEnvironmentUpdated, constants.EventMessageUpdatedEnvironmentSucceeded); e != nil {
			return ctrl.Result{}, e
		}
	}

	// retrieve latest deployment
	foundDeployment = &appsv1.Deployment{}
	if e := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundDeployment); e != nil {
		return ctrl.Result{}, e
	}
	// check config-volume, creds and ocikey
	if constants.IsUpdateRequiredForVolumes(desiredDeployment, foundDeployment) {
		foundDeployment.Spec.Template.Spec.Volumes = constants.GetExporterDeploymentVolumes(api)
		foundDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = constants.GetExporterDeploymentVolumeMounts(api)

		if e := r.updateDeployment(api, ctx, req, foundDeployment, constants.MessageExporterDeploymentVolumesUpdated, constants.EventMessageUpdatedVolumesSucceeded); e != nil {
			return ctrl.Result{}, e
		}
	}

	// update status for exporter config
	var setConfigmapNameStatus string
	for _, v := range desiredDeployment.Spec.Template.Spec.Volumes {
		if v.Name == constants.DefaultConfigVolumeString {
			setConfigmapNameStatus = v.ConfigMap.Name
			api.Status.ExporterConfig = setConfigmapNameStatus
		}
	}
	if api.Status.ExporterConfig != setConfigmapNameStatus {
		api.Status.ExporterConfig = constants.DefaultValue
	}
	r.Status().Update(ctx, api)

	// retrieve latest deployment
	foundDeployment = &appsv1.Deployment{}
	if e := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundDeployment); e != nil {
		return ctrl.Result{}, e
	}
	// check replicateCount
	if constants.IsUpdateRequiredForReplicas(desiredDeployment, foundDeployment) {
		desiredReplicaCount := constants.GetExporterReplicas(api)
		foundDeployment.Spec.Replicas = &desiredReplicaCount

		if e := r.updateDeployment(api, ctx, req, foundDeployment, constants.MessageExporterDeploymentReplicaUpdated, constants.EventMessageUpdatedReplicaSucceeded); e != nil {
			return ctrl.Result{}, e
		}
	}

	return ctrl.Result{}, nil
}

// updateDeployment method updates the deployment and sets the condition
func (r *DatabaseObserverReconciler) updateDeployment(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request, d *appsv1.Deployment, updateMessage string, recorderMessage string) error {

	// make update
	defer r.Status().Update(ctx, api)

	if e := r.Update(context.TODO(), d); e != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsExporterDeploymentReady,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonDeploymentUpdateFailed,
			Message: constants.MessageExporterDeploymentUpdateFailed,
		})
		r.Log.WithName(constants.LogExportersDeploy).Error(e, constants.ErrorDeploymentUpdate, "ResourceName", d.GetName(), "Kind", "Deployment", "Namespace", req.Namespace)
		return e
	}

	// update completed, however the pods needs to be validated for readiness
	meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
		Type:    constants.IsExporterDeploymentReady,
		Status:  metav1.ConditionFalse,
		Reason:  constants.ReasonDeploymentUpdated,
		Message: updateMessage,
	})
	r.Log.WithName(constants.LogExportersDeploy).Info(constants.LogResourceUpdated, "ResourceName", d.GetName(), "Kind", "Deployment", "Namespace", req.Namespace)
	r.Recorder.Event(api, corev1.EventTypeNormal, constants.EventReasonUpdateSucceeded, recorderMessage)

	return nil
}

// validateDeploymentReadiness method evaluates deployment readiness by checking the status of all deployment pods
func (r *DatabaseObserverReconciler) validateDeploymentReadiness(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	d := &appsv1.Deployment{}
	rName := constants.DefaultExporterDeploymentPrefix + api.Name

	// update after
	defer r.Status().Update(ctx, api)

	// get latest deployment
	if e := r.Get(context.TODO(), types.NamespacedName{Name: rName, Namespace: api.Namespace}, d); e != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsExporterDeploymentReady,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonGeneralResourceValidationFailureDueToError,
			Message: constants.MessageExporterDeploymentValidationFailed,
		})
		return ctrl.Result{}, e
	}

	// get deployment labels
	labels := d.Spec.Template.Labels
	cLabels := client.MatchingLabels{}
	for k, v := range labels {
		cLabels[k] = v
	}

	// list pods
	pods := &corev1.PodList{}
	if e := r.List(context.TODO(), pods, []client.ListOption{cLabels}...); e != nil {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsExporterDeploymentReady,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonDeploymentFailed,
			Message: constants.MessageExporterDeploymentListingFailed,
		})
		return ctrl.Result{}, e
	}

	// check each pod phase
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodFailed {
			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    constants.IsExporterDeploymentReady,
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonDeploymentFailed,
				Message: constants.MessageExporterDeploymentFailed,
			})
			return ctrl.Result{}, errors.New(constants.ErrorDeploymentPodsFailure)

		} else if pod.Status.Phase != corev1.PodRunning { // pod could be creating,
			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    constants.IsExporterDeploymentReady,
				Status:  metav1.ConditionUnknown,
				Reason:  constants.ReasonDeploymentPending,
				Message: constants.MessageExporterDeploymentPending,
			})
			return ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}, nil
		}
	}

	// once all pods are found to be running, mark deployment as ready and the exporter as ready
	meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
		Type:    constants.IsExporterDeploymentReady,
		Status:  metav1.ConditionTrue,
		Reason:  constants.ReasonDeploymentSuccessful,
		Message: constants.MessageExporterDeploymentSuccessful,
	})
	return ctrl.Result{}, nil
}

// validateCustomResourceReadiness method evaluates CR readiness by cycling through all conditions and checking for any condition with False Status
func (r *DatabaseObserverReconciler) validateCustomResourceReadiness(ctx context.Context, req ctrl.Request) {

	// get latest object
	api := &apiv1.DatabaseObserver{}
	if e := r.Get(context.TODO(), req.NamespacedName, api); e != nil {
		r.Log.WithName(constants.LogReconcile).Error(e, constants.ErrorCRRetrieve)
		return
	}

	// make update
	defer r.Status().Update(ctx, api)

	if meta.IsStatusConditionPresentAndEqual(api.Status.Conditions, constants.IsExporterDeploymentReady, metav1.ConditionUnknown) {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsCRAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonValidationInProgress,
			Message: constants.MessageCRValidationWaiting,
		})
		api.Status.Status = string(constants.StatusObservabilityPending)
	} else if meta.IsStatusConditionFalse(api.Status.Conditions, constants.IsExporterDeploymentReady) ||
		meta.IsStatusConditionFalse(api.Status.Conditions, constants.IsExporterServiceReady) ||
		meta.IsStatusConditionFalse(api.Status.Conditions, constants.IsExporterServiceMonitorReady) {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsCRAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  constants.ReasonReadyFailed,
			Message: constants.MessageCRValidationFailed,
		})
		api.Status.Status = string(constants.StatusObservabilityError)
	} else {
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    constants.IsCRAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  constants.ReasonReadyValidated,
			Message: constants.MessageCRValidated,
		})
		api.Status.Status = string(constants.StatusObservabilityReady)
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
