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

	// "strconv"
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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PrivateAi object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *PrivateAiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here
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

	// Manage SingleInstanceDatabase Deletion
	err, isPrivateAiDelTrue := r.managePrivateAiDeletion(privateAiInst)
	if err != nil {
		//r.setCrdLifeCycleState(instance, &result, &err, stateType)
		result = resultNq
		if isPrivateAiDelTrue == true {
			err = nil
			return result, err
		} else {
			return result, err
		}
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

	// Validation PaiAuthentication
	pstatus, _ := strconv.ParseBool(privateAiInst.Spec.PaiEnableAuthentication)
	if pstatus {
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
		privateAiInst.Status.PaiSecret.ApiKey = apiKey
		privateAiInst.Status.PaiSecret.Certpem = certPem
		r.Status().Update(ctx, privateAiInst)
	}

	if privateAiInst.Spec.PaiConfigFile != nil {
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
	if privateAiInst.Spec.StorageClass != "" {
		claims := aicommons.VolumeClaimTemplatesForPrivateAi(privateAiInst)
		for i := 0; i < len(claims); i++ {
			result, err = r.createPvc(privateAiInst, &claims[i])
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	}

	// ========================= Service Setup For Catalog===================
	// Following check and loop will make sure  to create the service
	lSvc := aicommons.BuildServiceDefForPrivateAi(privateAiInst, "local")
	result, err = r.createService(privateAiInst, lSvc)
	if err != nil {
		result = resultNq
		return result, err
	}

	plSvc, err := r.checkAiSvc(aicommons.GetSvcName(privateAiInst.Name, "local"), privateAiInst.Namespace)
	if err == nil {
		_, err = aicommons.UpdateSvcForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, lSvc, plSvc, r.Log)
		if err != nil {
			return resultNq, err
		}
	}

	exsvStatus, _ := strconv.ParseBool(privateAiInst.Spec.IsExternalSvc)
	if exsvStatus {
		eSvc := aicommons.BuildServiceDefForPrivateAi(privateAiInst, "external")
		result, err = r.createService(privateAiInst, eSvc)
		if err != nil {
			result = resultNq
			return result, err
		}

		// Service Update Block
		pexlSvc, err := r.checkAiSvc(aicommons.GetSvcName(privateAiInst.Name, "external"), privateAiInst.Namespace)
		if err == nil {
			_, err = aicommons.UpdateSvcForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, eSvc, pexlSvc, r.Log)
			if err != nil {
				return resultNq, err
			}
		}
	}

	// Populate LoadBalancerIP in status from the external service (if enabled)
	if exsvStatus {
		esvc, err := r.checkAiSvc(aicommons.GetSvcName(privateAiInst.Name, "external"), privateAiInst.Namespace)
		if err == nil && len(esvc.Status.LoadBalancer.Ingress) > 0 {
			ingress := esvc.Status.LoadBalancer.Ingress[0]

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


	// See if DeploymentSets already exists and create if it doesn't
	desiredDeploy := aicommons.BuildDeploySetForPrivateAI(privateAiInst)
	foundDeploy, err := r.checkAiDeploymentSet(desiredDeploy.Name, desiredDeploy.Namespace)
	if err != nil && apierrors.IsNotFound(err) {
		// didn't found create New deployment
		result, err = r.deployPrivateAiDeploymentSet(privateAiInst, desiredDeploy)
		if err != nil {
			return resultNq, err
		}

	} else if err == nil {
		//if exists check Pods & run
		podList := &corev1.PodList{}
		err = r.Client.List(ctx, podList,
			client.InNamespace(privateAiInst.Namespace),
			client.MatchingLabels(foundDeploy.Spec.Selector.MatchLabels),
		)
		if err != nil {
			return resultNq, err
		}

		var foundPod *corev1.Pod
		if len(podList.Items) > 0 {
			foundPod = &podList.Items[0]
		} else {
			foundPod = &corev1.Pod{}
		}

		// _, err = aicommons.ManageReplicas(privateAiInst, r.Client, r.Config, foundDeploy, podList, r.Log)
		_, err = aicommons.ManageReplicas(r, privateAiInst, r.Client, r.Config, foundDeploy, podList, ctx, req, r.Log)

		if err != nil {
			return resultNq, err
		}

		// update PodIP / NodeIP in status using the first pod
		if foundPod != nil {
			if privateAiInst.Status.PodIP != foundPod.Status.PodIP || privateAiInst.Status.NodeIP != foundPod.Status.HostIP {
				privateAiInst.Status.PodIP = foundPod.Status.PodIP
				privateAiInst.Status.NodeIP = foundPod.Status.HostIP
				_ = r.Status().Update(ctx, privateAiInst)
			}
		}

		_, err = aicommons.UpdateDeploySetForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, foundDeploy, foundPod, r.Log)
		if err != nil {
			return resultNq, err
		}

	} else {
		return resultNq, err
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
	// See if DeploymentSets already exists and create if it doesn't
	// Error : invalid memory address or nil pointer dereference" (runtime error: invalid memory address or nil pointer dereference)
	// This happens during unit test cases
	for i := 0; i < 5; i++ {
		if r.Scheme == nil {
			time.Sleep(time.Second * 40)
		} else {
			break
		}
	}
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

// ================================== CREATE FUNCTIONS =============================
// This function create a service based isExtern parameter set in the yaml file
func (r *PrivateAiReconciler) createService(instance *privateaiv4.PrivateAi,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	// See if Service already exists and create if it doesn't
	// We are getting error on nil pointer segment when r.scheme is null
	// Error : invalid memory address or nil pointer dereference" (runtime error: invalid memory address or nil pointer dereference)
	// This happens during unit test cases
	for i := 0; i < 5; i++ {
		if r.Scheme == nil {
			time.Sleep(time.Second * 40)
		} else {
			break
		}
	}
	controllerutil.SetControllerReference(instance, dep, r.Scheme)
	_, err := r.checkAiSvc(dep.Name, instance.Namespace)

	jsn, _ := json.Marshal(dep)
	aicommons.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && errors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a service")
		err = r.Client.Create(context.TODO(), dep)
		if err != nil {
			// Service creation failed
			reqLogger.Error(err, "Failed to create Service", "Service.space", dep.Namespace, "Service.Name", dep.Name)
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

// ================================== CREATE FUNCTIONS =============================
// This function create a PVC based isExtern parameter set in the yaml file
func (r *PrivateAiReconciler) createPvc(instance *privateaiv4.PrivateAi,
	dep *corev1.PersistentVolumeClaim,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	// See if Service already exists and create if it doesn't
	// We are getting error on nil pointer segment when r.scheme is null
	// Error : invalid memory address or nil pointer dereference" (runtime error: invalid memory address or nil pointer dereference)
	// This happens during unit test cases
	for i := 0; i < 5; i++ {
		if r.Scheme == nil {
			time.Sleep(time.Second * 40)
		} else {
			break
		}
	}
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

// FInalizer

// ================== Function to check insytance deletion timestamp and activate the finalizer code ========
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

// ========================== FInalizer Section ===================
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
	// TODO(user): Add the cleanup steps that the operator needs to do before the CR
	// can be deleted. Examples of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	var err error
	var pvcName string

	//r.checkProvInstance(instance)
	depSetFound := &appsv1.Deployment{}
	svcFound := &corev1.Service{}
	depSetFound, err = aicommons.CheckDepSet(instance, r.Client)
	if err == nil {
		// See if StatefulSets already exists and create if it doesn't
		err = r.Client.Delete(context.Background(), depSetFound)
		if err != nil {
			return err
		}
		if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
			pvcName = instance.Name + "-oradata-vol4-" + instance.Name + "-0"
			err = aicommons.DelPvc(pvcName, instance, r.Client, r.Log)
			if err != nil {
				return err
			}
		}

		exsvStatus, _ := strconv.ParseBool(instance.Spec.IsExternalSvc)
		if exsvStatus {
			// svcFound, err = aicommons.CheckSvc(instance.Name+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
			svcFound, err = aicommons.CheckSvc(instance.Name+"-svc", instance, r.Client)

			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), svcFound)
				if err != nil {
					return err
				}
			}
		}
		svcFound, err = aicommons.CheckSvc(instance.Name, instance, r.Client)
		if err == nil {
			// See if StatefulSets already exists and create if it doesn't
			err = r.Client.Delete(context.Background(), svcFound)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
