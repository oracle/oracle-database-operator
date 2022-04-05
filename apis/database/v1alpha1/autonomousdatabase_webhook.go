/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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

package v1alpha1

import (
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var autonomousdatabaselog = logf.Log.WithName("autonomousdatabase-resource")

func (r *AutonomousDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:verbs=create;update,path=/mutate-database-oracle-com-v1alpha1-autonomousdatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=autonomousdatabases,versions=v1alpha1,name=mautonomousdatabase.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &AutonomousDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *AutonomousDatabase) Default() {
	autonomousdatabaselog.Info("default", "name", r.Name)

	if !isDedicated(r) { // Shared database
		// AccessType is PUBLIC by default
		if r.Spec.Details.NetworkAccess.AccessType == "" {
			r.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypePublic
		}
	} else { // Dedicated database
		// AccessType can only be PRIVATE for a dedicated database
		r.Spec.Details.NetworkAccess.AccessType = NetworkAccessTypePrivate
	}

}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v1alpha1-autonomousdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=autonomousdatabases,versions=v1alpha1,name=vautonomousdatabase.kb.io,admissionReviewVersions={v1}

var _ webhook.Validator = &AutonomousDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
// ValidateCreate checks if the spec is valid for a provisioning or a binding operation
func (r *AutonomousDatabase) ValidateCreate() error {

	var allErrs field.ErrorList

	autonomousdatabaselog.Info("validate create", "name", r.Name)

	if r.Spec.Details.AutonomousDatabaseOCID == nil { // provisioning operation
		allErrs = validateCommon(r, allErrs)
		allErrs = validateNetworkAccess(r, allErrs)

		if r.Spec.Details.LifecycleState != "" {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("details").Child("LifecycleState"),
					"cannot apply lifecycleState to a provision operation"))
		}
	} else { // binding operation
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabase"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabase) ValidateUpdate(old runtime.Object) error {
	// Validate creation instead of update if there is no last successful spec, i.e. the database isn't provisioned or bound yet
	lastSucSpec, err := r.GetLastSuccessfulSpec()
	if err != nil {
		return err
	}
	if lastSucSpec == nil {
		return r.ValidateCreate()
	}

	var allErrs field.ErrorList
	var oldADB *AutonomousDatabase = old.(*AutonomousDatabase)

	autonomousdatabaselog.Info("validate update", "name", r.Name)

	// cannot modify autonomousDatabaseOCID
	if r.Spec.Details.AutonomousDatabaseOCID != nil &&
		oldADB.Spec.Details.AutonomousDatabaseOCID != nil &&
		*r.Spec.Details.AutonomousDatabaseOCID != *oldADB.Spec.Details.AutonomousDatabaseOCID {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("details").Child("autonomousDatabaseOCID"),
				"autonomousDatabaseOCID cannot be modified"))
	}

	// cannot apply lifecycleState with other fields together
	copyDetails := r.Spec.Details.DeepCopy()
	copyDetails.LifecycleState = oldADB.Spec.Details.LifecycleState
	onlyLifecycleStateChanged := reflect.DeepEqual(oldADB.Spec.Details, copyDetails)
	if onlyLifecycleStateChanged {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("details").Child("LifecycleState"),
				"cannot apply lifecycleState with other spec.details attributes at the same time"))
	}

	allErrs = validateCommon(r, allErrs)
	allErrs = validateNetworkAccess(r, allErrs)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabase"},
		r.Name, allErrs)
}

func validateCommon(adb *AutonomousDatabase, allErrs field.ErrorList) field.ErrorList {
	// password
	if adb.Spec.Details.AdminPassword.K8sSecret.Name != nil && adb.Spec.Details.AdminPassword.OCISecret.OCID != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("details").Child("adminPassword"),
				"cannot apply k8sSecret.name and ociSecret.ocid at the same time"))
	}

	if adb.Spec.Details.Wallet.Password.K8sSecret.Name != nil && adb.Spec.Details.Wallet.Password.OCISecret.OCID != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("details").Child("wallet").Child("password"),
				"cannot apply k8sSecret.name and ociSecret.ocid at the same time"))
	}

	return allErrs
}

func validateNetworkAccess(adb *AutonomousDatabase, allErrs field.ErrorList) field.ErrorList {
	if !isDedicated(adb) {
		// Shared database
		if adb.Spec.Details.NetworkAccess.AccessType == NetworkAccessTypeRestricted {
			if adb.Spec.Details.NetworkAccess.AccessControlList == nil {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("accessControlList"),
						fmt.Sprintf("accessControlList cannot be empty when the network access type is %s", NetworkAccessTypeRestricted)))
			}
		} else if adb.Spec.Details.NetworkAccess.AccessType == NetworkAccessTypePrivate { // the accessType is PRIVATE
			if adb.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID == nil {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("privateEndpoint").Child("subnetOCID"),
						fmt.Sprintf("subnetOCID cannot be empty when the network access type is %s", NetworkAccessTypePrivate)))
			}

			if adb.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs == nil {
				allErrs = append(allErrs,
					field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("privateEndpoint").Child("nsgOCIDs"),
						fmt.Sprintf("nsgOCIDs cannot be empty when the network access type is %s", NetworkAccessTypePrivate)))
			}
		}

		// IsAccessControlEnabled is not applicable to a shared database
		if adb.Spec.Details.NetworkAccess.IsAccessControlEnabled != nil {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("IsAccessControlEnabled"),
					fmt.Sprintf("isAccessControlEnabled is not applicable on a shared Autonomous Database")))
		}
	} else {
		// Dedicated database

		// accessControlList cannot be provided when Autonomous Database's access control is disabled
		if !*adb.Spec.Details.NetworkAccess.IsAccessControlEnabled && adb.Spec.Details.NetworkAccess.AccessControlList != nil {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("accessControlList"),
					"access control list cannot be provided when Autonomous Database's access control is disabled"))
		}

		// IsMTLSConnectionRequired is not supported by dedicated database
		if adb.Spec.Details.NetworkAccess.IsMTLSConnectionRequired != nil {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("details").Child("networkAccess").Child("isMTLSConnectionRequired"), "isMTLSConnectionRequired is not supported on a dedicated database"))
		}
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabase) ValidateDelete() error {
	autonomousdatabaselog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// Returns true if AutonomousContainerDatabaseOCID has value.
// We don't use Details.IsDedicated because the parameter might be null when it's a provision operation.
func isDedicated(adb *AutonomousDatabase) bool {
	return adb.Spec.Details.AutonomousContainerDatabase.K8sACD.Name != nil ||
		adb.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID != nil
}
