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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var oraclerestdataservicelog = logf.Log.WithName("oraclerestdataservice-resource")

func (r *OracleRestDataService) SetupWebhookWithManager(mgr ctrl.Manager) error {

	return ctrl.NewWebhookManagedBy[*OracleRestDataService](mgr, r).
		WithDefaulter(r).
		WithValidator(r).
		Complete()
}

// Ensure the CRD implements the generic admission interfaces
var _ admission.Defaulter[*OracleRestDataService] = &OracleRestDataService{}
var _ admission.Validator[*OracleRestDataService] = &OracleRestDataService{}

// Default implements admission.Defaulter[*OracleRestDataService]
func (r *OracleRestDataService) Default(ctx context.Context, obj *OracleRestDataService) error {
	// Access fields directly via 'obj'
	oraclerestdataservicelog.Info("Defaulting for OracleRestDataService", "name", obj.Name)
	return nil
}

// ValidateCreate implements admission.Validator[*OracleRestDataService]
func (r *OracleRestDataService) ValidateCreate(ctx context.Context, obj *OracleRestDataService) (admission.Warnings, error) {
	oraclerestdataservicelog.Info("ValidateCreate for OracleRestDataService", "name", obj.Name)
	return nil, nil
}

// ValidateUpdate implements admission.Validator[*OracleRestDataService]
func (r *OracleRestDataService) ValidateUpdate(ctx context.Context, oldObj, newObj *OracleRestDataService) (admission.Warnings, error) {
	oraclerestdataservicelog.Info("ValidateUpdate for OracleRestDataService", "name", newObj.Name)
	return nil, nil
}

// ValidateDelete implements admission.Validator[*OracleRestDataService]
func (r *OracleRestDataService) ValidateDelete(ctx context.Context, obj *OracleRestDataService) (admission.Warnings, error) {
	return nil, nil
}
