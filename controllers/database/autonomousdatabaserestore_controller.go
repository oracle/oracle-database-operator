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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oracle/oci-go-sdk/v63/common"
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/adb_family"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseRestoreReconciler reconciles a AutonomousDatabaseRestore object
type AutonomousDatabaseRestoreReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder

	adbService  oci.DatabaseService
	workService oci.WorkRequestService
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseRestore{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *AutonomousDatabaseRestoreReconciler) restoreStarted(restore *dbv1alpha1.AutonomousDatabaseRestore) bool {
	return restore.Status.TimeAccepted != "" ||
		(restore.Status.Status == dbv1alpha1.RestoreStatusInProgress ||
			restore.Status.Status == dbv1alpha1.RestoreStatusFailed ||
			restore.Status.Status == dbv1alpha1.RestoreStatusSucceeded)
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

	if r.restoreStarted(restore) {
		return emptyResult, nil
	}

	// ===================== Run Restore for the Target ADB ============================

	/******************************************************************
	* Look up the owner AutonomousDatabase and set the ownerReference
	* if the owner hasn't been set yet.
	******************************************************************/
	adbOCID, err := r.verifyTargetADB(restore)
	if err != nil {
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

	return emptyResult, nil
}

func (r *AutonomousDatabaseRestoreReconciler) getRestoreSDKTime(restore *dbv1alpha1.AutonomousDatabaseRestore) (*common.SDKTime, error) {
	if restore.Spec.Source.K8sADBBackup.Name != nil { // restore using backupName
		backup := &dbv1alpha1.AutonomousDatabaseBackup{}
		if err := k8s.FetchResource(r.KubeClient, restore.Namespace, *restore.Spec.Source.K8sADBBackup.Name, backup); err != nil {
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
	logger := r.Log.WithName("set-owner-reference")

	controllerutil.SetOwnerReference(adb, restore, r.Scheme)
	if err := r.KubeClient.Update(context.TODO(), restore); err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Set the owner of AutonomousDatabaseRestore %s to AutonomousDatabase %s", restore.Name, adb.Name))

	return nil
}

// verifyTargetADB searches if the target ADB is in the cluster, and set the owner reference to the ADB if it exists.
// The function returns the OCID of the target ADB.
func (r *AutonomousDatabaseRestoreReconciler) verifyTargetADB(restore *dbv1alpha1.AutonomousDatabaseRestore) (string, error) {
	// Get the target ADB OCID and the ADB resource
	ocid, ownerADB, err := adbfamily.VerifyTargetADB(r.KubeClient, restore.Spec.Target, restore.Namespace)

	if err != nil {
		return "", err
	}

	// Set the owner reference if needed
	if len(restore.GetOwnerReferences()) == 0 && ownerADB != nil {
		if err := r.setOwnerAutonomousDatabase(restore, ownerADB); err != nil {
			return "", err
		}
	}

	return ocid, nil
}

func (r *AutonomousDatabaseRestoreReconciler) updateResourceStatus(restore *dbv1alpha1.AutonomousDatabaseRestore) error {
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

	// Send event
	r.Recorder.Event(restore, corev1.EventTypeWarning, "ReconcileFailed", issue.Error())

	// Change the status to ERROR
	var combinedErr error = issue

	restore.Status.Status = dbv1alpha1.RestoreStatusError
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
	var err error

	resp, err := r.adbService.RestoreAutonomousDatabase(adbOCID, *restoreTime)
	if err != nil {
		return err
	}

	// Update status and wait for the work finish if a request is sent. Note that some of the requests (e.g. update displayName) won't return a work request ID.
	// It's important to update the status by reference otherwise Reconcile() won't be able to get the latest values
	restore.Status.DisplayName = *resp.AutonomousDatabase.DisplayName
	restore.Status.DbName = *resp.AutonomousDatabase.DbName
	restore.Status.AutonomousDatabaseOCID = *resp.AutonomousDatabase.Id

	workStart, err := r.workService.Get(*resp.OpcWorkRequestId)
	if err != nil {
		return err
	}
	restore.Status.Status = restore.ConvertWorkRequestStatus(workStart.Status)
	restore.Status.TimeAccepted = dbv1alpha1.FormatSDKTime(workStart.TimeAccepted)

	r.updateResourceStatus(restore)

	workEnd, err := r.workService.Wait(*resp.OpcWorkRequestId)
	if err != nil {
		return err
	}

	// Update status when the work is finished
	restore.Status.Status = restore.ConvertWorkRequestStatus(workEnd.Status)
	restore.Status.TimeStarted = dbv1alpha1.FormatSDKTime(workEnd.TimeStarted)
	restore.Status.TimeEnded = dbv1alpha1.FormatSDKTime(workEnd.TimeFinished)

	r.updateResourceStatus(restore)

	return nil
}
