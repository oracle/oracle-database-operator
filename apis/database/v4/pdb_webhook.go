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
var pdblog = logf.Log.WithName("pdb-webhook")

func (r *PDB) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-pdb,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=pdbs,verbs=create;update,versions=v4,name=mpdb.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomDefaulter = &PDB{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *PDB) Default(ctx context.Context, obj runtime.Object) error {
	pdblog.Info("Setting default values in PDB spec for : " + r.Name)

	action := strings.ToUpper(r.Spec.Action)

	if action == "DELETE" {
		if r.Spec.DropAction == "" {
			r.Spec.DropAction = "INCLUDING"
			pdblog.Info(" - dropAction : INCLUDING")
		}
	} else if action != "MODIFY" && action != "STATUS" {
		if r.Spec.ReuseTempFile == nil {
			r.Spec.ReuseTempFile = new(bool)
			*r.Spec.ReuseTempFile = true
			pdblog.Info(" - reuseTempFile : " + strconv.FormatBool(*(r.Spec.ReuseTempFile)))
		}
		if r.Spec.UnlimitedStorage == nil {
			r.Spec.UnlimitedStorage = new(bool)
			*r.Spec.UnlimitedStorage = true
			pdblog.Info(" - unlimitedStorage : " + strconv.FormatBool(*(r.Spec.UnlimitedStorage)))
		}
		if r.Spec.TDEImport == nil {
			r.Spec.TDEImport = new(bool)
			*r.Spec.TDEImport = false
			pdblog.Info(" - tdeImport : " + strconv.FormatBool(*(r.Spec.TDEImport)))
		}
		if r.Spec.TDEExport == nil {
			r.Spec.TDEExport = new(bool)
			*r.Spec.TDEExport = false
			pdblog.Info(" - tdeExport : " + strconv.FormatBool(*(r.Spec.TDEExport)))
		}
		if r.Spec.AsClone == nil {
			r.Spec.AsClone = new(bool)
			*r.Spec.AsClone = false
			pdblog.Info(" - asClone : " + strconv.FormatBool(*(r.Spec.AsClone)))
		}

	}

	if r.Spec.GetScript == nil {
		r.Spec.GetScript = new(bool)
		*r.Spec.GetScript = false
		pdblog.Info(" - getScript : " + strconv.FormatBool(*(r.Spec.GetScript)))
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-database-oracle-com-v4-pdb,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=pdbs,verbs=create;update,versions=v4,name=vpdb.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &PDB{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *PDB) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pdblog.Info("ValidateCreate-Validating PDB spec for : " + r.Name)

	var allErrs field.ErrorList

	r.validateCommon(&allErrs)

	r.validateAction(&allErrs)

	action := strings.ToUpper(r.Spec.Action)

	if len(allErrs) == 0 {
		pdblog.Info("PDB Resource : " + r.Name + " successfully validated for Action : " + action)
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "PDB"},
		r.Name, allErrs)
}

// Validate Action for required parameters
func (r *PDB) validateAction(allErrs *field.ErrorList) {
	action := strings.ToUpper(r.Spec.Action)

	pdblog.Info("Valdiating PDB Resource Action : " + action)

	if reflect.ValueOf(r.Spec.PDBTlsKey).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("pdbTlsKey"), "Please specify PDB Tls Key(secret)"))
	}

	if reflect.ValueOf(r.Spec.PDBTlsCrt).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("pdbTlsCrt"), "Please specify PDB Tls Certificate(secret)"))
	}

	if reflect.ValueOf(r.Spec.PDBTlsCat).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("pdbTlsCat"), "Please specify PDB Tls Certificate Authority(secret)"))
	}
	if reflect.ValueOf(r.Spec.PDBPriKey).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("pdbOrdsPrvKey"), "Please specify PDB Tls Certificate Authority(secret)"))
	}

	switch action {
	case "DELETE":
		/* BUG 36752336 - LREST OPERATOR - DELETE NON-EXISTENT PDB SHOWS LRPDB CREATED MESSAGE */
		if r.Status.OpenMode == "READ WRITE" {
			pdblog.Info("Cannot delete: pdb is open ")
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+r.Spec.PDBName+" "+r.Status.OpenMode))
		}
		r.CheckObjExistence("DELETE", allErrs, r)
	case "CREATE":
		if reflect.ValueOf(r.Spec.AdminName).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("adminName"), "Please specify PDB System Administrator user"))
		}
		if reflect.ValueOf(r.Spec.AdminPwd).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("adminPwd"), "Please specify PDB System Administrator Password"))
		}
		if reflect.ValueOf(r.Spec.WebServerUsr).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("WebServerUser"), "Please specify the http webServerUser"))
		}
		if reflect.ValueOf(r.Spec.WebServerPwd).IsZero() {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("webServerPwd"), "Please specify the http webserverPassword"))
		}

		if r.Spec.FileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("fileNameConversions"), "Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if r.Spec.TotalSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("totalSize"), "When the storage is not UNLIMITED the Total Size must be specified"))
		}
		if r.Spec.TempSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
		}
		if *(r.Spec.TDEImport) {
			r.validateTDEInfo(allErrs)
		}
	case "CLONE":
		// Sample Err: The PDB "pdb1-clone" is invalid: spec.srcPdbName: Required value: Please specify source PDB for Cloning
		if r.Spec.SrcPDBName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("srcPdbName"), "Please specify source PDB name for Cloning"))
		}
		if r.Spec.TotalSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("totalSize"), "When the storage is not UNLIMITED the Total Size must be specified"))
		}
		if r.Spec.TempSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
		}
		/* We don't need this check as ords open the pdb before cloninig */
		/*
			if r.Status.OpenMode == "MOUNTED" {
				pdblog.Info("Cannot clone: pdb is mount ")
				*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+r.Spec.PDBName+" "+r.Status.OpenMode))
			}
		*/
	case "PLUG":
		if r.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if r.Spec.FileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("fileNameConversions"), "Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if r.Spec.SourceFileNameConversions == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("sourceFileNameConversions"), "Please specify a value for sourceFileNameConversions. Values can be a filename convert pattern or NONE"))
		}
		if r.Spec.CopyAction == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("copyAction"), "Please specify a value for copyAction. Values can be COPY, NOCOPY or MOVE"))
		}
		if *(r.Spec.TDEImport) {
			r.validateTDEInfo(allErrs)
		}
	case "UNPLUG":
		if r.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if *(r.Spec.TDEExport) {
			r.validateTDEInfo(allErrs)
		}
		if r.Status.OpenMode == "READ WRITE" {
			pdblog.Info("Cannot unplug: pdb is open ")
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+r.Spec.PDBName+" "+r.Status.OpenMode))
		}
		r.CheckObjExistence("UNPLUG", allErrs, r)
	case "MODIFY":
		if r.Spec.PDBState == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("pdbState"), "Please specify target state of PDB"))
		}
		if r.Spec.ModifyOption == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("modifyOption"), "Please specify an option for opening/closing a PDB"))
		}
		r.CheckObjExistence("MODIY", allErrs, r)
	}
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *PDB) ValidateUpdate(ctx context.Context, old, newObj runtime.Object) (admission.Warnings, error) {
	pdblog.Info("ValidateUpdate-Validating PDB spec for : " + r.Name)

	isPDBMarkedToBeDeleted := r.GetDeletionTimestamp() != nil
	if isPDBMarkedToBeDeleted {
		return nil, nil
	}

	var allErrs field.ErrorList
	action := strings.ToUpper(r.Spec.Action)

	// If PDB CR has been created and in Ready state, only allow updates if the "action" value has changed as well
	if (r.Status.Phase == "Ready") && (r.Status.Action != "MODIFY") && (r.Status.Action != "STATUS") && (r.Status.Action == action) {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("action"), "New action also needs to be specified after PDB is in Ready state"))
	} else {

		// Check Common Validations
		r.validateCommon(&allErrs)

		// Validate required parameters for Action specified
		r.validateAction(&allErrs)

		// Check TDE requirements
		if (action != "DELETE") && (action != "MODIFY") && (action != "STATUS") && (*(r.Spec.TDEImport) || *(r.Spec.TDEExport)) {
			r.validateTDEInfo(&allErrs)
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "PDB"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *PDB) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pdblog.Info("ValidateDelete-Validating PDB spec for : " + r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// Validate common specs needed for all PDB Actions
func (r *PDB) validateCommon(allErrs *field.ErrorList) {
	pdblog.Info("validateCommon", "name", r.Name)

	if r.Spec.Action == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("action"), "Please specify PDB operation to be performed"))
	}
	if r.Spec.CDBResName == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("cdbResName"), "Please specify the name of the CDB Kubernetes resource to use for PDB operations"))
	}
	if r.Spec.PDBName == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("pdbName"), "Please specify name of the PDB to be created"))
	}
}

// Validate TDE information for Create, Plug and Unplug Actions
func (r *PDB) validateTDEInfo(allErrs *field.ErrorList) {
	pdblog.Info("validateTDEInfo", "name", r.Name)

	if reflect.ValueOf(r.Spec.TDEPassword).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdePassword"), "Please specify a value for tdePassword."))
	}
	if r.Spec.TDEKeystorePath == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdeKeystorePath"), "Please specify a value for tdeKeystorePath."))
	}
	if reflect.ValueOf(r.Spec.TDESecret).IsZero() {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("tdeSecret"), "Please specify a value for tdeSecret."))
	}

}

func (r *PDB) CheckObjExistence(action string, allErrs *field.ErrorList, pdb *PDB) {
	/* BUG 36752465 - lrest operator - open non-existent pdb creates a lrpdb with status failed */
	pdblog.Info("Action [" + action + "] checkin " + pdb.Spec.PDBName + " existence")
	if pdb.Status.OpenMode == "" {
		*allErrs = append(*allErrs, field.NotFound(field.NewPath("Spec").Child("PDBName"), " "+pdb.Spec.PDBName+" does not exist : action "+action+" failure"))

	}
}
