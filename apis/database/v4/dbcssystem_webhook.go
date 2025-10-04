/*
** Copyright (c) 2022-2024 Oracle and/or its affiliates.
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
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var dbcssystemlog = logf.Log.WithName("dbcssystem-resource")

// SetupWebhookWithManager registers the webhook with the manager.
func (r *DbcsSystem) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&DbcsSystem{}).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

// Ensure our CRD type implements the webhook interfaces
var _ webhook.CustomValidator = &DbcsSystem{}
var _ webhook.CustomDefaulter = &DbcsSystem{}

// +kubebuilder:webhook:path=/mutate-database-oracle-com-v4-dbcssystem,mutating=true,failurePolicy=fail,sideEffects=none,groups=database.oracle.com,resources=dbcssystems,verbs=create;update,versions=v4,name=mdbcssystemv4.kb.io,admissionReviewVersions=v1

// Default implements webhook.CustomDefaulter
func (r *DbcsSystem) Default(ctx context.Context, obj runtime.Object) error {
	cr, ok := obj.(*DbcsSystem)
	if !ok {
		return fmt.Errorf("expected *DbcsSystem but got %T", obj)
	}

	dbcssystemlog.Info("default", "name", cr.Name)

	// TODO: add your defaulting logic here
	return nil
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-database-oracle-com-v4-dbcssystem,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dbcssystems,versions=v4,name=vdbcssystemv4.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.CustomValidator
func (r *DbcsSystem) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate create")

	cr, ok := obj.(*DbcsSystem)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("expected *DbcsSystem but got %T", obj))
	}

	blockedStates := map[string]bool{
		"PROVISIONING": true,
		"UPDATING":     true,
		"TERMINATING":  true,
	}

	if blockedStates[string(cr.Status.State)] {
		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "database.oracle.com", Resource: "DbcsSystem"},
			cr.Name,
			fmt.Errorf("creation not allowed while resource is in state %q", cr.Status.State),
		)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator
func (r *DbcsSystem) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate update")

	oldCr, ok1 := oldObj.(*DbcsSystem)
	newCr, ok2 := newObj.(*DbcsSystem)
	if !ok1 || !ok2 {
		return nil, apierrors.NewInternalError(fmt.Errorf("expected *DbcsSystem but got %T/%T", oldObj, newObj))
	}

	blockedStates := map[string]bool{
		"UPDATING":     true,
		"PROVISIONING": true,
		"TERMINATING":  true,
	}

	if blockedStates[string(newCr.Status.State)] {
		if !reflect.DeepEqual(oldCr.Spec, newCr.Spec) {
			return nil, apierrors.NewForbidden(
				schema.GroupResource{Group: "database.oracle.com", Resource: "DbcsSystem"},
				newCr.Name,
				fmt.Errorf("spec updates not allowed while resource is in state %q", newCr.Status.State),
			)
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator
func (r *DbcsSystem) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dbcssystemlog.Info("validate delete")
	// TODO: Add delete validation if needed
	return nil, nil
}
