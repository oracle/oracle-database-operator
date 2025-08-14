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
var autonomousdatabaselog = logf.Log.WithName("autonomousdatabase-resource")

func (r *AutonomousDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v4-autonomousdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=autonomousdatabases,versions=v4,name=vautonomousdatabasev4.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &AutonomousDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
// ValidateCreate checks if the spec is valid for a provisioning or a binding operation
func (r *AutonomousDatabase) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	adb := obj.(*AutonomousDatabase)

	autonomousdatabaselog.Info("validate create", "name", adb.Name)

	namespaces := dbcommons.GetWatchNamespaces()
	_, hasEmptyString := namespaces[""]
	isClusterScoped := len(namespaces) == 1 && hasEmptyString
	if !isClusterScoped {
		_, containsNamespace := namespaces[adb.Namespace]
		// Check if the allowed namespaces maps contains the required namespace
		if len(namespaces) != 0 && !containsNamespace {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("metadata").Child("namespace"), adb.Namespace,
					"Oracle database operator doesn't watch over this namespace"))
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabase"},
		adb.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabase) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var (
		allErrs field.ErrorList
		newAdb  = newObj.(*AutonomousDatabase)
	)

	autonomousdatabaselog.Info("validate update", "name", newAdb.Name)

	allErrs = validateCommon(r, allErrs)

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabase"},
		newAdb.Name, allErrs)
}

func validateCommon(adb *AutonomousDatabase, allErrs field.ErrorList) field.ErrorList {
	// password
	if adb.Spec.Details.AdminPassword.K8sSecret.Name != nil && adb.Spec.Details.AdminPassword.OciSecret.Id != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("details").Child("adminPassword"),
				"cannot apply k8sSecret.name and ociSecret.ocid at the same time"))
	}

	if adb.Spec.Wallet.Password.K8sSecret.Name != nil && adb.Spec.Wallet.Password.OciSecret.Id != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("wallet").Child("password"),
				"cannot apply k8sSecret.name and ociSecret.ocid at the same time"))
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabase) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
