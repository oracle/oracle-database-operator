package observability

import (
	api "github.com/oracle/oracle-database-operator/apis/observability/v4"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"path/filepath"
	"strings"
)

func AddSidecarContainers(a *api.DatabaseObserver, listing *[]corev1.Container) {

	if containers := a.Spec.ExporterSidecars; len(containers) > 0 {
		for _, container := range containers {
			*listing = append(*listing, container)
		}

	}
}

func AddSidecarVolumes(a *api.DatabaseObserver, listing *[]corev1.Volume) {

	if volumes := a.Spec.SideCarVolumes; len(volumes) > 0 {
		for _, v := range volumes {
			*listing = append(*listing, v)
		}

	}
}

// GetLabels retrieves labels from the spec
func GetLabels(a *api.DatabaseObserver, customResourceLabels map[string]string) map[string]string {

	var l = make(map[string]string)

	// get inherited labels
	if iLabels := a.Spec.InheritLabels; iLabels != nil {
		for _, v := range iLabels {
			if v != DefaultSelectorLabelKey {
				l[v] = a.Labels[v]
			}
		}
	}

	if customResourceLabels != nil {
		for k, v := range customResourceLabels {
			if k != DefaultSelectorLabelKey {
				l[k] = v
			}
		}
	}

	// add app label
	l[DefaultSelectorLabelKey] = a.Name
	return l
}

// GetSelectorLabel adds selector label
func GetSelectorLabel(a *api.DatabaseObserver) map[string]string {
	selectors := make(map[string]string)
	selectors[DefaultSelectorLabelKey] = a.Name
	return selectors
}

// GetExporterVersion retrieves version of exporter used
func GetExporterVersion(a *api.DatabaseObserver) string {
	appVersion := "latest"
	whichImage := DefaultExporterImage
	if img := a.Spec.Exporter.Deployment.ExporterImage; img != "" {
		whichImage = img
	}

	// return tag in image:tag
	if str := strings.Split(whichImage, ":"); len(str) == 2 {
		appVersion = str[1]
	}
	return appVersion
}

// GetExporterArgs retrieves args
func GetExporterArgs(a *api.DatabaseObserver) []string {
	if args := a.Spec.Exporter.Deployment.ExporterArgs; args != nil || len(args) > 0 {
		return args
	}
	return nil
}

// GetExporterDeploymentSecurityContext retrieves security context for container
func GetExporterDeploymentSecurityContext(a *api.DatabaseObserver) *corev1.SecurityContext {
	if sc := a.Spec.Exporter.Deployment.SecurityContext; sc != nil {
		return sc
	}
	return &corev1.SecurityContext{}
}

// GetExporterPodSecurityContext retrieves security context for pods
func GetExporterPodSecurityContext(a *api.DatabaseObserver) *corev1.PodSecurityContext {
	if sc := a.Spec.Exporter.Deployment.DeploymentPodTemplate.SecurityContext; sc != nil {
		return sc
	}
	return &corev1.PodSecurityContext{}
}

// GetExporterCommands retrieves commands
func GetExporterCommands(a *api.DatabaseObserver) []string {
	if c := a.Spec.Exporter.Deployment.ExporterCommands; c != nil || len(c) > 0 {
		return c
	}
	return nil
}

// GetExporterServicePort function retrieves exporter service port from a or provides default
func GetExporterServicePort(a *api.DatabaseObserver) []corev1.ServicePort {

	servicePorts := make([]corev1.ServicePort, 0)

	// get service ports
	if ports := a.Spec.Exporter.Service.Ports; len(ports) > 0 {
		for _, port := range ports {
			servicePorts = append(servicePorts, port)
		}

	} else {
		// if not, provide default service port
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       DefaultPrometheusPort,
			Port:       DefaultServicePort,
			TargetPort: intstr.FromInt32(DefaultServiceTargetPort),
		})
	}

	return servicePorts

}

// GetEndpoints function
func GetEndpoints(a *api.DatabaseObserver) []monitorv1.Endpoint {

	endpoints := make([]monitorv1.Endpoint, 0)

	// get endpoints
	if es := a.Spec.Prometheus.ServiceMonitor.Endpoints; len(es) > 0 {
		for _, e := range es {
			endpoints = append(endpoints, e)
		}
	}

	// if not, provide default endpoint
	endpoints = append(endpoints, monitorv1.Endpoint{
		Port:     DefaultPrometheusPort,
		Interval: "20s",
	})

	return endpoints
}

func AddNamespaceSelector(a *api.DatabaseObserver, spec *monitorv1.ServiceMonitorSpec) {

	if ns := a.Spec.Prometheus.ServiceMonitor.NamespaceSelector; ns != nil {
		a.Spec.Prometheus.ServiceMonitor.NamespaceSelector.DeepCopyInto(&spec.NamespaceSelector)
	}

}

// GetExporterDeploymentVolumeMounts function retrieves volume mounts from a or provides default
func GetExporterDeploymentVolumeMounts(a *api.DatabaseObserver) []corev1.VolumeMount {

	volM := make([]corev1.VolumeMount, 0)

	if cVolumeSourceName := a.Spec.ExporterConfig.Configmap.Name; cVolumeSourceName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultConfigVolumeString,
			MountPath: DefaultExporterConfigMountRootPath,
		})
	}

	// a.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := a.Spec.Database.DBWallet.SecretName; secretName != "" {

		p := DefaultOracleTNSAdmin

		// Determine what the value of TNS_ADMIN
		// if custom TNS_ADMIN environment variable is set and found, use that instead as the path
		if rCustomEnvs := a.Spec.Exporter.Deployment.ExporterEnvs; rCustomEnvs != nil {
			if v, f := rCustomEnvs[EnvVarTNSAdmin]; f {
				p = v
			}
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultWalletVolumeString,
			MountPath: p,
		})
	}

	// a.Spec.OCIConfig.SecretName required if vault is used
	if secretName := a.Spec.OCIConfig.SecretName; secretName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultOCIPrivateKeyVolumeString,
			MountPath: DefaultVaultPrivateKeyRootPath,
		})
	}

	// a.Spec.Log.Destination path to mount for a custom log path, a volume is required
	if disabled := a.Spec.Log.Disable; !disabled {
		vName := DefaultLogVolumeString

		vDestination := a.Spec.Log.Destination
		if vDestination == "" {
			vDestination = DefaultLogDestination
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      vName,
			MountPath: vDestination,
		})
	}

	return volM
}

// GetExporterDeploymentVolumes function retrieves volumes from a or provides default
func GetExporterDeploymentVolumes(a *api.DatabaseObserver) []corev1.Volume {

	vol := make([]corev1.Volume, 0)

	// config-volume Volume
	// if null, the exporter uses the default built-in config
	if cVolumeSourceName := a.Spec.ExporterConfig.Configmap.Name; cVolumeSourceName != "" {

		cVolumeSourceKey := a.Spec.ExporterConfig.Configmap.Key
		cMSource := &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: cVolumeSourceName,
			},
			Items: []corev1.KeyToPath{{
				Key:  cVolumeSourceKey,
				Path: DefaultExporterConfigmapFilename,
			}},
		}

		vol = append(vol, corev1.Volume{Name: DefaultConfigVolumeString, VolumeSource: corev1.VolumeSource{ConfigMap: cMSource}})
	}

	// creds Volume
	// a.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := a.Spec.Database.DBWallet.SecretName; secretName != "" {

		vol = append(vol, corev1.Volume{
			Name: DefaultWalletVolumeString,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}

	// ocikey Volume
	// a.Spec.Database.DBWallet.SecretName optional
	if secretName := a.Spec.OCIConfig.SecretName; secretName != "" {

		OCIConfigSource := &corev1.SecretVolumeSource{
			SecretName: secretName,
			Items: []corev1.KeyToPath{{
				Key:  DefaultPrivateKeyFileKey,
				Path: DefaultPrivateKeyFileName,
			}},
		}

		vol = append(vol, corev1.Volume{
			Name:         DefaultOCIPrivateKeyVolumeString,
			VolumeSource: corev1.VolumeSource{Secret: OCIConfigSource},
		})
	}

	// log-volume Volume
	if disabled := a.Spec.Log.Disable; !disabled {
		vs := GetLogVolumeSource(a)
		vName := DefaultLogVolumeString

		vol = append(vol, corev1.Volume{
			Name:         vName,
			VolumeSource: vs,
		})
	}

	return vol
}

// GetExporterConfig function retrieves config name for status
func GetExporterConfig(a *api.DatabaseObserver) string {

	configName := DefaultValue
	if cmName := a.Spec.ExporterConfig.Configmap.Name; cmName != "" {
		configName = cmName
	}

	return configName
}

// GetLogVolumeSource function retrieves the source to help GetExporterDeploymentVolumes
func GetLogVolumeSource(a *api.DatabaseObserver) corev1.VolumeSource {

	vs := corev1.VolumeSource{}
	rLogVolumeClaimName := a.Spec.Log.Volume.PersistentVolumeClaim.ClaimName

	// volume claims take precedence
	if rLogVolumeClaimName != "" {
		vs.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: rLogVolumeClaimName,
		}
		return vs

	} else {
		vs.EmptyDir = &corev1.EmptyDirVolumeSource{}
		return vs
	}
}

// AddEnv is a helper method that appends an Env Var value
func AddEnv(env []corev1.EnvVar, existing map[string]string, name string, v string) []corev1.EnvVar {

	// Evaluate if env already exists
	if _, f := existing[name]; !f {
		env = append(env, corev1.EnvVar{Name: name, Value: v})
	}
	return env
}

// AddEnvFrom is a helper method that appends an Env Var value source
func AddEnvFrom(env []corev1.EnvVar, existing map[string]string, name string, v *corev1.EnvVarSource) []corev1.EnvVar {

	// Evaluate if env already exists
	if _, f := existing[name]; !f {
		env = append(env, corev1.EnvVar{Name: name, ValueFrom: v})
	}
	return env
}

// GetExporterEnvs function retrieves env from a or provides default
func GetExporterEnvs(a *api.DatabaseObserver) []corev1.EnvVar {

	optional := true
	rDBPasswordKey := a.Spec.Database.DBPassword.Key
	rDBPasswordName := a.Spec.Database.DBPassword.SecretName
	rDBConnectStrKey := a.Spec.Database.DBConnectionString.Key
	rDBConnectStrName := a.Spec.Database.DBConnectionString.SecretName
	rDBVaultSecretName := a.Spec.Database.DBPassword.VaultSecretName
	rDBVaultOCID := a.Spec.Database.DBPassword.VaultOCID
	rDBUserSKey := a.Spec.Database.DBUser.Key
	rDBUserSName := a.Spec.Database.DBUser.SecretName
	rOCIConfigCMName := a.Spec.OCIConfig.ConfigMapName
	rCustomEnvs := a.Spec.Exporter.Deployment.ExporterEnvs

	var env = make([]corev1.EnvVar, 0)

	// add CustomEnvs
	if rCustomEnvs != nil {
		for k, v := range rCustomEnvs {
			env = append(env, corev1.EnvVar{Name: k, Value: v})
		}
	}

	// DB_USERNAME environment variable
	if rDBUserSKey == "" { // overwrite
		rDBUserSKey = DefaultDbUserKey
	}
	envUser := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			Key:                  rDBUserSKey,
			LocalObjectReference: corev1.LocalObjectReference{Name: rDBUserSName},
			Optional:             &optional,
		}}
	env = AddEnvFrom(env, rCustomEnvs, EnvVarDataSourceUser, envUser)

	// DB_CONNECT_STRING environment variable
	if rDBConnectStrKey == "" {
		rDBConnectStrKey = DefaultDBConnectionStringKey
	}
	envConnectStr := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			Key:                  rDBConnectStrKey,
			LocalObjectReference: corev1.LocalObjectReference{Name: rDBConnectStrName},
			Optional:             &optional,
		}}
	env = AddEnvFrom(env, rCustomEnvs, EnvVarDataSourceConnectString, envConnectStr)

	// DB_PASSWORD environment variable
	// if useVault, add environment variables for Vault ID and Vault Secret Name
	useVault := rDBVaultSecretName != "" && rDBVaultOCID != ""
	if useVault {

		env = AddEnv(env, rCustomEnvs, EnvVarDataSourcePwdVaultSecretName, rDBVaultSecretName)
		env = AddEnv(env, rCustomEnvs, EnvVarDataSourcePwdVaultId, rDBVaultOCID)

		// Configuring the configProvider prefixed with vault_
		// https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/vault/vault.go
		configSourceFingerprintValue := &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				Key:                  DefaultOCIConfigFingerprintKey,
				LocalObjectReference: corev1.LocalObjectReference{Name: rOCIConfigCMName},
				Optional:             &optional,
			},
		}
		configSourceRegionValue := &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				Key:                  DefaultOCIConfigRegionKey,
				LocalObjectReference: corev1.LocalObjectReference{Name: rOCIConfigCMName},
				Optional:             &optional,
			},
		}
		configSourceTenancyValue := &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				Key:                  DefaultOCIConfigTenancyKey,
				LocalObjectReference: corev1.LocalObjectReference{Name: rOCIConfigCMName},
				Optional:             &optional,
			},
		}
		configSourceUserValue := &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				Key:                  DefaultOCIConfigUserKey,
				LocalObjectReference: corev1.LocalObjectReference{Name: rOCIConfigCMName},
				Optional:             &optional,
			},
		}
		env = AddEnvFrom(env, rCustomEnvs, EnvVarVaultFingerprint, configSourceFingerprintValue)
		env = AddEnvFrom(env, rCustomEnvs, EnvVarVaultUserOCID, configSourceUserValue)
		env = AddEnvFrom(env, rCustomEnvs, EnvVarVaultTenancyOCID, configSourceTenancyValue)
		env = AddEnvFrom(env, rCustomEnvs, EnvVarVaultRegion, configSourceRegionValue)
		env = AddEnv(env, rCustomEnvs, EnvVarVaultPrivateKeyPath, DefaultVaultPrivateKeyAbsolutePath)

	} else {

		if rDBPasswordKey == "" { // overwrite
			rDBPasswordKey = DefaultDBPasswordKey
		}
		dbPassword := &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				Key:                  rDBPasswordKey,
				LocalObjectReference: corev1.LocalObjectReference{Name: rDBPasswordName},
				Optional:             &optional,
			}}

		env = AddEnvFrom(env, rCustomEnvs, EnvVarDataSourcePassword, dbPassword)

	}

	// CUSTOM_METRICS environment variable
	if customMetricsName := a.Spec.ExporterConfig.Configmap.Name; customMetricsName != "" {
		customMetrics := DefaultExporterConfigmapAbsolutePath

		env = AddEnv(env, rCustomEnvs, EnvVarCustomConfigmap, customMetrics)
	}

	env = AddEnv(env, rCustomEnvs, EnvVarOracleHome, DefaultOracleHome)
	env = AddEnv(env, rCustomEnvs, EnvVarTNSAdmin, DefaultOracleTNSAdmin)

	// LOG_DESTINATION environment variable4
	if disabled := a.Spec.Log.Disable; !disabled {
		d := a.Spec.Log.Destination
		if d == "" {
			d = DefaultLogDestination
		}

		f := a.Spec.Log.Filename
		if f == "" {
			f = DefaultLogFilename
		}
		ld := filepath.Join(d, f)
		env = AddEnv(env, rCustomEnvs, EnvVarDataSourceLogDestination, ld)
	}
	return env
}

// GetExporterReplicas function retrieves replicaCount from a or provides default
func GetExporterReplicas(a *api.DatabaseObserver) int32 {
	if rc := a.Spec.Replicas; rc != 0 {
		return rc
	}
	return int32(DefaultReplicaCount)
}

// GetExporterImage function retrieves image from a or provides default
func GetExporterImage(a *api.DatabaseObserver) string {
	if img := a.Spec.Exporter.Deployment.ExporterImage; img != "" {
		return img
	}

	return DefaultExporterImage

}
