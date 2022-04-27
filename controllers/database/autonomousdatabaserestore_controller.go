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

	dbService   oci.DatabaseService
	workService oci.WorkRequestService
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
	logger := r.Log.WithValues("Namespace/Name", req.NamespacedName)

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
	* Get status from OCI WorkRequest
	******************************************************************/
	if restore.Status.WorkRequestOCID != "" {
		resp, err := r.workService.Get(restore.Status.WorkRequestOCID)
		if err != nil {
			return r.manageError(restore, err)
		}

		restore.Status.Status = resp.Status

		if dbv1alpha1.IsRestoreIntermediateState(resp.Status) {
			logger.WithName("validateStatus").Info("Reconcile queued")
			return requeueResult, nil
		}
	}

	/******************************************************************
	 * Start the restore or update the status
	 ******************************************************************/
	if restore.Status.WorkRequestOCID == "" {
		// Start restore
		adbResp, err := r.dbService.RestoreAutonomousDatabase(adbOCID, *restoreTime)
		if err != nil {
			return r.manageError(restore, err)
		}

		workResp, err := r.workService.Get(*adbResp.OpcWorkRequestId)
		if err != nil {
			return r.manageError(restore, err)
		}

		restore.UpdateStatus(adbResp.AutonomousDatabase, workResp)
		if err := r.KubeClient.Update(context.TODO(), restore); err != nil {
			return r.manageError(restore, err)
		}

	} else {
		// Update the status
		adbResp, err := r.dbService.GetAutonomousDatabase(adbOCID)
		if err != nil {
			return r.manageError(restore, err)
		}

		workResp, err := r.workService.Get(restore.Status.WorkRequestOCID)
		if err != nil {
			return r.manageError(restore, err)
		}

		restore.UpdateStatus(adbResp.AutonomousDatabase, workResp)
		if err := r.KubeClient.Update(context.TODO(), restore); err != nil {
			return r.manageError(restore, err)
		}

		if err := r.KubeClient.Update(context.TODO(), restore); err != nil {
			return r.manageError(restore, err)
		}
	}

	// Requeue if it's in intermediate state
	if dbv1alpha1.IsRestoreIntermediateState(restore.Status.Status) {
		logger.WithName("validateStatus").Info("Reconcile queued")
		return requeueResult, nil
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
	ownerADB, err := adbfamily.VerifyTargetADB(r.KubeClient, restore.Spec.Target, restore.Namespace)

	if err != nil {
		return "", err
	}

	// Set the owner reference if needed
	if len(restore.GetOwnerReferences()) == 0 && ownerADB != nil {
		if err := r.setOwnerAutonomousDatabase(restore, ownerADB); err != nil {
			return "", err
		}
	}

	if restore.Spec.Target.OCIADB.OCID != nil {
		return *restore.Spec.Target.OCIADB.OCID, nil
	}
	if ownerADB != nil && ownerADB.Spec.Details.AutonomousDatabaseOCID != nil {
		return *ownerADB.Spec.Details.AutonomousDatabaseOCID, nil
	}

	return "", errors.New("cannot get the OCID of the targetADB")
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

// manageError doesn't return the error so that the request won't be requeued
func (r *AutonomousDatabaseRestoreReconciler) manageError(restore *dbv1alpha1.AutonomousDatabaseRestore, issue error) (ctrl.Result, error) {
	// Send event
	r.Recorder.Event(restore, corev1.EventTypeWarning, "ReconcileFailed", issue.Error())

	return emptyResult, issue
}
