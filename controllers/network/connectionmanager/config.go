package connectionmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	connectionManagerUserConfigFilePath     = "/opt/oracle/cman-config/cman.ora"
	connectionManagerStartupScriptMountDir  = "/opt/oracle/cman-scripts"
	connectionManagerStartupScriptName      = "start-cman.sh"
	connectionManagerConfigDirEnvName       = "CMAN_CONFIG_DIR"
	connectionManagerRestSecretsMountDir    = "/run/secrets/cman-rest"
	connectionManagerRestPasswordFileName   = "RESTpwdsecret"
	connectionManagerRestPrivateKeyFileName = "RESTkeysecret"
)

var (
	connectionManagerRestPasswordFilePath   = filepath.Join(connectionManagerRestSecretsMountDir, connectionManagerRestPasswordFileName)
	connectionManagerRestPrivateKeyFilePath = filepath.Join(connectionManagerRestSecretsMountDir, connectionManagerRestPrivateKeyFileName)
)

func connectionManagerLabels(inst *networkv4.ConnectionManager) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       inst.Name,
		"app.kubernetes.io/component":  "connection-manager",
		"app.kubernetes.io/managed-by": "Oracle-Database-Operator",
	}
}

func connectionManagerDeploymentName(inst *networkv4.ConnectionManager) string {
	return inst.Name
}

func connectionManagerInternalServiceName(inst *networkv4.ConnectionManager) string {
	return inst.Name
}

func connectionManagerExternalServiceName(inst *networkv4.ConnectionManager) string {
	return inst.Name + "-ext"
}

func imagePullPolicyOrDefault(policy corev1.PullPolicy) corev1.PullPolicy {
	if policy == "" {
		return corev1.PullIfNotPresent
	}
	return policy
}

func buildConnectionManagerEnvVars(inst *networkv4.ConnectionManager) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(inst.Spec.Runtime.EnvVars)+12)
	for _, env := range inst.Spec.Runtime.EnvVars {
		if strings.TrimSpace(env.Name) == "" {
			continue
		}
		out = append(out, corev1.EnvVar{Name: env.Name, Value: env.Value})
	}
	out = append(out, buildConnectionManagerCmanEnvVars(inst)...)
	return out
}

func buildConnectionManagerCmanEnvVars(inst *networkv4.ConnectionManager) []corev1.EnvVar {
	out := []corev1.EnvVar{
		{Name: "PUBLIC_HOSTNAME", Value: connectionManagerInternalServiceName(inst)},
		{Name: "DOMAIN", Value: fmt.Sprintf("%s.svc.cluster.local", inst.Namespace)},
		{Name: "PORT", Value: strconv.Itoa(int(connectionManagerPrimaryPort(inst)))},
		{Name: "REST_PORT", Value: strconv.Itoa(int(connectionManagerRestPort(inst)))},
	}
	if connectionManagerMaintenanceEnabled(inst) {
		out = append(out, corev1.EnvVar{Name: "CMAN_MAINTENANCE_MODE", Value: "true"})
	}
	if connectionManagerMultiFileConfigEnabled(inst) {
		out = append(out, corev1.EnvVar{Name: connectionManagerConfigDirEnvName, Value: inst.Spec.ConfigSource.MountPath})
		if connectionManagerConfigSourceHasKey(inst) {
			out = append(out, corev1.EnvVar{Name: "USER_CMAN_FILE", Value: connectionManagerConfigSourcePath(inst)})
		}
		return out
	}
	if connectionManagerUsesFileConfig(inst) {
		return append(out, corev1.EnvVar{Name: "USER_CMAN_FILE", Value: connectionManagerUserConfigFilePath})
	}
	if strings.TrimSpace(inst.Spec.LogLevel) != "" {
		out = append(out, corev1.EnvVar{Name: "LOG_LEVEL", Value: inst.Spec.LogLevel})
	}
	if strings.TrimSpace(inst.Spec.TraceLevel) != "" {
		out = append(out, corev1.EnvVar{Name: "TRACE_LEVEL", Value: inst.Spec.TraceLevel})
	}
	if strings.TrimSpace(inst.Spec.RegistrationInvitedNodes) != "" {
		out = append(out, corev1.EnvVar{Name: "REGISTRATION_INVITED_NODES", Value: inst.Spec.RegistrationInvitedNodes})
	}
	if details := connectionManagerDBHostDetails(inst); details != "" {
		out = append(out, corev1.EnvVar{Name: "DB_HOSTDETAILS", Value: details})
	}
	if connectionManagerRestEnabled(inst) {
		out = append(out,
			corev1.EnvVar{Name: "ENABLE_REST_API", Value: "true"},
			corev1.EnvVar{Name: "REST_HOST", Value: connectionManagerRestHost(inst)},
			corev1.EnvVar{Name: "REST_PWD_FILE", Value: connectionManagerRestPasswordFilePath},
			corev1.EnvVar{Name: "REST_PWD_KEY", Value: connectionManagerRestPrivateKeyFilePath},
		)
	}
	return out
}

func connectionManagerMaintenanceEnabled(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.Maintenance != nil && inst.Spec.Maintenance.Enabled
}

func buildConnectionManagerVolumes(inst *networkv4.ConnectionManager) ([]corev1.VolumeMount, []corev1.Volume) {
	volumeMounts := make([]corev1.VolumeMount, 0)
	volumes := make([]corev1.Volume, 0)
	if inst.Spec.Storage != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      connectionManagerStorageVolumeName,
			MountPath: inst.Spec.Storage.MountPath,
		})
		volumes = append(volumes, corev1.Volume{
			Name: connectionManagerStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: connectionManagerStorageClaimName(inst),
				},
			},
		})
	}
	if connectionManagerMultiFileConfigEnabled(inst) {
		ref := inst.Spec.ConfigSource.ConfigMapRef
		defaultMode := int32(0444)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "cman-config-source",
			MountPath: inst.Spec.ConfigSource.MountPath,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "cman-config-source",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
					DefaultMode:          &defaultMode,
				},
			},
		})
	} else if connectionManagerUsesFileConfig(inst) && inst.Spec.ConfigSource != nil {
		if ref := inst.Spec.ConfigSource.ConfigMapRef; ref != nil {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "cman-user-config",
				MountPath: connectionManagerUserConfigFilePath,
				SubPath:   ref.Key,
				ReadOnly:  true,
			})
			volumes = append(volumes, corev1.Volume{
				Name: "cman-user-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
						Items:                []corev1.KeyToPath{{Key: ref.Key, Path: ref.Key}},
					},
				},
			})
		}
	}
	if connectionManagerStartupScriptEnabled(inst) {
		ref := inst.Spec.Runtime.StartupScript.ConfigMapRef
		defaultMode := int32(0555)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "cman-startup-script",
			MountPath: connectionManagerStartupScriptMountDir,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "cman-startup-script",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
					DefaultMode:          &defaultMode,
					Items:                []corev1.KeyToPath{{Key: ref.Key, Path: connectionManagerStartupScriptName}},
				},
			},
		})
	}
	if connectionManagerRestEnabled(inst) && inst.Spec.RestAPI != nil {
		sources := make([]corev1.VolumeProjection, 0, 2)
		if ref := inst.Spec.RestAPI.PasswordSecretRef; ref != nil {
			sources = append(sources, corev1.VolumeProjection{Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
				Items:                []corev1.KeyToPath{{Key: ref.Key, Path: connectionManagerRestPasswordFileName}},
			}})
		}
		if ref := inst.Spec.RestAPI.PrivateKeySecretRef; ref != nil {
			sources = append(sources, corev1.VolumeProjection{Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
				Items:                []corev1.KeyToPath{{Key: ref.Key, Path: connectionManagerRestPrivateKeyFileName}},
			}})
		}
		if len(sources) > 0 {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "cman-rest-secrets",
				MountPath: connectionManagerRestSecretsMountDir,
				ReadOnly:  true,
			})
			volumes = append(volumes, corev1.Volume{
				Name: "cman-rest-secrets",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{Sources: sources},
				},
			})
		}
	}
	return volumeMounts, volumes
}

func connectionManagerDBHostDetails(inst *networkv4.ConnectionManager) string {
	if len(inst.Spec.Rules) == 0 {
		return ""
	}
	parts := make([]string, 0, len(inst.Spec.Rules))
	for _, rule := range inst.Spec.Rules {
		ruleParts := []string{fmt.Sprintf("HOST=%s", rule.Host)}
		if strings.TrimSpace(rule.IP) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("IP=%s", rule.IP))
		}
		if strings.TrimSpace(rule.Src) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_SRC=%s", rule.Src))
		}
		if strings.TrimSpace(rule.Dst) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_DST=%s", rule.Dst))
		}
		if strings.TrimSpace(rule.Srv) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_SRV=%s", rule.Srv))
		}
		if strings.TrimSpace(rule.Action) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_ACT=%s", rule.Action))
		}
		parts = append(parts, strings.Join(ruleParts, ":"))
	}
	return strings.Join(parts, ",")
}

func connectionManagerPrimaryPort(inst *networkv4.ConnectionManager) int32 {
	if inst.Spec.Runtime.Port > 0 {
		return inst.Spec.Runtime.Port
	}
	for _, port := range inst.Spec.Service.Internal.Ports {
		if port.TargetPort > 0 {
			return port.TargetPort
		}
		if port.Port > 0 {
			return port.Port
		}
	}
	if inst.Spec.Service.Internal.TargetPort > 0 {
		return inst.Spec.Service.Internal.TargetPort
	}
	if inst.Spec.Service.Internal.Port > 0 {
		return inst.Spec.Service.Internal.Port
	}
	for _, port := range inst.Spec.Service.External.Ports {
		if port.TargetPort > 0 {
			return port.TargetPort
		}
		if port.Port > 0 {
			return port.Port
		}
	}
	if inst.Spec.Service.External.TargetPort > 0 {
		return inst.Spec.Service.External.TargetPort
	}
	if inst.Spec.Service.External.Port > 0 {
		return inst.Spec.Service.External.Port
	}
	return 1521
}

func connectionManagerStartupScriptEnabled(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.Runtime.StartupScript != nil && inst.Spec.Runtime.StartupScript.ConfigMapRef != nil
}

func connectionManagerMultiFileConfigEnabled(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.ConfigSource != nil && inst.Spec.ConfigSource.ConfigMapRef != nil && strings.TrimSpace(inst.Spec.ConfigSource.MountPath) != ""
}

func connectionManagerConfigSourceHasKey(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.ConfigSource != nil && inst.Spec.ConfigSource.ConfigMapRef != nil && strings.TrimSpace(inst.Spec.ConfigSource.ConfigMapRef.Key) != ""
}

func connectionManagerConfigSourcePath(inst *networkv4.ConnectionManager) string {
	if !connectionManagerConfigSourceHasKey(inst) {
		return ""
	}
	if connectionManagerMultiFileConfigEnabled(inst) {
		return filepath.Join(inst.Spec.ConfigSource.MountPath, inst.Spec.ConfigSource.ConfigMapRef.Key)
	}
	return connectionManagerUserConfigFilePath
}

func connectionManagerUsesFileConfig(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.ConfigSource != nil
}

func connectionManagerRestEnabled(inst *networkv4.ConnectionManager) bool {
	return inst != nil && inst.Spec.RestAPI != nil && inst.Spec.RestAPI.Enabled && !connectionManagerUsesFileConfig(inst)
}

func connectionManagerRestHost(inst *networkv4.ConnectionManager) string {
	if !connectionManagerRestEnabled(inst) {
		return ""
	}
	if host := strings.TrimSpace(inst.Spec.RestAPI.Host); host != "" {
		return host
	}
	return connectionManagerInternalServiceName(inst) + "." + inst.Namespace + ".svc.cluster.local"
}

func connectionManagerRestPort(inst *networkv4.ConnectionManager) int32 {
	if inst != nil && inst.Spec.RestAPI != nil && inst.Spec.RestAPI.Port > 0 {
		return inst.Spec.RestAPI.Port
	}
	return 1525
}

func connectionManagerConfigMode(inst *networkv4.ConnectionManager) string {
	if connectionManagerUsesFileConfig(inst) {
		return "file"
	}
	return "generated"
}

func connectionManagerReferencesConfigMap(inst *networkv4.ConnectionManager, name string) bool {
	if inst == nil {
		return false
	}
	if ref := inst.Spec.ConfigSource; ref != nil && ref.ConfigMapRef != nil && strings.TrimSpace(ref.ConfigMapRef.Name) == name {
		return true
	}
	if ref := inst.Spec.Runtime.StartupScript; ref != nil && ref.ConfigMapRef != nil && strings.TrimSpace(ref.ConfigMapRef.Name) == name {
		return true
	}
	return false
}

func connectionManagerReferencesSecret(inst *networkv4.ConnectionManager, name string) bool {
	if inst == nil || inst.Spec.RestAPI == nil || !inst.Spec.RestAPI.Enabled || connectionManagerUsesFileConfig(inst) {
		return false
	}
	if ref := inst.Spec.RestAPI.PasswordSecretRef; ref != nil && strings.TrimSpace(ref.Name) == name {
		return true
	}
	if ref := inst.Spec.RestAPI.PrivateKeySecretRef; ref != nil && strings.TrimSpace(ref.Name) == name {
		return true
	}
	return false
}

func (r *ConnectionManagerReconciler) resolveConnectionManagerConfigChecksum(ctx context.Context, inst *networkv4.ConnectionManager) (string, error) {
	payload := make([]string, 0, 8)
	if !connectionManagerUsesFileConfig(inst) {
		payload = append(payload, "generated:"+strings.Join(flattenEnvVars(buildConnectionManagerCmanEnvVars(inst)), "\n"))
	} else {
		if inst.Spec.ConfigSource == nil || inst.Spec.ConfigSource.ConfigMapRef == nil {
			return "", fmt.Errorf("spec.configSource.configMapRef must be set")
		}
		ref := inst.Spec.ConfigSource.ConfigMapRef
		configMap := &corev1.ConfigMap{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, configMap); err != nil {
			return "", fmt.Errorf("failed to get configMap %s: %w", ref.Name, err)
		}
		if connectionManagerConfigSourceHasKey(inst) {
			if _, ok := connectionManagerConfigMapValue(configMap, ref.Key); !ok {
				return "", fmt.Errorf("configMap %s is missing key %s", ref.Name, ref.Key)
			}
		}
		if !connectionManagerMultiFileConfigEnabled(inst) {
			value, _ := connectionManagerConfigMapValue(configMap, ref.Key)
			payload = append(payload, "legacy-config:"+ref.Name+":"+ref.Key+":"+hex.EncodeToString(value))
		} else {
			payload = append(payload, "config-source-mount:"+inst.Spec.ConfigSource.MountPath, "config-source:"+ref.Name)
			payload = appendConnectionManagerConfigMapData(payload, "config-source-data", configMap)
		}
	}
	var err error
	payload, err = r.appendConnectionManagerStartupScriptChecksum(ctx, inst, payload)
	if err != nil {
		return "", err
	}
	return checksumString(strings.Join(payload, "\n")), nil
}

func (r *ConnectionManagerReconciler) appendConnectionManagerStartupScriptChecksum(ctx context.Context, inst *networkv4.ConnectionManager, payload []string) ([]string, error) {
	if !connectionManagerStartupScriptEnabled(inst) {
		return payload, nil
	}
	ref := inst.Spec.Runtime.StartupScript.ConfigMapRef
	configMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, configMap); err != nil {
		return nil, fmt.Errorf("failed to get startup script configMap %s: %w", ref.Name, err)
	}
	value, ok := connectionManagerConfigMapValue(configMap, ref.Key)
	if !ok {
		return nil, fmt.Errorf("startup script configMap %s is missing key %s", ref.Name, ref.Key)
	}
	return append(payload, "startup-script:"+ref.Name+":"+ref.Key+":"+hex.EncodeToString(value)), nil
}

func connectionManagerConfigMapValue(configMap *corev1.ConfigMap, key string) ([]byte, bool) {
	if value, ok := configMap.Data[key]; ok {
		return []byte(value), true
	}
	value, ok := configMap.BinaryData[key]
	return value, ok
}

func appendConnectionManagerConfigMapData(payload []string, prefix string, configMap *corev1.ConfigMap) []string {
	keys := make([]string, 0, len(configMap.Data)+len(configMap.BinaryData))
	seen := make(map[string]struct{}, len(configMap.Data)+len(configMap.BinaryData))
	for key := range configMap.Data {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range configMap.BinaryData {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, _ := connectionManagerConfigMapValue(configMap, key)
		payload = append(payload, prefix+":"+key+":"+hex.EncodeToString(value))
	}
	return payload
}

func (r *ConnectionManagerReconciler) resolveConnectionManagerRestSecretChecksum(ctx context.Context, inst *networkv4.ConnectionManager) (string, error) {
	if !connectionManagerRestEnabled(inst) || inst.Spec.RestAPI == nil {
		return "", nil
	}
	payload := make([]byte, 0)
	for _, ref := range []*networkv4.ConnectionManagerSecretKeyRef{
		inst.Spec.RestAPI.PasswordSecretRef,
		inst.Spec.RestAPI.PrivateKeySecretRef,
	} {
		if ref == nil {
			continue
		}
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to get secret %s: %w", ref.Name, err)
		}
		value, ok := secret.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("secret %s is missing key %s", ref.Name, ref.Key)
		}
		payload = append(payload, value...)
	}
	if len(payload) == 0 {
		return "", nil
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func checksumString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func flattenEnvVars(envs []corev1.EnvVar) []string {
	out := make([]string, 0, len(envs))
	for _, env := range envs {
		out = append(out, env.Name+"="+env.Value)
	}
	sort.Strings(out)
	return out
}
