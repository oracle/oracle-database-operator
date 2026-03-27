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
	"strconv"
	"strings"
	"time"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

func (r *SingleInstanceDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-singleinstancedatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,verbs=create;update,versions=v4,name=msingleinstancedatabasev4.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomDefaulter = &SingleInstanceDatabase{}

func (r *SingleInstanceDatabase) Default(ctx context.Context, obj runtime.Object) error {
	sidb, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return apierrors.NewInternalError(fmt.Errorf("failed to cast obj to SingleInstanceDatabase"))
	}

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

	if sidb.Spec.AdminPassword.KeepSecret == nil {
		keepSecret := true
		sidb.Spec.AdminPassword.KeepSecret = &keepSecret
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

	return nil
}

//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v4,name=vsingleinstancedatabasev4.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &SingleInstanceDatabase{}

func (r *SingleInstanceDatabase) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sidb, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast obj to SingleInstanceDatabase"))
	}
	singleinstancedatabaselog.Info("validate create", "name", sidb.Name)

	allErrs := validateSingleInstanceDatabaseSpec(sidb)
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		sidb.Name, allErrs)
}

func (r *SingleInstanceDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldSidb, ok := oldObj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast old object to SingleInstanceDatabase"))
	}
	newSidb, ok := newObj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast new object to SingleInstanceDatabase"))
	}
	singleinstancedatabaselog.Info("validate update", "name", newSidb.Name)

	allErrs := validateSingleInstanceDatabaseSpec(newSidb)

	if oldSidb.Status.CreatedAs == "clone" {
		if newSidb.Spec.Edition != "" && oldSidb.Status.Edition != "" && !strings.EqualFold(oldSidb.Status.Edition, newSidb.Spec.Edition) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("edition"), "edition of a cloned database cannot be changed post creation"))
		}
		if !strings.EqualFold(oldSidb.Status.PrimaryDatabase, resolvePrimaryRefForCloneOrStandby(newSidb)) {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("primaryDatabaseRef"), "primary database of a cloned database cannot be changed post creation"))
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

	if newSidb.Spec.EnableTCPS {
		if oldSidb.Status.DgBroker != nil {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("enableTCPS"), "cannot enable tcps when dataguard broker is configured"))
		} else if len(oldSidb.Status.StandbyDatabases) != 0 {
			allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("enableTCPS"), "cannot enable tcps when standby databases depend on this primary"))
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		newSidb.Name, allErrs)
}

func (r *SingleInstanceDatabase) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*SingleInstanceDatabase)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast obj to SingleInstanceDatabase"))
	}
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

	if sidb.Spec.Persistence.Size == "" && (sidb.Spec.Persistence.AccessMode != "" || sidb.Spec.Persistence.StorageClass != "" || sidb.Spec.Persistence.DatafilesVolumeName != "") {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), sidb.Spec.Persistence.Size, "size is required when persistence settings are specified"))
	}
	if sidb.Spec.Persistence.Size != "" {
		if sidb.Spec.Persistence.AccessMode == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("persistence").Child("accessMode"), sidb.Spec.Persistence.AccessMode, "accessMode is required when persistence size is set"))
		}
		if sidb.Spec.Persistence.AccessMode != "ReadWriteMany" && sidb.Spec.Persistence.AccessMode != "ReadWriteOnce" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("persistence").Child("accessMode"), sidb.Spec.Persistence.AccessMode, "must be ReadWriteOnce or ReadWriteMany"))
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
	if mode == "clone" && resolvePrimaryRefForCloneOrStandby(sidb) == "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("primaryDatabaseRef"), sidb.Spec.PrimaryDatabaseRef, "primary reference is required for clone"))
	}
	if mode == "standby" {
		if resolveStandbyPrimaryInputPresent(sidb) == false {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig"), sidb.Spec.StandbyConfig, "standby requires one primary source: primaryDatabaseRef, standbyConfig.primaryDatabaseRef, standbyConfig.primaryConnectString, or external primary details host/sid"))
		}
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

	if details := resolveExternalPrimaryDetails(sidb); details != nil {
		if strings.TrimSpace(details.Host) == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("primaryDetails").Child("host"), details.Host, "host cannot be empty"))
		}
		if strings.TrimSpace(details.Sid) == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("primaryDetails").Child("sid"), details.Sid, "sid cannot be empty"))
		}
		if details.Port < 0 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("primaryDetails").Child("port"), details.Port, "port cannot be negative"))
		}
	}

	if sidb.Spec.StandbyConfig != nil {
		cfg := sidb.Spec.StandbyConfig
		if strings.TrimSpace(cfg.WalletSecretRef) == "" {
			if strings.TrimSpace(cfg.WalletMountPath) != "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("walletMountPath"), cfg.WalletMountPath, "walletMountPath requires walletSecretRef"))
			}
			if strings.TrimSpace(cfg.WalletZipFileKey) != "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("walletZipFileKey"), cfg.WalletZipFileKey, "walletZipFileKey requires walletSecretRef"))
			}
			if strings.TrimSpace(cfg.StandbyTDEWalletRoot) != "" {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("standbyConfig").Child("standbyTDEWalletRoot"), cfg.StandbyTDEWalletRoot, "standbyTDEWalletRoot requires walletSecretRef"))
			}
		}
	}

	if sidb.Spec.Replicas > 1 {
		valMsg := ""
		if sidb.Spec.Edition == "express" || sidb.Spec.Edition == "free" {
			valMsg = "should be 1 for express/free edition"
		}
		if sidb.Spec.Persistence.Size == "" {
			valMsg = "should be 1 when persistence size is not specified"
		}
		if valMsg != "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("replicas"), sidb.Spec.Replicas, valMsg))
		}
	}

	if sidb.Spec.CreateAs != "truecache" && len(sidb.Spec.TrueCacheServices) > 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("trueCacheServices"), sidb.Spec.TrueCacheServices, "only supported when createAs=truecache"))
	}

	if sidb.Spec.EnableTCPS && sidb.Spec.TcpsCertRenewInterval != "" {
		duration, err := time.ParseDuration(sidb.Spec.TcpsCertRenewInterval)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), sidb.Spec.TcpsCertRenewInterval, "invalid duration"))
		} else {
			maxLimit, _ := time.ParseDuration("8760h")
			minLimit, _ := time.ParseDuration("24h")
			if duration > maxLimit || duration < minLimit {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsCertRenewInterval"), sidb.Spec.TcpsCertRenewInterval, "must be in range 24h to 8760h"))
			}
		}
	}
	if !sidb.Spec.EnableTCPS && sidb.Spec.TcpsTlsSecret != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tcpsTlsSecret"), "allowed only when enableTCPS=true"))
	}
	if sidb.Spec.TcpsTlsSecret != "" && sidb.Spec.TcpsCertRenewInterval != "" {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec").Child("tcpsCertRenewInterval"), "not applicable when tcpsTlsSecret is provided"))
	}
	if sidb.Spec.EnableTCPS && sidb.Spec.ListenerPort != 0 && sidb.Spec.TcpsListenerPort != 0 && sidb.Spec.ListenerPort == sidb.Spec.TcpsListenerPort {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), sidb.Spec.TcpsListenerPort, "listenerPort and tcpsListenerPort cannot be equal"))
	}
	if !sidb.Spec.LoadBalancer {
		if sidb.Spec.ListenerPort != 0 && (sidb.Spec.ListenerPort < 30000 || sidb.Spec.ListenerPort > 32767) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("listenerPort"), sidb.Spec.ListenerPort, "must be in 30000-32767 for NodePort"))
		}
		if sidb.Spec.EnableTCPS && sidb.Spec.TcpsListenerPort != 0 && (sidb.Spec.TcpsListenerPort < 30000 || sidb.Spec.TcpsListenerPort > 32767) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("tcpsListenerPort"), sidb.Spec.TcpsListenerPort, "must be in 30000-32767 for NodePort"))
		}
	}

	if sidb.Spec.InitParams != nil {
		if (sidb.Spec.InitParams.PgaAggregateTarget != 0 && sidb.Spec.InitParams.SgaTarget == 0) || (sidb.Spec.InitParams.PgaAggregateTarget == 0 && sidb.Spec.InitParams.SgaTarget != 0) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("initParams"), sidb.Spec.InitParams, "provide both pgaAggregateTarget and sgaTarget"))
		}
	}

	return allErrs
}

func resolvePrimaryRefForCloneOrStandby(sidb *SingleInstanceDatabase) string {
	if sidb.Spec.StandbyConfig != nil {
		if ref := strings.TrimSpace(sidb.Spec.StandbyConfig.PrimaryDatabaseRef); ref != "" {
			return ref
		}
	}
	return strings.TrimSpace(sidb.Spec.PrimaryDatabaseRef)
}

func resolveExternalPrimaryDetails(sidb *SingleInstanceDatabase) *SingleInstanceDatabaseExternalPrimaryRef {
	if sidb.Spec.StandbyConfig != nil && sidb.Spec.StandbyConfig.PrimaryDetails != nil {
		d := sidb.Spec.StandbyConfig.PrimaryDetails
		return &SingleInstanceDatabaseExternalPrimaryRef{
			Host:    d.Host,
			Port:    d.Port,
			Sid:     d.Sid,
			Pdbname: d.Pdbname,
		}
	}
	return sidb.Spec.ExternalPrimaryDatabaseRef
}

func resolveStandbyPrimaryInputPresent(sidb *SingleInstanceDatabase) bool {
	if resolvePrimaryRefForCloneOrStandby(sidb) != "" {
		return true
	}
	if sidb.Spec.StandbyConfig != nil && strings.TrimSpace(sidb.Spec.StandbyConfig.PrimaryConnectString) != "" {
		return true
	}
	if ext := resolveExternalPrimaryDetails(sidb); ext != nil && strings.TrimSpace(ext.Host) != "" {
		return true
	}
	return false
}
