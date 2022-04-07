/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v63/common"
	"github.com/oracle/oci-go-sdk/v63/database"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/annotations"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousContainerDatabaseReconciler reconciles a AutonomousContainerDatabase object
type AutonomousContainerDatabaseReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	dbService   oci.DatabaseService
	workService oci.WorkRequestService
	lastSucSpec *dbv1alpha1.AutonomousContainerDatabaseSpec
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousContainerDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.AutonomousContainerDatabase{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Complete(r)
}

func (r *AutonomousContainerDatabaseReconciler) isIntermediateState(state database.AutonomousContainerDatabaseLifecycleStateEnum) bool {
	if state == database.AutonomousContainerDatabaseLifecycleStateProvisioning ||
		state == database.AutonomousContainerDatabaseLifecycleStateUpdating ||
		state == database.AutonomousContainerDatabaseLifecycleStateTerminating ||
		state == database.AutonomousContainerDatabaseLifecycleStateBackupInProgress ||
		state == database.AutonomousContainerDatabaseLifecycleStateRestoring ||
		state == database.AutonomousContainerDatabaseLifecycleStateRestarting ||
		state == database.AutonomousContainerDatabaseLifecycleStateMaintenanceInProgress ||
		state == database.AutonomousContainerDatabaseLifecycleStateRoleChangeInProgress {
		return true
	}
	return false
}

func (r *AutonomousContainerDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			desiredACD, acdOk := e.ObjectNew.(*dbv1alpha1.AutonomousContainerDatabase)
			if acdOk {
				oldACD := e.ObjectOld.(*dbv1alpha1.AutonomousContainerDatabase)

				oldSucSpec, oldSucSpecOk := oldACD.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]
				newSucSpec, newSucSpecOk := desiredACD.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]
				sucSpecChanged := oldSucSpecOk != newSucSpecOk || oldSucSpec != newSucSpec

				if !reflect.DeepEqual(oldACD.Status, desiredACD.Status) || sucSpecChanged {
					// Don't enqueue if the status or the lastSucSpec changes
					return false
				}

				return true
			}

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the object is deleted from the cluster.
			_, acdOk := e.Object.(*dbv1alpha1.AutonomousContainerDatabase)
			if acdOk {
				return false
			}

			return true
		},
	}

	return pred
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomouscontainerdatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomouscontainerdatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *AutonomousContainerDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	var err error

	// Get the autonomousdatabase instance from the cluster
	acd := &dbv1alpha1.AutonomousContainerDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, acd); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ACD, so we don't need to update the status
		return emptyResult, err
	}

	r.lastSucSpec, err = acd.GetLastSuccessfulSpec()
	if err != nil {
		return r.manageError(acd, err)
	}

	/******************************************************************
	* Get OCI database client and work request client
	******************************************************************/
	if err := r.setupOCIClients(acd); err != nil {
		logger.Error(err, "Fail to setup OCI clients")

		return r.manageError(acd, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	* Register/unregister finalizer
	* Deletion timestamp will be added to a object before it is deleted.
	* Kubernetes server calls the clean up function if a finalizer exitsts, and won't delete the real object until
	* all the finalizers are removed from the object metadata.
	* Refer to this page for more details of using finalizers: https://kubernetes.io/blog/2021/05/14/using-finalizers-to-control-deletion/
	******************************************************************/
	isACDDeleteTrue, err := r.validateFinalizer(acd)
	if err != nil {
		return r.manageError(acd, err)
	}
	if isACDDeleteTrue {
		return emptyResult, nil
	}

	/******************************************************************
	* Determine which Database operations need to be executed by checking the changes to spec.
	* There are three scenario:
	* 1. provision operation. The ACD OCID is missing, and the LastSucSpec annotation is missing.
	* 2. bind operation. The ACD OCID is provided, but the LastSucSpec annotation is missing.
	* 3. sync operation. The action field is SYNC.
	* 4. update operation. The changes which are not provision, bind or sync operations is an update operation.
	* Afterwards, update the resource from the remote database in OCI. This step will be executed right after
	* the above three cases during every reconcile.
	/******************************************************************/
	// difACD is nil when the action is PROVISION or BIND.
	// Use difACD to identify which fields are updated when it's an UPDATE operation.
	action, difACD, err := r.determineAction(acd)
	if err != nil {
		return r.manageError(acd, err)
	}

	switch action {
	case acdRecActionProvision:
		if err := r.createACD(acd); err != nil {
			return r.manageError(acd, err)
		}
	case acdRecActionBind:
		break
	case acdRecActionUpdate:
		// updateACD contains downloadWallet
		if err := r.updateACD(acd, difACD); err != nil {
			return r.manageError(acd, err)
		}
	case acdRecActionSync:
		break
	}

	/*****************************************************
	*	Sync resource and update the lastSucSpec
	*****************************************************/
	specUpdated, err := r.syncResource(acd)
	if err != nil {
		return r.manageError(acd, err)
	}

	if specUpdated {
		return emptyResult, nil
	}

	logger.Info("AutonomousContainerDatabase reconciles successfully")

	return emptyResult, nil
}

// updateResourceStatus updates only the status of the resource, not including the lastSucSpec.
// This function should not be called by the functions associated with the OCI update requests.
// The OCI update requests should use updateResource() to ensure all the spec, resource and the
// lastSucSpec are updated.
func (r *AutonomousContainerDatabaseReconciler) updateResourceStatus(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curACD := &dbv1alpha1.AutonomousContainerDatabase{}

		namespacedName := types.NamespacedName{
			Namespace: acd.GetNamespace(),
			Name:      acd.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curACD); err != nil {
			return err
		}

		curACD.Status = acd.Status
		return r.KubeClient.Status().Update(context.TODO(), curACD)
	})
}

// updateResource updates the specification, the status of AutonomousContainerDatabase resource
func (r *AutonomousContainerDatabaseReconciler) updateResource(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	// Update the spec
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curACD := &dbv1alpha1.AutonomousContainerDatabase{}

		namespacedName := types.NamespacedName{
			Namespace: acd.GetNamespace(),
			Name:      acd.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curACD); err != nil {
			return err
		}

		curACD.Spec = acd.Spec
		return r.KubeClient.Update(context.TODO(), curACD)
	}); err != nil {
		return err
	}

	return nil
}

// syncResource pulled information from OCI, update the lastSucSpec, and lastly, update the local resource.
// It's important to update the lastSucSpec prior than we update the local resource, because updating
// the local resource triggers the Reconcile, and the Reconcile determines the action by looking if
// the lastSucSpec is present.
// The function returns if the spec is updated (and the Reconcile is triggered).
func (r *AutonomousContainerDatabaseReconciler) syncResource(acd *dbv1alpha1.AutonomousContainerDatabase) (bool, error) {
	// Get the information from OCI
	resp, err := r.dbService.GetAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID, nil)
	if err != nil {
		return false, err
	}

	specChanged, statusChanged := acd.UpdateFromOCIACD(resp.AutonomousContainerDatabase)

	// Validate the status change
	if statusChanged {
		if err := r.updateResourceStatus(acd); err != nil {
			return false, err
		}
	}

	// Validate the spec change
	// If the spec changes, update the lastSucSpec and the resource, and then exit the Reconcile since it triggers another round of the Reconcile.
	if specChanged {
		if err := r.updateLastSuccessfulSpec(acd); err != nil {
			return false, err
		}

		if err := r.updateResource(acd); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

// updateLastSuccessfulSpec updates the lasSucSpec annotation, and returns the ORIGINAL object
// The object will NOT be updated with the returned content from the cluster since we want to
// update only the lasSucSpec. For example: After we get the ACD information from OCI, we want
// to update the lasSucSpec before updating the local resource. In this case, we want to keep
// the content returned from the OCI, not the one from the cluster.
func (r *AutonomousContainerDatabaseReconciler) updateLastSuccessfulSpec(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	specBytes, err := json.Marshal(acd.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		dbv1alpha1.LastSuccessfulSpec: string(specBytes),
	}

	copyACD := acd.DeepCopy()

	return annotations.PatchAnnotations(r.KubeClient, copyACD, anns)
}

func (r *AutonomousContainerDatabaseReconciler) manageError(acd *dbv1alpha1.AutonomousContainerDatabase, issue error) (ctrl.Result, error) {
	// Rollback if lastSucSpec exists
	if r.lastSucSpec != nil {
		// Send event
		r.Recorder.Event(acd, corev1.EventTypeWarning, "ReconcileIncompleted", issue.Error())

		var finalIssue = issue

		// Roll back
		if _, err := r.syncResource(acd); err != nil {
			finalIssue = k8s.CombineErrors(finalIssue, err)
		}

		return emptyResult, finalIssue
	} else {
		// Send event
		r.Recorder.Event(acd, corev1.EventTypeWarning, "CreateFailed", issue.Error())

		return emptyResult, issue
	}
}

func (r *AutonomousContainerDatabaseReconciler) setupOCIClients(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	var err error

	authData := oci.APIKeyAuth{
		ConfigMapName: acd.Spec.OCIConfig.ConfigMapName,
		SecretName:    acd.Spec.OCIConfig.SecretName,
		Namespace:     acd.GetNamespace(),
	}

	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		return err
	}

	r.dbService, err = oci.NewDatabaseService(r.Log, r.KubeClient, provider)
	if err != nil {
		return err
	}

	r.workService, err = oci.NewWorkRequestService(r.Log, r.KubeClient, provider)
	if err != nil {
		return err
	}

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) validateFinalizer(acd *dbv1alpha1.AutonomousContainerDatabase) (isACDDeleteTrue bool, err error) {
	logger := r.Log.WithName("finalizer")

	isACDToBeDeleted := acd.GetDeletionTimestamp() != nil
	if isACDToBeDeleted {
		if controllerutil.ContainsFinalizer(acd, dbv1alpha1.ACDFinalizer) {
			if (r.lastSucSpec != nil && r.lastSucSpec.AutonomousContainerDatabaseOCID != nil) && // lastSucSpec exists and the ACD_OCID isn't nil
				(acd.Status.LifecycleState != database.AutonomousContainerDatabaseLifecycleStateTerminating && acd.Status.LifecycleState != database.AutonomousContainerDatabaseLifecycleStateTerminated) {
				// Run finalization logic for finalizer. If the finalization logic fails, don't remove the finalizer so
				// that we can retry during the next reconciliation.
				logger.Info("Terminating Autonomous Container Database: " + *acd.Spec.DisplayName)
				if err := r.terminateACD(acd); err != nil {
					return false, err
				}
			} else {
				logger.Info("Missing AutonomousContaineratabaseOCID to terminate Autonomous Container Database", "Name", acd.Name, "Namespace", acd.Namespace)
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return false, nil
			}
		}
		// Send true because delete is in progress and it is a custom delete message
		// We don't need to print custom err stack as we are deleting the topology
		return true, nil
	}

	// Delete is not schduled. Update the finalizer for this CR if hardLink is present
	if acd.Spec.HardLink != nil {
		if *acd.Spec.HardLink {
			if err := k8s.AddFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return false, nil
			}
		} else {
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, acd, dbv1alpha1.ACDFinalizer); err != nil {
				return false, nil
			}
		}
	}

	return false, nil
}

type acdRecActionEnum string

const (
	acdRecActionProvision acdRecActionEnum = "PROVISION"
	acdRecActionBind      acdRecActionEnum = "BIND"
	acdRecActionUpdate    acdRecActionEnum = "UPDATE"
	acdRecActionSync      acdRecActionEnum = "SYNC"
)

func (r *AutonomousContainerDatabaseReconciler) determineAction(acd *dbv1alpha1.AutonomousContainerDatabase) (acdRecActionEnum, *dbv1alpha1.AutonomousContainerDatabase, error) {
	if r.lastSucSpec == nil {
		if acd.Spec.AutonomousContainerDatabaseOCID == nil {
			return acdRecActionProvision, nil, nil
		} else {
			return acdRecActionBind, nil, nil
		}
	} else {
		// Pre-process step for the UPDATE. Remove the unchanged fields from spec.details,
		difACD := acd.DeepCopy()
		specChanged, err := difACD.RemoveUnchangedSpec()
		if err != nil {
			return "", nil, err
		}

		if specChanged {
			// Return SYNC if the spec.action is SYNC
			if difACD.Spec.Action == dbv1alpha1.AcdActionSync {
				return acdRecActionSync, nil, nil
			}
			return acdRecActionUpdate, difACD, nil
		}

		return acdRecActionSync, nil, nil
	}
}

func (r *AutonomousContainerDatabaseReconciler) createACD(acd *dbv1alpha1.AutonomousContainerDatabase) error {
	logger := r.Log.WithName("provision-ACD")
	resp, err := r.dbService.CreateAutonomousContainerDatabase(acd)
	if err != nil {
		return err
	}

	// Update the ACD OCID and the status
	// The trick is to update the status first to prevent unwanted reconcile
	acd.Spec.AutonomousContainerDatabaseOCID = resp.AutonomousContainerDatabase.Id
	acd.UpdateStatusFromOCIACD(resp.AutonomousContainerDatabase)

	if err := r.updateResourceStatus(acd); err != nil {
		return err
	}

	// Patching is faster
	if err := k8s.Patch(r.KubeClient, acd, "/spec/autonomousContainerDatabaseOCID", acd.Spec.AutonomousContainerDatabaseOCID); err != nil {
		return err
	}

	// Wait for the provision operation to finish
	if _, err := r.workService.Wait(*resp.OpcWorkRequestId); err != nil {
		return err
	}

	return nil
}

func (r *AutonomousContainerDatabaseReconciler) updateGeneralFields(acd *dbv1alpha1.AutonomousContainerDatabase, difACD *dbv1alpha1.AutonomousContainerDatabase) error {
	if difACD.Spec.DisplayName == nil &&
		difACD.Spec.PatchModel == "" &&
		difACD.Spec.FreeformTags == nil {
		return nil
	}

	resp, err := r.dbService.UpdateAutonomousContainerDatabase(difACD)
	if err != nil {
		return err
	}

	// If the OpcWorkRequestId is nil (such as when the displayName is changed),
	// no need to update the resource and wail until the work is done
	if resp.OpcWorkRequestId == nil {
		return nil
	}

	acd.UpdateStatusFromOCIACD(resp.AutonomousContainerDatabase)
	if err := r.updateResourceStatus(acd); err != nil {
		return err
	}

	if _, err := r.workService.Wait(*resp.OpcWorkRequestId); err != nil {
		return err
	}
	return nil
}

func (r *AutonomousContainerDatabaseReconciler) terminateACD(acd *dbv1alpha1.AutonomousContainerDatabase) error {

	resp, err := r.dbService.TerminateAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
	if err != nil {
		return err
	}

	acd.Status.LifecycleState = database.AutonomousContainerDatabaseLifecycleStateTerminating

	if err := r.updateResourceStatus(acd); err != nil {
		return err
	}

	if _, err := r.workService.Wait(*resp.OpcWorkRequestId); err != nil {
		return err
	}
	return nil
}

func (r *AutonomousContainerDatabaseReconciler) updateLifecycleState(acd *dbv1alpha1.AutonomousContainerDatabase, difACD *dbv1alpha1.AutonomousContainerDatabase) error {
	if difACD.Spec.Action == dbv1alpha1.AcdActionBlank {
		return nil
	}

	var opcWorkRequestId string

	switch difACD.Spec.Action {
	case dbv1alpha1.AcdActionRestart:
		resp, err := r.dbService.RestartAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
		if err != nil {
			return err
		}

		acd.Status.LifecycleState = resp.LifecycleState
		opcWorkRequestId = *resp.OpcWorkRequestId
	case dbv1alpha1.AcdActionTerminate:
		resp, err := r.dbService.TerminateAutonomousContainerDatabase(*acd.Spec.AutonomousContainerDatabaseOCID)
		if err != nil {
			return err
		}

		acd.Status.LifecycleState = database.AutonomousContainerDatabaseLifecycleStateTerminating
		opcWorkRequestId = *resp.OpcWorkRequestId
	default:
		return errors.New("Unknown action")
	}

	// Update the status. The Action field will be erased at the sync operation before exiting the Reconcile.
	if err := r.updateResourceStatus(acd); err != nil {
		return err
	}

	if _, err := r.workService.Wait(opcWorkRequestId); err != nil {
		return err
	}
	return nil
}

func (r *AutonomousContainerDatabaseReconciler) updateACD(acd *dbv1alpha1.AutonomousContainerDatabase, difACD *dbv1alpha1.AutonomousContainerDatabase) error {
	if err := r.updateGeneralFields(acd, difACD); err != nil {
		return err
	}

	if err := r.updateLifecycleState(acd, difACD); err != nil {
		return err
	}

	return nil
}
