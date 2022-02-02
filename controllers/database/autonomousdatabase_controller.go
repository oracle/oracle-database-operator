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
	"reflect"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v51/database"
	"github.com/oracle/oci-go-sdk/v51/secrets"
	"github.com/oracle/oci-go-sdk/v51/workrequests"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbv1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	adbutil "github.com/oracle/oracle-database-operator/commons/autonomousdatabase"
	"github.com/oracle/oracle-database-operator/commons/finalizer"
	"github.com/oracle/oracle-database-operator/commons/oci"
)

// AutonomousDatabaseReconciler reconciles a AutonomousDatabase object
type AutonomousDatabaseReconciler struct {
	KubeClient client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
}

// SetupWithManager function
func (r *AutonomousDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.AutonomousDatabase{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}). // ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

func (r *AutonomousDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	pred := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldADB := e.ObjectOld.DeepCopyObject().(*dbv1alpha1.AutonomousDatabase)
			newADB := e.ObjectNew.DeepCopyObject().(*dbv1alpha1.AutonomousDatabase)

			// Reconciliation should NOT happen if the lastSuccessfulSpec annotation or status.state changes.
			oldSucSpec := oldADB.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]
			newSucSpec := newADB.GetAnnotations()[dbv1alpha1.LastSuccessfulSpec]

			lastSucSpecChanged := oldSucSpec != newSucSpec
			stateChanged := oldADB.Status.LifecycleState != newADB.Status.LifecycleState
			if lastSucSpecChanged || stateChanged {
				// Don't enqueue request
				return false
			}
			// Enqueue request
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
	currentLogger := r.Log.WithValues("Namespaced/Name", req.NamespacedName)

	// Get the autonomousdatabase instance from the cluster
	adb := &dbv1alpha1.AutonomousDatabase{}
	if err := r.KubeClient.Get(context.TODO(), req.NamespacedName, adb); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate requeue.
		// No need to change the since we don't know if we obtain the object.
		if !apiErrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	/******************************************************************
	* Get OCI database client and work request client
	******************************************************************/
	authData := oci.APIKeyAuth{
		ConfigMapName: adb.Spec.OCIConfig.ConfigMapName,
		SecretName:    adb.Spec.OCIConfig.SecretName,
		Namespace:     adb.GetNamespace(),
	}
	provider, err := oci.GetOCIProvider(r.KubeClient, authData)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI provider")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	dbClient, err := database.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI database client")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	secretClient, err := secrets.NewSecretsClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI secret client")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	workClient, err := workrequests.NewWorkRequestClientWithConfigurationProvider(provider)
	if err != nil {
		currentLogger.Error(err, "Fail to get OCI work request client")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	currentLogger.Info("OCI provider configured succesfully")

	/******************************************************************
	* Register/unregister finalizer
	* Deletion timestamp will be added to a object before it is deleted.
	* Kubernetes server calls the clean up function if a finalizer exitsts, and won't delete the real object until
	* all the finalizers are removed from the object metadata.
	* Refer to this page for more details of using finalizers: https://kubernetes.io/blog/2021/05/14/using-finalizers-to-control-deletion/
	******************************************************************/
	if adb.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted
		if *adb.Spec.HardLink && !finalizer.HasFinalizer(adb) {
			finalizer.Register(r.KubeClient, adb)
			currentLogger.Info("Finalizer registered successfully.")

		} else if !*adb.Spec.HardLink && finalizer.HasFinalizer(adb) {
			finalizer.Unregister(r.KubeClient, adb)
			currentLogger.Info("Finalizer unregistered successfully.")
		}
	} else {
		// The object is being deleted
		if adb.Spec.Details.AutonomousDatabaseOCID == nil {
			currentLogger.Info("Autonomous Database OCID is missing. Remove the resource only.")
		} else if adb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminating &&
			adb.Status.LifecycleState != database.AutonomousDatabaseLifecycleStateTerminated {
			// Don't send terminate request if the database is terminating or already terminated
			currentLogger.Info("Terminate Autonomous Database: " + *adb.Spec.Details.DbName)
			if _, err := oci.DeleteAutonomousDatabase(dbClient, *adb.Spec.Details.AutonomousDatabaseOCID); err != nil {
				currentLogger.Error(err, "Fail to terminate Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
			}
		}

		finalizer.Unregister(r.KubeClient, adb)
		currentLogger.Info("Finalizer unregistered successfully.")
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
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
	lastSucSpec, err := adb.GetLastSuccessfulSpec()
	if err != nil {
		return ctrl.Result{}, err
	}

	if lastSucSpec == nil || !reflect.DeepEqual(lastSucSpec.Details, adb.Spec.Details) {
		// spec.details changes
		if adb.Spec.Details.AutonomousDatabaseOCID == nil && lastSucSpec == nil {
			// If no AutonomousDatabaseOCID specified, create a database
			// Update from yaml file might not have an AutonomousDatabaseOCID. Don't create a database if it already has last successful spec.
			currentLogger.Info("AutonomousDatabase provisioning")

			resp, err := oci.CreateAutonomousDatabase(currentLogger, r.KubeClient, dbClient, secretClient, adb)

			if err != nil {
				currentLogger.Error(err, "Fail to provision and get Autonomous Database OCID")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}

			adb.Spec.Details.AutonomousDatabaseOCID = resp.AutonomousDatabase.Id

			// Update status.state
			adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
			if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, resp.OpcWorkRequestId); err != nil {
				currentLogger.Error(err, "Fail to watch the status of provision request. opcWorkRequestID = "+*resp.OpcWorkRequestId)

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
			}

			currentLogger.Info("AutonomousDatabase " + *adb.Spec.Details.DbName + " provisioned succesfully")

		} else if adb.Spec.Details.AutonomousDatabaseOCID != nil && lastSucSpec == nil {
			// Binding operation. We have the database ID but hasn't gotten complete infromation from OCI.
			// The next step is to get AutonomousDatabse details from a remote instance.

			adb, err = oci.GetAutonomousDatabaseResource(currentLogger, dbClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to get Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, nil
			}
		} else {
			// The object has successfully synced with the remote database at least once.
			// Update the Autonomous Database in OCI.
			// Change to the lifecycle state has the highest priority.

			// Get the Autonomous Database OCID from the last successful spec if not presented.
			// This happens when a database reference is already created in the cluster. User updates the target CR (specifying metadata.name) but doesn't provide database OCID.
			if adb.Spec.Details.AutonomousDatabaseOCID == nil {
				adb.Spec.Details.AutonomousDatabaseOCID = lastSucSpec.Details.AutonomousDatabaseOCID
			}

			// Start/Stop/Terminate
			setStateResp, err := oci.SetAutonomousDatabaseLifecycleState(currentLogger, dbClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to set the Autonomous Database lifecycle state")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, nil
			}

			if setStateResp != nil {
				var lifecycleState database.AutonomousDatabaseLifecycleStateEnum
				var opcWorkRequestID *string

				if startResp, isStartResponse := setStateResp.(database.StartAutonomousDatabaseResponse); isStartResponse {
					lifecycleState = startResp.AutonomousDatabase.LifecycleState
					opcWorkRequestID = startResp.OpcWorkRequestId

				} else if stopResp, isStopResponse := setStateResp.(database.StopAutonomousDatabaseResponse); isStopResponse {
					lifecycleState = stopResp.AutonomousDatabase.LifecycleState
					opcWorkRequestID = stopResp.OpcWorkRequestId

				} else if deleteResp, isDeleteResponse := setStateResp.(database.DeleteAutonomousDatabaseResponse); isDeleteResponse {
					// Special case. Delete response doen't contain lifecycle State
					lifecycleState = database.AutonomousDatabaseLifecycleStateTerminating
					opcWorkRequestID = deleteResp.OpcWorkRequestId

				} else {
					currentLogger.Error(err, "Unknown response type")

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
					return ctrl.Result{}, nil
				}

				// Update status.state
				adb.Status.LifecycleState = lifecycleState
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}

				if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, opcWorkRequestID); err != nil {
					currentLogger.Error(err, "Fail to watch the status of work request. opcWorkRequestID = "+*opcWorkRequestID)

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
				}
				currentLogger.Info(fmt.Sprintf("Set AutonomousDatabase %s lifecycle state to %s successfully\n",
					*adb.Spec.Details.DbName,
					adb.Spec.Details.LifecycleState))
			}

			// Update the database in OCI from the local resource.
			// The local resource will be synchronized again later.
			updateGenPassResp, err := oci.UpdateGeneralAndPasswordAttributes(currentLogger, r.KubeClient, dbClient, secretClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to update Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}

			if updateGenPassResp.OpcWorkRequestId != nil {
				// Update status.state
				adb.Status.LifecycleState = updateGenPassResp.AutonomousDatabase.LifecycleState
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}

				if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, updateGenPassResp.OpcWorkRequestId); err != nil {
					currentLogger.Error(err, "Fail to watch the status of work request. opcWorkRequestID = "+*updateGenPassResp.OpcWorkRequestId)

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
				}
				currentLogger.Info("Update AutonomousDatabase " + *adb.Spec.Details.DbName + " succesfully")
			}

			scaleResp, err := oci.UpdateScaleAttributes(currentLogger, r.KubeClient, dbClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to update Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
				return ctrl.Result{}, nil
			}

			if scaleResp.OpcWorkRequestId != nil {
				// Update status.state
				adb.Status.LifecycleState = scaleResp.AutonomousDatabase.LifecycleState
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}

				if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, scaleResp.OpcWorkRequestId); err != nil {
					currentLogger.Error(err, "Fail to watch the status of work request. opcWorkRequestID = "+*scaleResp.OpcWorkRequestId)

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
				}
				currentLogger.Info("Scale AutonomousDatabase " + *adb.Spec.Details.DbName + " succesfully")
			}

			oneWayTLSResp, err := oci.UpdateOneWayTLSAttribute(currentLogger, r.KubeClient, dbClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to update Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			}

			if oneWayTLSResp.OpcWorkRequestId != nil {
				// Update status.state
				adb.Status.LifecycleState = oneWayTLSResp.AutonomousDatabase.LifecycleState
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}

				if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, oneWayTLSResp.OpcWorkRequestId); err != nil {
					currentLogger.Error(err, "Fail to watch the status of work request. opcWorkRequestID = "+*oneWayTLSResp.OpcWorkRequestId)

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
				}
				currentLogger.Info("Update AutonomousDatabase " + *adb.Spec.Details.DbName + " 1-way TLS setting succesfully")
			}

			networkResp, err := oci.UpdateNetworkAttributes(currentLogger, r.KubeClient, dbClient, adb)
			if err != nil {
				currentLogger.Error(err, "Fail to update Autonomous Database")

				// Change the status to UNAVAILABLE
				adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
			}

			if networkResp.OpcWorkRequestId != nil {
				// Update status.state
				adb.Status.LifecycleState = networkResp.AutonomousDatabase.LifecycleState
				if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
					return ctrl.Result{}, statusErr
				}

				if err := oci.WaitUntilWorkCompleted(currentLogger, workClient, networkResp.OpcWorkRequestId); err != nil {
					currentLogger.Error(err, "Fail to watch the status of work request. opcWorkRequestID = "+*networkResp.OpcWorkRequestId)

					// Change the status to UNAVAILABLE
					adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
					if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
						return ctrl.Result{}, statusErr
					}
				}
				currentLogger.Info("Update AutonomousDatabase " + *adb.Spec.Details.DbName + " network settings succesfully")
			}
		}
	}

	// Get the information from OCI
	updatedADB, err := oci.GetAutonomousDatabaseResource(currentLogger, dbClient, adb)
	if err != nil {
		currentLogger.Error(err, "Fail to get Autonomous Database")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	adb = updatedADB

	// Update local object and the status
	if err := adbutil.UpdateAutonomousDatabaseDetails(currentLogger, r.KubeClient, adb); err != nil {
		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, err
	}

	/*****************************************************
	*	Instance Wallet
	*****************************************************/
	passwordSecretUpdate := (lastSucSpec == nil && adb.Spec.Details.Wallet.Password.K8sSecretName != nil) ||
		(lastSucSpec != nil && lastSucSpec.Details.Wallet.Password.K8sSecretName != adb.Spec.Details.Wallet.Password.K8sSecretName)
	passwordOCIDUpdate := (lastSucSpec == nil && adb.Spec.Details.Wallet.Password.OCISecretOCID != nil) ||
		(lastSucSpec != nil && lastSucSpec.Details.Wallet.Password.OCISecretOCID != adb.Spec.Details.Wallet.Password.OCISecretOCID)

	if (passwordSecretUpdate || passwordOCIDUpdate) && adb.Status.LifecycleState == database.AutonomousDatabaseLifecycleStateAvailable {
		if err := adbutil.CreateWalletSecret(currentLogger, r.KubeClient, dbClient, secretClient, adb); err != nil {
			currentLogger.Error(err, "Fail to download Instance Wallet")
			// Change the status to UNAVAILABLE
			adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
			if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, nil
		}
	}

	/*****************************************************
	*	Sync AutonomousDatabase Backups
	*****************************************************/
	if err := adbutil.SyncBackupResources(currentLogger, r.KubeClient, dbClient, adb); err != nil {
		currentLogger.Error(err, "Fail to sync Autonomous Database backups")

		// Change the status to UNAVAILABLE
		adb.Status.LifecycleState = database.AutonomousDatabaseLifecycleStateUnavailable
		if statusErr := adbutil.UpdateAutonomousDatabaseStatus(r.KubeClient, adb); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		// The reconciler should not requeue since the error returned from OCI during update will not be solved by requeue
		return ctrl.Result{}, nil
	}

	/*****************************************************
	*	Update last succesful spec
	*****************************************************/
	if err := adb.UpdateLastSuccessfulSpec(r.KubeClient); err != nil {
		return ctrl.Result{}, err
	}

	currentLogger.Info("AutonomousDatabase resource reconcile successfully")

	return ctrl.Result{}, nil
}
