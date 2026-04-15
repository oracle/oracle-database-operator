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

//nolint:unused // legacy validation helpers are retained for staged rollout and backward compatibility.
package v4

// revive:disable:unused-parameter,exported
// Legacy webhook signatures are preserved for interface compatibility.

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	shapes "github.com/oracle/oracle-database-operator/commons/shapes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

	reconcilingConditionType = lockpolicy.DefaultReconcilingConditionType
	updateLockReason         = lockpolicy.DefaultUpdateLockReason
	lockOverrideAnnotation   = lockpolicy.DefaultOverrideAnnotation
)

// log is for logging in this package.
var shardingdatabaselog = logf.Log.WithName("shardingdatabase-resource")

func isControllerUpdateLocked(cr *ShardingDatabase) (bool, int64, string) {
	if cr == nil {
		return false, 0, ""
	}
	return lockpolicy.IsControllerUpdateLocked(cr.Status.CrdStatus, reconcilingConditionType, updateLockReason)
}

func isUpdateLockOverrideEnabled(cr *ShardingDatabase) (bool, string) {
	if cr == nil {
		return false, "resource is nil"
	}
	return lockpolicy.IsUpdateLockOverrideEnabled(cr.GetAnnotations(), lockOverrideAnnotation)
}

func (r *ShardingDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

var _ admission.Defaulter[*ShardingDatabase] = &ShardingDatabase{}
var _ admission.Validator[*ShardingDatabase] = &ShardingDatabase{}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-shardingdatabase,mutating=true,failurePolicy=fail,sideEffects=none,groups=database.oracle.com,resources=shardingdatabases,verbs=create;update,versions=v4,name=mshardingdatabasev4.kb.io,admissionReviewVersions=v1

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *ShardingDatabase) Default(ctx context.Context, obj *ShardingDatabase) error {
	cr := obj

	logger := shardingdatabaselog.WithValues("name", cr.Name, "namespace", cr.Namespace)
	logger.Info("applying shardingdatabase defaults")

	if strings.TrimSpace(cr.Spec.GsmDevMode) == "" {
		cr.Spec.GsmDevMode = "dev"
	}
	if strings.TrimSpace(getTDEWalletEnabledFromSpec(&cr.Spec)) == "" {
		cr.Spec.IsTdeWallet = "disable"
	}
	if cr.Spec.Dataguard == nil {
		cr.Spec.Dataguard = &DataguardProducerSpec{}
	}
	cr.Spec.Dataguard.Mode = normalizeDataguardProducerMode(cr.Spec.Dataguard)

	applyGlobalShardingReplicationDefaults(&cr.Spec)

	for i := range cr.Spec.Shard {
		if cr.Spec.Shard[i].StorageSizeInGb <= 0 {
			cr.Spec.Shard[i].StorageSizeInGb = DefaultDiagSizeInGb
		}
		if strings.TrimSpace(strings.ToLower(cr.Spec.Shard[i].IsDelete)) == "" {
			cr.Spec.Shard[i].IsDelete = "disable"
		}
		cr.Spec.Shard[i].DeployAs = normalizeDeployAsCanonical(cr.Spec.Shard[i].DeployAs)
		defaultAdditionalPVCs(&cr.Spec.Shard[i].AdditionalPVCs)
	}

	for i := range cr.Spec.ShardInfo {
		if cr.Spec.ShardInfo[i].StorageSizeInGb <= 0 {
			cr.Spec.ShardInfo[i].StorageSizeInGb = DefaultDiagSizeInGb
		}
		if cr.Spec.ShardInfo[i].ShardGroupDetails != nil &&
			strings.TrimSpace(strings.ToLower(cr.Spec.ShardInfo[i].ShardGroupDetails.IsDelete)) == "" {
			cr.Spec.ShardInfo[i].ShardGroupDetails.IsDelete = "disable"
		}
		if cr.Spec.ShardInfo[i].ShardGroupDetails != nil {
			cr.Spec.ShardInfo[i].ShardGroupDetails.DeployAs = normalizeDeployAsCanonical(cr.Spec.ShardInfo[i].ShardGroupDetails.DeployAs)
		}
		if cr.Spec.ShardInfo[i].ShardSpaceDetails != nil {
			cr.Spec.ShardInfo[i].ShardSpaceDetails.DeployAs = normalizeDeployAsCanonical(cr.Spec.ShardInfo[i].ShardSpaceDetails.DeployAs)
		}
		defaultAdditionalPVCs(&cr.Spec.ShardInfo[i].AdditionalPVCs)
	}

	var totalShard int32
	modeHint := normalizeShardingType(&cr.Spec)
	for i := range cr.Spec.ShardInfo {
		count := getShardInfoCountByMode(modeHint, &cr.Spec.ShardInfo[i])
		if count == 0 {
			if modeHint == modeUser {
				count = 1
			} else {
				count = 2
			}
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
		if cr.Spec.Catalog[i].StorageSizeInGb <= 0 {
			cr.Spec.Catalog[i].StorageSizeInGb = DefaultDiagSizeInGb
		}
		defaultAdditionalPVCs(&cr.Spec.Catalog[i].AdditionalPVCs)
		if cfg, ok := shapes.LookupShapeConfig(cr.Spec.Catalog[i].Shape); ok {
			cr.Spec.Catalog[i].EnvVars = upsertEnvVars(
				cr.Spec.Catalog[i].EnvVars,
				envVarsFromPairs(cfg.EnvPairs()),
				true,
			)
			cr.Spec.Catalog[i].Resources = cfg.ResourceRequirements()
		}
	}

	applyGsmInfoDefaults(&cr.Spec)
	gsmInlineDefaults := extractAndStripInlineGsmDefaults(&cr.Spec)
	gsmDefaultResources := cr.Spec.GsmResources
	if gsmInlineDefaults.resources != nil {
		gsmDefaultResources = gsmInlineDefaults.resources
	}
	for i := range cr.Spec.Gsm {
		defaultAdditionalPVCs(&cr.Spec.Gsm[i].AdditionalPVCs)
		if cr.Spec.Gsm[i].StorageSizeInGb <= 0 && gsmInlineDefaults.storageSizeInGb > 0 {
			cr.Spec.Gsm[i].StorageSizeInGb = gsmInlineDefaults.storageSizeInGb
		}
		if cr.Spec.Gsm[i].StorageSizeInGb <= 0 {
			cr.Spec.Gsm[i].StorageSizeInGb = DefaultDiagSizeInGb
		}
		if cr.Spec.Gsm[i].ImagePulllPolicy == nil && gsmInlineDefaults.imagePullPolicy != nil {
			p := *gsmInlineDefaults.imagePullPolicy
			cr.Spec.Gsm[i].ImagePulllPolicy = &p
		}
		if cr.Spec.Gsm[i].Resources == nil && gsmDefaultResources != nil {
			cr.Spec.Gsm[i].Resources = gsmDefaultResources.DeepCopy()
		}
	}

	return nil
}

type inlineGsmDefaults struct {
	resources       *corev1.ResourceRequirements
	storageSizeInGb int32
	imagePullPolicy *corev1.PullPolicy
}

func applyGsmInfoDefaults(spec *ShardingDatabaseSpec) {
	if spec == nil || spec.GsmInfo == nil || len(spec.GsmInfo.Gsm) == 0 {
		return
	}

	common := spec.GsmInfo
	materialized := make([]GsmSpec, len(common.Gsm))
	for i := range common.Gsm {
		item := common.Gsm[i]

		if item.Resources == nil {
			if common.Resources != nil {
				item.Resources = common.Resources.DeepCopy()
			} else if spec.GsmResources != nil {
				item.Resources = spec.GsmResources.DeepCopy()
			}
		}

		if item.StorageSizeInGb <= 0 && common.StorageSizeInGb > 0 {
			item.StorageSizeInGb = common.StorageSizeInGb
		}

		if item.ImagePulllPolicy == nil && common.ImagePulllPolicy != nil {
			p := *common.ImagePulllPolicy
			item.ImagePulllPolicy = &p
		}

		item.EnvVars = upsertEnvVars(append([]EnvironmentVariable(nil), common.EnvVars...), item.EnvVars, true)
		item.ServiceAnnotations = mergeStringMapLocal(common.ServiceAnnotations, item.ServiceAnnotations)
		item.ExternalServiceAnnotations = mergeStringMapLocal(common.ExternalServiceAnnotations, item.ExternalServiceAnnotations)

		materialized[i] = item
	}

	spec.Gsm = materialized
}

func mergeStringMapLocal(base map[string]string, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func extractAndStripInlineGsmDefaults(spec *ShardingDatabaseSpec) inlineGsmDefaults {
	out := inlineGsmDefaults{}
	if spec == nil || len(spec.Gsm) == 0 {
		return out
	}

	kept := make([]GsmSpec, 0, len(spec.Gsm))
	for i := range spec.Gsm {
		g := spec.Gsm[i]
		if isInlineGsmDefaultsEntry(g) {
			if g.Resources != nil {
				out.resources = g.Resources.DeepCopy()
			}
			if g.StorageSizeInGb > 0 {
				out.storageSizeInGb = g.StorageSizeInGb
			}
			if g.ImagePulllPolicy != nil {
				p := *g.ImagePulllPolicy
				out.imagePullPolicy = &p
			}
			continue
		}
		kept = append(kept, g)
	}
	spec.Gsm = kept
	return out
}

func isInlineGsmDefaultsEntry(g GsmSpec) bool {
	if strings.TrimSpace(g.Name) != "" {
		return false
	}
	if g.Resources == nil && g.StorageSizeInGb <= 0 && g.ImagePulllPolicy == nil {
		return false
	}
	if len(g.EnvVars) > 0 || strings.TrimSpace(g.PvcName) != "" ||
		strings.TrimSpace(g.Label) != "" || strings.TrimSpace(g.IsDelete) != "" || len(g.NodeSelector) > 0 ||
		len(g.PvAnnotations) > 0 || len(g.PvMatchLabels) > 0 ||
		strings.TrimSpace(g.Region) != "" || strings.TrimSpace(g.DirectorName) != "" || g.GsmConfigData != nil ||
		g.GsmNum > 0 || strings.TrimSpace(g.GsmPrefix) != "" || strings.TrimSpace(g.Shape) != "" ||
		len(g.Regions) > 0 || g.RemoteOns > 0 || g.LocalOns > 0 || g.Listener > 0 ||
		strings.TrimSpace(g.Endpoint) != "" || strings.TrimSpace(g.RemoteEndpoint) != "" ||
		strings.TrimSpace(g.TraceLevel) != "" || strings.TrimSpace(g.Encryption) != "" ||
		strings.TrimSpace(g.Catalog) != "" || strings.TrimSpace(g.Pwd) != "" || strings.TrimSpace(g.WalletPassword) != "" ||
		len(g.AdditionalPVCs) > 0 || g.SecurityContext != nil || g.Capabilities != nil {
		return false
	}
	// allow only defaults-carrier fields on unnamed gsm item
	if g.StorageSizeInGb > 0 || g.ImagePulllPolicy != nil || g.Resources != nil {
		return true
	}
	return true
}

func defaultPasswordSecretConfig(secret *SecretDetails) {
	if secret == nil {
		return
	}
	if strings.TrimSpace(secret.MountPath) == "" {
		secret.MountPath = DefaultSecretMountPath
	}
	if strings.TrimSpace(secret.UseGsmWallet) == "" {
		secret.UseGsmWallet = "true"
	}
	if strings.TrimSpace(secret.GsmWalletRoot) == "" {
		secret.GsmWalletRoot = DefaultGsmWalletRoot
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

func defaultAdditionalPVCs(pvcs *[]AdditionalPVCSpec) {
	if pvcs == nil {
		return
	}
	for i := range *pvcs {
		(*pvcs)[i].MountPath = strings.TrimSpace((*pvcs)[i].MountPath)
		(*pvcs)[i].PvcName = strings.TrimSpace((*pvcs)[i].PvcName)
		(*pvcs)[i].StorageClass = strings.TrimSpace((*pvcs)[i].StorageClass)
		if (*pvcs)[i].PvcName != "" {
			continue
		}
		switch (*pvcs)[i].MountPath {
		case DefaultDiagMountPath:
			if (*pvcs)[i].StorageSizeInGb <= 0 {
				(*pvcs)[i].StorageSizeInGb = DefaultDiagSizeInGb
			}
		case DefaultGsmDiagMountPath:
			if (*pvcs)[i].StorageSizeInGb <= 0 {
				(*pvcs)[i].StorageSizeInGb = DefaultDiagSizeInGb
			}
		case DefaultGddLogMountPath:
			if (*pvcs)[i].StorageSizeInGb <= 0 {
				(*pvcs)[i].StorageSizeInGb = DefaultGddLogSizeInGb
			}
		}
	}
}

type effectivePVCSpec struct {
	MountPath       string
	PvcName         string
	StorageSizeInGb int32
}

func mergeAdditionalPVCs(primarySize int32, baseMountPath string, extras []AdditionalPVCSpec) map[string]effectivePVCSpec {
	result := map[string]effectivePVCSpec{
		baseMountPath: {
			MountPath:       baseMountPath,
			StorageSizeInGb: primarySize,
		},
	}

	for i := range extras {
		mountPath := strings.TrimSpace(extras[i].MountPath)
		if mountPath == "" {
			continue
		}
		cfg := result[mountPath]
		cfg.MountPath = mountPath
		if pvcName := strings.TrimSpace(extras[i].PvcName); pvcName != "" {
			cfg.PvcName = pvcName
		}
		if extras[i].StorageSizeInGb > 0 {
			cfg.StorageSizeInGb = extras[i].StorageSizeInGb
		}
		result[mountPath] = cfg
	}

	return result
}

func validateAdditionalPVCEntries(entries []AdditionalPVCSpec, baseMountPath, diagMountPath string, path *field.Path) field.ErrorList {
	var errs field.ErrorList
	seenPaths := map[string]int{}

	for i := range entries {
		itemPath := path.Index(i)
		mountPath := strings.TrimSpace(entries[i].MountPath)
		pvcName := strings.TrimSpace(entries[i].PvcName)

		if mountPath == "" {
			errs = append(errs, field.Required(itemPath.Child("mountPath"), "mountPath must be set"))
			continue
		}
		if !strings.HasPrefix(mountPath, "/") {
			errs = append(errs, field.Invalid(itemPath.Child("mountPath"), entries[i].MountPath, "mountPath must be an absolute path"))
		}
		if prev, found := seenPaths[mountPath]; found {
			errs = append(errs, field.Duplicate(itemPath.Child("mountPath"), fmt.Sprintf("duplicate mountPath already set at index %d", prev)))
		} else {
			seenPaths[mountPath] = i
		}

		isDefaultPath := mountPath == baseMountPath || mountPath == diagMountPath || mountPath == DefaultGddLogMountPath
		if pvcName == "" && entries[i].StorageSizeInGb <= 0 && !isDefaultPath {
			errs = append(errs, field.Required(itemPath.Child("storageSizeInGb"), "storageSizeInGb must be greater than 0 when pvcName is not provided"))
		}
		if mountPath != baseMountPath && pvcName == "" && entries[i].StorageSizeInGb <= 0 && isDefaultPath {
			errs = append(errs, field.Required(itemPath.Child("storageSizeInGb"), "storageSizeInGb must be greater than 0 when pvcName is not provided"))
		}
	}
	return errs
}

func validateAdditionalPVCUpdate(
	oldMap map[string]effectivePVCSpec,
	newMap map[string]effectivePVCSpec,
	path *field.Path,
) field.ErrorList {
	var errs field.ErrorList

	for mountPath, oldCfg := range oldMap {
		newCfg, found := newMap[mountPath]
		if !found {
			if oldCfg.PvcName == "" {
				errs = append(errs, field.Forbidden(path, fmt.Sprintf("cannot remove auto-templated mountPath %s after creation", mountPath)))
			}
			continue
		}

		oldTemplate := oldCfg.PvcName == ""
		newTemplate := newCfg.PvcName == ""
		if oldTemplate != newTemplate {
			errs = append(errs, field.Forbidden(path, fmt.Sprintf("cannot switch mountPath %s between template PVC and user pvcName", mountPath)))
			continue
		}

		if oldTemplate && newTemplate && newCfg.StorageSizeInGb < oldCfg.StorageSizeInGb {
			errs = append(errs, field.Forbidden(path, fmt.Sprintf("cannot shrink storage for mountPath %s from %dGi to %dGi", mountPath, oldCfg.StorageSizeInGb, newCfg.StorageSizeInGb)))
		}
	}

	for mountPath, newCfg := range newMap {
		if _, found := oldMap[mountPath]; found {
			continue
		}
		if newCfg.PvcName == "" {
			errs = append(errs, field.Forbidden(path, fmt.Sprintf("cannot add new auto-templated mountPath %s after creation; use pvcName", mountPath)))
		}
	}

	return errs
}

func computeSizingPath(shape string, resources *corev1.ResourceRequirements) string {
	hasShape := strings.TrimSpace(shape) != ""
	hasResources := resources != nil

	switch {
	case hasShape:
		return "shape"
	case hasResources:
		return "resources"
	default:
		return "none"
	}
}

func (r *ShardingDatabase) validateComputeSizingPathConfig() field.ErrorList {
	var validationErrs field.ErrorList

	for i := range r.Spec.Catalog {
		hasShape := strings.TrimSpace(r.Spec.Catalog[i].Shape) != ""
		hasResources := r.Spec.Catalog[i].Resources != nil
		if hasShape && hasResources {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("catalog").Index(i),
					"shape and resources are mutually exclusive; choose exactly one sizing path"))
		}
	}

	for i := range r.Spec.ShardInfo {
		hasShape := strings.TrimSpace(r.Spec.ShardInfo[i].Shape) != ""
		hasResources := r.Spec.ShardInfo[i].Resources != nil
		if hasShape && hasResources {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(i),
					"shape and resources are mutually exclusive; choose exactly one sizing path"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateComputeSizingPathUpdate(oldCR *ShardingDatabase) field.ErrorList {
	var validationErrs field.ErrorList
	if oldCR == nil {
		return nil
	}

	oldCatalogByName := map[string]CatalogSpec{}
	for i := range oldCR.Spec.Catalog {
		name := strings.TrimSpace(oldCR.Spec.Catalog[i].Name)
		if name == "" {
			continue
		}
		oldCatalogByName[name] = oldCR.Spec.Catalog[i]
	}
	for i := range r.Spec.Catalog {
		name := strings.TrimSpace(r.Spec.Catalog[i].Name)
		if name == "" {
			continue
		}
		oldSpec, found := oldCatalogByName[name]
		if !found {
			continue
		}
		oldPath := computeSizingPath(oldSpec.Shape, oldSpec.Resources)
		newPath := computeSizingPath(r.Spec.Catalog[i].Shape, r.Spec.Catalog[i].Resources)
		if oldPath != "none" && oldPath != newPath {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("catalog").Index(i),
					fmt.Sprintf("compute sizing path is immutable after creation (old=%s, new=%s)", oldPath, newPath)))
		}
	}

	oldShardInfoByPrefix := map[string]ShardingDetails{}
	for i := range oldCR.Spec.ShardInfo {
		prefix := strings.TrimSpace(oldCR.Spec.ShardInfo[i].ShardPreFixName)
		if prefix == "" {
			continue
		}
		oldShardInfoByPrefix[prefix] = oldCR.Spec.ShardInfo[i]
	}
	for i := range r.Spec.ShardInfo {
		prefix := strings.TrimSpace(r.Spec.ShardInfo[i].ShardPreFixName)
		if prefix == "" {
			continue
		}
		oldSpec, found := oldShardInfoByPrefix[prefix]
		if !found {
			continue
		}
		oldPath := computeSizingPath(oldSpec.Shape, oldSpec.Resources)
		newPath := computeSizingPath(r.Spec.ShardInfo[i].Shape, r.Spec.ShardInfo[i].Resources)
		if oldPath != "none" && oldPath != newPath {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(i),
					fmt.Sprintf("compute sizing path is immutable after creation (old=%s, new=%s)", oldPath, newPath)))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
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
	if useGsmWallet := strings.TrimSpace(r.Spec.DbSecret.UseGsmWallet); useGsmWallet != "" {
		v := strings.ToLower(useGsmWallet)
		if v != "true" && v != "false" {
			errs = append(errs, field.Invalid(dbSecretPath.Child("useGsmWallet"), r.Spec.DbSecret.UseGsmWallet, "useGsmWallet must be \"true\" or \"false\""))
		}
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

var _ admission.Validator[*ShardingDatabase] = &ShardingDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateCreate(ctx context.Context, obj *ShardingDatabase) (admission.Warnings, error) {
	logger := shardingdatabaselog.WithValues("webhook", "validateCreate")

	// TODO(user): fill in your validation logic upon object creation.
	// Check Secret configuration
	var validationErr field.ErrorList
	var validationErrs1 field.ErrorList
	cr := obj
	warnings := deprecatedLegacyPVCFieldWarnings(&cr.Spec)
	logger = logger.WithValues("name", cr.Name, "namespace", cr.Namespace)
	logger.Info("running create validation")
	validationErr = append(validationErr, validateDataguardProducerSpec(field.NewPath("spec").Child("dataguard"), cr.Spec.Dataguard)...)

	//namespaces := db.GetWatchNamespaces()
	//_, containsNamespace := namespaces[r.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	//	if len(namespaces) != 0 && !containsNamespace {
	//	validationErr = append(validationErr,
	//		field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
	//			"Oracle database operator doesn't watch over this namespace"))
	//}

	validationErr = append(validationErr, cr.validateDbSecretConfig()...)

	tdeEnabled := strings.EqualFold(strings.TrimSpace(getTDEWalletEnabledFromSpec(&cr.Spec)), "enable")
	tdeWalletPvc := strings.TrimSpace(getTDEWalletPVCFromSpec(&cr.Spec))
	if tdeEnabled {
		if cr.Spec.DbSecret == nil || cr.Spec.DbSecret.TDE == nil {
			validationErr = append(validationErr,
				field.Required(field.NewPath("spec").Child("dbSecret").Child("tde"),
					"tde credentials must be set when isTdeWallet is enable"))
		}
		if (len(cr.Spec.FssStorageClass) == 0) && (len(tdeWalletPvc) == 0) {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("fssStorageClass"), cr.Spec.FssStorageClass,
					"fssStorageClass or tdeWalletPvc must be set when isTdeWallet is enable"))

			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("tdeWalletPvc"), tdeWalletPvc,
					"fssStorageClass or tdeWalletPvc must be set when isTdeWallet is enable"))
		}
	}

	tdeEnabledValue := strings.TrimSpace(getTDEWalletEnabledFromSpec(&cr.Spec))
	if tdeEnabledValue != "" {
		if (strings.ToLower(tdeEnabledValue) != "enable") && (strings.ToLower(tdeEnabledValue) != "disable") {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("isTdeWallet"), tdeEnabledValue,
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

	validationErrs1 = cr.validateGsmConfig()
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

	validationErrs1 = cr.validateComputeSizingPathConfig()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = cr.validateAdditionalPVCConfig()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	// TODO(user): fill in your validation logic upon object creation.
	if len(validationErr) == 0 {
		logger.Info("create validation passed", "mode", detectShardingMode(&cr.Spec), "shards", len(cr.Spec.Shard), "shardInfo", len(cr.Spec.ShardInfo))
		return warnings, nil
	}
	logger.Info("create validation failed", "errorCount", len(validationErr))

	return warnings, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		cr.Name, validationErr)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj *ShardingDatabase) (admission.Warnings, error) {
	logger := shardingdatabaselog.WithValues("webhook", "validateUpdate", "name", r.Name, "namespace", r.Namespace)
	logger.Info("running update validation")

	var validationErr field.ErrorList

	oldCR := oldObj
	newCR := newObj
	warnings := deprecatedLegacyPVCFieldWarnings(&newCR.Spec)
	validationErr = append(validationErr, validateDataguardProducerSpec(field.NewPath("spec").Child("dataguard"), newCR.Spec.Dataguard)...)

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
			if overrideEnabled, overrideMsg := isUpdateLockOverrideEnabled(newCR); overrideEnabled {
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
		if isShardingDataguardTopologyLocked(oldCR) && hasShardingDataguardTopologyChange(oldCR, newCR) {
			validationErr = append(validationErr,
				field.Forbidden(field.NewPath("spec"),
					"Data Guard topology-defining fields cannot be changed after sharding dataguard topology is locked"))
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

	validationErrs1 = newCR.validateGsmConfig()
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

	validationErrs1 = newCR.validateComputeSizingPathConfig()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateComputeSizingPathUpdate(oldCR)
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateAdditionalPVCConfig()
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateAdditionalPVCUpdate(oldCR)
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	validationErrs1 = newCR.validateStorageSizeNoShrink(oldCR)
	if validationErrs1 != nil {
		validationErr = append(validationErr, validationErrs1...)
	}

	if len(validationErr) == 0 {
		logger.Info("update validation passed", "oldMode", oldMode, "newMode", newMode)
		return warnings, nil
	}
	logger.Info("update validation failed", "oldMode", oldMode, "newMode", newMode, "errorCount", len(validationErr))

	return warnings, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		r.Name, validationErr)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateDelete(ctx context.Context, obj *ShardingDatabase) (admission.Warnings, error) {
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

func (r *ShardingDatabase) validateStorageSizeNoShrink(oldCR *ShardingDatabase) field.ErrorList {
	if oldCR == nil {
		return nil
	}
	var errs field.ErrorList

	oldShards := map[string]ShardSpec{}
	for i := range oldCR.Spec.Shard {
		name := strings.TrimSpace(oldCR.Spec.Shard[i].Name)
		if name == "" {
			continue
		}
		oldShards[name] = oldCR.Spec.Shard[i]
	}
	for i := range r.Spec.Shard {
		name := strings.TrimSpace(r.Spec.Shard[i].Name)
		oldSpec, ok := oldShards[name]
		if !ok {
			continue
		}
		if oldSpec.StorageSizeInGb > 0 && r.Spec.Shard[i].StorageSizeInGb > 0 && r.Spec.Shard[i].StorageSizeInGb < oldSpec.StorageSizeInGb {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec").Child("shard").Index(i).Child("storageSizeInGb"),
				fmt.Sprintf("cannot shrink shard storage from %dGi to %dGi", oldSpec.StorageSizeInGb, r.Spec.Shard[i].StorageSizeInGb),
			))
		}
	}

	oldCatalogs := map[string]CatalogSpec{}
	for i := range oldCR.Spec.Catalog {
		name := strings.TrimSpace(oldCR.Spec.Catalog[i].Name)
		if name == "" {
			continue
		}
		oldCatalogs[name] = oldCR.Spec.Catalog[i]
	}
	for i := range r.Spec.Catalog {
		name := strings.TrimSpace(r.Spec.Catalog[i].Name)
		oldSpec, ok := oldCatalogs[name]
		if !ok {
			continue
		}
		if oldSpec.StorageSizeInGb > 0 && r.Spec.Catalog[i].StorageSizeInGb > 0 && r.Spec.Catalog[i].StorageSizeInGb < oldSpec.StorageSizeInGb {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec").Child("catalog").Index(i).Child("storageSizeInGb"),
				fmt.Sprintf("cannot shrink catalog storage from %dGi to %dGi", oldSpec.StorageSizeInGb, r.Spec.Catalog[i].StorageSizeInGb),
			))
		}
	}

	oldGsms := map[string]GsmSpec{}
	for i := range oldCR.Spec.Gsm {
		name := strings.TrimSpace(oldCR.Spec.Gsm[i].Name)
		if name == "" {
			continue
		}
		oldGsms[name] = oldCR.Spec.Gsm[i]
	}
	for i := range r.Spec.Gsm {
		name := strings.TrimSpace(r.Spec.Gsm[i].Name)
		oldSpec, ok := oldGsms[name]
		if !ok {
			continue
		}
		if oldSpec.StorageSizeInGb > 0 && r.Spec.Gsm[i].StorageSizeInGb > 0 && r.Spec.Gsm[i].StorageSizeInGb < oldSpec.StorageSizeInGb {
			errs = append(errs, field.Forbidden(
				field.NewPath("spec").Child("gsm").Index(i).Child("storageSizeInGb"),
				fmt.Sprintf("cannot shrink gsm storage from %dGi to %dGi", oldSpec.StorageSizeInGb, r.Spec.Gsm[i].StorageSizeInGb),
			))
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func deprecatedLegacyPVCFieldWarnings(spec *ShardingDatabaseSpec) admission.Warnings {
	if spec == nil {
		return nil
	}
	warnings := admission.Warnings{}
	appendWarn := func(path string) {
		warnings = append(warnings, path+" uses deprecated pvcName/pvAnnotations/pvMatchLabels fields; these fields are ignored by the operator. Use additionalPVCs instead.")
	}

	for i := range spec.Shard {
		if strings.TrimSpace(spec.Shard[i].PvcName) != "" || len(spec.Shard[i].PvAnnotations) > 0 || len(spec.Shard[i].PvMatchLabels) > 0 {
			appendWarn(fmt.Sprintf("spec.shard[%d]", i))
		}
	}
	for i := range spec.Catalog {
		if strings.TrimSpace(spec.Catalog[i].PvcName) != "" || len(spec.Catalog[i].PvAnnotations) > 0 || len(spec.Catalog[i].PvMatchLabels) > 0 {
			appendWarn(fmt.Sprintf("spec.catalog[%d]", i))
		}
	}
	for i := range spec.Gsm {
		if strings.TrimSpace(spec.Gsm[i].PvcName) != "" || len(spec.Gsm[i].PvAnnotations) > 0 || len(spec.Gsm[i].PvMatchLabels) > 0 {
			appendWarn(fmt.Sprintf("spec.gsm[%d]", i))
		}
	}
	return warnings
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

func (r *ShardingDatabase) validateGsmConfig() field.ErrorList {
	var validationErrs field.ErrorList

	for i := range r.Spec.Gsm {
		if strings.TrimSpace(r.Spec.Gsm[i].Name) == "" {
			validationErrs = append(validationErrs,
				field.Required(field.NewPath("spec").Child("gsm").Index(i).Child("name"),
					"gsm.name must be set"))
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

func (r *ShardingDatabase) validateAdditionalPVCConfig() field.ErrorList {
	var validationErrs field.ErrorList

	for i := range r.Spec.Shard {
		basePath := field.NewPath("spec").Child("shard").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCEntries(r.Spec.Shard[i].AdditionalPVCs, DefaultOraDataMountPath, DefaultDiagMountPath, basePath)...)
	}

	for i := range r.Spec.Catalog {
		basePath := field.NewPath("spec").Child("catalog").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCEntries(r.Spec.Catalog[i].AdditionalPVCs, DefaultOraDataMountPath, DefaultDiagMountPath, basePath)...)
	}

	for i := range r.Spec.Gsm {
		basePath := field.NewPath("spec").Child("gsm").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCEntries(r.Spec.Gsm[i].AdditionalPVCs, DefaultGsmDataMountPath, DefaultGsmDiagMountPath, basePath)...)
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) validateAdditionalPVCUpdate(oldCR *ShardingDatabase) field.ErrorList {
	var validationErrs field.ErrorList
	if oldCR == nil {
		return nil
	}

	oldShards := map[string]ShardSpec{}
	for i := range oldCR.Spec.Shard {
		oldShards[oldCR.Spec.Shard[i].Name] = oldCR.Spec.Shard[i]
	}
	for i := range r.Spec.Shard {
		newSpec := r.Spec.Shard[i]
		oldSpec, found := oldShards[newSpec.Name]
		if !found {
			continue
		}
		oldMap := mergeAdditionalPVCs(oldSpec.StorageSizeInGb, DefaultOraDataMountPath, oldSpec.AdditionalPVCs)
		newMap := mergeAdditionalPVCs(newSpec.StorageSizeInGb, DefaultOraDataMountPath, newSpec.AdditionalPVCs)
		basePath := field.NewPath("spec").Child("shard").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCUpdate(oldMap, newMap, basePath)...)
	}

	oldCatalogs := map[string]CatalogSpec{}
	for i := range oldCR.Spec.Catalog {
		oldCatalogs[oldCR.Spec.Catalog[i].Name] = oldCR.Spec.Catalog[i]
	}
	for i := range r.Spec.Catalog {
		newSpec := r.Spec.Catalog[i]
		oldSpec, found := oldCatalogs[newSpec.Name]
		if !found {
			continue
		}
		oldMap := mergeAdditionalPVCs(oldSpec.StorageSizeInGb, DefaultOraDataMountPath, oldSpec.AdditionalPVCs)
		newMap := mergeAdditionalPVCs(newSpec.StorageSizeInGb, DefaultOraDataMountPath, newSpec.AdditionalPVCs)
		basePath := field.NewPath("spec").Child("catalog").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCUpdate(oldMap, newMap, basePath)...)
	}

	oldGsms := map[string]GsmSpec{}
	for i := range oldCR.Spec.Gsm {
		oldGsms[oldCR.Spec.Gsm[i].Name] = oldCR.Spec.Gsm[i]
	}
	for i := range r.Spec.Gsm {
		newSpec := r.Spec.Gsm[i]
		oldSpec, found := oldGsms[newSpec.Name]
		if !found {
			continue
		}
		oldMap := mergeAdditionalPVCs(oldSpec.StorageSizeInGb, DefaultGsmDataMountPath, oldSpec.AdditionalPVCs)
		newMap := mergeAdditionalPVCs(newSpec.StorageSizeInGb, DefaultGsmDataMountPath, newSpec.AdditionalPVCs)
		basePath := field.NewPath("spec").Child("gsm").Index(i).Child("additionalPVCs")
		validationErrs = append(validationErrs, validateAdditionalPVCUpdate(oldMap, newMap, basePath)...)
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

	sysPrimaryReplicaCountByGroup := map[string]int32{}
	sysStandbyReplicaCountByGroup := map[string]int32{}
	sysRegionByGroup := map[string]string{}
	sysGroupByRegion := map[string]string{}
	userPrimarySourceCountBySpace := map[string]int32{}
	userPrimaryReplicaCountBySpace := map[string]int32{}
	userStandbyReplicaCountBySpace := map[string]int32{}
	userSpaceSeenInShardInfo := map[string]bool{}
	spacePrimaryGroupCount := map[string]int{}
	compositePrimaryGroupsBySpace := map[string]map[string]bool{}
	compositePrimaryReplicaCountBySpace := map[string]int32{}
	compositeStandbyReplicaCountBySpaceGroup := map[string]map[string]int32{}
	compositeNativeGroupRegionBySpace := map[string]map[string]string{}
	compositeNativeGroupByRegionBySpace := map[string]map[string]string{}
	compositeNativeGroupsBySpace := map[string]map[string]bool{}

	for pindex := range r.Spec.ShardInfo {
		if errs := validateStandbyConfigPrimarySourceExclusive(r, pindex); len(errs) > 0 {
			validationErrs = append(validationErrs, errs...)
		}

		replicas = getShardInfoCountByMode(modeHint, &r.Spec.ShardInfo[pindex])
		if replicas == 0 {
			replicas = 1
			r.Spec.ShardInfo[pindex].ShardNum = replicas
		}

		sg := r.Spec.ShardInfo[pindex].ShardGroupDetails
		ss := r.Spec.ShardInfo[pindex].ShardSpaceDetails
		if sg != nil {
			sg.DeployAs = normalizeDeployAsCanonical(sg.DeployAs)
		}
		if ss != nil {
			ss.DeployAs = normalizeDeployAsCanonical(ss.DeployAs)
		}

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
			ruMode := normalizeShardGroupRuModeCanonical(sg.RuMode)
			sg.RuMode = ruMode
			if deployAs != "" && !isValidDeployAs(deployAs) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						sg.DeployAs,
						"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
			}
			if ruMode != "" && !isValidShardGroupRuMode(ruMode) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
						sg.RuMode,
						"ru_mode must be one of READWRITE, READONLY"))
			}
			if deployAs != "" && ruMode != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails"),
						"deployAs and ru_mode are mutually exclusive"))
			}
			if replType == replNative && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						"deployAs is not supported for NATIVE replication"))
			}
			if replType != replNative && ruMode != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
						"ru_mode is only supported for NATIVE replication"))
			}
			if replType == replDG && deployAs == "" {
				sg.DeployAs = "STANDBY"
				deployAs = "STANDBY"
			}
			if modeHint == modeSystem {
				groupKey := strings.ToUpper(strings.TrimSpace(sg.Name))
				regionKey := strings.ToUpper(strings.TrimSpace(sg.Region))
				if groupKey != "" && regionKey != "" {
					if old, ok := sysRegionByGroup[groupKey]; ok && old != regionKey {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("region"),
								sg.Region,
								fmt.Sprintf("system sharding: shardGroup %s cannot span multiple regions (%s, %s)", groupKey, old, regionKey)))
					} else {
						sysRegionByGroup[groupKey] = regionKey
					}
					if oldGroup, ok := sysGroupByRegion[regionKey]; ok && oldGroup != groupKey {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("region"),
								sg.Region,
								fmt.Sprintf("system sharding: region %s is already used by shardGroup %s", regionKey, oldGroup)))
					} else {
						sysGroupByRegion[regionKey] = groupKey
					}
				}
				if groupKey != "" {
					replicaCount := replicas
					if replicaCount <= 0 {
						replicaCount = 1
					}
					switch deployAs {
					case "PRIMARY":
						sysPrimaryReplicaCountByGroup[groupKey] += replicaCount
					case "STANDBY", "ACTIVE_STANDBY":
						sysStandbyReplicaCountByGroup[groupKey] += replicaCount
						if cfg := r.Spec.ShardInfo[pindex].StandbyConfig; cfg != nil && cfg.StandbyPerPrimary > 0 {
							validationErrs = append(validationErrs,
								field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("standbyConfig").Child("standbyPerPrimary"),
									cfg.StandbyPerPrimary,
									"system sharding does not support standbyPerPrimary in shardInfo; standby mapping must follow shardGroup primary topology"))
						}
						if cfg := r.Spec.ShardInfo[pindex].StandbyConfig; cfg != nil && standbyConfigPrimaryCount(cfg) > 0 {
							validationErrs = append(validationErrs,
								field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("standbyConfig"),
									"system sharding does not support standbyConfig.primarySources; standby mapping must follow shardGroup primary topology"))
						}
					}
				}
			}

		case !hasGroup && hasSpace:
			typeCounts.spaceOnly++
			if sg != nil {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails"),
						sg,
						"User-defined sharding: shardGroupDetails must be omitted when only shardSpaceDetails is used"))
			}
			if modeHint == modeUser {
				deployAs := strings.ToUpper(strings.TrimSpace(ss.DeployAs))
				if deployAs != "" && !isValidDeployAs(deployAs) {
					validationErrs = append(validationErrs,
						field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardSpaceDetails").Child("deployAs"),
							ss.DeployAs,
							"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
				}
				if replType == replNative && deployAs != "" {
					validationErrs = append(validationErrs,
						field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardSpaceDetails").Child("deployAs"),
							"deployAs is not supported for NATIVE replication"))
				}
			}
			if modeHint == modeUser && replType == replDG {
				spaceKey := strings.ToUpper(strings.TrimSpace(ss.Name))
				if spaceKey != "" {
					userSpaceSeenInShardInfo[spaceKey] = true
					deployAs := strings.ToUpper(strings.TrimSpace(ss.DeployAs))
					if deployAs == "" {
						ss.DeployAs = "STANDBY"
						deployAs = "STANDBY"
					}
					//nolint:staticcheck // Keep explicit branch style in dense validation flow.
					if deployAs == "PRIMARY" {
						replicaCount := replicas
						if replicaCount <= 0 {
							replicaCount = 1
						}
						userPrimaryReplicaCountBySpace[spaceKey] += replicaCount
					} else if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
						replicaCount := replicas
						if replicaCount <= 0 {
							replicaCount = 1
						}
						userStandbyReplicaCountBySpace[spaceKey] += replicaCount
					}
					userPrimarySourceCountBySpace[spaceKey] += standbyConfigPrimaryCount(r.Spec.ShardInfo[pindex].StandbyConfig)
				}
			}

		case hasGroup && hasSpace:
			typeCounts.both++
			deployAs := strings.ToUpper(strings.TrimSpace(sg.DeployAs))
			ruMode := normalizeShardGroupRuModeCanonical(sg.RuMode)
			sg.RuMode = ruMode
			if deployAs != "" && !isValidDeployAs(deployAs) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						sg.DeployAs,
						"deployAs must be one of PRIMARY, STANDBY, ACTIVE_STANDBY"))
			}
			if ruMode != "" && !isValidShardGroupRuMode(ruMode) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
						sg.RuMode,
						"ru_mode must be one of READWRITE, READONLY"))
			}
			if deployAs != "" && ruMode != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails"),
						"deployAs and ru_mode are mutually exclusive"))
			}
			if replType == replNative && deployAs != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("deployAs"),
						"deployAs is not supported for NATIVE replication"))
			}
			if replType != replNative && ruMode != "" {
				validationErrs = append(validationErrs,
					field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
						"ru_mode is only supported for NATIVE replication"))
			}
			if replType == replDG && deployAs == "" {
				sg.DeployAs = "STANDBY"
				deployAs = "STANDBY"
			}
			if strings.EqualFold(deployAs, "PRIMARY") {
				spacePrimaryGroupCount[strings.TrimSpace(ss.Name)]++
			}
			if modeHint == modeComposite {
				spaceKey := strings.ToUpper(strings.TrimSpace(ss.Name))
				groupKey := strings.ToUpper(strings.TrimSpace(sg.Name))
				if replType == replNative {
					regionKey := strings.ToUpper(strings.TrimSpace(sg.Region))
					if regionKey == "" {
						validationErrs = append(validationErrs,
							field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("region"),
								"composite sharding with NATIVE replication requires shardGroupDetails.region"))
					}
					//nolint:staticcheck // Keep explicit branch style in dense validation flow
					if ruMode == "" {
						validationErrs = append(validationErrs,
							field.Required(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
								"composite sharding with NATIVE replication requires shardGroupDetails.ru_mode"))
					} else if ruMode != "READWRITE" {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("ru_mode"),
								sg.RuMode,
								"composite sharding with NATIVE replication currently supports only READWRITE shardGroups"))
					}
					if _, ok := compositeNativeGroupRegionBySpace[spaceKey]; !ok {
						compositeNativeGroupRegionBySpace[spaceKey] = map[string]string{}
					}
					if _, ok := compositeNativeGroupByRegionBySpace[spaceKey]; !ok {
						compositeNativeGroupByRegionBySpace[spaceKey] = map[string]string{}
					}
					if _, ok := compositeNativeGroupsBySpace[spaceKey]; !ok {
						compositeNativeGroupsBySpace[spaceKey] = map[string]bool{}
					}
					compositeNativeGroupsBySpace[spaceKey][groupKey] = true
					if regionKey != "" {
						if prevRegion, ok := compositeNativeGroupRegionBySpace[spaceKey][groupKey]; ok && prevRegion != regionKey {
							validationErrs = append(validationErrs,
								field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("region"),
									sg.Region,
									fmt.Sprintf("composite sharding with NATIVE replication: shardGroup %s in shardSpace %s cannot span multiple regions (%s, %s)", groupKey, spaceKey, prevRegion, regionKey)))
						} else {
							compositeNativeGroupRegionBySpace[spaceKey][groupKey] = regionKey
						}
						if prevGroup, ok := compositeNativeGroupByRegionBySpace[spaceKey][regionKey]; ok && prevGroup != groupKey {
							validationErrs = append(validationErrs,
								field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("shardGroupDetails").Child("region"),
									sg.Region,
									fmt.Sprintf("composite sharding with NATIVE replication: region %s in shardSpace %s is already used by shardGroup %s", regionKey, spaceKey, prevGroup)))
						} else {
							compositeNativeGroupByRegionBySpace[spaceKey][regionKey] = groupKey
						}
					}
				}
				if cfg := r.Spec.ShardInfo[pindex].StandbyConfig; cfg != nil && cfg.StandbyPerPrimary > 0 {
					validationErrs = append(validationErrs,
						field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("standbyConfig").Child("standbyPerPrimary"),
							cfg.StandbyPerPrimary,
							"composite sharding does not support standbyPerPrimary in shardInfo; standby mapping must follow shardGroup primary topology"))
				}
				if cfg := r.Spec.ShardInfo[pindex].StandbyConfig; cfg != nil && standbyConfigPrimaryCount(cfg) > 0 {
					validationErrs = append(validationErrs,
						field.Forbidden(field.NewPath("spec").Child("shardInfo").Index(pindex).Child("standbyConfig"),
							"composite sharding does not support standbyConfig.primarySources; standby mapping must follow shardGroup primary topology"))
				}
				replicaCount := replicas
				if replicaCount <= 0 {
					replicaCount = 1
				}
				if _, ok := compositePrimaryGroupsBySpace[spaceKey]; !ok {
					compositePrimaryGroupsBySpace[spaceKey] = map[string]bool{}
				}
				if _, ok := compositeStandbyReplicaCountBySpaceGroup[spaceKey]; !ok {
					compositeStandbyReplicaCountBySpaceGroup[spaceKey] = map[string]int32{}
				}
				switch deployAs {
				case "PRIMARY":
					compositePrimaryGroupsBySpace[spaceKey][groupKey] = true
					compositePrimaryReplicaCountBySpace[spaceKey] += replicaCount
				case "STANDBY", "ACTIVE_STANDBY":
					compositeStandbyReplicaCountBySpaceGroup[spaceKey][groupKey] += replicaCount
				}
			}

		default:
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex),
					r.Spec.ShardInfo[pindex],
					"Each shardInfo entry must define shardGroupDetails (system), shardSpaceDetails (user), or both (composite)"))
		}
	}

	if replType == replDG && typeCounts.groupOnly > 0 && typeCounts.spaceOnly == 0 && typeCounts.both == 0 {
		if modeHint == modeSystem {
			if len(sysPrimaryReplicaCountByGroup) != 1 {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("deployAs"),
						"PRIMARY",
						"System sharding: exactly one shardGroup must be PRIMARY"))
			}
			if len(sysPrimaryReplicaCountByGroup) == 1 {
				var primaryGroup string
				var primaryReplicaCount int32
				for g, c := range sysPrimaryReplicaCountByGroup {
					primaryGroup = g
					primaryReplicaCount = c
				}
				if sCount := sysStandbyReplicaCountByGroup[primaryGroup]; sCount > 0 {
					validationErrs = append(validationErrs,
						field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("name"),
							primaryGroup,
							"System sharding: PRIMARY shardGroup must not contain standby databases"))
				}
				for standbyGroup, standbyCount := range sysStandbyReplicaCountByGroup {
					if standbyGroup == primaryGroup {
						continue
					}
					if standbyCount > primaryReplicaCount {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("name"),
								standbyGroup,
								fmt.Sprintf("System sharding: standby shardGroup %s has %d standby databases but primary shardGroup %s has only %d primary databases", standbyGroup, standbyCount, primaryGroup, primaryReplicaCount)))
					}
				}
				var totalStandbyReplicas int32
				for standbyGroup, standbyCount := range sysStandbyReplicaCountByGroup {
					if standbyGroup == primaryGroup {
						continue
					}
					totalStandbyReplicas += standbyCount
				}
				if totalStandbyReplicas > 0 && totalStandbyReplicas != primaryReplicaCount {
					validationErrs = append(validationErrs,
						field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("name"),
							totalStandbyReplicas,
							fmt.Sprintf("System sharding: total standby databases across standby shardGroups must match primary shardGroup %s database count (%d), got %d", primaryGroup, primaryReplicaCount, totalStandbyReplicas)))
				}
			}
		} else {
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
		if modeHint == modeComposite {
			for spaceKey, primaryGroups := range compositePrimaryGroupsBySpace {
				if len(primaryGroups) != 1 {
					validationErrs = append(validationErrs,
						field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardSpaceDetails").Child("name"),
							spaceKey,
							"Composite sharding: each shardSpace must have exactly one PRIMARY shardGroup"))
					continue
				}
				primaryReplicaCount := compositePrimaryReplicaCountBySpace[spaceKey]
				for standbyGroup, standbyCount := range compositeStandbyReplicaCountBySpaceGroup[spaceKey] {
					if standbyCount > primaryReplicaCount {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("name"),
								standbyGroup,
								fmt.Sprintf("Composite sharding: standby shardGroup %s in shardSpace %s has %d standby databases but primary shardGroup has only %d primary databases", standbyGroup, spaceKey, standbyCount, primaryReplicaCount)))
					}
				}
			}
		}
	}

	if replType == replDG && modeHint == modeUser {
		for spaceKey := range userSpaceSeenInShardInfo {
			primaryReplicas := userPrimaryReplicaCountBySpace[spaceKey]
			standbyReplicas := userStandbyReplicaCountBySpace[spaceKey]
			externalSources := userPrimarySourceCountBySpace[spaceKey]
			if externalSources > 0 {
				if primaryReplicas > 0 {
					validationErrs = append(validationErrs,
						field.Forbidden(field.NewPath("spec").Child("shardInfo"),
							fmt.Sprintf("user sharding shardSpace %s uses standbyConfig.primarySources; do not set shardSpaceDetails.deployAs=PRIMARY", spaceKey)))
				}
				continue
			}
			if standbyReplicas > 0 && primaryReplicas == 0 {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo"),
						fmt.Sprintf("user sharding shardInfo requires at least one PRIMARY shard in shardSpace %s before defining standby shards", spaceKey)))
				continue
			}
			if primaryReplicas != 1 {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shardInfo"),
						fmt.Sprintf("user sharding shardInfo requires exactly one PRIMARY shard per shardSpace; shardSpace %s has %d", spaceKey, primaryReplicas)))
			}
		}
	}
	if replType == replNative && modeHint == modeComposite {
		for spaceKey, groups := range compositeNativeGroupsBySpace {
			if len(groups) > 1 {
				groupNames := make([]string, 0, len(groups))
				for groupKey := range groups {
					groupNames = append(groupNames, groupKey)
				}
				slices.Sort(groupNames)
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardSpaceDetails").Child("name"),
						spaceKey,
						fmt.Sprintf("composite sharding with NATIVE replication currently supports at most one shardGroup per shardSpace; shardSpace %s has groups: %s", spaceKey, strings.Join(groupNames, ","))))
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func validateStandbyConfigPrimarySourceExclusive(r *ShardingDatabase, index int) field.ErrorList {
	var errs field.ErrorList
	if r == nil || index < 0 || index >= len(r.Spec.ShardInfo) {
		return errs
	}
	cfg := r.Spec.ShardInfo[index].StandbyConfig
	if cfg == nil {
		return errs
	}

	path := field.NewPath("spec").Child("shardInfo").Index(index).Child("standbyConfig")
	seen := map[string]bool{}
	for i := range cfg.PrimarySources {
		source := &cfg.PrimarySources[i]
		sourcePath := path.Child("primarySources").Index(i)
		selected := 0
		if source.DatabaseRef != nil {
			selected++
		}
		if strings.TrimSpace(source.ConnectString) != "" {
			selected++
		}
		if source.Details != nil {
			selected++
		}
		if selected == 0 {
			errs = append(errs, field.Required(sourcePath, "set exactly one of databaseRef, connectString, or details"))
		}
		if selected > 1 {
			errs = append(errs, field.Forbidden(sourcePath, "databaseRef, connectString, and details are mutually exclusive; set only one"))
		}
		if source.DatabaseRef != nil && strings.TrimSpace(source.DatabaseRef.Name) == "" {
			errs = append(errs, field.Required(sourcePath.Child("databaseRef").Child("name"), "name is required"))
		}
		if details := source.Details; details != nil {
			if strings.TrimSpace(details.ConnectString) == "" {
				if strings.TrimSpace(details.Host) == "" {
					errs = append(errs, field.Required(sourcePath.Child("details").Child("host"), "host is required when connectString is not set"))
				}
				if strings.TrimSpace(details.CdbName) == "" {
					errs = append(errs, field.Required(sourcePath.Child("details").Child("cdbName"), "cdbName is required when connectString is not set"))
				}
			}
			if details.Port < 0 {
				errs = append(errs, field.Invalid(sourcePath.Child("details").Child("port"), details.Port, "must be >= 0"))
			}
		}
		if key := standbyPrimarySourceIdentityKey(source, r.Namespace); key != "" {
			if seen[key] {
				errs = append(errs, field.Duplicate(sourcePath, key))
				continue
			}
			seen[key] = true
		}
	}
	return errs
}

func isValidDeployAs(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "PRIMARY", "STANDBY", "ACTIVE_STANDBY":
		return true
	default:
		return false
	}
}

func isValidShardGroupRuMode(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "READWRITE", "READONLY":
		return true
	default:
		return false
	}
}

func normalizeShardGroupRuModeCanonical(v string) string {
	trimmed := strings.TrimSpace(v)
	switch strings.ToUpper(trimmed) {
	case "READWRITE":
		return "READWRITE"
	case "READONLY":
		return "READONLY"
	default:
		return trimmed
	}
}

func normalizeDeployAsCanonical(v string) string {
	trimmed := strings.TrimSpace(v)
	switch strings.ToUpper(trimmed) {
	case "PRIMARY":
		return "PRIMARY"
	case "STANDBY":
		return "STANDBY"
	case "ACTIVE_STANDBY":
		return "ACTIVE_STANDBY"
	default:
		return trimmed
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

func getTDEWalletEnabledFromSpec(spec *ShardingDatabaseSpec) string {
	if spec == nil {
		return ""
	}
	if spec.TDEWallet != nil {
		if v := strings.TrimSpace(spec.TDEWallet.IsEnabled); v != "" {
			return v
		}
	}
	return strings.TrimSpace(spec.IsTdeWallet)
}

func getTDEWalletPVCFromSpec(spec *ShardingDatabaseSpec) string {
	if spec == nil {
		return ""
	}
	if spec.TDEWallet != nil {
		if v := strings.TrimSpace(spec.TDEWallet.PVCName); v != "" {
			return v
		}
	}
	return strings.TrimSpace(spec.TdeWalletPvc)
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
	if modeHint == modeUser && replType == replNative {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("replicationType"),
				"user-defined sharding is not supported with RAFT/NATIVE replication"))
		return validationErrs
	}
	userPrimaryBySpace := map[string]int{}
	userSpaceSeen := map[string]bool{}
	userExternalPrimaryBySpace := map[string]bool{}
	userStandbyRegionsBySpace := map[string]map[string]bool{}
	compositeGroupSpaceByName := map[string]string{}
	compositeNativeGroupRegionBySpace := map[string]map[string]string{}
	compositeNativeGroupByRegionBySpace := map[string]map[string]string{}
	compositeNativeGroupsBySpace := map[string]map[string]bool{}

	for i := range r.Spec.ShardInfo {
		info := r.Spec.ShardInfo[i]
		if info.ShardSpaceDetails == nil {
			continue
		}
		spaceKey := strings.ToUpper(strings.TrimSpace(info.ShardSpaceDetails.Name))
		if spaceKey == "" {
			continue
		}
		if standbyConfigPrimaryCount(info.StandbyConfig) > 0 {
			userExternalPrimaryBySpace[spaceKey] = true
		}
		if modeHint == modeComposite && info.ShardGroupDetails != nil {
			if strings.EqualFold(strings.TrimSpace(info.ShardGroupDetails.IsDelete), deleteStateEnable) {
				continue
			}
			groupKey := strings.ToUpper(strings.TrimSpace(info.ShardGroupDetails.Name))
			if groupKey == "" {
				continue
			}
			if prevSpace, ok := compositeGroupSpaceByName[groupKey]; ok && prevSpace != spaceKey {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Index(i).Child("shardGroupDetails").Child("name"),
						info.ShardGroupDetails.Name,
						fmt.Sprintf("composite sharding shardGroup names must be unique across shardSpaces; shardGroup %s is used in both %s and %s", groupKey, prevSpace, spaceKey)))
			} else {
				compositeGroupSpaceByName[groupKey] = spaceKey
			}
			if replType == replNative {
				if _, ok := compositeNativeGroupsBySpace[spaceKey]; !ok {
					compositeNativeGroupsBySpace[spaceKey] = map[string]bool{}
				}
				compositeNativeGroupsBySpace[spaceKey][groupKey] = true
			}
		}
	}

	for i := range r.Spec.Shard {
		sh := r.Spec.Shard[i]
		hasGroup := strings.TrimSpace(sh.ShardGroup) != ""
		hasSpace := strings.TrimSpace(sh.ShardSpace) != ""
		mode := inferShardMode(modeHint, hasGroup, hasSpace)

		r.Spec.Shard[i].DeployAs = normalizeDeployAsCanonical(sh.DeployAs)
		deployAsRaw := strings.TrimSpace(r.Spec.Shard[i].DeployAs)
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
			if replType == replDG && hasSpace {
				spaceKey := strings.ToUpper(strings.TrimSpace(sh.ShardSpace))
				userSpaceSeen[spaceKey] = true

				if deployAs == "" {
					r.Spec.Shard[i].DeployAs = "STANDBY"
					deployAs = "STANDBY"
				}
				//nolint:staticcheck // Keep explicit branch style in dense validation flow.
				if deployAs == "PRIMARY" {
					userPrimaryBySpace[spaceKey]++
				} else if deployAs == "STANDBY" || deployAs == "ACTIVE_STANDBY" {
					regionKey := strings.ToUpper(strings.TrimSpace(sh.ShardRegion))
					if regionKey == "" {
						validationErrs = append(validationErrs,
							field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardRegion"),
								fmt.Sprintf("user sharding standby shard in shardSpace %s requires shardRegion", spaceKey)))
					} else {
						if _, ok := userStandbyRegionsBySpace[spaceKey]; !ok {
							userStandbyRegionsBySpace[spaceKey] = map[string]bool{}
						}
						if userStandbyRegionsBySpace[spaceKey][regionKey] {
							validationErrs = append(validationErrs,
								field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("shardRegion"),
									sh.ShardRegion,
									fmt.Sprintf("user sharding standby regions must be unique per shardSpace; shardSpace %s already uses region %s", spaceKey, regionKey)))
						} else {
							userStandbyRegionsBySpace[spaceKey][regionKey] = true
						}
					}
				}
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
			if hasGroup && hasSpace && !strings.EqualFold(strings.TrimSpace(sh.IsDelete), deleteStateEnable) {
				groupKey := strings.ToUpper(strings.TrimSpace(sh.ShardGroup))
				spaceKey := strings.ToUpper(strings.TrimSpace(sh.ShardSpace))
				if groupKey != "" && spaceKey != "" {
					if prevSpace, ok := compositeGroupSpaceByName[groupKey]; ok && prevSpace != spaceKey {
						validationErrs = append(validationErrs,
							field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
								sh.ShardGroup,
								fmt.Sprintf("composite sharding shardGroup names must be unique across shardSpaces; shardGroup %s is used in both %s and %s", groupKey, prevSpace, spaceKey)))
					} else {
						compositeGroupSpaceByName[groupKey] = spaceKey
					}
					if replType == replNative {
						regionKey := strings.ToUpper(strings.TrimSpace(sh.ShardRegion))
						if regionKey == "" {
							validationErrs = append(validationErrs,
								field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardRegion"),
									"composite sharding with NATIVE replication requires shardRegion"))
						}
						if _, ok := compositeNativeGroupRegionBySpace[spaceKey]; !ok {
							compositeNativeGroupRegionBySpace[spaceKey] = map[string]string{}
						}
						if _, ok := compositeNativeGroupByRegionBySpace[spaceKey]; !ok {
							compositeNativeGroupByRegionBySpace[spaceKey] = map[string]string{}
						}
						if regionKey != "" {
							if prevRegion, ok := compositeNativeGroupRegionBySpace[spaceKey][groupKey]; ok && prevRegion != regionKey {
								validationErrs = append(validationErrs,
									field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("shardRegion"),
										sh.ShardRegion,
										fmt.Sprintf("composite sharding with NATIVE replication: shardGroup %s in shardSpace %s cannot span multiple regions (%s, %s)", groupKey, spaceKey, prevRegion, regionKey)))
							} else {
								compositeNativeGroupRegionBySpace[spaceKey][groupKey] = regionKey
							}
							if prevGroup, ok := compositeNativeGroupByRegionBySpace[spaceKey][regionKey]; ok && prevGroup != groupKey {
								validationErrs = append(validationErrs,
									field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("shardRegion"),
										sh.ShardRegion,
										fmt.Sprintf("composite sharding with NATIVE replication: region %s in shardSpace %s is already used by shardGroup %s", regionKey, spaceKey, prevGroup)))
							} else {
								compositeNativeGroupByRegionBySpace[spaceKey][regionKey] = groupKey
							}
						}
						ruMode := resolveCompositeNativeShardGroupRuMode(&r.Spec, groupKey, spaceKey)
						//nolint:staticcheck // Keep explicit branch style in dense validation flow.
						if ruMode == "" {
							validationErrs = append(validationErrs,
								field.Required(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
									fmt.Sprintf("composite sharding with NATIVE replication requires ru_mode for shardGroup %s in shardSpace %s", groupKey, spaceKey)))
						} else if ruMode != "READWRITE" {
							validationErrs = append(validationErrs,
								field.Invalid(field.NewPath("spec").Child("shard").Index(i).Child("shardGroup"),
									sh.ShardGroup,
									"composite sharding with NATIVE replication currently supports only READWRITE shardGroups"))
						}
						if _, ok := compositeNativeGroupsBySpace[spaceKey]; !ok {
							compositeNativeGroupsBySpace[spaceKey] = map[string]bool{}
						}
						compositeNativeGroupsBySpace[spaceKey][groupKey] = true
					}
				}
			}
		}
	}

	if replType == replDG && modeHint == modeUser {
		for spaceKey := range userSpaceSeen {
			cnt := userPrimaryBySpace[spaceKey]
			if cnt > 1 {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shard"),
						spaceKey,
						fmt.Sprintf("user sharding allows at most one PRIMARY shard per shardSpace; shardSpace %s has %d", spaceKey, cnt)))
				continue
			}
			if userExternalPrimaryBySpace[spaceKey] {
				if cnt > 0 {
					validationErrs = append(validationErrs,
						field.Forbidden(field.NewPath("spec").Child("shard"),
							fmt.Sprintf("user sharding shardSpace %s uses standbyConfig.primarySources; do not set local deployAs=PRIMARY", spaceKey)))
				}
				continue
			}
			if cnt == 0 {
				validationErrs = append(validationErrs,
					field.Required(field.NewPath("spec").Child("shard"),
						fmt.Sprintf("user sharding requires exactly one PRIMARY shard per shardSpace; shardSpace %s has none", spaceKey)))
			}
		}
	}
	if replType == replNative && modeHint == modeComposite {
		for spaceKey, groups := range compositeNativeGroupsBySpace {
			if len(groups) > 1 {
				groupNames := make([]string, 0, len(groups))
				for groupKey := range groups {
					groupNames = append(groupNames, groupKey)
				}
				slices.Sort(groupNames)
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shard").Child("shardSpace"),
						spaceKey,
						fmt.Sprintf("composite sharding with NATIVE replication currently supports at most one shardGroup per shardSpace; shardSpace %s has groups: %s", spaceKey, strings.Join(groupNames, ","))))
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
				if r.Spec.ShardInfo[pindex].ShardGroupDetails == nil {
					r.Spec.Shard[shardIndex].DeployAs = r.Spec.ShardInfo[pindex].ShardSpaceDetails.DeployAs
				}
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
			if len(r.Spec.ShardInfo[pindex].AdditionalPVCs) > 0 {
				r.Spec.Shard[shardIndex].AdditionalPVCs = append([]AdditionalPVCSpec(nil), r.Spec.ShardInfo[pindex].AdditionalPVCs...)
			}
			r.Spec.Shard[shardIndex].ServiceAnnotations = r.Spec.ShardInfo[pindex].ServiceAnnotations
			r.Spec.Shard[shardIndex].ExternalServiceAnnotations = r.Spec.ShardInfo[pindex].ExternalServiceAnnotations
			r.Spec.Shard[shardIndex].SecurityContext = r.Spec.ShardInfo[pindex].SecurityContext
			r.Spec.Shard[shardIndex].Capabilities = r.Spec.ShardInfo[pindex].Capabilities
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
			merged.AdditionalPVCs = d.AdditionalPVCs
			merged.ServiceAnnotations = d.ServiceAnnotations
			merged.ExternalServiceAnnotations = d.ExternalServiceAnnotations
			merged.SecurityContext = d.SecurityContext
			merged.Capabilities = d.Capabilities
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

func resolveCompositeNativeShardGroupRuMode(spec *ShardingDatabaseSpec, groupKey, spaceKey string) string {
	if spec == nil {
		return ""
	}
	for i := range spec.ShardInfo {
		info := spec.ShardInfo[i]
		if info.ShardGroupDetails == nil || info.ShardSpaceDetails == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(info.ShardGroupDetails.IsDelete), deleteStateEnable) {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(info.ShardGroupDetails.Name)) != groupKey {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(info.ShardSpaceDetails.Name)) != spaceKey {
			continue
		}
		if ru := normalizeShardGroupRuModeCanonical(info.ShardGroupDetails.RuMode); ru != "" {
			return ru
		}
	}
	for i := range spec.ShardGroup {
		sg := spec.ShardGroup[i]
		if strings.ToUpper(strings.TrimSpace(sg.Name)) != groupKey {
			continue
		}
		sgSpaceKey := strings.ToUpper(strings.TrimSpace(sg.ShardSpace))
		if sgSpaceKey != "" && sgSpaceKey != spaceKey {
			continue
		}
		if ru := normalizeShardGroupRuModeCanonical(sg.RuMode); ru != "" {
			return ru
		}
	}
	return ""
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
	if c := standbyConfigDerivedShardCount(info.StandbyConfig); c > 0 {
		return c
	}
	if info.ShardNum > 0 {
		return info.ShardNum
	}
	if info.Replicas > 0 {
		return info.Replicas
	}
	return 0
}

func getShardInfoCountByMode(mode shardingMode, info *ShardingDetails) int32 {
	if info == nil {
		return 0
	}
	if mode == modeUser {
		if c := standbyConfigDerivedShardCount(info.StandbyConfig); c > 0 {
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

func standbyConfigDerivedShardCount(cfg *StandbyConfig) int32 {
	if cfg == nil {
		return 0
	}

	primaryCount := standbyConfigPrimaryCount(cfg)
	if primaryCount == 0 {
		return 0
	}

	perPrimary := cfg.StandbyPerPrimary
	if perPrimary <= 0 {
		perPrimary = 1
	}

	return primaryCount * perPrimary
}

func standbyConfigPrimaryCount(cfg *StandbyConfig) int32 {
	if cfg == nil {
		return 0
	}

	return standbyConfigPrimarySourceCount(cfg, "")
}

func standbyConfigPrimarySourceCount(cfg *StandbyConfig, defaultNamespace string) int32 {
	if cfg == nil {
		return 0
	}

	seen := map[string]bool{}
	var count int32
	for i := range cfg.PrimarySources {
		key := standbyPrimarySourceIdentityKey(&cfg.PrimarySources[i], defaultNamespace)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func standbyPrimarySourceIdentityKey(source *StandbyPrimarySource, defaultNamespace string) string {
	if source == nil {
		return ""
	}
	if ref := source.DatabaseRef; ref != nil && strings.TrimSpace(ref.Name) != "" {
		ns := strings.TrimSpace(ref.Namespace)
		if ns == "" {
			ns = defaultNamespace
		}
		return "ref:" + strings.ToLower(ns+"/"+strings.TrimSpace(ref.Name))
	}
	if connect := strings.TrimSpace(source.ConnectString); connect != "" {
		return "connect:" + strings.ToLower(connect)
	}
	if details := source.Details; details != nil {
		if connect := strings.TrimSpace(details.ConnectString); connect != "" {
			return "endpoint:" + strings.ToLower(connect)
		}
		host := strings.ToLower(strings.TrimSpace(details.Host))
		cdb := strings.ToLower(strings.TrimSpace(details.CdbName))
		pdb := strings.ToLower(strings.TrimSpace(details.PdbName))
		if host == "" && cdb == "" && pdb == "" {
			return ""
		}
		port := details.Port
		if port <= 0 {
			port = 1521
		}
		return fmt.Sprintf("endpoint:%s:%d/%s/%s", host, port, cdb, pdb)
	}
	return ""
}

func isShardingDataguardTopologyLocked(cr *ShardingDatabase) bool {
	return cr != nil && cr.Status.Dataguard != nil && cr.Status.Dataguard.TopologyLocked
}

func hasShardingDataguardTopologyChange(oldCR, newCR *ShardingDatabase) bool {
	return !reflect.DeepEqual(buildShardingDataguardTopologyProjection(oldCR), buildShardingDataguardTopologyProjection(newCR))
}

type shardingDataguardTopologyProjection struct {
	ReplicationType string
	ShardingType    string
	EnableTCPS      bool
	TcpsTlsSecret   string
	Shard           []shardingDataguardShardProjection
	ShardInfo       []shardingDataguardShardInfoProjection
	ShardGroup      []ShardGroupSpec
	ShardSpace      []ShardSpaceSpec
}

type shardingDataguardShardProjection struct {
	Name               string
	ShardSpace         string
	ShardGroup         string
	ShardRegion        string
	DeployAs           string
	PrimaryDatabaseRef *DatabaseRef
	StandbyConfig      *StandbyConfig
}

type shardingDataguardShardInfoProjection struct {
	ShardPreFixName    string
	ShardNum           int32
	Replicas           int32
	ShardGroupDetails  *ShardGroupSpec
	ShardSpaceDetails  *ShardSpaceSpec
	PrimaryDatabaseRef *DatabaseRef
	StandbyConfig      *StandbyConfig
}

func buildShardingDataguardTopologyProjection(cr *ShardingDatabase) shardingDataguardTopologyProjection {
	var projection shardingDataguardTopologyProjection
	if cr == nil {
		return projection
	}

	projection.ReplicationType = normalizeReplicationType(&cr.Spec)
	projection.ShardingType = string(detectShardingMode(&cr.Spec))
	projection.EnableTCPS = cr.Spec.EnableTCPS
	projection.TcpsTlsSecret = strings.TrimSpace(cr.Spec.TcpsTlsSecret)

	if len(cr.Spec.Shard) > 0 {
		projection.Shard = make([]shardingDataguardShardProjection, 0, len(cr.Spec.Shard))
		for i := range cr.Spec.Shard {
			shard := cr.Spec.Shard[i]
			projection.Shard = append(projection.Shard, shardingDataguardShardProjection{
				Name:               shard.Name,
				ShardSpace:         shard.ShardSpace,
				ShardGroup:         shard.ShardGroup,
				ShardRegion:        shard.ShardRegion,
				DeployAs:           shard.DeployAs,
				PrimaryDatabaseRef: shard.PrimaryDatabaseRef,
				StandbyConfig:      shard.StandbyConfig,
			})
		}
	}

	if len(cr.Spec.ShardInfo) > 0 {
		projection.ShardInfo = make([]shardingDataguardShardInfoProjection, 0, len(cr.Spec.ShardInfo))
		for i := range cr.Spec.ShardInfo {
			info := cr.Spec.ShardInfo[i]
			projection.ShardInfo = append(projection.ShardInfo, shardingDataguardShardInfoProjection{
				ShardPreFixName:    info.ShardPreFixName,
				ShardNum:           info.ShardNum,
				Replicas:           info.Replicas,
				ShardGroupDetails:  info.ShardGroupDetails,
				ShardSpaceDetails:  info.ShardSpaceDetails,
				PrimaryDatabaseRef: info.PrimaryDatabaseRef,
				StandbyConfig:      info.StandbyConfig,
			})
		}
	}

	if len(cr.Spec.ShardGroup) > 0 {
		projection.ShardGroup = append(projection.ShardGroup, cr.Spec.ShardGroup...)
	}
	if len(cr.Spec.ShardSpace) > 0 {
		projection.ShardSpace = append(projection.ShardSpace, cr.Spec.ShardSpace...)
	}

	return projection
}
