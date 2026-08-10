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

// revive:disable:unused-parameter,exported,var-naming
// Legacy webhook signatures and helper names are preserved for backward compatibility.

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

var singleInstanceDatabaseTNSAliasNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func (r *SingleInstanceDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-singleinstancedatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,verbs=create;update,versions=v4,name=msingleinstancedatabasev4.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Defaulter[*SingleInstanceDatabase] = &SingleInstanceDatabase{}
var _ admission.Validator[*SingleInstanceDatabase] = &SingleInstanceDatabase{}

// Default implements admission.Defaulter[*SingleInstanceDatabase]
func (r *SingleInstanceDatabase) Default(ctx context.Context, obj *SingleInstanceDatabase) error {
	sidb := obj

	singleinstancedatabaselog.Info("default", "name", sidb.Name)

	if sidb.Spec.LoadBalancer {
		if sidb.Spec.ServiceAnnotations == nil {
			sidb.Spec.ServiceAnnotations = make(map[string]string)
		}
		if _, ok := sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"]; !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"] = "flexible"
		}
		if _, ok := sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"]; !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"] = "10"
		}
		if _, ok := sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"]; !ok {
			sidb.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"] = "100"
		}
	}

	adminPassword := sidbAdminPasswordSpec(sidb)
	if adminPassword != nil && adminPassword.KeepSecret == nil && sidbAdminPasswordFieldsSet(adminPassword) {
		keepSecret := true
		adminPassword.KeepSecret = &keepSecret
	}

	if sidb.Spec.CreateAs == "" {
		sidb.Spec.CreateAs = "primary"
	}
	if sidb.Spec.Replicas == 0 {
		sidb.Spec.Replicas = 1
	}
	if sidb.Spec.Edition == "" && sidb.Spec.CreateAs == "clone" && !sidb.Spec.Image.PrebuiltDB {
		sidb.Spec.Edition = "enterprise"
	}
	if sidb.Spec.Sid == "" {
		switch sidb.Spec.Edition {
		case "express":
			sidb.Spec.Sid = "XE"
		case "free":
			sidb.Spec.Sid = "FREE"
		default:
			sidb.Spec.Sid = "ORCLCDB"
		}
	}
	if sidb.Spec.Pdbname == "" {
		switch sidb.Spec.Edition {
		case "express":
			sidb.Spec.Pdbname = "XEPDB1"
		case "free":
			sidb.Spec.Pdbname = "FREEPDB1"
		default:
			sidb.Spec.Pdbname = "ORCLPDB1"
		}
	}
	if sidb.Spec.TrueCacheServices == nil {
		sidb.Spec.TrueCacheServices = make([]string, 0)
	}
	if sidb.Spec.Dataguard == nil {
		sidb.Spec.Dataguard = &DataguardProducerSpec{}
	}
	sidb.Spec.Dataguard.Mode = normalizeDataguardProducerMode(sidb.Spec.Dataguard)
	defaultSIDBPersistence(&sidb.Spec.Persistence)
	defaultSIDBScripts(&sidb.Spec.Scripts)
	defaultSIDBAdditionalPVCs(&sidb.Spec.Persistence.AdditionalPVCs)
	defaultSIDBRestoreSpec(&sidb.Spec.Restore)

	return nil
}

//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v4,name=vsingleinstancedatabasev4.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Validator[*SingleInstanceDatabase] = &SingleInstanceDatabase{}

func sidbTcpsEnabled(sidb *SingleInstanceDatabase) bool {
	if sidb == nil {
		return false
	}
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && sidb.Spec.Security.TCPS.Enabled {
		return true
	}
	// Preserve legacy compatibility for manifests that only set deprecated TCPS
	// listenerPort field without the explicit enabled flag.
	if sidb.Spec.TcpsListenerPort != 0 {
		return true
	}
	return sidb.Spec.EnableTCPS
}

func sidbTcpsListenerPort(sidb *SingleInstanceDatabase) int {
	return sidb.Spec.TcpsListenerPort
}

func sidbTcpsTlsSecret(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.TlsSecret) != "" {
		return strings.TrimSpace(sidb.Spec.Security.TCPS.TlsSecret)
	}
	return strings.TrimSpace(sidb.Spec.TcpsTlsSecret)
}

func sidbTcpsClientWalletSecret(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.ClientWalletSecret) != "" {
		return strings.TrimSpace(sidb.Spec.Security.TCPS.ClientWalletSecret)
	}
	return ""
}

func sidbTcpsCertRenewInterval(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.CertRenewInterval) != "" {
		return strings.TrimSpace(sidb.Spec.Security.TCPS.CertRenewInterval)
	}
	return strings.TrimSpace(sidb.Spec.TcpsCertRenewInterval)
}

func sidbAdminPasswordSpec(sidb *SingleInstanceDatabase) *SingleInstanceDatabaseAdminPassword {
	if sidb == nil {
		return nil
	}
	if sidb.Spec.Security != nil && sidb.Spec.Security.Secrets != nil && sidb.Spec.Security.Secrets.Admin != nil {
		return sidb.Spec.Security.Secrets.Admin
	}
	return &sidb.Spec.AdminPassword
}

func sidbTDESecretSpec(sidb *SingleInstanceDatabase) *SingleInstanceDatabasePasswordSecret {
	if sidb == nil {
		return nil
	}
	if sidb.Spec.Security != nil && sidb.Spec.Security.Secrets != nil && sidb.Spec.Security.Secrets.TDE != nil {
		return sidb.Spec.Security.Secrets.TDE
	}
	return nil
}

func sidbHasLegacyAdminPasswordSpec(sidb *SingleInstanceDatabase) bool {
	if sidb == nil {
		return false
	}
	return sidbAdminPasswordFieldsSet(&sidb.Spec.AdminPassword)
}

func sidbAdminPasswordFieldsSet(admin *SingleInstanceDatabaseAdminPassword) bool {
	if admin == nil {
		return false
	}
	return strings.TrimSpace(admin.SecretName) != "" ||
		strings.TrimSpace(admin.MountPath) != "" ||
		(strings.TrimSpace(admin.SecretKey) != "" && strings.TrimSpace(admin.SecretName) != "") ||
		admin.KeepSecret != nil ||
		admin.SkipInitWallet
}

func sidbTrueCacheDBCredentialsWalletSpec(sidb *SingleInstanceDatabase) *SingleInstanceDatabaseTrueCacheDBCredentialsWallet {
	if sidb == nil || sidb.Spec.TrueCache == nil {
		return nil
	}
	return sidb.Spec.TrueCache.DBCredentialsWallet
}

func sidbOradataPersistence(sidb *SingleInstanceDatabase) *SingleInstanceDatabasePersistenceOradata {
	if sidb == nil {
		return nil
	}
	if sidb.Spec.Persistence.Oradata != nil {
		return sidb.Spec.Persistence.Oradata
	}
	if sidb.Spec.Persistence.Size == "" && sidb.Spec.Persistence.StorageClass == "" && sidb.Spec.Persistence.AccessMode == "" {
		return nil
	}
	return &SingleInstanceDatabasePersistenceOradata{
		Size:         sidb.Spec.Persistence.Size,
		StorageClass: sidb.Spec.Persistence.StorageClass,
		AccessMode:   sidb.Spec.Persistence.AccessMode,
	}
}

func defaultSIDBPersistence(p *SingleInstanceDatabasePersistence) {
	if p == nil {
		return
	}
	if p.Fra != nil {
		if strings.TrimSpace(p.Fra.MountPath) == "" {
			p.Fra.MountPath = "/opt/oracle/oradata/fast_recovery_area"
		}
		if strings.TrimSpace(p.Fra.PvcName) == "" && strings.TrimSpace(p.Fra.RecoveryAreaSize) == "" && strings.TrimSpace(p.Fra.Size) != "" {
			p.Fra.RecoveryAreaSize = strings.TrimSpace(p.Fra.Size)
		}
	}
}

func (r *SingleInstanceDatabase) ValidateCreate(ctx context.Context, obj *SingleInstanceDatabase) (admission.Warnings, error) {
	sidb := obj
	singleinstancedatabaselog.Info("validate create", "name", sidb.Name)

	allErrs := validateSingleInstanceDatabaseSpec(sidb)
	warnings := sidbDeprecatedFieldWarnings(sidb)
	if len(allErrs) == 0 {
		return warnings, nil
	}
	return warnings, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		sidb.Name, allErrs)
}

func (r *SingleInstanceDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj *SingleInstanceDatabase) (admission.Warnings, error) {
	oldSidb, newSidb := oldObj, newObj
	singleinstancedatabaselog.Info("validate update", "name", newSidb.Name)

	// Delete-time updates such as finalizer removal must be allowed whenever the
	// user spec is unchanged. Status may still drift during teardown, and
	// requiring status equality can wedge deletion after cleanup has completed.
	if !newSidb.DeletionTimestamp.IsZero() &&
		reflect.DeepEqual(oldSidb.Spec, newSidb.Spec) {
		return sidbDeprecatedFieldWarnings(newSidb), nil
	}

	allErrs := validateSingleInstanceDatabaseSpecWithLegacySelfSignedTCPS(newSidb, sidbUsesLegacySelfSignedTCPS(oldSidb))
	allErrs = append(allErrs, validateSIDBFraUpdate(oldSidb, newSidb)...)
	warnings := sidbDeprecatedFieldWarnings(newSidb)
	oldForCompare := oldSidb.DeepCopy()
	newForCompare := newSidb.DeepCopy()
	specChanged := !reflect.DeepEqual(oldForCompare.Spec, newForCompare.Spec)
	if specChanged {
		if locked, lockGen, lockMsg := lockpolicy.IsControllerUpdateLocked(oldSidb.Status.Conditions, lockpolicy.DefaultReconcilingConditionType, lockpolicy.DefaultUpdateLockReason); locked {
			if overrideEnabled, _ := lockpolicy.IsUpdateLockOverrideEnabled(newSidb.GetAnnotations(), lockpolicy.DefaultOverrideAnnotation); !overrideEnabled {
				allErrs = append(allErrs, field.Forbidden(
					field.NewPath("spec"),
					fmt.Sprintf("spec updates are blocked while controller operation is in progress (reason=%s, observedGeneration=%d). %s",
						lockpolicy.DefaultUpdateLockReason, lockGen, lockMsg),
				))
			}
		}
	}

	if oldSidb.Status.CreatedAs == "clone" {
		if newSidb.Spec.Edition != "" && oldSidb.Status.Edition != "" && !strings.EqualFold(oldSidb.Status.Edition, newSidb.Spec.Edition) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("edition"), "edition of a cloned database cannot be changed post creation"))
		}
	}
	if isPrimarySourceLocked(oldSidb) {
		if resolveEffectivePrimarySource(oldSidb) != resolveEffectivePrimarySource(newSidb) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("primarySource"), primarySourceLockedMessage(oldSidb)))
		}
	}

	if oldSidb.Status.Role != dbcommons.ValueUnavailable && oldSidb.Status.Role != "PRIMARY" {
		statusArchiveLog, _ := strconv.ParseBool(oldSidb.Status.ArchiveLog)
		if newSidb.Spec.ArchiveLog != nil && statusArchiveLog != *newSidb.Spec.ArchiveLog {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("archiveLog"), "cannot be changed for non-primary database"))
		}
		statusFlashBack, _ := strconv.ParseBool(oldSidb.Status.FlashBack)
		if newSidb.Spec.FlashBack != nil && statusFlashBack != *newSidb.Spec.FlashBack {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("flashBack"), "cannot be changed for non-primary database"))
		}
		statusForceLogging, _ := strconv.ParseBool(oldSidb.Status.ForceLogging)
		if newSidb.Spec.ForceLogging != nil && statusForceLogging != *newSidb.Spec.ForceLogging {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("forceLog"), "cannot be changed for non-primary database"))
		}
	}

	if len(allErrs) == 0 {
		return warnings, nil
	}
	return warnings, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		newSidb.Name, allErrs)
}

func validateSIDBFraUpdate(oldSidb, newSidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	if oldSidb == nil || newSidb == nil {
		return allErrs
	}

	oldFra := oldSidb.Spec.Persistence.Fra
	newFra := newSidb.Spec.Persistence.Fra
	fraPath := field.NewPath("spec").Child("persistence").Child("fra")

	if oldFra == nil {
		return allErrs
	}
	if newFra == nil {
		allErrs = append(allErrs, field.Forbidden(fraPath, "cannot be removed once configured"))
		return allErrs
	}

	normalizeMountPath := func(fra *SingleInstanceDatabasePersistenceFra) string {
		if fra == nil || strings.TrimSpace(fra.MountPath) == "" {
			return "/opt/oracle/oradata/fast_recovery_area"
		}
		return strings.TrimSpace(fra.MountPath)
	}

	oldHasPVC := strings.TrimSpace(oldFra.PvcName) != ""
	newHasPVC := strings.TrimSpace(newFra.PvcName) != ""
	if oldHasPVC != newHasPVC {
		allErrs = append(allErrs, field.Forbidden(fraPath, "cannot switch between pvcName and managed FRA after creation"))
		return allErrs
	}

	if normalizeMountPath(oldFra) != normalizeMountPath(newFra) {
		allErrs = append(allErrs, field.Forbidden(fraPath.Child("mountPath"), "cannot be changed once FRA is configured"))
	}

	if oldHasPVC {
		if strings.TrimSpace(oldFra.PvcName) != strings.TrimSpace(newFra.PvcName) {
			allErrs = append(allErrs, field.Forbidden(fraPath.Child("pvcName"), "cannot be changed once FRA pvcName is configured"))
		}
		return allErrs
	}

	if strings.TrimSpace(oldFra.StorageClass) != strings.TrimSpace(newFra.StorageClass) {
		allErrs = append(allErrs, field.Forbidden(fraPath.Child("storageClass"), "cannot be changed once managed FRA is configured"))
	}
	if strings.TrimSpace(oldFra.AccessMode) != strings.TrimSpace(newFra.AccessMode) {
		allErrs = append(allErrs, field.Forbidden(fraPath.Child("accessMode"), "cannot be changed once managed FRA is configured"))
	}

	oldSize := strings.TrimSpace(oldFra.Size)
	newSize := strings.TrimSpace(newFra.Size)
	if oldSize != "" && newSize != "" {
		oldQty, oldErr := resource.ParseQuantity(oldSize)
		newQty, newErr := resource.ParseQuantity(newSize)
		if oldErr == nil && newErr == nil && newQty.Cmp(oldQty) < 0 {
			allErrs = append(allErrs, field.Forbidden(fraPath.Child("size"), "cannot be decreased once managed FRA is configured"))
		}
	}

	return allErrs
}

func (r *SingleInstanceDatabase) ValidateDelete(ctx context.Context, obj *SingleInstanceDatabase) (admission.Warnings, error) {
	return nil, nil
}

func validateSingleInstanceDatabaseSpec(sidb *SingleInstanceDatabase) field.ErrorList {
	return validateSingleInstanceDatabaseSpecWithLegacySelfSignedTCPS(sidb, false)
}

// validateSingleInstanceDatabaseSpecWithLegacySelfSignedTCPS allows an existing
// self-signed TCPS resource to be updated until its owner migrates it to a TLS
// Secret. New resources must always provide a TLS Secret.
func validateSingleInstanceDatabaseSpecWithLegacySelfSignedTCPS(sidb *SingleInstanceDatabase, allowLegacySelfSignedTCPS bool) field.ErrorList {
	allErrs := field.ErrorList{}

	namespaces := dbcommons.GetWatchNamespaces()
	if len(namespaces) != 0 {
		if _, ok := namespaces[sidb.Namespace]; !ok {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("namespace"), sidb.Namespace, "operator does not watch this namespace"))
		}
	}
	allErrs = append(allErrs, validateDataguardProducerSpec(field.NewPath("spec").Child("dataguard"), sidb.Spec.Dataguard)...)
	allErrs = append(allErrs, validateSIDBImageSpec(field.NewPath("spec").Child("image"), &sidb.Spec.Image)...)
	allErrs = append(allErrs, validateSIDBServiceEndpoints(field.NewPath("spec").Child("services").Child("endpoints"), sidb)...)

	oradata := sidbOradataPersistence(sidb)
	if sidb.Spec.Persistence.Oradata != nil && strings.TrimSpace(sidb.Spec.Persistence.DatafilesVolumeName) != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("persistence").Child("datafilesVolumeName"), "datafilesVolumeName is mutually exclusive with persistence.oradata"))
	}
	if sidb.Spec.Persistence.Oradata != nil && (sidb.Spec.Persistence.Size != "" || sidb.Spec.Persistence.StorageClass != "" || sidb.Spec.Persistence.AccessMode != "") {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("persistence"), "do not mix deprecated persistence size/storageClass/accessMode with persistence.oradata"))
	}
	if oradata != nil {
		oradataPath := field.NewPath("spec").Child("persistence").Child("oradata")
		if sidb.Spec.Persistence.Oradata == nil {
			oradataPath = field.NewPath("spec").Child("persistence")
		}
		hasPvcName := strings.TrimSpace(oradata.PvcName) != ""
		hasDynamic := strings.TrimSpace(oradata.Size) != ""
		if hasPvcName && (strings.TrimSpace(oradata.Size) != "" || strings.TrimSpace(oradata.StorageClass) != "") {
			allErrs = append(allErrs, field.Forbidden(oradataPath, "pvcName is mutually exclusive with size/storageClass"))
		}
		if !hasPvcName && !hasDynamic && strings.TrimSpace(sidb.Spec.Persistence.DatafilesVolumeName) != "" {
			allErrs = append(allErrs, field.Invalid(oradataPath.Child("size"), oradata.Size, "size is required when datafilesVolumeName is specified without pvcName"))
		}
		if hasDynamic {
			if strings.TrimSpace(oradata.AccessMode) == "" {
				allErrs = append(allErrs, field.Invalid(oradataPath.Child("accessMode"), oradata.AccessMode, "accessMode is required when size is set"))
			}
			if strings.TrimSpace(oradata.AccessMode) != "" &&
				oradata.AccessMode != "ReadWriteMany" && oradata.AccessMode != "ReadWriteOnce" {
				allErrs = append(allErrs, field.Invalid(oradataPath.Child("accessMode"), oradata.AccessMode, "must be ReadWriteOnce or ReadWriteMany"))
			}
		}
	}
	if sidb.Spec.Persistence.Fra != nil {
		fra := sidb.Spec.Persistence.Fra
		fraPath := field.NewPath("spec").Child("persistence").Child("fra")
		hasPvcName := strings.TrimSpace(fra.PvcName) != ""
		hasDynamic := strings.TrimSpace(fra.Size) != ""
		if hasPvcName && (strings.TrimSpace(fra.Size) != "" || strings.TrimSpace(fra.StorageClass) != "" || strings.TrimSpace(fra.AccessMode) != "") {
			allErrs = append(allErrs, field.Forbidden(fraPath, "pvcName is mutually exclusive with size/storageClass/accessMode"))
		}
		if mountPath := strings.TrimSpace(fra.MountPath); mountPath != "" && !strings.HasPrefix(mountPath, "/") {
			allErrs = append(allErrs, field.Invalid(fraPath.Child("mountPath"), fra.MountPath, "mountPath must be an absolute path"))
		}
		if !hasPvcName && !hasDynamic {
			allErrs = append(allErrs, field.Required(fraPath.Child("size"), "size is required when pvcName is not provided"))
		}
		if !hasPvcName && hasDynamic && strings.TrimSpace(fra.AccessMode) == "" {
			allErrs = append(allErrs, field.Required(fraPath.Child("accessMode"), "accessMode is required when size is set"))
		}
		if hasPvcName && strings.TrimSpace(fra.RecoveryAreaSize) == "" {
			allErrs = append(allErrs, field.Required(fraPath.Child("recoveryAreaSize"), "required when pvcName is set"))
		}
		var parsedRecoverySize dbcommons.OracleBinarySizeLiteral
		hasParsedRecoverySize := false
		if strings.TrimSpace(fra.RecoveryAreaSize) != "" {
			parsed, err := dbcommons.ParseOracleBinarySizeLiteral(fra.RecoveryAreaSize)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fraPath.Child("recoveryAreaSize"), fra.RecoveryAreaSize, err.Error()))
			} else {
				parsedRecoverySize = parsed
				hasParsedRecoverySize = true
			}
		}
		if hasParsedRecoverySize && strings.TrimSpace(fra.Size) != "" {
			fraSize, errSize := resource.ParseQuantity(strings.TrimSpace(fra.Size))
			if errSize == nil && parsedRecoverySize.Quantity.Cmp(fraSize) > 0 {
				allErrs = append(allErrs, field.Invalid(fraPath.Child("recoveryAreaSize"), fra.RecoveryAreaSize, "must be less than or equal to fra.size"))
			}
		}
	}
	if sidb.Spec.Persistence.VolumeClaimAnnotation != "" {
		strParts := strings.SplitN(sidb.Spec.Persistence.VolumeClaimAnnotation, ":", 2)
		if len(strParts) != 2 || strings.TrimSpace(strParts[0]) == "" || strings.TrimSpace(strParts[1]) == "" {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("persistence").Child("volumeClaimAnnotation"),
				sidb.Spec.Persistence.VolumeClaimAnnotation,
				"volumeClaimAnnotation should be in <key>:<value> format",
			))
		}
	}

	mode := strings.ToLower(strings.TrimSpace(sidb.Spec.CreateAs))
	allErrs = append(allErrs, validateSIDBRestoreSpec(sidb, mode)...)
	allErrs = append(allErrs, validateSIDBTrueCacheByMode(sidb, mode)...)
	allErrs = append(allErrs, validatePrimarySourceSpec(sidb, mode)...)
	if sidb.Spec.Dataguard != nil && len(sidb.Spec.Dataguard.StandbySources) > 0 && mode != "" && mode != "primary" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("dataguard").Child("standbySources"), "standbySources are supported only for createAs=primary"))
	}
	if mode == "clone" || mode == "standby" || mode == "truecache" {
		if !resolvePrimarySourceInputPresent(sidb) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("primarySource"), sidb.Spec.PrimarySource, fmt.Sprintf("%s requires one primary source: primarySource.databaseRef, primarySource.connectString, primarySource.details, or deprecated spec.primaryDatabaseRef", mode)))
		}
	}
	if mode == "standby" {
		if sidb.Spec.ArchiveLog != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("archiveLog"), sidb.Spec.ArchiveLog, "archiveLog cannot be specified for standby"))
		}
		if sidb.Spec.FlashBack != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("flashBack"), sidb.Spec.FlashBack, "flashBack cannot be specified for standby"))
		}
		if sidb.Spec.ForceLogging != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("forceLog"), sidb.Spec.ForceLogging, "forceLog cannot be specified for standby"))
		}
		if sidb.Spec.InitParams != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("initParams"), sidb.Spec.InitParams, "initParams cannot be specified for standby"))
		}
	}

	if details := resolvePrimarySourceDetails(sidb); details != nil {
		if strings.TrimSpace(details.Host) == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("primarySource").Child("details").Child("host"), details.Host, "host cannot be empty"))
		}
		if strings.TrimSpace(details.Sid) == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("primarySource").Child("details").Child("sid"), details.Sid, "sid cannot be empty"))
		}
		if details.Port < 0 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("primarySource").Child("details").Child("port"), details.Port, "port cannot be negative"))
		}
	}

	if tde := sidbTDESecretSpec(sidb); tde != nil {
		tdeSecretName := strings.TrimSpace(tde.SecretName)
		if tdeSecretName == "" {
			if strings.TrimSpace(tde.WalletZipFileKey) != "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("security").Child("secrets").Child("tde").Child("walletZipFileKey"), tde.WalletZipFileKey, "walletZipFileKey requires secretName"))
			}
			if strings.TrimSpace(tde.WalletRoot) != "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("security").Child("secrets").Child("tde").Child("walletRoot"), tde.WalletRoot, "walletRoot requires secretName"))
			}
		}
	}

	if sidb.Spec.Replicas > 1 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("replicas"), sidb.Spec.Replicas, "should be 1; multi-replica SIDB is not supported"))
	}

	tcpsEnabled := sidbTcpsEnabled(sidb)
	tcpsTlsSecret := sidbTcpsTlsSecret(sidb)
	tcpsClientWalletSecret := sidbTcpsClientWalletSecret(sidb)
	tcpsCertRenewInterval := sidbTcpsCertRenewInterval(sidb)
	tcpsListenerPort := sidbTcpsListenerPort(sidb)

	if tcpsEnabled && tcpsTlsSecret == "" && !allowLegacySelfSignedTCPS {
		allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("security").Child("tcps").Child("tlsSecret"), "required when TCPS is enabled"))
	}
	if tcpsEnabled && tcpsCertRenewInterval != "" {
		duration, err := time.ParseDuration(tcpsCertRenewInterval)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), tcpsCertRenewInterval, "invalid duration"))
		} else {
			maxLimit, _ := time.ParseDuration("8760h")
			minLimit, _ := time.ParseDuration("24h")
			if duration > maxLimit || duration < minLimit {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), tcpsCertRenewInterval, "must be in range 24h to 8760h"))
			}
		}
	}
	if !tcpsEnabled && tcpsTlsSecret != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tcpsTlsSecret"), "allowed only when enableTCPS=true"))
	}
	if !tcpsEnabled && tcpsClientWalletSecret != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("security").Child("tcps").Child("clientWalletSecret"), "allowed only when enableTCPS=true"))
	}
	if tcpsTlsSecret != "" && tcpsCertRenewInterval != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tcpsCertRenewInterval"), "not applicable when tcpsTlsSecret is provided"))
	}
	if !sidbUsesServiceEndpoints(sidb) {
		if tcpsEnabled && sidb.Spec.ListenerPort != 0 && tcpsListenerPort != 0 && sidb.Spec.ListenerPort == tcpsListenerPort {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), tcpsListenerPort, "listenerPort and tcpsListenerPort cannot be equal"))
		}
		if sidb.Spec.ListenerPort != 0 && (sidb.Spec.ListenerPort < 1 || sidb.Spec.ListenerPort > 65535) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("listenerPort"), sidb.Spec.ListenerPort, "must be in 1-65535"))
		}
		if tcpsEnabled && tcpsListenerPort != 0 && (tcpsListenerPort < 1 || tcpsListenerPort > 65535) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), tcpsListenerPort, "must be in 1-65535"))
		}
	}
	allErrs = append(allErrs, validateTNSAliases(sidb)...)

	if sidb.Spec.InitParams != nil {
		if (sidb.Spec.InitParams.PgaAggregateTarget != 0 && sidb.Spec.InitParams.SgaTarget == 0) || (sidb.Spec.InitParams.PgaAggregateTarget == 0 && sidb.Spec.InitParams.SgaTarget != 0) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("initParams"), sidb.Spec.InitParams, "provide both pgaAggregateTarget and sgaTarget"))
		}
	}

	allErrs = append(allErrs, validateSingleInstanceDatabaseResourceFields(sidb)...)
	allErrs = append(allErrs, validateSIDBScripts(sidb)...)
	allErrs = append(allErrs, validateSingleInstanceDatabaseAdditionalPVCs(sidb)...)

	return allErrs
}

func sidbUsesLegacySelfSignedTCPS(sidb *SingleInstanceDatabase) bool {
	return sidbTcpsEnabled(sidb) && sidbTcpsTlsSecret(sidb) == ""
}

func sidbUsesServiceEndpoints(sidb *SingleInstanceDatabase) bool {
	return sidb != nil && sidb.Spec.Services != nil && len(sidb.Spec.Services.Endpoints) != 0
}

func sidbDeprecatedFieldWarnings(sidb *SingleInstanceDatabase) admission.Warnings {
	if sidb == nil {
		return nil
	}
	var warnings admission.Warnings
	if sidb.Spec.LoadBalancer {
		warnings = append(warnings, "spec.loadBalancer is deprecated; use spec.services.endpoints")
	}
	if sidb.Spec.ListenerPort != 0 {
		warnings = append(warnings, "spec.listenerPort is deprecated; use spec.services.endpoints.tcp")
	}
	if sidb.Spec.TcpsListenerPort != 0 {
		warnings = append(warnings, "spec.tcpsListenerPort is deprecated; use spec.services.endpoints.tcps")
	}
	if len(sidb.Spec.ServiceAnnotations) != 0 {
		warnings = append(warnings, "spec.serviceAnnotations is deprecated; use spec.services.endpoints.annotations")
	}
	if sidb.Spec.EnableTCPS {
		warnings = append(warnings, "spec.enableTCPS is deprecated; use spec.security.tcps.enabled")
	}
	if strings.TrimSpace(sidb.Spec.TcpsCertRenewInterval) != "" {
		warnings = append(warnings, "spec.tcpsCertRenewInterval is deprecated; certificate renewal is managed by the TLS Secret owner")
	}
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.CertRenewInterval) != "" {
		warnings = append(warnings, "spec.security.tcps.certRenewInterval is deprecated; certificate renewal is managed by the TLS Secret owner")
	}
	if strings.TrimSpace(sidb.Spec.TcpsTlsSecret) != "" {
		warnings = append(warnings, "spec.tcpsTlsSecret is deprecated; use spec.security.tcps.tlsSecret")
	}
	if strings.TrimSpace(sidb.Spec.AdminPassword.SecretName) != "" {
		warnings = append(warnings, "spec.adminPassword is deprecated; use spec.security.secrets.admin")
	}
	if strings.TrimSpace(sidb.Spec.Persistence.Size) != "" {
		warnings = append(warnings, "spec.persistence.size is deprecated; use spec.persistence.oradata.size")
	}
	if strings.TrimSpace(sidb.Spec.Persistence.StorageClass) != "" {
		warnings = append(warnings, "spec.persistence.storageClass is deprecated; use spec.persistence.oradata.storageClass")
	}
	if strings.TrimSpace(sidb.Spec.Persistence.AccessMode) != "" {
		warnings = append(warnings, "spec.persistence.accessMode is deprecated; use spec.persistence.oradata.accessMode")
	}
	if strings.TrimSpace(sidb.Spec.Persistence.DatafilesVolumeName) != "" {
		warnings = append(warnings, "spec.persistence.datafilesVolumeName is deprecated; use spec.persistence.oradata with a user-managed pvcName or managed size/storageClass")
	}
	if strings.TrimSpace(sidb.Spec.Persistence.ScriptsVolumeName) != "" {
		warnings = append(warnings, "spec.persistence.scriptsVolumeName is deprecated; use spec.scripts.setup/spec.scripts.startup pvcName references")
	}
	if sidb.Spec.Image.PullPolicy != nil {
		warnings = append(warnings, "spec.image.pullPolicy is deprecated; use spec.image.imagePullPolicy")
	}
	if sidb.Spec.ResourceRequirements != nil {
		warnings = append(warnings, "spec.resourceRequirements is deprecated; use spec.resources")
	}
	return warnings
}

func validateSIDBServiceEndpoints(path *field.Path, sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	if sidb == nil || sidb.Spec.Services == nil || len(sidb.Spec.Services.Endpoints) == 0 {
		return allErrs
	}
	if len(sidb.Spec.Services.Endpoints) > 3 {
		allErrs = append(allErrs, field.TooMany(path, len(sidb.Spec.Services.Endpoints), 3))
		return allErrs
	}

	seen := map[SingleInstanceDatabaseServiceEndpointName]bool{}
	for i, endpoint := range sidb.Spec.Services.Endpoints {
		itemPath := path.Index(i)
		name := endpoint.Name
		endpointType := endpoint.Type
		if name == "" {
			name = expectedSIDBEndpointNameForType(endpointType)
		}
		if name == "" {
			allErrs = append(allErrs, field.Required(itemPath.Child("name"), "name must be cluster, nodeport, or loadbalancer"))
			continue
		}
		if seen[name] {
			allErrs = append(allErrs, field.Duplicate(itemPath.Child("name"), name))
			continue
		}
		seen[name] = true

		expectedType := expectedSIDBEndpointTypeForName(name)
		if expectedType == "" {
			allErrs = append(allErrs, field.NotSupported(itemPath.Child("name"), name, []string{"cluster", "nodeport", "loadbalancer"}))
			continue
		}
		if endpointType == "" {
			endpointType = expectedType
		}
		if endpointType != expectedType {
			allErrs = append(allErrs, field.Invalid(itemPath.Child("type"), endpoint.Type, fmt.Sprintf("type must be %s when name is %s", expectedType, name)))
		}
		if endpoint.ExternalTrafficPolicy != "" {
			switch endpoint.ExternalTrafficPolicy {
			case corev1.ServiceExternalTrafficPolicyCluster, corev1.ServiceExternalTrafficPolicyLocal:
			default:
				allErrs = append(allErrs, field.NotSupported(
					itemPath.Child("externalTrafficPolicy"),
					endpoint.ExternalTrafficPolicy,
					[]string{string(corev1.ServiceExternalTrafficPolicyCluster), string(corev1.ServiceExternalTrafficPolicyLocal)},
				))
			}
			if name == SingleInstanceDatabaseServiceEndpointNameCluster {
				allErrs = append(allErrs, field.Forbidden(itemPath.Child("externalTrafficPolicy"), "externalTrafficPolicy is not used for the cluster endpoint"))
			}
		}

		tcpsEnabled := endpoint.TCPS != nil && endpoint.TCPS.Enabled
		if tcpsEnabled && !sidbTcpsEnabled(sidb) {
			allErrs = append(allErrs, field.Forbidden(itemPath.Child("tcps"), "tcps exposure requires TCPS to be enabled in the database spec"))
		}
		allErrs = append(allErrs, validateSIDBServiceEndpointPort(itemPath.Child("tcp"), name, endpoint.TCP, true)...)
		allErrs = append(allErrs, validateSIDBServiceEndpointPort(itemPath.Child("tcps"), name, endpoint.TCPS, false)...)

		if name == SingleInstanceDatabaseServiceEndpointNameLoadBalancer {
			tcpEnabled := endpoint.TCP == nil || endpoint.TCP.Enabled
			tcpPort := intOrDefault(0, 1521, tcpEnabled && (endpoint.TCP == nil || endpoint.TCP.Port == 0))
			if endpoint.TCP != nil && endpoint.TCP.Port != 0 {
				tcpPort = endpoint.TCP.Port
			}
			tcpsPort := intOrDefault(0, 2484, tcpsEnabled && endpoint.TCPS != nil && endpoint.TCPS.Port == 0)
			if endpoint.TCPS != nil && endpoint.TCPS.Port != 0 {
				tcpsPort = endpoint.TCPS.Port
			}
			if tcpEnabled && tcpsEnabled && tcpPort == tcpsPort {
				allErrs = append(allErrs, field.Invalid(itemPath.Child("tcps").Child("port"), tcpsPort, "tcp.port and tcps.port cannot be equal"))
			}
		}
		if name == SingleInstanceDatabaseServiceEndpointNameNodePort &&
			endpoint.TCP != nil && endpoint.TCP.Enabled &&
			endpoint.TCPS != nil && endpoint.TCPS.Enabled &&
			endpoint.TCP.NodePort != 0 && endpoint.TCP.NodePort == endpoint.TCPS.NodePort {
			allErrs = append(allErrs, field.Invalid(itemPath.Child("tcps").Child("nodePort"), endpoint.TCPS.NodePort, "tcp.nodePort and tcps.nodePort cannot be equal"))
		}
	}

	return allErrs
}

func validateSIDBServiceEndpointPort(path *field.Path, endpointName SingleInstanceDatabaseServiceEndpointName, port *SingleInstanceDatabaseServiceEndpointPort, isTCP bool) field.ErrorList {
	var allErrs field.ErrorList
	if port == nil {
		return allErrs
	}
	if !port.Enabled {
		if port.Port != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("port"), "port is allowed only when enabled=true"))
		}
		if port.NodePort != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("nodePort"), "nodePort is allowed only when enabled=true"))
		}
		return allErrs
	}

	switch endpointName {
	case SingleInstanceDatabaseServiceEndpointNameCluster:
		if port.NodePort != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("nodePort"), "nodePort is not used for the cluster endpoint"))
		}
		if port.Port != 0 && port.Port != 1521 && isTCP {
			allErrs = append(allErrs, field.Invalid(path.Child("port"), port.Port, "cluster tcp port is always 1521"))
		}
		if port.Port != 0 && (port.Port < 1 || port.Port > 65535) {
			allErrs = append(allErrs, field.Invalid(path.Child("port"), port.Port, "must be in 1-65535"))
		}
	case SingleInstanceDatabaseServiceEndpointNameLoadBalancer:
		if port.NodePort != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("nodePort"), "nodePort is not used for the loadbalancer endpoint"))
		}
		if port.Port != 0 && (port.Port < 1 || port.Port > 65535) {
			allErrs = append(allErrs, field.Invalid(path.Child("port"), port.Port, "must be in 1-65535"))
		}
	case SingleInstanceDatabaseServiceEndpointNameNodePort:
		if port.Port != 0 {
			allErrs = append(allErrs, field.Forbidden(path.Child("port"), "port is not used for the nodeport endpoint"))
		}
		if port.NodePort != 0 && (port.NodePort < 30000 || port.NodePort > 32767) {
			allErrs = append(allErrs, field.Invalid(path.Child("nodePort"), port.NodePort, "must be in 30000-32767 for NodePort"))
		}
	}

	return allErrs
}

func expectedSIDBEndpointNameForType(endpointType SingleInstanceDatabaseServiceEndpointType) SingleInstanceDatabaseServiceEndpointName {
	switch endpointType {
	case SingleInstanceDatabaseServiceEndpointTypeClusterIP:
		return SingleInstanceDatabaseServiceEndpointNameCluster
	case SingleInstanceDatabaseServiceEndpointTypeNodePort:
		return SingleInstanceDatabaseServiceEndpointNameNodePort
	case SingleInstanceDatabaseServiceEndpointTypeLoadBalancer:
		return SingleInstanceDatabaseServiceEndpointNameLoadBalancer
	default:
		return ""
	}
}

func expectedSIDBEndpointTypeForName(name SingleInstanceDatabaseServiceEndpointName) SingleInstanceDatabaseServiceEndpointType {
	switch name {
	case SingleInstanceDatabaseServiceEndpointNameCluster:
		return SingleInstanceDatabaseServiceEndpointTypeClusterIP
	case SingleInstanceDatabaseServiceEndpointNameNodePort:
		return SingleInstanceDatabaseServiceEndpointTypeNodePort
	case SingleInstanceDatabaseServiceEndpointNameLoadBalancer:
		return SingleInstanceDatabaseServiceEndpointTypeLoadBalancer
	default:
		return ""
	}
}

func intOrDefault(current, fallback int, useFallback bool) int {
	if useFallback {
		return fallback
	}
	return current
}

func validateTNSAliases(sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	seen := map[string]struct{}{}
	basePath := field.NewPath("spec").Child("tnsAliases")

	for i := range sidb.Spec.TNSAliases {
		alias := sidb.Spec.TNSAliases[i]
		aliasPath := basePath.Index(i)

		name := strings.TrimSpace(alias.Name)
		if name == "" {
			allErrs = append(allErrs, field.Required(aliasPath.Child("name"), "name is required"))
		} else {
			if !singleInstanceDatabaseTNSAliasNamePattern.MatchString(name) {
				allErrs = append(allErrs, field.Invalid(aliasPath.Child("name"), alias.Name, "must start with an alphanumeric character and contain only letters, digits, dot, underscore, or hyphen"))
			}
			key := strings.ToUpper(name)
			if _, exists := seen[key]; exists {
				allErrs = append(allErrs, field.Duplicate(aliasPath.Child("name"), alias.Name))
			}
			seen[key] = struct{}{}
		}

		if strings.TrimSpace(alias.Host) == "" {
			allErrs = append(allErrs, field.Required(aliasPath.Child("host"), "host is required"))
		}
		if strings.TrimSpace(alias.ServiceName) == "" {
			allErrs = append(allErrs, field.Required(aliasPath.Child("serviceName"), "serviceName is required"))
		}

		switch strings.ToUpper(strings.TrimSpace(string(alias.Protocol))) {
		case string(SingleInstanceDatabaseTNSAliasProtocolTCP), string(SingleInstanceDatabaseTNSAliasProtocolTCPS):
		default:
			allErrs = append(allErrs, field.Invalid(aliasPath.Child("protocol"), alias.Protocol, "must be TCP or TCPS"))
		}

		if alias.Port < 0 || alias.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(aliasPath.Child("port"), alias.Port, "must be in 0-65535"))
		}
	}

	return allErrs
}

func validateSIDBImageSpec(path *field.Path, image *SingleInstanceDatabaseImage) field.ErrorList {
	var allErrs field.ErrorList
	if image == nil {
		return allErrs
	}
	if image.ImagePullPolicy != nil && image.PullPolicy != nil && *image.ImagePullPolicy != *image.PullPolicy {
		allErrs = append(allErrs, field.Forbidden(path.Child("pullPolicy"), "cannot be set together with spec.image.imagePullPolicy"))
		return allErrs
	}

	policy := image.ImagePullPolicy
	policyPath := path.Child("imagePullPolicy")
	if policy == nil {
		policy = image.PullPolicy
		policyPath = path.Child("pullPolicy")
	}
	if policy == nil {
		return allErrs
	}

	switch *policy {
	case corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever:
		return allErrs
	default:
		return append(allErrs, field.NotSupported(
			policyPath,
			*policy,
			[]string{string(corev1.PullAlways), string(corev1.PullIfNotPresent), string(corev1.PullNever)},
		))
	}
}

func defaultSIDBRestoreSpec(restore **SingleInstanceDatabaseRestoreSpec) {
	if restore == nil || *restore == nil {
		return
	}
	r := *restore
	if r.FileSystem != nil {
		r.FileSystem.BackupPath = strings.TrimSpace(r.FileSystem.BackupPath)
		r.FileSystem.CatalogStartWith = strings.TrimSpace(r.FileSystem.CatalogStartWith)
		if r.FileSystem.CatalogStartWith == "" {
			r.FileSystem.CatalogStartWith = r.FileSystem.BackupPath
		}
	}
}

func validateSIDBRestoreSpec(sidb *SingleInstanceDatabase, mode string) field.ErrorList {
	var allErrs field.ErrorList
	if sidb.Spec.Restore == nil {
		return allErrs
	}
	restorePath := field.NewPath("spec").Child("restore")
	if mode != "" && mode != "primary" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("createAs"), sidb.Spec.CreateAs, "restore is supported only when createAs=primary"))
	}
	hasObjectStore := sidb.Spec.Restore.ObjectStore != nil
	hasFileSystem := sidb.Spec.Restore.FileSystem != nil
	if hasObjectStore && hasFileSystem {
		allErrs = append(allErrs, field.Forbidden(restorePath, "objectStore and fileSystem are mutually exclusive"))
	}
	if !hasObjectStore && !hasFileSystem {
		allErrs = append(allErrs, field.Required(restorePath, "exactly one of objectStore or fileSystem must be specified"))
	}

	if hasFileSystem && strings.TrimSpace(sidb.Spec.Restore.FileSystem.BackupPath) == "" {
		allErrs = append(allErrs, field.Required(restorePath.Child("fileSystem").Child("backupPath"), "backupPath is required"))
	}
	if hasObjectStore {
		if ref := sidb.Spec.Restore.ObjectStore.OCIConfig; ref == nil || strings.TrimSpace(ref.ConfigMapName) == "" || strings.TrimSpace(ref.Key) == "" {
			allErrs = append(allErrs, field.Required(restorePath.Child("objectStore").Child("ociConfig"), "configMapName and key are required"))
		}
		if ref := sidb.Spec.Restore.ObjectStore.PrivateKey; ref == nil || strings.TrimSpace(ref.SecretName) == "" || strings.TrimSpace(ref.Key) == "" {
			allErrs = append(allErrs, field.Required(restorePath.Child("objectStore").Child("privateKey"), "secretName and key are required"))
		}
		if ref := sidb.Spec.Restore.ObjectStore.OpcInstallerZip; (ref == nil || strings.TrimSpace(ref.ConfigMapName) == "" || strings.TrimSpace(ref.Key) == "") &&
			!hasSIDBEnvVar(sidb.Spec.EnvVars, "OPC_INSTALL_ZIP") {
			allErrs = append(allErrs, field.Required(restorePath.Child("objectStore").Child("opcInstallerZip"), "configMapName and key are required unless OPC_INSTALL_ZIP env var is provided"))
		}
		if sidb.Spec.Restore.ObjectStore.BackupIdentity == nil || strings.TrimSpace(sidb.Spec.Restore.ObjectStore.BackupIdentity.DBID) == "" {
			allErrs = append(allErrs, field.Required(restorePath.Child("objectStore").Child("backupIdentity").Child("dbid"), "dbid is required"))
		}
	}
	if hasFileSystem {
		if !hasSIDBEnvVar(sidb.Spec.EnvVars, "DBID") {
			allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("envVars"), "DBID env var is required when restore.fileSystem is used"))
		}
	}
	return allErrs
}

func hasSIDBEnvVar(envs []corev1.EnvVar, name string) bool {
	target := strings.TrimSpace(name)
	if target == "" {
		return false
	}
	for i := range envs {
		if strings.TrimSpace(envs[i].Name) != target {
			continue
		}
		if strings.TrimSpace(envs[i].Value) != "" || envs[i].ValueFrom != nil {
			return true
		}
	}
	return false
}

func validateSingleInstanceDatabaseResourceFields(sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	validateResourceRequirements := func(rr *corev1.ResourceRequirements, fld *field.Path) {
		if rr == nil {
			return
		}
		for name, q := range rr.Requests {
			if q.Sign() < 0 {
				allErrs = append(allErrs, field.Invalid(fld.Child("requests").Child(string(name)), q.String(), "must be non-negative"))
			}
		}
		for name, q := range rr.Limits {
			if q.Sign() < 0 {
				allErrs = append(allErrs, field.Invalid(fld.Child("limits").Child(string(name)), q.String(), "must be non-negative"))
			}
		}
		for name, request := range rr.Requests {
			limit, ok := rr.Limits[name]
			if ok && request.Cmp(limit) > 0 {
				allErrs = append(allErrs, field.Invalid(
					fld.Child("requests").Child(string(name)),
					request.String(),
					fmt.Sprintf("must not be greater than limit %s", limit.String()),
				))
			}
		}
	}

	validateResourceRequirements(sidb.Spec.Resources, specPath.Child("resources"))
	validateResourceRequirements(sidb.Spec.ResourceRequirements, specPath.Child("resourceRequirements"))
	if strings.TrimSpace(sidb.Spec.ShmSize) != "" {
		shmSize, err := resource.ParseQuantity(strings.TrimSpace(sidb.Spec.ShmSize))
		if err != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("shmSize"), sidb.Spec.ShmSize, "must be a valid Kubernetes quantity"))
		} else if shmSize.Sign() <= 0 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("shmSize"), sidb.Spec.ShmSize, "must be greater than zero"))
		}
	}

	return allErrs
}

func validateSIDBTrueCacheByMode(sidb *SingleInstanceDatabase, mode string) field.ErrorList {
	var allErrs field.ErrorList
	tcPath := field.NewPath("spec").Child("trueCache")
	legacyServicesPath := field.NewPath("spec").Child("trueCacheServices")
	tc := sidb.Spec.TrueCache
	hasLegacyServices := len(sidb.Spec.TrueCacheServices) > 0

	isPrimaryMode := mode == "" || mode == "primary"
	if isPrimaryMode {
		if hasLegacyServices {
			allErrs = append(allErrs, field.Forbidden(legacyServicesPath, "only supported when createAs=truecache"))
		}
		if tc == nil {
			return allErrs
		}
		if strings.TrimSpace(tc.BlobConfigMapRef) != "" {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("blobConfigMapRef"), "supported only when createAs=truecache"))
		}
		if strings.TrimSpace(tc.BlobConfigMapKey) != "" {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("blobConfigMapKey"), "supported only when createAs=truecache"))
		}
		if strings.TrimSpace(tc.BlobMountPath) != "" {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("blobMountPath"), "supported only when createAs=truecache"))
		}
		if tc.DBCredentialsWallet != nil {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("dbCredentialsWallet"), "supported only when createAs=truecache"))
		}
		if tc.AutoTCServiceRegistration {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("autoTCServiceRegistration"), "supported only when createAs=truecache"))
		}
		if len(tc.TrueCacheServices) > 0 {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("trueCacheServices"), "supported only when createAs=truecache"))
		}
		if strings.TrimSpace(tc.GeneratePath) != "" &&
			!tc.BlobGenerationEnabled() &&
			!tc.BlobConfigMapCreationEnabled() {
			allErrs = append(allErrs, field.Invalid(tcPath.Child("generatePath"), tc.GeneratePath, "requires generateBlob=true, createConfigMap=true, or generateEnabled=true"))
		}
		return allErrs
	}

	if mode == "truecache" {
		if strings.ToLower(strings.TrimSpace(sidb.Spec.Edition)) != "enterprise" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("edition"), sidb.Spec.Edition, "truecache requires edition=enterprise"))
		}
		if tc == nil {
			allErrs = append(allErrs, field.Required(tcPath, "spec.trueCache is required when createAs=truecache"))
			return allErrs
		}
		if admin := sidbAdminPasswordSpec(sidb); admin == nil || strings.TrimSpace(admin.SecretName) == "" || strings.TrimSpace(admin.SecretKey) == "" {
			allErrs = append(allErrs, field.Required(field.NewPath("spec").Child("security").Child("secrets").Child("admin"), "truecache requires spec.security.secrets.admin"))
		}
		if sidbHasLegacyAdminPasswordSpec(sidb) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("adminPassword"), "truecache must use spec.security.secrets.admin instead of spec.adminPassword"))
		}
		wallet := sidbTrueCacheDBCredentialsWalletSpec(sidb)
		if wallet != nil {
			if mountPath := strings.TrimSpace(wallet.MountPath); mountPath != "" && !strings.HasPrefix(mountPath, "/") {
				allErrs = append(allErrs, field.Invalid(tcPath.Child("dbCredentialsWallet").Child("mountPath"), wallet.MountPath, "mountPath must be an absolute path"))
			}
		}
		if tc.GenerateEnabled {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("generateEnabled"), "supported only when createAs=primary"))
		}
		if tc.GenerateBlob {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("generateBlob"), "supported only when createAs=primary"))
		}
		if tc.CreateConfigMap {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("createConfigMap"), "supported only when createAs=primary"))
		}
		if len(tc.TrueCacheServices) == 0 && !hasLegacyServices {
			allErrs = append(allErrs, field.Required(tcPath.Child("trueCacheServices"), "set trueCacheServices for createAs=truecache"))
		}
		if tc.AutoTCServiceRegistration && len(tc.TrueCacheServices) == 0 && !hasLegacyServices {
			allErrs = append(allErrs, field.Required(tcPath.Child("trueCacheServices"), "set trueCacheServices when autoTCServiceRegistration=true"))
		}
		if strings.TrimSpace(tc.GeneratePath) != "" {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("generatePath"), "supported only when createAs=primary"))
		}
		return allErrs
	}

	if hasLegacyServices {
		allErrs = append(allErrs, field.Forbidden(legacyServicesPath, "only supported when createAs=truecache"))
	}
	if tc != nil {
		allErrs = append(allErrs, field.Forbidden(tcPath, "supported only when createAs=primary (generateEnabled/generateBlob/createConfigMap/generatePath) or createAs=truecache (blobConfigMapRef/blobConfigMapKey/blobMountPath/autoTCServiceRegistration/trueCacheServices)"))
	}
	return allErrs
}

func defaultSIDBAdditionalPVCs(pvcs *[]AdditionalPVCSpec) {
	if pvcs == nil {
		return
	}
	for i := range *pvcs {
		(*pvcs)[i].MountPath = strings.TrimSpace((*pvcs)[i].MountPath)
		(*pvcs)[i].PvcName = strings.TrimSpace((*pvcs)[i].PvcName)
		(*pvcs)[i].StorageClass = strings.TrimSpace((*pvcs)[i].StorageClass)
	}
}

func defaultSIDBScripts(scripts **SingleInstanceDatabaseScriptsSpec) {
	if scripts == nil || *scripts == nil {
		return
	}
	if (*scripts).Setup != nil {
		(*scripts).Setup.PvcName = strings.TrimSpace((*scripts).Setup.PvcName)
	}
	if (*scripts).Startup != nil {
		(*scripts).Startup.PvcName = strings.TrimSpace((*scripts).Startup.PvcName)
	}
}

func validateSIDBScripts(sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	if sidb == nil {
		return allErrs
	}
	scriptsPath := field.NewPath("spec").Child("scripts")
	legacyPath := field.NewPath("spec").Child("persistence").Child("scriptsVolumeName")
	scripts := sidb.Spec.Scripts
	if scripts == nil {
		return allErrs
	}

	if strings.TrimSpace(sidb.Spec.Persistence.ScriptsVolumeName) != "" {
		allErrs = append(allErrs, field.Forbidden(scriptsPath, "cannot be used together with spec.persistence.scriptsVolumeName"))
		allErrs = append(allErrs, field.Forbidden(legacyPath, "cannot be used together with spec.scripts"))
	}

	hasSetup := scripts.Setup != nil && strings.TrimSpace(scripts.Setup.PvcName) != ""
	hasStartup := scripts.Startup != nil && strings.TrimSpace(scripts.Startup.PvcName) != ""
	if !hasSetup && !hasStartup {
		allErrs = append(allErrs, field.Required(scriptsPath, "set at least one of setup.pvcName or startup.pvcName"))
	}
	if scripts.Setup != nil && strings.TrimSpace(scripts.Setup.PvcName) == "" {
		allErrs = append(allErrs, field.Required(scriptsPath.Child("setup").Child("pvcName"), "pvcName is required when setup is specified"))
	}
	if scripts.Startup != nil && strings.TrimSpace(scripts.Startup.PvcName) == "" {
		allErrs = append(allErrs, field.Required(scriptsPath.Child("startup").Child("pvcName"), "pvcName is required when startup is specified"))
	}
	return allErrs
}

func validateSingleInstanceDatabaseAdditionalPVCs(sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec").Child("persistence").Child("additionalPVCs")
	seenMountPaths := map[string]struct{}{}

	for i := range sidb.Spec.Persistence.AdditionalPVCs {
		itemPath := basePath.Index(i)
		mountPath := strings.TrimSpace(sidb.Spec.Persistence.AdditionalPVCs[i].MountPath)
		pvcName := strings.TrimSpace(sidb.Spec.Persistence.AdditionalPVCs[i].PvcName)
		storageClass := strings.TrimSpace(sidb.Spec.Persistence.AdditionalPVCs[i].StorageClass)
		storageSize := sidb.Spec.Persistence.AdditionalPVCs[i].StorageSizeInGb
		if mountPath == "" {
			allErrs = append(allErrs, field.Required(itemPath.Child("mountPath"), "mountPath must be set"))
			continue
		}
		if !strings.HasPrefix(mountPath, "/") {
			allErrs = append(allErrs, field.Invalid(itemPath.Child("mountPath"), sidb.Spec.Persistence.AdditionalPVCs[i].MountPath, "mountPath must be an absolute path"))
		}
		if _, exists := seenMountPaths[mountPath]; exists {
			allErrs = append(allErrs, field.Duplicate(itemPath.Child("mountPath"), mountPath))
		} else {
			seenMountPaths[mountPath] = struct{}{}
		}

		if pvcName != "" {
			if storageClass != "" {
				allErrs = append(allErrs, field.Forbidden(itemPath.Child("storageClass"), "pvcName is mutually exclusive with storageClass"))
			}
			if storageSize > 0 {
				allErrs = append(allErrs, field.Forbidden(itemPath.Child("storageSizeInGb"), "pvcName is mutually exclusive with storageSizeInGb"))
			}
		} else {
			if storageSize <= 0 {
				allErrs = append(allErrs, field.Required(itemPath.Child("pvcName"), "provide a non-empty pvcName or a positive storageSizeInGb"))
			}
			if storageSize > 0 && storageClass == "" {
				allErrs = append(allErrs, field.Required(itemPath.Child("storageClass"), "storageClass is required when storageSizeInGb is set"))
			}
		}
	}

	return allErrs
}

func resolvePrimarySourceDatabaseRef(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.PrimarySource != nil {
		if ref := strings.TrimSpace(sidb.Spec.PrimarySource.DatabaseRef); ref != "" {
			return ref
		}
	}
	return strings.TrimSpace(sidb.Spec.PrimaryDatabaseRef)
}

func resolvePrimarySourceConnectString(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.PrimarySource != nil {
		if c := strings.TrimSpace(sidb.Spec.PrimarySource.ConnectString); c != "" {
			return c
		}
	}
	return ""
}

func resolvePrimarySourceDBName(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.PrimarySource != nil {
		if dbName := strings.TrimSpace(sidb.Spec.PrimarySource.DBName); dbName != "" {
			return dbName
		}
	}
	return ""
}

func resolvePrimarySourcePdbName(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.PrimarySource != nil {
		if pdbName := strings.TrimSpace(sidb.Spec.PrimarySource.Pdbname); pdbName != "" {
			return pdbName
		}
	}
	return ""
}

func resolvePrimarySourceDetails(sidb *SingleInstanceDatabase) *SingleInstanceDatabasePrimaryDetails {
	if sidb.Spec.PrimarySource != nil && sidb.Spec.PrimarySource.Details != nil {
		return sidb.Spec.PrimarySource.Details
	}
	return nil
}

func resolvePrimarySourceInputPresent(sidb *SingleInstanceDatabase) bool {
	if resolvePrimarySourceDatabaseRef(sidb) != "" {
		return true
	}
	if resolvePrimarySourceConnectString(sidb) != "" {
		return true
	}
	if details := resolvePrimarySourceDetails(sidb); details != nil && strings.TrimSpace(details.Host) != "" {
		return true
	}
	return false
}

func resolveEffectivePrimarySource(sidb *SingleInstanceDatabase) string {
	if ref := resolvePrimarySourceDatabaseRef(sidb); ref != "" {
		return "databaseRef:" + ref + "|dbName:" + resolvePrimarySourceDBName(sidb)
	}
	if connectString := resolvePrimarySourceConnectString(sidb); connectString != "" {
		return "connectString:" + connectString + "|dbName:" + resolvePrimarySourceDBName(sidb) + "|pdbName:" + resolvePrimarySourcePdbName(sidb)
	}
	if details := resolvePrimarySourceDetails(sidb); details != nil {
		return fmt.Sprintf("details:%s:%d/%s/%s|dbName:%s",
			strings.TrimSpace(details.Host),
			details.Port,
			strings.TrimSpace(details.Sid),
			strings.TrimSpace(details.Pdbname),
			resolvePrimarySourceDBName(sidb),
		)
	}
	return ""
}

func isPrimarySourceLocked(sidb *SingleInstanceDatabase) bool {
	if sidb == nil {
		return false
	}
	if sidb.Status.Dataguard != nil && sidb.Status.Dataguard.TopologyLocked {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(sidb.Status.CreatedAs)) {
	case "clone":
		return true
	case "standby":
		return strings.EqualFold(strings.TrimSpace(sidb.Status.DatafilesCreated), "true") ||
			(isPopulatedStatusValue(sidb.Status.Role) && !strings.EqualFold(strings.TrimSpace(sidb.Status.Role), "PRIMARY"))
	case "truecache":
		return strings.EqualFold(strings.TrimSpace(sidb.Status.DatafilesCreated), "true") ||
			isConditionTrue(sidb.Status.Conditions, "TrueCacheBlobSourceReady") ||
			isConditionTrue(sidb.Status.Conditions, "TrueCacheBlobReady")
	default:
		return false
	}
}

func primarySourceLockedMessage(sidb *SingleInstanceDatabase) string {
	if sidb != nil && sidb.Status.Dataguard != nil && sidb.Status.Dataguard.TopologyLocked {
		switch strings.ToLower(strings.TrimSpace(sidb.Status.CreatedAs)) {
		case "clone":
			return "primary source of a cloned database cannot be changed post creation"
		case "standby":
			return "primary source of a standby database cannot be changed after dataguard topology is locked"
		case "truecache":
			return "primary source of a truecache database cannot be changed after dataguard topology is locked"
		default:
			return "primary source cannot be changed after dataguard topology is locked"
		}
	}

	switch strings.ToLower(strings.TrimSpace(sidb.Status.CreatedAs)) {
	case "clone":
		return "primary source of a cloned database cannot be changed post creation"
	case "standby":
		return "primary source of a standby database cannot be changed after datafiles are created or the role is populated"
	case "truecache":
		return "primary source of a truecache database cannot be changed after source resolution or datafile creation begins"
	default:
		return "primary source cannot be changed after creation has progressed"
	}
}

func isPopulatedStatusValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed != dbcommons.ValueUnavailable
}

func isConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	condition := meta.FindStatusCondition(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func validateDataguardProducerSpec(path *field.Path, spec *DataguardProducerSpec) field.ErrorList {
	var allErrs field.ErrorList
	mode := normalizeDataguardProducerMode(spec)
	switch mode {
	case DataguardProducerModeDisabled, DataguardProducerModePreview:
	case DataguardProducerModeManaged:
		allErrs = append(allErrs, field.Forbidden(path.Child("mode"), "Managed mode is reserved for future DataguardBroker automation and is not supported yet"))
	default:
		allErrs = append(allErrs, field.Invalid(path.Child("mode"), mode, "must be Disabled, Preview, or Managed"))
	}
	if spec != nil && spec.Prereqs != nil {
		if dir := strings.TrimSpace(spec.Prereqs.BrokerConfigDir); dir != "" && !strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, "+") {
			allErrs = append(allErrs, field.Invalid(path.Child("prereqs").Child("brokerConfigDir"), spec.Prereqs.BrokerConfigDir, "must be an absolute path or ASM path starting with '+'"))
		}
		if size := strings.TrimSpace(spec.Prereqs.StandbyRedoSize); strings.ContainsAny(size, " \t\r\n") {
			allErrs = append(allErrs, field.Invalid(path.Child("prereqs").Child("standbyRedoSize"), spec.Prereqs.StandbyRedoSize, "must not contain whitespace"))
		}
	}
	if spec != nil {
		seenStandbys := make(map[string]int)
		for i, standby := range spec.StandbySources {
			standbyPath := path.Child("standbySources").Index(i)
			dbUniqueName := strings.ToUpper(strings.TrimSpace(standby.DBUniqueName))
			host := strings.TrimSpace(standby.Host)
			if dbUniqueName == "" {
				allErrs = append(allErrs, field.Required(standbyPath.Child("dbUniqueName"), "dbUniqueName is required"))
			}
			if host == "" {
				allErrs = append(allErrs, field.Required(standbyPath.Child("host"), "host is required"))
			}
			if standby.TCPPort < 0 || standby.TCPPort > 65535 {
				allErrs = append(allErrs, field.Invalid(standbyPath.Child("tcpPort"), standby.TCPPort, "must be between 0 and 65535"))
			}
			if dbUniqueName == "" {
				continue
			}
			key := strings.ToLower(dbUniqueName)
			if prior, exists := seenStandbys[key]; exists {
				allErrs = append(allErrs, field.Duplicate(standbyPath.Child("dbUniqueName"), spec.StandbySources[prior].DBUniqueName))
				continue
			}
			seenStandbys[key] = i
		}
	}
	return allErrs
}

func validatePrimarySourceSpec(sidb *SingleInstanceDatabase, mode string) field.ErrorList {
	var allErrs field.ErrorList
	sourcePath := field.NewPath("spec").Child("primarySource")
	legacyRefPath := field.NewPath("spec").Child("primaryDatabaseRef")

	if strings.TrimSpace(sidb.Spec.PrimaryDatabaseRef) != "" && sidb.Spec.PrimarySource != nil {
		allErrs = append(allErrs, field.Forbidden(legacyRefPath, "deprecated spec.primaryDatabaseRef cannot be used with spec.primarySource"))
	}

	if sidb.Spec.PrimarySource != nil {
		selected := 0
		if strings.TrimSpace(sidb.Spec.PrimarySource.DatabaseRef) != "" {
			selected++
		}
		if strings.TrimSpace(sidb.Spec.PrimarySource.ConnectString) != "" {
			selected++
		}
		if sidb.Spec.PrimarySource.Details != nil {
			selected++
		}

		if selected == 0 {
			allErrs = append(allErrs, field.Required(sourcePath, "set exactly one of databaseRef, connectString, or details"))
		}
		if selected > 1 {
			allErrs = append(allErrs, field.Forbidden(sourcePath, "databaseRef, connectString, and details are mutually exclusive; set only one"))
		}
		if pdbName := strings.TrimSpace(sidb.Spec.PrimarySource.Pdbname); pdbName != "" &&
			strings.TrimSpace(sidb.Spec.PrimarySource.ConnectString) == "" {
			allErrs = append(allErrs, field.Forbidden(sourcePath.Child("pdbName"), "pdbName is supported only together with primarySource.connectString"))
		}
	}

	if (mode == "" || mode == "primary") && (sidb.Spec.PrimarySource != nil || strings.TrimSpace(sidb.Spec.PrimaryDatabaseRef) != "") {
		allErrs = append(allErrs, field.Forbidden(sourcePath, "primary source is supported only when createAs=clone, standby, or truecache"))
	}

	return allErrs
}
