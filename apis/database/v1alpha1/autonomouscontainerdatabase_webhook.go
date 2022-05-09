/*
** Copyright (c) 2021 Oracle and/or its affiliates.
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var autonomouscontainerdatabaselog = logf.Log.WithName("autonomouscontainerdatabase-resource")

func (r *AutonomousContainerDatabase) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v1alpha1-autonomouscontainerdatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=autonomouscontainerdatabases,versions=v1alpha1,name=vautonomouscontainerdatabase.kb.io,admissionReviewVersions={v1}

var _ webhook.Validator = &AutonomousContainerDatabase{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousContainerDatabase) ValidateCreate() error {
	autonomouscontainerdatabaselog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousContainerDatabase) ValidateUpdate(old runtime.Object) error {
	var allErrs field.ErrorList
	var oldACD *AutonomousContainerDatabase = old.(*AutonomousContainerDatabase)

	autonomouscontainerdatabaselog.Info("validate update", "name", r.Name)

	// skip the update of adding ADB OCID or binding
	if oldACD.Status.LifecycleState == "" {
		return nil
	}

	// cannot update when the old state is in intermediate state, except for the terminate operatrion
	var copiedSpec *AutonomousContainerDatabaseSpec = r.Spec.DeepCopy()
	changed, err := removeUnchangedFields(oldACD.Spec, copiedSpec)
	if err != nil {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec"), err.Error()))
	}
	if IsACDIntermediateState(oldACD.Status.LifecycleState) && changed {
		allErrs = append(allErrs,
			field.Forbidden(field.NewPath("spec"),
				"cannot change the spec when the lifecycleState is in an intermdeiate state"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "database.oracle.com", Kind: "AutonomousContainerDatabase"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AutonomousContainerDatabase) ValidateDelete() error {
	autonomouscontainerdatabaselog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
