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

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oracle/oci-go-sdk/v54/database"
	"github.com/oracle/oci-go-sdk/v54/workrequests"
	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	restoreUtil "github.com/oracle/oracle-database-operator/commons/autonomousdatabase"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseRestoreReconciler reconciles a AutonomousDatabaseRestore object
type AutonomousDatabaseRestoreReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaserestores,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=autonomousdatabaserestores/status,verbs=get;update;patch

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
	currentLogger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	restore := &dbv1alpha1.AutonomousDatabaseRestore{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, restore); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change since we don't know if we obtain the object.
		if apiErrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	/******************************************************************
	 * Get OCI database client and work request client
	 ******************************************************************/
	authData := oci.APIKeyAuth{
		ConfigMapName: restore.Spec.OCIConfig.ConfigMapName,
		SecretName:    restore.Spec.OCIConfig.SecretName,
		Namespace:     restore.GetNamespace(),
	}
	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI provider")

		// Change the status to UNAVAILABLE
		restore.Status.LifecycleState = workrequests.WorkRequestStatusFailed
		if statusErr := restoreUtil.UpdateAutonomousDatabaseRestoreStatus(r.KubeClient, restore); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	dbClient, err := database.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI database client")

		// Change the status to UNAVAILABLE
		restore.Status.LifecycleState = workrequests.WorkRequestStatusFailed
		if statusErr := restoreUtil.UpdateAutonomousDatabaseRestoreStatus(r.KubeClient, restore); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	workClient, err := workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI work request client")

		// Change the status to UNAVAILABLE
		restore.Status.LifecycleState = workrequests.WorkRequestStatusFailed
		if statusErr := restoreUtil.UpdateAutonomousDatabaseRestoreStatus(r.KubeClient, restore); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	/******************************************************************
	 * Restore
	 ******************************************************************/
	if restore.Status.LifecycleState == "" {
		lifecycleState, err := restoreUtil.RestoreAutonomousDatabaseAndWait(currentLogger, r.KubeClient, dbClient, workClient, restore)
		if err != nil {
			currentLogger.Error(err, "Fail to restore database")
			return ctrl.Result{}, nil
		}

		restore.Status.LifecycleState = lifecycleState

		if statusErr := restoreUtil.UpdateAutonomousDatabaseRestoreStatus(r.KubeClient, restore); statusErr != nil {
			// No need to return the error since it will re-execute the Reconcile()
			currentLogger.Error(err, "Fail to update the status of AutonomousDatabaseRestore")
			return ctrl.Result{}, nil
		}
	}

	currentLogger.Info("AutonomousDatabaseRestore reconciles successfully")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutonomousDatabaseRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.AutonomousDatabaseRestore{}).
		Complete(r)
}
