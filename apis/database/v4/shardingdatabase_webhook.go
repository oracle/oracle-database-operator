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

  cr , ok :=  obj.(*ShardingDatabase)

 if  !ok {
    return fmt.Errorf("xpected  obj.*ShardingDatabase but got %T", obj)
 }

	shardingdatabaselog.Info("default", "name", cr.Name)

	var replicas int32


	// TODO(user): fill in your defaulting logic.
	if cr.Spec.GsmDevMode != "" {
		cr.Spec.GsmDevMode = "dev"
	}

	if cr.Spec.IsTdeWallet == "" {
		cr.Spec.IsTdeWallet = "disable"
	}
	for pindex := range cr.Spec.Shard {
		if strings.ToLower(cr.Spec.Shard[pindex].IsDelete) == "" {
			cr.Spec.Shard[pindex].IsDelete = "disable"
		}
	}

	for pindex := range cr.Spec.ShardInfo {
		if strings.ToLower(cr.Spec.ShardInfo[pindex].ShardGroupDetails.IsDelete) == "" {
			cr.Spec.ShardInfo[pindex].ShardGroupDetails.IsDelete = "disable"
		}
	}

	totalShard = 0
	for pindex := range cr.Spec.ShardInfo {
		replicas = 2
		if cr.Spec.ShardInfo[pindex].Replicas != 0 {
			replicas = cr.Spec.ShardInfo[pindex].Replicas
		}
		totalShard = totalShard + replicas
	}

	if totalShard > 0 {
		cr.Spec.Shard = make([]ShardSpec, totalShard)
		cr.initShardsSpec()
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
  cr , ok :=  obj.(*ShardingDatabase)

 if  !ok {
//    return fmt.Errorf("xpected  obj.*ShardingDatabase but got %T", obj)
    validationErr = append(validationErr,field.Invalid(field.NewPath("obj"),"obj","Expected  obj.*ShardingDatabase."))
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
func (r *ShardingDatabase) ValidateUpdate(ctx context.Context, old, newObj runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
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

	totalShard = 0
	for pindex := range r.Spec.ShardInfo {
		replicas = 2
		if r.Spec.ShardInfo[pindex].Replicas != 0 {
			replicas = r.Spec.ShardInfo[pindex].Replicas
		} else {
			r.Spec.ShardInfo[pindex].Replicas = replicas
		}

		totalShard = totalShard + replicas
		if r.Spec.ShardInfo[pindex].ShardGroupDetails != nil {
			if r.Spec.ShardInfo[pindex].ShardGroupDetails.DeployAs == "" {
				r.Spec.ShardInfo[pindex].ShardGroupDetails.DeployAs = "primary"
			}
			if (r.Spec.ShardInfo[pindex].ShardGroupDetails.DeployAs == "primary") && (replicas > 1) {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("replicas"), r.Spec.ShardInfo[pindex].Replicas,
						"Primary++ Shard Group can have only one replicas"))
			}
		} else {
			if replicas > 1 {
				validationErrs = append(validationErrs,
					field.Invalid(field.NewPath("spec").Child("shardInfo").Child("replicas"), r.Spec.ShardInfo[pindex].Replicas,
						"Primary!! Shard Group can have only one replicas"))
			}
		}
	}

	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func (r *ShardingDatabase) initShardsSpec() error {
	var shardIndex int

	shardIndex = 0
	for pindex := range r.Spec.ShardInfo {
		for i := 0; i < int(r.Spec.ShardInfo[pindex].Replicas); i++ {
			r.Spec.Shard[shardIndex].Name = r.Spec.ShardInfo[pindex].ShardPreFixName + strconv.Itoa(shardIndex+1)
			r.Spec.Shard[shardIndex].StorageSizeInGb = r.Spec.ShardInfo[pindex].StorageSizeInGb
			r.Spec.Shard[shardIndex].ShardGroup = r.Spec.ShardInfo[pindex].ShardGroupDetails.ShardGroupName
			r.Spec.Shard[shardIndex].ShardRegion = r.Spec.ShardInfo[pindex].ShardGroupDetails.Region
			r.Spec.Shard[shardIndex].DeployAs = r.Spec.ShardInfo[pindex].ShardGroupDetails.DeployAs
			r.Spec.Shard[shardIndex].IsDelete = r.Spec.ShardInfo[pindex].ShardGroupDetails.IsDelete
			r.Spec.Shard[shardIndex].ImagePulllPolicy = new(corev1.PullPolicy)
			*(r.Spec.Shard[shardIndex].ImagePulllPolicy) = corev1.PullPolicy("Always")
			fmt.Println("ShardName=[" + r.Spec.Shard[shardIndex].Name + "]")
			if r.Spec.ShardInfo[pindex].ShardSpaceDetails != nil {
				r.Spec.Shard[shardIndex].ShardSpace = r.Spec.ShardInfo[pindex].ShardSpaceDetails.ShardSpaceName
			}

			shardIndex++
		}

	}

	return nil
}
