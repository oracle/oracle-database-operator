/*
** Copyright (c) 2024 Oracle and/or its affiliates.
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
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DataguardBrokerReconciler reconciles a DataguardBroker object
type DataguardBrokerReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

const dataguardBrokerFinalizer = "database.oracle.com/dataguardbrokerfinalizer"

//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.oracle.com,resources=dataguardbrokers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;persistentvolumeclaims;services,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *DataguardBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("reconciler", req.NamespacedName)

	log.Info("Reconcile requested")

	// Get the dataguardbroker resource if already exists
	var dataguardBroker dbapi.DataguardBroker
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &dataguardBroker); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Resource deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	// Manage dataguardbroker deletion
	if !dataguardBroker.DeletionTimestamp.IsZero() {
		return r.manageDataguardBrokerDeletion(&dataguardBroker, ctx, req)
	}

	// initialize the dataguardbroker resource status
	if dataguardBroker.Status.Status == "" {
		r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Status Initialization", "initializing status fields for the resource")
		log.Info("Initializing status fields")
		dataguardBroker.Status.Status = dbcommons.StatusCreating
		dataguardBroker.Status.ExternalConnectString = dbcommons.ValueUnavailable
		dataguardBroker.Status.ClusterConnectString = dbcommons.ValueUnavailable
		dataguardBroker.Status.FastStartFailover = false
		if len(dataguardBroker.Status.DatabasesInDataguardConfig) == 0 {
			dataguardBroker.Status.DatabasesInDataguardConfig = map[string]string{}
		}
	}

	// Always refresh status before a reconcile
	defer r.Status().Update(ctx, &dataguardBroker)

	// Mange DataguardBroker Creation
	result, err := r.manageDataguardBrokerCreation(&dataguardBroker, ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if result.Requeue {
		return result, nil
	}

	// manage enabling and disabling faststartfailover
	if dataguardBroker.Spec.FastStartFailover {

		for _, DbResource := range dataguardBroker.Status.DatabasesInDataguardConfig {
			var singleInstanceDatabase dbapi.SingleInstanceDatabase
			if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: DbResource}, &singleInstanceDatabase); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
			r.Log.Info("Check the role for database", "database", singleInstanceDatabase.Name, "role", singleInstanceDatabase.Status.Role)
			if singleInstanceDatabase.Status.Role == "SNAPSHOT_STANDBY" {
				r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Enabling FSFO failed", "database %s is a snapshot database", singleInstanceDatabase.Name)
				r.Log.Info("Enabling FSFO failed, one of the database is a snapshot database", "snapshot database", singleInstanceDatabase.Name)
				return ctrl.Result{Requeue: true}, nil
			}
		}

		// set faststartfailover targets for all the singleinstancedatabases in the dataguard configuration
		if err := setFSFOTargets(r, &dataguardBroker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}

		// enable faststartfailover in the dataguard configuration
		if err := enableFSFOForDgConfig(r, &dataguardBroker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}

		// create Observer Pod
		if err := createObserverPods(r, &dataguardBroker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}

		// set faststartfailover status to true
		dataguardBroker.Status.FastStartFailover = true

	} else {

		// disable faststartfailover
		if err := disableFSFOForDGConfig(r, &dataguardBroker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}

		// delete Observer Pod
		observerReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", dataguardBroker.Name, dataguardBroker.Namespace, ctx, req)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if observerReadyPod.Name != "" {
			if err := r.Delete(ctx, &observerReadyPod); err != nil {
				return ctrl.Result{Requeue: false}, err
			}
		}

		r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeNormal, "Observer Deleted", "database observer pod deleted")
		log.Info("database observer deleted")

		// set faststartfailover status to false
		dataguardBroker.Status.FastStartFailover = false
	}

	// manage manual switchover
	if dataguardBroker.Spec.SetAsPrimaryDatabase != "" && dataguardBroker.Spec.SetAsPrimaryDatabase != dataguardBroker.Status.PrimaryDatabase {
		if _, ok := dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]; !ok {
			r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Cannot Switchover", fmt.Sprintf("database with SID %v not found in dataguardbroker configuration", dataguardBroker.Spec.SetAsPrimaryDatabase))
			log.Info(fmt.Sprintf("cannot perform switchover, database with SID %v not found in dataguardbroker configuration", dataguardBroker.Spec.SetAsPrimaryDatabase))
			return ctrl.Result{Requeue: false}, nil
		}
		r.Recorder.Eventf(&dataguardBroker, corev1.EventTypeWarning, "Manual Switchover", fmt.Sprintf("Switching over to %s database", dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]))
		log.Info(fmt.Sprintf("switching over to %s database", dataguardBroker.Status.DatabasesInDataguardConfig[dataguardBroker.Spec.SetAsPrimaryDatabase]))
		result, err := r.manageManualSwitchOver(dataguardBroker.Spec.SetAsPrimaryDatabase, &dataguardBroker, ctx, req)
		if err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		if result.Requeue {
			return result, nil
		}
	}

	// Update Status for broker and sidb resources
	if err := updateReconcileStatus(r, &dataguardBroker, ctx, req); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	dataguardBroker.Status.Status = dbcommons.StatusReady
	log.Info("Reconcile Completed")

	if dataguardBroker.Spec.FastStartFailover {
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	} else {
		return ctrl.Result{Requeue: false}, nil
	}
}

// #############################################################################################################################
//
//	Manage deletion and clean up of the dataguardBroker resource
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageDataguardBrokerDeletion(broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("manageDataguardBrokerDeletion", req.NamespacedName)

	log.Info(fmt.Sprintf("Deleting dataguard broker %v", broker.Name))
	// Check if the DataguardBroker instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	if controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {
		// Run finalization logic for dataguardBrokerFinalizer. If the
		// finalization logic fails, don't remove the finalizer so
		// that we can retry during the next reconciliation.
		if err := cleanupDataguardBroker(r, broker, req, ctx); err != nil {
			// handle the errors
			return ctrl.Result{Requeue: false}, err
		}

		// Remove dataguardBrokerFinalizer. Once all finalizers have been
		// removed, the object will be deleted.
		controllerutil.RemoveFinalizer(broker, dataguardBrokerFinalizer)
		if err := r.Update(ctx, broker); err != nil {
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Updating Resource", "Error while removing resource finalizers")
			log.Info("Error while removing resource finalizers")
			return ctrl.Result{Requeue: false}, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Manage validation of singleinstancedatabases and creation of the dataguard configuration
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageDataguardBrokerCreation(broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("manageDataguardBrokerCreation", req.NamespacedName)

	// Add finalizer for this dataguardbroker resource
	if !controllerutil.ContainsFinalizer(broker, dataguardBrokerFinalizer) {
		r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Updating Resource", "Adding finalizers")
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(broker, dataguardBrokerFinalizer)
		if err := r.Update(ctx, broker); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}

	// Check if a service for the dataguardbroker resources exists
	var service corev1.Service
	if err := r.Get(context.TODO(), types.NamespacedName{Name: broker.Name, Namespace: broker.Namespace}, &service); err != nil {
		// check if the required service is not found then create the service
		if apierrors.IsNotFound(err) {
			r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Creating Service", "creating service for the resource")
			log.Info("creating service for the dataguardbroker resource")

			// instantiate the service specification
			svc := dbcommons.NewRealServiceBuilder().
				SetName(broker.Name).
				SetNamespace(broker.Namespace).
				SetLabels(map[string]string{
					"app": broker.Name,
				}).
				SetAnnotation(func() map[string]string {
					annotations := make(map[string]string)
					if len(broker.Spec.ServiceAnnotations) != 0 {
						for key, value := range broker.Spec.ServiceAnnotations {
							annotations[key] = value
						}
					}
					return annotations
				}()).
				SetPorts([]corev1.ServicePort{
					{
						Name:     "listener",
						Port:     1521,
						Protocol: corev1.ProtocolTCP,
					},
					{
						Name:     "xmldb",
						Port:     5500,
						Protocol: corev1.ProtocolTCP,
					},
				}).
				SetSelector(map[string]string{
					"app": broker.Name,
				}).
				SetType(func() corev1.ServiceType {
					if broker.Spec.LoadBalancer {
						return corev1.ServiceType("LoadBalancer")
					}
					return corev1.ServiceType("NodePort")
				}()).
				Build()

			// Set the ownership of the service object to the dataguard broker resource object
			ctrl.SetControllerReference(broker, &svc, r.Scheme)

			// create the service for dataguardbroker resource
			if err = r.Create(ctx, &svc); err != nil {
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Service Creation", "service creation failed")
				log.Info("service creation failed")
				return ctrl.Result{Requeue: false}, err
			} else {
				timeout := 30
				// Waiting for Service to get created as sometimes it takes some time to create a service . 30 seconds TImeout
				err = dbcommons.WaitForStatusChange(r, svc.Name, broker.Namespace, ctx, req, time.Duration(timeout)*time.Second, "svc", "creation")
				if err != nil {
					log.Error(err, "Error in Waiting for svc status for Creation", "svc.Namespace", svc.Namespace, "SVC.Name", svc.Name)
					return ctrl.Result{Requeue: false}, err
				}
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Service Created", fmt.Sprintf("Succesfully Created New Service %v", svc.Name))
				log.Info("Succesfully Created New Service ", "Service.Name : ", svc.Name)
			}
			time.Sleep(10 * time.Second)
		} else {
			return ctrl.Result{Requeue: false}, err
		}
	}

	log.Info(" ", "Found Existing Service ", service.Name)

	// validate if all the databases have only one replicas
	for _, databaseRef := range broker.GetDatabasesInDataGuardConfiguration() {
		var singleinstancedatabase dbapi.SingleInstanceDatabase
		if err := r.Get(ctx, types.NamespacedName{Name: databaseRef, Namespace: broker.Namespace}, &singleinstancedatabase); err != nil {
			if apierrors.IsNotFound(err) {
				broker.Status.Status = dbcommons.StatusError
				r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", fmt.Sprintf("SingleInstanceDatabase %s not found", singleinstancedatabase.Name))
				log.Info(fmt.Sprintf("singleinstancedatabase %s not found", databaseRef))
				return ctrl.Result{Requeue: false}, nil
			}
			return ctrl.Result{Requeue: false}, err
		}
		if broker.Spec.FastStartFailover && singleinstancedatabase.Status.Replicas > 1 {
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SIDB Not supported", "dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			log.Info("dataguardbroker doesn't support multiple replicas sidb in FastStartFailover mode")
			broker.Status.Status = dbcommons.StatusError
			return ctrl.Result{Requeue: false}, nil
		}
	}

	// Get the current primary singleinstancedatabase resourcce
	var sidb dbapi.SingleInstanceDatabase
	namespacedName := types.NamespacedName{
		Namespace: broker.Namespace,
		Name:      broker.GetCurrentPrimaryDatabase(),
	}
	if err := r.Get(ctx, namespacedName, &sidb); err != nil {
		if apierrors.IsNotFound(err) {
			broker.Status.Status = dbcommons.StatusError
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "SingleInstanceDatabase Not Found", fmt.Sprintf("SingleInstanceDatabase %s not found", sidb.Name))
			log.Info(fmt.Sprintf("singleinstancedatabase %s not found", namespacedName.Name))
			return ctrl.Result{Requeue: false}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}
	if sidb.Status.Role != "PRIMARY" {
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Spec Validation", fmt.Sprintf("singleInstanceDatabase %v not in primary role", sidb.Name))
		log.Info(fmt.Sprintf("singleinstancedatabase %s expected to be in primary role", sidb.Name))
		log.Info("updating database status to check for possible FSFO")
		if err := updateReconcileStatus(r, broker, ctx, req); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
	}

	// validate current primary singleinstancedatabase readiness
	log.Info(fmt.Sprintf("Validating readiness for singleinstancedatabase %v", sidb.Name))
	if err := validateSidbReadiness(r, broker, &sidb, ctx, req); err != nil {
		if errors.Is(err, ErrCurrentPrimaryDatabaseNotReady) {
			if broker.Status.Status != "" && broker.Status.FastStartFailover {
				r.Recorder.Eventf(broker, corev1.EventTypeNormal, "Possible Failover", "Primary db not in ready state after setting up DG configuration")
			}
			if err := updateReconcileStatus(r, broker, ctx, req); err != nil {
				log.Info("Error updating Dgbroker status")
			}
			r.Recorder.Eventf(broker, corev1.EventTypeWarning, "Waiting", err.Error())
			return ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}, nil
		}
		return ctrl.Result{Requeue: false}, err
	}

	// setup dataguard configuration
	log.Info(fmt.Sprintf("setup Dataguard configuration for primary database %v", sidb.Name))
	if err := setupDataguardBrokerConfiguration(r, broker, &sidb, ctx, req); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Manange manual switchover to the target database
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) manageManualSwitchOver(targetSidbSid string, broker *dbapi.DataguardBroker, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := r.Log.WithValues("SetAsPrimaryDatabase", req.NamespacedName)

	if _, ok := broker.Status.DatabasesInDataguardConfig[targetSidbSid]; !ok {
		eventReason := "Cannot Switchover"
		eventMsg := fmt.Sprintf("Database %s not a part of the dataguard configuration", targetSidbSid)
		r.Recorder.Eventf(broker, corev1.EventTypeWarning, eventReason, eventMsg)
		return ctrl.Result{Requeue: false}, nil
	}

	// change broker status to updating to indicate manual switchover start
	broker.Status.Status = dbcommons.StatusUpdating
	r.Status().Update(ctx, broker)

	var sidb dbapi.SingleInstanceDatabase
	if err := r.Get(context.TODO(), types.NamespacedName{Name: broker.GetCurrentPrimaryDatabase(), Namespace: broker.Namespace}, &sidb); err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Fetch the primary database ready pod to create chk file
	sidbReadyPod, _, _, _, err := dbcommons.FindPods(r, sidb.Spec.Image.Version,
		sidb.Spec.Image.PullFrom, sidb.Name, sidb.Namespace, ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Fetch the target database ready pod to create chk file
	targetReadyPod, _, _, _, err := dbcommons.FindPods(r, "", "", broker.Status.DatabasesInDataguardConfig[targetSidbSid], req.Namespace,
		ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: broker.Status.DatabasesInDataguardConfig[targetSidbSid], Namespace: req.Namespace}})
	if err != nil {
		return ctrl.Result{Requeue: false}, err
	}

	// Create a chk File so that no other pods take the lock during Switchover .
	out, err := dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("Successfully Created chk file " + out)

	out, err = dbcommons.ExecCommand(r, r.Config, targetReadyPod.Name, targetReadyPod.Namespace, "", ctx, req, false, "bash", "-c", dbcommons.CreateChkFileCMD)
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("Successfully Created chk file " + out)

	eventReason := "Waiting"
	eventMsg := "Switchover In Progress"
	r.Recorder.Eventf(broker, corev1.EventTypeNormal, eventReason, eventMsg)

	// Get Admin password for current primary database
	var adminPasswordSecret corev1.Secret
	if err := r.Get(context.TODO(), types.NamespacedName{Name: sidb.Spec.AdminPassword.SecretName, Namespace: sidb.Namespace}, &adminPasswordSecret); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	var adminPassword string = string(adminPasswordSecret.Data[sidb.Spec.AdminPassword.SecretKey])

	// Connect to 'primarySid' db using dgmgrl and switchover to 'targetSidbSid' db to make 'targetSidbSid' db primary
	_, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, true, "bash", "-c",
		fmt.Sprintf(dbcommons.CreateAdminPasswordFile, adminPassword))
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("DB Admin pwd file created")

	out, err = dbcommons.ExecCommand(r, r.Config, sidbReadyPod.Name, sidbReadyPod.Namespace, "", ctx, req, false, "bash", "-c",
		fmt.Sprintf("dgmgrl sys@%s \"SWITCHOVER TO %s\" < admin.pwd", broker.Status.PrimaryDatabase, targetSidbSid))
	if err != nil {
		log.Error(err, err.Error())
		return ctrl.Result{Requeue: false}, err
	}
	log.Info("SWITCHOVER TO " + targetSidbSid + " Output")
	log.Info(out)

	return ctrl.Result{Requeue: false}, nil
}

// #############################################################################################################################
//
//	Setup the controller with the Manager
//
// #############################################################################################################################
func (r *DataguardBrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbapi.DataguardBroker{}).
		Owns(&corev1.Pod{}). //Watch for deleted pods of DataguardBroker Owner
		WithEventFilter(dbcommons.ResourceEventHandler()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}
