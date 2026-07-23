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
	// ErrorSpecServiceMonitorFieldNotAllowed indicates an unsupported ServiceMonitor field was specified.
	ErrorSpecServiceMonitorFieldNotAllowed = "unsupported ServiceMonitor field in DatabaseObserver"
)

// validateServiceMonitorAllowlist enforces the DatabaseObserver ServiceMonitor policy.
// Only fields listed here are accepted from DatabaseObserver and projected to the child ServiceMonitor.
func validateServiceMonitorAllowlist(sm ExporterServiceMonitor, fldPath *field.Path) field.ErrorList {
	var e field.ErrorList

	allowedServiceMonitorFields := map[string]struct{}{
		"Labels":    {},
		"Endpoints": {},
	}
	e = append(e, validateAllowlistedFields(sm, fldPath, allowedServiceMonitorFields)...)

	allowedEndpointFields := map[string]struct{}{
		"Port":                 {},
		"Path":                 {},
		"Scheme":               {},
		"Params":               {},
		"Interval":             {},
		"ScrapeTimeout":        {},
		"RelabelConfigs":       {},
		"MetricRelabelConfigs": {},
	}
	endpointsPath := fldPath.Child("endpoints")
	for i, endpoint := range sm.Endpoints {
		e = append(e, validateAllowlistedFields(endpoint, endpointsPath.Index(i), allowedEndpointFields)...)
	}

	return e
}

// validateAllowlistedFields rejects any non-zero exported struct field that is not explicitly allowed.
// This keeps future fields denied by default until they are intentionally added to the allowlist.
func validateAllowlistedFields(obj interface{}, fldPath *field.Path, allowedFields map[string]struct{}) field.ErrorList {
	var e field.ErrorList

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		structField := typ.Field(i)
		if structField.PkgPath != "" {
			continue
		}
		if val.Field(i).IsZero() {
			continue
		}
		if _, allowed := allowedFields[structField.Name]; allowed {
			continue
		}

		e = append(e, field.Forbidden(fldPath.Child(structField.Name), ErrorSpecServiceMonitorFieldNotAllowed))
	}

	return e
}

// SetupWebhookWithManager sets up the webhook with the manager.
func (r *DatabaseObserver) SetupWebhookWithManager(mgr ctrl.Manager) error {
	// 1. Add the generic type parameter [*DatabaseObserver] and pass (mgr, r)
	return ctrl.NewWebhookManagedBy[*DatabaseObserver](mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-observability-oracle-com-v4-databaseobserver,mutating=true,sideEffects=none,failurePolicy=fail,matchPolicy=Exact,groups=observability.oracle.com,resources=databaseobservers,verbs=create;update,versions=v4,name=mdatabaseobserverv4.kb.io,admissionReviewVersions=v1

// 2. Update interface guards to use admission.CustomDefaulter and CustomValidator with generics
var _ admission.Defaulter[*DatabaseObserver] = &DatabaseObserver{}
var _ admission.Validator[*DatabaseObserver] = &DatabaseObserver{}

// 3. Update Default: change runtime.Object to *DatabaseObserver

// Default sets default values for DatabaseObserver.
func (r *DatabaseObserver) Default(_ context.Context, obj *DatabaseObserver) error {
	obs := obj
	databaseobserverlog.Info("DatabaseObserver defaulting (webhook v4)", "name", obs.Name)

	return nil
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-observability-oracle-com-v4-databaseobserver,mutating=false,sideEffects=none,failurePolicy=fail,matchPolicy=Exact,groups=observability.oracle.com,resources=databaseobservers,versions=v4,name=vdatabaseobserverv4.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateCreate(_ context.Context, obj *DatabaseObserver) (admission.Warnings, error) {
	obs := obj
	databaseobserverlog.Info("ServiceMonitor policy validation on create (webhook v4)", "name", obs.Name)

	var e field.ErrorList
	ns := dbcommons.GetWatchNamespaces()

	// Reject unsupported ServiceMonitor customization before the controller projects it.
	serviceMonitorErrs := validateServiceMonitorAllowlist(obs.Spec.ServiceMonitor, field.NewPath("spec").Child("serviceMonitor"))
	if len(serviceMonitorErrs) > 0 {
		databaseobserverlog.Info("ServiceMonitor policy validation failed (webhook v4)", "operation", "create", "name", obs.Name, "violations", len(serviceMonitorErrs))
	}
	e = append(e, serviceMonitorErrs...)

	// Check for namespace/cluster scope access
	if _, isDesiredNamespaceWithinScope := ns[obs.Namespace]; !isDesiredNamespaceWithinScope && len(ns) > 0 {
		e = append(e,
			field.Invalid(field.NewPath("metadata").Child("namespace"), obs.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	// The other vault field must have value if one does
	if (obs.Spec.Database.OCIVault.VaultID != "" && obs.Spec.Database.OCIVault.VaultPasswordSecret == "") ||
		(obs.Spec.Database.OCIVault.VaultPasswordSecret != "" && obs.Spec.Database.OCIVault.VaultID == "") {

		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("oci"), obs.Spec.Database.OCIVault,
				ErrorSpecValidationMissingVaultField))
	}

	// The other vault field must have value if one does
	if (obs.Spec.Database.AzureVault.VaultID != "" && (obs.Spec.Database.AzureVault.VaultPasswordSecret == "" && obs.Spec.Database.AzureVault.VaultUsernameSecret == "")) ||
		(obs.Spec.Database.AzureVault.VaultPasswordSecret != "" && obs.Spec.Database.AzureVault.VaultID == "") ||
		(obs.Spec.Database.AzureVault.VaultUsernameSecret != "" && obs.Spec.Database.AzureVault.VaultID == "") {

		e = append(e,
			field.Invalid(field.NewPath("spec").Child("database").Child("azure"), obs.Spec.Database.AzureVault,
				ErrorSpecValidationMissingVaultField))
	}

	// disallow usage of any other image than the observability-exporter
	// temporarily disabled
	//if r.Spec.Deployment.ExporterImage != "" && !strings.HasPrefix(r.Spec.Deployment.ExporterImage, AllowedExporterImage) {
	//	e = append(e,
	//		field.Invalid(field.NewPath("spec").Child("exporter").Child("image"), r.Spec.Deployment.ExporterImage,
	//			ErrorSpecExporterImageNotAllowed))
	//}

	// Return if any errors
	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, obs.Name, e)
	}
	return nil, nil

}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateUpdate(_ context.Context, _ *DatabaseObserver, newObj *DatabaseObserver) (admission.Warnings, error) {
	obs := newObj
	databaseobserverlog.Info("ServiceMonitor policy validation on update (webhook v4)", "name", obs.Name)
	var e field.ErrorList

	// Reject unsupported ServiceMonitor customization before the controller projects it.
	serviceMonitorErrs := validateServiceMonitorAllowlist(obs.Spec.ServiceMonitor, field.NewPath("spec").Child("serviceMonitor"))
	if len(serviceMonitorErrs) > 0 {
		databaseobserverlog.Info("ServiceMonitor policy validation failed (webhook v4)", "operation", "update", "name", obs.Name, "violations", len(serviceMonitorErrs))
	}
	e = append(e, serviceMonitorErrs...)

	// disallow usage of any other image than the observability-exporter
	//if r.Spec.Deployment.ExporterImage != "" && !strings.HasPrefix(obs.Spec.Deployment.ExporterImage, AllowedExporterImage) {
	//	e = append(e,
	//		field.Invalid(field.NewPath("spec").Child("exporter").Child("image"), obs.Spec.Deployment.ExporterImage,
	//			ErrorSpecExporterImageNotAllowed))
	//}
	// Return if any errors
	if len(e) > 0 {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: "observability.oracle.com", Kind: "DatabaseObserver"}, obs.Name, e)
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *DatabaseObserver) ValidateDelete(_ context.Context, obj *DatabaseObserver) (admission.Warnings, error) {
	obs := obj
	databaseobserverlog.Info("DatabaseObserver validate delete (webhook v4)", "name", obs.Name)

	return nil, nil
}
