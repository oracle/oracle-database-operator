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
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/adb_family"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseBackupReconciler reconciles a AutonomousDatabaseBackup object
type AutonomousDatabaseBackupReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	adbService  oci.DatabaseService
	workService oci.WorkRequestService
}

func (r *AutonomousDatabaseBackupReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldStatus := e.ObjectOld.(*dbv1alpha1.AutonomousDatabaseBackup).Status.LifecycleState

			if oldStatus == dbv1alpha1.BackupStateCreating ||
				oldStatus == dbv1alpha1.BackupStateDeleting {
				return false
			}

			return true
		},
	}

	return pred
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseBackup{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

func (r *AutonomousDatabaseBackupReconciler) backupStarted(backup *dbv1alpha1.AutonomousDatabaseBackup) bool {
	return backup.Spec.AutonomousDatabaseBackupOCID != nil ||
		(backup.Status.LifecycleState == dbv1alpha1.BackupStateCreating ||
			backup.Status.LifecycleState == dbv1alpha1.BackupStateActive ||
			backup.Status.LifecycleState == dbv1alpha1.BackupStateDeleting ||
			backup.Status.LifecycleState == dbv1alpha1.BackupStateDeleted ||
			backup.Status.LifecycleState == dbv1alpha1.BackupStateFailed)
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabasebackups,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabasebackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabases,verbs=get;list

func (r *AutonomousDatabaseBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	backup := &dbv1alpha1.AutonomousDatabaseBackup{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, backup); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return emptyResult, nil
		}
		// Failed to get ADBBackup, so we don't need to update the status
		return emptyResult, err
	}

	/******************************************************************
	* Look up the owner AutonomousDatabase and set the ownerReference
	* if the owner hasn't been set yet.
	******************************************************************/
	adbOCID, err := r.verifyTargetADB(backup)
	if err != nil {
		return r.manageError(backup, err)
	}

	/******************************************************************
	* Get OCI database client and work request client
	******************************************************************/
	if err := r.setupOCIClients(backup); err != nil {
		return r.manageError(backup, err)
	}

	logger.Info("OCI clients configured succesfully")

	/******************************************************************
	 * If the Spec.AutonomousDatabaseBackupOCID is empty and the LifecycleState is never assigned , create a backup.
	 * LifecycleState is checked to avoid sending a duplicated backup request when the backup is creating.
	 * Otherwise, bind to an exisiting backup if the  Spec.AutonomousDatabaseBackupOCID isn't empty.
	 ******************************************************************/
	if !r.backupStarted(backup) {
		// Create a new backup
		backupResp, err := r.adbService.CreateAutonomousDatabaseBackup(backup, adbOCID)
		if err != nil {
			return r.manageError(backup, err)
		}

		adbResp, err := r.adbService.GetAutonomousDatabase(*backupResp.AutonomousDatabaseId)
		if err != nil {
			return r.manageError(backup, err)
		}

		// update the Backup status
		backup.UpdateStatusFromOCIBackup(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
		if err := r.KubeClient.Status().Update(context.TODO(), backup); err != nil {
			return r.manageError(backup, err)
		}

		// Wait until the work is done
		if _, err := r.workService.Wait(*backupResp.OpcWorkRequestId); err != nil {
			return r.manageError(backup, err)
		}

		logger.Info("AutonomousDatabaseBackup " + *backupResp.DisplayName + " created successfully")
	}

	/******************************************************************
	* Sync the resource status
	*******************************************************************/
	// get the backup ID
	var backupID string
	if backup.Spec.AutonomousDatabaseBackupOCID != nil {
		backupID = *backup.Spec.AutonomousDatabaseBackupOCID
	} else if backup.Status.AutonomousDatabaseBackupOCID != "" {
		backupID = backup.Status.AutonomousDatabaseBackupOCID
	} else {
		// Send the event and exit the Reconcile
		err := errors.New("the backup is incomplete and missing the OCID; the resource should be removed")
		logger.Error(err, "Reconcile stopped")
		r.Recorder.Event(backup, corev1.EventTypeWarning, "ReconcileFailed", err.Error())
		return emptyResult, nil
	}

	backupResp, err := r.adbService.GetAutonomousDatabaseBackup(backupID)
	if err != nil {
		return r.manageError(backup, err)
	}

	adbResp, err := r.adbService.GetAutonomousDatabase(*backupResp.AutonomousDatabaseId)
	if err != nil {
		return r.manageError(backup, err)
	}

	backup.UpdateStatusFromOCIBackup(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
	if err := r.KubeClient.Status().Update(context.TODO(), backup); err != nil {
		return r.manageError(backup, err)
	}

	return emptyResult, nil
}

// setOwnerAutonomousDatabase sets the owner of the AutonomousDatabaseBackup if the AutonomousDatabase resource with the same database OCID is found
func (r *AutonomousDatabaseBackupReconciler) setOwnerAutonomousDatabase(backup *dbv1alpha1.AutonomousDatabaseBackup, adb *dbv1alpha1.AutonomousDatabase) error {
	logger := r.Log.WithName("set-owner-reference")

	controllerutil.SetOwnerReference(adb, backup, r.Scheme)
	if err := r.KubeClient.Update(context.TODO(), backup); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Set the owner of AutonomousDatabaseBackup %s to AutonomousDatabase %s", backup.Name, adb.Name))

	return nil
}

// verifyTargetADB searches if the target ADB is in the cluster, and set the owner reference to the ADB if it exists.
// The function returns the OCID of the target ADB.
func (r *AutonomousDatabaseBackupReconciler) verifyTargetADB(backup *dbv1alpha1.AutonomousDatabaseBackup) (string, error) {
	// Get the target ADB OCID and the ADB resource
	ocid, ownerADB, err := adbfamily.VerifyTargetADB(r.KubeClient, backup.Spec.Target, backup.Namespace)

	if err != nil {
		return "", err
	}

	// Set the owner reference if needed
	if len(backup.GetOwnerReferences()) == 0 && ownerADB != nil {
		if err := r.setOwnerAutonomousDatabase(backup, ownerADB); err != nil {
			return "", err
		}
	}

	return ocid, nil
}

func (r *AutonomousDatabaseBackupReconciler) setupOCIClients(backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	var err error

	authData := oci.APIKeyAuth{
		ConfigMapName: backup.Spec.OCIConfig.ConfigMapName,
		SecretName:    backup.Spec.OCIConfig.SecretName,
		Namespace:     backup.GetNamespace(),
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

func (r *AutonomousDatabaseBackupReconciler) manageError(backup *dbv1alpha1.AutonomousDatabaseBackup, issue error) (ctrl.Result, error) {
	// Send event
	r.Recorder.Event(backup, corev1.EventTypeWarning, "ReconcileFailed", issue.Error())

	// Change the status to ERROR
	backup.Status.LifecycleState = dbv1alpha1.BackupStateError
	if statusErr := r.KubeClient.Status().Update(context.TODO(), backup); statusErr != nil {
		return emptyResult, k8s.CombineErrors(issue, statusErr)
	}

	return emptyResult, issue
}
