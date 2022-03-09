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
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/oracle/oci-go-sdk/v54/common"
	"github.com/oracle/oci-go-sdk/v54/ons"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	databasev1alpha1 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
	shardingv1 "github.com/oracle/oracle-database-operator/commons/sharding"
)

//Sharding Topology
type ShardingTopology struct {
	topicid         string
	Instance        *databasev1alpha1.ShardingDatabase
	deltopology     bool
	onsProvider     common.ConfigurationProvider
	onsProviderFlag bool
	rclient         ons.NotificationDataPlaneClient
}

// ShardingDatabaseReconciler reconciles a ShardingDatabase object
type ShardingDatabaseReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	kubeClient kubernetes.Interface
	kubeConfig clientcmd.ClientConfig
	Recorder   record.EventRecorder
	osh        []*ShardingTopology
}

// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods;pods/log;pods/exec;secrets;services;events;nodes;configmaps;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ShardingDatabase object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *ShardingDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//ctx := context.Background()
	//_ = r.Log.WithValues("shardingdatabase", req.NamespacedName)

	// your logic here
	var i int32
	//var ShardImageLatest []databasev1alpha1.ShardSpec
	var OraCatalogSpex databasev1alpha1.CatalogSpec
	var OraShardSpex databasev1alpha1.ShardSpec
	var OraGsmSpex databasev1alpha1.GsmSpec
	var result ctrl.Result
	var isShardTopologyDeleteTrue bool = false
	//var msg string
	var err error
	var stateType string
	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}
	var nilErr error = nil

	// On every reconcile, we will call setCrdLifeCycleState
	// To understand this, please refer https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/
	// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/meta/conditions.go

	// Kube Client Config Setup
	if r.kubeConfig == nil && r.kubeClient == nil {
		r.kubeConfig, r.kubeClient, err = shardingv1.GetK8sClientConfig(r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	// Fetch the ProvShard instance
	instance := &databasev1alpha1.ShardingDatabase{}
	err = r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile req.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the req.
		return ctrl.Result{}, err
	}

	idx, instFlag := r.checkProvInstance(instance)
	// assinging osh instance
	if !instFlag {
		// Sharding Topolgy Struct Assignment
		// ======================================
		osh := &ShardingTopology{}
		osh.Instance = instance
		r.osh = append(r.osh, osh)
	}
	defer r.setCrdLifeCycleState(instance, &result, &err, &stateType)
	// =============================== Check Deletion TimeStamp========
	// Check if the ProvOShard instance is marked to be deleted, which is
	// // indicated by the deletion timestamp being set.

	err, isShardTopologyDeleteTrue = r.finalizerShardingDatabaseInstance(instance)
	if err != nil {
		//r.setCrdLifeCycleState(instance, &result, &err, stateType)
		result = resultNq
		if isShardTopologyDeleteTrue == true {
			err = nilErr
			return result, err
		} else {
			return result, err
		}
	}

	// ======== Setting the flag and Index to be used later in this function ========
	idx, instFlag = r.checkProvInstance(instance)
	if !instFlag {
		//r.setCrdLifeCycleState(instance, &result, &err, stateType)
		result = resultNq
		return result, fmt.Errorf("DId not fid the instance in checkProvInstance")
	}

	// ================================  OCI Notification Provider ===========
	r.getOnsConfigProvider(instance, idx)

	// =============================== Checking Namespace ==============
	if instance.Spec.Namespace != "" {
		err = shardingv1.AddNamespace(instance, r.Client, r.Log)
		if err != nil {
			//r.setCrdLifeCycleState(instance, &result, &err, stateType)
			result = resultNq
			return result, err
		}
	} else {
		instance.Spec.Namespace = "default"
	}

	// ======================== Validate Specs ==============
	err = r.validateSpex(instance, idx)
	if err != nil {
		//r.setCrdLifeCycleState(instance, &result, &err, stateType)
		result = resultNq
		return result, err
	}

	// ========================= Service Setup For Catalog===================
	// Following check and loop will make sure  to create the service
	for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
		OraCatalogSpex = instance.Spec.Catalog[i]
		result, err = r.createService(instance, shardingv1.BuildServiceDefForCatalog(instance, 0, OraCatalogSpex, "local"))
		if err != nil {
			result = resultNq
			return result, err
		}
		if instance.Spec.IsExternalSvc {
			result, err = r.createService(instance, shardingv1.BuildServiceDefForCatalog(instance, 0, OraCatalogSpex, "external"))
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	}

	// ================================ Catalog Setup ===================
	if len(instance.Spec.Catalog) > 0 {
		for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
			OraCatalogSpex = instance.Spec.Catalog[i]
			// See if StatefulSets already exists and create if it doesn't
			result, err = r.deployStatefulSet(instance, shardingv1.BuildStatefulSetForCatalog(instance, OraCatalogSpex), "CATALOG")
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	}

	// ========================= Service Setup For Gsm===================
	// Following check and loop will make sure if we need service per replica pod or on a single pod
	// if user set replicasize greater than 1 but also set instance.Spec.OraDbPvcName then only one service will be created and one pod
	for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
		OraGsmSpex = instance.Spec.Gsm[i]
		result, err = r.createService(instance, shardingv1.BuildServiceDefForGsm(instance, 0, OraGsmSpex, "local"))
		if err != nil {
			result = resultNq
			return result, err
		}
		if instance.Spec.IsExternalSvc {
			result, err = r.createService(instance, shardingv1.BuildServiceDefForGsm(instance, 0, OraGsmSpex, "external"))
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	}

	// ========================= Service Setup For Gsm===================

	// Following check and loop will make sure if we need service per replica pod or on a single pod
	// if user set replicasize greater than 1 but also set instance.Spec.OraDbPvcName then only one service will be created and one pod
	// ================================ Gsm Setup ===================
	if len(instance.Spec.Gsm) > 0 {
		//   for _, OraGsmSpex := range instance.Spec.Gsm
		for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
			OraGsmSpex = instance.Spec.Gsm[i]
			result, err = r.deployStatefulSet(instance, shardingv1.BuildStatefulSetForGsm(instance, OraGsmSpex), "GSM")
			if err != nil {
				result = resultNq
				return result, err
			}
		}
	}

	// ========================= Service Setup For Shard===================

	// Following check and loop will make sure if we need service per replica pod or on a single pod
	// if user set replicasize greater than 1 but also set instance.Spec.OraDbPvcName then only one service will be created and one pod
	for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
		OraShardSpex = instance.Spec.Shard[i]
		if OraShardSpex.IsDelete != true {
			result, err = r.createService(instance, shardingv1.BuildServiceDefForShard(instance, 0, OraShardSpex, "local"))
			if err != nil {
				result = resultNq
				return result, err
			}
			if instance.Spec.IsExternalSvc {
				result, err = r.createService(instance, shardingv1.BuildServiceDefForShard(instance, 0, OraShardSpex, "external"))
				if err != nil {
					result = resultNq
					return result, err
				}
			}
		}
	}

	// ================================ Shard Setup ===================
	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex = instance.Spec.Shard[i]
			if OraShardSpex.IsDelete != true {
				result, err = r.deployStatefulSet(instance, shardingv1.BuildStatefulSetForShard(instance, OraShardSpex), "SHARD")
				if err != nil {
					result = resultNq
					return result, err
				}
			}
		}
	}
	//================ Validate the GSM and Catalog before procedding for Shard Setup ==============
	// If the GSM and Catalog is not configured then Requeue the loop unless it returns nil
	// Until GSM and Catalog is configured, the topology state remain provisioning
	err = r.validateGsmnCatalog(instance)
	if err != nil {
		//	r.setCrdLifeCycleState(instance, &result, &err, stateType)
		//	time.Sleep(30 * time.Second)
		err = nilErr
		result = resultQ
		return result, err
	}

	//set the Waiting state for Reconcile loop
	// Loop will be requeued only if Shard Statefulset is not ready or not configured.
	// Till that time Reconcilation loop will remain in blocked state
	// if the err is return because of Shard is not ready then blocked state is rmeoved and reconcilation state is set
	err = r.addPrimaryShards(instance, idx)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		err = nilErr
		result = resultQ
		return result, err
	}

	// Loop will be requeued only if Standby Shard Statefulset is not ready or not configured.
	// Till that time Reconcilation loop will remain in blocked state
	// if the err is return because of Shard is not ready then blocked state is rmeoved and reconcilation state is
	err = r.addStandbyShards(instance, idx)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		err = nilErr
		result = resultQ
		return result, err
	}

	// we don't need to run the requeue loop but still putting this condition to address any unkown situation
	// delShard function set the state to blocked and we do not allow any other operationn while delete is going on
	err = r.delGsmShard(instance, idx)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		err = nilErr
		result = resultQ
		return result, err
	}

	// ====================== Update Setup for Catalog ==============================
	for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
		OraCatalogSpex = instance.Spec.Catalog[i]
		sfSet, catalogPod, err := r.validateInvidualCatalog(instance, OraCatalogSpex, int(i))
		if err != nil {
			shardingv1.LogMessages("INFO", "Catalog "+sfSet.Name+" is not in available state.", nil, instance, r.Log)
			result = resultNq
			return result, err
		}
		result, err = shardingv1.UpdateProvForCatalog(instance, OraCatalogSpex, r.Client, sfSet, catalogPod, r.Log)
		if err != nil {
			shardingv1.LogMessages("INFO", "Error Occurred during catalog update operation.", nil, instance, r.Log)
			result = resultNq
			return result, err
		}
	}

	// ====================== Update Setup for Shard ==============================
	for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
		OraShardSpex = instance.Spec.Shard[i]
		if OraShardSpex.IsDelete != true {
			sfSet, shardPod, err := r.validateShard(instance, OraShardSpex, int(i))
			if err != nil {
				shardingv1.LogMessages("INFO", "Shard "+sfSet.Name+" is not in available state.", nil, instance, r.Log)
				result = resultNq
				return result, err
			}
			result, err = shardingv1.UpdateProvForShard(instance, OraShardSpex, r.Client, sfSet, shardPod, r.Log)
			if err != nil {
				shardingv1.LogMessages("INFO", "Error Occurred during shard update operation..", nil, instance, r.Log)
				result = resultNq
				return result, err
			}
		}
	}

	// ====================== Update Setup for Gsm ==============================
	for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
		OraGsmSpex = instance.Spec.Gsm[i]
		sfSet, gsmPod, err := r.validateInvidualGsm(instance, OraGsmSpex, int(i))
		if err != nil {
			shardingv1.LogMessages("INFO", "Gsm "+sfSet.Name+" is not in available state.", nil, instance, r.Log)
			result = resultNq
			return result, err
		}
		result, err = shardingv1.UpdateProvForGsm(instance, OraGsmSpex, r.Client, sfSet, gsmPod, r.Log)
		if err != nil {
			shardingv1.LogMessages("INFO", "Error Occurred during GSM update operation.", nil, instance, r.Log)
			result = resultNq
			return result, err
		}
	}

	// Calling updateShardTopology to update the entire sharding topology
	// This is required because we just executed updateShard,updateCatalog and UpdateGsm
	// If some state has changed it will update the topology

	err = r.updateShardTopologyStatus(instance)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		err = nilErr
		return result, err
	}

	stateType = string(databasev1alpha1.CrdReconcileCompeleteState)
	//	r.setCrdLifeCycleState(instance, &result, &err, stateType)
	// Set error to ni to avoid reconcilation state reconcilation error as we are passing err to setCrdLifeCycleState

	shardingv1.LogMessages("INFO", "Completed the Sharding topology setup reconcilation loop.", nil, instance, r.Log)
	result = resultNq
	err = nilErr
	return result, err
}

// SetupWithManager sets up the controller with the Manager.
// The default concurrent reconcilation loop is 1
// Check https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controller#Options to under MaxConcurrentReconciles
func (r *ShardingDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.ShardingDatabase{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}). //MaxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1
		Complete(r)
}

// ###################### Event Filter Predicate ######################
func (r *ShardingDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{

		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, podOk := e.Object.GetLabels()["statefulset.kubernetes.io/pod-name"]
			for i := 0; i < len(r.osh); i++ {
				if r.osh[i] != nil {
					oshInst := r.osh[i]
					if oshInst.deltopology == true {
						break

					}
					if e.Object.GetLabels()[string(databasev1alpha1.ShardingDelLabelKey)] == string(databasev1alpha1.ShardingDelLabelTrueValue) {
						break
					}

					if podOk {
						delObj := e.Object.(*corev1.Pod)
						if e.Object.GetLabels()["type"] == "Shard" && e.Object.GetLabels()["app"] == "OracleSharding" && e.Object.GetLabels()["oralabel"] == oshInst.Instance.Name {

							if delObj.DeletionTimestamp != nil {
								go r.gsmInvitedNodeOp(oshInst.Instance, delObj.Name)
							}
						}

						if e.Object.GetLabels()["type"] == "Catalog" && e.Object.GetLabels()["app"] == "OracleSharding" && e.Object.GetLabels()["oralabel"] == oshInst.Instance.Name {

							if delObj.DeletionTimestamp != nil {
								go r.gsmInvitedNodeOp(oshInst.Instance, delObj.Name)
							}
						}

					}

				}
			}

			return true
		},
	}
}

// ================== Function to get the Notification controller ==============
func (r *ShardingDatabaseReconciler) getOnsConfigProvider(instance *databasev1alpha1.ShardingDatabase, idx int,
) {
	var err error
	if instance.Spec.NsConfigMap != "" && instance.Spec.NsSecret != "" && r.osh[idx].onsProviderFlag != true {
		cmName := instance.Spec.NsConfigMap
		secName := instance.Spec.NsSecret
		shardingv1.LogMessages("DEBUG", "Received parameters are "+shardingv1.GetFmtStr(cmName)+","+shardingv1.GetFmtStr(secName), nil, instance, r.Log)
		region, user, tenancy, passphrase, fingerprint, topicid := shardingv1.ReadConfigMap(cmName, instance, r.Client, r.Log)
		privatekey := shardingv1.ReadSecret(secName, instance, r.Client, r.Log)
		r.osh[idx].topicid = topicid
		r.osh[idx].onsProvider = common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, privatekey, &passphrase)
		r.osh[idx].rclient, err = ons.NewNotificationDataPlaneClientWithConfigurationProvider(r.osh[idx].onsProvider)
		if err != nil {
			msg := "Error occurred in getting the OCI notification service based client."
			r.osh[idx].onsProviderFlag = false
			r.Log.Error(err, msg)
			shardingv1.LogMessages("Error", msg, nil, instance, r.Log)
		} else {
			r.osh[idx].onsProviderFlag = true
		}

	}
}

// ================== Function the Message  ==============
func (r *ShardingDatabaseReconciler) sendMessage(instance *databasev1alpha1.ShardingDatabase, title string, body string) {
	idx, instFlag := r.checkProvInstance(instance)
	if instFlag {
		if r.osh[idx].onsProviderFlag {
			shardingv1.SendNotification(title, body, instance, r.osh[idx].topicid, r.osh[idx].rclient, r.Log)
		}
	}
}

func (r *ShardingDatabaseReconciler) publishEvents(instance *databasev1alpha1.ShardingDatabase, eventMsg string, state string) {

	if state == string(databasev1alpha1.AvailableState) || state == string(databasev1alpha1.AddingShardState) || state == string(databasev1alpha1.ShardOnlineState) || state == string(databasev1alpha1.ProvisionState) || state == string(databasev1alpha1.DeletingState) || state == string(databasev1alpha1.Terminated) {
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "State Change", eventMsg)
	} else {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "State Change", eventMsg)

	}

}

// ================== Function to check insytance deletion timestamp and activate the finalizer code ========
func (r *ShardingDatabaseReconciler) finalizerShardingDatabaseInstance(instance *databasev1alpha1.ShardingDatabase,
) (error, bool) {

	isProvOShardToBeDeleted := instance.GetDeletionTimestamp() != nil
	if isProvOShardToBeDeleted {
		if controllerutil.ContainsFinalizer(instance, shardingv1.ShardingDatabaseFinalizer) {
			// Run finalization logic for finalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeShardingDatabase(instance); err != nil {
				return err, false
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(instance, shardingv1.ShardingDatabaseFinalizer)
			err := r.Client.Update(context.TODO(), instance)
			if err != nil {
				return err, false
			}
		}
		// Send true because delete is in progress and it is a custom delete message
		// We don't need to print custom err stack as we are deleting the topology
		return fmt.Errorf("Delete of the sharding topology is in progress"), true
	}

	// Add finalizer for this CR
	if instance.DeletionTimestamp == nil {
		if !controllerutil.ContainsFinalizer(instance, shardingv1.ShardingDatabaseFinalizer) {
			if err := r.addFinalizer(instance); err != nil {
				return err, false
			}
		}
	}

	return nil, false
}

// ========================== FInalizer Section ===================
func (r *ShardingDatabaseReconciler) addFinalizer(instance *databasev1alpha1.ShardingDatabase) error {
	reqLogger := r.Log.WithValues("instance.Spec.Namespace", instance.Spec.Namespace, "instance.Name", instance.Name)
	controllerutil.AddFinalizer(instance, shardingv1.ShardingDatabaseFinalizer)

	// Update CR
	err := r.Client.Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Failed to update Sharding Database  with finalizer")
		return err
	}
	return nil
}

func (r *ShardingDatabaseReconciler) finalizeShardingDatabase(instance *databasev1alpha1.ShardingDatabase) error {
	// TODO(user): Add the cleanup steps that the operator needs to do before the CR
	// can be deleted. Examples of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	var i int32
	var err error
	var pvcName string

	idx, _ := r.checkProvInstance(instance)
	sfSetFound := &appsv1.StatefulSet{}
	svcFound := &corev1.Service{}
	r.osh[idx].deltopology = true
	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex := instance.Spec.Shard[i]
			sfSetFound, err = shardingv1.CheckSfset(OraShardSpex.Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), sfSetFound)
				if err != nil {
					return err
				}
				if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
					pvcName = OraShardSpex.Name + "-oradata-vol4-" + OraShardSpex.Name + "-0"
					err = shardingv1.DelPvc(pvcName, instance, r.Client, r.Log)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	if len(instance.Spec.Gsm) > 0 {
		for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
			OraGsmSpex := instance.Spec.Gsm[i]
			sfSetFound, err = shardingv1.CheckSfset(OraGsmSpex.Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), sfSetFound)
				if err != nil {
					return err
				}
				if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
					pvcName = OraGsmSpex.Name + "-oradata-vol4-" + OraGsmSpex.Name + "-0"
					err = shardingv1.DelPvc(pvcName, instance, r.Client, r.Log)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	if len(instance.Spec.Catalog) > 0 {
		for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
			OraCatalogSpex := instance.Spec.Catalog[i]
			// See if StatefulSets already exists and create if it doesn't
			sfSetFound, err = shardingv1.CheckSfset(OraCatalogSpex.Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), sfSetFound)
				if err != nil {
					return err
				}
				if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
					pvcName = OraCatalogSpex.Name + "-oradata-vol4-" + OraCatalogSpex.Name + "-0"
					err = shardingv1.DelPvc(pvcName, instance, r.Client, r.Log)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			if instance.Spec.IsExternalSvc {
				svcFound, err = shardingv1.CheckSvc(instance.Spec.Shard[i].Name+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
				if err == nil {
					// See if StatefulSets already exists and create if it doesn't
					err = r.Client.Delete(context.Background(), svcFound)
					if err != nil {
						return err
					}
				}
			}
			svcFound, err = shardingv1.CheckSvc(instance.Spec.Shard[i].Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), svcFound)
				if err != nil {
					return err
				}
			}
		}
	}

	if len(instance.Spec.Catalog) > 0 {
		for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
			if instance.Spec.IsExternalSvc {
				svcFound, err = shardingv1.CheckSvc(instance.Spec.Catalog[i].Name+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
				if err == nil {
					// See if StatefulSets already exists and create if it doesn't
					err = r.Client.Delete(context.Background(), svcFound)
					if err != nil {
						return err
					}
				}
			}
			svcFound, err = shardingv1.CheckSvc(instance.Spec.Catalog[i].Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), svcFound)
				if err != nil {
					return err
				}
			}
		}
	}

	if len(instance.Spec.Gsm) > 0 {
		for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
			// See if StatefulSets already exists and create if it doesn't
			if len(instance.Spec.Gsm[i].PvcName) == 0 {
				if instance.Spec.IsExternalSvc {
					svcFound, err = shardingv1.CheckSvc(instance.Spec.Gsm[i].Name+strconv.FormatInt(int64(i), 10)+"-svc", instance, r.Client)
					if err == nil {
						// See if StatefulSets already exists and delete if it doesn't
						err = r.Client.Delete(context.Background(), svcFound)
						if err != nil {
							return err
						}
					}
				}
				svcFound, err = shardingv1.CheckSvc(instance.Spec.Gsm[i].Name, instance, r.Client)
				if err == nil {
					// See if StatefulSets already exists and delete if it doesn't
					err = r.Client.Delete(context.Background(), svcFound)
					if err != nil {
						return err
					}
				}
				if instance.Spec.IsExternalSvc {
					svcFound, err = shardingv1.CheckSvc(instance.Spec.Gsm[i].Name+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
					if err == nil {
						// See if StatefulSets already exists and create if it doesn't
						err = r.Client.Delete(context.Background(), svcFound)
						if err != nil {
							return err
						}
					}
				}
			} else {
				if instance.Spec.IsExternalSvc {
					svcFound, err = shardingv1.CheckSvc(instance.Spec.Gsm[i].Name+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
					if err == nil {
						// See if StatefulSets already exists and create if it doesn't
						err = r.Client.Delete(context.Background(), svcFound)
						if err != nil {
							return err
						}
					}
				}
				svcFound, err = shardingv1.CheckSvc(instance.Spec.Gsm[i].Name, instance, r.Client)
				if err == nil {
					// See if StatefulSets already exists and create if it doesn't
					err = r.Client.Delete(context.Background(), svcFound)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	// List the stateful for this instance's statefulset  and delete the all the stateful set which belong to this instance as a left over
	sfList := &appsv1.StatefulSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(shardingv1.LabelsForProvShardKind(instance, "shard")),
	}

	err = r.Client.List(context.TODO(), sfList, listOpts...)
	if err == nil {
		for _, sset := range sfList.Items {
			sfSetFound, err = shardingv1.CheckSfset(sset.Name, instance, r.Client)
			if err == nil {
				// See if StatefulSets already exists and create if it doesn't
				err = r.Client.Delete(context.Background(), sfSetFound)
				if err != nil {
					return err
				}
			}
		}
	}

	r.osh[idx].deltopology = false
	//r.osh[idx].addSem.Release(1)
	//r.osh[idx].delSem.Release(1)
	//instance1 := &shardingv1alpha1.ProvShard{}
	r.osh[idx].Instance = &databasev1alpha1.ShardingDatabase{}

	//r.osh[idx] = nil

	return nil
}

//==============

// Get the current instance
func (r *ShardingDatabaseReconciler) checkProvInstance(instance *databasev1alpha1.ShardingDatabase,
) (int, bool) {

	var status bool = false
	var idx int
	for i := 0; i < len(r.osh); i++ {
		idx = i
		if r.osh[i] != nil {
			if !r.osh[i].deltopology {
				if r.osh[i].Instance.Name == instance.Name {
					status = true
					break
				}
			}
		}
	}
	return idx, status
}

// =========== validate Specs ============
func (r *ShardingDatabaseReconciler) validateSpex(instance *databasev1alpha1.ShardingDatabase, idx int) error {

	var eventMsg string
	var eventErr string = "Spec Error"

	lastSuccSpec, err := instance.GetLastSuccessfulSpec()
	if err != nil {
		return nil
	}

	// Check if last Successful update nil or not
	if lastSuccSpec == nil {
		// Logic to check if inital Spec is good or not

		// Once the initial Spec is been validated then update the last Sucessful Spec
		err = instance.UpdateLastSuccessfulSpec(r.Client)
		if err != nil {
			return err
		}
	} else {
		// if the last sucessful spec is not nil
		// check the parameters which cannot be changed
		if lastSuccSpec.Namespace != instance.Spec.Namespace {
			eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " namespace changed from " + shardingv1.GetFmtStr(lastSuccSpec.Namespace) + " to " + shardingv1.GetFmtStr(instance.Spec.Namespace) + ". This change is not allowed."
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
			return fmt.Errorf("instance spec has changed and namespace change is not supported")
		}

		if lastSuccSpec.DbImage != instance.Spec.DbImage {
			eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " DBImage changed from " + shardingv1.GetFmtStr(lastSuccSpec.DbImage) + " to " + shardingv1.GetFmtStr(instance.Spec.DbImage) + ". This change is not allowed."
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
			return fmt.Errorf("instance spec has changed and DbImage change is not supported")
		}

		if lastSuccSpec.GsmImage != instance.Spec.GsmImage {
			eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " GsmImage changed from " + shardingv1.GetFmtStr(lastSuccSpec.GsmImage) + " to " + shardingv1.GetFmtStr(instance.Spec.GsmImage) + ". This change is not allowed."
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
			return fmt.Errorf("instance spec has changed and GsmImage change is not supported")
		}

		if lastSuccSpec.StorageClass != instance.Spec.StorageClass {
			eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " StorageClass changed from " + shardingv1.GetFmtStr(lastSuccSpec.StorageClass) + " to " + shardingv1.GetFmtStr(instance.Spec.StorageClass) + ". This change is not allowed."
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
			return fmt.Errorf("instance spec has changed and StorageClass change is not supported")
		}

		// Compare Env variables for shard begins here
		if !r.comapreShardEnvVariables(instance, lastSuccSpec) {
			return fmt.Errorf("Change of Shard env variables are not")
		}
		// Compare Env variables for catalog begins here
		if !r.comapreCatalogEnvVariables(instance, lastSuccSpec) {
			return fmt.Errorf("Change of Catalog env variables are not")
		}
		// Compare env variable for Catalog ends here
		if !r.comapreGsmEnvVariables(instance, lastSuccSpec) {
			return fmt.Errorf("Change of GSM env variables are not")
		}

	}
	return nil
}

// Compare GSM Env Variables

func (r *ShardingDatabaseReconciler) comapreGsmEnvVariables(instance *databasev1alpha1.ShardingDatabase, lastSuccSpec *databasev1alpha1.ShardingDatabaseSpec) bool {
	var eventMsg string
	var eventErr string = "Spec Error"
	var i, j int32

	if len(instance.Spec.Gsm) > 0 {
		for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
			OraGsmSpex := instance.Spec.Gsm[i]
			for j = 0; j < int32(len(lastSuccSpec.Gsm)); j++ {
				if OraGsmSpex.Name == lastSuccSpec.Gsm[j].Name {
					if !reflect.DeepEqual(OraGsmSpex.EnvVars, lastSuccSpec.Gsm[j].EnvVars) {
						eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " env vairable changes are not supported."
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
						return false
					}
				}
				// child for loop ens here
			}
			//Main  For loop ends here
		}
	}

	return true
}

func (r *ShardingDatabaseReconciler) comapreCatalogEnvVariables(instance *databasev1alpha1.ShardingDatabase, lastSuccSpec *databasev1alpha1.ShardingDatabaseSpec) bool {
	var eventMsg string
	var eventErr string = "Spec Error"
	var i, j int32

	if len(instance.Spec.Catalog) > 0 {
		for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
			OraCatalogSpex := instance.Spec.Catalog[i]
			for j = 0; j < int32(len(lastSuccSpec.Catalog)); j++ {
				if OraCatalogSpex.Name == lastSuccSpec.Catalog[j].Name {
					if !reflect.DeepEqual(OraCatalogSpex.EnvVars, lastSuccSpec.Catalog[j].EnvVars) {
						eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " env vairable changes are not supported."
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
						return false
					}
				}
				// child for loop ens here
			}
			//Main  For loop ends here
		}
	}

	return true
}

func (r *ShardingDatabaseReconciler) comapreShardEnvVariables(instance *databasev1alpha1.ShardingDatabase, lastSuccSpec *databasev1alpha1.ShardingDatabaseSpec) bool {
	var eventMsg string
	var eventErr string = "Spec Error"
	var i, j int32

	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex := instance.Spec.Shard[i]
			for j = 0; j < int32(len(lastSuccSpec.Shard)); j++ {
				if OraShardSpex.Name == lastSuccSpec.Shard[j].Name {
					if !reflect.DeepEqual(OraShardSpex.EnvVars, lastSuccSpec.Shard[j].EnvVars) {
						eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " env vairable changes are not supported."
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
						return false
					}
				}
				// child for loop ens here
			}
			//Main  For loop ends here
		}
	}

	return true
}

//===== Set the CRD resource life cycle state ========

func (r *ShardingDatabaseReconciler) setCrdLifeCycleState(instance *databasev1alpha1.ShardingDatabase, result *ctrl.Result, err *error, stateType *string) {

	var metaCondition metav1.Condition
	var updateFlag = false
	if *stateType == "ReconcileWaiting" {
		metaCondition = shardingv1.GetMetaCondition(instance, result, err, *stateType, string(databasev1alpha1.CrdReconcileWaitingReason))
		updateFlag = true
	} else if *stateType == "ReconcileComplete" {
		metaCondition = shardingv1.GetMetaCondition(instance, result, err, *stateType, string(databasev1alpha1.CrdReconcileCompleteReason))
		updateFlag = true
	} else if result.Requeue {
		metaCondition = shardingv1.GetMetaCondition(instance, result, err, string(databasev1alpha1.CrdReconcileQueuedState), string(databasev1alpha1.CrdReconcileQueuedReason))
		updateFlag = true
	} else if *err != nil {
		metaCondition = shardingv1.GetMetaCondition(instance, result, err, string(databasev1alpha1.CrdReconcileErrorState), string(databasev1alpha1.CrdReconcileErrorReason))
		updateFlag = true
	} else {

	}
	if updateFlag == true {
		if len(instance.Status.CrdStatus) > 0 {
			//meta.SetStatusCondition()
			meta.RemoveStatusCondition(&instance.Status.CrdStatus, metaCondition.Type)
		}
		meta.SetStatusCondition(&instance.Status.CrdStatus, metaCondition)
		// Always refresh status before a reconcile
		r.Client.Status().Update(context.TODO(), instance)
	}

}

func (r *ShardingDatabaseReconciler) validateGsmnCatalog(instance *databasev1alpha1.ShardingDatabase) error {
	var err error
	_, _, err = r.validateCatalog(instance)
	if err != nil {
		return err
	}
	_, _, err = r.validateGsm(instance)
	if err != nil {
		return err
	}
	return nil
}

func (r *ShardingDatabaseReconciler) validateGsm(instance *databasev1alpha1.ShardingDatabase,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	//var err error
	var i int32
	//var err error
	availableFlag := false

	gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}

	for i = 0; i < int32(len(instance.Spec.Gsm)); i++ {
		gsmSfSet1, gsmPod1, err := r.validateInvidualGsm(instance, instance.Spec.Gsm[i], int(i))
		if err == nil {
			if availableFlag != true {
				gsmSfSet = gsmSfSet1
				gsmPod = gsmPod1
				availableFlag = true
			}
		}
	}

	if availableFlag == true {
		return gsmSfSet, gsmPod, nil
	}
	return gsmSfSet, gsmPod, fmt.Errorf("GSM is not ready")
}

func (r *ShardingDatabaseReconciler) validateInvidualGsm(instance *databasev1alpha1.ShardingDatabase, OraGsmSpex databasev1alpha1.GsmSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	//var err error
	var i int32
	var msg string
	gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}

	var err error
	podList := &corev1.PodList{}
	var isPodExist bool

	gsmSfSet, err = shardingv1.CheckSfset(OraGsmSpex.Name, instance, r.Client)
	if err != nil {
		msg = "Unable to find  GSM statefulset " + shardingv1.GetFmtStr(OraGsmSpex.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, int(i), string(databasev1alpha1.StatefulSetNotFound))
		return gsmSfSet, gsmPod, err
	}

	podList, err = shardingv1.GetPodList(gsmSfSet.Name, "GSM", instance, r.Client)
	if err != nil {
		msg = "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(gsmSfSet.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, int(i), string(databasev1alpha1.PodNotFound))
		return gsmSfSet, gsmPod, err
	}

	isPodExist, gsmPod = shardingv1.PodListValidation(podList, gsmSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg = "Unable to validate GSM " + shardingv1.GetFmtStr(gsmPod.Name) + " pod. GSM pod doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, int(i), string(databasev1alpha1.PodNotReadyState))
		return gsmSfSet, gsmPod, fmt.Errorf("Pod doesn't exist")
	}
	err = shardingv1.CheckGsmStatus(gsmPod.Name, instance, r.kubeClient, r.kubeConfig, r.Log)
	if err != nil {
		msg = "Unable to validate GSM director. GSM director doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, int(i), string(databasev1alpha1.ProvisionState))
		return gsmSfSet, gsmPod, err
	}

	r.updateGsmStatus(instance, specId, string(databasev1alpha1.AvailableState))
	return gsmSfSet, gsmPod, nil
}

func (r *ShardingDatabaseReconciler) validateCatalog(instance *databasev1alpha1.ShardingDatabase,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	catalogSfSet := &appsv1.StatefulSet{}
	catalogPod := &corev1.Pod{}
	//var err error
	var i int32
	availlableFlag := false

	for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
		catalogSfSet1, catalogPod1, err := r.validateInvidualCatalog(instance, instance.Spec.Catalog[i], int(i))
		if err == nil {
			if availlableFlag != true {
				catalogSfSet = catalogSfSet1
				catalogPod = catalogPod1
				availlableFlag = true
			}
		}
	}

	if availlableFlag == true {
		return catalogSfSet, catalogPod, nil
	}

	return catalogSfSet, catalogPod, fmt.Errorf("Catalog is not available")
}

// === Validate Individual Catalog
func (r *ShardingDatabaseReconciler) validateInvidualCatalog(instance *databasev1alpha1.ShardingDatabase, OraCatalogSpex databasev1alpha1.CatalogSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	catalogSfSet := &appsv1.StatefulSet{}
	catalogPod := &corev1.Pod{}
	podList := &corev1.PodList{}
	var isPodExist bool

	catalogSfSet, err = shardingv1.CheckSfset(OraCatalogSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Catalog statefulset " + shardingv1.GetFmtStr(OraCatalogSpex.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev1alpha1.StatefulSetNotFound))
		return catalogSfSet, catalogPod, err
	}

	podList, err = shardingv1.GetPodList(catalogSfSet.Name, "CATALOG", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(catalogSfSet.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev1alpha1.PodNotFound))
		return catalogSfSet, catalogPod, err
	}
	isPodExist, catalogPod = shardingv1.PodListValidation(podList, catalogSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Catalog " + shardingv1.GetFmtStr(catalogSfSet.Name) + " pod. Catalog pod doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev1alpha1.PodNotReadyState))
		return catalogSfSet, catalogPod, fmt.Errorf("Pod doesn't exist")
	}
	err = shardingv1.ValidateDbSetup(catalogPod.Name, instance, r.kubeClient, r.kubeConfig, r.Log)
	if err != nil {
		msg := "Unable to validate Catalog. Catalog doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev1alpha1.ProvisionState))
		return catalogSfSet, catalogPod, err
	}

	r.updateCatalogStatus(instance, specId, string(databasev1alpha1.AvailableState))
	return catalogSfSet, catalogPod, nil

}

// ======= Function to validate Shard
func (r *ShardingDatabaseReconciler) validateShard(instance *databasev1alpha1.ShardingDatabase, OraShardSpex databasev1alpha1.ShardSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	shardSfSet := &appsv1.StatefulSet{}
	shardPod := &corev1.Pod{}

	shardSfSet, err = shardingv1.CheckSfset(OraShardSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Shard statefulset " + shardingv1.GetFmtStr(OraShardSpex.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev1alpha1.StatefulSetNotFound))
		return shardSfSet, shardPod, err
	}

	podList, err := shardingv1.GetPodList(shardSfSet.Name, "SHARD", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(shardSfSet.Name) + "."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev1alpha1.PodNotFound))
		return shardSfSet, shardPod, err
	}
	isPodExist, shardPod := shardingv1.PodListValidation(podList, shardSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Shard " + shardingv1.GetFmtStr(shardPod.Name) + " pod. Shard pod doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev1alpha1.PodNotReadyState))
		return shardSfSet, shardPod, err
	}
	err = shardingv1.ValidateDbSetup(shardPod.Name, instance, r.kubeClient, r.kubeConfig, r.Log)
	if err != nil {
		msg := "Unable to validate shard. Shard doesn't seems to be ready to accept the commands."
		shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev1alpha1.ProvisionState))
		return shardSfSet, shardPod, err
	}

	r.updateShardStatus(instance, specId, string(databasev1alpha1.AvailableState))
	return shardSfSet, shardPod, nil
}

// This function updates the shard topology over all
//
func (r *ShardingDatabaseReconciler) updateShardTopologyStatus(instance *databasev1alpha1.ShardingDatabase) error {
	//shardPod := &corev1.Pod{}
	//gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}
	var err error
	_, _, err = r.validateCatalog(instance)
	if err != nil {
		return err
	}
	_, gsmPod, err = r.validateGsm(instance)
	if err != nil {
		return err
	}
	r.updateShardTopologyShardsInGsm(instance, gsmPod)

	return nil

}

func (r *ShardingDatabaseReconciler) updateShardTopologyShardsInGsm(instance *databasev1alpha1.ShardingDatabase, gsmPod *corev1.Pod) {
	shardSfSet := &appsv1.StatefulSet{}
	//shardPod := &corev1.Pod{}
	//gsmSfSet := &appsv1.StatefulSet{}
	var err error
	var i int32
	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex := instance.Spec.Shard[i]
			//	stateStr := shardingv1.GetGsmShardStatus(instance, OraShardSpex.Name)
			if OraShardSpex.IsDelete != true {
				shardSfSet, _, err = r.validateShard(instance, OraShardSpex, int(i))
				if err != nil {
					continue
				} else {
					_ = r.verifyShards(instance, gsmPod, shardSfSet)
				}
			}

		}
	}
}

func (r *ShardingDatabaseReconciler) updateGsmStatus(instance *databasev1alpha1.ShardingDatabase, specIdx int, state string) {

	var currState string
	var eventMsg string
	var eventMsgFlag = true

	// Populating GSM Details
	name := instance.Spec.Gsm[specIdx].Name
	//ServiceNames := shardingv1.GetGsmSvcName(instance.Spec.Gsm[specIdx].EnvVars)

	if len(instance.Status.Gsm.State) > 0 {
		currState = instance.Status.Gsm.State
		if currState == state {
			eventMsgFlag = false
		}
		eventMsg = "The gsm " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state
	} else {
		eventMsg = "The gsm " + shardingv1.GetFmtStr(name) + " state set to " + state
	}

	//	if currState != state {
	instance.Status.Gsm.State = state
	shardingv1.UpdateGsmStatusData(instance, specIdx, state, r.kubeClient, r.kubeConfig, r.Log)
	r.Status().Update(context.Background(), instance)
	//	}
	if eventMsgFlag == true {
		r.publishEvents(instance, eventMsg, state)
	}
}

func (r *ShardingDatabaseReconciler) updateCatalogStatus(instance *databasev1alpha1.ShardingDatabase, specIdx int, state string) {
	var eventMsg string
	var currState string
	var eventMsgFlag = true

	name := instance.Spec.Catalog[specIdx].Name

	if len(instance.Status.Catalog) > 0 {
		currState = shardingv1.GetGsmCatalogStatusKey(instance, name+"_"+string(databasev1alpha1.State))
		if currState == state {
			eventMsgFlag = false
		}
		eventMsg = "The catalog " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state

	} else {
		eventMsg = "The catalog " + shardingv1.GetFmtStr(name) + " state set to " + state
	}

	//if currState != state {
	shardingv1.UpdateCatalogStatusData(instance, specIdx, state, r.kubeClient, r.kubeConfig, r.Log)
	r.Status().Update(context.Background(), instance)
	//}
	if eventMsgFlag == true {
		r.publishEvents(instance, eventMsg, state)
	}
}

func (r *ShardingDatabaseReconciler) updateShardStatus(instance *databasev1alpha1.ShardingDatabase, specIdx int, state string) {
	var eventMsg string
	var currState string
	var eventMsgFlag = true

	name := instance.Spec.Shard[specIdx].Name
	if len(instance.Status.Shard) > 0 {
		currState = shardingv1.GetGsmShardStatusKey(instance, name+"_"+string(databasev1alpha1.State))
		if currState == state {
			eventMsgFlag = false
		}
		eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state

	} else {
		eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state set to " + state
	}

	//if currState != state {
	shardingv1.UpdateShardStatusData(instance, specIdx, state, r.kubeClient, r.kubeConfig, r.Log)
	r.Status().Update(context.Background(), instance)
	//}
	if eventMsgFlag == true {
		r.publishEvents(instance, eventMsg, state)
	}
}

func (r *ShardingDatabaseReconciler) updateGsmShardStatus(instance *databasev1alpha1.ShardingDatabase, name string, state string) {
	var eventMsg string
	var currState string
	var eventMsgFlag = true

	if len(instance.Status.Gsm.Shards) > 0 {
		currState = shardingv1.GetGsmShardStatus(instance, name)
		if currState == state {
			eventMsgFlag = false
		}
		if currState != "NOSTATE" {
			eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state + " in Gsm"
		} else {
			eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state set to " + state + " in Gsm."
		}

	} else {
		eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state set to " + state + " in Gsm."
	}

	if currState != state {
		shardingv1.UpdateGsmShardStatus(instance, name, state)
		r.Status().Update(context.Background(), instance)
	}

	if eventMsgFlag == true {
		r.publishEvents(instance, eventMsg, state)
	}
}

// This function add the Primary Shards in GSM
func (r *ShardingDatabaseReconciler) addPrimaryShards(instance *databasev1alpha1.ShardingDatabase, idx int) error {
	//var result ctrl.Result
	var result ctrl.Result
	var i int32
	var err error
	shardSfSet := &appsv1.StatefulSet{}
	//shardPod := &corev1.Pod{}
	//gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}
	var sparams1 string
	var deployFlag = false
	var errStr = false
	//var msg string

	var setLifeCycleFlag = false
	var title string
	var message string

	shardingv1.LogMessages("DEBUG", "Starting the shard adding operaiton.", nil, instance, r.Log)
	// ================================ Add Shard  Logic ===================
	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex := instance.Spec.Shard[i]
			//	stateStr := shardingv1.GetGsmShardStatus(instance, OraShardSpex.Name)
			//	!strings.Contains(stateStr, "DELETE")
			if OraShardSpex.IsDelete != true {
				if setLifeCycleFlag != true {
					setLifeCycleFlag = true
					stateType := string(databasev1alpha1.CrdReconcileWaitingState)
					r.setCrdLifeCycleState(instance, &result, &err, &stateType)
				}
				// 1st Step is to check if Shard is in good state if not then just continue
				// validateShard will change  the shard state in Shard Status
				shardSfSet, _, err = r.validateShard(instance, OraShardSpex, int(i))
				if err != nil {
					errStr = true
					continue
				}
				// 2nd Step is to check if GSM is in good state if not then just return because you can't do anything
				_, gsmPod, err = r.validateGsm(instance)
				if err != nil {
					return err
				}
				// 3rd step to check if shard is in GSM if not then continue
				sparams := shardingv1.BuildShardParams(shardSfSet)
				sparams1 = sparams
				err = shardingv1.CheckShardInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err == nil {
					// if you are in this block then it means that shard already exist in the GSM and we do not need to anything
					continue
				}
				// If the shard doesn't exist in GSM then just add the shard statefulset and update GSM shard status
				// ADD Shard in GSM
				r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.AddingShardState))
				err := shardingv1.AddShardInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.AddingShardErrorState))
					title = "Shard Addition Failure"
					message = "Error occurred during shard " + shardingv1.GetFmtStr(OraShardSpex.Name) + " addition."
					r.sendMessage(instance, title, message)
				} else {
					deployFlag = true
				}
			}
		}

		// ======= Deploy Shard Logic =========
		if deployFlag == true {
			_ = shardingv1.DeployShardInGsm(gsmPod.Name, sparams1, instance, r.kubeClient, r.kubeConfig, r.Log)
			r.updateShardTopologyShardsInGsm(instance, gsmPod)
		}
	}
	if errStr == true {
		shardingv1.LogMessages("INFO", "Some shards are still pending for addition. Requeue the reconcile loop.", nil, instance, r.Log)
		return fmt.Errorf("Shard Addition is pending.")
	}
	shardingv1.LogMessages("INFO", "Completed the shard addition operation. For details, check the CRD resource status for GSM and Shards.", nil, instance, r.Log)
	return nil
}

// This function Check the online shard
func (r *ShardingDatabaseReconciler) verifyShards(instance *databasev1alpha1.ShardingDatabase, gsmPod *corev1.Pod, shardSfSet *appsv1.StatefulSet) error {
	//var result ctrl.Result
	//var i int32
	var err error
	var title string
	var message string
	// ================================ Check  Shards  ==================
	//veryify shard make shard state online and it must be executed to check shard state after every CRUD operation
	sparams := shardingv1.BuildShardParams(shardSfSet)
	err = shardingv1.CheckOnlineShardInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
	if err != nil {
		// If the shard doesn't exist in GSM then just delete the shard statefulset and update GSM shard status
		/// Terminate state means we will remove teh shard entry from GSM shard status
		r.updateGsmShardStatus(instance, shardSfSet.Name, string(databasev1alpha1.ShardOnlineErrorState))
		shardingv1.CancelChunksInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
		return err
	}
	oldStateStr := shardingv1.GetGsmShardStatus(instance, shardSfSet.Name)
	r.updateGsmShardStatus(instance, shardSfSet.Name, string(databasev1alpha1.ShardOnlineState))
	// Following logic will sent a email only once
	if oldStateStr != string(databasev1alpha1.ShardOnlineState) {
		title = "Shard Addition Completed"
		message = "Shard addition completed for shard " + shardingv1.GetFmtStr(shardSfSet.Name) + " in GSM."
		r.sendMessage(instance, title, message)
	}
	return nil
}

func (r *ShardingDatabaseReconciler) addStandbyShards(instance *databasev1alpha1.ShardingDatabase, idx int) error {
	//var result ctrl.Result

	return nil
}

// ========== Delete Shard Section====================
func (r *ShardingDatabaseReconciler) delGsmShard(instance *databasev1alpha1.ShardingDatabase, idx int) error {
	var result ctrl.Result
	var i int32
	var err error
	shardSfSet := &appsv1.StatefulSet{}
	shardPod := &corev1.Pod{}
	//gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}
	var msg string
	var title string
	var message string
	var setLifeCycleFlag = false

	shardingv1.LogMessages("DEBUG", "Starting shard deletion operation.", nil, instance, r.Log)
	// ================================ Shard Delete Logic ===================
	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex := instance.Spec.Shard[i]
			if OraShardSpex.IsDelete == true {
				if setLifeCycleFlag != true {
					setLifeCycleFlag = true
					stateType := string(databasev1alpha1.CrdReconcileWaitingState)
					r.setCrdLifeCycleState(instance, &result, &err, &stateType)
				}
				// Step 1st to check if GSM is in good state if not then just return because you can't do anything
				_, gsmPod, err = r.validateGsm(instance)
				if err != nil {
					return err
				}
				// 2nd Step is to check if Shard is in good state if not then just continue
				// 1St check if the instance.Status.Gsm.Shards contains the shard. If not then shard is already deleted
				// If the shard is found then check if shard exist
				// validateShard will change  the shard state in Shard Status
				chkState := shardingv1.GetGsmShardStatus(instance, OraShardSpex.Name)
				if chkState != "NOSTATE" {
					shardSfSet, shardPod, err = r.validateShard(instance, OraShardSpex, int(i))
					if err != nil {
						continue
					}
				} else {
					continue
				}
				// 3rd step to check if shard is in GSM if not then continue
				sparams := shardingv1.BuildShardParams(shardSfSet)
				err = shardingv1.CheckShardInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					// If the shard doesn't exist in GSM then just delete the shard statefulset and update GSM shard status
					/// Terminate state means we will remove teh shard entry from GSM shard status
					r.delShard(instance, shardSfSet.Name, shardSfSet, shardPod, int(i))
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.Terminated))
					r.updateShardStatus(instance, int(i), string(databasev1alpha1.Terminated))
					continue
				}
				// 4th step to check if shard is in GSM and shard is online if not then continue
				// CHeck before deletion if GSM is not ready set the Shard State to Delete Error
				r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.DeletingState))
				err = shardingv1.CheckOnlineShardInGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					// If the shard doesn't exist in GSM then just delete the shard statefulset and update GSM shard status
					/// Terminate state means we will remove teh shard entry from GSM shard status
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.DeleteErrorState))
					continue
				}
				// 5th Step
				// Move the chunks before performing any Delete
				// If you are in this block then it means that shard is ONline and can be deleted
				err = shardingv1.MoveChunks(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.ChunkMoveError))
					title = "Chunk Movement Failure"
					message = "Error occurred during chunk movement in shard " + shardingv1.GetFmtStr(OraShardSpex.Name) + " deletion."
					r.sendMessage(instance, title, message)
					continue
				}
				// 6th Step
				// Check if Chunks has moved before performing actual delete
				// This is a loop and will check unless there is a error or chunks has moved
				// Validate if the chunks has moved before performing shard deletion
				for {
					err = shardingv1.VerifyChunks(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
					if err == nil {
						break
					} else {
						msg = "Sleeping for 120 seconds and will check status again of chunks movement in gsm for shard: " + shardingv1.GetFmtStr(OraShardSpex.Name)
						shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
						time.Sleep(120 * time.Second)
					}
				}
				// 7th Step remove the shards from the GSM
				// This steps will delete the shard entry from the GSM
				// It will delete CDB from catalog
				// 6th Step has already moved the chunks so it is safe to delete
				err = shardingv1.RemoveShardFromGsm(gsmPod.Name, sparams, instance, r.kubeClient, r.kubeConfig, r.Log)
				if err != nil {
					msg = "Error occurred during shard" + shardingv1.GetFmtStr(OraShardSpex.Name) + "removal from Gsm"
					shardingv1.LogMessages("Error", msg, nil, instance, r.Log)
					r.updateShardStatus(instance, int(i), string(databasev1alpha1.ShardRemoveError))
					continue
				}
				// 8th Step
				// Delete the Statefulset as all the chunks has moved and Shard can be phyiscally deleted
				r.delShard(instance, shardSfSet.Name, shardSfSet, shardPod, int(i))
				r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev1alpha1.Terminated))
				r.updateShardStatus(instance, int(i), string(databasev1alpha1.Terminated))
				title = "Shard Deletion Completed"
				message = "Shard deletion completed for shard " + shardingv1.GetFmtStr(OraShardSpex.Name) + " in GSM."
				r.sendMessage(instance, title, message)
			}
		}
	}
	shardingv1.LogMessages("DEBUG", "Completed the shard deletion operation. For details, check the CRD resource status for GSM and Shards.", nil, instance, r.Log)
	return nil
}

// This function delete the physical shard
func (r *ShardingDatabaseReconciler) delShard(instance *databasev1alpha1.ShardingDatabase, sfSetName string, sfSetFound *appsv1.StatefulSet, sfsetPod *corev1.Pod, specIdx int) {

	//var status bool
	var err error
	var msg string
	svcFound := &corev1.Service{}

	err = shardingv1.SfsetLabelPatch(sfSetFound, sfsetPod, instance, r.Client)
	if err != nil {
		msg := "Failed to patch the Shard StatefulSet: " + sfSetFound.Name
		shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
		r.updateShardStatus(instance, specIdx, string(databasev1alpha1.LabelPatchingError))
		return
	}

	err = r.Client.Delete(context.Background(), sfSetFound)
	if err != nil {
		msg = "Failed to delete Shard StatefulSet: " + shardingv1.GetFmtStr(sfSetFound.Name)
		shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
		r.updateShardStatus(instance, specIdx, string(databasev1alpha1.DeleteErrorState))
		return
	}
	/// Delete External Service
	if instance.Spec.IsExternalSvc {
		svcFound, err = shardingv1.CheckSvc(sfSetName+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
		if err == nil {
			// See if StatefulSets already exists and create if it doesn't
			err = r.Client.Delete(context.Background(), svcFound)
			if err != nil {
				return
			}
		}
	}

	// Delete Internal Service
	svcFound, err = shardingv1.CheckSvc(sfSetName, instance, r.Client)
	if err == nil {
		// See if StatefulSets already exists and create if it doesn't
		err = r.Client.Delete(context.Background(), svcFound)
		if err != nil {
			return
		}
	}

	if instance.Spec.IsDeleteOraPvc && len(instance.Spec.StorageClass) > 0 {
		pvcName := sfSetFound.Name + "-oradata-vol4-" + sfSetFound.Name + "-0"
		err = shardingv1.DelPvc(pvcName, instance, r.Client, r.Log)
		if err != nil {
			msg = "Failed to delete Shard pvc claim " + shardingv1.GetFmtStr(pvcName)
			shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
			r.updateShardStatus(instance, specIdx, string(databasev1alpha1.DeletePVCError))
		}
	}
}

//======== GSM Invited Node ==========
// Remove and add GSM invited node
func (r *ShardingDatabaseReconciler) gsmInvitedNodeOp(instance *databasev1alpha1.ShardingDatabase, objName string,
) {

	var msg string
	//var err error
	count := 0
	msg = "Inside the  gsmInvitedNodeOp for adding and deleting the invited node "
	shardingv1.LogMessages("DEBUG", msg, nil, instance, r.Log)
	//status =
	for count < 10 {
		_, gsmPodName, err := r.validateGsm(instance)
		if err != nil {
			msg = "Unable to validate gsm sfSet. " + shardingv1.GetFmtStr(gsmPodName.Name) + " Sleeping for 30 seconds"
			shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
			time.Sleep(20 * time.Second)
			count = count + 1
			continue
		}
		err = shardingv1.ValidateDbSetup(objName, instance, r.kubeClient, r.kubeConfig, r.Log)
		if err != nil {
			msg = "Unable to validate sfSet. " + shardingv1.GetFmtStr(objName) + " Sleeping for 30 seconds"
			shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
			time.Sleep(20 * time.Second)
			count = count + 1
			continue
		}
		err, _, _ = shardingv1.ExecCommand(gsmPodName.Name, shardingv1.GetShardInviteNodeCmd(objName), r.kubeClient, r.kubeConfig, instance, r.Log)
		if err != nil {
			msg = "Invite delete and add node failed " + shardingv1.GetFmtStr(objName) + " details in GSM."
			shardingv1.LogMessages("DEBUG", msg, err, instance, r.Log)
		} else {

			msg = "Invited node operation completed sucessfully in GSM after pod " + shardingv1.GetFmtStr(objName) + " restart."
			shardingv1.LogMessages("INFO", msg, nil, instance, r.Log)
			break
		}
		count = count + 1

	}
}

// ================================== CREATE FUNCTIONS =============================
// This function create a service based isExtern parameter set in the yaml file
func (r *ShardingDatabaseReconciler) createService(instance *databasev1alpha1.ShardingDatabase,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Spec.Namespace, "Instance.Name", instance.Name)
	// See if Service already exists and create if it doesn't
	// We are getting error on nil pointer segment when r.scheme is null
	// Error : invalid memory address or nil pointer dereference" (runtime error: invalid memory address or nil pointer dereference)
	// This happens during unit test cases
	for i := 0; i < 5; i++ {
		if r.Scheme == nil {
			time.Sleep(time.Second * 40)
		} else {
			break
		}
	}
	controllerutil.SetControllerReference(instance, dep, r.Scheme)
	found := &corev1.Service{}

	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Spec.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	shardingv1.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && errors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a service")
		err = r.Client.Create(context.TODO(), dep)
		if err != nil {
			// Service creation failed
			reqLogger.Error(err, "Failed to create Service", "Service.space", dep.Namespace, "Service.Name", dep.Name)
			return ctrl.Result{}, err
		} else {
			// Service creation was successful
			return ctrl.Result{Requeue: true}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "Failed to find the  Service details")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// This function deploy the statefulset
func (r *ShardingDatabaseReconciler) deployStatefulSet(instance *databasev1alpha1.ShardingDatabase,
	dep *appsv1.StatefulSet,
	resType string,
) (ctrl.Result, error) {

	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Spec.Namespace, "Instance.Name", instance.Name)
	message := "Inside the deployStatefulSet function"
	shardingv1.LogMessages("DEBUG", message, nil, instance, r.Log)
	// See if StatefulSets already exists and create if it doesn't
	controllerutil.SetControllerReference(instance, dep, r.Scheme)
	found := &appsv1.StatefulSet{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Spec.Namespace,
	}, found)
	jsn, _ := json.Marshal(dep)
	shardingv1.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	if err != nil && errors.IsNotFound(err) {

		// Create the StatefulSet
		reqLogger.Info("Creating Stateful Shard")
		err = r.Client.Create(context.TODO(), dep)

		message := "Inside the create Stateful set block to create statefulset " + shardingv1.GetFmtStr(dep.Name)
		shardingv1.LogMessages("DEBUG", message, nil, instance, r.Log)

		if err != nil {
			// StatefulSet failed
			reqLogger.Error(err, "Failed to create StatefulSet", "StatefulSet.space", dep.Namespace, "StatefulSet.Name", dep.Name)
			//instance.Status.ShardStatus[dep.Name] = "Deployment Failed"
			return ctrl.Result{}, err
		}
	} else if err != nil {
		// Error that isn't due to the StaefulSet not existing
		reqLogger.Error(err, "Failed to get StatefulSet")
		return ctrl.Result{}, err
	}

	message = "Statefulset Exist " + shardingv1.GetFmtStr(dep.Name) + " already exist"
	shardingv1.LogMessages("DEBUG", message, nil, instance, r.Log)

	return ctrl.Result{}, nil
}
