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
	"reflect"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/oracle/oci-go-sdk/v54/database"
	"github.com/oracle/oci-go-sdk/v54/workrequests"
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	backupUtil "github.com/oracle/oracle-database-operator/commons/autonomousdatabase"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseBackupReconciler reconciles a AutonomousDatabaseBackup object
type AutonomousDatabaseBackupReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseBackup{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

func (r *AutonomousDatabaseBackupReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldBackup := e.ObjectOld.DeepCopyObject().(*dbv1alpha1.AutonomousDatabaseBackup)
			newBackup := e.ObjectNew.DeepCopyObject().(*dbv1alpha1.AutonomousDatabaseBackup)

			specChanged := !reflect.DeepEqual(oldBackup.Spec, newBackup.Spec)
			if specChanged {
				// Enqueue request
				return true
			}
			// Don't enqueue request
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not trigger reconciliation when the real object is deleted from the cluster.
			return false
		},
	}

	return pred
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabasebackups,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabasebackups/status,verbs=get;update;patch

func (r *AutonomousDatabaseBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	currentLogger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	adbBackup := &dbv1alpha1.AutonomousDatabaseBackup{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, adbBackup); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	/******************************************************************
	 * Get OCI database client and work request client
	 ******************************************************************/
	authData := oci.APIKeyAuth{
		ConfigMapName: adbBackup.Spec.OCIConfig.ConfigMapName,
		SecretName:    adbBackup.Spec.OCIConfig.SecretName,
		Namespace:     adbBackup.GetNamespace(),
	}
	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI provider")

		// Change the status to UNAVAILABLE
		adbBackup.Status.LifecycleState = database.AutonomousDatabaseBackupLifecycleStateFailed
		if statusErr := backupUtil.UpdateAutonomousDatabaseBackupStatus(r.KubeClient, adbBackup); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	dbClient, err := database.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI database client")

		// Change the status to UNAVAILABLE
		adbBackup.Status.LifecycleState = database.AutonomousDatabaseBackupLifecycleStateFailed
		if statusErr := backupUtil.UpdateAutonomousDatabaseBackupStatus(r.KubeClient, adbBackup); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	workClient, err := workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI work request client")

		// Change the status to UNAVAILABLE
		adbBackup.Status.LifecycleState = database.AutonomousDatabaseBackupLifecycleStateFailed
		if statusErr := backupUtil.UpdateAutonomousDatabaseBackupStatus(r.KubeClient, adbBackup); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	/******************************************************************
	 * If the Spec.AutonomousDatabaseBackupOCID is empty and the LifecycleState is never assigned , create a backup.
	 * LifecycleState is checked to avoid sending a duplicated backup request when the backup is creating.
	 * Otherwise, bind to an exisiting backup if the  Spec.AutonomousDatabaseBackupOCID isn't empty.
	 ******************************************************************/
	if adbBackup.Spec.AutonomousDatabaseBackupOCID == "" && adbBackup.Status.LifecycleState == "" {
		// Create a new backup
		backupResp, err := oci.CreateAutonomousDatabaseBackup(currentLogger, dbClient, adbBackup)
		if err != nil {
			currentLogger.Error(err, "Fail to create AutonomousDatabase Backup")
			return ctrl.Result{}, nil
		}

		adbResp, err := oci.GetAutonomousDatabase(dbClient, backupResp.AutonomousDatabaseId)
		if err != nil {
			currentLogger.Error(err, "Fail to get AutonomousDatabase. The AutonomousDatabase OCID = "+*backupResp.AutonomousDatabaseId)
			return ctrl.Result{}, nil
		}

		// update the status
		adbBackup.UpdateStatusFromAutonomousDatabaseBackupResponse(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
		backupUtil.UpdateAutonomousDatabaseBackupStatus(r.KubeClient, adbBackup)

		// Wait until the work is done
		if _, err := oci.GetWorkStatusAndWait(currentLogger, workClient, backupResp.OpcWorkRequestId); err != nil {
			currentLogger.Error(err, "Work request faied. opcWorkRequestID = "+*backupResp.OpcWorkRequestId)
			return ctrl.Result{}, nil
		}

		currentLogger.Info("AutonomousDatabaseBackup " + *backupResp.DisplayName + " created successfully")

	} else if adbBackup.Spec.AutonomousDatabaseBackupOCID != "" {
		// Bind to an existing backup
		adbBackup.Status.AutonomousDatabaseBackupOCID = adbBackup.Spec.AutonomousDatabaseBackupOCID
	}

	/******************************************************************
	 * Update the status of the resource if the
	 * Status.AutonomousDatabaseOCID isn't empty.
	 ******************************************************************/
	if adbBackup.Status.AutonomousDatabaseBackupOCID != "" {
		backupResp, err := oci.GetAutonomousDatabaseBackup(dbClient, adbBackup.Status.AutonomousDatabaseBackupOCID)
		if err != nil {
			currentLogger.Error(err, "Fail to get AutonomousDatabase Backup. The AutonomousDatabase Backup OCID = "+adbBackup.Status.AutonomousDatabaseBackupOCID)
			return ctrl.Result{}, nil
		}

		adbResp, err := oci.GetAutonomousDatabase(dbClient, backupResp.AutonomousDatabaseId)
		if err != nil {
			currentLogger.Error(err, "Fail to get AutonomousDatabase. The AutonomousDatabase OCID = "+*backupResp.AutonomousDatabaseId)
			return ctrl.Result{}, nil
		}

		adbBackup.UpdateStatusFromAutonomousDatabaseBackupResponse(backupResp.AutonomousDatabaseBackup, adbResp.AutonomousDatabase)
		backupUtil.UpdateAutonomousDatabaseBackupStatus(r.KubeClient, adbBackup)
	}

	/******************************************************************
	* Look up the owner AutonomousDatabase and set the ownerReference
	* if the owner hasn't been set yet.
	******************************************************************/
	if len(adbBackup.GetOwnerReferences()) == 0 && adbBackup.Status.AutonomousDatabaseOCID != "" {
		if err := backupUtil.SetOwnerAutonomousDatabase(currentLogger, r.KubeClient, adbBackup); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
