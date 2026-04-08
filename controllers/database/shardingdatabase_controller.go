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

//nolint:staticcheck,unused,revive // legacy sharding reconciliation helpers/signatures are retained for compatibility.
package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dataguardcommon "github.com/oracle/oracle-database-operator/commons/dataguard"
	dgsharding "github.com/oracle/oracle-database-operator/commons/dataguard/sharding"
	sharedk8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	sharedk8sutil "github.com/oracle/oracle-database-operator/commons/k8sutil"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	"github.com/oracle/oracle-database-operator/commons/shapes"
	shardingv1 "github.com/oracle/oracle-database-operator/commons/sharding"
)

// ShardingDatabaseReconciler reconciles a ShardingDatabase object
type ShardingDatabaseReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	kubeConfig *rest.Config
	Recorder   record.EventRecorder
	APIReader  client.Reader
}

type phaseResult struct {
	wait         bool
	requeueAfter time.Duration
	err          error
	reason       string
	message      string
}

type conditionSet struct {
	observedGen int64
	ready       metav1.Condition
	reconciling metav1.Condition
	degraded    metav1.Condition
}

const (
	reconcilingType      = lockpolicy.DefaultReconcilingConditionType
	updateLockReason     = lockpolicy.DefaultUpdateLockReason
	updateLockRequeue    = 15 * time.Second
	statusRefreshRequeue = 60 * time.Second

	lockOverrideAnnotation = lockpolicy.DefaultOverrideAnnotation

	credentialSyncConditionType         = "CredentialSync"
	credentialSyncHashAnnotation        = "database.oracle.com/credential-sync-hash"
	credentialSyncRetryCountAnnotation  = "database.oracle.com/credential-sync-retry-count"
	credentialSyncNextRetryAtAnnotation = "database.oracle.com/credential-sync-next-retry-at"
	credentialSyncLastErrorAnnotation   = "database.oracle.com/credential-sync-last-error"
	credentialRetryInitialBackoff       = 30 * time.Second
	credentialRetryMaxBackoff           = 10 * time.Minute

	tdeKeyExportedAnnotation       = "database.oracle.com/tde-key-exported"
	tdeKeyImportedShardsAnnotation = "database.oracle.com/tde-key-imported-shards"
	tdeKeyRefreshAnnotation        = "database.oracle.com/tde-key-refresh"
)

var (
	exportTDEKeyFn = shardingv1.ExportTDEKey
	importTDEKeyFn = shardingv1.ImportTDEKey
)

// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.oracle.com,resources=shardingdatabases/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods;pods/log;pods/exec;secrets;containers;services;events;configmaps;persistentvolumeclaims;namespaces,verbs=get;list;watch;create;update;patch;delete
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
// Reconcile orchestrates phase-based convergence for a ShardingDatabase resource.
func (r *ShardingDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//ctx := context.Background()
	//_ = r.Log.WithValues("shardingdatabase", req.NamespacedName)
	inst := &databasev4.ShardingDatabase{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if inst.DeletionTimestamp == nil {
		if locked, lockGen, lockMsg := isUpdateLockActive(inst); locked && inst.Generation > lockGen {
			enforceLock := true
			if lastSuccSpec, err := inst.GetLastSuccessfulSpec(); err != nil {
				r.logLegacy("WARNING", "Unable to evaluate conditional update lock; enforcing lock. err="+err.Error(), err, inst, r.Log)
			} else if lastSuccSpec != nil {
				enforceLock = shouldEnforceUpdateLock(lastSuccSpec, &inst.Spec)
			}

			if enforceLock {
				overrideEnabled, overrideMsg := isUpdateLockOverrideEnabled(inst)
				if overrideEnabled {
					r.logLegacy("WARNING", "Bypassing update lock due to break-glass override. "+overrideMsg, nil, inst, r.Log)
				} else {
					status := inst.Status.DeepCopy()
					finalCond := newConditionSet(inst.Generation)
					msg := fmt.Sprintf("Previous update is still in progress (lockedGeneration=%d). %s", lockGen, lockMsg)
					if strings.TrimSpace(overrideMsg) != "" {
						msg = msg + " Break-glass override rejected: " + overrideMsg
					}
					r.markReconciling(finalCond, updateLockReason, msg)
					_ = r.flushStatus(ctx, req.NamespacedName, status, finalCond)
					return ctrl.Result{RequeueAfter: updateLockRequeue}, nil
				}
			} else {
				r.logLegacy("INFO", "Update lock not enforced for non-topology spec change", nil, inst, r.Log)
			}
		}
	}

	status := inst.Status.DeepCopy()
	// Phase 2: Condition + Status Writer (single-writer pattern)
	finalCond := newConditionSet(inst.Generation)

	// Phase 3: Delete
	pr := r.phaseDelete(ctx, inst, status, finalCond)
	if pr.err != nil {
		r.markDegraded(finalCond, pr.reason, pr.message)
		_ = r.flushStatus(ctx, req.NamespacedName, status, finalCond)
		return ctrl.Result{}, pr.err
	}
	if pr.wait {
		r.markReconciling(finalCond, pr.reason, pr.message)
		_ = r.flushStatus(ctx, req.NamespacedName, status, finalCond)
		return ctrl.Result{RequeueAfter: pr.requeueAfter}, nil
	}

	//Calling different pahses from following block
	phases := []func(context.Context, *databasev4.ShardingDatabase, *databasev4.ShardingDatabaseStatus, *conditionSet) phaseResult{
		r.phaseValidateAndPlan,
		r.phaseEnsureCoreResources,
		r.phaseValidateCoreReady,
		r.phasePrimaryShardOps,
		r.phaseStandbyShardOps,
		r.phaseScaleOps,
		r.phasePostSync,
	}

	for _, p := range phases {
		pr = p(ctx, inst, status, finalCond)
		if pr.err != nil {
			r.markDegraded(finalCond, pr.reason, pr.message)
			_ = r.flushStatus(ctx, req.NamespacedName, status, finalCond)
			return ctrl.Result{}, pr.err
		}
		if pr.wait {
			reason := pr.reason
			message := pr.message
			if isMutatingProgressReason(pr.reason) {
				reason = updateLockReason
				message = fmt.Sprintf("%s: %s", pr.reason, pr.message)
			}
			r.markReconciling(finalCond, reason, message)
			_ = r.flushStatus(ctx, req.NamespacedName, status, finalCond)
			return ctrl.Result{RequeueAfter: pr.requeueAfter}, nil
		}
	}

	credentialRetryAfter := r.syncCredentialRotationNonBlocking(ctx, inst, status)

	r.markReady(finalCond)
	if err := r.flushStatus(ctx, req.NamespacedName, status, finalCond); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if credentialRetryAfter > 0 {
		return ctrl.Result{RequeueAfter: credentialRetryAfter}, nil
	}
	return ctrl.Result{RequeueAfter: statusRefreshRequeue}, nil
}

// Phase 2 :Condition + Status Writer (single-writer pattern)
// newConditionSet initializes Ready/Reconciling/Degraded conditions for one reconcile pass.
func newConditionSet(gen int64) *conditionSet {
	now := metav1.Now()
	return &conditionSet{
		observedGen: gen,
		ready:       metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, LastTransitionTime: now, ObservedGeneration: gen},
		reconciling: metav1.Condition{Type: "Reconciling", Status: metav1.ConditionTrue, LastTransitionTime: now, ObservedGeneration: gen},
		degraded:    metav1.Condition{Type: "Degraded", Status: metav1.ConditionFalse, LastTransitionTime: now, ObservedGeneration: gen},
	}
}

// flushStatus writes the status draft and conditions using conflict-retry semantics.
func (r *ShardingDatabaseReconciler) flushStatus(
	ctx context.Context,
	key types.NamespacedName,
	draft *databasev4.ShardingDatabaseStatus,
	conds *conditionSet,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		curr.Status = *draft
		meta.SetStatusCondition(&curr.Status.CrdStatus, conds.ready)
		meta.SetStatusCondition(&curr.Status.CrdStatus, conds.reconciling)
		meta.SetStatusCondition(&curr.Status.CrdStatus, conds.degraded)
		return r.Status().Update(ctx, curr)
	})
}

// Phase 3: Delete
// phaseDelete ensures finalizer presence and executes cleanup when deletion is requested.
func (r *ShardingDatabaseReconciler) phaseDelete(
	_ context.Context,
	inst *databasev4.ShardingDatabase,
	_ *databasev4.ShardingDatabaseStatus,
	_ *conditionSet,
) phaseResult {
	if inst.DeletionTimestamp == nil {
		if !controllerutil.ContainsFinalizer(inst, shardingv1.ShardingDatabaseFinalizer) {
			if err := sharedk8sutil.AddFinalizerAndPatch(r.Client, inst, shardingv1.ShardingDatabaseFinalizer); err != nil {
				return phaseResult{err: err, reason: "FinalizerAddFailed", message: err.Error()}
			}
		}
		return phaseResult{}
	}
	if err := r.finalizeShardingDatabase(inst); err != nil {
		return phaseResult{wait: true, requeueAfter: 15 * time.Second, reason: "FinalizeRetry", message: err.Error()}
	}
	if err := sharedk8sutil.RemoveFinalizerAndPatch(r.Client, inst, shardingv1.ShardingDatabaseFinalizer); err != nil {
		return phaseResult{err: err, reason: "FinalizerRemoveFailed", message: err.Error()}
	}
	return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "Deleting", message: "Waiting object removal"}
}

// Phase 4 : Validate + Plan
// phaseValidateAndPlan validates spec and applies prerequisite spec patches before core reconciliation.
func (r *ShardingDatabaseReconciler) phaseValidateAndPlan(
	ctx context.Context, inst *databasev4.ShardingDatabase, _ *databasev4.ShardingDatabaseStatus, _ *conditionSet,
) phaseResult {
	plog := r.phaseLogger(inst, "validate_plan")
	if err := r.validateSpex(inst); err != nil {
		return phaseResult{err: err, reason: "SpecInvalid", message: err.Error()}
	}

	origStandbyPlan := inst.DeepCopy()
	standbyPlanChanged := r.ensureStandbyShardNumFromConfig(inst)
	if standbyPlanChanged {
		if perr := r.Patch(ctx, inst, client.MergeFrom(origStandbyPlan)); perr != nil {
			plog.Error(perr, "failed to patch shardNum from standbyConfig", "reason", "StandbyPlanPatchRetry")
			return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "StandbyPlanPatchRetry", message: perr.Error()}
		}
		plog.Info("patched shardNum values derived from standbyConfig", "reason", "StandbyPlanPatched")
		return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "StandbyPlanPatched", message: "Standby plan patched from standbyConfig"}
	}

	orig := inst.DeepCopy()
	changed, err := r.ensurePrimaryRefForStandby(ctx, inst)
	if err != nil {
		return phaseResult{wait: true, requeueAfter: 20 * time.Second, reason: "WaitingPrimaryRef", message: err.Error()}
	}
	if changed {
		if perr := r.Patch(ctx, inst, client.MergeFrom(orig)); perr != nil {
			plog.Error(perr, "failed to patch primaryDatabaseRef", "reason", "PrimaryRefPatchRetry")
			return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "PrimaryRefPatchRetry", message: perr.Error()}
		}
		plog.Info("patched primaryDatabaseRef for standby shards", "reason", "SpecPatched")
		return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "SpecPatched", message: "Standby primary refs patched"}
	}

	if err := r.syncDgPairsFromStandbyConfig(inst); err != nil {
		plog.Error(err, "failed to sync dg pair status from standbyConfig", "reason", "DgPairStatusSyncRetry")
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "DgPairStatusSyncRetry", message: err.Error()}
	}

	origScaleOut := inst.DeepCopy()
	scaleOutChanged := r.applyReplicaScaleOutUnmarks(inst)
	if scaleOutChanged {
		if perr := r.Patch(ctx, inst, client.MergeFrom(origScaleOut)); perr != nil {
			plog.Error(perr, "failed to patch scale-out isDelete resets", "reason", "ScaleOutUnmarkPatchRetry")
			return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "ScaleOutUnmarkPatchRetry", message: perr.Error()}
		}
		plog.Info("patched scale-out isDelete resets into spec", "reason", "ScaleOutSpecPatched")
		return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "ScaleOutSpecPatched", message: "Scale-out unmark patch applied"}
	}

	origScale := inst.DeepCopy()
	scaleChanged := r.applyReplicaScaleInMarks(inst)
	if scaleChanged {
		if perr := r.Patch(ctx, inst, client.MergeFrom(origScale)); perr != nil {
			plog.Error(perr, "failed to patch scale-in isDelete marks", "reason", "ScaleInMarkPatchRetry")
			return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "ScaleInMarkPatchRetry", message: perr.Error()}
		}
		plog.Info("patched scale-in isDelete marks into spec", "reason", "ScaleInSpecPatched")
		return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "ScaleInSpecPatched", message: "Scale-in mark patch applied"}
	}

	if cerr := r.cleanupOrphanShardResources(inst); cerr != nil {
		plog.Error(cerr, "failed to cleanup orphan shard resources", "reason", "OrphanCleanupRetry")
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "OrphanCleanupRetry", message: cerr.Error()}
	}

	return phaseResult{}
}

// phaseEnsureCoreResources creates/updates core Services and StatefulSets for catalog, GSM, and shard members.
func (r *ShardingDatabaseReconciler) phaseEnsureCoreResources(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = ctx
	_ = st
	_ = c

	var i int32
	externalCatalogMode := r.isExternalCatalogMode(inst)
	if externalCatalogMode {
		if err := r.validateExternalCatalogRef(inst); err != nil {
			return phaseResult{err: err, reason: "ExternalCatalogConfigInvalid", message: err.Error()}
		}
	}

	// Service setup for Catalog
	if !externalCatalogMode {
		for i = 0; i < int32(len(inst.Spec.Catalog)); i++ {
			oraCatalogSpec := inst.Spec.Catalog[i]
			if _, err := r.createService(inst, shardingv1.BuildServiceDefForCatalog(inst, 0, oraCatalogSpec, "local")); err != nil {
				return phaseResult{err: err, reason: "CatalogServiceCreateFailed", message: err.Error()}
			}
			if inst.Spec.IsExternalSvc {
				if _, err := r.createService(inst, shardingv1.BuildServiceDefForCatalog(inst, 0, oraCatalogSpec, "external")); err != nil {
					return phaseResult{err: err, reason: "CatalogExternalServiceCreateFailed", message: err.Error()}
				}
			}
		}
	}

	// Catalog StatefulSet setup
	if !externalCatalogMode {
		for i = 0; i < int32(len(inst.Spec.Catalog)); i++ {
			oraCatalogSpec := inst.Spec.Catalog[i]
			if len(oraCatalogSpec.Name) > 9 {
				return phaseResult{
					err:     fmt.Errorf("catalog name cannot be greater than 9 characters"),
					reason:  "CatalogNameTooLong",
					message: "catalog name cannot be greater than 9 characters",
				}
			}
			catalogSts, err := shardingv1.BuildStatefulSetForCatalog(inst, oraCatalogSpec)
			if err != nil {
				return phaseResult{err: err, reason: "CatalogMemoryValidationFailed", message: err.Error()}
			}
			if _, err := r.deployStatefulSet(
				inst,
				catalogSts,
				"CATALOG",
				oraCatalogSpec.Name,
				oraCatalogSpec.Resources,
			); err != nil {
				return phaseResult{err: err, reason: "CatalogStatefulSetFailed", message: err.Error()}
			}
			if err := r.reconcilePVCExpansion(inst, oraCatalogSpec.Name, normalizedPVCResizeSpecs(oraCatalogSpec.Name, oraCatalogSpec.StorageSizeInGb, oraCatalogSpec.DisableDefaultLogVolumeClaims, oraCatalogSpec.AdditionalPVCs)); err != nil {
				return phaseResult{err: err, reason: "CatalogPVCExpandFailed", message: err.Error()}
			}
		}
	}

	// Service setup for GSM
	for i = 0; i < int32(len(inst.Spec.Gsm)); i++ {
		oraGsmSpec := inst.Spec.Gsm[i]
		if _, err := r.createService(inst, shardingv1.BuildServiceDefForGsm(inst, 0, oraGsmSpec, "local")); err != nil {
			return phaseResult{err: err, reason: "GsmServiceCreateFailed", message: err.Error()}
		}
		if inst.Spec.IsExternalSvc {
			if _, err := r.createService(inst, shardingv1.BuildServiceDefForGsm(inst, 0, oraGsmSpec, "external")); err != nil {
				return phaseResult{err: err, reason: "GsmExternalServiceCreateFailed", message: err.Error()}
			}
		}
	}

	// GSM StatefulSet setup
	for i = 0; i < int32(len(inst.Spec.Gsm)); i++ {
		oraGsmSpec := inst.Spec.Gsm[i]
		if _, err := r.deployStatefulSet(
			inst,
			shardingv1.BuildStatefulSetForGsm(inst, oraGsmSpec),
			"GSM",
			oraGsmSpec.Name,
			oraGsmSpec.Resources,
		); err != nil {
			return phaseResult{err: err, reason: "GsmStatefulSetFailed", message: err.Error()}
		}
		if err := r.reconcilePVCExpansion(inst, oraGsmSpec.Name, normalizedGsmPVCResizeSpecs(oraGsmSpec.Name, oraGsmSpec.StorageSizeInGb, oraGsmSpec.DisableDefaultLogVolumeClaims, oraGsmSpec.AdditionalPVCs)); err != nil {
			return phaseResult{err: err, reason: "GsmPVCExpandFailed", message: err.Error()}
		}
	}

	// Service setup for Shard
	for i = 0; i < int32(len(inst.Spec.Shard)); i++ {
		oraShardSpec := inst.Spec.Shard[i]
		if len(oraShardSpec.Name) > 9 {
			return phaseResult{
				err:     fmt.Errorf("shard name cannot be greater than 9 characters"),
				reason:  "ShardNameTooLong",
				message: "shard name cannot be greater than 9 characters",
			}
		}
		if shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, inst, r.Log) {
			continue
		}
		if _, err := r.createService(inst, shardingv1.BuildServiceDefForShard(inst, 0, oraShardSpec, "local")); err != nil {
			return phaseResult{err: err, reason: "ShardServiceCreateFailed", message: err.Error()}
		}
		if inst.Spec.IsExternalSvc {
			if _, err := r.createService(inst, shardingv1.BuildServiceDefForShard(inst, 0, oraShardSpec, "external")); err != nil {
				return phaseResult{err: err, reason: "ShardExternalServiceCreateFailed", message: err.Error()}
			}
		}
	}

	// Shard StatefulSet setup
	for i = 0; i < int32(len(inst.Spec.Shard)); i++ {
		oraShardSpec := inst.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, inst, r.Log) {
			continue
		}
		shardSts, err := shardingv1.BuildStatefulSetForShard(inst, oraShardSpec)
		if err != nil {
			return phaseResult{err: err, reason: "ShardMemoryValidationFailed", message: err.Error()}
		}
		if _, err := r.deployStatefulSet(
			inst,
			shardSts,
			"SHARD",
			oraShardSpec.Name,
			oraShardSpec.Resources,
		); err != nil {
			return phaseResult{err: err, reason: "ShardStatefulSetFailed", message: err.Error()}
		}
		if err := r.reconcilePVCExpansion(inst, oraShardSpec.Name, normalizedPVCResizeSpecs(oraShardSpec.Name, oraShardSpec.StorageSizeInGb, oraShardSpec.DisableDefaultLogVolumeClaims, oraShardSpec.AdditionalPVCs)); err != nil {
			return phaseResult{err: err, reason: "ShardPVCExpandFailed", message: err.Error()}
		}
	}

	// Ordered shape reconcile (existing behavior: requeue on blocked/error)
	blocked, serr := r.reconcileOrderedShapeChanges(inst)
	if serr != nil {
		r.logLegacy("INFO", "Ordered shape reconcile failed: "+serr.Error(), nil, inst, r.Log)
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "OrderedShapeReconcileRetry", message: serr.Error()}
	}
	if blocked {
		r.logLegacy("INFO", "Ordered shape reconcile in progress. Requeue.", nil, inst, r.Log)
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "OrderedShapeReconcileBlocked", message: "Ordered shape reconcile in progress"}
	}

	// Ordered non-shape shard template reconcile (resources/security context/capabilities).
	// This is shard-only and one-by-one; shape-targeted shards are excluded to avoid double restarts.
	shardTemplateBlocked, shardTemplateErr := r.reconcileOrderedShardTemplateChanges(inst)
	if shardTemplateErr != nil {
		r.logLegacy("INFO", "Ordered shard template reconcile failed: "+shardTemplateErr.Error(), nil, inst, r.Log)
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "OrderedShardTemplateReconcileRetry", message: shardTemplateErr.Error()}
	}
	if shardTemplateBlocked {
		r.logLegacy("INFO", "Ordered shard template reconcile in progress. Requeue.", nil, inst, r.Log)
		return phaseResult{wait: true, requeueAfter: 30 * time.Second, reason: "OrderedShardTemplateReconcileBlocked", message: "Ordered shard template reconcile in progress"}
	}

	return phaseResult{}
}

type pvcResizeSpec struct {
	volumeName      string
	pvcName         string
	storageSizeInGb int32
}

func normalizedPVCResizeSpecs(ownerName string, baseStorageSize int32, disableDefaultLogPVCs bool, additionalPVCs []databasev4.AdditionalPVCSpec) []pvcResizeSpec {
	return normalizedPVCResizeSpecsWithDefaults(ownerName, baseStorageSize, disableDefaultLogPVCs, databasev4.DefaultOraDataMountPath, databasev4.DefaultDiagMountPath, additionalPVCs)
}

func normalizedGsmPVCResizeSpecs(ownerName string, baseStorageSize int32, disableDefaultLogPVCs bool, additionalPVCs []databasev4.AdditionalPVCSpec) []pvcResizeSpec {
	return normalizedPVCResizeSpecsWithDefaults(ownerName, baseStorageSize, disableDefaultLogPVCs, databasev4.DefaultGsmDataMountPath, databasev4.DefaultGsmDiagMountPath, additionalPVCs)
}

func normalizedPVCResizeSpecsWithDefaults(ownerName string, baseStorageSize int32, disableDefaultLogPVCs bool, baseMountPath, diagMountPath string, additionalPVCs []databasev4.AdditionalPVCSpec) []pvcResizeSpec {
	trimmedOwner := strings.TrimSpace(ownerName)
	specByPath := map[string]pvcResizeSpec{
		baseMountPath: {
			volumeName:      trimmedOwner + "-oradata-vol4",
			storageSizeInGb: baseStorageSize,
		},
	}
	if !disableDefaultLogPVCs {
		specByPath[diagMountPath] = pvcResizeSpec{
			volumeName:      trimmedOwner + "-diag-vol10",
			storageSizeInGb: databasev4.DefaultDiagSizeInGb,
		}
		specByPath[databasev4.DefaultGddLogMountPath] = pvcResizeSpec{
			volumeName:      trimmedOwner + "-gdd-vol11",
			storageSizeInGb: databasev4.DefaultGddLogSizeInGb,
		}
	}

	for i := range additionalPVCs {
		mountPath := strings.TrimSpace(additionalPVCs[i].MountPath)
		if mountPath == "" {
			continue
		}
		spec, exists := specByPath[mountPath]
		if !exists {
			hash := sha256.Sum256([]byte(trimmedOwner + ":" + mountPath))
			spec = pvcResizeSpec{
				volumeName:      trimmedOwner + "-extra-vol-" + hex.EncodeToString(hash[:])[:8],
				storageSizeInGb: additionalPVCs[i].StorageSizeInGb,
			}
		}
		if pvcName := strings.TrimSpace(additionalPVCs[i].PvcName); pvcName != "" {
			spec.pvcName = pvcName
		}
		if additionalPVCs[i].StorageSizeInGb > 0 {
			spec.storageSizeInGb = additionalPVCs[i].StorageSizeInGb
		}
		specByPath[mountPath] = spec
	}

	result := make([]pvcResizeSpec, 0, len(specByPath))
	result = append(result, specByPath[baseMountPath])
	delete(specByPath, baseMountPath)
	if cfg, ok := specByPath[diagMountPath]; ok {
		result = append(result, cfg)
		delete(specByPath, diagMountPath)
	}
	if cfg, ok := specByPath[databasev4.DefaultGddLogMountPath]; ok {
		result = append(result, cfg)
		delete(specByPath, databasev4.DefaultGddLogMountPath)
	}
	extraPaths := make([]string, 0, len(specByPath))
	for mountPath := range specByPath {
		extraPaths = append(extraPaths, mountPath)
	}
	sort.Strings(extraPaths)
	for _, mountPath := range extraPaths {
		result = append(result, specByPath[mountPath])
	}

	return result
}

func (r *ShardingDatabaseReconciler) reconcilePVCExpansion(instance *databasev4.ShardingDatabase, statefulSetName string, specs []pvcResizeSpec) error {
	ctx := context.Background()
	replicas := int32(1)
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: instance.Namespace}, sts); err == nil {
		if sts.Spec.Replicas != nil && *sts.Spec.Replicas > 0 {
			replicas = *sts.Spec.Replicas
		}
	} else if !errors.IsNotFound(err) {
		return err
	}

	scExpansionAllowed := map[string]bool{}
	for _, spec := range specs {
		if strings.TrimSpace(spec.pvcName) != "" || spec.storageSizeInGb <= 0 {
			continue
		}
		desired := resource.MustParse(strconv.FormatInt(int64(spec.storageSizeInGb), 10) + "Gi")
		for ordinal := int32(0); ordinal < replicas; ordinal++ {
			pvcName := spec.volumeName + "-" + statefulSetName + "-" + strconv.FormatInt(int64(ordinal), 10)

			pvc := &corev1.PersistentVolumeClaim{}
			if err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, pvc); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return err
			}

			current, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			if !ok {
				return fmt.Errorf("pvc %s has no storage request to compare for expansion", pvcName)
			}
			if current.Cmp(desired) > 0 {
				return fmt.Errorf("shrink is not supported for pvc %s: current=%s desired=%s", pvcName, current.String(), desired.String())
			}
			if current.Cmp(desired) == 0 {
				continue
			}

			storageClassName := ""
			if pvc.Spec.StorageClassName != nil {
				storageClassName = strings.TrimSpace(*pvc.Spec.StorageClassName)
			}
			if storageClassName == "" {
				return fmt.Errorf("cannot expand pvc %s because storageClassName is empty", pvcName)
			}
			allowed, cached := scExpansionAllowed[storageClassName]
			if !cached {
				sc := &storagev1.StorageClass{}
				if err := r.Get(ctx, types.NamespacedName{Name: storageClassName}, sc); err != nil {
					return fmt.Errorf("failed to read StorageClass %s for pvc %s: %w", storageClassName, pvcName, err)
				}
				allowed = sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
				scExpansionAllowed[storageClassName] = allowed
			}
			if !allowed {
				return fmt.Errorf("cannot expand pvc %s because StorageClass %s does not allow volume expansion", pvcName, storageClassName)
			}

			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				latest := &corev1.PersistentVolumeClaim{}
				if gerr := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, latest); gerr != nil {
					return gerr
				}
				if latest.Spec.Resources.Requests == nil {
					latest.Spec.Resources.Requests = corev1.ResourceList{}
				}
				cur := latest.Spec.Resources.Requests[corev1.ResourceStorage]
				if cur.Cmp(desired) > 0 {
					return fmt.Errorf("shrink is not supported for pvc %s: current=%s desired=%s", pvcName, cur.String(), desired.String())
				}
				if cur.Cmp(desired) == 0 {
					return nil
				}
				latest.Spec.Resources.Requests[corev1.ResourceStorage] = desired
				return r.Update(ctx, latest)
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Phase # 6: Validate Core Ready
// phaseValidateCoreReady waits until catalog and GSM are operational.
func (r *ShardingDatabaseReconciler) phaseValidateCoreReady(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = ctx
	_ = st
	_ = c
	plog := r.phaseLogger(inst, "validate_core")

	// Existing reconcile behavior: if GSM/Catalog are not ready, keep waiting/requeueing.
	if err := r.validateGsmnCatalog(inst); err != nil {
		plog.Info("gsm/catalog validation not ready; requeue", "reason", "WaitingForCatalogGsm", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "WaitingForCatalogGsm",
			message:      err.Error(),
		}
	}

	return phaseResult{}
}

// Phase 7: Primary Shard Ops
// phasePrimaryShardOps reconciles primary-shard registration and readiness in GSM.
func (r *ShardingDatabaseReconciler) phasePrimaryShardOps(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = ctx
	_ = st
	_ = c
	plog := r.phaseLogger(inst, "primary_shard_ops")

	// Existing behavior: check shard state first; if transitional/error states exist, requeue.
	if err := r.checkShardState(inst); err != nil {
		plog.Info("shard state indicates pending work; requeue", "reason", "ShardStatePending", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "ShardStatePending",
			message:      err.Error(),
		}
	}

	// Existing behavior: primary shard add/registration is long-running and retried via requeue.
	if err := r.addPrimaryShards(ctx, inst); err != nil {
		plog.Info("primary shard flow not complete yet; requeue", "reason", "PrimaryShardProgress", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "PrimaryShardProgress",
			message:      err.Error(),
		}
	}

	return phaseResult{}
}

// Phase 8: Standby + DG Ops
// phaseStandbyShardOps reconciles standby shards, including DG broker orchestration when enabled.
func (r *ShardingDatabaseReconciler) phaseStandbyShardOps(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = ctx
	_ = st
	_ = c
	plog := r.phaseLogger(inst, "standby_shard_ops")

	// Existing behavior: standby shard + DG broker enablement is retried until complete.
	if err := r.addStandbyShards(ctx, inst); err != nil {
		plog.Info("standby shard flow not complete yet; requeue", "reason", "StandbyShardProgress", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "StandbyShardProgress",
			message:      err.Error(),
		}
	}

	if pr, handled := r.phaseManualTDERefresh(ctx, inst, st); handled {
		return pr
	}

	return phaseResult{}
}

// phase 9 # Scale Ops
// phaseScaleOps reconciles shard deletion/scale-in operations.
func (r *ShardingDatabaseReconciler) phaseScaleOps(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = ctx
	_ = st
	_ = c
	plog := r.phaseLogger(inst, "scale_ops")

	if err := r.pruneImportedTDEShardsAnnotation(ctx, inst); err != nil {
		plog.Info("failed to prune tde import annotation; requeue", "reason", "TDEImportAnnotationPruneRetry", "error", err.Error(), "requeueAfter", 10*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 10 * time.Second,
			reason:       "TDEImportAnnotationPruneRetry",
			message:      err.Error(),
		}
	}

	// Existing behavior: shard delete/scale-in is asynchronous; keep requeueing while in progress.
	if err := r.delGsmShard(inst); err != nil {
		plog.Info("scale-in/delete flow not complete yet; requeue", "reason", "ScaleInProgress", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "ScaleInProgress",
			message:      err.Error(),
		}
	}

	return phaseResult{}
}

// phase 10 # Final consistency snapshot before marking Ready.
// phasePostSync refreshes aggregate topology status and persists last successful spec snapshot.
func (r *ShardingDatabaseReconciler) phasePostSync(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	_ = st
	_ = c
	plog := r.phaseLogger(inst, "post_sync")

	// Existing behavior: always refresh aggregate shard topology status near end of reconcile.
	defer r.updateShardTopologyStatus(inst)

	// Existing behavior: persist last successful spec snapshot; requeue on transient failure.
	if err := inst.UpdateLastSuccessfulSpec(r.Client); err != nil {
		plog.Error(err, "failed to update lastSuccessfulSpec", "reason", "LastSuccessfulSpecUpdateRetry")
		return phaseResult{
			wait:         true,
			requeueAfter: 10 * time.Second,
			reason:       "LastSuccessfulSpecUpdateRetry",
			message:      err.Error(),
		}
	}

	plog.Info("completed sharding topology setup reconciliation loop")
	_ = ctx
	return phaseResult{}
}

// SetupWithManager sets up the controller with the Manager.
// The default concurrent reconcilation loop is 1
// Check https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controller#Options to under MaxConcurrentReconciles
// SetupWithManager handles setup with manager for the sharding database controller.
func (r *ShardingDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	cfg := mgr.GetConfig()
	if cfg == nil {
		return fmt.Errorf("manager config is nil")
	}

	if r.kubeConfig == nil {
		r.kubeConfig = rest.CopyConfig(cfg)
	}
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("ShardingDatabase")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev4.ShardingDatabase{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Secret{}).
		WithEventFilter(r.eventFilterPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 50}). //MaxConcurrentReconciles is the maximum number of concurrent Reconciles which can be run. Defaults to 1
		Complete(r)
}

// phaseLogger returns a logger pre-populated with phase and object identity fields.
func (r *ShardingDatabaseReconciler) phaseLogger(inst *databasev4.ShardingDatabase, phase string) logr.Logger {
	log := r.Log.WithValues("phase", phase)
	if inst != nil {
		log = log.WithValues("namespace", inst.Namespace, "name", inst.Name)
	}
	return log
}

// logLegacy preserves old call signatures while emitting structured logs.
// logLegacy preserves legacy log-call signatures while emitting structured logs.
func (r *ShardingDatabaseReconciler) logLegacy(msgType, msg string, err error, inst *databasev4.ShardingDatabase, logger logr.Logger) {
	level := strings.ToUpper(strings.TrimSpace(msgType))
	log := logger.WithValues("component", "sharding-controller", "level", level)
	if inst != nil {
		log = log.WithValues("namespace", inst.Namespace, "name", inst.Name)
	}

	switch level {
	case "DEBUG":
		if inst != nil && inst.Spec.IsDebug {
			if err != nil {
				log.Error(err, msg)
			} else {
				log.Info(msg)
			}
		}
	case "ERROR", "ERR", "FATAL", "WARN", "WARNING":
		log.Error(err, msg)
	default:
		if err != nil {
			log.Info(msg, "error", err.Error())
		} else {
			log.Info(msg)
		}
	}
}

func isTransientDbSetupNotReady(err error) bool {
	if err == nil {
		return false
	}

	lerr := strings.ToLower(err.Error())
	return strings.Contains(lerr, "ora-01034") ||
		strings.Contains(lerr, "ora-27101") ||
		strings.Contains(lerr, "sp2-0640") ||
		strings.Contains(lerr, "not open") ||
		strings.Contains(lerr, "connection refused") ||
		strings.Contains(lerr, "i/o timeout") ||
		strings.Contains(lerr, "context deadline exceeded") ||
		strings.Contains(lerr, "exit code 127") ||
		strings.Contains(lerr, "command terminated with exit code 127")
}

// Function for markDegraded
// markDegraded sets conditions for a failed reconcile cycle.
func (r *ShardingDatabaseReconciler) markDegraded(c *conditionSet, reason, message string) {
	now := metav1.Now()

	c.degraded.Status = metav1.ConditionTrue
	c.degraded.Reason = reason
	c.degraded.Message = message
	c.degraded.ObservedGeneration = c.observedGen
	c.degraded.LastTransitionTime = now

	c.ready.Status = metav1.ConditionFalse
	c.ready.Reason = "Degraded"
	c.ready.Message = message
	c.ready.ObservedGeneration = c.observedGen
	c.ready.LastTransitionTime = now

	c.reconciling.Status = metav1.ConditionFalse
	c.reconciling.Reason = "ReconcileFailed"
	c.reconciling.Message = message
	c.reconciling.ObservedGeneration = c.observedGen
	c.reconciling.LastTransitionTime = now
}

// Functin for markReconiling
// markReconciling sets conditions for an in-progress reconcile cycle.
func (r *ShardingDatabaseReconciler) markReconciling(c *conditionSet, reason, message string) {
	now := metav1.Now()

	c.reconciling.Status = metav1.ConditionTrue
	c.reconciling.Reason = reason
	c.reconciling.Message = message
	c.reconciling.ObservedGeneration = c.observedGen
	c.reconciling.LastTransitionTime = now

	c.ready.Status = metav1.ConditionFalse
	c.ready.Reason = "Reconciling"
	c.ready.Message = message
	c.ready.ObservedGeneration = c.observedGen
	c.ready.LastTransitionTime = now

	c.degraded.Status = metav1.ConditionFalse
	c.degraded.Reason = "NoError"
	c.degraded.Message = ""
	c.degraded.ObservedGeneration = c.observedGen
	c.degraded.LastTransitionTime = now
}

// markReady sets conditions for a successfully converged reconcile cycle.
func (r *ShardingDatabaseReconciler) markReady(c *conditionSet) {
	now := metav1.Now()

	c.ready.Status = metav1.ConditionTrue
	c.ready.Reason = "ReconcileComplete"
	c.ready.Message = "Last reconcile cycle completed successfully"
	c.ready.ObservedGeneration = c.observedGen
	c.ready.LastTransitionTime = now

	c.reconciling.Status = metav1.ConditionFalse
	c.reconciling.Reason = "LastReconcileCycleCompleted"
	c.reconciling.Message = "No pending reconcile work"
	c.reconciling.ObservedGeneration = c.observedGen
	c.reconciling.LastTransitionTime = now

	c.degraded.Status = metav1.ConditionFalse
	c.degraded.Reason = "NoError"
	c.degraded.Message = ""
	c.degraded.ObservedGeneration = c.observedGen
	c.degraded.LastTransitionTime = now
}

// ###################### Event Filter Predicate ######################
// eventFilterPredicate controls which watched events enqueue reconcile requests.
func (r *ShardingDatabaseReconciler) eventFilterPredicate() predicate.Predicate {
	return predicate.Funcs{

		CreateFunc: func(_ event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			// //instance := &databasev4.ShardingDatabase{}
			// _, podOk := e.Object.GetLabels()["statefulset.kubernetes.io/pod-name"]
			// instance, _ := e.Object.DeepCopyObject().(*databasev4.ShardingDatabase)
			// if e.Object.GetDeletionTimestamp() == nil {
			// 	if e.Object.GetLabels()[string(databasev4.ShardingDelLabelKey)] == string(databasev4.ShardingDelLabelTrueValue) {
			// 	}
			// 	if podOk {
			// 		delObj := e.Object.(*corev1.Pod)
			// 		if e.Object.GetLabels()["type"] == "Shard" && e.Object.GetLabels()["app"] == "OracleSharding" && e.Object.GetLabels()["oralabel"] == instance.Name {
			// 			if delObj.DeletionTimestamp != nil {
			// 				go r.gsmInvitedNodeOp(instance, delObj.Name)
			// 			}
			// 		}

			// 		if e.Object.GetLabels()["type"] == "Catalog" && e.Object.GetLabels()["app"] == "OracleSharding" && e.Object.GetLabels()["oralabel"] == instance.Name {

			// 			if delObj.DeletionTimestamp != nil {
			// 				go r.gsmInvitedNodeOp(instance, delObj.Name)
			// 			}
			// 		}
			// 	}

			// }
			return true
		},
	}
}

func (r *ShardingDatabaseReconciler) isTDEKeyExported(instance *databasev4.ShardingDatabase) bool {
	if instance == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(instance.GetAnnotations()[tdeKeyExportedAnnotation]), "true")
}

func parseImportedTDEShards(raw string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" {
			continue
		}
		out[v] = true
	}
	return out
}

func serializeImportedTDEShards(shards map[string]bool) string {
	if len(shards) == 0 {
		return ""
	}
	items := make([]string, 0, len(shards))
	for name, ok := range shards {
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		items = append(items, strings.ToLower(strings.TrimSpace(name)))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func (r *ShardingDatabaseReconciler) isTDEKeyImportedForShard(instance *databasev4.ShardingDatabase, shardName string) bool {
	if instance == nil {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(shardName))
	if target == "" {
		return false
	}
	imported := parseImportedTDEShards(instance.GetAnnotations()[tdeKeyImportedShardsAnnotation])
	return imported[target]
}

func (r *ShardingDatabaseReconciler) markTDEKeyExported(ctx context.Context, key types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		anns[tdeKeyExportedAnnotation] = "true"
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}

func (r *ShardingDatabaseReconciler) markTDEKeyImportedForShard(ctx context.Context, key types.NamespacedName, shardName string) error {
	target := strings.ToLower(strings.TrimSpace(shardName))
	if target == "" {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		imported := parseImportedTDEShards(anns[tdeKeyImportedShardsAnnotation])
		imported[target] = true
		anns[tdeKeyImportedShardsAnnotation] = serializeImportedTDEShards(imported)
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}

func (r *ShardingDatabaseReconciler) ensureTDEKeysExported(ctx context.Context, instance *databasev4.ShardingDatabase) error {
	if instance == nil || !shardingv1.CheckIsTDEWalletFlag(instance, r.Log) {
		return nil
	}
	if r.isTDEKeyExported(instance) {
		return nil
	}
	if len(instance.Spec.Catalog) == 0 || strings.TrimSpace(instance.Spec.Catalog[0].Name) == "" {
		return fmt.Errorf("tde export requires at least one catalog with a valid name")
	}

	exportTDEFile := "expTDEFile"
	podName := strings.TrimSpace(instance.Spec.Catalog[0].Name) + "-0"
	if err := exportTDEKeyFn(podName, exportTDEFile, instance, r.kubeConfig, r.Log); err != nil {
		return fmt.Errorf("tde export failed for catalog pod %s: %w", podName, err)
	}

	key := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	if err := r.markTDEKeyExported(ctx, key); err != nil {
		return fmt.Errorf("tde export marker update failed: %w", err)
	}
	return nil
}

func (r *ShardingDatabaseReconciler) ensureTDEKeysImportedForShard(ctx context.Context, instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) error {
	if instance == nil || !shardingv1.CheckIsTDEWalletFlag(instance, r.Log) {
		return nil
	}
	if shardingv1.CheckIsDeleteFlag(shard.IsDelete, instance, r.Log) {
		return nil
	}
	deployAs := strings.ToUpper(strings.TrimSpace(shard.DeployAs))
	if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
		return nil
	}
	shardName := strings.TrimSpace(shard.Name)
	if shardName == "" {
		return nil
	}
	if !r.isTDEKeyExported(instance) {
		if err := r.ensureTDEKeysExported(ctx, instance); err != nil {
			return err
		}
	}
	if r.isTDEKeyImportedForShard(instance, shardName) {
		return nil
	}

	importTDEFile := "impTDEFile"
	if err := importTDEKeyFn(shardName+"-0", importTDEFile, instance, r.kubeConfig, r.Log); err != nil {
		return fmt.Errorf("tde import failed for shard pod %s-0: %w", shardName, err)
	}

	key := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	if err := r.markTDEKeyImportedForShard(ctx, key, shardName); err != nil {
		return fmt.Errorf("tde import marker update failed for shard %s: %w", shardName, err)
	}
	return nil
}

func (r *ShardingDatabaseReconciler) pruneImportedTDEShardsAnnotation(ctx context.Context, instance *databasev4.ShardingDatabase) error {
	if instance == nil {
		return nil
	}
	key := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if len(anns) == 0 {
			return nil
		}
		raw := anns[tdeKeyImportedShardsAnnotation]
		if strings.TrimSpace(raw) == "" {
			return nil
		}

		imported := parseImportedTDEShards(raw)
		if len(imported) == 0 {
			return nil
		}
		active := map[string]bool{}
		for i := range curr.Spec.Shard {
			sh := curr.Spec.Shard[i]
			name := strings.ToLower(strings.TrimSpace(sh.Name))
			if name == "" || shardingv1.CheckIsDeleteFlag(sh.IsDelete, curr, r.Log) {
				continue
			}
			active[name] = true
		}

		changed := false
		for name := range imported {
			if !active[name] {
				delete(imported, name)
				changed = true
			}
		}
		if !changed {
			return nil
		}

		serialized := serializeImportedTDEShards(imported)
		if serialized == "" {
			delete(anns, tdeKeyImportedShardsAnnotation)
		} else {
			anns[tdeKeyImportedShardsAnnotation] = serialized
		}
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}

func containsStringFold(list []string, target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	if t == "" {
		return false
	}
	for i := range list {
		if strings.ToLower(strings.TrimSpace(list[i])) == t {
			return true
		}
	}
	return false
}

func appendUniqueStringFold(list []string, value string) []string {
	if containsStringFold(list, value) {
		return list
	}
	return append(list, strings.TrimSpace(value))
}

func isStandbyDeployAs(deployAs string) bool {
	v := strings.ToUpper(strings.TrimSpace(deployAs))
	return v == "STANDBY" || v == "ACTIVE_STANDBY"
}

func orderedManualTDERefreshTargets(instance *databasev4.ShardingDatabase) []string {
	primaries := make([]string, 0)
	standbys := make([]string, 0)
	for i := range instance.Spec.Shard {
		sh := instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(sh.IsDelete, instance, logr.Discard()) {
			continue
		}
		name := strings.TrimSpace(sh.Name)
		if name == "" {
			continue
		}
		if isStandbyDeployAs(sh.DeployAs) {
			standbys = append(standbys, name)
		} else {
			primaries = append(primaries, name)
		}
	}
	sort.Strings(primaries)
	sort.Strings(standbys)
	return append(primaries, standbys...)
}

func (r *ShardingDatabaseReconciler) clearManualTDERefreshAnnotation(ctx context.Context, key types.NamespacedName) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if len(anns) == 0 {
			return nil
		}
		if _, ok := anns[tdeKeyRefreshAnnotation]; !ok {
			return nil
		}
		delete(anns, tdeKeyRefreshAnnotation)
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}

func (r *ShardingDatabaseReconciler) phaseManualTDERefresh(
	ctx context.Context,
	inst *databasev4.ShardingDatabase,
	st *databasev4.ShardingDatabaseStatus,
) (phaseResult, bool) {
	token := strings.TrimSpace(inst.GetAnnotations()[tdeKeyRefreshAnnotation])
	if token == "" {
		return phaseResult{}, false
	}
	if st == nil {
		return phaseResult{wait: true, requeueAfter: 15 * time.Second, reason: "TDEKeyRefreshStatusUnavailable", message: "status draft is nil"}, true
	}
	if st.TDEKeyRefresh == nil || strings.TrimSpace(st.TDEKeyRefresh.RequestedToken) != token {
		st.TDEKeyRefresh = &databasev4.TDEKeyRefreshStatus{
			RequestedToken:     token,
			Phase:              "Pending",
			CompletedShards:    []string{},
			FailedShards:       []string{},
			UserActionRequired: false,
		}
	}

	rs := st.TDEKeyRefresh
	if strings.EqualFold(rs.Phase, "Failed") && rs.UserActionRequired && strings.TrimSpace(rs.RequestedToken) == token {
		return phaseResult{
			wait:         true,
			requeueAfter: 60 * time.Second,
			reason:       "TDEKeyRefreshUserActionRequired",
			message:      "TDE key refresh is blocked for token " + token + "; fix issue and set a new token to retry",
		}, true
	}
	rs.Phase = "Running"
	rs.RequestedToken = token

	if !rs.Exported {
		if err := r.ensureTDEKeysExported(ctx, inst); err != nil {
			rs.Phase = "Failed"
			rs.UserActionRequired = true
			rs.LastError = err.Error()
			return phaseResult{
				wait:         true,
				requeueAfter: 60 * time.Second,
				reason:       "TDEKeyRefreshUserActionRequired",
				message:      "TDE key refresh export failed; user action required: " + err.Error(),
			}, true
		}
		rs.Exported = true
	}

	targets := orderedManualTDERefreshTargets(inst)
	if len(targets) == 0 {
		rs.Phase = "Succeeded"
		rs.UserActionRequired = false
		rs.CurrentShard = ""
		rs.LastError = ""
		if err := r.clearManualTDERefreshAnnotation(ctx, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}); err != nil {
			return phaseResult{
				wait:         true,
				requeueAfter: 10 * time.Second,
				reason:       "TDEKeyRefreshAnnotationClearRetry",
				message:      err.Error(),
			}, true
		}
		return phaseResult{
			wait:         true,
			requeueAfter: 2 * time.Second,
			reason:       "TDEKeyRefreshCompleted",
			message:      "TDE key refresh completed; annotation cleared",
		}, true
	}

	nextShard := ""
	for i := range targets {
		if !containsStringFold(rs.CompletedShards, targets[i]) {
			nextShard = targets[i]
			break
		}
	}
	if nextShard == "" {
		rs.Phase = "Succeeded"
		rs.UserActionRequired = false
		rs.CurrentShard = ""
		rs.LastError = ""
		rs.FailedShards = nil
		if err := r.clearManualTDERefreshAnnotation(ctx, types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}); err != nil {
			return phaseResult{
				wait:         true,
				requeueAfter: 10 * time.Second,
				reason:       "TDEKeyRefreshAnnotationClearRetry",
				message:      err.Error(),
			}, true
		}
		return phaseResult{
			wait:         true,
			requeueAfter: 2 * time.Second,
			reason:       "TDEKeyRefreshCompleted",
			message:      "TDE key refresh completed; annotation cleared",
		}, true
	}

	rs.CurrentShard = nextShard
	if err := importTDEKeyFn(nextShard+"-0", "impTDEFile", inst, r.kubeConfig, r.Log); err != nil {
		rs.Phase = "Failed"
		rs.UserActionRequired = true
		rs.LastError = err.Error()
		rs.FailedShards = appendUniqueStringFold(rs.FailedShards, nextShard)
		return phaseResult{
			wait:         true,
			requeueAfter: 60 * time.Second,
			reason:       "TDEKeyRefreshUserActionRequired",
			message:      "TDE key refresh failed for shard " + nextShard + "; user action required: " + err.Error(),
		}, true
	}
	rs.CompletedShards = appendUniqueStringFold(rs.CompletedShards, nextShard)
	rs.LastError = ""
	rs.UserActionRequired = false
	return phaseResult{
		wait:         true,
		requeueAfter: 2 * time.Second,
		reason:       "TDEKeyRefreshProgress",
		message:      "TDE key refresh progressed on shard " + nextShard,
	}, true
}

// ================ Function to check secret update=============

// UpdateSecret probes the configured DB secret; retained for legacy compatibility.
func (r *ShardingDatabaseReconciler) UpdateSecret(instance *databasev4.ShardingDatabase, kClient client.Client, _ logr.Logger) (ctrl.Result, error) {

	sc := &corev1.Secret{}
	//var err error

	// Reading a Secret
	var err error = kClient.Get(context.TODO(), types.NamespacedName{
		Name:      instance.Spec.DbSecret.Name,
		Namespace: instance.Namespace,
	}, sc)

	if err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// publishEvents emits a normal or warning Kubernetes event based on lifecycle state.
func (r *ShardingDatabaseReconciler) publishEvents(instance *databasev4.ShardingDatabase, eventMsg string, state string) {

	if state == string(databasev4.AvailableState) || state == string(databasev4.AddingShardState) || state == string(databasev4.ShardOnlineState) || state == string(databasev4.ProvisionState) || state == string(databasev4.DeletingState) || state == string(databasev4.Terminated) {
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "State Change", eventMsg)
	} else {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "State Change", eventMsg)

	}

}

// finalizeShardingDatabase performs best-effort idempotent cleanup of owned resources before finalizer removal.
func (r *ShardingDatabaseReconciler) finalizeShardingDatabase(instance *databasev4.ShardingDatabase) error {
	var cleanupErrs []error

	appendCleanupErr := func(err error, format string, args ...interface{}) {
		if err == nil {
			return
		}
		cleanupErrs = append(cleanupErrs, fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err))
	}

	deleteIfFoundSts := func(name string) {
		sfSetFound, err := shardingv1.CheckSfset(name, instance, r.Client)
		if err != nil {
			if !errors.IsNotFound(err) {
				appendCleanupErr(err, "check statefulset %s", name)
			}
			return
		}
		if err := r.Client.Delete(context.Background(), sfSetFound); err != nil && !errors.IsNotFound(err) {
			appendCleanupErr(err, "delete statefulset %s", name)
		}
	}

	deleteIfFoundSvc := func(name string) {
		svcFound, err := shardingv1.CheckSvc(name, instance, r.Client)
		if err != nil {
			if !errors.IsNotFound(err) {
				appendCleanupErr(err, "check service %s", name)
			}
			return
		}
		if err := r.Client.Delete(context.Background(), svcFound); err != nil && !errors.IsNotFound(err) {
			appendCleanupErr(err, "delete service %s", name)
		}
	}

	deletePvcIfNeeded := func(ownerName string) {
		if !instance.Spec.IsDeleteOraPvc || len(instance.Spec.StorageClass) == 0 {
			return
		}
		pvcName := ownerName + "-oradata-vol4-" + ownerName + "-0"
		if err := shardingv1.DelPvc(pvcName, instance, r.Client, r.Log); err != nil && !errors.IsNotFound(err) {
			appendCleanupErr(err, "delete pvc %s", pvcName)
		}
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		spec := instance.Spec.Shard[i]
		deleteIfFoundSts(spec.Name)
		deletePvcIfNeeded(spec.Name)
	}

	for i := int32(0); i < int32(len(instance.Spec.Gsm)); i++ {
		spec := instance.Spec.Gsm[i]
		deleteIfFoundSts(spec.Name)
		deletePvcIfNeeded(spec.Name)
	}

	for i := int32(0); i < int32(len(instance.Spec.Catalog)); i++ {
		spec := instance.Spec.Catalog[i]
		deleteIfFoundSts(spec.Name)
		deletePvcIfNeeded(spec.Name)
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		spec := instance.Spec.Shard[i]
		if instance.Spec.IsExternalSvc {
			deleteIfFoundSvc(spec.Name + strconv.FormatInt(0, 10) + "-svc")
		}
		deleteIfFoundSvc(spec.Name)
	}

	for i := int32(0); i < int32(len(instance.Spec.Catalog)); i++ {
		spec := instance.Spec.Catalog[i]
		if instance.Spec.IsExternalSvc {
			deleteIfFoundSvc(spec.Name + strconv.FormatInt(0, 10) + "-svc")
		}
		deleteIfFoundSvc(spec.Name)
	}

	for i := int32(0); i < int32(len(instance.Spec.Gsm)); i++ {
		spec := instance.Spec.Gsm[i]
		deleteIfFoundSvc(spec.Name)
		if instance.Spec.IsExternalSvc {
			deleteIfFoundSvc(spec.Name + strconv.FormatInt(0, 10) + "-svc")
		}
	}

	// Delete leftover StatefulSets for this instance label set.
	sfList := &appsv1.StatefulSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(shardingv1.LabelsForProvShardKind(instance, "shard")),
	}
	if err := r.Client.List(context.TODO(), sfList, listOpts...); err != nil {
		appendCleanupErr(err, "list leftover statefulsets")
	} else {
		for _, sset := range sfList.Items {
			deleteIfFoundSts(sset.Name)
		}
	}

	if len(cleanupErrs) > 0 {
		return utilerrors.NewAggregate(cleanupErrs)
	}

	return nil
}

// =========== validate Specs ============
// validateSpex validates requested topology changes against supported mutation rules.
func (r *ShardingDatabaseReconciler) validateSpex(instance *databasev4.ShardingDatabase) error {

	var eventMsg string
	var eventErr string = "Spec Error"
	var i int32

	lastSuccSpec, err := instance.GetLastSuccessfulSpec()
	if err != nil {
		return nil
	}

	if err := r.validatePrimaryTopologyConstraint(instance); err != nil {
		return err
	}

	// Check if last Successful update nil or not
	if lastSuccSpec == nil {
		// Logic to check if inital Spec is good or not

		err = r.checkShardingType(instance)
		if err != nil {
			return err
		}

		if len(instance.Spec.Shard) > 0 {
			for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
				OraShardSpex := instance.Spec.Shard[i]
				if !shardingv1.CheckIsDeleteFlag(OraShardSpex.IsDelete, instance, r.Log) {
					err = r.checkShardSpace(instance, OraShardSpex)
					if err != nil {
						return err
					}
					err = r.checkShardGroup(instance, OraShardSpex)
					if err != nil {
						return err
					}
				}
			}
		}

		// Once the initial Spec is been validated then update the last Sucessful Spec
		err = instance.UpdateLastSuccessfulSpec(r.Client)
		if err != nil {
			return err
		}
	} else {
		// if the last sucessful spec is not nil
		// check the parameters which cannot be changed

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
			return fmt.Errorf("change of Shard env variables are not")
		}
		// Compare Env variables for catalog begins here
		if !r.comapreCatalogEnvVariables(instance, lastSuccSpec) {
			return fmt.Errorf("change of Catalog env variables are not")
		}
		// Compare env variable for Catalog ends here
		if !r.comapreGsmEnvVariables(instance, lastSuccSpec) {
			return fmt.Errorf("change of GSM env variables are not")
		}

	}
	return nil
}

// checkShardingType validates the configured sharding type.
func (r *ShardingDatabaseReconciler) checkShardingType(instance *databasev4.ShardingDatabase) error {
	var i, k int32
	var regionFlag bool

	for k = 0; k < int32(len(instance.Spec.Gsm)); k++ {
		regionFlag = false
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			if instance.Spec.Gsm[k].Region == instance.Spec.Shard[i].ShardRegion {
				regionFlag = true
			}
		}
		if !regionFlag {
			msg := instance.Spec.Gsm[k].Region + " does not match with any region with Shard region. Region will be created during shard director provisioning"
			r.logLegacy("INFO", msg, nil, instance, r.Log)
		}
	}

	return nil
}

// Check the ShardGroups/ Shard Space and Shard group Name
// checkShrdGSR is Shardgroup/ShardSpace/ShardRegion

func resolveShardMode(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	typeHint := strings.ToUpper(strings.TrimSpace(instance.Spec.ShardingType))
	if typeHint == "SYSTEM" || typeHint == "USER" || typeHint == "COMPOSITE" {
		return typeHint
	}

	hasGroup := strings.TrimSpace(shard.ShardGroup) != ""
	hasSpace := strings.TrimSpace(shard.ShardSpace) != ""

	switch {
	case hasGroup && hasSpace:
		return "COMPOSITE"
	case hasGroup:
		return "SYSTEM"
	case hasSpace:
		return "USER"
	default:
		return "SYSTEM"
	}
}

// Check the ShardGroups/ Shard Space and Shard group Name
// checkShrdGSR is Shardgroup/ShardSpace/ShardRegion

// checkShardSpace validates shard-space requirements for the selected sharding mode.
func (r *ShardingDatabaseReconciler) checkShardSpace(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) error {
	mode := resolveShardMode(instance, OraShardSpex)
	if mode != "USER" && mode != "COMPOSITE" {
		return nil
	}

	if len(OraShardSpex.ShardRegion) == 0 {
		return fmt.Errorf("Shard region cannot be empty in %s", OraShardSpex.Name)
	}
	if len(OraShardSpex.ShardSpace) == 0 {
		return fmt.Errorf("Shard space in %s cannot be empty", OraShardSpex.Name)
	}
	return nil
}

// Check the ShardGroups/ Shard Space and Shard group Name
// checkShrdGSR is Shardgroup/ShardSpace/ShardRegion

// checkShardGroup validates shard-group and region constraints.
func (r *ShardingDatabaseReconciler) checkShardGroup(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec) error {
	mode := resolveShardMode(instance, OraShardSpex)
	if mode != "SYSTEM" && mode != "COMPOSITE" {
		return nil
	}

	if len(OraShardSpex.ShardRegion) == 0 {
		return fmt.Errorf("Shard region cannot be empty in %s", OraShardSpex.Name)
	}
	if len(OraShardSpex.ShardGroup) == 0 {
		return fmt.Errorf("Shard group in %s cannot be empty", OraShardSpex.Name)
	}
	return nil
}

// comapreGsmEnvVariables handles comapre gsm env variables for the sharding database controller.
func (r *ShardingDatabaseReconciler) comapreGsmEnvVariables(instance *databasev4.ShardingDatabase, lastSuccSpec *databasev4.ShardingDatabaseSpec) bool {
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

func (r *ShardingDatabaseReconciler) ensureStandbyShardNumFromConfig(instance *databasev4.ShardingDatabase) bool {
	if effectiveShardingTypeForConstraints(instance) != "USER" {
		return false
	}
	changed := false
	for i := range instance.Spec.ShardInfo {
		info := &instance.Spec.ShardInfo[i]
		derived := standbyConfigDerivedShardCountController(info.StandbyConfig)
		if derived <= 0 {
			continue
		}
		if info.ShardNum != derived {
			info.ShardNum = derived
			changed = true
		}
	}
	return changed
}

func (r *ShardingDatabaseReconciler) syncDgPairsFromStandbyConfig(instance *databasev4.ShardingDatabase) error {
	desired := r.buildDgPairsFromStandbyConfig(instance)
	if reflect.DeepEqual(instance.Status.Dg.Pairs, desired) {
		return nil
	}
	return r.updateStatusWithRetry(instance, func(latest *databasev4.ShardingDatabase) {
		latest.Status.Dg.Pairs = desired
	})
}

type primaryIdentity struct {
	Key     string
	Connect string
	Source  string
}

func (r *ShardingDatabaseReconciler) buildDgPairsFromStandbyConfig(instance *databasev4.ShardingDatabase) []databasev4.DgPairStatus {
	if effectiveShardingTypeForConstraints(instance) != "USER" {
		return nil
	}
	pairs := []databasev4.DgPairStatus{}

	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		prefix := strings.TrimSpace(info.ShardPreFixName)
		if prefix == "" || info.StandbyConfig == nil {
			continue
		}

		identities := buildPrimaryIdentities(instance, info.StandbyConfig)
		if len(identities) == 0 {
			continue
		}

		perPrimary := info.StandbyConfig.StandbyPerPrimary
		if perPrimary <= 0 {
			perPrimary = 1
		}

		shardIdx := 1
		for _, pid := range identities {
			for n := int32(0); n < perPrimary; n++ {
				shardName := prefix + strconv.Itoa(shardIdx)
				pairs = append(pairs, databasev4.DgPairStatus{
					PrimaryKey:           pid.Key,
					PrimarySource:        pid.Source,
					PrimaryConnectString: pid.Connect,
					StandbyShardName:     shardName,
					StandbyShardNum:      int32(shardIdx),
					State:                "MAPPED",
					Message:              "mapped from standbyConfig",
				})
				shardIdx++
			}
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].StandbyShardName == pairs[j].StandbyShardName {
			return pairs[i].PrimaryKey < pairs[j].PrimaryKey
		}
		return pairs[i].StandbyShardName < pairs[j].StandbyShardName
	})

	return pairs
}

func buildPrimaryIdentities(instance *databasev4.ShardingDatabase, cfg *databasev4.StandbyConfig) []primaryIdentity {
	if cfg == nil {
		return nil
	}

	ids := []primaryIdentity{}
	seen := map[string]bool{}
	appendID := func(pid primaryIdentity) {
		key := strings.ToLower(strings.TrimSpace(pid.Key))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ids = append(ids, pid)
	}

	appendRefs := func() {
		for i := range cfg.PrimaryDatabaseRefs {
			ref := cfg.PrimaryDatabaseRefs[i]
			name := strings.TrimSpace(ref.Name)
			if name == "" {
				continue
			}
			ns := strings.TrimSpace(ref.Namespace)
			if ns == "" {
				ns = instance.Namespace
			}
			appendID(primaryIdentity{
				Key:    ns + "/" + name,
				Source: "PrimaryDatabaseRef",
			})
		}
	}
	appendConnects := func() {
		for i := range cfg.PrimaryConnectStrings {
			c := strings.TrimSpace(cfg.PrimaryConnectStrings[i])
			if c == "" {
				continue
			}
			appendID(primaryIdentity{
				Key:     c,
				Connect: c,
				Source:  "ConnectString",
			})
		}
	}
	appendEndpoints := func() {
		for i := range cfg.PrimaryEndpoints {
			e := cfg.PrimaryEndpoints[i]
			connect := strings.TrimSpace(e.ConnectString)
			key := connect
			if key == "" {
				host := strings.TrimSpace(e.Host)
				cdb := strings.TrimSpace(e.CdbName)
				pdb := strings.TrimSpace(e.PdbName)
				if host == "" && cdb == "" && pdb == "" {
					continue
				}
				key = strings.ToLower(host) + ":" + strconv.Itoa(int(e.Port)) + "/" + strings.ToUpper(cdb) + "/" + strings.ToUpper(pdb)
			}
			appendID(primaryIdentity{
				Key:     key,
				Connect: connect,
				Source:  "Endpoint",
			})
		}
	}

	appendRefs()
	appendConnects()
	appendEndpoints()

	sort.Slice(ids, func(i, j int) bool { return ids[i].Key < ids[j].Key })
	return ids
}

// isShapeManagedEnv handles is shape managed env for the sharding database controller.
func isShapeManagedEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "init_sga_size",
		"init_pga_size",
		"init_process",
		"init_cpu_count",
		"init_total_size":
		return true
	default:
		return false
	}
}

// filterShapeManagedEnvVars handles filter shape managed env vars for the sharding database controller.
func filterShapeManagedEnvVars(envs []databasev4.EnvironmentVariable) []databasev4.EnvironmentVariable {
	out := make([]databasev4.EnvironmentVariable, 0, len(envs))
	for _, e := range envs {
		if isShapeManagedEnv(e.Name) {
			continue
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(out[i].Name)) <
			strings.ToLower(strings.TrimSpace(out[j].Name))
	})

	return out
}

// equalEnvVarsIgnoringShapeManaged handles equal env vars ignoring shape managed for the sharding database controller.
func equalEnvVarsIgnoringShapeManaged(a, b []databasev4.EnvironmentVariable) bool {
	return reflect.DeepEqual(
		filterShapeManagedEnvVars(a),
		filterShapeManagedEnvVars(b),
	)
}

// syncStatefulSetContainerShape handles sync stateful set container shape for the sharding database controller.
func syncStatefulSetContainerShape(found, desired *appsv1.StatefulSet) bool {
	changed := false

	desiredByName := map[string]corev1.Container{}
	for _, c := range desired.Spec.Template.Spec.Containers {
		desiredByName[c.Name] = c
	}

	for i := range found.Spec.Template.Spec.Containers {
		curr := &found.Spec.Template.Spec.Containers[i]
		want, ok := desiredByName[curr.Name]
		if !ok {
			continue
		}

		if !reflect.DeepEqual(curr.Env, want.Env) {
			curr.Env = want.Env
			changed = true
		}

		if !reflect.DeepEqual(curr.Resources, want.Resources) {
			curr.Resources = want.Resources
			changed = true
		}
	}

	return changed
}

// comapreCatalogEnvVariables handles comapre catalog env variables for the sharding database controller.
func (r *ShardingDatabaseReconciler) comapreCatalogEnvVariables(instance *databasev4.ShardingDatabase, lastSuccSpec *databasev4.ShardingDatabaseSpec) bool {
	var eventMsg string
	var eventErr string = "Spec Error"
	var i, j int32

	if len(instance.Spec.Catalog) > 0 {
		for i = 0; i < int32(len(instance.Spec.Catalog)); i++ {
			curr := instance.Spec.Catalog[i]
			for j = 0; j < int32(len(lastSuccSpec.Catalog)); j++ {
				old := lastSuccSpec.Catalog[j]
				if curr.Name != old.Name {
					continue
				}

				// Allow shape-driven env changes, but still block non-shape env changes
				if !strings.EqualFold(strings.TrimSpace(curr.Shape), strings.TrimSpace(old.Shape)) {
					if !reflect.DeepEqual(stripShapeManagedDbEnv(curr.EnvVars), stripShapeManagedDbEnv(old.EnvVars)) {
						eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " non-shape catalog env variable changes are not supported."
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
						return false
					}
					continue
				}

				if !reflect.DeepEqual(curr.EnvVars, old.EnvVars) {
					eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " env vairable changes are not supported."
					r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
					return false
				}
			}
		}
	}

	return true
}

// comapreShardEnvVariables handles comapre shard env variables for the sharding database controller.
func (r *ShardingDatabaseReconciler) comapreShardEnvVariables(instance *databasev4.ShardingDatabase, lastSuccSpec *databasev4.ShardingDatabaseSpec) bool {
	var eventMsg string
	var eventErr string = "Spec Error"
	var i, j int32

	if len(instance.Spec.Shard) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			curr := instance.Spec.Shard[i]
			for j = 0; j < int32(len(lastSuccSpec.Shard)); j++ {
				old := lastSuccSpec.Shard[j]
				if curr.Name != old.Name {
					continue
				}

				// Allow shape-driven env changes, but still block non-shape env changes
				if shardShapeChangedForName(&instance.Spec, lastSuccSpec, curr.Name) {
					if !reflect.DeepEqual(stripShapeManagedDbEnv(curr.EnvVars), stripShapeManagedDbEnv(old.EnvVars)) {
						eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " non-shape shard env variable changes are not supported."
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
						return false
					}
					continue
				}

				if !reflect.DeepEqual(curr.EnvVars, old.EnvVars) {
					eventMsg = "ShardingDatabase CRD resource " + shardingv1.GetFmtStr(instance.Name) + " env vairable changes are not supported."
					r.Recorder.Eventf(instance, corev1.EventTypeWarning, eventErr, eventMsg)
					return false
				}
			}
		}
	}

	return true
}

// validateGsmnCatalog validates both catalog and GSM availability gates.
func (r *ShardingDatabaseReconciler) validateGsmnCatalog(instance *databasev4.ShardingDatabase) error {
	if r.isExternalCatalogMode(instance) {
		if err := r.validateExternalCatalogRef(instance); err != nil {
			return fmt.Errorf("external catalog reference validation failed: %w", err)
		}
		r.logLegacy("INFO", "External catalog mode enabled; skipping local catalog pod validation", nil, instance, r.Log)
	} else {
		if _, _, err := r.validateCatalog(instance); err != nil {
			return fmt.Errorf("catalog validation failed: %w", err)
		}
	}
	if _, _, err := r.validateGsm(instance); err != nil {
		return fmt.Errorf("gsm validation failed: %w", err)
	}
	return nil
}

func (r *ShardingDatabaseReconciler) isExternalCatalogMode(instance *databasev4.ShardingDatabase) bool {
	if instance == nil || len(instance.Spec.Catalog) == 0 {
		return false
	}

	cat := instance.Spec.Catalog[0]
	if cat.UseExistingCatalog {
		return true
	}
	return cat.CatalogDatabaseRef != nil && strings.TrimSpace(cat.CatalogDatabaseRef.Host) != ""
}

func (r *ShardingDatabaseReconciler) validateExternalCatalogRef(instance *databasev4.ShardingDatabase) error {
	if instance == nil || len(instance.Spec.Catalog) == 0 {
		return fmt.Errorf("catalog spec is required when using external catalog mode")
	}

	cat := instance.Spec.Catalog[0]
	if cat.CatalogDatabaseRef == nil {
		return fmt.Errorf("catalogDatabaseRef is required when useExistingCatalog is enabled")
	}

	ref := cat.CatalogDatabaseRef
	if strings.TrimSpace(ref.Host) == "" {
		return fmt.Errorf("catalogDatabaseRef.host is required in external catalog mode")
	}
	if ref.Port <= 0 {
		return fmt.Errorf("catalogDatabaseRef.port must be > 0 in external catalog mode")
	}
	if strings.TrimSpace(ref.CdbName) == "" {
		return fmt.Errorf("catalogDatabaseRef.cdbName is required in external catalog mode")
	}
	if strings.TrimSpace(ref.PdbName) == "" {
		return fmt.Errorf("catalogDatabaseRef.pdbName is required in external catalog mode")
	}
	return nil
}

// validateGsm returns the first available GSM StatefulSet/pod pair.
func (r *ShardingDatabaseReconciler) validateGsm(instance *databasev4.ShardingDatabase,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}

	if len(instance.Spec.Gsm) == 0 {
		return gsmSfSet, gsmPod, fmt.Errorf("no GSM spec configured")
	}

	available := false
	var lastErr error
	for i := int32(0); i < int32(len(instance.Spec.Gsm)); i++ {
		sf, pod, err := r.validateInvidualGsm(instance, instance.Spec.Gsm[i], int(i))
		if err != nil {
			lastErr = err
			continue
		}
		if !available {
			gsmSfSet = sf
			gsmPod = pod
			available = true
		}
	}

	if available {
		return gsmSfSet, gsmPod, nil
	}
	if lastErr != nil {
		return gsmSfSet, gsmPod, fmt.Errorf("GSM is not ready: %w", lastErr)
	}
	return gsmSfSet, gsmPod, fmt.Errorf("GSM is not ready")
}

// validateInvidualGsm validates one GSM member and updates its status projection.
func (r *ShardingDatabaseReconciler) validateInvidualGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec, specID int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	var msg string
	var gsmSfSet *appsv1.StatefulSet
	gsmPod := &corev1.Pod{}

	var err error
	var podList *corev1.PodList
	var isPodExist bool

	gsmSfSet, err = shardingv1.CheckSfset(OraGsmSpex.Name, instance, r.Client)
	if err != nil {
		msg = "Unable to find  GSM statefulset " + shardingv1.GetFmtStr(OraGsmSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specID, string(databasev4.StatefulSetNotFound))
		return gsmSfSet, gsmPod, err
	}

	podList, err = shardingv1.GetPodList(gsmSfSet.Name, "GSM", instance, r.Client)
	if err != nil {
		msg = "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(gsmSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specID, string(databasev4.PodNotFound))
		return gsmSfSet, gsmPod, err
	}

	isPodExist, gsmPod = shardingv1.PodListValidation(podList, gsmSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg = "Unable to validate GSM " + shardingv1.GetFmtStr(OraGsmSpex.Name) + " pod. GSM pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specID, string(databasev4.PodNotReadyState))
		return gsmSfSet, gsmPod, fmt.Errorf("pod doesn't exist")
	}
	err = shardingv1.CheckGsmStatus(gsmPod.Name, instance, r.kubeConfig, r.Log)
	if err != nil {
		msg = "Unable to validate GSM director. GSM director doesn't seems to be ready to accept the commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specID, string(databasev4.ProvisionState))
		return gsmSfSet, gsmPod, err
	}

	r.updateGsmStatus(instance, specID, string(databasev4.AvailableState))
	return gsmSfSet, gsmPod, nil
}

// validateCatalog returns the first available catalog StatefulSet/pod pair.
func (r *ShardingDatabaseReconciler) validateCatalog(instance *databasev4.ShardingDatabase,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	catalogSfSet := &appsv1.StatefulSet{}
	catalogPod := &corev1.Pod{}
	if len(instance.Spec.Catalog) == 0 {
		return catalogSfSet, catalogPod, fmt.Errorf("no Catalog spec configured")
	}

	available := false
	var lastErr error
	for i := int32(0); i < int32(len(instance.Spec.Catalog)); i++ {
		sf, pod, err := r.validateInvidualCatalog(instance, instance.Spec.Catalog[i], int(i))
		if err != nil {
			lastErr = err
			continue
		}
		if !available {
			catalogSfSet = sf
			catalogPod = pod
			available = true
		}
	}

	if available {
		return catalogSfSet, catalogPod, nil
	}
	if lastErr != nil {
		return catalogSfSet, catalogPod, fmt.Errorf("Catalog is not available: %w", lastErr)
	}
	return catalogSfSet, catalogPod, fmt.Errorf("Catalog is not available")
}

// === Validate Individual Catalog
// validateInvidualCatalog validates one catalog member and updates its status projection.
func (r *ShardingDatabaseReconciler) validateInvidualCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec, specID int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	var catalogSfSet *appsv1.StatefulSet
	catalogPod := &corev1.Pod{}
	var podList *corev1.PodList
	var isPodExist bool

	catalogSfSet, err = shardingv1.CheckSfset(OraCatalogSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Catalog statefulset " + shardingv1.GetFmtStr(OraCatalogSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specID, string(databasev4.StatefulSetNotFound))
		return catalogSfSet, catalogPod, err
	}

	podList, err = shardingv1.GetPodList(catalogSfSet.Name, "CATALOG", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(catalogSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specID, string(databasev4.PodNotFound))
		return catalogSfSet, catalogPod, err
	}
	isPodExist, catalogPod = shardingv1.PodListValidation(podList, catalogSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Catalog " + shardingv1.GetFmtStr(catalogSfSet.Name) + " pod. Catalog pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specID, string(databasev4.PodNotReadyState))
		return catalogSfSet, catalogPod, fmt.Errorf("pod doesn't exist")
	}
	err = shardingv1.ValidateDbSetup(catalogPod.Name, instance, r.kubeConfig, r.Log)
	if err != nil {
		msg := "Unable to validate Catalog. Catalog doesn't seems to be ready to accept the commands."
		if isTransientDbSetupNotReady(err) {
			r.logLegacy("INFO", msg+" cause: "+err.Error(), nil, instance, r.Log)
		} else {
			r.logLegacy("Error", msg+" cause: "+err.Error(), nil, instance, r.Log)
		}
		r.updateCatalogStatus(instance, specID, string(databasev4.ProvisionState))
		return catalogSfSet, catalogPod, err
	}

	r.updateCatalogStatus(instance, specID, string(databasev4.AvailableState))
	return catalogSfSet, catalogPod, nil

}

// ======= Function to validate Shard
// validateShard validates one shard member and updates its status projection.
func (r *ShardingDatabaseReconciler) validateShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec, specID int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	var shardSfSet *appsv1.StatefulSet
	shardPod := &corev1.Pod{}

	shardSfSet, err := shardingv1.CheckSfset(OraShardSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Shard statefulset " + shardingv1.GetFmtStr(OraShardSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specID, string(databasev4.StatefulSetNotFound))
		return shardSfSet, shardPod, err
	}

	podList, err := shardingv1.GetPodList(shardSfSet.Name, "SHARD", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(shardSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specID, string(databasev4.PodNotFound))
		return shardSfSet, shardPod, err
	}
	isPodExist, shardPod := shardingv1.PodListValidation(podList, shardSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Shard " + shardingv1.GetFmtStr(OraShardSpex.Name) + " pod. Shard pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specID, string(databasev4.PodNotReadyState))
		return shardSfSet, shardPod, fmt.Errorf("pod doesn't exist")
	}
	err = shardingv1.ValidateDbSetup(shardPod.Name, instance, r.kubeConfig, r.Log)
	if err != nil {
		msg := "Unable to validate shard. Shard doesn't seem to be ready to accept commands."
		if isTransientDbSetupNotReady(err) {
			r.logLegacy("INFO", msg+" cause: "+err.Error(), nil, instance, r.Log)
		} else {
			r.logLegacy("Error", msg+" cause: "+err.Error(), nil, instance, r.Log)
		}
		r.updateShardStatus(instance, specID, string(databasev4.ProvisionState))
		return shardSfSet, shardPod, err
	}

	r.updateShardStatus(instance, specID, string(databasev4.AvailableState))
	return shardSfSet, shardPod, nil
}

// This function updates the shard topology over all
// updateShardTopologyStatus refreshes aggregate topology status from current catalog/GSM/shard readiness.
func (r *ShardingDatabaseReconciler) updateShardTopologyStatus(instance *databasev4.ShardingDatabase) {
	// Keep validating catalog to preserve status refresh side-effects.
	if _, _, err := r.validateCatalog(instance); err != nil {
		r.logLegacy("DEBUG", "updateShardTopologyStatus: catalog not ready: "+err.Error(), nil, instance, r.Log)
	}

	_, gsmPod, err := r.validateGsm(instance)
	if err != nil {
		r.logLegacy("DEBUG", "updateShardTopologyStatus: gsm not ready: "+err.Error(), nil, instance, r.Log)
		return
	}
	if gsmPod == nil || strings.TrimSpace(gsmPod.Name) == "" {
		r.logLegacy("DEBUG", "updateShardTopologyStatus: empty gsm pod, skipping shard-online verification", nil, instance, r.Log)
		return
	}

	r.updateShardTopologyShardsInGsm(instance, gsmPod)
}

// updateShardTopologyShardsInGsm verifies shard online-state visibility from GSM.
func (r *ShardingDatabaseReconciler) updateShardTopologyShardsInGsm(instance *databasev4.ShardingDatabase, gsmPod *corev1.Pod) {
	if gsmPod == nil || strings.TrimSpace(gsmPod.Name) == "" || len(instance.Spec.Shard) == 0 {
		return
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		oraShardSpec := instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(oraShardSpec.IsDelete), "failed") {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, instance, r.Log) {
			continue
		}

		shardSfSet, _, err := r.validateShard(instance, oraShardSpec, int(i))
		if err != nil {
			r.logLegacy("DEBUG", "updateShardTopologyShardsInGsm: shard not ready for verification: "+oraShardSpec.Name+": "+err.Error(), nil, instance, r.Log)
			continue
		}
		if err := r.verifyShards(instance, gsmPod, shardSfSet, oraShardSpec); err != nil {
			r.logLegacy("DEBUG", "updateShardTopologyShardsInGsm: verifyShards returned error for "+oraShardSpec.Name+": "+err.Error(), nil, instance, r.Log)
		}
	}
}

// updateGsmStatus updates GSM status fields and emits transition events.
func (r *ShardingDatabaseReconciler) updateGsmStatus(instance *databasev4.ShardingDatabase, specIdx int, state string) {
	if specIdx < 0 || specIdx >= len(instance.Spec.Gsm) {
		r.logLegacy("DEBUG", "updateGsmStatus: invalid specIdx", fmt.Errorf("specIdx=%d", specIdx), instance, r.Log)
		return
	}

	currState := strings.TrimSpace(instance.Status.Gsm.State)
	name := instance.Spec.Gsm[specIdx].Name

	eventChanged := currState != state
	eventMsg := "The gsm " + shardingv1.GetFmtStr(name) + " state set to " + state
	if currState != "" {
		eventMsg = "The gsm " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state
	}

	instance.Status.Gsm.State = state
	shardingv1.UpdateGsmStatusData(instance, specIdx, state, r.kubeConfig, r.Log)

	if eventChanged {
		r.publishEvents(instance, eventMsg, state)
	}
}

// updateCatalogStatus updates catalog status fields and emits transition events.
func (r *ShardingDatabaseReconciler) updateCatalogStatus(instance *databasev4.ShardingDatabase, specIdx int, state string) {
	if specIdx < 0 || specIdx >= len(instance.Spec.Catalog) {
		r.logLegacy("DEBUG", "updateCatalogStatus: invalid specIdx", fmt.Errorf("specIdx=%d", specIdx), instance, r.Log)
		return
	}

	name := instance.Spec.Catalog[specIdx].Name
	currState := shardingv1.GetGsmCatalogStatusKey(instance, name+"_"+string(databasev4.State))
	eventChanged := currState != state

	eventMsg := "The catalog " + shardingv1.GetFmtStr(name) + " state set to " + state
	if currState != "NOSTATE" {
		eventMsg = "The catalog " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state
	}

	shardingv1.UpdateCatalogStatusData(instance, specIdx, state, r.kubeConfig, r.Log)
	if err := r.Status().Update(context.Background(), instance); err != nil {
		r.logLegacy("DEBUG", "updateCatalogStatus: status update failed", err, instance, r.Log)
	}

	if eventChanged {
		r.publishEvents(instance, eventMsg, state)
	}
}

// updateShardStatus updates shard status fields and emits transition events.
func (r *ShardingDatabaseReconciler) updateShardStatus(instance *databasev4.ShardingDatabase, specIdx int, state string) {
	if specIdx < 0 || specIdx >= len(instance.Spec.Shard) {
		r.logLegacy("DEBUG", "updateShardStatus: invalid specIdx", fmt.Errorf("specIdx=%d", specIdx), instance, r.Log)
		return
	}

	name := instance.Spec.Shard[specIdx].Name
	currState := shardingv1.GetGsmShardStatusKey(instance, name+"_"+string(databasev4.State))
	eventChanged := currState != state

	eventMsg := "The shard " + shardingv1.GetFmtStr(name) + " state set to " + state
	if currState != "NOSTATE" {
		eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state
	}

	shardingv1.UpdateShardStatusData(instance, specIdx, state, r.kubeConfig, r.Log)
	if err := r.Status().Update(context.Background(), instance); err != nil {
		r.logLegacy("DEBUG", "updateShardStatus: status update failed", err, instance, r.Log)
	}

	if eventChanged {
		r.publishEvents(instance, eventMsg, state)
	}
}

// updateGsmShardStatus updates per-shard GSM lifecycle status and emits transition events.
func (r *ShardingDatabaseReconciler) updateGsmShardStatus(instance *databasev4.ShardingDatabase, name string, state string) {
	currState := shardingv1.GetGsmShardStatus(instance, name)
	eventChanged := currState != state

	eventMsg := "The shard " + shardingv1.GetFmtStr(name) + " state set to " + state + " in Gsm."
	if currState != "NOSTATE" {
		eventMsg = "The shard " + shardingv1.GetFmtStr(name) + " state changed from " + currState + " to " + state + " in Gsm"
	}

	if eventChanged {
		shardingv1.UpdateGsmShardStatus(instance, name, state)
		if err := r.Status().Update(context.Background(), instance); err != nil {
			r.logLegacy("DEBUG", "updateGsmShardStatus: status update failed", err, instance, r.Log)
		}
		r.publishEvents(instance, eventMsg, state)
	}
}

// ensurePrimaryRefForStandby fills spec.shard[].primaryDatabaseRef for standby shards,
// using the PRIMARY shard from the same shardGroup (or first PRIMARY shard as fallback).
// ensurePrimaryRefForStandby populates standby primaryDatabaseRef values from discovered primary shards.
func (r *ShardingDatabaseReconciler) ensurePrimaryRefForStandby(
	ctx context.Context,
	instance *databasev4.ShardingDatabase,
) (bool, error) {
	_ = ctx

	primByGroup := map[string]*databasev4.ShardSpec{}
	var firstPrim *databasev4.ShardSpec
	shardingType := effectiveShardingTypeForConstraints(instance)
	requireShardGroup := shardingType == "SYSTEM" || shardingType == "COMPOSITE"

	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			if firstPrim == nil {
				firstPrim = s
			}
			g := normalizeShardGroupKey(s.ShardGroup)
			if g != "" {
				if _, ok := primByGroup[g]; !ok {
					primByGroup[g] = s
				}
			}
		}
	}

	changed := false

	for si := range instance.Spec.ShardInfo {
		info := &instance.Spec.ShardInfo[si]
		if info.ShardGroupDetails == nil {
			continue
		}
		if shardingType == "SYSTEM" || shardingType == "COMPOSITE" {
			// SYSTEM standby mapping is resolved per-standby at runtime from the
			// single PRIMARY shardGroup.
			// COMPOSITE standby mapping is resolved per-standby from PRIMARY shards
			// in the same shardSpace.
			// Do not force one shardInfo-level primary ref in these modes.
			continue
		}

		if !strings.EqualFold(strings.TrimSpace(info.ShardGroupDetails.DeployAs), "STANDBY") {
			continue
		}

		if info.PrimaryDatabaseRef != nil && strings.TrimSpace(info.PrimaryDatabaseRef.Host) != "" {
			continue
		}

		if standbyConfigPrimaryCountController(info.StandbyConfig) > 0 {
			// standbyConfig explicitly provides primary source(s); no local primary-ref autofill required.
			continue
		}

		if firstPrim == nil {
			return false, fmt.Errorf("primary shard not found yet")
		}

		key := normalizeShardGroupKey(info.ShardGroupDetails.Name)
		if requireShardGroup && shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" && key == "" {
			return false, fmt.Errorf("standby shardInfo prefix %s must define shardGroupDetails.name in DG replication mode", strings.TrimSpace(info.ShardPreFixName))
		}
		prim := firstPrim
		if requireShardGroup && key != "" {
			if p, ok := primByGroup[key]; ok {
				prim = p
			} else {
				return false, fmt.Errorf("no PRIMARY shard found in shardGroup %s for standby shardInfo prefix %s", key, strings.TrimSpace(info.ShardPreFixName))
			}
		}

		ref := &databasev4.DatabaseRef{
			Host:    fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", prim.Name, prim.Name, instance.Namespace),
			Port:    1521,
			CdbName: strings.ToUpper(prim.Name),
			PdbName: strings.ToUpper(prim.Name) + "PDB",
		}

		// Persist on shardInfo (webhook regenerates spec.shard from shardInfo)
		info.PrimaryDatabaseRef = ref

		// Also set in-memory for this reconcile pass (so STS/env builder sees it now)
		prefix := strings.TrimSpace(info.ShardPreFixName)
		for sj := range instance.Spec.Shard {
			s := &instance.Spec.Shard[sj]
			if strings.EqualFold(strings.TrimSpace(s.DeployAs), "STANDBY") && prefix != "" && strings.HasPrefix(s.Name, prefix) {
				s.PrimaryDatabaseRef = ref
			}
		}

		changed = true
	}

	return changed, nil
}

// This function add the Primary Shards in GSM
// addPrimaryShards adds missing primary shards in GSM and deploys them once added.
func (r *ShardingDatabaseReconciler) addPrimaryShards(ctx context.Context, instance *databasev4.ShardingDatabase) error {
	var err error
	var shardSfSet *appsv1.StatefulSet
	gsmPod := &corev1.Pod{}

	hasPrimary := false
	notReadyCount := 0
	addFailedCount := 0
	deployFlag := "none"
	var deployParams string

	r.logLegacy("DEBUG", "Starting the shard adding operation.", nil, instance, r.Log)

	if len(instance.Spec.Shard) == 0 {
		return nil
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		oraShardSpec := instance.Spec.Shard[i]
		deployAs := strings.ToUpper(strings.TrimSpace(oraShardSpec.DeployAs))
		if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, instance, r.Log) {
			continue
		}
		hasPrimary = true
		break
	}

	if !hasPrimary {
		return nil
	}

	if err := r.ensureTDEKeysExported(ctx, instance); err != nil {
		return err
	}

	// Validate GSM once per reconcile pass for all primary shards.
	_, gsmPod, err = r.validateGsm(instance)
	if err != nil {
		return err
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		oraShardSpec := instance.Spec.Shard[i]
		deployAs := strings.ToUpper(strings.TrimSpace(oraShardSpec.DeployAs))
		if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, instance, r.Log) {
			continue
		}

		// 1) Validate shard before any GSM operation.
		shardSfSet, _, err = r.validateShard(instance, oraShardSpec, int(i))
		if err != nil {
			notReadyCount++
			continue
		}
		if err := r.ensureTDEKeysImportedForShard(ctx, instance, oraShardSpec); err != nil {
			r.logLegacy("INFO", "ImportTDEKey pending for primary shard; requeue: "+err.Error(), nil, instance, r.Log)
			return err
		}

		// 2) Ensure shard exists in GSM. Add only when missing.
		sparamsCheck := shardingv1.BuildShardParams(instance, shardSfSet, oraShardSpec)
		if err = shardingv1.CheckShardInGsm(gsmPod.Name, sparamsCheck, instance, r.kubeConfig, r.Log); err != nil {
			sparamsAdd := shardingv1.BuildShardParamsForAdd(instance, shardSfSet, oraShardSpec)
			if preErr := r.ensureShardAddPrerequisitesInGsm(instance, gsmPod.Name, oraShardSpec); preErr != nil {
				r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.AddingShardErrorState))
				r.logLegacy("Error", instance.Namespace+":Shard pre-add prerequisite setup failure:"+preErr.Error(), nil, instance, r.Log)
				addFailedCount++
				deployFlag = "false"
				continue
			}
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.AddingShardState))
			if err = shardingv1.AddShardInGsm(gsmPod.Name, sparamsAdd, instance, r.kubeConfig, r.Log); err != nil {
				r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.AddingShardErrorState))
				r.logLegacy("Error", instance.Namespace+":Shard Addition Failure:"+err.Error(), nil, instance, r.Log)
				addFailedCount++
				deployFlag = "false"
				continue
			}
			deployFlag = "true"
			deployParams = sparamsAdd
		}
	}

	if notReadyCount > 0 {
		r.logLegacy("INFO", "Some shards are still pending for addition. Requeue the reconcile loop.", nil, instance, r.Log)
		return fmt.Errorf("shards are not ready for addition: %d", notReadyCount)
	}

	if addFailedCount > 0 {
		r.logLegacy("INFO", "Shards are not added in GSM. Deploy operation will happen after shard addition. Requeue the reconcile loop.", nil, instance, r.Log)
		return fmt.Errorf("shards addition are pending: %d shard add operation(s) failed", addFailedCount)
	}

	// Deploy shard changes for each shard that is not yet deployed in GSM.
	if deployFlag == "true" {
		if derr := shardingv1.DeployShardInGsm(gsmPod.Name, deployParams, instance, r.kubeConfig, r.Log); derr != nil {
			r.logLegacy("INFO", "DeployShardInGsm pending; requeue: "+derr.Error(), nil, instance, r.Log)
			return fmt.Errorf("deploy shard in GSM pending: %w", derr)
		}
	}

	if len(deployParams) > 0 {
		r.updateShardTopologyShardsInGsm(instance, gsmPod)
	}

	r.logLegacy("INFO", "Completed the shard addition operation. For details, check the CRD resource status for GSM and Shards.", nil, instance, r.Log)
	return nil
}

// This function Check the online shard
// verifyShards checks whether a shard is online in GSM and updates shard-online status.
func (r *ShardingDatabaseReconciler) verifyShards(instance *databasev4.ShardingDatabase, gsmPod *corev1.Pod, shardSfSet *appsv1.StatefulSet, OraShardSpex databasev4.ShardSpec) error {
	if gsmPod == nil || strings.TrimSpace(gsmPod.Name) == "" {
		r.logLegacy("DEBUG", "verifyShards skipped: GSM pod is empty", nil, instance, r.Log)
		return nil
	}
	if shardSfSet == nil || strings.TrimSpace(shardSfSet.Name) == "" {
		r.logLegacy("DEBUG", "verifyShards skipped: shard StatefulSet is empty for "+OraShardSpex.Name, nil, instance, r.Log)
		return nil
	}

	if shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" {
		deployAs := strings.ToUpper(strings.TrimSpace(OraShardSpex.DeployAs))
		if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
			r.logLegacy("INFO", "DG mode: skipping CheckOnlineShardInGsm for standby shard "+OraShardSpex.Name, nil, instance, r.Log)
			return nil
		}
	}

	// Verify shard online state in GSM.
	sparams := shardingv1.BuildShardParams(instance, shardSfSet, OraShardSpex)
	if err := shardingv1.CheckOnlineShardInGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log); err != nil {
		// Treat as transient; don't flip status to ERROR in GSM status
		r.logLegacy("INFO", "CheckOnlineShardInGsm failed; will retry: "+err.Error(), nil, instance, r.Log)
		return nil
	}

	shardName := OraShardSpex.Name
	oldStateStr := shardingv1.GetGsmShardStatus(instance, shardName)
	r.updateGsmShardStatus(instance, shardName, string(databasev4.ShardOnlineState))

	// Emit completion log only on transition to online.
	if oldStateStr != string(databasev4.ShardOnlineState) {
		title := instance.Namespace + ":Shard Addition Completed"
		message := ":Shard addition completed for shard " + shardingv1.GetFmtStr(shardName) + " in GSM."
		r.logLegacy("INFO", title+":"+message, nil, instance, r.Log)
	}
	return nil
}

// addStandbyShards adds standby shards and performs DG broker provisioning when configured.
func (r *ShardingDatabaseReconciler) addStandbyShards(_ context.Context, instance *databasev4.ShardingDatabase) error {
	var err error
	isDGReplication := shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG"

	var shardSfSet *appsv1.StatefulSet
	gsmPod := &corev1.Pod{}

	addFailedCount := 0
	notReadyCount := 0
	hasStandby := false
	deployParamsList := []string{}

	r.logLegacy("DEBUG", "Starting standby shard adding operation.", nil, instance, r.Log)

	if len(instance.Spec.Shard) == 0 {
		return nil
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		OraShardSpex := instance.Spec.Shard[i]

		if shardingv1.CheckIsDeleteFlag(OraShardSpex.IsDelete, instance, r.Log) {
			continue
		}

		deployAs := strings.ToUpper(strings.TrimSpace(OraShardSpex.DeployAs))
		if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
			continue
		}
		hasStandby = true
		break
	}

	if !hasStandby {
		return nil
	}

	// Validate GSM once per reconcile pass for all standby shards.
	_, gsmPod, err = r.validateGsm(instance)
	if err != nil {
		return err
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		OraShardSpex := instance.Spec.Shard[i]

		if shardingv1.CheckIsDeleteFlag(OraShardSpex.IsDelete, instance, r.Log) {
			continue
		}

		deployAs := strings.ToUpper(strings.TrimSpace(OraShardSpex.DeployAs))
		if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
			continue
		}

		// 1) validate standby shard is up
		shardSfSet, _, err = r.validateShard(instance, OraShardSpex, int(i))
		if err != nil {
			notReadyCount++
			continue
		}
		r.logLegacy("INFO", "Skipping ImportTDEKey for standby shard "+OraShardSpex.Name+"; standby wallet copy is authoritative", nil, instance, r.Log)

		// 3) Non-DG flow: standby shard add in GSM
		if !isDGReplication {
			sparamsCheck := shardingv1.BuildShardParams(instance, shardSfSet, OraShardSpex)

			if inGsmErr := shardingv1.CheckShardInGsm(gsmPod.Name, sparamsCheck, instance, r.kubeConfig, r.Log); inGsmErr != nil {
				sparamsAdd := shardingv1.BuildShardParamsForAdd(instance, shardSfSet, OraShardSpex)
				if preErr := r.ensureShardAddPrerequisitesInGsm(instance, gsmPod.Name, OraShardSpex); preErr != nil {
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev4.AddingShardErrorState))
					addFailedCount++
					continue
				}

				r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev4.AddingShardState))
				err = shardingv1.AddShardInGsm(gsmPod.Name, sparamsAdd, instance, r.kubeConfig, r.Log)
				if err != nil {
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev4.AddingShardErrorState))
					addFailedCount++
					continue
				}
				deployParamsList = append(deployParamsList, sparamsAdd)
			}

			// Deploy whenever standby shard exists but is not yet deployed.
		} else {
			// DG flow: skip standby shard add in GSM
			if instance.Status.Dg.Broker == nil {
				instance.Status.Dg.Broker = map[string]string{}
			}
			if strings.TrimSpace(instance.Status.Dg.Broker[OraShardSpex.Name]) == "" {
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "SKIPPED_GSM")
			}
		}

		// 4) DG flow
		if isDGReplication && !r.dgBrokerDone(instance, OraShardSpex.Name) {
			if instance.Status.Dg.Broker == nil {
				instance.Status.Dg.Broker = map[string]string{}
			}
			instance.Status.Dg.State = "PENDING"
			r.setDgBrokerStatus(instance, OraShardSpex.Name, "pending")
			if err := r.validateStandbyWalletSecretRef(instance, OraShardSpex); err != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:wallet-secret:"+err.Error())
				return err
			}

			primary, primaryConnects, primarySource, perr := r.resolvePrimaryForStandbyDG(instance, OraShardSpex)
			if perr != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:"+perr.Error())
				return perr
			}
			if len(primaryConnects) == 0 {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:primary connect identifier is empty")
				return fmt.Errorf("primary connect identifier is empty for standby %s", OraShardSpex.Name)
			}
			if primary == nil {
				// In connect-string/endpoint-only modes we may not have a local primary pod to run SQL*Plus steps.
				// Keep pair mapping/status, but defer broker role operations to the DG broker controller.
				r.logLegacy(
					"INFO",
					"Skipping local DG pod orchestration for standby "+OraShardSpex.Name+"; resolved primary source="+primarySource+" has no local PRIMARY pod",
					nil, instance, r.Log,
				)
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "SKIPPED_NO_LOCAL_PRIMARY")
				continue
			}

			cfgName := r.buildDgConfigName(instance, *primary, OraShardSpex)
			standbyConnects := []string{r.buildShardConnectIdentifier(instance, OraShardSpex, strings.ToUpper(strings.TrimSpace(OraShardSpex.Name)))}

			workflow := dgsharding.NewStandbyWorkflow(dgsharding.StandbyWorkflowOptions{
				Instance:        instance,
				Primary:         *primary,
				Standby:         OraShardSpex,
				CfgName:         cfgName,
				PrimaryConnects: primaryConnects,
				StandbyConnects: standbyConnects,
				KubeConfig:      r.kubeConfig,
				Log:             r.Log,
				ConfigurePrimaryRedoTransport: func(instance *databasev4.ShardingDatabase, primary, standby databasev4.ShardSpec) error {
					allStandbysForPrimary := r.collectStandbysForPrimary(instance, primary)
					return r.setupPrimaryRedoTransport(instance, primary, standby, allStandbysForPrimary)
				},
				EnsureStandbyApplyRunning: func(instance *databasev4.ShardingDatabase, standby databasev4.ShardSpec) error {
					return r.EnsureStandbyApplyRunning(instance, standby)
				},
				ForceArchiveAndCheckTransport: func(instance *databasev4.ShardingDatabase, primary databasev4.ShardSpec) error {
					return r.ForceArchiveAndCheckRedoTransport(instance, primary)
				},
				SetDgConnectIdentifiers: func(instance *databasev4.ShardingDatabase, primary, standby databasev4.ShardSpec) error {
					return r.SetDgConnectIdentifiers(instance, primary, standby)
				},
			})
			if e := dataguardcommon.RunStandbyDGBrokerWorkflow(workflow); e != nil {
				instance.Status.Dg.State = "ERROR"
				status := "error:workflow:" + e.Error()
				if stepErr, ok := e.(*dataguardcommon.StepError); ok {
					status = dgsharding.StatusForWorkflowStep(stepErr.Step) + ":" + stepErr.Err.Error()
				}
				r.setDgBrokerStatus(instance, OraShardSpex.Name, status)
				return e
			}

			r.setDgBrokerStatus(instance, OraShardSpex.Name, "true")
			instance.Status.Dg.State = "ENABLED"
			_ = r.updateStatusWithRetry(instance, func(latest *databasev4.ShardingDatabase) {
				latest.Status.Dg.State = "ENABLED"
			})
		}
	}

	if notReadyCount > 0 {
		r.logLegacy("INFO", "Some standby shards are still pending for readiness. Requeue reconcile loop.", nil, instance, r.Log)
		return fmt.Errorf("standby shards are not ready for operation: %d", notReadyCount)
	}

	if addFailedCount > 0 {
		r.logLegacy("INFO", "Standby shard flow not complete yet; requeue.", nil, instance, r.Log)
		return fmt.Errorf("standby shard flow pending: %d shard add operation(s) failed", addFailedCount)
	}

	if !isDGReplication {
		for _, deployParams := range deployParamsList {
			if derr := shardingv1.DeployShardInGsm(gsmPod.Name, deployParams, instance, r.kubeConfig, r.Log); derr != nil {
				r.logLegacy("INFO", "DeployShardInGsm pending for standby shard; requeue: "+derr.Error(), nil, instance, r.Log)
				return fmt.Errorf("standby deploy in GSM pending: %w", derr)
			}
		}
		if len(deployParamsList) > 0 {
			r.updateShardTopologyShardsInGsm(instance, gsmPod)
		}
	} else {
		r.logLegacy("INFO", "DG standby shard flow: skipping DeployShardInGsm() for standby shard", nil, instance, r.Log)
	}

	r.logLegacy("INFO", "Completed standby shard operation.", nil, instance, r.Log)
	return nil
}

// ========== Delete Shard Section====================
// delGsmShard orchestrates shard scale-in/delete, including chunk movement and GSM cleanup.
func (r *ShardingDatabaseReconciler) delGsmShard(instance *databasev4.ShardingDatabase) error {
	var err error
	var shardPod *corev1.Pod
	gsmPod := &corev1.Pod{}

	const (
		pollInterval     = 20 * time.Second
		maxVerifyRetries = 90
		stallLimit       = 4
		maxMoveResubmits = 5
	)

	r.logLegacy("DEBUG", "Starting shard deletion operation.", nil, instance, r.Log)

	if len(instance.Spec.Shard) == 0 {
		return nil
	}

	// Fast-path: no shard selected for delete in this reconcile pass.
	hasDeleteTargets := false
	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		if shardingv1.CheckIsDeleteFlag(instance.Spec.Shard[i].IsDelete, instance, r.Log) {
			hasDeleteTargets = true
			break
		}
	}
	if !hasDeleteTargets {
		return nil
	}

	// GSM readiness is shared across all shard delete operations.
	_, gsmPod, err = r.validateGsm(instance)
	if err != nil {
		return err
	}

	for i := int32(0); i < int32(len(instance.Spec.Shard)); i++ {
		oraShardSpec := instance.Spec.Shard[i]

		if !shardingv1.CheckIsDeleteFlag(oraShardSpec.IsDelete, instance, r.Log) {
			continue
		}

		r.logLegacy("INFO", "Selected shard "+oraShardSpec.Name+" for deletion", nil, instance, r.Log)

		patchDeleteFlag := func(val string) error {
			oldObj := instance.DeepCopy()
			newObj := instance.DeepCopy()
			newObj.Spec.Shard[i].IsDelete = val

			if err := shardingv1.InstanceShardPatch(oldObj, newObj, r.Client, i, "isDelete", val); err != nil {
				return err
			}

			instance.Spec.Shard[i].IsDelete = val
			return nil
		}

		markChunkMoveFailed := func(cause error) error {
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.ChunkMoveError))
			if perr := patchDeleteFlag("failed"); perr != nil {
				r.logLegacy("Error", "Failed to patch isDelete=failed for shard "+oraShardSpec.Name, perr, instance, r.Log)
				return perr
			}
			return cause
		}

		submitMoveChunks := func(tag string, sparams string) error {
			if tag == "initial" {
				r.logLegacy("INFO", "Starting chunk movement for shard "+oraShardSpec.Name, nil, instance, r.Log)
			} else {
				r.logLegacy("INFO", "Retrying chunk movement for shard "+oraShardSpec.Name, nil, instance, r.Log)
			}
			return shardingv1.MoveChunks(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log)
		}

		chkState := shardingv1.GetGsmShardStatus(instance, oraShardSpec.Name)

		// Step 2: check physical shard existence directly
		shardSfSet, err := shardingv1.CheckSfset(oraShardSpec.Name, instance, r.Client)
		if err != nil {
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.Terminated))
			r.updateShardStatus(instance, int(i), string(databasev4.Terminated))
			continue
		}

		// best effort pod lookup
		shardPod = nil
		podList, perr := shardingv1.GetPodList(shardSfSet.Name, "SHARD", instance, r.Client)
		if perr == nil {
			_, pod := shardingv1.PodListValidation(podList, shardSfSet.Name, instance, r.Client)
			if pod != nil && pod.Name != "" {
				shardPod = pod
			}
		}

		// If shard is not present in GSM cache, still delete physical resources
		if chkState == "NOSTATE" {
			r.logLegacy("INFO", "Shard "+oraShardSpec.Name+" is already absent in GSM; deleting physical resources", nil, instance, r.Log)
			r.delShard(instance, shardSfSet.Name, shardSfSet, shardPod, int(i))
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.Terminated))
			r.updateShardStatus(instance, int(i), string(databasev4.Terminated))
			continue
		}

		// Step 3: best-effort validate shard
		_, _, err = r.validateShard(instance, oraShardSpec, int(i))
		if err != nil {
			r.logLegacy("DEBUG", "validateShard failed during delete flow for "+oraShardSpec.Name+": "+err.Error(), nil, instance, r.Log)
		}

		// Step 4: check if shard exists in GSM
		sparams := shardingv1.BuildShardParams(instance, shardSfSet, oraShardSpec)

		err = shardingv1.CheckShardInGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log)
		if err != nil {
			r.logLegacy("INFO", "Shard "+oraShardSpec.Name+" not found in GSM; deleting physical resources", nil, instance, r.Log)
			r.delShard(instance, shardSfSet.Name, shardSfSet, shardPod, int(i))
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.Terminated))
			r.updateShardStatus(instance, int(i), string(databasev4.Terminated))
			continue
		}

		// Step 5: ensure online in GSM
		r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.DeletingState))

		err = shardingv1.CheckOnlineShardInGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log)
		if err != nil {
			r.logLegacy("INFO", "Shard "+oraShardSpec.Name+" is not online in GSM; retrying later", nil, instance, r.Log)
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.DeleteErrorState))
			continue
		}

		// Step 6: move chunks before deleting PRIMARY shard
		if r.shouldMoveChunksBeforeDelete(instance, oraShardSpec) {
			if err = submitMoveChunks("initial", sparams); err != nil {
				return markChunkMoveFailed(err)
			}

			lastSummary := ""
			stallCount := 0
			resubmitCount := 0
			chunksCleared := false

			for retry := 1; retry <= maxVerifyRetries; retry++ {
				time.Sleep(pollInterval)

				remaining, summary, cerr := shardingv1.CheckChunksRemaining(
					gsmPod.Name,
					sparams,
					instance,
					r.kubeConfig,
					r.Log,
				)
				if cerr != nil {
					return markChunkMoveFailed(cerr)
				}

				if !remaining {
					r.logLegacy("INFO", "All chunks moved successfully for shard "+oraShardSpec.Name, nil, instance, r.Log)
					chunksCleared = true
					break
				}

				if retry == 1 || retry%5 == 0 {
					progressMsg := fmt.Sprintf("Chunks are still moving for shard %s", oraShardSpec.Name)
					if strings.TrimSpace(summary) != "" {
						progressMsg += ": " + summary
					}
					r.logLegacy("INFO", progressMsg, nil, instance, r.Log)
				}

				if summary == lastSummary {
					stallCount++
				} else {
					lastSummary = summary
					stallCount = 0
				}

				if stallCount >= stallLimit {
					if resubmitCount < maxMoveResubmits {
						resubmitCount++
						stallCount = 0
						lastSummary = ""

						if err = submitMoveChunks(fmt.Sprintf("resubmit-%d/%d", resubmitCount, maxMoveResubmits), sparams); err != nil {
							return markChunkMoveFailed(err)
						}
						continue
					}

					return markChunkMoveFailed(fmt.Errorf("chunk movement stalled for shard %s", oraShardSpec.Name))
				}

				if retry == maxVerifyRetries {
					return markChunkMoveFailed(fmt.Errorf("chunk movement timed out for shard %s", oraShardSpec.Name))
				}
			}

			if !chunksCleared {
				return markChunkMoveFailed(fmt.Errorf("chunk movement incomplete for shard %s", oraShardSpec.Name))
			}
		} else {
			r.logLegacy("INFO", "Skipping chunk movement for shard "+oraShardSpec.Name, nil, instance, r.Log)
		}

		// Step 7: remove from GSM
		r.logLegacy("INFO", "Removing shard "+oraShardSpec.Name+" from GSM", nil, instance, r.Log)
		err = shardingv1.RemoveShardFromGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log)
		if err != nil {
			r.logLegacy("Error", "Error occurred during shard removal from GSM for "+oraShardSpec.Name, err, instance, r.Log)
			r.updateShardStatus(instance, int(i), string(databasev4.ShardRemoveError))
			if perr := patchDeleteFlag("failed"); perr != nil {
				return perr
			}
			continue
		}

		// Step 8: delete physical resources
		r.logLegacy("INFO", "Deleting Kubernetes resources for shard "+oraShardSpec.Name, nil, instance, r.Log)
		r.delShard(instance, shardSfSet.Name, shardSfSet, shardPod, int(i))
		r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.Terminated))
		r.updateShardStatus(instance, int(i), string(databasev4.Terminated))

		r.logLegacy("INFO", "Shard "+oraShardSpec.Name+" scale-in completed successfully", nil, instance, r.Log)
	}

	return nil
}

// This function delete the physical shard
// delShard deletes shard Kubernetes resources and associated PVCs (when enabled).
func (r *ShardingDatabaseReconciler) delShard(instance *databasev4.ShardingDatabase, sfSetName string, sfSetFound *appsv1.StatefulSet, sfsetPod *corev1.Pod, specIdx int) {
	var err error
	var msg string
	var svcFound *corev1.Service

	if sfsetPod != nil && sfsetPod.Name != "" {
		err = shardingv1.SfsetLabelPatch(sfSetFound, sfsetPod, instance, r.Client)
		if err != nil {
			msg = "Failed to patch the Shard StatefulSet: " + sfSetFound.Name
			r.logLegacy("DEBUG", msg, err, instance, r.Log)
			r.updateShardStatus(instance, specIdx, string(databasev4.LabelPatchingError))
			return
		}
	}

	err = r.Client.Delete(context.Background(), sfSetFound)
	if err != nil {
		msg = "Failed to delete Shard StatefulSet: " + shardingv1.GetFmtStr(sfSetFound.Name)
		r.logLegacy("DEBUG", msg, err, instance, r.Log)
		r.updateShardStatus(instance, specIdx, string(databasev4.DeleteErrorState))
		return
	}

	if instance.Spec.IsExternalSvc {
		svcFound, err = shardingv1.CheckSvc(sfSetName+strconv.FormatInt(int64(0), 10)+"-svc", instance, r.Client)
		if err == nil {
			err = r.Client.Delete(context.Background(), svcFound)
			if err != nil {
				return
			}
		}
	}

	svcFound, err = shardingv1.CheckSvc(sfSetName, instance, r.Client)
	if err == nil {
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
			r.logLegacy("DEBUG", msg, err, instance, r.Log)
			r.updateShardStatus(instance, specIdx, string(databasev4.DeletePVCError))
		}
	}
}

// ======== GSM Invited Node ==========
// Remove and add GSM invited node
// gsmInvitedNodeOp handles gsm invited node op for the sharding database controller.
func (r *ShardingDatabaseReconciler) gsmInvitedNodeOp(instance *databasev4.ShardingDatabase, objName string,
) {
	const (
		maxRetries   = 10
		retryBackoff = 20 * time.Second
	)

	r.logLegacy("DEBUG", "Starting GSM invited-node operation for "+shardingv1.GetFmtStr(objName), nil, instance, r.Log)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, gsmPod, err := r.validateGsm(instance)
		if err != nil {
			r.logLegacy("DEBUG",
				fmt.Sprintf("GSM validation failed for invited-node op (attempt %d/%d), retrying", attempt, maxRetries),
				err, instance, r.Log)
			time.Sleep(retryBackoff)
			continue
		}

		if err := shardingv1.ValidateDbSetup(objName, instance, r.kubeConfig, r.Log); err != nil {
			r.logLegacy("DEBUG",
				fmt.Sprintf("Target pod %s not ready for invited-node op (attempt %d/%d), retrying", shardingv1.GetFmtStr(objName), attempt, maxRetries),
				err, instance, r.Log)
			time.Sleep(retryBackoff)
			continue
		}

		_, _, err = shardingv1.ExecCommand(gsmPod.Name, shardingv1.GetShardInviteNodeCmd(objName), r.kubeConfig, instance, r.Log)
		if err != nil {
			r.logLegacy("DEBUG",
				fmt.Sprintf("Invite node operation failed for %s (attempt %d/%d)", shardingv1.GetFmtStr(objName), attempt, maxRetries),
				err, instance, r.Log)
			time.Sleep(retryBackoff)
			continue
		}

		r.logLegacy("INFO", "Invited node operation completed successfully in GSM after pod "+shardingv1.GetFmtStr(objName)+" restart.", nil, instance, r.Log)
		return
	}

	r.logLegacy("DEBUG", "Invited node operation did not complete within retry budget for "+shardingv1.GetFmtStr(objName), nil, instance, r.Log)
}

// ================================== CREATE FUNCTIONS =============================
// This function create a service based isExtern parameter set in the yaml file
// createService ensures a Service exists for the requested topology member.
func (r *ShardingDatabaseReconciler) createService(instance *databasev4.ShardingDatabase,
	dep *corev1.Service,
) (ctrl.Result, error) {
	if dep == nil {
		return ctrl.Result{}, fmt.Errorf("createService received nil Service")
	}

	reqLogger := r.Log.WithValues(
		"instanceNamespace", instance.Namespace,
		"instanceName", instance.Name,
		"serviceName", dep.GetName(),
	)

	if strings.TrimSpace(dep.Namespace) == "" {
		dep.Namespace = instance.Namespace
	}
	if r.Scheme == nil {
		return ctrl.Result{}, fmt.Errorf("kubernetes scheme is nil; cannot set controller reference for service %s", dep.Name)
	}
	if err := controllerutil.SetControllerReference(instance, dep, r.Scheme); err != nil {
		reqLogger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	changed, err := sharedk8sobjects.EnsureService(context.TODO(), r.Client, dep.Namespace, dep, sharedk8sobjects.ServiceSyncOptions{
		NodePortMerge:             sharedk8sobjects.NodePortMergeByNamePortAndProtocol,
		SyncOwnerReferences:       true,
		SyncSessionAffinityCfg:    true,
		SyncPublishNotReady:       true,
		SyncInternalTrafficPolicy: true,
		SyncLoadBalancerFields:    true,
		SyncHealthCheckNodePort:   true,
	})
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}
	if changed {
		reqLogger.Info("Service reconciled to desired state")
		return ctrl.Result{Requeue: true}, nil
	}
	r.logLegacy("DEBUG", "Service "+shardingv1.GetFmtStr(dep.Name)+" already in desired state", nil, instance, r.Log)
	return ctrl.Result{}, nil
}

// This function deploy the statefulset

// deployStatefulSet ensures a StatefulSet exists for the requested topology member.
func (r *ShardingDatabaseReconciler) deployStatefulSet(
	instance *databasev4.ShardingDatabase,
	dep *appsv1.StatefulSet,
	resType string,
	containerName string,
	desiredResources *corev1.ResourceRequirements,
) (ctrl.Result, error) {
	if dep == nil {
		return ctrl.Result{}, fmt.Errorf("deployStatefulSet received nil StatefulSet for %s", strings.ToUpper(strings.TrimSpace(resType)))
	}

	reqLogger := r.Log.WithValues(
		"instanceNamespace", instance.Namespace,
		"instanceName", instance.Name,
		"resourceType", strings.ToUpper(strings.TrimSpace(resType)),
		"statefulSetName", dep.GetName(),
	)
	if strings.TrimSpace(dep.Namespace) == "" {
		dep.Namespace = instance.Namespace
	}
	if r.Scheme == nil {
		return ctrl.Result{}, fmt.Errorf("kubernetes scheme is nil; cannot set controller reference for statefulset %s", dep.Name)
	}

	if err := controllerutil.SetControllerReference(instance, dep, r.Scheme); err != nil {
		reqLogger.Error(err, "Failed to set controller reference")
		return ctrl.Result{}, err
	}

	result, err := sharedk8sobjects.ReconcileStatefulSet(
		context.TODO(),
		r.Client,
		dep.Namespace,
		dep,
		func(found *appsv1.StatefulSet, desired *appsv1.StatefulSet) bool {
			return syncShardingStatefulSetScopedFields(found, desired, containerName, desiredResources)
		},
	)
	if err != nil {
		reqLogger.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}
	if result.Created {
		reqLogger.Info("Creating StatefulSet")
		r.logLegacy("INFO", "Created StatefulSet "+shardingv1.GetFmtStr(dep.Name), nil, instance, r.Log)
		return ctrl.Result{Requeue: true}, nil
	}
	if result.Updated {
		reqLogger.Info("StatefulSet reconciled to desired scoped fields")
		r.logLegacy("INFO", "Updated StatefulSet "+shardingv1.GetFmtStr(dep.Name), nil, instance, r.Log)
		return ctrl.Result{Requeue: true}, nil
	}
	r.logLegacy("DEBUG", "StatefulSet "+shardingv1.GetFmtStr(dep.Name)+" already in desired scoped state", nil, instance, r.Log)
	return ctrl.Result{}, nil
}

func syncShardingStatefulSetScopedFields(
	found *appsv1.StatefulSet,
	desired *appsv1.StatefulSet,
	containerName string,
	desiredResources *corev1.ResourceRequirements,
) bool {
	updated := false

	if desired.Spec.Replicas != nil {
		if found.Spec.Replicas == nil || *found.Spec.Replicas != *desired.Spec.Replicas {
			replica := *desired.Spec.Replicas
			found.Spec.Replicas = &replica
			updated = true
		}
	}

	if desiredResources != nil {
		for i := range found.Spec.Template.Spec.Containers {
			if found.Spec.Template.Spec.Containers[i].Name != containerName {
				continue
			}
			if !reflect.DeepEqual(found.Spec.Template.Spec.Containers[i].Resources, *desiredResources) {
				found.Spec.Template.Spec.Containers[i].Resources = *desiredResources
				updated = true
			}
			break
		}
	}

	return updated
}

// checkShardState evaluates shard lifecycle state machine transitions and gate conditions.
func (r *ShardingDatabaseReconciler) checkShardState(instance *databasev4.ShardingDatabase) error {

	var i int32
	var err error = nil
	var OraShardSpex databasev4.ShardSpec
	var currState string
	var eventMsg string
	var msg string

	msg = "checkShardState():ShardType=" + strings.TrimSpace(strings.ToUpper(instance.Spec.ShardingType))
	r.logLegacy("INFO", msg, nil, instance, r.Log)
	if strings.TrimSpace(strings.ToUpper(instance.Spec.ShardingType)) != "USER" {
		// ShardingType is not "USER", so return
		return err
	}

	if len(instance.Status.Gsm.Shards) > 0 {
		for i = 0; i < int32(len(instance.Spec.Shard)); i++ {
			OraShardSpex = instance.Spec.Shard[i]
			currState = shardingv1.GetGsmShardStatus(instance, OraShardSpex.Name)
			if OraShardSpex.IsDelete == "failed" {
				eventMsg = "Shard Deletion  failed for [" + OraShardSpex.Name + "]. Retry shard deletion after manually moving the chunks. Requeuing"
				err = fmt.Errorf("%s", eventMsg)
			} else if currState == string(databasev4.AddingShardState) {
				eventMsg = "Shard Addition in progress for [" + OraShardSpex.Name + "]. Requeuing"
				err = fmt.Errorf("%s", eventMsg)
			} else if currState == string(databasev4.DeletingState) {
				eventMsg = "Shard Deletion in progress for [" + OraShardSpex.Name + "]. Requeuing"
				err = nil
			} else if currState == string(databasev4.DeleteErrorState) {
				eventMsg = "Shard Deletion  Error for [" + OraShardSpex.Name + "]. Manual intervention required. Requeuing"
				err = fmt.Errorf("%s", eventMsg)
			} else if currState == string(databasev4.ShardRemoveError) {
				eventMsg = "Shard Deletion  Error for [" + OraShardSpex.Name + "]. Manual intervention required. Requeuing"
				err = fmt.Errorf("%s", eventMsg)
			} else {
				eventMsg = "checkShardState() : Shard State[" + OraShardSpex.Name + "]=[" + currState + "]"
				r.logLegacy("INFO", eventMsg, nil, instance, r.Log)
				err = nil
			}
			r.publishEvents(instance, eventMsg, currState)
		}
	}
	return err
}

// dgBrokerDone handles dg broker done for the sharding database controller.
func (r *ShardingDatabaseReconciler) dgBrokerDone(instance *databasev4.ShardingDatabase, shardName string) bool {
	if instance.Status.Dg.Broker == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(instance.Status.Dg.Broker[shardName]))
	return v == "true" || v == "enabled" || v == "configured"
}

// setupPrimaryRedoTransport handles setup primary redo transport for the sharding database controller.
func (r *ShardingDatabaseReconciler) setupPrimaryRedoTransport(
	instance *databasev4.ShardingDatabase,
	primary databasev4.ShardSpec,
	standby databasev4.ShardSpec,
	allStandbys []databasev4.ShardSpec,
) error {

	primaryPod := primary.Name + "-0"
	standbyDbUnique := strings.ToUpper(strings.TrimSpace(standby.Name))
	primaryDbUnique := strings.ToUpper(strings.TrimSpace(primary.Name))

	standbyConnect := r.buildShardConnectIdentifier(instance, standby, standbyDbUnique)

	dgNames := []string{primaryDbUnique}
	seen := map[string]bool{primaryDbUnique: true}
	for _, s := range allStandbys {
		dbu := strings.ToUpper(strings.TrimSpace(s.Name))
		if dbu == "" {
			continue
		}
		if !seen[dbu] {
			seen[dbu] = true
			dgNames = append(dgNames, dbu)
		}
	}

	logArchiveConfigSQL := fmt.Sprintf(
		"alter system set log_archive_config='dg_config=(%s)' scope=both sid='*';",
		strings.Join(dgNames, ","),
	)

	logArchiveDest2SQL := fmt.Sprintf(
		"alter system set log_archive_dest_2='service=\"%s\" async valid_for=(online_logfiles,primary_role) db_unique_name=%s' scope=both sid='*';",
		standbyConnect, standbyDbUnique,
	)

	logArchiveDestState2SQL := "alter system set log_archive_dest_state_2=enable scope=both sid='*';"

	standbySvc := shardingv1.BuildDgmgrlServiceName(standbyDbUnique)
	primarySvc := shardingv1.BuildDgmgrlServiceName(primaryDbUnique)

	falServerSQL := fmt.Sprintf(
		"alter system set fal_server='%s' scope=both sid='*';",
		standbySvc,
	)

	falClientSQL := fmt.Sprintf(
		"alter system set fal_client='%s' scope=both sid='*';",
		primarySvc,
	)

	if err := shardingv1.RunSQLPlusInPod(primaryPod, logArchiveConfigSQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}
	if err := shardingv1.RunSQLPlusInPod(primaryPod, logArchiveDest2SQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}
	if err := shardingv1.RunSQLPlusInPod(primaryPod, logArchiveDestState2SQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}
	if err := shardingv1.RunSQLPlusInPod(primaryPod, falServerSQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}
	if err := shardingv1.RunSQLPlusInPod(primaryPod, falClientSQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}

	return nil
}

// ForceArchiveAndCheckRedoTransport handles force archive and check redo transport for the sharding database controller.
func (r *ShardingDatabaseReconciler) ForceArchiveAndCheckRedoTransport(
	instance *databasev4.ShardingDatabase,
	primary databasev4.ShardSpec,
) error {

	primaryPod := primary.Name + "-0"

	forceArchiveSQL := `
alter system archive log current;
alter system archive log current;
`

	checkDestSQL := `
set pages 200 lines 200
col dest_name for a20
col status for a12
col error for a120
col destination for a60
col db_unique_name for a20
select dest_id, dest_name, status, destination, db_unique_name, error
from v$archive_dest_status
where dest_id = 2;
`

	if err := shardingv1.RunSQLPlusInPod(primaryPod, forceArchiveSQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	if err := shardingv1.RunSQLPlusInPod(primaryPod, checkDestSQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}

	return nil
}

// EnsureStandbyApplyRunning handles ensure standby apply running for the sharding database controller.
func (r *ShardingDatabaseReconciler) EnsureStandbyApplyRunning(
	instance *databasev4.ShardingDatabase,
	standby databasev4.ShardSpec,
) error {

	standbyPod := standby.Name + "-0"

	startApplySQL := `
alter database recover managed standby database using current logfile disconnect from session;
`

	verifyApplySQL := `
set pages 200 lines 200
select process,status,thread#,sequence#
from v$managed_standby
order by process;
`

	if err := shardingv1.RunSQLPlusInPod(standbyPod, startApplySQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	if err := shardingv1.RunSQLPlusInPod(standbyPod, verifyApplySQL, instance, r.kubeConfig, r.Log); err != nil {
		return err
	}

	return nil
}

// SetDgConnectIdentifiers handles set dg connect identifiers for the sharding database controller.
func (r *ShardingDatabaseReconciler) SetDgConnectIdentifiers(
	instance *databasev4.ShardingDatabase,
	primary databasev4.ShardSpec,
	standby databasev4.ShardSpec,
) error {

	primaryPod := primary.Name + "-0"

	primaryDbUnique := strings.ToUpper(strings.TrimSpace(primary.Name))
	standbyDbUnique := strings.ToUpper(strings.TrimSpace(standby.Name))

	primaryConnect := r.buildShardConnectIdentifier(instance, primary, primaryDbUnique)
	standbyConnect := r.buildShardConnectIdentifier(instance, standby, standbyDbUnique)

	primaryStatic := r.buildShardStaticConnectIdentifier(instance, primary, primaryDbUnique)
	standbyStatic := r.buildShardStaticConnectIdentifier(instance, standby, standbyDbUnique)

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
dgmgrl -silent / <<'EOF'
edit database %s set property DGConnectIdentifier='%s';
edit database %s set property StaticConnectIdentifier='%s';
edit database %s set property DGConnectIdentifier='%s';
edit database %s set property StaticConnectIdentifier='%s';
show database verbose %s;
show database verbose %s;
exit
EOF
`,
		primaryDbUnique, primaryConnect,
		primaryDbUnique, primaryStatic,
		standbyDbUnique, standbyConnect,
		standbyDbUnique, standbyStatic,
		primaryDbUnique,
		standbyDbUnique,
	)}

	stdout, stderr, err := shardingv1.ExecCommand(primaryPod, cmd, r.kubeConfig, instance, r.Log)
	if err != nil {
		r.logLegacy("ERROR", "SetDgConnectIdentifiers failed stdout="+stdout+" stderr="+stderr, err, instance, r.Log)
		return err
	}

	r.logLegacy("INFO", "Set DG connect identifiers successfully", nil, instance, r.Log)
	return nil
}

// setDgBrokerStatus handles set dg broker status for the sharding database controller.
func (r *ShardingDatabaseReconciler) setDgBrokerStatus(instance *databasev4.ShardingDatabase, shardName string, val string) {
	err := r.updateStatusWithRetry(instance, func(latest *databasev4.ShardingDatabase) {
		if latest.Status.Dg.Broker == nil {
			latest.Status.Dg.Broker = map[string]string{}
		}
		latest.Status.Dg.State = instance.Status.Dg.State
		latest.Status.Dg.Broker[shardName] = val
	})
	if err != nil {
		r.logLegacy("WARNING", "setDgBrokerStatus update failed for "+shardName+": "+err.Error(), err, instance, r.Log)
	}
}

// findPrimaryForStandby handles find primary for standby for the sharding database controller.
func (r *ShardingDatabaseReconciler) findPrimaryForStandby(instance *databasev4.ShardingDatabase, standby databasev4.ShardSpec) (*databasev4.ShardSpec, error) {
	shardingType := effectiveShardingTypeForConstraints(instance)
	if shardingType == "SYSTEM" && shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" {
		standbyGroup := normalizeShardGroupKey(standby.ShardGroup)
		if standbyGroup == "" {
			return nil, fmt.Errorf("standby %s must set shardGroup in DG replication mode", standby.Name)
		}

		primaryGroup, primaries, err := systemPrimaryShardsSorted(instance, r)
		if err != nil {
			return nil, err
		}
		if standbyGroup == primaryGroup {
			return nil, fmt.Errorf("standby %s cannot belong to PRIMARY shardGroup %s", standby.Name, primaryGroup)
		}

		standbys := systemStandbysInGroupSorted(instance, standbyGroup, r)
		pos := -1
		for i := range standbys {
			if strings.EqualFold(strings.TrimSpace(standbys[i].Name), strings.TrimSpace(standby.Name)) {
				pos = i
				break
			}
		}
		if pos < 0 {
			return nil, fmt.Errorf("standby %s not found in shardGroup %s", standby.Name, standbyGroup)
		}
		if pos >= len(primaries) {
			return nil, fmt.Errorf("no PRIMARY database mapping available for standby %s in shardGroup %s", standby.Name, standbyGroup)
		}
		return primaries[pos], nil
	}

	if shardingType == "USER" && shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" {
		ss := normalizeShardSpaceKey(standby.ShardSpace)
		if ss == "" {
			return nil, fmt.Errorf("standby %s must set shardSpace in USER sharding DG mode", standby.Name)
		}
		for i := range instance.Spec.Shard {
			s := &instance.Spec.Shard[i]
			if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") && normalizeShardSpaceKey(s.ShardSpace) == ss {
				return s, nil
			}
		}
		return nil, fmt.Errorf("no PRIMARY shard found in shardSpace %s for standby %s", ss, standby.Name)
	}
	if shardingType == "COMPOSITE" && shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" {
		spaceKey := normalizeShardSpaceKey(standby.ShardSpace)
		groupKey := normalizeShardGroupKey(standby.ShardGroup)
		if spaceKey == "" {
			return nil, fmt.Errorf("standby %s must set shardSpace in COMPOSITE sharding DG mode", standby.Name)
		}
		if groupKey == "" {
			return nil, fmt.Errorf("standby %s must set shardGroup in COMPOSITE sharding DG mode", standby.Name)
		}

		primaries := compositePrimariesInShardSpaceSorted(instance, spaceKey, r)
		if len(primaries) == 0 {
			return nil, fmt.Errorf("no PRIMARY shard found in shardSpace %s for standby %s", spaceKey, standby.Name)
		}
		standbys := compositeStandbysInSpaceGroupSorted(instance, spaceKey, groupKey, r)
		pos := -1
		for i := range standbys {
			if strings.EqualFold(strings.TrimSpace(standbys[i].Name), strings.TrimSpace(standby.Name)) {
				pos = i
				break
			}
		}
		if pos < 0 {
			return nil, fmt.Errorf("standby %s not found in shardSpace %s and shardGroup %s", standby.Name, spaceKey, groupKey)
		}
		if pos >= len(primaries) {
			return nil, fmt.Errorf("no PRIMARY database mapping available for standby %s in shardGroup %s and shardSpace %s", standby.Name, groupKey, spaceKey)
		}
		return primaries[pos], nil
	}

	requireShardGroup := shardingType == "SYSTEM" || shardingType == "COMPOSITE"
	sg := normalizeShardGroupKey(standby.ShardGroup)
	if requireShardGroup && shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType) == "DG" && sg == "" {
		return nil, fmt.Errorf("standby %s must set shardGroup in DG replication mode", standby.Name)
	}

	// prefer same shardGroup primary
	if requireShardGroup {
		for i := range instance.Spec.Shard {
			s := &instance.Spec.Shard[i]
			if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
				if sg != "" && normalizeShardGroupKey(s.ShardGroup) == sg {
					return s, nil
				}
			}
		}
		if sg != "" {
			return nil, fmt.Errorf("no PRIMARY shard found in shardGroup %s for standby %s", sg, standby.Name)
		}
	}

	// fallback: first primary
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			return s, nil
		}
	}

	return nil, fmt.Errorf("no PRIMARY shard found for standby %s", standby.Name)
}

func (r *ShardingDatabaseReconciler) validatePrimaryTopologyConstraint(instance *databasev4.ShardingDatabase) error {
	shardingType := effectiveShardingTypeForConstraints(instance)
	replType := shardingv1.EffectiveReplicationType(instance.Spec.ReplicationType)
	if shardingType == "USER" && replType == "NATIVE" {
		return fmt.Errorf("user-defined sharding is not supported with RAFT/NATIVE replication")
	}
	if shardingType != "SYSTEM" && shardingType != "COMPOSITE" && (shardingType != "USER" || replType != "DG") {
		return nil
	}
	if shardingType == "COMPOSITE" {
		if err := r.validateCompositeShardGroupUniqueness(instance); err != nil {
			return err
		}
	}

	if shardingType == "SYSTEM" {
		primaryGroups := map[string]int{}
		standbyGroups := map[string]int{}
		regionByGroup := map[string]string{}
		groupByRegion := map[string]string{}
		for i := range instance.Spec.Shard {
			s := instance.Spec.Shard[i]
			if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
				continue
			}
			g := normalizeShardGroupKey(s.ShardGroup)
			if g == "" {
				continue
			}
			region := strings.ToUpper(strings.TrimSpace(s.ShardRegion))
			if region != "" {
				if old, ok := regionByGroup[g]; ok && old != region {
					return fmt.Errorf("system sharding: shardGroup %s cannot span multiple regions (%s, %s)", g, old, region)
				}
				regionByGroup[g] = region
				if oldGroup, ok := groupByRegion[region]; ok && oldGroup != g {
					return fmt.Errorf("system sharding: region %s is already used by shardGroup %s", region, oldGroup)
				}
				groupByRegion[region] = g
			}

			deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
			switch deployAs {
			case "PRIMARY":
				primaryGroups[g]++
			case "STANDBY", "ACTIVE_STANDBY":
				standbyGroups[g]++
			}
		}

		if len(primaryGroups) != 1 {
			groups := make([]string, 0, len(primaryGroups))
			for g := range primaryGroups {
				groups = append(groups, g)
			}
			slices.Sort(groups)
			return fmt.Errorf("system sharding requires exactly one PRIMARY shardgroup; found %d groups: %s", len(groups), strings.Join(groups, ","))
		}

		var primaryGroup string
		var primaryCount int
		for g, c := range primaryGroups {
			primaryGroup = g
			primaryCount = c
		}
		if standbyGroups[primaryGroup] > 0 {
			return fmt.Errorf("system sharding: PRIMARY shardGroup %s must not contain standby databases", primaryGroup)
		}
		for standbyGroup, standbyCount := range standbyGroups {
			if standbyGroup == primaryGroup {
				continue
			}
			if standbyCount > primaryCount {
				return fmt.Errorf("system sharding: standby shardGroup %s has %d standby databases but primary shardGroup %s has only %d primary databases", standbyGroup, standbyCount, primaryGroup, primaryCount)
			}
		}

		return nil
	}
	if shardingType == "COMPOSITE" && replType == "DG" {
		primaryGroupsBySpace := map[string]map[string]bool{}
		primaryReplicaCountBySpace := map[string]int{}
		standbyReplicaCountBySpaceGroup := map[string]map[string]int{}
		for i := range instance.Spec.Shard {
			s := instance.Spec.Shard[i]
			if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
				continue
			}
			spaceKey := normalizeShardSpaceKey(s.ShardSpace)
			groupKey := normalizeShardGroupKey(s.ShardGroup)
			if spaceKey == "" || groupKey == "" {
				continue
			}
			if _, ok := primaryGroupsBySpace[spaceKey]; !ok {
				primaryGroupsBySpace[spaceKey] = map[string]bool{}
			}
			if _, ok := standbyReplicaCountBySpaceGroup[spaceKey]; !ok {
				standbyReplicaCountBySpaceGroup[spaceKey] = map[string]int{}
			}
			deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
			switch deployAs {
			case "PRIMARY":
				primaryGroupsBySpace[spaceKey][groupKey] = true
				primaryReplicaCountBySpace[spaceKey]++
			case "STANDBY", "ACTIVE_STANDBY":
				standbyReplicaCountBySpaceGroup[spaceKey][groupKey]++
			}
		}

		for spaceKey, groups := range primaryGroupsBySpace {
			if len(groups) != 1 {
				groupNames := make([]string, 0, len(groups))
				for g := range groups {
					groupNames = append(groupNames, g)
				}
				slices.Sort(groupNames)
				return fmt.Errorf("composite sharding requires exactly one PRIMARY shardGroup per shardSpace; shardSpace %s has %d groups: %s", spaceKey, len(groups), strings.Join(groupNames, ","))
			}
			primaryCount := primaryReplicaCountBySpace[spaceKey]
			for standbyGroup, standbyCount := range standbyReplicaCountBySpaceGroup[spaceKey] {
				if standbyCount > primaryCount {
					return fmt.Errorf("composite sharding: standby shardGroup %s in shardSpace %s has %d standby databases but primary shardGroup has only %d primary databases", standbyGroup, spaceKey, standbyCount, primaryCount)
				}
			}
		}
		return nil
	}
	if shardingType == "COMPOSITE" && replType == "NATIVE" {
		groupRegionBySpace := map[string]map[string]string{}
		groupByRegionBySpace := map[string]map[string]string{}
		readWriteShardCountByGroup := map[string]int{}

		for i := range instance.Spec.Shard {
			s := instance.Spec.Shard[i]
			if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
				continue
			}
			spaceKey := normalizeShardSpaceKey(s.ShardSpace)
			groupKey := normalizeShardGroupKey(s.ShardGroup)
			if spaceKey == "" || groupKey == "" {
				continue
			}
			regionKey := strings.ToUpper(strings.TrimSpace(s.ShardRegion))
			if regionKey == "" {
				return fmt.Errorf("composite sharding with NATIVE replication requires shardRegion for shard %s", strings.TrimSpace(s.Name))
			}

			if _, ok := groupRegionBySpace[spaceKey]; !ok {
				groupRegionBySpace[spaceKey] = map[string]string{}
			}
			if _, ok := groupByRegionBySpace[spaceKey]; !ok {
				groupByRegionBySpace[spaceKey] = map[string]string{}
			}
			if prevRegion, ok := groupRegionBySpace[spaceKey][groupKey]; ok && prevRegion != regionKey {
				return fmt.Errorf("composite sharding with NATIVE replication: shardGroup %s in shardSpace %s cannot span multiple regions (%s, %s)", groupKey, spaceKey, prevRegion, regionKey)
			}
			groupRegionBySpace[spaceKey][groupKey] = regionKey
			if prevGroup, ok := groupByRegionBySpace[spaceKey][regionKey]; ok && prevGroup != groupKey {
				return fmt.Errorf("composite sharding with NATIVE replication: region %s in shardSpace %s is already used by shardGroup %s", regionKey, spaceKey, prevGroup)
			}
			groupByRegionBySpace[spaceKey][regionKey] = groupKey

			ruMode := r.resolveCompositeNativeShardGroupRuMode(instance, groupKey, spaceKey)
			if ruMode == "" {
				return fmt.Errorf("composite sharding with NATIVE replication requires ru_mode for shardGroup %s in shardSpace %s", groupKey, spaceKey)
			}
			if ruMode == "READWRITE" {
				readWriteShardCountByGroup[groupKey]++
			}
		}
		for groupKey, rwCount := range readWriteShardCountByGroup {
			if rwCount > 1 {
				return fmt.Errorf("composite sharding with NATIVE replication: shardGroup %s allows at most one READWRITE database; found %d", groupKey, rwCount)
			}
		}
		return nil
	}

	spacePrimaryCount := map[string]int{}
	spaceSeen := map[string]bool{}
	spaceExternalPrimary := map[string]bool{}
	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		if info.ShardSpaceDetails == nil {
			continue
		}
		ss := normalizeShardSpaceKey(info.ShardSpaceDetails.Name)
		if ss == "" {
			continue
		}
		if standbyConfigPrimaryCountController(info.StandbyConfig) > 0 {
			spaceExternalPrimary[ss] = true
		}
	}
	for i := range instance.Spec.Shard {
		s := instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		ss := normalizeShardSpaceKey(s.ShardSpace)
		if ss == "" {
			continue
		}
		deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
		spaceSeen[ss] = true
		if deployAs == "PRIMARY" {
			spacePrimaryCount[ss]++
		}
	}

	for ss := range spaceSeen {
		cnt := spacePrimaryCount[ss]
		if cnt > 1 {
			return fmt.Errorf("user sharding DG allows at most one PRIMARY shard per shardSpace; shardSpace %s has %d", ss, cnt)
		}
		if spaceExternalPrimary[ss] {
			if cnt > 0 {
				return fmt.Errorf("user sharding shardSpace %s uses standbyConfig primary source; do not set local deployAs=PRIMARY", ss)
			}
			continue
		}
		if cnt == 0 {
			return fmt.Errorf("user sharding DG requires exactly one PRIMARY shard per shardSpace; shardSpace %s has none", ss)
		}
	}
	return nil
}

func effectiveShardingTypeForConstraints(instance *databasev4.ShardingDatabase) string {
	if instance == nil {
		return "SYSTEM"
	}
	if v := strings.ToUpper(strings.TrimSpace(instance.Spec.ShardingType)); v == "SYSTEM" || v == "USER" || v == "COMPOSITE" {
		return v
	}

	hasGroup := false
	hasSpace := false
	for i := range instance.Spec.Shard {
		if strings.TrimSpace(instance.Spec.Shard[i].ShardGroup) != "" {
			hasGroup = true
		}
		if strings.TrimSpace(instance.Spec.Shard[i].ShardSpace) != "" {
			hasSpace = true
		}
	}
	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		if info.ShardGroupDetails != nil && strings.TrimSpace(info.ShardGroupDetails.Name) != "" {
			hasGroup = true
		}
		if info.ShardSpaceDetails != nil && strings.TrimSpace(info.ShardSpaceDetails.Name) != "" {
			hasSpace = true
		}
	}

	switch {
	case hasGroup && hasSpace:
		return "COMPOSITE"
	case hasSpace:
		return "USER"
	default:
		return "SYSTEM"
	}
}

func (r *ShardingDatabaseReconciler) resolvePrimaryForStandbyDG(
	instance *databasev4.ShardingDatabase,
	standby databasev4.ShardSpec,
) (*databasev4.ShardSpec, []string, string, error) {
	if pair := r.findDgPairForStandby(instance, standby.Name); pair != nil {
		source := strings.TrimSpace(pair.PrimarySource)
		connect := strings.TrimSpace(pair.PrimaryConnectString)

		if p := r.findPrimaryByPair(instance, *pair); p != nil {
			connects := []string{}
			if connect != "" {
				connects = append(connects, connect)
			}
			connects = append(connects, r.buildShardConnectIdentifier(instance, *p, strings.ToUpper(strings.TrimSpace(p.Name))))
			return p, uniqueNonEmpty(connects), source, nil
		}

		if connect != "" {
			return nil, []string{connect}, source, nil
		}

		return nil, nil, source, fmt.Errorf("dg pair for standby %s has no usable primary mapping", standby.Name)
	}

	p, err := r.findPrimaryForStandby(instance, standby)
	if err != nil {
		return nil, nil, "", err
	}
	if p == nil {
		return nil, nil, "", fmt.Errorf("primary is nil for standby %s", standby.Name)
	}
	connect := r.buildShardConnectIdentifier(instance, *p, strings.ToUpper(strings.TrimSpace(p.Name)))
	return p, []string{connect}, "SpecFallback", nil
}

func (r *ShardingDatabaseReconciler) findDgPairForStandby(instance *databasev4.ShardingDatabase, standbyName string) *databasev4.DgPairStatus {
	target := strings.TrimSpace(standbyName)
	if target == "" {
		return nil
	}
	for i := range instance.Status.Dg.Pairs {
		p := &instance.Status.Dg.Pairs[i]
		if strings.TrimSpace(p.StandbyShardName) == target {
			return p
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findPrimaryByPair(instance *databasev4.ShardingDatabase, pair databasev4.DgPairStatus) *databasev4.ShardSpec {
	if p := r.findPrimaryByPairKey(instance, pair.PrimaryKey); p != nil {
		return p
	}
	if p := r.findPrimaryByConnectString(instance, pair.PrimaryConnectString); p != nil {
		return p
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findPrimaryByPairKey(instance *databasev4.ShardingDatabase, key string) *databasev4.ShardSpec {
	raw := strings.TrimSpace(key)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		raw = strings.TrimSpace(parts[len(parts)-1])
	}
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") && strings.EqualFold(strings.TrimSpace(s.Name), raw) {
			return s
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findPrimaryByConnectString(instance *databasev4.ShardingDatabase, connect string) *databasev4.ShardSpec {
	target := strings.TrimSpace(connect)
	if target == "" {
		return nil
	}
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if !strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(s.Connect), target) {
			return s
		}
		dbu := strings.ToUpper(strings.TrimSpace(s.Name))
		if strings.EqualFold(r.buildShardConnectIdentifier(instance, *s, dbu), target) {
			return s
		}
	}
	return nil
}

func uniqueNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for i := range in {
		v := strings.TrimSpace(in[i])
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

func normalizeShardGroupKey(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func normalizeShardSpaceKey(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func normalizeShardGroupRuModeKey(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "READWRITE":
		return "READWRITE"
	case "READONLY":
		return "READONLY"
	default:
		return ""
	}
}

func (r *ShardingDatabaseReconciler) resolveCompositeNativeShardGroupRuMode(instance *databasev4.ShardingDatabase, groupKey, spaceKey string) string {
	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		if info.ShardGroupDetails == nil || info.ShardSpaceDetails == nil {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(info.ShardGroupDetails.IsDelete, instance, r.Log) {
			continue
		}
		if normalizeShardGroupKey(info.ShardGroupDetails.Name) != groupKey {
			continue
		}
		if normalizeShardSpaceKey(info.ShardSpaceDetails.Name) != spaceKey {
			continue
		}
		if ru := normalizeShardGroupRuModeKey(info.ShardGroupDetails.RuMode); ru != "" {
			return ru
		}
	}
	for i := range instance.Spec.ShardGroup {
		sg := instance.Spec.ShardGroup[i]
		if normalizeShardGroupKey(sg.Name) != groupKey {
			continue
		}
		sgSpace := normalizeShardSpaceKey(sg.ShardSpace)
		if sgSpace != "" && sgSpace != spaceKey {
			continue
		}
		if ru := normalizeShardGroupRuModeKey(sg.RuMode); ru != "" {
			return ru
		}
	}
	return ""
}

func (r *ShardingDatabaseReconciler) findShardGroupSpecByName(instance *databasev4.ShardingDatabase, groupName string) *databasev4.ShardGroupSpec {
	key := normalizeShardGroupKey(groupName)
	if key == "" {
		return nil
	}
	for i := range instance.Spec.ShardGroup {
		if normalizeShardGroupKey(instance.Spec.ShardGroup[i].Name) == key {
			return &instance.Spec.ShardGroup[i]
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findShardSpaceSpecByName(instance *databasev4.ShardingDatabase, spaceName string) *databasev4.ShardSpaceSpec {
	key := normalizeShardSpaceKey(spaceName)
	if key == "" {
		return nil
	}
	for i := range instance.Spec.ShardSpace {
		if normalizeShardSpaceKey(instance.Spec.ShardSpace[i].Name) == key {
			return &instance.Spec.ShardSpace[i]
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findShardGroupDetailsFromShardInfo(instance *databasev4.ShardingDatabase, groupName string) *databasev4.ShardGroupSpec {
	key := normalizeShardGroupKey(groupName)
	if key == "" {
		return nil
	}
	for i := range instance.Spec.ShardInfo {
		d := instance.Spec.ShardInfo[i].ShardGroupDetails
		if d == nil {
			continue
		}
		if normalizeShardGroupKey(d.Name) == key {
			return d
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) findShardSpaceDetailsFromShardInfo(instance *databasev4.ShardingDatabase, spaceName string) *databasev4.ShardSpaceSpec {
	key := normalizeShardSpaceKey(spaceName)
	if key == "" {
		return nil
	}
	for i := range instance.Spec.ShardInfo {
		d := instance.Spec.ShardInfo[i].ShardSpaceDetails
		if d == nil {
			continue
		}
		if normalizeShardSpaceKey(d.Name) == key {
			return d
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) buildAddShardGroupParamsFromSpec(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) (string, bool) {
	groupName := strings.TrimSpace(shard.ShardGroup)
	if groupName == "" {
		return "", false
	}

	groupSpec := r.findShardGroupSpecByName(instance, groupName)
	groupRegion := strings.TrimSpace(shard.ShardRegion)
	deployAs := strings.TrimSpace(shard.DeployAs)
	shardSpace := strings.TrimSpace(shard.ShardSpace)
	repFactor := int32(0)
	if groupSpec != nil {
		if groupRegion == "" {
			groupRegion = strings.TrimSpace(groupSpec.Region)
		}
		if deployAs == "" {
			deployAs = strings.TrimSpace(groupSpec.DeployAs)
		}
		if shardSpace == "" {
			shardSpace = strings.TrimSpace(groupSpec.ShardSpace)
		}
		repFactor = groupSpec.RepFactor
	} else if infoGroupSpec := r.findShardGroupDetailsFromShardInfo(instance, groupName); infoGroupSpec != nil {
		if groupRegion == "" {
			groupRegion = strings.TrimSpace(infoGroupSpec.Region)
		}
		if deployAs == "" {
			deployAs = strings.TrimSpace(infoGroupSpec.DeployAs)
		}
		if shardSpace == "" {
			shardSpace = strings.TrimSpace(infoGroupSpec.ShardSpace)
		}
		repFactor = infoGroupSpec.RepFactor
	}
	if groupRegion == "" {
		return "", false
	}

	parts := []string{
		"group_name=" + groupName,
		"group_region=" + groupRegion,
	}
	if shardSpace != "" {
		parts = append(parts, "shardspace="+shardSpace)
	}
	if deployAs != "" {
		parts = append(parts, "deploy_as="+strings.ToLower(deployAs))
	}
	if repFactor > 0 {
		parts = append(parts, "repfactor="+fmt.Sprint(repFactor))
	}
	return strings.Join(parts, ";"), true
}

func (r *ShardingDatabaseReconciler) buildAddShardSpaceParamsFromSpec(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) (string, bool) {
	spaceName := strings.TrimSpace(shard.ShardSpace)
	if spaceName == "" {
		if groupSpec := r.findShardGroupSpecByName(instance, shard.ShardGroup); groupSpec != nil {
			spaceName = strings.TrimSpace(groupSpec.ShardSpace)
		} else if infoGroupSpec := r.findShardGroupDetailsFromShardInfo(instance, shard.ShardGroup); infoGroupSpec != nil {
			spaceName = strings.TrimSpace(infoGroupSpec.ShardSpace)
		}
	}
	if spaceName == "" {
		return "", false
	}

	spaceSpec := r.findShardSpaceSpecByName(instance, spaceName)
	if spaceSpec == nil {
		spaceSpec = r.findShardSpaceDetailsFromShardInfo(instance, spaceName)
	}
	parts := []string{"sspace_name=" + spaceName}
	if spaceSpec != nil {
		if spaceSpec.Chunks > 0 {
			parts = append(parts, "chunks="+fmt.Sprint(spaceSpec.Chunks))
		}
		if spaceSpec.RepFactor > 0 {
			parts = append(parts, "repfactor="+fmt.Sprint(spaceSpec.RepFactor))
		}
		if spaceSpec.RepUnits > 0 {
			parts = append(parts, "repunits="+fmt.Sprint(spaceSpec.RepUnits))
		}
		if v := strings.TrimSpace(spaceSpec.ProtectMode); v != "" {
			parts = append(parts, "protectedmode="+v)
		}
	}
	return strings.Join(parts, ";"), true
}

func (r *ShardingDatabaseReconciler) ensureShardAddPrerequisitesInGsm(instance *databasev4.ShardingDatabase, gsmPodName string, shard databasev4.ShardSpec) error {
	mode := resolveShardMode(instance, shard)
	if mode == "SYSTEM" || mode == "COMPOSITE" {
		if groupParams, ok := r.buildAddShardGroupParamsFromSpec(instance, shard); ok {
			r.logLegacy("INFO", "Ensuring shardgroup exists with CR values before add-shard: "+groupParams, nil, instance, r.Log)
			if err := shardingv1.AddShardGroupInGsm(gsmPodName, groupParams, instance, r.kubeConfig, r.Log); err != nil {
				return fmt.Errorf("failed to ensure shardgroup for shard %s: %w", shard.Name, err)
			}
		}
	}

	if mode == "USER" || mode == "COMPOSITE" {
		if spaceParams, ok := r.buildAddShardSpaceParamsFromSpec(instance, shard); ok {
			r.logLegacy("INFO", "Ensuring shardspace exists with CR values before add-shard: "+spaceParams, nil, instance, r.Log)
			if err := shardingv1.AddShardSpaceInGsm(gsmPodName, spaceParams, instance, r.kubeConfig, r.Log); err != nil {
				return fmt.Errorf("failed to ensure shardspace for shard %s: %w", shard.Name, err)
			}
		}
	}

	return nil
}

func (r *ShardingDatabaseReconciler) validateCompositeShardGroupUniqueness(instance *databasev4.ShardingDatabase) error {
	groupToSpaceFromShard := map[string]string{}
	for i := range instance.Spec.Shard {
		s := instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		groupKey := normalizeShardGroupKey(s.ShardGroup)
		spaceKey := normalizeShardSpaceKey(s.ShardSpace)
		if groupKey == "" || spaceKey == "" {
			continue
		}
		if prev, ok := groupToSpaceFromShard[groupKey]; ok && prev != spaceKey {
			return fmt.Errorf(
				"composite sharding: shardGroup %s is used in multiple shardSpaces (%s, %s); shardGroup names must be unique across shardSpaces",
				groupKey, prev, spaceKey,
			)
		}
		groupToSpaceFromShard[groupKey] = spaceKey
	}

	groupToSpaceFromShardInfo := map[string]string{}
	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		if info.ShardGroupDetails == nil || info.ShardSpaceDetails == nil {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(info.ShardGroupDetails.IsDelete, instance, r.Log) {
			continue
		}
		groupKey := normalizeShardGroupKey(info.ShardGroupDetails.Name)
		spaceKey := normalizeShardSpaceKey(info.ShardSpaceDetails.Name)
		if groupKey == "" || spaceKey == "" {
			continue
		}
		if prev, ok := groupToSpaceFromShardInfo[groupKey]; ok && prev != spaceKey {
			return fmt.Errorf(
				"composite sharding: shardInfo shardGroup %s is used in multiple shardSpaces (%s, %s); shardGroup names must be unique across shardSpaces",
				groupKey, prev, spaceKey,
			)
		}
		groupToSpaceFromShardInfo[groupKey] = spaceKey
	}

	return nil
}

func systemPrimaryShardsSorted(instance *databasev4.ShardingDatabase, r *ShardingDatabaseReconciler) (string, []*databasev4.ShardSpec, error) {
	primaryGroups := map[string][]*databasev4.ShardSpec{}
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			continue
		}
		g := normalizeShardGroupKey(s.ShardGroup)
		if g == "" {
			continue
		}
		primaryGroups[g] = append(primaryGroups[g], s)
	}
	if len(primaryGroups) != 1 {
		groups := make([]string, 0, len(primaryGroups))
		for g := range primaryGroups {
			groups = append(groups, g)
		}
		slices.Sort(groups)
		return "", nil, fmt.Errorf("system sharding requires exactly one PRIMARY shardgroup; found %d groups: %s", len(groups), strings.Join(groups, ","))
	}
	for group, primaries := range primaryGroups {
		sort.Slice(primaries, func(i, j int) bool {
			return strings.ToUpper(strings.TrimSpace(primaries[i].Name)) < strings.ToUpper(strings.TrimSpace(primaries[j].Name))
		})
		return group, primaries, nil
	}
	return "", nil, fmt.Errorf("system sharding requires exactly one PRIMARY shardgroup")
}

func systemStandbysInGroupSorted(instance *databasev4.ShardingDatabase, standbyGroup string, r *ShardingDatabaseReconciler) []*databasev4.ShardSpec {
	out := make([]*databasev4.ShardSpec, 0)
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		if normalizeShardGroupKey(s.ShardGroup) != standbyGroup {
			continue
		}
		deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
		if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToUpper(strings.TrimSpace(out[i].Name)) < strings.ToUpper(strings.TrimSpace(out[j].Name))
	})
	return out
}

func compositePrimariesInShardSpaceSorted(instance *databasev4.ShardingDatabase, spaceKey string, r *ShardingDatabaseReconciler) []*databasev4.ShardSpec {
	out := make([]*databasev4.ShardSpec, 0)
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		if normalizeShardSpaceKey(s.ShardSpace) != spaceKey {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToUpper(strings.TrimSpace(out[i].Name)) < strings.ToUpper(strings.TrimSpace(out[j].Name))
	})
	return out
}

func compositeStandbysInSpaceGroupSorted(instance *databasev4.ShardingDatabase, spaceKey, groupKey string, r *ShardingDatabaseReconciler) []*databasev4.ShardSpec {
	out := make([]*databasev4.ShardSpec, 0)
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(s.IsDelete, instance, r.Log) {
			continue
		}
		if normalizeShardSpaceKey(s.ShardSpace) != spaceKey || normalizeShardGroupKey(s.ShardGroup) != groupKey {
			continue
		}
		deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
		if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToUpper(strings.TrimSpace(out[i].Name)) < strings.ToUpper(strings.TrimSpace(out[j].Name))
	})
	return out
}

func (r *ShardingDatabaseReconciler) buildDgConfigName(
	instance *databasev4.ShardingDatabase,
	primary databasev4.ShardSpec,
	standby databasev4.ShardSpec,
) string {
	parts := []string{
		strings.ToUpper(strings.TrimSpace(instance.Name)),
		strings.ToUpper(strings.TrimSpace(primary.Name)),
	}
	if sg := normalizeShardGroupKey(standby.ShardGroup); sg != "" {
		parts = append(parts, sg)
	}
	parts = append(parts, "DGCFG")
	return strings.Join(parts, "_")
}

func (r *ShardingDatabaseReconciler) collectStandbysForPrimary(instance *databasev4.ShardingDatabase, primary databasev4.ShardSpec) []databasev4.ShardSpec {
	out := []databasev4.ShardSpec{}
	primaryName := strings.TrimSpace(primary.Name)
	for i := range instance.Spec.Shard {
		s := instance.Spec.Shard[i]
		deployAs := strings.ToUpper(strings.TrimSpace(s.DeployAs))
		if deployAs != "STANDBY" && deployAs != "ACTIVE_STANDBY" {
			continue
		}
		p, err := r.findPrimaryForStandby(instance, s)
		if err != nil || p == nil {
			continue
		}
		if strings.TrimSpace(p.Name) == primaryName {
			out = append(out, s)
		}
	}
	return out
}

func (r *ShardingDatabaseReconciler) buildShardConnectIdentifier(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec, dbUniqueName string) string {
	host, port := r.resolveShardHostPort(instance, shard)
	return fmt.Sprintf("//%s:%d/%s", host, port, shardingv1.BuildDgmgrlServiceName(dbUniqueName))
}

func (r *ShardingDatabaseReconciler) buildShardStaticConnectIdentifier(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec, dbUniqueName string) string {
	host, port := r.resolveShardHostPort(instance, shard)
	svc := shardingv1.BuildDgmgrlServiceName(dbUniqueName)
	inst := strings.ToUpper(strings.TrimSpace(dbUniqueName))
	return fmt.Sprintf(
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=%s)(PORT=%d))(CONNECT_DATA=(SERVICE_NAME=%s)(INSTANCE_NAME=%s)(SERVER=DEDICATED)))",
		host, port, svc, inst,
	)
}

func (r *ShardingDatabaseReconciler) resolveShardHostPort(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) (string, int32) {
	if host, port, ok := parseConnectHostPort(shard.Connect); ok {
		return host, port
	}
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shard.Name, shard.Name, instance.Namespace)
	return host, 1521
}

func parseConnectHostPort(connect string) (string, int32, bool) {
	raw := strings.TrimSpace(connect)
	if raw == "" {
		return "", 0, false
	}

	s := strings.TrimPrefix(raw, "//")
	slash := strings.Index(s, "/")
	if slash <= 0 {
		return "", 0, false
	}
	hostPort := s[:slash]

	host := hostPort
	port := int32(1521)
	if strings.Contains(hostPort, ":") {
		idx := strings.LastIndex(hostPort, ":")
		h := strings.TrimSpace(hostPort[:idx])
		p := strings.TrimSpace(hostPort[idx+1:])
		if h == "" || p == "" {
			return "", 0, false
		}
		pi, err := strconv.Atoi(p)
		if err != nil || pi <= 0 {
			return "", 0, false
		}
		host = h
		port = int32(pi)
	}
	if strings.TrimSpace(host) == "" {
		return "", 0, false
	}
	return host, port, true
}

func (r *ShardingDatabaseReconciler) updateStatusWithRetry(instance *databasev4.ShardingDatabase, mutate func(*databasev4.ShardingDatabase)) error {
	key := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &databasev4.ShardingDatabase{}
		if err := r.Get(context.Background(), key, latest); err != nil {
			return err
		}
		mutate(latest)
		if err := r.Status().Update(context.Background(), latest); err != nil {
			return err
		}
		instance.Status = latest.Status
		return nil
	})
}

// shardOrdinal handles shard ordinal for the sharding database controller.
func shardOrdinal(name string) int {
	n := 0
	mult := 1
	foundDigit := false

	for i := len(name) - 1; i >= 0; i-- {
		if unicode.IsDigit(rune(name[i])) {
			foundDigit = true
			n += int(name[i]-'0') * mult
			mult *= 10
			continue
		}
		if foundDigit {
			return n
		}
	}
	if foundDigit {
		return n
	}
	return 0
}

// applyReplicaScaleInMarks handles apply replica scale in marks for the sharding database controller.
func (r *ShardingDatabaseReconciler) applyReplicaScaleInMarks(instance *databasev4.ShardingDatabase) bool {
	candidatesByPrefix := r.getScaleInCandidates(instance)
	if len(candidatesByPrefix) == 0 {
		return false
	}

	candidateIdx := map[int]bool{}
	for _, idxs := range candidatesByPrefix {
		for _, idx := range idxs {
			candidateIdx[idx] = true
		}
	}

	changed := false

	for i := range instance.Spec.Shard {
		shard := instance.Spec.Shard[i]
		curr := strings.ToLower(strings.TrimSpace(shard.IsDelete))

		if curr == "failed" {
			curr = "disable"
		}

		if candidateIdx[i] {
			if r.isShardReadyForScaleInDelete(instance, shard, i) {
				if curr != "enable" {
					instance.Spec.Shard[i].IsDelete = "enable"
					changed = true
					r.logLegacy(
						"INFO",
						fmt.Sprintf("Auto-marking shard %s as isDelete=enable because it is extra and ready for scale-in deletion", shard.Name),
						nil,
						instance,
						r.Log,
					)
				}
			} else {
				if curr == "" {
					instance.Spec.Shard[i].IsDelete = "disable"
					changed = true
				}
			}
			continue
		}

		if curr == "" {
			instance.Spec.Shard[i].IsDelete = "disable"
			changed = true
		}
	}

	return changed
}

// getScaleInCandidates handles get scale in candidates for the sharding database controller.
func (r *ShardingDatabaseReconciler) getScaleInCandidates(
	instance *databasev4.ShardingDatabase,
) map[string][]int {
	type shardRef struct {
		idx   int
		name  string
		order int
	}

	desiredByPrefix := map[string]int32{}
	for i := range instance.Spec.ShardInfo {
		info := instance.Spec.ShardInfo[i]
		prefix := strings.TrimSpace(info.ShardPreFixName)
		if prefix == "" {
			continue
		}
		desiredByPrefix[prefix] = shardInfoDesiredCount(instance, info)
	}

	currentByPrefix := map[string][]shardRef{}
	for i := range instance.Spec.Shard {
		s := instance.Spec.Shard[i]
		for prefix := range desiredByPrefix {
			if strings.HasPrefix(s.Name, prefix) {
				currentByPrefix[prefix] = append(currentByPrefix[prefix], shardRef{
					idx:   i,
					name:  s.Name,
					order: shardOrdinal(s.Name),
				})
				break
			}
		}
	}

	out := map[string][]int{}

	for prefix, shards := range currentByPrefix {
		sort.Slice(shards, func(i, j int) bool {
			return shards[i].order > shards[j].order
		})

		desired := desiredByPrefix[prefix]
		current := int32(len(shards))
		extra := current - desired
		if extra <= 0 {
			continue
		}

		for i := 0; i < int(extra); i++ {
			out[prefix] = append(out[prefix], shards[i].idx)
		}
	}

	return out
}

// isShardReadyForScaleInDelete handles is shard ready for scale in delete for the sharding database controller.
func (r *ShardingDatabaseReconciler) isShardReadyForScaleInDelete(
	instance *databasev4.ShardingDatabase,
	shard databasev4.ShardSpec,
	specIdx int,
) bool {
	shardSfSet, err := shardingv1.CheckSfset(shard.Name, instance, r.Client)
	if err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: StatefulSet not found for shard "+shard.Name, nil, instance, r.Log)
		return false
	}

	podList, err := shardingv1.GetPodList(shardSfSet.Name, "SHARD", instance, r.Client)
	if err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: pod list not available for shard "+shard.Name, nil, instance, r.Log)
		return false
	}

	isPodExist, _ := shardingv1.PodListValidation(podList, shardSfSet.Name, instance, r.Client)
	if !isPodExist {
		r.logLegacy("DEBUG", "Scale-in readiness: pod not ready for shard "+shard.Name, nil, instance, r.Log)
		return false
	}

	_, _, err = r.validateShard(instance, shard, specIdx)
	if err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: validateShard not ready for shard "+shard.Name+": "+err.Error(), nil, instance, r.Log)
		return false
	}

	_, gsmPod, err := r.validateGsm(instance)
	if err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: GSM not ready for shard "+shard.Name+": "+err.Error(), nil, instance, r.Log)
		return false
	}

	sparams := shardingv1.BuildShardParams(instance, shardSfSet, shard)
	if err := shardingv1.CheckShardInGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log); err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: shard not present in GSM for "+shard.Name+": "+err.Error(), nil, instance, r.Log)
		return false
	}

	if err := shardingv1.CheckOnlineShardInGsm(gsmPod.Name, sparams, instance, r.kubeConfig, r.Log); err != nil {
		r.logLegacy("DEBUG", "Scale-in readiness: shard not online in GSM for "+shard.Name+": "+err.Error(), nil, instance, r.Log)
		return false
	}

	return true
}

// shouldMoveChunksBeforeDelete handles should move chunks before delete for the sharding database controller.
func (r *ShardingDatabaseReconciler) shouldMoveChunksBeforeDelete(
	instance *databasev4.ShardingDatabase,
	shard databasev4.ShardSpec,
) bool {
	deployAs := strings.ToUpper(strings.TrimSpace(shard.DeployAs))

	if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
		return false
	}

	if shardingv1.IsNativeReplication(instance.Spec.ReplicationType) {
		return false
	}

	return true
}

// cleanupOrphanShardResources removes shard resources that are no longer declared in desired spec.
func (r *ShardingDatabaseReconciler) cleanupOrphanShardResources(instance *databasev4.ShardingDatabase) error {
	if instance == nil {
		return fmt.Errorf("cleanupOrphanShardResources: instance is nil")
	}

	latest := &databasev4.ShardingDatabase{}
	reader := r.APIReader
	if reader == nil {
		// Fallback for deployments/tests where APIReader is not injected.
		reader = r.Client
	}
	if err := reader.Get(
		context.Background(),
		types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
		latest,
	); err != nil {
		return err
	}

	desiredFromShardInfo := desiredShardNamesFromShardInfo(latest)
	specShardNames := shardNamesFromSpec(latest)

	sfList := &appsv1.StatefulSetList{}
	if err := r.Client.List(context.TODO(), sfList, client.InNamespace(latest.Namespace)); err != nil {
		return err
	}

	for i := range sfList.Items {
		sf := &sfList.Items[i]
		name := strings.TrimSpace(sf.Name)
		lbls := sf.GetLabels()
		if lbls == nil {
			continue
		}

		if lbls["type"] != "Shard" {
			continue
		}
		if lbls["oralabel"] != latest.Name {
			continue
		}

		// Keep:
		// 1. shards still desired by shardInfo
		// 2. shards still present in spec.shard
		if desiredFromShardInfo[name] || specShardNames[name] {
			continue
		}

		r.logLegacy("INFO", "Deleting orphan shard resources for "+name, nil, latest, r.Log)

		if err := r.Client.Delete(context.Background(), sf); err != nil && !errors.IsNotFound(err) {
			return err
		}

		svcFound, err := shardingv1.CheckSvc(name, latest, r.Client)
		if err == nil && svcFound != nil {
			if derr := r.Client.Delete(context.Background(), svcFound); derr != nil && !errors.IsNotFound(derr) {
				return derr
			}
		}

		if latest.Spec.IsExternalSvc {
			svcExt, err := shardingv1.CheckSvc(name+strconv.Itoa(0)+"-svc", latest, r.Client)
			if err == nil && svcExt != nil {
				if derr := r.Client.Delete(context.Background(), svcExt); derr != nil && !errors.IsNotFound(derr) {
					return derr
				}
			}
		}

		if latest.Spec.IsDeleteOraPvc && len(latest.Spec.StorageClass) > 0 {
			pvcName := name + "-oradata-vol4-" + name + "-0"
			if err := shardingv1.DelPvc(pvcName, latest, r.Client, r.Log); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func shardInfoDesiredCount(instance *databasev4.ShardingDatabase, info databasev4.ShardingDetails) int32 {
	if effectiveShardingTypeForConstraints(instance) == "USER" {
		if c := standbyConfigDerivedShardCountController(info.StandbyConfig); c > 0 {
			return c
		}
	}
	if info.ShardNum > 0 {
		return info.ShardNum
	}
	if info.Replicas > 0 {
		return info.Replicas
	}
	return 0
}

func standbyConfigDerivedShardCountController(cfg *databasev4.StandbyConfig) int32 {
	if cfg == nil {
		return 0
	}

	primaryCount := standbyConfigPrimaryCountController(cfg)
	if primaryCount == 0 {
		return 0
	}

	perPrimary := cfg.StandbyPerPrimary
	if perPrimary <= 0 {
		perPrimary = 1
	}

	return primaryCount * perPrimary
}

func standbyConfigPrimaryCountController(cfg *databasev4.StandbyConfig) int32 {
	if cfg == nil {
		return 0
	}

	if c := countUniquePrimaryDatabaseRefsController(cfg.PrimaryDatabaseRefs); c > 0 {
		return c
	}
	if c := countUniqueStringsController(cfg.PrimaryConnectStrings); c > 0 {
		return c
	}
	return countUniquePrimaryEndpointsController(cfg.PrimaryEndpoints)
}

func countUniquePrimaryDatabaseRefsController(in []databasev4.PrimaryDatabaseCRRef) int32 {
	seen := map[string]bool{}
	var count int32
	for i := range in {
		ref := in[i]
		name := strings.ToLower(strings.TrimSpace(ref.Name))
		if name == "" {
			continue
		}
		ns := strings.ToLower(strings.TrimSpace(ref.Namespace))
		key := ns + "/" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func countUniqueStringsController(in []string) int32 {
	seen := map[string]bool{}
	var count int32
	for i := range in {
		v := strings.ToLower(strings.TrimSpace(in[i]))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		count++
	}
	return count
}

func countUniquePrimaryEndpointsController(in []databasev4.PrimaryEndpointRef) int32 {
	seen := map[string]bool{}
	var count int32
	for i := range in {
		e := in[i]
		key := strings.ToLower(strings.TrimSpace(e.ConnectString))
		if key == "" {
			host := strings.ToLower(strings.TrimSpace(e.Host))
			cdb := strings.ToLower(strings.TrimSpace(e.CdbName))
			pdb := strings.ToLower(strings.TrimSpace(e.PdbName))
			if host == "" && cdb == "" && pdb == "" {
				continue
			}
			key = host + ":" + strconv.Itoa(int(e.Port)) + "/" + cdb + "/" + pdb
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func (r *ShardingDatabaseReconciler) resolveStandbyWalletSecretRef(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if ref := strings.TrimSpace(shard.StandbyConfig.TDEWallet.SecretRef); ref != "" {
				return ref
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if ref := strings.TrimSpace(info.StandbyConfig.TDEWallet.SecretRef); ref != "" {
			return ref
		}
	}
	return ""
}

func (r *ShardingDatabaseReconciler) resolveStandbyWalletZipFileKey(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) string {
	if shard.StandbyConfig != nil {
		if shard.StandbyConfig.TDEWallet != nil {
			if ref := strings.TrimSpace(shard.StandbyConfig.TDEWallet.ZipFileKey); ref != "" {
				return ref
			}
		}
	}

	info := matchShardInfoByLongestPrefix(instance, shard.Name)
	if info != nil && info.StandbyConfig != nil && info.StandbyConfig.TDEWallet != nil {
		if ref := strings.TrimSpace(info.StandbyConfig.TDEWallet.ZipFileKey); ref != "" {
			return ref
		}
	}
	return ""
}

func matchShardInfoByLongestPrefix(instance *databasev4.ShardingDatabase, shardName string) *databasev4.ShardingDetails {
	if instance == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(shardName))
	if name == "" {
		return nil
	}
	best := -1
	bestLen := -1
	for i := range instance.Spec.ShardInfo {
		prefix := strings.ToLower(strings.TrimSpace(instance.Spec.ShardInfo[i].ShardPreFixName))
		if prefix == "" || !strings.HasPrefix(name, prefix) {
			continue
		}
		if len(prefix) > bestLen {
			best = i
			bestLen = len(prefix)
		}
	}
	if best < 0 {
		return nil
	}
	return &instance.Spec.ShardInfo[best]
}

func (r *ShardingDatabaseReconciler) validateStandbyWalletSecretRef(instance *databasev4.ShardingDatabase, shard databasev4.ShardSpec) error {
	ref := r.resolveStandbyWalletSecretRef(instance, shard)
	if ref == "" {
		return nil
	}
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: instance.Namespace, Name: ref}, secret); err != nil {
		return fmt.Errorf("standbyConfig.tdeWallet.secretRef %q not found for shard %s: %w", ref, shard.Name, err)
	}
	if len(secret.Data) == 0 {
		return fmt.Errorf("standbyConfig.tdeWallet.secretRef %q for shard %s has no data", ref, shard.Name)
	}
	if zipKey := r.resolveStandbyWalletZipFileKey(instance, shard); zipKey != "" {
		if _, ok := secret.Data[zipKey]; !ok {
			return fmt.Errorf("standbyConfig.tdeWallet.zipFileKey %q not found in secret %q for shard %s", zipKey, ref, shard.Name)
		}
	}
	return nil
}

// desiredShardNamesFromShardInfo handles desired shard names from shard info for the sharding database controller.
func desiredShardNamesFromShardInfo(instance *databasev4.ShardingDatabase) map[string]bool {
	desired := map[string]bool{}

	for i := range instance.Spec.ShardInfo {
		prefix := strings.TrimSpace(instance.Spec.ShardInfo[i].ShardPreFixName)
		if prefix == "" {
			continue
		}

		replicas := shardInfoDesiredCount(instance, instance.Spec.ShardInfo[i])
		if replicas == 0 {
			replicas = 2
		}

		for j := 1; j <= int(replicas); j++ {
			desired[prefix+strconv.Itoa(j)] = true
		}
	}

	return desired
}

// shardNamesFromSpec handles shard names from spec for the sharding database controller.
func shardNamesFromSpec(instance *databasev4.ShardingDatabase) map[string]bool {
	out := map[string]bool{}

	for i := range instance.Spec.Shard {
		name := strings.TrimSpace(instance.Spec.Shard[i].Name)
		if name == "" {
			continue
		}
		out[name] = true
	}

	return out
}

var shapeManagedEnvKeys = map[string]bool{
	"INIT_SGA_SIZE":   true,
	"INIT_PGA_SIZE":   true,
	"INIT_PROCESS":    true,
	"INIT_CPU_COUNT":  true,
	"INIT_TOTAL_SIZE": true,
}

// isShapeManagedEnvName handles is shape managed env name for the sharding database controller.
func isShapeManagedEnvName(name string) bool {
	return shapeManagedEnvKeys[strings.ToUpper(strings.TrimSpace(name))]
}

// stripShapeManagedDbEnv handles strip shape managed db env for the sharding database controller.
func stripShapeManagedDbEnv(envs []databasev4.EnvironmentVariable) []databasev4.EnvironmentVariable {
	out := make([]databasev4.EnvironmentVariable, 0, len(envs))
	for _, e := range envs {
		if isShapeManagedEnvName(e.Name) {
			continue
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(out[i].Name)) < strings.ToLower(strings.TrimSpace(out[j].Name))
	})
	return out
}

// findCatalogShape handles find catalog shape for the sharding database controller.
func findCatalogShape(spec *databasev4.ShardingDatabaseSpec, name string) string {
	for i := range spec.Catalog {
		if strings.TrimSpace(spec.Catalog[i].Name) == strings.TrimSpace(name) {
			return strings.TrimSpace(spec.Catalog[i].Shape)
		}
	}
	return ""
}

// findShardShapeForName handles find shard shape for name for the sharding database controller.
func findShardShapeForName(spec *databasev4.ShardingDatabaseSpec, shardName string) string {
	name := strings.TrimSpace(shardName)
	for i := range spec.ShardInfo {
		prefix := strings.TrimSpace(spec.ShardInfo[i].ShardPreFixName)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			return strings.TrimSpace(spec.ShardInfo[i].Shape)
		}
	}
	return ""
}

// shardShapeChangedForName handles shard shape changed for name for the sharding database controller.
func shardShapeChangedForName(curr *databasev4.ShardingDatabaseSpec, old *databasev4.ShardingDatabaseSpec, shardName string) bool {
	currShape := findShardShapeForName(curr, shardName)
	oldShape := findShardShapeForName(old, shardName)

	if currShape == "" || oldShape == "" {
		return false
	}
	return !strings.EqualFold(currShape, oldShape)
}

type shapeRollTarget struct {
	kind    string
	name    string
	specIdx int
}

// extractShapeEnvCore handles extract shape env core for the sharding database controller.
func extractShapeEnvCore(envs []corev1.EnvVar) map[string]string {
	out := map[string]string{}
	for _, e := range envs {
		k := strings.ToUpper(strings.TrimSpace(e.Name))
		if shapeManagedEnvKeys[k] {
			out[k] = e.Value
		}
	}
	return out
}

// statefulSetNeedsShapeRecreate handles stateful set needs shape recreate for the sharding database controller.
func statefulSetNeedsShapeRecreate(current, desired *appsv1.StatefulSet) bool {
	if current == nil || desired == nil {
		return false
	}
	if len(current.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return false
	}

	currC := current.Spec.Template.Spec.Containers[0]
	wantC := desired.Spec.Template.Spec.Containers[0]

	if !reflect.DeepEqual(currC.Resources, wantC.Resources) {
		return true
	}

	if !reflect.DeepEqual(extractShapeEnvCore(currC.Env), extractShapeEnvCore(wantC.Env)) {
		return true
	}

	return false
}

func containerCapabilities(c corev1.Container) *corev1.Capabilities {
	if c.SecurityContext == nil {
		return nil
	}
	return c.SecurityContext.Capabilities
}

type managedPodSecurityContext struct {
	RunAsNonRoot        *bool
	RunAsUser           *int64
	RunAsGroup          *int64
	FSGroup             *int64
	FSGroupChangePolicy *corev1.PodFSGroupChangePolicy
	SupplementalGroups  []int64
	Sysctls             []corev1.Sysctl
}

func normalizeResourceRequirements(rr corev1.ResourceRequirements) corev1.ResourceRequirements {
	out := rr.DeepCopy()
	if len(out.Requests) == 0 {
		out.Requests = nil
	}
	if len(out.Limits) == 0 {
		out.Limits = nil
	}
	if len(out.Claims) == 0 {
		out.Claims = nil
	}
	return *out
}

func normalizeCapabilities(caps *corev1.Capabilities) *corev1.Capabilities {
	if caps == nil {
		return nil
	}
	out := caps.DeepCopy()
	if len(out.Add) == 0 {
		out.Add = nil
	}
	if len(out.Drop) == 0 {
		out.Drop = nil
	}
	sort.Slice(out.Add, func(i, j int) bool {
		return out.Add[i] < out.Add[j]
	})
	sort.Slice(out.Drop, func(i, j int) bool {
		return out.Drop[i] < out.Drop[j]
	})
	return out
}

func normalizeManagedPodSecurityContext(sc *corev1.PodSecurityContext) *managedPodSecurityContext {
	if sc == nil {
		return nil
	}
	out := &managedPodSecurityContext{}
	if sc.RunAsNonRoot != nil {
		v := *sc.RunAsNonRoot
		out.RunAsNonRoot = &v
	}
	if sc.RunAsUser != nil {
		v := *sc.RunAsUser
		out.RunAsUser = &v
	}
	if sc.RunAsGroup != nil {
		v := *sc.RunAsGroup
		out.RunAsGroup = &v
	}
	if sc.FSGroup != nil {
		v := *sc.FSGroup
		out.FSGroup = &v
	}
	if sc.FSGroupChangePolicy != nil {
		v := *sc.FSGroupChangePolicy
		out.FSGroupChangePolicy = &v
	}
	if len(sc.SupplementalGroups) > 0 {
		out.SupplementalGroups = append([]int64(nil), sc.SupplementalGroups...)
		slices.Sort(out.SupplementalGroups)
	}
	if len(sc.Sysctls) > 0 {
		out.Sysctls = append([]corev1.Sysctl(nil), sc.Sysctls...)
		sort.Slice(out.Sysctls, func(i, j int) bool {
			if out.Sysctls[i].Name == out.Sysctls[j].Name {
				return out.Sysctls[i].Value < out.Sysctls[j].Value
			}
			return out.Sysctls[i].Name < out.Sysctls[j].Name
		})
	}
	return out
}

func nonShapeShardTemplateDriftReasons(current, desired *appsv1.StatefulSet) []string {
	if current == nil || desired == nil {
		return nil
	}

	reasons := make([]string, 0, 3)
	if !reflect.DeepEqual(
		normalizeManagedPodSecurityContext(current.Spec.Template.Spec.SecurityContext),
		normalizeManagedPodSecurityContext(desired.Spec.Template.Spec.SecurityContext),
	) {
		reasons = append(reasons, "securityContext")
	}

	desiredByName := map[string]corev1.Container{}
	for _, c := range desired.Spec.Template.Spec.Containers {
		desiredByName[c.Name] = c
	}

	resourcesDrift := false
	capabilitiesDrift := false
	for _, curr := range current.Spec.Template.Spec.Containers {
		want, ok := desiredByName[curr.Name]
		if !ok {
			continue
		}
		if !resourcesDrift && !reflect.DeepEqual(
			normalizeResourceRequirements(curr.Resources),
			normalizeResourceRequirements(want.Resources),
		) {
			resourcesDrift = true
		}
		if !capabilitiesDrift && !reflect.DeepEqual(
			normalizeCapabilities(containerCapabilities(curr)),
			normalizeCapabilities(containerCapabilities(want)),
		) {
			capabilitiesDrift = true
		}
		if resourcesDrift && capabilitiesDrift {
			break
		}
	}
	if resourcesDrift {
		reasons = append(reasons, "resources")
	}
	if capabilitiesDrift {
		reasons = append(reasons, "capabilities")
	}
	return reasons
}

// statefulSetNeedsNonShapeShardTemplateRecreate detects shard pod template drift
// for non-shape fields that require a shard pod restart.
func statefulSetNeedsNonShapeShardTemplateRecreate(current, desired *appsv1.StatefulSet) bool {
	return len(nonShapeShardTemplateDriftReasons(current, desired)) > 0
}

func shapeTargetKeySet(targets []shapeRollTarget) map[string]bool {
	out := map[string]bool{}
	for _, t := range targets {
		kind := strings.ToUpper(strings.TrimSpace(t.kind))
		name := strings.TrimSpace(t.name)
		if name == "" {
			continue
		}
		out[kind+"|"+name] = true
	}
	return out
}

func orderedNonShapeTemplateRollTargets(instance *databasev4.ShardingDatabase, skipKeys map[string]bool) []shapeRollTarget {
	targets := make([]shapeRollTarget, 0, len(instance.Spec.Catalog)+len(instance.Spec.Gsm)+len(instance.Spec.Shard))

	// Catalog first, in spec order.
	for i := range instance.Spec.Catalog {
		name := strings.TrimSpace(instance.Spec.Catalog[i].Name)
		if name == "" {
			continue
		}
		if skipKeys["CATALOG|"+name] {
			continue
		}
		targets = append(targets, shapeRollTarget{
			kind:    "CATALOG",
			name:    name,
			specIdx: i,
		})
	}

	// GSM next, in spec order.
	for i := range instance.Spec.Gsm {
		name := strings.TrimSpace(instance.Spec.Gsm[i].Name)
		if name == "" {
			continue
		}
		if skipKeys["GSM|"+name] {
			continue
		}
		targets = append(targets, shapeRollTarget{
			kind:    "GSM",
			name:    name,
			specIdx: i,
		})
	}

	// Shards last, ordered by shard ordinal.
	shardTargets := make([]shapeRollTarget, 0, len(instance.Spec.Shard))
	for i := range instance.Spec.Shard {
		sh := instance.Spec.Shard[i]
		name := strings.TrimSpace(sh.Name)
		if name == "" {
			continue
		}
		if shardingv1.CheckIsDeleteFlag(sh.IsDelete, instance, logr.Discard()) {
			continue
		}
		if skipKeys["SHARD|"+name] {
			continue
		}
		shardTargets = append(shardTargets, shapeRollTarget{
			kind:    "SHARD",
			name:    name,
			specIdx: i,
		})
	}

	sort.Slice(shardTargets, func(i, j int) bool {
		return shardOrdinal(shardTargets[i].name) < shardOrdinal(shardTargets[j].name)
	})

	targets = append(targets, shardTargets...)
	return targets
}

// desiredShapeForTarget handles desired shape for target for the sharding database controller.
func desiredShapeForTarget(instance *databasev4.ShardingDatabase, t shapeRollTarget) string {
	switch t.kind {
	case "CATALOG":
		return findCatalogShape(&instance.Spec, t.name)
	case "SHARD":
		return findShardShapeForName(&instance.Spec, t.name)
	default:
		return ""
	}
}

// desiredCfgForTarget handles desired cfg for target for the sharding database controller.
func desiredCfgForTarget(instance *databasev4.ShardingDatabase, t shapeRollTarget) (shapes.ShapeConfig, bool) {
	shape := desiredShapeForTarget(instance, t)
	if shape == "" {
		return shapes.ShapeConfig{}, false
	}
	return shapes.LookupShapeConfig(shape)
}

// orderedShapeChangeTargets handles ordered shape change targets for the sharding database controller.
func (r *ShardingDatabaseReconciler) orderedShapeChangeTargets(instance *databasev4.ShardingDatabase) ([]shapeRollTarget, error) {
	lastSuccSpec, err := instance.GetLastSuccessfulSpec()
	if err != nil {
		return nil, nil
	}
	if lastSuccSpec == nil {
		return nil, nil
	}

	targets := make([]shapeRollTarget, 0)

	// catalog first, in spec order
	for i := range instance.Spec.Catalog {
		curr := instance.Spec.Catalog[i]
		for j := range lastSuccSpec.Catalog {
			old := lastSuccSpec.Catalog[j]
			if strings.TrimSpace(curr.Name) != strings.TrimSpace(old.Name) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(curr.Shape), strings.TrimSpace(old.Shape)) {
				targets = append(targets, shapeRollTarget{
					kind:    "CATALOG",
					name:    curr.Name,
					specIdx: i,
				})
			}
			break
		}
	}

	// then shards one by one in ordinal order
	shardTargets := make([]shapeRollTarget, 0)
	for i := range instance.Spec.Shard {
		curr := instance.Spec.Shard[i]
		if shardingv1.CheckIsDeleteFlag(curr.IsDelete, instance, r.Log) {
			continue
		}
		if shardShapeChangedForName(&instance.Spec, lastSuccSpec, curr.Name) {
			shardTargets = append(shardTargets, shapeRollTarget{
				kind:    "SHARD",
				name:    curr.Name,
				specIdx: i,
			})
		}
	}

	sort.Slice(shardTargets, func(i, j int) bool {
		return shardOrdinal(shardTargets[i].name) < shardOrdinal(shardTargets[j].name)
	})

	targets = append(targets, shardTargets...)
	return targets, nil
}

// validateShapeTargetReady handles validate shape target ready for the sharding database controller.
func (r *ShardingDatabaseReconciler) validateShapeTargetReady(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
) error {
	switch t.kind {
	case "CATALOG":
		_, _, err := r.validateInvidualCatalog(instance, instance.Spec.Catalog[t.specIdx], t.specIdx)
		return err
	case "GSM":
		_, _, err := r.validateInvidualGsm(instance, instance.Spec.Gsm[t.specIdx], t.specIdx)
		return err
	case "SHARD":
		_, _, err := r.validateShard(instance, instance.Spec.Shard[t.specIdx], t.specIdx)
		return err
	default:
		return fmt.Errorf("unknown target kind %s", t.kind)
	}
}

// desiredStatefulSetForShapeTarget handles desired stateful set for shape target for the sharding database controller.
func (r *ShardingDatabaseReconciler) desiredStatefulSetForShapeTarget(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
) (*appsv1.StatefulSet, error) {
	switch t.kind {
	case "CATALOG":
		return shardingv1.BuildStatefulSetForCatalog(instance, instance.Spec.Catalog[t.specIdx])
	case "GSM":
		return shardingv1.BuildStatefulSetForGsm(instance, instance.Spec.Gsm[t.specIdx]), nil
	case "SHARD":
		return shardingv1.BuildStatefulSetForShard(instance, instance.Spec.Shard[t.specIdx])
	default:
		return nil, fmt.Errorf("unknown target kind %s", t.kind)
	}
}

// restartShapeTargetStatefulSet handles restart shape target stateful set for the sharding database controller.
func (r *ShardingDatabaseReconciler) restartShapeTargetStatefulSet(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
	currSts *appsv1.StatefulSet,
	reason string,
) (bool, error) {
	r.logLegacy(
		"INFO",
		fmt.Sprintf("Ordered shape rollout deleting %s %s for %s", t.kind, t.name, reason),
		nil,
		instance,
		r.Log,
	)

	if err := r.Client.Delete(context.Background(), currSts); err != nil && !errors.IsNotFound(err) {
		return true, err
	}
	return true, nil
}

// compareShapeSize handles compare shape size for the sharding database controller.
func compareShapeSize(oldCfg, newCfg shapes.ShapeConfig) int {
	oldScore := oldCfg.CPU + oldCfg.SGAGB + oldCfg.PGAGB + oldCfg.Processes
	newScore := newCfg.CPU + newCfg.SGAGB + newCfg.PGAGB + newCfg.Processes

	switch {
	case newScore > oldScore:
		return 1
	case newScore < oldScore:
		return -1
	default:
		return 0
	}
}

// isDownscaleShapeTarget handles is downscale shape target for the sharding database controller.
func (r *ShardingDatabaseReconciler) isDownscaleShapeTarget(
	instance *databasev4.ShardingDatabase,
	lastSuccSpec *databasev4.ShardingDatabaseSpec,
	t shapeRollTarget,
) bool {
	var oldShape, newShape string

	switch t.kind {
	case "CATALOG":
		oldShape = findCatalogShape(lastSuccSpec, t.name)
		newShape = findCatalogShape(&instance.Spec, t.name)
	case "SHARD":
		oldShape = findShardShapeForName(lastSuccSpec, t.name)
		newShape = findShardShapeForName(&instance.Spec, t.name)
	default:
		return false
	}

	if strings.TrimSpace(oldShape) == "" || strings.TrimSpace(newShape) == "" {
		return false
	}

	oldCfg, okOld := shapes.LookupShapeConfig(oldShape)
	newCfg, okNew := shapes.LookupShapeConfig(newShape)
	if !okOld || !okNew {
		return false
	}

	return compareShapeSize(oldCfg, newCfg) < 0
}

type shapeDbValues struct {
	SGABytes    int64
	SGAMaxBytes int64
	PGABytes    int64
	Processes   int64
	CPUCount    int64
}

func expectedShapeDbValues(cfg shapes.ShapeConfig) shapeDbValues {
	return shapeDbValues{
		SGABytes:    int64(cfg.SGAGB) * 1024 * 1024 * 1024,
		SGAMaxBytes: int64(cfg.SGAGB) * 1024 * 1024 * 1024,
		PGABytes:    int64(cfg.PGAGB) * 1024 * 1024 * 1024,
		Processes:   int64(cfg.Processes),
		CPUCount:    int64(cfg.CPU),
	}
}

func shapeDbValuesEqual(a, b shapeDbValues) bool {
	return a.SGABytes == b.SGABytes &&
		a.SGAMaxBytes == b.SGAMaxBytes &&
		a.PGABytes == b.PGABytes &&
		a.Processes == b.Processes &&
		a.CPUCount == b.CPUCount
}

func normalizeShapeReadLine(raw string) string {
	line := strings.TrimSpace(raw)
	for {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "SQL>") {
			line = strings.TrimSpace(line[4:])
			continue
		}
		break
	}
	return line
}

func parseShapeDbValues(out string) (shapeDbValues, error) {
	var vals shapeDbValues
	found := map[string]bool{}

	lines := strings.Split(out, "\n")
	for _, raw := range lines {
		line := normalizeShapeReadLine(raw)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		valStr := strings.TrimSpace(parts[1])

		n, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "INIT_SGA_SIZE":
			vals.SGABytes = n
			found[key] = true
		case "INIT_SGA_MAX_SIZE":
			vals.SGAMaxBytes = n
			found[key] = true
		case "INIT_PGA_SIZE":
			vals.PGABytes = n
			found[key] = true
		case "INIT_PROCESS":
			vals.Processes = n
			found[key] = true
		case "INIT_CPU_COUNT":
			vals.CPUCount = n
			found[key] = true
		}
	}

	required := []string{
		"INIT_SGA_SIZE",
		"INIT_SGA_MAX_SIZE",
		"INIT_PGA_SIZE",
		"INIT_PROCESS",
		"INIT_CPU_COUNT",
	}
	for _, k := range required {
		if !found[k] {
			return vals, fmt.Errorf("missing %s in DB verify output: %s", k, out)
		}
	}

	return vals, nil
}

func (r *ShardingDatabaseReconciler) readShapeDbValues(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
) (shapeDbValues, error) {
	switch t.kind {
	case "CATALOG":
		_, pod, err := r.validateInvidualCatalog(instance, instance.Spec.Catalog[t.specIdx], t.specIdx)
		if err != nil {
			return shapeDbValues{}, fmt.Errorf("catalog %s not ready: %w", t.name, err)
		}

		stdout, stderr, err := shardingv1.ReadDbShapeParams(pod.Name, "CATALOG", instance, r.kubeConfig, r.Log)
		if err != nil {
			return shapeDbValues{}, fmt.Errorf("readShapeDbValues failed stdout=%s stderr=%s err=%w", stdout, stderr, err)
		}
		return parseShapeDbValues(stdout)

	case "SHARD":
		_, pod, err := r.validateShard(instance, instance.Spec.Shard[t.specIdx], t.specIdx)
		if err != nil {
			return shapeDbValues{}, fmt.Errorf("shard %s not ready: %w", t.name, err)
		}

		stdout, stderr, err := shardingv1.ReadDbShapeParams(pod.Name, "SHARD", instance, r.kubeConfig, r.Log)
		if err != nil {
			return shapeDbValues{}, fmt.Errorf("readShapeDbValues failed stdout=%s stderr=%s err=%w", stdout, stderr, err)
		}
		return parseShapeDbValues(stdout)
	}

	return shapeDbValues{}, fmt.Errorf("unknown target kind %s", t.kind)
}

func shapeParamStringFromCfg(cfg shapes.ShapeConfig) string {
	return fmt.Sprintf(
		"INIT_SGA_SIZE=%d;INIT_PGA_SIZE=%d;INIT_PROCESS=%d;INIT_CPU_COUNT=%d;INIT_TOTAL_SIZE=%d",
		cfg.SGAGB*1024,
		cfg.PGAGB*1024,
		cfg.Processes,
		cfg.CPU,
		(cfg.SGAGB+cfg.PGAGB+1)*1024,
	)
}

func (r *ShardingDatabaseReconciler) applyShapeParamsViaScript(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
	cfg shapes.ShapeConfig,
) error {
	sparams := shapeParamStringFromCfg(cfg)

	switch t.kind {
	case "CATALOG":
		_, pod, err := r.validateInvidualCatalog(instance, instance.Spec.Catalog[t.specIdx], t.specIdx)
		if err != nil {
			return fmt.Errorf("catalog %s not ready: %w", t.name, err)
		}
		return shardingv1.ApplyDbShapeParams(pod.Name, sparams, "CATALOG", instance, r.kubeConfig, r.Log)

	case "SHARD":
		_, pod, err := r.validateShard(instance, instance.Spec.Shard[t.specIdx], t.specIdx)
		if err != nil {
			return fmt.Errorf("shard %s not ready: %w", t.name, err)
		}
		return shardingv1.ApplyDbShapeParams(pod.Name, sparams, "SHARD", instance, r.kubeConfig, r.Log)
	}

	return fmt.Errorf("unknown target kind %s", t.kind)
}

func (r *ShardingDatabaseReconciler) applyShapeDbParamsIfNeeded(
	instance *databasev4.ShardingDatabase,
	t shapeRollTarget,
) error {
	cfg, ok := desiredCfgForTarget(instance, t)
	if !ok {
		return fmt.Errorf("unable to resolve desired shape config for %s %s", t.kind, t.name)
	}

	r.logLegacy(
		"INFO",
		fmt.Sprintf("Applying DB shape params through python scripts for %s %s", t.kind, t.name),
		nil,
		instance,
		r.Log,
	)

	return r.applyShapeParamsViaScript(instance, t, cfg)
}

// reconcileOrderedShapeChanges applies shape changes in a controlled, restart-aware sequence.
func (r *ShardingDatabaseReconciler) reconcileOrderedShapeChanges(instance *databasev4.ShardingDatabase) (bool, error) {
	lastSuccSpec, err := instance.GetLastSuccessfulSpec()
	if err != nil {
		return false, err
	}
	if lastSuccSpec == nil {
		return false, nil
	}

	targets, err := r.orderedShapeChangeTargets(instance)
	if err != nil {
		return false, err
	}
	if len(targets) == 0 {
		return false, nil
	}

	for _, t := range targets {
		currSts, err := shardingv1.CheckSfset(t.name, instance, r.Client)
		if err != nil {
			r.logLegacy("INFO", "Ordered shape rollout waiting for StatefulSet "+t.name+" to be recreated", nil, instance, r.Log)
			return true, nil
		}

		desiredSts, err := r.desiredStatefulSetForShapeTarget(instance, t)
		if err != nil {
			return true, err
		}

		needsK8sRestart := statefulSetNeedsShapeRecreate(currSts, desiredSts)
		isDownscale := r.isDownscaleShapeTarget(instance, lastSuccSpec, t)

		if needsK8sRestart && isDownscale {
			if err := r.validateShapeTargetReady(instance, t); err != nil {
				r.logLegacy("INFO", "Ordered shape rollout waiting for "+t.kind+" "+t.name+" to become ready before downscale", nil, instance, r.Log)
				return true, nil
			}

			currentVals, err := r.readShapeDbValues(instance, t)
			if err != nil {
				r.logLegacy("INFO", "Ordered shape rollout DB param read failed for "+t.name+": "+err.Error(), nil, instance, r.Log)
				return true, err
			}

			cfg, ok := desiredCfgForTarget(instance, t)
			if !ok {
				return true, fmt.Errorf("unable to resolve desired shape config for %s %s", t.kind, t.name)
			}
			expectedVals := expectedShapeDbValues(cfg)

			if !shapeDbValuesEqual(currentVals, expectedVals) {
				if err := r.applyShapeDbParamsIfNeeded(instance, t); err != nil {
					r.logLegacy("INFO", "Ordered shape rollout pre-restart DB param apply failed for "+t.name+": "+err.Error(), nil, instance, r.Log)
					return true, err
				}
			}

			return r.restartShapeTargetStatefulSet(instance, t, currSts, "downscale Kubernetes shape change")
		}

		if needsK8sRestart && !isDownscale {
			return r.restartShapeTargetStatefulSet(instance, t, currSts, "Kubernetes shape change")
		}

		if err := r.validateShapeTargetReady(instance, t); err != nil {
			r.logLegacy("INFO", "Ordered shape rollout waiting for "+t.kind+" "+t.name+" to become ready", nil, instance, r.Log)
			return true, nil
		}

		cfg, ok := desiredCfgForTarget(instance, t)
		if !ok {
			return true, fmt.Errorf("unable to resolve desired shape config for %s %s", t.kind, t.name)
		}

		currentVals, err := r.readShapeDbValues(instance, t)
		if err != nil {
			r.logLegacy("INFO", "Ordered shape rollout DB param read failed for "+t.name+": "+err.Error(), nil, instance, r.Log)
			return true, err
		}

		expectedVals := expectedShapeDbValues(cfg)

		if shapeDbValuesEqual(currentVals, expectedVals) {
			r.logLegacy("INFO", "Ordered shape rollout DB params already correct for "+t.name, nil, instance, r.Log)
			continue
		}

		if isDownscale {
			r.logLegacy("INFO", "Ordered shape rollout waiting for downscale DB params to reflect for "+t.name, nil, instance, r.Log)
			return true, nil
		}

		if err := r.applyShapeDbParamsIfNeeded(instance, t); err != nil {
			r.logLegacy("INFO", "Ordered shape rollout DB param apply failed for "+t.name+": "+err.Error(), nil, instance, r.Log)
			return true, err
		}

		return r.restartShapeTargetStatefulSet(instance, t, currSts, "DB parameter restart")
	}

	return false, nil
}

// reconcileOrderedShardTemplateChanges applies non-shape pod-template changes
// one target at a time (catalog, gsm, then shards in ordinal order).
func (r *ShardingDatabaseReconciler) reconcileOrderedShardTemplateChanges(instance *databasev4.ShardingDatabase) (bool, error) {
	shapeTargets, err := r.orderedShapeChangeTargets(instance)
	if err != nil {
		return false, err
	}
	shapeTargetKeys := shapeTargetKeySet(shapeTargets)
	targets := orderedNonShapeTemplateRollTargets(instance, shapeTargetKeys)
	if len(targets) == 0 {
		return false, nil
	}

	for _, t := range targets {
		currSts, err := shardingv1.CheckSfset(t.name, instance, r.Client)
		if err != nil {
			r.logLegacy("INFO", "Ordered shard template rollout waiting for StatefulSet "+t.name+" to be recreated", nil, instance, r.Log)
			return true, nil
		}

		desiredSts, err := r.desiredStatefulSetForShapeTarget(instance, t)
		if err != nil {
			return true, err
		}

		driftReasons := nonShapeShardTemplateDriftReasons(currSts, desiredSts)
		if len(driftReasons) == 0 {
			continue
		}
		r.logLegacy(
			"INFO",
			fmt.Sprintf(
				"Ordered shard template rollout detected drift for %s %s: %s",
				t.kind,
				t.name,
				strings.Join(driftReasons, ","),
			),
			nil,
			instance,
			r.Log,
		)

		if err := r.validateShapeTargetReady(instance, t); err != nil {
			r.logLegacy("INFO", "Ordered shard template rollout waiting for "+t.kind+" "+t.name+" to become ready", nil, instance, r.Log)
			return true, nil
		}

		return r.restartShapeTargetStatefulSet(instance, t, currSts, "non-shape shard template change")
	}

	return false, nil
}

// applyReplicaScaleOutUnmarks handles apply replica scale out unmarks for the sharding database controller.
func (r *ShardingDatabaseReconciler) applyReplicaScaleOutUnmarks(instance *databasev4.ShardingDatabase) bool {
	desired := desiredShardNamesFromShardInfo(instance)
	changed := false

	for i := range instance.Spec.Shard {
		name := strings.TrimSpace(instance.Spec.Shard[i].Name)
		if name == "" {
			continue
		}

		// only touch shards that are desired again
		if !desired[name] {
			continue
		}

		curr := strings.ToLower(strings.TrimSpace(instance.Spec.Shard[i].IsDelete))
		if curr == "enable" || curr == "failed" {
			instance.Spec.Shard[i].IsDelete = "disable"
			changed = true

			r.logLegacy(
				"INFO",
				fmt.Sprintf("Resetting shard %s isDelete=disable because it is desired again for scale-out", name),
				nil,
				instance,
				r.Log,
			)
		}
	}

	return changed
}

func (r *ShardingDatabaseReconciler) syncCredentialRotationNonBlocking(
	ctx context.Context,
	inst *databasev4.ShardingDatabase,
	status *databasev4.ShardingDatabaseStatus,
) time.Duration {
	if inst == nil || status == nil {
		return 0
	}

	nn := types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}
	secretName := ""
	if inst.Spec.DbSecret != nil {
		secretName = strings.TrimSpace(inst.Spec.DbSecret.Name)
	}
	if secretName == "" {
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "ConfigError", "dbSecret.name is empty")
		return 0
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: inst.Namespace}, secret); err != nil {
		msg := "failed to read db secret: " + err.Error()
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "SecretReadFailed", msg)
		r.Recorder.Eventf(inst, corev1.EventTypeWarning, "CredentialSync", msg)
		return r.recordCredentialSyncFailure(ctx, nn, msg)
	}

	fingerprint, err := buildCredentialFingerprint(inst, secret)
	if err != nil {
		msg := "failed to build credential fingerprint: " + err.Error()
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "FingerprintFailed", msg)
		r.Recorder.Eventf(inst, corev1.EventTypeWarning, "CredentialSync", msg)
		return r.recordCredentialSyncFailure(ctx, nn, msg)
	}

	anns := inst.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}

	if fingerprint == strings.TrimSpace(anns[credentialSyncHashAnnotation]) {
		r.setCredentialSyncCondition(status, metav1.ConditionTrue, "InSync", "credentials are in sync")
		_ = r.clearCredentialSyncRetryState(ctx, nn)
		return 0
	}

	if nextRetryAt, ok := parseCredentialRetryTime(anns[credentialSyncNextRetryAtAnnotation]); ok {
		now := time.Now().UTC()
		if now.Before(nextRetryAt) {
			delay := nextRetryAt.Sub(now)
			msg := fmt.Sprintf("credential sync retry scheduled at %s", nextRetryAt.Format(time.RFC3339))
			r.setCredentialSyncCondition(status, metav1.ConditionFalse, "RetryScheduled", msg)
			return delay
		}
	}

	if err := validateCredentialSecretMaterial(inst, secret); err != nil {
		msg := "credential reset pre-check failed: " + err.Error()
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "PasswordResetFailed", msg)
		r.Recorder.Eventf(inst, corev1.EventTypeWarning, "CredentialSync", msg)
		return r.recordCredentialSyncFailure(ctx, nn, msg)
	}

	if err := shardingv1.ChangePassword(inst, r.kubeConfig, r.Log); err != nil {
		msg := "password change failed: " + err.Error()
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "PasswordResetFailed", msg)
		r.Recorder.Eventf(inst, corev1.EventTypeWarning, "CredentialSync", msg)
		return r.recordCredentialSyncFailure(ctx, nn, msg)
	}

	if err := r.recordCredentialSyncSuccess(ctx, nn, fingerprint); err != nil {
		msg := "credential sync metadata update failed: " + err.Error()
		r.setCredentialSyncCondition(status, metav1.ConditionFalse, "MetadataUpdateFailed", msg)
		r.Recorder.Eventf(inst, corev1.EventTypeWarning, "CredentialSync", msg)
		return credentialRetryInitialBackoff
	}

	r.setCredentialSyncCondition(status, metav1.ConditionTrue, "Synced", "credential update applied")
	r.Recorder.Eventf(inst, corev1.EventTypeNormal, "CredentialSync", "Credential sync completed successfully")
	return 0
}

func buildCredentialFingerprint(instance *databasev4.ShardingDatabase, secret *corev1.Secret) (string, error) {
	if instance == nil || instance.Spec.DbSecret == nil || secret == nil {
		return "", fmt.Errorf("missing credential inputs")
	}

	h := sha256.New()

	appendEntry := func(label string, cfg databasev4.PasswordSecretConfig) error {
		pwdKey := strings.TrimSpace(cfg.PasswordKey)
		if pwdKey == "" {
			return fmt.Errorf("%s passwordKey is empty", label)
		}
		pwdVal, ok := secret.Data[pwdKey]
		if !ok {
			return fmt.Errorf("%s passwordKey %q not found in secret %s", label, pwdKey, secret.Name)
		}
		_, _ = h.Write([]byte(label + ":pwdkey:" + pwdKey + "\n"))
		_, _ = h.Write(pwdVal)
		_, _ = h.Write([]byte("\n"))

		if v := strings.TrimSpace(cfg.PrivateKeyKey); v != "" {
			_, _ = h.Write([]byte(label + ":pkkey:" + v + "\n"))
		}
		if v := strings.TrimSpace(cfg.Pkeyopt); v != "" {
			_, _ = h.Write([]byte(label + ":pkeyopt:" + v + "\n"))
		}
		return nil
	}

	if err := appendEntry("dbAdmin", instance.Spec.DbSecret.DbAdmin); err != nil {
		return "", err
	}
	if instance.Spec.DbSecret.TDE != nil {
		if err := appendEntry("tde", *instance.Spec.DbSecret.TDE); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func validateCredentialSecretMaterial(instance *databasev4.ShardingDatabase, secret *corev1.Secret) error {
	if instance == nil || instance.Spec.DbSecret == nil || secret == nil {
		return fmt.Errorf("missing credential inputs")
	}

	validateOne := func(label string, cfg databasev4.PasswordSecretConfig) error {
		pwdKey := strings.TrimSpace(cfg.PasswordKey)
		if pwdKey == "" {
			return fmt.Errorf("%s passwordKey is empty", label)
		}
		if _, ok := secret.Data[pwdKey]; !ok {
			return fmt.Errorf("%s passwordKey %q not found", label, pwdKey)
		}
		if pk := strings.TrimSpace(cfg.PrivateKeyKey); pk != "" {
			if _, ok := secret.Data[pk]; !ok {
				return fmt.Errorf("%s privateKeyKey %q not found", label, pk)
			}
		}
		return nil
	}

	if err := validateOne("dbAdmin", instance.Spec.DbSecret.DbAdmin); err != nil {
		return err
	}
	tdeEnabled := strings.TrimSpace(instance.Spec.IsTdeWallet)
	if instance.Spec.TDEWallet != nil {
		if mode := strings.TrimSpace(instance.Spec.TDEWallet.IsEnabled); mode != "" {
			tdeEnabled = mode
		}
	}
	if strings.EqualFold(tdeEnabled, "enable") {
		if instance.Spec.DbSecret.TDE == nil {
			return fmt.Errorf("tde credential config is required when isTdeWallet=enable")
		}
		if err := validateOne("tde", *instance.Spec.DbSecret.TDE); err != nil {
			return err
		}
	}
	return nil
}

func (r *ShardingDatabaseReconciler) setCredentialSyncCondition(
	status *databasev4.ShardingDatabaseStatus,
	condStatus metav1.ConditionStatus,
	reason string,
	message string,
) {
	if status == nil {
		return
	}
	now := metav1.Now()
	meta.SetStatusCondition(&status.CrdStatus, metav1.Condition{
		Type:               credentialSyncConditionType,
		Status:             condStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

func parseCredentialRetryTime(raw string) (time.Time, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func credentialRetryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return credentialRetryInitialBackoff
	}
	d := credentialRetryInitialBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= credentialRetryMaxBackoff {
			return credentialRetryMaxBackoff
		}
	}
	return d
}

func (r *ShardingDatabaseReconciler) recordCredentialSyncFailure(
	ctx context.Context,
	key types.NamespacedName,
	errMsg string,
) time.Duration {
	attempt := 1
	nextDelay := credentialRetryInitialBackoff
	now := time.Now().UTC()

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		prevAttempt, convErr := strconv.Atoi(strings.TrimSpace(anns[credentialSyncRetryCountAnnotation]))
		if convErr == nil && prevAttempt > 0 {
			attempt = prevAttempt + 1
		}
		nextDelay = credentialRetryBackoff(attempt)
		nextAt := now.Add(nextDelay).Format(time.RFC3339)

		anns[credentialSyncRetryCountAnnotation] = strconv.Itoa(attempt)
		anns[credentialSyncNextRetryAtAnnotation] = nextAt
		anns[credentialSyncLastErrorAnnotation] = errMsg
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
	if err != nil {
		return credentialRetryInitialBackoff
	}
	return nextDelay
}

func (r *ShardingDatabaseReconciler) clearCredentialSyncRetryState(
	ctx context.Context,
	key types.NamespacedName,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if anns == nil {
			return nil
		}
		changed := false
		for _, k := range []string{
			credentialSyncRetryCountAnnotation,
			credentialSyncNextRetryAtAnnotation,
			credentialSyncLastErrorAnnotation,
		} {
			if _, ok := anns[k]; ok {
				delete(anns, k)
				changed = true
			}
		}
		if !changed {
			return nil
		}
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}

func (r *ShardingDatabaseReconciler) recordCredentialSyncSuccess(
	ctx context.Context,
	key types.NamespacedName,
	hash string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		curr := &databasev4.ShardingDatabase{}
		if err := r.Get(ctx, key, curr); err != nil {
			return err
		}
		anns := curr.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		anns[credentialSyncHashAnnotation] = hash
		delete(anns, credentialSyncRetryCountAnnotation)
		delete(anns, credentialSyncNextRetryAtAnnotation)
		delete(anns, credentialSyncLastErrorAnnotation)
		curr.SetAnnotations(anns)
		return r.Update(ctx, curr)
	})
}
func isMutatingProgressReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "PrimaryShardProgress",
		"StandbyShardProgress",
		"ScaleInProgress",
		"ShardStatePending",
		"OrderedShapeReconcileRetry",
		"OrderedShapeReconcileBlocked":
		return true
	default:
		return false
	}
}

func shouldEnforceUpdateLock(oldSpec, newSpec *databasev4.ShardingDatabaseSpec) bool {
	if oldSpec == nil || newSpec == nil {
		return true
	}

	oldCopy := oldSpec.DeepCopy()
	newCopy := newSpec.DeepCopy()
	if oldCopy == nil || newCopy == nil {
		return true
	}

	// Allow non-blocking spec updates for credential/event metadata only.
	oldCopy.DbSecret = nil
	newCopy.DbSecret = nil
	oldCopy.TopicId = ""
	newCopy.TopicId = ""

	return !reflect.DeepEqual(*oldCopy, *newCopy)
}

func isUpdateLockActive(inst *databasev4.ShardingDatabase) (bool, int64, string) {
	if inst == nil {
		return false, 0, ""
	}
	return lockpolicy.IsControllerUpdateLocked(inst.Status.CrdStatus, reconcilingType, updateLockReason)
}

func isUpdateLockOverrideEnabled(inst *databasev4.ShardingDatabase) (bool, string) {
	if inst == nil {
		return false, "resource is nil"
	}
	return lockpolicy.IsUpdateLockOverrideEnabled(inst.GetAnnotations(), lockOverrideAnnotation)
}
