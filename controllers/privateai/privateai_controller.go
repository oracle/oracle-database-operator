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

//nolint:staticcheck,revive // compatibility-oriented controller signatures and requeue patterns are intentionally retained.
package privateai

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	k8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	sharedk8sutil "github.com/oracle/oracle-database-operator/commons/k8sutil"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
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

const PrivateAiFinalizer = "privateai.oracle.com/privateaifinalizer"

const (
	phaseInit         = "InitFetch"
	phaseDeletion     = "Deletion"
	phaseValidation   = "Validation"
	phaseDependencies = "Dependencies"
	phaseWorkloadSync = "WorkloadSync"
	phaseFinalize     = "StatusFinalize"
)

type reconcileState struct {
	phase         string
	completed     bool
	blocked       bool
	updateLock    bool
	updateLockMsg string
}

// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=configmaps;secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=network.oracle.com,resources=trafficmanagers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PrivateAiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	state := &reconcileState{phase: phaseInit}

	privateAiInst := &privateaiv4.PrivateAi{}
	if err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, privateAiInst); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found")
			return requeueN, nil
		}
		log.Error(err, "failed to fetch PrivateAi")
		return requeueY, err
	}

	defer func() {
		if statusErr := r.updateReconcileStatus(privateAiInst, ctx, result, err, state); statusErr != nil {
			log.Error(statusErr, "failed to update reconcile status")
			if err == nil {
				err = statusErr
				result = requeueY
			}
		}
	}()

	if result, err = r.runPhase(ctx, log, state, phaseDeletion, func() (ctrl.Result, error) {
		inProgress, delErr := r.managePrivateAiDeletion(ctx, privateAiInst)
		if delErr != nil {
			return resultNq, delErr
		}
		if inProgress {
			state.blocked = true
			log.Info("deletion in progress")
			return requeueN, nil
		}
		return requeueN, nil
	}); err != nil || result.Requeue {
		return result, err
	}

	if locked, lockMsg, lockErr := r.shouldBlockForUpdateLock(privateAiInst); lockErr != nil {
		return resultNq, lockErr
	} else if locked {
		state.blocked = true
		log.Info("reconcile blocked by controller update lock", "reason", lockpolicy.DefaultUpdateLockReason, "message", lockMsg)
		return requeueY, nil
	}

	if privateAiInst.Status.Status == "" {
		privateAiInst.Status.Status = privateaiv4.StatusPending
		privateAiInst.Status.Replicas = 0
		privateAiInst.Status.ReleaseUpdate = privateaiv4.ValueUnavailable
	}
	if privateAiInst.Spec.TrafficManager != nil && strings.TrimSpace(privateAiInst.Spec.TrafficManager.Ref) != "" {
		privateAiInst.Status.Mode = "traffic-managed"
		privateAiInst.Status.TrafficManager.Ref = strings.TrimSpace(privateAiInst.Spec.TrafficManager.Ref)
		privateAiInst.Status.TrafficManager.RoutePath = resolvedTrafficManagerRoutePath(privateAiInst)
		r.populateTrafficManagerAccessStatus(ctx, req.Namespace, privateAiInst)
	} else {
		privateAiInst.Status.Mode = "direct"
		privateAiInst.Status.TrafficManager = privateaiv4.TrafficManagerRefStatus{}
	}
	if privateAiInst.Spec.Logging != nil {
		privateAiInst.Status.Logging.Enabled = privateAiInst.Spec.Logging.Enabled
		privateAiInst.Status.Logging.SidecarImage = privateAiInst.Spec.Logging.SidecarImage
	} else {
		privateAiInst.Status.Logging = privateaiv4.LoggingStatus{}
	}

	if result, err = r.runPhase(ctx, log, state, phaseValidation, func() (ctrl.Result, error) {
		return r.validate(privateAiInst, ctx, req)
	}); err != nil || result.Requeue {
		return result, err
	}

	if result, err = r.runPhase(ctx, log, state, phaseDependencies, func() (ctrl.Result, error) {
		return r.reconcileDependencies(ctx, req, privateAiInst)
	}); err != nil || result.Requeue {
		return result, err
	}

	if result, err = r.runPhase(ctx, log, state, phaseWorkloadSync, func() (ctrl.Result, error) {
		return r.reconcileWorkload(ctx, req, privateAiInst, state)
	}); err != nil || result.Requeue {
		return result, err
	}

	state.completed = true
	_, _ = r.runPhase(ctx, log, state, phaseFinalize, func() (ctrl.Result, error) {
		return requeueN, nil
	})
	log.Info("reconcile completed")
	return requeueN, nil
}

// #############################################################################
//
//	Update each reconcile condtion/status
//
// #############################################################################
func (r *PrivateAiReconciler) updateReconcileStatus(
	m *privateaiv4.PrivateAi,
	ctx context.Context,
	result ctrl.Result,
	recErr error,
	state *reconcileState,
) error {
	m.Status.Replicas = int(m.Spec.Replicas)
	rolloutInProgress := false
	if recErr == nil {
		if deploy, err := r.getDeployment(ctx, m.Name, m.Namespace); err == nil {
			rolloutInProgress = deploymentRolloutInProgress(m, deploy)
		}
	}
	if rolloutInProgress {
		m.Status.Status = privateaiv4.StatusUpdating
	} else if state.completed {
		m.Status.Status = privateaiv4.StatusReady
		m.Status.ReleaseUpdate = "V2.0"
	}
	if recErr != nil {
		m.Status.Status = privateaiv4.StatusError
	}

	errMsg := "no reconcile errors"
	if recErr != nil {
		errMsg = recErr.Error()
	}

	var condition metav1.Condition
	hasPrimaryCondition := true
	switch {
	case state.completed:
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileCompelete,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileCompleteReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	case state.blocked:
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileBlocked,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileBlockedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	case result.Requeue:
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileQueued,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileQueuedReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	case recErr != nil:
		condition = metav1.Condition{
			Type:               privateaiv4.ReconcileError,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: m.GetGeneration(),
			Reason:             privateaiv4.ReconcileErrorReason,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	default:
		hasPrimaryCondition = false
	}
	if hasPrimaryCondition {
		meta.RemoveStatusCondition(&m.Status.Conditions, condition.Type)
		meta.SetStatusCondition(&m.Status.Conditions, condition)
	}

	// Maintain a controller update-lock condition for rollout-impacting changes.
	// The lock blocks newer generations unless break-glass override is enabled.
	lockConditionChanged := false
	if state.updateLock {
		lockCond := metav1.Condition{
			Type:               lockpolicy.DefaultReconcilingConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             lockpolicy.DefaultUpdateLockReason,
			ObservedGeneration: m.GetGeneration(),
			LastTransitionTime: metav1.Now(),
			Message:            state.updateLockMsg,
		}
		meta.RemoveStatusCondition(&m.Status.Conditions, lockCond.Type)
		meta.SetStatusCondition(&m.Status.Conditions, lockCond)
		lockConditionChanged = true
	} else if existing := lockpolicy.FindStatusCondition(m.Status.Conditions, lockpolicy.DefaultReconcilingConditionType); existing != nil && existing.Status == metav1.ConditionTrue && (state.completed || recErr != nil) {
		releaseCond := metav1.Condition{
			Type:               lockpolicy.DefaultReconcilingConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "UpdateSettled",
			ObservedGeneration: m.GetGeneration(),
			LastTransitionTime: metav1.Now(),
			Message:            "controller update lock released",
		}
		meta.RemoveStatusCondition(&m.Status.Conditions, releaseCond.Type)
		meta.SetStatusCondition(&m.Status.Conditions, releaseCond)
		lockConditionChanged = true
	}

	if !hasPrimaryCondition && !lockConditionChanged {
		return nil
	}

	return r.Status().Update(ctx, m)
}

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *PrivateAiReconciler) validate(m *privateaiv4.PrivateAi, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("Entering reconcile validation")
	if m.Spec.TrafficManager != nil && strings.TrimSpace(m.Spec.TrafficManager.Ref) != "" {
		trafficManager := &networkv4.TrafficManager{}
		ref := strings.TrimSpace(m.Spec.TrafficManager.Ref)
		if err := r.Client.Get(ctx, types.NamespacedName{Name: ref, Namespace: req.Namespace}, trafficManager); err != nil {
			return resultNq, err
		}
		if trafficManager.Spec.Type != networkv4.TrafficManagerTypeNginx {
			return resultNq, fmt.Errorf("spec.trafficManager.ref %q points to unsupported TrafficManager type %q", ref, trafficManager.Spec.Type)
		}
		if path := strings.TrimSpace(m.Spec.TrafficManager.RoutePath); path != "" &&
			(!strings.HasPrefix(path, "/") || !strings.HasSuffix(path, "/")) {
			return resultNq, fmt.Errorf("spec.trafficManager.routePath must start and end with '/'")
		}
	}

	r.Log.Info("Completed reconcile validation")

	return requeueN, nil

}

func (r *PrivateAiReconciler) populateTrafficManagerAccessStatus(ctx context.Context, namespace string, inst *privateaiv4.PrivateAi) {
	ref := strings.TrimSpace(inst.Spec.TrafficManager.Ref)
	if ref == "" {
		inst.Status.TrafficManager.ServiceName = ""
		inst.Status.TrafficManager.Endpoint = ""
		inst.Status.TrafficManager.PublicURL = ""
		return
	}

	trafficManager := &networkv4.TrafficManager{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: ref, Namespace: namespace}, trafficManager); err != nil {
		inst.Status.TrafficManager.ServiceName = ""
		inst.Status.TrafficManager.Endpoint = ""
		inst.Status.TrafficManager.PublicURL = ""
		return
	}

	serviceName := strings.TrimSpace(trafficManager.Status.ExternalService)
	if serviceName == "" {
		serviceName = strings.TrimSpace(trafficManager.Status.InternalService)
	}
	inst.Status.TrafficManager.ServiceName = serviceName
	inst.Status.TrafficManager.Endpoint = normalizedTrafficManagerEndpoint(trafficManager)
	inst.Status.TrafficManager.PublicURL = ""
	if inst.Status.TrafficManager.Endpoint != "" {
		inst.Status.TrafficManager.PublicURL = strings.TrimRight(inst.Status.TrafficManager.Endpoint, "/") + resolvedTrafficManagerRoutePath(inst)
	}
}

func resolvedTrafficManagerRoutePath(inst *privateaiv4.PrivateAi) string {
	if inst.Spec.TrafficManager != nil {
		if path := strings.TrimSpace(inst.Spec.TrafficManager.RoutePath); path != "" {
			return path
		}
	}
	return fmt.Sprintf("/%s/v1/", strings.ToLower(strings.TrimSpace(inst.Name)))
}

func normalizedTrafficManagerEndpoint(trafficManager *networkv4.TrafficManager) string {
	endpoint := strings.TrimSpace(trafficManager.Status.ExternalEndpoint)
	if endpoint == "" {
		return ""
	}
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	scheme := "http"
	if trafficManager.Spec.Security.TLS.Enabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, endpoint)
}

func (r *PrivateAiReconciler) runPhase(
	ctx context.Context,
	log logr.Logger,
	state *reconcileState,
	phase string,
	fn func() (ctrl.Result, error),
) (ctrl.Result, error) {
	state.phase = phase
	phaseLog := log.WithValues("phase", phase)
	start := time.Now()
	phaseLog.Info("phase started")
	result, err := fn()
	if err != nil {
		phaseLog.Error(err, "phase failed", "duration", time.Since(start).String())
		return result, err
	}
	phaseLog.Info("phase completed", "duration", time.Since(start).String(), "requeue", result.Requeue, "requeueAfter", result.RequeueAfter.String())
	return result, nil
}

func (r *PrivateAiReconciler) reconcileDependencies(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi) (ctrl.Result, error) {
	authEnabled := parseBoolFlag(privateAiInst.Spec.PaiEnableAuthentication)
	storageEnabled := privateAiInst.Spec.StorageClass != ""
	trafficManaged := privateAiInst.Spec.TrafficManager != nil && strings.TrimSpace(privateAiInst.Spec.TrafficManager.Ref) != ""
	externalSvcEnabled := aicommons.ExternalServiceEnabledForPrivateAI(privateAiInst) && !trafficManaged

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
	if storageEnabled {
		if _, err := r.ensurePVCs(ctx, privateAiInst); err != nil {
			return resultNq, err
		}
	}
	if _, err := r.ensureServices(ctx, privateAiInst, "local"); err != nil {
		return resultNq, err
	}
	if externalSvcEnabled {
		if _, err := r.ensureServices(ctx, privateAiInst, "external"); err != nil {
			return resultNq, err
		}
	} else {
		privateAiInst.Status.ExternalService = ""
		if err := r.deleteServiceIfExists(ctx, privateAiInst.Namespace, aicommons.GetSvcName(privateAiInst.Name, "external")); err != nil {
			return resultNq, err
		}
	}
	return requeueN, nil
}

func (r *PrivateAiReconciler) reconcileWorkload(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi, state *reconcileState) (ctrl.Result, error) {
	desiredDeploy := aicommons.BuildDeploySetForPrivateAI(privateAiInst)
	r.waitForScheme()
	if err := controllerutil.SetControllerReference(privateAiInst, desiredDeploy, r.Scheme); err != nil {
		return resultNq, err
	}
	foundDeploy, depResult, err := k8sobjects.ReconcileDeployment(ctx, r.Client, privateAiInst.Namespace, desiredDeploy, nil)
	if err != nil {
		return resultNq, err
	}
	if depResult.Created {
		return requeueY, nil
	}
	if changed, msg := requiresRolloutUpdate(privateAiInst, foundDeploy); changed {
		state.updateLock = true
		state.updateLockMsg = msg
	}

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{client.InNamespace(privateAiInst.Namespace)}
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

	if _, err := aicommons.ManageReplicas(ctx, r, privateAiInst, r.Client, r.Config, foundDeploy, podList, req, r.Log); err != nil {
		return resultNq, err
	}

	if firstPod != nil {
		podIP := firstPod.Status.PodIP
		hostIP := firstPod.Status.HostIP
		if privateAiInst.Status.PodIP != podIP || privateAiInst.Status.NodeIP != hostIP {
			privateAiInst.Status.PodIP = podIP
			privateAiInst.Status.NodeIP = hostIP
		}
	}
	if _, err := aicommons.UpdateDeploySetForPrivateAI(privateAiInst, privateAiInst.Spec, r.Client, r.Config, foundDeploy, firstPod, r.Log); err != nil {
		return resultNq, err
	}
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
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.privateAiRequestsForSecret(ctx, obj)
		})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *PrivateAiReconciler) privateAiRequestsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	list := &privateaiv4.PrivateAiList{}
	if err := r.List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.PaiSecret == nil || strings.TrimSpace(item.Spec.PaiSecret.Name) != secret.Name {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace},
		})
	}

	return requests
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
func (r *PrivateAiReconciler) managePrivateAiDeletion(ctx context.Context, instance *privateaiv4.PrivateAi) (bool, error) {

	isPrivateToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isPrivateToBeDeleted {
		if controllerutil.ContainsFinalizer(instance, PrivateAiFinalizer) {
			// Run finalization logic for finalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizePrivateAi(ctx, instance); err != nil {
				return true, err
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			if err := sharedk8sutil.RemoveFinalizerAndPatch(r.Client, instance, PrivateAiFinalizer); err != nil {
				return true, err
			}
		}
		// Deletion is in progress; there is nothing else to reconcile for this cycle.
		return true, nil
	}

	// Add finalizer for this CR
	if instance.DeletionTimestamp == nil {
		if !controllerutil.ContainsFinalizer(instance, PrivateAiFinalizer) {
			if err := sharedk8sutil.AddFinalizerAndPatch(r.Client, instance, PrivateAiFinalizer); err != nil {
				return false, err
			}
		}
	}

	return false, nil
}

func (r *PrivateAiReconciler) finalizePrivateAi(ctx context.Context, instance *privateaiv4.PrivateAi) error {
	deploymentNames := []string{instance.Name}
	for _, depName := range deploymentNames {
		if depSet, err := r.getDeployment(ctx, depName, instance.Namespace); err == nil {
			if err := r.Client.Delete(ctx, depSet); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		} else if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if instance.Spec.IsDeleteOraPvc {
		claims := aicommons.VolumeClaimTemplatesForPrivateAi(instance)
		for i := range claims {
			pvc := &corev1.PersistentVolumeClaim{}
			if err := r.Client.Get(ctx, types.NamespacedName{Name: claims[i].Name, Namespace: instance.Namespace}, pvc); err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
				continue
			}
			if err := r.Client.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	serviceNames := []string{
		aicommons.GetSvcName(instance.Name, "local"),
		aicommons.GetSvcName(instance.Name, "external"),
		instance.Name,
		instance.Name + "-svc",
	}
	seen := map[string]struct{}{}
	for _, svcName := range serviceNames {
		if svcName == "" {
			continue
		}
		if _, ok := seen[svcName]; ok {
			continue
		}
		seen[svcName] = struct{}{}

		svc := &corev1.Service{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: svcName, Namespace: instance.Namespace}, svc); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			continue
		}
		if err := r.Client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *PrivateAiReconciler) waitForScheme() {
	for r.Scheme == nil {
		r.Log.Info("Waiting for scheme to be initialized...")
		time.Sleep(5 * time.Second)
	}
}

func (r *PrivateAiReconciler) ensureConfigMap(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi) (reconcile.Result, error) {
	cfg := privateAiInst.Spec.PaiConfigFile
	if cfg.Name == "" || cfg.MountLocation == "" {
		return ctrl.Result{}, nil
	}

	_ = aicommons.PatchConfigMap(cfg.Name, privateAiInst, r.Client, r.Log)
	privateAiInst.Status.PaiConfigMap.Name = cfg.Name

	currentVersion := privateAiInst.Status.PaiConfigMap.ResourceVersion
	latestVersion := aicommons.GetConfigMapResourceVersion(cfg.Name, privateAiInst, r.Client, r.Log)

	if latestVersion == "" || latestVersion == "None" {
		privateAiInst.Status.PaiConfigMap.ResourceVersion = latestVersion
		return ctrl.Result{}, nil
	}

	if currentVersion == "" {
		privateAiInst.Status.PaiConfigMap.ResourceVersion = latestVersion
		return ctrl.Result{}, nil
	}

	if currentVersion == latestVersion {
		return ctrl.Result{}, nil
	}

	privateAiInst.Status.Status = privateaiv4.StatusUpdating

	r.Log.Info("ConfigMap change detected", "configMap", cfg.Name)

	if deploy, err := r.getDeployment(ctx, privateAiInst.Name, privateAiInst.Namespace); err == nil {
		if err := aicommons.UpdateRestartedAtAnnotation(ctx, r, privateAiInst, r.Client, r.Config, deploy, req, r.Log); err != nil {
			privateAiInst.Status.Status = privateaiv4.StatusError
			r.Log.Info("Error occurred while rolling out the deployments after detecting secrets change")
			return ctrl.Result{}, err
		}
	}

	privateAiInst.Status.PaiConfigMap.ResourceVersion = latestVersion

	return ctrl.Result{}, nil
}

func (r *PrivateAiReconciler) ensureSecret(ctx context.Context, req ctrl.Request, privateAiInst *privateaiv4.PrivateAi) (reconcile.Result, error) {
	secretName := privateAiInst.Spec.PaiSecret.Name
	if secretName == "" {
		return requeueN, fmt.Errorf("PaiAuthentication is enabled but no PaiSecret is defined")
	}

	apiKey, certPem := aicommons.ReadSecret(secretName, privateAiInst, r.Client, r.Log)
	if apiKey == "" || apiKey == "NONE" {
		return requeueN, fmt.Errorf("PaiAuthentication is enabled but apikey is not found in secret")
	}
	if certPem == "NONE" {
		r.Log.Info("PaiAuthentication is enabled but cert.pem not found in secret")
	}

	_ = aicommons.PatchSecret(secretName, privateAiInst, r.Client, r.Log)

	privateAiInst.Status.PaiSecret.Name = secretName
	privateAiInst.Status.PaiSecret.HasAPIKey = apiKey != "" && apiKey != "NONE"
	privateAiInst.Status.PaiSecret.HasCertPem = certPem != "" && certPem != "NONE"
	// Keep legacy fields populated for backward compatibility with existing consumers.
	privateAiInst.Status.PaiSecret.APIKey = secretName
	privateAiInst.Status.PaiSecret.Certpem = secretName

	currentVersion := privateAiInst.Status.PaiSecret.ResourceVersion
	latestVersion := aicommons.GetSecretResourceVersion(secretName, privateAiInst, r.Client, r.Log)

	if currentVersion == "" {
		privateAiInst.Status.PaiSecret.ResourceVersion = latestVersion
		return ctrl.Result{}, nil
	}

	if currentVersion == latestVersion {
		return ctrl.Result{}, nil
	}

	privateAiInst.Status.Status = privateaiv4.StatusUpdating

	r.Log.Info("Secret change detected", "secret", secretName)

	if deploy, err := r.getDeployment(ctx, privateAiInst.Name, privateAiInst.Namespace); err == nil {
		if err := aicommons.UpdateRestartedAtAnnotation(ctx, r, privateAiInst, r.Client, r.Config, deploy, req, r.Log); err != nil {
			privateAiInst.Status.Status = privateaiv4.StatusError
			r.Log.Info("Error occurred while rolling out the deployments after detecting secrets change")
			return ctrl.Result{}, err
		}
	}

	privateAiInst.Status.PaiSecret.ResourceVersion = latestVersion

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
		claim := &claims[i]
		r.waitForScheme()
		if err := controllerutil.SetControllerReference(privateAiInst, claim, r.Scheme); err != nil {
			return resultNq, err
		}
		_, _, err := k8sobjects.EnsurePersistentVolumeClaim(ctx, r.Client, claim)
		if err != nil {
			return resultNq, err
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
	r.waitForScheme()
	if err := controllerutil.SetControllerReference(privateAiInst, sSvc, r.Scheme); err != nil {
		return resultNq, err
	}
	_, err := k8sobjects.EnsureService(ctx, r.Client, privateAiInst.Namespace, sSvc, k8sobjects.ServiceSyncOptions{
		NodePortMerge:             k8sobjects.NodePortMergeByNamePortAndProtocol,
		SyncOwnerReferences:       true,
		SyncSessionAffinityCfg:    true,
		SyncPublishNotReady:       true,
		SyncInternalTrafficPolicy: true,
		SyncLoadBalancerFields:    true,
		SyncHealthCheckNodePort:   true,
	})
	if err != nil {
		return resultNq, err
	}
	pexlSvc, err := aicommons.CheckSvc(aicommons.GetSvcName(privateAiInst.Name, svcType), privateAiInst, r.Client)
	if err != nil {
		return resultNq, err
	}
	if svcType == "external" {
		privateAiInst.Status.ExternalService = pexlSvc.Name
		if len(pexlSvc.Status.LoadBalancer.Ingress) > 0 {
			ingress := pexlSvc.Status.LoadBalancer.Ingress[0]

			lb := ingress.IP
			if lb == "" {
				lb = ingress.Hostname
			}

			if privateAiInst.Status.LoadBalancerIP != lb {
				privateAiInst.Status.LoadBalancerIP = lb
			}
		}
		privateAiInst.Status.ClusterIP = ""
	} else if svcType == "local" {
		privateAiInst.Status.LocalService = pexlSvc.Name
		privateAiInst.Status.ClusterIP = pexlSvc.Spec.ClusterIP
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

func (r *PrivateAiReconciler) deleteServiceIfExists(ctx context.Context, namespace, name string) error {
	if name == "" {
		return nil
	}
	svc := &corev1.Service{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *PrivateAiReconciler) getDeployment(ctx context.Context, name, namespace string) (*appsv1.Deployment, error) {
	found := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found); err != nil {
		return found, err
	}
	return found, nil
}

func (r *PrivateAiReconciler) shouldBlockForUpdateLock(inst *privateaiv4.PrivateAi) (bool, string, error) {
	locked, lockGen, lockMsg := lockpolicy.IsControllerUpdateLocked(
		inst.Status.Conditions,
		lockpolicy.DefaultReconcilingConditionType,
		lockpolicy.DefaultUpdateLockReason,
	)
	if !locked {
		return false, "", nil
	}
	// Same generation can continue to reconcile and finish the in-flight update.
	if inst.Generation <= lockGen {
		return false, "", nil
	}

	overrideEnabled, overrideMsg := lockpolicy.IsUpdateLockOverrideEnabled(inst.GetAnnotations(), lockpolicy.DefaultOverrideAnnotation)
	if overrideEnabled {
		r.Log.Info("update lock override accepted", "annotation", lockpolicy.DefaultOverrideAnnotation, "message", overrideMsg)
		return false, "", nil
	}

	msg := fmt.Sprintf("previous update in progress at generation %d. %s", lockGen, lockMsg)
	return true, msg, nil
}

func requiresRolloutUpdate(inst *privateaiv4.PrivateAi, foundDeploy *appsv1.Deployment) (bool, string) {
	if foundDeploy == nil {
		return false, ""
	}
	containerName := foundDeploy.Name
	var current *corev1.Container
	for i := range foundDeploy.Spec.Template.Spec.Containers {
		if foundDeploy.Spec.Template.Spec.Containers[i].Name == containerName {
			current = &foundDeploy.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if current == nil {
		return false, ""
	}

	if desired := inst.Spec.PaiImage; desired != "" && desired != current.Image {
		return true, "controller update lock: image rollout in progress"
	}
	if desired := inst.Spec.Resources; desired != nil && !reflect.DeepEqual(current.Resources, *desired) {
		return true, "controller update lock: resource rollout in progress"
	}
	return false, ""
}

func deploymentRolloutInProgress(inst *privateaiv4.PrivateAi, deploy *appsv1.Deployment) bool {
	if deploy == nil {
		return false
	}
	desired := desiredPrivateAIReplicas(inst)
	if deploy.Status.ObservedGeneration < deploy.Generation {
		return true
	}
	if deploy.Status.UpdatedReplicas < desired {
		return true
	}
	if deploy.Status.ReadyReplicas < desired {
		return true
	}
	if deploy.Status.AvailableReplicas < desired {
		return true
	}
	return false
}

func desiredPrivateAIReplicas(inst *privateaiv4.PrivateAi) int32 {
	if inst == nil || inst.Spec.Replicas <= 0 {
		return 1
	}
	return inst.Spec.Replicas
}
