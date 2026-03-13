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
	"sort"
	"strconv"
	"strings"
	"unicode"

	shapes "github.com/oracle/oracle-database-operator/commons/shapes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
)

var totalShard int32 = 0

// log is for logging in this package.
var shardingdatabaselog = logf.Log.WithName("shardingdatabase-resource")

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

	shardingdatabaselog.Info("default", "name", cr.Name)

	if strings.TrimSpace(cr.Spec.GsmDevMode) == "" {
		cr.Spec.GsmDevMode = "dev"
	}
	if strings.TrimSpace(cr.Spec.IsTdeWallet) == "" {
		cr.Spec.IsTdeWallet = "disable"
	}

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

	totalShard = 0
	for i := range cr.Spec.ShardInfo {
		if cr.Spec.ShardInfo[i].Replicas == 0 {
			cr.Spec.ShardInfo[i].Replicas = 2
		}
		totalShard += cr.Spec.ShardInfo[i].Replicas
	}

	if totalShard > 0 {
		desired := cr.buildDesiredShardSpec()
		cr.Spec.Shard = mergeDesiredAndExistingShards(cr.Spec.Shard, desired)
	}

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

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-shardingdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=shardingdatabases,versions=v4,name=vshardingdatabasev4.kb.io,admissionReviewVersions={v1}

var _ webhook.CustomValidator = &ShardingDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	// Check Secret configuration
	var validationErr field.ErrorList
	var validationErrs1 field.ErrorList
	cr, ok := obj.(*ShardingDatabase)

	if !ok {
		//    return fmt.Errorf("xpected  obj.*ShardingDatabase but got %T", obj)
		validationErr = append(validationErr, field.Invalid(field.NewPath("obj"), "obj", "Expected  obj.*ShardingDatabase."))
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
			cr.Name, validationErr)
	}

	//namespaces := db.GetWatchNamespaces()
	//_, containsNamespace := namespaces[r.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	//	if len(namespaces) != 0 && !containsNamespace {
	//	validationErr = append(validationErr,
	//		field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
	//			"Oracle database operator doesn't watch over this namespace"))
	//}

	if cr.Spec.DbSecret == nil {
		validationErr = append(validationErr,
			field.Invalid(field.NewPath("spec").Child("DbSecret"), cr.Spec.DbSecret,
				"DbSecret cannot be set to nil"))
	} else {
		if len(cr.Spec.DbSecret.Name) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("Name"), cr.Spec.DbSecret.Name,
					"Secret name cannot be set empty"))
		}
		if len(cr.Spec.DbSecret.PwdFileName) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileName"), cr.Spec.DbSecret.PwdFileName,
					"Password file name cannot be set empty"))
		}
		if strings.ToLower(cr.Spec.DbSecret.EncryptionType) != "base64" {
			if strings.ToLower(cr.Spec.DbSecret.KeyFileName) == "" {
				validationErr = append(validationErr,
					field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileName"), cr.Spec.DbSecret.KeyFileName,
						"Key file name cannot be empty"))
			}
		}

		/**
		if len(r.Spec.DbSecret.PwdFileMountLocation) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileMountLocation"), r.Spec.DbSecret.PwdFileMountLocation,
					"Password file mount location cannot be empty"))
		}

		if len(r.Spec.DbSecret.KeyFileMountLocation) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileMountLocation"), r.Spec.DbSecret.KeyFileMountLocation,
					"KeyFileMountLocation file mount location cannot be empty"))
		}
		**/
	}

	if cr.Spec.IsTdeWallet == "enable" {
		if (len(cr.Spec.FssStorageClass) == 0) && (len(cr.Spec.TdeWalletPvc) == 0) {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("FssStorageClass"), cr.Spec.FssStorageClass,
					"FssStorageClass or TdeWalletPvc cannot be set empty if isTdeWallet set to true"))

			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("TdeWalletPvc"), cr.Spec.TdeWalletPvc,
					"FssStorageClass or TdeWalletPvc cannot be set empty if isTdeWallet set to true"))
		}
	}

	if cr.Spec.IsTdeWallet != "" {
		if (strings.ToLower(strings.TrimSpace(cr.Spec.IsTdeWallet)) != "enable") && (strings.ToLower(strings.TrimSpace(cr.Spec.IsTdeWallet)) != "disable") {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("isTdeWallet"), cr.Spec.IsTdeWallet,
					"isTdeWallet can be set to only \"enable\" or \"disable\""))
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

	fmt.Println("TotalShard=[" + strconv.Itoa(int(totalShard)) + "]")
	fmt.Println("Original shard buffer len=[" + strconv.Itoa(len(cr.Spec.Shard)) + "]")
	fmt.Println("Original shard buffer capacity=[" + strconv.Itoa(cap(cr.Spec.Shard)) + "]")

	// TODO(user): fill in your validation logic upon object creation.
	if len(validationErr) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		cr.Name, validationErr)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.Info("validate update", "name", r.Name)

	var validationErr field.ErrorList

	oldCR, ok1 := oldObj.(*ShardingDatabase)
	newCR, ok2 := newObj.(*ShardingDatabase)
	if !ok1 || !ok2 {
		validationErr = append(validationErr,
			field.Invalid(field.NewPath("objectType"),
				fmt.Sprintf("%T -> %T", oldObj, newObj),
				"expected *ShardingDatabase for both old and new objects"))
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
			r.Name, validationErr)
	}

	oldMode := detectShardingMode(&oldCR.Spec)
	newMode := detectShardingMode(&newCR.Spec)

	if oldMode == modeSystem && (newMode == modeUser || newMode == modeComposite) {
		validationErr = append(validationErr,
			field.Forbidden(field.NewPath("spec").Child("shardInfo"),
				"Cannot switch from System Sharding to User-Defined/Composite after creation"))
	}

	if len(validationErr) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		r.Name, validationErr)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// ###### Vlaidation Block #################

func (r *ShardingDatabase) validateShardIsDelete() field.ErrorList {

	var validationErrs field.ErrorList

	for pindex := range r.Spec.Shard {
		if (strings.ToLower(strings.TrimSpace(r.Spec.Shard[pindex].IsDelete)) != "enable") && (strings.ToLower(strings.TrimSpace(r.Spec.Shard[pindex].IsDelete)) != "disable") && (strings.ToLower(strings.TrimSpace(r.Spec.Shard[pindex].IsDelete)) != "failed") {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shard").Child("isDelete"), r.Spec.Shard[pindex].IsDelete,
					"r.Spec.Shard[pindex].IsDelete can be set to only enable|disable|failed"))
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
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("shard").Child("EnvVars"), r.Spec.Shard[i].EnvVars[index].Name,
							"r.Spec.Shard[i].EnvVars[index].Name ORACLE_SID value can only be set to free"))
					}
				}
				if variable.Name == "ORACLE_PDB" {
					if strings.ToLower(variable.Value) != "freepdb" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("shard").Child("EnvVars"), r.Spec.Shard[i].EnvVars[index].Name,
							"r.Spec.Shard[i].EnvVars[index].Name ORACLE_PDB value can only be set to freepdb"))
					}
				}
			}
		}
		// Catalog Spec Checks
		for i := 0; i < len(r.Spec.Catalog); i++ {
			for index, variable := range r.Spec.Catalog[i].EnvVars {
				if variable.Name == "ORACLE_SID" {
					if strings.ToLower(variable.Value) != "free" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("catalog").Child("EnvVars"), r.Spec.Catalog[i].EnvVars[index].Name,
							"r.Spec.Catalog[i].EnvVars[index].Name ORACLE_SID value can only be set to free"))
					}
				}
				if variable.Name == "ORACLE_PDB" {
					if strings.ToLower(variable.Value) != "freepdb" {
						validationErrs = append(validationErrs, field.Invalid(field.NewPath("spec").Child("catalog").Child("EnvVars"), r.Spec.Catalog[i].EnvVars[index].Name,
							"r.Spec.Catalog[i].EnvVars[index].Name ORACLE_PDB value can only be set to freepdb"))
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
				field.Invalid(field.NewPath("spec").Child("shard").Child("Name"), r.Spec.Shard[pindex].Name,
					"Shard Name cannot be greater than 9 characters."))
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
				field.Invalid(field.NewPath("spec").Child("catalog").Child("Name"), r.Spec.Catalog[pindex].Name,
					"Catalog Name cannot be greater than 9 characters."))
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

	typeCounts := struct {
		groupOnly int
		spaceOnly int
		both      int
	}{}

	sysPrimaryGroups := 0
	spacePrimaryGroupCount := map[string]int{}

	totalShard = 0

	for pindex := range r.Spec.ShardInfo {
		replicas = r.Spec.ShardInfo[pindex].Replicas
		if replicas == 0 {
			replicas = 1
			r.Spec.ShardInfo[pindex].Replicas = replicas
		}
		totalShard += replicas

		sg := r.Spec.ShardInfo[pindex].ShardGroupDetails
		ss := r.Spec.ShardInfo[pindex].ShardSpaceDetails

		hasGroup := sg != nil && strings.TrimSpace(sg.Name) != ""
		hasSpace := ss != nil && strings.TrimSpace(ss.Name) != ""

		switch {
		case hasGroup && !hasSpace:
			typeCounts.groupOnly++
			if strings.TrimSpace(sg.DeployAs) == "" {
				sg.DeployAs = "PRIMARY"
			}
			if strings.EqualFold(strings.TrimSpace(sg.DeployAs), "PRIMARY") {
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
			if strings.TrimSpace(sg.DeployAs) == "" {
				sg.DeployAs = "PRIMARY"
			}
			if strings.EqualFold(strings.TrimSpace(sg.DeployAs), "PRIMARY") {
				spacePrimaryGroupCount[strings.TrimSpace(ss.Name)]++
			}

		default:
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shardInfo").Index(pindex),
					r.Spec.ShardInfo[pindex],
					"Each shardInfo entry must define shardGroupDetails (system), shardSpaceDetails (user), or both (composite)"))
		}
	}

	if typeCounts.groupOnly > 0 && typeCounts.spaceOnly == 0 && typeCounts.both == 0 {
		if sysPrimaryGroups != 1 {
			validationErrs = append(validationErrs,
				field.Invalid(field.NewPath("spec").Child("shardInfo").Child("shardGroupDetails").Child("deployAs"),
					"PRIMARY",
					"System sharding: exactly one shardGroup must be PRIMARY"))
		}
	}

	if typeCounts.both > 0 || (typeCounts.groupOnly > 0 && typeCounts.spaceOnly > 0) {
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

func (r *ShardingDatabase) initShardsSpec() error {
	shardIndex := 0

	for pindex := range r.Spec.ShardInfo {
		replicas := r.Spec.ShardInfo[pindex].Replicas
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
