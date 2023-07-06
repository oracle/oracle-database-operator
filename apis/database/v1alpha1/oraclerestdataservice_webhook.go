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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var oraclerestdataservicelog = logf.Log.WithName("oraclerestdataservice-resource")

func (r *OracleRestDataService) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-oraclerestdataservice,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=oraclerestdataservices,verbs=create;update,versions=v1alpha1,name=moraclerestdataservice.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &OracleRestDataService{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *OracleRestDataService) Default() {
	oraclerestdataservicelog.Info("default", "name", r.Name)
	// OracleRestDataService Currently supports single replica
	r.Spec.Replicas = 1
	keepSecret := true
	if r.Spec.OrdsPassword.KeepSecret == nil {
		r.Spec.OrdsPassword.KeepSecret = &keepSecret
	}
	if r.Spec.ApexPassword.KeepSecret == nil {
		r.Spec.ApexPassword.KeepSecret = &keepSecret
	}
	if r.Spec.AdminPassword.KeepSecret == nil {
		r.Spec.AdminPassword.KeepSecret = &keepSecret
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v1alpha1-oraclerestdataservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=oraclerestdataservices,versions=v1alpha1,name=voraclerestdataservice.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &OracleRestDataService{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OracleRestDataService) ValidateCreate() error {
	oraclerestdataservicelog.Info("validate create", "name", r.Name)

	var allErrs field.ErrorList

	// Persistence spec validation
	if r.Spec.Persistence.Size == "" && (r.Spec.Persistence.AccessMode != "" ||
		r.Spec.Persistence.StorageClass != "" || r.Spec.Persistence.VolumeName != "") {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), r.Spec.Persistence,
				"invalid persistence specification, specify required size"))
	}

	if r.Spec.Persistence.Size != "" {
		if r.Spec.Persistence.AccessMode == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("size"), r.Spec.Persistence,
					"invalid persistence specification, specify accessMode"))
		}
		if r.Spec.Persistence.AccessMode != "ReadWriteMany" && r.Spec.Persistence.AccessMode != "ReadWriteOnce" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence").Child("accessMode"),
					r.Spec.Persistence.AccessMode, "should be either \"ReadWriteOnce\" or \"ReadWriteMany\""))
		}
	}

	// Validating databaseRef and ORDS kind name not to be same
	if r.Spec.DatabaseRef == r.Name {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("Name"),
					 "cannot be same as DatabaseRef: " + r.Spec.DatabaseRef))

	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "OracleRestDataService"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OracleRestDataService) ValidateUpdate(oldRuntimeObject runtime.Object) error {
	oraclerestdataservicelog.Info("validate update", "name", r.Name)

	var allErrs field.ErrorList

	// check creation validations first
	err := r.ValidateCreate()
	if err != nil {
		return err
	}

	// Now check for updation errors
	old, ok := oldRuntimeObject.(*OracleRestDataService)
	if !ok {
		return nil
	}

	if old.Status.DatabaseRef != "" && old.Status.DatabaseRef != r.Spec.DatabaseRef {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("databaseRef"), "cannot be changed"))
	}
	if old.Status.Image.PullFrom != "" && old.Status.Image != r.Spec.Image {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("image"), "cannot be changed"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "OracleRestDataService"},
		r.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OracleRestDataService) ValidateDelete() error {
	oraclerestdataservicelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
