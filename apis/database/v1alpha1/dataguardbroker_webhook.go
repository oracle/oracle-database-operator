/*
** Copyright (c) 2023 Oracle and/or its affiliates.
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
	"fmt"
	"strconv"
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
var dataguardbrokerlog = logf.Log.WithName("dataguardbroker-resource")

func (r *DataguardBroker) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v1alpha1-dataguardbroker,mutating=true,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,verbs=create;update,versions=v1alpha1,name=mdataguardbroker.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomDefaulter = &DataguardBroker{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DataguardBroker) Default(ctx context.Context, obj runtime.Object) error {
	dg, ok := obj.(*DataguardBroker)
	if !ok {
		return apierrors.NewInternalError(fmt.Errorf("failed to cast obj object to DataguardBroker"))
	}
	dataguardbrokerlog.Info("default", "name", dg.Name)

	if dg.Spec.LoadBalancer {
		if dg.Spec.ServiceAnnotations == nil {
			dg.Spec.ServiceAnnotations = make(map[string]string)
		}
		// Annotations required for a flexible load balancer on oci
		_, ok := dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"]
		if !ok {
			dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape"] = "flexible"
		}
		_, ok = dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"]
		if !ok {
			dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-min"] = "10"
		}
		_, ok = dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"]
		if !ok {
			dg.Spec.ServiceAnnotations["service.beta.kubernetes.io/oci-load-balancer-shape-flex-max"] = "100"
		}
	}

	if dg.Spec.SetAsPrimaryDatabase != "" {
		dg.Spec.SetAsPrimaryDatabase = strings.ToUpper(dg.Spec.SetAsPrimaryDatabase)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v1alpha1-dataguardbroker,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,versions=v1alpha1,name=vdataguardbroker.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.CustomValidator = &DataguardBroker{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dg, ok := obj.(*DataguardBroker)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast obj object to DataguardBroker"))
	}
	dataguardbrokerlog.Info("validate create", "name", dg.Name)
	var allErrs field.ErrorList
	namespaces := dbcommons.GetWatchNamespaces()
	_, containsNamespace := namespaces[dg.Namespace]
	// Check if the allowed namespaces maps contains the required namespace
	if len(namespaces) != 0 && !containsNamespace {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("metadata").Child("namespace"), dg.Namespace,
				"Oracle database operator doesn't watch over this namespace"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "Dataguard"},
		dg.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateUpdate(ctx context.Context, old, newObj runtime.Object) (admission.Warnings, error) {
	new, ok := newObj.(*DataguardBroker)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast newObj object to DataguardBroker"))
	}

	dataguardbrokerlog.Info("validate update", "name", new.Name)
	var allErrs field.ErrorList

	// check creation validations first
	_, err := new.ValidateCreate(ctx, newObj)
	if err != nil {
		return nil, err
	}

	// Validate Deletion
	if new.GetDeletionTimestamp() != nil {
		warnings, err := new.ValidateDelete(ctx, newObj)
		if err != nil {
			return warnings, err
		}
	}

	// Now check for updation errors
	oldObj, okay := old.(*DataguardBroker)
	if !okay {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to cast old object to DataguardBroker"))
	}

	if oldObj.Status.ProtectionMode != "" && !strings.EqualFold(new.Spec.ProtectionMode, oldObj.Status.ProtectionMode) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("protectionMode"), "cannot be changed"))
	}
	if oldObj.Status.PrimaryDatabaseRef != "" && !strings.EqualFold(oldObj.Status.PrimaryDatabaseRef, new.Spec.PrimaryDatabaseRef) {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("primaryDatabaseRef"), "cannot be changed"))
	}
	fastStartFailoverStatus, _ := strconv.ParseBool(oldObj.Status.FastStartFailover)
	if (fastStartFailoverStatus || new.Spec.FastStartFailover) && new.Spec.SetAsPrimaryDatabase != "" {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec").Child("setAsPrimaryDatabase"), "switchover not supported when fastStartFailover is true"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "DataguardBroker"},
		new.Name, allErrs)

}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DataguardBroker) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dataguardbrokerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
