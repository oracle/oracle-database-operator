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
	"context"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var databaseobserverlog = logf.Log.WithName("databaseobserver-resource")

const (
	// AllowedExporterImage is the approved base image for exporter deployments.
	AllowedExporterImage = "container-registry.oracle.com/database/observability-exporter"
	// ErrorSpecValidationMissingConnString indicates required DB connection-string secret config is missing.
	ErrorSpecValidationMissingConnString = "a required field for database connection string secret is missing or does not have a value"
	// ErrorSpecValidationMissingDBUser indicates required DB user secret config is missing.
	ErrorSpecValidationMissingDBUser = "a required field for database user secret is missing or does not have a value"
	// ErrorSpecValidationMissingVaultField indicates incomplete vault field combinations in spec.
	ErrorSpecValidationMissingVaultField = "a field for configuring the vault has a value but the other required field(s) is missing or does not have a value"
	// ErrorSpecValidationMissingOCIConfig indicates OCI config values are missing when OCI vault is used.
	ErrorSpecValidationMissingOCIConfig = "a field(s) for the OCI Config is missing or does not have a value when fields for the OCI vault has values"
	// ErrorSpecValidationMissingDBPasswordSecret indicates required DB password secret config is missing.
	ErrorSpecValidationMissingDBPasswordSecret = "a required field for the database password secret is missing or does not have a value"
	// ErrorSpecExporterImageNotAllowed indicates a non-approved exporter image was specified.
	ErrorSpecExporterImageNotAllowed = "a different exporter image was found, only official database exporter container images are currently supported"
)

// SetupWebhookWithManager registers mutating and validating webhooks for DatabaseObserver.
func (r *DatabaseObserver) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// 1. Use the generic builder with [*DatabaseObserver]
	// 2. Pass both 'mgr' and 'r' to the constructor
	return ctrl.NewWebhookManagedBy[*DatabaseObserver](mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-observability-oracle-com-v1alpha1-databaseobserver,mutating=true,sideEffects=none,failurePolicy=fail,groups=observability.oracle.com,resources=databaseobservers,verbs=create;update,versions=v1alpha1,name=mdatabaseobserver.kb.io,admissionReviewVersions=v1

// 3. Update guards to generic admission package
var _ admission.Defaulter[*DatabaseObserver] = &DatabaseObserver{}
var _ admission.Validator[*DatabaseObserver] = &DatabaseObserver{}

// Default implements admission.Defaulter[*DatabaseObserver]
func (r *DatabaseObserver) Default(_ context.Context, obj *DatabaseObserver) error {
	// Casting is no longer required. Use 'obj' directly.
	databaseobserverlog.Info("default", "name", obj.Name)

	return nil
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-observability-oracle-com-v1alpha1-databaseobserver,mutating=false,sideEffects=none,failurePolicy=fail,groups=observability.oracle.com,resources=databaseobservers,versions=v1alpha1,name=vdatabaseobserver.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.Validator[*DatabaseObserver]
func (r *DatabaseObserver) ValidateCreate(_ context.Context, obj *DatabaseObserver) (admission.Warnings, error) {
	databaseobserverlog.Info("validate create", "name", obj.Name)

	var e field.ErrorList
	ns := dbcommons.GetWatchNamespaces()

	// Check for namespace scope access
	if _, isDesiredNamespaceWithinScope := ns[obj.Namespace]; !isDesiredNamespaceWithinScope && len(ns) > 0 {
		e = append(e,
			field.Invalid(field.NewPath("metadata").Child("namespace"), obj.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	// OCI Vault validation
	if (obj.Spec.Database.OCIVault.VaultID != "" && obj.Spec.Database.OCIVault.VaultPasswordSecret == "") ||
		(obj.Spec.Database.OCIVault.VaultPasswordSecret != "" && obj.Spec.Database.OCIVault.VaultID == "") {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("oci"), obj.Spec.Database.OCIVault,
				ErrorSpecValidationMissingVaultField))
	}

	// Azure Vault validation
	if (obj.Spec.Database.AzureVault.VaultID != "" && (obj.Spec.Database.AzureVault.VaultPasswordSecret == "" && obj.Spec.Database.AzureVault.VaultUsernameSecret == "")) ||
		(obj.Spec.Database.AzureVault.VaultPasswordSecret != "" && obj.Spec.Database.AzureVault.VaultID == "") ||
		(obj.Spec.Database.AzureVault.VaultUsernameSecret != "" && obj.Spec.Database.AzureVault.VaultID == "") {
		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("azure"), obj.Spec.Database.AzureVault,
				ErrorSpecValidationMissingVaultField))
	}

	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, obj.Name, e)
	}
	return nil, nil
}

// ValidateUpdate implements admission.Validator[*DatabaseObserver]
func (r *DatabaseObserver) ValidateUpdate(_ context.Context, _ *DatabaseObserver, newObj *DatabaseObserver) (admission.Warnings, error) {
	databaseobserverlog.Info("validate update", "name", newObj.Name)
	var e field.ErrorList

	// Re-run creation validations if necessary or add update-specific logic here

	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, newObj.Name, e)
	}
	return nil, nil
}

// ValidateDelete implements admission.Validator[*DatabaseObserver]
func (r *DatabaseObserver) ValidateDelete(_ context.Context, obj *DatabaseObserver) (admission.Warnings, error) {
	databaseobserverlog.Info("validate delete", "name", obj.Name)
	return nil, nil
}
