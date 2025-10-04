package observability

import (
	"path/filepath"
	"strings"

	api "github.com/oracle/oracle-database-operator/apis/observability/v4"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func AddSidecarContainers(a *api.DatabaseObserver, listing *[]corev1.Container) {

	if containers := a.Spec.Sidecar.Containers; len(containers) > 0 {
		for _, container := range containers {
			*listing = append(*listing, container)
		}
	}
}

func AddSidecarVolumes(a *api.DatabaseObserver, listing *[]corev1.Volume) {

	if volumes := a.Spec.Sidecar.Volumes; len(volumes) > 0 {
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
	if img := a.Spec.Deployment.ExporterImage; img != "" {
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
	if args := a.Spec.Deployment.ExporterArgs; args != nil || len(args) > 0 {
		return args
	}
	return nil
}

// GetExporterDeploymentSecurityContext retrieves security context for container
func GetExporterDeploymentSecurityContext(a *api.DatabaseObserver) *corev1.SecurityContext {
	if sc := a.Spec.Deployment.SecurityContext; sc != nil {
		return sc
	}
	return &corev1.SecurityContext{}
}

// GetExporterPodSecurityContext retrieves security context for pods
func GetExporterPodSecurityContext(a *api.DatabaseObserver) *corev1.PodSecurityContext {
	if sc := a.Spec.Deployment.PodSecurityContext; sc != nil {
		return sc
	}
	return &corev1.PodSecurityContext{}
}

// GetExporterCommands retrieves commands
func GetExporterCommands(a *api.DatabaseObserver) []string {
	if c := a.Spec.Deployment.ExporterCommands; c != nil || len(c) > 0 {
		return c
	}
	return nil
}

// GetExporterServicePort function retrieves exporter service port from a or provides default
func GetExporterServicePort(a *api.DatabaseObserver) []corev1.ServicePort {

	servicePorts := make([]corev1.ServicePort, 0)

	// get service ports
	if ports := a.Spec.Service.Ports; len(ports) > 0 {
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
	if es := a.Spec.ServiceMonitor.Endpoints; len(es) > 0 {
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

	if ns := a.Spec.ServiceMonitor.NamespaceSelector; ns != nil {
		a.Spec.ServiceMonitor.NamespaceSelector.DeepCopyInto(&spec.NamespaceSelector)
	}

}

// GetExporterDeploymentVolumeMounts function retrieves volume mounts from a or provides default
func GetExporterDeploymentVolumeMounts(a *api.DatabaseObserver) []corev1.VolumeMount {

	volM := make([]corev1.VolumeMount, 0)

	if len(a.Spec.Metrics.Configmap) > 0 {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultConfigVolumeString,
			MountPath: DefaultExporterConfigMountRootPath,
		})
	}

	// a.Spec.Wallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := a.Spec.Wallet.SecretName; secretName != "" {

		// Determine where to mount
		p := a.Spec.Wallet.MountPath
		if p == "" {
			p = DefaultOracleTNSAdmin
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultWalletVolumeString,
			MountPath: p,
		})
	}

	// a.Spec.Wallet.AdditionalWallets
	if add := a.Spec.Wallet.AdditionalWallets; add != nil && len(add) > 0 {
		for _, w := range add {
			// Determine where to mount
			volM = append(volM, corev1.VolumeMount{
				Name:      w.Name,
				MountPath: w.MountPath,
			})
		}

	}

	// a.Spec.OCIConfig.ConfigMap
	// a.Spec.OCIConfig.SecretName
	if oci := a.Spec.OCIConfig; oci.ConfigMap.Name != "" {

		p := oci.MountPath
		if p == "" { // overwrite
			p = DefaultOCIConfigPath
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultOCIConfigVolumeName,
			MountPath: p,
		})
	}

	// a.Spec.Log.Destination path to mount for a custom log path, a volume is required
	if disabled := a.Spec.Log.Disable; !disabled {

		vName := a.Spec.Log.Volume.Name
		if vName == "" {
			vName = DefaultLogVolumeString
		}

		vDestination := a.Spec.Log.Destination
		if vDestination == "" {
			vDestination = DefaultLogDestination
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      vName,
			MountPath: vDestination,
		})
	}

	// ObserverConfig VolumeMount
	if volumeName := a.Spec.ExporterConfig.ConfigMap.Name; volumeName != "" {
		mp := a.Spec.ExporterConfig.MountPath
		if mp == "" {
			mp = DefaultConfigMountPath
		}

		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultConfigVolumeName,
			MountPath: mp,
		})
	}

	return volM
}

// GetExporterDeploymentVolumes function retrieves volumes from a or provides default
func GetExporterDeploymentVolumes(a *api.DatabaseObserver) []corev1.Volume {

	vol := make([]corev1.Volume, 0)

	// metrics-volume Volume
	// if null, the exporter uses the default built-in metrics config
	if len(a.Spec.Metrics.Configmap) > 1 {

		vp := make([]corev1.VolumeProjection, 0)
		for _, cm := range a.Spec.Metrics.Configmap {
			cmSource := &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cm.Name,
				},
			}

			vp = append(vp, corev1.VolumeProjection{ConfigMap: cmSource})
		}

		vol = append(vol, corev1.Volume{
			Name: DefaultConfigVolumeString,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: vp,
				},
			}})

	} else if len(a.Spec.Metrics.Configmap) == 1 {

		cm := a.Spec.Metrics.Configmap[0]
		cmSource := &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: cm.Name,
			},
		}

		vol = append(vol, corev1.Volume{
			Name:         DefaultConfigVolumeString,
			VolumeSource: corev1.VolumeSource{ConfigMap: cmSource},
		})
	}

	// creds Volume
	// a.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := a.Spec.Wallet.SecretName; secretName != "" {

		vol = append(vol, corev1.Volume{
			Name: DefaultWalletVolumeString,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}

	// a.Spec.Wallet.AdditionalWallets
	if add := a.Spec.Wallet.AdditionalWallets; add != nil && len(add) > 0 {
		for _, w := range add {

			vol = append(vol, corev1.Volume{
				Name: w.Name,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: w.SecretName,
					},
				},
			})
		}

	}

	// oci-config-volume Volume
	// a.Spec.OCIConfig.PrivateKey.SecretName optional
	if oci := a.Spec.OCIConfig; oci.ConfigMap.Name != "" && oci.PrivateKey.SecretName != "" {

		vp := make([]corev1.VolumeProjection, 0)

		cmSource := &corev1.ConfigMapProjection{
			LocalObjectReference: corev1.LocalObjectReference{Name: oci.ConfigMap.Name},
		}
		secretSource := &corev1.SecretProjection{
			LocalObjectReference: corev1.LocalObjectReference{Name: oci.PrivateKey.SecretName},
		}

		vp = append(vp, corev1.VolumeProjection{ConfigMap: cmSource})
		vp = append(vp, corev1.VolumeProjection{Secret: secretSource})

		vol = append(vol, corev1.Volume{
			Name: DefaultOCIConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: vp,
				},
			},
		})

	}

	// log-volume Volume
	if disabled := a.Spec.Log.Disable; !disabled {
		vs := GetLogVolumeSource(a)

		vName := a.Spec.Log.Volume.Name
		if vName == "" {
			vName = DefaultLogVolumeString
		}

		vol = append(vol, corev1.Volume{
			Name:         vName,
			VolumeSource: vs,
		})
	}

	// ObserverConfig Volume
	if volumeName := a.Spec.ExporterConfig.ConfigMap.Name; volumeName != "" {
		cMSource := &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: volumeName,
			},
		}

		vol = append(vol, corev1.Volume{Name: DefaultConfigVolumeName, VolumeSource: corev1.VolumeSource{ConfigMap: cMSource}})
	}

	return vol
}

// GetMetricsConfig function retrieves config name for status
func GetMetricsConfig(a *api.DatabaseObserver) string {

	cms := a.Spec.Metrics.Configmap
	if len(cms) > 1 {
		metricsConfigList := make([]string, 0)
		for _, cm := range cms {
			metricsConfigList = append(metricsConfigList, cm.Name)
		}
		return strings.Join(metricsConfigList, ",")

	} else if len(cms) == 1 {
		return cms[0].Name
	}
	return DefaultValue
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
		existing[name] = ""
	}
	return env
}

func AddEnvFromConfigMap(env []corev1.EnvVar, existing map[string]string, environmentName string, key string, configMap string) []corev1.EnvVar {
	// Evaluate if env already exists
	if _, f := existing[environmentName]; !f {

		optional := true
		cm := &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				Key:                  key,
				LocalObjectReference: corev1.LocalObjectReference{Name: configMap},
				Optional:             &optional,
			},
		}
		env = append(env, corev1.EnvVar{Name: environmentName, ValueFrom: cm})
	}
	return env
}

func AddEnvFromSecret(env []corev1.EnvVar, existing map[string]string, environmentName string, key string, secretName string) []corev1.EnvVar {
	// Evaluate if env already exists
	if _, f := existing[environmentName]; !f {

		optional := true
		e := &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				Key:                  key,
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Optional:             &optional,
			}}

		env = append(env, corev1.EnvVar{Name: environmentName, ValueFrom: e})
	}
	return env
}

func AddSingleDatabaseEnvs(a *api.DatabaseObserver, e map[string]string, source []corev1.EnvVar) []corev1.EnvVar {

	u := a.Spec.Database.DBUser
	c := a.Spec.Database.DBConnectionString
	p := a.Spec.Database.DBPassword
	o := a.Spec.Database.OCIVault
	z := a.Spec.Database.AzureVault

	// DB_USERNAME environment variable
	if IsUsingAzureVault(z, VaultUsernameInUse) {
		source = AddEnv(source, e, EnvVarAzureVaultUsernameSecret, z.VaultUsernameSecret)
		source = AddEnv(source, e, EnvVarAzureVaultID, z.VaultID)

	} else if u.SecretName != "" {
		uKey := u.Key
		if uKey == "" { // overwrite
			uKey = DefaultDbUserKey
		}
		source = AddEnvFromSecret(source, e, EnvVarDataSourceUsername, uKey, u.SecretName)
	}

	// DB_CONNECT_STRING environment variable
	if c.SecretName != "" {
		cKey := c.Key
		if cKey == "" {
			cKey = DefaultDBConnectionStringKey
		}
		source = AddEnvFromSecret(source, e, EnvVarDataSourceConnectString, cKey, c.SecretName)
	}

	// DB_PASSWORD environment variable
	if IsUsingOCIVault(o) {
		source = AddEnv(source, e, EnvVarDataSourcePwdVaultSecretName, o.VaultPasswordSecret)
		source = AddEnv(source, e, EnvVarDataSourcePwdVaultId, o.VaultID)

	} else if IsUsingAzureVault(z, VaultPasswordInUse) {
		source = AddEnv(source, e, EnvVarAzureVaultPasswordSecret, z.VaultPasswordSecret)
		source = AddEnv(source, e, EnvVarAzureVaultID, z.VaultID)

	} else if p.SecretName != "" {
		pKey := p.Key
		if pKey == "" { // overwrite
			pKey = DefaultDBPasswordKey
		}
		env := p.EnvName
		if env == "" { // overwrite
			env = EnvVarDataSourcePassword
		}
		source = AddEnvFromSecret(source, e, env, pKey, p.SecretName)
	}

	// Add OCI Vault Required Values
	if a.Spec.OCIConfig.ConfigMap.Name != "" {
		ociConfig := a.Spec.OCIConfig.ConfigMap.Name
		source = AddEnv(source, e, EnvVarOCIVaultPrivateKeyPath, DefaultVaultPrivateKeyAbsolutePath)
		source = AddEnvFromConfigMap(source, e, EnvVarOCIVaultFingerprint, DefaultOCIConfigFingerprintKey, ociConfig)
		source = AddEnvFromConfigMap(source, e, EnvVarOCIVaultUserOCID, DefaultOCIConfigUserKey, ociConfig)
		source = AddEnvFromConfigMap(source, e, EnvVarOCIVaultTenancyOCID, DefaultOCIConfigTenancyKey, ociConfig)
		source = AddEnvFromConfigMap(source, e, EnvVarOCIVaultRegion, DefaultOCIConfigRegionKey, ociConfig)
	}

	// Add Azure Vault Required Values
	if a.Spec.AzureConfig.ConfigMap.Name != "" {
		azureConfig := a.Spec.AzureConfig.ConfigMap.Name
		source = AddEnvFromConfigMap(source, e, EnvVarAzureTenantID, DefaultAzureConfigTenantId, azureConfig)
		source = AddEnvFromConfigMap(source, e, EnvVarAzureClientID, DefaultAzureConfigClientId, azureConfig)
		source = AddEnvFromConfigMap(source, e, EnvVarAzureClientSecret, DefaultAzureConfigClientSecret, azureConfig)
	}

	return source
}

func AddMultiDatabaseEnvs(a *api.DatabaseObserver, e map[string]string, source []corev1.EnvVar) []corev1.EnvVar {

	for key, db := range a.Spec.Databases {
		u := db.DBUser
		c := db.DBConnectionString
		p := db.DBPassword

		// DB_USERNAME environment variable, if secret is defined
		if u.SecretName != "" {
			uKey := u.Key
			if uKey == "" { // overwrite
				uKey = DefaultDbUserKey
			}
			envUsername := u.EnvName
			if envUsername == "" {
				envUsername = key + DefaultEnvUserSuffix
			}
			source = AddEnvFromSecret(source, e, envUsername, uKey, u.SecretName)
		}

		// DB_CONNECT_STRING environment variable, if secret is defined
		if c.SecretName != "" {
			cKey := c.Key
			if cKey == "" {
				cKey = key + DefaultEnvConnectionStringSuffix
			}
			envConnectionString := c.EnvName
			if envConnectionString == "" {
				envConnectionString = key + DefaultEnvConnectionStringSuffix
			}
			source = AddEnvFromSecret(source, e, envConnectionString, cKey, c.SecretName)
		}

		// DB_PASSWORD environment variable, if secret is defined
		if p.SecretName != "" {
			pKey := p.Key
			if pKey == "" { // overwrite
				pKey = DefaultDBPasswordKey
			}
			env := p.EnvName
			if env == "" { // overwrite
				env = key + DefaultEnvPasswordSuffix
			}
			source = AddEnvFromSecret(source, e, env, pKey, p.SecretName)
		}

	}
	return source
}

func IsUsingOCIVault(f api.DBOCIVault) bool {
	return f.VaultPasswordSecret != "" && f.VaultID != ""
}

func IsUsingAzureVault(f api.DBAzureVault, v string) bool {

	if v == VaultPasswordInUse {
		return f.VaultID != "" && f.VaultPasswordSecret != ""
	}
	if v == VaultUsernameInUse {
		return f.VaultID != "" && f.VaultUsernameSecret != ""
	}
	if v == VaultIDProvided {
		return f.VaultID != ""
	}

	return false

}

func IsMultipleDatabasesDefined(a *api.DatabaseObserver) bool {
	return a.Spec.Databases != nil && len(a.Spec.Databases) > 0
}

// GetExporterEnvs function retrieves env from a or provides default
func GetExporterEnvs(a *api.DatabaseObserver) []corev1.EnvVar {

	// create slices
	var env = make([]corev1.EnvVar, 0)

	// First, add all environment variables provided
	e := a.Spec.Deployment.ExporterEnvs
	if e != nil {
		for k, v := range e {
			env = append(env, corev1.EnvVar{Name: k, Value: v})
		}
	} else {
		e = make(map[string]string)
	}

	// Add database environment variables based on single or multi DB configuration
	if IsMultipleDatabasesDefined(a) {
		env = AddMultiDatabaseEnvs(a, e, env)
	} else {
		env = AddSingleDatabaseEnvs(a, e, env)
	}

	// CUSTOM_METRICS environment variable
	if cms := a.Spec.Metrics.Configmap; cms != nil && len(cms) > 0 {
		metricsConfigList := make([]string, 0)
		for _, cm := range cms {
			metricsConfigList = append(metricsConfigList, DefaultExporterConfigMountRootPath+"/"+cm.Key)
		}
		customMetrics := strings.Join(metricsConfigList, ",")
		env = AddEnv(env, e, EnvVarCustomConfigmap, customMetrics)
	}

	env = AddEnv(env, e, EnvVarOracleHome, DefaultOracleHome)
	env = AddEnv(env, e, EnvVarTNSAdmin, DefaultOracleTNSAdmin)

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
		env = AddEnv(env, e, EnvVarDataSourceLogDestination, ld)
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
	if img := a.Spec.Deployment.ExporterImage; img != "" {
		return img
	}
	return DefaultExporterImage

}
