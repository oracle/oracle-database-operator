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
	"strings"

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
var databaseobserverlog = logf.Log.WithName("databaseobserver-resource")

const (
	AllowedExporterImage                       = "container-registry.oracle.com/database/observability-exporter"
	ErrorSpecValidationMissingConnString       = "a required field for database connection string secret is missing or does not have a value"
	ErrorSpecValidationMissingDBUser           = "a required field for database user secret is missing or does not have a value"
	ErrorSpecValidationMissingDBVaultField     = "a field for the OCI vault has a value but the other required field is missing or does not have a value"
	ErrorSpecValidationMissingOCIConfig        = "a field(s) for the OCI Config is missing or does not have a value when fields for the OCI vault has values"
	ErrorSpecValidationMissingDBPasswordSecret = "a required field for the database password secret is missing or does not have a value"
	ErrorSpecExporterImageNotAllowed           = "a different exporter image was found, only official database exporter container images are currently supported"
)

func (r *DatabaseObserver) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-observability-oracle-com-v4-databaseobserver,mutating=true,sideEffects=none,failurePolicy=fail,groups=observability.oracle.com,resources=databaseobservers,verbs=create;update,versions=v4,name=mdatabaseobserver.kb.io,admissionReviewVersions=v1

var _ webhook.CustomDefaulter = &DatabaseObserver{}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type
func (r *DatabaseObserver) Default(ctx context.Context, obj runtime.Object) error {
	databaseobserverlog.Info("default", "name", r.Name)

	return nil
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-observability-oracle-com-v4-databaseobserver,mutating=false,sideEffects=none,failurePolicy=fail,groups=observability.oracle.com,resources=databaseobservers,versions=v4,name=vdatabaseobserver.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &DatabaseObserver{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	databaseobserverlog.Info("validate create", "name", r.Name)

	var e field.ErrorList
	ns := dbcommons.GetWatchNamespaces()

	// Check for namespace/cluster scope access
	if _, isDesiredNamespaceWithinScope := ns[r.Namespace]; !isDesiredNamespaceWithinScope && len(ns) > 0 {
		e = append(e,
			field.Invalid(field.NewPath("metadata").Child("namespace"), r.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	// Check required secret for db user has value
	if r.Spec.Database.DBUser.SecretName == "" {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("dbUser").Child("secret"), r.Spec.Database.DBUser.SecretName,
				ErrorSpecValidationMissingDBUser))
	}

	// Check required secret for db connection string has value
	if r.Spec.Database.DBConnectionString.SecretName == "" {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("dbConnectionString").Child("secret"), r.Spec.Database.DBConnectionString.SecretName,
				ErrorSpecValidationMissingConnString))
	}

	// The other vault field must have value if one does
	if (r.Spec.Database.DBPassword.VaultOCID != "" && r.Spec.Database.DBPassword.VaultSecretName == "") ||
		(r.Spec.Database.DBPassword.VaultSecretName != "" && r.Spec.Database.DBPassword.VaultOCID == "") {

		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("dbPassword"), r.Spec.Database.DBPassword,
				ErrorSpecValidationMissingDBVaultField))
	}

	// if vault fields have value, ociConfig must have values
	if r.Spec.Database.DBPassword.VaultOCID != "" && r.Spec.Database.DBPassword.VaultSecretName != "" &&
		(r.Spec.OCIConfig.SecretName == "" || r.Spec.OCIConfig.ConfigMapName == "") {

		e = append(e,
			field.Invalid(field.NewPath("spec").Child("ociConfig"), r.Spec.OCIConfig,
				ErrorSpecValidationMissingOCIConfig))
	}

	// If all of {DB Password Secret Name and vaultOCID+vaultSecretName} have no value, then error out
	if r.Spec.Database.DBPassword.SecretName == "" &&
		r.Spec.Database.DBPassword.VaultOCID == "" &&
		r.Spec.Database.DBPassword.VaultSecretName == "" {

		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("dbPassword").Child("secret"), r.Spec.Database.DBPassword.SecretName,
				ErrorSpecValidationMissingDBPasswordSecret))
	}

	// disallow usage of any other image than the observability-exporter
	if r.Spec.Exporter.Deployment.ExporterImage != "" && !strings.HasPrefix(r.Spec.Exporter.Deployment.ExporterImage, AllowedExporterImage) {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("exporter").Child("image"), r.Spec.Exporter.Deployment.ExporterImage,
				ErrorSpecExporterImageNotAllowed))
	}

	// Return if any errors
	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, r.Name, e)
	}
	return nil, nil

}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	databaseobserverlog.Info("validate update", "name", r.Name)
	var e field.ErrorList

	// disallow usage of any other image than the observability-exporter
	if r.Spec.Exporter.Deployment.ExporterImage != "" && !strings.HasPrefix(r.Spec.Exporter.Deployment.ExporterImage, AllowedExporterImage) {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("exporter").Child("image"), r.Spec.Exporter.Deployment.ExporterImage,
				ErrorSpecExporterImageNotAllowed))
	}
	// Return if any errors
	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, r.Name, e)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	databaseobserverlog.Info("validate delete", "name", r.Name)

	return nil, nil
}
