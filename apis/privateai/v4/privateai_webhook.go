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
	"path/filepath"
	"strconv"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var privateailog = logf.Log.WithName("privateai-resource")

// SetupPrivateAiWebhookWithManager registers the webhook for PrivateAi in the manager.
func (r *PrivateAi) SetupPrivateAiWebhookWithManager(mgr ctrl.Manager) error {
	// 1. Add generic type parameter [*PrivateAi] and pass (mgr, r)
	return ctrl.NewWebhookManagedBy[*PrivateAi](mgr, r).
		WithValidator(&PrivateAi{}).
		WithDefaulter(&PrivateAi{}).
		Complete()
}

// 2. Ensure your CustomValidator and CustomDefaulter implementations use admission interfaces
var _ admission.Validator[*PrivateAi] = &PrivateAi{}
var _ admission.Defaulter[*PrivateAi] = &PrivateAi{}

// +kubebuilder:webhook:path=/mutate-privateai-oracle-com-v4-privateai,mutating=true,failurePolicy=fail,sideEffects=None,groups=privateai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=mprivateai-v4.kb.io,admissionReviewVersions=v1

// PrivateAiCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind PrivateAi when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind PrivateAi.
func (r *PrivateAi) Default(_ context.Context, obj *PrivateAi) error {
	privateai := obj

	privateailog.Info("Defaulting for PrivateAi", "name", privateai.GetName())

	// TODO(user): fill in your defaulting logic.

	pHtpStatus, _ := strconv.ParseBool(privateai.Spec.PaiHTTPEnabled)
	pHtpsStatus, _ := strconv.ParseBool(privateai.Spec.PaiHTTPSEnabled)
	if pHtpStatus && !pHtpsStatus {
		privateai.Spec.PaiHTTPSEnabled = "true"
	}
	if pHtpStatus && pHtpsStatus {
		return fmt.Errorf("paiHTTPEnabled and PaiHTTPSEnabled both cannot be true")
	}
	if pHtpStatus {
		if privateai.Spec.PaiHTTPPort == 0 {
			privateai.Spec.PaiHTTPPort = 8080
		}
		privateai.Spec.PaiHTTPSEnabled = "false"
		privateai.Spec.PaiHTTPSPort = 0

	} else {
		privateai.Spec.PaiHTTPSEnabled = "true"
		if privateai.Spec.PaiHTTPSPort == 0 {
			privateai.Spec.PaiHTTPSPort = 8443
		}
		privateai.Spec.PaiHTTPEnabled = "false"
		privateai.Spec.PaiHTTPPort = 0
	}

	if len(privateai.Spec.PaiService.PortMappings) == 0 {
		portInfo := PaiPortMapping{}

		portInfo.Port = 443

		if pHtpStatus {
			portInfo.TargetPort = privateai.Spec.PaiHTTPPort
		} else {
			portInfo.TargetPort = privateai.Spec.PaiHTTPSPort
		}
		portInfo.Protocol = "TCP"

		privateai.Spec.PaiService.PortMappings = append(privateai.Spec.PaiService.PortMappings, portInfo)
	}
	// set default MountLocation for PaiConfigFile
	if privateai.Spec.PaiConfigFile != nil {
		if privateai.Spec.PaiConfigFile.MountLocation == "" {
			privateai.Spec.PaiConfigFile.MountLocation = "/privateai/config"
		}
	}

	// set default MountLocation for PaiSecret
	if privateai.Spec.PaiSecret != nil {
		if privateai.Spec.PaiSecret.MountLocation == "" {
			privateai.Spec.PaiSecret.MountLocation = "/privateai/ssl"
		}
	}

	if privateai.Spec.Gateway != nil {
		if strings.TrimSpace(privateai.Spec.Gateway.Type) == "" {
			privateai.Spec.Gateway.Type = "nginx"
		}
		if privateai.Spec.Gateway.ContainerPort == 0 {
			privateai.Spec.Gateway.ContainerPort = 8080
		}
		if privateai.Spec.Gateway.ConfigFileKey == "" {
			privateai.Spec.Gateway.ConfigFileKey = "nginx.conf"
		}
		if privateai.Spec.Gateway.ConfigMap.MountLocation == "" && strings.EqualFold(privateai.Spec.Gateway.Type, "nginx") {
			privateai.Spec.Gateway.ConfigMap.MountLocation = "/etc/nginx/nginx.conf"
		}
		if strings.TrimSpace(privateai.Spec.Gateway.TLSSecretName) != "" &&
			strings.TrimSpace(privateai.Spec.Gateway.TLSMountLocation) == "" &&
			strings.EqualFold(privateai.Spec.Gateway.Type, "nginx") {
			privateai.Spec.Gateway.TLSMountLocation = "/etc/nginx/tls"
		}
	}

	return nil
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-privateai-oracle-com-v4-privateai,mutating=false,failurePolicy=fail,sideEffects=None,groups=privateai.oracle.com,resources=privateais,verbs=create;update,versions=v4,name=vprivateai-v4.kb.io,admissionReviewVersions=v1

// PrivateAiCustomValidator is retained for generated deepcopy compatibility.
// Webhook behavior now uses *PrivateAi receiver methods directly.
type PrivateAiCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (r *PrivateAi) ValidateCreate(_ context.Context, obj *PrivateAi) (admission.Warnings, error) {
	privateai := obj

	privateailog.Info("Validation for PrivateAi upon creation", "name", privateai.GetName())

	pstatus, _ := strconv.ParseBool(privateai.Spec.PaiEnableAuthentication)
	if pstatus {
		if privateai.Spec.PaiSecret == nil || privateai.Spec.PaiSecret.Name == "" {
			return nil, fmt.Errorf("paiEnableAuthentication is true but paiSecret.name is empty")
		}
	}
	if privateai.Spec.Gateway != nil {
		gwType := strings.ToLower(strings.TrimSpace(privateai.Spec.Gateway.Type))
		if gwType == "" {
			gwType = "nginx"
		}
		if gwType != "nginx" && gwType != "litellm" {
			return nil, fmt.Errorf("gateway.type must be one of nginx or litellm")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.ConfigMap.Name) == "" {
			return nil, fmt.Errorf("gateway.configMap.name must be set when gateway is configured")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.ConfigMap.MountLocation) == "" {
			return nil, fmt.Errorf("gateway.configMap.mountLocation must be set when gateway is configured")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.TLSSecretName) != "" {
			if !strings.EqualFold(gwType, "nginx") {
				return nil, fmt.Errorf("gateway.tlsSecretName is currently supported only for gateway.type=nginx")
			}
			if strings.TrimSpace(privateai.Spec.Gateway.TLSMountLocation) == "" {
				return nil, fmt.Errorf("gateway.tlsMountLocation must be set when gateway.tlsSecretName is configured")
			}
			if !filepath.IsAbs(strings.TrimSpace(privateai.Spec.Gateway.TLSMountLocation)) {
				return nil, fmt.Errorf("gateway.tlsMountLocation must be an absolute path")
			}
		}
	}

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (r *PrivateAi) ValidateUpdate(_ context.Context, _ *PrivateAi, newObj *PrivateAi) (admission.Warnings, error) {
	privateai := newObj

	privateailog.Info("Validation for PrivateAi upon update", "name", privateai.GetName())
	pstatus, _ := strconv.ParseBool(privateai.Spec.PaiEnableAuthentication)
	if pstatus {
		if privateai.Spec.PaiSecret == nil || privateai.Spec.PaiSecret.Name == "" {
			return nil, fmt.Errorf("paiEnableAuthentication=true requires paiSecret.name to be set")
		}
	}
	if privateai.Spec.Gateway != nil {
		gwType := strings.ToLower(strings.TrimSpace(privateai.Spec.Gateway.Type))
		if gwType == "" {
			gwType = "nginx"
		}
		if gwType != "nginx" && gwType != "litellm" {
			return nil, fmt.Errorf("gateway.type must be one of nginx or litellm")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.ConfigMap.Name) == "" {
			return nil, fmt.Errorf("gateway.configMap.name must be set when gateway is configured")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.ConfigMap.MountLocation) == "" {
			return nil, fmt.Errorf("gateway.configMap.mountLocation must be set when gateway is configured")
		}
		if strings.TrimSpace(privateai.Spec.Gateway.TLSSecretName) != "" {
			if !strings.EqualFold(gwType, "nginx") {
				return nil, fmt.Errorf("gateway.tlsSecretName is currently supported only for gateway.type=nginx")
			}
			if strings.TrimSpace(privateai.Spec.Gateway.TLSMountLocation) == "" {
				return nil, fmt.Errorf("gateway.tlsMountLocation must be set when gateway.tlsSecretName is configured")
			}
			if !filepath.IsAbs(strings.TrimSpace(privateai.Spec.Gateway.TLSMountLocation)) {
				return nil, fmt.Errorf("gateway.tlsMountLocation must be an absolute path")
			}
		}
	}

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type PrivateAi.
func (r *PrivateAi) ValidateDelete(_ context.Context, obj *PrivateAi) (admission.Warnings, error) {
	privateai := obj

	privateailog.Info("Validation for PrivateAi upon deletion", "name", privateai.GetName())

	return nil, nil
}
