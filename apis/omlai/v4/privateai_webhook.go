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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var privateailog = logf.Log.WithName("privateai-resource")

// SetupPrivateAiWebhookWithManager registers the webhook for PrivateAi in the manager.
func SetupPrivateAiWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&PrivateAi{}).
		WithValidator(&PrivateAiCustomValidator{}).
		WithDefaulter(&PrivateAiCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-omlai-oracle-com-v4-privateai,mutating=true,failurePolicy=fail,sideEffects=None,groups=omlai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=mprivateai-v4.kb.io,admissionReviewVersions=v1

// PrivateAiCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind PrivateAi when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type PrivateAiCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &PrivateAiCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind PrivateAi.
func (d *PrivateAiCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	privateai, ok := obj.(*PrivateAi)

	if !ok {
		return fmt.Errorf("expected an PrivateAi object but got %T", obj)
	}
	privateailog.Info("Defaulting for PrivateAi", "name", privateai.GetName())

	// TODO(user): fill in your defaulting logic.

	if !privateai.Spec.PaiHTTPEnabled && !privateai.Spec.PaiHTTPSEnabled {
		privateai.Spec.PaiHTTPSEnabled = true
	}
	if privateai.Spec.PaiHTTPEnabled && privateai.Spec.PaiHTTPSEnabled {
		return fmt.Errorf("paiHTTPEnabled and PaiHTTPSEnabled cannot be true")
	}
	if privateai.Spec.PaiHTTPEnabled {
		if privateai.Spec.PaiHTTPPort == 0 {
			privateai.Spec.PaiHTTPPort = 8080
		}
		privateai.Spec.PaiHTTPSEnabled = false
		privateai.Spec.PaiHTTPSPort = 0

	} else {
		privateai.Spec.PaiHTTPSEnabled = true
		if privateai.Spec.PaiHTTPSPort == 0 {
			privateai.Spec.PaiHTTPSPort = 8443
		}
		privateai.Spec.PaiHTTPEnabled = false
		privateai.Spec.PaiHTTPPort = 0
	}

	if privateai.Spec.PaiAuthentication {
		if privateai.Spec.PaiSecret.Name == "" {
			return fmt.Errorf("PaiAuthentication is ture but paisecret is empty")
		}
	}

	if len(privateai.Spec.PaiService.PortMappings) == 0 {
		portInfo := PaiPortMapping{}

		portInfo.Port = 443

		if privateai.Spec.PaiHTTPEnabled {
			portInfo.TargetPort = privateai.Spec.PaiHTTPPort
		} else {
			portInfo.TargetPort = privateai.Spec.PaiHTTPSPort
		}
		portInfo.Protocol = "TCP"

		privateai.Spec.PaiService.PortMappings = append(privateai.Spec.PaiService.PortMappings, portInfo)
	}
	// set default MountLocation for PaiConfigFile
	if privateai.Spec.PaiConfigFile.MountLocation == "" {
		privateai.Spec.PaiConfigFile.MountLocation = "/oml/config"
	}
	// set default MountLocation for PaiSecret
	if privateai.Spec.PaiSecret.MountLocation == "" {
		privateai.Spec.PaiSecret.MountLocation = "/oml/ssl"
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-omlai-oracle-com-v4-privateai,mutating=false,failurePolicy=fail,sideEffects=None,groups=omlai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=vprivateai-v4.kb.io,admissionReviewVersions=v1

// PrivateAiCustomValidator struct is responsible for validating the PrivateAi resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type PrivateAiCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &PrivateAiCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (v *PrivateAiCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	privateai, ok := obj.(*PrivateAi)
	if !ok {
		return nil, fmt.Errorf("expected a PrivateAi object but got %T", obj)
	}
	privateailog.Info("Validation for PrivateAi upon creation", "name", privateai.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (v *PrivateAiCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	privateai, ok := newObj.(*PrivateAi)
	if !ok {
		return nil, fmt.Errorf("expected a PrivateAi object for the newObj but got %T", newObj)
	}
	privateailog.Info("Validation for PrivateAi upon update", "name", privateai.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (v *PrivateAiCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	privateai, ok := obj.(*PrivateAi)
	if !ok {
		return nil, fmt.Errorf("expected a PrivateAi object but got %T", obj)
	}
	privateailog.Info("Validation for PrivateAi upon deletion", "name", privateai.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
