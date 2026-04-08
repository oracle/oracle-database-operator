// Package v4 provides RAC API definitions aligned with docs/rac and Kubernetes controller guidance.
//
// Support:
//   - Operator user guide: docs/rac
//   - Kubernetes controller overview: https://kubernetes.io/docs/concepts/architecture/controller/
//
// Contributing:
//   - Repository guidelines: https://github.com/oracle/oracle-database-operator/blob/main/CONTRIBUTING.md
//   - Example manifests: https://github.com/oracle/oracle-database-operator/blob/main/docs/rac/provisioning/racdb_prov_quickstart.yaml
//
// Help:
//   - Issues tracker: https://github.com/oracle/oracle-database-operator/blob/main/README.md#help
//   - Sample CRD walkthrough: https://github.com/oracle/oracle-database-operator/blob/main/docs/rac/README.md

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

//nolint:unused // legacy validation helpers are retained for staged rollout and backward compatibility.
package v4

// revive:disable:unused-parameter,receiver-naming,var-naming
// Legacy webhook signatures and helper names are preserved for backward compatibility.

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	utils "github.com/oracle/oracle-database-operator/commons/crs/rac/utils"
	sharedresources "github.com/oracle/oracle-database-operator/commons/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var racdatabaselog = logf.Log.WithName("racdatabase-resource")

// SetupWebhookWithManager registers the RAC database webhook with the manager.
func (r *RacDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy[*RacDatabase](mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-racdatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=racdatabases,verbs=create;update,versions=v4,name=mracdatabase.kb.io,admissionReviewVersions={v1}

var _ admission.Defaulter[*RacDatabase] = &RacDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *RacDatabase) Default(ctx context.Context, obj *RacDatabase) error {
	cr := obj

	racdatabaselog.Info("default", "name", cr.Name)

	if cr.Spec.ImagePullPolicy == nil || *cr.Spec.ImagePullPolicy == corev1.PullPolicy("") {
		policy := corev1.PullPolicy("Always")
		cr.Spec.ImagePullPolicy = &policy
	}

	if cr.Spec.SshKeySecret != nil {
		if cr.Spec.SshKeySecret.KeyMountLocation == "" {
			cr.Spec.SshKeySecret.KeyMountLocation = utils.OraRacSSHSecretMount
		}
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

	}

	return nil
}

//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-racdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=racdatabases,versions=v4,name=vracdatabase.kb.io,admissionReviewVersions={v1}

var _ admission.Validator[*RacDatabase] = &RacDatabase{}

// ValidateCreate implements webhook.CustomValidator
func (r *RacDatabase) ValidateCreate(ctx context.Context, obj *RacDatabase) (admission.Warnings, error) {
	cr := obj
	racdatabaselog.Info("validate create", "name", cr.Name)

	var validationErrs field.ErrorList
	var warnings admission.Warnings

	// ----- NAMESPACE VALIDATION -----
	namespaces := utils.GetWatchNamespaces()
	if len(namespaces) > 0 {
		if _, ok := namespaces[cr.Namespace]; !ok {
			validationErrs = append(validationErrs,
				field.Invalid(
					field.NewPath("metadata").Child("namespace"),
					cr.Namespace,
					"Oracle database operator doesn't watch over this namespace",
				),
			)
		}
	}

	if cr.Spec.Image == "" {
		validationErrs = append(validationErrs,
			field.Invalid(
				field.NewPath("spec").Child("image"),
				cr.Spec.Image,
				"image cannot be set to empty",
			),
		)
	}

	// ----- RUN EXISTING VALIDATIONS -----
	for _, vfn := range []func() field.ErrorList{
		cr.validateSshSecret,
		cr.validateDbSecret,
		cr.validateTdeSecret,
		cr.validateServiceSpecs,
		cr.validateAsmStorage,
		cr.validateGeneric,
	} {
		if errs := vfn(); errs != nil {
			validationErrs = append(validationErrs, errs...)
		}
	}

	cp := cr.Spec.ConfigParams
	fldPath := field.NewPath("spec").Child("configParams")
	const safetyPct = 0.8

	if cp != nil {
		// ----- BASIC CONFIG PARAM VALIDATION -----
		if cp.CpuCount != 0 && cp.CpuCount < 1 {
			validationErrs = append(validationErrs,
				field.Invalid(fldPath.Child("cpuCount"), cp.CpuCount, "if specified, must be greater than zero"))
		}
		if cp.Processes != 0 && cp.Processes < 1 {
			validationErrs = append(validationErrs,
				field.Invalid(fldPath.Child("processes"), cp.Processes, "if specified, must be greater than zero"))
		}
		if cp.HugePages < 0 {
			validationErrs = append(validationErrs,
				field.Invalid(fldPath.Child("hugePages"), cp.HugePages, "cannot be negative"))
		}
		if cp.SgaSize != "" {
			if err := validateMemorySize(cp.SgaSize); err != nil {
				validationErrs = append(validationErrs,
					field.Invalid(fldPath.Child("sgaSize"), cp.SgaSize, err.Error()))
			}
		}
		if cp.PgaSize != "" {
			if err := validateMemorySize(cp.PgaSize); err != nil {
				validationErrs = append(validationErrs,
					field.Invalid(fldPath.Child("pgaSize"), cp.PgaSize, err.Error()))
			}
		}
	}

	sgaInput := ""
	pgaInput := ""
	if cp != nil {
		sgaInput = cp.SgaSize
		pgaInput = cp.PgaSize
	}

	// ----- PARSE SGA / PGA -----
	sga, errSga := parseMem(sgaInput)
	pga, errPga := parseMem(pgaInput)
	if errSga != nil {
		validationErrs = append(validationErrs,
			field.Invalid(fldPath.Child("sgaSize"), sgaInput, "invalid format"))
	}
	if errPga != nil {
		validationErrs = append(validationErrs,
			field.Invalid(fldPath.Child("pgaSize"), pgaInput, "invalid format"))
	}

	memLimit, hugeMem := sharedresources.ExtractMemoryAndHugePagesBytes(cr.Spec.Resources)

	// ----- SGA + PGA SAFETY CHECK (FIXED) -----
	if err := sharedresources.ValidateSgaPgaSafety(sga, pga, memLimit, hugeMem, safetyPct); err != nil {
		validationErrs = append(validationErrs,
			field.Invalid(
				fldPath,
				sga+pga,
				err.Error(),
			),
		)
	}

	// ----- VALIDATE HUGEPAGES (FIXED) -----
	if err := sharedresources.ValidateHugePagesAtLeastSga(hugeMem, sga); err != nil {
		validationErrs = append(validationErrs,
			field.Invalid(
				fldPath.Child("hugePages"),
				hugeMem,
				err.Error(),
			),
		)
	}

	// ----- MIN MEMORY VALIDATION (UNCHANGED) -----
	const minMemoryBytes = 16 * 1024 * 1024 * 1024 // 16GiB
	validationErrs = append(validationErrs,
		validateMinMemoryLimit(
			cr.Spec.Resources,
			minMemoryBytes,
			field.NewPath("spec"),
		)...,
	)
	// // ----- HUGE PAGES REQUIRE CPU + MEMORY (MANDATORY) -----
	// if hugeMem > 0 {

	// 	// ---- memory check ----
	// 	if memLimit == 0 {
	// 		validationErrs = append(validationErrs,
	// 			field.Required(
	// 				field.NewPath("spec").Child("resources").Child("limits").Child("memory"),
	// 				"memory limit is mandatory when hugepages are specified",
	// 			),
	// 		)
	// 	}

	// 	// ---- cpu check ----
	// 	var cpuLimit int64
	// 	if cr.Spec.Resources != nil {
	// 		if cpuQ, ok := cr.Spec.Resources.Limits[corev1.ResourceCPU]; ok {
	// 			cpuLimit = cpuQ.MilliValue()
	// 		}
	// 		if cpuLimit == 0 {
	// 			if cpuQ, ok := cr.Spec.Resources.Requests[corev1.ResourceCPU]; ok {
	// 				cpuLimit = cpuQ.MilliValue()
	// 			}
	// 		}
	// 	}

	// 	if cpuLimit == 0 {
	// 		validationErrs = append(validationErrs,
	// 			field.Required(
	// 				field.NewPath("spec").Child("resources").Child("limits").Child("cpu"),
	// 				"cpu limit or request is mandatory when hugepages are specified",
	// 			),
	// 		)
	// 	}

	// }

	// ---- cpuCount sanity ----
	// if cp != nil && cp.CpuCount <= 0 {
	// 	validationErrs = append(validationErrs,
	// 		field.Invalid(
	// 			field.NewPath("spec").Child("configParams").Child("cpuCount"),
	// 			cp.CpuCount,
	// 			"cpuCount must be set when hugepages are specified",
	// 		),
	// 	)
	// }

	if len(validationErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "RacDatabase"},
			cr.Name,
			validationErrs,
		)
	}

	return warnings, nil
}

// validateAsmStorage ensures ASM storage specifications meet required rules.
func (cr *RacDatabase) validateAsmStorage() field.ErrorList {
	var allErrs field.ErrorList

	// Check at least one disk group is provided
	if len(cr.Spec.AsmStorageDetails) == 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("asmDiskGroupDetails"), "At least one ASM disk group must be defined"))
		return allErrs
	}

	// Check at least one disk is provided in total
	atLeastOneDisk := false
	for _, dg := range cr.Spec.AsmStorageDetails {
		if len(dg.Disks) > 0 {
			atLeastOneDisk = true
			break
		}
	}
	if !atLeastOneDisk {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("asmDiskGroupDetails"), "At least one disk must be specified in at least one ASM disk group"))
		return allErrs
	}
	allowedDGTypes := map[AsmDiskDGTypes]bool{
		CrsAsmDiskDg:     true,
		DbDataDiskDg:     true,
		DbRecoveryDiskDg: true,
		RedoDiskDg:       true,
		OthersDiskDg:     true,
	}
	for idx, dg := range cr.Spec.AsmStorageDetails {
		dp := field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx)

		// NEW: Validate disk group type is supported
		if dg.Type != "" {
			if _, ok := allowedDGTypes[dg.Type]; !ok {
				allErrs = append(allErrs,
					field.Invalid(
						dp.Child("type"),
						dg.Type,
						fmt.Sprintf(
							"Invalid ASM disk group type. Supported values are: %s",
							strings.Join([]string{
								string(CrsAsmDiskDg),
								string(DbDataDiskDg),
								string(DbRecoveryDiskDg),
								string(RedoDiskDg),
								string(OthersDiskDg),
							}, ", "),
						),
					),
				)
				continue
			}
		}
	}
	diskToGroup := make(map[string]string)
	seenTypes := make(map[AsmDiskDGTypes]string)

	for idx, dg := range cr.Spec.AsmStorageDetails {
		// 1. Types must be unique per group
		if existingName, exists := seenTypes[dg.Type]; exists {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx).Child("type"),
					dg.Type, fmt.Sprintf("Type %q is already used by disk group %q. Types must be unique.", dg.Type, existingName)))
		} else {
			seenTypes[dg.Type] = dg.Name
		}

		// 2. Disks must be unique
		for didx, disk := range dg.Disks {
			if existingGroup, exists := diskToGroup[disk]; exists {
				allErrs = append(allErrs,
					field.Duplicate(field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx).Child("disks").Index(didx), disk))
				allErrs = append(allErrs,
					field.Invalid(field.NewPath("spec").Child("asmDiskGroupDetails"),
						disk, fmt.Sprintf("Disk %s is already assigned to group %q. Disks must be unique among all groups.", disk, existingGroup)))
			} else {
				diskToGroup[disk] = dg.Name
			}
		}
	}

	gridRspSet := false
	if cp := cr.Spec.ConfigParams; cp != nil {
		if cp.GridResponseFile != nil && cp.GridResponseFile.ConfigMapName != "" {
			gridRspSet = true
		}
	}

	for idx, dg := range cr.Spec.AsmStorageDetails {
		dp := field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx)
		effectiveStorageClass := strings.TrimSpace(dg.StorageClass)
		if effectiveStorageClass == "" {
			effectiveStorageClass = strings.TrimSpace(cr.Spec.StorageClass)
		}
		if effectiveStorageClass != "" && dg.AsmStorageSizeInGb <= 0 {
			allErrs = append(allErrs,
				field.Invalid(
					dp.Child("asmStorageSizeInGb"),
					dg.AsmStorageSizeInGb,
					"asmStorageSizeInGb must be greater than zero when storageClass is configured for this disk group",
				),
			)
		}

		if gridRspSet {
			// type: Can only be "OTHERS" or blank
			if dg.Type != "" && dg.Type != AsmDiskDGTypes("OTHERS") {
				allErrs = append(allErrs,
					field.Invalid(dp.Child("type"), dg.Type, "When gridResponseFile is set, type must be 'OTHERS' or omitted"))
			}
			// name must be blank
			if dg.Name != "" {
				allErrs = append(allErrs,
					field.Invalid(dp.Child("name"), dg.Name, "When gridResponseFile is set, name must not be specified"))
			}
			// redundancy must be blank
			if dg.Redundancy != "" {
				allErrs = append(allErrs,
					field.Invalid(dp.Child("redundancy"), dg.Redundancy, "When gridResponseFile is set, redundancy must not be specified"))
			}

		} else {
			// Call redundancy/disk count validation only when gridRsp is not set
			redundancy := dg.Redundancy
			devList := strings.Join(dg.Disks, ",") // adjust to fit getDeviceCount
			paramField := fmt.Sprintf("asmDiskGroupDetails[%d].disks", idx)
			asmErrs := cr.validateRACAsmRedundancyAndDisks(devList, redundancy, paramField)
			allErrs = append(allErrs, asmErrs...)
		}
	}

	// Check at least one disk group is provided
	if len(cr.Spec.AsmStorageDetails) == 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("asmDiskGroupDetails"), "At least one ASM disk group must be defined"))
		return allErrs
	}

	return allErrs
}

// ValidateDelete implements webhook.CustomValidator
func (r *RacDatabase) ValidateDelete(ctx context.Context, obj *RacDatabase) (admission.Warnings, error) {
	racdatabaselog.Info("validate delete", "name", r.Name)
	// Add delete validation logic if needed
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator
func (r *RacDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj *RacDatabase) (admission.Warnings, error) {
	racdatabaselog.Info("validate update", "name", r.Name)

	oldCr := oldObj
	newCr := newObj

	racdatabaselog.Info("validate update", "name", newCr.Name)

	var validationErrs field.ErrorList
	var warnings admission.Warnings

	// Block spec updates in certain states
	if newCr.Status.State == "PROVISIONING" ||
		newCr.Status.State == "UPDATING" ||
		newCr.Status.State == "PODAVAILABLE" ||
		newCr.Status.State == string(RACAddInstState) ||
		newCr.Status.State == string(RACDeletingState) {
		if !reflect.DeepEqual(oldCr.Spec, newCr.Spec) {
			return nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "database.oracle.com", Resource: "RacDatabase"},
				newCr.Name,
				fmt.Errorf("updates to RAC Spec are not allowed while RAC is in state %s", newCr.Status.State))
		}
	}

	// Fields locked during ASM disk changes
	lockedFields := []string{"hostSwStageLocation", "swStagePvc", "swStagePvcMountLocation", "gridSwZipFile", "dbSwZipFile", "image", "racSwPrefix"}
	// if isDiskChanged {
	if oldCr.Spec.ConfigParams.HostSwStageLocation != newCr.Spec.ConfigParams.HostSwStageLocation ||
		oldCr.Spec.ConfigParams.SwStagePvc != newCr.Spec.ConfigParams.SwStagePvc ||
		oldCr.Spec.ConfigParams.SwStagePvcMountLocation != newCr.Spec.ConfigParams.SwStagePvcMountLocation ||
		oldCr.Spec.ConfigParams.GridSwZipFile != newCr.Spec.ConfigParams.GridSwZipFile ||
		oldCr.Spec.ConfigParams.DbSwZipFile != newCr.Spec.ConfigParams.DbSwZipFile ||
		oldCr.Spec.RacSwPrefix != newCr.Spec.RacSwPrefix ||
		oldCr.Spec.Image != newCr.Spec.Image {
		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "RacDatabase"},
			newCr.Name,
			// fmt.Errorf("updates to the following fields are not allowed during ASM disk updates: %v", lockedFields))
			fmt.Errorf("updates to the following fields are not allowed : %v", lockedFields))
	}
	// }

	// Reuse create-time validations (optional but sometimes useful)
	createWarnings, err := newCr.ValidateCreate(ctx, newCr)
	if err != nil {
		return createWarnings, err
	}
	warnings = append(warnings, createWarnings...)

	// Validate deletion if object is terminating
	if !newCr.DeletionTimestamp.IsZero() {
		deleteWarnings, err := newCr.ValidateDelete(ctx, newCr)
		if err != nil {
			return deleteWarnings, err
		}
		warnings = append(warnings, deleteWarnings...)
	}

	// Run delete validations if deletion timestamp is set
	if newCr.GetDeletionTimestamp() != nil {
		warnings, err = newCr.ValidateDelete(ctx, newObj)
		if err != nil {
			return warnings, err
		}
	}

	// Only finalizer updates → allow
	if reflect.DeepEqual(oldCr.Spec, newCr.Spec) && reflect.DeepEqual(oldCr.Status, newCr.Status) {
		return nil, nil
	}

	for _, vfn := range []func(*RacDatabase) field.ErrorList{
		newCr.validateUpdateSshSecret,
		newCr.validateUpdateDbSecret,
		newCr.validateUpdateTdeSecret,
		newCr.validateUpdateServiceSpecs,
		newCr.validateUpdatePrivateIPSpecs,
		newCr.validateUpdateAsmStorage,
		newCr.validateUpdateGeneric,
	} {
		if errs := vfn(oldCr); errs != nil {
			validationErrs = append(validationErrs, errs...)
		}
	}

	// Forbid downscale or warn on SGA/PGA
	oldSga, _ := parseMem(oldCr.Spec.ConfigParams.SgaSize)
	newSga, _ := parseMem(newCr.Spec.ConfigParams.SgaSize)
	if newSga < oldSga {
		validationErrs = append(validationErrs, field.Invalid(
			field.NewPath("spec").Child("configParams").Child("sgaSize"),
			newCr.Spec.ConfigParams.SgaSize, "reducing SGA size after initial deploy is not allowed"))
	}
	// Likewise for PGA
	oldSga, _ = parseMem(oldCr.Spec.ConfigParams.PgaSize)
	newSga, _ = parseMem(newCr.Spec.ConfigParams.PgaSize)
	if newSga < oldSga {
		validationErrs = append(validationErrs, field.Invalid(
			field.NewPath("spec").Child("configParams").Child("sgaSize"),
			newCr.Spec.ConfigParams.SgaSize, "reducing SGA size after initial deploy is not allowed"))
	}
	// ----- VALIDATE MIN MEMORY (16GB) -----
	const minMemoryBytes = 16 * 1024 * 1024 * 1024 // 16GB

	validationErrs = append(validationErrs,
		validateMinMemoryLimit(
			newCr.Spec.Resources,
			minMemoryBytes,
			field.NewPath("spec"),
		)...,
	)
	// ------------------------------------------------------------------
	// REUSE CREATE-TIME MEMORY / HUGEPAGE VALIDATIONS FOR UPDATE
	// ------------------------------------------------------------------

	cp := newCr.Spec.ConfigParams
	fldPath := field.NewPath("spec").Child("configParams")
	const safetyPct = 0.8

	sgaInput := ""
	pgaInput := ""
	if cp != nil {
		sgaInput = cp.SgaSize
		pgaInput = cp.PgaSize
	}

	// ----- PARSE SGA / PGA -----
	sga, errSga := parseMem(sgaInput)
	pga, errPga := parseMem(pgaInput)

	if errSga != nil {
		validationErrs = append(validationErrs,
			field.Invalid(fldPath.Child("sgaSize"), sgaInput, "invalid format"))
	}
	if errPga != nil {
		validationErrs = append(validationErrs,
			field.Invalid(fldPath.Child("pgaSize"), pgaInput, "invalid format"))
	}

	memLimit, hugeMem := sharedresources.ExtractMemoryAndHugePagesBytes(newCr.Spec.Resources)

	// ----- SGA + PGA SAFETY CHECK -----
	if err := sharedresources.ValidateSgaPgaSafety(sga, pga, memLimit, hugeMem, safetyPct); err != nil {
		validationErrs = append(validationErrs,
			field.Invalid(
				fldPath,
				sga+pga,
				err.Error(),
			),
		)
	}

	// ----- HUGE PAGES >= SGA -----
	if err := sharedresources.ValidateHugePagesAtLeastSga(hugeMem, sga); err != nil {
		validationErrs = append(validationErrs,
			field.Invalid(
				fldPath.Child("hugePages"),
				hugeMem,
				err.Error(),
			),
		)
	}

	// // ----- HUGE PAGES REQUIRE CPU + MEMORY -----
	// if hugeMem > 0 {

	// 	// memory mandatory
	// 	if memLimit == 0 {
	// 		validationErrs = append(validationErrs,
	// 			field.Required(
	// 				field.NewPath("spec").Child("resources").Child("limits").Child("memory"),
	// 				"memory limit is mandatory when hugepages are specified",
	// 			),
	// 		)
	// 	}

	// 	// cpu mandatory
	// 	var cpuLimit int64
	// 	if newCr.Spec.Resources != nil {
	// 		if cpuQ, ok := newCr.Spec.Resources.Limits[corev1.ResourceCPU]; ok {
	// 			cpuLimit = cpuQ.MilliValue()
	// 		}
	// 		if cpuLimit == 0 {
	// 			if cpuQ, ok := newCr.Spec.Resources.Requests[corev1.ResourceCPU]; ok {
	// 				cpuLimit = cpuQ.MilliValue()
	// 			}
	// 		}
	// 	}

	// 	if cpuLimit == 0 {
	// 		validationErrs = append(validationErrs,
	// 			field.Required(
	// 				field.NewPath("spec").Child("resources").Child("limits").Child("cpu"),
	// 				"cpu limit or request is mandatory when hugepages are specified",
	// 			),
	// 		)
	// 	}

	// 	// cpuCount mandatory
	// 	if cp != nil && cp.CpuCount <= 0 {
	// 		validationErrs = append(validationErrs,
	// 			field.Invalid(
	// 				field.NewPath("spec").Child("configParams").Child("cpuCount"),
	// 				cp.CpuCount,
	// 				"cpuCount must be set when hugepages are specified",
	// 			),
	// 		)
	// 	}
	// }

	if len(validationErrs) > 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "RacDatabase"},
			newCr.Name, validationErrs)
	}

	return warnings, nil
}

// validateSshSecret verifies SSH secret references and required fields.
// validateSshSecret verifies SSH secret references and required fields.
func (r *RacDatabase) validateSshSecret() field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.SshKeySecret == nil {
		validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("SshKeySecret"), r.Spec.SshKeySecret,
			"SshSecret cannot be set to empty"))
	} else {
		if r.Spec.SshKeySecret.Name == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("SshKeySecret").Child("Name"), r.Spec.SshKeySecret.Name,
				"SshSecret name cannot be set to empty"))
		}

		if r.Spec.SshKeySecret.PrivKeySecretName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("PrivKeySecretName"), r.Spec.SshKeySecret.PrivKeySecretName,
				"PrivKeySecretName cannot be set to empty"))
		}

		if r.Spec.SshKeySecret.PubKeySecretName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("PubKeySecretName"), r.Spec.SshKeySecret.PubKeySecretName,
				"PubKeySecretName cannot be set to empty"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil
}

// validateDbSecret checks database secret configuration for completeness.
// validateDbSecret checks database secret configuration for completeness.
func (r *RacDatabase) validateDbSecret() field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.DbSecret == nil {
		return nil
	}

	if r.Spec.DbSecret.Name == "" {
		return nil
	}

	if r.Spec.DbSecret.SecretKey != "" {
		return nil
	}

	if r.Spec.DbSecret.KeyFileName != "" || r.Spec.DbSecret.PwdFileName != "" {
		if r.Spec.DbSecret.KeyFileName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileName"), r.Spec.DbSecret.KeyFileName,
				"KeyFileName cannot be set to empty"))
		}

		if r.Spec.DbSecret.PwdFileName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileName"), r.Spec.DbSecret.PwdFileName,
				"PwdFileName cannot be set to empty"))
		}

		if len(validationErrs) > 0 {
			return validationErrs
		}

		return nil
	}

	validationErrs = append(validationErrs, field.Invalid(
		field.NewPath("spec").Child("DbSecret"),
		r.Spec.DbSecret,
		"either 'key' OR ('keyFileName' and 'pwdFileName') must be specified",
	))
	return validationErrs
}

// validateTdeSecret validates TDE wallet secret references.
// validateTdeSecret validates TDE wallet secret references.
func (r *RacDatabase) validateTdeSecret() field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.TdeWalletSecret == nil {
		return nil
	}

	if r.Spec.TdeWalletSecret.Name == "" {
		return nil
	}

	if r.Spec.TdeWalletSecret.SecretKey != "" {
		return nil
	}

	if r.Spec.TdeWalletSecret.KeyFileName != "" || r.Spec.TdeWalletSecret.PwdFileName != "" {
		if r.Spec.TdeWalletSecret.KeyFileName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("KeyFileName"), r.Spec.TdeWalletSecret.KeyFileName,
				"KeyFileName cannot be set to empty"))
		}

		if r.Spec.TdeWalletSecret.PwdFileName == "" {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("PwdFileName"), r.Spec.TdeWalletSecret.PwdFileName,
				"PwdFileName cannot be set to empty"))
		}

		if len(validationErrs) > 0 {
			return validationErrs
		}

		return nil
	}

	validationErrs = append(validationErrs, field.Invalid(
		field.NewPath("spec").Child("TdeWalletSecret"),
		r.Spec.TdeWalletSecret,
		"either 'key' OR ('keyFileName' and 'pwdFileName') must be specified",
	))
	return validationErrs
}

// validateServiceSpecs validates RAC service configuration settings.
// validateServiceSpecs validates RAC service configuration settings.
func (r *RacDatabase) validateServiceSpecs() field.ErrorList {

	var validationErrs field.ErrorList

	// ======> Service Specs Check Start here ====>
	if r.Spec.ServiceDetails.Name != "" {
		//Check Service cardinality
		if (r.Spec.ServiceDetails.Cardinality != "") && (len(r.Spec.ServiceDetails.Preferred) > 0) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("Preferred"), r.Spec.ServiceDetails.Preferred,
					"Preferred cannot be used with cardinality. You can use either one of them"))
		}

		if (r.Spec.ServiceDetails.Cardinality != "") && (len(r.Spec.ServiceDetails.Available) > 0) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("Available"), r.Spec.ServiceDetails.Available,
					"Available cannot be used with cardinality. You can use either one of them"))
		}

		if r.Spec.ServiceDetails.Cardinality != "" {
			if !utils.CheckStringInList(r.Spec.ServiceDetails.Cardinality, utils.GetServiceCardinality()) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("Cardinality"), r.Spec.ServiceDetails.Cardinality,
						"Cardinality values can be only set to"+strings.Join(utils.GetServiceCardinality(), " ")))
			}
		}

		if r.Spec.ServiceDetails.TafPolicy != "" {
			if !utils.CheckStringInList(r.Spec.ServiceDetails.TafPolicy, utils.GetTafPolicy()) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("TafPolicy"), r.Spec.ServiceDetails.TafPolicy,
						"TafPolicy values can be only set to"+strings.Join(utils.GetTafPolicy(), " ")))
			}
		}

		if r.Spec.ServiceDetails.FailOverType != "" {
			if !utils.CheckStringInList(r.Spec.ServiceDetails.TafPolicy, utils.GetServiceFailoverType()) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("FailoverType"), r.Spec.ServiceDetails.FailOverType,
						"FailoverType values can be only set to"+strings.Join(utils.GetServiceFailoverType(), " ")))
			}
		}

		if r.Spec.ServiceDetails.Role != "" {
			if !utils.CheckStringInList(r.Spec.ServiceDetails.Role, utils.GetServiceRole()) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("ServiceDetails").Child("FailoverType"), r.Spec.ServiceDetails.Role,
						"FailoverType values can be only set to"+strings.Join(utils.GetServiceRole(), " ")))
			}
		}
	}

	// ======> Service Specs Check Ends here here ====>

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil
}

// validateConfigParams enforces limits on core configuration parameters.
func (r *RacDatabase) validateConfigParams() field.ErrorList {

	var validationErrs field.ErrorList

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil
}

// validateGeneric performs overall spec validation including naming rules.
func (r *RacDatabase) validateGeneric() field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.ConfigParams == nil {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("ConfigParams"), r.Spec.ConfigParams,
				"ConfigParams cannot be set empty"))
	} else {
		if _, err := r.Spec.ResolveSwStorageMode(); err != nil {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec"), r.Spec, err.Error()))
		}

		cfg := r.Spec.ConfigParams

		gridRspSet := false
		var gridRsp, dbRsp *RacResponseFile

		if cfg != nil {
			gridRsp = cfg.GridResponseFile
			dbRsp = cfg.DbResponseFile
			gridRspSet = gridRsp != nil && gridRsp.ConfigMapName != ""
		}

		if gridRspSet {

			if gridRsp.Name == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("GridResponseFile").Child("Name"),
						gridRsp.Name,
						"GridResponsefile name cannot be set empty",
					))
			}

			if cfg.Inventory != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("Inventory"),
						cfg.Inventory,
						"Inventory name cannot be used when GridResponsefile is set",
					))
			}

			if cfg.GridBase != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("GridBase"),
						cfg.GridBase,
						"GridBase cannot be used when GridResponsefile is set",
					))
			}

		} else {

			if cfg.GridBase == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("GridBase"),
						cfg.GridBase,
						"GridBase cannot be set empty",
					))
			}

			if cfg.GridHome == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("GridHome"),
						cfg.GridHome,
						"GridHome cannot be set empty",
					))
			}

			if cfg.Inventory == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("Inventory"),
						cfg.Inventory,
						"Inventory cannot be set empty",
					))
			}
		}

		// -----------------------------
		// DB RESPONSE FILE VALIDATION
		// -----------------------------
		dbRspSet := dbRsp != nil && dbRsp.ConfigMapName != ""

		if dbRspSet {

			if dbRsp.Name == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbResponseFile").Child("Name"),
						dbRsp.Name,
						"DbResponsefile cannot be set empty",
					))
			}

			if cfg.DbCharSet != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbCharSet"),
						cfg.DbCharSet,
						"DbCharSet cannot be used when DbResponsefile is set",
					))
			}

			if cfg.DbConfigType != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbConfigType"),
						cfg.DbConfigType,
						"DbConfigType cannot be used when DbResponsefile is set",
					))
			}

			if cfg.DbType != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbType"),
						cfg.DbType,
						"DbType cannot be used when DbResponsefile is set",
					))
			}

			if cfg.DbUniqueName != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbUniqueName"),
						cfg.DbUniqueName,
						"DbUniqueName cannot be used when DbResponsefile is set",
					))
			}

			if cfg.DbStorageType != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbStorageType"),
						cfg.DbStorageType,
						"DbStorageType cannot be used when DbResponsefile is set",
					))
			}

			if cfg.DbName != "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbName"),
						cfg.DbName,
						"DbName cannot be used when DbResponsefile is set",
					))
			}

		} else {

			if cfg.DbBase == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbBase"),
						cfg.DbBase,
						"DbBase cannot be set empty",
					))
			}

			if cfg.DbHome == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbHome"),
						cfg.DbHome,
						"DbHome cannot be set empty",
					))
			}

			if cfg.DbName == "" {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams").Child("DbName"),
						cfg.DbName,
						"DbName cannot be set empty",
					))
			}
		}

		// -----------------------------
		// COMMON REQUIRED FIELDS
		// -----------------------------
		if cfg.GridSwZipFile == "" {
			validationErrs = append(validationErrs,
				field.Invalid(
					field.NewPath("spec").Child("ConfigParams").Child("GridSwZipFile"),
					cfg.GridSwZipFile,
					"GridSwZipFile cannot be set empty",
				))
		}

		if cfg.DbSwZipFile == "" {
			validationErrs = append(validationErrs,
				field.Invalid(
					field.NewPath("spec").Child("ConfigParams").Child("DbSwZipFile"),
					cfg.DbSwZipFile,
					"DbSwZipFile cannot be set empty",
				))
		}

		if !utils.CheckStatusFlag(r.Spec.UseNfsforSwStorage) {
			mode, err := cfg.ResolveSwStageMode()
			if err != nil {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams"),
						cfg,
						err.Error(),
					))
			} else if mode == RacSwStageNone {
				validationErrs = append(validationErrs,
					field.Invalid(
						field.NewPath("spec").Child("ConfigParams"),
						cfg,
						"either (swStagePvc + swStagePvcMountLocation) OR hostSwStageLocation must be specified",
					))
			}
		}
	}

	if r.Spec.Image == "" {
		validationErrs = append(validationErrs,
			field.Invalid(field.NewPath("spec").Child("Image"), r.Spec.Image,
				"Image cannot be set empty"))
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil
}

// validateUpdateGeneric restricts updates to immutable or controlled fields.
func (r *RacDatabase) validateUpdateGeneric(oldCr *RacDatabase) field.ErrorList {
	var validationErrs field.ErrorList

	// Compare Spec.ConfigParams between old and new
	if r.Spec.ConfigParams != nil && oldCr.Spec.ConfigParams != nil {
		// Validate DbName
		if r.Spec.ConfigParams.DbName != "" && oldCr.Spec.ConfigParams.DbName != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbName, r.Spec.ConfigParams.DbName) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbName"), "DbName cannot be changed post creation"))
		}

		// Validate GridBase
		if r.Spec.ConfigParams.GridBase != "" && oldCr.Spec.ConfigParams.GridBase != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.GridBase, r.Spec.ConfigParams.GridBase) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("GridBase"), "GridBase cannot be changed post creation"))
		}

		// Validate GridHome
		if r.Spec.ConfigParams.GridHome != "" && oldCr.Spec.ConfigParams.GridHome != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.GridHome, r.Spec.ConfigParams.GridHome) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("GridHome"), "GridHome cannot be changed post creation"))
		}

		// Validate DbBase
		if r.Spec.ConfigParams.DbBase != "" && oldCr.Spec.ConfigParams.DbBase != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbBase, r.Spec.ConfigParams.DbBase) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbBase"), "DbBase cannot be changed post creation"))
		}

		// Validate DbHome
		if r.Spec.ConfigParams.DbHome != "" && oldCr.Spec.ConfigParams.DbHome != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbHome, r.Spec.ConfigParams.DbHome) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbHome"), "DbHome cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.DbCharSet != "" && oldCr.Spec.ConfigParams.DbCharSet != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbCharSet, r.Spec.ConfigParams.DbCharSet) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbCharSet"), "DbCharSet cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.DbConfigType != "" && oldCr.Spec.ConfigParams.DbConfigType != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbConfigType, r.Spec.ConfigParams.DbConfigType) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbConfigType"), "DbConfigType cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.DbUniqueName != "" && oldCr.Spec.ConfigParams.DbUniqueName != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbUniqueName, r.Spec.ConfigParams.DbUniqueName) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbUniqueName"), "DbUniqueName cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.DbStorageType != "" && oldCr.Spec.ConfigParams.DbStorageType != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbStorageType, r.Spec.ConfigParams.DbStorageType) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbStorageType"), "DbStorageType cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.DbSwZipFile != "" && oldCr.Spec.ConfigParams.DbSwZipFile != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.DbSwZipFile, r.Spec.ConfigParams.DbSwZipFile) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("DbSwZipFile"), "DbSwZipFile cannot be changed post creation"))
		}

		if r.Spec.ConfigParams.GridSwZipFile != "" && oldCr.Spec.ConfigParams.GridSwZipFile != "" && !strings.EqualFold(oldCr.Spec.ConfigParams.GridSwZipFile, r.Spec.ConfigParams.GridSwZipFile) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("ConfigParams").Child("GridSwZipFile"), "GridSwZipFile cannot be changed post creation"))
		}

		// ------------------------------------------------------------
		// GridResponseFile immutability checks (panic-safe)
		// ------------------------------------------------------------
		if r.Spec.ConfigParams != nil &&
			oldCr.Spec.ConfigParams != nil &&
			r.Spec.ConfigParams.GridResponseFile != nil &&
			oldCr.Spec.ConfigParams.GridResponseFile != nil {

			// GridResponseFile.ConfigMapName
			if r.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" &&
				oldCr.Spec.ConfigParams.GridResponseFile.ConfigMapName != "" &&
				!strings.EqualFold(
					oldCr.Spec.ConfigParams.GridResponseFile.ConfigMapName,
					r.Spec.ConfigParams.GridResponseFile.ConfigMapName,
				) {

				validationErrs = append(validationErrs,
					field.Forbidden(
						field.NewPath("spec").
							Child("configParams").
							Child("gridResponseFile").
							Child("configMapName"),
						"GridResponseFile.ConfigMapName cannot be changed post creation",
					),
				)
			}

			// GridResponseFile.Name
			if r.Spec.ConfigParams.GridResponseFile.Name != "" &&
				oldCr.Spec.ConfigParams.GridResponseFile.Name != "" &&
				!strings.EqualFold(
					oldCr.Spec.ConfigParams.GridResponseFile.Name,
					r.Spec.ConfigParams.GridResponseFile.Name,
				) {

				validationErrs = append(validationErrs,
					field.Forbidden(
						field.NewPath("spec").
							Child("configParams").
							Child("gridResponseFile").
							Child("name"),
						"GridResponseFile.Name cannot be changed post creation",
					),
				)
			}
		}

		// ------------------------------------------------------------
		// DbResponseFile immutability checks (panic-safe)
		// ------------------------------------------------------------
		if r.Spec.ConfigParams != nil &&
			oldCr.Spec.ConfigParams != nil &&
			r.Spec.ConfigParams.DbResponseFile != nil &&
			oldCr.Spec.ConfigParams.DbResponseFile != nil {

			// DbResponseFile.ConfigMapName
			if r.Spec.ConfigParams.DbResponseFile.ConfigMapName != "" &&
				oldCr.Spec.ConfigParams.DbResponseFile.ConfigMapName != "" &&
				!strings.EqualFold(
					oldCr.Spec.ConfigParams.DbResponseFile.ConfigMapName,
					r.Spec.ConfigParams.DbResponseFile.ConfigMapName,
				) {

				validationErrs = append(validationErrs,
					field.Forbidden(
						field.NewPath("spec").
							Child("configParams").
							Child("dbResponseFile").
							Child("configMapName"),
						"DbResponseFile.ConfigMapName cannot be changed post creation",
					),
				)
			}

			// DbResponseFile.Name
			if r.Spec.ConfigParams.DbResponseFile.Name != "" &&
				oldCr.Spec.ConfigParams.DbResponseFile.Name != "" &&
				!strings.EqualFold(
					oldCr.Spec.ConfigParams.DbResponseFile.Name,
					r.Spec.ConfigParams.DbResponseFile.Name,
				) {

				validationErrs = append(validationErrs,
					field.Forbidden(
						field.NewPath("spec").
							Child("configParams").
							Child("dbResponseFile").
							Child("name"),
						"DbResponseFile.Name cannot be changed post creation",
					),
				)
			}
		}

	}

	return validationErrs
}

// validateUpdateServiceSpecs checks service-related changes during updates.
func (r *RacDatabase) validateUpdateServiceSpecs(oldCr *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.ServiceDetails.Name != "" && oldCr.Status.ServiceDetails.Name != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Name, r.Spec.ServiceDetails.Name) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Name"), "Service Name cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Cardinality != "" && oldCr.Status.ServiceDetails.Cardinality != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Cardinality, r.Spec.ServiceDetails.Cardinality) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Cardinality"), "Cardinality cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Notification != "" && oldCr.Status.ServiceDetails.Notification != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Notification, r.Spec.ServiceDetails.Notification) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Notification"), "Notification cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.ClbGoal != "" && oldCr.Status.ServiceDetails.ClbGoal != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.ClbGoal, r.Spec.ServiceDetails.ClbGoal) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("ClbGoal"), "ClbGoal cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.CommitOutCome != "" && oldCr.Status.ServiceDetails.CommitOutCome != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.CommitOutCome, r.Spec.ServiceDetails.CommitOutCome) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("CommitOutCome"), "CommitOutCome cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.CommitOutComeFastPath != "" && oldCr.Status.ServiceDetails.CommitOutComeFastPath != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.CommitOutComeFastPath, r.Spec.ServiceDetails.CommitOutComeFastPath) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("CommitOutComeFastPath"), "CommitOutComeFastPath cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Dtp != "" && oldCr.Status.ServiceDetails.Dtp != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Dtp, r.Spec.ServiceDetails.Dtp) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Dtp"), "Dtp cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.SessionState != "" && oldCr.Status.ServiceDetails.SessionState != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.SessionState, r.Spec.ServiceDetails.SessionState) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("SessionState"), "SessionState cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Edition != "" && oldCr.Status.ServiceDetails.Edition != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Edition, r.Spec.ServiceDetails.Edition) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Edition"), "Edition cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.FailBack != "" && oldCr.Status.ServiceDetails.FailBack != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.FailBack, r.Spec.ServiceDetails.FailBack) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("FailBack"), "FailBack cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.FailOverRestore != "" && oldCr.Status.ServiceDetails.FailOverRestore != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.FailOverRestore, r.Spec.ServiceDetails.FailOverRestore) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("FailBack"), "FailBack cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.FailOverType != "" && oldCr.Status.ServiceDetails.FailOverType != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.FailOverType, r.Spec.ServiceDetails.FailOverType) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("FailOverType"), "FailOverType cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.TafPolicy != "" && oldCr.Status.ServiceDetails.TafPolicy != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.TafPolicy, r.Spec.ServiceDetails.TafPolicy) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("TafPolicy"), "TafPolicy cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.RlbGoal != "" && oldCr.Status.ServiceDetails.RlbGoal != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.RlbGoal, r.Spec.ServiceDetails.RlbGoal) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("RlbGoal"), "RlbGoal cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Role != "" && oldCr.Status.ServiceDetails.Role != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Role, r.Spec.ServiceDetails.Role) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Role"), "Role cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.SessionState != "" && oldCr.Status.ServiceDetails.SessionState != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.SessionState, r.Spec.ServiceDetails.SessionState) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("SessionState"), "SessionState cannot be changed post creation"))
	}

	if r.Spec.ServiceDetails.Pdb != "" && oldCr.Status.ServiceDetails.Pdb != "" && !strings.EqualFold(oldCr.Status.ServiceDetails.Pdb, r.Spec.ServiceDetails.Pdb) {
		validationErrs = append(validationErrs,
			field.Forbidden(field.NewPath("spec").Child("ServiceDetail").Child("Pdb"), "Pdb cannot be changed post creation"))
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}

// validateUpdateAsmStorage ensures ASM changes comply with redundancy rules.
func (r *RacDatabase) validateUpdateAsmStorage(oldCr *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList
	// Map of old group names and types for lookup
	oldGroupTypes := make(map[string]AsmDiskDGTypes)
	for _, dg := range oldCr.Spec.AsmStorageDetails {
		oldGroupTypes[dg.Name] = dg.Type
	}

	// Uniqueness check for types and disks in the new spec (r.Spec)
	diskToGroup := make(map[string]string)
	seenTypes := make(map[AsmDiskDGTypes]string)

	for idx, dg := range r.Spec.AsmStorageDetails {
		// New group detection (not in old spec)
		_, existed := oldGroupTypes[dg.Name]
		if !existed && dg.Type != OthersDiskDg {
			validationErrs = append(validationErrs,
				field.Forbidden(
					field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx),
					fmt.Sprintf("Addition of new disk group %q (type: %s) is not allowed except for groups of type OTHERS.", dg.Name, dg.Type)))
		}
		// Types must be unique per group
		if existingName, exists := seenTypes[dg.Type]; exists {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx).Child("type"),
					dg.Type, fmt.Sprintf("Type %q is already used by disk group %q. Types must be unique.", dg.Type, existingName)))
		} else {
			seenTypes[dg.Type] = dg.Name
		}

		// Disks must be unique
		for didx, disk := range dg.Disks {
			if existingGroup, exists := diskToGroup[disk]; exists {
				validationErrs = append(validationErrs,
					field.Duplicate(field.NewPath("spec").Child("asmDiskGroupDetails").Index(idx).Child("disks").Index(didx), disk))
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("asmDiskGroupDetails"), disk,
						fmt.Sprintf("Disk %s is already assigned to group %q. Disks must be unique among all groups.", disk, existingGroup)))
			} else {
				diskToGroup[disk] = dg.Name
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}

// validateUpdateDbSecret validates modifications to the database secret.
func (r *RacDatabase) validateUpdateDbSecret(oldCr *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.DbSecret != nil && oldCr.Status.DbSecret != nil {
		if r.Spec.DbSecret.Name != "" && oldCr.Status.DbSecret.Name != "" &&
			!strings.EqualFold(oldCr.Status.DbSecret.Name, r.Spec.DbSecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("DbSecret").Child("Name"),
					"DbSecret name cannot be changed post creation"))
		}

		if r.Spec.DbSecret.KeyFileName != "" && oldCr.Status.DbSecret.KeyFileName != "" &&
			!strings.EqualFold(oldCr.Status.DbSecret.KeyFileName, r.Spec.DbSecret.KeyFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileName"),
					r.Spec.DbSecret.KeyFileName, "KeyFileName cannot be changed post creation"))
		}

		if r.Spec.DbSecret.SecretKey != "" && oldCr.Status.DbSecret.SecretKey != "" &&
			!strings.EqualFold(oldCr.Status.DbSecret.SecretKey, r.Spec.DbSecret.SecretKey) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("key"),
					r.Spec.DbSecret.SecretKey, "key cannot be changed post creation"))
		}

		if r.Spec.DbSecret.PwdFileName != "" && oldCr.Status.DbSecret.PwdFileName != "" &&
			!strings.EqualFold(oldCr.Status.DbSecret.PwdFileName, r.Spec.DbSecret.PwdFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileName"),
					r.Spec.DbSecret.PwdFileName, "PwdFileName cannot be changed post creation"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}
func validateMinMemoryLimit(
	resources *corev1.ResourceRequirements,
	minBytes int64,
	fldPath *field.Path,
) field.ErrorList {
	var errs field.ErrorList

	if resources == nil {
		return errs
	}

	memLimit, ok := resources.Limits[corev1.ResourceMemory]
	if !ok {
		return errs
	}

	if memLimit.Value() < minBytes {
		errs = append(errs, field.Invalid(
			fldPath.Child("resources").Child("limits").Child("memory"),
			memLimit.String(),
			fmt.Sprintf("memory limit must be greater than %d bytes", minBytes),
		))
	}

	return errs
}

// validateUpdateTdeSecret validates updates to the TDE wallet secret.
func (r *RacDatabase) validateUpdateTdeSecret(oldCr *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.TdeWalletSecret != nil && oldCr.Status.TdeWalletSecret != nil {
		if r.Spec.TdeWalletSecret.Name != "" && oldCr.Status.TdeWalletSecret.Name != "" &&
			!strings.EqualFold(oldCr.Status.TdeWalletSecret.Name, r.Spec.TdeWalletSecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("TdeWalletSecret").Child("Name"),
					"TdeWalletSecret name cannot be changed post creation"))
		}

		if r.Spec.TdeWalletSecret.KeyFileName != "" && oldCr.Status.TdeWalletSecret.KeyFileName != "" &&
			!strings.EqualFold(oldCr.Status.TdeWalletSecret.KeyFileName, r.Spec.TdeWalletSecret.KeyFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("KeyFileName"),
					r.Spec.TdeWalletSecret.KeyFileName, "KeyFileName cannot be changed post creation"))
		}

		if r.Spec.TdeWalletSecret.SecretKey != "" && oldCr.Status.TdeWalletSecret.SecretKey != "" &&
			!strings.EqualFold(oldCr.Status.TdeWalletSecret.SecretKey, r.Spec.TdeWalletSecret.SecretKey) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("key"),
					r.Spec.TdeWalletSecret.SecretKey, "key cannot be changed post creation"))
		}

		if r.Spec.TdeWalletSecret.PwdFileName != "" && oldCr.Status.TdeWalletSecret.PwdFileName != "" &&
			!strings.EqualFold(oldCr.Status.TdeWalletSecret.PwdFileName, r.Spec.TdeWalletSecret.PwdFileName) {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("TdeWalletSecret").Child("PwdFileName"),
					r.Spec.TdeWalletSecret.PwdFileName, "PwdFileName cannot be changed post creation"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}

// validateUpdateSshSecret enforces SSH secret immutability rules.
func (r *RacDatabase) validateUpdateSshSecret(oldCr *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList

	if r.Spec.SshKeySecret != nil && oldCr.Status.SshKeySecret != nil {
		if r.Spec.SshKeySecret.Name != "" && oldCr.Status.SshKeySecret.Name != "" && !strings.EqualFold(oldCr.Status.SshKeySecret.Name, r.Spec.SshKeySecret.Name) {
			validationErrs = append(validationErrs,
				field.Forbidden(field.NewPath("spec").Child("TdeWalletSecret").Child("Name"), "SshKeySecret name cannot be changed post creation"))
		}

		if r.Spec.SshKeySecret.PrivKeySecretName != "" && oldCr.Status.SshKeySecret.PrivKeySecretName != "" && !strings.EqualFold(oldCr.Status.SshKeySecret.PrivKeySecretName, r.Spec.SshKeySecret.PrivKeySecretName) {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("SshKeySecret").Child("PrivKeySecretName"), r.Spec.SshKeySecret.PrivKeySecretName,
				"PrivKeySecretName cannot be changed post creation"))
		}

		if r.Spec.SshKeySecret.PubKeySecretName != "" && oldCr.Status.SshKeySecret.PubKeySecretName != "" && !strings.EqualFold(oldCr.Status.SshKeySecret.PubKeySecretName, r.Spec.SshKeySecret.PubKeySecretName) {
			validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("SshKeySecret").Child("PubKeySecretName"), r.Spec.SshKeySecret.PubKeySecretName,
				"PubKeySecretName cannot be changed post creation"))
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}

// validateUpdatePrivateIPSpecs verifies private IP changes are safe.
func (r *RacDatabase) validateUpdatePrivateIPSpecs(old *RacDatabase) field.ErrorList {

	var validationErrs field.ErrorList

	if len(validationErrs) > 0 {
		return validationErrs
	}

	return nil

}

// validateRACAsmRedundancyAndDisks checks ASM redundancy versus disk counts.
func (r *RacDatabase) validateRACAsmRedundancyAndDisks(
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
