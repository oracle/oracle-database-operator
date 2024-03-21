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
var shardingdatabaselog = logf.Log.WithName("shardingdatabase-resource")

func (r *ShardingDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-shardingdatabase,mutating=true,failurePolicy=fail,groups=database.oracle.com,resources=shardingdatabases,verbs=create;update,versions=v1alpha1,name=mshardingdatabase.kb.io

var _ webhook.Defaulter = &ShardingDatabase{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *ShardingDatabase) Default() {
	shardingdatabaselog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	if r.Spec.GsmDevMode != "" {
		r.Spec.GsmDevMode = "dev"
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v1alpha1-shardingdatabase,mutating=false,failurePolicy=fail,groups=database.oracle.com,resources=shardingdatabases,versions=v1alpha1,name=vshardingdatabase.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &ShardingDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateCreate() (admission.Warnings, error) {
	shardingdatabaselog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	// Check Secret configuration
	var validationErr field.ErrorList

	//namespaces := db.GetWatchNamespaces()
	//_, containsNamespace := namespaces[r.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	//	if len(namespaces) != 0 && !containsNamespace {
	//	validationErr = append(validationErr,
	//		field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
	//			"Oracle database operator doesn't watch over this namespace"))
	//}

	if r.Spec.DbSecret == nil {
		validationErr = append(validationErr,
			field.Invalid(field.NewPath("spec").Child("DbSecret"), r.Spec.DbSecret,
				"DbSecret cannot be set to nil"))
	} else {
		if len(r.Spec.DbSecret.Name) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("Name"), r.Spec.DbSecret.Name,
					"Secret name cannot be set empty"))
		}
		if len(r.Spec.DbSecret.PwdFileName) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileName"), r.Spec.DbSecret.PwdFileName,
					"Password file name cannot be set empty"))
		}
		if strings.ToLower(r.Spec.DbSecret.EncryptionType) != "base64" {
			if strings.ToLower(r.Spec.DbSecret.KeyFileName) == "" {
				validationErr = append(validationErr,
					field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileName"), r.Spec.DbSecret.KeyFileName,
						"Key file name cannot be empty"))
			}
		}
		if len(r.Spec.DbSecret.PwdFileMountLocation) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("PwdFileMountLocation"), r.Spec.DbSecret.PwdFileMountLocation,
					"Password file mount location cannot be empty"))
		}

		if len(r.Spec.DbSecret.KeyFileMountLocation) == 0 {
			validationErr = append(validationErr,
				field.Invalid(field.NewPath("spec").Child("DbSecret").Child("KeyFileMountLocation"), r.Spec.DbSecret.KeyFileMountLocation,
					"KeyFileMountLocation file mount location cannot be empty"))
		}
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "ShardingDatabase"},
		r.Name, validationErr)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	shardingdatabaselog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ShardingDatabase) ValidateDelete() (admission.Warnings, error) {
	shardingdatabaselog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
