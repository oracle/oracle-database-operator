package v4

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *ConnectionManager) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy[*ConnectionManager](mgr, r).
		WithValidator(&ConnectionManager{}).
		WithDefaulter(&ConnectionManager{}).
		Complete()
}

var _ admission.Defaulter[*ConnectionManager] = &ConnectionManager{}
var _ admission.Validator[*ConnectionManager] = &ConnectionManager{}

// +kubebuilder:webhook:path=/mutate-network-oracle-com-v4-connectionmanager,mutating=true,failurePolicy=fail,sideEffects=None,groups=network.oracle.com,resources=connectionmanagers,verbs=create;update,versions=v4,name=mconnectionmanager-v4.kb.io,admissionReviewVersions=v1
func (r *ConnectionManager) Default(_ context.Context, obj *ConnectionManager) error {
	if obj.Spec.Runtime.Replicas <= 0 {
		obj.Spec.Runtime.Replicas = 1
	}
	if obj.Spec.Runtime.Port <= 0 && !connectionManagerServiceHasExplicitPort(obj.Spec.Service) {
		obj.Spec.Runtime.Port = 1521
	}
	if obj.Spec.ConfigSource == nil {
		if strings.TrimSpace(obj.Spec.LogLevel) == "" {
			obj.Spec.LogLevel = "user"
		}
		if strings.TrimSpace(obj.Spec.TraceLevel) == "" {
			obj.Spec.TraceLevel = "user"
		}
		if strings.TrimSpace(obj.Spec.RegistrationInvitedNodes) == "" {
			obj.Spec.RegistrationInvitedNodes = "*"
		}
		if obj.Spec.RestAPI != nil && obj.Spec.RestAPI.Enabled {
			if obj.Spec.RestAPI.Port == 0 {
				obj.Spec.RestAPI.Port = 1525
			}
			if strings.TrimSpace(obj.Spec.RestAPI.Host) == "" {
				obj.Spec.RestAPI.Host = connectionManagerInternalServiceDNS(obj)
			}
		}
	}
	if obj.Spec.Service.Internal.Enabled == nil {
		enable := true
		obj.Spec.Service.Internal.Enabled = &enable
	}
	if obj.Spec.Storage != nil {
		obj.Spec.Storage.MountPath = strings.TrimSpace(obj.Spec.Storage.MountPath)
		obj.Spec.Storage.PvcName = strings.TrimSpace(obj.Spec.Storage.PvcName)
		obj.Spec.Storage.StorageClass = strings.TrimSpace(obj.Spec.Storage.StorageClass)
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-network-oracle-com-v4-connectionmanager,mutating=false,failurePolicy=fail,sideEffects=None,groups=network.oracle.com,resources=connectionmanagers,verbs=create;update,versions=v4,name=vconnectionmanager-v4.kb.io,admissionReviewVersions=v1
func (r *ConnectionManager) ValidateCreate(_ context.Context, obj *ConnectionManager) (admission.Warnings, error) {
	return nil, validateConnectionManager(obj)
}

func (r *ConnectionManager) ValidateUpdate(_ context.Context, _ *ConnectionManager, newObj *ConnectionManager) (admission.Warnings, error) {
	return nil, validateConnectionManager(newObj)
}

func (r *ConnectionManager) ValidateDelete(_ context.Context, _ *ConnectionManager) (admission.Warnings, error) {
	return nil, nil
}

func validateConnectionManager(obj *ConnectionManager) error {
	if strings.TrimSpace(obj.Spec.Runtime.Image) == "" {
		return fmt.Errorf("spec.runtime.image must be set")
	}
	if err := validateConnectionManagerRuntime(&obj.Spec.Runtime); err != nil {
		return err
	}
	if err := validateConnectionManagerStorage(obj.Spec.Storage); err != nil {
		return err
	}
	if obj.Spec.RestAPI != nil && (obj.Spec.RestAPI.Port < 0 || obj.Spec.RestAPI.Port > 65535) {
		return fmt.Errorf("spec.restApi.port must be between 1 and 65535 when set")
	}
	if obj.Spec.ConfigSource == nil {
		for i, rule := range obj.Spec.Rules {
			if strings.TrimSpace(rule.Host) == "" {
				return fmt.Errorf("spec.rules[%d].host must be set", i)
			}
			if strings.TrimSpace(rule.Action) == "" {
				continue
			}
			switch strings.TrimSpace(rule.Action) {
			case "accept", "reject", "drop":
			default:
				return fmt.Errorf("spec.rules[%d].action %q is invalid", i, rule.Action)
			}
		}
		if obj.Spec.RestAPI != nil && obj.Spec.RestAPI.Enabled {
			if obj.Spec.RestAPI.PasswordSecretRef == nil {
				return fmt.Errorf("spec.restApi.passwordSecretRef must be set when spec.restApi.enabled=true")
			}
			if obj.Spec.RestAPI.PrivateKeySecretRef == nil {
				return fmt.Errorf("spec.restApi.privateKeySecretRef must be set when spec.restApi.enabled=true")
			}
		}
		return nil
	}
	if obj.Spec.ConfigSource.ConfigMapRef == nil {
		return fmt.Errorf("spec.configSource.configMapRef must be set when spec.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.ConfigSource.ConfigMapRef.Name) == "" {
		return fmt.Errorf("spec.configSource.configMapRef.name must be set")
	}
	mountPath := strings.TrimSpace(obj.Spec.ConfigSource.MountPath)
	if mountPath != obj.Spec.ConfigSource.MountPath {
		return fmt.Errorf("spec.configSource.mountPath must not contain leading or trailing whitespace")
	}
	if mountPath != "" {
		if !filepath.IsAbs(mountPath) {
			return fmt.Errorf("spec.configSource.mountPath must be absolute")
		}
		cleanPath := filepath.Clean(mountPath)
		if cleanPath == string(filepath.Separator) || cleanPath != mountPath {
			return fmt.Errorf("spec.configSource.mountPath must be a clean absolute path other than %q", string(filepath.Separator))
		}
		if connectionManagerPathsOverlap(cleanPath, "/opt/oracle/cman-scripts") {
			return fmt.Errorf("spec.configSource.mountPath must not overlap /opt/oracle/cman-scripts")
		}
		if strings.TrimSpace(obj.Spec.ConfigSource.ConfigMapRef.Key) == "" && obj.Spec.Runtime.StartupScript == nil {
			return fmt.Errorf("spec.configSource.configMapRef.key must be set when no startupScript is provided")
		}
	} else if strings.TrimSpace(obj.Spec.ConfigSource.ConfigMapRef.Key) == "" {
		return fmt.Errorf("spec.configSource.configMapRef.key must be set")
	}
	if strings.TrimSpace(obj.Spec.LogLevel) != "" {
		return fmt.Errorf("spec.logLevel cannot be set when spec.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.TraceLevel) != "" {
		return fmt.Errorf("spec.traceLevel cannot be set when spec.configSource is provided")
	}
	if strings.TrimSpace(obj.Spec.RegistrationInvitedNodes) != "" {
		return fmt.Errorf("spec.registrationInvitedNodes cannot be set when spec.configSource is provided")
	}
	if len(obj.Spec.Rules) > 0 {
		return fmt.Errorf("spec.rules cannot be set when spec.configSource is provided")
	}
	if obj.Spec.RestAPI != nil {
		return fmt.Errorf("spec.restApi cannot be set when spec.configSource is provided")
	}
	return nil
}

func validateConnectionManagerRuntime(runtime *ConnectionManagerRuntimeSpec) error {
	if runtime.Port < 0 || runtime.Port > 65535 {
		return fmt.Errorf("spec.runtime.port must be between 1 and 65535 when set")
	}
	if runtime.StartupScript == nil {
		return nil
	}
	if runtime.StartupScript.ConfigMapRef == nil {
		return fmt.Errorf("spec.runtime.startupScript.configMapRef must be set")
	}
	if strings.TrimSpace(runtime.StartupScript.ConfigMapRef.Name) == "" {
		return fmt.Errorf("spec.runtime.startupScript.configMapRef.name must be set")
	}
	if strings.TrimSpace(runtime.StartupScript.ConfigMapRef.Key) == "" {
		return fmt.Errorf("spec.runtime.startupScript.configMapRef.key must be set")
	}
	return nil
}

func connectionManagerServiceHasExplicitPort(service ConnectionManagerServiceSpec) bool {
	for _, endpoint := range []ConnectionManagerServiceEndpointSpec{service.Internal, service.External} {
		if endpoint.Port > 0 || endpoint.TargetPort > 0 || len(endpoint.Ports) > 0 {
			return true
		}
	}
	return false
}

func validateConnectionManagerStorage(storage *ConnectionManagerStorageSpec) error {
	if storage == nil {
		return nil
	}

	mountPath := strings.TrimSpace(storage.MountPath)
	if mountPath == "" {
		return fmt.Errorf("spec.storage.mountPath must be set")
	}
	if mountPath != storage.MountPath {
		return fmt.Errorf("spec.storage.mountPath must not contain leading or trailing whitespace")
	}
	if !filepath.IsAbs(mountPath) {
		return fmt.Errorf("spec.storage.mountPath must be absolute")
	}
	cleanPath := filepath.Clean(mountPath)
	if cleanPath == string(filepath.Separator) || cleanPath != mountPath {
		return fmt.Errorf("spec.storage.mountPath must be a clean absolute path other than %q", string(filepath.Separator))
	}
	for _, reservedPath := range []string{"/opt/oracle/cman-config", "/opt/oracle/cman-scripts"} {
		if connectionManagerPathsOverlap(cleanPath, reservedPath) {
			return fmt.Errorf("spec.storage.mountPath must not overlap %s", reservedPath)
		}
	}

	pvcName := strings.TrimSpace(storage.PvcName)
	storageClass := strings.TrimSpace(storage.StorageClass)
	if pvcName != "" {
		if storageClass != "" {
			return fmt.Errorf("spec.storage.pvcName is mutually exclusive with spec.storage.storageClass")
		}
		if storage.StorageSizeInGb > 0 {
			return fmt.Errorf("spec.storage.pvcName is mutually exclusive with spec.storage.storageSizeInGb")
		}
		return nil
	}
	if storage.StorageSizeInGb <= 0 {
		return fmt.Errorf("spec.storage.pvcName or a positive spec.storage.storageSizeInGb must be provided")
	}
	if storageClass == "" {
		return fmt.Errorf("spec.storage.storageClass must be set when spec.storage.storageSizeInGb is provided")
	}
	return nil
}

func connectionManagerPathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	separator := string(filepath.Separator)
	return left == right || strings.HasPrefix(left, right+separator) || strings.HasPrefix(right, left+separator)
}

func connectionManagerInternalServiceDNS(obj *ConnectionManager) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", obj.Name, obj.Namespace)
}
