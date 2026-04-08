/*
** Copyright (c) 2022, 2026 Oracle and/or its affiliates.
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

// Package controllers houses the controller-runtime reconcilers for Oracle
// Database API groups. It manages Autonomous Databases, Autonomous Container
// Databases, database backups and restores, Data Guard broker resources,
// Oracle Restart, ORDS, LREST/LRPDB services, Single Instance databases,
// sharding deployments, and RAC clusters.
//
// Highlights:
//   - RAC controllers coordinate shared storage (ASM) and cluster resources
//     following docs/rac guidance.
//   - Autonomous family controllers integrate with OCI to provision and manage
//     ADB and ACD lifecycle events.
//   - DBCS, ORDS, Oracle Restart, and sharding reconcilers reuse helpers from
//     commons/ to assemble Kubernetes objects and interact with Oracle services.
//
// Support resources:
//   - Operator user guide: docs/rac and docs/adbs
//   - Kubernetes controller overview: https://kubernetes.io/docs/concepts/architecture/controller/
//
// Contribution references:
//   - Repository guidelines: https://github.com/oracle/oracle-database-operator/blob/main/CONTRIBUTING.md
//   - Example manifests: docs/rac/provisioning/racdb_prov_quickstart.yaml and docs/adbs
//
// Additional help:
//   - Issues tracker: https://github.com/oracle/oracle-database-operator/blob/main/README.md#help
//   - Sample CRD walkthroughs: docs/rac/README.md and docs/rac/provisioning
//
//nolint:staticcheck,unused,revive // legacy RAC reconciliation helpers/signatures are retained for compatibility.
package controllers

// This file implements the RacDatabaseReconciler, which manages the lifecycle of RacDatabase resources.
import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	v4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedasm "github.com/oracle/oracle-database-operator/commons/crs/asm"
	raccommon "github.com/oracle/oracle-database-operator/commons/crs/rac"
	utils "github.com/oracle/oracle-database-operator/commons/crs/rac/utils"
	shareddiskcheck "github.com/oracle/oracle-database-operator/commons/crs/shared/diskcheck"
	sharedenvfile "github.com/oracle/oracle-database-operator/commons/crs/shared/envfile"
	sharedorautil "github.com/oracle/oracle-database-operator/commons/crs/shared/orautil"
	sharedspecguard "github.com/oracle/oracle-database-operator/commons/crs/shared/specguard"
	sharedstatusmerge "github.com/oracle/oracle-database-operator/commons/crs/shared/statusmerge"
	sharedk8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// RacDatabaseReconciler reconciles `RacDatabase` resources defined in
// apis/database/v4 and orchestrates their associated Kubernetes primitives.
type RacDatabaseReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	Config     *rest.Config
	kubeClient kubernetes.Interface
	kubeConfig clientcmd.ClientConfig
	Recorder   record.EventRecorder
}

const racDatabaseFinalizer = "database.oracle.com/racdatabasefinalizer"
const racDiskCheckReadyTimeout = 15 * time.Minute
const racConfigMapHashAnnotation = "database.oracle.com/rac-configmap-hash"

var errRACDiskDiscoveryPending = errors.New("ASM disk discovery results not available yet")

type racReconcilePhase string

const (
	racPhaseInitAndFetch         racReconcilePhase = "InitAndFetch"
	racPhaseStateGuard           racReconcilePhase = "StateGuard"
	racPhaseDeletionAndIntent    racReconcilePhase = "DeletionAndIntent"
	racPhaseCleanup              racReconcilePhase = "Cleanup"
	racPhasePendingAndRecovery   racReconcilePhase = "PendingAndRecovery"
	racPhaseValidationAndDefault racReconcilePhase = "ValidationAndDefaults"
	racPhaseServiceSync          racReconcilePhase = "ServiceSync"
	racPhaseStorageSync          racReconcilePhase = "StorageSync"
	racPhaseWorkloadSync         racReconcilePhase = "WorkloadSync"
	racPhaseFinalize             racReconcilePhase = "Finalize"
)

const (
	racOpTypeAddNodes    = "ADD_NODES"
	racOpTypeDeleteNodes = "DELETE_NODES"
)

const (
	racBreakGlassOverrideAnnotation = "database.oracle.com/breakglass-override"
	racBreakGlassReasonAnnotation   = "database.oracle.com/breakglass-reason" // optional, for audit log context
	racBreakGlassActorAnnotation    = "database.oracle.com/breakglass-actor"  // optional, for audit log context
)

func (r *RacDatabaseReconciler) phaseLogger(req ctrl.Request, phase racReconcilePhase) logr.Logger {
	return r.Log.WithValues(
		"controller", "racdatabase",
		"namespace", req.Namespace,
		"name", req.Name,
		"phase", string(phase),
	)
}

func (r *RacDatabaseReconciler) phaseInfo(req ctrl.Request, phase racReconcilePhase, msg string, keysAndValues ...interface{}) {
	r.phaseLogger(req, phase).Info(msg, keysAndValues...)
}

func (r *RacDatabaseReconciler) phaseError(req ctrl.Request, phase racReconcilePhase, err error, msg string, keysAndValues ...interface{}) {
	r.phaseLogger(req, phase).Error(err, msg, keysAndValues...)
}

func markRACFailedStatus(obj *racdb.RacDatabase) {
	if obj == nil {
		return
	}
	if obj.Status.State == "" {
		obj.Status.State = string(racdb.RACFailedState)
	}
	obj.Status.DbState = string(racdb.RACFailedState)
}

func effectiveAsmStorageClassForDG(dg racdb.AsmDiskGroupDetails, globalStorageClass string) string {
	if sc := strings.TrimSpace(dg.StorageClass); sc != "" {
		return sc
	}
	return strings.TrimSpace(globalStorageClass)
}

func isRawAsmDiskGroup(dg racdb.AsmDiskGroupDetails, globalStorageClass string) bool {
	return effectiveAsmStorageClassForDG(dg, globalStorageClass) == ""
}

func hasAnyRawAsmDiskGroup(spec *racdb.RacDatabaseSpec) bool {
	if spec == nil {
		return false
	}
	for i := range spec.AsmStorageDetails {
		if isRawAsmDiskGroup(spec.AsmStorageDetails[i], spec.StorageClass) {
			return true
		}
	}
	return false
}

func clearRACFailedStatus(obj *racdb.RacDatabase) {
	if obj == nil {
		return
	}
	if obj.Status.State == string(racdb.RACFailedState) {
		obj.Status.State = string(racdb.RACUpdateState)
	}
}

func isRACFailedStatus(obj *racdb.RacDatabase) bool {
	if obj == nil {
		return false
	}
	return obj.Status.State == string(racdb.RACFailedState) ||
		obj.Status.DbState == string(racdb.RACFailedState)
}

//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="database.oracle.com",resources=racdatabases/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods;pods/log;pods/exec;secrets;serviceaccounts;endpoints;services;events;configmaps;persistentvolumes;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups='',resources=statefulsets/finalizers,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the controller-runtime loop for `RacDatabase` resources.
// It orchestrates the lifecycle described in docs/rac/provisioning/racdb_prov_quickstart.yaml, including:
//
// 1. Resource Retrieval and Validation
//   - Fetches the RacDatabase resource from the cluster
//   - Validates that cluster-level RAC configuration is present
//
// 2. Status Initialization
//   - Initializes ConfigParams and status fields with default values
//   - Sets up Kubernetes client configuration for RAC operations
//
// 3. Deletion Management
//   - Handles RacDatabase resource deletion
//   - Performs cleanup of RAC instances based on old specifications
//   - Validates RAC object state via webhooks (when enabled)
//
// 4. Service Creation
//   - Creates or replaces Kubernetes Services for RAC components:
//   - VIP services for virtual IPs
//   - Local services for node-local connectivity
//   - SCAN services for Oracle SCAN listeners
//   - External services for ONS and listeners (based on port configuration)
//   - NodePort services for external access
//
// 5. ASM Storage Management
//   - Detects new setups, upgrades, and disk changes
//   - Runs disk-check DaemonSet for ASM disk discovery
//   - Creates and updates PersistentVolumes and PersistentVolumeClaims
//   - Handles ASM disk addition and removal (exclusive operations)
//
// 6. Configuration and StatefulSet Creation
//   - Generates ConfigMaps with RAC configuration parameters
//   - Creates or updates StatefulSets for RAC database instances
//   - Supports ASM disk changes with automatic configuration updates
//
// 7. Post-Reconciliation Steps
//   - Updates current specification annotations
//   - Defers status update via updateReconcileStatus
//
// Returns a ctrl.Result indicating whether requeuing is needed and any errors encountered.
// Uses a 60-second requeue interval for most operations and 10-second intervals during disk discovery.
func (r *RacDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	//ctx := context.Background()
	_ = r.Log.WithValues("racdatabase", req.NamespacedName)

	r.phaseInfo(req, racPhaseInitAndFetch, "Reconcile requested")
	var result ctrl.Result
	var err error
	completed := false
	blocked := false
	var nilErr error = nil
	phase := racPhaseInitAndFetch
	resultNq := ctrl.Result{Requeue: false}
	resultQ := ctrl.Result{Requeue: true, RequeueAfter: 60 * time.Second}
	// time.Sleep(50000 * time.Second)

	racDatabase := &racdb.RacDatabase{}

	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, racDatabase)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Resource not found")
			return requeueN, nil
		}
		r.Log.Error(err, err.Error())
		return resultQ, err
	}
	r.phaseInfo(req, phase, "Entering reconcile phase")
	// Kube Client Config Setup
	if r.kubeConfig == nil && r.kubeClient == nil {
		r.kubeConfig, r.kubeClient, err = raccommon.GetRacK8sClientConfig(r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if err = validateRACSpecLayout(&racDatabase.Spec); err != nil {
		r.phaseError(req, phase, err, "Invalid RAC spec layout")
		return resultQ, err
	}
	// Execute for every reconcile except deletion where it give error in logs
	if racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		defer r.updateReconcileStatus(racDatabase, ctx, req, &result, &err, &blocked, &completed)
	}

	// Retrieve the old spec from annotations
	oldSpec, err := r.GetOldSpec(racDatabase)
	if err != nil {
		r.Log.Error(err, "Failed to update old spec annotation")
		return resultQ, nil
	}

	phase = racPhaseStateGuard
	r.phaseInfo(req, phase, "Entering reconcile phase")
	webhooksEnabled := os.Getenv("ENABLE_WEBHOOKS") != "false"
	if racDatabase.GetDeletionTimestamp() == nil && webhooksEnabled {
		err = checkRACStateAndReturn(racDatabase)
		if err != nil {
			blocked = true
			result = resultQ
			r.phaseInfo(req, phase, "RAC object is in restricted state, returning back")
			return result, nilErr
		}
	}

	// Initialize racDatabase.Status if it's not already initialized
	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = &racdb.RacInitParams{}
	}

	// Initialize ConfigParams fields if they are not already initialized
	if racDatabase.Status.ConfigParams.DbHome == "" {
		racDatabase.Status.ConfigParams.DbHome = string(racdb.RACFieldNotDefined)
	}

	if racDatabase.Status.DbState == "" {
		racDatabase.Status.State = string(racdb.RACPendingState)
		racDatabase.Status.DbState = string(racdb.RACPendingState)
		racDatabase.Status.Role = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConnectString = string(racdb.RACFieldNotDefined)
		racDatabase.Status.PdbConnectString = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ReleaseUpdate = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConfigParams.DbHome = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ConfigParams.GridHome = string(racdb.RACFieldNotDefined)
		racDatabase.Status.ClientEtcHost = []string{string(racdb.RACFieldNotDefined)}
		if err := r.Status().Update(ctx, racDatabase); err != nil {
			return resultNq, err
		}
	}

	phase = racPhaseDeletionAndIntent
	r.phaseInfo(req, phase, "Entering reconcile phase")
	// Manage RACDatabase Deletion , if delete topology is called
	deletionHandled, err := r.manageRacDatabaseDeletion(req, ctx, racDatabase)
	if err != nil {
		result = resultNq
		return result, err
	}
	if deletionHandled {
		r.phaseInfo(req, phase, "Deletion handled by finalizer path; skipping normal reconcile")
		return resultNq, nil
	}

	addingNodes, deletingNodes := detectRACNodeOperationIntent(racDatabase, oldSpec)
	if addingNodes && deletingNodes {
		blocked = true
		result = resultQ
		err = fmt.Errorf("invalid reconcile intent: node add and node delete cannot run together in the same spec update")
		r.phaseError(req, phase, err, "Controller-level guard blocked mixed node operations")
		return result, nilErr
	}
	operationType := deriveRACOperationType(addingNodes, deletingNodes)
	lockHeld := false
	if operationType != "" {
		if lerr := r.acquireRACOperationLock(ctx, req, racDatabase, oldSpec, operationType, string(phase)); lerr != nil {
			blocked = true
			result = resultQ
			r.phaseInfo(req, phase, "Operation lock conflict; requeueing", "operation", operationType, "error", lerr.Error())
			return result, nilErr
		}
		lockHeld = true
		defer func() {
			if !lockHeld {
				return
			}
			if completed || err != nil || racDatabase.GetDeletionTimestamp() != nil {
				if lerr := r.releaseRACOperationLock(ctx, req, operationType); lerr != nil {
					r.phaseError(req, racPhaseFinalize, lerr, "Failed to release RAC operation lock", "operation", operationType)
				}
			}
		}()
	}

	phase = racPhaseCleanup
	r.phaseInfo(req, phase, "Entering reconcile phase")
	// cleanup RAC Instance
	_, err = r.cleanupRacInstance(req, ctx, racDatabase, effectiveOldSpec(oldSpec))
	if err != nil {
		result = resultQ
		r.phaseInfo(req, phase, err.Error())
		return result, nilErr
	}

	phase = racPhasePendingAndRecovery
	r.phaseInfo(req, phase, "Entering reconcile phase")
	podList := &corev1.PodList{}

	err = r.List(ctx, podList,
		client.InNamespace(req.Namespace),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedPods := podsOwnedByRacDatabase(podList.Items, racDatabase)

	handled, err := updatePendingStateIfAny(ctx, r, racDatabase, ownedPods)
	if err != nil {
		return ctrl.Result{}, err
	}
	if handled {
		r.phaseInfo(req, phase, "Some RAC pods are Pending; requeueing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if racDatabase.Status.State == string(racdb.RACFailedState) &&
		racDatabase.Generation != racDatabase.Status.ObservedGeneration {

		r.Log.Info("Spec updated after FAILED state — allowing recovery")

		racDatabase.Status.State = string(racdb.RACUpdateState)

		err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
			latest.Status = racDatabase.Status
		})
		if err != nil {
			r.phaseError(req, phase, err, "Failed to update status while recovering from FAILED")
			return resultQ, err
		}
	}

	if !webhooksEnabled {
		r.Log.Info("Webhooks disabled — skipping RAC state validation")
	}

	// If the object is being deleted, stop reconcile here
	if racDatabase.GetDeletionTimestamp() != nil {
		r.phaseInfo(req, phase, "RacDatabase is being deleted, skipping normal reconcile")
		return ctrl.Result{}, nil
	}
	result, completed, err = r.runRACProvisionPhases(
		ctx, req, racDatabase, oldSpec, resultNq, resultQ,
	)
	return result, err
}

func (r *RacDatabaseReconciler) runRACProvisionPhases(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec,
	resultNq ctrl.Result,
	resultQ ctrl.Result,
) (ctrl.Result, bool, error) {
	var (
		svcType       string
		err           error
		phase         racReconcilePhase
		configMapData       = make(map[string]string)
		nilErr        error = nil
		completed     bool  = false
	)

	phase = racPhaseValidationAndDefault
	r.phaseInfo(req, phase, "Entering reconcile phase")
	cName, fName := resolveGridOrDBResponseFileRef(racDatabase.Spec.ConfigParams)

	err = setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(racDatabase, r.Client, cName, fName)
	if err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return ctrl.Result{}, completed, err
	}

	err = r.validateSpex(racDatabase, oldSpec, ctx)
	if err != nil {
		r.Log.Info("Spec validation failed")
		r.Log.Info(err.Error())
		return resultQ, completed, nilErr
	}

	err = r.updateGiConfigParamStatus(racDatabase)
	if err != nil {
		r.Log.Info(err.Error())
		return resultQ, completed, nilErr
	}

	err = r.updateDbConfigParamStatus(racDatabase)
	if err != nil {
		r.Log.Info(err.Error())
		return resultQ, completed, nilErr
	}

	phase = racPhaseServiceSync
	r.phaseInfo(req, phase, "Entering reconcile phase")
	if racDatabase.Spec.ExternalSvcType != nil {
		svcType = *racDatabase.Spec.ExternalSvcType
	} else {
		svcType = "nodeport"
	}

	cd := racDatabase.Spec.ClusterDetails
	for i := 0; i < cd.NodeCount; i++ {
		if _, err = r.createOrReplaceService(ctx, racDatabase,
			raccommon.BuildClusterServiceDefForRac(racDatabase, cd, i, "vip")); err != nil {
			return resultNq, completed, err
		}

		if _, err = r.createOrReplaceService(ctx, racDatabase,
			raccommon.BuildClusterServiceDefForRac(racDatabase, cd, i, "local")); err != nil {
			return resultNq, completed, err
		}

		if cd.BaseOnsTargetPort > 0 {
			if _, err = r.createOrReplaceService(ctx, racDatabase,
				raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "onssvc")); err != nil {
				return resultNq, completed, err
			}
		}

		if cd.BaseLsnrTargetPort > 0 {
			if _, err = r.createOrReplaceService(ctx, racDatabase,
				raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "lsnrsvc")); err != nil {
				return resultNq, completed, err
			}
		}
		if i == 0 {
			if _, err = r.createOrReplaceService(ctx, racDatabase,
				raccommon.BuildClusterExternalServiceDefForRac(racDatabase, cd, i, svcType, "scansvc")); err != nil {
				return resultNq, completed, err
			}
		}

	}
	if _, err = r.createOrReplaceService(ctx, racDatabase,
		raccommon.BuildClusterServiceDefForRac(racDatabase, cd, 0, "scan")); err != nil {
		return resultNq, completed, err
	}

	r.ensureAsmStorageStatus(racDatabase)
	phase = racPhaseStorageSync
	r.phaseInfo(req, phase, "Entering reconcile phase")

	isNewSetup := true
	upgradeSetup := false

	if oldSpec != nil && oldSpec.OldAsmStorageDetails != nil {
		upgradeSetup = true
		isNewSetup = false
		r.Log.Info("Detected upgrade scenario — marking upgradeSetup = true")
	} else {
		for _, diskgroup := range racDatabase.Status.AsmDiskGroups {
			if len(diskgroup.Disks) > 0 && diskgroup.Name != "Pending" {
				isNewSetup = false
				break
			}
		}
	}

	isDiskChanged := false
	addedAsmDisks := []string{}
	removedAsmDisks := []string{}

	if !isNewSetup && oldSpec != nil {
		addedAsmDisks, removedAsmDisks, err = r.computeDiskChanges(racDatabase, oldSpec)
		if err != nil {
			return ctrl.Result{}, completed, err
		}

		if len(addedAsmDisks) > 0 && len(removedAsmDisks) > 0 {
			r.Log.Info("Detected addition as well as deletion; cannot process both together",
				"addedAsmDisks", addedAsmDisks, "removedAsmDisks", removedAsmDisks)
			return resultQ, completed, fmt.Errorf("cannot add and remove ASM disks in the same step")
		}

		if len(addedAsmDisks) > 0 {
			r.Log.Info("Detected addition of ASM disks", "addedAsmDisks", addedAsmDisks)
			isDiskChanged = true
		}
		if len(removedAsmDisks) > 0 {
			r.Log.Info("Detected removal of ASM disks", "removedAsmDisks", removedAsmDisks)
			isDiskChanged = true
		}
	}

	missingSize := false
	for _, dg := range racDatabase.Status.AsmDiskGroups {
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
	hasRawDiskGroups := hasAnyRawAsmDiskGroup(&racDatabase.Spec)

	shouldRunDiscovery :=
		hasRawDiskGroups &&
			(len(removedAsmDisks) == 0) && (isNewSetup ||
			upgradeSetup ||
			missingSize ||
			len(addedAsmDisks) > 0 ||
			len(racDatabase.Status.AsmDiskGroups) == 0)

	var disks []racdb.AsmDiskStatus

	if shouldRunDiscovery {
		r.Log.Info("ASM discovery decision",
			"removed", len(removedAsmDisks),
			"added", len(addedAsmDisks),
			"missingSize", missingSize,
			"isNewSetup", isNewSetup,
			"upgradeSetup", upgradeSetup,
			"dgCount", len(racDatabase.Status.AsmDiskGroups),
		)

		if err := r.createDaemonSet(racDatabase, ctx); err != nil {
			r.Log.Error(err, "failed to create disk-check daemonset")
			return ctrl.Result{}, completed, err
		}

		ready, err := checkRacDaemonSetStatusforRAC(ctx, r, racDatabase)
		if err != nil {
			r.Log.Error(err, "ASM disk-check daemonset status error, cleaning up")

			_ = r.cleanupDaemonSet(racDatabase, ctx)

			racDatabase.Status.State = string(racdb.RACFailedState)

			meta.SetStatusCondition(&racDatabase.Status.Conditions, metav1.Condition{
				Type:               string(racdb.RacCrdReconcileErrorState),
				Status:             metav1.ConditionTrue,
				Reason:             string(racdb.RacCrdReconcileErrorReason),
				Message:            err.Error(),
				ObservedGeneration: racDatabase.Generation,
				LastTransitionTime: metav1.Now(),
			})

			err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
				latest.Status = racDatabase.Status
			})
			if err != nil {
				return resultNq, completed, err
			}

			return resultNq, completed, nil
		}

		if !ready {
			r.Log.Info("ASM disks not ready yet. Waiting for disk-check daemonset to complete discovery.")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, completed, nil
		}

		disks, err = r.updateDiskSizes(ctx, racDatabase)
		if err != nil {
			if errors.Is(err, errRACDiskDiscoveryPending) {
				r.Log.Info("ASM disk discovery output is not available yet. Waiting for disk-check daemonset logs.")
				return ctrl.Result{RequeueAfter: 10 * time.Second}, completed, nil
			}
			r.Log.Error(err, "failed updating disk sizes")
			return ctrl.Result{}, completed, err
		}
	}

	if len(racDatabase.Status.AsmDiskGroups) == 0 && hasRawDiskGroups {
		return resultNq, completed, fmt.Errorf("no ASM disk group status available")
	}
	err = setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(racDatabase, r.Client, cName, fName)
	if err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return ctrl.Result{}, completed, err
	}

	diskStatusMap := make(map[string]racdb.AsmDiskStatus)
	for _, d := range disks {
		diskStatusMap[d.Name] = d
	}

	for dgIndex, dgSpec := range racDatabase.Spec.AsmStorageDetails {
		groupName := dgSpec.Name
		dgType := dgSpec.Type
		dgIsRaw := isRawAsmDiskGroup(dgSpec, racDatabase.Spec.StorageClass)

		if dgType == racdb.OthersDiskDg {
			for diskIdx, diskName := range dgSpec.Disks {
				var sizeStr string
				if dgIsRaw {
					diskStatus, ok := diskStatusMap[diskName]
					if !ok || !diskStatus.Valid || diskStatus.SizeInGb == 0 {
						continue
					}
					sizeStr = fmt.Sprintf("%dGi", diskStatus.SizeInGb)
					pv := raccommon.VolumePVForASM(
						racDatabase, dgIndex, diskIdx,
						diskName, groupName, sizeStr,
					)
					if _, _, err = r.createOrReplaceAsmPv(ctx, racDatabase, pv, string(dgType)); err != nil {
						return resultNq, completed, err
					}
				} else {
					if dgSpec.AsmStorageSizeInGb == 0 {
						r.Log.Info("ASM disk group storage size not set for storage class provisioning, skipping", "diskGroup", groupName, "disk", diskName)
						continue
					}
					sizeStr = fmt.Sprintf("%dGi", dgSpec.AsmStorageSizeInGb)
				}

				pvc := raccommon.VolumePVCForASM(
					racDatabase, dgIndex, diskIdx,
					diskName, groupName, sizeStr,
				)
				if _, _, err = r.createOrReplaceAsmPvC(ctx, racDatabase, pvc, string(dgType)); err != nil {
					return resultNq, completed, err
				}
			}
			continue
		}

		var dgStatus *racdb.AsmDiskGroupStatus
		for i, dgSt := range racDatabase.Status.AsmDiskGroups {
			if dgSt.Name == groupName {
				dgStatus = &racDatabase.Status.AsmDiskGroups[i]
				break
			}
		}
		if dgStatus == nil && dgIsRaw {
			r.Log.Info("ASM disk group not present in status, skipping", "diskGroup", groupName)
			continue
		}

		for diskIdx, diskName := range dgSpec.Disks {
			var diskStatus *racdb.AsmDiskStatus
			if dgIsRaw {
				for i, d := range dgStatus.Disks {
					if d.Name == diskName {
						diskStatus = &dgStatus.Disks[i]
						break
					}
				}
				if diskStatus == nil || !diskStatus.Valid || diskStatus.SizeInGb == 0 {
					continue
				}
			}

			var sizeStr string
			if dgIsRaw {
				sizeStr = fmt.Sprintf("%dGi", diskStatus.SizeInGb)
			} else {
				if dgSpec.AsmStorageSizeInGb == 0 {
					r.Log.Info("ASM disk group storage size not set for storage class provisioning, skipping", "diskGroup", groupName, "disk", diskName)
					continue
				}
				sizeStr = fmt.Sprintf("%dGi", dgSpec.AsmStorageSizeInGb)
			}

			if dgIsRaw {
				pv := raccommon.VolumePVForASM(
					racDatabase, dgIndex, diskIdx,
					diskName, groupName, sizeStr,
				)
				if _, _, err = r.createOrReplaceAsmPv(ctx, racDatabase, pv, string(dgType)); err != nil {
					return resultNq, completed, err
				}
			}

			pvc := raccommon.VolumePVCForASM(
				racDatabase, dgIndex, diskIdx,
				diskName, groupName, sizeStr,
			)
			if _, _, err = r.createOrReplaceAsmPvC(ctx, racDatabase, pvc, string(dgType)); err != nil {
				return resultNq, completed, err
			}
		}
	}

	if hasRawDiskGroups {
		err = r.cleanupDaemonSet(racDatabase, ctx)
		if err != nil {
			return resultQ, completed, nilErr
		}
	}

	phase = racPhaseWorkloadSync
	r.phaseInfo(req, phase, "Entering reconcile phase")

	if racDatabase.Spec.ConfigParams != nil {
		configMapData, err = r.generateConfigMap(racDatabase)
		if err != nil {
			return resultNq, completed, err
		}
	}
	if usesClusterRACSpec(&racDatabase.Spec) && racDatabase.Spec.ClusterDetails != nil {
		cd := racDatabase.Spec.ClusterDetails
		isDiskChangedNew := isDiskChanged && !isNewSetup
		if err = raccommon.CreateServiceAccountIfNotExists(racDatabase, r.Client); err != nil {
			return resultNq, completed, err
		}

		for i := 0; i < cd.NodeCount; i++ {

			isLast := i == int(cd.NodeCount)-1
			nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
			cmName := nodeName + racDatabase.Name + "-cmap"

			switch {
			case isNewSetup || !isDiskChangedNew:

				cm := raccommon.ConfigMapSpecs(racDatabase, configMapData, cmName)
				if _, err = r.createConfigMap(ctx, racDatabase, cm); err != nil {
					return resultNq, completed, err
				}

				spec := raccommon.BuildStatefulSpecForRacCluster(racDatabase, cd, i, r.Client)
				dep := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeName,
						Namespace: racDatabase.Namespace,
						Labels:    raccommon.BuildLabelsForRac(racDatabase, "RAC"),
					},
					Spec: *spec,
				}

				if _, err = r.createOrReplaceSfs(
					ctx, req, racDatabase, dep, i, isLast, racDatabase.Status.State,
				); err != nil {
					return resultNq, completed, err
				}

			case isDiskChangedNew && !isNewSetup:

				configMapDataAutoUpdate, err :=
					r.generateConfigMapAutoUpdateCluster(ctx, racDatabase, cmName)
				if err != nil {
					return resultNq, completed, err
				}

				if _, err = r.updateConfigMap(ctx, racDatabase, configMapDataAutoUpdate, cmName); err != nil {
					return resultNq, completed, err
				}

				r.Log.Info("ConfigMap updated successfully with new ASM disk details (new-style cluster mode)")

				spec := raccommon.BuildStatefulSpecForRacCluster(racDatabase, cd, i, r.Client)
				dep := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nodeName,
						Namespace: racDatabase.Namespace,
						Labels:    raccommon.BuildLabelsForRac(racDatabase, "RAC"),
					},
					Spec: *spec,
				}

				if _, err = r.createOrReplaceSfsAsmCluster(
					ctx, req, racDatabase, dep, i, isLast, oldSpec,
				); err != nil {
					return resultNq, completed, err
				}
			}
		}

	}
	completed = true
	phase = racPhaseFinalize
	r.phaseInfo(req, phase, "Entering reconcile phase")
	if err = r.SetCurrentSpecAndObservedGeneration(ctx, racDatabase, req); err != nil {
		r.Log.Error(err, "Failed to persist current spec / observed generation")
		return resultQ, completed, err
	}
	r.phaseInfo(req, phase, "Reconcile completed")
	return resultNq, completed, nil
}

// podsOwnedByRacDatabase returns pods owned by the specified RAC database based on naming and owner references.
func podsOwnedByRacDatabase(pods []corev1.Pod, racdb *racdb.RacDatabase) []corev1.Pod {
	var owned []corev1.Pod

	// Determine RAC node name prefix
	var nodePrefix string
	if racdb.Spec.ClusterDetails != nil {
		nodePrefix = racdb.Spec.ClusterDetails.RacNodeName
	}

	for _, pod := range pods {

		// Namespace must match
		if pod.Namespace != racdb.Namespace {
			continue
		}

		// Pod name must match RAC naming convention: <racNodeName>-<ordinal>
		// Examples: racnode1-0, racnode2-0
		if nodePrefix != "" && strings.HasPrefix(pod.Name, nodePrefix) {
			owned = append(owned, pod)
			continue
		}

		// Fallback: match via StatefulSet owner reference name
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "StatefulSet" &&
				nodePrefix != "" &&
				strings.HasPrefix(ref.Name, nodePrefix) {
				owned = append(owned, pod)
				break
			}
		}
	}

	return owned
}

// updatePendingStateIfAny sets the RAC status to pending when owned pods remain pending.
func updatePendingStateIfAny(
	ctx context.Context,
	r *RacDatabaseReconciler,
	racDatabase *racdb.RacDatabase,
	pods []corev1.Pod,
) (bool, error) {

	var pendingPods []string
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			pendingPods = append(pendingPods, pod.Name)
		}
	}

	if len(pendingPods) == 0 {
		return false, nil
	}

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		// Always fetch latest version
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(racDatabase), latest); err != nil {
			return true, err
		}

		// ----- Update only fields owned by this logic -----
		latest.Status.State = string(racdb.RACPendingState)
		latest.Status.DbState = string(racdb.RACPendingState)

		// Merge or upsert condition (do NOT overwrite entire slice)
		cond := metav1.Condition{
			Type:               string(racdb.RacCrdReconcileWaitingState),
			Status:             metav1.ConditionTrue,
			Reason:             string(racdb.RacCrdReconcileWaitingReason),
			Message:            fmt.Sprintf("RAC pods pending: %v", pendingPods),
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.Now(),
		}
		meta.SetStatusCondition(&latest.Status.Conditions, cond)

		// Attempt update
		err := r.Status().Update(ctx, latest)
		if err == nil {
			// Success → copy back so caller continues with fresh object
			racDatabase.Status = latest.Status
			racDatabase.ResourceVersion = latest.ResourceVersion
			return true, nil
		}

		if apierrors.IsConflict(err) {
			r.Log.Info(
				"Conflict updating pending state, retrying",
				"attempt", attempt,
				"racDatabase", racDatabase.Name,
			)
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		// Non-conflict error → return immediately
		return true, err
	}

	return true, fmt.Errorf(
		"failed to update pending state after %d retries: %w",
		maxRetries, lastErr,
	)
}

// computeDiskChanges inspects spec and recorded status to find ASM disks
// that were added or removed since the last reconcile cycle. It blends spec
// deltas with runtime observations to decide follow-up operations.
func (r *RacDatabaseReconciler) computeDiskChanges(
	racDatabase *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec,
) (addedAsmDisks []string, removedAsmDisks []string, err error) {

	if oldSpec == nil {
		return nil, nil, nil
	}
	podList := &corev1.PodList{}
	_ = r.Client.List(context.TODO(), podList,
		client.InNamespace(racDatabase.Namespace),
	)

	ownedPods := podsOwnedByRacDatabase(podList.Items, racDatabase)

	if len(ownedPods) > 0 && !isRacSetupStable(racDatabase, ownedPods) {
		r.Log.Info(
			"RAC setup not stable yet — skipping ASM runtime disk diff",
			"state", racDatabase.Status.State,
			"pods", len(ownedPods),
		)
		// IMPORTANT: return only spec-based changes
		addedAsmDisks, removedAsmDisks = getRACDisksChangedSpecforRAC(*racDatabase, *oldSpec)
		return addedAsmDisks, removedAsmDisks, nil

	}

	// 1. Compare spec changes
	addedAsmDisks, removedAsmDisks = getRACDisksChangedSpecforRAC(*racDatabase, *oldSpec)

	// --- NEW: If any diskgroup's AutoUpdate toggled to true, perform ASM state diff ---
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		// Defensive: bounds/slice check for oldSpec
		if i >= len(oldSpec.AsmStorageDetails) {
			continue
		}
		if dg.Type == racdb.OthersDiskDg {
			continue
		}
		// if strings.ToLower(oldSpec.AsmStorageDetails[i].AutoUpdate) == "false" &&
		// 	strings.ToLower(dg.AutoUpdate) == "true" {

		// Get pod name of at least one active node (use 1st node; change as needed)
		sfsName := racDatabase.Spec.ClusterDetails.RacNodeName + "1"

		// Safely get statefulset and pod list
		racSfSet, err := raccommon.CheckSfset(sfsName, racDatabase, r.Client)
		if err != nil {
			return nil, nil, fmt.Errorf("failed retrieving StatefulSet '%s': %w", sfsName, err)
		}
		podList, err := r.getPodsForStatefulSet(context.TODO(), racDatabase, racSfSet.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot get pod list for ASM inspection: %w", err)
		}
		if len(podList.Items) == 0 {
			return nil, nil, fmt.Errorf("no pods available for ASM disk validation")
		}

		// Use the last pod (or any pod)
		podName := podList.Items[len(podList.Items)-1].Name

		// Use helper to query true ASM disk state for this diskgroup
		asmGroups := raccommon.GetAsmInstState(podName, racDatabase, i, r.kubeClient, r.kubeConfig, r.Log)

		// Find group for this dg
		var foundState racdb.AsmDiskGroupStatus
		found := false
		for _, group := range asmGroups {
			if group.Name == dg.Name {
				foundState = group
				found = true
				break
			}
		}

		// Build set of actually present disks
		asmPresentDisks := map[string]struct{}{}
		if found {
			for _, diskStatus := range foundState.Disks {
				asmPresentDisks[diskStatus.Name] = struct{}{}
			}
		}

		// For every disk in spec, if not present in ASM, add to "addedAsmDisks"
		for _, specDisk := range dg.Disks {
			if _, inASM := asmPresentDisks[specDisk]; !inASM {
				addedAsmDisks = append(addedAsmDisks, specDisk)
			}
		}
		// }
	}

	// 2. Include disks to add from status (unchanged)
	if disksToAdd, addErr := getDisksToAddStatusforRAC(racDatabase); addErr != nil {
		markRACFailedStatus(racDatabase)
		return nil, nil, fmt.Errorf("cannot get ASM disks to add: %w", addErr)
	} else if len(disksToAdd) > 0 && len(addedAsmDisks) == 0 {
		addedAsmDisks = disksToAdd
	}

	// 3. Include disks to remove from status (unchanged)
	if disksToRemove, removeErr := getDisksToRemoveStatusforRAC(racDatabase); removeErr != nil {
		markRACFailedStatus(racDatabase)
		return nil, nil, fmt.Errorf("cannot get ASM disks to remove: %w", removeErr)
	} else if len(disksToRemove) > 0 && len(removedAsmDisks) == 0 {
		removedAsmDisks = disksToRemove
	}

	// 4. Final check for disks pending to be mounted on cluster nodes (unchanged)
	pendingDisks := getPendingDisksToMount(racDatabase)
	if len(pendingDisks) > 0 {
		existing := make(map[string]bool)
		for _, disk := range addedAsmDisks {
			existing[disk] = true
		}
		for _, pdisk := range pendingDisks {
			if !existing[pdisk] {
				addedAsmDisks = append(addedAsmDisks, pdisk)
			}
		}
	}

	// 5. Validate removed disks using ASM runtime state (unchanged)
	if len(removedAsmDisks) > 0 {
		sfsName := racDatabase.Spec.ClusterDetails.RacNodeName + "1"
		racSfSet, err := raccommon.CheckSfset(sfsName, racDatabase, r.Client)
		if err != nil {
			return nil, nil, fmt.Errorf("failed retrieving StatefulSet '%s': %w", sfsName, err)
		}
		podList, err := r.getPodsForStatefulSet(context.TODO(), racDatabase, racSfSet.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot get pod list for ASM inspection: %w", err)
		}
		if len(podList.Items) == 0 {
			return nil, nil, fmt.Errorf("no pods available for ASM disk validation")
		}
		podName := podList.Items[len(podList.Items)-1].Name
		asmGroups := raccommon.GetAsmInstState(podName, racDatabase, 0, r.kubeClient, r.kubeConfig, r.Log)
		for _, removed := range removedAsmDisks {
			for _, group := range asmGroups {
				for _, disk := range group.Disks {
					if disk.Name == removed {
						return nil, nil, fmt.Errorf(
							"disk '%s' is still part of diskgroup '%s' and must be removed manually",
							removed, group.Name,
						)
					}
				}
			}
		}
	}

	return addedAsmDisks, removedAsmDisks, nil
}

// isRacSetupStable reports whether the RAC cluster is fully available with every pod ready.
func isRacSetupStable(racDatabase *racdb.RacDatabase, pods []corev1.Pod) bool {
	// State-based guard
	switch racdb.RacLifecycleState(racDatabase.Status.State) {
	case racdb.RACAvailableState:
		// only AVAILABLE is safe
	default:
		return false
	}

	// Pod-based guard
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status != corev1.ConditionTrue {
				return false
			}
		}
	}

	return true
}

// getPendingDisksToMount walks desired ASM disk entries and cross-checks
// mounted device information from status. It returns disks not yet mounted
// so the controller can trigger discovery or retry orchestration steps.
// getPendingDisksToMount returns ASM disks requested in spec but not yet mounted on any RAC node.
func getPendingDisksToMount(racDatabase *racdb.RacDatabase) []string {
	pending := make(map[string]bool)
	specDisks := map[string]bool{}
	// Gather all desired disks from spec
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		for _, disk := range dg.Disks {
			if disk != "" {
				specDisks[disk] = true
			}
		}
	}
	// For each node's mounted devices, mark disks as mounted
	mounted := map[string]bool{}
	for _, node := range racDatabase.Status.RacNodes { // assuming .RacNodes slice
		for _, disk := range node.NodeDetails.MountedDevices {
			if disk != "" {
				mounted[disk] = true
			}
		}
	}
	// If a disk exists in spec but is missing from mounted on any node, mark as pending
	for disk := range specDisks {
		if !mounted[disk] {
			pending[disk] = true
		}
	}
	// Flatten set to slice
	pendingList := []string{}
	for disk := range pending {
		pendingList = append(pendingList, disk)
	}
	return pendingList
}

// checkRACStateAndReturn guards reconcile operations against restricted
// lifecycle phases on the RAC object. It returns an error when the state or
// spec flags indicate work should not proceed.
// checkRACStateAndReturn blocks reconcile operations for restricted RAC lifecycle states.
func checkRACStateAndReturn(racDatabase *racdb.RacDatabase) error {

	// Block only transient in-progress states
	switch racDatabase.Status.State {
	case string(racdb.RACProvisionState),
		string(racdb.RACUpdateState),
		string(racdb.RACPodAvailableState),
		string(racdb.RACAddInstState),
		string(racdb.RACDeletingState):
		return fmt.Errorf("oracle RAC database is busy: %s", racDatabase.Status.State)

	case string(racdb.RACManualState):
		return fmt.Errorf("oracle RAC database is in manual state")
	}
	return nil
}

func usesClusterRACSpec(spec *racdb.RacDatabaseSpec) bool {
	return spec != nil && spec.ClusterDetails != nil
}

func validateRACSpecLayout(spec *racdb.RacDatabaseSpec) error {
	if !usesClusterRACSpec(spec) {
		return fmt.Errorf("invalid specification: clusterDetails is required")
	}
	return nil
}

// detectRACNodeOperationIntent derives coarse node-op intent from old/new specs.
// It is used as a controller-level guard to prevent mixed add+delete node operations.
func detectRACNodeOperationIntent(
	racDatabase *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec,
) (bool, bool) {
	if racDatabase == nil || oldSpec == nil {
		return false, false
	}
	if racDatabase.Spec.ClusterDetails == nil || oldSpec.ClusterDetails == nil {
		return false, false
	}
	newCount := racDatabase.Spec.ClusterDetails.NodeCount
	oldCount := oldSpec.ClusterDetails.NodeCount
	adding := newCount > oldCount
	deleting := newCount < oldCount
	return adding, deleting
}

func deriveRACOperationType(adding, deleting bool) string {
	if adding {
		return racOpTypeAddNodes
	}
	if deleting {
		return racOpTypeDeleteNodes
	}
	return ""
}

func effectiveOldSpec(oldSpec *racdb.RacDatabaseSpec) *racdb.RacDatabaseSpec {
	if oldSpec != nil {
		return oldSpec
	}
	return &racdb.RacDatabaseSpec{}
}

func resolveGridResponseFileRef(cfg *racdb.RacInitParams) (string, string) {
	if cfg == nil || cfg.GridResponseFile == nil {
		return "", ""
	}
	return cfg.GridResponseFile.ConfigMapName, cfg.GridResponseFile.Name
}

func resolveDBResponseFileRef(cfg *racdb.RacInitParams) (string, string) {
	if cfg == nil || cfg.DbResponseFile == nil {
		return "", ""
	}
	return cfg.DbResponseFile.ConfigMapName, cfg.DbResponseFile.Name
}

func resolveGridOrDBResponseFileRef(cfg *racdb.RacInitParams) (string, string) {
	cName, fName := resolveGridResponseFileRef(cfg)
	if cName != "" || fName != "" {
		return cName, fName
	}
	return resolveDBResponseFileRef(cfg)
}

func parseRACBreakGlassOverride(meta metav1.Object) (bool, string, string) {
	annotations := meta.GetAnnotations()
	if len(annotations) == 0 {
		return false, "", ""
	}
	if !strings.EqualFold(strings.TrimSpace(annotations[racBreakGlassOverrideAnnotation]), "true") {
		return false, "", ""
	}
	reason := strings.TrimSpace(annotations[racBreakGlassReasonAnnotation])
	actor := strings.TrimSpace(annotations[racBreakGlassActorAnnotation])
	return true, reason, actor
}

func racControllerLevelLockBypassAllowedFields() map[string]struct{} {
	// Maintain this allowlist in code when specific field-level lock bypasses are safe.
	// Example:
	// return map[string]struct{}{
	//   "spec.details.someNonDisruptiveField": {},
	// }
	return map[string]struct{}{}
}

func diffJSONPaths(prefix string, oldVal interface{}, newVal interface{}, out map[string]struct{}) {
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
			next := prefix + "." + k
			diffJSONPaths(next, oldMap[k], newMap[k], out)
		}
		return
	}
	// Keep list diffs stable and compact: mark at the list path itself.
	_, oldSliceOK := oldVal.([]interface{})
	_, newSliceOK := newVal.([]interface{})
	if oldSliceOK || newSliceOK {
		out[prefix] = struct{}{}
		return
	}
	out[prefix] = struct{}{}
}

func changedRACSpecPaths(oldSpec *racdb.RacDatabaseSpec, newSpec racdb.RacDatabaseSpec) ([]string, error) {
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
	diffJSONPaths("spec", oldObj, newObj, outSet)
	out := make([]string, 0, len(outSet))
	for k := range outSet {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func shouldBypassRACOperationLockBySpecDelta(latest *racdb.RacDatabase, oldSpec *racdb.RacDatabaseSpec) (bool, []string, error) {
	if latest == nil || oldSpec == nil {
		return false, nil, nil
	}
	allowed := racControllerLevelLockBypassAllowedFields()
	if len(allowed) == 0 {
		return false, nil, nil
	}
	changed, err := changedRACSpecPaths(oldSpec, latest.Spec)
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

func (r *RacDatabaseReconciler) acquireRACOperationLock(
	ctx context.Context,
	req ctrl.Request,
	_ *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec,
	operationType string,
	phase string,
) error {
	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)
	if operationType == "" {
		return nil
	}
	holder := req.NamespacedName.String()
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		latest := &racdb.RacDatabase{}
		if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		breakGlassEnabled, reason, actor := parseRACBreakGlassOverride(latest)
		if breakGlassEnabled {
			// Manual operator override: bypass lock acquisition and clear any existing lock.
			if latest.Status.Operation != nil {
				latest.Status.Operation = nil
				if err := r.Status().Update(ctx, latest); err != nil {
					if apierrors.IsConflict(err) {
						lastErr = err
						time.Sleep(retryDelay)
						continue
					}
					return err
				}
			}
			r.phaseInfo(req, racPhaseStateGuard, "Break-glass lock override enabled; skipping controller-level operation lock",
				"annotation", racBreakGlassOverrideAnnotation,
				"reason", reason,
				"actor", actor)
			return nil
		}
		existing := latest.Status.Operation
		if existing != nil && existing.Type != "" && existing.Type != operationType {
			bypassLock, changedPaths, bypassErr := shouldBypassRACOperationLockBySpecDelta(latest, oldSpec)
			if bypassErr != nil {
				return bypassErr
			}
			if bypassLock {
				r.phaseInfo(req, racPhaseStateGuard, "Bypassing RAC operation lock based on function-level spec-delta allowlist",
					"requestedOperation", operationType,
					"heldOperation", existing.Type,
					"heldBy", existing.Holder,
					"changedPaths", strings.Join(changedPaths, ","))
			} else {
				r.phaseInfo(req, racPhaseStateGuard, "RAC operation lock held by another operation",
					"heldOperation", existing.Type,
					"heldBy", existing.Holder,
					"heldGeneration", existing.TargetGeneration,
					"requestedOperation", operationType)
				return fmt.Errorf(
					"operation lock held by %s (holder=%s, generation=%d), requested=%s",
					existing.Type, existing.Holder, existing.TargetGeneration, operationType)
			}
		}
		needsFreshStart := existing == nil ||
			existing.Type != operationType ||
			existing.TargetGeneration != latest.Generation
		if latest.Status.Operation == nil {
			latest.Status.Operation = &racdb.RacOperationStatus{}
		}
		latest.Status.Operation.Type = operationType
		latest.Status.Operation.Holder = holder
		latest.Status.Operation.Phase = phase
		latest.Status.Operation.TargetGeneration = latest.Generation
		if needsFreshStart {
			latest.Status.Operation.StartedAt = metav1.Now()
		}

		if err := r.Status().Update(ctx, latest); err != nil {
			if apierrors.IsConflict(err) {
				lastErr = err
				time.Sleep(retryDelay)
				continue
			}
			return err
		}
		r.phaseInfo(req, racPhaseStateGuard, "Acquired RAC operation lock",
			"operation", operationType,
			"holder", holder,
			"generation", latest.Generation)
		return nil
	}
	return fmt.Errorf("failed to acquire operation lock after retries: %w", lastErr)
}

func (r *RacDatabaseReconciler) releaseRACOperationLock(
	ctx context.Context,
	req ctrl.Request,
	operationType string,
) error {
	if operationType == "" {
		return nil
	}
	holder := req.NamespacedName.String()
	err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
		if latest.Status.Operation == nil {
			return
		}
		if latest.Status.Operation.Type != operationType {
			return
		}
		if latest.Status.Operation.Holder != "" && latest.Status.Operation.Holder != holder {
			return
		}
		latest.Status.Operation = nil
	})
	if err == nil {
		r.phaseInfo(req, racPhaseFinalize, "Released RAC operation lock", "operation", operationType, "holder", holder)
	}
	return err
}

// generateConfigMapAutoUpdate reloads a RAC ConfigMap and refreshes its
// environment payload with current ASM device details. It returns the updated
// data map for reuse when persisting the ConfigMap.
func (r *RacDatabaseReconciler) generateConfigMapAutoUpdate(ctx context.Context, instance *racdb.RacDatabase, cmName string) (map[string]string, error) {
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

	// CRS_ASM_DEVICE_LIST
	if crsList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.CrsAsmDiskDg); crsList != "" {
		envVars["CRS_ASM_DEVICE_LIST"] = crsList
	}
	if recoList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbRecoveryDiskDg); recoList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = recoList
	}
	if redoList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.RedoDiskDg); redoList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = redoList
	}
	if dataList := raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbDataDiskDg); dataList != "" {
		envVars["DATA_ASM_DEVICE_LIST"] = dataList
	}
	configMapData["envfile"] = sharedenvfile.SerializeMap(envVars)

	return configMapData, nil
}

// generateConfigMapAutoUpdateCluster synthesizes cluster-scoped environment
// configuration for RAC nodes. It pulls latest disk information and returns
// an updated data map suited for ConfigMap persistence.
func (r *RacDatabaseReconciler) generateConfigMapAutoUpdateCluster(
	ctx context.Context,
	instance *racdb.RacDatabase,
	cmName string,
) (map[string]string, error) {

	// ---------------------------------------------------------
	// 1. Read existing ConfigMap
	// ---------------------------------------------------------
	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: instance.Namespace}, cm)
	if err != nil {
		return nil, err
	}

	r.Log.Info("Updating existing configmap (cluster-style ASM update)", "ConfigMap", cmName)

	configMapData := cm.Data
	envFile := configMapData["envfile"]

	// Parse into key=value map
	envVars := sharedenvfile.ParseMap(envFile)

	// ---------------------------------------------------------
	// 2. Extract ASMDG names + redundancy from Spec (NEW MODEL)
	// ---------------------------------------------------------
	crsDiskGroup := ""
	crsRedundancy := ""

	dataDgName := ""
	dataRedundancy := ""

	recoDgName := ""
	recoRedundancy := ""

	redoDgName := ""
	redoRedundancy := ""

	for _, dg := range instance.Spec.AsmStorageDetails {
		switch dg.Type {
		case racdb.CrsAsmDiskDg:
			if dg.Name != "" {
				crsDiskGroup = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				crsRedundancy = dg.Redundancy
			}

		case racdb.DbDataDiskDg:
			if dg.Name != "" {
				dataDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				dataRedundancy = dg.Redundancy
			}

		case racdb.DbRecoveryDiskDg:
			if dg.Name != "" {
				recoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				recoRedundancy = dg.Redundancy
			}

		case racdb.RedoDiskDg:
			if dg.Name != "" {
				redoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				redoRedundancy = dg.Redundancy
			}
		}
	}

	// Fallback default for CRS if not present
	if crsDiskGroup == "" {
		crsDiskGroup = "+DATA"
	}

	// ---------------------------------------------------------
	// 3. Build ASM DEVICE LISTS using runtime Status
	// ---------------------------------------------------------

	crsDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.CrsAsmDiskDg)
	dataDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.DbDataDiskDg)
	recoDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.DbRecoveryDiskDg)
	redoDeviceList := raccommon.GetAsmDevicesForCluster(instance, racdb.RedoDiskDg)

	// ---------------------------------------------------------
	// 4. Populate envVars (overwrite or add)
	// ---------------------------------------------------------
	// CRS
	envVars["CRS_ASM_DISKGROUP"] = crsDiskGroup
	if crsDeviceList != "" {
		envVars["CRS_ASM_DEVICE_LIST"] = crsDeviceList
	}
	if crsRedundancy != "" {
		envVars["CRS_ASMDG_REDUNDANCY"] = crsRedundancy
	}

	// DATA
	if dataDgName != "" {
		envVars["DB_DATA_FILE_DEST"] = dataDgName
	} else {
		envVars["DB_DATA_FILE_DEST"] = crsDiskGroup
	}

	if dataDeviceList != "" {
		envVars["DB_ASM_DEVICE_LIST"] = dataDeviceList
	}
	if dataRedundancy != "" {
		envVars["DB_ASMDG_PROPERTIES"] = "redundancy:" + dataRedundancy
	}

	// RECO
	if recoDgName != "" {
		envVars["DB_RECOVERY_FILE_DEST"] = recoDgName
	} else {
		envVars["DB_RECOVERY_FILE_DEST"] = crsDiskGroup
	}

	if recoDeviceList != "" {
		envVars["RECO_ASM_DEVICE_LIST"] = recoDeviceList
	}
	if recoRedundancy != "" {
		envVars["RECO_ASMDG_PROPERTIES"] = "redundancy:" + recoRedundancy
	}

	// REDO
	if redoDgName != "" {
		envVars["LOG_FILE_DEST"] = redoDgName
	}
	if redoDeviceList != "" {
		envVars["REDO_ASM_DEVICE_LIST"] = redoDeviceList
	}
	if redoRedundancy != "" {
		envVars["REDO_ASMDG_PROPERTIES"] = "redundancy:" + redoRedundancy
	}

	// Charset default
	if instance.Spec.ConfigParams.DbCharSet == "" {
		instance.Spec.ConfigParams.DbCharSet = "AL32UTF8"
	}
	envVars["DB_CHARACTERSET"] = instance.Spec.ConfigParams.DbCharSet

	// ---------------------------------------------------------
	// 5. Convert back to envfile format
	// ---------------------------------------------------------
	configMapData["envfile"] = sharedenvfile.SerializeMap(envVars)

	return configMapData, nil
}

// updateConfigMap writes revised RAC configuration into Kubernetes by
// updating the ConfigMap resource. It ensures the cluster sees the latest
// environment file contents derived during reconcile.
func (r *RacDatabaseReconciler) updateConfigMap(ctx context.Context, instance *racdb.RacDatabase, configMapData map[string]string, cmName string) (ctrl.Result, error) {
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

// updateReconcileStatus maintains reconcile conditions and topology details
// after each controller pass. It refreshes status fields based on reconcile
// outcome flags and observed cluster data.
func (r *RacDatabaseReconciler) updateReconcileStatus(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, result *ctrl.Result, err *error, blocked *bool, completed *bool) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	if racDatabase == nil || !racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		// object is deleted or being deleted; skip status update
		return
	}

	// First update RAC topology
	if racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {

		podNames, nodeDetails, err1 :=
			r.updateRacInstTopologyStatus(racDatabase, ctx, req)

		// ---- CASE 1: Pending pods → topology intentionally skipped ----
		if err1 == nil && len(podNames) == 0 {
			r.Log.Info(
				"RAC topology update skipped (pods pending)",
				"racDatabase", racDatabase.Name,
			)
			// Do NOT update DB topology
		}

		// ---- CASE 2: Hard error ----
		if err1 != nil {
			r.Log.Info(
				"RAC topology update encountered a non-fatal issue",
				"racDatabase", racDatabase.Name,
				"error", err1,
			)
		}

		// ---- CASE 3: Topology valid → update DB topology ----
		if len(podNames) > 0 {
			if err := r.updateRacDbTopologyStatus(
				racDatabase, ctx, req, podNames, nodeDetails,
			); err != nil {
				r.Log.Error(
					err,
					"Failed to update RAC DB topology",
					"racDatabase", racDatabase.Name,
				)
			}
		}
	}

	// ---------------------------------------------
	// CLEAN OLD RECONCILE CONDITIONS
	// ---------------------------------------------
	for _, t := range []string{
		string(racdb.RacCrdReconcileCompeleteState),
		string(racdb.RacCrdReconcileQueuedState),
		string(racdb.RacCrdReconcileWaitingState),
		string(racdb.RacCrdReconcileErrorState), // ← ADD THIS
	} {
		meta.RemoveStatusCondition(&racDatabase.Status.Conditions, t)
	}
	// ---------------------------------------------
	// BUILD NEW CONDITION
	// ---------------------------------------------
	errMsg := func() string {
		if *err != nil {
			return (*err).Error()
		}
		return "no reconcile errors"
	}()

	var condition metav1.Condition

	switch {
	case *completed:
		condition = metav1.Condition{
			Type:               string(racdb.RacCrdReconcileCompeleteState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileCompleteReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case *blocked:
		condition = metav1.Condition{
			Type:               string(racdb.RacCrdReconcileWaitingState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileWaitingReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case result.Requeue:
		condition = metav1.Condition{
			Type:               string(racdb.RacCrdReconcileQueuedState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileQueuedReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	case err != nil && *err != nil:
		condition = metav1.Condition{
			Type:               string(racdb.RacCrdReconcileErrorState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: racDatabase.GetGeneration(),
			Reason:             string(racdb.RacCrdReconcileErrorReason),
			Message:            errMsg,
			Status:             metav1.ConditionTrue,
		}

	default:
		return
	}
	// Keep transition time stable when reconcile condition content is unchanged.
	if prev := meta.FindStatusCondition(racDatabase.Status.Conditions, condition.Type); prev != nil &&
		prev.Status == condition.Status &&
		prev.Reason == condition.Reason &&
		prev.Message == condition.Message &&
		prev.ObservedGeneration == condition.ObservedGeneration {
		condition.LastTransitionTime = prev.LastTransitionTime
	}

	// ---------------------------------------------
	// SET ONLY THE NEW CONDITION
	// ---------------------------------------------
	meta.SetStatusCondition(&racDatabase.Status.Conditions, condition)

	if racDatabase.Status.State == string(racdb.RACPodAvailableState) && condition.Type == string(racdb.RacCrdReconcileCompeleteState) {
		r.Log.Info("All validations and updation are completed. Changing State to AVAILABLE")
		racDatabase.Status.State = string(racdb.RACAvailableState)
	}
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest version of the object
		latestInstance := &racdb.RacDatabase{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if apierrors.IsNotFound(err) {
			// Object already deleted, no update needed
			return
		}
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of RAC instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry fetching the latest instance
		}

		// Merge the instance fields into latestInstance
		err = mergeRacInstancesFromLatest(racDatabase, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to merge instances, retrying...")
			time.Sleep(retryDelay)
			continue // Retry merging
		}
		// Skip status write when nothing changed.
		if reflect.DeepEqual(racDatabase.Status, latestInstance.Status) {
			r.Log.Info("No RAC status changes detected; skipping status patch", "Instance", racDatabase.Name)
			return
		}

		// Update the ResourceVersion of instance from latestInstance to avoid conflict
		racDatabase.ResourceVersion = latestInstance.ResourceVersion
		err = r.Client.Status().Patch(ctx, racDatabase, client.MergeFrom(latestInstance))

		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected, retrying update...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				continue // Retry on conflict
			}
			r.Log.Error(err, "Failed to update the RAC DB instance, retrying...")
			time.Sleep(retryDelay)
			continue // Retry on other errors
		}

		// If update was successful, exit the loop
		r.Log.Info("Updated RAC instance status successfully", "Instance", racDatabase.Name)
		break
	}

	r.Log.Info("Returning from updateReconcileStatus")
	if racDatabase == nil || !racDatabase.ObjectMeta.DeletionTimestamp.IsZero() {
		r.Log.Info("Completed Rac Database Deletion, no more reconcile required")
	}
}

// validateSpex performs comprehensive validation of RAC database specs.
// It checks secrets, response files, network capacity, and ASM settings to
// ensure reconcile work proceeds with well-formed configuration.
func (r *RacDatabaseReconciler) validateSpex(racDatabase *racdb.RacDatabase, oldSpec *racdb.RacDatabaseSpec, ctx context.Context) error {
	var err error
	eventReason := "Spec Error"
	//var eventMsgs []string

	r.Log.Info("Entering reconcile validation")

	//First check image pull secrets
	if racDatabase.Spec.ImagePullSecret != "" {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: racDatabase.Spec.ImagePullSecret, Namespace: racDatabase.Namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Secret not found
				r.Recorder.Eventf(racDatabase, corev1.EventTypeWarning, eventReason, err.Error())
				r.Log.Info(err.Error())
				return err
			}
			r.Log.Error(err, err.Error())
			return err
		}
	}

	// ========  Config Params Checks
	// Checking Secret for ssh key
	privKeyFlag, pubKeyFlag := raccommon.GetSSHkey(racDatabase, racDatabase.Spec.SshKeySecret.Name, r.Client)
	if !privKeyFlag {
		return errors.New("private key name is not set to " + racDatabase.Spec.SshKeySecret.PrivKeySecretName + " in SshKeySecret")
	}
	if !pubKeyFlag {
		return errors.New("public key name is not set to " + racDatabase.Spec.SshKeySecret.PubKeySecretName + " in SshKeySecret")
	}

	// Checking Gi Responsefile
	cfg := racDatabase.Spec.ConfigParams

	// ---------------------------
	// GRID RESPONSE FILE CHECK
	// ---------------------------
	if cfg != nil && cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {

		giRspFlg, _ := raccommon.GetGiResponseFile(racDatabase, r.Client)

		if !giRspFlg {
			return errors.New("GridResponseFile name must be " + cfg.GridResponseFile.Name)
		}
	}

	// ---------------------------
	// DB RESPONSE FILE CHECK
	// ---------------------------
	if cfg != nil && cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {

		DbRspFlg, _ := raccommon.GetDbResponseFile(racDatabase, r.Client)

		if !DbRspFlg {
			return errors.New("DbResponseFile name must be " + cfg.DbResponseFile.Name)
		}
	}

	r.ensureAsmStorageStatus(racDatabase)
	_, diskRemoveErr := getDisksToRemoveStatusforRAC(racDatabase)
	if diskRemoveErr != nil {
		markRACFailedStatus(racDatabase)
		return diskRemoveErr
	}
	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// Find matching group in spec
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
			if specDG.Name == statusDG.Name {
				specDisks = specDG.Disks
				break
			}
		}
		if specDisks == nil {
			// Optionally skip or handle missing group
			continue
		}
		// Collect status side disk names for this group
		var statusDiskNames []string
		for _, disk := range statusDG.Disks {
			statusDiskNames = append(statusDiskNames, disk.Name)
		}
		// If number of disks in spec is greater than in status, we're trying to add new disks
		if len(specDisks) > len(statusDiskNames) {
			if _, err := findRacDisksToAddforRAC(specDisks, statusDiskNames, racDatabase, oldSpec); err != nil {
				return err
			}
		}
	}
	// Checking the network cards in response files

	cfg = racDatabase.Spec.ConfigParams

	// -----------------------------------------------------------------------------
	// 1. GRID RESPONSE FILE NETWORK INTERFACE VALIDATION (nil-safe)
	// -----------------------------------------------------------------------------
	if cfg != nil && cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {

		netRspData, err := raccommon.CheckRspData(
			racDatabase,
			r.Client,
			"networkInterfaceList",
			cfg.GridResponseFile.ConfigMapName,
			cfg.GridResponseFile.Name,
		)
		if err != nil {
			markRACFailedStatus(racDatabase)
			return err
		}

		clusterSpec := racDatabase.Spec.ClusterDetails
		if clusterSpec != nil {
			for _, iface := range clusterSpec.PrivateIPDetails {
				interfaceName := iface.Interface
				err = raccommon.ValidateNetInterface(interfaceName, racDatabase, netRspData)
				if err != nil {
					markRACFailedStatus(racDatabase)
					return fmt.Errorf(
						"The network card name '%s' does not match the interface list in the Grid Response File",
						interfaceName,
					)
				}
			}
		}
	}

	// -----------------------------------------------------------------------------
	// 2. GRID RESPONSE FILE EXISTENCE / DISK VALIDATION (nil-safe)
	// -----------------------------------------------------------------------------
	if cfg != nil && cfg.GridResponseFile != nil &&
		cfg.GridResponseFile.ConfigMapName != "" &&
		cfg.GridResponseFile.Name != "" {

		cm := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      cfg.GridResponseFile.ConfigMapName,
			Namespace: racDatabase.Namespace,
		}, cm)
		if err != nil {
			return fmt.Errorf("error fetching Grid ResponseFile ConfigMap: %v", err)
		}

		rspContent, exists := cm.Data[cfg.GridResponseFile.Name]
		if !exists {
			return fmt.Errorf(
				"response file %s not found in ConfigMap %s",
				cfg.GridResponseFile.Name, cfg.GridResponseFile.ConfigMapName,
			)
		}

		// --- Parse ASM disks from RSP file ---
		rspDisks := parseAsmDisksFromRsp(rspContent)

		// --- Collect disks from CRD spec ---
		specDisks := make(map[string]bool)
		for _, dg := range racDatabase.Spec.AsmStorageDetails {
			for _, disk := range dg.Disks {
				specDisks[disk] = true
			}
		}

		// --- Compare RSP disks with CRD spec ---
		for _, disk := range rspDisks {
			if !specDisks[disk] {
				return fmt.Errorf(
					"ASM disk %s appears in Grid Response File but is not listed in the CRD spec",
					disk,
				)
			}
		}
	}
	clusterSpec := racDatabase.Spec.ClusterDetails

	if clusterSpec != nil && len(clusterSpec.PrivateIPDetails) > 0 {
		for _, net := range clusterSpec.PrivateIPDetails {

			// Validate NAD capacity
			err := validateNadIPCapacity(
				clusterSpec.NodeCount,
				racDatabase.Namespace,
				net.Name,
				r.Client,
			)
			if err != nil {
				return field.Invalid(
					field.NewPath("spec").Child("clusterDetails").Child("privateIPDetails"),
					net.Name,
					err.Error(),
				)
			}
		}
	}

	r.Log.Info("Completed Validation of Spex")

	return nil

}

// validateNadIPCapacity confirms the target NAD provides enough IP addresses
// for the requested RAC node count. It parses the NAD IP range and raises
// errors when capacity is insufficient or malformed.
func validateNadIPCapacity(
	nodeCount int,
	namespace, nadName string,
	kClient client.Client,
) error {

	ipam, err := getNadIPAM(namespace, nadName, kClient)
	if err != nil {
		return fmt.Errorf("failed to read IPAM from NAD %s: %v", nadName, err)
	}

	startIP := net.ParseIP(ipam.RangeStart).To4()
	endIP := net.ParseIP(ipam.RangeEnd).To4()

	if startIP == nil || endIP == nil {
		return fmt.Errorf("invalid IPAM rangeStart or rangeEnd in NAD %s", nadName)
	}

	startInt := uint32(startIP[0])<<24 | uint32(startIP[1])<<16 | uint32(startIP[2])<<8 | uint32(startIP[3])
	endInt := uint32(endIP[0])<<24 | uint32(endIP[1])<<16 | uint32(endIP[2])<<8 | uint32(endIP[3])

	if endInt < startInt {
		return fmt.Errorf("NAD %s has an invalid IP range: start > end", nadName)
	}

	totalIPs := int(endInt-startInt) + 1

	if nodeCount > totalIPs {
		return fmt.Errorf(
			"NAD %s has insufficient IP range: only %d usable IPs, but RAC nodeCount=%d",
			nadName, totalIPs, nodeCount,
		)
	}

	return nil
}

// getNadIPAM retrieves the macvlan IPAM configuration from a NAD resource
// so callers can inspect available address ranges during validation logic.
func getNadIPAM(namespace, name string, kClient client.Client) (*racdb.MacvlanIPAM, error) {
	nad := &unstructured.Unstructured{}
	nad.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "k8s.cni.cncf.io",
		Version: "v1",
		Kind:    "NetworkAttachmentDefinition",
	})

	err := kClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, nad)
	if err != nil {
		return nil, err
	}

	configStr, found, err := unstructured.NestedString(nad.Object, "spec", "config")
	if err != nil || !found {
		return nil, fmt.Errorf("NAD spec.config not found")
	}

	var mv *racdb.MacvlanConfig
	if err := json.Unmarshal([]byte(configStr), &mv); err != nil {
		return nil, fmt.Errorf("failed to decode NAD config JSON: %v", err)
	}

	return &mv.IPAM, nil
}

// parseAsmDisksFromRsp scans a Grid response file blob to extract ASM disk
// entries. It normalizes disks listed under supported keys into a string slice.
func parseAsmDisksFromRsp(rspContent string) []string {
	var disks []string
	lines := strings.Split(rspContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var value string
		if strings.HasPrefix(line, "oracle.install.asm.diskGroup.disks=") {
			value = strings.TrimPrefix(line, "oracle.install.asm.diskGroup.disks=")
		} else if strings.HasPrefix(line, "diskList=") {
			value = strings.TrimPrefix(line, "diskList=")
		} else {
			continue
		}
		// Split and collect disks
		for _, d := range strings.Split(value, ",") {
			disk := strings.TrimSpace(d)
			if disk != "" {
				disks = append(disks, disk)
			}
		}
	}
	return disks
}

// getDisksToRemoveStatusforRAC compares spec vs. status to determine which
// ASM disks should be removed. It filters duplicates and validates against
// disk group membership before returning the removal list.
func getDisksToRemoveStatusforRAC(racDatabase *racdb.RacDatabase) ([]string, error) {
	disksToRemove := []string{}
	disksToRemoveSet := make(map[string]struct{})

	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// Find matching group in spec
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
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

			groupDisksToRemove, err := findRacDisksToRemoveforRAC(specDisks, statusDiskNames, racDatabase)
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

// getDisksToAddStatusforRAC inspects desired ASM disk groups against recorded
// status entries. It returns new disks that should be provisioned so the
// controller can expand storage appropriately.
func getDisksToAddStatusforRAC(racDatabase *racdb.RacDatabase) ([]string, error) {
	disksToAdd := []string{}
	disksToAddSet := make(map[string]struct{})

	for _, statusDG := range racDatabase.Status.AsmDiskGroups {
		// // Find matching group in spec
		// if len(statusDG.Disks) == 0 {
		// 	continue
		// }
		var specDisks []string
		for _, specDG := range racDatabase.Spec.AsmStorageDetails {
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

// flattenAsmDisksForRAC flattens all ASM disk references from the RAC spec
// into a single slice. Utilities reuse this to cross-check disk lists.
func flattenAsmDisksForRAC(racDbSpec *racdb.RacDatabaseSpec) []string {
	var allDisks []string

	if racDbSpec == nil {
		return allDisks
	}

	if racDbSpec.AsmStorageDetails == nil {
		return allDisks
	}

	for _, dg := range racDbSpec.AsmStorageDetails {
		if dg.Disks == nil {
			continue
		}
		allDisks = append(allDisks, dg.Disks...)
	}

	return allDisks
}

// createDaemonSet ensures the disk discovery DaemonSet exists with the
// expected spec. It creates or updates the workload so ASM disk metadata
// stays current across reconcile iterations.
func (r *RacDatabaseReconciler) createDaemonSet(racDatabase *racdb.RacDatabase, ctx context.Context) error {
	r.Log.Info("Validate New ASM Disks")

	// Build the desired DaemonSet (disk-check)
	desiredDaemonSet := raccommon.BuildDiskCheckDaemonSet(racDatabase)

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
				markRACFailedStatus(racDatabase)
				return err
			}
			r.Log.Info("DaemonSet created successfully", "DaemonSet.Name", desiredDaemonSet.Name)

		} else {
			markRACFailedStatus(racDatabase)
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

func diskCheckLabelSelectorForRAC(racDatabase *racdb.RacDatabase) string {
	return shareddiskcheck.LabelSelectorForDaemonSet(racDatabase, "disk-check")
}

func (r *RacDatabaseReconciler) collectDiskCheckResults(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
) ([]racdb.AsmDiskStatus, bool, error) {
	podList, err := r.kubeClient.CoreV1().Pods(racDatabase.Namespace).List(
		ctx,
		metav1.ListOptions{LabelSelector: diskCheckLabelSelectorForRAC(racDatabase)},
	)
	if err != nil {
		return nil, false, err
	}
	if len(podList.Items) == 0 {
		return nil, false, nil
	}

	expectedDisks := flattenAsmDisksForRAC(&racDatabase.Spec)
	if len(expectedDisks) == 0 {
		return nil, true, nil
	}

	discovered := make(map[string]racdb.AsmDiskStatus, len(expectedDisks))
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
			return nil, false, nil
		}

		rawLogs, err := r.kubeClient.CoreV1().Pods(pod.Namespace).GetLogs(
			pod.Name,
			&corev1.PodLogOptions{Container: "disk-check"},
		).DoRaw(ctx)
		if err != nil {
			r.Log.Info("Disk-check pod logs are not available yet", "pod", pod.Name)
			return nil, false, nil
		}
		if len(bytes.TrimSpace(rawLogs)) == 0 {
			continue
		}

		scanner := bufio.NewScanner(bytes.NewReader(rawLogs))
		for scanner.Scan() {
			var entry struct {
				Disk   string `json:"disk"`
				Valid  bool   `json:"valid"`
				SizeGb int    `json:"sizeGb"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			if strings.TrimSpace(entry.Disk) == "" {
				continue
			}
			discovered[entry.Disk] = racdb.AsmDiskStatus{
				Name:     entry.Disk,
				SizeInGb: entry.SizeGb,
				Valid:    entry.Valid,
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, false, err
		}
	}

	results := make([]racdb.AsmDiskStatus, 0, len(discovered))
	for _, diskName := range expectedDisks {
		diskName = strings.TrimSpace(diskName)
		if diskName == "" {
			continue
		}
		status, ok := discovered[diskName]
		if !ok {
			return nil, false, nil
		}
		results = append(results, status)
	}

	return results, true, nil
}

// updateDiskSizes refreshes ASM disk size information within status using
// latest metrics from disk discovery outputs. It aligns spec sizing with
// what was actually discovered on cluster nodes.
func (r *RacDatabaseReconciler) updateDiskSizes(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
) ([]racdb.AsmDiskStatus, error) {

	// 1. Collect discovered disks (ASM + OTHERS) directly from API server so
	// newly created disk-check pods are visible on the first successful run.
	disks, complete, err := r.collectDiskCheckResults(ctx, racDatabase)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, errRACDiskDiscoveryPending
	}

	// 2. Build ASM disk group status (exclude OTHERS)
	var diskGroups []racdb.AsmDiskGroupStatus

	for _, dgSpec := range racDatabase.Spec.AsmStorageDetails {

		// ---- FIX: Skip OTHERS in ASM status ----
		if dgSpec.Type == racdb.OthersDiskDg {
			continue
		}

		groupName := strings.TrimSpace(dgSpec.Name)
		if groupName == "" {
			groupName = "+DATA"
		}

		var groupDisks []racdb.AsmDiskStatus
		for _, diskName := range dgSpec.Disks {
			for _, d := range disks {
				if d.Name == diskName {
					groupDisks = append(groupDisks, d)
					break
				}
			}
		}

		if len(groupDisks) == 0 {
			continue
		}

		// Size consistency check
		if len(groupDisks) > 1 {
			expected := groupDisks[0].SizeInGb
			for _, gd := range groupDisks[1:] {
				if gd.SizeInGb != expected {
					return nil, fmt.Errorf(
						"disk group %q has mismatched disk sizes: disk %q = %dGB, expected %dGB",
						groupName, gd.Name, gd.SizeInGb, expected,
					)
				}
			}
		}

		diskGroups = append(diskGroups, racdb.AsmDiskGroupStatus{
			Name:         groupName,
			Redundancy:   dgSpec.Redundancy,
			Type:         dgSpec.Type,
			AutoUpdate:   dgSpec.AutoUpdate,
			StorageClass: dgSpec.StorageClass,
			Disks:        groupDisks,
		})
	}

	// 3. Persist ASM disk group status
	racDatabase.Status.AsmDiskGroups = diskGroups

	// 4. Patch status with retry
	const maxRetries = 3
	const retryDelay = 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(
			ctx,
			client.ObjectKey{
				Namespace: racDatabase.Namespace,
				Name:      racDatabase.Name,
			},
			latest,
		); err != nil {
			return nil, err
		}

		latest.Status.AsmDiskGroups = racDatabase.Status.AsmDiskGroups

		if err := mergeRacInstancesFromLatest(racDatabase, latest); err != nil {
			return nil, err
		}

		racDatabase.ResourceVersion = latest.ResourceVersion
		if err := r.Client.Status().Update(ctx, racDatabase); err != nil {
			if apierrors.IsConflict(err) {
				time.Sleep(retryDelay)
				continue
			}
			return nil, err
		}

		// Success
		return disks, nil
	}

	return nil, fmt.Errorf("failed to update disk sizes after %d retries", maxRetries)
}

// cleanupDaemonSet removes the temporary disk discovery DaemonSet when it is
// no longer required. This keeps the cluster clean once ASM updates finish.
func (r *RacDatabaseReconciler) cleanupDaemonSet(racDatabase *racdb.RacDatabase, ctx context.Context) error {
	// r.Log.Info("CleanupDaemonSet")
	desiredDaemonSet := raccommon.BuildDiskCheckDaemonSet(racDatabase)

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
				// r.Log.Info("DaemonSet deleted successfully", "DaemonSet.Name", existingDaemonSet.Name)
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

// findRacDisksToRemoveforRAC identifies disks present in status but missing
// from spec, ensuring they are safe to remove and not referenced elsewhere
// in the RAC configuration.
func findRacDisksToRemoveforRAC(
	specDisks []string,
	statusDisks []string,
	instance *racdb.RacDatabase,
) ([]string, error) {

	// ---------------------------------------
	// 1. Spec disks for THIS disk group
	// ---------------------------------------
	localSpecSet := make(map[string]struct{})
	for _, d := range specDisks {
		localSpecSet[d] = struct{}{}
	}

	// ---------------------------------------
	// 2. ALL spec disks across ALL disk groups
	// ---------------------------------------
	globalSpecSet := make(map[string]struct{})
	for _, dg := range instance.Spec.AsmStorageDetails {
		for _, d := range dg.Disks {
			globalSpecSet[d] = struct{}{}
		}
	}

	// ---------------------------------------
	// 3. Decide removals safely
	// ---------------------------------------
	var disksToRemove []string

	for _, disk := range statusDisks {

		// 🔒 HARD SAFETY:
		// If disk exists in ANY diskgroup spec → NEVER remove
		if _, exists := globalSpecSet[disk]; exists {
			continue
		}

		// Disk does not belong to this DG spec → removable
		if _, exists := localSpecSet[disk]; !exists {
			disksToRemove = append(disksToRemove, disk)
		}
	}

	return disksToRemove, nil
}

// findRacDisksToAddforRAC determines new ASM disks that should be provisioned
// by comparing updated specs against both prior spec and current status,
// rejecting duplicates or conflicting allocations.
func findRacDisksToAddforRAC(newSpecDisks, statusDisks []string, _ *racdb.RacDatabase, oldSpec *racdb.RacDatabaseSpec) ([]string, error) {
	// Create a set for statusDisks to allow valid reuse of existing disks
	// Step 1: Check for duplicates within newSpecDisks itself
	oldAsmDisks := flattenAsmDisksForRAC(oldSpec)

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

// updateGiConfigParamStatus ensures Grid Infrastructure parameters in status
// are populated by reading from response files or spec defaults when needed.
func (r *RacDatabaseReconciler) updateGiConfigParamStatus(racDatabase *racdb.RacDatabase) error {
	cName, fName := resolveGridResponseFileRef(racDatabase.Spec.ConfigParams)

	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = new(racdb.RacInitParams)
	}

	// --- GI Parameters (Inventory, GridBase, GridHome, ScanSvcName) ---

	if racDatabase.Status.ConfigParams.Inventory == "" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.Inventory != "" {
			racDatabase.Status.ConfigParams.Inventory = racDatabase.Spec.ConfigParams.Inventory
		} else {
			invlocation, err := raccommon.CheckRspData(racDatabase, r.Client, "INVENTORY_LOCATION", cName, fName)
			if err != nil {
				markRACFailedStatus(racDatabase)
				return errors.New("error in responsefile, unable to read INVENTORY_LOCATION")
			}
			racDatabase.Status.ConfigParams.Inventory = invlocation
		}
	}

	if racDatabase.Status.ConfigParams.GridBase == "" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.GridBase != "" {
			racDatabase.Status.ConfigParams.GridBase = racDatabase.Spec.ConfigParams.GridBase
		} else {
			gibase, err := raccommon.CheckRspData(racDatabase, r.Client, "ORACLE_BASE", cName, fName)
			if err != nil {
				markRACFailedStatus(racDatabase)
				return errors.New("error in responsefile, unable to read ORACLE_BASE")
			}
			racDatabase.Status.ConfigParams.GridBase = gibase
		}
	}

	if racDatabase.Status.ConfigParams.GridHome == "" || racDatabase.Status.ConfigParams.GridHome == "NOT_DEFINED" {
		if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.GridHome != "" {
			racDatabase.Status.ConfigParams.GridHome = racDatabase.Spec.ConfigParams.GridHome
		} else {
			gihome, err := raccommon.CheckRspData(racDatabase, r.Client, "GRID_HOME", cName, fName)
			if err != nil {
				markRACFailedStatus(racDatabase)
				return errors.New("error in responsefile, unable to read GRID_HOME")
			} else {
				racDatabase.Status.ConfigParams.GridHome = gihome
			}
		}
	}

	if racDatabase.Status.ScanSvcName == "" {
		if racDatabase.Spec.ScanSvcName != "" {
			racDatabase.Status.ScanSvcName = racDatabase.Spec.ScanSvcName
		} else {
			scanname, err := raccommon.CheckRspData(racDatabase, r.Client, "scanName", cName, fName)
			if err != nil {
				markRACFailedStatus(racDatabase)
				return errors.New("error in responsefile, unable to read scanName")
			} else {
				racDatabase.Status.ScanSvcName = scanname
			}
		}
	}

	return nil
}

// setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC harmonizes ASM disk
// group configuration by ensuring mandatory groups exist and default values
// populate when they are missing from spec or status.
func setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(
	racDatabase *racdb.RacDatabase,
	client client.Client,
	cName, fName string,
) error {
	return sharedasm.EnsureDefaults(newRacAsmAdapter(racDatabase, client), cName, fName)
}

// ensureCrsDiskGroupforRAC injects or enriches the CRS disk group entry
// derived from response files, guaranteeing type and redundancy defaults.
func ensureCrsDiskGroupforRAC(racDatabase *racdb.RacDatabase, client client.Client, cName, fName string) {
	crsDgFound := false
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		// If name, redundancy, and type are missing but disks are provided,
		// treat it as a candidate for CRSDG populated from the response file.
		if dg.Name == "" && dg.Redundancy == "" && dg.Type == "" && len(dg.Disks) > 0 {
			name := lookupCrsDgResponseValueforRAC(racDatabase, client, cName, fName)
			redundancy := lookupRedundancyResponseValueforRAC(racDatabase, client, cName, fName)
			racDatabase.Spec.AsmStorageDetails[i].Name = name
			racDatabase.Spec.AsmStorageDetails[i].Redundancy = redundancy
			racDatabase.Spec.AsmStorageDetails[i].Type = racdb.CrsAsmDiskDg
			crsDgFound = true
		} else if dg.Type == racdb.CrsAsmDiskDg {
			// If type is already set to CRSDG, still fill defaults if missing
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = lookupCrsDgResponseValueforRAC(racDatabase, client, cName, fName)
			}
			if dg.Redundancy == "" {
				racDatabase.Spec.AsmStorageDetails[i].Redundancy = lookupRedundancyResponseValueforRAC(racDatabase, client, cName, fName)
			}
			crsDgFound = true
		}
	}

	if !crsDgFound {
		// Add default if no CRSDG found
		racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
			Name:       "+DATA",
			Type:       racdb.CrsAsmDiskDg,
			Redundancy: "EXTERNAL",
			Disks:      []string{},
		})
	}
}

// lookupCrsDgResponseValueforRAC fetches disk group names from response
// files, falling back to defaults when explicit values are unavailable.
func lookupCrsDgResponseValueforRAC(
	racDatabase *racdb.RacDatabase,
	client client.Client,
	cName, fName string,
) string {

	normalize := func(val string) string {
		val = strings.TrimSpace(val)
		if val == "" {
			return ""
		}

		// DB rsp format: +DG/...
		if strings.HasPrefix(val, "+") {
			if idx := strings.Index(val, "/"); idx > 0 {
				return val[:idx]
			}
			return val
		}

		// GI rsp format: DG (no '+')
		if strings.Contains(val, "/") {
			// Defensive: CRSDATA/something → CRSDATA
			return strings.SplitN(val, "/", 2)[0]
		}

		return val
	}

	// --- GI response file keys ---
	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"oracle.install.asm.diskGroup.name",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"diskGroupName",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	// --- DB response file keys (ASM destinations) ---
	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"datafileDestination",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	if name, err := raccommon.CheckRspData(
		racDatabase, client,
		"db_create_file_dest",
		cName, fName,
	); err == nil {
		if dg := normalize(name); dg != "" {
			return dg
		}
	}

	return "+DATA"
}

// lookupRedundancyResponseValueforRAC reads redundancy settings from response
// files so disk groups receive consistent redundancy defaults.
func lookupRedundancyResponseValueforRAC(racDatabase *racdb.RacDatabase, client client.Client, cName, fName string) string {
	redundancy, err := raccommon.CheckRspData(racDatabase, client, "redundancy", cName, fName)
	if err == nil && redundancy != "" {
		return redundancy
	}
	return "EXTERNAL"
}

// ensureDbDataDiskGroupforRAC guarantees the DB data disk group exists and
// inherits sensible defaults derived from CRS group metadata.
func ensureDbDataDiskGroupforRAC(racDatabase *racdb.RacDatabase) {
	var crsName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.CrsAsmDiskDg {
			crsName = dg.Name
			break
		}
	}

	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbDataDiskDg {
			// Set to CRS disk group name if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = crsName
			}
			return
		}
	}
	// Not found, add default, use CRS name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
		Name: crsName, Type: racdb.DbDataDiskDg,
	})
}

// ensureDbRecoveryDiskGroupforRAC enforces the presence of a recovery disk
// group, aligning its naming with the primary data group when unspecified.
func ensureDbRecoveryDiskGroupforRAC(racDatabase *racdb.RacDatabase) {
	var dataName string
	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbDataDiskDg {
			dataName = dg.Name
			break
		}
	}
	for i, dg := range racDatabase.Spec.AsmStorageDetails {
		if dg.Type == racdb.DbRecoveryDiskDg {
			// Set to DATA disk group if blank
			if dg.Name == "" {
				racDatabase.Spec.AsmStorageDetails[i].Name = dataName
			}
			return
		}
	}
	// Not found, add default, use DATA name
	racDatabase.Spec.AsmStorageDetails = append(racDatabase.Spec.AsmStorageDetails, racdb.AsmDiskGroupDetails{
		Name: dataName, Type: racdb.DbRecoveryDiskDg,
	})
}

// ensureDefaultCharsetforRAC applies the default database character set when
// the spec omits it, avoiding host-side deployment issues.
func ensureDefaultCharsetforRAC(racDatabase *racdb.RacDatabase) {
	if racDatabase.Spec.ConfigParams != nil && racDatabase.Spec.ConfigParams.DbCharSet == "" {
		racDatabase.Spec.ConfigParams.DbCharSet = "AL32UTF8"
	}
}

// updateDbConfigParamStatus backs fills database-specific configuration
// values into status, reading from env files or response data when the spec
// doesn't provide them directly.
func (r *RacDatabaseReconciler) updateDbConfigParamStatus(
	racDatabase *racdb.RacDatabase,
) error {

	cName, fName := resolveDBResponseFileRef(racDatabase.Spec.ConfigParams)
	var rspData string
	cfg := racDatabase.Spec.ConfigParams

	if racDatabase.Spec.ConfigParams == nil {
		return nil
	}

	if racDatabase.Status.ConfigParams == nil {
		racDatabase.Status.ConfigParams = new(racdb.RacInitParams)
	}

	// Load response file ONCE (only if needed)
	loadRspData := func() (string, error) {
		if rspData != "" {
			return rspData, nil
		}
		data, err := raccommon.CheckRspData(
			racDatabase,
			r.Client,
			"variables=",
			cName,
			fName,
		)
		if err != nil {
			markRACFailedStatus(racDatabase)
			return "", fmt.Errorf("error in responsefile, unable to read variables")
		}
		rspData = data
		return rspData, nil
	}

	// DbName
	if racDatabase.Status.ConfigParams.DbName == "" {
		if cfg.DbName != "" {
			racDatabase.Status.ConfigParams.DbName = cfg.DbName
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			dbName := utils.GetValue(data, "DB_NAME")
			if dbName == "" {
				markRACFailedStatus(racDatabase)
				return fmt.Errorf("error in responsefile, unable to read DB_NAME")
			}
			racDatabase.Status.ConfigParams.DbName = dbName
		}
	}

	// DbBase (ORACLE_BASE)
	if racDatabase.Status.ConfigParams.DbBase == "" {
		if cfg.DbBase != "" {
			racDatabase.Status.ConfigParams.DbBase = cfg.DbBase
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			obase := utils.GetValue(data, "ORACLE_BASE")
			if obase == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_BASE")
			}
			racDatabase.Status.ConfigParams.DbBase = obase
		}
	}

	// DbHome (ORACLE_HOME)
	if racDatabase.Status.ConfigParams.DbHome == "" ||
		racDatabase.Status.ConfigParams.DbHome == "NOT_DEFINED" {

		if cfg.DbHome != "" {
			racDatabase.Status.ConfigParams.DbHome = cfg.DbHome
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			ohome := utils.GetValue(data, "ORACLE_HOME")
			if ohome == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_HOME")
			}
			racDatabase.Status.ConfigParams.DbHome = ohome
		}
	}

	// GridHome
	if racDatabase.Status.ConfigParams.GridHome == "" {
		if cfg.GridHome != "" {
			racDatabase.Status.ConfigParams.GridHome = cfg.GridHome
		} else {
			data, err := loadRspData()
			if err != nil {
				return err
			}
			ghome := utils.GetValue(data, "ORACLE_HOME")
			if ghome == "" {
				return fmt.Errorf("error in responsefile, unable to read ORACLE_HOME")
			}
			racDatabase.Status.ConfigParams.GridHome = ghome
		}
	}

	return nil
}

// updateRacInstTopologyStatus collects pod and node details for RAC
// instances and updates topology fields in status. It returns state needed
// by downstream topology validation logic.
func (r *RacDatabaseReconciler) updateRacInstTopologyStatus(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
) ([]string, map[string]*corev1.Node, error) {

	var (
		err         error
		pod         *corev1.Pod
		podNames    []string
		nodeDetails = make(map[string]*corev1.Node)
	)

	// -------------------------------------------------------------
	// STEP 0: HARD STOP if ANY RAC pod is Pending
	// -------------------------------------------------------------
	pending, err := hasPendingRacPods(ctx, r, racDatabase)
	if err != nil {
		return podNames, nodeDetails, err
	}

	if pending {
		r.Log.Info(
			"RAC topology validation skipped: pods are Pending",
			"racDatabase", racDatabase.Name,
		)

		err := r.updateStatusWithRetry(ctx, req, func(latest *racdb.RacDatabase) {
			latest.Status.State = string(racdb.RACPendingState)
			latest.Status.DbState = string(racdb.RACPendingState)
			meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
				Type:               string(racdb.RacCrdReconcileWaitingState),
				Status:             metav1.ConditionTrue,
				Reason:             string(racdb.RacCrdReconcileWaitingReason),
				Message:            "Waiting for all RAC pods to become Running",
				ObservedGeneration: latest.Generation,
				LastTransitionTime: metav1.Now(),
			})
		})
		if err != nil {
			return nil, nil, err
		}

		// DO NOT validate, DO NOT fail
		return podNames, nodeDetails, nil
	}

	// -------------------------------------------------------------
	// STEP 1: Validate ONLY when cluster is converged
	// -------------------------------------------------------------
	clusterSpec := racDatabase.Spec.ClusterDetails
	for index := 0; index < clusterSpec.NodeCount; index++ {

		_, pod, err = r.validateRacNodeCluster(
			racDatabase, ctx, req, clusterSpec, index,
		)
		if err != nil {
			return podNames, nodeDetails, err
		}
		if pod == nil {
			continue
		}

		podNames = append(podNames, pod.Name)

		node, err := r.getNodeDetails(pod.Spec.NodeName)
		if err != nil {
			return podNames, nodeDetails,
				fmt.Errorf("failed to get node details for pod %s: %w", pod.Name, err)
		}
		nodeDetails[pod.Name] = node
	}

	desiredNodes := map[string]struct{}{}
	for i := 0; i < clusterSpec.NodeCount; i++ {
		stsName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, i+1)
		podName := fmt.Sprintf("%s-0", stsName)
		desiredNodes[podName] = struct{}{}
	}

	filteredStatus := []*v4.RacNodeStatus{}
	for _, nodeStatus := range racDatabase.Status.RacNodes {
		if nodeStatus == nil {
			continue
		}
		if _, ok := desiredNodes[nodeStatus.Name]; ok {
			filteredStatus = append(filteredStatus, nodeStatus)
		} else {
			r.Log.Info("Pruning stale RAC node from status", "node", nodeStatus.Name)
		}
	}

	racDatabase.Status.RacNodes = filteredStatus
	err = r.updateStatusNoGetRetry(ctx, racDatabase)
	if err != nil {
		return nil, nil, err
	}

	// -------------------------------------------------------------
	// STEP 2: Final sanity (ONLY after convergence)
	// -------------------------------------------------------------
	if len(podNames) == 0 || len(nodeDetails) == 0 {
		// Not Pending → real failure
		markRACFailedStatus(racDatabase)
		return podNames, nodeDetails,
			errors.New("failed to collect RAC pod or node details")
	}

	clearRACFailedStatus(racDatabase)
	return podNames, nodeDetails, nil
}

func hasPendingRacPods(
	ctx context.Context,
	r *RacDatabaseReconciler,
	racDatabase *racdb.RacDatabase,
) (bool, error) {

	podList := &corev1.PodList{}

	err := r.List(ctx, podList,
		client.InNamespace(racDatabase.Namespace),
	)
	if err != nil {
		return false, err
	}

	for _, p := range podsOwnedByRacDatabase(podList.Items, racDatabase) {
		if p.Status.Phase == corev1.PodPending {
			return true, nil
		}
	}
	return false, nil
}

func (r *RacDatabaseReconciler) getPodsForStatefulSet(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	statefulSetName string,
) (*corev1.PodList, error) {
	allPods := &corev1.PodList{}
	if err := r.List(ctx, allPods, client.InNamespace(racDatabase.Namespace)); err != nil {
		return nil, err
	}

	filtered := &corev1.PodList{}
	prefix := statefulSetName + "-"
	for _, pod := range allPods.Items {
		if strings.HasPrefix(pod.Name, prefix) {
			filtered.Items = append(filtered.Items, pod)
			continue
		}
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "StatefulSet" && ref.Name == statefulSetName {
				filtered.Items = append(filtered.Items, pod)
				break
			}
		}
	}
	return filtered, nil
}

// validateRacNodeCluster inspects cluster-level spec fields, nodes, and
// network resources to verify topology prerequisites are satisfied before
// provisioning RAC structures.
func (r *RacDatabaseReconciler) validateRacNodeCluster(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, nodeIndex+1)
	racSfSet, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
	if err != nil {
		r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, string(racdb.RACProvisionState))
		return racSfSet, nil, err
	}
	if racSfSet == nil {
		return nil, nil, fmt.Errorf("StatefulSet for %s not found", nodeName)
	}
	podList, err := r.getPodsForStatefulSet(ctx, racDatabase, racSfSet.Name)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + raccommon.GetFmtStr(racSfSet.Name) + "."
		raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
		r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, string(racdb.RACProvisionState))
		return racSfSet, nil, err
	}

	isPodExist, racPod, notReadyPod := raccommon.PodListValidation(podList, racSfSet.Name, racDatabase, r.Client)
	if !isPodExist {
		var msg string
		if notReadyPod != nil {
			msg = "unable to validate RAC pod. The  pod not ready  is: " + notReadyPod.Name
		} else {
			msg = "unable to validate RAC pod. No pods matching the criteria were found"
		}
		raccommon.LogMessages("INFO", msg, nil, racDatabase, r.Log)
		return racSfSet, racPod, fmt.Errorf("%s", msg)
	}

	// Update status when PODs are ready
	state := racDatabase.Status.State
	if racDatabase.Spec.IsManual {
		state = string(racdb.RACManualState)
	}
	if isRACFailedStatus(racDatabase) {
		state = string(racdb.RACFailedState)
	}

	switch {
	case isPodExist && (state == string(racdb.RACProvisionState) ||
		state == string(racdb.RACUpdateState) ||
		state == string(racdb.RACPendingState)):
		state = string(racdb.RACPodAvailableState)
	case state == string(racdb.RACFailedState):
		state = string(racdb.RACFailedState)
	case state == string(racdb.RACManualState):
		state = string(racdb.RACManualState)
	default:
		state = racDatabase.Status.State
	}
	r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, nodeIndex, state)
	r.Log.Info("Completed Update of RAC cluster node status", "NodeName", nodeName)
	return racSfSet, racPod, nil
}

// getNodeDetails retrieves Kubernetes node metadata for RAC validation
// steps, allowing the controller to inspect node labels and status fields.
func (r *RacDatabaseReconciler) getNodeDetails(nodeName string) (*corev1.Node, error) {
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

// updateRacDbTopologyStatus synchronizes database-level topology status using
// pod placement and node details gathered earlier in the reconcile loop.
func (r *RacDatabaseReconciler) updateRacDbTopologyStatus(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node) error {

	//racPod := &corev1.Pod{}
	var err error
	_, _, err = r.validateRacDb(racDatabase, ctx, req, podNames, nodeDetails)
	if err != nil {
		return err
	}
	return nil
}

// validateRacDb checks database pods, placement, and configuration to ensure
// runtime topology aligns with expectations before status updates proceed.
func (r *RacDatabaseReconciler) validateRacDb(racDatabase *racdb.RacDatabase, ctx context.Context, req ctrl.Request, podNames []string, nodeDetails map[string]*corev1.Node,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	racSfSet := &appsv1.StatefulSet{}
	racPod := &corev1.Pod{}
	const maxRetries = 5
	const retryDelay = 2 * time.Second
	raccommon.UpdateRacDbStatusData(racDatabase, ctx, req, podNames, r.kubeClient, r.kubeConfig, r.Log, nodeDetails)
	// Log the start of the status update process
	r.Log.Info(
		"Updating RAC instance status with validateRacDb",
		"Instance", racDatabase.Name,
	)

	err := r.updateStatusNoGetRetry(ctx, racDatabase)
	if err != nil {
		r.Log.Error(err, "Failed to update RAC instance status with validateRacDb")
		return racSfSet, racPod, err
	}

	r.Log.Info(
		"Updated RAC instance status with validateRacDb",
		"Instance", racDatabase.Name,
	)

	return racSfSet, racPod, nil

}

// RacGetRestrictedFields lists immutable spec fields enforced by webhook
// validation to prevent unsupported manual edits.
func RacGetRestrictedFields() map[string]struct{} {
	return sharedspecguard.RestrictedConfigParamFields()
}

// mergeInstancesFromUpdated updates latestInstance with fields from updatedInstance
// except those that are restricted by the RacGetRestrictedFields function in align with webhooks.
// Assuming mergeInstancesFromUpdated merges instance details from updatedInstance to latestInstance.

// mergeRacInstancesFromLatest copies mutable fields from the latest object
// into the reconcile instance, ensuring status updates patch cleanly.
func mergeRacInstancesFromLatest(instance, latestInstance *racdb.RacDatabase) error {
	return sharedstatusmerge.MergeNamedStructField(
		instance,
		latestInstance,
		"Status",
		sharedstatusmerge.Options{
			PointerMode: sharedstatusmerge.PointerDeepMerge,
			SliceMode:   sharedstatusmerge.SliceMergeByIndex,
		},
	)
}

type racEnvAccumulator struct {
	lines        []string
	seenKeyIndex map[string]int
}

func newRACEnvAccumulator(capHint int) *racEnvAccumulator {
	if capHint < 0 {
		capHint = 0
	}
	return &racEnvAccumulator{
		lines:        make([]string, 0, capHint),
		seenKeyIndex: make(map[string]int, capHint),
	}
}

func (e *racEnvAccumulator) AddRaw(entry string) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		e.lines = append(e.lines, entry)
		return
	}

	key := strings.TrimSpace(parts[0])
	if idx, ok := e.seenKeyIndex[key]; ok {
		e.lines[idx] = entry
		return
	}
	e.seenKeyIndex[key] = len(e.lines)
	e.lines = append(e.lines, entry)
}

func (e *racEnvAccumulator) AddKV(key, value string) {
	e.AddRaw(fmt.Sprintf("%s=%s", key, value))
}

func (e *racEnvAccumulator) Values() []string {
	return e.lines
}

type racAsmEnvValues struct {
	crsDiskGroup   string
	crsDeviceList  string
	crsRedundancy  string
	dataDgName     string
	dataDeviceList string
	dataRedundancy string
	recoDgName     string
	recoDeviceList string
	recoRedundancy string
	redoDgName     string
	redoDeviceList string
	redoRedundancy string
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func setRACSecretMountDefaults(instance *racdb.RacDatabase) {
	if instance.Spec.SshKeySecret != nil && instance.Spec.SshKeySecret.KeyMountLocation == "" {
		instance.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSSHSecretMount
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
}

func collectRACAsmEnvValues(instance *racdb.RacDatabase) racAsmEnvValues {
	values := racAsmEnvValues{}
	for _, dg := range instance.Spec.AsmStorageDetails {
		switch dg.Type {
		case racdb.CrsAsmDiskDg:
			if dg.Name != "" {
				values.crsDiskGroup = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				values.crsRedundancy = dg.Redundancy
			}
		case racdb.DbDataDiskDg:
			if dg.Name != "" {
				values.dataDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				values.dataRedundancy = dg.Redundancy
			}
		case racdb.DbRecoveryDiskDg:
			if dg.Name != "" {
				values.recoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				values.recoRedundancy = dg.Redundancy
			}
		case racdb.RedoDiskDg:
			if dg.Name != "" {
				values.redoDgName = ensurePlusPrefixforRAC(dg.Name)
			}
			if dg.Redundancy != "" {
				values.redoRedundancy = dg.Redundancy
			}
		}
	}

	values.crsDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.CrsAsmDiskDg)
	values.dataDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbDataDiskDg)
	values.recoDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.DbRecoveryDiskDg)
	values.redoDeviceList = raccommon.AsmDevicesByType(instance.Status.AsmDiskGroups, racdb.RedoDiskDg)
	return values
}

type racNodeTopologyInfo struct {
	newNodes       string
	healthyNodes   string
	unhealthyNodes string
	installNode    string
}

func (r *RacDatabaseReconciler) getRACNodeTopologyInfo(instance *racdb.RacDatabase) racNodeTopologyInfo {
	info := racNodeTopologyInfo{}
	info.newNodes, info.healthyNodes, info.unhealthyNodes, info.installNode, _ =
		raccommon.GetCrsNodesForCluster(instance, r.kubeClient, r.kubeConfig, r.Log, r.Client)
	return info
}

func appendRACNodeIntentEnv(cfg *racdb.RacInitParams, env *racEnvAccumulator, topo racNodeTopologyInfo) (bool, error) {
	addnodeFlag := false
	if topo.newNodes == "" {
		return addnodeFlag, nil
	}
	if topo.unhealthyNodes != "" {
		return addnodeFlag, errors.New("cannot perform node addition as there are unhealthy CRS nodes")
	}
	env.AddKV("CRS_NODES", topo.newNodes)
	if topo.healthyNodes != "" {
		env.AddKV("EXISTING_CLS_NODE", topo.healthyNodes)
		if cfg.OpType == "" {
			env.AddKV("OP_TYPE", "racaddnode")
			env.AddKV("ADD_CDP", "true")
			addnodeFlag = true
		}
		return addnodeFlag, nil
	}
	if cfg.OpType == "" {
		env.AddKV("OP_TYPE", "setuprac")
	}
	return addnodeFlag, nil
}

func appendRACSpecEnvVars(instance *racdb.RacDatabase, env *racEnvAccumulator) {
	for _, e := range instance.Spec.EnvVars {
		env.AddKV(e.Name, e.Value)
	}
}

func appendRACServiceAndListenerEnv(instance *racdb.RacDatabase, cfg *racdb.RacInitParams, env *racEnvAccumulator) {
	if instance.Spec.ServiceDetails.Name != "" {
		env.AddKV("DB_SERVICE", raccommon.GetServiceParams(instance))
	}
	if cfg.PdbName != "" {
		env.AddKV("ORACLE_PDB", cfg.PdbName)
	}

	var (
		endp      string
		locallsnr string
		err       error
	)
	locallsnr, endp, err = raccommon.GetDBLsnrEndPointsForCluster(instance)
	if err != nil {
		return
	}
	if endp != "" {
		env.AddKV("DB_LISTENER_ENDPOINTS", endp)
	}
	if locallsnr != "" {
		env.AddKV("LOCAL_LISTENER", locallsnr)
	}
}

func appendRACCorePathEnv(cfg *racdb.RacInitParams, statusCfg *racdb.RacInitParams, env *racEnvAccumulator) {
	if v := firstNonEmpty(cfg.DbHome, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.DbHome
	}()); v != "" {
		env.AddKV("DB_HOME", v)
	}
	if v := firstNonEmpty(cfg.DbBase, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.DbBase
	}()); v != "" {
		env.AddKV("DB_BASE", v)
	}
	if v := firstNonEmpty(cfg.GridBase, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.GridBase
	}()); v != "" {
		env.AddKV("GRID_BASE", v)
	}
	if v := firstNonEmpty(cfg.GridHome, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.GridHome
	}()); v != "" {
		env.AddKV("GRID_HOME", v)
	}
	if v := firstNonEmpty(cfg.Inventory, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.Inventory
	}()); v != "" {
		env.AddKV("INVENTORY", v)
	}
}

func (r *RacDatabaseReconciler) appendRACSecretEnv(instance *racdb.RacDatabase, env *racEnvAccumulator) {
	if instance.Spec.SshKeySecret != nil && strings.TrimSpace(instance.Spec.SshKeySecret.Name) != "" {
		env.AddKV("SSH_PRIVATE_KEY", instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PrivKeySecretName)
		env.AddKV("SSH_PUBLIC_KEY", instance.Spec.SshKeySecret.KeyMountLocation+"/"+instance.Spec.SshKeySecret.PubKeySecretName)
	}

	if instance.Spec.DbSecret != nil && instance.Spec.DbSecret.Name != "" {
		env.AddKV("SECRET_VOLUME", instance.Spec.DbSecret.PwdFileMountLocation)
		keyFile := instance.Spec.DbSecret.KeyFileName
		if keyFile == "" {
			keyFile = instance.Spec.DbSecret.SecretKey
		}
		commonpassflag, pwdkeyflag, legacyPwdFile := raccommon.GetDbSecret(instance, instance.Spec.DbSecret.Name, r.Client)
		if pwdkeyflag {
			env.AddKV("PWD_KEY", keyFile)
			if commonpassflag && instance.Spec.DbSecret.PwdFileName != "" {
				env.AddKV("DB_PWD_FILE", instance.Spec.DbSecret.PwdFileName)
			} else if legacyPwdFile {
				env.AddKV("PASSWORD_FILE", "pwdfile")
			} else {
				env.AddKV("PASSWORD_FILE", "pwdfile")
			}
		} else {
			env.AddKV("PASSWORD_FILE", "pwdfile")
		}
	}

	if instance.Spec.TdeWalletSecret != nil && instance.Spec.TdeWalletSecret.Name != "" {
		env.AddKV("TDE_SECRET_VOLUME", instance.Spec.TdeWalletSecret.PwdFileMountLocation)
		env.AddKV("SETUP_TDE_WALLET", "true")
		tdeKeyFile := instance.Spec.TdeWalletSecret.KeyFileName
		if tdeKeyFile == "" {
			tdeKeyFile = instance.Spec.TdeWalletSecret.SecretKey
		}
		tdepassflag, tdepwdkeyflag, legacyPwdFile := raccommon.GetTdeWalletSecret(instance, instance.Spec.TdeWalletSecret.Name, r.Client)
		if tdepwdkeyflag {
			env.AddKV("TDE_PWD_KEY", tdeKeyFile)
			if tdepassflag && instance.Spec.TdeWalletSecret.PwdFileName != "" {
				env.AddKV("TDE_PWD_FILE", instance.Spec.TdeWalletSecret.PwdFileName)
			} else if legacyPwdFile {
				env.AddKV("PASSWORD_FILE", "pwdfile")
			} else {
				env.AddKV("PASSWORD_FILE", "tdepwdfile")
			}
		} else {
			env.AddKV("PASSWORD_FILE", "tdepwdfile")
		}
	}
}

func appendRACIdentityAndSoftwareEnv(cfg *racdb.RacInitParams, statusCfg *racdb.RacInitParams, scanName, installNode string, env *racEnvAccumulator) {
	env.AddKV("PROFILE_FLAG", "true")
	env.AddKV("SCAN_NAME", scanName)
	env.AddKV("INSTALL_NODE", installNode)

	if v := firstNonEmpty(cfg.DbName, func() string {
		if statusCfg == nil {
			return ""
		}
		return statusCfg.DbName
	}()); v != "" {
		env.AddKV("DB_NAME", v)
	}
	if cfg.DbUniqueName != "" {
		env.AddKV("DB_UNIQUE_NAME", cfg.DbUniqueName)
	} else if statusCfg != nil && statusCfg.DbUniqueName != "" {
		env.AddKV("DB_UNIQUE_NAME", statusCfg.DbUniqueName)
	}

	if cfg.GridSwZipFile != "" {
		env.AddKV("GRID_SW_ZIP_FILE", cfg.GridSwZipFile)
	}
	if cfg.HostSwStageLocation != "" {
		env.AddKV("STAGING_SOFTWARE_LOC", utils.OraSwStageLocation)
	} else if cfg.SwStagePvcMountLocation != "" {
		env.AddKV("STAGING_SOFTWARE_LOC", cfg.SwStagePvcMountLocation)
	}
	if cfg.RuPatchLocation != "" {
		env.AddKV("APPLY_RU_LOCATION", utils.OraRuPatchStageLocation)
	}
	if cfg.RuFolderName != "" {
		env.AddKV("RU_FOLDER_NAME", cfg.RuFolderName)
	}
	if cfg.OPatchLocation != "" {
		env.AddKV("OPATCH_ZIP_FILE", utils.OraOPatchStageLocation+"/"+cfg.OPatchSwZipFile)
	}
	if cfg.OneOffLocation != "" {
		env.AddKV("ONEOFF_FOLDER_NAME", cfg.OneOffLocation)
	}
	if cfg.DbOneOffIds != "" {
		env.AddKV("DB_ONEOFF_IDS", cfg.DbOneOffIds)
	}
	if cfg.GridOneOffIds != "" {
		env.AddKV("GRID_ONEOFF_IDS", cfg.GridOneOffIds)
	}
	if cfg.DbSwZipFile != "" {
		env.AddKV("DB_SW_ZIP_FILE", cfg.DbSwZipFile)
	}
}

func appendRACAsmStorageEnv(instance *racdb.RacDatabase, cfg *racdb.RacInitParams, env *racEnvAccumulator) {
	asmValues := collectRACAsmEnvValues(instance)
	if asmValues.crsDiskGroup == "" {
		asmValues.crsDiskGroup = "+DATA"
	}
	env.AddKV("CRS_ASM_DISKGROUP", asmValues.crsDiskGroup)
	if asmValues.crsDeviceList != "" {
		env.AddKV("CRS_ASM_DEVICE_LIST", asmValues.crsDeviceList)
	}
	if asmValues.crsRedundancy != "" {
		env.AddKV("CRS_ASMDG_REDUNDANCY", asmValues.crsRedundancy)
	}
	if asmValues.dataDgName == "" {
		env.AddKV("DB_DATA_FILE_DEST", asmValues.crsDiskGroup)
	} else {
		env.AddKV("DB_DATA_FILE_DEST", asmValues.dataDgName)
	}
	if asmValues.dataDeviceList != "" {
		env.AddKV("DB_ASM_DEVICE_LIST", asmValues.dataDeviceList)
	}
	if asmValues.dataRedundancy != "" {
		env.AddKV("DB_ASMDG_PROPERTIES", "redundancy:"+asmValues.dataRedundancy)
	}
	if asmValues.recoDeviceList != "" {
		env.AddKV("RECO_ASM_DEVICE_LIST", asmValues.recoDeviceList)
	}
	if asmValues.recoDgName == "" {
		env.AddKV("DB_RECOVERY_FILE_DEST", asmValues.crsDiskGroup)
	} else {
		env.AddKV("DB_RECOVERY_FILE_DEST", asmValues.recoDgName)
	}
	if asmValues.recoRedundancy != "" {
		env.AddKV("RECO_ASMDG_PROPERTIES", "redundancy:"+asmValues.recoRedundancy)
	}
	if asmValues.redoDgName != "" {
		env.AddKV("LOG_FILE_DEST", asmValues.redoDgName)
	}
	if asmValues.redoDeviceList != "" {
		env.AddKV("REDO_ASM_DEVICE_LIST", asmValues.redoDeviceList)
	}
	if asmValues.redoRedundancy != "" {
		env.AddKV("REDO_ASMDG_PROPERTIES", "redundancy:"+asmValues.redoRedundancy)
	}
	if cfg.DbCharSet == "" {
		env.AddKV("DB_CHARACTERSET", "AL32UTF8")
	}
}

func appendRACDbInitEnv(cfg *racdb.RacInitParams, addnodeFlag bool, env *racEnvAccumulator) {
	if addnodeFlag {
		return
	}
	if cfg.DbStorageType != "" {
		env.AddKV("DB_STORAGE_TYPE", cfg.DbStorageType)
	}
	if cfg.DbCharSet != "" {
		env.AddKV("DB_CHARACTERSET", cfg.DbCharSet)
	}
	if cfg.DbType != "" {
		env.AddKV("DB_TYPE", cfg.DbType)
	}
	if cfg.DbConfigType != "" {
		env.AddKV("DB_CONFIG_TYPE", cfg.DbConfigType)
	}
	if cfg.EnableArchiveLog != "" {
		env.AddKV("ENABLE_ARCHIVELOG", cfg.EnableArchiveLog)
	}
	if cfg.GridResponseFile != nil && cfg.GridResponseFile.ConfigMapName != "" {
		env.AddKV("GRID_RESPONSE_FILE", utils.OraGiRsp+"/"+cfg.GridResponseFile.Name)
	}
	if cfg.DbResponseFile != nil && cfg.DbResponseFile.ConfigMapName != "" {
		env.AddKV("DBCA_RESPONSE_FILE", utils.OraDbRsp+"/"+cfg.DbResponseFile.Name)
	}
	if cfg.SgaSize != "" {
		env.AddKV("INIT_SGA_SIZE", cfg.SgaSize)
		env.AddKV("INIT_SGA_SIZE", normalizeOracleMemoryUnitforRAC(cfg.SgaSize))
	}
	if cfg.PgaSize != "" {
		env.AddKV("INIT_PGA_SIZE", cfg.PgaSize)
		env.AddKV("INIT_PGA_SIZE", normalizeOracleMemoryUnitforRAC(cfg.PgaSize))
	}
	if cfg.Processes > 0 {
		env.AddKV("INIT_PROCESSES", strconv.Itoa(cfg.Processes))
		env.AddKV("INIT_PROCESSES", strconv.Itoa(cfg.Processes))
	}
	if cfg.CpuCount > 0 {
		env.AddKV("CPU_COUNT", strconv.Itoa(cfg.CpuCount))
		env.AddKV("CPU_COUNT", strconv.Itoa(cfg.CpuCount))
	}
}

// generateConfigMap builds ConfigMap data for RAC setup, producing the envfile
// content tailored to either legacy or cluster-style configurations.
func (r *RacDatabaseReconciler) generateConfigMap(instance *racdb.RacDatabase) (map[string]string, error) {
	configMapData := make(map[string]string, 0)
	cfg := instance.Spec.ConfigParams
	if cfg == nil {
		cfg = &racdb.RacInitParams{}
	}
	statusCfg := instance.Status.ConfigParams
	topo := r.getRACNodeTopologyInfo(instance)
	env := newRACEnvAccumulator(96)
	scanName := raccommon.GetScanname(instance)

	//Defaults from webhook
	if instance.Spec.ImagePullPolicy == nil || *instance.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		instance.Spec.ImagePullPolicy = &policy
	}

	setRACSecretMountDefaults(instance)
	if topo.newNodes == "" {
		return configMapData, nil
	}

	addnodeFlag, err := appendRACNodeIntentEnv(cfg, env, topo)
	if err != nil {
		return configMapData, err
	}
	appendRACSpecEnvVars(instance, env)
	appendRACServiceAndListenerEnv(instance, cfg, env)
	appendRACCorePathEnv(cfg, statusCfg, env)
	r.appendRACSecretEnv(instance, env)
	appendRACIdentityAndSoftwareEnv(cfg, statusCfg, scanName, topo.installNode, env)
	appendRACAsmStorageEnv(instance, cfg, env)
	appendRACDbInitEnv(cfg, addnodeFlag, env)

	configMapData["envfile"] = strings.Join(env.Values(), "\r\n")
	return configMapData, nil
}

// normalizeOracleMemoryUnitforRAC standardizes memory strings onto Oracle's
// expected units, simplifying downstream comparisons and validation work.
func normalizeOracleMemoryUnitforRAC(s string) string {
	return sharedorautil.NormalizeOracleMemoryUnit(s)
}

// ensurePlusPrefixforRAC guarantees ASM disk group names start with '+' to
// meet Oracle conventions when the spec omits the prefix.
func ensurePlusPrefixforRAC(name string) string {
	return sharedorautil.EnsurePlusPrefix(name)
}

// createConfigMap ensures the target ConfigMap exists with the desired
// contents, creating it when first provisioning RAC configuration.
func (r *RacDatabaseReconciler) createConfigMap(ctx context.Context, instance *racdb.RacDatabase, cm *corev1.ConfigMap) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	created, err := sharedk8sobjects.EnsureConfigMapExists(ctx, r.Client, instance.Namespace, cm)
	if err != nil {
		reqLogger.Error(err, "failed to reconcile configmap", "namespace", instance.Namespace)
		var cmErr *sharedk8sobjects.ConfigMapReconcileError
		if errors.As(err, &cmErr) && cmErr.Op == sharedk8sobjects.ConfigMapOpCreate {
			// Preserve historical RAC behavior: create failures were logged and not returned.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if created {
		reqLogger.Info("Creating Configmap Normally")
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil

}

// createOrReplaceService deploys or updates the Kubernetes Service backing
// RAC networking endpoints, reconciling metadata with the desired template.
func (r *RacDatabaseReconciler) createOrReplaceService(ctx context.Context, instance *racdb.RacDatabase,
	dep *corev1.Service,
) (ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name, "dep.Name", dep.Name)
	changed, err := sharedk8sobjects.EnsureService(ctx, r.Client, instance.Namespace, dep, sharedk8sobjects.ServiceSyncOptions{
		NodePortMerge:             sharedk8sobjects.NodePortMergeByNamePortAndProtocol,
		SyncOwnerReferences:       true,
		SyncSessionAffinityCfg:    true,
		SyncPublishNotReady:       true,
		SyncInternalTrafficPolicy: true,
		SyncLoadBalancerFields:    true,
	})
	if err != nil {
		markRACFailedStatus(instance)
		reqLogger.Error(err, "Failed to reconcile Service", "Service.Namespace", dep.Namespace, "Service.Name", dep.Name)
		return ctrl.Result{}, err
	}
	if changed {
		reqLogger.Info("Service reconciled to desired state", "Service.Namespace", dep.Namespace, "Service.Name", dep.Name)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func mergeServicePortsWithAssignedNodePorts(existing []corev1.ServicePort, desired []corev1.ServicePort) []corev1.ServicePort {
	return sharedk8sobjects.MergeServicePortsWithAssignedNodePortsByNamePortProtocol(existing, desired)
}

// createOrReplaceAsmPv reconciles ASM persistent volumes for RAC disk
// groups, creating or patching PV resources to match the desired spec.
func (r *RacDatabaseReconciler) createOrReplaceAsmPv(
	ctx context.Context,
	instance *racdb.RacDatabase,
	dep *corev1.PersistentVolume,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)

	name, created, err := sharedk8sobjects.EnsurePersistentVolume(context.TODO(), r.Client, dep)
	if err != nil {
		markRACFailedStatus(instance)
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

// createOrReplaceAsmPvC handles the ConfigMap variant of ASM PV reconcilation,
// ensuring consistent provisioning across configuration sources.
func (r *RacDatabaseReconciler) createOrReplaceAsmPvC(ctx context.Context, instance *racdb.RacDatabase,
	dep *corev1.PersistentVolumeClaim,
	dgType string,
) (string, ctrl.Result, error) {
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)

	jsn, _ := json.Marshal(dep)
	raccommon.LogMessages("DEBUG", string(jsn), nil, instance, r.Log)
	name, created, err := sharedk8sobjects.EnsurePersistentVolumeClaim(ctx, r.Client, dep)
	if err != nil {
		markRACFailedStatus(instance)
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

// ensureAsmStorageStatus initializes ASM DiskGroup details
// and restores ASM device and disk group info for each RAC instance.
// ensureAsmStorageStatus guarantees ASM disk group status structures exist
// before reconcile logic attempts to populate them with discovery results.
func (r *RacDatabaseReconciler) ensureAsmStorageStatus(racDatabase *racdb.RacDatabase) {
	r.Log.Info("Ensuring ASM DiskGroup status initialization")

	if racDatabase.Status.AsmDiskGroups == nil {
		racDatabase.Status.AsmDiskGroups = []racdb.AsmDiskGroupStatus{}
		clusterSpec := racDatabase.Spec.ClusterDetails
		if clusterSpec != nil && clusterSpec.NodeCount > 0 {
			nodeName := fmt.Sprintf("%s%d", clusterSpec.RacNodeName, 1)
			podName := nodeName + "-0"

			crsDeviceList := raccommon.GetAsmDevicesForCluster(racDatabase, racdb.CrsAsmDiskDg)
			dbDeviceList := raccommon.GetAsmDevicesForCluster(racDatabase, racdb.DbDataDiskDg)
			raccommon.SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.CrsAsmDiskDg, crsDeviceList)
			raccommon.SetAsmDiskGroupDevices(&racDatabase.Status.AsmDiskGroups, racdb.DbDataDiskDg, dbDeviceList)

			diskGroup := raccommon.GetAsmDiskgroup(podName, racDatabase, 0, r.kubeClient, r.kubeConfig, r.Log)
			if diskGroup != "" {
				for i, dgStatus := range racDatabase.Status.AsmDiskGroups {
					if dgStatus.Name == diskGroup {
						racDatabase.Status.AsmDiskGroups[i].Name = diskGroup
						break
					}
				}
			}
		}
		r.Log.Info("ASM DiskGroup devices restored successfully", "DiskGroupsCount", len(racDatabase.Status.AsmDiskGroups))
	}
}

// ensureStatefulSetUpdated performs rolling updates on RAC StatefulSets when
// templates change, handling pod recreation and status monitoring.
func (r *RacDatabaseReconciler) ensureStatefulSetUpdated(ctx context.Context,
	reqLogger logr.Logger,
	racDatabase *racdb.RacDatabase,
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
		Namespace: racDatabase.Namespace,
	}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the StatefulSet doesn't exist, create it
			reqLogger.Info("StatefulSet not found, creating new one", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
			return r.Create(ctx, desired)
		}
		reqLogger.Error(err, "Failed to get StatefulSet", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
		return err
	}

	// Compare the existing StatefulSet spec with the desired spec, sfs is replaced when ASM devices are added or removed
	if len(existing.Spec.Template.Spec.Containers[0].VolumeDevices) != len(desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
		r.Log.Info("Change State to UPDATING")

		if racDatabase.Spec.ClusterDetails != nil && racDatabase.Spec.ClusterDetails.NodeCount > 0 {
			clusterSpec := racDatabase.Spec.ClusterDetails
			for index := 0; index < clusterSpec.NodeCount; index++ {
				r.updateRacNodeStatusForCluster(racDatabase, ctx, req, clusterSpec, index, string(racdb.RACProvisionState))
			}
		}
		reqLogger.Info("StatefulSet spec differs for volume devices, updating StatefulSet (pods may be recreated)", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)

		// Perform the update
		err := r.Update(ctx, desired)
		if err != nil {
			reqLogger.Error(err, "Failed to update StatefulSet", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
			return err
		}

		reqLogger.Info("StatefulSet update applied, waiting for pod recreation", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)

		// Wait for the update to be applied
		for {
			select {
			case <-timeoutCtx.Done():
				reqLogger.Error(timeoutCtx.Err(), "Timed out waiting for StatefulSet update", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
				return timeoutCtx.Err()

			default:
				updated := &appsv1.StatefulSet{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      desired.Name,
					Namespace: racDatabase.Namespace,
				}, updated)

				if err != nil {
					reqLogger.Error(err, "Failed to get StatefulSet after update", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
					return err
				}

				if reflect.DeepEqual(updated.Spec.Template.Spec.Containers[0].VolumeDevices, desired.Spec.Template.Spec.Containers[0].VolumeDevices) {
					reqLogger.Info("StatefulSet update is applied successfully", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
					return nil
				}

				reqLogger.Info("Waiting for StatefulSet update to be applied", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
				time.Sleep(5 * time.Second)
			}
		}
		// }
	} else {
		reqLogger.Info("StatefulSet matches for  ASM devices, SFS wont be updated", "StatefulSet.Namespace", racDatabase.Namespace, "StatefulSet.Name", desired.Name)
		return nil
	}
}

// diskGroupExists executes ASM queries against a pod to determine whether a
// specific disk group is present, aiding add/remove workflows.
func (r *RacDatabaseReconciler) diskGroupExists(
	podName, diskGroupName string,
	resp *raccommon.ExecCommandResp,
	instance *racdb.RacDatabase,
	logger logr.Logger,
) (bool, error) {
	_ = logger
	reqLogger := r.Log.WithValues("Instance.Namespace", instance.Namespace, "Instance.Name", instance.Name)
	cmd := fmt.Sprintf("su - grid -c 'asmcmd lsdg | grep -w %s'", diskGroupName)
	stdout, _, err := raccommon.ExecCommandWithResp(podName, []string{"bash", "-c", cmd}, resp, instance, reqLogger)
	if err != nil {
		return false, err
	}
	if strings.Contains(stdout, diskGroupName) {
		return true, nil
	}
	return true, nil
}

// addDisks orchestrates the addition of new ASM disks by invoking helper
// jobs across RAC pods and waits for completion before proceeding.
func (r *RacDatabaseReconciler) addDisks(
	ctx context.Context,
	podList *corev1.PodList,
	instance *racdb.RacDatabase,
	diskGroupName string,
	deviceList []string,
) error {

	reqLogger := r.Log.WithValues(
		"Instance.Namespace", instance.Namespace,
		"Instance.Name", instance.Name,
	)
	resp := raccommon.NewExecCommandResp(r.kubeClient, r.kubeConfig)

	if len(podList.Items) == 0 {
		return fmt.Errorf("no pods available to add ASM disks")
	}

	// Pick exactly ONE pod to run once
	podName := podList.Items[0].Name
	reqLogger.Info("Using pod to add ASM disks", "Pod.Name", podName)

	if strings.HasPrefix(diskGroupName, "+") {
		diskGroupName = strings.TrimPrefix(diskGroupName, "+")
	}

	// Check disk group exists ONCE
	exists, err := r.diskGroupExists(
		podName,
		diskGroupName,
		resp,
		instance,
		reqLogger,
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("disk group %s does not exist", diskGroupName)
	}

	// Add all disks ONCE
	for _, disk := range deviceList {
		cmd := fmt.Sprintf(
			"python3 /opt/scripts/startup/scripts/main.py --updateasmdevices=\"diskname=%s;diskgroup=%s;processtype=addition\"",
			disk,
			diskGroupName,
		)

		reqLogger.Info("Executing command to add disk",
			"Pod.Name", podName,
			"Disk", disk,
			"DiskGroup", diskGroupName,
		)

		stdout, stderr, err := raccommon.ExecCommandWithResp(
			podName, []string{"bash", "-c", cmd}, resp, instance, reqLogger,
		)
		if err != nil {
			// tolerate "already added"
			if strings.Contains(stdout, "UPDATE_ASMDEVICES_NOT_UPDATED") {
				reqLogger.Info("ASM disk already exists, skipping",
					"Disk", disk,
					"DiskGroup", diskGroupName,
				)
				continue
			}

			markRACFailedStatus(instance)
			reqLogger.Error(err, "Failed to execute command",
				"Stdout", stdout,
				"Stderr", stderr,
			)
			return err
		}
	}

	return nil
}

// checkRacDaemonSetStatusforRAC verifies disk-check DaemonSet readiness,
// returning a boolean that indicates whether discovery completed.
func checkRacDaemonSetStatusforRAC(ctx context.Context, r *RacDatabaseReconciler, racDatabase *racdb.RacDatabase) (bool, error) {
	timeout := time.After(racDiskCheckReadyTimeout)
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	checkOnce := func() (bool, error) {
		ready, invalidDevice, err := shareddiskcheck.CheckDaemonSetReadyAndDiskValidation(
			ctx,
			r.Client,
			r.kubeClient,
			racDatabase.Namespace,
			"disk-check-daemonset",
			diskCheckLabelSelectorForRAC(racDatabase),
		)
		if err != nil {
			return false, err
		}
		if invalidDevice {
			return false, fmt.Errorf("disk validation failed: not a valid block device")
		}
		if !ready {
			return false, nil
		}

		_, discoveryReady, err := r.collectDiskCheckResults(ctx, racDatabase)
		if err != nil {
			return false, err
		}
		if !discoveryReady {
			return false, nil
		}

		return true, nil
	}

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-timeout:
			ready, err := checkOnce()
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
			return false, fmt.Errorf("timeout waiting for disk-check daemonset readiness and ASM disk discovery after %s", racDiskCheckReadyTimeout)
		case <-tick.C:
			ready, err := checkOnce()
			if err != nil {
				return false, err
			}
			if ready {
				return true, nil
			}
		}
	}
}

// createOrReplaceSfs reconciles per-instance StatefulSets ensuring pods match
// the desired spec, handling lookups, updates, and requeue timing.
func (r *RacDatabaseReconciler) createOrReplaceSfs(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldState string,
) (ctrl.Result, error) {
	_ = oldState
	reqLogger := r.Log.WithValues("Instance.Namespace", racDatabase.Namespace, "Instance.Name", racDatabase.Name)
	configMapHash, err := r.computeRACConfigMapHash(ctx, racDatabase.Namespace, &dep.Spec.Template)
	if err != nil {
		reqLogger.Error(err, "Failed to compute RAC ConfigMap hash", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
		return ctrl.Result{}, err
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations[racConfigMapHashAnnotation] = configMapHash
	result, err := sharedk8sobjects.ReconcileStatefulSet(ctx, r.Client, racDatabase.Namespace, dep, func(found, desired *appsv1.StatefulSet) bool {
		return syncRACStatefulSetScopedFields(found, desired)
	})
	if err != nil {
		markRACFailedStatus(racDatabase)
		reqLogger.Error(err, "Failed to reconcile StatefulSet", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
		return ctrl.Result{}, err
	}
	if result.Created {
		r.markRACProvisioningForStatefulSet(ctx, req, racDatabase, index)
		reqLogger.Info("Creating a StatefulSet Normally", "StatefulSetName", dep.Name)
		if !isLast {
			return ctrl.Result{}, nil
		}
	}
	if result.Updated {
		r.markRACProvisioningForStatefulSet(ctx, req, racDatabase, index)
		reqLogger.Info("Updating StatefulSet to desired spec", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
	}

	return ctrl.Result{}, nil
}

func (r *RacDatabaseReconciler) computeRACConfigMapHash(ctx context.Context, namespace string, template *corev1.PodTemplateSpec) (string, error) {
	if template == nil {
		return "", nil
	}

	type configMapFingerprint struct {
		volumeName string
		configName string
		data       []string
		binaryData []string
	}

	fingerprints := make([]configMapFingerprint, 0)
	for _, volume := range template.Spec.Volumes {
		if volume.ConfigMap == nil {
			continue
		}
		if !strings.HasSuffix(volume.Name, "-oradata-envfile") {
			continue
		}
		found := &corev1.ConfigMap{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: volume.ConfigMap.Name, Namespace: namespace}, found); err != nil {
			return "", err
		}

		fp := configMapFingerprint{
			volumeName: volume.Name,
			configName: volume.ConfigMap.Name,
		}
		for k, v := range found.Data {
			fp.data = append(fp.data, k+"="+v)
		}
		for k, v := range found.BinaryData {
			fp.binaryData = append(fp.binaryData, k+"="+hex.EncodeToString(v))
		}
		sort.Strings(fp.data)
		sort.Strings(fp.binaryData)
		fingerprints = append(fingerprints, fp)
	}

	sort.Slice(fingerprints, func(i, j int) bool {
		if fingerprints[i].volumeName != fingerprints[j].volumeName {
			return fingerprints[i].volumeName < fingerprints[j].volumeName
		}
		return fingerprints[i].configName < fingerprints[j].configName
	})

	sum := sha256.New()
	for _, fp := range fingerprints {
		sum.Write([]byte(fp.volumeName))
		sum.Write([]byte{0})
		sum.Write([]byte(fp.configName))
		sum.Write([]byte{0})
		for _, entry := range fp.data {
			sum.Write([]byte(entry))
			sum.Write([]byte{0})
		}
		for _, entry := range fp.binaryData {
			sum.Write([]byte(entry))
			sum.Write([]byte{0})
		}
	}

	return hex.EncodeToString(sum.Sum(nil)), nil
}

func syncRACStatefulSetScopedFields(found, desired *appsv1.StatefulSet) bool {
	updated := false
	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.Annotations, desired.Annotations) {
		found.Annotations = desired.Annotations
		updated = true
	}
	if syncRACPodTemplateScopedFields(&found.Spec.Template, &desired.Spec.Template) {
		updated = true
	}

	return updated
}

func syncRACPodTemplateScopedFields(found, desired *corev1.PodTemplateSpec) bool {
	updated := false

	foundHash := ""
	if found.Annotations != nil {
		foundHash = found.Annotations[racConfigMapHashAnnotation]
	}
	desiredHash := ""
	if desired.Annotations != nil {
		desiredHash = desired.Annotations[racConfigMapHashAnnotation]
	}
	if foundHash != desiredHash {
		if found.Annotations == nil {
			found.Annotations = map[string]string{}
		}
		found.Annotations[racConfigMapHashAnnotation] = desiredHash
		updated = true
	}
	if !equalRACContainerVolumeDevices(found.Spec.Containers, desired.Spec.Containers) {
		found.Spec.Containers = desired.Spec.Containers
		updated = true
	}
	return updated
}

func equalRACContainerVolumeDevices(found, desired []corev1.Container) bool {
	if len(found) != len(desired) {
		return false
	}
	for i := range found {
		if found[i].Name != desired[i].Name {
			return false
		}
		if !reflect.DeepEqual(found[i].VolumeDevices, desired[i].VolumeDevices) {
			return false
		}
	}
	return true
}

func (r *RacDatabaseReconciler) markRACProvisioningForStatefulSet(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	index int,
) {
	clusterSpec := racDatabase.Spec.ClusterDetails
	if clusterSpec == nil {
		return
	}
	r.updateRacNodeStatusForCluster(
		racDatabase, ctx, req,
		clusterSpec, index,
		string(racdb.RACProvisionState),
	)
}

// createOrReplaceSfsAsmCluster manages new cluster-style StatefulSet updates
// when ASM configuration changes are detected across the RAC topology.
func (r *RacDatabaseReconciler) createOrReplaceSfsAsmCluster(
	ctx context.Context,
	req ctrl.Request,
	racDatabase *racdb.RacDatabase,
	dep *appsv1.StatefulSet,
	index int,
	isLast bool,
	oldSpec *racdb.RacDatabaseSpec,
) (ctrl.Result, error) {

	asmAutoUpdate := true
	reqLogger := r.Log.WithValues("racDatabase.Namespace", racDatabase.Namespace,
		"racDatabase.Name", racDatabase.Name)

	found := &appsv1.StatefulSet{}

	// ---------------------------------------------------------
	// STEP 1 — Retrieve existing StatefulSet
	// ---------------------------------------------------------
	err := r.Get(ctx, types.NamespacedName{
		Name:      dep.Name,
		Namespace: racDatabase.Namespace,
	}, found)
	// debugger
	if err != nil {
		reqLogger.Error(err, "Failed to find existing StatefulSet to update")
		return ctrl.Result{}, err
	}

	// ---------------------------------------------------------
	// STEP 2 — Compute disk changes
	// ---------------------------------------------------------
	addedAsmDisks, removedAsmDisks, err := r.computeDiskChanges(racDatabase, oldSpec)
	if err != nil {
		return ctrl.Result{}, err
	}
	//debugger
	// addedAsmDisks = []string{"/dev/disk/by-partlabel/ocne_asm_disk_03"}

	inUse := false

	// ---------------------------------------------------------
	// STEP 3 — Check ASM disk usage before removal
	// ---------------------------------------------------------
	if len(removedAsmDisks) > 0 {
		// 3a: Retrieve StatefulSet
		racSfSet, err := raccommon.CheckSfset(dep.Name, racDatabase, r.Client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to retrieve StatefulSet '%s': %w", dep.Name, err)
		}

		// 3b: Get pod list for this RAC node (cluster mode)
		podList, err := r.getPodsForStatefulSet(ctx, racDatabase, racSfSet.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to retrieve pod list for StatefulSet '%s': %w",
				racSfSet.Name, err)
		}
		if len(podList.Items) == 0 {
			return ctrl.Result{}, fmt.Errorf("no pods found for StatefulSet '%s'", racSfSet.Name)
		}

		// 3c: Query ASM disk groups using last pod
		podName := podList.Items[len(podList.Items)-1].Name
		racDatabase.Status.AsmDiskGroups =
			raccommon.GetAsmInstState(podName, racDatabase, index, r.kubeClient, r.kubeConfig, r.Log)

	}

	// Sync ASM status
	r.ensureAsmStorageStatus(racDatabase)

	// ---------------------------------------------------------
	// STEP 4 — Apply StatefulSet ASM Update
	// ---------------------------------------------------------
	err = r.ensureStatefulSetUpdated(ctx, reqLogger, racDatabase, dep, asmAutoUpdate, req)
	if err != nil {
		markRACFailedStatus(racDatabase)
		reqLogger.Error(err, "Failed to ensure StatefulSet update")

		// Update cluster node statuses
		clusterSpec := racDatabase.Spec.ClusterDetails
		for nodeIdx := 0; nodeIdx < clusterSpec.NodeCount; nodeIdx++ {
			r.updateRacNodeStatusForCluster(
				racDatabase, ctx, req, clusterSpec, nodeIdx, string(racdb.RACProvisionState))
		}
		return ctrl.Result{}, err
	}

	// ---------------------------------------------------------
	// STEP 5 — Check if all pods are Running and Ready (non-blocking)
	// ---------------------------------------------------------
	podList := &corev1.PodList{}

	reqLogger.Info("Checking if all pods are Running and Ready")

	err = r.List(ctx, podList,
		client.InNamespace(dep.Namespace),
		client.MatchingLabels(dep.Spec.Template.Labels),
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(podList.Items) == 0 {
		reqLogger.Info("No pods found yet, requeueing")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	for _, pod := range podList.Items {

		if pod.Status.Phase != corev1.PodRunning {
			reqLogger.Info("Pod not Running yet, requeueing", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady &&
				cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		if !ready {
			reqLogger.Info("Pod not Ready yet, requeueing", "pod", pod.Name)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	reqLogger.Info("All pods are Running and Ready, proceeding with next steps")

	// ---------------------------------------------------------
	// STEP 6 — PVC/PV deletion for removed disks (last iteration)
	// ---------------------------------------------------------
	if isLast && !inUse {

		for _, removed := range removedAsmDisks {

			pvcName := raccommon.GetAsmPvcName(removed, racDatabase.Name)
			pvName := raccommon.GetAsmPvName(removed, racDatabase.Name)

			pvc := &corev1.PersistentVolumeClaim{}
			pv := &corev1.PersistentVolume{}

			// Delete PVC
			_ = r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: racDatabase.Namespace}, pvc)
			_ = r.Delete(ctx, pvc)

			// Delete PV
			_ = r.Get(ctx, client.ObjectKey{Name: pvName}, pv)
			_ = r.Delete(ctx, pv)

			r.Log.Info("Deleted ASM PVC & PV", "PVC", pvcName, "PV", pvName)
		}

	}

	// ---------------------------------------------------------
	// STEP 7 — Add new ASM disks after recreation (last iteration)
	// ---------------------------------------------------------
	if isLast && len(addedAsmDisks) > 0 {
		podList, err := r.getPodsForStatefulSet(ctx, racDatabase, dep.Name)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Map disk group name to autoUpdate (bool)
		dgAutoUpdate := map[string]bool{}

		for _, dg := range racDatabase.Spec.AsmStorageDetails {
			if strings.EqualFold(dg.AutoUpdate, "true") {
				dgAutoUpdate[dg.Name] = true
			} else if strings.EqualFold(dg.AutoUpdate, "false") {
				dgAutoUpdate[dg.Name] = false
			} else {
				// default only if key does not exist yet
				if _, exists := dgAutoUpdate[dg.Name]; !exists {
					dgAutoUpdate[dg.Name] = true // default true
				}
			}
		}

		// Group added disks by their diskgroup
		disksByDG := map[string][]string{}
		for _, disk := range addedAsmDisks {
			dg := getDeviceDG(disk, &racDatabase.Spec, reqLogger)
			if dg == "" {
				reqLogger.Info("No diskgroup found for disk, skipping", "disk", disk)
				continue
			}
			disksByDG[dg] = append(disksByDG[dg], disk)
		}

		// Add disks only for groups where autoUpdate == "true"
		for dg, disks := range disksByDG {
			if !dgAutoUpdate[dg] {
				reqLogger.Info("Skipping ASM disk add: autoUpdate is disabled", "diskgroup", dg)
				continue
			}
			if len(disks) == 0 {
				continue
			}
			err = r.addDisks(ctx, podList, racDatabase, dg, disks)
			if err != nil {
				return ctrl.Result{}, err
			}
			reqLogger.Info("New ASM disks added", "diskgroup", dg, "disks", disks)
		}
	}

	return ctrl.Result{}, nil
}

// getDeviceDG returns the disk group name for a given device by scanning
// the RAC spec, aiding logging and validations during reconcile.
func getDeviceDG(disk string, spec *racdb.RacDatabaseSpec, reqLogger logr.Logger) string {
	for _, dg := range spec.AsmStorageDetails {
		for _, dgDisk := range dg.Disks {
			if strings.TrimSpace(dgDisk) == strings.TrimSpace(disk) {
				reqLogger.Info("Disk found in ASM DiskGroup", "disk", disk, "diskGroup", dg.Name, "type", dg.Type)
				return dg.Name // or return dg.Type if you need type
			}
		}
	}
	return ""
}

// getRACDisksChangedSpecforRAC compares old vs. new specs to produce lists of
// added and removed disks, which guide follow-up reconcile operations.
func getRACDisksChangedSpecforRAC(racDatabase racdb.RacDatabase, oldSpec racdb.RacDatabaseSpec) ([]string, []string) {
	newAdapter := newRacAsmAdapter(&racDatabase, nil)
	oldObj := racdb.RacDatabase{Spec: oldSpec}
	oldAdapter := newRacAsmAdapter(&oldObj, nil)
	return sharedasm.GetDisksChanged(newAdapter, oldAdapter)
}

// manageRacDatabaseDeletion orchestrates finalizer and cleanup logic when
// the RAC database resource is being deleted, including final status updates.
// Returns handled=true when reconcile should stop normal processing.
func (r *RacDatabaseReconciler) manageRacDatabaseDeletion(req ctrl.Request, ctx context.Context, racDatabase *racdb.RacDatabase) (bool, error) {
	log := r.Log.WithValues("manageRacDatabaseDeletion", req.NamespacedName)

	// Check if the RacDatabase instance is marked to be deleted
	isRacDatabaseMarkedToBeDeleted := racDatabase.GetDeletionTimestamp() != nil
	if isRacDatabaseMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(racDatabase, racDatabaseFinalizer) {
			// Run cleanup
			if err := r.cleanupRacDatabase(req, ctx, racDatabase); err != nil {
				return true, err
			}

			// Remove finalizer
			if err := r.patchFinalizer(ctx, racDatabase, false); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return true, err
			}

			log.Info("Successfully removed RacDatabase finalizer")
			return true, nil
		}

		// Finalizer already gone, just let K8s delete it
		return true, nil
	}

	// Add finalizer for this CR if not present
	if !controllerutil.ContainsFinalizer(racDatabase, racDatabaseFinalizer) {
		if err := r.patchFinalizer(ctx, racDatabase, true); err != nil {
			log.Error(err, "Failed to add finalizer")
			return false, err
		}
	}
	return false, nil
}

// patchFinalizer updates the finalizer for the given resource
// patchFinalizer adds or removes the custom finalizer using a merge patch so
// the operator can manage cleanup semantics during deletion.
func (r *RacDatabaseReconciler) patchFinalizer(ctx context.Context, racDatabase *racdb.RacDatabase, add bool) error {
	const maxRetries = 5
	for attempt := 1; attempt <= maxRetries; attempt++ {
		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      racDatabase.Name,
			Namespace: racDatabase.Namespace,
		}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		if add {
			controllerutil.AddFinalizer(latest, racDatabaseFinalizer)
		} else {
			controllerutil.RemoveFinalizer(latest, racDatabaseFinalizer)
		}
		if err := r.Client.Patch(ctx, latest, client.MergeFrom(original)); err != nil {
			if apierrors.IsConflict(err) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("failed to patch finalizer after retries")
}

// cleanupRacDatabase handles resource teardown for RAC objects, including
// waiting for StatefulSets and services to drain before final removal.
func (r *RacDatabaseReconciler) cleanupRacDatabase(
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
) error {
	log := r.Log.WithValues("cleanupRacDatabase", req.NamespacedName)
	cd := racDatabase.Spec.ClusterDetails
	if cd == nil {
		log.Error(nil, "ClusterDetails is nil in cleanup")
		return fmt.Errorf("internal error: ClusterDetails is nil in cleanup")
	}

	log.Info("Running cleanup for RacDatabase (cluster style)")
	for i := 0; i < cd.NodeCount; i++ {
		nodeName := fmt.Sprintf("%s%d", cd.RacNodeName, i+1)
		sfSetFound, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			sfSetFound = nil
		}
		if sfSetFound != nil {
			log.Info("Deleting RAC Statefulset " + sfSetFound.Name)
			if err := r.Client.Delete(ctx, sfSetFound); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}

		cmName := fmt.Sprintf("%s%s-cmap", nodeName, racDatabase.Name)
		configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
		if err == nil {
			log.Info("Deleting RAC Configmap " + configMapFound.Name)
			if err = r.Client.Delete(ctx, configMapFound); err != nil {
				return err
			}
		}
		if err := raccommon.DelRacSwPvcClusterStyle(racDatabase, cd, i, r.Client, r.Log); err != nil {
			return err
		}
	}

	daemonSetName := "disk-check-daemonset"
	daemonSet := &appsv1.DaemonSet{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: daemonSetName, Namespace: racDatabase.Namespace}, daemonSet)
	if err == nil {
		if err = r.Client.Delete(ctx, daemonSet); err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	for _, dg := range racDatabase.Spec.AsmStorageDetails {
		for _, diskName := range dg.Disks {
			if err := raccommon.DelRacPvc(racDatabase, diskName, dg.Name, r.Client, r.Log); err != nil {
				return err
			}
			if isRawAsmDiskGroup(dg, racDatabase.Spec.StorageClass) {
				if err := raccommon.DelRacPv(racDatabase, diskName, dg.Name, r.Client, r.Log); err != nil {
					return err
				}
			}
		}
	}

	for _, svcType := range []string{"vip", "local", "onssvc", "lsnrsvc", "scansvc", "scan"} {
		if err := r.deleteRacServices(req, ctx, racDatabase, svcType); err != nil {
			return err
		}
	}

	log.Info("Successfully cleaned up RacDatabase (cluster style)")
	return nil
}

// deleteRacServices removes all RAC-related Services, cleaning up network
// endpoints as part of instance teardown routines.
func (r *RacDatabaseReconciler) deleteRacServices(
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	svcType string,
) error {
	log := r.Log.WithValues("deleteRacServices", req.NamespacedName)

	cluster := racDatabase.Spec.ClusterDetails
	if cluster == nil {
		log.Error(nil, "ClusterDetails is nil during service deletion")
		return fmt.Errorf("ClusterDetails is nil")
	}

	for i := 0; i < cluster.NodeCount; i++ {
		svcName := raccommon.GetClusterSvcName(racDatabase, cluster, i, svcType)
		svc := &corev1.Service{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: svcName, Namespace: racDatabase.Namespace}, svc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		log.Info("Deleting RAC Service " + svc.Name)
		if err := r.Client.Delete(ctx, svc); err != nil {
			return client.IgnoreNotFound(err)
		}
	}
	return nil
}

// cleanupRacInstance coordinates instance-level cleanup such as PVC removal,
// StatefulSet deletion, and ASM reconfiguration when instances are removed.
func (r *RacDatabaseReconciler) cleanupRacInstance(
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	oldSpec *racdb.RacDatabaseSpec) (int32, error) {
	log := r.Log.WithValues("cleanupRacInstance", req.NamespacedName)
	if oldSpec == nil || oldSpec.ClusterDetails == nil || oldSpec.ClusterDetails.NodeCount <= 0 {
		return 0, nil
	}
	cd := racDatabase.Spec.ClusterDetails
	if cd == nil {
		log.Error(nil, "ClusterDetails is nil in cleanup instance")
		return 0, fmt.Errorf("internal error: ClusterDetails is nil in new style cleanup instance")
	}

	// 1. Build sets of previous and current nodes
	oldNodeNames := []string{}
	currentNodeNames := []string{}

	// Assume 'oldSpec' is obtained from annotation or similar
	if oldSpec.ClusterDetails.NodeCount > 0 {
		for idx := 0; idx < oldSpec.ClusterDetails.NodeCount; idx++ {
			oldNodeNames = append(oldNodeNames, fmt.Sprintf("%s%d", oldSpec.ClusterDetails.RacNodeName, idx+1))
		}
	}

	// Current node names
	for idx := 0; idx < cd.NodeCount; idx++ {
		currentNodeNames = append(currentNodeNames, fmt.Sprintf("%s%d", cd.RacNodeName, idx+1))
	}

	// 2. Create a lookup set for current nodes
	currentNodeSet := make(map[string]struct{}, len(currentNodeNames))
	for _, name := range currentNodeNames {
		currentNodeSet[name] = struct{}{}
	}

	// 3. Clean up only nodes in old but not in new
	deletedCount := 0
	for _, name := range oldNodeNames {
		if _, stillExists := currentNodeSet[name]; !stillExists {
			log.Info("Starting delete RAC instance for node " + name)
			err := r.deleteRACInst(name, req, ctx, racDatabase)
			if err != nil {
				log.Info(fmt.Sprintf("Error occurred during cluster node %s deletion", name))
				return int32(deletedCount), err
			}
			deletedCount++
		}
	}
	return int32(deletedCount), nil

}

// deleteRACInst drives deletion logic for individual RAC instances, ensuring
// Kubernetes objects and ASM state are cleaned up safely.
func (r *RacDatabaseReconciler) deleteRACInst(
	nodeName string,
	req ctrl.Request,
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
) error {
	log := r.Log.WithValues("deleteRACInst", req.NamespacedName)
	podName := nodeName + "-0"
	if err := raccommon.DelRacNode(podName, racDatabase, r.kubeClient, r.kubeConfig, r.Log); err != nil {
		return err
	}

	sfSetFound, err := raccommon.CheckSfset(nodeName, racDatabase, r.Client)
	if err == nil && sfSetFound != nil {
		if err = r.Client.Delete(context.Background(), sfSetFound); err != nil {
			return err
		}
	}

	cmName := nodeName + racDatabase.Name + "-cmap"
	configMapFound, err := raccommon.CheckConfigMap(racDatabase, cmName, r.Client)
	if err == nil {
		if err = r.Client.Delete(context.Background(), configMapFound); err != nil {
			return err
		}
	}

	nodeIndex := extractNodeIndexFromName(nodeName)
	for _, svcType := range []string{"vip", "local", "onssvc", "lsnrsvc", "nodeport"} {
		svcName := raccommon.GetClusterSvcName(racDatabase, racDatabase.Spec.ClusterDetails, nodeIndex, svcType)
		svcFound, err := raccommon.CheckRacSvcForCluster(
			racDatabase, racDatabase.Spec.ClusterDetails, nodeIndex, svcType, svcName, r.Client)
		if err == nil && svcFound != nil {
			if err = r.Client.Delete(context.Background(), svcFound); err != nil {
				return err
			}
			log.Info("Deleted Svc", "svcName", svcName)
		}
	}

	_, endp, err := raccommon.GetDBLsnrEndPointsForCluster(racDatabase)
	if err != nil {
		return fmt.Errorf("endpoint generation error in delete block")
	}
	healthyNode, err := raccommon.GetHealthyNode(racDatabase)
	if err != nil {
		return fmt.Errorf("no healthy node found in the cluster to perform delete node operator. manual intervention required")
	}
	if racDatabase.Spec.ClusterDetails != nil && racDatabase.Spec.ClusterDetails.NodeCount == 3 {
		if err := raccommon.UpdateAsmCount(racDatabase.Spec.ConfigParams.GridHome, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log); err != nil {
			log.Info("error occurred while updating the asm count")
		}
	}
	if err = raccommon.UpdateTCPPort(racDatabase.Spec.ConfigParams.GridHome, endp, "dblsnr", healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log); err != nil {
		log.Info("error occurred while updating the listener tcp ports")
	}
	if err = raccommon.UpdateScanEP(racDatabase.Spec.ConfigParams.GridHome, racDatabase.Spec.ScanSvcName, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log); err != nil {
		log.Info("error occurred while updating the scan end points")
	}
	if err = raccommon.UpdateCDP(racDatabase.Spec.ConfigParams.GridHome, healthyNode, racDatabase, r.kubeClient, r.kubeConfig, r.Log); err != nil {
		log.Info("error occurred while updating the CDP")
	}
	log.Info("Successfully cleaned up RacInstance")
	return nil
}

// extractNodeIndexFromName parses the numeric suffix from a RAC node name and
// converts it to a zero-based index, accommodating optional pod ordinal suffixes.
func extractNodeIndexFromName(nodeName string) int {
	// Remove a "-0" pod ordinal suffix if present
	base := nodeName
	if strings.HasSuffix(nodeName, "-0") {
		base = strings.TrimSuffix(nodeName, "-0")
	}
	// Find the last group of digits in the base name
	re := regexp.MustCompile(`(\d+)$`)
	match := re.FindStringSubmatch(base)
	if len(match) > 1 {
		if idx, err := strconv.Atoi(match[1]); err == nil {
			return idx - 1
		}
	}
	return 0
}

// SetupWithManager registers the reconciler with controller-runtime so RAC
// resources and their owned objects are watched with the configured
// concurrency.
func (r *RacDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&racdb.RacDatabase{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		Owns(&appsv1.StatefulSet{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 100}). //ReconcileHandler is never invoked concurrently with the same object.
		Complete(r)
}

const oldRacSpecAnnotation = "racdatabases.database.oracle.com/old-spec"

// GetOldSpec returns the previous RAC spec snapshot stored in annotations,
// allowing reconciles to detect specification changes. Missing annotations yield
// a nil result.
func (r *RacDatabaseReconciler) GetOldSpec(racDatabase *racdb.RacDatabase) (*racdb.RacDatabaseSpec, error) {
	// Check if annotations exist
	annotations := racDatabase.GetAnnotations()
	if annotations == nil {
		r.Log.Info("No annotations found on RacDatabase")
		return nil, nil
	}

	// Retrieve the specific annotation
	val, ok := annotations[oldRacSpecAnnotation]
	if !ok {
		r.Log.Info("Old spec annotation not found")
		return nil, nil
	}

	// Unmarshal the old spec JSON string
	specBytes := []byte(val)
	var oldSpec racdb.RacDatabaseSpec
	if err := json.Unmarshal(specBytes, &oldSpec); err != nil {
		r.Log.Error(err, "Failed to unmarshal old spec from annotation")
		return nil, fmt.Errorf("failed to unmarshal old spec from annotation: %w", err)
	}

	// r.Log.Info("Successfully retrieved old spec from annotation", "spec", oldSpec)
	return &oldSpec, nil
}

// SetCurrentSpecAndObservedGeneration stores the current spec as an annotation with retry logic,
// updating only if the annotation value has changed.
// SetCurrentSpecAndObservedGeneration persists the current spec into annotations with conflict
// retries, enabling future reconciles to detect spec changes accurately.
func (r *RacDatabaseReconciler) SetCurrentSpecAndObservedGeneration(
	ctx context.Context,
	racDatabase *racdb.RacDatabase,
	req ctrl.Request,
) error {

	cName, fName := resolveGridResponseFileRef(racDatabase.Spec.ConfigParams)

	// Normalize DG defaults
	if err := setRacDgFromStatusAndSpecWithMinimumDefaultsforRAC(
		racDatabase, r.Client, cName, fName,
	); err != nil {
		r.Log.Info("Failed to set disk group defaults")
		return err
	}

	// Serialize spec
	currentSpecData, err := json.Marshal(racDatabase.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal current spec: %w", err)
	}
	currentSpecStr := string(currentSpecData)

	// Ensure annotations map
	if racDatabase.Annotations == nil {
		racDatabase.Annotations = map[string]string{}
	}

	// Detect change
	existingSpecStr := racDatabase.Annotations[oldRacSpecAnnotation]
	specChanged := existingSpecStr != currentSpecStr

	// -------------------------------
	// STEP 1: PATCH METADATA (ONLY)
	// -------------------------------
	if specChanged {
		original := racDatabase.DeepCopy()
		racDatabase.Annotations[oldRacSpecAnnotation] = currentSpecStr

		if err := r.Patch(ctx, racDatabase, client.MergeFrom(original)); err != nil {
			return err
		}
	}

	// -------------------------------
	// STEP 2: RE-FETCH OBJECT
	// -------------------------------
	if err := r.Get(ctx, req.NamespacedName, racDatabase); err != nil {
		return err
	}

	// -------------------------------
	// STEP 3: UPDATE STATUS
	// -------------------------------
	if racDatabase.Status.ObservedGeneration != racDatabase.Generation {
		original := racDatabase.DeepCopy()
		racDatabase.Status.ObservedGeneration = racDatabase.Generation

		if err := r.Status().Patch(ctx, racDatabase, client.MergeFrom(original)); err != nil {
			return err
		}
	}

	r.Log.Info("RAC spec annotation and observedGeneration updated successfully")
	return nil
}

// updateRacNodeStatusForCluster updates the RAC node status for a cluster with retry logic.
// It synchronizes node status changes from the working RacDatabase object to the API server,
// handling resource version conflicts and retrying up to maxRetries times with exponential backoff.
//
// Parameters:
//   - racDatabase: The RacDatabase instance containing updated node status information
//   - ctx: The context for API operations and cancellation
//   - req: The reconciliation request containing the namespaced name of the resource
//   - clusterSpec: The cluster specification details for the RAC database
//   - nodeIndex: The index of the node being updated within the cluster
//   - state: The new state to set for the RAC node
//
// The function performs the following steps:
//  1. Calls the cluster-style status update logic to prepare status data
//  2. Attempts to fetch the latest instance from the Kubernetes API
//  3. Syncs the RacNodes status from the working object to the latest instance
//  4. Updates the resource version and persists changes to the API server
//  5. Retries on conflicts (up to 5 times) with a 2-second delay between attempts
//  6. Logs errors and completion status for troubleshooting
func (r *RacDatabaseReconciler) updateRacNodeStatusForCluster(
	racDatabase *racdb.RacDatabase,
	ctx context.Context,
	req ctrl.Request,
	clusterSpec *racdb.RacClusterDetailSpec,
	nodeIndex int,
	state string,
) {
	const maxRetries = 5
	const retryDelay = 2 * time.Second

	var lastErr error
	var failedUpdate bool

	// Call the cluster-style status update logic
	raccommon.UpdateRacNodeStatusDataForCluster(
		racDatabase, ctx, req, clusterSpec, nodeIndex, state,
		r.kubeClient, r.kubeConfig, r.Log, r.Client,
	)

	// Retry update with resource version logic
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest instance from the API
		latestInstance := &racdb.RacDatabase{}
		err := r.Client.Get(ctx, req.NamespacedName, latestInstance)
		if err != nil {
			r.Log.Error(err, "Failed to fetch the latest version of RAC instance")
			lastErr = err
			continue // Continue to retry
		}
		// Sync RACNodes from working object
		latestInstance.Status.RacNodes = racDatabase.Status.RacNodes

		// If you need further merging/ensuring logic for cluster style, add here

		racDatabase.ResourceVersion = latestInstance.ResourceVersion
		err = r.Status().Update(ctx, racDatabase)
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Info("Conflict detected in updateRacNodeStatusForCluster, retrying...", "attempt", attempt+1)
				time.Sleep(retryDelay)
				failedUpdate = true
				continue // Retry
			}
			r.Log.Error(err, "Failed to update the RAC instance (cluster)")
			lastErr = err
			failedUpdate = true
			continue // Continue to retry
		}
		r.Log.Info("RAC Object updated with updateRacNodeStatusForCluster")
		failedUpdate = false
		break // Break if update succeeded
	}
	// Log if all retries exhausted
	if failedUpdate {
		r.Log.Info("Failed to update RAC instance (cluster) after 5 attempts", "lastErr", lastErr)
	}
}

// waitForPodReady polls the specified pod until it is in Ready state or the timeout is reached, returning an error if the pod does not become ready in time.
func (r *RacDatabaseReconciler) waitForPodReady(ctx context.Context, podName string, namespace string, timeout time.Duration) error {
	t := time.After(timeout)
	for {
		select {
		case <-t:
			return fmt.Errorf("timed out waiting for pod %s to become ready", podName)
		default:
			pod := &corev1.Pod{}
			err := r.Get(ctx, client.ObjectKey{Name: podName, Namespace: namespace}, pod)
			if err != nil {
				// Pod might not exist yet - continue
				time.Sleep(5 * time.Second)
				continue
			}
			// Check for Ready condition
			ready := false
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if ready {
				return nil
			}
			time.Sleep(5 * time.Second)
		}
	}
}

// updateStatusWithRetry attempts to update the status of the RacDatabase resource with retry logic to handle conflicts.
func (r *RacDatabaseReconciler) updateStatusWithRetry(
	ctx context.Context,
	req ctrl.Request,
	apply func(latest *racdb.RacDatabase),
) error {

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		latest := &racdb.RacDatabase{}
		if err := r.Client.Get(ctx, req.NamespacedName, latest); err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}

		// Apply desired mutation on latest object
		apply(latest)

		err := r.Status().Update(ctx, latest)
		if err == nil {
			return nil
		}
		if apierrors.IsConflict(err) {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		return err
	}

	return fmt.Errorf("status update failed after retries: %w", lastErr)
}

// updateStatusNoGetRetry attempts to update the status of the RacDatabase resource without re-fetching the latest version on conflict, instead relying on the caller to ensure the object is up-to-date before calling this function.
func (r *RacDatabaseReconciler) updateStatusNoGetRetry(
	ctx context.Context,
	obj *racdb.RacDatabase,
) error {

	const (
		maxRetries = 5
		retryDelay = 200 * time.Millisecond
	)

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {

		err := r.Status().Update(ctx, obj)
		if err == nil {
			return nil
		}

		if apierrors.IsConflict(err) {
			lastErr = err

			latest := &racdb.RacDatabase{}
			if getErr := r.Client.Get(
				ctx,
				types.NamespacedName{
					Name:      obj.Name,
					Namespace: obj.Namespace,
				},
				latest,
			); getErr != nil {
				return getErr
			}

			// --------------------------------------------------
			// Preserve ASM disk groups from latest object
			// --------------------------------------------------
			obj.Status.AsmDiskGroups = latest.Status.AsmDiskGroups

			// Refresh resourceVersion only
			obj.ResourceVersion = latest.ResourceVersion

			time.Sleep(retryDelay)
			continue
		}

		return err
	}

	return fmt.Errorf("status update failed after retries: %w", lastErr)
}
