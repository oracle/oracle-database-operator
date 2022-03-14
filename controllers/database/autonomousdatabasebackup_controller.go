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
	"fmt"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oracle/oci-go-sdk/v54/database"
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/k8s"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// *AutonomousDatabaseBackupReconciler reconciles a AutonomousDatabaseBackup object
type AutonomousDatabaseBackupReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme

	adbService  oci.DatabaseService
	workService oci.WorkRequestService
	ownerADB    *dbv1alpha1.AutonomousDatabase
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseBackup{}).
		WithEventFilter(predicate.And(predicate.GenerationChangedPredicate{}, r.eventFilterPredicate())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

func (r *AutonomousDatabaseBackupReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldStatus := e.ObjectOld.(*dbv1alpha1.AutonomousDatabaseBackup).Status.LifecycleState

			if oldStatus == database.AutonomousDatabaseBackupLifecycleStateCreating ||
				oldStatus == database.AutonomousDatabaseBackupLifecycleStateDeleting {
				// All the requests other than the terminate request, should be discarded during the intermediate states
				return false
			}

			return true
		},
	}

	return pred
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
	if err := r.verifyOwnerADB(backup); err != nil {
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
	if backup.Spec.AutonomousDatabaseBackupOCID == "" && backup.Status.LifecycleState == "" {
		// Create a new backup
		backupResp, err := r.adbService.CreateAutonomousDatabaseBackup(backup, *r.ownerADB.Spec.Details.AutonomousDatabaseOCID)
		if err != nil {
			return r.manageError(backup, err)
		}

		adbResp, err := r.adbService.GetAutonomousDatabase(*backupResp.AutonomousDatabaseId)
		if err != nil {
			return r.manageError(backup, err)
		}

		// update the Backup status
		backup.UpdateFromOCIBackup(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
		if err := r.updateResourceStatus(backup); err != nil {
			return r.manageError(backup, err)
		}

		// Wait until the work is done
		if _, err := r.workService.Wait(*backupResp.OpcWorkRequestId); err != nil {
			return r.manageError(backup, err)
		}

		logger.Info("AutonomousDatabaseBackup " + *backupResp.DisplayName + " created successfully")

		return ctrl.Result{Requeue: true}, nil
	}

	/******************************************************************
	 * Update the status of the resource if the
	 * Spec.AutonomousDatabaseOCID isn't empty.
	 ******************************************************************/
	if backup.Spec.AutonomousDatabaseBackupOCID != "" {
		backupResp, err := r.adbService.GetAutonomousDatabaseBackup(backup.Spec.AutonomousDatabaseBackupOCID)
		if err != nil {
			return r.manageError(backup, err)
		}

		adbResp, err := r.adbService.GetAutonomousDatabase(*backupResp.AutonomousDatabaseId)
		if err != nil {
			return r.manageError(backup, err)
		}

		backup.UpdateFromOCIBackup(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
		if err := r.updateResourceStatus(backup); err != nil {
			return r.manageError(backup, err)
		}

		return emptyResult, nil
	}

	return emptyResult, nil
}

func (r *AutonomousDatabaseBackupReconciler) verifyOwnerADB(backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	if len(backup.GetOwnerReferences()) == 0 {
		var err error

		if backup.Spec.TargetADB.Name != "" {
			r.ownerADB, err = k8s.FetchAutonomousDatabase(r.KubeClient, backup.Namespace, backup.Spec.TargetADB.Name)
			if err != nil {
				return err
			}
		} else {
			r.ownerADB, err = k8s.FetchAutonomousDatabaseWithOCID(r.KubeClient, backup.Namespace, backup.Spec.TargetADB.OCID)
			if err != nil {
				return err
			}
		}

		if err := r.setOwnerAutonomousDatabase(backup, r.ownerADB); err != nil {
			return err
		}
	}
	return nil
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

// updateResource updates the specification and the status of AutonomousDatabase resource without trigger a reconcile loop
func (r *AutonomousDatabaseBackupReconciler) updateResource(backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	// Update the spec
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseBackup{}

		namespacedName := types.NamespacedName{
			Namespace: backup.GetNamespace(),
			Name:      backup.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Spec = *backup.Spec.DeepCopy()
		curBackup.ObjectMeta = *backup.ObjectMeta.DeepCopy() // ownerReference
		return r.KubeClient.Update(context.TODO(), curBackup)
	}); err != nil {
		return err
	}

	// Update the status
	if err := r.updateResourceStatus(backup); err != nil {
		return err
	}

	return nil
}

func (r *AutonomousDatabaseBackupReconciler) updateResourceStatus(backup *dbv1alpha1.AutonomousDatabaseBackup) error {
	// sync the ADB status every time when the Backup status is updated
	if err := r.syncADBStatus(); err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curBackup := &dbv1alpha1.AutonomousDatabaseBackup{}

		namespacedName := types.NamespacedName{
			Namespace: backup.GetNamespace(),
			Name:      backup.GetName(),
		}

		if err := r.KubeClient.Get(context.TODO(), namespacedName, curBackup); err != nil {
			return err
		}

		curBackup.Status = backup.Status
		return r.KubeClient.Status().Update(context.TODO(), curBackup)
	})
}

// No-op if the ownerADB is not assigned
func (r *AutonomousDatabaseBackupReconciler) syncADBStatus() error {
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

// setOwnerAutonomousDatabase sets the owner of the AutonomousDatabaseBackup if the AutonomousDatabase resource with the same database OCID is found
func (r *AutonomousDatabaseBackupReconciler) setOwnerAutonomousDatabase(backup *dbv1alpha1.AutonomousDatabaseBackup, adb *dbv1alpha1.AutonomousDatabase) error {
	logger := r.Log.WithName("set-owner")

	backup.SetOwnerReferences(k8s.NewOwnerReference(adb))
	r.updateResource(backup)
	logger.Info(fmt.Sprintf("Set the owner of %s to %s", backup.Name, adb.Name))

	return nil
}

func (r *AutonomousDatabaseBackupReconciler) manageError(backup *dbv1alpha1.AutonomousDatabaseBackup, issue error) (ctrl.Result, error) {
	// Change the status to FAILED
	backup.Status.LifecycleState = database.AutonomousDatabaseBackupLifecycleStateFailed
	if statusErr := r.updateResourceStatus(backup); statusErr != nil {
		return emptyResult, k8s.CombineErrors(issue, statusErr)
	}

	return emptyResult, issue
}
