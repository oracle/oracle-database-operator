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
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
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

	dbv4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// name of our custom finalizer
const ADB_FINALIZER = "database.oracle.com/adb-finalizer"

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
		For(&dbv4.AutonomousDatabase{}).
		Watches(
			&dbv4.AutonomousDatabaseRestore{},
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
			_, restoreOk := e.Object.(*dbv4.AutonomousDatabaseRestore)
			// Don't enqueue if the event is from Backup or Restore
			return !restoreOk
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Enqueue the update event only when the status changes the first time
			desiredRestore, restoreOk := e.ObjectNew.(*dbv4.AutonomousDatabaseRestore)
			if restoreOk {
				oldRestore := e.ObjectOld.(*dbv4.AutonomousDatabaseRestore)
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
			desiredAdb, adbOk := e.ObjectNew.(*dbv4.AutonomousDatabase)
			if adbOk {
				oldAdb := e.ObjectOld.(*dbv4.AutonomousDatabase)

				specChanged := !reflect.DeepEqual(oldAdb.Spec, desiredAdb.Spec)
				statusChanged := !reflect.DeepEqual(oldAdb.Status, desiredAdb.Status)

				if (!specChanged && statusChanged) ||
					(controllerutil.ContainsFinalizer(oldAdb, ADB_FINALIZER) != controllerutil.ContainsFinalizer(desiredAdb, ADB_FINALIZER)) {
					// Don't enqueue in the folowing condition:
					// 1. only status changes 2. ADB_FINALIZER changes
					return false
				}

				return true
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the object is deleted from the cluster.
			_, adbOk := e.Object.(*dbv4.AutonomousDatabase)
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

func (r *AutonomousDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespace/Name", req.NamespacedName)

	var err error
	// Indicates whether spec has been changed at the end of the reconcile.
	var specChanged bool = false

	// Get the autonomousdatabase instance from the cluster
	desiredAdb := &dbv4.AutonomousDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, desiredAdb); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		return emptyResult, err
	}

	/******************************************************************
	* Get OCI database client
	******************************************************************/
	if err := r.setupOCIClients(logger, desiredAdb); err != nil {
		return r.manageError(
			logger.WithName("setupOCIClients"),
			desiredAdb,
			fmt.Errorf("Failed to get OCI Database Client: %w", err))
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	* Fill the empty fields in the local resource at the beginning of
	* the reconciliation.
	******************************************************************/
	// Fill the empty fields in the AutonomousDatabase resource by
	// syncing up with the Autonomous Database in OCI. Only the fields
	// that have nil values will be overwritten.
	var stateBeforeFirstSync = desiredAdb.Status.LifecycleState
	if _, err = r.syncAutonomousDatabase(logger, desiredAdb, false); err != nil {
		return r.manageError(
			logger.WithName("syncAutonomousDatabase"),
			desiredAdb,
			fmt.Errorf("Failed to sync AutonomousDatabase: %w", err))
	}

	// If the lifecycle state changes from any other states to
	// AVAILABLE and spec.action is an empty string, it means that
	// the resource in OCI just finished the work, and the spec
	// of the Autonomous Database in OCI might also change.
	// This is because OCI won't update the spec until the work
	// completes. In this case, we need to update the spec of
	// the resource in local cluster.
	if stateBeforeFirstSync != database.AutonomousDatabaseLifecycleStateAvailable &&
		desiredAdb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateAvailable {
		if specChanged, err = r.syncAutonomousDatabase(logger, desiredAdb, true); err != nil {
			return r.manageError(
				logger.WithName("syncAutonomousDatabase"),
				desiredAdb,
				fmt.Errorf("Failed to sync AutonomousDatabase: %w", err))
		}
	}

	/******************************************************************
	* Determine if the external resource needs to be cleaned up.
	* If yes, delete the Autonomous Database in OCI and exits the
	* reconcile function immediately.
	*
	* There is no need to check the other fields if the resource is
	* under deletion. This method should be executed soon after the OCI
	* database client is obtained and the local resource is synced in
	* the above two steps.
	*
	* Kubernetes server calls the clean up function if a finalizer exitsts,
	* and won't delete the object until all the finalizers are removed
	* from the object metadata.
	******************************************************************/
	if desiredAdb.GetDeletionTimestamp().IsZero() {
		// The Autonomous Database is not being deleted. Update the finalizer.
		if desiredAdb.Spec.HardLink != nil &&
			*desiredAdb.Spec.HardLink &&
			!controllerutil.ContainsFinalizer(desiredAdb, ADB_FINALIZER) {

			if err := k8s.AddFinalizerAndPatch(r.KubeClient, desiredAdb, ADB_FINALIZER); err != nil {
				return emptyResult, fmt.Errorf("Failed to add finalizer to Autonomous Database "+desiredAdb.Name+": %w", err)
			}
		} else if desiredAdb.Spec.HardLink != nil &&
			!*desiredAdb.Spec.HardLink &&
			controllerutil.ContainsFinalizer(desiredAdb, ADB_FINALIZER) {

			if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, desiredAdb, ADB_FINALIZER); err != nil {
				return emptyResult, fmt.Errorf("Failed to remove finalizer to Autonomous Database "+desiredAdb.Name+": %w", err)
			}
		}
	} else {
		// The Autonomous Database is being deleted
		if controllerutil.ContainsFinalizer(desiredAdb, ADB_FINALIZER) {
			if dbv4.IsAdbIntermediateState(desiredAdb.Status.LifecycleState) {
				// No-op
			} else if desiredAdb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateTerminated {
				// The Autonomous Database in OCI has been deleted. Remove the finalizer.
				if err := k8s.RemoveFinalizerAndPatch(r.KubeClient, desiredAdb, ADB_FINALIZER); err != nil {
					return emptyResult, fmt.Errorf("Failed to remove finalizer to Autonomous Database "+desiredAdb.Name+": %w", err)
				}
			} else {
				// Remove the Autonomous Database in OCI.
				// Change the action to Terminate and proceed with the rest of the reconcile logic
				desiredAdb.Spec.Action = "Terminate"
			}
		}
	}

	if !dbv4.IsAdbIntermediateState(desiredAdb.Status.LifecycleState) {
		/******************************************************************
		* Perform operations
		******************************************************************/
		var specChangedAfterOperation bool
		specChangedAfterOperation, err = r.performOperation(logger, desiredAdb)
		if err != nil {
			return r.manageError(
				logger.WithName("performOperation"),
				desiredAdb,
				fmt.Errorf("Failed to operate database action: %w", err))
		}

		if specChangedAfterOperation {
			specChanged = true
		}

		/******************************************************************
		*	Sync AutonomousDatabase Backups from OCI.
		* The backups will not be synced when the lifecycle state is
		* TERMINATING or TERMINATED.
		******************************************************************/
		if desiredAdb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminating &&
			desiredAdb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminated {
			if err := r.syncBackupResources(logger, desiredAdb); err != nil {
				return r.manageError(logger.WithName("syncBackupResources"), desiredAdb, err)
			}
		}

		/*****************************************************
		*	Validate Wallet
		*****************************************************/
		if err := r.validateWallet(logger, desiredAdb); err != nil {
			return r.manageError(
				logger.WithName("validateWallet"),
				desiredAdb,
				fmt.Errorf("Failed to validate Wallet: %w", err))
		}
	}

	/******************************************************************
	* Update the Autonomous Database at the end of every reconcile.
	******************************************************************/
	if specChanged {
		if err := r.KubeClient.Update(context.TODO(), desiredAdb); err != nil {
			return r.manageError(
				logger.WithName("updateSpec"),
				desiredAdb,
				fmt.Errorf("Failed to update AutonomousDatabase spec: %w", err))
		}
		// Immediately exit the reconcile loop if the resource is updated, and let
		// the next run continue.
		return emptyResult, nil
	}

	updateCondition(desiredAdb, nil)
	if err := r.KubeClient.Status().Update(context.TODO(), desiredAdb); err != nil {
		return r.manageError(
			logger,
			desiredAdb,
			fmt.Errorf("Failed to update AutonomousDatabase status: %w", err))
	}

	/******************************************************************
	*	Requeue the request in the following cases:
	* 1. the ADB is in intermediate state
	* 2. the ADB is terminated, but the finalizer is not yet removed.
	******************************************************************/
	if dbv4.IsAdbIntermediateState(desiredAdb.Status.LifecycleState) {
		logger.
			WithName("IsAdbIntermediateState").
			Info("LifecycleState is " + string(desiredAdb.Status.LifecycleState) + "; reconciliation queued")
		return requeueResult, nil
	} else {
		logger.Info("AutonomousDatabase reconciles successfully")
		return emptyResult, nil
	}
}

func (r *AutonomousDatabaseReconciler) setupOCIClients(logger logr.Logger, adb *dbv4.AutonomousDatabase) error {
	var err error

	authData := oci.ApiKeyAuth{
		ConfigMapName: adb.Spec.OciConfig.ConfigMapName,
		SecretName:    adb.Spec.OciConfig.SecretName,
		Namespace:     adb.GetNamespace(),
	}

	provider, err := oci.GetOciProvider(r.KubeClient, authData)
	if err != nil {
		return err
	}

	r.dbService, err = oci.NewDatabaseService(logger, r.KubeClient, provider)
	if err != nil {
		return err
	}

	return nil
}

// Upates the status with the error and returns an empty result
func (r *AutonomousDatabaseReconciler) manageError(logger logr.Logger, adb *dbv4.AutonomousDatabase, err error) (ctrl.Result, error) {
	l := logger.WithName("manageError")

	l.Error(err, "Error occured")

	updateCondition(adb, err)
	if err := r.KubeClient.Status().Update(context.TODO(), adb); err != nil {
		return emptyResult, fmt.Errorf("Failed to update status: %w", err)
	}
	return emptyResult, nil
}

const CONDITION_TYPE_AVAILABLE = "Available"
const CONDITION_REASON_AVAILABLE = "Available"
const CONDITION_TYPE_RECONCILE_QUEUED = "ReconcileQueued"
const CONDITION_REASON_RECONCILE_QUEUED = "LastReconcileQueued"
const CONDITION_TYPE_RECONCILE_ERROR = "ReconfileError"
const CONDITION_REASON_RECONCILE_ERROR = "LastReconcileError"

func updateCondition(adb *dbv4.AutonomousDatabase, err error) {
	var condition metav1.Condition
	var errMsg string

	if err != nil {
		errMsg = err.Error()
	}

	// Clean up the Conditions array
	if len(adb.Status.Conditions) > 0 {
		var allConditions = []string{
			CONDITION_TYPE_AVAILABLE,
			CONDITION_TYPE_RECONCILE_QUEUED,
			CONDITION_TYPE_RECONCILE_ERROR}

		for _, conditionType := range allConditions {
			meta.RemoveStatusCondition(&adb.Status.Conditions, conditionType)
		}
	}

	// If error occurs, the condition status will be marked as false and the error message will still be listed
	// If the ADB lifecycleState is intermediate, then condition status will be marked as true
	// Otherwise, then condition status will be marked as true if no error occurs
	if err != nil {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_RECONCILE_ERROR,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_RECONCILE_ERROR,
			Message:            errMsg,
			Status:             metav1.ConditionFalse,
		}
	} else if dbv4.IsAdbIntermediateState(adb.Status.LifecycleState) {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_RECONCILE_QUEUED,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_RECONCILE_QUEUED,
			Message:            "no reconcile errors",
			Status:             metav1.ConditionTrue,
		}
	} else {
		condition = metav1.Condition{
			Type:               CONDITION_TYPE_AVAILABLE,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: adb.GetGeneration(),
			Reason:             CONDITION_REASON_AVAILABLE,
			Message:            "no reconcile errors",
			Status:             metav1.ConditionTrue,
		}
	}

	meta.SetStatusCondition(&adb.Status.Conditions, condition)
}

func (r *AutonomousDatabaseReconciler) performOperation(
	logger logr.Logger,
	adb *dbv4.AutonomousDatabase) (specChanged bool, err error) {

	l := logger.WithName("validateOperation")

	switch adb.Spec.Action {
	case "Create":
		l.Info("Create operation")
		err := r.createAutonomousDatabase(logger, adb)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		return true, nil

	case "Sync":
		l.Info("Sync operation")
		_, err = r.syncAutonomousDatabase(logger, adb, true)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		return true, nil

	case "Update":
		l.Info("Update operation")
		err = r.updateAutonomousDatabase(logger, adb)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		return true, nil

	case "Stop":
		l.Info("Sending StopAutonomousDatabase request to OCI")

		resp, err := r.dbService.StopAutonomousDatabase(*adb.Spec.Details.Id)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		adb.Status.LifecycleState = resp.LifecycleState
		return true, nil

	case "Start":
		l.Info("Sending StartAutonomousDatabase request to OCI")

		resp, err := r.dbService.StartAutonomousDatabase(*adb.Spec.Details.Id)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		adb.Status.LifecycleState = resp.LifecycleState
		return true, nil

	case "Terminate":
		// OCI only allows terminate operation when the ADB is in an valid state, otherwise requeue the reconcile.
		if dbv4.CanBeTerminated(adb.Status.LifecycleState) {
			l.Info("Sending DeleteAutonomousDatabase request to OCI")

			_, err := r.dbService.DeleteAutonomousDatabase(*adb.Spec.Details.Id)
			if err != nil {
				return false, err
			}

			if err := r.removeBackupResources(l, adb); err != nil {
				return false, err
			}

			adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateTerminating
		} else if dbv4.IsAdbIntermediateState(adb.Status.LifecycleState) {
			l.Info("Can not terminate an ADB in an intermediate state; exit reconcile")
		}

		adb.Spec.Action = ""
		return true, nil

	case "Clone":
		resp, err := r.dbService.CreateAutonomousDatabaseClone(adb)
		if err != nil {
			return false, err
		}
		adb.Status.LifecycleState = resp.LifecycleState

		adb.Spec.Action = ""

		// Create cloned Autonomous Database resource
		clonedAdb := &dbv4.AutonomousDatabase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *adb.Spec.Clone.DisplayName,
				Namespace: adb.Namespace,
			},
			Spec: dbv4.AutonomousDatabaseSpec{
				OciConfig: *adb.Spec.OciConfig.DeepCopy(),
			},
		}
		clonedAdb.UpdateFromOciAdb(resp.AutonomousDatabase, true)
		if err := r.KubeClient.Create(context.TODO(), clonedAdb); err != nil {
			return false, err
		}
		return true, nil

	case "Switchover":
		l.Info("Sending SwitchoverAutonomousDatabase request to OCI")

		resp, err := r.dbService.SwitchoverAutonomousDatabase(*adb.Spec.Details.Id)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		adb.Status.LifecycleState = resp.LifecycleState
		return true, nil

	case "Failover":
		l.Info("Sending FailOverAutonomousDatabase request to OCI")

		resp, err := r.dbService.FailoverAutonomousDatabase(*adb.Spec.Details.Id)
		if err != nil {
			return false, err
		}

		adb.Spec.Action = ""
		adb.Status.LifecycleState = resp.LifecycleState
		return true, nil

	case "":
		// No-op
		return false, nil
	default:
		adb.Spec.Action = ""
		return true, errors.New("Unknown action: " + adb.Spec.Action)
	}
}

func (r *AutonomousDatabaseReconciler) createAutonomousDatabase(logger logr.Logger, adb *dbv4.AutonomousDatabase) error {
	logger.WithName("createADB").Info("Sending CreateAutonomousDatabase request to OCI")
	resp, err := r.dbService.CreateAutonomousDatabase(adb)
	if err != nil {
		return err
	}

	adb.UpdateFromOciAdb(resp.AutonomousDatabase, true)

	return nil
}

// syncAutonomousDatabase retrieve the information of AutonomousDatabase from
// OCI and "overwrite" decides whether the spec and the status of "adb" will
// be overwritten.
// It will be a no-op if "Spec.Details.AutonomousDatabaseOCID" of the provided
// AutonomousDatabase is nil.
// This method does not update the actual resource in the cluster.
//
// The returned values are:
// 1. bool: indicates whether the spec is changed after the sync
// 2. error: not nil if an error occurs during the sync
func (r *AutonomousDatabaseReconciler) syncAutonomousDatabase(
	logger logr.Logger,
	adb *dbv4.AutonomousDatabase, overwrite bool) (specChanged bool, err error) {
	if adb.Spec.Details.Id == nil {
		return false, nil
	}

	l := logger.WithName("syncAutonomousDatabase")

	// Get the information from OCI
	l.Info("Sending GetAutonomousDatabase request to OCI")
	resp, err := r.dbService.GetAutonomousDatabase(*adb.Spec.Details.Id)
	if err != nil {
		return false, err
	}

	specChanged = adb.UpdateFromOciAdb(resp.AutonomousDatabase, overwrite)
	return specChanged, nil
}

// updateAutonomousDatabase returns true if an OCI request is sent.
// The AutonomousDatabase is updated with the returned object from the OCI requests.
func (r *AutonomousDatabaseReconciler) updateAutonomousDatabase(
	logger logr.Logger,
	adb *dbv4.AutonomousDatabase) (err error) {

	// Get OCI AutonomousDatabase and update the lifecycleState of the CR,
	// so that the validatexx functions know when the state changes back to AVAILABLE
	ociAdb := adb.DeepCopy()
	_, err = r.syncAutonomousDatabase(logger, ociAdb, true)
	if err != nil {
		return err
	}

	// Start update
	// difAdb is used to store ONLY the values of Autonomous Database that are
	// difference from the one in OCI
	difAdb := adb.DeepCopy()

	detailsAreChanged, err := difAdb.RemoveUnchangedDetails(ociAdb.Spec)
	if err != nil {
		return err
	}

	// Do the update request only if the current ADB is actually different from the OCI ADB
	if detailsAreChanged {
		logger.Info("Sending UpdateAutonomousDatabase request to OCI")

		resp, err := r.dbService.UpdateAutonomousDatabase(*adb.Spec.Details.Id, difAdb)
		if err != nil {
			return err
		}
		_ = adb.UpdateFromOciAdb(resp.AutonomousDatabase, true)
	}

	return nil
}

func (r *AutonomousDatabaseReconciler) validateWallet(logger logr.Logger, adb *dbv4.AutonomousDatabase) error {
	if adb.Spec.Wallet.Name == nil &&
		adb.Spec.Wallet.Password.K8sSecret.Name == nil &&
		adb.Spec.Wallet.Password.OciSecret.Id == nil {
		return nil
	}

	if adb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateProvisioning {
		return nil
	}

	l := logger.WithName("validateWallet")

	// lastSucSpec may be nil if this is the first time entering the reconciliation loop
	var walletName string

	if adb.Spec.Wallet.Name == nil {
		walletName = adb.GetName() + "-instance-wallet"
	} else {
		walletName = *adb.Spec.Wallet.Name
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
func (r *AutonomousDatabaseReconciler) syncBackupResources(logger logr.Logger, adb *dbv4.AutonomousDatabase) error {
	l := logger.WithName("syncBackupResources")

	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := k8s.FetchAutonomousDatabaseBackups(r.KubeClient, adb.Namespace, adb.Name)
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

	resp, err := r.dbService.ListAutonomousDatabaseBackups(*adb.Spec.Details.Id)
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

func (r *AutonomousDatabaseReconciler) ifBackupExists(backupSummary database.AutonomousDatabaseBackupSummary, curBackupOCIDs map[string]bool, backupList *dbv4.AutonomousDatabaseBackupList) bool {
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

// removeBackupResources remove all the AutonomousDatabasBackups that
// are associated with the adb
func (r *AutonomousDatabaseReconciler) removeBackupResources(logger logr.Logger, adb *dbv4.AutonomousDatabase) error {
	l := logger.WithName("removeBackupResources")

	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := k8s.FetchAutonomousDatabaseBackups(r.KubeClient, adb.Namespace, adb.Name)
	if err != nil {
		return err
	}

	for _, backup := range backupList.Items {
		if err := r.KubeClient.Delete(context.TODO(), &backup); err != nil {
			return err
		}
		l.Info("Delete AutonomousDatabaseBackup " + backup.Name)
	}

	return nil
}
