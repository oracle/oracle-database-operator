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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager registers the DataguardBroker webhook with the manager.
func (r *DataguardBroker) SetupWebhookWithManager(mgr ctrl.Manager) error {

	return ctrl.NewWebhookManagedBy[*DataguardBroker](mgr, r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-database-oracle-com-v4-dataguardbroker,mutating=false,failurePolicy=fail,sideEffects=None,groups=database.oracle.com,resources=dataguardbrokers,versions=v4,name=vdataguardbrokerv4.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*DataguardBroker] = &DataguardBroker{}

// ValidateCreate validates DataguardBroker create requests.
func (r *DataguardBroker) ValidateCreate(ctx context.Context, obj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	_ = obj
	return nil, nil
}

// ValidateUpdate validates DataguardBroker update requests.
func (r *DataguardBroker) ValidateUpdate(ctx context.Context, oldObj, newObj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	_ = oldObj
	_ = newObj
	return nil, nil
}

// ValidateDelete validates DataguardBroker delete requests.
func (r *DataguardBroker) ValidateDelete(ctx context.Context, obj *DataguardBroker) (admission.Warnings, error) {
	_ = ctx
	_ = obj
	return nil, nil
}
