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

package v4

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	shapes "github.com/oracle/oracle-database-operator/commons/shapes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type shardingMode string

const (
	modeSystem    shardingMode = "system"
	modeUser      shardingMode = "user"
	modeComposite shardingMode = "composite"
	modeUnknown   shardingMode = "unknown"

	replDG     = "DG"
	replNative = "NATIVE"

	deleteStateEnable  = "enable"
	deleteStateDisable = "disable"
	deleteStateFailed  = "failed"

	reconcilingConditionType = "Reconciling"
	updateLockReason         = "UpdateInProgress"

	lockOverrideAnnotation       = "database.oracle.com/lock-override"
	lockOverrideReasonAnnotation = "database.oracle.com/lock-override-reason"
	lockOverrideByAnnotation     = "database.oracle.com/lock-override-by"
	lockOverrideUntilAnnotation  = "database.oracle.com/lock-override-until"
	lockOverrideMaxTTL           = 30 * time.Minute
)

// log is for logging in this package.
var shardingdatabaselog = logf.Log.WithName("shardingdatabase-resource")

func findStatusCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

func isControllerUpdateLocked(cr *ShardingDatabase) (bool, int64, string) {
	if cr == nil {
		return false, 0, ""
	}

	cond := findStatusCondition(cr.Status.CrdStatus, reconcilingConditionType)
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

func isUpdateLockOverrideEnabled(cr *ShardingDatabase, now time.Time) (bool, string) {
	if cr == nil {
		return false, "resource is nil"
	}

	annotations := cr.GetAnnotations()
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

func (r *ShardingDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ShardingDatabase{}).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

var _ webhook.CustomDefaulter = &ShardingDatabase{}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-shardingdatabase,mutating=true,failurePolicy=fail,sideEffects=none,groups=database.oracle.com,resources=shardingdatabases,verbs=create;update,versions=v4,name=mshardingdatabasev4.kb.io,admissionReviewVersions=v1

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *ShardingDatabase) Default(ctx context.Context, obj runtime.Object) error {
	cr, ok := obj.(*ShardingDatabase)
	if !ok {
		return fmt.Errorf("expected obj *ShardingDatabase but got %T", obj)
	}

	logger := shardingdatabaselog.WithValues("name", cr.Name, "namespace", cr.Namespace)
	logger.Info("applying shardingdatabase defaults")

	if strings.TrimSpace(cr.Spec.GsmDevMode) == "" {
		cr.Spec.GsmDevMode = "dev"
	}
	if strings.TrimSpace(cr.Spec.IsTdeWallet) == "" {
		cr.Spec.IsTdeWallet = "disable"
	}

	applyGlobalShardingReplicationDefaults(&cr.Spec)

	for i := range cr.Spec.Shard {
		if strings.TrimSpace(strings.ToLower(cr.Spec.Shard[i].IsDelete)) == "" {
			cr.Spec.Shard[i].IsDelete = "disable"
		}
	}

	for i := range cr.Spec.ShardInfo {
		if cr.Spec.ShardInfo[i].ShardGroupDetails != nil &&
			strings.TrimSpace(strings.ToLower(cr.Spec.ShardInfo[i].ShardGroupDetails.IsDelete)) == "" {
			cr.Spec.ShardInfo[i].ShardGroupDetails.IsDelete = "disable"
		}
	}

	var totalShard int32
	for i := range cr.Spec.ShardInfo {
		count := getShardInfoCount(&cr.Spec.ShardInfo[i])
		if count == 0 {
			count = 2
			cr.Spec.ShardInfo[i].ShardNum = count
		}
		totalShard += count
	}

	if totalShard > 0 {
		logger.Info("rebuilding shard spec from shardInfo", "desiredShardCount", totalShard)
		desired := cr.buildDesiredShardSpec()
		cr.Spec.Shard = mergeDesiredAndExistingShards(cr.Spec.Shard, desired)
	}

	defaultPasswordSecretConfig(cr.Spec.DbSecret)

	// apply shape on catalog
	for i := range cr.Spec.Catalog {
		if cfg, ok := shapes.LookupShapeConfig(cr.Spec.Catalog[i].Shape); ok {
			cr.Spec.Catalog[i].EnvVars = upsertEnvVars(
				cr.Spec.Catalog[i].EnvVars,
				envVarsFromPairs(cfg.EnvPairs()),
				true,
			)
			cr.Spec.Catalog[i].Resources = cfg.ResourceRequirements()
		}
	}

	return nil
}

func defaultPasswordSecretConfig(secret *SecretDetails) {
	if secret == nil {
		return
	}
	if strings.TrimSpace(secret.MountPath) == "" {
		secret.MountPath = DefaultSecretMountPath
	}
	defaultPasswordEntry(&secret.DbAdmin)
	if secret.TDE != nil {
		defaultPasswordEntry(secret.TDE)
	}
}

func defaultPasswordEntry(entry *PasswordSecretConfig) {
	if entry == nil {
		return
	}
	if strings.TrimSpace(entry.PrivateKeyKey) != "" && strings.TrimSpace(entry.Pkeyopt) == "" {
		entry.Pkeyopt = DefaultPkeyopt
	}
}

func (r *ShardingDatabase) validateDbSecretConfig() field.ErrorList {
	var errs field.ErrorList
	dbSecretPath := field.NewPath("spec").Child("dbSecret")
	if r.Spec.DbSecret == nil {
		return append(errs, field.Required(dbSecretPath, "dbSecret must be set"))
	}

	if strings.TrimSpace(r.Spec.DbSecret.Name) == "" {
		errs = append(errs, field.Required(dbSecretPath.Child("name"), "secret name must not be empty"))
	}

	if mountPath := strings.TrimSpace(r.Spec.DbSecret.MountPath); mountPath != "" && !strings.HasPrefix(mountPath, "/") {
		errs = append(errs, field.Invalid(dbSecretPath.Child("mountPath"), r.Spec.DbSecret.MountPath, "mountPath must be an absolute path"))
	}

	errs = append(errs, validatePasswordSecretConfig(r.Spec.DbSecret.DbAdmin, dbSecretPath.Child("dbAdmin"))...)
	if r.Spec.DbSecret.TDE != nil {
		errs = append(errs, validatePasswordSecretConfig(*r.Spec.DbSecret.TDE, dbSecretPath.Child("tde"))...)
	}

	return errs
}

func validatePasswordSecretConfig(cfg PasswordSecretConfig, p *field.Path) field.ErrorList {
	var errs field.ErrorList

	if strings.TrimSpace(cfg.PasswordKey) == "" {
		errs = append(errs, field.Required(p.Child("passwordKey"), "passwordKey must be set"))
	}

	if strings.TrimSpace(cfg.Pkeyopt) != "" && strings.TrimSpace(cfg.PrivateKeyKey) == "" {
		errs = append(errs, field.Forbidden(p.Child("pkeyopt"), "pkeyopt requires privateKeyKey"))
	}

	return errs
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-shardingdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=shardingdatabases,versions=v4,name=vshardingdatabasev4.kb.io,admissionReviewVersions={v1}

var _ webhook.CustomValidator = &ShardingDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := shardingdatabaselog.WithValues("webhook", "validateCreate")

	// TODO(user): fill in your validation logic upon object creation.
	// Check Secret configuration
	var validationErr field.ErrorList
	var validationErrs1 field.ErrorList
	cr, ok := obj.(*ShardingDatabase)

	if !ok {
		validationErr = append(validationErr, field.Invalid(field.NewPath("obj"), "obj", "expected *ShardingDatabase"))
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
			r.Name, validationErr)
	}
	logger = logger.WithValues("name", cr.Name, "namespace", cr.Namespace)
	logger.Info("running create validation")

	//namespaces := db.GetWatchNamespaces()
	//_, containsNamespace := namespaces[r.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	//	if len(namespaces) != 0 && !containsNamespace {
	//	validationErr = append(validationErr,
	//		field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
	//			"Oracle database operator doesn't watch over this namespace"))
	//}

	validationErr = append(validationErr, cr.validateDbSecretConfig()...)

	if cr.Spec.IsTdeWallet == "enable" {
		if cr.Spec.DbSecret == nil || cr.Spec.DbSecret.TDE == nil {
			validationErr = append(validationErr,
				field.Required(field.NewPath("spec").Child("dbSecret").Child("tde"),
					"tde credentials must be set when isTdeWallet is enable"))
		}
		if (len(cr.Spec.FssStorageClass) == 0) && (len(cr.Spec.TdeWalletPvc) == 0) {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("fssStorageClass"), cr.Spec.FssStorageClass,
					"fssStorageClass or tdeWalletPvc must be set when isTdeWallet is enable"))

			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("tdeWalletPvc"), cr.Spec.TdeWalletPvc,
					"fssStorageClass or tdeWalletPvc must be set when isTdeWallet is enable"))
		}
	}

	if cr.Spec.IsTdeWallet != "" {
		if (strings.ToLower(strings.TrimSpace(cr.Spec.IsTdeWallet)) != "enable") && (strings.ToLower(strings.TrimSpace(cr.Spec.IsTdeWallet)) != "disable") {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("isTdeWallet"), cr.Spec.IsTdeWallet,
					"isTdeWallet must be either \"enable\" or \"disable\""))
		}
	}

	validationErrs1 = cr.validateShardIsDelete()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateFreeEdition()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateCatalogName()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	//	validationErrs1 = r.validateShardName()
	//	if validationErrs1 != nil {
	//		validationErr = append(validationErr, validationErrs1...)
	//	}

	validationErrs1 = cr.validateShardInfo()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateShardOperationRules()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateCatalogTopologyConsistency()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateShardAdvancedParams()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateCatalogAdvancedParams()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	// TODO(user): fill in your validation logic upon object creation.
	if len(validationErr) == 0 {
		logger.Info("create validation passed", "mode", detectShardingMode(&cr.Spec), "shards", len(cr.Spec.Shard), "shardInfo", len(cr.Spec.ShardInfo))
		return nil, nil
	}
	logger.Info("create validation failed", "errorCount", len(validationErr))

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		cr.Name, validationErr)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	logger := shardingdatabaselog.WithValues("webhook", "validateUpdate", "name", r.Name, "namespace", r.Namespace)
	logger.Info("running update validation")

	var validationErr field.ErrorList

	oldCR, ok1 := oldObj.(*ShardingDatabase)
	newCR, ok2 := newObj.(*ShardingDatabase)
	if !ok1 || !ok2 {
		validationErr = append(validationErr,
			field.Invalid(field.NewPath("objectType"),
				fmt.Sprintf("%T -> %T", oldObj, newObj),
				"expected *ShardingDatabase for both old and new objects"))
		logger.Info("update validation failed due to invalid object type")
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
			r.Name, validationErr)
	}

	oldMode := detectShardingMode(&oldCR.Spec)
	newMode := detectShardingMode(&newCR.Spec)
	oldRepl := normalizeReplicationType(&oldCR.Spec)
	newRepl := normalizeReplicationType(&newCR.Spec)
	oldSharding := normalizeShardingType(&oldCR.Spec)
	newSharding := normalizeShardingType(&newCR.Spec)

	if oldRepl != "" && newRepl != "" && oldRepl != newRepl {
		validationErr = append(validationErr,
			field.Forbidden(field.NewPath("spec").Child("replicationType"),
				fmt.Sprintf("replicationType is immutable after creation (old=%s, new=%s)", oldRepl, newRepl)))
	}
	if oldSharding != modeUnknown && newSharding != modeUnknown && oldSharding != newSharding {
		validationErr = append(validationErr,
			field.Forbidden(field.NewPath("spec").Child("shardingType"),
				fmt.Sprintf("shardingType is immutable after creation (old=%s, new=%s)", oldSharding, newSharding)))
	}

	if oldMode == modeSystem && (newMode == modeUser || newMode == modeComposite) {
		validationErr = append(validationErr,
			field.Forbidden(field.NewPath("spec").Child("shardInfo"),
				"Cannot switch from System Sharding to User-Defined/Composite after creation"))
	}

	specChanged := !reflect.DeepEqual(oldCR.Spec, newCR.Spec)
	if specChanged {
		if locked, lockGen, lockMsg := isControllerUpdateLocked(oldCR); locked {
			if overrideEnabled, overrideMsg := isUpdateLockOverrideEnabled(newCR, time.Now().UTC()); overrideEnabled {
				logger.Info("allowing spec update due to break-glass override", "observedGeneration", lockGen, "override", overrideMsg)
			} else {
				msg := fmt.Sprintf("spec updates are blocked while controller operation is in progress (reason=%s, observedGeneration=%d). %s",
					updateLockReason, lockGen, lockMsg)
				if strings.TrimSpace(overrideMsg) != "" {
					msg = msg + " Break-glass override rejected: " + overrideMsg
				}
				validationErr = append(validationErr,
					field.Forbidden(field.NewPath("spec"), msg),
				)
			}
		}
	}

	validationErrs1 := newCR.validateShardInfo()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateShardOperationRules()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateCatalogTopologyConsistency()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateShardAdvancedParams()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateCatalogAdvancedParams()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	if len(validationErr) == 0 {
		logger.Info("update validation passed", "oldMode", oldMode, "newMode", newMode)
		return nil, nil
	}
	logger.Info("update validation failed", "oldMode", oldMode, "newMode", newMode, "errorCount", len(validationErr))

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		r.Name, validationErr)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.WithValues("webhook", "validateDelete", "name", r.Name, "namespace", r.Namespace).
		Info("running delete validation")

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// ###### Validation Block #################

func (r *ShardingDatabase) validateShardIsDelete() field.ErrorList {

	var validationErrs field.ErrorList

	for pindex := range r.Spec.Shard {
		val := strings.ToLower(strings.TrimSpace(r.Spec.Shard[pindex].IsDelete))
		if val != deleteStateEnable && val != deleteStateDisable && val != deleteStateFailed {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shard").Child("isDelete"), r.Spec.Shard[pindex].IsDelete,
					"spec.shard[].isDelete must be one of: enable, disable, failed"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateFreeEdition() field.ErrorList {

	var validationErrs field.ErrorList
	if strings.ToLower(r.Spec.DbEdition) == "free" {
		// Shard Spec Checks
		for i := 0; i < len(r.Spec.Shard); i++ {
			for index, variable := range r.Spec.Shard[i].EnvVars {
				if variable.Name == "ORACLE_SID" {
					if strings.ToLower(variable.Value) != "free" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("shard").Child("envVars"), r.Spec.Shard[i].EnvVars[index].Name,
							"ORACLE_SID value must be set to free for free edition"))
					}
				}
				if variable.Name == "ORACLE_PDB" {
					if strings.ToLower(variable.Value) != "freepdb" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("shard").Child("envVars"), r.Spec.Shard[i].EnvVars[index].Name,
							"ORACLE_PDB value must be set to freepdb for free edition"))
					}
				}
			}
		}
		// Catalog Spec Checks
		for i := 0; i < len(r.Spec.Catalog); i++ {
			for index, variable := range r.Spec.Catalog[i].EnvVars {
				if variable.Name == "ORACLE_SID" {
					if strings.ToLower(variable.Value) != "free" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("catalog").Child("envVars"), r.Spec.Catalog[i].EnvVars[index].Name,
							"ORACLE_SID value must be set to free for free edition"))
					}
				}
				if variable.Name == "ORACLE_PDB" {
					if strings.ToLower(variable.Value) != "freepdb" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("catalog").Child("envVars"), r.Spec.Catalog[i].EnvVars[index].Name,
							"ORACLE_PDB value must be set to freepdb for free edition"))
					}
				}
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateShardName() field.ErrorList {
	var validationErrs field.ErrorList

	for pindex := range r.Spec.Shard {
		if len(r.Spec.Shard[pindex].Name) > 9 {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shard").Child("name"), r.Spec.Shard[pindex].Name,
					"shard name must not exceed 9 characters"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateCatalogName() field.ErrorList {
	var validationErrs field.ErrorList

	for pindex := range r.Spec.Catalog {
		if len(r.Spec.Catalog[pindex].Name) > 9 {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("catalog").Child("name"), r.Spec.Catalog[pindex].Name,
					"catalog name must not exceed 9 characters"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateShardInfo() field.ErrorList {
	var validationErrs field.ErrorList
	var replicas int32
	replType := normalizeReplicationType(&r.Spec)
	modeHint := normalizeShardingType(&r.Spec)

	typeCounts := struct {
		groupOnly int
		spaceOnly int
		both      int
	}{}

	sysPrimaryGroups := 0
	spacePrimaryGroupCount := map[string]int{}

	for pindex := range r.Spec.ShardInfo {
		replicas = getShardInfoCount(&r.Spec.ShardInfo[pindex])
		if replicas == 0 {
			replicas = 1
			r.Spec.ShardInfo[pindex].ShardNum = replicas
		}

		sg := r.Spec.ShardInfo[pindex].ShardGroupDetails
		ss := r.Spec.ShardInfo[pindex].ShardSpaceDetails

		hasGroup := sg != nil && strings.TrimSpace(sg.Name) != ""
		hasSpace := ss != nil && strings.TrimSpace(ss.Name) != ""

		switch modeHint {
		case modeUser:
			if hasGroup {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails"),
						"user sharding shardInfo must not include shardGroupDetails"))
			}
			if !hasSpace {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardSpaceDetails"),
						"user sharding shardInfo requires shardSpaceDetails"))
			}
		case modeSystem:
			if !hasGroup {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails"),
						"system sharding shardInfo requires shardGroupDetails"))
			}
			if hasSpace {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardSpaceDetails"),
						"system sharding shardInfo must not include shardSpaceDetails"))
			}
		case modeComposite:
			if !hasGroup {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails"),
						"composite sharding shardInfo requires shardGroupDetails"))
			}
			if !hasSpace {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardSpaceDetails"),
						"composite sharding shardInfo requires shardSpaceDetails"))
			}
		}

		switch {
		case hasGroup && !hasSpace:
			typeCounts.groupOnly++
			deployAs := strings.ToUpper(strings.TrimSpace(sg.DeployAs))
			if deployAs != "" && !isValidDeployAs(deployAs) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						sg.DeployAs,
						"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
			}
			if replType == replNative && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						"deployAs is not supported for NATIVE replication"))
			}
			if replType == replDG && deployAs == "" {
				sg.DeployAs = "STANDBY"
				deployAs = "STANDBY"
			}
			if strings.EqualFold(deployAs, "PRIMARY") {
				sysPrimaryGroups++
			}

		case !hasGroup && hasSpace:
			typeCounts.spaceOnly++
			if sg != nil {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails"),
						sg,
						"User-defined sharding: shardGroupDetails must be omitted when only shardSpaceDetails is used"))
			}

		case hasGroup && hasSpace:
			typeCounts.both++
			deployAs := strings.ToUpper(strings.TrimSpace(sg.DeployAs))
			if deployAs != "" && !isValidDeployAs(deployAs) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						sg.DeployAs,
						"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
			}
			if replType == replNative && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						"deployAs is not supported for NATIVE replication"))
			}
			if replType == replDG && deployAs == "" {
				sg.DeployAs = "STANDBY"
				deployAs = "STANDBY"
			}
			if strings.EqualFold(deployAs, "PRIMARY") {
				spacePrimaryGroupCount[strings.TrimSpace(ss.Name)]++
			}

		default:
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex),
					r.Spec.ShardInfo[pindex],
					"Each shardInfo entry must define shardGroupDetails (system), shardSpaceDetails (user), or both (composite)"))
		}
	}

	if replType == replDG && typeCounts.groupOnly > 0 && typeCounts.spaceOnly == 0 && typeCounts.both == 0 {
		if sysPrimaryGroups != 1 {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("deployAs"),
					"PRIMARY",
					"System sharding: exactly one shardGroup must be PRIMARY"))
		}
	}

	if replType == replDG && (typeCounts.both > 0 || (typeCounts.groupOnly > 0 && typeCounts.spaceOnly > 0)) {
		for sp, cnt := range spacePrimaryGroupCount {
			if cnt > 1 {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardSpaceDetails").Child("name"),
						sp,
						"Composite sharding: each shardSpace can have only one PRIMARY shardGroup"))
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func isValidDeployAs(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "PRIMARY", "STANDBY", "ACTIVE_STANDBY":
		return true
	default:
		return false
	}
}

func normalizeReplicationValue(v string) string {
	rep := strings.ToUpper(strings.TrimSpace(v))
	switch rep {
	case replNative, "RAFT", "RAFTREPLICATION", "RAFTREPLICATIN":
		return replNative
	case replDG:
		return replDG
	default:
		return ""
	}
}

func normalizeShardingValue(v string) string {
	s := strings.ToUpper(strings.TrimSpace(v))
	switch s {
	case "SYSTEM", "USER", "COMPOSITE":
		return s
	default:
		return ""
	}
}

func shardingModeToSpecValue(mode shardingMode) string {
	switch mode {
	case modeSystem:
		return "SYSTEM"
	case modeUser:
		return "USER"
	case modeComposite:
		return "COMPOSITE"
	default:
		return ""
	}
}

func applyGlobalShardingReplicationDefaults(spec *ShardingDatabaseSpec) {
	if spec == nil {
		return
	}

	topSharding := normalizeShardingValue(spec.ShardingType)
	if topSharding == "" {
		for i := range spec.Catalog {
			if v := normalizeShardingValue(spec.Catalog[i].Sharding); v != "" {
				topSharding = v
				break
			}
		}
	}
	if topSharding == "" {
		topSharding = shardingModeToSpecValue(detectShardingMode(spec))
	}
	if topSharding != "" {
		spec.ShardingType = topSharding
	}

	topRepl := normalizeReplicationValue(spec.ReplicationType)
	if topRepl == "" {
		for i := range spec.Catalog {
			if v := normalizeReplicationValue(spec.Catalog[i].Repl); v != "" {
				topRepl = v
				break
			}
		}
	}
	if topRepl == "" {
		topRepl = replDG
	}
	spec.ReplicationType = topRepl

	for i := range spec.Catalog {
		if topSharding != "" {
			if v := normalizeShardingValue(spec.Catalog[i].Sharding); v == "" {
				spec.Catalog[i].Sharding = topSharding
			} else {
				spec.Catalog[i].Sharding = v
			}
		}
		if v := normalizeReplicationValue(spec.Catalog[i].Repl); v == "" {
			spec.Catalog[i].Repl = topRepl
		} else {
			spec.Catalog[i].Repl = v
		}
	}
}

func (r *ShardingDatabase) validateCatalogTopologyConsistency() field.ErrorList {
	var validationErrs field.ErrorList

	topSharding := normalizeShardingValue(r.Spec.ShardingType)
	topRepl := normalizeReplicationValue(r.Spec.ReplicationType)
	if topRepl == "" {
		topRepl = replDG
	}

	for i := range r.Spec.Catalog {
		cat := r.Spec.Catalog[i]
		if csh := normalizeShardingValue(cat.Sharding); topSharding != "" && csh != "" && topSharding != csh {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("catalog").Index(i).Child("sharding"),
					cat.Sharding,
					"catalog.sharding must match spec.shardingType when both are set"))
		}

		if crepl := normalizeReplicationValue(cat.Repl); crepl != "" && topRepl != crepl {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("catalog").Index(i).Child("repl"),
					cat.Repl,
					"catalog.repl must match spec.replicationType when both are set"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func normalizeReplicationType(spec *ShardingDatabaseSpec) string {
	if spec == nil {
		return replDG
	}
	if rep := normalizeReplicationValue(spec.ReplicationType); rep != "" {
		return rep
	}
	return replDG
}

func (r *ShardingDatabase) validateShardOperationRules() field.ErrorList {
	var validationErrs field.ErrorList
	replType := normalizeReplicationType(&r.Spec)
	modeHint := normalizeShardingType(&r.Spec)

	for i := range r.Spec.Shard {
		sh := r.Spec.Shard[i]
		hasGroup := strings.TrimSpace(sh.ShardGroup) != ""
		hasSpace := strings.TrimSpace(sh.ShardSpace) != ""
		mode := inferShardMode(modeHint, hasGroup, hasSpace)

		deployAsRaw := strings.TrimSpace(sh.DeployAs)
		deployAs := strings.ToUpper(deployAsRaw)
		if deployAs != "" && !isValidDeployAs(deployAs) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("deployAs"),
					sh.DeployAs,
					"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
		}
		if replType == replNative && deployAs != "" {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("deployAs"),
					"deployAs is not supported for NATIVE replication"))
		}

		switch mode {
		case modeUser:
			if hasGroup {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
						"user sharding add shard does not allow shardGroup"))
			}
			if !hasSpace {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardSpace"),
						"user sharding add shard requires shardSpace"))
			}
			if replType == replDG && deployAs == "" {
				r.Spec.Shard[i].DeployAs = "STANDBY"
			}

		case modeSystem:
			if !hasGroup {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
						"system sharding add shard requires shardGroup"))
			}
			if hasSpace {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("shardSpace"),
						"system sharding add shard cannot use shardSpace directly"))
			}
			if hasGroup && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("deployAs"),
						"deployAs cannot be combined with shardGroup in system sharding"))
			}

		case modeComposite:
			if !hasGroup {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
						"composite sharding add shard requires shardGroup"))
			}
			if !hasSpace {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardSpace"),
						"composite sharding add shard requires shardSpace"))
			}
			if hasGroup && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("deployAs"),
						"deployAs cannot be combined with shardGroup in composite sharding"))
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateShardAdvancedParams() field.ErrorList {
	var validationErrs field.ErrorList
	replType := normalizeReplicationType(&r.Spec)

	for i := range r.Spec.Shard {
		sh := r.Spec.Shard[i]
		if strings.TrimSpace(sh.GgService) != "" && replType != replNative {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("ggService"),
					"ggService is only supported for NATIVE replication"))
		}
		if strings.TrimSpace(sh.Replace) != "" && replType != replNative {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("replace"),
					"replace is only supported for NATIVE replication"))
		}
		if sh.CloneSchemas && replType != replNative {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shard").Index(i).Child("cloneSchemas"),
					"cloneSchemas is only supported for NATIVE replication"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateCatalogAdvancedParams() field.ErrorList {
	var validationErrs field.ErrorList
	topRepl := normalizeReplicationType(&r.Spec)
	topMode := normalizeShardingType(&r.Spec)

	for i := range r.Spec.Catalog {
		cat := r.Spec.Catalog[i]
		replType := topRepl
		if v := normalizeReplicationValue(cat.Repl); v != "" {
			replType = v
		}
		mode := topMode
		if v := normalizeShardingValue(cat.Sharding); v != "" {
			mode = shardingMode(strings.ToLower(v))
		}

		if replType != replNative {
			if cat.MultiWriter {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("multiwriter"),
						"multiwriter is only supported for NATIVE replication"))
			}
			if cat.RepFactor > 0 {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("repFactor"),
						"repFactor is only supported for NATIVE replication"))
			}
			if cat.RepUnits > 0 {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("repUnits"),
						"repUnits is only supported for NATIVE replication"))
			}
		}
		if replType == replNative && mode == modeUser && cat.RepFactor > 0 {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("repFactor"),
					"repFactor is not applicable for USER sharding catalog"))
		}

		if replType == replNative && strings.TrimSpace(cat.ProtectMode) != "" {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("protectMode"),
					"protectMode is only supported for DG replication"))
		}

		if mode == modeUser && cat.Chunks > 0 {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("catalog").Index(i).Child("chunks"),
					"chunks is not applicable for USER sharding catalog"))
		}

		if cat.UseExistingCatalog && cat.CatalogDatabaseRef == nil {
			validationErrs = append(validationErrs,
				field.Required(field.NewPath("spec").Child("catalog").Index(i).Child("catalogDatabaseRef"),
					"catalogDatabaseRef is required when useExistingCatalog is true"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func normalizeShardingType(spec *ShardingDatabaseSpec) shardingMode {
	if spec == nil {
		return modeUnknown
	}
	mode := strings.ToLower(strings.TrimSpace(spec.ShardingType))
	switch mode {
	case string(modeSystem):
		return modeSystem
	case string(modeUser):
		return modeUser
	case string(modeComposite):
		return modeComposite
	default:
		return detectShardingMode(spec)
	}
}

func inferShardMode(hint shardingMode, hasGroup, hasSpace bool) shardingMode {
	if hint != modeUnknown {
		return hint
	}
	switch {
	case hasGroup && hasSpace:
		return modeComposite
	case hasGroup:
		return modeSystem
	case hasSpace:
		return modeUser
	default:
		return modeUnknown
	}
}

func (r *ShardingDatabase) initShardsSpec() error {
	shardIndex := 0

	for pindex := range r.Spec.ShardInfo {
		replicas := getShardInfoCount(&r.Spec.ShardInfo[pindex])
		if replicas == 0 {
			replicas = 2
		}

		for i := 0; i < int(replicas); i++ {
			r.Spec.Shard[shardIndex].Name = r.Spec.ShardInfo[pindex].ShardPreFixName + strconv.Itoa(shardIndex+1)
			r.Spec.Shard[shardIndex].StorageSizeInGb = r.Spec.ShardInfo[pindex].StorageSizeInGb

			if r.Spec.ShardInfo[pindex].ShardGroupDetails != nil {
				r.Spec.Shard[shardIndex].ShardGroup = r.Spec.ShardInfo[pindex].ShardGroupDetails.Name
				r.Spec.Shard[shardIndex].ShardRegion = r.Spec.ShardInfo[pindex].ShardGroupDetails.Region
				r.Spec.Shard[shardIndex].DeployAs = r.Spec.ShardInfo[pindex].ShardGroupDetails.DeployAs
				r.Spec.Shard[shardIndex].IsDelete = r.Spec.ShardInfo[pindex].ShardGroupDetails.IsDelete
			}

			r.Spec.Shard[shardIndex].PrimaryDatabaseRef = r.Spec.ShardInfo[pindex].PrimaryDatabaseRef

			r.Spec.Shard[shardIndex].ImagePulllPolicy = new(corev1.PullPolicy)
			*(r.Spec.Shard[shardIndex].ImagePulllPolicy) = corev1.PullPolicy("Always")

			if r.Spec.ShardInfo[pindex].ShardSpaceDetails != nil {
				r.Spec.Shard[shardIndex].ShardSpace = r.Spec.ShardInfo[pindex].ShardSpaceDetails.Name
			}

			// Apply shape defaults
			if cfg, ok := shapes.LookupShapeConfig(r.Spec.ShardInfo[pindex].Shape); ok {
				r.Spec.Shard[shardIndex].EnvVars = upsertEnvVars(
					r.Spec.Shard[shardIndex].EnvVars,
					envVarsFromPairs(cfg.EnvPairs()),
					true,
				)
				r.Spec.Shard[shardIndex].Resources = cfg.ResourceRequirements()
			}

			// Explicit shardInfo env/resources override shape defaults
			if len(r.Spec.ShardInfo[pindex].EnvVars) > 0 {
				r.Spec.Shard[shardIndex].EnvVars = upsertEnvVars(
					r.Spec.Shard[shardIndex].EnvVars,
					r.Spec.ShardInfo[pindex].EnvVars,
					true,
				)
			}
			if r.Spec.ShardInfo[pindex].Resources != nil {
				r.Spec.Shard[shardIndex].Resources = r.Spec.ShardInfo[pindex].Resources
			}

			fmt.Println("ShardName=[" + r.Spec.Shard[shardIndex].Name + "]")
			shardIndex++
		}
	}

	return nil
}
func (r *ShardingDatabase) buildDesiredShardSpec() []ShardSpec {
	tmp := &ShardingDatabase{}
	tmp.Spec = r.Spec

	totalShard := countTotalReplicas(tmp.Spec.ShardInfo)
	tmp.Spec.Shard = make([]ShardSpec, totalShard)
	_ = tmp.initShardsSpec()

	return tmp.Spec.Shard
}

func mergeDesiredAndExistingShards(existing []ShardSpec, desired []ShardSpec) []ShardSpec {
	existingByName := map[string]ShardSpec{}
	for _, s := range existing {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		existingByName[name] = s
	}

	desiredNames := map[string]bool{}
	out := make([]ShardSpec, 0, len(existing)+len(desired))

	for _, d := range desired {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		desiredNames[name] = true

		if old, ok := existingByName[name]; ok {
			merged := old

			// refresh generated/defaulted fields from desired
			merged.Name = d.Name
			merged.StorageSizeInGb = d.StorageSizeInGb
			merged.ShardGroup = d.ShardGroup
			merged.ShardRegion = d.ShardRegion
			merged.DeployAs = d.DeployAs
			merged.PrimaryDatabaseRef = d.PrimaryDatabaseRef
			merged.ImagePulllPolicy = d.ImagePulllPolicy
			merged.ShardSpace = d.ShardSpace
			merged.EnvVars = d.EnvVars
			merged.Resources = d.Resources

			// preserve controller-marked delete flag if already set
			if strings.TrimSpace(strings.ToLower(old.IsDelete)) != "" {
				merged.IsDelete = old.IsDelete
			} else {
				merged.IsDelete = d.IsDelete
			}

			out = append(out, merged)
		} else {
			out = append(out, d)
		}
	}

	// preserve extra old shards during scale-in so controller can mark/delete them properly
	extras := make([]ShardSpec, 0)
	for _, s := range existing {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		if desiredNames[name] {
			continue
		}
		extras = append(extras, s)
	}

	sort.Slice(extras, func(i, j int) bool {
		return shardOrdinalWebhook(extras[i].Name) < shardOrdinalWebhook(extras[j].Name)
	})

	out = append(out, extras...)
	return out
}

func shardOrdinalWebhook(name string) int {
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

func detectShardingMode(spec *ShardingDatabaseSpec) shardingMode {
	hasGroupOnly := false
	hasSpaceOnly := false
	hasBoth := false

	for i := range spec.ShardInfo {
		sg := spec.ShardInfo[i].ShardGroupDetails
		ss := spec.ShardInfo[i].ShardSpaceDetails

		hasGroup := sg != nil && strings.TrimSpace(sg.Name) != ""
		hasSpace := ss != nil && strings.TrimSpace(ss.Name) != ""

		switch {
		case hasGroup && !hasSpace:
			hasGroupOnly = true
		case !hasGroup && hasSpace:
			hasSpaceOnly = true
		case hasGroup && hasSpace:
			hasBoth = true
		}
	}

	if hasBoth || (hasGroupOnly && hasSpaceOnly) {
		return modeComposite
	}
	if hasGroupOnly {
		return modeSystem
	}
	if hasSpaceOnly {
		return modeUser
	}
	return modeUnknown
}

func upsertEnvVars(base []EnvironmentVariable, add []EnvironmentVariable, overwrite bool) []EnvironmentVariable {
	if base == nil {
		return append([]EnvironmentVariable{}, add...)
	}

	idx := map[string]int{}
	for i, e := range base {
		idx[strings.ToLower(strings.TrimSpace(e.Name))] = i
	}

	for _, e := range add {
		k := strings.ToLower(strings.TrimSpace(e.Name))
		if pos, ok := idx[k]; ok {
			if overwrite {
				base[pos].Value = e.Value
			}
		} else {
			base = append(base, e)
		}
	}

	return base
}

func envVarsFromPairs(pairs [][2]string) []EnvironmentVariable {
	out := make([]EnvironmentVariable, 0, len(pairs))
	for _, kv := range pairs {
		out = append(out, EnvironmentVariable{
			Name:  kv[0],
			Value: kv[1],
		})
	}
	return out
}

func countTotalReplicas(shardInfo []ShardingDetails) int32 {
	var total int32
	for i := range shardInfo {
		replicas := getShardInfoCount(&shardInfo[i])
		if replicas == 0 {
			replicas = 2
		}
		total += replicas
	}
	return total
}

func getShardInfoCount(info *ShardingDetails) int32 {
	if info == nil {
		return 0
	}
	if info.ShardNum > 0 {
		return info.ShardNum
	}
	if info.Replicas > 0 {
		return info.Replicas
	}
	return 0
}
