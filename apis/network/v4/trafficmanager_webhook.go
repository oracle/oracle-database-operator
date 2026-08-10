package v4

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *TrafficManager) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy[*TrafficManager](mgr, r).
		WithValidator(&TrafficManager{}).
		WithDefaulter(&TrafficManager{}).
		Complete()
}

var _ admission.Defaulter[*TrafficManager] = &TrafficManager{}
var _ admission.Validator[*TrafficManager] = &TrafficManager{}

// +kubebuilder:webhook:path=/mutate-network-oracle-com-v4-trafficmanager,mutating=true,failurePolicy=fail,sideEffects=None,groups=network.oracle.com,resources=trafficmanagers,verbs=create;update,versions=v4,name=mtrafficmanager-v4.kb.io,admissionReviewVersions=v1
func (r *TrafficManager) Default(_ context.Context, obj *TrafficManager) error {
	if obj.Spec.Type == "" {
		obj.Spec.Type = TrafficManagerTypeNginx
	}
	if obj.Spec.Runtime.Replicas <= 0 {
		obj.Spec.Runtime.Replicas = 1
	}
	if obj.Spec.Type == TrafficManagerTypeNginx {
		if obj.Spec.Nginx == nil {
			obj.Spec.Nginx = &NginxTrafficManagerSpec{}
		}
		if obj.Spec.Nginx.Config == nil {
			obj.Spec.Nginx.Config = &TrafficManagerConfigSpec{}
		}
		if strings.TrimSpace(obj.Spec.Nginx.Config.ConfigMapName) == "" {
			obj.Spec.Nginx.Config.ConfigMapName = obj.Name + "-nginx"
		}
		if strings.TrimSpace(obj.Spec.Nginx.Config.MountLocation) == "" {
			obj.Spec.Nginx.Config.MountLocation = "/etc/nginx"
		}
		if obj.Spec.Service.Internal.Enabled == nil {
			enable := true
			obj.Spec.Service.Internal.Enabled = &enable
		}
		containerPort := trafficManagerContainerPort(obj)
		if obj.Spec.Service.Internal.Port == 0 {
			obj.Spec.Service.Internal.Port = containerPort
		}
		if obj.Spec.Service.Internal.TargetPort == 0 {
			obj.Spec.Service.Internal.TargetPort = containerPort
		}
		if obj.Spec.Service.External.Port == 0 {
			if obj.Spec.Security.TLS.Enabled {
				obj.Spec.Service.External.Port = 443
			} else {
				obj.Spec.Service.External.Port = 80
			}
		}
		if obj.Spec.Service.External.TargetPort == 0 {
			obj.Spec.Service.External.TargetPort = containerPort
		}
		if obj.Spec.Security.TLS.Enabled && strings.TrimSpace(obj.Spec.Security.TLS.MountLocation) == "" {
			obj.Spec.Security.TLS.MountLocation = "/etc/nginx/tls"
		}
		if obj.Spec.Security.BackendTLS != nil {
			if strings.TrimSpace(obj.Spec.Security.BackendTLS.MountLocation) == "" {
				obj.Spec.Security.BackendTLS.MountLocation = "/etc/nginx/backend-ca"
			}
			if strings.TrimSpace(obj.Spec.Security.BackendTLS.TrustFileName) == "" {
				obj.Spec.Security.BackendTLS.TrustFileName = "ca.crt"
			}
		}
	}
	if obj.Spec.Type == TrafficManagerTypeCman {
		if obj.Spec.Cman == nil {
			obj.Spec.Cman = &CmanTrafficManagerSpec{}
		}
		if obj.Spec.Cman.ConfigSource == nil {
			if strings.TrimSpace(obj.Spec.Cman.LogLevel) == "" {
				obj.Spec.Cman.LogLevel = "user"
			}
			if strings.TrimSpace(obj.Spec.Cman.TraceLevel) == "" {
				obj.Spec.Cman.TraceLevel = "user"
			}
			if strings.TrimSpace(obj.Spec.Cman.RegistrationInvitedNodes) == "" {
				obj.Spec.Cman.RegistrationInvitedNodes = "*"
			}
			if obj.Spec.Cman.RestAPI != nil && obj.Spec.Cman.RestAPI.Enabled {
				if obj.Spec.Cman.RestAPI.Port == 0 {
					obj.Spec.Cman.RestAPI.Port = 1525
				}
				if strings.TrimSpace(obj.Spec.Cman.RestAPI.Host) == "" {
					obj.Spec.Cman.RestAPI.Host = defaultTrafficManagerInternalServiceDNS(obj)
				}
			}
		}
		if obj.Spec.Service.Internal.Enabled == nil {
			enable := true
			obj.Spec.Service.Internal.Enabled = &enable
		}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-network-oracle-com-v4-trafficmanager,mutating=false,failurePolicy=fail,sideEffects=None,groups=network.oracle.com,resources=trafficmanagers,verbs=create;update,versions=v4,name=vtrafficmanager-v4.kb.io,admissionReviewVersions=v1
func (r *TrafficManager) ValidateCreate(_ context.Context, obj *TrafficManager) (admission.Warnings, error) {
	return nil, validateTrafficManager(obj)
}

func (r *TrafficManager) ValidateUpdate(_ context.Context, _ *TrafficManager, newObj *TrafficManager) (admission.Warnings, error) {
	return nil, validateTrafficManager(newObj)
}

func (r *TrafficManager) ValidateDelete(_ context.Context, _ *TrafficManager) (admission.Warnings, error) {
	return nil, nil
}

func validateTrafficManager(obj *TrafficManager) error {
	switch obj.Spec.Type {
	case TrafficManagerTypeNginx:
	case TrafficManagerTypeCman:
	default:
		return fmt.Errorf("spec.type %q is not supported yet", obj.Spec.Type)
	}
	if strings.TrimSpace(obj.Spec.Runtime.Image) == "" {
		return fmt.Errorf("spec.runtime.image must be set")
	}
	if obj.Spec.Type == TrafficManagerTypeNginx {
		if obj.Spec.Nginx == nil {
			return fmt.Errorf("spec.nginx must be set when spec.type=nginx")
		}
		if obj.Spec.Nginx.Config != nil && strings.TrimSpace(obj.Spec.Nginx.Config.MountLocation) != "" &&
			!filepath.IsAbs(strings.TrimSpace(obj.Spec.Nginx.Config.MountLocation)) {
			return fmt.Errorf("spec.nginx.config.mountLocation must be an absolute path")
		}
		if obj.Spec.Security.TLS.Enabled {
			if strings.TrimSpace(obj.Spec.Security.TLS.SecretName) == "" {
				return fmt.Errorf("spec.security.tls.secretName must be set when TLS is enabled")
			}
			if strings.TrimSpace(obj.Spec.Security.TLS.MountLocation) == "" {
				return fmt.Errorf("spec.security.tls.mountLocation must be set when TLS is enabled")
			}
			if !filepath.IsAbs(strings.TrimSpace(obj.Spec.Security.TLS.MountLocation)) {
				return fmt.Errorf("spec.security.tls.mountLocation must be an absolute path")
			}
		}
		if obj.Spec.Security.BackendTLS != nil {
			if strings.TrimSpace(obj.Spec.Security.BackendTLS.TrustSecretName) == "" {
				return fmt.Errorf("spec.security.backendTLS.trustSecretName must be set when backendTLS is provided")
			}
			if strings.TrimSpace(obj.Spec.Security.BackendTLS.MountLocation) != "" &&
				!filepath.IsAbs(strings.TrimSpace(obj.Spec.Security.BackendTLS.MountLocation)) {
				return fmt.Errorf("spec.security.backendTLS.mountLocation must be an absolute path")
			}
			if strings.TrimSpace(obj.Spec.Security.BackendTLS.TrustFileName) == "" {
				return fmt.Errorf("spec.security.backendTLS.trustFileName must be set when backendTLS is provided")
			}
			if strings.Contains(strings.TrimSpace(obj.Spec.Security.BackendTLS.TrustFileName), "/") {
				return fmt.Errorf("spec.security.backendTLS.trustFileName must be a file name, not a path")
			}
		}
		return nil
	}

	if obj.Spec.Cman == nil {
		return fmt.Errorf("spec.cman must be set when spec.type=cman")
	}
	if obj.Spec.Cman.ConfigSource == nil {
		for i, rule := range obj.Spec.Cman.Rules {
			if strings.TrimSpace(rule.Host) == "" {
				return fmt.Errorf("spec.cman.rules[%d].host must be set", i)
			}
			if strings.TrimSpace(rule.Action) == "" {
				continue
			}
			switch strings.TrimSpace(rule.Action) {
			case "accept", "reject", "drop":
			default:
				return fmt.Errorf("spec.cman.rules[%d].action %q is invalid", i, rule.Action)
			}
		}
		if obj.Spec.Cman.RestAPI != nil && obj.Spec.Cman.RestAPI.Enabled {
			if obj.Spec.Cman.RestAPI.PasswordSecretRef == nil {
				return fmt.Errorf("spec.cman.restApi.passwordSecretRef must be set when spec.cman.restApi.enabled=true")
			}
			if obj.Spec.Cman.RestAPI.PrivateKeySecretRef == nil {
				return fmt.Errorf("spec.cman.restApi.privateKeySecretRef must be set when spec.cman.restApi.enabled=true")
			}
		}
		return nil
	}
	if obj.Spec.Cman.ConfigSource.ConfigMapRef == nil {
		return fmt.Errorf("spec.cman.configSource.configMapRef must be set when spec.cman.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.Cman.LogLevel) != "" {
		return fmt.Errorf("spec.cman.logLevel cannot be set when spec.cman.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.Cman.TraceLevel) != "" {
		return fmt.Errorf("spec.cman.traceLevel cannot be set when spec.cman.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.Cman.RegistrationInvitedNodes) != "" {
		return fmt.Errorf("spec.cman.registrationInvitedNodes cannot be set when spec.cman.configSource is provided")
	}
	if len(obj.Spec.Cman.Rules) > 0 {
		return fmt.Errorf("spec.cman.rules cannot be set when spec.cman.configSource is provided")
	}
	if obj.Spec.Cman.RestAPI != nil {
		return fmt.Errorf("spec.cman.restApi cannot be set when spec.cman.configSource is provided")
	}
	return nil
}

func trafficManagerContainerPort(obj *TrafficManager) int32 {
	if obj.Spec.Type == TrafficManagerTypeCman {
		return trafficManagerPrimaryPort(obj)
	}
	if obj.Spec.Service.Internal.TargetPort > 0 {
		return obj.Spec.Service.Internal.TargetPort
	}
	if obj.Spec.Service.External.TargetPort > 0 {
		return obj.Spec.Service.External.TargetPort
	}
	if obj.Spec.Security.TLS.Enabled {
		return 8443
	}
	return 8080
}

func trafficManagerPrimaryPort(obj *TrafficManager) int32 {
	if len(obj.Spec.Service.Internal.Ports) > 0 {
		for _, port := range obj.Spec.Service.Internal.Ports {
			if port.TargetPort > 0 {
				return port.TargetPort
			}
			if port.Port > 0 {
				return port.Port
			}
		}
	}
	if obj.Spec.Service.Internal.TargetPort > 0 {
		return obj.Spec.Service.Internal.TargetPort
	}
	if obj.Spec.Service.External.TargetPort > 0 {
		return obj.Spec.Service.External.TargetPort
	}
	if obj.Spec.Service.Internal.Port > 0 {
		return obj.Spec.Service.Internal.Port
	}
	if obj.Spec.Service.External.Port > 0 {
		return obj.Spec.Service.External.Port
	}
	if obj.Spec.Type == TrafficManagerTypeCman {
		return 1521
	}
	return trafficManagerContainerPort(obj)
}

func defaultTrafficManagerInternalServiceDNS(obj *TrafficManager) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", obj.Name, obj.Namespace)
}
