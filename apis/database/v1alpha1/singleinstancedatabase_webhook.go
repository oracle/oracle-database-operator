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
	"strings"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var singleinstancedatabaselog = logf.Log.WithName("singleinstancedatabase-resource")

func (r *SingleInstanceDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,verbs=create;update,versions=v1alpha1,name=msingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &SingleInstanceDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SingleInstanceDatabase) Default() {
	singleinstancedatabaselog.Info("default", "name", r.Name)

	if r.Spec.Edition == "express" {
		r.Spec.Replicas = 1
		if r.Spec.Sid == "" {
			r.Spec.Sid = "XE"
		}
	}
	if r.Spec.Pdbname == "" {
		if r.Spec.Edition == "express" {
			r.Spec.Pdbname = "XEPDB1"
		} else {
			r.Spec.Pdbname = "ORCLPDB1"
		}

	}

}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v1alpha1-singleinstancedatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=singleinstancedatabases,versions=v1alpha1,name=vsingleinstancedatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &SingleInstanceDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateCreate() error {
	singleinstancedatabaselog.Info("validate create", "name", r.Name)
	var allErrs field.ErrorList

	if r.Spec.Replicas > 1 {
		valMsg := ""
		if r.Spec.Edition == "express" {
			valMsg = "should be 1 for express edition"
		}
		if r.Spec.Image.PrebuiltDB {
			valMsg = "should be 1 for prebuiltDB"
		}
		if valMsg != "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("replicas"), r.Spec.Replicas, valMsg))
		}
	}

	// Persistence spec validation
	if r.Spec.Persistence.Size == "" && (r.Spec.Persistence.AccessMode != "" || r.Spec.Persistence.StorageClass != "" ) || 
		(r.Spec.Persistence.Size != "" && r.Spec.Persistence.AccessMode == "" ) {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("persistence"), r.Spec.Replicas, "invalid persistence spec"))
	}

	// Pre-built db
	if r.Spec.Image.PrebuiltDB {
		if r.Spec.Pdbname != "" && r.Spec.Edition != "express" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("pdbName"), r.Spec.Pdbname,
					"cannot change pdbName for prebuilt db"))
		}
		if r.Spec.CloneFrom != "" && r.Spec.Edition != "express" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec").Child("cloneFrom"), r.Spec.CloneFrom,
					"cannot clone to create a prebuilt db"))
		}
		if len(allErrs) == 0 {
			return nil
		}
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
			r.Name, allErrs)
	}

	if !r.Spec.Image.PrebuiltDB &&
		r.Spec.Persistence.AccessMode != "ReadWriteMany" && r.Spec.Persistence.AccessMode != "ReadWriteOnce" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("persistence"), r.Spec.Persistence.AccessMode,
				"should be either \"ReadWriteOnce\" or \"ReadWriteMany\""))
	}

	if r.Spec.Persistence.AccessMode == "ReadWriteOnce" && r.Spec.Replicas != 1 {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("replicas"), r.Spec.Replicas,
				"should be 1 for accessMode \"ReadWriteOnce\""))
	}
	if r.Spec.Edition == "express" && r.Spec.CloneFrom != "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("cloneFrom"), r.Spec.CloneFrom,
				"Cloning not supported for Express edition"))
	}
	if r.Spec.Edition == "express" && strings.ToUpper(r.Spec.Sid) != "XE" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("sid"), r.Spec.Sid,
				"Express edition SID must be XE"))
	}
	if r.Spec.Edition == "express" && strings.ToUpper(r.Spec.Pdbname) != "XEPDB1" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("pdbName"), r.Spec.Pdbname,
				"Express edition PDB must be XEPDB1"))
	}
	//Edition must be passed when cloning from a source database other than same k8s cluster
	if strings.Contains(r.Spec.CloneFrom, ":") && strings.Contains(r.Spec.CloneFrom, "/") && r.Spec.Edition == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec").Child("edition"), r.Spec.CloneFrom,
				"Edition must be passed when cloning from a source database other than same k8s cluster"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateUpdate(oldRuntimeObject runtime.Object) error {
	singleinstancedatabaselog.Info("validate update", "name", r.Name)
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

	// Pre-built db
	if r.Spec.Image.PrebuiltDB {
		return nil
	}

	// Now check for updation errors
	old, ok := oldRuntimeObject.(*SingleInstanceDatabase)
	if !ok {
		return nil
	}

	edition := r.Spec.Edition
	if r.Spec.Edition == "" {
		edition = "Enterprise"
	}
	if r.Spec.CloneFrom == "" && old.Status.Edition != "" && !strings.EqualFold(old.Status.Edition, edition) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("edition"), "cannot be changed"))
	}
	if old.Status.Charset != "" && !strings.EqualFold(old.Status.Charset, r.Spec.Charset) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("charset"), "cannot be changed"))
	}
	if old.Status.Sid != "" && !strings.EqualFold(r.Spec.Sid, old.Status.Sid) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("sid"), "cannot be changed"))
	}
	if old.Status.Pdbname != "" && !strings.EqualFold(old.Status.Pdbname, r.Spec.Pdbname) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("pdbname"), "cannot be changed"))
	}
	if old.Status.CloneFrom != "" &&
		(old.Status.CloneFrom == dbcommons.NoCloneRef && r.Spec.CloneFrom != "" ||
			old.Status.CloneFrom != dbcommons.NoCloneRef && old.Status.CloneFrom != r.Spec.CloneFrom) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("cloneFrom"), "cannot be changed"))
	}
	if old.Status.OrdsReference != "" && r.Status.Persistence != r.Spec.Persistence {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("persistence"), "uninstall ORDS to change Persistence"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SingleInstanceDatabase) ValidateDelete() error {
	singleinstancedatabaselog.Info("validate delete", "name", r.Name)
	var allErrs field.ErrorList
	if r.Status.OrdsReference != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("status").Child("ordsInstalled"), "uninstall ORDS to cleanup this SIDB"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "SingleInstanceDatabase"},
		r.Name, allErrs)
}
