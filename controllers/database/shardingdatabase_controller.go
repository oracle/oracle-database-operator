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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
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

var tdeKeySyncState = struct {
	sync.Mutex
	exported map[types.NamespacedName]bool
	imported map[types.NamespacedName]map[string]bool
}{
	exported: make(map[types.NamespacedName]bool),
	imported: make(map[types.NamespacedName]map[string]bool),
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
	reconcilingType   = "Reconciling"
	updateLockReason  = "UpdateInProgress"
	updateLockRequeue = 15 * time.Second

	lockOverrideAnnotation       = "database.oracle.com/lock-override"
	lockOverrideReasonAnnotation = "database.oracle.com/lock-override-reason"
	lockOverrideByAnnotation     = "database.oracle.com/lock-override-by"
	lockOverrideUntilAnnotation  = "database.oracle.com/lock-override-until"
	lockOverrideMaxTTL           = 30 * time.Minute

	credentialSyncConditionType         = "CredentialSync"
	credentialSyncHashAnnotation        = "database.oracle.com/credential-sync-hash"
	credentialSyncRetryCountAnnotation  = "database.oracle.com/credential-sync-retry-count"
	credentialSyncNextRetryAtAnnotation = "database.oracle.com/credential-sync-next-retry-at"
	credentialSyncLastErrorAnnotation   = "database.oracle.com/credential-sync-last-error"
	credentialRetryInitialBackoff       = 30 * time.Second
	credentialRetryMaxBackoff           = 10 * time.Minute
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
				overrideEnabled, overrideMsg := isUpdateLockOverrideEnabled(inst, time.Now().UTC())
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
	return ctrl.Result{}, nil
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
	ctx context.Context,
	inst *databasev4.ShardingDatabase,
	st *databasev4.ShardingDatabaseStatus,
	c *conditionSet,
) phaseResult {
	if inst.DeletionTimestamp == nil {
		if !controllerutil.ContainsFinalizer(inst, shardingv1.ShardingDatabaseFinalizer) {
			controllerutil.AddFinalizer(inst, shardingv1.ShardingDatabaseFinalizer)
			if err := r.Update(ctx, inst); err != nil {
				return phaseResult{err: err, reason: "FinalizerAddFailed", message: err.Error()}
			}
		}
		return phaseResult{}
	}
	if err := r.finalizeShardingDatabase(inst); err != nil {
		return phaseResult{wait: true, requeueAfter: 15 * time.Second, reason: "FinalizeRetry", message: err.Error()}
	}
	controllerutil.RemoveFinalizer(inst, shardingv1.ShardingDatabaseFinalizer)
	if err := r.Update(ctx, inst); err != nil {
		return phaseResult{err: err, reason: "FinalizerRemoveFailed", message: err.Error()}
	}
	return phaseResult{wait: true, requeueAfter: 2 * time.Second, reason: "Deleting", message: "Waiting object removal"}
}

// Phase 4 : Validate + Plan
// phaseValidateAndPlan validates spec and applies prerequisite spec patches before core reconciliation.
func (r *ShardingDatabaseReconciler) phaseValidateAndPlan(
	ctx context.Context, inst *databasev4.ShardingDatabase, st *databasev4.ShardingDatabaseStatus, c *conditionSet,
) phaseResult {
	plog := r.phaseLogger(inst, "validate_plan")
	if err := r.validateSpex(inst); err != nil {
		return phaseResult{err: err, reason: "SpecInvalid", message: err.Error()}
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

	// Service setup for Catalog
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

	// Catalog StatefulSet setup
	for i = 0; i < int32(len(inst.Spec.Catalog)); i++ {
		oraCatalogSpec := inst.Spec.Catalog[i]
		if len(oraCatalogSpec.Name) > 9 {
			return phaseResult{
				err:     fmt.Errorf("Catalog Name cannot be greater than 9 characters."),
				reason:  "CatalogNameTooLong",
				message: "Catalog Name cannot be greater than 9 characters.",
			}
		}
		if _, err := r.deployStatefulSet(inst, shardingv1.BuildStatefulSetForCatalog(inst, oraCatalogSpec), "CATALOG"); err != nil {
			return phaseResult{err: err, reason: "CatalogStatefulSetFailed", message: err.Error()}
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
		if _, err := r.deployStatefulSet(inst, shardingv1.BuildStatefulSetForGsm(inst, oraGsmSpec), "GSM"); err != nil {
			return phaseResult{err: err, reason: "GsmStatefulSetFailed", message: err.Error()}
		}
	}

	// Service setup for Shard
	for i = 0; i < int32(len(inst.Spec.Shard)); i++ {
		oraShardSpec := inst.Spec.Shard[i]
		if len(oraShardSpec.Name) > 9 {
			return phaseResult{
				err:     fmt.Errorf("Shard Name cannot be greater than 9 characters."),
				reason:  "ShardNameTooLong",
				message: "Shard Name cannot be greater than 9 characters.",
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
		if _, err := r.deployStatefulSet(inst, shardingv1.BuildStatefulSetForShard(inst, oraShardSpec), "SHARD"); err != nil {
			return phaseResult{err: err, reason: "ShardStatefulSetFailed", message: err.Error()}
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

	return phaseResult{}
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
	if err := r.addPrimaryShards(inst); err != nil {
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
	if err := r.addStandbyShards(inst); err != nil {
		plog.Info("standby shard flow not complete yet; requeue", "reason", "StandbyShardProgress", "error", err.Error(), "requeueAfter", 30*time.Second)
		return phaseResult{
			wait:         true,
			requeueAfter: 30 * time.Second,
			reason:       "StandbyShardProgress",
			message:      err.Error(),
		}
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

		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
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

// tdeStateKey builds a stable per-resource key for in-memory TDE sync state.
func tdeStateKey(instance *databasev4.ShardingDatabase) types.NamespacedName {
	return types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}
}

// ensureTDEKeySyncState initializes and garbage-collects in-memory TDE import/export markers.
func (r *ShardingDatabaseReconciler) ensureTDEKeySyncState(instance *databasev4.ShardingDatabase) {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()

	perShard, ok := tdeKeySyncState.imported[key]
	if !ok {
		perShard = make(map[string]bool)
		tdeKeySyncState.imported[key] = perShard
	}

	liveShardNames := make(map[string]struct{}, len(instance.Spec.Shard))
	for _, shard := range instance.Spec.Shard {
		liveShardNames[shard.Name] = struct{}{}
		if _, ok := perShard[shard.Name]; !ok {
			perShard[shard.Name] = false
		}
	}

	for name := range perShard {
		if _, ok := liveShardNames[name]; !ok {
			delete(perShard, name)
		}
	}
}

// hasExportedTDEKeys reports whether TDE keys were exported for this resource.
func (r *ShardingDatabaseReconciler) hasExportedTDEKeys(instance *databasev4.ShardingDatabase) bool {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()
	return tdeKeySyncState.exported[key]
}

// setExportedTDEKeys stores the exported-TDE-keys marker for this resource.
func (r *ShardingDatabaseReconciler) setExportedTDEKeys(instance *databasev4.ShardingDatabase, val bool) {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()
	tdeKeySyncState.exported[key] = val
}

// hasImportedTDEKeys reports whether TDE keys were imported for a shard.
func (r *ShardingDatabaseReconciler) hasImportedTDEKeys(instance *databasev4.ShardingDatabase, shardName string) bool {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()
	perShard := tdeKeySyncState.imported[key]
	if perShard == nil {
		return false
	}
	return perShard[shardName]
}

// setImportedTDEKeys stores the imported-TDE-keys marker for a shard.
func (r *ShardingDatabaseReconciler) setImportedTDEKeys(instance *databasev4.ShardingDatabase, shardName string, val bool) {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()
	perShard, ok := tdeKeySyncState.imported[key]
	if !ok {
		perShard = make(map[string]bool)
		tdeKeySyncState.imported[key] = perShard
	}
	perShard[shardName] = val
}

// clearTDEKeySyncState removes all in-memory TDE markers for a deleted resource.
func (r *ShardingDatabaseReconciler) clearTDEKeySyncState(instance *databasev4.ShardingDatabase) {
	key := tdeStateKey(instance)

	tdeKeySyncState.Lock()
	defer tdeKeySyncState.Unlock()
	delete(tdeKeySyncState.exported, key)
	delete(tdeKeySyncState.imported, key)
}

// ================ Function to check secret update=============
// UpdateSecret probes the configured DB secret; retained for legacy compatibility.
func (r *ShardingDatabaseReconciler) UpdateSecret(instance *databasev4.ShardingDatabase, kClient client.Client, logger logr.Logger) (ctrl.Result, error) {

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

// ========================== FInalizer Section ===================
// addFinalizer attaches the sharding finalizer to the CR.
func (r *ShardingDatabaseReconciler) addFinalizer(instance *databasev4.ShardingDatabase) error {
	reqLogger := r.Log.WithValues("instance.Namespace", instance.Namespace, "instance.Name", instance.Name)
	controllerutil.AddFinalizer(instance, shardingv1.ShardingDatabaseFinalizer)

	// Update CR
	err := r.Client.Update(context.TODO(), instance)
	if err != nil {
		reqLogger.Error(err, "Failed to update Sharding Database  with finalizer")
		return err
	}
	return nil
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
		if instance.Spec.IsExternalSvc && len(spec.PvcName) == 0 {
			deleteIfFoundSvc(spec.Name + strconv.FormatInt(int64(i), 10) + "-svc")
		}
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
	if _, _, err := r.validateCatalog(instance); err != nil {
		return fmt.Errorf("catalog validation failed: %w", err)
	}
	if _, _, err := r.validateGsm(instance); err != nil {
		return fmt.Errorf("gsm validation failed: %w", err)
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
func (r *ShardingDatabaseReconciler) validateInvidualGsm(instance *databasev4.ShardingDatabase, OraGsmSpex databasev4.GsmSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	var msg string
	gsmSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}

	var err error
	podList := &corev1.PodList{}
	var isPodExist bool

	gsmSfSet, err = shardingv1.CheckSfset(OraGsmSpex.Name, instance, r.Client)
	if err != nil {
		msg = "Unable to find  GSM statefulset " + shardingv1.GetFmtStr(OraGsmSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specId, string(databasev4.StatefulSetNotFound))
		return gsmSfSet, gsmPod, err
	}

	podList, err = shardingv1.GetPodList(gsmSfSet.Name, "GSM", instance, r.Client)
	if err != nil {
		msg = "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(gsmSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specId, string(databasev4.PodNotFound))
		return gsmSfSet, gsmPod, err
	}

	isPodExist, gsmPod = shardingv1.PodListValidation(podList, gsmSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg = "Unable to validate GSM " + shardingv1.GetFmtStr(OraGsmSpex.Name) + " pod. GSM pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specId, string(databasev4.PodNotReadyState))
		return gsmSfSet, gsmPod, fmt.Errorf("pod doesn't exist")
	}
	err = shardingv1.CheckGsmStatus(gsmPod.Name, instance, r.kubeConfig, r.Log)
	if err != nil {
		msg = "Unable to validate GSM director. GSM director doesn't seems to be ready to accept the commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateGsmStatus(instance, specId, string(databasev4.ProvisionState))
		return gsmSfSet, gsmPod, err
	}

	r.updateGsmStatus(instance, specId, string(databasev4.AvailableState))
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
func (r *ShardingDatabaseReconciler) validateInvidualCatalog(instance *databasev4.ShardingDatabase, OraCatalogSpex databasev4.CatalogSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {

	var err error
	catalogSfSet := &appsv1.StatefulSet{}
	catalogPod := &corev1.Pod{}
	podList := &corev1.PodList{}
	var isPodExist bool

	catalogSfSet, err = shardingv1.CheckSfset(OraCatalogSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Catalog statefulset " + shardingv1.GetFmtStr(OraCatalogSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev4.StatefulSetNotFound))
		return catalogSfSet, catalogPod, err
	}

	podList, err = shardingv1.GetPodList(catalogSfSet.Name, "CATALOG", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(catalogSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev4.PodNotFound))
		return catalogSfSet, catalogPod, err
	}
	isPodExist, catalogPod = shardingv1.PodListValidation(podList, catalogSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Catalog " + shardingv1.GetFmtStr(catalogSfSet.Name) + " pod. Catalog pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateCatalogStatus(instance, specId, string(databasev4.PodNotReadyState))
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
		r.updateCatalogStatus(instance, specId, string(databasev4.ProvisionState))
		return catalogSfSet, catalogPod, err
	}

	r.updateCatalogStatus(instance, specId, string(databasev4.AvailableState))
	return catalogSfSet, catalogPod, nil

}

// ======= Function to validate Shard
// validateShard validates one shard member and updates its status projection.
func (r *ShardingDatabaseReconciler) validateShard(instance *databasev4.ShardingDatabase, OraShardSpex databasev4.ShardSpec, specId int,
) (*appsv1.StatefulSet, *corev1.Pod, error) {
	shardSfSet := &appsv1.StatefulSet{}
	shardPod := &corev1.Pod{}

	shardSfSet, err := shardingv1.CheckSfset(OraShardSpex.Name, instance, r.Client)
	if err != nil {
		msg := "Unable to find Shard statefulset " + shardingv1.GetFmtStr(OraShardSpex.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev4.StatefulSetNotFound))
		return shardSfSet, shardPod, err
	}

	podList, err := shardingv1.GetPodList(shardSfSet.Name, "SHARD", instance, r.Client)
	if err != nil {
		msg := "Unable to find any pod in statefulset " + shardingv1.GetFmtStr(shardSfSet.Name) + "."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev4.PodNotFound))
		return shardSfSet, shardPod, err
	}
	isPodExist, shardPod := shardingv1.PodListValidation(podList, shardSfSet.Name, instance, r.Client)
	if !isPodExist {
		msg := "Unable to validate Shard " + shardingv1.GetFmtStr(OraShardSpex.Name) + " pod. Shard pod doesn't seem to be ready to accept commands."
		r.logLegacy("Error", msg, nil, instance, r.Log)
		r.updateShardStatus(instance, specId, string(databasev4.PodNotReadyState))
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
		r.updateShardStatus(instance, specId, string(databasev4.ProvisionState))
		return shardSfSet, shardPod, err
	}

	r.updateShardStatus(instance, specId, string(databasev4.AvailableState))
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

	primByGroup := map[string]*databasev4.ShardSpec{}
	var firstPrim *databasev4.ShardSpec

	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			if firstPrim == nil {
				firstPrim = s
			}
			g := strings.TrimSpace(s.ShardGroup)
			if g != "" {
				if _, ok := primByGroup[g]; !ok {
					primByGroup[g] = s
				}
			}
		}
	}

	if firstPrim == nil {
		return false, fmt.Errorf("primary shard not found yet")
	}

	changed := false

	for si := range instance.Spec.ShardInfo {
		info := &instance.Spec.ShardInfo[si]
		if info.ShardGroupDetails == nil {
			continue
		}

		if !strings.EqualFold(strings.TrimSpace(info.ShardGroupDetails.DeployAs), "STANDBY") {
			continue
		}

		if info.PrimaryDatabaseRef != nil && strings.TrimSpace(info.PrimaryDatabaseRef.Host) != "" {
			continue
		}

		key := strings.TrimSpace(info.ShardGroupDetails.Name)
		prim := firstPrim
		if key != "" {
			if p, ok := primByGroup[key]; ok {
				prim = p
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
func (r *ShardingDatabaseReconciler) addPrimaryShards(instance *databasev4.ShardingDatabase) error {
	var err error
	shardSfSet := &appsv1.StatefulSet{}
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

		// 2) Ensure shard exists in GSM. Add only when missing.
		sparamsCheck := shardingv1.BuildShardParams(instance, shardSfSet, oraShardSpec)
		if err = shardingv1.CheckShardInGsm(gsmPod.Name, sparamsCheck, instance, r.kubeConfig, r.Log); err != nil {
			sparamsAdd := shardingv1.BuildShardParamsForAdd(instance, shardSfSet, oraShardSpec)
			r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.AddingShardState))
			if err = shardingv1.AddShardInGsm(gsmPod.Name, sparamsAdd, instance, r.kubeConfig, r.Log); err != nil {
				r.updateGsmShardStatus(instance, oraShardSpec.Name, string(databasev4.AddingShardErrorState))
				r.logLegacy("Error", instance.Namespace+":Shard Addition Failure:"+err.Error(), nil, instance, r.Log)
				addFailedCount++
				deployFlag = "false"
				continue
			} else {
				deployFlag = "true"
				deployParams = sparamsAdd
			}
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

	if instance.Spec.IsDataGuard {
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
func (r *ShardingDatabaseReconciler) addStandbyShards(instance *databasev4.ShardingDatabase) error {
	var err error

	shardSfSet := &appsv1.StatefulSet{}
	gsmPod := &corev1.Pod{}

	addFailedCount := 0
	notReadyCount := 0
	hasStandby := false
	deployFlag := "none"
	var deployParams string

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

		// 3) Non-DG flow: standby shard add in GSM
		if !instance.Spec.IsDataGuard {
			sparamsCheck := shardingv1.BuildShardParams(instance, shardSfSet, OraShardSpex)

			if inGsmErr := shardingv1.CheckShardInGsm(gsmPod.Name, sparamsCheck, instance, r.kubeConfig, r.Log); inGsmErr != nil {
				sparamsAdd := shardingv1.BuildShardParamsForAdd(instance, shardSfSet, OraShardSpex)

				r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev4.AddingShardState))
				err = shardingv1.AddShardInGsm(gsmPod.Name, sparamsAdd, instance, r.kubeConfig, r.Log)
				if err != nil {
					r.updateGsmShardStatus(instance, OraShardSpex.Name, string(databasev4.AddingShardErrorState))
					addFailedCount++
					deployFlag = "false"
					continue
				} else {
					deployFlag = "true"
					deployParams = sparamsAdd
				}
			}

			// Deploy whenever standby shard exists but is not yet deployed.
		} else {
			// DG flow: skip standby shard add in GSM
			if instance.Status.Dg.Broker == nil {
				instance.Status.Dg.Broker = map[string]string{}
			}
			if strings.TrimSpace(instance.Status.Dg.Broker[OraShardSpex.Name]) == "" {
				instance.Status.Dg.Broker[OraShardSpex.Name] = "SKIPPED_GSM"
				_ = r.Status().Update(context.Background(), instance)
			}
		}

		// 4) DG flow
		if instance.Spec.IsDataGuard && !r.dgBrokerDone(instance, OraShardSpex.Name) {
			if instance.Status.Dg.Broker == nil {
				instance.Status.Dg.Broker = map[string]string{}
			}
			instance.Status.Dg.State = "PENDING"
			r.setDgBrokerStatus(instance, OraShardSpex.Name, "pending")

			primary, perr := r.findPrimaryForStandby(instance, OraShardSpex)
			if perr != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:"+perr.Error())
				return perr
			}

			primaryPod := primary.Name + "-0"
			standbyPod := OraShardSpex.Name + "-0"

			primaryDbUnique := strings.ToUpper(strings.TrimSpace(primary.Name))
			standbyDbUnique := strings.ToUpper(strings.TrimSpace(OraShardSpex.Name))
			cfgName := strings.ToUpper(strings.TrimSpace(instance.Name)) + "_DGCFG"

			primaryConnects := []string{
				shardingv1.BuildDgmgrlConnectIdentifier(instance, primary.Name, primaryDbUnique),
			}
			standbyConnects := []string{
				shardingv1.BuildDgmgrlConnectIdentifier(instance, OraShardSpex.Name, standbyDbUnique),
			}

			// 4A) Fix broker files + start broker on both sides
			if e := shardingv1.EnsureDgBrokerFilesAndStart(primaryPod, primaryDbUnique, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:broker_files_primary:"+e.Error())
				return e
			}
			if e := shardingv1.EnsureDgBrokerFilesAndStart(standbyPod, standbyDbUnique, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:broker_files_standby:"+e.Error())
				return e
			}

			// 4B) Primary prerequisite SQL
			if e := shardingv1.RunStandbyDatabasePrerequisitesSQL(primaryPod, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:prereqs-primary:"+e.Error())
				return e
			}

			// 4C) Enable archive log on primary
			if e := shardingv1.EnableArchiveLogInPod(primaryPod, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:archivelog-primary:"+e.Error())
				return e
			}

			// 4D) Force logging on primary
			if e := shardingv1.RunSQLPlusInPod(primaryPod, dbcommons.ForceLoggingTrueSQL, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:forcelogging-primary:"+e.Error())
				return e
			}

			// 4E) Flashback on primary
			if e := shardingv1.RunSQLPlusInPod(primaryPod, dbcommons.FlashBackTrueSQL, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:flashback-primary:"+e.Error())
				return e
			}
			// 4F) Ensure standby redo logs on both primary and standby
			if e := shardingv1.EnsureStandbyRedoLogsForShards(primaryPod, standbyPod, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:standby-redo-logs:"+e.Error())
				return e
			}

			// 4G) Primary redo transport setup
			if e := r.setupPrimaryRedoTransport(instance, *primary, OraShardSpex); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:redo-transport:"+e.Error())
				return e
			}

			// 4H) Start standby apply
			if e := r.EnsureStandbyApplyRunning(instance, OraShardSpex); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:start-apply:"+e.Error())
				return e
			}

			// 4I) Force archive and verify transport
			if e := r.ForceArchiveAndCheckRedoTransport(instance, *primary); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:force-archive-check:"+e.Error())
				return e
			}

			// 4J) Create broker config
			if e := shardingv1.CreateDgBrokerConfigTryConnects(primaryPod, cfgName, primaryDbUnique, primaryConnects, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:create-config:"+e.Error())
				return e
			}

			// 4K) Add standby to broker config
			if e := shardingv1.AddStandbyToDgBrokerConfigTryConnects(primaryPod, standbyDbUnique, standbyConnects, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:add-standby:"+e.Error())
				return e
			}

			// 4L) Set DGConnectIdentifier and StaticConnectIdentifier
			if e := r.SetDgConnectIdentifiers(instance, *primary, OraShardSpex); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:set-connect-identifiers:"+e.Error())
				return e
			}

			// 4M) Enable and validate broker
			if e := shardingv1.EnableAndValidateDgBroker(primaryPod, cfgName, instance, r.kubeConfig, r.Log); e != nil {
				instance.Status.Dg.State = "ERROR"
				r.setDgBrokerStatus(instance, OraShardSpex.Name, "error:enable-validate:"+e.Error())
				return e
			}

			r.setDgBrokerStatus(instance, OraShardSpex.Name, "true")
			instance.Status.Dg.State = "ENABLED"
			_ = r.Status().Update(context.Background(), instance)
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

	if !instance.Spec.IsDataGuard {
		if deployFlag == "true" {
			if derr := shardingv1.DeployShardInGsm(gsmPod.Name, deployParams, instance, r.kubeConfig, r.Log); derr != nil {
				r.logLegacy("INFO", "DeployShardInGsm pending for standby shard; requeue: "+derr.Error(), nil, instance, r.Log)
				return fmt.Errorf("standby deploy in GSM pending: %w", derr)
			}
		}
		if len(deployParams) > 0 {
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
	svcFound := &corev1.Service{}

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

	found := &corev1.Service{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil {
		if !errors.IsNotFound(err) {
			reqLogger.Error(err, "Failed to get Service")
			return ctrl.Result{}, err
		}

		reqLogger.Info("Creating Service")
		if err := r.Client.Create(context.TODO(), dep); err != nil {
			reqLogger.Error(err, "Failed to create Service")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	r.logLegacy("DEBUG", "Service "+shardingv1.GetFmtStr(dep.Name)+" already exists", nil, instance, r.Log)
	return ctrl.Result{}, nil
}

// This function deploy the statefulset

// deployStatefulSet ensures a StatefulSet exists for the requested topology member.
func (r *ShardingDatabaseReconciler) deployStatefulSet(
	instance *databasev4.ShardingDatabase,
	dep *appsv1.StatefulSet,
	resType string,
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

	found := &appsv1.StatefulSet{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
	if err != nil {
		if !errors.IsNotFound(err) {
			reqLogger.Error(err, "Failed to get StatefulSet")
			return ctrl.Result{}, err
		}

		reqLogger.Info("Creating StatefulSet")
		if err := r.Client.Create(context.TODO(), dep); err != nil {
			reqLogger.Error(err, "Failed to create StatefulSet")
			return ctrl.Result{}, err
		}

		r.logLegacy("INFO", "Created StatefulSet "+shardingv1.GetFmtStr(dep.Name), nil, instance, r.Log)
		return ctrl.Result{Requeue: true}, nil
	}

	r.logLegacy("DEBUG", "StatefulSet "+shardingv1.GetFmtStr(dep.Name)+" already exists", nil, instance, r.Log)
	return ctrl.Result{}, nil
}

// checkShardState evaluates shard lifecycle state machine transitions and gate conditions.
func (r *ShardingDatabaseReconciler) checkShardState(instance *databasev4.ShardingDatabase) error {

	var i int32
	var err error = nil
	var OraShardSpex databasev4.ShardSpec
	var currState string
	var eventMsg string
	var msg string

	currState = ""
	eventMsg = ""

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
				err = fmt.Errorf("%s", eventMsg)
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
) error {

	primaryPod := primary.Name + "-0"
	standbyDbUnique := strings.ToUpper(strings.TrimSpace(standby.Name))
	primaryDbUnique := strings.ToUpper(strings.TrimSpace(primary.Name))

	standbyConnect := shardingv1.BuildDgmgrlConnectIdentifier(instance, standby.Name, standbyDbUnique)

	logArchiveConfigSQL := fmt.Sprintf(
		"alter system set log_archive_config='dg_config=(%s,%s)' scope=both sid='*';",
		primaryDbUnique, standbyDbUnique,
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

	primaryConnect := shardingv1.BuildDgmgrlConnectIdentifier(instance, primary.Name, primaryDbUnique)
	standbyConnect := shardingv1.BuildDgmgrlConnectIdentifier(instance, standby.Name, standbyDbUnique)

	primaryStatic := shardingv1.BuildDgmgrlStaticConnectIdentifier(instance, primary.Name, primaryDbUnique)
	standbyStatic := shardingv1.BuildDgmgrlStaticConnectIdentifier(instance, standby.Name, standbyDbUnique)

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
	if instance.Status.Dg.Broker == nil {
		instance.Status.Dg.Broker = map[string]string{}
	}
	instance.Status.Dg.Broker[shardName] = val
	_ = r.Status().Update(context.Background(), instance)
}

// findPrimaryForStandby handles find primary for standby for the sharding database controller.
func (r *ShardingDatabaseReconciler) findPrimaryForStandby(instance *databasev4.ShardingDatabase, standby databasev4.ShardSpec) (*databasev4.ShardSpec, error) {
	sg := strings.TrimSpace(standby.ShardGroup)

	// prefer same shardGroup primary
	for i := range instance.Spec.Shard {
		s := &instance.Spec.Shard[i]
		if strings.EqualFold(strings.TrimSpace(s.DeployAs), "PRIMARY") {
			if sg != "" && strings.EqualFold(strings.TrimSpace(s.ShardGroup), sg) {
				return s, nil
			}
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
		desiredByPrefix[prefix] = shardInfoDesiredCount(info)
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
	if err := r.APIReader.Get(
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

func shardInfoDesiredCount(info databasev4.ShardingDetails) int32 {
	if info.ShardNum > 0 {
		return info.ShardNum
	}
	if info.Replicas > 0 {
		return info.Replicas
	}
	return 0
}

// desiredShardNamesFromShardInfo handles desired shard names from shard info for the sharding database controller.
func desiredShardNamesFromShardInfo(instance *databasev4.ShardingDatabase) map[string]bool {
	desired := map[string]bool{}

	for i := range instance.Spec.ShardInfo {
		prefix := strings.TrimSpace(instance.Spec.ShardInfo[i].ShardPreFixName)
		if prefix == "" {
			continue
		}

		replicas := shardInfoDesiredCount(instance.Spec.ShardInfo[i])
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
) *appsv1.StatefulSet {
	switch t.kind {
	case "CATALOG":
		return shardingv1.BuildStatefulSetForCatalog(instance, instance.Spec.Catalog[t.specIdx])
	case "SHARD":
		return shardingv1.BuildStatefulSetForShard(instance, instance.Spec.Shard[t.specIdx])
	default:
		return nil
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
	SGABytes  int64
	PGABytes  int64
	Processes int64
	CPUCount  int64
}

func expectedShapeDbValues(cfg shapes.ShapeConfig) shapeDbValues {
	return shapeDbValues{
		SGABytes:  int64(cfg.SGAGB * 1024 * 1024 * 1024),
		PGABytes:  int64(cfg.PGAGB * 1024 * 1024 * 1024),
		Processes: int64(cfg.Processes),
		CPUCount:  int64(cfg.CPU),
	}
}

func shapeDbValuesEqual(a, b shapeDbValues) bool {
	return a.SGABytes == b.SGABytes &&
		a.PGABytes == b.PGABytes &&
		a.Processes == b.Processes &&
		a.CPUCount == b.CPUCount
}

func parseShapeDbValues(out string) (shapeDbValues, error) {
	var vals shapeDbValues
	lines := strings.Split(out, "\n")

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])

		n, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return vals, fmt.Errorf("unable to parse %s value %q: %w", key, valStr, err)
		}

		switch key {
		case "INIT_SGA_SIZE":
			vals.SGABytes = n
		case "INIT_PGA_SIZE":
			vals.PGABytes = n
		case "INIT_PROCESS":
			vals.Processes = n
		case "INIT_CPU_COUNT":
			vals.CPUCount = n
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

		desiredSts := r.desiredStatefulSetForShapeTarget(instance, t)
		if desiredSts == nil {
			return true, fmt.Errorf("unable to build desired StatefulSet for %s %s", t.kind, t.name)
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
	if strings.EqualFold(strings.TrimSpace(instance.Spec.IsTdeWallet), "enable") {
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

func getCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
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

	cond := getCondition(inst.Status.CrdStatus, reconcilingType)
	if cond == nil {
		return false, 0, ""
	}
	if cond.Status != metav1.ConditionTrue {
		return false, 0, ""
	}
	if strings.TrimSpace(cond.Reason) != updateLockReason {
		return false, 0, ""
	}

	return true, cond.ObservedGeneration, cond.Message
}

func isUpdateLockOverrideEnabled(inst *databasev4.ShardingDatabase, now time.Time) (bool, string) {
	if inst == nil {
		return false, "resource is nil"
	}

	annotations := inst.GetAnnotations()
	if len(annotations) == 0 {
		return false, ""
	}

	if !strings.EqualFold(strings.TrimSpace(annotations[lockOverrideAnnotation]), "true") {
		return false, ""
	}

	reason := strings.TrimSpace(annotations[lockOverrideReasonAnnotation])
	if reason == "" {
		return false, "missing override reason annotation"
	}

	by := strings.TrimSpace(annotations[lockOverrideByAnnotation])
	if by == "" {
		return false, "missing override by annotation"
	}

	untilRaw := strings.TrimSpace(annotations[lockOverrideUntilAnnotation])
	if untilRaw == "" {
		return false, "missing override until annotation"
	}

	until, err := time.Parse(time.RFC3339, untilRaw)
	if err != nil {
		return false, "invalid override until timestamp (must be RFC3339)"
	}

	now = now.UTC()
	if !until.After(now) {
		return false, "override has expired"
	}
	if until.After(now.Add(lockOverrideMaxTTL)) {
		return false, fmt.Sprintf("override exceeds max ttl of %s", lockOverrideMaxTTL)
	}

	msg := fmt.Sprintf("override accepted by=%s until=%s reason=%s", by, until.Format(time.RFC3339), reason)
	return true, msg
}
