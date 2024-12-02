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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var dbcssystemlog = logf.Log.WithName("dbcssystem-resource")

func (r *DbcsSystem) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-database-oracle-com-v4-dbcssystem,mutating=true,failurePolicy=fail,sideEffects=none,groups=database.oracle.com,resources=dbcssystems,verbs=create;update,versions=v4,name=mdbcssystem.kb.io,admissionReviewVersions={v1}

var _ webhook.Defaulter = &DbcsSystem{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DbcsSystem) Default() {
	dbcssystemlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-dbcssystem,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dbcssystems,versions=v4,name=vdbcssystem.kb.io,admissionReviewVersions={v1}
var _ webhook.Validator = &DbcsSystem{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DbcsSystem) ValidateCreate() (admission.Warnings, error) {
	dbcssystemlog.Info("validate create", "name", r.Name)

	// 	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// // ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DbcsSystem) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate update", "name", r.Name)

	// 	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DbcsSystem) ValidateDelete() (admission.Warnings, error) {
	dbcssystemlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
