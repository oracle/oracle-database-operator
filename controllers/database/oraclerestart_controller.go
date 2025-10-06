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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	oraclerestartcommon "github.com/oracle/oracle-database-operator/commons/oraclerestart"
	utils "github.com/oracle/oracle-database-operator/commons/oraclerestart/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OracleRestartReconciler reconciles a OracleRestart object
type OracleRestartReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Config     *rest.Config
	kubeClient kubernetes.Interface
	kubeConfig clientcmd.ClientConfig
	Recorder   record.EventRecorder
}

const oracleRestartFinalizer = "database.oracle.com/oraclerestartfinalizer"

//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;secrets;endpoints;services;events;configmaps;persistentvolumes;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OracleRestart object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *OracleRestartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	//ctx := context.Background()
	_ = r.Log.WithValues("oraclerestart", req.NamespacedName)
	r.Log.Info("Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false
	// var svcType string
	var nilErr error = nil

	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}

	oracleRestart := &oraclerestartdb.OracleRestart{}
	configMapData := make(map[string]string)

	// Execute for every reconcile
	defer r.updateReconcileStatus(oracleRestart, ctx, req, &result, &err, &blocked, &completed)

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, oracleRestart)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		oracleRestart.Spec.IsFailed = true
		return resultQ, err
	}

	// Retrieve the old spec from annotations
	oldSpec, err := r.GetOldSpec(oracleRestart)
	if err != nil {
		r.Log.Error(err, "Failed to update old spec annotation")
		oracleRestart.Spec.IsFailed = true
		return resultQ, nil
	}

	// Initialize oracleRestart.Status if it's not already initialized
	if oracleRestart.Status.ConfigParams == nil {
		oracleRestart.Status.ConfigParams = &oraclerestartdb.InitParams{}
	}

	// Initialize ConfigParams fields if they are not already initialized
	if oracleRestart.Status.ConfigParams.DbHome == "" {
		oracleRestart.Status.ConfigParams.DbHome = string(oraclerestartdb.OracleRestartFieldNotDefined)
	}
	if oracleRestart.Status.DbState == "" {
		oracleRestart.Status.State = string(oraclerestartdb.OracleRestartPendingState)
		oracleRestart.Status.DbState = string(oraclerestartdb.OracleRestartPendingState)
		oracleRestart.Status.Role = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.ConnectString = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.PdbConnectString = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.ExternalConnectString = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.ReleaseUpdate = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.ConfigParams.DbHome = string(oraclerestartdb.OracleRestartFieldNotDefined)
		oracleRestart.Status.ConfigParams.GridHome = string(oraclerestartdb.OracleRestartFieldNotDefined)
		r.Status().Update(ctx, oracleRestart)
	}

	// Kube Client Config Setup
	if r.kubeConfig == nil && r.kubeClient == nil {
		r.kubeConfig, r.kubeClient, err = oraclerestartcommon.GetRacK8sClientConfig(r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Manage OracleRestart Deletion , if delete topology is called
	err = r.manageOracleRestartDeletion(req, ctx, oracleRestart)
	if err != nil {
		result = resultNq
		return result, err
	}

	// cleanup RAC Instance
	// This is a special case if user wants just delete the pod and recreate it
	_, err = r.cleanupOracleRestartInstance(req, ctx, oracleRestart)
	if err != nil {
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	// debugging
	err = checkOracleRestartState(oracleRestart)
	if err != nil {
		result = resultQ
		r.Log.Info("Oracle Restart object is in restricted state, returning back")
		return result, nilErr
	}

	// First Validate
	err = r.validateSpex(oracleRestart, oldSpec, ctx)
	if err != nil {
		r.Log.Info("Spec validation failed")
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	err = r.setDefaults(oracleRestart)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	// Update RAC ConfigParams
	err = r.updateGiConfigParamStatus(oracleRestart)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		r.Log.Info(err.Error())
		return result, nilErr
	}

	err = r.updateDbConfigParamStatus(oracleRestart)
	if err != nil {
		//	time.Sleep(30 * time.Second)
		result = resultQ
		r.Log.Info(err.Error())
		err = nilErr
		return result, err
	}

	if oracleRestart.Spec.ConfigParams != nil {
		configMapData, err = r.generateConfigMap(oracleRestart)
		if err != nil {
			result = resultNq
			return result, err
		}
	}
	var svcType string

	result, err = r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, "local"))
	if err != nil {
		result = resultNq
		return result, err
	}

	if len(oracleRestart.Spec.NodePortSvc.PortMappings) != 0 {
		result, err = r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildExternalServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, svcType, "nodeport"))
		if err != nil {
			result = resultNq
			return result, err
		}
	}

	if len(oracleRestart.Spec.LbService.PortMappings) != 0 {
		result, err = r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildExternalServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, svcType, "lbservice"))
		if err != nil {
			result = resultNq
			return result, err
		}
	}

	r.ensureAsmStorageStatus(oracleRestart)
	isNewSetup := true
	for _, n := range oracleRestart.Status.OracleRestartNodes {
		if n.NodeDetails.State == "AVAILABLE" &&
			n.NodeDetails.InstanceState == "OPEN" &&
			n.NodeDetails.PodState == "AVAILABLE" &&
			n.NodeDetails.ClusterState == "HEALTHY" &&
			len(n.NodeDetails.MountedDevices) > 0 {
			// Found at least one node that is healthy and operational
			isNewSetup = false
			break
		}
	}

	isDiskChanged := false
	addedAsmDisks := []string{}
	removedAsmDisks := []string{}
	if !isNewSetup {
		if oldSpec != nil { // old spec required for comparison
			// Check each RAC node for mounted devices
			for index, _ := range oracleRestart.Status.OracleRestartNodes {
				addedAsmDisks, removedAsmDisks = getAddedAndRemovedDisks(oracleRestart, oldSpec, index)
				if len(addedAsmDisks) > 0 && len(removedAsmDisks) > 0 {
					r.Log.Info("Detected Addition as well as Deletion, setup cannot run both together", "addedAsmDisks", addedAsmDisks, "removedAsmDisks", removedAsmDisks)
					result = resultQ
					return result, err
				}
				// You can now use the added and removed disks as needed
				if len(addedAsmDisks) > 0 {
					r.Log.Info("Detected Addition of ASM Disks:", "addedAsmDisks", addedAsmDisks)
				}

				if len(removedAsmDisks) > 0 {
					r.Log.Info("Detected Removal of ASM Disks:", "removedAsmDisks", removedAsmDisks)
				}
				if len(addedAsmDisks) > 0 || len(removedAsmDisks) > 0 {

					isDiskChanged = true
					break // Exit loop once a difference is found
				}
			}
		}
	}

	var autoUpdate bool
	// Initialize autoUpdate based on the AutoUpdate field in the specification
	switch strings.ToLower(oracleRestart.Spec.AsmStorageDetails.AutoUpdate) {
	case "false":
		// If AutoUpdate is explicitly set to "false" by the user, set autoUpdate to false
		autoUpdate = false
		r.Log.Info("Initialized autoUpdate from provided specification", "autoUpdate", autoUpdate)
	default:
		// If AutoUpdate is not set or is set to any value other than "false", default to true
		autoUpdate = true
		r.Log.Info("Initialized autoUpdate as true (default)")
	}

	// PV Creation
	if oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" {
		if isNewSetup || isDiskChanged {
			if oracleRestart.Spec.AsmStorageDetails != nil {
				for _, diskBySize := range oracleRestart.Spec.AsmStorageDetails.DisksBySize {
					for _, diskName := range diskBySize.DiskNames {
						pvName := oraclerestartcommon.GetAsmPvName(oracleRestart.Name, diskName, oracleRestart)
						pvVolume := oraclerestartcommon.VolumePVForASM(
							oracleRestart,
							diskName,
							diskBySize.StorageSizeInGb,
							oracleRestart.Spec.AsmStorageDetails,
							pvName,
							r.Client,
						)
						// if pvVolume == nil {
						// 	r.Log.Info("VolumePVForASM returned nil for Dynamic Provisioning", "diskName", diskName, "index", index)
						// 	continue // or return error
						// }
						_, result, err = r.createOrReplaceAsmPv(ctx, oracleRestart, pvVolume)
						if err != nil {
							result = resultNq
							return result, err
						}
					}
				}
			}
		}
	}

	// PVC Creation
	if isNewSetup || isDiskChanged {
		if oracleRestart.Spec.AsmStorageDetails != nil {
			for _, diskBySize := range oracleRestart.Spec.AsmStorageDetails.DisksBySize {
				for _, diskName := range diskBySize.DiskNames {
					dgType := oraclerestartcommon.CheckDiskInAsmDeviceList(oracleRestart, diskName)
					pvcName := oraclerestartcommon.GetAsmPvcName(oracleRestart.Name, diskName, oracleRestart)
					pvcVolume := oraclerestartcommon.VolumePVCForASM(
						oracleRestart,
						diskBySize.StorageSizeInGb,
						diskName,
						diskBySize.StorageSizeInGb,
						oracleRestart.Spec.AsmStorageDetails,
						pvcName,
						dgType,
						r.Client,
					)

					_, result, err = r.createOrReplaceAsmPvC(ctx, oracleRestart, pvcVolume)
					if err != nil {
						result = resultNq
						return result, err
					}
				}
			}
		}
	}

	index := 0
	isLast := true
	oldState := oracleRestart.Status.State
	if !utils.CheckStatusFlag(oracleRestart.Spec.InstDetails.IsDelete) {
		switch {
		case isNewSetup && !isDiskChanged:
			cmName := oracleRestart.Spec.InstDetails.Name + oracleRestart.Name + "-cmap"
			cm := oraclerestartcommon.ConfigMapSpecs(oracleRestart, configMapData, cmName)
			result, configmapEnvKeyChanged, err := r.createConfigMap(ctx, *oracleRestart, cm)
			if err != nil {
				// handle error
			}
			if err != nil {
				result = resultNq
				return result, err
			}
			err = oraclerestartcommon.CreateServiceAccountIfNotExists(oracleRestart, r.Client)
			if err != nil {
				result = resultNq
				return result, err
			}

			oracleRestart.Spec.InstDetails.EnvFile = cmName
			dep := oraclerestartcommon.BuildStatefulSetForOracleRestart(oracleRestart, oracleRestart.Spec.InstDetails, r.Client)
			result, err = r.createOrReplaceSfs(ctx, req, *oracleRestart, dep, index, isLast, oldState, configmapEnvKeyChanged)
			if err != nil {
				result = resultNq
				return result, err
			}

		case isDiskChanged && !isNewSetup:
			if len(addedAsmDisks) > 0 {
				err = r.validateASMDisks(oracleRestart, ctx)
				if err != nil {
					result = resultQ
					r.Log.Info(err.Error())
					err = nilErr
					return result, err
				}
				if oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" {
					if ready, err := checkDaemonSetStatus(ctx, r, oracleRestart); err != nil || !ready {
						msg := "Any of provided ASM Disks are invalid, pls check disk-check daemon set for logs. Fix the asm disk to the valid one and redeploy."
						r.Log.Info(msg)
						err = r.cleanupDaemonSet(oracleRestart, ctx)
						if err != nil {
							result = resultQ
							r.Log.Info(err.Error())
							err = nilErr
							return result, err
						}
						addedAsmDisksMap := make(map[string]bool)
						for _, disk := range addedAsmDisks {
							addedAsmDisksMap[disk] = true
						}
						for pindex, diskBySize := range oracleRestart.Spec.AsmStorageDetails.DisksBySize {
							for cindex, diskName := range diskBySize.DiskNames {
								if _, ok := addedAsmDisksMap[diskName]; ok {
									// r.Log.Info("Found disk at index", "index", index)

									err = oraclerestartcommon.DelORestartPVC(oracleRestart, pindex, cindex, diskName, oracleRestart.Spec.AsmStorageDetails, r.Client, r.Log)
									if err != nil {
										return resultQ, err
									}

									err = oraclerestartcommon.DelORestartPv(oracleRestart, pindex, cindex, diskName, oracleRestart.Spec.AsmStorageDetails, r.Client, r.Log)
									if err != nil {
										return resultQ, err
									}
								}
							}
						}

						if err = r.SetCurrentSpec(ctx, oracleRestart, req); err != nil {
							r.Log.Error(err, "Failed to set current spec annotation")
							oracleRestart.Spec.IsFailed = true
							return resultQ, err
						}
						return result, errors.New(msg)
					} else {
						r.Log.Info("Provided ASM Disks are valid, proceeding further")
					}
				}
			}
			cmName := oracleRestart.Spec.InstDetails.Name + oracleRestart.Name + "-cmap"
			configMapDataAutoUpdate, err := r.generateConfigMapAutoUpdate(ctx, oracleRestart, cmName)
			if err != nil {
				result = resultNq
				return result, err
			}
			result, err = r.updateConfigMap(ctx, oracleRestart, configMapDataAutoUpdate, cmName)
			if err != nil {
				result = resultNq
				return result, err
			}
			r.Log.Info("Config Map updated successfully with new asm details")
			oracleRestart.Spec.InstDetails.EnvFile = cmName
			result, err = r.createOrReplaceSfsAsm(ctx, req, oracleRestart, oraclerestartcommon.BuildStatefulSetForOracleRestart(oracleRestart, oracleRestart.Spec.InstDetails, r.Client), autoUpdate, index, isLast, oldSpec)
			if err != nil {
				if autoUpdate {
					result = resultQ
				} else {
					result = resultNq
				}
				result = resultQ
				return result, err
			}
		}
	}
	if len(addedAsmDisks) > 0 {

		err = r.cleanupDaemonSet(oracleRestart, ctx)
		if err != nil {
			result = resultQ
			r.Log.Info(err.Error())
			err = nilErr
			return result, err
		}
	}

	if oracleRestart.Spec.EnableOns == "enable" || oracleRestart.Spec.EnableOns == "disable" {
		OraRestartSpex := oracleRestart.Spec.InstDetails
		orestartSfSet, err := oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
		if err != nil {
			r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.StatefulSetNotFound), r.Client, false)
			return ctrl.Result{}, err
		}

		podList, err := oraclerestartcommon.GetPodList(orestartSfSet.Name, oracleRestart, r.Client, oracleRestart.Spec.InstDetails)
		if err != nil {
			r.Log.Error(err, "Failed to list pods")
			return ctrl.Result{}, err
		}
		// default is to start
		onsOp := "start"
		if oracleRestart.Spec.EnableOns == "disable" {
			onsOp = "stop"
		}

		err = r.updateONS(ctx, podList, oracleRestart, onsOp)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.expandStorageClassSWVolume(ctx, oracleRestart, oldSpec)
	if err != nil {
		return ctrl.Result{}, err
	}

	completed = true
	// // Update the current spec after successful reconciliation
	if err = r.SetCurrentSpec(ctx, oracleRestart, req); err != nil {
		r.Log.Error(err, "Failed to set current spec annotation")
		oracleRestart.Spec.IsFailed = true
		return resultQ, err
	}
	r.Log.Info("Reconcile completed. Requeuing....")
	// uncomment this only to debugging null pointer exception
	// r.updateReconcileStatus(OracleRestart, ctx, req, &result, &err, &blocked, &completed)
	// time.Sleep(1 * time.Minute)
	return resultQ, nil
}

// Function to check the RAC topology state and return/dont proceed when matched.
func checkOracleRestartState(oracleRestart *oraclerestartdb.OracleRestart) error {
	if oracleRestart.Status.State == string(oraclerestartdb.OracleRestartProvisionState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartUpdateState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartPodAvailableState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartAddInstState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartDeletingState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartFailedState) ||
		oracleRestart.Status.State == string(oraclerestartdb.OracleRestartManualState) ||
		oracleRestart.Spec.IsFailed ||
		oracleRestart.Spec.IsManual {
		return errors.New(fmt.Sprintf("oracle restart database is in a restricted state: %s", oracleRestart.Status.State))
	}
	return nil
}

func (r *OracleRestartReconciler) generateConfigMapAutoUpdate(ctx context.Context, instance *oraclerestartdb.OracleRestart, cmName string) (map[string]string, error) {
	// Fetch the existing ConfigMap
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: instance.Namespace}, cm)
	if err != nil {
		return nil, err
	}
	r.Log.Info("Updating existing configmap")
	// Get the existing config map data
	configMapData := cm.Data
	envFileData := configMapData["envfile"]
	envVars := make(map[string]string)

	// Parse the envfile into a map
	lines := strings.Split(envFileData, "\r\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Get latest ASM devices
	asmDevices := oraclerestartcommon.GetAsmDevices(instance)

	// Update selective fields
	if instance.Spec.ConfigParams.CrsAsmDeviceList != "" {
		envVars["CRS_ASM_DEVICE_LIST"] = instance.Spec.ConfigParams.CrsAsmDeviceList
	} else {
		envVars["CRS_ASM_DEVICE_LIST"] = asmDevices
	}

	if instance.Spec.ConfigParams.RecoAsmDeviceList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = instance.Spec.ConfigParams.RecoAsmDeviceList
	} else if instance.Status.ConfigParams != nil && instance.Status.ConfigParams.RecoAsmDeviceList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = instance.Status.ConfigParams.RecoAsmDeviceList
	}

	if instance.Spec.ConfigParams.RedoAsmDeviceList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = instance.Spec.ConfigParams.RedoAsmDeviceList
	} else if instance.Status.ConfigParams != nil && instance.Status.ConfigParams.RedoAsmDeviceList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = instance.Status.ConfigParams.RedoAsmDeviceList
	}

	// Convert the envVars map back to a single string
	var updatedData []string
	for key, value := range envVars {
		updatedData = append(updatedData, fmt.Sprintf("%s=%s", key, value))
	}
	configMapData["envfile"] = strings.Join(updatedData, "\r\n")

	return configMapData, nil
}

func (r *OracleRestartReconciler) updateConfigMap(ctx context.Context, instance *oraclerestartdb.OracleRestart, configMapData map[string]string, cmName string) (ctrl.Result, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: instance.Namespace,
		},
		Data: configMapData,
	}

	err := r.Client.Update(ctx, cm)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// #############################################################################
//
//	Update each reconcile condition/status
//
// #############################################################################
func (r *OracleRestartReconciler) updateReconcileStatus(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, result *ctrl.Result, err *error, blocked *bool, completed *bool) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	// First update RAC topology
	podNames, nodeDetails, err1 := r.updateOracleRestartInstTopologyStatus(oracleRestart, ctx, req)

	// Update RAC DB topology
	if err1 == nil {
		_ = r.updateoraclerestartdbTopologyStatus(oracleRestart, ctx, req, podNames, nodeDetails)
	} else {
		r.Log.Info("Error during Oracle Restart update", "err1", err1)
	}

	var condition metav1.Condition

	if *completed {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.CrdReconcileCompeleteState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileCompleteReason),
			Message:            "reconcile completed successfully", // no error text
			Status:             metav1.ConditionTrue,
		}
	} else if *blocked {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.CrdReconcileWaitingState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileWaitingReason),
			Message:            "reconcile is waiting on dependencies", // neutral message
			Status:             metav1.ConditionTrue,
		}
	} else if result.Requeue {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.CrdReconcileQueuedState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.CrdReconcileQueuedReason),
			Message:            "reconcile has been queued", // neutral message
			Status:             metav1.ConditionTrue,
		}
	} else if err != nil && *err != nil {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.CrdReconcileErrorState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.CrdReconcileErrorReason),
			Message:            (*err).Error(), // show actual error only here
			Status:             metav1.ConditionTrue,
		}
	} else {
		return
	}

	if len(oracleRestart.Status.Conditions) > 0 {
		meta.RemoveStatusCondition(&oracleRestart.Status.Conditions, condition.Type)
	}
	meta.SetStatusCondition(&oracleRestart.Status.Conditions, condition)

	if oracleRestart.Status.State == string(oraclerestartdb.OracleRestartPodAvailableState) &&
		condition.Type == string(oraclerestartdb.CrdReconcileCompeleteState) {
		r.Log.Info("All validations and updation are completed. Changing State to AVAILABLE")
		oracleRestart.Status.State = string(oraclerestartdb.OracleRestartAvailableState)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest version of the object
		latestInstance := &oraclerestartdb.OracleRestart{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of Oracle Restart instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry fetching the latest instance
		}

		// Merge the instance fields into latestInstance
		err = mergeInstancesFromLatest(oracleRestart, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to merge instances, retrying...")
			time.Sleep(retryDelay)
			continue // Retry merging
		}

		// Update the ResourceVersion of instance from latestInstance to avoid conflict
		oracleRestart.ResourceVersion = latestInstance.ResourceVersion
		err = r.Client.Status().Patch(ctx, oracleRestart, client.MergeFrom(latestInstance))

		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected, retrying update...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				continue // Retry on conflict
			}
			r.Log.Error(err, "Failed to update the Oracle Restart DB instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry on other errors
		}

		// If update was successful, exit the loop
		r.Log.Info("Updated Oracle Restart instance status successfully", "Instance", oracleRestart.Name)
		break
	}

	r.Log.Info("Returning from updateReconcileStatus")
}

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *OracleRestartReconciler) validateSpex(oracleRestart *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec, ctx context.Context) error {
	var err error
	eventReason := "Spec Error"

	//var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	//First check image pull secrets
	if oracleRestart.Spec.ImagePullSecret != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: oracleRestart.Spec.ImagePullSecret, Namespace: oracleRestart.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(oracleRestart, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return err
			}
			r.Log.Error(err, err.Error())
			return err
		}
	}

	// ========  Config Params Checks
	// Checking Secret for ssh key
	privKeyFlag, pubKeyFlag := oraclerestartcommon.GetSSHkey(oracleRestart, oracleRestart.Spec.SshKeySecret.Name, r.Client)
	if !privKeyFlag {
		return errors.New("private key name is not set to " + oracleRestart.Spec.SshKeySecret.PrivKeySecretName + " in SshKeySecret")
	}
	if !pubKeyFlag {
		return errors.New("public key name is not set to " + oracleRestart.Spec.SshKeySecret.PubKeySecretName + " in SshKeySecret")
	}

	// Checking Gi Responsefile
	if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
		giRspFlg, _ := oraclerestartcommon.GetGiResponseFile(oracleRestart, r.Client)
		if !(giRspFlg) {
			return errors.New("GridResponseFile name must be " + oracleRestart.Spec.ConfigParams.GridResponseFile.Name)
		}
	}

	if oracleRestart.Spec.ConfigParams.DbResponseFile.ConfigMapName != "" {
		DbRspFlg, _ := oraclerestartcommon.GetDbResponseFile(oracleRestart, r.Client)
		if !(DbRspFlg) {
			return errors.New("DbResponseFile name must be " + oracleRestart.Spec.ConfigParams.DbResponseFile.Name)
		}
	}
	r.ensureAsmStorageStatus(oracleRestart)

	specDisks := flattenDisksBySize(&oracleRestart.Spec)

	// Loop through all disk groups in Status.AsmDetails
	for _, diskgroup := range oracleRestart.Status.AsmDetails.Diskgroup {
		// Compare the number of disks in each diskgroup to the number of disks in Spec
		if len(specDisks) < len(diskgroup.Disks) {
			r.Log.Info("Validating Disk to remove for Diskgroup", "DiskgroupName", diskgroup.Name)

			// Call findDisksToRemove for this diskgroup to validate disk removal
			_, err := findDisksToRemove(specDisks, diskgroup.Disks, oracleRestart)
			if err != nil {
				oracleRestart.Spec.IsFailed = true
				return errors.New("required Disk is part of the disk group " + diskgroup.Name + " and cannot be removed. Review it manually.")
			}
			// else {
			// 	r.Log.Info("Disks to be removed validated for Diskgroup", "DiskgroupName", diskgroup.Name)
			// }
		}
	}

	// Validation to check if new ASM Disk is already part of POD; return error if it is.
	// Loop through all disk groups in Status.AsmDetails
	for _, diskgroup := range oracleRestart.Status.AsmDetails.Diskgroup {
		// Compare the number of disks in each diskgroup to the number of disks in Spec
		for _, diskgroupDisks := range diskgroup.Disks {
			disks := strings.Split(diskgroupDisks, ",")
			if len(specDisks) > len(disks) {
				// r.Log.Info("Validating newly added Disk for Diskgroup", "DiskgroupName", diskgroup.Name)

				// Call findDisksToAdd to validate the newly added disks
				_, err := findDisksToAdd(specDisks, diskgroup.Disks, oracleRestart, oldSpec)
				if err != nil {
					return err
				}
				// else {
				// 	r.Log.Info("Disk to be added validated for Diskgroup", "DiskgroupName", diskgroup.Name, "Disk", fmt.Sprintf("%v", disk))
				// }
			}
		}
	}

	// Checking the network cards in response files

	if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
		_, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "networkInterfaceList", oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName, oracleRestart.Spec.ConfigParams.GridResponseFile.Name)
		if err != nil {
			oracleRestart.Spec.IsFailed = true
			return err
		}

		// Check if IsDelete is defined
		switch isDeleteStr := oracleRestart.Spec.InstDetails.IsDelete; isDeleteStr {
		case "true":

			r.Log.Info("Performing operation for IsDelete true")

		default:
			// Validate network cards for both "false" and when IsDelete is not defined
			if isDeleteStr != "" {
				r.Log.Info("Unexpected value for IsDelete: " + isDeleteStr)
			}

		}

	}

	r.Log.Info("Completed reconcile validation")

	return nil

}

// Helper function to flatten DisksBySize into a single slice of disk names
func flattenDisksBySize(oraclerestartdbSpec *oraclerestartdb.OracleRestartSpec) []string {
	disksBySize := oraclerestartdbSpec.AsmStorageDetails.DisksBySize
	var allDisks []string
	for _, diskBySize := range disksBySize {
		allDisks = append(allDisks, diskBySize.DiskNames...)
	}
	return allDisks
}

// #############################################################################
//
//	Validate the CRD specs
//
// #############################################################################
func (r *OracleRestartReconciler) validateASMDisks(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context) error {
	//var eventMsgs []string

	r.Log.Info("Validate New ASM Disks")
	desiredDaemonSet := oraclerestartcommon.BuildDiskCheckDaemonSet(oracleRestart)

	// Try to get the existing DaemonSet
	existingDaemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desiredDaemonSet.Name,
		Namespace: desiredDaemonSet.Namespace,
	}, existingDaemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// DaemonSet does not exist, so create it
			r.Log.Info("Creating DaemonSet:", "desiredDaemonSet.Name", desiredDaemonSet.Name)
			if err := r.Client.Create(ctx, desiredDaemonSet); err != nil {
				oracleRestart.Spec.IsFailed = true
				return err
			}
		} else {
			// Some other error occurred in fetching the DaemonSet
			oracleRestart.Spec.IsFailed = true
			return err
		}
	} else {
		// DaemonSet exists, so check if an update is necessary
		if !reflect.DeepEqual(existingDaemonSet.Spec.Template.Spec.Volumes, desiredDaemonSet.Spec.Template.Spec.Volumes) {
			// Update the existing DaemonSet with the desired state
			r.Log.Info("Updating DaemonSet:", "desiredDaemonSet.Name", desiredDaemonSet.Name)
			existingDaemonSet.Spec = desiredDaemonSet.Spec
			if err := r.Client.Update(ctx, existingDaemonSet); err != nil {
				return err
			}
			r.Log.Info("Updating Daemon set, takes upto 1 minute")
			time.Sleep(1 * time.Second * 60)
			//update takes times to terminate and recreate
		}
	}

	// r.Log.Info("Checking ASM DaemonSet Pod Status")

	return nil

}

func (r *OracleRestartReconciler) cleanupDaemonSet(OracleRestart *oraclerestartdb.OracleRestart, ctx context.Context) error {
	// r.Log.Info("CleanupDaemonSet")
	desiredDaemonSet := oraclerestartcommon.BuildDiskCheckDaemonSet(OracleRestart)

	// Try to get the existing DaemonSet
	existingDaemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desiredDaemonSet.Name,
		Namespace: desiredDaemonSet.Namespace,
	}, existingDaemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// DaemonSet does not exist, so nothing to delete
			// r.Log.Info("No DaemonSet Found To Delete")
			return nil
		}
		// Some other error occurred in fetching the DaemonSet
		// r.Log.Error(err, "Some other error occurred in fetching the DaemonSet")
		return err
	}

	// DaemonSet exists, attempt to delete it
	// r.Log.Info("Deleting DaemonSet", "DaemonSet.Name", existingDaemonSet.Name)
	if err := r.Client.Delete(ctx, existingDaemonSet); err != nil {
		r.Log.Error(err, "Failed to delete DaemonSet", "DaemonSet.Name", existingDaemonSet.Name)
		return err
	}

	// Poll for the DaemonSet to be deleted
	timeout := 30 * time.Second
	pollInterval := 5 * time.Second
	startTime := time.Now()

	for {
		// Check if we have exceeded the timeout
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for DaemonSet %s to be deleted", existingDaemonSet.Name)
		}

		// Check if the DaemonSet still exists
		err = r.Client.Get(ctx, types.NamespacedName{
			Name:      existingDaemonSet.Name,
			Namespace: existingDaemonSet.Namespace,
		}, existingDaemonSet)

		if err != nil {
			if apierrors.IsNotFound(err) {
				// DaemonSet no longer exists
				r.Log.Info("DaemonSet deleted successfully", "DaemonSet.Name", existingDaemonSet.Name)
				return nil
			}
			// Some other error occurred in fetching the DaemonSet
			r.Log.Error(err, "Error checking for DaemonSet deletion", "DaemonSet.Name", existingDaemonSet.Name)
			return err
		}

		// Wait before checking again
		time.Sleep(pollInterval)
	}
}

func findDisksToRemove(specDisks, statusDisks []string, instance *oraclerestartdb.OracleRestart) ([]string, error) {
	// Convert specDisks to a set for fast lookups
	specDiskSet := make(map[string]struct{})
	for _, disk := range specDisks {
		specDiskSet[disk] = struct{}{}
	}

	// Find disks in statusDisks that are not in specDiskSet
	var disksToRemove []string
	for _, disk := range statusDisks {
		if _, found := specDiskSet[disk]; !found {
			disksToRemove = append(disksToRemove, disk)
		}
	}

	// Validate that disks to be removed are not part of any other ASM device list
	combinedList := strings.Join([]string{
		instance.Spec.ConfigParams.CrsAsmDeviceList,
		instance.Spec.ConfigParams.RecoAsmDeviceList,
		instance.Spec.ConfigParams.RedoAsmDeviceList,
		instance.Spec.ConfigParams.DbAsmDeviceList,
	}, ",")
	combinedSet := make(map[string]struct{})
	for _, disk := range strings.Split(combinedList, ",") {
		combinedSet[disk] = struct{}{}
	}

	// Check for any disks to remove that are part of the combined ASM device list
	var validatedDisks []string
	for _, disk := range disksToRemove {
		if _, found := combinedSet[disk]; found {
			return nil, fmt.Errorf("disk %s to be removed is part of a disk group, hence cannot be removed", disk)
		}
		validatedDisks = append(validatedDisks, disk)
	}

	return validatedDisks, nil
}

func findDisksToAdd(newSpecDisks, statusDisks []string, instance *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec) ([]string, error) {
	// Create a set for statusDisks to allow valid reuse of existing disks
	// Step 1: Check for duplicates within newSpecDisks itself
	oldAsmDisks := flattenDisksBySize(oldSpec)

	if len(oldAsmDisks) == len(newSpecDisks) {
		oldDiskSet := make(map[string]struct{})
		for _, disk := range oldAsmDisks {
			oldDiskSet[strings.TrimSpace(disk)] = struct{}{}
		}

		allDisksMatch := true
		for _, newDisk := range newSpecDisks {
			if _, found := oldDiskSet[strings.TrimSpace(newDisk)]; !found {
				allDisksMatch = false
				break
			}
		}

		if allDisksMatch {
			return nil, nil // No new disks to add
		}
	}

	seenDisks := make(map[string]struct{})
	for _, newDisk := range newSpecDisks {
		trimmedDisk := strings.TrimSpace(newDisk)

		// Check if the disk is already in the seenDisks set, indicating a duplicate within newSpecDisks
		if _, found := seenDisks[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is defined more than once in the new spec and cannot be added multiple times", trimmedDisk)
		}
		seenDisks[trimmedDisk] = struct{}{}
	}

	// Step 2: Create a set for the actual statusDisks by splitting each entry
	statusDiskSet := make(map[string]struct{})
	for _, diskEntry := range statusDisks {
		// Split the disk entry by commas to handle multiple disks in a single string
		for _, disk := range strings.Split(diskEntry, ",") {
			statusDiskSet[strings.TrimSpace(disk)] = struct{}{}
		}
	}

	// Create sets for each of the individual ASM device lists
	crsAsmDeviceSet := make(map[string]struct{})
	recoAsmDeviceSet := make(map[string]struct{})
	redoAsmDeviceSet := make(map[string]struct{})
	dbAsmDeviceSet := make(map[string]struct{})

	// Step 4: Create a set to track newly added disks that are valid for addition
	var validDisksToAdd []string
	newDiskSet := make(map[string]struct{})

	for _, newDisk := range newSpecDisks {
		trimmedDisk := strings.TrimSpace(newDisk)

		// If the disk is already part of the statusDisks (existing disks), allow it to stay
		if _, found := statusDiskSet[trimmedDisk]; found {
			continue
		}

		// Check if the disk is already part of any individual ASM device list
		if _, found := crsAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of CRS ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := recoAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of RECO ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := redoAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of REDO ASM device list and cannot be added again", trimmedDisk)
		}
		if _, found := dbAsmDeviceSet[trimmedDisk]; found {
			return nil, fmt.Errorf("disk '%s' is already part of DB ASM device list and cannot be added again", trimmedDisk)
		}

		// Add the disk to newDiskSet and consider it valid for addition
		newDiskSet[trimmedDisk] = struct{}{}
		validDisksToAdd = append(validDisksToAdd, trimmedDisk)
	}

	return validDisksToAdd, nil
}

func (r *OracleRestartReconciler) setDefaults(oracleRestart *oraclerestartdb.OracleRestart) error {

	if oracleRestart.Spec.ImagePullPolicy == nil {
		*oracleRestart.Spec.ImagePullPolicy = "Always"
	}

	if oracleRestart.Spec.SshKeySecret != nil {
		if oracleRestart.Spec.SshKeySecret.KeyMountLocation == "" {
			oracleRestart.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSshSecretMount
		}
	}

	if oracleRestart.Spec.DbSecret != nil {
		if oracleRestart.Spec.DbSecret.Name != "" {
			if oracleRestart.Spec.DbSecret.PwdFileMountLocation == "" {
				oracleRestart.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
			}
			if oracleRestart.Spec.DbSecret.KeyFileMountLocation == "" {
				oracleRestart.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
			}
		}
	}

	if oracleRestart.Spec.TdeWalletSecret != nil {
		if oracleRestart.Spec.TdeWalletSecret.Name != "" {
			if oracleRestart.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
				oracleRestart.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
			}
			if oracleRestart.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
				oracleRestart.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
			}
		}
	}

	if oracleRestart.Spec.ConfigParams != nil {
		if oracleRestart.Spec.ConfigParams.SwMountLocation == "" {
			oracleRestart.Spec.ConfigParams.SwMountLocation = utils.OraSwLocation
		}

		if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName == "" {
			if oracleRestart.Spec.ConfigParams.CrsAsmDiskDg == "" {
				oracleRestart.Spec.ConfigParams.CrsAsmDiskDg = "+DATA"
			}

			if oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy == "" {
				oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy = "external"
			}
		}

		if oracleRestart.Spec.ConfigParams.DbResponseFile.ConfigMapName == "" {
			if oracleRestart.Spec.ConfigParams.DbDataFileDestDg == "" {
				oracleRestart.Spec.ConfigParams.DbDataFileDestDg = oracleRestart.Spec.ConfigParams.CrsAsmDiskDg
			}

			if oracleRestart.Spec.ConfigParams.DbRecoveryFileDest == "" {
				oracleRestart.Spec.ConfigParams.DbRecoveryFileDest = oracleRestart.Spec.ConfigParams.DbDataFileDestDg
			}

			if oracleRestart.Spec.ConfigParams.DbCharSet == "" {
				oracleRestart.Spec.ConfigParams.DbCharSet = "AL32UTF8"
			}
		}

	}
	return nil

}

func (r *OracleRestartReconciler) updateGiConfigParamStatus(oracleRestart *oraclerestartdb.OracleRestart) error {

	//orestartPod := &corev1.Pod{}

	var cName, fName string

	if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
		cName = oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName
	}
	if oracleRestart.Spec.ConfigParams.GridResponseFile.Name != "" {
		fName = oracleRestart.Spec.ConfigParams.GridResponseFile.Name
	}

	if oracleRestart.Status.ConfigParams == nil {
		oracleRestart.Status.ConfigParams = new(oraclerestartdb.InitParams)
	}

	if oracleRestart.Spec.ConfigParams != nil {
		if oracleRestart.Status.ConfigParams.CrsAsmDeviceList == "" {
			if oracleRestart.Spec.ConfigParams.CrsAsmDeviceList != "" {
				oracleRestart.Status.ConfigParams.CrsAsmDeviceList = oracleRestart.Spec.ConfigParams.CrsAsmDeviceList
			} else {
				diskList, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "diskList", cName, fName)
				if err != nil {
					return errors.New(("error in responsefile, unable to read diskList"))
				}
				oracleRestart.Status.ConfigParams.CrsAsmDeviceList = diskList
			}
		}
		if oracleRestart.Status.ConfigParams.Inventory == "" {
			if oracleRestart.Spec.ConfigParams.Inventory != "" {
				oracleRestart.Status.ConfigParams.Inventory = oracleRestart.Spec.ConfigParams.Inventory
			} else {
				invlocation, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "INVENTORY_LOCATION", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read inventory_location"))
				} else {
					oracleRestart.Status.ConfigParams.Inventory = invlocation
				}
			}
		}

		if oracleRestart.Status.ConfigParams.GridBase == "" {
			if oracleRestart.Spec.ConfigParams.GridBase != "" {
				oracleRestart.Status.ConfigParams.GridBase = oracleRestart.Spec.ConfigParams.GridBase
			} else {
				gibase, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "ORACLE_BASE", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read oracle_base"))
				} else {
					oracleRestart.Status.ConfigParams.GridBase = gibase
				}
			}
		}
		if oracleRestart.Status.ConfigParams.GridHome == "NOT_DEFINED" {
			if oracleRestart.Spec.ConfigParams.GridHome != "" {
				oracleRestart.Status.ConfigParams.GridHome = oracleRestart.Spec.ConfigParams.GridHome
			} else {
				gihome, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "GRID_HOME", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read oracle_base"))
				} else {
					oracleRestart.Status.ConfigParams.GridHome = gihome
				}
			}
		}

		// if oracleRestart.Status.ScanSvcName == "" {
		// 	if oracleRestart.Spec.ScanSvcName != "" {
		// 		oracleRestart.Status.ScanSvcName = oracleRestart.Spec.ScanSvcName
		// 	} else {
		// 		scanname, err := oraclerestartcommon.CheckRspData(OracleRestart, r.Client, "scanName", cName, fName)
		// 		if err != nil {
		// 			oracleRestart.Spec.IsFailed = true
		// 			return errors.New(("error in responsefile, unable to read scanName"))
		// 		} else {
		// 			oracleRestart.Status.ScanSvcName = scanname
		// 		}
		// 	}
		// }

		if oracleRestart.Status.ConfigParams.CrsAsmDiskDg == "" {
			if oracleRestart.Spec.ConfigParams.CrsAsmDiskDg != "" {
				oracleRestart.Status.ConfigParams.CrsAsmDiskDg = oracleRestart.Spec.ConfigParams.CrsAsmDiskDg
			} else {
				diskGroupName, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "diskGroupName", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read diskGroupName"))
				} else {
					oracleRestart.Status.ConfigParams.CrsAsmDiskDg = diskGroupName
				}
			}
		}

		if oracleRestart.Status.ConfigParams.CrsAsmDiskDgRedundancy == "" {
			if oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy != "" {
				oracleRestart.Status.ConfigParams.CrsAsmDiskDgRedundancy = oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy
			} else {
				redundancy, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "redundancy", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read redundancy"))
				} else {
					oracleRestart.Status.ConfigParams.CrsAsmDiskDgRedundancy = redundancy
				}
			}
		}
	}

	return nil

}

func (r *OracleRestartReconciler) updateDbConfigParamStatus(oracleRestart *oraclerestartdb.OracleRestart) error {

	//orestartPod := &corev1.Pod{}

	var cName, fName string

	if oracleRestart.Spec.ConfigParams.DbResponseFile.ConfigMapName != "" {
		cName = oracleRestart.Spec.ConfigParams.DbResponseFile.ConfigMapName
	}
	if oracleRestart.Spec.ConfigParams.DbResponseFile.Name != "" {
		fName = oracleRestart.Spec.ConfigParams.DbResponseFile.Name
	}

	if oracleRestart.Status.ConfigParams == nil {
		oracleRestart.Status.ConfigParams = new(oraclerestartdb.InitParams)
	}

	if oracleRestart.Spec.ConfigParams != nil {
		if oracleRestart.Spec.ConfigParams.DbAsmDeviceList != "" {
			oracleRestart.Status.ConfigParams.DbAsmDeviceList = oracleRestart.Spec.ConfigParams.DbAsmDeviceList
		}

		if oracleRestart.Spec.ConfigParams.RecoAsmDeviceList != "" {
			oracleRestart.Status.ConfigParams.RecoAsmDeviceList = oracleRestart.Spec.ConfigParams.RecoAsmDeviceList
		}
		if oracleRestart.Spec.ConfigParams.DBAsmDiskDgRedundancy != "" {
			oracleRestart.Status.ConfigParams.DBAsmDiskDgRedundancy = oracleRestart.Spec.ConfigParams.DBAsmDiskDgRedundancy
		}

		if oracleRestart.Status.ConfigParams.DbName == "" {
			if oracleRestart.Spec.ConfigParams.DbName != "" {
				oracleRestart.Status.ConfigParams.DbName = oracleRestart.Spec.ConfigParams.DbName
			} else {
				variable, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "variables=", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read variable"))
				}
				dbName := utils.GetValue(variable, "DB_NAME")
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read DB_NAME"))
				}
				oracleRestart.Status.ConfigParams.DbName = dbName
			}
		}

		if oracleRestart.Status.ConfigParams.DbDataFileDestDg == "" {
			if oracleRestart.Spec.ConfigParams.DbDataFileDestDg != "" {
				oracleRestart.Status.ConfigParams.DbDataFileDestDg = oracleRestart.Spec.ConfigParams.DbDataFileDestDg
				if oracleRestart.Spec.ConfigParams.DbAsmDeviceList != "" {
					oracleRestart.Status.ConfigParams.DbAsmDeviceList = oracleRestart.Spec.ConfigParams.DbAsmDeviceList
					// Logic to validate and set Disk group using grid response file and dbca response file
					var gcName, gfName string

					if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
						gcName = oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName
					}
					if oracleRestart.Spec.ConfigParams.GridResponseFile.Name != "" {
						gfName = oracleRestart.Spec.ConfigParams.GridResponseFile.Name
					}
					if gcName != "" && gfName != "" {
						diskGroupName, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "diskGroupName", gcName, gfName)
						if err != nil {
							oracleRestart.Spec.IsFailed = true
							return errors.New(("error in responsefile, unable to read diskGroupName"))
						}

						if oracleRestart.Status.ConfigParams.DbDataFileDestDg != diskGroupName {
							return nil
						} else {
							oracleRestart.Status.ConfigParams.DbDataFileDestDg = diskGroupName
						}
					}
				}
			} else {
				// Logic to validate and set Disk group using grid response file and dbca response file
				var gcName, gfName string

				if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
					gcName = oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName
				}
				if oracleRestart.Spec.ConfigParams.GridResponseFile.Name != "" {
					gfName = oracleRestart.Spec.ConfigParams.GridResponseFile.Name
				}
				if gcName != "" && gfName != "" {
					diskGroupName, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "diskGroupName", gcName, gfName)
					if err != nil {
						oracleRestart.Spec.IsFailed = true
						return errors.New(("error in grid responsefile, unable to read diskGroupName to set DbDataFileDestDg"))
					}
					oracleRestart.Status.ConfigParams.DbDataFileDestDg = diskGroupName
					dbdgloc, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "datafileDestination", cName, fName)
					if err != nil {
						oracleRestart.Spec.IsFailed = true
						oracleRestart.Status.ConfigParams.DbDataFileDestDg = diskGroupName
					} else {
						dbdg := strings.Split(dbdgloc, "/")
						if len(dbdg) == 0 {
							return errors.New("error in responsefile, unable to read datafileDestination diskgroup")
						}
						oracleRestart.Status.ConfigParams.DbDataFileDestDg = dbdg[0]
					}
				} else {
					return errors.New("neither DbDataFileDestDg is set , nor grid response file is set. One of them is required")
				}
			}
		}

		if oracleRestart.Status.ConfigParams.DbRecoveryFileDest == "" {
			if oracleRestart.Spec.ConfigParams.DbRecoveryFileDest != "" {
				oracleRestart.Status.ConfigParams.DbRecoveryFileDest = oracleRestart.Spec.ConfigParams.DbRecoveryFileDest
			} else {
				if cName != "" && fName != "" {
					recodgloc, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "recoveryAreaDestination", cName, fName)
					if err != nil {
						oracleRestart.Spec.IsFailed = true
						return errors.New(("error in responsefile, unable to read recoveryAreaDestination"))
					}
					recodg := strings.Split(recodgloc, "/")
					if len(recodg) == 0 {
						return errors.New("error in responsefile, unable to read recoveryAreaDestination diskgroup")
					}
					oracleRestart.Status.ConfigParams.DbDataFileDestDg = recodg[0]
				}
			}
		}

		if oracleRestart.Status.ConfigParams.DbBase == "" {
			if oracleRestart.Spec.ConfigParams.DbBase != "" {
				oracleRestart.Status.ConfigParams.DbBase = oracleRestart.Spec.ConfigParams.DbBase
			} else {
				variable, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "variables=", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read variable"))
				}
				obase := utils.GetValue(variable, "ORACLE_BASE")
				if len(obase) == 0 {
					return errors.New(("error in responsefile, unable to read ORACLE_BASE"))
				}
				oracleRestart.Status.ConfigParams.DbBase = obase
			}
		}
		if oracleRestart.Status.ConfigParams.DbHome == "NOT_DEFINED" {
			if oracleRestart.Spec.ConfigParams.DbHome != "" {
				oracleRestart.Status.ConfigParams.DbHome = oracleRestart.Spec.ConfigParams.DbHome
			} else {
				variable, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "variables=", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read variable"))
				}
				ohome := utils.GetValue(variable, "ORACLE_HOME")
				if len(ohome) == 0 {
					return errors.New(("error in responsefile, unable to read ORACLE_BASE"))
				}
				oracleRestart.Status.ConfigParams.DbHome = ohome
			}
		}

		if oracleRestart.Status.ConfigParams.DbHome == "" {
			if oracleRestart.Spec.ConfigParams.DbHome != "" {
				oracleRestart.Status.ConfigParams.DbHome = oracleRestart.Spec.ConfigParams.DbHome
			} else {
				variable, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "variables=", cName, fName)
				if err != nil {
					oracleRestart.Spec.IsFailed = true
					return errors.New(("error in responsefile, unable to read variable"))
				}
				ohome := utils.GetValue(variable, "ORACLE_HOME")
				if len(ohome) == 0 {
					return errors.New(("error in responsefile, unable to read ORACLE_HOME"))
				}
				oracleRestart.Status.ConfigParams.DbHome = ohome
			}
		}
	}

	if oracleRestart.Status.ConfigParams.GridHome == "" {
		if oracleRestart.Spec.ConfigParams.GridHome != "" {
			oracleRestart.Status.ConfigParams.GridHome = oracleRestart.Spec.ConfigParams.GridHome
		} else {
			variable, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "variables=", cName, fName)
			if err != nil {
				oracleRestart.Spec.IsFailed = true
				return errors.New(("error in responsefile, unable to read variable"))
			}
			ghome := utils.GetValue(variable, "ORACLE_HOME")
			if len(ghome) == 0 {
				return errors.New(("error in responsefile, unable to read ORACLE_HOME"))
			}
			oracleRestart.Status.ConfigParams.GridHome = ghome
		}
	}

	return nil

}

func (r *OracleRestartReconciler) updateOracleRestartInstTopologyStatus(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request) ([]string, map[string]*corev1.Node, error) {

	//orestartPod := &corev1.Pod{}
	var podNames []string
	nodeDetails := make(map[string]*corev1.Node)

	if strings.ToLower(oracleRestart.Spec.InstDetails.IsDelete) != "true" {
		_, pod, err := r.validateOracleRestartInst(oracleRestart, ctx, req, oracleRestart.Spec.InstDetails, 0)
		if err != nil {
			return podNames, nodeDetails, err
		}
		podNames = append(podNames, pod.Name)

		// Get node details for the node where the pod is running
		node, err := r.getNodeDetails(pod.Spec.NodeName)
		if err != nil {
			return podNames, nodeDetails, fmt.Errorf("failed to get node details for pod %s: %v", pod.Name, err)
		}
		nodeDetails[pod.Name] = node
	}

	if len(podNames) == 0 || len(nodeDetails) == 0 {
		oracleRestart.Spec.IsFailed = true
		return podNames, nodeDetails, errors.New("error occurred while collecting Oracle Restart pod or node details")
	} else {
		oracleRestart.Spec.IsFailed = false
	}

	return podNames, nodeDetails, nil
}

func (r *OracleRestartReconciler) getNodeDetails(nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	err := r.Client.Get(context.TODO(), client.ObjectKey{
		Namespace: "",
		Name:      nodeName,
	}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (r *OracleRestartReconciler) updateoraclerestartdbTopologyStatus(OracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node) error {

	//orestartPod := &corev1.Pod{}
	var err error
	_, _, err = r.validateoraclerestartdb(OracleRestart, ctx, req, podNames, nodeDetails)
	if err != nil {
		return err
	}
	return nil
}

func (r *OracleRestartReconciler) validateoraclerestartdb(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	orestartSfSet := &appsv1.StatefulSet{}
	orestartPod := &corev1.Pod{}
	const maxRetries = 5
	const retryDelay = 2 * time.Second
	oraclerestartcommon.UpdateoraclerestartdbStatusData(oracleRestart, ctx, req, podNames, r.kubeClient, r.kubeConfig, r.Log, nodeDetails)
	// Log the start of the status update process
	r.Log.Info("Updating Oracle Restart instance status with validateoraclerestartdb", "Instance", oracleRestart.Name)

	for attempt := 0; attempt < maxRetries; attempt++ {
		// // Fetch the latest version of the object
		latestInstance := &oraclerestartdb.OracleRestart{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of Oracle Restart instance")
			return orestartSfSet, orestartPod, err // Return the error if fetching the latest version fails
		}

		// Merge the instance fields into latestInstance
		err = mergeInstancesFromLatest(oracleRestart, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to merge instances")
			return orestartSfSet, orestartPod, err
		}

		// Attempt to update the status of the instance directly
		// err = r.Status().Update(ctx, instance)

		// Update the ResourceVersion of instance from latestInstance to avoid conflict
		oracleRestart.ResourceVersion = latestInstance.ResourceVersion
		err = r.Client.Status().Patch(ctx, oracleRestart, client.MergeFrom(latestInstance))

		if err != nil {
			if apierrors.IsConflict(err) {
				// Handle the conflict and retry
				r.Log.Info("Conflict detected in validateoraclerestartdb, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				continue
				// Retry
			}
			// For other errors, log and continue the retry loop
			r.Log.Error(err, "Failed to update the Oracle Restart DB instance, retrying")
			continue
		}

		// If update was successful, exit the loop
		r.Log.Info("Updated Oracle Restart instance status with validateoraclerestartdb", "Instance", oracleRestart.Name)

		return orestartSfSet, orestartPod, nil
	}

	// If all retries fail, return an error
	return orestartSfSet, orestartPod, fmt.Errorf("failed to update Oracle Restart DB Status after %d attempts", maxRetries)
}

// ======= Function to validate Shard
func (r *OracleRestartReconciler) validateOracleRestartInst(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, OraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, specId int) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	orestartSfSet := &appsv1.StatefulSet{}
	orestartPod := &corev1.Pod{}

	orestartSfSet, err = oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
	if err != nil {
		//msg := "Unable to find Oracle Restart statefulset " + oraclerestartcommon.GetFmtStr(OraRestartSpex.Name) + "."
		//oraclerestartcommon.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.StatefulSetNotFound), r.Client, false)
		return orestartSfSet, orestartPod, err
	}

	podList, err := oraclerestartcommon.GetPodList(orestartSfSet.Name, oracleRestart, r.Client, OraRestartSpex)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + oraclerestartcommon.GetFmtStr(orestartSfSet.Name) + "."
		oraclerestartcommon.LogMessages("INFO", msg, nil, oracleRestart, r.Log)
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.PodNotFound), r.Client, false)
		return orestartSfSet, orestartPod, err
	}
	// Validate the pod list and get the list of not ready pods
	isPodExist, orestartPod, notReadyPod := oraclerestartcommon.PodListValidation(podList, orestartSfSet.Name, oracleRestart, r.Client)
	// Check if the pod is ready
	if !isPodExist {
		msg := ""
		if notReadyPod != nil {
			// Log the name of the first not ready pod
			msg = "unable to validate Oracle Restart pod. The  pod not ready  is: " + notReadyPod.Name
			oraclerestartcommon.LogMessages("INFO", msg, nil, oracleRestart, r.Log)
			return orestartSfSet, orestartPod, fmt.Errorf(msg)
		} else {
			// Handle the case where no pods were found at all
			msg = "unable to validate Oracle Restart pod. No pods matching the criteria were found"
			oraclerestartcommon.LogMessages("INFO", msg, nil, oracleRestart, r.Log)
			return orestartSfSet, orestartPod, fmt.Errorf(msg)
		}

	}
	// Update status when PODs are ready
	state := oracleRestart.Status.State
	if oracleRestart.Spec.IsManual { // if user changes spec  to manual mode, lets change status column same as well
		state = string(oraclerestartdb.OracleRestartManualState)
	}
	if oracleRestart.Spec.IsFailed { // if controller changes spec  to failed mode, lets change status column same as well
		state = string(oraclerestartdb.OracleRestartFailedState)
	}

	switch {
	case isPodExist && (state == string(oraclerestartdb.OracleRestartProvisionState) ||
		state == string(oraclerestartdb.OracleRestartUpdateState) ||
		state == string(oraclerestartdb.OracleRestartPendingState)):
		// When previous update or provision is there, change to POD available intermittent state
		state = string(oraclerestartdb.OracleRestartPodAvailableState)
	case state == string(oraclerestartdb.OracleRestartFailedState):
		// Failed state handling, remain in failed state or take specific action
		state = string(oraclerestartdb.OracleRestartFailedState)
	case state == string(oraclerestartdb.OracleRestartManualState):
		// Manual state handling, e.g., do not modify state automatically
		state = string(oraclerestartdb.OracleRestartManualState)
	default:
		// Continue with the current state for others, if no conditions are met
		state = oracleRestart.Status.State
	}

	r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, state, r.Client, true)
	r.Log.Info("Completed Update of Oracle Restart instance status")
	return orestartSfSet, orestartPod, nil
}

func (r *OracleRestartReconciler) updateOracleRestartInstStatus(
	oracleRestart *oraclerestartdb.OracleRestart,
	ctx context.Context,
	req ctrl.Request,
	OraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec,
	state string,
	kClient client.Client,
	mergingRequired bool,
) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastErr error
	var failedUpdate bool
	// Get/Update RAC instance status data
	oraclerestartcommon.UpdateOracleRestartInstStatusData(oracleRestart, ctx, req, OraRestartSpex, state, r.kubeClient, r.kubeConfig, r.Log, r.Client)

	for attempt := 0; attempt < maxRetries; attempt++ {

		// Fetch the latest version of the object
		latestInstance := &oraclerestartdb.OracleRestart{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of Oracle Restart instance")
			lastErr = err
			continue // Continue to retry
		}
		latestInstance.Status.OracleRestartNodes = oracleRestart.Status.OracleRestartNodes
		if mergingRequired {

			// Ensure latestInstance has the most recent version
			r.ensureAsmStorageStatus(latestInstance)

			// Merge the instance fields into latestInstance
			err = mergeInstancesFromLatest(oracleRestart, latestInstance)
			if err != nil {
				r.Log.Error(err, "Failed to merge instances")
			}
		}

		// Attempt to update the combined instance back to the Kubernetes API
		// err = r.Status().Update(ctx, instance)
		oracleRestart.ResourceVersion = latestInstance.ResourceVersion

		err = r.Status().Update(ctx, oracleRestart)
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected in updateOracleRestartInstStatus, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				failedUpdate = true
				continue // Retry
			}
			// For other errors, log and return
			r.Log.Error(err, "Failed to update the Oracle Restart instance")
			lastErr = err
			failedUpdate = true
			continue // Continue to retry
		}
		r.Log.Info("Oracle Restart Object updated with updateOracleRestartInstStatus")
		failedUpdate = false
		break //break if its updated successfully
	}

	// If we exhaust all retries, print the last error encountered
	if failedUpdate {
		r.Log.Info("failed to update Oracle Restart instance after 5 attempts", "lastErr", lastErr)
	}
}

// GetRestrictedFields returns a set of field names that are restricted from being updated.
func GetRestrictedFields() map[string]struct{} {
	return map[string]struct{}{
		"ConfigParams.DbName":                         {},
		"ConfigParams.GridBase":                       {},
		"ConfigParams.GridHome":                       {},
		"ConfigParams.DbBase":                         {},
		"ConfigParams.DbHome":                         {},
		"ConfigParams.CrsAsmDiskDg":                   {},
		"ConfigParams.CrsAsmDiskDgRedundancy":         {},
		"ConfigParams.DBAsmDiskDgRedundancy":          {},
		"ConfigParams.DbCharSet":                      {},
		"ConfigParams.DbConfigType":                   {},
		"ConfigParams.DbDataFileDestDg":               {},
		"ConfigParams.DbUniqueName":                   {},
		"ConfigParams.DbRecoveryFileDest":             {},
		"ConfigParams.DbRedoFileSize":                 {},
		"ConfigParams.DbStorageType":                  {},
		"ConfigParams.DbSwZipFile":                    {},
		"ConfigParams.GridSwZipFile":                  {},
		"ConfigParams.GridResponseFile.ConfigMapName": {},
		"ConfigParams.GridResponseFile.Name":          {},
		"ConfigParams.DbResponseFile.ConfigMapName":   {},
		"ConfigParams.DbResponseFile.Name":            {},
	}
}

func mergeInstancesFromLatest(instance, latestInstance *oraclerestartdb.OracleRestart) error {
	instanceVal := reflect.ValueOf(instance).Elem()
	latestVal := reflect.ValueOf(latestInstance).Elem()

	// Assuming `Status` is a field in `OracleRestart`
	instanceStatus := instanceVal.FieldByName("Status")
	latestStatus := latestVal.FieldByName("Status")

	if !instanceStatus.IsValid() || !latestStatus.IsValid() {
		return fmt.Errorf("status field is not valid in one of the instances")
	}

	// Merge the Status field
	return mergeStructFields(instanceStatus, latestStatus)
}

func mergeStructFields(instanceField, latestField reflect.Value) error {
	if instanceField.Kind() != reflect.Struct || latestField.Kind() != reflect.Struct {
		return fmt.Errorf("fields to be merged must be of struct type")
	}

	for i := 0; i < instanceField.NumField(); i++ {
		subField := instanceField.Type().Field(i)
		instanceSubField := instanceField.Field(i)
		latestSubField := latestField.Field(i)

		if !isExported(subField) || !instanceSubField.CanSet() {
			continue
		}

		switch latestSubField.Kind() {
		case reflect.Ptr:
			if !latestSubField.IsNil() && instanceSubField.IsNil() {
				instanceSubField.Set(latestSubField)
			}
		case reflect.String:
			if latestSubField.String() != "" && latestSubField.String() != "NOT_DEFINED" && instanceSubField.String() == "" {
				instanceSubField.Set(latestSubField)
			}
		case reflect.Struct:
			if err := mergeStructFields(instanceSubField, latestSubField); err != nil {
				return err
			}
		default:
			if reflect.DeepEqual(instanceSubField.Interface(), reflect.Zero(instanceSubField.Type()).Interface()) {
				instanceSubField.Set(latestSubField)
			}
		}
	}
	return nil
}

func isExported(field reflect.StructField) bool {
	return field.PkgPath == ""
}

// Create Configmap
func (r *OracleRestartReconciler) generateConfigMap(instance *oraclerestartdb.OracleRestart) (map[string]string, error) {
	configMapData := make(map[string]string, 0)
	// new_crs_nodes, existing_crs_nodes_healthy, existing_crs_nodes_not_healthy, install_node, new_crs_nodes_list := oraclerestartcommon.GetCrsNodes(instance, r.kubeClient, r.kubeConfig, r.Log, r.Client)
	install_node := instance.Spec.InstDetails.Name + "-0"
	asm_devices := oraclerestartcommon.GetAsmDevices(instance)
	var data []string
	var addnodeFlag bool

	data = append(data, "OP_TYPE=setuprac")
	// --- Pick ALL envVars directly from CR spec ---
	for _, e := range instance.Spec.InstDetails.EnvVars {
		data = append(data, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}

	// Service Parameters
	if instance.Spec.ServiceDetails.Name != "" {
		sparams := oraclerestartcommon.GetServiceParams(instance)
		data = append(data, "DB_SERVICE="+sparams)
	}
	data = append(data, "CRS_GPC=true")

	if instance.Spec.ConfigParams.PdbName != "" {
		data = append(data, "ORACLE_PDB="+instance.Spec.ConfigParams.PdbName)
	}

	if instance.Spec.ConfigParams.DbHome != "" {
		data = append(data, "DB_HOME="+instance.Spec.ConfigParams.DbHome)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbHome != "" {
				data = append(data, "DB_HOME="+instance.Status.ConfigParams.DbHome)
			}
		}
	}

	if instance.Spec.ConfigParams.DbBase != "" {
		data = append(data, "DB_BASE="+instance.Spec.ConfigParams.DbBase)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbBase != "" {
				data = append(data, "DB_BASE="+instance.Status.ConfigParams.DbBase)
			}
		}
	}

	if instance.Spec.ConfigParams.GridBase != "" {
		data = append(data, "GRID_BASE="+instance.Spec.ConfigParams.GridBase)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.GridBase != "" {
				data = append(data, "GRID_BASE="+instance.Status.ConfigParams.GridBase)
			}
		}
	}

	if instance.Spec.ConfigParams.GridHome != "" {
		data = append(data, "GRID_HOME="+instance.Spec.ConfigParams.GridHome)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.GridHome != "" {
				data = append(data, "GRID_HOME="+instance.Status.ConfigParams.GridHome)
			}
		}
	}

	if instance.Spec.ConfigParams.Inventory != "" {
		data = append(data, "INVENTORY="+instance.Spec.ConfigParams.Inventory)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.Inventory != "" {
				data = append(data, "INVENTORY="+instance.Status.ConfigParams.Inventory)
			}
		}
	}

	if instance.Spec.SshKeySecret.Name != " " {
		//SecretMap check is done in ValidateSpex
		data = append(data, "SSH_PRIVATE_KEY="+instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PrivKeySecretName)
		data = append(data, "SSH_PUBLIC_KEY="+instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PubKeySecretName)
	}

	if instance.Spec.DbSecret != nil {
		if instance.Spec.DbSecret.Name != "" {
			data = append(data, "SECRET_VOLUME="+instance.Spec.DbSecret.PwdFileMountLocation)
			commonpassflag, pwdkeyflag, _ := oraclerestartcommon.GetDbSecret(instance, instance.Spec.DbSecret.Name, r.Client)
			if commonpassflag && pwdkeyflag {
				data = append(data, "DB_PWD_FILE="+instance.Spec.DbSecret.PwdFileName)
				data = append(data, "PWD_KEY="+instance.Spec.DbSecret.KeyFileName)
			} else {
				data = append(data, "PASSWORD_FILE=pwdfile")
			}
		}
	}

	if instance.Spec.TdeWalletSecret != nil {
		if instance.Spec.TdeWalletSecret.Name != "" {
			data = append(data, "TDE_SECRET_VOLUME="+instance.Spec.TdeWalletSecret.PwdFileMountLocation)
			data = append(data, "SETUP_TDE_WALLET=true")
			tdepassflag, tdepwdkeyflag, _ := oraclerestartcommon.GetTdeWalletSecret(instance, instance.Spec.TdeWalletSecret.Name, r.Client)
			if tdepassflag && tdepwdkeyflag {
				data = append(data, "TDE_PWD_FILE="+instance.Spec.TdeWalletSecret.PwdFileName)
				data = append(data, "TDE_PWD_KEY="+instance.Spec.TdeWalletSecret.KeyFileName)
			} else {
				data = append(data, "PASSWORD_FILE=tdepwdfile")
			}
		}
	}

	data = append(data, "PROFILE_FLAG=true")
	// data = append(data, "SCAN_NAME="+scan_name)

	data = append(data, "INSTALL_NODE="+install_node)

	if instance.Spec.ConfigParams.DbName != "" {
		data = append(data, "DB_NAME="+instance.Spec.ConfigParams.DbName)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbName != "" {
				data = append(data, "DB_NAME="+instance.Status.ConfigParams.DbName)
			}
		}
	}

	if instance.Spec.ConfigParams.PdbName != "" {
		data = append(data, "ORACLE_PDB_NAME="+instance.Spec.ConfigParams.PdbName)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.PdbName != "" {
				data = append(data, "ORACLE_PDB_NAME="+instance.Status.ConfigParams.PdbName)
			}
		}
	}

	if instance.Spec.ConfigParams.DbUniqueName != "" {
		// Configmap check is done in ValidateSpex
		data = append(data, "DB_UNIQUE_NAME="+instance.Spec.ConfigParams.DbUniqueName)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbUniqueName != "" {
				data = append(data, "DB_UNIQUE_NAME="+instance.Status.ConfigParams.DbUniqueName)
			}
		}
	}

	if instance.Spec.ConfigParams.GridSwZipFile != "" {
		data = append(data, "GRID_SW_ZIP_FILE="+instance.Spec.ConfigParams.GridSwZipFile)
		//data = append(data, "COPY_GRID_SOFTWARE=true")
	}

	if instance.Spec.ConfigParams.HostSwStageLocation != "" {
		data = append(data, "STAGING_SOFTWARE_LOC="+instance.Spec.ConfigParams.HostSwStageLocation)
	} else {
		data = append(data, "STAGING_SOFTWARE_LOC="+utils.OraSwStageLocation)
	}

	if instance.Spec.ConfigParams.RuPatchLocation != "" {
		data = append(data, "APPLY_RU_LOCATION="+instance.Spec.ConfigParams.RuPatchLocation)
	}

	if instance.Spec.ConfigParams.RuFolderName != "" {
		data = append(data, "RU_FOLDER_NAME="+instance.Spec.ConfigParams.RuFolderName)
	}

	if instance.Spec.ConfigParams.OPatchLocation != "" {
		data = append(data, "OPATCH_ZIP_FILE="+instance.Spec.ConfigParams.OPatchLocation+"/"+instance.Spec.ConfigParams.OPatchSwZipFile)
	}
	if instance.Spec.ConfigParams.OneOffLocation != "" {
		data = append(data, "ONEOFF_FOLDER_NAME="+instance.Spec.ConfigParams.OneOffLocation)
	}
	if instance.Spec.ConfigParams.DbOneOffIds != "" {
		data = append(data, "DB_ONEOFF_IDS="+instance.Spec.ConfigParams.DbOneOffIds)
	}

	if instance.Spec.ConfigParams.GridOneOffIds != "" {
		data = append(data, "GRID_ONEOFF_IDS="+instance.Spec.ConfigParams.GridOneOffIds)
	}

	if instance.Spec.ConfigParams.DbSwZipFile != "" {
		data = append(data, "DB_SW_ZIP_FILE="+instance.Spec.ConfigParams.DbSwZipFile)
		//data = append(data, "COPY_DB_SOFTWARE=true")
	}

	if instance.Spec.ConfigParams.CrsAsmDiskDg != "" {
		data = append(data, "CRS_ASM_DISKGROUP="+instance.Spec.ConfigParams.CrsAsmDiskDg)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.CrsAsmDiskDg != "" {
				data = append(data, "CRS_ASM_DISKGROUP="+instance.Status.ConfigParams.CrsAsmDiskDg)
			}
		}
	}

	if instance.Spec.ConfigParams.DbAsmDeviceList != "" {
		data = append(data, "DB_ASM_DEVICE_LIST="+instance.Spec.ConfigParams.DbAsmDeviceList)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.DbAsmDeviceList != "" {
				data = append(data, "DB_ASM_DEVICE_LIST="+instance.Status.ConfigParams.DbAsmDeviceList)
			}
		}
	}

	if instance.Spec.ConfigParams.CrsAsmDeviceList != "" {
		data = append(data, "CRS_ASM_DEVICE_LIST="+instance.Spec.ConfigParams.CrsAsmDeviceList)
	} else {
		data = append(data, "CRS_ASM_DEVICE_LIST="+asm_devices)
	}

	if instance.Spec.ConfigParams.RecoAsmDeviceList != "" {
		data = append(data, "RECO_ASM_DEVICE_LIST="+instance.Spec.ConfigParams.RecoAsmDeviceList)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.RecoAsmDeviceList != "" {
				data = append(data, "RECO_ASM_DEVICE_LIST="+instance.Status.ConfigParams.RecoAsmDeviceList)
			}
		}
	}

	if instance.Spec.ConfigParams.RedoAsmDeviceList != "" {
		data = append(data, "REDO_ASM_DEVICE_LIST="+instance.Spec.ConfigParams.RedoAsmDeviceList)
	} else {
		if instance.Status.ConfigParams != nil {
			if instance.Status.ConfigParams.RedoAsmDeviceList != "" {
				data = append(data, "REDO_ASM_DEVICE_LIST="+instance.Status.ConfigParams.RedoAsmDeviceList)
			}
		}
	}

	// Perform following if operation is not add node
	if !addnodeFlag {
		if instance.Spec.ConfigParams.DbDataFileDestDg != "" {
			data = append(data, "DB_DATA_FILE_DEST="+instance.Spec.ConfigParams.DbDataFileDestDg)
		}

		if instance.Spec.ConfigParams.DbStorageType != "" {
			data = append(data, "DB_STORAGE_TYPE="+instance.Spec.ConfigParams.DbStorageType)
		}

		if instance.Spec.ConfigParams.DbCharSet != "" {
			data = append(data, "DB_CHARACTERSET="+instance.Spec.ConfigParams.DbCharSet)
		}

		if instance.Spec.ConfigParams.DbRedoFileSize != "" {
			data = append(data, "DB_REDO_FILE_SIZE="+instance.Spec.ConfigParams.DbRedoFileSize)
		}

		if instance.Spec.ConfigParams.DbType != "" {
			data = append(data, "DB_TYPE="+instance.Spec.ConfigParams.DbType)
		}

		if instance.Spec.ConfigParams.DbConfigType != "" {
			data = append(data, "DB_CONFIG_TYPE="+instance.Spec.ConfigParams.DbConfigType)
		}

		if instance.Spec.ConfigParams.EnableArchiveLog != "" {
			data = append(data, "ENABLE_ARCHIVELOG="+instance.Spec.ConfigParams.EnableArchiveLog)
		}

		if instance.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "GRID_RESPONSE_FILE="+utils.OraGiRsp+"/"+instance.Spec.ConfigParams.GridResponseFile.Name)
		}

		if instance.Spec.ConfigParams.DbResponseFile.ConfigMapName != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "DBCA_RESPONSE_FILE="+utils.OraDbRsp+"/"+instance.Spec.ConfigParams.DbResponseFile.Name)
		}

		// Getting DB Related paraeters

		if instance.Spec.ConfigParams.SgaSize != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "INIT_SGA_SIZE="+instance.Spec.ConfigParams.SgaSize)
		}

		if instance.Spec.ConfigParams.PgaSize != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "INIT_PGA_SIZE="+instance.Spec.ConfigParams.PgaSize)
		}

		if instance.Spec.ConfigParams.Processes > 0 {
			// Configmap check is done in ValidateSpex
			data = append(data, "INIT_PROCESSES="+strconv.Itoa(instance.Spec.ConfigParams.Processes))
		}

		if instance.Spec.ConfigParams.CpuCount > 0 {
			// Configmap check is done in ValidateSpex
			data = append(data, "CPU_COUNT="+strconv.Itoa(instance.Spec.ConfigParams.CpuCount))
		}

		if instance.Spec.ConfigParams.DbRecoveryFileDest != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "DB_RECOVERY_FILE_DEST="+instance.Spec.ConfigParams.DbRecoveryFileDest)
		}

		if instance.Spec.ConfigParams.DbRecoveryFileDestSize != "" {
			// Configmap check is done in ValidateSpex
			data = append(data, "DB_RECOVERY_FILE_DEST_SIZE="+instance.Spec.ConfigParams.DbRecoveryFileDestSize)
		}
		if instance.Spec.ConfigParams.DBAsmDiskDgRedundancy != "" {
			data = append(data, "DB_ASMDG_PROPERTIES="+"redundancy:"+instance.Spec.ConfigParams.DBAsmDiskDgRedundancy)
		}

		if instance.Spec.ConfigParams.RedoAsmDiskDgRedudancy != "" {
			data = append(data, "REDO_ASMDG_PROPERTIES="+"redundancy:"+instance.Spec.ConfigParams.RedoAsmDiskDgRedudancy)
		}

		if instance.Spec.ConfigParams.RecoAsmDiskDgRedundancy != "" {
			data = append(data, "RECO_ASMDG_PROPERTIES="+"redundancy:"+instance.Spec.ConfigParams.RecoAsmDiskDgRedundancy)
		}

		if instance.Spec.ConfigParams.CrsAsmDiskDgRedundancy != "" {
			data = append(data, "CRS_ASMDG_REDUNDANCY="+"redundancy="+instance.Spec.ConfigParams.CrsAsmDiskDgRedundancy)
		}
	}

	configMapData["envfile"] = strings.Join(data, "\r\n")

	return configMapData, nil
}

// ================================== CREATE FUNCTIONS =============================

// Create the configmap

func (r *OracleRestartReconciler) createConfigMap(
	ctx context.Context,
	instance oraclerestartdb.OracleRestart,
	cm *corev1.ConfigMap,
) (ctrl.Result, bool, error) { // Added `bool` return
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cm.Name,
		Namespace: instance.Namespace,
	}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// ConfigMap does not exist - create it
		reqLogger.Info("Creating Configmap Normally")
		if err = r.Create(ctx, cm); err != nil {
			reqLogger.Error(err, "failed to create configmap", "namespace", instance.Namespace)
			return ctrl.Result{}, false, err
		}
		return ctrl.Result{Requeue: true}, true, nil // Indicate configmap was created
	} else if err != nil {
		// Error getting ConfigMap
		reqLogger.Error(err, "failed to find the configmap details")
		return ctrl.Result{}, false, err
	}

	// At this point, ConfigMap exists: found
	// Compare data and update if needed only for environment variables changes
	if found.Data["envfile"] != cm.Data["envfile"] {
		reqLogger.Info("ConfigMap env key changed, updating")
		found.Data["envfile"] = cm.Data["envfile"]
		if err := r.Update(ctx, found); err != nil {
			reqLogger.Error(err, "failed to update configmap", "namespace", instance.Namespace)
			return ctrl.Result{}, false, err
		}
		return ctrl.Result{Requeue: true}, true, nil // Indicate data was changed
	}

	// No changes needed
	return ctrl.Result{}, false, nil
}

// This function create a service based isExtern parameter set in the yaml file
func (r *OracleRestartReconciler) createOrReplaceService(ctx context.Context, instance *oraclerestartdb.OracleRestart,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	// See if Service already exists and create if it doesn't

	found := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && apierrors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a service")
		err = r.Create(ctx, dep)
		if err != nil {
			// Service creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Service", "Service.Namespace", dep.Namespace, "Service.Name", dep.Name)
			return ctrl.Result{}, nil
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

// ================================== CREATE FUNCTIONS =============================
// This function create a service based isExtern parameter set in the yaml file

func (r *OracleRestartReconciler) createOrReplaceAsmPv(
	ctx context.Context,
	instance *oraclerestartdb.OracleRestart,
	dep *corev1.PersistentVolume,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	found := &corev1.PersistentVolume{}
	if dep == nil {
		reqLogger.Error(nil, "PersistentVolume spec (dep) is nil")
		return "", ctrl.Result{}, fmt.Errorf("PV object is nil")
	}

	// Fetch the existing PV
	err := r.Get(context.TODO(), types.NamespacedName{
		Name: dep.Name,
	}, found)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	if err != nil && apierrors.IsNotFound(err) {
		// PV does not exist, create it
		reqLogger.Info("Creating a new PV", "dep.Name", dep.Name)
		err = r.Create(context.TODO(), dep)
		if err != nil {
			// PV creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Persistent Volume", "PV.Name", dep.Name)
			return "", ctrl.Result{}, err
		}
		return dep.Name, ctrl.Result{}, nil
	} else if err != nil {
		// Other errors fetching the PV
		reqLogger.Error(err, "Failed to get Persistent Volume details")
		return "", ctrl.Result{}, err
	}

	// Check if the disk path or configuration differs from the existing PV
	if !reflect.DeepEqual(dep.Spec.PersistentVolumeSource.Local, found.Spec.PersistentVolumeSource.Local) {
		// Disk configuration has changed, delete the old PV and create a new one
		reqLogger.Info("Detected existing PV with different disk details and as the configuration has changed, setup cannot continue", "dep.Name", dep.Name)
		return "", ctrl.Result{}, fmt.Errorf("persistent volume %s has a different disk configuration. Please delete or update the existing PV to proceed", dep.Name)
	}

	reqLogger.Info("PV Found", "dep.Name", dep.Name)

	return found.Name, ctrl.Result{}, nil
}

// ================================== CREATE FUNCTIONS =============================
// This function create a PVC set in the yaml file
func (r *OracleRestartReconciler) createOrReplaceAsmPvC(ctx context.Context, instance *oraclerestartdb.OracleRestart,
	dep *corev1.PersistentVolumeClaim,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	found := &corev1.PersistentVolumeClaim{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: instance.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	if err != nil && apierrors.IsNotFound(err) {
		// Create the Service
		reqLogger.Info("Creating a PVC")
		dep.Spec.Selector = nil
		err = r.Create(ctx, dep)
		if err != nil {
			// Service creation failed
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create Persistent Volume", "PVC.Namespace", dep.Namespace, "PersistentVolume.Name", dep.Name)
			return "", ctrl.Result{}, err
		} else {
			// Service creation was successful
			return dep.Name, ctrl.Result{}, nil
		}
	} else if err != nil {
		// Error that isn't due to the Service not existing
		reqLogger.Error(err, "Failed to find the persistent volume Claim details")
		return "", ctrl.Result{}, err
	}

	return found.Name, ctrl.Result{}, nil
}

// ensureAsmStorageStatus initializes AsmStorageDetails and AsmStorageStatus if they are nil
func (r *OracleRestartReconciler) ensureAsmStorageStatus(oracleRestart *oraclerestartdb.OracleRestart) {
	// Check if AsmDetails is nil and initialize it if necessary
	if oracleRestart.Status.AsmDetails == nil {
		oracleRestart.Status.AsmDetails = &oraclerestartdb.AsmInstanceStatus{
			Diskgroup: []oraclerestartdb.AsmDiskgroupStatus{},
		}
	}

}

func (r *OracleRestartReconciler) ensureStatefulSetUpdated(ctx context.Context,
	reqLogger logr.Logger,
	oracleRestart *oraclerestartdb.OracleRestart,
	desired *appsv1.StatefulSet,
	asmAutoUpdate bool,
	// isDelete bool,
	req ctrl.Request) error {
	timeout := 15 * time.Minute // Set a timeout for the update wait
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Fetch the existing StatefulSet
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desired.Name,
		Namespace: oracleRestart.Namespace,
	}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the StatefulSet doesn't exist, create it
			reqLogger.Info("StatefulSet not found, creating new one", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
			return r.Create(ctx, desired)
		}
		reqLogger.Error(err, "Failed to get StatefulSet", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
		return err
	}

	// Compare the existing StatefulSet spec with the desired spec, sfs is replaced when ASM devices are added or removed
	if len(existing.Spec.Template.Spec.Containers[0].VolumeDevices) != len(desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
		r.Log.Info("Change State to UPDATING")

		// Update status for each instance
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, oracleRestart.Spec.InstDetails, string(oraclerestartdb.OracleRestartUpdateState), r.Client, true)

		reqLogger.Info("StatefulSet spec differs for volume devices, updating StatefulSet (pods may be recreated)", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)

		// Perform the update
		err := r.Update(ctx, desired)
		if err != nil {
			reqLogger.Error(err, "Failed to update StatefulSet", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
			return err
		}

		reqLogger.Info("StatefulSet update applied, waiting for pod recreation", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
		// } else {
		// r.Log.Info("Change State to UPDATING")

		// Wait for the update to be applied
		for {
			select {
			case <-timeoutCtx.Done():
				reqLogger.Error(timeoutCtx.Err(), "Timed out waiting for StatefulSet update", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
				return timeoutCtx.Err()

			default:
				updated := &appsv1.StatefulSet{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      desired.Name,
					Namespace: oracleRestart.Namespace,
				}, updated)

				if err != nil {
					reqLogger.Error(err, "Failed to get StatefulSet after update", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
					return err
				}

				if reflect.DeepEqual(updated.Spec.Template.Spec.Containers[0].VolumeDevices, desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
					reqLogger.Info("StatefulSet update is applied successfully", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
					return nil
				}

				reqLogger.Info("Waiting for StatefulSet update to be applied", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
				time.Sleep(5 * time.Second)
			}
		}
		// }
	} else {
		reqLogger.Info("StatefulSet matches for  ASM devices, SFS wont be updated", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
		return nil
	}
}

func executeDiskGroupCommand(podName string, cmd []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *oraclerestartdb.OracleRestart, logger logr.Logger) (string, string, error) {
	return oraclerestartcommon.ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, logger)
}

// Function to get the disk group name
func getDiskGroupName(deviceDg string, oracleRestart *oraclerestartdb.OracleRestart) string {
	switch deviceDg {
	case oracleRestart.Spec.ConfigParams.CrsAsmDeviceList:
		return oracleRestart.Spec.ConfigParams.CrsAsmDiskDg
	case oracleRestart.Spec.ConfigParams.DbAsmDeviceList:
		return oracleRestart.Spec.ConfigParams.DbDataFileDestDg
	default:
		return ""
	}
}

// Function to check if a disk group exists
func (r *OracleRestartReconciler) diskGroupExists(podName, diskGroupName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *oraclerestartdb.OracleRestart, logger logr.Logger) (bool, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	cmd := "python3 /opt/scripts/startup/scripts/main.py --getasmdiskgroup=true"
	stdout, _, err := oraclerestartcommon.ExecCommand(podName, []string{"bash", "-c", cmd}, r.kubeClient, r.kubeConfig, instance, reqLogger)
	if err != nil {
		return false, err
	}
	if strings.Contains(stdout, diskGroupName) {
		return true, nil
	}
	return false, nil
}

// Function to add disks
func (r *OracleRestartReconciler) addDisks(ctx context.Context, podList *corev1.PodList, instance *oraclerestartdb.OracleRestart, diskGroupName string, deviceList []string) error {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	// Remove '+' prefix if present
	if strings.HasPrefix(diskGroupName, "+") {
		diskGroupName = strings.TrimPrefix(diskGroupName, "+")
	}

	for _, pod := range podList.Items {
		podName := pod.Name

		// Check if the disk group exists before trying to add disks
		exists, err := r.diskGroupExists(podName, diskGroupName, r.kubeClient, r.kubeConfig, instance, reqLogger)
		if err != nil {
			reqLogger.Error(err, "Failed to check if disk group exists", "Pod.Name", podName, "DiskGroup", diskGroupName)
			return err
		}
		if !exists {
			err = fmt.Errorf("disk group %s does not exist", diskGroupName)
			reqLogger.Error(err, "Disk group does not exist", "Pod.Name", podName, "DiskGroup", diskGroupName)
			return err
		}

		for _, disk := range deviceList {
			cmd := fmt.Sprintf("python3 /opt/scripts/startup/scripts/main.py --updateasmdevices=\"diskname=%s;diskgroup=%s;processtype=addition\"", disk, diskGroupName)
			reqLogger.Info("Executing command to add disk", "Pod.Name", podName, "Command", cmd)
			stdout, stderr, err := oraclerestartcommon.ExecCommand(podName, []string{"bash", "-c", cmd}, r.kubeClient, r.kubeConfig, instance, reqLogger)
			if err != nil {
				instance.Spec.IsFailed = true
				reqLogger.Error(err, "Failed to execute command", "Pod.Name", podName, "Command", cmd, "Stdout", stdout, "Stderr", stderr)
				return err
			}
		}
	}
	return nil
}

// Function to check DaemonSet status with retry, timeout, and log analysis
func checkDaemonSetStatus(ctx context.Context, r *OracleRestartReconciler, oracleRestart *oraclerestartdb.OracleRestart) (bool, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	defer tick.Stop()
	// Sleep for 60 seconds
	for {
		select {
		case <-timeout:
			// Timeout reached
			ds := &appsv1.DaemonSet{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      "disk-check-daemonset",
				Namespace: oracleRestart.Namespace,
			}, ds)
			if err != nil {
				return false, err
			}

			// Fetch the list of Pods managed by the DaemonSet
			pods, err := r.kubeClient.CoreV1().Pods(oracleRestart.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=disk-check",
			})
			if err != nil {
				return false, err
			}

			// Check logs from each Pod
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					// Pod is not running, check for logs and errors
					logs, err := r.kubeClient.CoreV1().Pods(oracleRestart.Namespace).GetLogs(
						pod.Name,
						&corev1.PodLogOptions{},
					).DoRaw(ctx)
					if err != nil {
						return false, err
					}

					if bytes.Contains(logs, []byte("not a valid block device")) {
						// Disk validation failed
						return false, nil
					}
				}
			}

			// DaemonSet did not become ready or running within the timeout
			return false, fmt.Errorf("DaemonSet %s/%s did not become ready or running within 5 minutes", oracleRestart.Namespace, "disk-check-daemonset")

		case <-tick.C:
			// Check DaemonSet status
			ds := &appsv1.DaemonSet{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Name:      "disk-check-daemonset",
				Namespace: oracleRestart.Namespace,
			}, ds)
			if err != nil {
				return false, err
			}

			// Check DaemonSet readiness
			if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.NumberReady > 0 {
				// DaemonSet is running and ready
				return true, nil
			}

			// If DaemonSet is not ready, fetch the list of Pods managed by the DaemonSet
			pods, err := r.kubeClient.CoreV1().Pods(oracleRestart.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=disk-check",
			})
			if err != nil {
				return false, err
			}

			// Check logs from each Pod
			for _, pod := range pods.Items {
				// Pod is not running, check for logs and errors
				logs, err := r.kubeClient.CoreV1().Pods(oracleRestart.Namespace).GetLogs(
					pod.Name,
					&corev1.PodLogOptions{},
				).DoRaw(ctx)
				if err != nil {
					return false, err
				}

				if bytes.Contains(logs, []byte("not a valid block device")) {
					// Disk validation failed
					return false, nil
				}

			}
		}
	}
}

// ================================== CREATE FUNCTIONS =============================
func (r *OracleRestartReconciler) createOrReplaceSfs(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart oraclerestartdb.OracleRestart,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldState string,
	configmapChanged bool,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", oracleRestart.Namespace, "Instance.Name", oracleRestart.Name)
	found := &appsv1.StatefulSet{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: oracleRestart.Namespace,
	}, found)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, &oracleRestart, r.Log)

	if err != nil && apierrors.IsNotFound(err) {
		// CREATE
		r.updateOracleRestartInstStatus(&oracleRestart, ctx, req, oracleRestart.Spec.InstDetails,
			string(oraclerestartdb.OracleRestartProvisionState), r.Client, true)
		reqLogger.Info("Creating a StatefulSet Normally", "StatefulSetName", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			oracleRestart.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to create StatefulSet", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
			return ctrl.Result{}, err
		} else if !isLast {
			// StatefulSet creation was successful
			return ctrl.Result{}, nil
		}
	} else if err != nil {
		// Any other Get error
		reqLogger.Error(err, "Failed to find the StatefulSet details")
		return ctrl.Result{}, err
	} else {
		// Compare resource requirements
		foundRes := found.Spec.Template.Spec.Containers[0].Resources
		depRes := dep.Spec.Template.Spec.Containers[0].Resources
		resourcesChanged := !reflect.DeepEqual(foundRes, depRes)

		// Compare configMap relevant data (example: pass in variable configmapChanged)
		if resourcesChanged || configmapChanged {
			// Copy metadata fields that must be preserved
			dep.ResourceVersion = found.ResourceVersion
			dep.UID = found.UID
			dep.CreationTimestamp = found.CreationTimestamp
			dep.ManagedFields = found.ManagedFields
			dep.Status = found.Status

			reason := "unknown"
			if resourcesChanged && configmapChanged {
				reason = "resource and configmap change"
			} else if resourcesChanged {
				reason = "resource change"
			} else if configmapChanged {
				reason = "configmap change"
			}

			reqLogger.Info("Updating StatefulSet due to "+reason, "StatefulSetName", dep.Name)
			err = r.Update(ctx, dep)
			if err != nil {
				oracleRestart.Spec.IsFailed = true
				reqLogger.Error(err, "Failed to update StatefulSet", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// ================================== CREATE FUNCTIONS =============================
// This function create a PVC set in the yaml file
func (r *OracleRestartReconciler) createOrReplaceSfsAsm(ctx context.Context, req ctrl.Request, oracleRestart *oraclerestartdb.OracleRestart,
	dep *appsv1.StatefulSet, asmAutoUpdate bool, index int, isLast bool, oldSpec *oraclerestartdb.OracleRestartSpec,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("oracleRestart.Namespace", oracleRestart.Namespace, "oracleRestart.Name", oracleRestart.Name)

	found := &appsv1.StatefulSet{}

	// Check if the StatefulSet was found successfully
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: oracleRestart.Namespace,
	}, found)
	if err != nil {
		reqLogger.Error(err, "Failed to find existing StatefulSet to update")
		return ctrl.Result{}, err
	}

	addedAsmDisks, removedAsmDisks := getAddedAndRemovedDisks(oracleRestart, oldSpec, index)

	// Deletion Process execution
	// isDelete := false
	inUse := false
	if len(removedAsmDisks) > 0 {
		OraRacSpex := oracleRestart.Spec.InstDetails
		racSfSet, err := oraclerestartcommon.CheckSfset(OraRacSpex.Name, oracleRestart, r.Client)
		if err != nil {
			errMsg := fmt.Errorf("failed to retrieve StatefulSet for RAC database '%s': %w", OraRacSpex.Name, err)
			r.Log.Error(err, errMsg.Error())
			return reconcile.Result{}, errMsg
		}

		// Step 2: Get the Pod list
		podList, err := oraclerestartcommon.GetPodList(racSfSet.Name, oracleRestart, r.Client, OraRacSpex)
		if err != nil {
			errMsg := fmt.Errorf("failed to retrieve pod list for StatefulSet '%s': %w", racSfSet.Name, err)
			r.Log.Error(err, errMsg.Error())
			return reconcile.Result{}, errMsg
		}
		if len(podList.Items) == 0 {
			errMsg := fmt.Errorf("no pods found for StatefulSet '%s'", racSfSet.Name)
			r.Log.Error(errMsg, "Empty pod list")
			return reconcile.Result{}, errMsg
		}

		// Step 3: Use last pod to get ASM state
		podName := podList.Items[len(podList.Items)-1].Name
		oracleRestart.Status.AsmDetails = oraclerestartcommon.GetAsmInstState(podName, oracleRestart, 0, r.kubeClient, r.kubeConfig, r.Log)

		// Check if removed disks are in use in any diskgroup
		asmInstanceStatus := oracleRestart.Status.AsmDetails
		// isDelete = true
		for _, removedAsmDisk := range removedAsmDisks {
			for _, diskgroup := range asmInstanceStatus.Diskgroup {
				for _, asmDiskStr := range diskgroup.Disks {
					asmDisks := strings.Split(asmDiskStr, ",")
					// Now compare each disk with removedAsmDisk
					for _, asmDisk := range asmDisks {
						if removedAsmDisk == asmDisk {
							inUse = true
							// / Disk is in use, return a message to the user and dont proceed further
							err := fmt.Errorf("disk '%s' is part of diskgroup '%s' and must be manually removed before proceeding", removedAsmDisk, diskgroup.Name)
							r.Log.Info("Disk is in use and cannot be removed. Must be manually removed before proceeding", "disk", removedAsmDisk, "diskgroup", diskgroup.Name)
							return reconcile.Result{}, err
						}
					}
				}
			}
		}
	}

	r.ensureAsmStorageStatus(oracleRestart)

	// Ensure the StatefulSet is updated or re-created based on autoUpdate set to true/false
	// err = r.ensureStatefulSetUpdated(ctx, reqLogger, OracleRestart, dep, autoUpdate, isDelete, req)
	err = r.ensureStatefulSetUpdated(ctx, reqLogger, oracleRestart, dep, asmAutoUpdate, req)
	if err != nil {
		oracleRestart.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to ensure StatefulSet is updated or created")
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, oracleRestart.Spec.InstDetails, string(oraclerestartdb.OracleRestartFailedState), r.Client, true)
		return ctrl.Result{}, err
	}

	// Wait for all Pods to be created and running
	podList := &corev1.PodList{}
	timeout := time.After(15 * time.Minute) // 15-minute timeout
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	allPodsRunning := false

waitLoop:
	for {
		select {
		case <-timeout:
			reqLogger.Info("Timed out waiting for all Pods to be created and running", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
			return reconcile.Result{}, fmt.Errorf("timed out waiting for all Pods to be created and running")
		case <-ticker.C:
			err = r.List(ctx, podList, client.InNamespace(dep.Namespace), client.MatchingLabels(dep.Spec.Template.Labels))
			if err != nil {
				reqLogger.Error(err, "Failed to list Pods", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
				return reconcile.Result{}, err
			}

			allPodsRunning = true
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					allPodsRunning = false
					reqLogger.Info("Waiting for Pod to be running", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
					break
				}
			}

			if allPodsRunning {
				reqLogger.Info("All Pods are running", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
				break waitLoop
			}
		}
	}

	if allPodsRunning {
		const (
			podCheckInterval = 15 * time.Second // Interval between pod readiness checks
			podReadyTimeout  = 15 * time.Minute // Maximum wait time for pod readiness
		)

		timeoutCtx, cancel := context.WithTimeout(ctx, podReadyTimeout)
		defer cancel()

		// Wait for StatefulSet pods to be ready
		for i := 0; i < len(dep.Spec.Template.Spec.Containers); i++ {
			podName := fmt.Sprintf("%s-%d", dep.Name, i)
			var isPodReady bool

		waitForPodASM:
			for {
				select {
				case <-timeoutCtx.Done():
					reqLogger.Error(timeoutCtx.Err(), "Timed out waiting for pod to be ready", "Pod.Name", podName)
					r.updateOracleRestartInstStatus(oracleRestart, ctx, req, oracleRestart.Spec.InstDetails, string(oraclerestartdb.OracleRestartFailedState), r.Client, true)
					return ctrl.Result{}, timeoutCtx.Err()
				default:
					podList, err := oraclerestartcommon.GetPodList(dep.Name, oracleRestart, r.Client, oracleRestart.Spec.InstDetails)
					if err != nil {
						reqLogger.Error(err, "Failed to list pods")
						return ctrl.Result{}, err
					}
					time.Sleep(podCheckInterval)
					isPodReady, _, _ = oraclerestartcommon.PodListValidation(podList, dep.Name, oracleRestart, r.Client)
					if isPodReady {
						reqLogger.Info("Pod is ready", "Pod.Name", podName)
						break waitForPodASM // Break out of the labeled loop
					} else {
						reqLogger.Info("Pod is not ready yet", "Pod.Name", podName)
						time.Sleep(podCheckInterval)
					}
				}
			}
		}

	}

	// Disk is not in use, proceed with PV and PVC deletion in last stage
	if isLast && !inUse {
		// Use oraclerestartcommon.GetAsmPvcName and oraclerestartcommon.getAsmPvName to generate PVC and PV names

		// Find and delete the corresponding PVC
		for _, diskName := range oracleRestart.Status.OracleRestartNodes[index].NodeDetails.MountedDevices {
			for _, removedAsmDisk := range removedAsmDisks {
				if diskName == removedAsmDisk {
					pvcName := oraclerestartcommon.GetAsmPvcName(oracleRestart.Name, diskName, oracleRestart) // Use the existing function
					pvc := &corev1.PersistentVolumeClaim{}
					err := r.Get(ctx, client.ObjectKey{
						Name:      pvcName,
						Namespace: oracleRestart.Namespace,
					}, pvc)
					if err != nil {
						if !apierrors.IsNotFound(err) {
							r.Log.Error(err, "Failed to get PVC", "PVC.Name", pvcName)
							return reconcile.Result{}, err
						}
						// PVC already deleted
					} else {
						err = r.Delete(ctx, pvc)
						if err != nil {
							r.Log.Error(err, "Failed to delete PVC", "PVC.Name", pvcName)
							return reconcile.Result{}, err
						}
						r.Log.Info("Successfully deleted PVC", "PVC.Name", pvcName)
					}

					// Find and delete the corresponding PV
					pvName := oraclerestartcommon.GetAsmPvName(oracleRestart.Name, diskName, oracleRestart) // Use the existing function
					pv := &corev1.PersistentVolume{}
					err = r.Get(ctx, client.ObjectKey{
						Name: pvName,
					}, pv)
					if err != nil {
						if !apierrors.IsNotFound(err) {
							r.Log.Error(err, "Failed to get PV", "PV.Name", pvName)
							return reconcile.Result{}, err
						}
						// PV already deleted
					} else {
						err = r.Delete(ctx, pv)
						if err != nil {
							r.Log.Error(err, "Failed to delete PV", "PV.Name", pvName)
							return reconcile.Result{}, err
						}
						r.Log.Info("Successfully deleted PV", "PV.Name", pvName)
					}
				}
			}
		}
	}

	if isLast && asmAutoUpdate {
		// last iteration
		deviceDg := ""
		for _, disk := range addedAsmDisks {
			if isDiskInDeviceList(disk, oracleRestart.Spec.ConfigParams.CrsAsmDeviceList) {
				reqLogger.Info("New disk to be added to CRS ASM device list ", "disk", disk)
				deviceDg = oracleRestart.Spec.ConfigParams.CrsAsmDiskDg
			}
			if isDiskInDeviceList(disk, oracleRestart.Spec.ConfigParams.DbAsmDeviceList) {
				reqLogger.Info("New disk to be added to DB ASM device list ", "disk", disk)
				deviceDg = oracleRestart.Spec.ConfigParams.DbDataFileDestDg
			}
			if isDiskInDeviceList(disk, oracleRestart.Spec.ConfigParams.RecoAsmDeviceList) {
				reqLogger.Info("New disk to be added to RECO ASM device list ", "disk", disk)
				deviceDg = oracleRestart.Spec.ConfigParams.RecoAsmDiskDgRedundancy
			}
			if isDiskInDeviceList(disk, oracleRestart.Spec.ConfigParams.RedoAsmDeviceList) {
				reqLogger.Info("New disk to be added to REDO ASM device list ", "disk", disk)
				deviceDg = oracleRestart.Spec.ConfigParams.RedoAsmDiskDgRedudancy
			}
		}
		if deviceDg != "" {
			// Add disks after POD recreation
			podList, err := oraclerestartcommon.GetPodList(dep.Name, oracleRestart, r.Client, oracleRestart.Spec.InstDetails)
			if err != nil {
				reqLogger.Error(err, "Failed to list pods")
				return ctrl.Result{}, err
			}
			err = r.addDisks(ctx, podList, oracleRestart, deviceDg, addedAsmDisks)
			if err != nil {
				return ctrl.Result{}, err
			}
			reqLogger.Info("New Disks added to CRS Disks Group")
		}
	}

	return ctrl.Result{}, nil
}

func getAddedAndRemovedDisks(oracleRestart *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec, index int) ([]string, []string) {
	addedAsmDisks := []string{}
	removedAsmDisks := []string{}

	// Helper function to compare desired and previous disk lists
	compareDisks := func(newDisks, oldDisks []string) ([]string, []string) {
		newDiskMap := make(map[string]bool)
		oldDiskMap := make(map[string]bool)

		for _, disk := range newDisks {
			if disk != "" {
				newDiskMap[disk] = true
			}
		}
		for _, disk := range oldDisks {
			if disk != "" {
				oldDiskMap[disk] = true
			}
		}

		// Initialize added and removed slices for this comparison
		added := []string{}
		removed := []string{}

		for _, disk := range newDisks {
			if disk != "" && !oldDiskMap[disk] {
				added = append(added, disk)
			}
		}
		for _, disk := range oldDisks {
			if disk != "" && !newDiskMap[disk] {
				removed = append(removed, disk)
			}
		}

		return added, removed
	}

	// Flatten the new desired ASM disk lists
	desiredAsmDisks := flattenDisksBySize(&oracleRestart.Spec)
	oldAsmDisks := flattenDisksBySize(oldSpec)

	// Additional device lists
	newCrsAsmDisks := strings.Split(oracleRestart.Spec.ConfigParams.CrsAsmDeviceList, ",")
	oldCrsAsmDisks := strings.Split(oldSpec.ConfigParams.CrsAsmDeviceList, ",")

	newDbAsmDisks := strings.Split(oracleRestart.Spec.ConfigParams.DbAsmDeviceList, ",")
	oldDbAsmDisks := strings.Split(oldSpec.ConfigParams.DbAsmDeviceList, ",")

	newRecoAsmDisks := strings.Split(oracleRestart.Spec.ConfigParams.RecoAsmDeviceList, ",")
	oldRecoAsmDisks := strings.Split(oldSpec.ConfigParams.RecoAsmDeviceList, ",")

	newRedoAsmDisks := strings.Split(oracleRestart.Spec.ConfigParams.RedoAsmDeviceList, ",")
	oldRedoAsmDisks := strings.Split(oldSpec.ConfigParams.RedoAsmDeviceList, ",")

	// Track unique added and removed disks
	addedDiskSet := make(map[string]bool)
	removedDiskSet := make(map[string]bool)

	// Compare ASM and other device lists
	for _, diskLists := range [][2][]string{
		{desiredAsmDisks, oldAsmDisks},
		{newCrsAsmDisks, oldCrsAsmDisks},
		{newDbAsmDisks, oldDbAsmDisks},
		{newRecoAsmDisks, oldRecoAsmDisks},
		{newRedoAsmDisks, oldRedoAsmDisks},
	} {
		added, removed := compareDisks(diskLists[0], diskLists[1])
		for _, disk := range added {
			addedDiskSet[disk] = true
		}
		for _, disk := range removed {
			removedDiskSet[disk] = true
		}
	}

	// Convert sets back to slices
	for disk := range addedDiskSet {
		addedAsmDisks = append(addedAsmDisks, disk)
	}
	for disk := range removedDiskSet {
		removedAsmDisks = append(removedAsmDisks, disk)
	}

	// Return the final list of added and removed disks
	return addedAsmDisks, removedAsmDisks
}

// Function to check if a disk is part of a device list
func isDiskInDeviceList(disk string, deviceList string) bool {
	devices := strings.Split(deviceList, ",")
	for _, device := range devices {
		if strings.TrimSpace(device) == disk {
			return true
		}
	}
	return false
}

// #############################################################################
//
//	Manage Finalizer to cleanup before deletion of OracleRestart
//
// #############################################################################

// manageOracleRestartDeletion manages the deletion of the OracleRestart resource
func (r *OracleRestartReconciler) manageOracleRestartDeletion(req ctrl.Request, ctx context.Context, oracleRestart *oraclerestartdb.OracleRestart) error {
	log := r.Log.WithValues("manageOracleRestartDeletion", req.NamespacedName)

	// Check if the OracleRestart instance is marked to be deleted
	isOracleRestartMarkedToBeDeleted := oracleRestart.GetDeletionTimestamp() != nil
	if isOracleRestartMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(oracleRestart, oracleRestartFinalizer) {
			// Run finalization logic
			err := r.cleanupOracleRestart(req, oracleRestart)
			if err != nil {
				return err
			}

			// Remove finalizer and update the resource
			if err := r.patchFinalizer(ctx, oracleRestart, false); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return err
			}
			log.Info("Successfully removed OracleRestart finalizer")
		}
		return errors.New("deletion pending")
	}

	// Add finalizer for this CR if not present
	if !controllerutil.ContainsFinalizer(oracleRestart, oracleRestartFinalizer) {
		if err := r.patchFinalizer(ctx, oracleRestart, true); err != nil {
			log.Error(err, "Failed to add finalizer")
			return err
		}
	}
	return nil
}

// patchFinalizer updates the finalizer for the given resource
func (r *OracleRestartReconciler) patchFinalizer(ctx context.Context, cr *oraclerestartdb.OracleRestart, add bool) error {
	var finalizers []string
	if add {
		finalizers = append(cr.GetFinalizers(), oracleRestartFinalizer)
	} else {
		for _, finalizer := range cr.GetFinalizers() {
			if finalizer != oracleRestartFinalizer {
				finalizers = append(finalizers, finalizer)
			}
		}
	}

	// Prepare patch payload
	patchData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": finalizers,
		},
	}
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return err
	}

	patch := client.RawPatch(types.MergePatchType, patchBytes)
	return r.Client.Patch(ctx, cr, patch, &client.PatchOptions{
		FieldManager: "rac-database-finalizer-manager",
	})
}

// #############################################################################
//
//	Finalization logic for OracleRestartFinalizer
//
// #############################################################################
func (r *OracleRestartReconciler) cleanupOracleRestart(req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart) error {
	log := r.Log.WithValues("cleanupOracleRestart", req.NamespacedName)
	// Cleanup steps that the operator needs to do before the CR can be deleted.

	sfSetFound := &appsv1.StatefulSet{}

	var err error

	oraRestartSpex := oracleRestart.Spec.InstDetails
	sfSetFound, err = oraclerestartcommon.CheckSfset(oraRestartSpex.Name, oracleRestart, r.Client)
	if err == nil {
		log.Info("Deleting ORestart Statefulset " + sfSetFound.Name)
		if err := r.Client.Delete(context.Background(), sfSetFound); err != nil {
			return err
		}
	}

	cmName := oraRestartSpex.Name + oracleRestart.Name + "-cmap"
	configMapFound, err := oraclerestartcommon.CheckConfigMap(oracleRestart, cmName, r.Client)
	if err == nil {
		log.Info("Deleting Oracle Restart Configmap " + configMapFound.Name)
		if err := r.Client.Delete(context.Background(), configMapFound); err != nil {
			return err
		}
	}

	if err := oraclerestartcommon.DelRestartSwPvc(oracleRestart, oraRestartSpex, r.Client, r.Log); err != nil {
		return err
	}

	// // Deleting the DaemonSet
	daemonSetName := "disk-check-daemonset"
	daemonSet := &appsv1.DaemonSet{}

	// Attempt to get the DaemonSet
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      daemonSetName,
		Namespace: oracleRestart.Namespace,
	}, daemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("DaemonSet not found, skipping deletion", "DaemonSet.Name", daemonSetName)
		} else {
			r.Log.Error(err, "Failed to get DaemonSet", "DaemonSet.Name", daemonSetName)
			return err
		}
	} else {
		// DaemonSet exists, attempt to delete it
		// r.Log.Info("Deleting DaemonSet", "DaemonSet.Name", daemonSetName)
		err = r.Client.Delete(context.TODO(), daemonSet)
		if err != nil {
			r.Log.Error(err, "Failed to delete DaemonSet", "DaemonSet.Name", daemonSetName)
			return err
		}
	}

	if oracleRestart.Spec.AsmStorageDetails != nil {
		// Delete PVCs for each disk in DisksBySize
		for pindex, diskBySize := range oracleRestart.Spec.AsmStorageDetails.DisksBySize {
			for cindex, disk := range diskBySize.DiskNames {
				err = oraclerestartcommon.DelORestartPVC(oracleRestart, pindex, cindex, disk, oracleRestart.Spec.AsmStorageDetails, r.Client, r.Log)
				if err != nil {
					return err
				}
			}
		}
	}

	if oraclerestartcommon.IsStaticProvisioning(r.Client, oracleRestart) {
		if oracleRestart.Spec.AsmStorageDetails != nil {
			// Delete PVs for each disk in DisksBySize
			for pindex, diskBySize := range oracleRestart.Spec.AsmStorageDetails.DisksBySize {
				for cindex, disk := range diskBySize.DiskNames {
					err = oraclerestartcommon.DelORestartPv(oracleRestart, pindex, cindex, disk, oracleRestart.Spec.AsmStorageDetails, r.Client, r.Log)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	svcTypes := []string{"local", "lbservice", "nodeport"}
	for _, svcType := range svcTypes {
		svcFound, err := oraclerestartcommon.CheckORestartSvc(oracleRestart, svcType, oraRestartSpex, "", r.Client)
		if err == nil {
			log.Info("Deleting ORestart Service " + svcFound.Name)
			if err := r.Client.Delete(context.Background(), svcFound); err != nil {
				return err
			}
		}
	}

	log.Info("Successfully cleaned up OracleRestart")
	return nil
}

// #############################################################################
//
//	CLeanup RAC Instance
//
// #############################################################################
func (r *OracleRestartReconciler) cleanupOracleRestartInstance(req ctrl.Request, ctx context.Context, oracleRestart *oraclerestartdb.OracleRestart) (int32, error) {
	log := r.Log.WithValues("cleanupOracleRestartInstance", req.NamespacedName)
	// Cleanup steps that the operator needs to do before the CR can be deleted.

	var i int32
	var err error

	OraRestartSpex := oracleRestart.Spec.InstDetails
	if utils.CheckStatusFlag(OraRestartSpex.IsDelete) {
		if len(oracleRestart.Status.OracleRestartNodes) > 0 {
			for _, oraRacSatus := range oracleRestart.Status.OracleRestartNodes {
				if strings.ToUpper(oraRacSatus.Name) == (strings.ToUpper(OraRestartSpex.Name) + "-0") {
					if !utils.CheckStatusFlag(oraRacSatus.NodeDetails.IsDelete) {
						oraRacSatus.NodeDetails.IsDelete = "true"
						log.Info("Setting Oracle Restart status instance " + oraRacSatus.Name + " delete flag true")
						err = r.deleteOracleRestartInst(OraRestartSpex, req, ctx, oracleRestart)
						oraRacSatus.NodeDetails.IsDelete = "false"
						if err != nil {
							log.Info("Error occurred Oracle Restart instance " + oraRacSatus.Name + " deletion")
							return 0, err // return value should be adjusted according to the function signature
						}
					}
				}
			}
		}
	}

	return i, nil
}

func (r *OracleRestartReconciler) deleteOracleRestartInst(OraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, req ctrl.Request, ctx context.Context, oracleRestart *oraclerestartdb.OracleRestart) error {
	log := r.Log.WithValues("cleanupOracleRestartInstance", req.NamespacedName)
	// delete steps that the operator needs to do before the CR can be deleted.

	//var i int32
	var err error
	var cmName string

	//nodeCount, err = oraclerestartcommon.GetHealthyNodeCounts(oracleRestart)
	//healthyNode, err = oraclerestartcommon.GetHealthyNode(oracleRestart)
	if err != nil {
		return fmt.Errorf("no healthy node found in the cluster to perform delete node operator. manual intervention required")
	}

	// var endp string = ""
	// _, endp, err = oraclerestartcommon.GetDBLsnrEndPoints(oracleRestart)
	// if err != nil {
	// 	return fmt.Errorf("endpoint generation error in delete block")
	// }

	sfSetFound := &appsv1.StatefulSet{}
	svcFound := &corev1.Service{}
	configMapFound := &corev1.ConfigMap{}

	sfSetFound, err = oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
	if err == nil {
		// See if StatefulSets already exists and create if it doesn't
		if strings.ToLower(OraRestartSpex.IsDelete) != "force" {
			err = oraclerestartcommon.DelOracleRestartNode(sfSetFound.Name+"-0", oracleRestart, r.kubeClient, r.kubeConfig, r.Log)
			if err != nil {
				return err
			}
		}
		err = r.Client.Delete(context.Background(), sfSetFound)
		if err != nil {
			return err
		}
	}
	if !utils.CheckStatusFlag(OraRestartSpex.IsKeepPVC) {
		err = oraclerestartcommon.DelRestartSwPvc(oracleRestart, OraRestartSpex, r.Client, r.Log)
		if err != nil {
			return err
		}
	}

	//cmName = oracleRestart.Spec.InstDetails[i].Name + oracleRestart.Name + "-cmap"
	cmName = OraRestartSpex.Name + oracleRestart.Name + "-cmap"
	configMapFound, err = oraclerestartcommon.CheckConfigMap(oracleRestart, cmName, r.Client)
	if err == nil {

		err = r.Client.Delete(context.Background(), configMapFound)
		if err != nil {
			return err
		}
	}

	svcFound, err = oraclerestartcommon.CheckORestartSvc(oracleRestart, "local", OraRestartSpex, "", r.Client)
	if err == nil {
		// See if service already exists and create if it doesn't
		err = r.Client.Delete(context.Background(), svcFound)
		if err != nil {
			return err
		}
	}

	//NodePort Service
	if len(oracleRestart.Spec.NodePortSvc.PortMappings) != 0 {
		svcFound, err = oraclerestartcommon.CheckORestartSvc(oracleRestart, "nodeport", OraRestartSpex, "", r.Client)
		if err == nil {
			// See if service already exists and create if it doesn't
			err = r.Client.Delete(context.Background(), svcFound)
			if err != nil {
				return err
			}
		}
	}

	//NodePort Service
	if len(oracleRestart.Spec.LbService.PortMappings) != 0 {
		svcFound, err = oraclerestartcommon.CheckORestartSvc(oracleRestart, "lbservice", OraRestartSpex, "", r.Client)
		if err == nil {
			// See if service already exists and create if it doesn't
			err = r.Client.Delete(context.Background(), svcFound)
			if err != nil {
				return err
			}
		}
	}

	log.Info("Successfully cleaned up OracleRestartInstance")
	return nil
}

func IsStaticProvisioningUsed(ctx context.Context, c client.Client, storageClassName string) bool {
	if storageClassName != "" {
		return false
	}

	var scList storagev1.StorageClassList
	err := c.List(ctx, &scList)
	if err != nil {
		// Can't determine SCs — safest to assume static provisioning
		return true
	}

	for _, sc := range scList.Items {
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return false
		}
	}

	// No default SC found → static provisioning is expected
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *OracleRestartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oraclerestartdb.OracleRestart{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		Owns(&appsv1.StatefulSet{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

const oldSpecAnnotation = "OracleRestarts.database.oracle.com/old-spec"

// GetOldSpec retrieves the old spec from annotations.
// Returns nil, nil if the annotation does not exist.
func (r *OracleRestartReconciler) GetOldSpec(oracleRestart *oraclerestartdb.OracleRestart) (*oraclerestartdb.OracleRestartSpec, error) {
	// Check if annotations exist
	annotations := oracleRestart.GetAnnotations()
	if annotations == nil {
		r.Log.Info("No annotations found on OracleRestart")
		return nil, nil
	}

	// Retrieve the specific annotation
	val, ok := annotations[oldSpecAnnotation]
	if !ok {
		r.Log.Info("Old spec annotation not found")
		return nil, nil
	}

	// Unmarshal the old spec JSON string
	specBytes := []byte(val)
	var oldSpec oraclerestartdb.OracleRestartSpec
	if err := json.Unmarshal(specBytes, &oldSpec); err != nil {
		r.Log.Error(err, "Failed to unmarshal old spec from annotation")
		return nil, fmt.Errorf("failed to unmarshal old spec from annotation: %w", err)
	}

	// r.Log.Info("Successfully retrieved old spec from annotation", "spec", oldSpec)
	return &oldSpec, nil
}

// SetCurrentSpec stores the current spec as an annotation with retry logic, updating only if the annotation value has changed.
func (r *OracleRestartReconciler) SetCurrentSpec(ctx context.Context, oracleRestart *oraclerestartdb.OracleRestart, req ctrl.Request) error {
	// Marshal the current spec into JSON
	currentSpecData, err := json.Marshal(oracleRestart.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal current spec: %w", err)
	}
	currentSpecStr := string(currentSpecData)

	// Ensure Annotations map is initialized
	if oracleRestart.Annotations == nil {
		oracleRestart.Annotations = make(map[string]string)
	}

	// Check if the annotation value has changed
	existingSpecStr, exists := oracleRestart.Annotations[oldSpecAnnotation]
	if exists && existingSpecStr == currentSpecStr {
		r.Log.Info("Annotations are already up to date. Skipping update.")
		return nil // No update needed
	}

	// Update the annotation with the new spec
	oracleRestart.Annotations[oldSpecAnnotation] = currentSpecStr

	// // Create a patch to update only the annotations
	patchData, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": oracleRestart.Annotations,
		},
	})
	if err != nil {
		r.Log.Error(err, "Failed to marshal annotation patch data")
		return err
	}

	// Apply the patch
	err = r.Patch(ctx, oracleRestart, client.RawPatch(types.MergePatchType, patchData))
	if err != nil {
		if apierrors.IsConflict(err) {
			r.Log.Info("Conflict detected while updating annotations, retrying...")
			return fmt.Errorf("conflict occurred while updating annotations: %w", err)
		}
		r.Log.Error(err, "Failed to update Oracle Restart instance annotations")
		return fmt.Errorf("failed to update Oracle Restart  instance annotations: %w", err)
	}

	r.Log.Info("Oracle Restart Object annotations updated with current spec annotation")
	return nil
}

func (r *OracleRestartReconciler) updateONS(ctx context.Context, podList *corev1.PodList, instance *oraclerestartdb.OracleRestart, onsState string) error {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	for _, pod := range podList.Items {
		podName := pod.Name

		cmd := fmt.Sprintf("python3 /opt/scripts/startup/scripts/main.py --ons=%s", onsState)
		// reqLogger.Info("Executing command to update ONS", "Pod.Name", podName, "Command", cmd)

		stdout, stderr, err := oraclerestartcommon.ExecCommand(
			podName,
			[]string{"bash", "-c", cmd},
			r.kubeClient,
			r.kubeConfig,
			instance,
			reqLogger,
		)
		if err != nil {
			instance.Spec.IsFailed = true
			reqLogger.Error(err, "Failed to execute command", "Pod.Name", podName, "Command", cmd, "Stdout", stdout, "Stderr", stderr)
			return err
		}
		r.Log.Info("ONS Running successfully", "podName", podName)
	}

	return nil
}
func (r *OracleRestartReconciler) expandStorageClassSWVolume(ctx context.Context, instance *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec) error {
	//reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	if oldSpec != nil {
		// fmt.Printf("Received OldSpec", oldSpec.InstDetails.SwLocStorageSizeInGb)
		if instance.Spec.InstDetails.SwLocStorageSizeInGb > oldSpec.InstDetails.SwLocStorageSizeInGb {
			fmt.Printf("Inside OldSpec and newSpec Change", oldSpec.InstDetails.SwLocStorageSizeInGb, instance.Spec.InstDetails.SwLocStorageSizeInGb)
			storageClass := &storagev1.StorageClass{}
			pvc := &corev1.PersistentVolumeClaim{}

			if instance.Spec.SwStorageClass != "" {

				err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.SwStorageClass}, storageClass)
				if err != nil {
					return fmt.Errorf("error while fetching the storage class")
				}

				pvcName := oraclerestartcommon.GetSwPvcName(instance.Spec.InstDetails.Name) + "-" + instance.Spec.InstDetails.Name + "-0"
				err = r.Get(ctx, types.NamespacedName{
					Name:      pvcName,
					Namespace: instance.Namespace,
				}, pvc)

				// fmt.Printf("PvcName set to ", pvc.Name)

				if err == nil {
					if storageClass.AllowVolumeExpansion == nil || !*storageClass.AllowVolumeExpansion {
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, "PVC not resizable", "The storage class doesn't support volume expansion")
						return fmt.Errorf("the storage class %s doesn't support volume expansion", instance.Spec.SwStorageClass)
					}

					newPVCSize := resource.MustParse(strconv.Itoa(instance.Spec.InstDetails.SwLocStorageSizeInGb) + "Gi")
					newPVCSizeAdd := &newPVCSize

					fmt.Printf("New PvcSize set to ", newPVCSizeAdd)
					if newPVCSizeAdd.Cmp(pvc.Spec.Resources.Requests["storage"]) < 0 {
						return fmt.Errorf("Resizing PVC to lower size volume not allowed")
					}

					pvc.Spec.Resources.Requests["storage"] = resource.MustParse(strconv.Itoa(instance.Spec.InstDetails.SwLocStorageSizeInGb) + "Gi")
					fmt.Printf("Updating PVC", "pvc", pvc.Name, "volume", pvc.Spec.VolumeName)
					err = r.Update(ctx, pvc)
					if err != nil {
						return fmt.Errorf("error while updating the PVCs")
					}

				}
			}
		}
	}
	return nil
}
