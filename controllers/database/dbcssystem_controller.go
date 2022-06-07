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
	"reflect"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	dbcsv1 "github.com/oracle/oracle-database-operator/commons/dbcssystem"
	"github.com/oracle/oracle-database-operator/commons/finalizer"
	"github.com/oracle/oracle-database-operator/commons/oci"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v64/core"
	"github.com/oracle/oci-go-sdk/v64/database"
	"github.com/oracle/oci-go-sdk/v64/workrequests"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DbcsSystemReconciler reconciles a DbcsSystem object
type DbcsSystemReconciler struct {
	KubeClient client.Client
	Scheme     *runtime.Scheme
	Logv1      logr.Logger
	Logger     logr.Logger
	dbClient   database.DatabaseClient
	nwClient   core.VirtualNetworkClient
	wrClient   workrequests.WorkRequestClient
	Recorder   record.EventRecorder
}

//+kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=dbcssystems/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps;secrets;namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DbcsSystem object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DbcsSystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger = log.FromContext(ctx)

	// your logic here

	//r.Logger = r.Logv1.WithValues("Instance.Namespace", req.NamespacedName)
	var err error
	// Get the dbcs instance from the cluster
	dbcsInst := &databasev1alpha1.DbcsSystem{}

	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, dbcsInst); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Create oci-go-sdk client
	authData := oci.APIKeyAuth{
		ConfigMapName: &dbcsInst.Spec.OCIConfigMap,
		SecretName:    &dbcsInst.Spec.OCISecret,
		Namespace:     dbcsInst.GetNamespace(),
	}
	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.dbClient, err = database.NewDatabaseClientWithConfigurationProvider(provider)

	if err != nil {
		return ctrl.Result{}, err
	}

	r.nwClient, err = core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.wrClient, err = workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	r.Logger.Info("OCI provider configured succesfully")

	/*
	   Using Finalizer for object deletion
	*/

	if dbcsInst.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted
		if dbcsInst.Spec.HardLink && !finalizer.HasFinalizer(dbcsInst) {
			finalizer.Register(r.KubeClient, dbcsInst)
			r.Logger.Info("Finalizer registered successfully.")
		} else if !dbcsInst.Spec.HardLink && finalizer.HasFinalizer(dbcsInst) {
			finalizer.Unregister(r.KubeClient, dbcsInst)
			r.Logger.Info("Finalizer unregistered successfully.")
		}
	} else {
		// The object is being deleted
		r.Logger.Info("Terminate DbcsSystem Database: " + dbcsInst.Spec.DbSystem.DisplayName)
		if err := dbcsv1.DeleteDbcsSystemSystem(r.dbClient, *dbcsInst.Spec.Id); err != nil {
			r.Logger.Error(err, "Fail to terminate DbcsSystem Instance")
			// Change the status to Failed
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Terminate, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			return ctrl.Result{}, nil
		}
		finalizer.Unregister(r.KubeClient, dbcsInst)
		r.Logger.Info("Finalizer unregistered successfully.")
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	/*
	   Determine whether it's a provision or bind operation
	*/
	lastSucSpec, err := dbcsInst.GetLastSuccessfulSpec()
	if err != nil {
		return ctrl.Result{}, err
	}

	if dbcsInst.Spec.Id == nil && lastSucSpec == nil {
		// If no DbcsSystem ID specified, create a DB System
		// ======================== Validate Specs ==============
		err = dbcsv1.ValidateSpex(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.Recorder)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Logger.Info("DbcsSystem DBSystem provisioning")
		dbcsID, err := dbcsv1.CreateAndGetDbcsId(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient)
		if err != nil {
			r.Logger.Error(err, "Fail to provision and get DbcsSystem System ID")

			// Change the status to Failed
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			return ctrl.Result{}, nil
		}

		assignDBCSID(dbcsInst, dbcsID)
		if err := dbcsv1.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
			// Change the status to Failed
			assignDBCSID(dbcsInst, dbcsID)
			if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, err
		}

		r.Logger.Info("DbcsSystem system provisioned succesfully")
		assignDBCSID(dbcsInst, dbcsID)
		if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
			return ctrl.Result{}, err
		}
		assignDBCSID(dbcsInst, dbcsID)
	} else {
		if lastSucSpec == nil {
			if err := dbcsv1.GetDbSystemId(r.Logger, r.dbClient, dbcsInst); err != nil {
				// Change the status to Failed
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}
			if err := dbcsv1.SetDBCSDatabaseLifecycleState(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
				// Change the status to required state
				return ctrl.Result{}, err
			}

			dbcsInstId := *dbcsInst.Spec.Id
			if err := dbcsv1.UpdateDbcsSystemId(r.KubeClient, dbcsInst); err != nil {
				// Change the status to Failed
				assignDBCSID(dbcsInst, dbcsInstId)
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}

			r.Logger.Info("Sync information from remote DbcsSystem System successfully")

			dbcsInstId = *dbcsInst.Spec.Id
			if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
				return ctrl.Result{}, err
			}
			assignDBCSID(dbcsInst, dbcsInstId)
		} else {
			if dbcsInst.Spec.Id == nil {
				dbcsInst.Spec.Id = lastSucSpec.Id
			}

			if err := dbcsv1.UpdateDbcsSystemIdInst(r.Logger, r.dbClient, dbcsInst, r.KubeClient, r.nwClient, r.wrClient); err != nil {
				r.Logger.Error(err, "Fail to update DbcsSystem Id")

				// Change the status to Failed
				if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Failed, r.nwClient, r.wrClient); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}
			if err := dbcsv1.SetDBCSDatabaseLifecycleState(r.Logger, r.KubeClient, r.dbClient, dbcsInst, r.nwClient, r.wrClient); err != nil {
				// Change the status to required state
				return ctrl.Result{}, err
			}
		}
	}

	// Update the Wallet Secret when the secret name is given
	//r.updateWalletSecret(dbcs)

	// Update the last succesful spec
	dbcsInstId := *dbcsInst.Spec.Id
	if err := dbcsInst.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
		return ctrl.Result{}, err
	}
	//assignDBCSID(dbcsInst,dbcsI)
	// Change the phase to "Available"
	assignDBCSID(dbcsInst, dbcsInstId)
	if statusErr := dbcsv1.SetLifecycleState(r.KubeClient, r.dbClient, dbcsInst, databasev1alpha1.Available, r.nwClient, r.wrClient); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, nil
}

func assignDBCSID(dbcsInst *databasev1alpha1.DbcsSystem, dbcsID string) {
	dbcsInst.Spec.Id = &dbcsID
}

func (r *DbcsSystemReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Get the dbName as old dbName when an update event happens
			oldObject := e.ObjectOld.DeepCopyObject().(*databasev1alpha1.DbcsSystem)
			newObject := e.ObjectNew.DeepCopyObject().(*databasev1alpha1.DbcsSystem)
			specObject := !reflect.DeepEqual(oldObject.Spec, newObject.Spec)

			deletionTimeStamp := !reflect.DeepEqual(oldObject.GetDeletionTimestamp(), newObject.GetDeletionTimestamp())

			if specObject || deletionTimeStamp {
				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbcsSystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.DbcsSystem{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}).
		Complete(r)
}
