/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var dataguardbrokerlog = logf.Log.WithName("dataguardbroker-resource")

func (r *DataguardBroker) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-dataguardbroker,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,verbs=create;update,versions=v1alpha1,name=mdataguardbroker.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &DataguardBroker{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DataguardBroker) Default() {
	dataguardbrokerlog.Info("default", "name", r.Name)

	if r.Spec.AdminPassword.KeepSecret == nil {
		keepSecret := true
		r.Spec.AdminPassword.KeepSecret = &keepSecret
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v1alpha1-dataguardbroker,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,versions=v1alpha1,name=vdataguardbroker.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &DataguardBroker{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateCreate() error {
	dataguardbrokerlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateUpdate(old runtime.Object) error {
	dataguardbrokerlog.Info("validate update", "name", r.Name)

	dataguardbrokerlog.Info("validate update", "name", r.Name)
	var allErrs field.ErrorList

	// check creation validations first
	err := r.ValidateCreate()
	if err != nil {
		return err
	}

	// Validate Deletion
	if r.GetDeletionTimestamp() != nil {
		err := r.ValidateDelete()
		if err != nil {
			return err
		}
	}

	// Now check for updation errors
	oldObj, ok := old.(*DataguardBroker)
	if !ok {
		return nil
	}

	if oldObj.Status.ProtectionMode != "" && !strings.EqualFold(r.Spec.ProtectionMode, oldObj.Status.ProtectionMode) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("protectionMode"), "cannot be changed"))
	}
	if oldObj.Status.PrimaryDatabaseRef != "" && !strings.EqualFold(oldObj.Status.PrimaryDatabaseRef, r.Spec.PrimaryDatabaseRef) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("primaryDatabaseRef"), "cannot be changed"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "DataguardBroker"},
		r.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateDelete() error {
	dataguardbrokerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
