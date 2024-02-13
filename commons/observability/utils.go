package observability

import (
	apiv1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// GetExporterLabels function retrieves exporter labels from api or provides default
func GetExporterLabels(api *apiv1.DatabaseObserver) map[string]string {
	var l = make(map[string]string)

	if labels := api.Spec.Prometheus.Labels; labels != nil && len(labels) > 0 {
		for k, v := range labels {
			l[k] = v
		}
		l["release"] = "stable"
		return l
	}
	return map[string]string{
		DefaultLabelKey: DefaultLabelPrefix + api.Name,
		"release":       "stable",
	}

}

// GetExporterServicePort function retrieves exporter service port from api or provides default
func GetExporterServicePort(api *apiv1.DatabaseObserver) int32 {
	if rPort := api.Spec.Exporter.Service.Port; rPort != 0 {
		return rPort
	}
	return int32(DefaultServicePort)
}

// GetExporterServiceMonitorPort function retrieves exporter service monitor port from api or provides default
func GetExporterServiceMonitorPort(api *apiv1.DatabaseObserver) string {
	if rPort := api.Spec.Prometheus.Port; rPort != "" {
		return rPort
	}
	return DefaultPrometheusPort

}

// GetExporterDeploymentVolumeMounts function retrieves volume mounts from api or provides default
func GetExporterDeploymentVolumeMounts(api *apiv1.DatabaseObserver) []corev1.VolumeMount {

	volM := make([]corev1.VolumeMount, 0)

	if cVolumeSourceName := api.Spec.Exporter.ExporterConfig.Configmap.Name; cVolumeSourceName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultConfigVolumeString,
			MountPath: DefaultExporterConfigMountRootPath,
		})
	}

	// api.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := api.Spec.Database.DBWallet.SecretName; secretName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultWalletVolumeString,
			MountPath: DefaultOracleTNSAdmin,
		})
	}

	// api.Spec.OCIConfig.SecretName required if vault is used
	if secretName := api.Spec.OCIConfig.SecretName; secretName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      DefaultOCIPrivateKeyVolumeString,
			MountPath: DefaultVaultPrivateKeyRootPath,
		})
	}
	return volM
}

// GetExporterDeploymentVolumes function retrieves volumes from api or provides default
func GetExporterDeploymentVolumes(api *apiv1.DatabaseObserver) []corev1.Volume {

	vol := make([]corev1.Volume, 0)

	// config-volume Volume
	// if null, the exporter uses the default built-in config
	if cVolumeSourceName := api.Spec.Exporter.ExporterConfig.Configmap.Name; cVolumeSourceName != "" {

		cVolumeSourceKey := api.Spec.Exporter.ExporterConfig.Configmap.Key
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
	// api.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := api.Spec.Database.DBWallet.SecretName; secretName != "" {

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
	// api.Spec.Database.DBWallet.SecretName optional
	if secretName := api.Spec.OCIConfig.SecretName; secretName != "" {

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
	return vol
}

// GetExporterSelector function retrieves labels from api or provides default
func GetExporterSelector(api *apiv1.DatabaseObserver) map[string]string {
	var s = make(map[string]string)
	if labels := api.Spec.Prometheus.Labels; labels != nil && len(labels) > 0 {
		for k, v := range labels {
			s[k] = v
		}
		return s

	}
	return map[string]string{DefaultLabelKey: DefaultLabelPrefix + api.Name}

}

// GetExporterEnvs function retrieves env from api or provides default
func GetExporterEnvs(api *apiv1.DatabaseObserver) []corev1.EnvVar {

	optional := true
	rDBPasswordKey := api.Spec.Database.DBPassword.Key
	rDBPasswordName := api.Spec.Database.DBPassword.SecretName
	rDBConnectStrKey := api.Spec.Database.DBConnectionString.Key
	rDBConnectStrName := api.Spec.Database.DBConnectionString.SecretName
	rDBVaultSecretName := api.Spec.Database.DBPassword.VaultSecretName
	rDBVaultOCID := api.Spec.Database.DBPassword.VaultOCID
	rDBUserSKey := api.Spec.Database.DBUser.Key
	rDBUserSName := api.Spec.Database.DBUser.SecretName
	rOCIConfigCMName := api.Spec.OCIConfig.ConfigMapName

	var env = make([]corev1.EnvVar, 0)

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
	env = append(env, corev1.EnvVar{Name: EnvVarDataSourceUser, ValueFrom: envUser})

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
	env = append(env, corev1.EnvVar{Name: EnvVarDataSourceConnectString, ValueFrom: envConnectStr})

	// DB_PASSWORD environment variable
	// if useVault, add environment variables for Vault ID and Vault Secret Name
	useVault := rDBVaultSecretName != "" && rDBVaultOCID != ""
	if useVault {

		env = append(env, corev1.EnvVar{Name: EnvVarDataSourcePwdVaultSecretName, Value: rDBVaultSecretName})
		env = append(env, corev1.EnvVar{Name: EnvVarDataSourcePwdVaultId, Value: rDBVaultOCID})

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

		env = append(env, corev1.EnvVar{Name: EnvVarVaultFingerprint, ValueFrom: configSourceFingerprintValue})
		env = append(env, corev1.EnvVar{Name: EnvVarVaultUserOCID, ValueFrom: configSourceUserValue})
		env = append(env, corev1.EnvVar{Name: EnvVarVaultTenancyOCID, ValueFrom: configSourceTenancyValue})
		env = append(env, corev1.EnvVar{Name: EnvVarVaultRegion, ValueFrom: configSourceRegionValue})
		env = append(env, corev1.EnvVar{Name: EnvVarVaultPrivateKeyPath, Value: DefaultVaultPrivateKeyAbsolutePath})

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

		env = append(env, corev1.EnvVar{Name: EnvVarDataSourcePassword, ValueFrom: dbPassword})

	}

	// CUSTOM_METRICS environment variable
	if customMetricsName := api.Spec.Exporter.ExporterConfig.Configmap.Name; customMetricsName != "" {
		customMetrics := DefaultExporterConfigmapAbsolutePath
		env = append(env, corev1.EnvVar{Name: EnvVarCustomConfigmap, Value: customMetrics})
	}

	env = append(env, corev1.EnvVar{Name: EnvVarOracleHome, Value: DefaultOracleHome})
	env = append(env, corev1.EnvVar{Name: EnvVarTNSAdmin, Value: DefaultOracleTNSAdmin})
	return env
}

// GetExporterReplicas function retrieves replicaCount from api or provides default
func GetExporterReplicas(api *apiv1.DatabaseObserver) int32 {
	if rc := api.Spec.Replicas; rc != 0 {
		return rc
	}
	return int32(DefaultReplicaCount)
}

// GetExporterImage function retrieves image from api or provides default
func GetExporterImage(api *apiv1.DatabaseObserver) string {
	if img := api.Spec.Exporter.ExporterImage; img != "" {
		return img
	}
	return DefaultExporterImage

}

func IsUpdateRequiredForContainerImage(desired *appsv1.Deployment, found *appsv1.Deployment) bool {
	foundImage := found.Spec.Template.Spec.Containers[0].Image
	desiredImage := desired.Spec.Template.Spec.Containers[0].Image

	return foundImage != desiredImage
}

func IsUpdateRequiredForEnvironmentVars(desired *appsv1.Deployment, found *appsv1.Deployment) bool {
	var updateEnvsRequired bool
	desiredEnvValues := make(map[string]string)

	foundEnvs := found.Spec.Template.Spec.Containers[0].Env
	desiredEnvs := desired.Spec.Template.Spec.Containers[0].Env
	if len(foundEnvs) != len(desiredEnvs) {
		updateEnvsRequired = true
	} else {
		for _, v := range desiredEnvs {

			if v.Name == EnvVarDataSourceUser ||
				v.Name == EnvVarDataSourceConnectString ||
				v.Name == EnvVarDataSourcePassword {

				ref := *(*v.ValueFrom).SecretKeyRef
				desiredEnvValues[v.Name] = ref.Key + "-" + ref.Name

			} else if v.Name == EnvVarVaultFingerprint ||
				v.Name == EnvVarVaultRegion ||
				v.Name == EnvVarVaultTenancyOCID ||
				v.Name == EnvVarVaultUserOCID {

				ref := *(*v.ValueFrom).ConfigMapKeyRef
				desiredEnvValues[v.Name] = ref.Key + "-" + ref.Name

			} else if v.Name == EnvVarDataSourcePwdVaultId ||
				v.Name == EnvVarDataSourcePwdVaultSecretName ||
				v.Name == EnvVarCustomConfigmap {

				desiredEnvValues[v.Name] = v.Value
			}
		}

		for _, v := range foundEnvs {
			var foundValue string

			if v.Name == EnvVarDataSourceUser ||
				v.Name == EnvVarDataSourceConnectString ||
				v.Name == EnvVarDataSourcePassword {

				ref := *(*v.ValueFrom).SecretKeyRef
				foundValue = ref.Key + "-" + ref.Name

			} else if v.Name == EnvVarVaultFingerprint ||
				v.Name == EnvVarVaultRegion ||
				v.Name == EnvVarVaultTenancyOCID ||
				v.Name == EnvVarVaultUserOCID {

				ref := *(*v.ValueFrom).ConfigMapKeyRef
				foundValue = ref.Key + "-" + ref.Name

			} else if v.Name == EnvVarDataSourcePwdVaultId ||
				v.Name == EnvVarDataSourcePwdVaultSecretName ||
				v.Name == EnvVarCustomConfigmap {

				foundValue = v.Value
			}

			if desiredEnvValues[v.Name] != foundValue {
				updateEnvsRequired = true
			}
		}
	}
	return updateEnvsRequired
}

func IsUpdateRequiredForVolumes(desired *appsv1.Deployment, found *appsv1.Deployment) bool {
	var updateVolumesRequired bool
	var foundConfigmap, desiredConfigmap string
	var foundWalletSecret, desiredWalletSecret string
	var foundOCIConfig, desiredOCIConfig string

	desiredVolumes := desired.Spec.Template.Spec.Volumes
	foundVolumes := found.Spec.Template.Spec.Volumes

	if len(desiredVolumes) != len(foundVolumes) {
		updateVolumesRequired = true
	} else {
		for _, v := range desiredVolumes {
			if v.Name == DefaultConfigVolumeString {
				desiredConfigmap = v.ConfigMap.Name
				for _, key := range v.ConfigMap.Items {
					desiredConfigmap += key.Key
				}
			} else if v.Name == DefaultWalletVolumeString {
				desiredWalletSecret = v.VolumeSource.Secret.SecretName

			} else if v.Name == DefaultOCIPrivateKeyVolumeString {
				desiredOCIConfig = v.VolumeSource.Secret.SecretName
			}
		}

		for _, v := range foundVolumes {
			if v.Name == DefaultConfigVolumeString {
				foundConfigmap = v.ConfigMap.Name
				for _, key := range v.ConfigMap.Items {
					foundConfigmap += key.Key
				}
			} else if v.Name == DefaultWalletVolumeString {
				foundWalletSecret = v.VolumeSource.Secret.SecretName

			} else if v.Name == DefaultOCIPrivateKeyVolumeString {
				foundOCIConfig = v.VolumeSource.Secret.SecretName
			}
		}
	}

	return updateVolumesRequired ||
		desiredConfigmap != foundConfigmap ||
		desiredWalletSecret != foundWalletSecret ||
		desiredOCIConfig != foundOCIConfig
}

func IsUpdateRequiredForReplicas(desired *appsv1.Deployment, found *appsv1.Deployment) bool {
	foundReplicas := *found.Spec.Replicas
	desiredReplicas := *desired.Spec.Replicas

	return desiredReplicas != foundReplicas
}
