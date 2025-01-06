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
var autonomousdatabaserestorelog = logf.Log.WithName("autonomousdatabaserestore-resource")

func (r *AutonomousDatabaseRestore) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v4-autonomousdatabaserestore,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=autonomousdatabaserestores,versions=v4,name=vautonomousdatabaserestorev4.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &AutonomousDatabaseRestore{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabaseRestore) ValidateCreate() (admission.Warnings, error) {
	autonomousdatabaserestorelog.Info("validate create", "name", r.Name)

	var allErrs field.ErrorList

	namespaces := dbcommons.GetWatchNamespaces()
	_, hasEmptyString := namespaces[""]
	isClusterScoped := len(namespaces) == 1 && hasEmptyString
	if !isClusterScoped {
		_, containsNamespace := namespaces[r.Namespace]
		// Check if the allowed namespaces maps contains the required namespace
		if len(namespaces) != 0 && !containsNamespace {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
					"Oracle database operator doesn't watch over this namespace"))
		}
	}

	// Validate the target ADB
	if r.Spec.Target.K8sAdb.Name == nil && r.Spec.Target.OciAdb.OCID == nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("target"), "target ADB is empty"))
	}

	if r.Spec.Target.K8sAdb.Name != nil && r.Spec.Target.OciAdb.OCID != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("target"), "specify either k8sADB.name or ociADB.ocid, but not both"))
	}

	// Validate the restore source
	if r.Spec.Source.K8sAdbBackup.Name == nil &&
		r.Spec.Source.PointInTime.Timestamp == nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("source"), "retore source is empty"))
	}

	if r.Spec.Source.K8sAdbBackup.Name != nil &&
		r.Spec.Source.PointInTime.Timestamp != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("source"), "cannot apply backupName and the PITR parameters at the same time"))
	}

	// Verify the timestamp format if it's PITR
	if r.Spec.Source.PointInTime.Timestamp != nil {
		_, err := ParseDisplayTime(*r.Spec.Source.PointInTime.Timestamp)
		if err != nil {
			allErrs = append(allErrs,
				field.Forbidden(field.NewPath("spec").Child("source").Child("pointInTime").Child("timestamp"), "invalid timestamp format"))
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabaseRestore"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabaseRestore) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	autonomousdatabaserestorelog.Info("validate update", "name", r.Name)

	var allErrs field.ErrorList

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousDatabaseRestore"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousDatabaseRestore) ValidateDelete() (admission.Warnings, error) {
	autonomousdatabaserestorelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
