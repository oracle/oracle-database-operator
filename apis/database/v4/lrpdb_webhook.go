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

	. "github.com/oracle/oracle-database-operator/commons/multitenant/lrest"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	//"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// log is for logging in this package.
var lrpdblog = logf.Log.WithName("lrpdb-webhook")
var lrpdbAuthzClient authorizationclient.AuthorizationV1Interface

// SetupWebhookWithManager set the webhook
func (r *LRPDB) SetupWebhookWithManager(mgr ctrl.Manager) error {
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	lrpdbAuthzClient = clientset.AuthorizationV1()
	return ctrl.NewWebhookManagedBy[*LRPDB](mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-lrpdb,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrpdbs,verbs=create;update,versions=v4,name=mlrpdb.kb.io,admissionReviewVersions=v1

// Use the generic admission interfaces
var _ admission.Defaulter[*LRPDB] = &LRPDB{}
var _ admission.Validator[*LRPDB] = &LRPDB{}

//+kubebuilder:webhook:path=/validate-database-oracle-com-v4-lrpdb,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrpdbs,verbs=create;update,versions=v4,name=vlrpdb.kb.io,admissionReviewVersions=v1

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LRPDB) Default(ctx context.Context, obj *LRPDB) error {
	pdb := obj

	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		// lrpdblog.Info("Setting default values in LRPDB spec for : " + pdb.Name)
		lrpdblog.Info("Setting default values in LRPDB spec for : " + r.Name)
	}

	action := strings.ToUpper(pdb.Spec.Action)

	if action == "DELETE" {
		if pdb.Spec.DropAction == "" {
			pdb.Spec.DropAction = "KEEP"
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - dropAction : KEEP")
			}
		}
	} else if action != "MODIFY" && action != "STATUS" {
		if pdb.Spec.ReuseTempFile == nil {
			pdb.Spec.ReuseTempFile = new(bool)
			*pdb.Spec.ReuseTempFile = true
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - reuseTempFile : " + strconv.FormatBool(*(pdb.Spec.ReuseTempFile)))
			}
		}
		if pdb.Spec.UnlimitedStorage == nil {
			pdb.Spec.UnlimitedStorage = new(bool)
			*pdb.Spec.UnlimitedStorage = true
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - unlimitedStorage : " + strconv.FormatBool(*(pdb.Spec.UnlimitedStorage)))
			}
		}
		if pdb.Spec.LTDEImport == nil {
			pdb.Spec.LTDEImport = new(bool)
			*pdb.Spec.LTDEImport = false
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - tdeImport : " + strconv.FormatBool(*(pdb.Spec.LTDEImport)))
			}
		}
		if pdb.Spec.LTDEExport == nil {
			pdb.Spec.LTDEExport = new(bool)
			*pdb.Spec.LTDEExport = false
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - tdeExport : " + strconv.FormatBool(*(pdb.Spec.LTDEExport)))
			}
		}
		if pdb.Spec.AsClone == nil {
			pdb.Spec.AsClone = new(bool)
			*pdb.Spec.AsClone = false
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info(" - asClone : " + strconv.FormatBool(*(pdb.Spec.AsClone)))
			}
		}
	}

	if pdb.Spec.GetScript == nil {
		pdb.Spec.GetScript = new(bool)
		*pdb.Spec.GetScript = false
		lrpdblog.Info(" - getScript : " + strconv.FormatBool(*(pdb.Spec.GetScript)))
	}
	return nil
}

//+kubebuilder:webhook:path=/validate-database-oracle-com-v4-lrpdb,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=lrpdbs,verbs=create;update,versions=v4,name=vlrpdb.kb.io,admissionReviewVersions={v4,v1beta1}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateCreate(ctx context.Context, obj *LRPDB) (admission.Warnings, error) {
	pdb := obj
	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("ValidateCreate-Validating LRPDB spec for : " + r.Name)
	}

	var allErrs field.ErrorList

	r.validateCommon(ctx, &allErrs, *pdb)

	r.validateAction(ctx, &allErrs, *pdb)

	r.validatePdbAppUserSecretAccess(ctx, &allErrs, pdb)

	action := strings.ToUpper(pdb.Spec.Action)

	if len(allErrs) == 0 {
		if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
			lrpdblog.Info("LRPDB Resource : " + r.Name + " successfully validated for Action : " + action)
		}
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LRPDB"},
		r.Name, allErrs)
}

// Validate Action for required parameters
func (r *LRPDB) validateAction(ctx context.Context, allErrs *field.ErrorList, pdb LRPDB) {

	pdbstate := strings.ToUpper(pdb.Spec.LRPDBState)
	scrdatabase := strings.ToUpper(pdb.Spec.SrcLRPDBName)
	//plsql := strings.ToUpper(pdb.Spec.PLSQLBlock)

	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("Valdiating LRPDB Resource ")
	}
	/* Parameters required by the creation */
	if Bit(pdb.Status.PDBBitMask, PDBCRT) == false && pdb.Spec.PDBBitMask == 00 {
		/* the condition pdbitmask == 00 is required by the aut0-discovery feature
		if the pdb already exists then administrative user is already available */

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
		/*
			if pdb.Spec.TempSize == "" {
				*allErrs = append(*allErrs,
					field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
			}
			if *(pdb.Spec.LTDEImport) {
				r.validateTDEInfo(allErrs, ctx, pdb)
			}
		*/

		if strings.Contains(pdb.Spec.LRPDBName, "-") {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("pdbName"), "cannot contains dash "))

		}
	}

	/* We cannot open|close|delete|unplug a non existing pdb */
	if (pdbstate == "OPEN" || pdbstate == "CLOSE" || pdbstate == "DELETE" || pdbstate == "UNPLUG") && Bit(pdb.Status.PDBBitMask, PDBCRT) == false {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("LRPDBState"), "PDB does not exists"))
	}

	if pdbstate == "CLOSE" || pdbstate == "OPEN" || pdbstate == "DELETE" || Bit(pdb.Status.PDBBitMask, PDBCRT) == true {
		//var Impdel *bool

		if pdb.Spec.ImperativeLrpdbDeletion != true {
			//*allErrs = append(*allErrs,
			//	field.Required(field.NewPath("spec").Child("ImperativeLrpdbDeletion"), "Imperative Deletetion must be set"))
			lrpdblog.Info("pdb.Spec.ImperativeLrpdbDeletion:true ")
		}

	}

	/* Database already exists
	if scrdatabase != "" && Bit(pdb.Status.PDBBitMask, PDBCRT) == true {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("SrcLRPDBName"), "PDB already exists/Cannot clone"))
	}
	*/

	if pdbstate == "PLUG" && pdb.Spec.XMLFileName != "" && Bit(pdb.Status.PDBBitMask, PDBCRT) == false && Bit(pdb.Status.PDBBitMask, PDBPLE) == false {
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
			r.validateTDEInfo(ctx, allErrs, pdb)
		}

	}

	if pdbstate == "UNPLUG" && pdb.Spec.XMLFileName != "" && Bit(pdb.Status.PDBBitMask, PDBCRT) == true && Bit(pdb.Status.PDBBitMask, FNALAZ) == true && Bit(pdb.Status.PDBBitMask, PDBUPE) == false {
		if pdb.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if *(pdb.Spec.LTDEExport) {
			r.validateTDEInfo(ctx, allErrs, pdb)
		}
		if pdb.Status.OpenMode == "READ WRITE" {
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info("Cannot unplug: pdb is open ")
			}
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}
		r.CheckObjExistence(ctx, "UNPLUG", allErrs, pdb)
	}

	/*
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
	*/

	/* Check clone parameters */
	if scrdatabase != "" && Bit(pdb.Status.PDBBitMask, PDBCRT|FNALAZ|PDBCRE) == false {
		if pdb.Spec.TotalSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("totalSize"), "When the storage is not UNLIMITED the Total Size must be specified"))
		}
		if pdb.Spec.TempSize == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("tempSize"), "When the storage is not UNLIMITED the Temp Size must be specified"))
		}
		if pdb.Status.OpenMode == "MOUNT" {
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info("Cannot clone: pdb is mount ")
			}
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}

	}

	if pdbstate == "UNPLUG" {
		if pdb.Spec.XMLFileName == "" {
			*allErrs = append(*allErrs,
				field.Required(field.NewPath("spec").Child("xmlFileName"), "Please specify XML metadata filename"))
		}
		if *(pdb.Spec.LTDEExport) {
			r.validateTDEInfo(ctx, allErrs, pdb)
		}
		if pdb.Status.OpenMode == "READ WRITE" {
			if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
				lrpdblog.Info("Cannot unplug: pdb is open ")
			}
			*allErrs = append(*allErrs, field.Invalid(field.NewPath("status").Child("OpenMode"), "READ WRITE", "pdb "+pdb.Spec.LRPDBName+" "+pdb.Status.OpenMode))
		}
		r.CheckObjExistence(ctx, "UNPLUG", allErrs, pdb)
	}
}

// CheckObjExistence - BUG 36752465 - lrest operator - open non-existent pdb creates a lrpdb with status failed
func (r *LRPDB) CheckObjExistence(ctx context.Context, action string, allErrs *field.ErrorList, pdb LRPDB) {
	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("obj:" + r.Name)
		lrpdblog.Info("Action [" + action + "] checkin " + pdb.Spec.LRPDBName + " existence")
	}
	if pdb.Status.OpenMode == "" {
		*allErrs = append(*allErrs, field.NotFound(field.NewPath("Spec").Child("LRPDBName"), " "+pdb.Spec.LRPDBName+" does not exist : action "+action+" failure"))

	}
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateUpdate(ctx context.Context, old, obj *LRPDB) (admission.Warnings, error) {
	pdbold := old
	if Bit(pdbold.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("ValidateUpdate-Validating LRPDB spec for : " + r.Name)
	}

	pdb := obj
	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("ValidateUpdate-Validating LRPDB spec for : " + r.Name)
	}

	isLRPDBMarkedToBeDeleted := r.GetDeletionTimestamp() != nil
	if isLRPDBMarkedToBeDeleted {
		return nil, nil
	}

	var allErrs field.ErrorList
	action := strings.ToUpper(pdb.Spec.Action)

	if pdb.Spec.Pdbappuser != "" && pdb.Spec.Pdbappuser != pdbold.Spec.Pdbappuser {
		r.validatePdbAppUserSecretAccess(ctx, &allErrs, pdb)
	}

	// If LRPDB CR has been created and in Ready state, only allow updates if the "action" value has changed as well
	if (pdb.Status.Phase == "Ready") && (pdb.Status.Action != "MODIFY") && (pdb.Status.Action != "STATUS") && (pdb.Status.Action != "NOACTION") && (pdb.Status.Action == action) {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("action"), "New action also needs to be specified after LRPDB is in Ready state"))
	} else {

		// Check Common Validations
		r.validateCommon(ctx, &allErrs, *pdb)

		// Validate required parameters for Action specified
		r.validateAction(ctx, &allErrs, *pdb)

		// Check TDE requirements
		if (action != "DELETE") && (action != "MODIFY") && (action != "STATUS") && (*(pdb.Spec.LTDEImport) || *(pdb.Spec.LTDEExport)) {
			r.validateTDEInfo(ctx, &allErrs, *pdb)
		}
	}

	//* Make sure that only one method is used to reset status *//
	if pdb.Spec.PDBBitMaskStr != "" && pdb.Spec.PDBBitMask != 0 && pdb.Spec.LRPDBState == "RESET" {
		allErrs = append(allErrs,
			field.Required(field.NewPath("spec").Child("state"), "you cannot reset state using string format and number values at the same time"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "LRPDB"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LRPDB) ValidateDelete(ctx context.Context, obj *LRPDB) (admission.Warnings, error) {
	pdb := obj
	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("ValidateDelete-Validating LRPDB spec for : " + r.Name)
	}

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// Validate common specs needed for all LRPDB Actions
func (r *LRPDB) validateCommon(ctx context.Context, allErrs *field.ErrorList, pdb LRPDB) {
	if Bit(pdb.Spec.Trclvl, TRCWEB) == true {
		lrpdblog.Info("obj:" + r.Name)
		lrpdblog.Info("validateCommon", "name", pdb.Name)
	}

	/* if pdb.Spec.Action == "" {
		*allErrs = append(*allErrs,
			field.Required(field.NewPath("spec").Child("action"), "Please specify LRPDB operation to be performed"))
	} */
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
func (r *LRPDB) validateTDEInfo(ctx context.Context, allErrs *field.ErrorList, pdb LRPDB) {
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

//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *LRPDB) validatePdbAppUserSecretAccess(ctx context.Context, allErrs *field.ErrorList, pdb *LRPDB) {
	if pdb.Spec.Pdbappuser == "" {
		return
	}

	path := field.NewPath("spec").Child("pdbappuser")
	if lrpdbAuthzClient == nil {
		*allErrs = append(*allErrs,
			field.InternalError(path, fmt.Errorf("authorization client is not initialized")))
		return
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		*allErrs = append(*allErrs,
			field.InternalError(path, fmt.Errorf("cannot read admission request user info: %w", err)))
		return
	}
	resources := []struct {
		resource string
		name     string
	}{
		{
			resource: "secrets",
			name:     pdb.Spec.Pdbappuser,
		},
		{
			resource: "configmaps",
			name:     pdb.Spec.PDBConfigMap,
		},
		{
			resource: "configmaps",
			name:     pdb.Spec.PLSQLBlock,
		},
	}

	/*
		sar := &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   req.UserInfo.Username,
				Groups: req.UserInfo.Groups,
				Extra:  convertAdmissionExtra(req.UserInfo.Extra),
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: pdb.Namespace, Verb: "get", Group: "", Resource: "secrets", Name: pdb.Spec.Pdbappuser,
				},
			},
		}
	*/

	sar := &authorizationv1.SubjectAccessReview{}
	for _, r := range resources {
		sar = &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User:   req.UserInfo.Username,
				Groups: req.UserInfo.Groups,
				Extra:  convertAdmissionExtra(req.UserInfo.Extra),
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: pdb.Namespace,
					Verb:      "get",
					Group:     "",
					Resource:  r.resource,
					Name:      r.name,
				},
			},
		}
	}

	result, err := lrpdbAuthzClient.SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		*allErrs = append(*allErrs,
			field.InternalError(path, fmt.Errorf("subject access review failed: %w", err)))
		return
	}

	if !result.Status.Allowed {
		reason := result.Status.Reason
		if reason == "" {
			reason = "requesting user is not allowed to get this Secret"
		}
		*allErrs = append(*allErrs,
			field.Forbidden(path, fmt.Sprintf("cannot reference Secret %q in namespace %q: %s",
				pdb.Spec.Pdbappuser, pdb.Namespace, reason)))
	}
}

func convertAdmissionExtra(extra map[string]authenticationv1.ExtraValue) map[string]authorizationv1.ExtraValue {
	if len(extra) == 0 {
		return nil
	}

	converted := make(map[string]authorizationv1.ExtraValue, len(extra))
	for key, values := range extra {
		converted[key] = authorizationv1.ExtraValue(values)
	}
	return converted
}
