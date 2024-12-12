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

/*    MODIFIED   (MM/DD/YY)
**    rcitton     07/14/22 - 33822886
 */

package v4

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

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
var lrpdblog = logf.Log.WithName("lrpdb-webhook")

func (r *LRPDB) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		WithValidator(&LRPDB{}).
		WithDefaulter(&LRPDB{}).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-lrpdb,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrpdbs,verbs=create;update,versions=v4,name=mlrpdb.kb.io,admissionReviewVersions={v4,v1beta1}

var _ webhook.CustomDefaulter = &LRPDB{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LRPDB) Default(ctx context.Context, obj runtime.Object) error {
	pdb, ok := obj.(*LRPDB)
	if !ok {
		return fmt.Errorf("expected an LRPDB object but got %T", obj)
	}
	lrpdblog.Info("Setting default values in LRPDB spec for : " + pdb.Name)

	action := strings.ToUpper(pdb.Spec.Action)

	if action == "DELETE" {
		if pdb.Spec.DropAction == "" {
			pdb.Spec.DropAction = "KEEP"
			lrpdblog.Info(" - dropAction : KEEP")
		}
	} else if action != "MODIFY" && action != "STATUS" {
		if pdb.Spec.ReuseTempFile == nil {
			pdb.Spec.ReuseTempFile = new(bool)
			*pdb.Spec.ReuseTempFile = true
			lrpdblog.Info(" - reuseTempFile : " + strconv.FormatBool(*(pdb.Spec.ReuseTempFile)))
		}
		if pdb.Spec.UnlimitedStorage == nil {
			pdb.Spec.UnlimitedStorage = new(bool)
			*pdb.Spec.UnlimitedStorage = true
			lrpdblog.Info(" - unlimitedStorage : " + strconv.FormatBool(*(pdb.Spec.UnlimitedStorage)))
		}
		if pdb.Spec.LTDEImport == nil {
			pdb.Spec.LTDEImport = new(bool)
			*pdb.Spec.LTDEImport = false
			lrpdblog.Info(" - tdeImport : " + strconv.FormatBool(*(pdb.Spec.LTDEImport)))
		}
		if pdb.Spec.LTDEExport == nil {
			pdb.Spec.LTDEExport = new(bool)
			*pdb.Spec.LTDEExport = false
			lrpdblog.Info(" - tdeExport : " + strconv.FormatBool(*(pdb.Spec.LTDEExport)))
		}
		if pdb.Spec.AsClone == nil {
			pdb.Spec.AsClone = new(bool)
			*pdb.Spec.AsClone = false
			lrpdblog.Info(" - asClone : " + strconv.FormatBool(*(pdb.Spec.AsClone)))
		}
	}

	if pdb.Spec.GetScript == nil {
		pdb.Spec.GetScript = new(bool)
		*pdb.Spec.GetScript = false
		lrpdblog.Info(" - getScript : " + strconv.FormatBool(*(pdb.Spec.GetScript)))
	}
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-database-oracle-com-v4-lrpdb,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrpdbs,verbs=create;update,versions=v4,name=vlrpdb.kb.io,admissionReviewVersions={v4,v1beta1}

var _ webhook.CustomValidator = &LRPDB{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	lrpdblog.Info("ValidateCreate-Validating LRPDB spec for : " + r.Name)
	pdb := obj.(*LRPDB)

	var allErrs field.ErrorList

	r.validateCommon(&allErrs, ctx, *pdb)

	r.validateAction(&allErrs, ctx, *pdb)

	action := strings.ToUpper(pdb.Spec.Action)

	if len(allErrs) == 0 {
		lrpdblog.Info("LRPDB Resource : " + r.Name + " successfully validated for Action : " + action)
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LRPDB"},
		r.Name, allErrs)
	return nil, nil
}

// Validate Action for required parameters
func (r *LRPDB) validateAction(allErrs *field.ErrorList, ctx context.Context, pdb LRPDB) {
	action := strings.ToUpper(pdb.Spec.Action)

	lrpdblog.Info("Valdiating LRPDB Resource Action : " + action)

	if reflect.ValueOf(pdb.Spec.LRPDBTlsKey).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("lrpdbTlsKey"), "Please specify LRPDB Tls Key(secret)"))
	}

	if reflect.ValueOf(pdb.Spec.LRPDBTlsCrt).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("lrpdbTlsCrt"), "Please specify LRPDB Tls Certificate(secret)"))
	}

	if reflect.ValueOf(pdb.Spec.LRPDBTlsCat).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("lrpdbTlsCat"), "Please specify LRPDB Tls Certificate Authority(secret)"))
	}

	switch action {
	case "DELETE":
		/* BUG 36752336 - LREST OPERATOR - DELETE NON-EXISTENT PDB SHOWS LRPDB CREATED MESSAGE */
		if pdb.Status.OpenMode == "READ WRITE" {
			lrpdblog.Info("Cannot delete: pdb is open ")
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}
		r.CheckObjExistence("DELETE", allErrs, ctx, pdb)
	case "CREATE":
		if reflect.ValueOf(pdb.Spec.AdminpdbUser).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("adminpdbUser"), "Please specify LRPDB System Administrator user"))
		}
		if reflect.ValueOf(pdb.Spec.AdminpdbPass).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("adminpdbPass"), "Please specify LRPDB System Administrator Password"))
		}
		if pdb.Spec.FileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("fileNameConversions"), "Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if pdb.Spec.TotalSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("totalSize"), "When the storage is not UNLIMITED the Total Size must be specified"))
		}
		if pdb.Spec.TempSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
		}
		if *(pdb.Spec.LTDEImport) {
			r.validateTDEInfo(allErrs, ctx, pdb)
		}
	case "CLONE":
		// Sample Err: The LRPDB "lrpdb1-clone" is invalid: spec.srcPdbName: Required value: Please specify source LRPDB for Cloning
		if pdb.Spec.SrcLRPDBName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("srcPdbName"), "Please specify source LRPDB name for Cloning"))
		}
		if pdb.Spec.TotalSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("totalSize"), "When the storage is not UNLIMITED the Total Size must be specified"))
		}
		if pdb.Spec.TempSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
		}
		if pdb.Status.OpenMode == "MOUNT" {
			lrpdblog.Info("Cannot clone: pdb is mount ")
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}
	case "PLUG":
		if pdb.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if pdb.Spec.FileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("fileNameConversions"), "Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if pdb.Spec.SourceFileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("sourceFileNameConversions"), "Please specify a value for sourceFileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if pdb.Spec.CopyAction == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("copyAction"), "Please specify a value for copyAction. Values can be COPY, NOCOPY or MOVE"))
		}
		if *(pdb.Spec.LTDEImport) {
			r.validateTDEInfo(allErrs, ctx, pdb)
		}
	case "UNPLUG":
		if pdb.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if *(pdb.Spec.LTDEExport) {
			r.validateTDEInfo(allErrs, ctx, pdb)
		}
		if pdb.Status.OpenMode == "READ WRITE" {
			lrpdblog.Info("Cannot unplug: pdb is open ")
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}
		r.CheckObjExistence("UNPLUG", allErrs, ctx, pdb)
	case "MODIFY":

		if pdb.Spec.LRPDBState == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("lrpdbState"), "Please specify target state of LRPDB"))
		}
		if pdb.Spec.ModifyOption == "" && pdb.Spec.AlterSystem == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("modifyOption"), "Please specify an option for opening/closing a LRPDB or alter system parameter"))
		}
		r.CheckObjExistence("MODIFY", allErrs, ctx, pdb)
	}
}

func (r *LRPDB) CheckObjExistence(action string, allErrs *field.ErrorList, ctx context.Context, pdb LRPDB) {
	/* BUG 36752465 - lrest operator - open non-existent pdb creates a lrpdb with status failed */
	lrpdblog.Info("Action [" + action + "] checkin " + pdb.Spec.LRPDBName + " existence")
	if pdb.Status.OpenMode == "" {
		*allErrs = append(*allErrs, field.NotFound(field.NewPath("Spec").Child("LRPDBName"), " "+pdb.Spec.LRPDBName+" does not exist : action "+action+" failure"))

	}
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateUpdate(ctx context.Context, obj runtime.Object, old runtime.Object) (admission.Warnings, error) {
	lrpdblog.Info("ValidateUpdate-Validating LRPDB spec for : " + r.Name)
	pdb := old.(*LRPDB)

	isLRPDBMarkedToBeDeleted := r.GetDeletionTimestamp() != nil
	if isLRPDBMarkedToBeDeleted {
		return nil, nil
	}

	var allErrs field.ErrorList
	action := strings.ToUpper(pdb.Spec.Action)

	// If LRPDB CR has been created and in Ready state, only allow updates if the "action" value has changed as well
	if (pdb.Status.Phase == "Ready") && (pdb.Status.Action != "MODIFY") && (pdb.Status.Action != "STATUS") && (pdb.Status.Action != "NOACTION") && (pdb.Status.Action == action) {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("action"), "New action also needs to be specified after LRPDB is in Ready state"))
	} else {

		// Check Common Validations
		r.validateCommon(&allErrs, ctx, *pdb)

		// Validate required parameters for Action specified
		r.validateAction(&allErrs, ctx, *pdb)

		// Check TDE requirements
		if (action != "DELETE") && (action != "MODIFY") && (action != "STATUS") && (*(pdb.Spec.LTDEImport) || *(pdb.Spec.LTDEExport)) {
			r.validateTDEInfo(&allErrs, ctx, *pdb)
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LRPDB"},
		r.Name, allErrs)
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	lrpdblog.Info("ValidateDelete-Validating LRPDB spec for : " + r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// Validate common specs needed for all LRPDB Actions
func (r *LRPDB) validateCommon(allErrs *field.ErrorList, ctx context.Context, pdb LRPDB) {
	lrpdblog.Info("validateCommon", "name", pdb.Name)

	if pdb.Spec.Action == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("action"), "Please specify LRPDB operation to be performed"))
	}
	if pdb.Spec.CDBResName == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("cdbResName"), "Please specify the name of the CDB Kubernetes resource to use for LRPDB operations"))
	}
	if pdb.Spec.CDBNamespace == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("cdbNamespace"), "Please specify the namespace of the rest server  to use for LRPDB operations"))
	}
	if pdb.Spec.LRPDBName == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("lrpdbName"), "Please specify name of the LRPDB to be created"))
	}
}

// Validate TDE information for Create, Plug and Unplug Actions
func (r *LRPDB) validateTDEInfo(allErrs *field.ErrorList, ctx context.Context, pdb LRPDB) {
	lrpdblog.Info("validateTDEInfo", "name", r.Name)

	if reflect.ValueOf(pdb.Spec.LTDEPassword).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdePassword"), "Please specify a value for tdePassword."))
	}
	if pdb.Spec.LTDEKeystorePath == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdeKeystorePath"), "Please specify a value for tdeKeystorePath."))
	}
	if reflect.ValueOf(pdb.Spec.LTDESecret).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdeSecret"), "Please specify a value for tdeSecret."))
	}

}
