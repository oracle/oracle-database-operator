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

package v1alpha1

import (
	"reflect"
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
var cdblog = logf.Log.WithName("cdb-webhook")

func (r *CDB) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-cdb,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=cdbs,verbs=create;update,versions=v1alpha1,name=mcdb.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &CDB{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *CDB) Default() {
	cdblog.Info("Setting default values in CDB spec for : " + r.Name)

	if r.Spec.ORDSPort == 0 {
		r.Spec.ORDSPort = 8888
	}

	if r.Spec.Replicas == 0 {
		r.Spec.Replicas = 1
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-database-oracle-com-v1alpha1-cdb,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=cdbs,verbs=create;update,versions=v1alpha1,name=vcdb.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &CDB{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *CDB) ValidateCreate() error {
	cdblog.Info("ValidateCreate", "name", r.Name)

	var allErrs field.ErrorList

	if r.Spec.ServiceName == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("serviceName"), "Please specify CDB Service name"))
	}
        
        if reflect.ValueOf(r.Spec.CDBTlsKey).IsZero()  {
                allErrs = append(allErrs,
                        field.Required(field.NewPath("spec").Child("cdbTlsKey"), "Please specify CDB Tls key(secret)"))
        }

        if reflect.ValueOf(r.Spec.CDBTlsCrt).IsZero()  {
                allErrs = append(allErrs,
                        field.Required(field.NewPath("spec").Child("cdbTlsCrt"), "Please specify CDB Tls Certificate(secret)"))
        }

	if r.Spec.SCANName == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("scanName"), "Please specify SCAN Name for CDB"))
	}
	if r.Spec.DBServer == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbServer"), "Please specify Database Server Name or IP Address"))
	}
	if r.Spec.DBPort == 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify DB Server Port"))
	}
	if r.Spec.DBPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify a valid DB Server Port"))
	}
	if r.Spec.ORDSPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPort"), "Please specify a valid ORDS Port"))
	}
	if r.Spec.Replicas < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("replicas"), "Please specify a valid value for Replicas"))
	}
	if r.Spec.ORDSImage == "" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsImage"), "Please specify name of ORDS Image to be used"))
	}
	if reflect.ValueOf(r.Spec.CDBAdminUser).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("cdbAdminUser"), "Please specify user in the root container with sysdba priviledges to manage PDB lifecycle"))
	}
	if reflect.ValueOf(r.Spec.CDBAdminPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("cdbAdminPwd"), "Please specify password for the CDB Administrator to manage PDB lifecycle"))
	}
	if reflect.ValueOf(r.Spec.ORDSPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPwd"), "Please specify password for user ORDS_PUBLIC_USER"))
	}
	if reflect.ValueOf(r.Spec.WebServerUser).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("webServerUser"), "Please specify the Web Server User having SQL Administrator role"))
	}
	if reflect.ValueOf(r.Spec.WebServerPwd).IsZero() {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("webServerPwd"), "Please specify password for the Web Server User having SQL Administrator role"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "CDB"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *CDB) ValidateUpdate(old runtime.Object) error {
	cdblog.Info("validate update", "name", r.Name)

	isCDBMarkedToBeDeleted := r.GetDeletionTimestamp() != nil
	if isCDBMarkedToBeDeleted {
		return nil
	}

	var allErrs field.ErrorList

	// Check for updation errors
	oldCDB, ok := old.(*CDB)
	if !ok {
		return nil
	}

	if r.Spec.DBPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("dbPort"), "Please specify a valid DB Server Port"))
	}
	if r.Spec.ORDSPort < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("ordsPort"), "Please specify a valid ORDS Port"))
	}
	if r.Spec.Replicas < 0 {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("replicas"), "Please specify a valid value for Replicas"))
	}
	if !strings.EqualFold(oldCDB.Spec.ServiceName, r.Spec.ServiceName) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("replicas"), "cannot be changed"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "CDB"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *CDB) ValidateDelete() error {
	cdblog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
