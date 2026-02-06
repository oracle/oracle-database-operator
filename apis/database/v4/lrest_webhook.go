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
	"reflect"
	"strings"

	. "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"
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
var lrestlog = logf.Log.WithName("lrest-webhook")

func (r *LREST) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-lrest,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrests,verbs=create;update,versions=v4,name=mlrest.kb.io,admissionReviewVersions={v4,v1beta1}

var _ webhook.CustomDefaulter = &LREST{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LREST) Default(ctx context.Context, obj runtime.Object) error {
	lrest := obj.(*LREST)
	if Bit(lrest.Spec.Trclvl, TRCWEB) == true {
		lrestlog.Info("Setting default values in LREST spec for : " + r.Name)
	}

	if r.Spec.LRESTPort == 0 {
		r.Spec.LRESTPort = 8888
	}

	if r.Spec.Replicas == 0 {
		r.Spec.Replicas = 1
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-database-oracle-com-v4-lrest,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrests,verbs=create;update,versions=v4,name=vlrest.kb.io,admissionReviewVersions={v4,v1beta1}

var _ webhook.CustomValidator = &LREST{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LREST) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	lrest := obj.(*LREST)
	if Bit(lrest.Spec.Trclvl, TRCWEB) == true {
		lrestlog.Info("ValidateCreate", "name", r.Name)
	}

	var allErrs field.ErrorList

	if lrest.Spec.ServiceName == "" && lrest.Spec.DBServer != "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("serviceName"), "Please specify LREST Service name"))
	}

	if reflect.ValueOf(lrest.Spec.LRESTTlsKey).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("lrestTlsKey"), "Please specify LREST Tls key(secret)"))
	}

	if reflect.ValueOf(lrest.Spec.LRESTTlsCrt).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("lrestTlsCrt"), "Please specify LREST Tls Certificate(secret)"))
	}

	/*if r.Spec.SCANName == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("scanName"), "Please specify SCAN Name for LREST"))
	}*/

	if (lrest.Spec.DBServer == "" && lrest.Spec.DBTnsurl == "") || (lrest.Spec.DBServer != "" && lrest.Spec.DBTnsurl != "") {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbServer"), "Please specify Database Server Name/IP Address or tnsalias string"))
	}

	if lrest.Spec.DBTnsurl != "" && (lrest.Spec.DBServer != "" || lrest.Spec.DBPort != 0 || lrest.Spec.ServiceName != "") {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbServer"), "DBtnsurl is orthogonal to (DBServer,DBport,Services)"))
	}

	if lrest.Spec.DBPort == 0 && lrest.Spec.DBServer != "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify DB Server Port"))
	}
	if lrest.Spec.DBPort < 0 && lrest.Spec.DBServer != "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify a valid DB Server Port"))
	}
	if lrest.Spec.LRESTPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPort"), "Please specify a valid LREST Port"))
	}
	if lrest.Spec.Replicas < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("replicas"), "Please specify a valid value for Replicas"))
	}
	if lrest.Spec.LRESTImage == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("lrestImage"), "Please specify name of LREST Image to be used"))
	}
	if lrest.Spec.PwdProtection != "ORAPKI" && reflect.ValueOf(lrest.Spec.LRESTAdminUser).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("cdbAdminUser"), "Please specify user in the root container with sysdba priviledges to manage PDB lifecycle"))
	}
	if lrest.Spec.PwdProtection != "ORAPKI" && reflect.ValueOf(lrest.Spec.LRESTAdminPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("lrestAdminPwd"), "Please specify password for the LREST Administrator to manage PDB lifecycle"))
	}
	/*	if reflect.ValueOf(r.Spec.LRESTPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPwd"), "Please specify password for user LREST_PUBLIC_USER"))
	} */
	if reflect.ValueOf(lrest.Spec.WebLrestServerUser).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("webServerUser"), "Please specify the Web Server User having SQL Administrator role"))
	}
	if reflect.ValueOf(lrest.Spec.WebLrestServerPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("webServerPwd"), "Please specify password for the Web Server User having SQL Administrator role"))
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LREST"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LREST) ValidateUpdate(ctx context.Context, old, newObj runtime.Object) (admission.Warnings, error) {

	isLRESTMarkedToBeDeleted := r.GetDeletionTimestamp() != nil
	if isLRESTMarkedToBeDeleted {
		return nil, nil
	}

	var allErrs field.ErrorList

	// Check for updation errors
	oldLREST, ok := old.(*LREST)
	if !ok {
		return nil, nil
	}
	if Bit(oldLREST.Spec.Trclvl, TRCWEB) == true {
		lrestlog.Info("validate update", "name", r.Name)
	}

	if r.Spec.DBPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify a valid DB Server Port"))
	}
	if r.Spec.LRESTPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPort"), "Please specify a valid LREST Port"))
	}
	if r.Spec.Replicas < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("replicas"), "Please specify a valid value for Replicas"))
	}
	if !strings.EqualFold(oldLREST.Spec.ServiceName, r.Spec.ServiceName) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("replicas"), "cannot be changed"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LREST"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LREST) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	lrest := obj.(*LREST)
	if Bit(lrest.Spec.Trclvl, TRCWEB) == true {
		lrestlog.Info("validate delete", "name", r.Name)
	}

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
