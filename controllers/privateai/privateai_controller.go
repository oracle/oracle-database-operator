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

package privateai

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PrivateAiReconciler reconciles a PrivateAi object
type PrivateAiReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Config   *rest.Config
	Recorder record.EventRecorder
}

// To requeue after 15 secs allowing graceful state changes
var requeueY ctrl.Result = ctrl.Result{Requeue: true, RequeueAfter: 25 * time.Second}
var requeueN ctrl.Result = ctrl.Result{}

var resultNq = ctrl.Result{Requeue: false}
var resultQ = ctrl.Result{Requeue: true, RequeueAfter: 40 * time.Second}

const PrivateAiFinalizer = "privateai.oracle.com/privateaifinalizer"

// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods;pods/log;pods/exec;secrets;containers;services;events;configmaps;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PrivateAiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	r.Log.Info("Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false

	privateAiInst := &privateaiv4.PrivateAi{}
	defer r.updateReconcileStatus(privateAiInst, ctx, &result, &err, &blocked, &completed)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, privateAiInst)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		return requeueY, err
	}

	// Manage resource cleanup when deletion has been requested.
	if err, inProgress := r.managePrivateAiDeletion(privateAiInst); err != nil {
		result = resultNq
		if inProgress {
			return result, nil
		}
		return result, err
	}

	if result.Requeue {
		r.Log.Info("Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Error(err, err.Error())
		return result, err
	}

	// aicommons.resetState()

	// Allow to go inside only if status is Available or completed.

	if privateAiInst.Status.Status == privateaiv4.StatusPending || privateAiInst.Status.Status == privateaiv4.StatusUpdating || privateAiInst.Status.Status == privateaiv4.StatusError {
		r.Log.Info("Checking Status for " + privateAiInst.Name + ". privateAiInst.Status.Status set to " + privateAiInst.Status.Status)
		return requeueY, nil
	}

	/* Initialize Status */
	if privateAiInst.Status.Status == "" {
		privateAiInst.Status.Status = privateaiv4.StatusPending
		privateAiInst.Status.Replicas = 0
		privateAiInst.Status.ReleaseUpdate = privateaiv4.ValueUnavailable
		r.Status().Update(ctx, privateAiInst)
	}

	authEnabled := parseBoolFlag(privateAiInst.Spec.PaiEnableAuthentication)
	externalSvcEnabled := parseBoolFlag(privateAiInst.Spec.IsExternalSvc)
	storageEnabled := privateAiInst.Spec.StorageClass != ""

	if authEnabled {
		if _, err := r.ensureSecret(ctx, req, privateAiInst); err != nil {
			return resultNq, err
		}
	}

	if privateAiInst.Spec.PaiConfigFile != nil &&
		privateAiInst.Spec.PaiConfigFile.Name != "" &&
		privateAiInst.Spec.PaiConfigFile.MountLocation != "" {
		if _, err := r.ensureConfigMap(ctx, req, privateAiInst); err != nil {
			return resultNq, err
		}
	}
	// First validate
	result, err = r.validate(privateAiInst, ctx, req)
	if result.Requeue {
		r.Log.Info("Spec validation failed, Reconcile queued")
		return result, nil
	}
	if err != nil {
		r.Log.Info("Spec validation failed")
		return result, nil
	}

	// Create PVC
	if storageEnabled {
		// ensurePVCs verifies that all required PersistentVolumeClaims (PVCs) exist for the given PrivateAI instance.
		// It creates any missing PVCs and returns an error if the operation fails.
		if _, err := r.ensurePVCs(ctx, privateAiInst); err != nil {
			return resultNq, err
		}
	}

	// ensureServices verifies that the required Kubernetes Services are created and configured for the PrivateAI instance.
	// It takes the provided context, PrivateAI instance, and a flag indicating whether external service access is enabled.
	// Returns an error if the service creation or configuration fails.
	if _, err := r.ensureServices(ctx, privateAiInst, "local"); err != nil {
		return resultNq, err
	}

	// Populate LoadBalancerIP in status from the external service (if enabled)
	if externalSvcEnabled {
		if _, err := r.ensureServices(ctx, privateAiInst, "external"); err != nil {
			return resultNq, err
		}
	}

	// desiredDeploy constructs and returns a Deployment object configured for the PrivateAI instance.
	// It uses the provided privateAiInst to build the desired deployment specification with appropriate
	// settings, replicas, and container configuration for the PrivateAI workload.
	desiredDeploy := aicommons.BuildDeploySetForPrivateAI(privateAiInst)
	foundDeploy, err := r.checkAiDeploymentSet(desiredDeploy.Name, desiredDeploy.Namespace)
	if apierrors.IsNotFound(err) {
		result, err = r.deployPrivateAiDeploymentSet(privateAiInst, desiredDeploy)
		if err != nil {
			return resultNq, err
		}
	} else if err != nil {
		return resultNq, err
	} else {
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(privateAiInst.Namespace),
		}
		if foundDeploy.Spec.Selector != nil {
			listOpts = append(listOpts, client.MatchingLabels(foundDeploy.Spec.Selector.MatchLabels))
		}
		if err := r.Client.List(ctx, podList, listOpts...); err != nil {
			return resultNq, err
		}

		var firstPod *corev1.Pod
		if len(podList.Items) > 0 {
			firstPod = &podList.Items[0]
		}

		if _, err := aicommons.ManageReplicas(r, privateAiInst, r.Client, r.Config, foundDeploy, podList, ctx, req, r.Log); err != nil {
			return resultNq, err
		}

		if firstPod != nil {
			podIP := firstPod.Status.PodIP
			hostIP := firstPod.Status.HostIP
			if privateAiInst.Status.PodIP != podIP || privateAiInst.Status.NodeIP != hostIP {
				privateAiInst.Status.PodIP = podIP
				privateAiInst.Status.NodeIP = hostIP
				if err := r.Status().Update(ctx, privateAiInst); err != nil {
					return resultNq, err
				}
			}
		}

		if _, err := aicommons.UpdateDeploySetForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, foundDeploy, firstPod, r.Log); err != nil {
			return resultNq, err
		}
	}

	completed = true
	r.Log.Info("Reconcile completed")

	return resultQ, nil
}

// #############################################################################
//
//	Update each reconcile condtion/status
//
// #############################################################################
func (r *PrivateAiReconciler) updateReconcileStatus(m *privateaiv4.PrivateAi, ctx context.Context,
	result *ctrl.Result, err *error, blocked *bool, completed *bool) {

	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, m)

	m.Status.Replicas = int(m.Spec.Replicas)
	if m.Status.Status != "" {
		if m.Status.Status != privateaiv4.StatusReady {
			r.Log.Info("Changing status from " + m.Status.Status + " to " + privateaiv4.StatusReady)
		}
	}
	m.Status.Status = privateaiv4.StatusReady
	m.Status.ReleaseUpdate = "V2.0"
	//m.Status.ApiKey = m.Spec.PaiSecret.Name
	//m.Status.PodIP =
	// m.Status.LoadBalancerIP =

	errMsg := func() string {
		if *err != nil {
			return (*err).Error()
		}
		return "no reconcile errors"
	}()
	var condition metav1.Condition
	if *completed {
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileCompelete,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileCompleteReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if *blocked {
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileBlocked,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileBlockedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if result.Requeue {
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileQueued,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileQueuedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if *err != nil {
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileError,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileErrorReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else {
		return
	}
	if len(m.Status.Conditions) > 0 {
		meta.RemoveStatusCondition(&m.Status.Conditions, condition.Type)
	}
	meta.SetStatusCondition(&m.Status.Conditions, condition)
}

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *PrivateAiReconciler) validate(m *privateaiv4.PrivateAi, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//var err error
	//eventReason := "Spec Error"
	//var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	r.Log.Info("Completed reconcile validation")

	return requeueN, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *PrivateAiReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&privateaiv4.PrivateAi{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}).
		Complete(r)
}

// ###################### Event Filter Predicate ######################
func (r *PrivateAiReconciler) checkAiDeploymentSet(name string,
	namespace string,
) (*appsv1.Deployment, error) {

	found := &appsv1.Deployment{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, found)

	if err != nil {
		return found, err
	}
	return found, nil

}

func (r *PrivateAiReconciler) checkAiSvc(name string,
	namespace string,
) (*corev1.Service, error) {

	found := &corev1.Service{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, found)

	if err != nil {
		return found, err
	}

	return found, nil
}

// This function deploy the DeploymentSet
func (r *PrivateAiReconciler) deployPrivateAiDeploymentSet(instance *privateaiv4.PrivateAi,
	dep *appsv1.Deployment,
) (ctrl.Result, error) {

	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	message := "Inside the deployDeploymentSet function"
	aicommons.LogMessages("DEBUG", message, nil, instance, r.Log)

	r.waitForScheme()
	controllerutil.SetControllerReference(instance, dep, r.Scheme)
	_, err := r.checkAiDeploymentSet(dep.Name, instance.Namespace)
	jsn, _ := json.Marshal(dep)
	aicommons.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	if err != nil && errors.IsNotFound(err) {

		// Create the DeploymentSet
		reqLogger.Info("Creating Deployment for PrivateAI")
		err = r.Client.Create(context.TODO(), dep)

		message := "Inside the create Deployment set block to create DeploymentSet " + aicommons.GetFmtStr(dep.Name)
		aicommons.LogMessages("DEBUG", message, nil, instance, r.Log)

		if err != nil {
			// DeploymentSet failed
			reqLogger.Error(err, "Failed to create DeploymentSet", "DeploymentSet.space", dep.Namespace, "DeploymentSet.Name", dep.Name)
			//instance.Status.ShardStatus[dep.Name] = "Deployment Failed"
			return ctrl.Result{}, err
		}
	} else if err != nil {
		// Error that isn't due to the StaefulSet not existing
		reqLogger.Error(err, "Failed to get DeploymentSet")
		return ctrl.Result{}, err
	}

	message = "DeploymentSet Exist " + aicommons.GetFmtStr(dep.Name) + " already exist"
	aicommons.LogMessages("DEBUG", message, nil, instance, r.Log)

	return ctrl.Result{}, nil
}

// createService creates a Kubernetes Service resource for the PrivateAi instance.
// It configures the service with appropriate ports and selectors to expose the PrivateAi
// deployment within the cluster. Returns an error if the service creation fails.
func (r *PrivateAiReconciler) createService(instance *privateaiv4.PrivateAi,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	ctx := context.TODO()

	r.waitForScheme()
	controllerutil.SetControllerReference(instance, dep, r.Scheme)

	if err := r.Client.Get(ctx, types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, &corev1.Service{}); err != nil {
		if !errors.IsNotFound(err) {
			reqLogger.Error(err, "Failed to retrieve Service", "Service.namespace", dep.Namespace, "Service.name", dep.Name)
			return ctrl.Result{}, err
		}

		reqLogger.Info("Creating service", "Service.namespace", dep.Namespace, "Service.name", dep.Name)
		if jsn, merr := json.Marshal(dep); merr == nil {
			aicommons.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
		}
		if err := r.Client.Create(ctx, dep); err != nil {
			reqLogger.Error(err, "Failed to create Service", "Service.namespace", dep.Namespace, "Service.name", dep.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	reqLogger.Info("Service already exists", "Service.namespace", dep.Namespace, "Service.name", dep.Name)
	return ctrl.Result{}, nil
}

// createPvc creates a PersistentVolumeClaim for the PrivateAi instance if it does not already exist.
// It sets the controller reference to establish ownership, attempts to retrieve an existing PVC,
// and creates a new one if not found. The function logs the PVC configuration and any errors encountered.
// Returns a reconciliation result with Requeue set to true if the PVC was successfully created,
// indicating that reconciliation should be rerun. Returns an error if the PVC creation fails
// or if there is an unexpected error retrieving the PVC.
func (r *PrivateAiReconciler) createPvc(instance *privateaiv4.PrivateAi,
	dep *corev1.PersistentVolumeClaim,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	r.waitForScheme()
	controllerutil.SetControllerReference(instance, dep, r.Scheme)
	found := &corev1.PersistentVolumeClaim{}

	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	aicommons.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && errors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating PVC")
		err = r.Client.Create(context.TODO(), dep)
		if err != nil {
			// Service creation failed
			reqLogger.Error(err, "Failed to create PVC", "PVC.namespace", dep.Namespace, "PVC.Name", dep.Name)
			return ctrl.Result{}, err
		} else {
			// Service creation was successful
			return ctrl.Result{Requeue: true}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "Failed to find the  Service details")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// managePrivateAiDeletion handles the deletion lifecycle of a PrivateAi resource.
// It ensures proper cleanup by managing finalizers and executing finalization logic.
//
// The function performs the following steps:
// 1. Checks if the PrivateAi instance is marked for deletion via DeletionTimestamp
// 2. If marked for deletion and has the PrivateAiFinalizer:
//   - Executes finalization logic to clean up associated resources
//   - Removes the finalizer to allow Kubernetes to proceed with deletion
//   - Updates the instance in the cluster
//
// 3. If not marked for deletion, ensures the PrivateAiFinalizer is added to the instance
//
// Parameters:
//   - instance: The PrivateAi resource being reconciled
//
// Returns:
//   - error: An error if finalization or update operations fail; a custom message
//     if deletion is in progress
//   - bool: true if deletion is in progress (error should not be logged as a stack trace),
//     false otherwise
//
// Note: When deletion is in progress (bool=true), the returned error is informational
// and should not be treated as a critical failure.
func (r *PrivateAiReconciler) managePrivateAiDeletion(instance *privateaiv4.PrivateAi) (error, bool) {

	isPrivateToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isPrivateToBeDeleted {
		if controllerutil.ContainsFinalizer(instance, PrivateAiFinalizer) {
			// Run finalization logic for finalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizePrivateAi(instance); err != nil {
				return err, false
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(instance, PrivateAiFinalizer)
			err := r.Client.Update(context.TODO(), instance)
			if err != nil {
				return err, false
			}
		}
		// Send true because delete is in progress and it is a custom delete message
		// We don't need to print custom err stack as we are deleting the topology
		return fmt.Errorf("delete of the privateai topology is in progress"), true
	}

	// Add finalizer for this CR
	if instance.DeletionTimestamp == nil {
		if !controllerutil.ContainsFinalizer(instance, PrivateAiFinalizer) {
			if err := r.addFinalizer(instance); err != nil {
				return err, false
			}
		}
	}

	return nil, false
}

func (r *PrivateAiReconciler) addFinalizer(instance *privateaiv4.PrivateAi) error {
	reqLogger := r.Log.WithValues("instance.Namespace", instance.Namespace, "instance.Name", instance.Name)
	controllerutil.AddFinalizer(instance, PrivateAiFinalizer)

	// Update CR
	err := r.Client.Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Failed to update privateai Database  with finalizer")
		return err
	}
	return nil
}

func (r *PrivateAiReconciler) finalizePrivateAi(instance *privateaiv4.PrivateAi) error {
	ctx := context.Background()

	if depSet, err := aicommons.CheckDepSet(instance, r.Client); err == nil {
		if err := r.Client.Delete(ctx, depSet); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
		pvcName := instance.Name + "-oradata-vol4-" + instance.Name + "-0"
		if err := aicommons.DelPvc(pvcName, instance, r.Client, r.Log); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if exSvcEnabled, err := strconv.ParseBool(instance.Spec.IsExternalSvc); err == nil && exSvcEnabled {
		if svc, err := aicommons.CheckSvc(instance.Name+"-svc", instance, r.Client); err == nil {
			if err := r.Client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		} else if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if svc, err := aicommons.CheckSvc(instance.Name, instance, r.Client); err == nil {
		if err := r.Client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *PrivateAiReconciler) waitForScheme() {
	for r.Scheme == nil {
		r.Log.Info("Waiting for scheme to be initialized...")
		time.Sleep(3 * time.Second)
	}
}

func (r *PrivateAiReconciler) ensureConfigMap(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi) (reconcile.Result, error) {
	if privateAiInst.Spec.PaiConfigFile.Name != "" && privateAiInst.Spec.PaiConfigFile.MountLocation != "" {
		_ = aicommons.PatchConfigMap(privateAiInst.Spec.PaiConfigFile.Name, privateAiInst, r.Client, r.Log)
		privateAiInst.Status.PaiConfigMap.Name = privateAiInst.Spec.PaiConfigFile.Name
		rversion := aicommons.GetConfigMapResourceVersion(privateAiInst.Spec.PaiConfigFile.Name, privateAiInst, r.Client, r.Log)
		if privateAiInst.Status.PaiConfigMap.ResourceVersion == "" {
			privateAiInst.Status.PaiConfigMap.ResourceVersion = rversion
		} else if privateAiInst.Status.PaiConfigMap.ResourceVersion != rversion {
			// Updating Status before Update
			privateAiInst.Status.Status = privateaiv4.StatusUpdating
			r.Status().Update(ctx, privateAiInst)

			r.Log.Info("change in ConfigMap " + privateAiInst.Spec.PaiConfigFile.Name + " is detected ")
			foundDeploy, err := r.checkAiDeploymentSet(privateAiInst.Name, privateAiInst.Namespace)
			if err == nil {
				err = aicommons.UpdateRestartedAtAnnotation(r, privateAiInst, r.Client, r.Config, foundDeploy, ctx, req, r.Log)
				if err != nil {
					privateAiInst.Status.Status = privateaiv4.StatusError
					r.Status().Update(context.Background(), privateAiInst)
					r.Log.Info("Error occurred while rolling out the deployments after detecting secrets change")
					return ctrl.Result{}, err
				}
			}
			privateAiInst.Status.PaiConfigMap.ResourceVersion = rversion
		} else {
			privateAiInst.Status.PaiConfigMap.ResourceVersion = rversion
		}
	}
	return ctrl.Result{}, nil
}

func (r *PrivateAiReconciler) ensureSecret(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi) (reconcile.Result, error) {
	if privateAiInst.Spec.PaiSecret.Name == "" {
		return requeueN, fmt.Errorf("PaiAuthentication is enabled but no PaiSecret is defined")
	}
	apiKey, certPem := aicommons.ReadSecret(privateAiInst.Spec.PaiSecret.Name, privateAiInst, r.Client, r.Log)
	if apiKey == "" || apiKey == "NONE" {
		return requeueN, fmt.Errorf("PaiAuthentication is enabled but apikey is not found in secret")
	}
	if certPem == "NONE" {
		r.Log.Info("PaiAuthentication is enabled but cert.pem not found in secret") // optional soft warning
	}

	_ = aicommons.PatchSecret(privateAiInst.Spec.PaiSecret.Name, privateAiInst, r.Client, r.Log)

	privateAiInst.Status.PaiSecret.Name = privateAiInst.Spec.PaiSecret.Name
	rversion := aicommons.GetSecretResourceVersion(privateAiInst.Spec.PaiSecret.Name, privateAiInst, r.Client, r.Log)
	if privateAiInst.Status.PaiSecret.ResourceVersion == "" {
		privateAiInst.Status.PaiSecret.ResourceVersion = rversion
	} else if privateAiInst.Status.PaiSecret.ResourceVersion != rversion {
		// Updating Status before Update
		privateAiInst.Status.Status = privateaiv4.StatusUpdating
		r.Status().Update(ctx, privateAiInst)

		r.Log.Info("change in Secret " + privateAiInst.Spec.PaiSecret.Name + " is detected ")
		foundDeploy, err := r.checkAiDeploymentSet(privateAiInst.Name, privateAiInst.Namespace)
		if err == nil {
			err = aicommons.UpdateRestartedAtAnnotation(r, privateAiInst, r.Client, r.Config, foundDeploy, ctx, req, r.Log)
			if err != nil {
				privateAiInst.Status.Status = privateaiv4.StatusError
				r.Status().Update(context.Background(), privateAiInst)
				r.Log.Info("Error occurred while rolling out the deployments after detecting secrets change")
				return ctrl.Result{}, err
			}
		}
		privateAiInst.Status.PaiSecret.ResourceVersion = rversion
	} else {
		privateAiInst.Status.PaiSecret.ResourceVersion = rversion
	}
	privateAiInst.Status.PaiSecret.ApiKey = privateAiInst.Spec.PaiSecret.Name
	privateAiInst.Status.PaiSecret.Certpem = privateAiInst.Spec.PaiSecret.Name
	r.Status().Update(ctx, privateAiInst)
	return ctrl.Result{}, nil
}

// ensurePVCs ensures that all required PersistentVolumeClaims (PVCs) for the PrivateAi instance are created and configured.
// It reconciles the desired PVC state with the actual state in the cluster.
// ctx is the context for the operation, allowing for cancellation and timeouts.
// privateAiInst is the PrivateAi custom resource instance for which PVCs need to be ensured.
// Returns a ctrl.Result indicating the reconciliation result and any error that occurred during PVC creation or validation.
func (r *PrivateAiReconciler) ensurePVCs(ctx context.Context, privateAiInst *privateaiv4.PrivateAi) (ctrl.Result, error) {
	claims := aicommons.VolumeClaimTemplatesForPrivateAi(privateAiInst)

	for i := 0; i < len(claims); i++ {
		result, err := r.createPvc(privateAiInst, &claims[i])
		if err != nil {
			result = resultNq
			return result, err
		}
	}
	return ctrl.Result{}, nil
}

// ensureServices ensures that the required Kubernetes Services for PrivateAI are created and configured.
// It creates an internal Service for the PrivateAI instance and optionally creates an external Service
// if externalSvcEnabled is true. Both services are configured with controller references to establish
// ownership relationships.
//
// Parameters:
//   - ctx: Context for API operations and cancellation
//   - instance: The PrivateAi custom resource instance for which services are being ensured
//   - externalSvcEnabled: Boolean flag to determine whether to create an external-facing service
//
// Returns:
//   - error: Returns an error if service creation, retrieval, or controller reference configuration fails;
//     nil on success
func (r *PrivateAiReconciler) ensureServices(ctx context.Context, privateAiInst *privateaiv4.PrivateAi, svcType string) (ctrl.Result, error) {
	// Create internal service
	sSvc := aicommons.BuildServiceDefForPrivateAi(privateAiInst, svcType)
	result, err := r.createService(privateAiInst, sSvc)
	if err != nil {
		result = resultNq
		return result, err
	}
	pexlSvc, err := r.checkAiSvc(aicommons.GetSvcName(privateAiInst.Name, svcType), privateAiInst.Namespace)
	if err == nil {
		_, err = aicommons.UpdateSvcForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, sSvc, pexlSvc, r.Log)
		if err != nil {
			return resultNq, err
		}
	}
	if svcType == "external" {
		if err == nil && len(sSvc.Status.LoadBalancer.Ingress) > 0 {
			ingress := sSvc.Status.LoadBalancer.Ingress[0]

			lb := ingress.IP
			if lb == "" {
				lb = ingress.Hostname
			}

			if privateAiInst.Status.LoadBalancerIP != lb {
				privateAiInst.Status.LoadBalancerIP = lb
				_ = r.Status().Update(ctx, privateAiInst)
			}
		}
	}
	return ctrl.Result{}, nil
}
func parseBoolFlag(flag string) bool {
	val, err := strconv.ParseBool(flag)
	if err != nil {
		return false
	}
	return val
}
