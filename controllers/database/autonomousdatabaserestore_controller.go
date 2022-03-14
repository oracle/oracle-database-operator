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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/workrequests"
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseRestoreReconciler reconciles a AutonomousDatabaseRestore object
type AutonomousDatabaseRestoreReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme

	adbService  oci.DatabaseService
	workService oci.WorkRequestService
	ownerADB    *dbv1alpha1.AutonomousDatabase
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseRestore{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaserestores,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaserestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AutonomousDatabaseRestore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *AutonomousDatabaseRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	restore := &dbv1alpha1.AutonomousDatabaseRestore{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, restore); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ADBRestore, so we don't need to update the status
		return emptyResult, err
	}

	if restore.Status.Status != "" {
		return emptyResult, nil
	}

	// ===================== Run Restore for the Target ADB ============================

	/******************************************************************
	* Look up the owner AutonomousDatabase and set the ownerReference
	* if the owner hasn't been set yet.
	******************************************************************/
	if err := r.verifyOwnerADB(restore); err != nil {
		return r.manageError(restore, err)
	}

	/******************************************************************
	* Extract the restoreTime from the spec
	******************************************************************/
	restoreTime, err := r.getRestoreSDKTime(restore)
	if err != nil {
		return r.manageError(restore, err)
	}

	/******************************************************************
	* Get the target ADB OCID
	******************************************************************/
	adbOCID, err := r.getADBOCID(restore)
	if err != nil {
		return r.manageError(restore, err)
	}

	/******************************************************************
	* Get OCI database client and work request client
	******************************************************************/
	if err := r.setupOCIClients(restore); err != nil {
		return r.manageError(restore, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	 * Start restore
	 ******************************************************************/
	if err := r.restoreAutonomousDatabase(restore, restoreTime, adbOCID); err != nil {
		return r.manageError(restore, err)
	}

	logger.Info("AutonomousDatabaseRestore reconciles successfully")

	return ctrl.Result{}, nil
}

func (r *AutonomousDatabaseRestoreReconciler) getRestoreSDKTime(restore *dbv1alpha1.AutonomousDatabaseRestore) (*common.SDKTime, error) {
	if restore.Spec.Source.AutonomousDatabaseBackup.Name != "" { // restore using backupName
		backup := &dbv1alpha1.AutonomousDatabaseBackup{}
		if err := k8s.FetchResource(r.KubeClient, restore.Namespace, restore.Spec.Source.AutonomousDatabaseBackup.Name, backup); err != nil {
			return nil, err
		}

		if backup.Status.TimeEnded == "" {
			return nil, errors.New("broken backup: ended time is missing from the AutonomousDatabaseBackup " + backup.GetName())
		}
		restoreTime, err := backup.GetTimeEnded()
		if err != nil {
			return nil, err
		}

		return restoreTime, nil

	} else { // PIT restore
		// The validation of the pitr.timestamp has been handled by the webhook, so the error return is ignored
		restoreTime, _ := restore.GetPIT()
		return restoreTime, nil
	}
}

// setOwnerAutonomousDatabase sets the owner of the AutonomousDatabaseBackup if the AutonomousDatabase resource with the same database OCID is found
func (r *AutonomousDatabaseRestoreReconciler) setOwnerAutonomousDatabase(restore *dbv1alpha1.AutonomousDatabaseRestore, adb *dbv1alpha1.AutonomousDatabase) error {
	logger := r.Log.WithName("set-owner")

	restore.SetOwnerReferences(k8s.NewOwnerReference(adb))

	if err := r.KubeClient.Update(context.TODO(), restore); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Set the owner of %s to %s", restore.Name, adb.Name))

	return nil
}

func (r *AutonomousDatabaseRestoreReconciler) verifyOwnerADB(restore *dbv1alpha1.AutonomousDatabaseRestore) error {
	if len(restore.GetOwnerReferences()) == 0 {
		var err error

		if restore.Spec.TargetADB.Name != "" {
			r.ownerADB, err = k8s.FetchAutonomousDatabase(r.KubeClient, restore.Namespace, restore.Spec.TargetADB.Name)
			if err != nil {
				return err
			}
		} else {
			r.ownerADB, err = k8s.FetchAutonomousDatabaseWithOCID(r.KubeClient, restore.Namespace, restore.Spec.TargetADB.OCID)
			if err != nil {
				return err
			}
		}

		if r.ownerADB == nil {
			if err := r.setOwnerAutonomousDatabase(restore, r.ownerADB); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *AutonomousDatabaseRestoreReconciler) getADBOCID(restore *dbv1alpha1.AutonomousDatabaseRestore) (string, error) {
	if r.ownerADB != nil {
		return *r.ownerADB.Spec.Details.AutonomousDatabaseOCID, nil
	} else {
		backup := &dbv1alpha1.AutonomousDatabaseBackup{}
		if err := k8s.FetchResource(r.KubeClient, restore.Namespace, restore.Spec.Source.AutonomousDatabaseBackup.Name, backup); err != nil {
			return "", err
		}
		return backup.Status.AutonomousDatabaseOCID, nil
	}
}

func (r *AutonomousDatabaseRestoreReconciler) updateResourceStatus(restore *dbv1alpha1.AutonomousDatabaseRestore) error {
	// sync the ADB status every time when the Backup status is updated
	if err := r.syncADBStatus(); err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseRestore{}

		namespacedName := types.NamespacedName{
			Namespace: restore.GetNamespace(),
			Name:      restore.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Status = restore.Status
		return r.KubeClient.Status().Update(context.TODO(), curBackup)
	})
}

// No-op if the ownerADB is not assigned
func (r *AutonomousDatabaseRestoreReconciler) syncADBStatus() error {
	if r.ownerADB == nil {
		return nil
	}

	resp, err := r.adbService.GetAutonomousDatabase(*r.ownerADB.Spec.Details.AutonomousDatabaseOCID)
	if err != nil {
		return err
	}

	r.ownerADB.Status.LifecycleState = resp.LifecycleState

	if err := k8s.UpdateADBStatus(r.KubeClient, r.ownerADB); err != nil {
		return err
	}

	return nil
}

func (r *AutonomousDatabaseRestoreReconciler) setupOCIClients(restore *dbv1alpha1.AutonomousDatabaseRestore) error {
	var err error

	authData := oci.APIKeyAuth{
		ConfigMapName: restore.Spec.OCIConfig.ConfigMapName,
		SecretName:    restore.Spec.OCIConfig.SecretName,
		Namespace:     restore.GetNamespace(),
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

// manageError doesn't return the error so that the request won't be requeued
func (r *AutonomousDatabaseRestoreReconciler) manageError(restore *dbv1alpha1.AutonomousDatabaseRestore, issue error) (ctrl.Result, error) {
	nsn := types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Name,
	}
	logger := r.Log.WithValues("Namespaced/Name", nsn)

	// Change the status to FAILED
	var combinedErr error = issue

	restore.Status.Status = dbv1alpha1.RestoreStatusFailed
	if statusErr := r.updateResourceStatus(restore); statusErr != nil {
		combinedErr = k8s.CombineErrors(issue, statusErr)
	}

	logger.Error(combinedErr, "Fail to restore Autonomous Database")

	return emptyResult, nil
}

func (r *AutonomousDatabaseRestoreReconciler) restoreAutonomousDatabase(
	restore *dbv1alpha1.AutonomousDatabaseRestore,
	restoreTime *common.SDKTime,
	adbOCID string) error {

	resp, err := r.adbService.RestoreAutonomousDatabase(adbOCID, *restoreTime)
	if err != nil {
		return err
	}

	// Update status and wait for the work finish if a request is sent. Note that some of the requests (e.g. update displayName) won't return a work request ID.
	// It's important to update the status by reference otherwise Reconcile() won't be able to get the latest values
	restore.Status.DisplayName = *resp.AutonomousDatabase.DisplayName
	restore.Status.DbName = *resp.AutonomousDatabase.DbName
	restore.Status.AutonomousDatabaseOCID = *resp.AutonomousDatabase.Id
	restore.Status.Status = restore.ConvertWorkRequestStatus(workrequests.WorkRequestStatusEnum(resp.LifecycleState))

	r.updateResourceStatus(restore)

	workStatus, err := r.workService.Wait(*resp.OpcWorkRequestId)
	if err != nil {
		return err
	}

	// Update status when the work is finished
	restore.Status.Status = restore.ConvertWorkRequestStatus(workStatus)
	r.updateResourceStatus(restore)

	return nil
}