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
	"regexp"
	"strconv"
	"strings"

	utils "github.com/oracle/oracle-database-operator/commons/oraclerestart/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1 "k8s.io/api/core/v1"
)

// log is for logging in this package.
var OracleRestartlog = logf.Log.WithName("OracleRestart-resource")

func (r *OracleRestart) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&OracleRestart{}).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-oraclerestart,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=oraclerestarts,verbs=create;update,versions=v4,name=moraclerestart.kb.io,admissionReviewVersions={v1}

var _ webhook.CustomDefaulter = &OracleRestart{}

func (r *OracleRestart) Default(ctx context.Context, obj runtime.Object) error {
	cr, ok := obj.(*OracleRestart)
	if !ok {
		return fmt.Errorf("expected *OracleRestart but got %T", obj)
	}

	OracleRestartlog.Info("default", "name", cr.Name)

	if cr.Spec.ImagePullPolicy == nil {
		policy := corev1.PullAlways
		cr.Spec.ImagePullPolicy = &policy
	}

	if cr.Spec.SshKeySecret != nil && cr.Spec.SshKeySecret.KeyMountLocation == "" {
		cr.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSshSecretMount
	}

	if cr.Spec.DbSecret != nil && cr.Spec.DbSecret.Name != "" {
		if cr.Spec.DbSecret.PwdFileMountLocation == "" {
			cr.Spec.DbSecret.PwdFileMountLocation = utils.OraRacDbPwdFileSecretMount
		}
		if cr.Spec.DbSecret.KeyFileMountLocation == "" {
			cr.Spec.DbSecret.KeyFileMountLocation = utils.OraRacDbKeyFileSecretMount
		}
	}

	if cr.Spec.TdeWalletSecret != nil && cr.Spec.TdeWalletSecret.Name != "" {
		if cr.Spec.TdeWalletSecret.PwdFileMountLocation == "" {
			cr.Spec.TdeWalletSecret.PwdFileMountLocation = utils.OraRacTdePwdFileSecretMount
		}
		if cr.Spec.TdeWalletSecret.KeyFileMountLocation == "" {
			cr.Spec.TdeWalletSecret.KeyFileMountLocation = utils.OraRacTdeKeyFileSecretMount
		}
	}

	if cr.Spec.ConfigParams != nil {
		if cr.Spec.ConfigParams.SwMountLocation == "" {
			cr.Spec.ConfigParams.SwMountLocation = utils.OraSwLocation
		}

		if cr.Spec.ConfigParams.DbCharSet == "" {
			cr.Spec.ConfigParams.DbCharSet = "AL32UTF8"
		}
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-oraclerestart,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=oraclerestarts,versions=v4,name=voraclerestart.kb.io,admissionReviewVersions={v1}

var _ webhook.CustomValidator = &OracleRestart{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OracleRestart) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cr, ok := obj.(*OracleRestart)
	if !ok {
		return nil, fmt.Errorf("expected *OracleRestart but got %T", obj)
	}

	OracleRestartlog.Info("validate create", "name", cr.Name)
	var validationErrs field.ErrorList
	var warnings admission.Warnings

	namespaces := utils.GetWatchNamespaces()
	_, containsNamespace := namespaces[cr.Namespace]

	if len(namespaces) != 0 && !containsNamespace {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("metadata").Child("namespace"), cr.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	if cr.Spec.Image == "" {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("image"), cr.Spec.Image,
				"image cannot be set to empty"))
	}

	// validationErrs = append(validationErrs, cr.validateSshSecret()...)
	validationErrs = append(validationErrs, cr.validateDbSecret()...)
	validationErrs = append(validationErrs, cr.validateTdeSecret()...)
	validationErrs = append(validationErrs, cr.validateServiceSpecs()...)
	validationErrs = append(validationErrs, cr.validateAsmStorage()...)
	validationErrs = append(validationErrs, cr.validateGeneric()...)

	cp := cr.Spec.ConfigParams
	fldPath := field.NewPath("spec").Child("configParams")
	safetyPct := 0.8

	if cp != nil {
		// ----- BASIC CONFIG PARAMS VALIDATION -----
		if cp.CpuCount != 0 && cp.CpuCount < 1 {
			validationErrs = append(validationErrs, field.Invalid(
				fldPath.Child("cpuCount"), cp.CpuCount, "if specified, must be greater than zero"))
		}
		if cp.Processes != 0 && cp.Processes < 1 {
			validationErrs = append(validationErrs, field.Invalid(
				fldPath.Child("processes"), cp.Processes, "if specified, must be greater than zero"))
		}
		if cp.HugePages < 0 {
			validationErrs = append(validationErrs, field.Invalid(
				fldPath.Child("hugePages"), cp.HugePages, "cannot be negative"))
		}
		if cp.SgaSize != "" {
			if err := validateMemorySize(cp.SgaSize); err != nil {
				validationErrs = append(validationErrs, field.Invalid(
					fldPath.Child("sgaSize"), cp.SgaSize, err.Error()))
			}
		}
		if cp.PgaSize != "" {
			if err := validateMemorySize(cp.PgaSize); err != nil {
				validationErrs = append(validationErrs, field.Invalid(
					fldPath.Child("pgaSize"), cp.PgaSize, err.Error()))
			}
		}
	}
	var deviceWarnings []string
	// ----- PARSE SGA AND PGA -----
	sga, errSga := parseMem(cp.SgaSize)
	pga, errPga := parseMem(cp.PgaSize)
	if errSga != nil {
		validationErrs = append(validationErrs, field.Invalid(fldPath.Child("sgaSize"), cp.SgaSize, "invalid format"))
	}
	if errPga != nil {
		validationErrs = append(validationErrs, field.Invalid(fldPath.Child("pgaSize"), cp.PgaSize, "invalid format"))
	}

	// ----- EXTRACT POD RESOURCE LIMITS -----
	memLimit := int64(0)
	if cr.Spec.Resources != nil {
		if memQ, ok := cr.Spec.Resources.Limits[corev1.ResourceMemory]; ok {
			memLimit = memQ.Value()
		}
	}

	// ----- SGA + PGA MUST BE WITHIN MEMORY LIMIT -----
	totalMem := sga + pga
	if memLimit > 0 && totalMem > int64(float64(memLimit)*safetyPct) {
		validationErrs = append(validationErrs, field.Invalid(
			fldPath, totalMem,
			fmt.Sprintf("SGA (%dB) + PGA (%dB) must not exceed %d%% of pod memory limit (%dB)", sga, pga, int(safetyPct*100), memLimit)))
	}

	// ----- EXTRACT HUGE PAGES -----
	hugeMem := int64(0)
	if cr.Spec.Resources != nil {
		// Check limits first
		if hpQ, ok := cr.Spec.Resources.Limits["hugepages-2Mi"]; ok {
			hugeMem = hpQ.Value()
		}
		// Fallback: check requests if limits not set
		if hugeMem == 0 {
			if hpQ, ok := cr.Spec.Resources.Requests["hugepages-2Mi"]; ok {
				hugeMem = hpQ.Value()
			}
		}
	}

	// ----- VALIDATE HUGEPAGES -----
	if hugeMem > 0 {
		if hugeMem < sga {
			validationErrs = append(validationErrs, field.Invalid(
				fldPath.Child("hugePages"), hugeMem,
				fmt.Sprintf("HugePages (%d bytes) must be >= SGA size (%d bytes)", hugeMem, sga)))
		}
		if memLimit > 0 && hugeMem > memLimit {
			validationErrs = append(validationErrs, field.Invalid(
				fldPath.Child("hugePages"), hugeMem,
				fmt.Sprintf("HugePages (%d bytes) exceeds pod memory limit (%d bytes)", hugeMem, memLimit)))
		}
	}

	for _, warning := range deviceWarnings {
		warnings = append(warnings, warning)
	}

	if len(validationErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "OracleRestart"},
			cr.Name, validationErrs)
	}

	return warnings, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OracleRestart) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	old, okOld := oldObj.(*OracleRestart)
	newCr, okNew := newObj.(*OracleRestart)
	if !okOld || !okNew {
		return nil, fmt.Errorf("expected *OracleRestart for both old and new objects")
	}

	OracleRestartlog.Info("validate update", "name", newCr.Name)

	if newCr.Status.State == "PROVISIONING" || newCr.Status.State == "UPDATING" || newCr.Status.State == "PODAVAILABLE" {
		if !reflect.DeepEqual(old.Spec, newCr.Spec) {
			return nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
				newCr.Name, fmt.Errorf("updates to Oracle Restart Spec is not allowed while its in state %s", newCr.Status.State))
		}
	}

	if old.Spec.DataDgStorageClass != newCr.Spec.DataDgStorageClass {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("updates to the Data storageclass is forbidden: %s", old.Spec.DataDgStorageClass))
	}

	if old.Spec.RecoDgStorageClass != newCr.Spec.RecoDgStorageClass {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("updates to the Reco storageclass is forbidden: %s", old.Spec.RecoDgStorageClass))
	}

	if old.Spec.RedoDgStorageClass != newCr.Spec.RedoDgStorageClass {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("updates to the Redo storageclass is forbidden: %s", old.Spec.RedoDgStorageClass))
	}

	if old.Spec.SwStorageClass != newCr.Spec.SwStorageClass {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("updates to the Swstorageclass is forbidden: %s", old.Spec.SwStorageClass))
	}

	if old.Spec.CrsDgStorageClass != newCr.Spec.CrsDgStorageClass {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("updates to the CrsDgStorageClass is forbidden: %s", old.Spec.CrsDgStorageClass))
	}

	if newCr.Spec.InstDetails.SwLocStorageSizeInGb < old.Spec.InstDetails.SwLocStorageSizeInGb {

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
			newCr.Name, fmt.Errorf("SwLocStorageSizeInGb Storage size shrink is not allowed. Old value : %d and New value: %d. ", old.Spec.InstDetails.SwLocStorageSizeInGb, newCr.Spec.InstDetails.SwLocStorageSizeInGb))
	}

	var validationErrs field.ErrorList

	// Re-use create validations on update
	warnings, err := r.ValidateCreate(ctx, newObj)
	if err != nil {
		return warnings, err
	}

	// ValidateDelete logic if being deleted
	if newCr.GetDeletionTimestamp() != nil {
		warnings, err := r.ValidateDelete(ctx, newObj)
		if err != nil {
			return warnings, err
		}
	}

	// Skip if only metadata is changing
	if reflect.DeepEqual(old.Spec, newCr.Spec) && reflect.DeepEqual(old.Status, newCr.Status) {
		return nil, nil
	}

	validationErrs = append(validationErrs, newCr.validateUpdateSshSecret(old)...)
	validationErrs = append(validationErrs, newCr.validateUpdateDbSecret(old)...)
	validationErrs = append(validationErrs, newCr.validateUpdateTdeSecret(old)...)
	validationErrs = append(validationErrs, newCr.validateUpdateServiceSpecs(old)...)
	validationErrs = append(validationErrs, newCr.validateUpdateAsmStorage(old)...)
	validationErrs = append(validationErrs, newCr.validateUpdateGeneric(old)...)

	if old.Spec.ConfigParams != nil && newCr.Spec.ConfigParams != nil {

		// // CRS
		// if err := validateRedundancyOnUpdate(old.Spec.ConfigParams.CrsAsmDiskDgRedundancy, newCr.Spec.ConfigParams.CrsAsmDiskDgRedundancy, "crsAsmDiskDgRedundancy"); err != nil {
		// 	validationErrs = append(validationErrs, err)
		// }
		// // DB
		// if err := validateRedundancyOnUpdate(old.Spec.ConfigParams.DBAsmDiskDgRedundancy, newCr.Spec.ConfigParams.DBAsmDiskDgRedundancy, "dbAsmDiskDgRedundancy"); err != nil {
		// 	validationErrs = append(validationErrs, err)
		// }
		// // RECO
		// if err := validateRedundancyOnUpdate(old.Spec.ConfigParams.RecoAsmDiskDgRedundancy, newCr.Spec.ConfigParams.RecoAsmDiskDgRedundancy, "recoAsmDiskDgRedundancy"); err != nil {
		// 	validationErrs = append(validationErrs, err)
		// }
	}

	// if old.Spec.AsmStorageDetails != nil && newCr.Spec.AsmStorageDetails != nil {
	// 	errs := validateAsmNoDiskResize(
	// 		old.Spec.AsmStorageDetails.DisksBySize,
	// 		newCr.Spec.AsmStorageDetails.DisksBySize,
	// 		field.NewPath("spec").Child("asmStorageDetails").Child("disksBySize"),
	// 	)
	// 	validationErrs = append(validationErrs, errs...)
	// }

	// Forbid downscale or warn on SGA/PGA
	oldSga, _ := parseMem(old.Spec.ConfigParams.SgaSize)
	newSga, _ := parseMem(newCr.Spec.ConfigParams.SgaSize)
	if newSga < oldSga {
		validationErrs = append(validationErrs, field.Invalid(
			field.NewPath("spec").Child("configParams").Child("sgaSize"),
			newCr.Spec.ConfigParams.SgaSize, "reducing SGA size after initial deploy is not allowed"))
	}
	// Likewise for PGA
	oldSga, _ = parseMem(old.Spec.ConfigParams.PgaSize)
	newSga, _ = parseMem(newCr.Spec.ConfigParams.PgaSize)
	if newSga < oldSga {
		validationErrs = append(validationErrs, field.Invalid(
			field.NewPath("spec").Child("configParams").Child("sgaSize"),
			newCr.Spec.ConfigParams.SgaSize, "reducing SGA size after initial deploy is not allowed"))
	}

	if len(validationErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "OracleRestart"},
			newCr.Name, validationErrs)
	}

	return nil, nil
}

func (r *OracleRestart) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cr, ok := obj.(*OracleRestart)
	if !ok {
		return nil, fmt.Errorf("expected *OracleRestart but got %T", obj)
	}

	OracleRestartlog.Info("validate delete", "name", cr.Name)

	// TODO: Add any deletion-specific logic if required
	return nil, nil
}

//========== User Functions to check the fields ==========

func (r *OracleRestart) validateDbSecret() field.ErrorList {
	var validationErrs field.ErrorList
	dbPath := field.NewPath("spec").Child("DbSecret")

	if r.Spec.DbSecret.Name != "" && strings.ToLower(r.Spec.DbSecret.EncryptionType) != "base64" {
		if r.Spec.DbSecret.KeyFileName == "" {
			validationErrs = append(validationErrs,
				field.Required(dbPath.Child("KeyFileName"), "KeyFileName cannot be empty when encryptionType is not 'base64'"))
		}
		if r.Spec.DbSecret.PwdFileName == "" {
			validationErrs = append(validationErrs,
				field.Required(dbPath.Child("PwdFileName"), "PwdFileName cannot be empty when encryptionType is not 'base64'"))
		}
	}

	return validationErrs
}
func (r *OracleRestart) validateTdeSecret() field.ErrorList {
	var validationErrs field.ErrorList
	tdePath := field.NewPath("spec").Child("TdeWalletSecret")

	if r.Spec.TdeWalletSecret != nil &&
		r.Spec.TdeWalletSecret.Name != "" &&
		strings.ToLower(r.Spec.TdeWalletSecret.EncryptionType) != "base64" {

		if r.Spec.TdeWalletSecret.KeyFileName == "" {
			validationErrs = append(validationErrs,
				field.Required(tdePath.Child("KeyFileName"), "KeyFileName cannot be empty when encryptionType is not 'base64'"))
		}
		if r.Spec.TdeWalletSecret.PwdFileName == "" {
			validationErrs = append(validationErrs,
				field.Required(tdePath.Child("PwdFileName"), "PwdFileName cannot be empty when encryptionType is not 'base64'"))
		}
	}

	return validationErrs
}
func (r *OracleRestart) validateServiceSpecs() field.ErrorList {
	var validationErrs field.ErrorList
	svcPath := field.NewPath("spec").Child("ServiceDetails")

	svc := r.Spec.ServiceDetails
	if svc.Name == "" {
		return nil
	}

	if svc.Cardinality != "" {
		if len(svc.Preferred) > 0 {
			validationErrs = append(validationErrs,
				field.Invalid(svcPath.Child("Preferred"), svc.Preferred,
					"Preferred cannot be used with Cardinality. Use one or the other."))
		}
		if len(svc.Available) > 0 {
			validationErrs = append(validationErrs,
				field.Invalid(svcPath.Child("Available"), svc.Available,
					"Available cannot be used with Cardinality. Use one or the other."))
		}
		if !utils.CheckStringInList(svc.Cardinality, utils.GetServiceCardinality()) {
			validationErrs = append(validationErrs,
				field.NotSupported(svcPath.Child("Cardinality"), svc.Cardinality, utils.GetServiceCardinality()))
		}
	}

	if svc.TafPolicy != "" && !utils.CheckStringInList(svc.TafPolicy, utils.GetTafPolicy()) {
		validationErrs = append(validationErrs,
			field.NotSupported(svcPath.Child("TafPolicy"), svc.TafPolicy, utils.GetTafPolicy()))
	}

	if svc.FailOverType != "" && !utils.CheckStringInList(svc.FailOverType, utils.GetServiceFailoverType()) {
		validationErrs = append(validationErrs,
			field.NotSupported(svcPath.Child("FailOverType"), svc.FailOverType, utils.GetServiceFailoverType()))
	}

	if svc.Role != "" && !utils.CheckStringInList(svc.Role, utils.GetServiceRole()) {
		validationErrs = append(validationErrs,
			field.NotSupported(svcPath.Child("Role"), svc.Role, utils.GetServiceRole()))
	}

	return validationErrs
}
func (r *OracleRestart) validateAsmStorage() field.ErrorList {
	var validationErrs field.ErrorList
	asmPath := field.NewPath("spec").Child("AsmStorageDetails")

	if r.Spec.AsmStorageDetails == nil {
		validationErrs = append(validationErrs,
			field.Required(asmPath, "ASM storage details must be provided"))
		return validationErrs
	}

	return validationErrs
}

func (r *OracleRestart) validateGeneric() field.ErrorList {
	var validationErrs field.ErrorList

	if !utils.CheckStatusFlag(r.Spec.InstDetails.IsDelete) {
		isAlphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]*$`).MatchString(r.Spec.InstDetails.Name)
		if !isAlphanumeric {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("InstDetails").Child("Name"), r.Spec.InstDetails.Name,
					"Name must contain only alphanumeric characters"))
		}

		if r.Spec.InstDetails.HostSwLocation == "" && r.Spec.SwStorageClass == "" {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("InstDetails").Child("HostSwLocation"), r.Spec.InstDetails.HostSwLocation,
					"Either HostSwLocation or SwStorageClass must be specified"))
		}
	}

	if r.Spec.SwStorageClass != "" {
		if r.Spec.InstDetails.SwLocStorageSizeInGb < 60 {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("InstDetails").Child("SwLocStorageSizeInGb"), r.Spec.InstDetails.SwLocStorageSizeInGb,
					"SwLocStorageSizeInGb must be greater than 60GB"))
		}
	}

	if r.Spec.ConfigParams == nil {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("ConfigParams"), r.Spec.ConfigParams,
				"ConfigParams cannot be empty"))
		return validationErrs
	}

	cfg := r.Spec.ConfigParams
	cfgPath := field.NewPath("spec").Child("ConfigParams")

	// Grid Response File validation
	if cfg.GridResponseFile.ConfigMapName != "" {
		if cfg.GridResponseFile.Name == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("GridResponseFile").Child("Name"), cfg.GridResponseFile.Name,
					"GridResponseFile name cannot be empty"))
		}

	}

	if cfg.GridSwZipFile == "" {
		validationErrs = append(validationErrs,
			field.Invalid(cfgPath.Child("GridSwZipFile"), cfg.GridSwZipFile,
				"GridSwZipFile cannot be empty"))
	}
	if cfg.DbSwZipFile == "" {
		validationErrs = append(validationErrs,
			field.Invalid(cfgPath.Child("DbSwZipFile"), cfg.DbSwZipFile,
				"DbSwZipFile cannot be empty"))
	}

	if cfg.HostSwStageLocation == "" && r.Spec.SwStorageClass == "" {
		validationErrs = append(validationErrs,
			field.Invalid(cfgPath.Child("HostSwStageLocation"), cfg.HostSwStageLocation,
				"Either HostSwStageLocation or SwDgStorageClass must be specified"))
	}

	if r.Spec.ConfigParams.RuPatchLocation != "" {
		_, isPVCKey := r.Spec.InstDetails.PvcName[r.Spec.ConfigParams.RuPatchLocation]
		if !isPVCKey {
			// Not found in PVC map, treat as direct path — validate format
			if !strings.HasPrefix(r.Spec.ConfigParams.RuPatchLocation, "/") {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("configParams").Child("ruPatchLocation"),
						r.Spec.ConfigParams.RuPatchLocation,
						"ruPatchLocation must be either a key in instDetails.pvcName or an absolute path starting with '/'"))
			}
		}
	}

	if r.Spec.Image == "" {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("Image"), r.Spec.Image,
				"Image cannot be empty"))
	}

	return validationErrs
}

//========================== Validate inital specs check ends here ================

// =========================== Update specs checks block begin Here =======================

func (r *OracleRestart) validateUpdateGeneric(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList

	check := func(path *field.Path, oldVal, newVal string) {
		if oldVal != "" && newVal != "" && !strings.EqualFold(oldVal, newVal) {
			validationErrs = append(validationErrs, field.Forbidden(path, path.String()+" cannot be changed post creation"))
		}
	}

	if r.Spec.ConfigParams != nil && old.Spec.ConfigParams != nil {
		cpPath := field.NewPath("spec", "ConfigParams")
		check(cpPath.Child("DbName"), old.Spec.ConfigParams.DbName, r.Spec.ConfigParams.DbName)
		check(cpPath.Child("GridBase"), old.Spec.ConfigParams.GridBase, r.Spec.ConfigParams.GridBase)
		check(cpPath.Child("GridHome"), old.Spec.ConfigParams.GridHome, r.Spec.ConfigParams.GridHome)
		check(cpPath.Child("DbBase"), old.Spec.ConfigParams.DbBase, r.Spec.ConfigParams.DbBase)
		check(cpPath.Child("DbHome"), old.Spec.ConfigParams.DbHome, r.Spec.ConfigParams.DbHome)
		check(cpPath.Child("DbCharSet"), old.Spec.ConfigParams.DbCharSet, r.Spec.ConfigParams.DbCharSet)
		check(cpPath.Child("DbConfigType"), old.Spec.ConfigParams.DbConfigType, r.Spec.ConfigParams.DbConfigType)
		check(cpPath.Child("DbUniqueName"), old.Spec.ConfigParams.DbUniqueName, r.Spec.ConfigParams.DbUniqueName)
		check(cpPath.Child("DbStorageType"), old.Spec.ConfigParams.DbStorageType, r.Spec.ConfigParams.DbStorageType)
		check(cpPath.Child("DbSwZipFile"), old.Spec.ConfigParams.DbSwZipFile, r.Spec.ConfigParams.DbSwZipFile)
		check(cpPath.Child("GridSwZipFile"), old.Spec.ConfigParams.GridSwZipFile, r.Spec.ConfigParams.GridSwZipFile)

		// Nested response files
		check(cpPath.Child("GridResponseFile", "ConfigMapName"), old.Spec.ConfigParams.GridResponseFile.ConfigMapName, r.Spec.ConfigParams.GridResponseFile.ConfigMapName)
		check(cpPath.Child("GridResponseFile", "Name"), old.Spec.ConfigParams.GridResponseFile.Name, r.Spec.ConfigParams.GridResponseFile.Name)
		check(cpPath.Child("DbResponseFile", "ConfigMapName"), old.Spec.ConfigParams.DbResponseFile.ConfigMapName, r.Spec.ConfigParams.DbResponseFile.ConfigMapName)
		check(cpPath.Child("DbResponseFile", "Name"), old.Spec.ConfigParams.DbResponseFile.Name, r.Spec.ConfigParams.DbResponseFile.Name)
	}

	return validationErrs
}

func (r *OracleRestart) validateUpdateServiceSpecs(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList

	check := func(path *field.Path, oldVal, newVal string) {
		if oldVal != "" && newVal != "" && !strings.EqualFold(oldVal, newVal) {
			validationErrs = append(validationErrs, field.Forbidden(path, path.String()+" cannot be changed post creation"))
		}
	}

	sdPath := field.NewPath("spec", "ServiceDetail")

	check(sdPath.Child("Name"), old.Status.ServiceDetails.Name, r.Spec.ServiceDetails.Name)
	check(sdPath.Child("Cardinality"), old.Status.ServiceDetails.Cardinality, r.Spec.ServiceDetails.Cardinality)
	check(sdPath.Child("Notification"), old.Status.ServiceDetails.Notification, r.Spec.ServiceDetails.Notification)
	check(sdPath.Child("ClbGoal"), old.Status.ServiceDetails.ClbGoal, r.Spec.ServiceDetails.ClbGoal)
	check(sdPath.Child("CommitOutCome"), old.Status.ServiceDetails.CommitOutCome, r.Spec.ServiceDetails.CommitOutCome)
	check(sdPath.Child("CommitOutComeFastPath"), old.Status.ServiceDetails.CommitOutComeFastPath, r.Spec.ServiceDetails.CommitOutComeFastPath)
	check(sdPath.Child("Dtp"), old.Status.ServiceDetails.Dtp, r.Spec.ServiceDetails.Dtp)
	check(sdPath.Child("SessionState"), old.Status.ServiceDetails.SessionState, r.Spec.ServiceDetails.SessionState)
	check(sdPath.Child("Edition"), old.Status.ServiceDetails.Edition, r.Spec.ServiceDetails.Edition)
	check(sdPath.Child("FailBack"), old.Status.ServiceDetails.FailBack, r.Spec.ServiceDetails.FailBack)
	check(sdPath.Child("FailOverRestore"), old.Status.ServiceDetails.FailOverRestore, r.Spec.ServiceDetails.FailOverRestore) // ✅ Fixed error message
	check(sdPath.Child("FailOverType"), old.Status.ServiceDetails.FailOverType, r.Spec.ServiceDetails.FailOverType)
	check(sdPath.Child("TafPolicy"), old.Status.ServiceDetails.TafPolicy, r.Spec.ServiceDetails.TafPolicy)
	check(sdPath.Child("RlbGoal"), old.Status.ServiceDetails.RlbGoal, r.Spec.ServiceDetails.RlbGoal)
	check(sdPath.Child("Role"), old.Status.ServiceDetails.Role, r.Spec.ServiceDetails.Role)
	check(sdPath.Child("Pdb"), old.Status.ServiceDetails.Pdb, r.Spec.ServiceDetails.Pdb)

	return validationErrs
}

func (r *OracleRestart) validateUpdateAsmStorage(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList
	// Add actual validation logic here if needed
	if !strings.EqualFold(old.Spec.CrsDgStorageClass, r.Spec.CrsDgStorageClass) {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("CrsDgStorageClass"),
				r.Spec.CrsDgStorageClass, "CrsDgStorageClass cannot be changed post creation"))
	}

	if !strings.EqualFold(old.Spec.CrsDgStorageClass, r.Spec.CrsDgStorageClass) {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("CrsDgStorageClass"),
				r.Spec.CrsDgStorageClass, "CrsDgStorageClass cannot be changed post creation"))
	}

	if !strings.EqualFold(old.Spec.DataDgStorageClass, r.Spec.DataDgStorageClass) {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("DataDgStorageClass"),
				r.Spec.CrsDgStorageClass, "DataDgStorageClass cannot be changed post creation"))
	}

	if !strings.EqualFold(old.Spec.RedoDgStorageClass, r.Spec.RedoDgStorageClass) {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("RedoDgStorageClass"),
				r.Spec.CrsDgStorageClass, "RedoDgStorageClass cannot be changed post creation"))
	}

	if !strings.EqualFold(old.Spec.RecoDgStorageClass, r.Spec.RecoDgStorageClass) {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("RecoDgStorageClass"),
				r.Spec.CrsDgStorageClass, "RecoDgStorageClass cannot be changed post creation"))
	}

	return validationErrs
}

func (r *OracleRestart) validateUpdateDbSecret(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList

	if r.Spec.DbSecret != nil && old.Status.DbSecret != nil {
		if r.Spec.DbSecret.Name != "" && old.Status.DbSecret.Name != "" &&
			!strings.EqualFold(old.Status.DbSecret.Name, r.Spec.DbSecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("DbSecret").Child("Name"),
					"DbSecret name cannot be changed post creation"))
		}

		if r.Spec.DbSecret.KeyFileName != "" && old.Status.DbSecret.KeyFileName != "" &&
			!strings.EqualFold(old.Status.DbSecret.KeyFileName, r.Spec.DbSecret.KeyFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileName"),
					r.Spec.DbSecret.KeyFileName, "KeyFileName cannot be changed post creation"))
		}

		if r.Spec.DbSecret.PwdFileName != "" && old.Status.DbSecret.PwdFileName != "" &&
			!strings.EqualFold(old.Status.DbSecret.PwdFileName, r.Spec.DbSecret.PwdFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileName"),
					r.Spec.DbSecret.PwdFileName, "PwdFileName cannot be changed post creation"))
		}
	}

	return validationErrs
}

func (r *OracleRestart) validateUpdateTdeSecret(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList

	if r.Spec.TdeWalletSecret != nil && old.Status.TdeWalletSecret != nil {
		if r.Spec.TdeWalletSecret.Name != "" && old.Status.TdeWalletSecret.Name != "" &&
			!strings.EqualFold(old.Status.TdeWalletSecret.Name, r.Spec.TdeWalletSecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("TdeWalletSecret").Child("Name"),
					"TdeWalletSecret name cannot be changed post creation"))
		}

		if r.Spec.TdeWalletSecret.KeyFileName != "" && old.Status.TdeWalletSecret.KeyFileName != "" &&
			!strings.EqualFold(old.Status.TdeWalletSecret.KeyFileName, r.Spec.TdeWalletSecret.KeyFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("KeyFileName"),
					r.Spec.TdeWalletSecret.KeyFileName, "KeyFileName cannot be changed post creation"))
		}

		if r.Spec.TdeWalletSecret.PwdFileName != "" && old.Status.TdeWalletSecret.PwdFileName != "" &&
			!strings.EqualFold(old.Status.TdeWalletSecret.PwdFileName, r.Spec.TdeWalletSecret.PwdFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("PwdFileName"),
					r.Spec.TdeWalletSecret.PwdFileName, "PwdFileName cannot be changed post creation"))
		}
	}

	return validationErrs
}

func (r *OracleRestart) validateUpdateSshSecret(old *OracleRestart) field.ErrorList {
	var validationErrs field.ErrorList

	if r.Spec.SshKeySecret != nil && old.Status.SshKeySecret != nil {
		if r.Spec.SshKeySecret.Name != "" && old.Status.SshKeySecret.Name != "" &&
			!strings.EqualFold(old.Status.SshKeySecret.Name, r.Spec.SshKeySecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("SshKeySecret").Child("Name"),
					"SshKeySecret name cannot be changed post creation"))
		}

		if r.Spec.SshKeySecret.PrivKeySecretName != "" && old.Status.SshKeySecret.PrivKeySecretName != "" &&
			!strings.EqualFold(old.Status.SshKeySecret.PrivKeySecretName, r.Spec.SshKeySecret.PrivKeySecretName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("SshKeySecret").Child("PrivKeySecretName"),
					r.Spec.SshKeySecret.PrivKeySecretName, "PrivKeySecretName cannot be changed post creation"))
		}

		if r.Spec.SshKeySecret.PubKeySecretName != "" && old.Status.SshKeySecret.PubKeySecretName != "" &&
			!strings.EqualFold(old.Status.SshKeySecret.PubKeySecretName, r.Spec.SshKeySecret.PubKeySecretName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("SshKeySecret").Child("PubKeySecretName"),
					r.Spec.SshKeySecret.PubKeySecretName, "PubKeySecretName cannot be changed post creation"))
		}
	}

	return validationErrs
}

func getDeviceCount(deviceList string) int {
	if deviceList == "" {
		return 0
	}
	devices := strings.Split(deviceList, ",")
	count := 0
	for _, d := range devices {
		if strings.TrimSpace(d) != "" {
			count++
		}
	}
	return count
}

func (r *OracleRestart) validateAsmRedundancyAndDisks(
	devList, redundancy, paramField string,
) field.ErrorList {
	var errs field.ErrorList
	diskCount := getDeviceCount(devList)

	// Only validate if at least ONE of devList or redundancy is set/non-empty
	if strings.TrimSpace(redundancy) == "" {
		// Both are empty, nothing to validate
		return errs
	}

	switch strings.ToUpper(redundancy) {
	case "EXTERNAL":
		if diskCount < 1 {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("configParams").Child(paramField),
				devList,
				"EXTERNAL redundancy requires disk count minimum 1",
			))
		}
	case "NORMAL":
		if diskCount < 2 {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("configParams").Child(paramField),
				devList,
				"NORMAL redundancy requires disk count minimum 2",
			))
		}
	case "HIGH":
		if diskCount < 3 {
			errs = append(errs, field.Invalid(
				field.NewPath("spec").Child("configParams").Child(paramField),
				devList,
				"HIGH redundancy requires disk count minimum 3",
			))
		}
	default:
		errs = append(errs, field.Invalid(
			field.NewPath("spec").Child("configParams").Child(paramField),
			redundancy,
			"Invalid redundancy type; must be EXTERNAL, NORMAL, or HIGH",
		))
	}
	return errs
}

func validateMemorySize(sizeStr string) error {
	matched, _ := regexp.MatchString(`^\d+(Gi|Mi|G|M)$`, sizeStr)
	if !matched {
		return fmt.Errorf("memory size must be of form <number>[M|G|Mi|Gi], e.g., 3G, 1024M, 16Gi")
	}
	return nil
}

const safetyPct = 0.80 // Only 80% of pod memory can be used for SGA+PGA

func parseMem(memStr string) (int64, error) {
	if memStr == "" {
		return 0, nil
	}

	// Identify unit (supports M, G, Mi, Gi)
	var numStr string
	var multiplier int64

	if strings.HasSuffix(memStr, "Gi") || strings.HasSuffix(memStr, "gi") {
		numStr = memStr[:len(memStr)-2]
		multiplier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(memStr, "Mi") || strings.HasSuffix(memStr, "mi") {
		numStr = memStr[:len(memStr)-2]
		multiplier = 1024 * 1024
	} else if strings.HasSuffix(memStr, "G") || strings.HasSuffix(memStr, "g") {
		numStr = memStr[:len(memStr)-1]
		multiplier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(memStr, "M") || strings.HasSuffix(memStr, "m") {
		numStr = memStr[:len(memStr)-1]
		multiplier = 1024 * 1024
	} else {
		return 0, fmt.Errorf("invalid memory unit in %s", memStr)
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in %s", memStr)
	}

	return int64(num) * multiplier, nil
}

func validateOracleSysctls(sysctls map[string]string, sgaBytes int64, pageSize int64) field.ErrorList {
	var errs field.ErrorList
	// kernel.sem = "250 32000 100 128"
	if val, ok := sysctls["kernel.sem"]; ok {
		parts := strings.Fields(val)
		if len(parts) != 4 {
			errs = append(errs, field.Invalid(field.NewPath("kernel.sem"), val, "must have 4 space-separated integers"))
		} else {
			if v, _ := strconv.Atoi(parts[0]); v < 250 {
				errs = append(errs, field.Invalid(field.NewPath("kernel.sem"), parts[0], "semmsl must be >= 250"))
			}
			if v, _ := strconv.Atoi(parts[1]); v < 32000 {
				errs = append(errs, field.Invalid(field.NewPath("kernel.sem"), parts[1], "semmns must be >= 32000"))
			}
			if v, _ := strconv.Atoi(parts[2]); v < 100 {
				errs = append(errs, field.Invalid(field.NewPath("kernel.sem"), parts[2], "semopm must be >= 100"))
			}
			if v, _ := strconv.Atoi(parts[3]); v < 128 {
				errs = append(errs, field.Invalid(field.NewPath("kernel.sem"), parts[3], "semmni must be >= 128"))
			}
		}
	}

	// kernel.shmall ≥ kernel.shmmax / pageSize
	if val, ok := sysctls["kernel.shmall"]; ok {
		shmall, err := strconv.ParseInt(val, 10, 64)
		if err != nil || shmall < sgaBytes/pageSize {
			errs = append(errs, field.Invalid(field.NewPath("kernel.shmall"), val, fmt.Sprintf("must be >= shmmax/pageSize = %d", sgaBytes/pageSize)))
		}
	}

	// kernel.shmmax ≥ half of physical RAM (at minimum, let's assume SGA or higher as stricter validation)
	if val, ok := sysctls["kernel.shmmax"]; ok {
		shmmax, err := strconv.ParseInt(val, 10, 64)
		if err != nil || shmmax < (sgaBytes) {
			errs = append(errs, field.Invalid(field.NewPath("kernel.shmmax"), val, fmt.Sprintf("should be >= SGA size (%d)", sgaBytes)))
		}
	}

	// kernel.shmmni ≥ 4096
	if val, ok := sysctls["kernel.shmmni"]; ok {
		shmmni, err := strconv.ParseInt(val, 10, 64)
		if err != nil || shmmni < 4096 {
			errs = append(errs, field.Invalid(field.NewPath("kernel.shmmni"), val, "should be >= 4096"))
		}
	}

	// kernel.panic_on_oops = 1
	if val, ok := sysctls["kernel.panic_on_oops"]; ok {
		pan, err := strconv.Atoi(val)
		if err != nil || pan != 1 {
			errs = append(errs, field.Invalid(field.NewPath("kernel.panic_on_oops"), val, "must be 1"))
		}
	}

	// kernel.panic ≥ 10
	if val, ok := sysctls["kernel.panic"]; ok {
		pan, err := strconv.Atoi(val)
		if err != nil || pan < 10 {
			errs = append(errs, field.Invalid(field.NewPath("kernel.panic"), val, "should be at least 10"))
		}
	}

	// fs.file-max ≥ 6815744
	if val, ok := sysctls["fs.file-max"]; ok {
		fm, err := strconv.ParseInt(val, 10, 64)
		if err != nil || fm < 6815744 {
			errs = append(errs, field.Invalid(field.NewPath("fs.file-max"), val, "should be ≥ 6815744"))
		}
	}

	// fs.aio-max-nr ≥ 1048576
	if val, ok := sysctls["fs.aio-max-nr"]; ok {
		aio, err := strconv.ParseInt(val, 10, 64)
		if err != nil || aio < 1048576 {
			errs = append(errs, field.Invalid(field.NewPath("fs.aio-max-nr"), val, "should be ≥ 1048576"))
		}
	}

	// net.ipv4.ip_local_port_range must have min >= 9000, max <= 65535
	if val, ok := sysctls["net.ipv4.ip_local_port_range"]; ok {
		parts := strings.Fields(val)
		if len(parts) != 2 {
			errs = append(errs, field.Invalid(field.NewPath("net.ipv4.ip_local_port_range"), val, "must have two space-separated integers (e.g., 9000 65535)"))
		} else {
			min, _ := strconv.Atoi(parts[0])
			max, _ := strconv.Atoi(parts[1])
			if min < 9000 {
				errs = append(errs, field.Invalid(field.NewPath("net.ipv4.ip_local_port_range"), parts[0], "minimum must be ≥ 9000"))
			}
			if max > 65535 {
				errs = append(errs, field.Invalid(field.NewPath("net.ipv4.ip_local_port_range"), parts[1], "maximum must be ≤ 65535"))
			}
		}
	}

	// net.core.rmem_default ≥ 262144
	if val, ok := sysctls["net.core.rmem_default"]; ok {
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil || v < 262144 {
			errs = append(errs, field.Invalid(field.NewPath("net.core.rmem_default"), val, "should be ≥ 262144"))
		}
	}

	// net.core.rmem_max ≥ 4194304
	if val, ok := sysctls["net.core.rmem_max"]; ok {
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil || v < 4194304 {
			errs = append(errs, field.Invalid(field.NewPath("net.core.rmem_max"), val, "should be ≥ 4194304"))
		}
	}

	// net.core.wmem_default ≥ 262144
	if val, ok := sysctls["net.core.wmem_default"]; ok {
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil || v < 262144 {
			errs = append(errs, field.Invalid(field.NewPath("net.core.wmem_default"), val, "should be ≥ 262144"))
		}
	}

	// net.core.wmem_max ≥ 1048576
	if val, ok := sysctls["net.core.wmem_max"]; ok {
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil || v < 1048576 {
			errs = append(errs, field.Invalid(field.NewPath("net.core.wmem_max"), val, "should be ≥ 1048576"))
		}
	}
	return errs
}

// =========================== Update specs checks block ends Here =======================
