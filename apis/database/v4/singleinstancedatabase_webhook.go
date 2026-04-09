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
	"strconv"
	"strings"
	"time"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

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
	if adminPassword != nil && adminPassword.KeepSecret == nil {
		keepSecret := true
		adminPassword.KeepSecret = &keepSecret
	}

	if sidb.Spec.CreateAs == "" {
		sidb.Spec.CreateAs = "primary"
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
	defaultSIDBPersistence(&sidb.Spec.Persistence)
	defaultSIDBAdditionalPVCs(&sidb.Spec.Persistence.AdditionalPVCs)
	defaultSIDBRestoreSpec(&sidb.Spec.Restore)

	return nil
}

//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v4,name=vsingleinstancedatabasev4.kb.io,admissionReviewVersions={v1,v1beta1}

var _ admission.Validator[*SingleInstanceDatabase] = &SingleInstanceDatabase{}

func sidbTcpsEnabled(sidb *SingleInstanceDatabase) bool {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && sidb.Spec.Security.TCPS.Enabled {
		return true
	}
	if sidb.Spec.TCPS != nil && sidb.Spec.TCPS.Enabled {
		return true
	}
	return sidb.Spec.EnableTCPS
}

func sidbTcpsListenerPort(sidb *SingleInstanceDatabase) int {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && sidb.Spec.Security.TCPS.ListenerPort != 0 {
		return sidb.Spec.Security.TCPS.ListenerPort
	}
	if sidb.Spec.TCPS != nil && sidb.Spec.TCPS.ListenerPort != 0 {
		return sidb.Spec.TCPS.ListenerPort
	}
	return sidb.Spec.TcpsListenerPort
}

func sidbTcpsTlsSecret(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.TlsSecret) != "" {
		return strings.TrimSpace(sidb.Spec.Security.TCPS.TlsSecret)
	}
	if sidb.Spec.TCPS != nil && strings.TrimSpace(sidb.Spec.TCPS.TlsSecret) != "" {
		return strings.TrimSpace(sidb.Spec.TCPS.TlsSecret)
	}
	return strings.TrimSpace(sidb.Spec.TcpsTlsSecret)
}

func sidbTcpsCertRenewInterval(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.Security != nil && sidb.Spec.Security.TCPS != nil && strings.TrimSpace(sidb.Spec.Security.TCPS.CertRenewInterval) != "" {
		return strings.TrimSpace(sidb.Spec.Security.TCPS.CertRenewInterval)
	}
	if sidb.Spec.TCPS != nil && strings.TrimSpace(sidb.Spec.TCPS.CertRenewInterval) != "" {
		return strings.TrimSpace(sidb.Spec.TCPS.CertRenewInterval)
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
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		sidb.Name, allErrs)
}

func (r *SingleInstanceDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj *SingleInstanceDatabase) (admission.Warnings, error) {
	oldSidb, newSidb := oldObj, newObj
	singleinstancedatabaselog.Info("validate update", "name", newSidb.Name)

	allErrs := validateSingleInstanceDatabaseSpec(newSidb)
	specChanged := !reflect.DeepEqual(oldSidb.Spec, newSidb.Spec)
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
		if resolveEffectivePrimarySource(oldSidb) != resolveEffectivePrimarySource(newSidb) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("primarySource"), "primary source of a cloned database cannot be changed post creation"))
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
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		newSidb.Name, allErrs)
}

func (r *SingleInstanceDatabase) ValidateDelete(ctx context.Context, obj *SingleInstanceDatabase) (admission.Warnings, error) {
	return nil, nil
}

func validateSingleInstanceDatabaseSpec(sidb *SingleInstanceDatabase) field.ErrorList {
	allErrs := field.ErrorList{}

	namespaces := dbcommons.GetWatchNamespaces()
	if len(namespaces) != 0 {
		if _, ok := namespaces[sidb.Namespace]; !ok {
			allErrs = append(allErrs, field.Invalid(field.NewPath("metadata").Child("namespace"), sidb.Namespace, "operator does not watch this namespace"))
		}
	}

	oradata := sidbOradataPersistence(sidb)
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
		if strings.TrimSpace(fra.RecoveryAreaSize) != "" && strings.TrimSpace(fra.Size) != "" {
			fraSize, errSize := resource.ParseQuantity(strings.TrimSpace(fra.Size))
			recoverySize, errRecovery := resource.ParseQuantity(strings.TrimSpace(fra.RecoveryAreaSize))
			if errSize == nil && errRecovery == nil && recoverySize.Cmp(fraSize) > 0 {
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
		valMsg := ""
		if sidb.Spec.Edition == "express" || sidb.Spec.Edition == "free" {
			valMsg = "should be 1 for express/free edition"
		}
		if oradata == nil || (strings.TrimSpace(oradata.Size) == "" && strings.TrimSpace(oradata.PvcName) == "") {
			valMsg = "should be 1 when persistence size is not specified"
		}
		if valMsg != "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("replicas"), sidb.Spec.Replicas, valMsg))
		}
	}

	tcpsEnabled := sidbTcpsEnabled(sidb)
	tcpsTlsSecret := sidbTcpsTlsSecret(sidb)
	tcpsCertRenewInterval := sidbTcpsCertRenewInterval(sidb)
	tcpsListenerPort := sidbTcpsListenerPort(sidb)

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
	if tcpsTlsSecret != "" && tcpsCertRenewInterval != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tcpsCertRenewInterval"), "not applicable when tcpsTlsSecret is provided"))
	}
	if tcpsEnabled && sidb.Spec.ListenerPort != 0 && tcpsListenerPort != 0 && sidb.Spec.ListenerPort == tcpsListenerPort {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), tcpsListenerPort, "listenerPort and tcpsListenerPort cannot be equal"))
	}
	if !sidb.Spec.LoadBalancer {
		if sidb.Spec.ListenerPort != 0 && (sidb.Spec.ListenerPort < 30000 || sidb.Spec.ListenerPort > 32767) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("listenerPort"), sidb.Spec.ListenerPort, "must be in 30000-32767 for NodePort"))
		}
		if tcpsEnabled && tcpsListenerPort != 0 && (tcpsListenerPort < 30000 || tcpsListenerPort > 32767) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), tcpsListenerPort, "must be in 30000-32767 for NodePort"))
		}
	}
	allErrs = append(allErrs, validateTNSAliases(sidb)...)

	if sidb.Spec.InitParams != nil {
		if (sidb.Spec.InitParams.PgaAggregateTarget != 0 && sidb.Spec.InitParams.SgaTarget == 0) || (sidb.Spec.InitParams.PgaAggregateTarget == 0 && sidb.Spec.InitParams.SgaTarget != 0) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("initParams"), sidb.Spec.InitParams, "provide both pgaAggregateTarget and sgaTarget"))
		}
	}

	allErrs = append(allErrs, validateSingleInstanceDatabaseResourceFields(sidb)...)
	allErrs = append(allErrs, validateSingleInstanceDatabaseAdditionalPVCs(sidb)...)

	return allErrs
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

	validateLegacyQuantity := func(value string, fld *field.Path) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		q, err := resource.ParseQuantity(trimmed)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fld, value, "invalid quantity"))
			return
		}
		if q.Sign() < 0 {
			allErrs = append(allErrs, field.Invalid(fld, value, "must be non-negative"))
		}
	}

	if sidb.Spec.Resources.Requests != nil {
		validateLegacyQuantity(sidb.Spec.Resources.Requests.Cpu, specPath.Child("resources").Child("requests").Child("cpu"))
		validateLegacyQuantity(sidb.Spec.Resources.Requests.Memory, specPath.Child("resources").Child("requests").Child("memory"))
	}
	if sidb.Spec.Resources.Limits != nil {
		validateLegacyQuantity(sidb.Spec.Resources.Limits.Cpu, specPath.Child("resources").Child("limits").Child("cpu"))
		validateLegacyQuantity(sidb.Spec.Resources.Limits.Memory, specPath.Child("resources").Child("limits").Child("memory"))
	}

	if sidb.Spec.ResourceRequirements != nil {
		for name, q := range sidb.Spec.ResourceRequirements.Requests {
			if q.Sign() < 0 {
				allErrs = append(allErrs, field.Invalid(specPath.Child("resourceRequirements").Child("requests").Child(string(name)), q.String(), "must be non-negative"))
			}
		}
		for name, q := range sidb.Spec.ResourceRequirements.Limits {
			if q.Sign() < 0 {
				allErrs = append(allErrs, field.Invalid(specPath.Child("resourceRequirements").Child("limits").Child(string(name)), q.String(), "must be non-negative"))
			}
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
		if len(tc.TrueCacheServices) > 0 {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("trueCacheServices"), "supported only when createAs=truecache"))
		}
		if strings.TrimSpace(tc.GeneratePath) != "" && !tc.GenerateEnabled {
			allErrs = append(allErrs, field.Invalid(tcPath.Child("generatePath"), tc.GeneratePath, "requires generateEnabled=true"))
		}
		return allErrs
	}

	if mode == "truecache" {
		if tc == nil {
			return allErrs
		}
		if tc.GenerateEnabled {
			allErrs = append(allErrs, field.Forbidden(tcPath.Child("generateEnabled"), "supported only when createAs=primary"))
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
		allErrs = append(allErrs, field.Forbidden(tcPath, "supported only when createAs=primary (generateEnabled/generatePath) or createAs=truecache"))
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
		if (*pvcs)[i].MountPath == DefaultDiagMountPath && (*pvcs)[i].PvcName == "" && (*pvcs)[i].StorageSizeInGb <= 0 {
			(*pvcs)[i].StorageSizeInGb = DefaultDiagSizeInGb
		}
	}
}

func validateSingleInstanceDatabaseAdditionalPVCs(sidb *SingleInstanceDatabase) field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec").Child("persistence").Child("additionalPVCs")
	seenMountPaths := map[string]struct{}{}

	for i := range sidb.Spec.Persistence.AdditionalPVCs {
		itemPath := basePath.Index(i)
		mountPath := strings.TrimSpace(sidb.Spec.Persistence.AdditionalPVCs[i].MountPath)
		pvcName := strings.TrimSpace(sidb.Spec.Persistence.AdditionalPVCs[i].PvcName)
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

		if pvcName == "" && sidb.Spec.Persistence.AdditionalPVCs[i].StorageSizeInGb <= 0 && mountPath != DefaultDiagMountPath {
			allErrs = append(allErrs, field.Required(itemPath.Child("storageSizeInGb"), "storageSizeInGb must be greater than 0 when pvcName is not provided"))
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
		return "databaseRef:" + ref
	}
	if connectString := resolvePrimarySourceConnectString(sidb); connectString != "" {
		return "connectString:" + connectString
	}
	if details := resolvePrimarySourceDetails(sidb); details != nil {
		return fmt.Sprintf("details:%s:%d/%s/%s",
			strings.TrimSpace(details.Host),
			details.Port,
			strings.TrimSpace(details.Sid),
			strings.TrimSpace(details.Pdbname),
		)
	}
	return ""
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
	}

	if (mode == "" || mode == "primary") && (sidb.Spec.PrimarySource != nil || strings.TrimSpace(sidb.Spec.PrimaryDatabaseRef) != "") {
		allErrs = append(allErrs, field.Forbidden(sourcePath, "primary source is supported only when createAs=clone, standby, or truecache"))
	}

	return allErrs
}
