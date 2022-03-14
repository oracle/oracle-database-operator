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
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v54/database"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/annotations"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// name of our custom finalizer
const finalizer = "database.oracle.com/oraoperator-finalizer"
var emptyResult ctrl.Result = ctrl.Result{}

// *AutonomousDatabaseReconciler reconciles a AutonomousDatabase object
type AutonomousDatabaseReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	adbService  oci.DatabaseService
	workService oci.WorkRequestService
	lastSucSpec *dbv1alpha1.AutonomousDatabaseSpec
}

// SetupWithManager function
func (r *AutonomousDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.AutonomousDatabase{}).
		WithEventFilter(predicate.And(predicate.GenerationChangedPredicate{}, r.eventFilterPredicate())).
		// WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

func (r *AutonomousDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldStatus := e.ObjectOld.(*dbv1alpha1.AutonomousDatabase).Status.LifecycleState
			desiredStatus := e.ObjectNew.(*dbv1alpha1.AutonomousDatabase).Spec.Details.LifecycleState

			if oldStatus == database.AutonomousDatabaseLifecycleStateProvisioning ||
				oldStatus == database.AutonomousDatabaseLifecycleStateUpdating ||
				oldStatus == database.AutonomousDatabaseLifecycleStateStarting ||
				oldStatus == database.AutonomousDatabaseLifecycleStateStopping ||
				oldStatus == database.AutonomousDatabaseLifecycleStateTerminating ||
				oldStatus == database.AutonomousDatabaseLifecycleStateRestoreInProgress ||
				oldStatus == database.AutonomousDatabaseLifecycleStateBackupInProgress ||
				oldStatus == database.AutonomousDatabaseLifecycleStateMaintenanceInProgress ||
				oldStatus == database.AutonomousDatabaseLifecycleStateRestarting ||
				oldStatus == database.AutonomousDatabaseLifecycleStateRecreating ||
				oldStatus == database.AutonomousDatabaseLifecycleStateRoleChangeInProgress ||
				oldStatus == database.AutonomousDatabaseLifecycleStateUpgrading {

				// Except for the case that the ADB is already terminating, we should let the terminate requests to be enqueued
				if oldStatus != database.AutonomousDatabaseLifecycleStateTerminating &&
					desiredStatus == database.AutonomousDatabaseLifecycleStateTerminated {
					return true
				}

				// All the requests other than the terminate request, should be discarded during the intermediate states
				return false
			}

			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the real object is deleted from the cluster.
			return false
		},
	}

	return pred
}

// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases/status,verbs=update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaseBackups,verbs=get;list;create;update;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=create;get;list;update
// +kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the funtion that the operator calls every time when the reconciliation loop is triggered.
// It go to the beggining of the reconcile if an error is returned. We won't return a error if it is related
// to OCI, because the issues cannot be solved by re-run the reconcile.
func (r *AutonomousDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	var err error

	// Get the autonomousdatabase instance from the cluster
	adb := &dbv1alpha1.AutonomousDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, adb); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ADB, so we don't need to update the status
		return emptyResult, err
	}

	r.lastSucSpec, err = adb.GetLastSuccessfulSpec()
	if err != nil {
		return r.manageError(adb, err)
	}

	/******************************************************************
	* Get OCI database client and work request client
	******************************************************************/
	if err := r.setupOCIClients(adb); err != nil {
		logger.Error(err, "Fail to setup OCI clients")

		return r.manageError(adb, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	* Register/unregister finalizer
	* Deletion timestamp will be added to a object before it is deleted.
	* Kubernetes server calls the clean up function if a finalizer exitsts, and won't delete the real object until
	* all the finalizers are removed from the object metadata.
	* Refer to this page for more details of using finalizers: https://kubernetes.io/blog/2021/05/14/using-finalizers-to-control-deletion/
	******************************************************************/
	isADBDeleteTrue, err := r.validateFinalizer(adb)
	if err != nil {
		return r.manageError(adb, err)
	}
	if isADBDeleteTrue {
		return emptyResult, nil
	}

	/******************************************************************
	* Determine which Database operations need to be executed by checking the changes to spec.details.
	* There are three scenario:
	* 1. provision operation. The AutonomousDatabaseOCID is missing, and the LastSucSpec annotation is missing.
	* 2. bind operation. The AutonomousDatabaseOCID is provided, but the LastSucSpec annotation is missing.
	* 3. update operation. Every changes other than the above two cases goes here.
	* Afterwards, update the resource from the remote database in OCI. This step will be executed right after
	* the above three cases during every reconcile.
	/******************************************************************/
	// difADB is nil when the action is PROVISION or BIND.
	// Use difADB to identify which fields are updated when it's UPDATE operation.
	action, difADB, err := r.determineAction(adb)
	if err != nil {
		return r.manageError(adb, err)
	}

	switch action {
	case adbActionProvision:
		if err := r.createADB(adb); err != nil {
			return r.manageError(adb, err)
		}

		if err := r.downloadWallet(adb); err != nil {
			return r.manageError(adb, err)
		}
	case adbActionBind:
		fallthrough
	case adbActionSync:
		err = r.syncResource(adb)
		if err != nil {
			return r.manageError(adb, err)
		}

		if err := r.downloadWallet(adb); err != nil {
			return r.manageError(adb, err)
		}
	case adbActionUpdate:
		if err := r.updateADB(adb, difADB); err != nil {
			return r.manageError(adb, err)
		}

		err = r.syncResource(adb)
		if err != nil {
			return r.manageError(adb, err)
		}
	}

	/*****************************************************
	*	Sync AutonomousDatabase Backups from OCI
	*****************************************************/
	if err := r.syncBackupResources(adb); err != nil {
		return r.manageError(adb, err)
	}

	/*****************************************************
	*	Update the lastSucSpec
	*****************************************************/
	if err := r.updateLastSuccessfulSpec(adb); err != nil {
		return r.manageError(adb, err)
	}

	logger.Info("AutonomousDatabase reconciles successfully")

	return emptyResult, nil
}

func (r *AutonomousDatabaseReconciler) setupOCIClients(adb *dbv1alpha1.AutonomousDatabase) error {
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

	r.adbService, err = oci.NewDatabaseService(r.Log, r.KubeClient, provider)
	if err != nil {
		return err
	}

	r.workService, err = oci.NewWorkRequestService(r.Log, r.KubeClient, provider)
	if err != nil {
		return err
	}

	return nil
}

func (r *AutonomousDatabaseReconciler) manageError(adb *dbv1alpha1.AutonomousDatabase, issue error) (ctrl.Result, error) {
	// Rollback if lastSucSpec exists
	if r.lastSucSpec != nil {
		// Send event
		r.Recorder.Event(adb, corev1.EventTypeWarning, "ReconcileIncompleted", issue.Error())

		var finalIssue = issue

		if err := r.syncResource(adb); err != nil {
			finalIssue = k8s.CombineErrors(finalIssue, err)
		}

		if err := r.updateLastSuccessfulSpec(adb); err != nil {
			finalIssue = k8s.CombineErrors(finalIssue, err)
		}

		// r.updateResource has already triggered another Reconcile loop, so we simply log the error and return a nil
		return emptyResult, finalIssue
	} else {
		// Send event
		r.Recorder.Event(adb, corev1.EventTypeWarning, "CreateFailed", issue.Error())

		return emptyResult, issue
	}
}

func (r *AutonomousDatabaseReconciler) validateFinalizer(adb *dbv1alpha1.AutonomousDatabase) (isADBDeleteTrue bool, err error) {
	logger := r.Log.WithName("finalizer")

	if adb.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted
		if *adb.Spec.HardLink && !k8s.HasFinalizer(adb, finalizer) {
			if err := k8s.Register(r.KubeClient, adb, finalizer); err != nil {
				return false, err
			}
			logger.Info("Finalizer registered successfully.")

		} else if !*adb.Spec.HardLink && !k8s.HasFinalizer(adb, finalizer) {
			if err := k8s.Unregister(r.KubeClient, adb, finalizer); err != nil {
				return false, err
			}
			logger.Info("Finalizer unregistered successfully.")
		}
		return false, nil
	} else {
		// The object is being deleted
		if adb.Spec.Details.AutonomousDatabaseOCID == nil {
			logger.Info("Autonomous Database OCID is missing. Remove the resource only.")
		} else if adb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminating &&
			adb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminated {
			// Don't send terminate request if the database is terminating or already terminated
			logger.Info("Terminate Autonomous Database: " + *adb.Spec.Details.DbName)

			if err := r.deleteAutonomousDatabase(adb); err != nil {
				return true, err
			}
		}

		if err := k8s.Unregister(r.KubeClient, adb, finalizer); err != nil {
			return true, err
		}
		logger.Info("Finalizer unregistered successfully.")
		// Stop reconciliation as the item is being deleted
		return true, nil
	}
}

func (r *AutonomousDatabaseReconciler) updateLastSuccessfulSpec(adb *dbv1alpha1.AutonomousDatabase) error {
	specBytes, err := json.Marshal(adb.Spec)
	if err != nil {
		return err
	}

	anns := map[string]string{
		dbv1alpha1.LastSuccessfulSpec: string(specBytes),
	}

	return annotations.SetAnnotations(r.KubeClient, adb, anns)
}

// A helper function to wait until the work completes after an update/start/stop/delete AutonomousDatabase request is sent
// , and then sync the resource to make sure everything is updated when the work is finished
func (r *AutonomousDatabaseReconciler) wait(adb *dbv1alpha1.AutonomousDatabase, opcWorkRequestId string) error {
	// Wait until the work is finished
	if _, err := r.workService.Wait(opcWorkRequestId); err != nil {
		return err
	}

	if err := r.syncResource(adb); err != nil {
		return err
	}

	return nil
}

// updateResource updates the specification, the status of AutonomousDatabase resource, and the lastSucSpec
func (r *AutonomousDatabaseReconciler) updateResource(adb *dbv1alpha1.AutonomousDatabase) error {
	// Update the status first to prevent unwanted Reconcile()
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	// Update the spec
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curADB := &dbv1alpha1.AutonomousDatabase{}

		namespacedName := types.NamespacedName{
			Namespace: adb.GetNamespace(),
			Name:      adb.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curADB); err != nil {
			return err
		}

		curADB.Spec.Details = adb.Spec.Details
		return r.KubeClient.Update(context.TODO(), curADB)
	}); err != nil {
		return err
	}

	return nil
}

// updateResourceStatus updates only the status of the resource, not including the lastSucSpec.
// This function should not be called by the functions associated with the OCI update requests.
// The OCI update requests should use updateResource() to ensure all the spec, resource and the
// lastSucSpec are updated.
func (r *AutonomousDatabaseReconciler) updateResourceStatus(adb *dbv1alpha1.AutonomousDatabase) error {
	return k8s.UpdateADBStatus(r.KubeClient, adb)
}

func (r *AutonomousDatabaseReconciler) syncResource(adb *dbv1alpha1.AutonomousDatabase) error {
	// Get the information from OCI
	resp, err := r.adbService.GetAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.UpdateFromOCIADB(resp.AutonomousDatabase)

	if err := r.updateResource(adb); err != nil {
		return err
	}

	return nil
}

type ADBActionEnum string

const (
	adbActionProvision = "PROVISION"
	adbActionBind      = "BIND"
	adbActionUpdate    = "UPDATE"
	adbActionSync      = "SYNC"
)

func (r *AutonomousDatabaseReconciler) determineAction(adb *dbv1alpha1.AutonomousDatabase) (ADBActionEnum, *dbv1alpha1.AutonomousDatabase, error) {
	if r.lastSucSpec == nil {
		if adb.Spec.Details.AutonomousDatabaseOCID == nil {
			return adbActionProvision, nil, nil
		} else {
			return adbActionBind, nil, nil
		}
	} else {
		// Pre-process step for the UPDATE. Remove the unchanged fields from spec.details,
		difADB := adb.DeepCopy()
		detailsChanged, err := difADB.RemoveUnchangedDetails()
		if err != nil {
			return "", nil, err
		}

		if detailsChanged {
			return adbActionUpdate, difADB, nil
		}

		return adbActionSync, nil, nil
	}
}

func (r *AutonomousDatabaseReconciler) createADB(adb *dbv1alpha1.AutonomousDatabase) error {
	resp, err := r.adbService.CreateAutonomousDatabase(adb)
	if err != nil {
		return err
	}

	// Update the ADB OCID and the status
	adb.Spec.Details.AutonomousDatabaseOCID = resp.AutonomousDatabase.Id
	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResource(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateGeneralFields(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.DisplayName == nil &&
		difADB.Spec.Details.DbName == nil &&
		difADB.Spec.Details.DbVersion == nil &&
		difADB.Spec.Details.FreeformTags == nil {
		return nil
	}

	resp, err := r.adbService.UpdateAutonomousDatabaseGeneralFields(difADB)
	if err != nil {
		return err
	}

	// If the OpcWorkRequestId is nil (such as when the displayName is changed),
	// no need to update the resource and wail until the work is done
	if resp.OpcWorkRequestId == nil {
		return nil
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateAdminPassword(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.AdminPassword.K8sSecretName == nil &&
		difADB.Spec.Details.AdminPassword.OCISecretOCID == nil {
		return nil
	}

	_, err := r.adbService.UpdateAutonomousDatabaseAdminPassword(difADB)
	if err != nil {
		return err
	}

	// UpdateAdminPassword request doesn't return the workrequest ID
	return nil
}

func (r *AutonomousDatabaseReconciler) updateDbWorkload(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.DbWorkload == "" {
		return nil
	}

	resp, err := r.adbService.UpdateAutonomousDatabaseDBWorkload(difADB)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateLicenseModel(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.LicenseModel == "" {
		return nil
	}

	resp, err := r.adbService.UpdateAutonomousDatabaseLicenseModel(difADB)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateScalingFields(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.DataStorageSizeInTBs == nil &&
		difADB.Spec.Details.CPUCoreCount == nil &&
		difADB.Spec.Details.IsAutoScalingEnabled == nil {
		return nil
	}

	resp, err := r.adbService.UpdateAutonomousDatabaseScalingFields(difADB)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) deleteAutonomousDatabase(adb *dbv1alpha1.AutonomousDatabase) error {

	resp, err := r.adbService.DeleteAutonomousDatabase(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateTerminating

	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateLifecycleState(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.LifecycleState == "" {
		return nil
	}

	var opcWorkRequestId string

	switch difADB.Spec.Details.LifecycleState {
	case database.AutonomousDatabaseLifecycleStateAvailable:
		resp, err := r.adbService.StartAutonomousDatabase(*difADB.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return err
		}

		adb.Status.LifecycleState = resp.LifecycleState
		opcWorkRequestId = *resp.OpcWorkRequestId
	case database.AutonomousDatabaseLifecycleStateStopped:
		resp, err := r.adbService.StopAutonomousDatabase(*difADB.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return err
		}

		adb.Status.LifecycleState = resp.LifecycleState
		opcWorkRequestId = *resp.OpcWorkRequestId
	case database.AutonomousDatabaseLifecycleStateTerminated:
		resp, err := r.adbService.DeleteAutonomousDatabase(*difADB.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return err
		}

		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateTerminating
		opcWorkRequestId = *resp.OpcWorkRequestId
	default:
		return errors.New("Unknown state")
	}

	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, opcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) setMTLSRequired(adb *dbv1alpha1.AutonomousDatabase) error {
	resp, err := r.adbService.UpdateNetworkAccessMTLSRequired(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateMTLS(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired == nil {
		return nil
	}

	resp, err := r.adbService.UpdateNetworkAccessMTLS(difADB)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) setNetworkAccessPublic(adb *dbv1alpha1.AutonomousDatabase) error {
	resp, err := r.adbService.UpdateNetworkAccessPublic(r.lastSucSpec.Details.NetworkAccess.AccessType, *adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

func (r *AutonomousDatabaseReconciler) updateNetworkAccess(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.NetworkAccess.AccessType == "" &&
		difADB.Spec.Details.NetworkAccess.IsAccessControlEnabled == nil &&
		difADB.Spec.Details.NetworkAccess.AccessControlList == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix == nil {
		return nil
	}
	resp, err := r.adbService.UpdateNetworkAccess(difADB)
	if err != nil {
		return err
	}

	adb.UpdateStatusFromOCIADB(resp.AutonomousDatabase)
	if err := r.updateResourceStatus(adb); err != nil {
		return err
	}

	return r.wait(adb, *resp.OpcWorkRequestId)
}

// The logic of updating the network access configurations is as follows:
// 1. Shared databases:
// 	 If the network access type changes
//   a. to PUBLIC:
//     was RESTRICTED: re-enable IsMTLSConnectionRequired if its not. Then set WhitelistedIps to an array with a single empty string entry.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if its not. Then set PrivateEndpointLabel to an emtpy string.
//   b. to RESTRICTED:
//     was PUBLIC: set WhitelistedIps to desired IPs/CIDR blocks/VCN OCID. Configure the IsMTLSConnectionRequired settings if it is set to disabled.
//     was PRIVATE: re-enable IsMTLSConnectionRequired if its not. Set the type to PUBLIC first, and then configure the WhitelistedIps. Finally resume the IsMTLSConnectionRequired settings if it was, or is configured as disabled.
//   c. to PRIVATE:
//     was PUBLIC: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//     was RESTRICTED: set subnetOCID and nsgOCIDs. Configure the IsMTLSConnectionRequired settings if it is set.
//
// 	 Otherwise, if the network access type remains the same, apply the network configuration, and then set the IsMTLSConnectionRequired.
//
// 2. Dedicated databases:
//   Apply the configs directly
func (r *AutonomousDatabaseReconciler) determineNetworkAccessUpdate(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if difADB.Spec.Details.NetworkAccess.AccessType == "" &&
		difADB.Spec.Details.NetworkAccess.IsAccessControlEnabled == nil &&
		difADB.Spec.Details.NetworkAccess.AccessControlList == nil &&
		difADB.Spec.Details.NetworkAccess.IsMTLSConnectionRequired == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil &&
		difADB.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix == nil {
		return nil
	}

	if !*adb.Spec.Details.IsDedicated {
		var lastAccessType = r.lastSucSpec.Details.NetworkAccess.AccessType
		var difAccessType = difADB.Spec.Details.NetworkAccess.AccessType

		if difAccessType != "" {
			switch difAccessType {
			case dbv1alpha1.NetworkAccessTypePublic:
				// OCI validation requires IsMTLSConnectionRequired to be enabled before changing the network access type to PUBLIC
				if !*r.lastSucSpec.Details.NetworkAccess.IsMTLSConnectionRequired {
					if err := r.setMTLSRequired(adb); err != nil {
						return err
					}
				}

				if err := r.setNetworkAccessPublic(adb); err != nil {
					return err
				}
			case dbv1alpha1.NetworkAccessTypeRestricted:
				// If the access type was PRIVATE, then OCI validation requires IsMTLSConnectionRequired
				// to be enabled before setting ACL. Also we can only change the network access type from
				// PRIVATE to PUBLIC.
				if lastAccessType == dbv1alpha1.NetworkAccessTypePrivate {
					if !*r.lastSucSpec.Details.NetworkAccess.IsMTLSConnectionRequired {
						var oldMTLS bool = *adb.Spec.Details.NetworkAccess.IsMTLSConnectionRequired
						if err := r.setMTLSRequired(adb); err != nil {
							return err
						}
						// restore IsMTLSConnectionRequired
						adb.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = &oldMTLS
					}

					if err := r.setNetworkAccessPublic(adb); err != nil {
						return err
					}
				}

				if err := r.updateNetworkAccess(adb, difADB); err != nil {
					return err
				}

				if err := r.updateMTLS(adb, difADB); err != nil {
					return err
				}
			case dbv1alpha1.NetworkAccessTypePrivate:
				if err := r.updateNetworkAccess(adb, difADB); err != nil {
					return err
				}

				if err := r.updateMTLS(adb, difADB); err != nil {
					return err
				}
			}
		} else {
			// Access type doesn't change
			if err := r.updateNetworkAccess(adb, difADB); err != nil {
				return err
			}

			if err := r.updateMTLS(adb, difADB); err != nil {
				return err
			}
		}
	} else {
		// Dedicated database
		if err := r.updateNetworkAccess(adb, difADB); err != nil {
			return err
		}
	}

	return nil
}

func (r *AutonomousDatabaseReconciler) downloadWallet(adb *dbv1alpha1.AutonomousDatabase) error {
	if adb.Spec.Details.Wallet.Name == nil &&
		adb.Spec.Details.Wallet.Password.K8sSecretName == nil &&
		adb.Spec.Details.Wallet.Password.OCISecretOCID == nil {
		return nil
	}

	logger := r.Log.WithName("download-wallet")

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
			logger.Info("wallet existed but has a different label; skip the download")
		}
		// No-op if Wallet is already downloaded
		return nil
	} else if !apiErrors.IsNotFound(err) {
		return err
	}

	resp, err := r.adbService.DownloadWallet(adb)
	if err != nil {
		return err
	}

	data, err := oci.ExtractWallet(resp.Content)
	if err != nil {
		return err
	}

	label := map[string]string{"app": adb.GetName()}

	if err := k8s.CreateSecret(r.KubeClient, adb.Namespace, walletName, data, adb, label); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Wallet is stored in the Secret %s", walletName))

	return nil
}

func (r *AutonomousDatabaseReconciler) getValidBackupName(name string, usedNames map[string]bool) string {
	returnedName := name
	var i = 1

	_, ok := usedNames[returnedName]
	for ok {
		returnedName = fmt.Sprintf("%s-%d", name, i)
		_, ok = usedNames[returnedName]
		i++
	}

	return returnedName
}

func (r *AutonomousDatabaseReconciler) updateADB(adb *dbv1alpha1.AutonomousDatabase, difADB *dbv1alpha1.AutonomousDatabase) error {
	if err := r.updateGeneralFields(adb, difADB); err != nil {
		return err
	}

	if err := r.updateAdminPassword(adb, difADB); err != nil {
		return err
	}

	if err := r.updateDbWorkload(adb, difADB); err != nil {
		return err
	}

	if err := r.updateLicenseModel(adb, difADB); err != nil {
		return err
	}

	if err := r.updateScalingFields(adb, difADB); err != nil {
		return err
	}

	if err := r.determineNetworkAccessUpdate(adb, difADB); err != nil {
		return err
	}

	if err := r.updateLifecycleState(adb, difADB); err != nil {
		return err
	}

	if err := r.downloadWallet(adb); err != nil {
		return err
	}

	return nil
}

// updateBackupResources get the list of AutonomousDatabasBackups and
// create a backup object if it's not found in the same namespace
func (r *AutonomousDatabaseReconciler) syncBackupResources(adb *dbv1alpha1.AutonomousDatabase) error {
	logger := r.Log.WithName("update-backups")

	// Get the list of AutonomousDatabaseBackupOCID in the same namespace
	backupList, err := k8s.FetchAutonomousDatabaseBackups(r.KubeClient, adb.Namespace)
	if err != nil {
		return err
	}

	curBackupNames := make(map[string]bool)
	curBackupOCIDs := make(map[string]bool)

	for _, backup := range backupList.Items {
		curBackupNames[backup.Name] = true

		if backup.Spec.AutonomousDatabaseBackupOCID != "" {
			curBackupOCIDs[backup.Spec.AutonomousDatabaseBackupOCID] = true
		}
	}

	resp, err := r.adbService.ListAutonomousDatabaseBackups(*adb.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	for _, backupSummary := range resp.Items {
		// Create the resource if the AutonomousDatabaseBackupOCID doesn't exist
		_, ok := curBackupOCIDs[*backupSummary.Id]
		if !ok {
			// Convert the string to lowercase, and replace spaces, commas, and colons with hyphens
			resourceName := *backupSummary.DisplayName
			resourceName = strings.ToLower(resourceName)

			re, err := regexp.Compile(`[^-a-zA-Z0-9]`)
			if err != nil {
				return err
			}
			resourceName = re.ReplaceAllString(resourceName, "-")
			resourceName = r.getValidBackupName(resourceName, curBackupNames)

			if err := k8s.CreateAutonomousBackup(r.KubeClient, resourceName, backupSummary, adb); err != nil {
				return err
			}

			// Add the used names and ocids
			curBackupNames[resourceName] = true
			curBackupOCIDs[*backupSummary.AutonomousDatabaseId] = true

			logger.Info("Create AutonomousDatabaseBackup " + resourceName)
		}
	}

	return nil
}
