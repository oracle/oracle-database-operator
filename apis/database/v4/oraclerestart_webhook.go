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

		if cr.Spec.ConfigParams.GridResponseFile.ConfigMapName == "" {
			if cr.Spec.ConfigParams.CrsAsmDiskDg == "" {
				cr.Spec.ConfigParams.CrsAsmDiskDg = "+DATA"
			}
			if cr.Spec.ConfigParams.CrsAsmDiskDgRedundancy == "" {
				cr.Spec.ConfigParams.CrsAsmDiskDgRedundancy = "external"
			}
		}

		if cr.Spec.ConfigParams.DbResponseFile.ConfigMapName == "" {
			if cr.Spec.ConfigParams.DbDataFileDestDg == "" {
				cr.Spec.ConfigParams.DbDataFileDestDg = cr.Spec.ConfigParams.CrsAsmDiskDg
			}
			if cr.Spec.ConfigParams.DbRecoveryFileDest == "" {
				cr.Spec.ConfigParams.DbRecoveryFileDest = cr.Spec.ConfigParams.DbDataFileDestDg
			}
			if cr.Spec.ConfigParams.DbCharSet == "" {
				cr.Spec.ConfigParams.DbCharSet = "AL32UTF8"
			}
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

	validationErrs = append(validationErrs, cr.validateSshSecret()...)
	validationErrs = append(validationErrs, cr.validateDbSecret()...)
	validationErrs = append(validationErrs, cr.validateTdeSecret()...)
	validationErrs = append(validationErrs, cr.validateServiceSpecs()...)
	validationErrs = append(validationErrs, cr.validateAsmStorage()...)
	validationErrs = append(validationErrs, cr.validateGeneric()...)

	// ASM disk warnings
	var deviceWarnings []string
	w, errs := cr.validateCrsAsmDeviceListSize()
	deviceWarnings = append(deviceWarnings, w...)
	validationErrs = append(validationErrs, errs...)

	w, errs = cr.validateDbAsmDeviceList()
	deviceWarnings = append(deviceWarnings, w...)
	validationErrs = append(validationErrs, errs...)

	w, errs = cr.validateRecoAsmDeviceList()
	deviceWarnings = append(deviceWarnings, w...)
	validationErrs = append(validationErrs, errs...)

	w, errs = cr.validateRedoAsmDeviceList()
	deviceWarnings = append(deviceWarnings, w...)
	validationErrs = append(validationErrs, errs...)

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
				newCr.Name, fmt.Errorf("updates to RAC Spec are not allowed while RAC is in state %s", newCr.Status.State))
		}
	}

	isDiskChanged := !reflect.DeepEqual(old.Spec.AsmStorageDetails.DisksBySize, newCr.Spec.AsmStorageDetails.DisksBySize)
	if isDiskChanged {
		if old.Spec.ConfigParams.HostSwStageLocation != newCr.Spec.ConfigParams.HostSwStageLocation ||
			old.Spec.ConfigParams.GridSwZipFile != newCr.Spec.ConfigParams.GridSwZipFile ||
			old.Spec.ConfigParams.DbSwZipFile != newCr.Spec.ConfigParams.DbSwZipFile ||
			old.Spec.Image != newCr.Spec.Image {

			return nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "database.oracle.com", Resource: "OracleRestart"},
				newCr.Name, fmt.Errorf("updates to the following fields are not allowed during ASM disk updates: %v", []string{"hostSwStageLocation", "gridSwZipFile", "dbSwZipFile", "image"}))
		}
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

func (r *OracleRestart) validateSshSecret() field.ErrorList {
	var validationErrs field.ErrorList
	sshPath := field.NewPath("spec").Child("SshKeySecret")

	if r.Spec.SshKeySecret == nil {
		validationErrs = append(validationErrs,
			field.Required(sshPath, "SshKeySecret must be specified"))
		return validationErrs
	}

	if r.Spec.SshKeySecret.Name == "" {
		validationErrs = append(validationErrs,
			field.Required(sshPath.Child("Name"), "SshKeySecret.Name cannot be empty"))
	}
	if r.Spec.SshKeySecret.PrivKeySecretName == "" {
		validationErrs = append(validationErrs,
			field.Required(sshPath.Child("PrivKeySecretName"), "PrivKeySecretName cannot be empty"))
	}
	if r.Spec.SshKeySecret.PubKeySecretName == "" {
		validationErrs = append(validationErrs,
			field.Required(sshPath.Child("PubKeySecretName"), "PubKeySecretName cannot be empty"))
	}

	return validationErrs
}
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

	if len(r.Spec.AsmStorageDetails.DisksBySize) == 0 {
		validationErrs = append(validationErrs,
			field.Invalid(asmPath.Child("DisksBySize"), r.Spec.AsmStorageDetails.DisksBySize,
				"At least one disk size group must be defined"))
	} else {
		for i, group := range r.Spec.AsmStorageDetails.DisksBySize {
			if len(group.DiskNames) == 0 {
				validationErrs = append(validationErrs,
					field.Invalid(asmPath.Child("DisksBySize").Index(i).Child("DiskNames"), group.DiskNames,
						"Each disk size group must have at least one disk name"))
			}
		}
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

		if r.Spec.InstDetails.HostSwLocation == "" && r.Spec.StorageClass == "" {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("InstDetails").Child("HostSwLocation"), r.Spec.InstDetails.HostSwLocation,
					"Either HostSwLocation or StorageClass must be specified"))
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

		for _, fieldVal := range []struct {
			name  string
			value string
		}{
			{"Inventory", cfg.Inventory},
			{"CrsAsmDeviceList", cfg.CrsAsmDeviceList},
			{"GridBase", cfg.GridBase},
			{"CrsAsmDiskDg", cfg.CrsAsmDiskDg},
			{"CrsAsmDiskDgRedundancy", cfg.CrsAsmDiskDgRedundancy},
		} {
			if fieldVal.value != "" {
				validationErrs = append(validationErrs,
					field.Invalid(cfgPath.Child(fieldVal.name), fieldVal.value,
						fmt.Sprintf("%s cannot be used when GridResponseFile is set", fieldVal.name)))
			}
		}
	} else {
		if cfg.GridBase == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("GridBase"), cfg.GridBase, "GridBase cannot be empty"))
		}
		if cfg.GridHome == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("GridHome"), cfg.GridHome, "GridHome cannot be empty"))
		}
		if cfg.Inventory == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("Inventory"), cfg.Inventory, "Inventory cannot be empty"))
		}
		if cfg.CrsAsmDeviceList == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("CrsAsmDeviceList"), cfg.CrsAsmDeviceList, "CrsAsmDeviceList cannot be empty"))
		}
	}

	// DB Response File validation
	if cfg.DbResponseFile.ConfigMapName != "" {
		if cfg.DbResponseFile.Name == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("DbResponseFile").Child("Name"), cfg.DbResponseFile.Name,
					"DbResponseFile name cannot be empty"))
		}

		for _, fieldVal := range []struct {
			name  string
			value string
		}{
			{"DbCharSet", cfg.DbCharSet},
			{"DbConfigType", cfg.DbConfigType},
			{"DbRedoFileSize", cfg.DbRedoFileSize},
			{"DbType", cfg.DbType},
			{"DbUniqueName", cfg.DbUniqueName},
			{"DbStorageType", cfg.DbStorageType},
			{"DbName", cfg.DbName},
		} {
			if fieldVal.value != "" {
				validationErrs = append(validationErrs,
					field.Invalid(cfgPath.Child(fieldVal.name), fieldVal.value,
						fmt.Sprintf("%s cannot be used when DbResponseFile is set", fieldVal.name)))
			}
		}
	} else {
		if cfg.DbBase == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("DbBase"), cfg.DbBase, "DbBase cannot be empty"))
		}
		if cfg.DbHome == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("DbHome"), cfg.DbHome, "DbHome cannot be empty"))
		}
		if cfg.DbName == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("DbName"), cfg.DbName, "DbName cannot be empty"))
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

	if !utils.CheckStatusFlag(r.Spec.UseNfsforSwStorage) {
		if cfg.HostSwStageLocation == "" && r.Spec.StorageClass == "" {
			validationErrs = append(validationErrs,
				field.Invalid(cfgPath.Child("HostSwStageLocation"), cfg.HostSwStageLocation,
					"Either HostSwStageLocation or StorageClass must be specified"))
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
		check(cpPath.Child("CrsAsmDiskDg"), old.Spec.ConfigParams.CrsAsmDiskDg, r.Spec.ConfigParams.CrsAsmDiskDg)
		check(cpPath.Child("CrsAsmDiskDgRedundancy"), old.Spec.ConfigParams.CrsAsmDiskDgRedundancy, r.Spec.ConfigParams.CrsAsmDiskDgRedundancy)
		check(cpPath.Child("DBAsmDiskDgRedundancy"), old.Spec.ConfigParams.DBAsmDiskDgRedundancy, r.Spec.ConfigParams.DBAsmDiskDgRedundancy)
		check(cpPath.Child("DbCharSet"), old.Spec.ConfigParams.DbCharSet, r.Spec.ConfigParams.DbCharSet)
		check(cpPath.Child("DbConfigType"), old.Spec.ConfigParams.DbConfigType, r.Spec.ConfigParams.DbConfigType)
		check(cpPath.Child("DbDataFileDestDg"), old.Spec.ConfigParams.DbDataFileDestDg, r.Spec.ConfigParams.DbDataFileDestDg)
		check(cpPath.Child("DbUniqueName"), old.Spec.ConfigParams.DbUniqueName, r.Spec.ConfigParams.DbUniqueName)
		check(cpPath.Child("DbRecoveryFileDest"), old.Spec.ConfigParams.DbRecoveryFileDest, r.Spec.ConfigParams.DbRecoveryFileDest)
		check(cpPath.Child("DbRedoFileSize"), old.Spec.ConfigParams.DbRedoFileSize, r.Spec.ConfigParams.DbRedoFileSize)
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

func (r *OracleRestart) validateAsmDeviceList(deviceListStr, deviceListName string) ([]string, field.ErrorList) {
	var warnings []string
	var validationErrs field.ErrorList

	// Skip validation if the device list is empty or not provided
	if deviceListStr == "" {
		return warnings, validationErrs
	}

	if r.Spec.AsmStorageDetails == nil {
		validationErrs = append(validationErrs,
			field.Required(field.NewPath("spec").Child("AsmStorageDetails"),
				"ASM storage details must be provided when device list is specified"))
		return warnings, validationErrs
	}

	deviceList := strings.Split(deviceListStr, ",")
	var sizeGroup string // Placeholder for the expected storage size as string

	for _, device := range deviceList {
		found := false
		for _, diskBySize := range r.Spec.AsmStorageDetails.DisksBySize {
			// Check if the device exists in the current size group
			if contains(diskBySize.DiskNames, device) {
				// Check for storage size mismatch
				if sizeGroup == "" {
					// Set the expected size group on first match
					sizeGroup = fmt.Sprintf("%d", diskBySize.StorageSizeInGb)
				} else if sizeGroup != fmt.Sprintf("%d", diskBySize.StorageSizeInGb) {
					// Add warning for size mismatch
					warnings = append(warnings,
						fmt.Sprintf("Disk %s in %s is not of the same storage size as others (%s GB expected, but found %s GB)",
							device, deviceListName, sizeGroup, fmt.Sprintf("%d", diskBySize.StorageSizeInGb)))
				}
				found = true
				break
			}
		}
		// Error if a device in the list is not found in any DisksBySize group
		if !found {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("AsmStorageDetails").Child(deviceListName), device,
					fmt.Sprintf("Disk %s not found in any storage size group", device)))
		}
	}

	return warnings, validationErrs
}

func (r *OracleRestart) validateCrsAsmDeviceListSize() ([]string, field.ErrorList) {
	var warnings []string
	var validationErrs field.ErrorList

	if r.Spec.ConfigParams == nil || r.Spec.ConfigParams.CrsAsmDeviceList == "" {
		return warnings, validationErrs
	}

	return r.validateAsmDeviceList(r.Spec.ConfigParams.CrsAsmDeviceList, "CrsAsmDeviceList")
}

func (r *OracleRestart) validateDbAsmDeviceList() ([]string, field.ErrorList) {
	var warnings []string
	var validationErrs field.ErrorList

	if r.Spec.ConfigParams == nil || r.Spec.ConfigParams.DbAsmDeviceList == "" {
		return warnings, validationErrs
	}

	return r.validateAsmDeviceList(r.Spec.ConfigParams.DbAsmDeviceList, "DbAsmDeviceList")
}

func (r *OracleRestart) validateRecoAsmDeviceList() ([]string, field.ErrorList) {
	var warnings []string
	var validationErrs field.ErrorList

	if r.Spec.ConfigParams == nil || r.Spec.ConfigParams.RecoAsmDeviceList == "" {
		return warnings, validationErrs
	}

	return r.validateAsmDeviceList(r.Spec.ConfigParams.RecoAsmDeviceList, "RecoAsmDeviceList")
}

func (r *OracleRestart) validateRedoAsmDeviceList() ([]string, field.ErrorList) {
	var warnings []string
	var validationErrs field.ErrorList

	if r.Spec.ConfigParams == nil || r.Spec.ConfigParams.RedoAsmDeviceList == "" {
		return warnings, validationErrs
	}

	return r.validateAsmDeviceList(r.Spec.ConfigParams.RedoAsmDeviceList, "RedoAsmDeviceList")
}

// Helper function to check if a slice contains a specific element
func contains(slice []string, item string) bool {
	for _, elem := range slice {
		if elem == item {
			return true
		}
	}
	return false
}

// =========================== Update specs checks block ends Here =======================
