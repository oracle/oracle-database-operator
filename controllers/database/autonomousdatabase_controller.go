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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/database"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/annotations"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

var requeueResult ctrl.Result = ctrl.Result{Requeue: true, RequeueAfter: 15 * time.Second}
var emptyResult ctrl.Result = ctrl.Result{}

// *AutonomousDatabaseReconciler reconciles a AutonomousDatabase object
type AutonomousDatabaseReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	dbService oci.DatabaseService
}

// SetupWithManager function
func (r *AutonomousDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.AutonomousDatabase{}).
		Watches(
			&dbv1alpha1.AutonomousDatabaseBackup{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueMapFn),
		).
		Watches(
			&dbv1alpha1.AutonomousDatabaseRestore{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueMapFn),
		).
		WithEventFilter(predicate.And(r.eventFilterPredicate(), r.watchPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
func (r *AutonomousDatabaseReconciler) enqueueMapFn(ctx context.Context, o client.Object) []reconcile.Request {
	reqs := make([]reconcile.Request, len(o.GetOwnerReferences()))

	for _, owner := range o.GetOwnerReferences() {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      owner.Name,
				Namespace: o.GetNamespace(),
			},
		})
	}

	return reqs
}

func (r *AutonomousDatabaseReconciler) watchPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, backupOk := e.Object.(*dbv1alpha1.AutonomousDatabaseBackup)
			_, restoreOk := e.Object.(*dbv1alpha1.AutonomousDatabaseRestore)
			// Don't enqueue if the event is from Backup or Restore
			return !(backupOk || restoreOk)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Enqueue the update event only when the status changes the first time
			desiredBackup, backupOk := e.ObjectNew.(*dbv1alpha1.AutonomousDatabaseBackup)
			if backupOk {
				oldBackup := e.ObjectOld.(*dbv1alpha1.AutonomousDatabaseBackup)
				return oldBackup.Status.LifecycleState == "" && desiredBackup.Status.LifecycleState != ""
			}

			desiredRestore, restoreOk := e.ObjectNew.(*dbv1alpha1.AutonomousDatabaseRestore)
			if restoreOk {
				oldRestore := e.ObjectOld.(*dbv1alpha1.AutonomousDatabaseRestore)
				return oldRestore.Status.Status == "" && desiredRestore.Status.Status != ""
			}

			// Enqueue if the event is not from Backup or Restore
			return true
		},
	}
}

func (r *AutonomousDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// source object can be AutonomousDatabase, AutonomousDatabaseBackup, or AutonomousDatabaseRestore
			desiredADB, adbOk := e.ObjectNew.(*dbv1alpha1.AutonomousDatabase)
			if adbOk {
				oldADB := e.ObjectOld.(*dbv1alpha1.AutonomousDatabase)

				specChanged := !reflect.DeepEqual(oldADB.Spec, desiredADB.Spec)
				statusChanged := !reflect.DeepEqual(oldADB.Status, desiredADB.Status)

				oldLastSucSpec := oldADB.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]
				desiredLastSucSpec := desiredADB.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]
				lastSucSpecChanged := oldLastSucSpec != desiredLastSucSpec

				if (!specChanged && statusChanged) || lastSucSpecChanged ||
					(controllerutil.ContainsFinalizer(oldADB, dbv1alpha1.ADB_FINALIZER) != controllerutil.ContainsFinalizer(desiredADB, dbv1alpha1.ADB_FINALIZER)) {
					// Don't enqueue in the folowing condition:
					// 1. only status changes 2. lastSucSpec changes 3. ADB_FINALIZER changes
					return false
				}

				return true
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the object is deleted from the cluster.
			_, adbOk := e.Object.(*dbv1alpha1.AutonomousDatabase)
			return !adbOk
		},
	}
}

// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases/status,verbs=update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabasebackups,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaserestores,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomouscontainerdatabases,verbs=get;list
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the funtion that the operator calls every time when the reconciliation loop is triggered.
// It go to the beggining of the reconcile if an error is returned. We won't return a error if it is related
// to OCI, because the issues cannot be solved by re-run the reconcile.
func (r *AutonomousDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespace/Name", req.NamespacedName)

	var err error
	var ociADB *dbv1alpha1.AutonomousDatabase

	// Get the autonomousdatabase instance from the cluster
	desiredADB := &dbv1alpha1.AutonomousDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, desiredADB); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ADB, so we don't need to update the status
		return emptyResult, err
	}

	/******************************************************************
	* Get OCI database client
	******************************************************************/
	if err := r.setupOCIClients(logger, desiredADB); err != nil {
		logger.Error(err, "Fail to setup OCI clients")

		return r.manageError(logger.WithName("setupOCIClients"), desiredADB, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	* Cleanup the resource if the resource is to be deleted.
	* Deletion timestamp will be added to a object before it is deleted.
	* Kubernetes server calls the clean up function if a finalizer exitsts, and won't delete the real object until
	* all the finalizers are removed from the object metadata.
	* Refer to this page for more details of using finalizers: https://kubernetes.io/blog/2022/05/14/using-finalizers-to-control-deletion/
	******************************************************************/
	exitReconcile, err := r.validateCleanup(logger, desiredADB)
	if err != nil {
		return r.manageError(logger.WithName("validateCleanup"), desiredADB, err)
	}

	if exitReconcile {
		return emptyResult, nil
	}

	/******************************************************************
	* Register/unregister the finalizer
	******************************************************************/
	exit, err := r.validateFinalizer(logger, desiredADB)
	if err != nil {
		return r.manageError(logger.WithName("validateFinalizer"), desiredADB, err)
	}

	if exit {
		return emptyResult, nil
	}

	/******************************************************************
	* Validate operations
	******************************************************************/
	modifiedADB := desiredADB.DeepCopy() // the ADB which stores the changes
	exitReconcile, result, err := r.validateOperation(logger, modifiedADB, ociADB)
	if err != nil {
		return r.manageError(logger.WithName("validateOperation"), modifiedADB, err)
	}
	if exitReconcile {
		return result, nil
	}

	/*****************************************************
	*	Sync AutonomousDatabase Backups from OCI
	*****************************************************/
	if err := r.syncBackupResources(logger, modifiedADB); err != nil {
		return r.manageError(logger.WithName("syncBackupResources"), modifiedADB, err)
	}

	/*****************************************************
	*	Validate Wallet
	*****************************************************/
	if err := r.validateWallet(logger, modifiedADB); err != nil {
		return r.manageError(logger.WithName("validateWallet"), modifiedADB, err)
	}

	/******************************************************************
	* Update the resource if the spec has been changed.
	*	This will trigger another reconcile, so returns with an empty
	* result.
	******************************************************************/
	if !reflect.DeepEqual(modifiedADB.Spec, desiredADB.Spec) {
		if err := r.KubeClient.Update(context.TODO(), modifiedADB); err != nil {
			return r.manageError(logger.WithName("updateSpec"), modifiedADB, err)
		}
		return emptyResult, nil
	}

	/******************************************************************
	* Update the status at the end of every reconcile.
	******************************************************************/
	copiedADB := modifiedADB.DeepCopy()

	updateCondition(modifiedADB, nil)
	if err := r.KubeClient.Status().Update(context.TODO(), modifiedADB); err != nil {
		return r.manageError(logger.WithName("Status().Update"), modifiedADB, err)
	}
	modifiedADB.Spec = copiedADB.Spec

	if dbv1alpha1.IsADBIntermediateState(modifiedADB.Status.LifecycleState) {
		logger.WithName("IsADBIntermediateState").Info("LifecycleState is " + string(modifiedADB.Status.LifecycleState) + "; reconcile queued")
		return requeueResult, nil
	}

	/******************************************************************
	* Update the lastSucSpec, and then finish the reconcile.
	*	Requeue if the ADB is terminated, but the finalizer is not yet
	* removed.
	******************************************************************/

	var requeue bool = false
	if modifiedADB.GetDeletionTimestamp() != nil &&
		controllerutil.ContainsFinalizer(modifiedADB, dbv1alpha1.ADB_FINALIZER) &&
		modifiedADB.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminated {
		logger.Info("The ADB is TERMINATED. The CR is to be deleted but finalizer is not yet removed; reconcile queued")
		requeue = true
	}

	if err := r.patchLastSuccessfulSpec(modifiedADB); err != nil {
		return r.manageError(logger.WithName("patchLastSuccessfulSpec"), modifiedADB, err)
	}

	if requeue {
		logger.Info("Reconcile queued")
		return requeueResult, nil

	} else {
		logger.Info("AutonomousDatabase reconciles successfully")
		return emptyResult, nil
	}
}

func (r *AutonomousDatabaseReconciler) setupOCIClients(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) error {
	var err error

	authData := oci.APIKeyAuth{
		ConfigMapName: adb.Spec.OCIConfig.ConfigMapName,
		SecretName:    adb.Spec.OCIConfig.SecretName,
		Namespace:     adb.GetNamespace(),
	}

	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		return err
	}

	r.dbService, err = oci.NewDatabaseService(logger, r.KubeClient, provider)
	if err != nil {
		return err
	}

	return nil
}

func (r *AutonomousDatabaseReconciler) manageError(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase, err error) (ctrl.Result, error) {
	l := logger.WithName("manageError")
	if adb.Status.LifecycleState == "" {
		// First time entering reconcile
		updateCondition(adb, err)

		l.Error(err, "CreateFailed")

		return emptyResult, nil
	} else {
		// Has synced at least once
		var finalError = err

		// Roll back
		ociADB := adb.DeepCopy()
		specChanged, err := r.getADB(l, ociADB)
		if err != nil {
			finalError = k8s.CombineErrors(finalError, err)
		}

		// Will exit the Reconcile anyway after the manageError is called.
		if specChanged {
			// Clear the lifecycleState first to avoid the webhook error when update during an intermediate state
			adb.Status.LifecycleState = ""
			if err := r.KubeClient.Status().Update(context.TODO(), adb); err != nil {
				finalError = k8s.CombineErrors(finalError, err)
			}

			adb.Spec = ociADB.Spec

			if err := r.KubeClient.Update(context.TODO(), adb); err != nil {
				finalError = k8s.CombineErrors(finalError, err)
			}
		}

		updateCondition(adb, err)

		l.Error(finalError, "UpdateFailed")

		return emptyResult, nil
	}
}

const CONDITION_TYPE_COMPLETE = "Complete"
const CONDITION_REASON_COMPLETE = "ReconcileComplete"

func updateCondition(adb *dbv1alpha1.AutonomousDatabase, err error) {
	var condition metav1.Condition

	errMsg := func() string {
		if err != nil {
			return err.Error()
		}
		return "no reconcile errors"
	}()

	// If error occurs, ReconcileComplete will be marked as true and the error message will still be listed
	// If the ADB lifecycleState is intermediate, then ReconcileComplete will be marked as false
	if err != nil {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_COMPLETE,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_COMPLETE,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	} else if dbv1alpha1.IsADBIntermediateState(adb.Status.LifecycleState) {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_COMPLETE,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_COMPLETE,
			Message:            errMsg,
			Status:             metav1.ConditionFalse,
		}
	} else {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_COMPLETE,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_COMPLETE,
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}
	}

	if len(adb.Status.Conditions) > 0 {
		meta.RemoveStatusCondition(&adb.Status.Conditions, condition.Type)
	}
	meta.SetStatusCondition(&adb.Status.Conditions, condition)
}

func (r *AutonomousDatabaseReconciler) validateOperation(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (exit bool, result ctrl.Result, err error) {

	lastSucSpec, err := adb.GetLastSuccessfulSpec()
	if err != nil {
		return false, emptyResult, err
	}

	l := logger.WithName("validateOperation")

	// If lastSucSpec is nil, then it's CREATE or BIND opertaion
	if lastSucSpec == nil {
		if adb.Spec.Details.AutonomousDatabaseOCID == nil {
			l.Info("Create operation")
			err := r.createADB(logger, adb)
			if err != nil {
				return false, emptyResult, err
			}

			// Update the ADB OCID
			if err := r.updateCR(adb); err != nil {
				return false, emptyResult, err
			}

			l.Info("AutonomousDatabaseOCID updated; exit reconcile")
			return true, emptyResult, nil
		} else {
			l.Info("Bind operation")
			_, err := r.getADB(logger, adb)
			if err != nil {
				return false, emptyResult, err
			}

			if err := r.updateCR(adb); err != nil {
				return false, emptyResult, err
			}

			l.Info("spec updated; exit reconcile")
			return true, emptyResult, nil
		}
	}

	// If it's not CREATE or BIND opertaion, then it's UPDATE or SYNC operation.
	// In most of the case the user changes the spec, and we update the oci ADB, but when the user updates on
	// the Cloud Console, the controller cannot tell the direction and how to update the resource.
	// Thus we compare the current spec with the lastSucSpec. If the details are different, it means that
	// the user updates the spec (UPDATE operation), otherwise it's a SYNC operation.
	lastDifADB := adb.DeepCopy()

	lastDetailsChanged, err := lastDifADB.RemoveUnchangedDetails(*lastSucSpec)
	if err != nil {
		return false, emptyResult, err
	}

	if lastDetailsChanged {
		// Double check if the user input spec is actually different from the spec in OCI. If so, then update the resource.
		// When the update completes and the status changes from UPDATING to AVAILABLE, the lastSucSpec is not updated yet,
		// so we compare with the oci ADB again to make sure that the updates are completed.

		l.Info("Update operation")

		exit, err := r.updateADB(logger, adb)
		if err != nil {
			return false, emptyResult, err
		}

		return exit, emptyResult, nil

	} else {
		l.Info("No operation specified; sync the resource")

		// The user doesn't change the spec and the controller should pull the spec from the OCI.
		specChanged, err := r.getADB(logger, adb)
		if err != nil {
			return false, emptyResult, err
		}

		if specChanged {
			l.Info("The local spec doesn't match the oci's spec; update the CR")

			// Erase the status.lifecycleState temporarily to avoid the webhook error.
			tmpADB := adb.DeepCopy()
			adb.Status.LifecycleState = ""
			if err := r.KubeClient.Status().Update(context.TODO(), adb); err != nil {
				return false, emptyResult, err
			}
			adb.Spec = tmpADB.Spec

			if err := r.updateCR(adb); err != nil {
				return false, emptyResult, err
			}

			return true, emptyResult, nil
		}
		return false, emptyResult, nil
	}
}

func (r *AutonomousDatabaseReconciler) validateCleanup(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) (exitReconcile bool, err error) {
	l := logger.WithName("validateCleanup")

	isADBToBeDeleted := adb.GetDeletionTimestamp() != nil

	if !isADBToBeDeleted {
		return false, nil
	}

	if controllerutil.ContainsFinalizer(adb, dbv1alpha1.ADB_FINALIZER) {
		if adb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminating {
			// Delete in progress, continue with the reconcile logic
			return false, nil
		}

		if adb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminated {
			// The adb has been deleted. Remove the finalizer and exit the reconcile.
			// Once all finalizers have been removed, the object will be deleted.
			l.Info("Resource is in TERMINATED state; remove the finalizer")
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, adb, dbv1alpha1.ADB_FINALIZER); err != nil {
				return false, err
			}
			return true, nil
		}

		if adb.Spec.Details.AutonomousDatabaseOCID == nil {
			l.Info("Missing AutonomousDatabaseOCID to terminate Autonomous Database; remove the finalizer anyway", "Name", adb.Name, "Namespace", adb.Namespace)
			// Remove finalizer anyway.
			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, adb, dbv1alpha1.ADB_FINALIZER); err != nil {
				return false, err
			}
			return true, nil
		}

		if adb.Spec.Details.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminated {
			// Run finalization logic for finalizer. If the finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			l.Info("Terminating Autonomous Database")
			adb.Spec.Details.LifecycleState = database.AutonomousDatabaseLifecycleStateTerminated
			if err := r.KubeClient.Update(context.TODO(), adb); err != nil {
				return false, err
			}
			// Exit the reconcile since we have updated the spec
			return true, nil
		}

		// Continue with the reconcile logic
		return false, nil
	}

	// Exit the Reconcile since the to-be-deleted resource doesn't has a finalizer
	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateFinalizer(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) (exit bool, err error) {
	l := logger.WithName("validateFinalizer")

	// Delete is not schduled. Update the finalizer for this CR if hardLink is present
	var finalizerChanged = false
	if adb.Spec.HardLink != nil {
		if *adb.Spec.HardLink && !controllerutil.ContainsFinalizer(adb, dbv1alpha1.ADB_FINALIZER) {
			l.Info("Finalizer added")
			if err := k8s.AddFinalizerAndPatch(r.KubeClient, adb, dbv1alpha1.ADB_FINALIZER); err != nil {
				return false, err
			}

			finalizerChanged = true

		} else if !*adb.Spec.HardLink && controllerutil.ContainsFinalizer(adb, dbv1alpha1.ADB_FINALIZER) {
			l.Info("Finalizer removed")

			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, adb, dbv1alpha1.ADB_FINALIZER); err != nil {
				return false, err
			}

			finalizerChanged = true
		}
	}

	// If the finalizer is changed during an intermediate state, e.g. set hardLink to true and
	// delete the resource, then there must be another ongoing reconcile. In this case we should
	// exit the reconcile.
	if finalizerChanged && dbv1alpha1.IsADBIntermediateState(adb.Status.LifecycleState) {
		l.Info("Finalizer changed during an intermediate state, exit the reconcile")
		return true, nil
	}

	return false, nil
}

// updateCR updates the lastSucSpec and the CR
func (r *AutonomousDatabaseReconciler) updateCR(adb *dbv1alpha1.AutonomousDatabase) error {
	// Update the lastSucSpec
	// Should patch the lastSuccessfulSpec first, otherwise, the update event will be
	// filtered out by predicate since the lastSuccessfulSpec is changed.
	if err := r.patchLastSuccessfulSpec(adb); err != nil {
		return err
	}

	if err := r.KubeClient.Update(context.TODO(), adb); err != nil {
		return err
	}
	return nil
}

func (r *AutonomousDatabaseReconciler) patchLastSuccessfulSpec(adb *dbv1alpha1.AutonomousDatabase) error {
	copyADB := adb.DeepCopy()

	specBytes, err := json.Marshal(adb.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		dbv1alpha1.LastSuccessfulSpec: string(specBytes),
	}

	annotations.PatchAnnotations(r.KubeClient, adb, anns)

	adb.Spec = copyADB.Spec
	adb.Status = copyADB.Status

	return nil
}

func (r *AutonomousDatabaseReconciler) createADB(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) error {
	logger.WithName("createADB").Info("Sending CreateAutonomousDatabase request to OCI")
	resp, err := r.dbService.CreateAutonomousDatabase(adb)
	if err != nil {
		return err
	}

	// Restore the admin password after updating from OCI ADB
	adminPass := adb.Spec.Details.AdminPassword
	adb.UpdateFromOCIADB(resp.AutonomousDatabase)
	adb.Spec.Details.AdminPassword = adminPass

	return nil
}

// getADB gets the information from OCI and overwrites the spec and the status, but not update the CR in the cluster
func (r *AutonomousDatabaseReconciler) getADB(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) (bool, error) {
	if adb == nil {
		return false, errors.New("AutonomousDatabase OCID is missing")
	}

	l := logger.WithName("getADB")

	// Get the information from OCI
	l.Info("Sending GetAutonomousDatabase request to OCI")
	resp, err := r.dbService.GetAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return false, err
	}

	specChanged := adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return specChanged, nil
}

// updateADB returns true if an OCI request is sent.
// The AutonomousDatabase is updated with the returned object from the OCI requests.
func (r *AutonomousDatabaseReconciler) updateADB(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase) (exit bool, err error) {

	l := logger.WithName("updateADB")

	// Get OCI AutonomousDatabase and update the lifecycleState of the CR,
	// so that the validatexx functions know when the state changes back to AVAILABLE
	ociADB := adb.DeepCopy()
	_, err = r.getADB(logger, ociADB)
	if err != nil {
		return false, err
	}

	adb.Status.LifecycleState = ociADB.Status.LifecycleState

	// Start update
	difADB := adb.DeepCopy()

	ociDetailsChanged, err := difADB.RemoveUnchangedDetails(ociADB.Spec)
	if err != nil {
		return false, err
	}

	// Do the update request only if the current ADB is actually different from the OCI ADB
	if ociDetailsChanged {
		// Special case: if the oci ADB is terminating, then update the spec and exit the reconcile.
		// This happens when the lifecycleState changes to TERMINATED during an intermediate state,
		// whatever is in progress should be abandonded and the desired spec should the same as oci ADB.
		if ociADB.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminating {
			l.Info("OCI ADB is in TERMINATING state; update the spec and exit the reconcile")

			adb.Status.LifecycleState = ""
			if err := r.KubeClient.Status().Update(context.TODO(), adb); err != nil {
				return false, err
			}

			adb.Spec = ociADB.Spec
			if err := r.KubeClient.Update(context.TODO(), adb); err != nil {
				return false, err
			}
			return true, nil
		}

		// Special case: if the lifecycleState is changed, it might have to exit the reconcile in some cases.
		sent, exit, err := r.validateDesiredLifecycleState(logger, adb, difADB, ociADB)
		if err != nil {
			return false, err
		}
		if sent {
			return exit, nil
		}

		validations := []func(logr.Logger, *dbv1alpha1.AutonomousDatabase, *dbv1alpha1.AutonomousDatabase, *dbv1alpha1.AutonomousDatabase) (bool, error){
			r.validateGeneralFields,
			r.validateAdminPassword,
			r.validateDbWorkload,
			r.validateLicenseModel,
			r.validateScalingFields,
			r.validateGeneralNetworkAccess,
		}

		for _, op := range validations {
			sent, err := op(logger, adb, difADB, ociADB)
			if err != nil {
				return false, err
			}

			if sent {
				return false, nil
			}
		}
	}

	return false, nil
}

func (r *AutonomousDatabaseReconciler) validateGeneralFields(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.DisplayName == nil &&
		difADB.Spec.Details.DbName == nil &&
		difADB.Spec.Details.DbVersion == nil &&
		difADB.Spec.Details.FreeformTags == nil {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateGeneralFields")

	l.Info("Sending UpdateAutonomousDatabase request to OCI")
	resp, err := r.dbService.UpdateAutonomousDatabaseGeneralFields(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

// Special case: compare with lastSpec but not ociSpec
func (r *AutonomousDatabaseReconciler) validateAdminPassword(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.AdminPassword.K8sSecret.Name == nil &&
		difADB.Spec.Details.AdminPassword.OCISecret.OCID == nil {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateAdminPassword")

	l.Info("Sending UpdateAutonomousDatabase request to OCI")
	resp, err := r.dbService.UpdateAutonomousDatabaseAdminPassword(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)
	// Update the admin password fields because they are missing in the ociADB
	adb.Spec.Details.AdminPassword = difADB.Spec.Details.AdminPassword

	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateDbWorkload(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.DbWorkload == "" {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateDbWorkload")

	l.Info("Sending UpdateAutonomousDatabase request to OCI")
	resp, err := r.dbService.UpdateAutonomousDatabaseDBWorkload(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateLicenseModel(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.LicenseModel == "" {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateLicenseModel")

	l.Info("Sending UpdateAutonomousDatabase request to OCI")
	resp, err := r.dbService.UpdateAutonomousDatabaseLicenseModel(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateScalingFields(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.DataStorageSizeInTBs == nil &&
		difADB.Spec.Details.CPUCoreCount == nil &&
		difADB.Spec.Details.IsAutoScalingEnabled == nil {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateScalingFields")

	l.Info("Sending UpdateAutonomousDatabase request to OCI")
	resp, err := r.dbService.UpdateAutonomousDatabaseScalingFields(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateDesiredLifecycleState(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, exit bool, err error) {

	if difADB.Spec.Details.LifecycleState == "" {
		return false, false, nil
	}

	if difADB.Spec.Details.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminated {
		// OCI only allows terminate operation when the ADB is in an valid state, otherwise requeue the reconcile.
		if !dbv1alpha1.ValidADBTerminateState(adb.Status.LifecycleState) {
			return false, false, nil
		}
	} else if dbv1alpha1.IsADBIntermediateState(ociADB.Status.LifecycleState) {
		// Other lifecycle management operation; requeue the reconcile if it's in an intermediate state
		return false, false, nil
	}

	l := logger.WithName("validateDesiredLifecycleState")

	switch difADB.Spec.Details.LifecycleState {
	case database.AutonomousDatabaseLifecycleStateAvailable:
		l.Info("Sending StartAutonomousDatabase request to OCI")

		resp, err := r.dbService.StartAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return false, false, err
		}

		adb.Status.LifecycleState = resp.LifecycleState
	case database.AutonomousDatabaseLifecycleStateStopped:
		l.Info("Sending StopAutonomousDatabase request to OCI")

		resp, err := r.dbService.StopAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return false, false, err
		}

		adb.Status.LifecycleState = resp.LifecycleState
	case database.AutonomousDatabaseLifecycleStateTerminated:
		l.Info("Sending DeleteAutonomousDatabase request to OCI")

		_, err := r.dbService.DeleteAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return false, false, err
		}

		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateTerminating

		// The controller allows terminate during some intermediate states.
		// Exit the reconcile because there is already another ongoing reconcile.
		if dbv1alpha1.IsADBIntermediateState(ociADB.Status.LifecycleState) {
			l.Info("Terminating an ADB which is in an intermediate state; exit reconcile")
			return true, true, nil
		}
	default:
		return false, false, errors.New("unknown lifecycleState")
	}

	return true, false, nil
}

// The logic of updating the network access configurations is as follows:
//
//  1. Shared databases:
//     If the network access type changes
//     a. to PUBLIC:
//     was RESTRICTED: re-enable IsMTLSConnectionRequired if its not. Then set WhitelistedIps to an array with a single empty string entry.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if its not. Then set PrivateEndpointLabel to an emtpy string.
//     b. to RESTRICTED:
//     was PUBLIC: set WhitelistedIps to desired IPs/CIDR blocks/VCN OCID. Configure the IsMTLSConnectionRequired settings if it is set to disabled.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if its not. Set the type to PUBLIC first, and then configure the WhitelistedIps. Finally resume the IsMTLSConnectionRequired settings if it was, or is configured as disabled.
//     c. to PRIVATE:
//     was PUBLIC: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//     was RESTRICTED: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//     *Note: OCI requires nsgOCIDs to be an empty string rather than nil when we don't want the adb to be included in any network security group.
//
//     Otherwise, if the network access type remains the same, apply the network configuration, and then set the IsMTLSConnectionRequired.
//
//  2. Dedicated databases:
//     Apply the configs directly
func (r *AutonomousDatabaseReconciler) validateGeneralNetworkAccess(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.NetworkAccess.AccessType == "" &&
		difADB.Spec.Details.NetworkAccess.IsAccessControlEnabled == nil &&
		difADB.Spec.Details.NetworkAccess.AccessControlList == nil &&
		difADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix == nil {
		return false, nil
	}

	if ociADB.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateAvailable {
		return false, nil
	}

	l := logger.WithName("validateGeneralNetworkAccess")

	if !*adb.Spec.Details.IsDedicated {
		var lastAccessType = ociADB.Spec.Details.NetworkAccess.AccessType
		var difAccessType = difADB.Spec.Details.NetworkAccess.AccessType

		if difAccessType != "" {
			switch difAccessType {
			case dbv1alpha1.NetworkAccessTypePublic:
				l.Info("Configuring network access type to PUBLIC")
				// OCI validation requires IsMTLSConnectionRequired to be enabled before changing the network access type to PUBLIC
				if !*ociADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired {
					if err := r.setMTLSRequired(logger, adb); err != nil {
						return false, err
					}
					return true, nil
				}

				if err := r.setNetworkAccessPublic(logger, ociADB.Spec.Details.NetworkAccess.AccessType, adb); err != nil {
					return false, err
				}
				return true, nil
			case dbv1alpha1.NetworkAccessTypeRestricted:
				l.Info("Configuring network access type to RESTRICTED")
				// If the access type was PRIVATE, then OCI validation requires IsMTLSConnectionRequired
				// to be enabled before setting ACL. Also, we can only change the network access type from
				// PRIVATE to PUBLIC, so the steps are PRIVATE->(requeue)->PUBLIC->(requeue)->RESTRICTED.
				if lastAccessType == dbv1alpha1.NetworkAccessTypePrivate {
					if !*ociADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired {
						if err := r.setMTLSRequired(logger, adb); err != nil {
							return false, err
						}
						return true, nil
					}

					if err := r.setNetworkAccessPublic(logger, ociADB.Spec.Details.NetworkAccess.AccessType, adb); err != nil {
						return false, err
					}
					return true, nil
				}

				sent, err := r.validateNetworkAccess(logger, adb, difADB, ociADB)
				if err != nil {
					return false, err
				}
				if sent {
					return true, nil
				}

				sent, err = r.validateMTLS(logger, adb, difADB, ociADB)
				if err != nil {
					return false, err
				}
				if sent {
					return true, nil
				}
			case dbv1alpha1.NetworkAccessTypePrivate:
				l.Info("Configuring network access type to PRIVATE")

				sent, err := r.validateNetworkAccess(logger, adb, difADB, ociADB)
				if err != nil {
					return false, err
				}
				if sent {
					return true, nil
				}

				sent, err = r.validateMTLS(logger, adb, difADB, ociADB)
				if err != nil {
					return false, err
				}
				if sent {
					return true, nil
				}
			}
		} else {
			// Access type doesn't change
			sent, err := r.validateNetworkAccess(logger, adb, difADB, ociADB)
			if err != nil {
				return false, err
			}
			if sent {
				return true, nil
			}

			sent, err = r.validateMTLS(logger, adb, difADB, ociADB)
			if err != nil {
				return false, err
			}
			if sent {
				return true, nil
			}
		}
	} else {
		// Dedicated database
		sent, err := r.validateNetworkAccess(logger, adb, difADB, ociADB)
		if err != nil {
			return false, err
		}
		if sent {
			return true, nil
		}
	}

	return false, nil
}

// Set the mTLS to true but not changing the spec
func (r *AutonomousDatabaseReconciler) setMTLSRequired(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) error {
	l := logger.WithName("setMTLSRequired")

	l.Info("Sending request to OCI to set IsMtlsConnectionRequired to true")

	adb.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = common.Bool(true)

	resp, err := r.dbService.UpdateNetworkAccessMTLSRequired(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return nil
}

func (r *AutonomousDatabaseReconciler) validateMTLS(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired == nil {
		return false, nil
	}

	l := logger.WithName("validateMTLS")

	l.Info("Sending request to OCI to configure IsMtlsConnectionRequired")

	resp, err := r.dbService.UpdateNetworkAccessMTLS(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

func (r *AutonomousDatabaseReconciler) setNetworkAccessPublic(logger logr.Logger, lastAcessType dbv1alpha1.NetworkAccessTypeEnum, adb *dbv1alpha1.AutonomousDatabase) error {
	adb.Spec.Details.NetworkAccess.AccessType = dbv1alpha1.NetworkAccessTypePublic
	adb.Spec.Details.NetworkAccess.AccessControlList = nil
	adb.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix = common.String("")
	adb.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = nil
	adb.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID = nil

	l := logger.WithName("setNetworkAccessPublic")

	l.Info("Sending request to OCI to configure network access options to PUBLIC")

	resp, err := r.dbService.UpdateNetworkAccessPublic(lastAcessType, *adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return nil
}

func (r *AutonomousDatabaseReconciler) validateNetworkAccess(
	logger logr.Logger,
	adb *dbv1alpha1.AutonomousDatabase,
	difADB *dbv1alpha1.AutonomousDatabase,
	ociADB *dbv1alpha1.AutonomousDatabase) (sent bool, err error) {

	if difADB.Spec.Details.NetworkAccess.AccessType == "" &&
		difADB.Spec.Details.NetworkAccess.IsAccessControlEnabled == nil &&
		difADB.Spec.Details.NetworkAccess.AccessControlList == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix == nil {
		return false, nil
	}

	l := logger.WithName("validateNetworkAccess")

	l.Info("Sending request to OCI to configure network access options")

	// When the network access type is set to PRIVATE, any nil type of nsgOCIDs needs to be set to an empty string, otherwise, OCI SDK returns a 400 error
	if difADB.Spec.Details.NetworkAccess.AccessType == dbv1alpha1.NetworkAccessTypePrivate &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil {
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = []string{}
	}

	resp, err := r.dbService.UpdateNetworkAccess(*adb.Spec.Details.AutonomousDatabaseOCID, difADB)
	if err != nil {
		return false, err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	return true, nil
}

func (r *AutonomousDatabaseReconciler) validateWallet(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) error {
	if adb.Spec.Details.Wallet.Name == nil &&
		adb.Spec.Details.Wallet.Password.K8sSecret.Name == nil &&
		adb.Spec.Details.Wallet.Password.OCISecret.OCID == nil {
		return nil
	}

	if adb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateProvisioning {
		return nil
	}

	l := logger.WithName("validateWallet")

	// lastSucSpec may be nil if this is the first time entering the reconciliation loop
	var walletName string

	if adb.Spec.Details.Wallet.Name == nil {
		walletName = adb.GetName() + "-instance-wallet"
	} else {
		walletName = *adb.Spec.Details.Wallet.Name
	}

	secret, err := k8s.FetchSecret(r.KubeClient, adb.GetNamespace(), walletName)
	if err == nil {
		val, ok := secret.Labels["app"]
		if !ok || val != adb.Name {
			// Overwrite if the fetched secret has a different label
			l.Info("wallet existed but has a different label; skip the download")
		}
		// No-op if Wallet is already downloaded
		return nil
	} else if !apiErrors.IsNotFound(err) {
		return err
	}

	resp, err := r.dbService.DownloadWallet(adb)
	if err != nil {
		return err
	}

	data, err := oci.ExtractWallet(resp.Content)
	if err != nil {
		return err
	}

	adb.Status.WalletExpiringDate = oci.WalletExpiringDate(data)

	label := map[string]string{"app": adb.GetName()}

	if err := k8s.CreateSecret(r.KubeClient, adb.Namespace, walletName, data, adb, label); err != nil {
		return err
	}

	l.Info(fmt.Sprintf("Wallet is stored in the Secret %s", walletName))

	return nil
}

// updateBackupResources get the list of AutonomousDatabasBackups and
// create a backup object if it's not found in the same namespace
func (r *AutonomousDatabaseReconciler) syncBackupResources(logger logr.Logger, adb *dbv1alpha1.AutonomousDatabase) error {
	l := logger.WithName("syncBackupResources")

	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := k8s.FetchAutonomousDatabaseBackups(r.KubeClient, adb.Namespace)
	if err != nil {
		return err
	}

	curBackupNames := make(map[string]bool)
	curBackupOCIDs := make(map[string]bool)

	for _, backup := range backupList.Items {
		// mark the backup name that exists
		curBackupNames[backup.Name] = true

		// mark the backup ocid that exists
		if backup.Spec.AutonomousDatabaseBackupOCID != nil {
			curBackupOCIDs[*backup.Spec.AutonomousDatabaseBackupOCID] = true
		}
	}

	resp, err := r.dbService.ListAutonomousDatabaseBackups(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	for _, backupSummary := range resp.Items {
		// Create the resource if the backup doesn't exist
		if !r.ifBackupExists(backupSummary, curBackupOCIDs, backupList) {
			validBackupName, err := r.getValidBackupName(*backupSummary.DisplayName, curBackupNames)
			if err != nil {
				return err
			}

			if err := k8s.CreateAutonomousBackup(r.KubeClient, validBackupName, backupSummary, adb); err != nil {
				return err
			}

			// Add the used name and ocid
			curBackupNames[validBackupName] = true
			curBackupOCIDs[*backupSummary.AutonomousDatabaseId] = true

			l.Info("Create AutonomousDatabaseBackup " + validBackupName)
		}
	}

	return nil
}

func (r *AutonomousDatabaseReconciler) getValidBackupName(displayName string, usedNames map[string]bool) (string, error) {
	// Convert the displayName to lowercase, and replace spaces, commas, and colons with hyphens
	baseName := strings.ToLower(displayName)

	re, err := regexp.Compile(`[^-a-zA-Z0-9]`)
	if err != nil {
		return "", err
	}

	baseName = re.ReplaceAllString(baseName, "-")

	finalName := baseName
	var i = 1
	_, ok := usedNames[finalName]
	for ok {
		finalName = fmt.Sprintf("%s-%d", baseName, i)
		_, ok = usedNames[finalName]
		i++
	}

	return finalName, nil
}

func (r *AutonomousDatabaseReconciler) ifBackupExists(backupSummary database.AutonomousDatabaseBackupSummary, curBackupOCIDs map[string]bool, backupList *dbv1alpha1.AutonomousDatabaseBackupList) bool {
	_, ok := curBackupOCIDs[*backupSummary.Id]
	if ok {
		return true
	}

	// Special case: when a Backup is creating and hasn't updated the OCID, a duplicated Backup might be created by mistake.
	// To handle this case, skip creating the AutonomousDatabaseBackup resource if the current backupSummary is with CREATING state,
	// and there is another AutonomousBackup with the same displayName in the cluster is also at CREATING state.
	if backupSummary.LifecycleState == database.AutonomousDatabaseBackupSummaryLifecycleStateCreating {
		for _, backup := range backupList.Items {
			if (backup.Spec.DisplayName != nil && *backup.Spec.DisplayName == *backupSummary.DisplayName) &&
				(backup.Status.LifecycleState == "" ||
					backup.Status.LifecycleState == database.AutonomousDatabaseBackupLifecycleStateCreating) {
				return true
			}
		}
	}

	return false
}
