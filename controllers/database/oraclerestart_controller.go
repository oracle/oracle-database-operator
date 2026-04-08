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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	v4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedasm "github.com/oracle/oracle-database-operator/commons/crs/asm"
	oraclerestartcommon "github.com/oracle/oracle-database-operator/commons/crs/restart"
	utils "github.com/oracle/oracle-database-operator/commons/crs/restart/utils"
	shareddiskcheck "github.com/oracle/oracle-database-operator/commons/crs/shared/diskcheck"
	sharedenvfile "github.com/oracle/oracle-database-operator/commons/crs/shared/envfile"
	sharedorautil "github.com/oracle/oracle-database-operator/commons/crs/shared/orautil"
	sharedspecguard "github.com/oracle/oracle-database-operator/commons/crs/shared/specguard"
	sharedstatusmerge "github.com/oracle/oracle-database-operator/commons/crs/shared/statusmerge"
	sharedk8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
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
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

func (r *OracleRestartReconciler) phaseLogger(req ctrl.Request, phase oracleRestartReconcilePhase) logr.Logger {
	return r.Log.WithValues("oraclerestart", req.NamespacedName, "phase", string(phase))
}

func (r *OracleRestartReconciler) phaseInfo(req ctrl.Request, phase oracleRestartReconcilePhase, msg string, keysAndValues ...interface{}) {
	r.phaseLogger(req, phase).Info(msg, keysAndValues...)
}

func (r *OracleRestartReconciler) phaseError(req ctrl.Request, phase oracleRestartReconcilePhase, err error, msg string, keysAndValues ...interface{}) {
	r.phaseLogger(req, phase).Error(err, msg, keysAndValues...)
}

const oracleRestartFinalizer = "database.oracle.com/oraclerestartfinalizer"

const (
	oracleRestartLockHolderAnnotation     = "database.oracle.com/oraclerestart-lock-holder"
	oracleRestartLockPhaseAnnotation      = "database.oracle.com/oraclerestart-lock-phase"
	oracleRestartLockAcquiredAtAnnotation = "database.oracle.com/oraclerestart-lock-acquired-at"
	oracleRestartLockGenerationAnnotation = "database.oracle.com/oraclerestart-lock-generation"
	oracleRestartBreakGlassAnnotation     = "database.oracle.com/breakglass-override"
	oracleRestartBreakGlassReason         = "database.oracle.com/breakglass-reason" // optional, audit context
	oracleRestartBreakGlassActor          = "database.oracle.com/breakglass-actor"  // optional, audit context
)

type oracleRestartReconcilePhase string

const (
	orPhaseInitAndFetch       oracleRestartReconcilePhase = "InitAndFetch"
	orPhaseDeletionAndGuards  oracleRestartReconcilePhase = "DeletionAndGuards"
	orPhaseValidationDefaults oracleRestartReconcilePhase = "ValidationAndDefaults"
	orPhaseServiceSync        oracleRestartReconcilePhase = "ServiceSync"
	orPhaseStorageSync        oracleRestartReconcilePhase = "StorageSync"
	orPhaseWorkloadSync       oracleRestartReconcilePhase = "WorkloadSync"
	orPhaseFinalize           oracleRestartReconcilePhase = "Finalize"
)

//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="database.oracle.com",resources=oraclerestarts/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;secrets;endpoints;services;events;configmaps;persistentvolumes;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the reconciliation loop for OracleRestart resources.
// It manages the lifecycle of Oracle Restart deployments on Kubernetes, including:
// - Resource retrieval and validation
// - Spec and status initialization
// - Service creation and management (local, nodeport, and load balancer services)
// - ASM (Automatic Storage Management) disk discovery and validation
// - PV/PVC creation for ASM disk groups
// - Configuration map generation and updates
// - StatefulSet creation and updates for new setups and disk changes
// - Pod readiness validation
// - ONS (Oracle Notification Service) configuration
// - Resource cleanup and deletion handling
//
// The reconciler detects various scenarios:
// - New setup: Initial deployment with no existing disk groups
// - Upgrade scenario: Migration from old ASM storage configuration
// - Disk changes: Addition or removal of ASM disks (cannot process both together)
// - Missing disk sizes: Triggers disk discovery via daemonset
//
// Returns:
// - ctrl.Result: Requeue policy (with or without delay)
// - error: Any error encountered during reconciliation
//
// The method defers status updates and handles cleanup of temporary resources (daemonsets)
// upon completion or error conditions.
func (r *OracleRestartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	//ctx := context.Background()
	r.phaseInfo(req, orPhaseInitAndFetch, "Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false
	lockAcquired := false
	// var svcType string
	var nilErr error = nil

	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}

	oracleRestart := &oraclerestartdb.OracleRestart{}
	// time.Sleep(50000 * time.Second)
	// Execute for every reconcile
	defer r.updateReconcileStatus(oracleRestart, ctx, req, &result, &err, &blocked, &completed)
	defer func() {
		if lockAcquired {
			if lErr := r.releaseOracleRestartReconcileLock(ctx, req); lErr != nil {
				r.phaseError(req, orPhaseFinalize, lErr, "Failed to release reconcile lock")
			}
		}
	}()

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
			if err := r.Status().Update(ctx, oracleRestart); err != nil {
				return ctrl.Result{}, err
			}
		}

	// Kube Client Config Setup
	if r.kubeConfig == nil && r.kubeClient == nil {
		r.kubeConfig, r.kubeClient, err = oraclerestartcommon.GetRacK8sClientConfig(r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Manage OracleRestart Deletion , if delete topology is called
	r.phaseInfo(req, orPhaseDeletionAndGuards, "Running deletion and guard checks")
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

	webhooksEnabled := os.Getenv("ENABLE_WEBHOOKS") != "false"

	if webhooksEnabled {
		err = checkOracleRestartState(oracleRestart)
		if err != nil {
			result = resultQ
			r.Log.Info("Oracle Restart object is in restricted state, returning back")
			return result, nilErr
		}
	}
	lockAcquired, err = r.acquireOracleRestartReconcileLock(ctx, req, oracleRestart, oldSpec, orPhaseValidationDefaults)
	if err != nil {
		blocked = true
		r.phaseInfo(req, orPhaseDeletionAndGuards, "Reconcile lock held by another cycle; requeueing", "error", err.Error())
		return resultQ, nil
	}

	result, completed, err = r.runOracleRestartProvisionPhases(ctx, req, oracleRestart, oldSpec, webhooksEnabled)
	return result, err
}

func (r *OracleRestartReconciler) runOracleRestartProvisionPhases(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	webhooksEnabled bool,
) (ctrl.Result, bool, error) {
	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}

	cName, fName, earlyResult, earlyErr, earlyExit := r.oracleRestartPhaseValidationAndDefaults(
		ctx, req, oracleRestart, oldSpec, webhooksEnabled, resultQ,
	)
	if earlyExit {
		return earlyResult, false, earlyErr
	}

	earlyResult, earlyErr, earlyExit = r.oracleRestartPhaseServiceSync(ctx, req, oracleRestart, resultNq)
	if earlyExit {
		return earlyResult, false, earlyErr
	}

	storageState, earlyResult, earlyErr, earlyExit := r.oracleRestartPhaseStorageSync(
		ctx, req, oracleRestart, oldSpec, cName, fName, resultNq, resultQ,
	)
	if earlyExit {
		return earlyResult, false, earlyErr
	}

	earlyResult, earlyErr, earlyExit = r.oracleRestartPhaseWorkloadSync(
		ctx, req, oracleRestart, oldSpec, storageState, resultNq, resultQ,
	)
	if earlyExit {
		return earlyResult, false, earlyErr
	}

	return r.oracleRestartPhaseFinalize(ctx, req, oracleRestart, resultQ)
}

type oracleRestartStoragePhaseState struct {
	isNewSetup          bool
	isDiskChanged       bool
	discoverySuccessful bool
	addedAsmDisks       []string
	removedAsmDisks     []string
	configMapData       map[string]string
}

func (r *OracleRestartReconciler) oracleRestartPhaseValidationAndDefaults(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	webhooksEnabled bool,
	resultQ ctrl.Result,
) (string, string, ctrl.Result, error, bool) {
	var cName, fName string
	var nilErr error = nil

	r.phaseInfo(req, orPhaseValidationDefaults, "Applying defaults and validation")
	if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" {
		cName = oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName
	}
	if oracleRestart.Spec.ConfigParams.GridResponseFile.Name != "" {
		fName = oracleRestart.Spec.ConfigParams.GridResponseFile.Name
	}
	if err := setRacDgFromStatusAndSpecWithMinimumDefaults(oracleRestart, r.Client, cName, fName); err != nil {
		return cName, fName, ctrl.Result{}, err, true
	}
	if webhooksEnabled {
		if err := checkOracleRestartState(oracleRestart); err != nil {
			return cName, fName, resultQ, nilErr, true
		}
	}
	if err := r.validateSpex(oracleRestart, oldSpec, ctx, req); err != nil {
		return cName, fName, resultQ, nilErr, true
	}
	if err := r.setDefaults(oracleRestart); err != nil {
		return cName, fName, resultQ, nilErr, true
	}
	if err := r.updateGiConfigParamStatus(oracleRestart); err != nil {
		return cName, fName, resultQ, nilErr, true
	}
	if err := r.updateDbConfigParamStatus(oracleRestart); err != nil {
		return cName, fName, resultQ, nilErr, true
	}
	return cName, fName, ctrl.Result{}, nil, false
}

func (r *OracleRestartReconciler) oracleRestartPhaseServiceSync(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	resultNq ctrl.Result,
) (ctrl.Result, error, bool) {
	r.phaseInfo(req, orPhaseServiceSync, "Reconciling services")
	var svcType string
	if _, err := r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, "local")); err != nil {
		return resultNq, err, true
	}
	if len(oracleRestart.Spec.NodePortSvc.PortMappings) != 0 {
		if _, err := r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildExternalServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, svcType, "nodeport")); err != nil {
			return resultNq, err, true
		}
	}
	if len(oracleRestart.Spec.LbService.PortMappings) != 0 {
		if _, err := r.createOrReplaceService(ctx, oracleRestart, oraclerestartcommon.BuildExternalServiceDefForOracleRestart(oracleRestart, 0, oracleRestart.Spec.InstDetails, svcType, "lbservice")); err != nil {
			return resultNq, err, true
		}
	}
	return ctrl.Result{}, nil, false
}

func (r *OracleRestartReconciler) oracleRestartPhaseStorageSync(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	cName string,
	fName string,
	resultNq ctrl.Result,
	resultQ ctrl.Result,
) (oracleRestartStoragePhaseState, ctrl.Result, error, bool) {
	var nilErr error = nil
	state := oracleRestartStoragePhaseState{configMapData: make(map[string]string)}

	r.ensureAsmStorageStatus(ctx, oracleRestart, req)
	r.phaseInfo(req, orPhaseStorageSync, "Reconciling ASM storage state")
	isNewSetup := true
	upgradeSetup := false
	if oldSpec != nil && oldSpec.AsmStorageDetailsOld != nil {
		upgradeSetup = true
		isNewSetup = false
	} else {
		for _, diskgroup := range oracleRestart.Status.AsmDiskGroups {
			if len(diskgroup.Disks) > 0 && diskgroup.Name != "Pending" {
				isNewSetup = false
				break
			}
		}
	}
	state.isNewSetup = isNewSetup

	addedAsmDisks := []string{}
	removedAsmDisks := []string{}
	isDiskChanged := false
	if !isNewSetup && oldSpec != nil {
		var err error
		addedAsmDisks, removedAsmDisks, err = r.computeDiskChanges(oracleRestart, oldSpec)
		if err != nil {
			return state, ctrl.Result{}, err, true
		}
		if len(addedAsmDisks) > 0 && len(removedAsmDisks) > 0 {
			return state, resultQ, fmt.Errorf("cannot add and remove ASM disks in the same step"), true
		}
		if len(addedAsmDisks) > 0 || len(removedAsmDisks) > 0 {
			isDiskChanged = true
		}
		oldMap := make(map[string]v4.AsmDiskGroupDetails)
		for _, dg := range oldSpec.AsmStorageDetails {
			oldMap[dgKey(dg.Name, dg.Type)] = dg
		}
		for _, newDG := range oracleRestart.Spec.AsmStorageDetails {
			if len(normalizeDisks(newDG.Disks)) == 0 {
				continue
			}
			key := dgKey(newDG.Name, newDG.Type)
			oldDG, exists := oldMap[key]
			if !exists || len(normalizeDisks(oldDG.Disks)) == 0 {
				continue
			}
			if !strings.EqualFold(oldDG.AutoUpdate, newDG.AutoUpdate) {
				isDiskChanged = true
				break
			}
		}
	}
	state.isDiskChanged = isDiskChanged
	state.addedAsmDisks = addedAsmDisks
	state.removedAsmDisks = removedAsmDisks

	missingSize := false
	for _, dg := range oracleRestart.Status.AsmDiskGroups {
		for _, disk := range dg.Disks {
			if disk.SizeInGb == 0 {
				missingSize = true
				break
			}
		}
		if missingSize {
			break
		}
	}
	discoverySuccessful := false
	shouldRunDiscovery := oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" &&
		(len(removedAsmDisks) == 0) && (isNewSetup || upgradeSetup || missingSize || len(addedAsmDisks) > 0 || len(oracleRestart.Status.AsmDiskGroups) == 0)
	if shouldRunDiscovery {
		if err := r.createDaemonSet(oracleRestart, ctx); err != nil {
			return state, ctrl.Result{}, err, true
		}
		ready, err := checkRacDaemonSetStatus(ctx, r, oracleRestart)
		if err != nil || !ready {
			return state, ctrl.Result{RequeueAfter: 10 * time.Second}, nil, true
		}
		if err := r.updateDiskSizes(ctx, oracleRestart); err == nil && len(oracleRestart.Status.AsmDiskGroups) > 0 {
			discoverySuccessful = true
		}
	}
	state.discoverySuccessful = discoverySuccessful

	if len(oracleRestart.Status.AsmDiskGroups) == 0 && oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" {
		return state, resultNq, fmt.Errorf("no ASM disk group status available"), true
	}
	if err := setRacDgFromStatusAndSpecWithMinimumDefaults(oracleRestart, r.Client, cName, fName); err != nil {
		return state, ctrl.Result{}, err, true
	}

	for dgIndex, dgSpec := range oracleRestart.Spec.AsmStorageDetails {
		groupName := dgSpec.Name
		dgType := dgSpec.Type
		var dgStatus *oraclerestartdb.AsmDiskGroupStatus
		for i, dgSt := range oracleRestart.Status.AsmDiskGroups {
			if dgSt.Name == groupName {
				dgStatus = &oracleRestart.Status.AsmDiskGroups[i]
				break
			}
		}
		if dgStatus == nil && oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" {
			continue
		}
		isStatic := oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC"
		for diskIdx, diskName := range dgSpec.Disks {
			var diskStatus *oraclerestartdb.AsmDiskStatus
			if isStatic {
				for i, d := range dgStatus.Disks {
					if d.Name == diskName {
						diskStatus = &dgStatus.Disks[i]
						break
					}
				}
				if diskStatus == nil || diskStatus.SizeInGb == 0 || !diskStatus.Valid {
					continue
				}
			}
			sizeStr := fmt.Sprintf("%dGi", oracleRestart.Spec.AsmStorageSizeInGb)
			if isStatic {
				sizeStr = fmt.Sprintf("%dGi", diskStatus.SizeInGb)
			}
			if isStatic {
				pvVolume := oraclerestartcommon.VolumePVForASM(oracleRestart, dgIndex, diskIdx, diskName, groupName, sizeStr, r.Client)
				if _, _, err := r.createOrReplaceAsmPv(ctx, oracleRestart, pvVolume, string(dgType)); err != nil {
					return state, resultNq, err, true
				}
			}
			pvcVolume := oraclerestartcommon.VolumePVCForASM(oracleRestart, dgIndex, diskIdx, diskName, groupName, sizeStr, string(dgType), r.Client)
			if _, _, err := r.createOrReplaceAsmPvC(ctx, oracleRestart, pvcVolume, string(dgType)); err != nil {
				return state, resultNq, err, true
			}
		}
	}

	if oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" {
		if err := r.cleanupDaemonSet(oracleRestart, ctx); err != nil {
			return state, resultQ, nilErr, true
		}
	}

	if oracleRestart.Spec.ConfigParams != nil {
		configMapData, err := r.generateConfigMap(oracleRestart)
		if err != nil {
			return state, resultNq, err, true
		}
		state.configMapData = configMapData
	}

	return state, ctrl.Result{}, nil, false
}

func (r *OracleRestartReconciler) oracleRestartPhaseWorkloadSync(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	storageState oracleRestartStoragePhaseState,
	resultNq ctrl.Result,
	resultQ ctrl.Result,
) (ctrl.Result, error, bool) {
	var nilErr error = nil
	index := 0
	isLast := true
	oldState := oracleRestart.Status.State
	r.phaseInfo(req, orPhaseWorkloadSync, "Reconciling workload objects")
	if !utils.CheckStatusFlag(oracleRestart.Spec.InstDetails.IsDelete) {
		switch {
		case !storageState.isDiskChanged:
			cmName := oracleRestart.Spec.InstDetails.Name + oracleRestart.Name + "-cmap"
			cm := oraclerestartcommon.ConfigMapSpecs(oracleRestart, storageState.configMapData, cmName)
			_, configmapEnvKeyChanged, err := r.createConfigMap(ctx, *oracleRestart, cm)
			if err != nil {
				return resultNq, err, true
			}
			if err = oraclerestartcommon.CreateServiceAccountIfNotExists(oracleRestart, r.Client); err != nil {
				return resultNq, err, true
			}
			oracleRestart.Spec.InstDetails.EnvFile = cmName
			dep, err := oraclerestartcommon.BuildStatefulSetForOracleRestart(oracleRestart, oracleRestart.Spec.InstDetails, r.Client)
			if err != nil {
				return resultNq, err, true
			}
			if _, err = r.createOrReplaceSfs(ctx, req, *oracleRestart, dep, index, isLast, oldState, configmapEnvKeyChanged); err != nil {
				return resultNq, err, true
			}
		case storageState.isDiskChanged && !storageState.isNewSetup:
			if oraclerestartcommon.CheckStorageClass(oracleRestart) == "NOSC" && !storageState.discoverySuccessful && len(storageState.removedAsmDisks) == 0 {
				msg := "Any of provided ASM Disks are invalid, pls check disk-check daemon set for logs. Fix the asm disk to the valid one and redeploy."
				if err := r.cleanupDaemonSet(oracleRestart, ctx); err != nil {
					return resultQ, nilErr, true
				}
				addedAsmDisksMap := make(map[string]bool)
				for _, disk := range storageState.addedAsmDisks {
					addedAsmDisksMap[disk] = true
				}
				for pindex, dgSpec := range oracleRestart.Spec.AsmStorageDetails {
					for cindex, diskName := range dgSpec.Disks {
						if _, ok := addedAsmDisksMap[diskName]; ok {
							if err := oraclerestartcommon.DelORestartPVC(oracleRestart, pindex, cindex, diskName, r.Client, r.Log); err != nil {
								return resultQ, err, true
							}
							if err := oraclerestartcommon.DelORestartPv(oracleRestart, pindex, cindex, diskName, r.Client, r.Log); err != nil {
								return resultQ, err, true
							}
						}
					}
				}
				if err := r.SetCurrentSpec(ctx, oracleRestart, req); err != nil {
					return resultQ, err, true
				}
				return resultQ, errors.New(msg), true
			}
			cmName := oracleRestart.Spec.InstDetails.Name + oracleRestart.Name + "-cmap"
			cm := oraclerestartcommon.ConfigMapSpecs(oracleRestart, storageState.configMapData, cmName)
			_, configmapEnvKeyChanged, err := r.createConfigMap(ctx, *oracleRestart, cm)
			if err != nil {
				return resultNq, err, true
			}
			oracleRestart.Spec.InstDetails.EnvFile = cmName
			dep, buildErr := oraclerestartcommon.BuildStatefulSetForOracleRestart(oracleRestart, oracleRestart.Spec.InstDetails, r.Client)
			if buildErr != nil {
				return resultNq, buildErr, true
			}
			if _, err = r.createOrReplaceSfsAsm(ctx, req, oracleRestart, dep, index, isLast, oldSpec, storageState.discoverySuccessful, configmapEnvKeyChanged); err != nil {
				return resultNq, err, true
			}
		}
	}
	if err := r.expandStorageClassSWVolume(ctx, oracleRestart, oldSpec); err != nil {
		return ctrl.Result{}, err, true
	}
	return ctrl.Result{}, nil, false
}

func (r *OracleRestartReconciler) oracleRestartPhaseFinalize(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	resultQ ctrl.Result,
) (ctrl.Result, bool, error) {
	r.phaseInfo(req, orPhaseFinalize, "Finalizing reconcile state")
	if err := r.SetCurrentSpec(ctx, oracleRestart, req); err != nil {
		return resultQ, false, err
	}
	OraRestartSpex := oracleRestart.Spec.InstDetails
	orestartSfSet, err := oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
	if err != nil {
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.StatefulSetNotFound), r.Client, false)
		return ctrl.Result{}, false, err
	}
	podList, err := oraclerestartcommon.GetPodList(orestartSfSet.Name, oracleRestart, r.Client, OraRestartSpex)
	if err != nil {
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.PodNotFound), r.Client, false)
		return ctrl.Result{}, false, err
	}
	isPodExist, _, notReadyPod := oraclerestartcommon.PodListValidation(podList, orestartSfSet.Name, oracleRestart, r.Client)
	if isPodExist && notReadyPod == nil {
		if oracleRestart.Spec.EnableOns == "enable" || oracleRestart.Spec.EnableOns == "disable" {
			orestartSfSet, err := oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
			if err != nil {
				r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.StatefulSetNotFound), r.Client, false)
				return ctrl.Result{}, false, err
			}
			podList, err := oraclerestartcommon.GetPodList(orestartSfSet.Name, oracleRestart, r.Client, oracleRestart.Spec.InstDetails)
			if err != nil {
				return ctrl.Result{}, false, err
			}
			onsOp := "start"
			if oracleRestart.Spec.EnableOns == "disable" {
				onsOp = "stop"
			}
			if err = r.updateONS(ctx, podList, oracleRestart, onsOp); err != nil {
				return ctrl.Result{}, false, err
			}
		}
	}

	r.Log.Info("Reconcile completed. Requeuing....")
	return resultQ, true, nil
}

// normalizeDisks trims whitespace and sorts disk lists for stable comparison
func normalizeDisks(disks []string) []string {
	var cleaned []string

	for _, d := range disks {
		d = strings.TrimSpace(d)
		if d != "" {
			cleaned = append(cleaned, d)
		}
	}

	sort.Strings(cleaned) // IMPORTANT for stable comparison
	return cleaned
}

// dgKey generates a unique key for a disk group based on its name and type, used for mapping old vs new disk groups during change detection.
func dgKey(name string, t v4.AsmDiskDGTypes) string {
	return name + "|" + string(t)
}

// checkRacDaemonSetStatus checks daemonset progress by polling for readiness
// and scanning pod logs. It returns true when the disk-check job completes.
// checkRacDaemonSetStatus verifies the ASM discovery daemonset for Oracle Restart has succeeded before continuing reconciliation.
func checkRacDaemonSetStatus(ctx context.Context, r *OracleRestartReconciler, oracleRestart *oraclerestartdb.OracleRestart) (bool, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	// Initial delay before starting checks
	time.Sleep(10 * time.Second)
	defer tick.Stop()
	// Sleep for 60 seconds
	for {
		select {
		case <-timeout:
			ready, invalidDevice, err := shareddiskcheck.CheckDaemonSetReadyAndDiskValidation(
				ctx, r.Client, r.kubeClient, oracleRestart.Namespace, "disk-check-daemonset",
				shareddiskcheck.LabelSelectorForDaemonSet(oracleRestart, "disk-check"),
			)
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
			if invalidDevice {
				return false, nil
			}

			// DaemonSet did not become ready or running within the timeout
			r.Log.Info("ASM disk-check daemonset still pending readiness; will requeue",
				"namespace", oracleRestart.Namespace,
				"daemonset", "disk-check-daemonset")
			return false, nil

		case <-tick.C:
			ready, invalidDevice, err := shareddiskcheck.CheckDaemonSetReadyAndDiskValidation(
				ctx, r.Client, r.kubeClient, oracleRestart.Namespace, "disk-check-daemonset",
				shareddiskcheck.LabelSelectorForDaemonSet(oracleRestart, "disk-check"),
			)
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
			if invalidDevice {
				return false, nil
			}
		}
	}
}

// computeDiskChanges compares spec and status to determine disks to add/remove
func (r *OracleRestartReconciler) computeDiskChanges(
	instance *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
) (addedAsmDisks []string, removedAsmDisks []string, err error) {

	if oldSpec == nil {
		return nil, nil, nil
	}

	// 1. Compare spec changes
	addedAsmDisks, removedAsmDisks = getRACDisksChangedSpec(*instance, *oldSpec)

	// 2. Include disks to add from status
	if disksToAdd, addErr := getDisksToAddStatus(instance); addErr != nil {
		instance.Spec.IsFailed = true
		return nil, nil, fmt.Errorf("cannot get ASM disks to add: %w", addErr)
	} else if len(disksToAdd) > 0 && len(addedAsmDisks) == 0 {
		addedAsmDisks = disksToAdd
	}

	// 3. Include disks to remove from status
	if disksToRemove, removeErr := getDisksToRemoveStatus(instance); removeErr != nil {
		instance.Spec.IsFailed = true
		return nil, nil, fmt.Errorf("cannot get ASM disks to remove: %w", removeErr)
	} else if len(disksToRemove) > 0 && len(removedAsmDisks) == 0 {
		removedAsmDisks = disksToRemove
	}

	return addedAsmDisks, removedAsmDisks, nil
}

// checkOracleRestartState blocks reconciliation when Oracle Restart enters restricted lifecycle states.
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

func parseOracleRestartBreakGlassOverride(meta metav1.Object) (bool, string, string) {
	annotations := meta.GetAnnotations()
	if len(annotations) == 0 {
		return false, "", ""
	}
	if !strings.EqualFold(strings.TrimSpace(annotations[oracleRestartBreakGlassAnnotation]), "true") {
		return false, "", ""
	}
	reason := strings.TrimSpace(annotations[oracleRestartBreakGlassReason])
	actor := strings.TrimSpace(annotations[oracleRestartBreakGlassActor])
	return true, reason, actor
}

func diffORJSONPaths(prefix string, oldVal interface{}, newVal interface{}, out map[string]struct{}) {
	if reflect.DeepEqual(oldVal, newVal) {
		return
	}
	oldMap, oldMapOK := oldVal.(map[string]interface{})
	newMap, newMapOK := newVal.(map[string]interface{})
	if oldMapOK && newMapOK {
		keys := map[string]struct{}{}
		for k := range oldMap {
			keys[k] = struct{}{}
		}
		for k := range newMap {
			keys[k] = struct{}{}
		}
		for k := range keys {
			diffORJSONPaths(prefix+"."+k, oldMap[k], newMap[k], out)
		}
		return
	}
	_, oldSliceOK := oldVal.([]interface{})
	_, newSliceOK := newVal.([]interface{})
	if oldSliceOK || newSliceOK {
		out[prefix] = struct{}{}
		return
	}
	out[prefix] = struct{}{}
}

func changedOracleRestartSpecPaths(oldSpec *oraclerestartdb.OracleRestartSpec, newSpec oraclerestartdb.OracleRestartSpec) ([]string, error) {
	if oldSpec == nil {
		return nil, nil
	}
	oldBytes, err := json.Marshal(oldSpec)
	if err != nil {
		return nil, err
	}
	newBytes, err := json.Marshal(newSpec)
	if err != nil {
		return nil, err
	}
	var oldObj map[string]interface{}
	var newObj map[string]interface{}
	if err := json.Unmarshal(oldBytes, &oldObj); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(newBytes, &newObj); err != nil {
		return nil, err
	}
	outSet := map[string]struct{}{}
	diffORJSONPaths("spec", oldObj, newObj, outSet)
	out := make([]string, 0, len(outSet))
	for k := range outSet {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func oracleRestartControllerLevelLockBypassAllowedFields() map[string]struct{} {
	// Maintain this allowlist in code when specific field-level lock bypasses are safe.
	// Example:
	// return map[string]struct{}{
	//   "spec.details.someNonDisruptiveField": {},
	// }
	return map[string]struct{}{}
}

func shouldBypassOracleRestartReconcileLockBySpecDelta(latest *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec) (bool, []string, error) {
	if latest == nil || oldSpec == nil {
		return false, nil, nil
	}
	allowed := oracleRestartControllerLevelLockBypassAllowedFields()
	if len(allowed) == 0 {
		return false, nil, nil
	}
	changed, err := changedOracleRestartSpecPaths(oldSpec, latest.Spec)
	if err != nil {
		return false, nil, err
	}
	if len(changed) == 0 {
		return false, nil, nil
	}
	for _, path := range changed {
		if _, ok := allowed[path]; !ok {
			return false, changed, nil
		}
	}
	return true, changed, nil
}

func (r *OracleRestartReconciler) acquireOracleRestartReconcileLock(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	phase oracleRestartReconcilePhase,
) (bool, error) {
	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
		lockTTL    = 20 * time.Minute
	)
	holder := req.NamespacedName.String()
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		latest := &oraclerestartdb.OracleRestart{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		breakGlassEnabled, reason, actor := parseOracleRestartBreakGlassOverride(latest)
		if breakGlassEnabled {
			original := latest.DeepCopy()
			delete(latest.Annotations, oracleRestartLockHolderAnnotation)
			delete(latest.Annotations, oracleRestartLockPhaseAnnotation)
			delete(latest.Annotations, oracleRestartLockAcquiredAtAnnotation)
			delete(latest.Annotations, oracleRestartLockGenerationAnnotation)
			if err := r.Patch(ctx, latest, client.MergeFrom(original)); err != nil {
				if apierrors.IsConflict(err) {
					lastErr = err
					time.Sleep(retryDelay)
					continue
				}
				return false, err
			}
			r.phaseInfo(req, orPhaseDeletionAndGuards, "Break-glass lock override enabled; skipping controller-level reconcile lock",
				"annotation", oracleRestartBreakGlassAnnotation, "reason", reason, "actor", actor)
			return false, nil
		}

		bypassLock, changedPaths, bypassErr := shouldBypassOracleRestartReconcileLockBySpecDelta(latest, oldSpec)
		if bypassErr != nil {
			return false, bypassErr
		}
		if bypassLock {
			r.phaseInfo(req, orPhaseDeletionAndGuards, "Bypassing Oracle Restart reconcile lock based on function-level spec-delta allowlist",
				"changedPaths", strings.Join(changedPaths, ","))
			return false, nil
		}

		existingHolder := latest.Annotations[oracleRestartLockHolderAnnotation]
		existingAt := latest.Annotations[oracleRestartLockAcquiredAtAnnotation]

		lockExpired := false
		if existingAt != "" {
			if ts, err := time.Parse(time.RFC3339Nano, existingAt); err == nil {
				lockExpired = time.Since(ts) > lockTTL
			}
		}
		if existingHolder != "" && existingHolder != holder && !lockExpired {
			return false, fmt.Errorf("reconcile lock held by %s", existingHolder)
		}

		original := latest.DeepCopy()
		latest.Annotations[oracleRestartLockHolderAnnotation] = holder
		latest.Annotations[oracleRestartLockPhaseAnnotation] = string(phase)
		latest.Annotations[oracleRestartLockGenerationAnnotation] = strconv.FormatInt(latest.Generation, 10)
		latest.Annotations[oracleRestartLockAcquiredAtAnnotation] = time.Now().UTC().Format(time.RFC3339Nano)

		if err := r.Patch(ctx, latest, client.MergeFrom(original)); err != nil {
			if apierrors.IsConflict(err) {
				lastErr = err
				time.Sleep(retryDelay)
				continue
			}
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("failed to acquire reconcile lock after retries: %w", lastErr)
}

func (r *OracleRestartReconciler) releaseOracleRestartReconcileLock(
	ctx context.Context,
	req ctrl.Request,
) error {
	holder := req.NamespacedName.String()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &oraclerestartdb.OracleRestart{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			return nil
		}
		if existing := latest.Annotations[oracleRestartLockHolderAnnotation]; existing != "" && existing != holder {
			return nil
		}
		original := latest.DeepCopy()
		delete(latest.Annotations, oracleRestartLockHolderAnnotation)
		delete(latest.Annotations, oracleRestartLockPhaseAnnotation)
		delete(latest.Annotations, oracleRestartLockAcquiredAtAnnotation)
		delete(latest.Annotations, oracleRestartLockGenerationAnnotation)
		return r.Patch(ctx, latest, client.MergeFrom(original))
	})
}

// generateConfigMapAutoUpdate refreshes the envfile data in an existing
// ConfigMap with current configuration values pulled from status and spec.
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
	envVars := sharedenvfile.ParseMap(envFileData)
	configMapData["envfile"] = sharedenvfile.SerializeMap(envVars)

	return configMapData, nil
}

// updateConfigMap writes updated configuration data back to Kubernetes so the
// Oracle Restart components observe the latest envfile contents.
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

// updateReconcileStatus updates the reconciliation status of an OracleRestart resource.
// It updates the RAC topology information, sets the appropriate reconciliation condition
// based on the current state (completed, blocked, queued, or error), and persists the
// status changes to the API server with retry logic.
//
// The function performs the following operations:
// 1. Updates the Oracle Restart instance topology and database topology status
// 2. Determines and sets the appropriate reconciliation condition based on:
//   - *completed: Sets condition to CrdReconcileCompleteState
//   - *blocked: Sets condition to CrdReconcileWaitingState
//   - result.Requeue: Sets condition to CrdReconcileQueuedState
//   - *err != nil: Sets condition to CrdReconcileErrorState
//     3. Manages status conditions by removing duplicates and setting the new condition
//     4. Transitions the resource state from PodAvailableState to AvailableState when
//     reconciliation is complete
//     5. Attempts to patch the status with up to maxRetries attempts, handling conflicts
//     and fetch errors with exponential backoff retry strategy
//
// Parameters:
//   - oracleRestart: The OracleRestart resource to update
//   - ctx: Context for API operations
//   - req: The reconciliation request containing namespace and name
//   - result: The reconciliation result indicating if requeue is needed
//   - err: Pointer to error that occurred during reconciliation
//   - blocked: Indicates if reconciliation is blocked on dependencies
//   - completed: Indicates if reconciliation has completed successfully
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
			Type:               string(oraclerestartdb.OracleRestartCrdReconcileCompeleteState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileCompleteReason),
			Message:            "reconcile completed successfully", // no error text
			Status:             metav1.ConditionTrue,
		}
	} else if *blocked {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.OracleRestartCrdReconcileWaitingState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileWaitingReason),
			Message:            "reconcile is waiting on dependencies", // neutral message
			Status:             metav1.ConditionTrue,
		}
	} else if result.Requeue {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.OracleRestartCrdReconcileQueuedState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileQueuedReason),
			Message:            "reconcile has been queued", // neutral message
			Status:             metav1.ConditionTrue,
		}
	} else if err != nil && *err != nil {
		condition = metav1.Condition{
			Type:               string(oraclerestartdb.OracleRestartCrdReconcileErrorState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: oracleRestart.GetGeneration(),
			Reason:             string(oraclerestartdb.OracleRestartCrdReconcileErrorReason),
			Message:            (*err).Error(), // show actual error only here
			Status:             metav1.ConditionTrue,
		}
	} else {
		return
	}
	// Preserve transition time when condition semantics did not change.
	if prev := meta.FindStatusCondition(oracleRestart.Status.Conditions, condition.Type); prev != nil &&
		prev.Status == condition.Status &&
		prev.Reason == condition.Reason &&
		prev.Message == condition.Message &&
		prev.ObservedGeneration == condition.ObservedGeneration {
		condition.LastTransitionTime = prev.LastTransitionTime
	}

	if len(oracleRestart.Status.Conditions) > 0 {
		meta.RemoveStatusCondition(&oracleRestart.Status.Conditions, condition.Type)
	}
	meta.SetStatusCondition(&oracleRestart.Status.Conditions, condition)

	if oracleRestart.Status.State == string(oraclerestartdb.OracleRestartPodAvailableState) &&
		condition.Type == string(oraclerestartdb.OracleRestartCrdReconcileCompeleteState) {
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
		// Avoid noisy status writes when no effective status change exists.
		if reflect.DeepEqual(oracleRestart.Status, latestInstance.Status) {
			r.Log.Info("No Oracle Restart status changes detected; skipping status patch", "Instance", oracleRestart.Name)
			return
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

// validateSpex validates the OracleRestart specification by performing a series of checks
// on the provided OracleRestart object. It verifies image pull secrets, grid and database
// response files, and network interface configurations. If any validation fails, it records
// an event and returns an error. The oldSpec parameter is available for future use in
// comparing specification changes. It logs the start and completion of validation and
// returns nil if all validations pass successfully.
func (r *OracleRestartReconciler) validateSpex(oracleRestart *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec, ctx context.Context, req ctrl.Request) error {
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
	r.ensureAsmStorageStatus(
		ctx,
		oracleRestart,
		req,
	)

	specDisks := flattenDisksBySize(&oracleRestart.Spec)

	// ----------------------------------------------------
	// VALIDATE REMOVED DISKS
	// ----------------------------------------------------
	for _, dg := range oracleRestart.Status.AsmDiskGroups {

		var runtimeDisks []string
		for _, d := range dg.Disks {
			name := strings.TrimSpace(d.Name)
			if name != "" {
				runtimeDisks = append(runtimeDisks, name)
			}
		}

		if len(specDisks) < len(runtimeDisks) {

			r.Log.Info(
				"Validating Disk to remove",
				"DiskgroupName", dg.Name,
			)

			_, err := findDisksToRemove(
				specDisks,
				runtimeDisks,
				oracleRestart,
			)

			if err != nil {

				oracleRestart.Spec.IsFailed = true

				return errors.New(
					"required Disk is part of diskgroup " +
						dg.Name +
						" and cannot be removed. Review manually.",
				)
			}
		}
	}

	// ----------------------------------------------------
	// VALIDATE ADDED DISKS
	// ----------------------------------------------------
	for _, dg := range oracleRestart.Status.AsmDiskGroups {

		var runtimeDisks []string
		for _, d := range dg.Disks {
			name := strings.TrimSpace(d.Name)
			if name != "" {
				runtimeDisks = append(runtimeDisks, name)
			}
		}

		if len(specDisks) > len(runtimeDisks) {

			_, err := findDisksToAdd(
				specDisks,
				runtimeDisks,
				oracleRestart,
				oldSpec,
			)

			if err != nil {
				return err
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

// flattenDisksBySize returns all disks defined in ASM spec
func flattenDisksBySize(spec *oraclerestartdb.OracleRestartSpec) []string {

	var all []string

	for _, dg := range spec.AsmStorageDetails {

		for _, d := range dg.Disks {

			name := strings.TrimSpace(d)

			if name != "" {
				all = append(all, name)
			}
		}
	}

	return all
}

// findDisksToRemove identifies disks that are present in the runtime status but missing from the new spec, indicating they should be removed. It returns a list of disks to remove and any validation errors encountered during the comparison.
func findDisksToRemove(
	specDisks []string,
	statusDisks []string,
	instance *oraclerestartdb.OracleRestart,
) ([]string, error) {

	specSet := make(map[string]struct{})

	for _, d := range specDisks {

		d = strings.TrimSpace(d)

		if d != "" {
			specSet[d] = struct{}{}
		}
	}

	var toRemove []string

	for _, d := range statusDisks {

		d = strings.TrimSpace(d)

		if d == "" {
			continue
		}

		if _, exists := specSet[d]; !exists {
			toRemove = append(toRemove, d)
		}
	}

	return toRemove, nil
}

// findDisksToAdd identifies disks that are present in the new spec but missing from the runtime status, indicating they should be added. It also checks for duplicates in the new spec and returns any validation errors encountered during the comparison.
func findDisksToAdd(
	newSpecDisks []string,
	statusDisks []string,
	instance *oraclerestartdb.OracleRestart,
	oldSpec *oraclerestartdb.OracleRestartSpec,
) ([]string, error) {

	// detect duplicates in spec

	seen := make(map[string]struct{})

	for _, d := range newSpecDisks {

		d = strings.TrimSpace(d)

		if d == "" {
			continue
		}

		if _, exists := seen[d]; exists {

			return nil, fmt.Errorf(
				"disk '%s' defined more than once in spec",
				d,
			)
		}

		seen[d] = struct{}{}
	}

	// build runtime set

	statusSet := make(map[string]struct{})

	for _, d := range statusDisks {

		d = strings.TrimSpace(d)

		if d != "" {
			statusSet[d] = struct{}{}
		}
	}

	var toAdd []string

	for _, d := range newSpecDisks {

		d = strings.TrimSpace(d)

		if d == "" {
			continue
		}

		if _, exists := statusSet[d]; !exists {
			toAdd = append(toAdd, d)
		}
	}

	return toAdd, nil
}

// validateASMDisks ensures disk discovery runs when needed and reconciles
// asm-related DaemonSets to keep status aligned with actual storage.
func (r *OracleRestartReconciler) validateASMDisks(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context) error {
	//var eventMsgs []string

	r.Log.Info("Validate New ASM Disks")
	if oraclerestartcommon.CheckStorageClass(oracleRestart) != "NOSC" {
		r.Log.Info("Skipping ASM disk validation because storage classes are configured")
		return nil
	}
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

// createDaemonSet creates or updates the disk-check DaemonSet used to detect
// ASM devices for Oracle Restart deployments.
func (r *OracleRestartReconciler) createDaemonSet(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context) error {
	r.Log.Info("Validate New ASM Disks")

	// Build the desired DaemonSet (disk-check)
	desiredDaemonSet := oraclerestartcommon.BuildDiskCheckDaemonSet(oracleRestart)

	// Try to get the existing DaemonSet
	existingDaemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desiredDaemonSet.Name,
		Namespace: desiredDaemonSet.Namespace,
	}, existingDaemonSet)

	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Creating DaemonSet", "name", desiredDaemonSet.Name)
			if err := r.Client.Create(ctx, desiredDaemonSet); err != nil {
				oracleRestart.Spec.IsFailed = true
				return err
			}
			r.Log.Info("DaemonSet created successfully", "DaemonSet.Name", desiredDaemonSet.Name)

		} else {
			oracleRestart.Spec.IsFailed = true
			return err
		}
	} else {
		// Check if volumes need update
		if !reflect.DeepEqual(existingDaemonSet.Spec.Template.Spec.Volumes, desiredDaemonSet.Spec.Template.Spec.Volumes) {
			r.Log.Info("Updating DaemonSet volumes", "name", desiredDaemonSet.Name)
			existingDaemonSet.Spec.Template.Spec.Volumes = desiredDaemonSet.Spec.Template.Spec.Volumes
			existingDaemonSet.Spec.Template.Spec.Containers = desiredDaemonSet.Spec.Template.Spec.Containers
			if err := r.Client.Update(ctx, existingDaemonSet); err != nil {
				return err
			}
			r.Log.Info("DaemonSet updated, waiting for pods to restart...")
		} else {
			r.Log.Info("DaemonSet already up-to-date", "name", desiredDaemonSet.Name)
		}
	}

	// r.Log.Info("Disk sizes updated successfully")
	return nil
}

// updateDiskSizes captures ASM disk information from daemonset logs and
// stores results under the Oracle Restart status block.
func (r *OracleRestartReconciler) updateDiskSizes(
	ctx context.Context,
	oracleRestart *oraclerestartdb.OracleRestart,
) error {
	// 1. Build a map for quick lookup: disk group name → spec details
	dgSpecMap := make(map[string]oraclerestartdb.AsmDiskGroupDetails)
	for _, dg := range oracleRestart.Spec.AsmStorageDetails {
		dgSpecMap[dg.Name] = dg
	}

	// -- BEGIN CHANGE: Declare diskStatus slice
	var disks []oraclerestartdb.AsmDiskStatus
	// -- END CHANGE

	podList := &corev1.PodList{}
	labels := oraclerestartcommon.BuildLabelsForDaemonSet(oracleRestart, "disk-check")
	if err := r.Client.List(ctx, podList, client.InNamespace(oracleRestart.Namespace), client.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, pod := range podList.Items {
		req := r.kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: "disk-check",
		})
		logs, err := req.Stream(ctx)
			if err != nil {
				r.Log.Error(err, "Failed to stream logs", "pod", pod.Name)
				continue
			}
			func() { // Scope for defer
				defer func() {
					if closeErr := logs.Close(); closeErr != nil {
						r.Log.Error(closeErr, "Failed to close logs stream", "pod", pod.Name)
					}
				}()
				scanner := bufio.NewScanner(logs)
			for scanner.Scan() {
				var entry struct {
					Disk   string `json:"disk"`
					Valid  bool   `json:"valid"`
					SizeGb int    `json:"sizeGb"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
					r.Log.Error(err, "Failed to unmarshal disk info", "pod", pod.Name)
					continue
				}
				diskStatus := oraclerestartdb.AsmDiskStatus{
					Name:     entry.Disk,
					SizeInGb: entry.SizeGb,
					Valid:    entry.Valid,
				}

				// -- BEGIN CHANGE: Append to disks slice
				disks = append(disks, diskStatus)
				// -- END CHANGE
			}
		}()
	}

	// Build final slice: populate each group's Disks from spec
	var diskGroups []oraclerestartdb.AsmDiskGroupStatus
	for _, dgSpec := range oracleRestart.Spec.AsmStorageDetails {
		// Use '+DATA' if group name is missing or empty
		groupName := dgSpec.Name
		if strings.TrimSpace(groupName) == "" {
			groupName = "+DATA"
		}

		var groupDisks []oraclerestartdb.AsmDiskStatus
		for _, diskName := range dgSpec.Disks {
			for _, d := range disks {
				if d.Name == diskName {
					groupDisks = append(groupDisks, d)
					break
				}
			}
		}
		// Skip empty disk groups
		if len(groupDisks) == 0 {
			continue
		}
		diskGroupStatus := oraclerestartdb.AsmDiskGroupStatus{
			Name:         groupName,
			Redundancy:   dgSpec.Redundancy,
			Type:         dgSpec.Type,
			AutoUpdate:   dgSpec.AutoUpdate,
			StorageClass: oracleRestart.Spec.CrsDgStorageClass,
			Disks:        groupDisks,
		}
		diskGroups = append(diskGroups, diskGroupStatus)
	}

	// 4. Update/persist status (directly on AsmDiskGroups)
	oracleRestart.Status.AsmDiskGroups = diskGroups

	// 5. Patch Status with retries for conflicts
	const maxRetries = 3
	const retryDelay = time.Second * 2
	for attempt := 0; attempt < maxRetries; attempt++ {
		latestInstance := &oraclerestartdb.OracleRestart{}
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: oracleRestart.Namespace, Name: oracleRestart.Name}, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch latest RAC instance (for patch retry)")
			return err
		}
		latestInstance.Status.AsmDiskGroups = oracleRestart.Status.AsmDiskGroups

		if err := mergeInstancesFromLatest(oracleRestart, latestInstance); err != nil {
			r.Log.Error(err, "Failed to merge status from latest instance (for patch retry)")
			return err
		}
		oracleRestart.ResourceVersion = latestInstance.ResourceVersion
		err = r.Client.Status().Update(ctx, oracleRestart)
		// err = r.Client.Status().Patch(ctx, oracleRestart, client.MergeFrom(latestInstance))
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected while patching disk status, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				continue
			}
			r.Log.Error(err, "Failed to update disk status on RAC DB instance")
			return err
		}
		// Patch succeeded!
		return nil
	}
	return fmt.Errorf("failed to update disk sizes after %d retries", maxRetries)
}

// cleanupDaemonSet removes the disk-check DaemonSet when ASM discovery has
// finished so no helper pods remain running unnecessarily.
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

// setDefaults sets default values for the OracleRestart specification.
// It configures default values for:
//   - ImagePullPolicy: defaults to "Always"
//   - SshKeySecret: sets default SSH key mount location if not specified
//   - DbSecret: sets default database secret mount locations for password and key files if not specified
//   - TdeWalletSecret: sets default TDE wallet secret mount locations for password and key files if not specified
//   - ConfigParams: sets default software mount location and database character set (AL32UTF8)
//
// This function modifies the OracleRestart object in place and returns an error if any issues occur.
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

		// if oracleRestart.Spec.ConfigParams.GridResponseFile.ConfigMapName == "" {
		// 	if oracleRestart.Spec.ConfigParams.CrsAsmDiskDg == "" {
		// 		oracleRestart.Spec.ConfigParams.CrsAsmDiskDg = "+DATA"
		// 	}

		// 	if oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy == "" {
		// 		oracleRestart.Spec.ConfigParams.CrsAsmDiskDgRedundancy = "external"
		// 	}
		// }

		// if oracleRestart.Spec.ConfigParams.DbResponseFile.ConfigMapName == "" {
		// 	if oracleRestart.Spec.ConfigParams.DbDataFileDestDg == "" {
		// 		oracleRestart.Spec.ConfigParams.DbDataFileDestDg = oracleRestart.Spec.ConfigParams.CrsAsmDiskDg
		// 	}

		// 	if oracleRestart.Spec.ConfigParams.DbRecoveryFileDest == "" {
		// 		oracleRestart.Spec.ConfigParams.DbRecoveryFileDest = oracleRestart.Spec.ConfigParams.DbDataFileDestDg
		// 	}

		if oracleRestart.Spec.ConfigParams.DbCharSet == "" {
			oracleRestart.Spec.ConfigParams.DbCharSet = "AL32UTF8"
		}
	}

	// }
	return nil

}

// updateGiConfigParamStatus updates the configuration parameters status in the OracleRestart object.
// It populates the Status.ConfigParams fields (Inventory, GridBase, GridHome) by either copying them
// from the Spec if provided, or by extracting them from a Grid response file stored in a ConfigMap.
// The response file location is determined by ConfigMapName and Name fields in GridResponseFile.
// If extraction from the response file fails, it sets IsFailed to true and returns an error.
// It initializes Status.ConfigParams if it is nil before processing.
// Returns an error if reading the response file fails.
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
		// if oracleRestart.Status.ConfigParams.CrsAsmDeviceList == "" {
		// 	if oracleRestart.Spec.ConfigParams.CrsAsmDeviceList != "" {
		// 		oracleRestart.Status.ConfigParams.CrsAsmDeviceList = oracleRestart.Spec.ConfigParams.CrsAsmDeviceList
		// 	} else {
		// 		diskList, err := oraclerestartcommon.CheckRspData(oracleRestart, r.Client, "diskList", cName, fName)
		// 		if err != nil {
		// 			return errors.New(("error in responsefile, unable to read diskList"))
		// 		}
		// 		oracleRestart.Status.ConfigParams.CrsAsmDeviceList = diskList
		// 	}
		// }
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

	}

	return nil

}

// updateDbConfigParamStatus updates the configuration parameters status in the OracleRestart resource.
// It populates the Status.ConfigParams fields by reading values from the Spec.ConfigParams or by
// extracting them from response files stored in ConfigMaps.
//
// The method handles the following configuration parameters:
// - DbName: Database name, read from response file if not explicitly specified
// - DbBase: Oracle base directory, read from response file if not explicitly specified
// - DbHome: Oracle home directory, read from response file if not explicitly specified
// - GridHome: Grid home directory, read from response file if not explicitly specified
//
// For each parameter, the method follows this priority:
// 1. If the status parameter is already set, it is not modified
// 2. If the spec parameter is set, it is copied to the status
// 3. If neither is set, the value is extracted from the response file specified in DbResponseFile
//
// The response file location is determined by the ConfigMapName and Name fields in DbResponseFile.
// If the response file cannot be read or required values are missing, the method sets IsFailed to true
// and returns an error.
//
// Parameters:
//   - oracleRestart: A pointer to the OracleRestart resource to be updated
//
// Returns:
//   - error: An error if the response file cannot be read or required configuration values are missing
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

// updateOracleRestartInstTopologyStatus retrieves and returns the topology status information for an Oracle Restart instance.
// It validates the Oracle Restart instance, retrieves the associated pod, and collects node details where the pod is running.
// If the instance is not marked for deletion, it gathers pod names and node information.
// If pod or node details cannot be collected, it marks the OracleRestart as failed and returns an error.
// Returns a slice of pod names, a map of pod names to their corresponding nodes, and any error encountered during the process.
func (r *OracleRestartReconciler) updateOracleRestartInstTopologyStatus(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request) ([]string, map[string]*corev1.Node, error) {

	//orestartPod := &corev1.Pod{}
	var podNames []string
	nodeDetails := make(map[string]*corev1.Node)

	if strings.ToLower(oracleRestart.Spec.InstDetails.IsDelete) != "true" {
		_, pod, err := r.validateOracleRestartInst(oracleRestart, ctx, req, oracleRestart.Spec.InstDetails, 0)
		if err != nil {
			return podNames, nodeDetails, err
		}
		if pod == nil {
			return nil, nil, fmt.Errorf("Pod not found for Oracle Restart instance")
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

// getNodeDetails fetches Kubernetes node metadata used for topology checks.
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

// updateoraclerestartdbTopologyStatus updates database-level topology status
// using the collected pod and node information.
func (r *OracleRestartReconciler) updateoraclerestartdbTopologyStatus(OracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node) error {

	//orestartPod := &corev1.Pod{}
	var err error
	_, _, err = r.validateoraclerestartdb(OracleRestart, ctx, req, podNames, nodeDetails)
	if err != nil {
		return err
	}
	return nil
}

// validateoraclerestartdb ensures Oracle Restart database status is updated
// after verifying StatefulSet and pod health across nodes.
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

	// validateOracleRestartInst validates a single Oracle Restart instance,
	// inspecting associated StatefulSet and pods to drive status updates.
func (r *OracleRestartReconciler) validateOracleRestartInst(oracleRestart *oraclerestartdb.OracleRestart, ctx context.Context, req ctrl.Request, OraRestartSpex oraclerestartdb.OracleRestartInstDetailSpec, specId int) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	var orestartSfSet *appsv1.StatefulSet
	orestartPod := &corev1.Pod{}

	orestartSfSet, err = oraclerestartcommon.CheckSfset(OraRestartSpex.Name, oracleRestart, r.Client)
	if err != nil && orestartSfSet != nil {
		//msg := "Unable to find Oracle Restart statefulset " + oraclerestartcommon.GetFmtStr(OraRestartSpex.Name) + "."
		//oraclerestartcommon.LogMessages("INFO", msg, nil, instance, r.Log)
		r.updateOracleRestartInstStatus(oracleRestart, ctx, req, OraRestartSpex, string(oraclerestartdb.StatefulSetNotFound), r.Client, false)
		return orestartSfSet, orestartPod, err
	}
	if orestartSfSet == nil {
		return orestartSfSet, nil, nil
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
			return orestartSfSet, orestartPod, errors.New(msg)
		} else {
			// Handle the case where no pods were found at all
			msg = "unable to validate Oracle Restart pod. No pods matching the criteria were found"
			oraclerestartcommon.LogMessages("INFO", msg, nil, oracleRestart, r.Log)
			return orestartSfSet, orestartPod, errors.New(msg)
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

// updateOracleRestartInstStatus updates Oracle Restart instance status with
// retry logic to handle concurrent modifications.
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
			r.ensureAsmStorageStatus(
				ctx,
				latestInstance,
				req,
			)

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
	return sharedspecguard.RestrictedConfigParamFields()
}

// mergeInstancesFromLatest copies relevant fields from the latest object into
// the working instance to avoid clobbering concurrent status updates.
// mergeInstancesFromLatest copies exported fields from the latest Oracle Restart object into the reconcile instance.
func mergeInstancesFromLatest(instance, latestInstance *oraclerestartdb.OracleRestart) error {
	return sharedstatusmerge.MergeNamedStructField(
		instance,
		latestInstance,
		"Status",
		sharedstatusmerge.Options{
			PointerMode: sharedstatusmerge.PointerCopyIfNil,
			SliceMode:   sharedstatusmerge.SliceReplace,
			SkipSliceFields: map[string]struct{}{
				"AsmDiskGroups": {},
			},
		},
	)
}

// generateConfigMap builds the primary envfile ConfigMap for Oracle Restart
// deployments, assembling data based on the current spec.
func (r *OracleRestartReconciler) generateConfigMap(instance *oraclerestartdb.OracleRestart) (map[string]string, error) {
	configMapData := make(map[string]string, 0)
	// new_crs_nodes, existing_crs_nodes_healthy, existing_crs_nodes_not_healthy, install_node, new_crs_nodes_list := oraclerestartcommon.GetCrsNodes(instance, r.kubeClient, r.kubeConfig, r.Log, r.Client)
	install_node := instance.Spec.InstDetails.Name + "-0"
	// asm_devices := oraclerestartcommon.GetAsmDevices(instance)
	var data []string
	var addnodeFlag bool

	//Defaults from webhook
	if instance.Spec.ImagePullPolicy == nil || *instance.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		instance.Spec.ImagePullPolicy = &policy
	}

	if instance.Spec.SshKeySecret != nil {
		if instance.Spec.SshKeySecret.KeyMountLocation == "" {
			instance.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSshSecretMount
		}
	}

	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		if instance.Spec.DbSecret.PwdFileMountLocation == "" {
			instance.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
		}
		if instance.Spec.DbSecret.KeyFileMountLocation == "" {
			instance.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
		}
	}

	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		if instance.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
		}
		if instance.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
			instance.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
		}
	}

	// --- Pick ALL envVars directly from CR spec ---
	for _, e := range instance.Spec.InstDetails.EnvVars {
		data = append(data, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}

	if len(instance.Spec.ConfigParams.OpType) == 0 {
		data = append(data, "OP_TYPE=setuprac")
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

	// Check for nil SshKeySecret and ensure Name is not empty/whitespace
	if instance.Spec.SshKeySecret != nil && strings.TrimSpace(instance.Spec.SshKeySecret.Name) != "" {
		if instance.Spec.SshKeySecret.KeyMountLocation != "" && instance.Spec.SshKeySecret.PrivKeySecretName != "" {
			data = append(data, "SSH_PRIVATE_KEY="+
				instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PrivKeySecretName)
		}
		// PubKeySecretName may be optional; only append if present
		if instance.Spec.SshKeySecret.KeyMountLocation != "" && instance.Spec.SshKeySecret.PubKeySecretName != "" {
			data = append(data, "SSH_PUBLIC_KEY="+
				instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PubKeySecretName)
		}
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

	// ---- ASM DISK GROUP FIELDS: now using new model ----
	crsDiskGroup := ""
	crsDeviceList := ""
	crsRedundancy := ""
	dataDeviceList := ""
	recoDeviceList := ""
	redoDeviceList := ""
	dataDgName := ""
	recoDgName := ""
	redoDgName := ""
	dataRedundancy := ""
	recoRedundancy := ""
	redoRedundancy := ""

	for _, dg := range instance.Spec.AsmStorageDetails {
		switch dg.Type {
		case oraclerestartdb.CrsAsmDiskDg:
			if dg.Name != "" {
				crsDiskGroup = ensurePlusPrefix(dg.Name)
			}
			if dg.Redundancy != "" {
				crsRedundancy = dg.Redundancy
			}
		case oraclerestartdb.DbDataDiskDg:
			if dg.Name != "" {
				dataDgName = ensurePlusPrefix(dg.Name)
			}
			if dg.Redundancy != "" {
				dataRedundancy = dg.Redundancy
			}
		case oraclerestartdb.DbRecoveryDiskDg:
			if dg.Name != "" {
				recoDgName = ensurePlusPrefix(dg.Name)
			}
			if dg.Redundancy != "" {
				recoRedundancy = dg.Redundancy
			}
		case oraclerestartdb.RedoDiskDg:
			if dg.Name != "" {
				redoDgName = ensurePlusPrefix(dg.Name)
			}
			if dg.Redundancy != "" {
				redoRedundancy = dg.Redundancy
			}
		}
	}

	asmDevicesByType := func(
		specGroups []oraclerestartdb.AsmDiskGroupDetails,
		statusGroups []oraclerestartdb.AsmDiskGroupStatus,
		typ oraclerestartdb.AsmDiskDGTypes,
	) string {

		var result []string

		for _, group := range specGroups {
			if group.Type == typ {
				for _, diskName := range group.Disks {
					result = append(result, diskName)
				}
			}
		}

		return strings.Join(result, ",")
	}

	crsDeviceList = asmDevicesByType(
		instance.Spec.AsmStorageDetails,
		instance.Status.AsmDiskGroups,
		oraclerestartdb.CrsAsmDiskDg,
	)

	dataDeviceList = asmDevicesByType(
		instance.Spec.AsmStorageDetails,
		instance.Status.AsmDiskGroups,
		oraclerestartdb.DbDataDiskDg,
	)

	recoDeviceList = asmDevicesByType(
		instance.Spec.AsmStorageDetails,
		instance.Status.AsmDiskGroups,
		oraclerestartdb.DbRecoveryDiskDg,
	)

	redoDeviceList = asmDevicesByType(
		instance.Spec.AsmStorageDetails,
		instance.Status.AsmDiskGroups,
		oraclerestartdb.RedoDiskDg,
	)

	// Environment variables ("KEY=VAL" entries), set only if non-empty
	if crsDiskGroup != "" {
		data = append(data, "CRS_ASM_DISKGROUP="+crsDiskGroup)
	} else {
		crsDiskGroup = "+DATA"
		data = append(data, "CRS_ASM_DISKGROUP="+crsDiskGroup)
	}
	if crsDeviceList != "" {
		data = append(data, "CRS_ASM_DEVICE_LIST="+crsDeviceList)
	}
	if crsRedundancy != "" {
		data = append(data, "CRS_ASMDG_REDUNDANCY="+crsRedundancy)
	}
	if dataDgName == "" {
		data = append(data, "DB_DATA_FILE_DEST="+crsDiskGroup)
	} else {
		data = append(data, "DB_DATA_FILE_DEST="+dataDgName)
	}
	if dataDeviceList != "" {
		data = append(data, "DB_ASM_DEVICE_LIST="+dataDeviceList)
	}
	if dataRedundancy != "" {
		data = append(data, "DB_ASMDG_PROPERTIES=redundancy:"+dataRedundancy)
	}
	if recoDeviceList != "" {
		data = append(data, "RECO_ASM_DEVICE_LIST="+recoDeviceList)
	}
	if recoDgName == "" {
		data = append(data, "DB_RECOVERY_FILE_DEST="+crsDiskGroup)
	} else {
		data = append(data, "DB_RECOVERY_FILE_DEST="+recoDgName)
	}
	if recoRedundancy != "" {
		data = append(data, "RECO_ASMDG_PROPERTIES=redundancy:"+recoRedundancy)
	}
	if redoDgName != "" {
		data = append(data, "LOG_FILE_DEST="+redoDgName)
	}
	if redoDeviceList != "" {
		data = append(data, "REDO_ASM_DEVICE_LIST="+redoDeviceList)
	}
	if redoRedundancy != "" {
		data = append(data, "REDO_ASMDG_PROPERTIES=redundancy:"+redoRedundancy)
	}
	if instance.Spec.ConfigParams.DbCharSet == "" {
		instance.Spec.ConfigParams.DbCharSet = "AL32UTF8"
		data = append(data, "DB_CHARACTERSET="+instance.Spec.ConfigParams.DbCharSet)
	}

	// ---- ALL OTHER CONFIG PARAMS - use as before ----

	if !addnodeFlag {
		cfg := instance.Spec.ConfigParams
		if cfg != nil {
			if cfg.DbStorageType != "" {
				data = append(data, "DB_STORAGE_TYPE="+cfg.DbStorageType)
			}
			if cfg.DbCharSet != "" {
				data = append(data, "DB_CHARACTERSET="+cfg.DbCharSet)
			}

			if cfg.DbType != "" {
				data = append(data, "DB_TYPE="+cfg.DbType)
			}
			if cfg.DbConfigType != "" {
				data = append(data, "DB_CONFIG_TYPE="+cfg.DbConfigType)
			}
			if cfg.EnableArchiveLog != "" {
				data = append(data, "ENABLE_ARCHIVELOG="+cfg.EnableArchiveLog)
			}
			if cfg.GridResponseFile.ConfigMapName != "" {
				data = append(data, "GRID_RESPONSE_FILE="+utils.OraGiRsp+"/"+cfg.GridResponseFile.Name)
			}
			if cfg.DbResponseFile.ConfigMapName != "" {
				data = append(data, "DBCA_RESPONSE_FILE="+utils.OraDbRsp+"/"+cfg.DbResponseFile.Name)
			}
			if cfg.SgaSize != "" {
				data = append(data, "INIT_SGA_SIZE="+cfg.SgaSize)
			}
			if cfg.PgaSize != "" {
				data = append(data, "INIT_PGA_SIZE="+cfg.PgaSize)
			}
			if cfg.Processes > 0 {
				data = append(data, "INIT_PROCESSES="+strconv.Itoa(cfg.Processes))
			}
			if cfg.CpuCount > 0 {
				data = append(data, "CPU_COUNT="+strconv.Itoa(cfg.CpuCount))
			}
			// Later in your code where you append INIT_* values:
			if instance.Spec.ConfigParams.SgaSize != "" {
				normalizedSGA := normalizeOracleMemoryUnit(instance.Spec.ConfigParams.SgaSize)
				data = append(data, "INIT_SGA_SIZE="+normalizedSGA)
			}

			if instance.Spec.ConfigParams.PgaSize != "" {
				normalizedPGA := normalizeOracleMemoryUnit(instance.Spec.ConfigParams.PgaSize)
				data = append(data, "INIT_PGA_SIZE="+normalizedPGA)
			}
			if instance.Spec.ConfigParams.Processes > 0 {
				// Configmap check is done in ValidateSpex
				data = append(data, "INIT_PROCESSES="+strconv.Itoa(instance.Spec.ConfigParams.Processes))
			}

			if instance.Spec.ConfigParams.CpuCount > 0 {
				// Configmap check is done in ValidateSpex
				data = append(data, "CPU_COUNT="+strconv.Itoa(instance.Spec.ConfigParams.CpuCount))
			}

		}
	}

	configMapData["envfile"] = strings.Join(data, "\r\n")
	return configMapData, nil
}

// ensurePlusPrefix ensures ASM disk group names include the '+' prefix.
func ensurePlusPrefix(name string) string {
	return sharedorautil.EnsurePlusPrefix(name)
}

// normalizeOracleMemoryUnit converts "Gi"/"Mi" suffixes into Oracle DBCA
// compatible units like "G" and "M".
// normalizeOracleMemoryUnit ensures Oracle memory values include canonical units for downstream comparisons.
func normalizeOracleMemoryUnit(s string) string {
	return sharedorautil.NormalizeOracleMemoryUnit(s)
}

// createConfigMap ensures the configuration ConfigMap exists and updates its
// envfile data when changes are detected.
func (r *OracleRestartReconciler) createConfigMap(
	ctx context.Context,
	instance oraclerestartdb.OracleRestart,
	cm *corev1.ConfigMap,
) (ctrl.Result, bool, error) { // Added `bool` return
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	changed, err := sharedk8sobjects.EnsureConfigMapEnvfile(ctx, r.Client, instance.Namespace, cm)
	if err != nil {
		reqLogger.Error(err, "failed to reconcile configmap", "namespace", instance.Namespace)
		return ctrl.Result{}, false, err
	}
	if changed {
		return ctrl.Result{Requeue: true}, true, nil
	}
	return ctrl.Result{}, false, nil
}

// createOrReplaceService reconciles Services backing Oracle Restart network
// endpoints using the desired definition.
func (r *OracleRestartReconciler) createOrReplaceService(ctx context.Context, instance *oraclerestartdb.OracleRestart,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	changed, err := sharedk8sobjects.EnsureService(ctx, r.Client, instance.Namespace, dep, sharedk8sobjects.ServiceSyncOptions{
		NodePortMerge:           sharedk8sobjects.NodePortMergeByName,
		SyncPublishNotReady:     true,
		SyncLoadBalancerFields:  true,
		SyncHealthCheckNodePort: true,
	})
	if err != nil {
		instance.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to reconcile Service", "Service.Namespace", dep.Namespace, "Service.Name", dep.Name)
		return ctrl.Result{}, err
	}
	if changed {
		reqLogger.Info("Service reconciled to desired state", "service", dep.Name)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func mergeOracleRestartServicePortsWithAssignedNodePorts(existing []corev1.ServicePort, desired []corev1.ServicePort) []corev1.ServicePort {
	return sharedk8sobjects.MergeServicePortsWithAssignedNodePortByName(existing, desired)
}

// createOrReplaceAsmPv reconciles PersistentVolumes for ASM disk devices,
// ensuring the existing PV matches the requested configuration.
func (r *OracleRestartReconciler) createOrReplaceAsmPv(
	ctx context.Context,
	instance *oraclerestartdb.OracleRestart,
	dep *corev1.PersistentVolume,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	name, created, err := sharedk8sobjects.EnsurePersistentVolume(context.TODO(), r.Client, dep)
	if err != nil {
		instance.Spec.IsFailed = true
		if strings.Contains(err.Error(), "different disk configuration") {
			reqLogger.Info("Detected existing PV with different disk details and as the configuration has changed, setup cannot continue", "dep.Name", dep.Name)
		} else {
			reqLogger.Error(err, "Failed to reconcile Persistent Volume", "PV.Name", dep.Name)
		}
		return "", ctrl.Result{}, err
	}
	if created {
		reqLogger.Info("Creating a new PV", "dep.Name", dep.Name)
		return dep.Name, ctrl.Result{}, nil
	}

	reqLogger.Info("PV Found", "dep.Name", dep.Name, "dgType", dgType)
	return name, ctrl.Result{}, nil

}

// createOrReplaceAsmPvC manages PersistentVolumeClaims for ASM disks,
// creating or validating claims to satisfy storage requirements.
func (r *OracleRestartReconciler) createOrReplaceAsmPvC(ctx context.Context, instance *oraclerestartdb.OracleRestart,
	dep *corev1.PersistentVolumeClaim,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	jsn, _ := json.Marshal(dep)
	oraclerestartcommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	name, created, err := sharedk8sobjects.EnsurePersistentVolumeClaim(ctx, r.Client, dep)
	if err != nil {
		instance.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to reconcile Persistent Volume Claim", "PVC.Namespace", dep.Namespace, "PersistentVolume.Name", dep.Name)
		return "", ctrl.Result{}, err
	}
	if created {
		reqLogger.Info("Creating a PVC")
		return dep.Name, ctrl.Result{}, nil
	}
	reqLogger.Info("PVC Found", "dep.Name", dep.Name, "dgType", dgType)
	return name, ctrl.Result{}, nil

}

// ensureAsmStorageStatus updates the Oracle Restart status with the current state of ASM disk groups,
func (r *OracleRestartReconciler) ensureAsmStorageStatus(
	ctx context.Context,
	oracleRestart *oraclerestartdb.OracleRestart,
	req ctrl.Request,
) {

	r.Log.Info("Reconciling ASM DiskGroup status")

	oraSpex := oracleRestart.Spec.InstDetails

	if strings.ToLower(oraSpex.IsDelete) == "true" {
		return
	}

	isDynamic := oraclerestartcommon.CheckStorageClass(oracleRestart) != "NOSC"

	// =====================================================
	// Dynamic Provisioning Logic
	// =====================================================
	if isDynamic {

		podName := fmt.Sprintf("%s-%d", oraSpex.Name, 0)

		var newStatus []oraclerestartdb.AsmDiskGroupStatus
		assignedDisks := make(map[string]bool)

		for _, dgSpec := range oracleRestart.Spec.AsmStorageDetails {

			if len(dgSpec.Disks) == 0 {
				continue
			}

			dgStatus := oraclerestartdb.AsmDiskGroupStatus{
				Name: dgSpec.Name,
				Type: dgSpec.Type,
			}

			disks := oraclerestartcommon.GetAsmDisks(
				podName,
				dgSpec.Name,
				oracleRestart,
				0,
				r.kubeClient,
				r.kubeConfig,
				r.Log,
			)

			for _, d := range disks {

				clean := strings.TrimSpace(d)
				clean = strings.Trim(clean, "\"")

				if clean == "" || clean == "Pending" {
					continue
				}

				parts := strings.Split(clean, ",")

				for _, p := range parts {

					name := strings.TrimSpace(p)
					if name == "" {
						continue
					}

					if assignedDisks[name] {
						continue
					}

					assignedDisks[name] = true

					dgStatus.Disks = append(dgStatus.Disks,
						oraclerestartdb.AsmDiskStatus{
							Name:     name,
							Valid:    true,
							SizeInGb: oracleRestart.Spec.AsmStorageSizeInGb,
						},
					)
				}
			}

			if len(dgStatus.Disks) > 0 {
				newStatus = append(newStatus, dgStatus)
			}
		}

		if !reflect.DeepEqual(oracleRestart.Status.AsmDiskGroups, newStatus) {
			oracleRestart.Status.AsmDiskGroups = newStatus
			r.Log.Info("ASM DiskGroup status updated",
				"Count", len(newStatus))
		}

		return
	}

	// =====================================================
	// Static Provisioning Logic (UNCHANGED)
	// =====================================================

	if oracleRestart.Status.AsmDiskGroups == nil {
		oracleRestart.Status.AsmDiskGroups = []oraclerestartdb.AsmDiskGroupStatus{}

		idx := 0
		oraRacSpex := oracleRestart.Spec.InstDetails

		if strings.ToLower(oraRacSpex.IsDelete) == "true" {
			return
		}

		podName := fmt.Sprintf("%s-%d", oraRacSpex.Name, 0)

		r.Log.Info("Restoring ASM DiskGroup devices for instance",
			"Instance", oraRacSpex.Name)

		diskGroup := oraclerestartcommon.GetAsmDiskgroup(
			podName,
			oracleRestart,
			idx,
			r.kubeClient,
			r.kubeConfig,
			r.Log,
		)

		if diskGroup != "" {
			for i, dgStatus := range oracleRestart.Status.AsmDiskGroups {
				if dgStatus.Name == diskGroup {
					oracleRestart.Status.AsmDiskGroups[i].Name = diskGroup
					break
				}
			}
		}
	}
}

// ensureStatefulSetUpdated performs rolling updates on the Oracle Restart
// StatefulSet when volume device configuration changes.
func (r *OracleRestartReconciler) ensureStatefulSetUpdated(ctx context.Context,
	reqLogger logr.Logger,
	oracleRestart *oraclerestartdb.OracleRestart,
	desired *appsv1.StatefulSet,
	asmAutoUpdate bool,
	configmapEnvKeyChanged bool,
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
		reqLogger.Info("StatefulSet matches for  Volumes, SFS wont be updated", "StatefulSet.Namespace", oracleRestart.Namespace, "StatefulSet.Name", desired.Name)
		// return nil
	}
	if configmapEnvKeyChanged {
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
	}

	return nil
}

// executeDiskGroupCommand runs a command inside the specified pod to inspect
// or manipulate ASM disk groups.
// executeDiskGroupCommand runs an ASM disk management command on the target pod and returns stdout and stderr.
func executeDiskGroupCommand(podName string, cmd []string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *oraclerestartdb.OracleRestart, logger logr.Logger) (string, string, error) {
	return oraclerestartcommon.ExecCommand(
		podName,
		cmd,
		oraclerestartcommon.NewExecCommandResp(kubeClient, kubeConfig),
		instance,
		logger,
	)
}

// diskGroupExists checks if a disk group is present by querying the ASM state
// within a pod.
func (r *OracleRestartReconciler) diskGroupExists(podName, diskGroupName string, kubeClient kubernetes.Interface, kubeConfig clientcmd.ClientConfig, instance *oraclerestartdb.OracleRestart, logger logr.Logger) (bool, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	cmd := "python3 /opt/scripts/startup/scripts/main.py --getasmdiskgroup=true"
	stdout, _, err := oraclerestartcommon.ExecCommand(
		podName,
		[]string{"bash", "-c", cmd},
		oraclerestartcommon.NewExecCommandResp(r.kubeClient, r.kubeConfig),
		instance,
		reqLogger,
	)
	if err != nil {
		return false, err
	}
	if strings.Contains(stdout, diskGroupName) {
		return true, nil
	}
	return false, nil
}

// addDisks adds new ASM devices to an existing disk group by invoking helper
// scripts inside each Oracle Restart pod.
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
			stdout, stderr, err := oraclerestartcommon.ExecCommand(
				podName,
				[]string{"bash", "-c", cmd},
				oraclerestartcommon.NewExecCommandResp(r.kubeClient, r.kubeConfig),
				instance,
				reqLogger,
			)
			if err != nil {
				instance.Spec.IsFailed = true
				reqLogger.Error(err, "Failed to execute command", "Pod.Name", podName, "Command", cmd, "Stdout", stdout, "Stderr", stderr)
				return err
			}
		}
	}
	return nil
}

// checkDaemonSetStatus monitors the disk-check DaemonSet until all pods
// complete successfully, returning readiness or timeout errors.
// checkDaemonSetStatus inspects the Oracle Restart daemonset and reports success once all pods complete.
func checkDaemonSetStatus(ctx context.Context, r *OracleRestartReconciler, oracleRestart *oraclerestartdb.OracleRestart) (bool, error) {
	timeout := time.After(2 * time.Minute)
	tick := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	defer tick.Stop()
	// Sleep for 60 seconds
	for {
		select {
		case <-timeout:
			ready, invalidDevice, err := shareddiskcheck.CheckDaemonSetReadyAndDiskValidation(
				ctx, r.Client, r.kubeClient, oracleRestart.Namespace, "disk-check-daemonset",
				shareddiskcheck.LabelSelectorForDaemonSet(oracleRestart, "disk-check"),
			)
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
			if invalidDevice {
				return false, nil
			}

			// DaemonSet did not become ready or running within the timeout
			return false, fmt.Errorf("DaemonSet %s/%s did not become ready or running within 5 minutes", oracleRestart.Namespace, "disk-check-daemonset")

		case <-tick.C:
			ready, invalidDevice, err := shareddiskcheck.CheckDaemonSetReadyAndDiskValidation(
				ctx, r.Client, r.kubeClient, oracleRestart.Namespace, "disk-check-daemonset",
				shareddiskcheck.LabelSelectorForDaemonSet(oracleRestart, "disk-check"),
			)
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
			if invalidDevice {
				return false, nil
			}
		}
	}
}

// createOrReplaceSfs reconciles the Oracle Restart StatefulSet template with
// the desired specification, creating or updating it as needed.
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
	updateReason := ""
	result, err := sharedk8sobjects.ReconcileStatefulSet(ctx, r.Client, oracleRestart.Namespace, dep, func(found, desired *appsv1.StatefulSet) bool {
		foundRes := found.Spec.Template.Spec.Containers[0].Resources
		depRes := desired.Spec.Template.Spec.Containers[0].Resources
		resourcesChanged := !reflect.DeepEqual(foundRes, depRes)
		if !(resourcesChanged || configmapChanged) {
			return false
		}
		switch {
		case resourcesChanged && configmapChanged:
			updateReason = "resource and configmap change"
		case resourcesChanged:
			updateReason = "resource change"
		case configmapChanged:
			updateReason = "configmap change"
		default:
			updateReason = "unknown"
		}
		// Preserve existing metadata/status while applying desired spec/template.
		found.Labels = desired.Labels
		found.Annotations = desired.Annotations
		found.Spec = desired.Spec
		return true
	})
	if err != nil {
		oracleRestart.Spec.IsFailed = true
		reqLogger.Error(err, "Failed to reconcile StatefulSet", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
		return ctrl.Result{}, err
	}
	if result.Created {
		r.updateOracleRestartInstStatus(&oracleRestart, ctx, req, oracleRestart.Spec.InstDetails,
			string(oraclerestartdb.OracleRestartProvisionState), r.Client, true)
		reqLogger.Info("Creating a StatefulSet Normally", "StatefulSetName", dep.Name)
		if !isLast {
			return ctrl.Result{}, nil
		}
	}
	if result.Updated {
		reason := updateReason
		if reason == "" {
			reason = "resource and/or configmap change"
		}
		reqLogger.Info("Updating StatefulSet due to "+reason, "StatefulSetName", dep.Name)
	}

	return ctrl.Result{}, nil
}

// getAsmAutoUpdateForDisk checks if a given ASM disk is configured for auto-update based on the OracleRestart spec.
func getAsmAutoUpdateForDisk(
	instance *oraclerestartdb.OracleRestart,
	disk string,
) bool {

	for _, dg := range instance.Spec.AsmStorageDetails {

		if strings.EqualFold(dg.AutoUpdate, "true") {

			for _, d := range dg.Disks {
				if d == disk {
					return true
				}
			}

		} else {

			for _, d := range dg.Disks {
				if d == disk {
					return false
				}
			}

		}
	}

	// default if not found
	return false
}

// createOrReplaceSfsAsm updates the StatefulSet when ASM changes require pod
// recycling or spec adjustments.
func (r *OracleRestartReconciler) createOrReplaceSfsAsm(
	ctx context.Context,
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldSpec *oraclerestartdb.OracleRestartSpec,
	discoverySuccessful bool,
	configMapChange bool,
) (ctrl.Result, error) {

	reqLogger := r.Log.WithValues(
		"oracleRestart.Namespace", oracleRestart.Namespace,
		"oracleRestart.Name", oracleRestart.Name,
	)

	// --------------------------------------------------
	// Get existing StatefulSet
	// --------------------------------------------------
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: oracleRestart.Namespace,
	}, found)
	if err != nil {
		reqLogger.Error(err, "Failed to find existing StatefulSet")
		return ctrl.Result{}, err
	}
	// Determine AutoUpdate for changed disks
	asmAutoUpdate := true
	addedAsmDisks, removedAsmDisks, err := r.computeDiskChanges(oracleRestart, oldSpec)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, d := range addedAsmDisks {
		if getAsmAutoUpdateForDisk(oracleRestart, d) {
			asmAutoUpdate = true
			break
		} else {
			asmAutoUpdate = false
			break
		}
	}
	for _, d := range removedAsmDisks {
		if getAsmAutoUpdateForDisk(oracleRestart, d) {
			asmAutoUpdate = true
			break
		} else {
			asmAutoUpdate = false
			break
		}
	}

	// if !asmAutoUpdate {
	// 	for _, d := range removedAsmDisks {
	// 		if getAsmAutoUpdateForDisk(oracleRestart, d) {
	// 			asmAutoUpdate = true
	// 			break
	// 		} else {
	// 			asmAutoUpdate = false
	// 			break
	// 		}
	// 	}
	// }

	// isDelete := false
	inUse := false

	// --------------------------------------------------
	// VALIDATE REMOVED DISKS
	// --------------------------------------------------
	if len(removedAsmDisks) > 0 {

		OraRacSpex := oracleRestart.Spec.InstDetails

		racSfSet, err :=
			oraclerestartcommon.CheckSfset(
				OraRacSpex.Name,
				oracleRestart,
				r.Client,
			)
		if err != nil {
			return ctrl.Result{}, err
		}

		podList, err :=
			oraclerestartcommon.GetPodList(
				racSfSet.Name,
				oracleRestart,
				r.Client,
				OraRacSpex,
			)
		if err != nil {
			return ctrl.Result{}, err
		}

		if len(podList.Items) == 0 {
			return ctrl.Result{}, fmt.Errorf("no pods found")
		}

		// Get ASM state
		podName := podList.Items[len(podList.Items)-1].Name

		asmInstanceStatus :=
			oraclerestartcommon.GetAsmInstState(
				podName,
				oracleRestart,
				0,
				r.kubeClient,
				r.kubeConfig,
				r.Log,
			)

		// asmInstanceStatus = oracleRestart.Status.AsmDiskGroups
		// isDelete = true
		for _, removedDisk := range removedAsmDisks {

			rd := strings.TrimSpace(removedDisk)

			for _, dg := range asmInstanceStatus {

				for _, asmDisk := range dg.Disks {

					// SPLIT HERE (this was missing)
					diskList := strings.Split(asmDisk.Name, ",")

					for _, d := range diskList {

						if rd == strings.TrimSpace(d) {
							reqLogger.Error(err, "Failed to remove disk in use", "Disk", rd, "DiskGroup", dg.Name)
							return ctrl.Result{}, fmt.Errorf(
								"disk '%s' is part of diskgroup '%s'",
								removedDisk,
								dg.Name,
							)
						}
					}
				}
			}
		}

	}

	// --------------------------------------------------
	// Ensure StatefulSet updated
	// --------------------------------------------------
	err = r.ensureStatefulSetUpdated(
		ctx,
		reqLogger,
		oracleRestart,
		dep,
		asmAutoUpdate,
		configMapChange,
		req,
	)

	if err != nil {
		oracleRestart.Spec.IsFailed = true
		return ctrl.Result{}, err
	}

	// --------------------------------------------------
	// WAIT FOR PODS RUNNING
	// --------------------------------------------------
	timeout := time.After(15 * time.Minute)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {

		case <-timeout:
			return ctrl.Result{},
				fmt.Errorf("timeout waiting pods")

		case <-ticker.C:

			podList, err :=
				oraclerestartcommon.GetPodList(
					dep.Name,
					oracleRestart,
					r.Client,
					oracleRestart.Spec.InstDetails,
				)
			if err != nil {
				return ctrl.Result{}, err
			}

			ready, _, _ :=
				oraclerestartcommon.PodListValidation(
					podList,
					dep.Name,
					oracleRestart,
					r.Client,
				)

			if ready {
				goto podsReady
			}
		}
	}

podsReady:

	// --------------------------------------------------
	// DELETE PVC/PV for removed disks
	// --------------------------------------------------
	if isLast && !inUse {

		for _, removedDisk := range removedAsmDisks {

			_ = oraclerestartcommon.DelORestartPVC(
				oracleRestart,
				0, 0,
				removedDisk,
				r.Client,
				r.Log,
			)

			_ = oraclerestartcommon.DelORestartPv(
				oracleRestart,
				0, 0,
				removedDisk,
				r.Client,
				r.Log,
			)
		}
	}

	// --------------------------------------------------
	// ADD NEW DISKS TO ASM
	// --------------------------------------------------
	if isLast && asmAutoUpdate {

		dgToDisks := map[string][]string{}

		for _, disk := range addedAsmDisks {

			for _, dg := range oracleRestart.Spec.AsmStorageDetails {

				for _, d := range dg.Disks {

					if strings.TrimSpace(d) ==
						strings.TrimSpace(disk) {

						dgToDisks[dg.Name] =
							append(dgToDisks[dg.Name], disk)

						break
					}
				}
			}
		}

		for dgName, disks := range dgToDisks {

			reqLogger.Info(
				"Adding ASM disks",
				"dg", dgName,
				"disks", disks,
			)

			podList, err :=
				oraclerestartcommon.GetPodList(
					dep.Name,
					oracleRestart,
					r.Client,
					oracleRestart.Spec.InstDetails,
				)
			if err != nil {
				return ctrl.Result{}, err
			}

			err = r.addDisks(
				ctx,
				podList,
				oracleRestart,
				dgName,
				disks,
			)

			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

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

// cleanupOracleRestart removes Oracle Restart resources such as StatefulSets,
// services, and storage when the custom resource is being deleted.
func (r *OracleRestartReconciler) cleanupOracleRestart(
	req ctrl.Request,
	oracleRestart *oraclerestartdb.OracleRestart,
) error {
	// time.Sleep(15 * time.Minute)

	log := r.Log.WithValues("cleanupOracleRestart", req.NamespacedName)
	ctx := context.Background()
	oraRestartSpex := oracleRestart.Spec.InstDetails

	// --------------------------------------------------
	// Delete StatefulSet
	// --------------------------------------------------
	sfSetFound, err := oraclerestartcommon.CheckSfset(
		oraRestartSpex.Name, oracleRestart, r.Client)
	if err == nil && sfSetFound != nil {
		log.Info("Deleting ORestart StatefulSet", "Name", sfSetFound.Name)
		if err := r.Client.Delete(ctx, sfSetFound); err != nil {
			return err
		}
	}

	// --------------------------------------------------
	// Delete ConfigMap
	// --------------------------------------------------
	cmName := oraRestartSpex.Name + oracleRestart.Name + "-cmap"
	configMapFound, err := oraclerestartcommon.CheckConfigMap(
		oracleRestart, cmName, r.Client)
	if err == nil && configMapFound != nil {
		log.Info("Deleting Oracle Restart ConfigMap", "Name", configMapFound.Name)
		if err := r.Client.Delete(ctx, configMapFound); err != nil {
			return err
		}
	}

	// --------------------------------------------------
	// Delete DaemonSet
	// --------------------------------------------------
	daemonSet := &appsv1.DaemonSet{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      "disk-check-daemonset",
		Namespace: oracleRestart.Namespace,
	}, daemonSet)

	if err == nil {
		log.Info("Deleting DaemonSet", "Name", daemonSet.Name)
		if err := r.Client.Delete(ctx, daemonSet); err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	// --------------------------------------------------
	// Delete PVC + PV (ALWAYS, unless IsKeepPVC=true)
	// --------------------------------------------------
	if !utils.CheckStatusFlag(oraRestartSpex.IsKeepPVC) &&
		oracleRestart.Spec.AsmStorageDetails != nil {

		for pindex, dg := range oracleRestart.Spec.AsmStorageDetails {
			for cindex, disk := range dg.Disks {

				// Delete PVC first
				if err := oraclerestartcommon.DelORestartPVC(
					oracleRestart, pindex, cindex, disk, r.Client, r.Log); err != nil {
					return err
				}

				// Then delete PV (if it exists)
				if err := oraclerestartcommon.DelORestartPv(
					oracleRestart, pindex, cindex, disk, r.Client, r.Log); err != nil {
					return err
				}
			}
		}
	}

	// --------------------------------------------------
	// Delete Software PVC (unless IsKeepPVC=true)
	// --------------------------------------------------
	if !utils.CheckStatusFlag(oraRestartSpex.IsKeepPVC) {
		if err := oraclerestartcommon.DelRestartSwPvc(
			oracleRestart, r.Client, r.Log); err != nil {
			return err
		}
	}

	// --------------------------------------------------
	// Delete Services
	// --------------------------------------------------
	svcTypes := []string{"local", "lbservice", "nodeport"}
	for _, svcType := range svcTypes {
		svcFound, err := oraclerestartcommon.CheckORestartSvc(
			oracleRestart, svcType, oraRestartSpex, "", r.Client)

		if err == nil && svcFound != nil {
			log.Info("Deleting ORestart Service", "Name", svcFound.Name)
			if err := r.Client.Delete(ctx, svcFound); err != nil {
				return err
			}
		}
	}

	log.Info("Successfully cleaned up OracleRestart")
	return nil
}

// cleanupOracleRestartInstance tears down resources for an individual Oracle
// Restart instance, including StatefulSets, ConfigMaps, and storage artifacts.
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

// deleteOracleRestartInst removes an individual Oracle Restart instance and
// its associated Kubernetes resources when requested.
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

	var sfSetFound *appsv1.StatefulSet
	var svcFound *corev1.Service
	var configMapFound *corev1.ConfigMap

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
		err = oraclerestartcommon.DelRestartSwPvc(oracleRestart, r.Client, r.Log)
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

// IsStaticProvisioningUsed determines whether static provisioning should be
// assumed by checking for unnamed storage class usage or listing failures.
// IsStaticProvisioningUsed checks a storage class to determine whether static provisioning is configured.
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

// updateONS orchestrates ONS configuration updates across Oracle Restart pods
// based on the requested operation state string.
func (r *OracleRestartReconciler) updateONS(ctx context.Context, podList *corev1.PodList, instance *oraclerestartdb.OracleRestart, onsState string) error {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	for _, pod := range podList.Items {
		podName := pod.Name

		cmd := fmt.Sprintf("python3 /opt/scripts/startup/scripts/main.py --ons=%s", onsState)
		// reqLogger.Info("Executing command to update ONS", "Pod.Name", podName, "Command", cmd)

		stdout, stderr, err := oraclerestartcommon.ExecCommand(
			podName,
			[]string{"bash", "-c", cmd},
			oraclerestartcommon.NewExecCommandResp(r.kubeClient, r.kubeConfig),
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

// expandStorageClassSWVolume handles StorageClass expansion for the Oracle
// Restart software volume when config changes demand more space.
func (r *OracleRestartReconciler) expandStorageClassSWVolume(ctx context.Context, instance *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec) error {
	//reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	if oldSpec != nil {
		// fmt.Printf("Received OldSpec", oldSpec.InstDetails.SwLocStorageSizeInGb)
		if instance.Spec.InstDetails.SwLocStorageSizeInGb > oldSpec.InstDetails.SwLocStorageSizeInGb {
			fmt.Printf("Inside OldSpec and newSpec Change: old=%d new=%d\n", oldSpec.InstDetails.SwLocStorageSizeInGb, instance.Spec.InstDetails.SwLocStorageSizeInGb)
			storageClass := &storagev1.StorageClass{}
			pvc := &corev1.PersistentVolumeClaim{}

			if instance.Spec.SwStorageClass != "" {

				err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.SwStorageClass}, storageClass)
				if err != nil {
					return fmt.Errorf("error while fetching the storage class")
				}

				pvcName := oraclerestartcommon.GetSwPvcName(instance.Name, instance)
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

					fmt.Printf("New PvcSize set to %s\n", newPVCSizeAdd.String())
					if newPVCSizeAdd.Cmp(pvc.Spec.Resources.Requests["storage"]) < 0 {
						return fmt.Errorf("Resizing PVC to lower size volume not allowed")
					}

					pvc.Spec.Resources.Requests["storage"] = resource.MustParse(strconv.Itoa(instance.Spec.InstDetails.SwLocStorageSizeInGb) + "Gi")
					fmt.Printf("Updating PVC %s volume %s\n", pvc.Name, pvc.Spec.VolumeName)
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

// getDisksToRemoveStatus compares spec and status to determine which disks
// should be removed from ASM groups.
// getDisksToRemoveStatus derives the ASM disks requested for removal from Oracle Restart status fields.
func getDisksToRemoveStatus(instance *oraclerestartdb.OracleRestart) ([]string, error) {
	disksToRemove := []string{}
	disksToRemoveSet := make(map[string]struct{})

	for _, statusDG := range instance.Status.AsmDiskGroups {
		// Find matching group in spec
		var specDisks []string
		for _, specDG := range instance.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			continue
		}

		if len(specDisks) < len(statusDG.Disks) {
			// Unique disk names for status group
			statusDiskSet := make(map[string]struct{})
			statusDiskNames := make([]string, 0, len(statusDG.Disks))
			for _, disk := range statusDG.Disks {
				if disk.Name == "" {
					continue
				}
				if _, exists := statusDiskSet[disk.Name]; !exists {
					statusDiskSet[disk.Name] = struct{}{}
					statusDiskNames = append(statusDiskNames, disk.Name)
				}
			}

			groupDisksToRemove, err := findRacDisksToRemove(specDisks, statusDiskNames, instance)
			if err != nil {
				return groupDisksToRemove, fmt.Errorf("required disk is part of the disk group %s and cannot be removed. Review it manually", statusDG.Name)
			}
			for _, disk := range groupDisksToRemove {
				if _, exists := disksToRemoveSet[disk]; !exists {
					disksToRemoveSet[disk] = struct{}{}
					disksToRemove = append(disksToRemove, disk)
				}
			}
		}
	}

	return disksToRemove, nil
}

// findRacDisksToRemove identifies disks that exist in status but not in spec
// so they can be removed from ASM groups.
// findRacDisksToRemove compares spec and status disks to compute removals for Oracle Restart setups.
func findRacDisksToRemove(specDisks, statusDisks []string, instance *oraclerestartdb.OracleRestart) ([]string, error) {
	// Convert specDisks to a set for fast lookups
	specDiskSet := make(map[string]struct{})
	for _, disk := range specDisks {
		specDiskSet[disk] = struct{}{}
	}

	// Find disks in statusDisks that are not in specDiskSet (these would be removed)
	var disksToRemove []string
	for _, disk := range statusDisks {
		if _, found := specDiskSet[disk]; !found {
			disksToRemove = append(disksToRemove, disk)
		}
	}

	// Gather all disks from all disk groups in spec (for cross-group validation)
	combinedSet := make(map[string]struct{})
	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, disk := range dg.Disks {
			combinedSet[disk] = struct{}{}
		}
	}

	// Check that disks we want to remove aren't part of any other ASM disk group in the spec
	for _, disk := range disksToRemove {
		if _, found := combinedSet[disk]; found {
			return nil, fmt.Errorf("disk %s to be removed is part of a disk group, hence cannot be removed", disk)
		}
	}

	return disksToRemove, nil
}

// findRacDisksToAdd identifies which disks from the new specification should be added to the Oracle Restart database.
// It performs the following validations:
// 1. Checks if the new disk specification matches the old specification (no changes needed)
// 2. Detects duplicate disks within the new specification and returns an error if found
// 3. Filters out disks that already exist in the status (existing disks)
// 4. Validates that new disks are not already part of any individual ASM device list (CRS, RECO, REDO, DB)
//
// Parameters:
// - newSpecDisks: List of disk paths from the new specification
// - statusDisks: List of disks currently in status (may contain comma-separated values)
// - instance: The OracleRestart instance being processed
// - oldSpec: The previous specification to compare against
//
// Returns:
// - A slice of disk paths that are valid to be added
// - An error if duplicates are found in newSpecDisks or if a disk already exists in an ASM device list
// - nil if no new disks need to be added or all validations pass
// findRacDisksToAdd identifies ASM disks newly requested in spec compared to status and old spec.
func findRacDisksToAdd(newSpecDisks, statusDisks []string, instance *oraclerestartdb.OracleRestart, oldSpec *oraclerestartdb.OracleRestartSpec) ([]string, error) {
	// Create a set for statusDisks to allow valid reuse of existing disks
	// Step 1: Check for duplicates within newSpecDisks itself
	oldAsmDisks := flattenAsmDisks(oldSpec)

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

// getDisksToAddStatus reads status annotations to determine ASM disks pending addition.
func getDisksToAddStatus(instance *oraclerestartdb.OracleRestart) ([]string, error) {
	disksToAdd := []string{}
	disksToAddSet := make(map[string]struct{})

	for _, statusDG := range instance.Status.AsmDiskGroups {
		// // Find matching group in spec
		// if len(statusDG.Disks) == 0 {
		// 	continue
		// }
		var specDisks []string
		for _, specDG := range instance.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			continue
		}

		if len(specDisks) > len(statusDG.Disks) {
			// Unique disk names for status group
			statusDiskSet := make(map[string]struct{})
			for _, disk := range statusDG.Disks {
				if disk.Name != "" {
					statusDiskSet[disk.Name] = struct{}{}
				}
			}

			// Find disks in spec that are not in status
			for _, disk := range specDisks {
				if disk == "" {
					continue
				}
				if _, exists := statusDiskSet[disk]; !exists {
					if _, alreadyAdded := disksToAddSet[disk]; !alreadyAdded {
						disksToAddSet[disk] = struct{}{}
						disksToAdd = append(disksToAdd, disk)
					}
				}
			}
		}
	}

	return disksToAdd, nil
}

// Helper function to flatten all disk names in AsmStorageDetails
// flattenAsmDisks flattens nested ASM disk group definitions into a single slice of device names.
func flattenAsmDisks(oraclerestartdbSpec *oraclerestartdb.OracleRestartSpec) []string {
	var allDisks []string

	if oraclerestartdbSpec == nil {
		return allDisks
	}

	if oraclerestartdbSpec.AsmStorageDetails == nil {
		return allDisks
	}

	for _, dg := range oraclerestartdbSpec.AsmStorageDetails {
		if dg.Disks == nil {
			continue
		}
		allDisks = append(allDisks, dg.Disks...)
	}

	return allDisks
}

// getRACDisksChangedSpec returns ASM disks added or removed between current and previous Oracle Restart specs.
func getRACDisksChangedSpec(racDatabase oraclerestartdb.OracleRestart, oldSpec oraclerestartdb.OracleRestartSpec) ([]string, []string) {
	newAdapter := newOracleRestartAsmAdapter(&racDatabase, nil)
	oldObj := oraclerestartdb.OracleRestart{Spec: oldSpec}
	oldAdapter := newOracleRestartAsmAdapter(&oldObj, nil)
	return sharedasm.GetDisksChanged(newAdapter, oldAdapter)
}

// setRacDgFromStatusAndSpecWithMinimumDefaults ensures ASM disk group definitions include minimum default values.
func setRacDgFromStatusAndSpecWithMinimumDefaults(
	racDatabase *oraclerestartdb.OracleRestart,
	client client.Client,
	cName, fName string,
) error {
	return sharedasm.EnsureDefaults(newOracleRestartAsmAdapter(racDatabase, client), cName, fName)
}

// ensureCrsDiskGroup guarantees the CRS ASM disk group configuration exists and applies default parameters.
func ensureCrsDiskGroup(racDatabase *oraclerestartdb.OracleRestart, client client.Client, cName, fName string) {
	crsDgFound := false
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		// If name, redundancy, and type are missing but disks are provided,
		// treat it as a candidate for CRSDG populated from the response file.
		if dg.Name == "" && dg.Redundancy == "" && dg.Type == "" && len(dg.Disks) > 0 {
			name := lookupCrsDgResponseValue(racDatabase, client, cName, fName)
			redundancy := lookupRedundancyResponseValue(racDatabase, client, cName, fName)
			racDatabase.Spec.AsmStorageDetails[i].Name = name
			racDatabase.Spec.AsmStorageDetails[i].Redundancy = redundancy
			racDatabase.Spec.AsmStorageDetails[i].Type = oraclerestartdb.CrsAsmDiskDg
			crsDgFound = true
		} else if dg.Type == oraclerestartdb.CrsAsmDiskDg {
			// If type is already set to CRSDG, still fill defaults if missing
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = lookupCrsDgResponseValue(racDatabase, client, cName, fName)
			}
			if dg.Redundancy == "" {
				racDatabase.Spec.AsmStorageDetails[i].Redundancy = lookupRedundancyResponseValue(racDatabase, client, cName, fName)
			}
			crsDgFound = true
		}
	}

	if !crsDgFound {
		// Add default if no CRSDG found
		racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, oraclerestartdb.AsmDiskGroupDetails{
			Name:       "+DATA",
			Type:       oraclerestartdb.CrsAsmDiskDg,
			Redundancy: "EXTERNAL",
			Disks:      []string{},
		})
	}
}

// lookupCrsDgResponseValue retrieves CRS disk group values from response files or returns defaults.
func lookupCrsDgResponseValue(racDatabase *oraclerestartdb.OracleRestart, client client.Client, cName, fName string) string {
	name, err := oraclerestartcommon.CheckRspData(racDatabase, client, "oracle.install.asm.diskGroup.name", cName, fName)
	if err == nil && name != "" {
		return name
	}
	altName, errAlt := oraclerestartcommon.CheckRspData(racDatabase, client, "diskGroupName", cName, fName)
	if errAlt == nil && altName != "" {
		return altName
	}
	return "+DATA"
}

// lookupRedundancyResponseValue obtains redundancy settings from response files, defaulting when absent.
func lookupRedundancyResponseValue(racDatabase *oraclerestartdb.OracleRestart, client client.Client, cName, fName string) string {
	redundancy, err := oraclerestartcommon.CheckRspData(racDatabase, client, "redundancy", cName, fName)
	if err == nil && redundancy != "" {
		return redundancy
	}
	return "EXTERNAL"
}

// ensureDbDataDiskGroup validates or sets defaults for the database data ASM disk group.
func ensureDbDataDiskGroup(racDatabase *oraclerestartdb.OracleRestart) {
	var crsName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == oraclerestartdb.CrsAsmDiskDg {
			crsName = dg.Name
			break
		}
	}

	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == oraclerestartdb.DbDataDiskDg {
			// Set to CRS disk group name if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = crsName
			}
			return
		}
	}
	// Not found, add default, use CRS name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, oraclerestartdb.AsmDiskGroupDetails{
		Name: crsName, Type: oraclerestartdb.DbDataDiskDg,
	})
}

// ensureDbRecoveryDiskGroup validates or defaults the recovery ASM disk group configuration.
func ensureDbRecoveryDiskGroup(racDatabase *oraclerestartdb.OracleRestart) {
	var dataName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == oraclerestartdb.DbDataDiskDg {
			dataName = dg.Name
			break
		}
	}
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == oraclerestartdb.DbRecoveryDiskDg {
			// Set to DATA disk group if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = dataName
			}
			return
		}
	}
	// Not found, add default, use DATA name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, oraclerestartdb.AsmDiskGroupDetails{
		Name: dataName, Type: oraclerestartdb.DbRecoveryDiskDg,
	})
}

// ensureDefaultCharset assigns a default database character set when the spec omits one.
func ensureDefaultCharset(racDatabase *oraclerestartdb.OracleRestart) {
	if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.DbCharSet == "" {
		racDatabase.Spec.ConfigParams.DbCharSet = "AL32UTF8"
	}
}
