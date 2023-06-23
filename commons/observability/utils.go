package observability

import (
	"encoding/json"
	apiv1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func GetGrafanaJSONData(api *apiv1.DatabaseObserver) ([]byte, error) {
	dbName := api.Spec.Database.DBName
	dash := GenerateDashboard(dbName)
	j, err := json.Marshal(dash)
	if err != nil {
		return nil, err
	}
	return j, nil

}

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

func GetExporterServicePort(api *apiv1.DatabaseObserver) int32 {
	if rPort := api.Spec.Exporter.Service.Port; rPort != 0 {
		return rPort
	}
	return int32(DefaultServicePort)
}

func GetExporterServiceMonitorPort(api *apiv1.DatabaseObserver) string {
	if rPort := api.Spec.Prometheus.Port; rPort != "" {
		return rPort
	}
	return DefaultPrometheusPort

}
func GetExporterDeploymentVolumeMounts(api *apiv1.DatabaseObserver) []corev1.VolumeMount {

	volM := make([]corev1.VolumeMount, 0)
	rConfigMountPath := DefaultExporterConfigMountRootPath

	volM = append(volM, corev1.VolumeMount{
		Name:      "config-volume",
		MountPath: rConfigMountPath,
	})

	// api.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := api.Spec.Database.DBWallet.SecretName; secretName != "" {
		volM = append(volM, corev1.VolumeMount{
			Name:      "creds",
			MountPath: DefaultDBWalletMountPath,
		})
	}
	return volM
}

func GetExporterDeploymentVolumes(api *apiv1.DatabaseObserver) []corev1.Volume {

	vol := make([]corev1.Volume, 0)
	cVolumeSourceName := api.Spec.Exporter.ExporterConfig.Configmap.Name
	cVolumeSourceKey := api.Spec.Exporter.ExporterConfig.Configmap.Key
	rVolumeSourceName := DefaultExporterConfigmapPrefix + api.Name

	// config-volume Volume
	var nameToUse = rVolumeSourceName
	var keyToUse = DefaultConfigurationConfigmapKey

	if cVolumeSourceName != "" { // override
		nameToUse = cVolumeSourceName
	}
	if cVolumeSourceKey != "" { // override
		keyToUse = cVolumeSourceKey
	}
	cMSource := &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: nameToUse,
		},
		Items: []corev1.KeyToPath{{
			Key:  keyToUse,
			Path: DefaultExporterConfigmapPath,
		}},
	}
	vol = append(vol, corev1.Volume{Name: "config-volume", VolumeSource: corev1.VolumeSource{ConfigMap: cMSource}})

	// creds Volume
	// api.Spec.Database.DBWallet.SecretName optional
	// if null, consider the database NON-ADB and connect as such
	if secretName := api.Spec.Database.DBWallet.SecretName; secretName != "" {

		vol = append(vol, corev1.Volume{
			Name: "creds",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}
	return vol
}

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

func GetExporterEnvs(api *apiv1.DatabaseObserver) []corev1.EnvVar {
	var e = make([]corev1.EnvVar, 0)

	rDBPasswordKey := api.Spec.Database.DBPassword.Key
	rDBPasswordName := api.Spec.Database.DBPassword.SecretName
	optional := true

	// DEFAULT_METRICS environment variable
	e = append(e, corev1.EnvVar{
		Name:  "DEFAULT_METRICS",
		Value: DefaultExporterConfigMountPathFull,
	})

	// DATA_SOURCE_SERVICENAME environment variable
	// DATA_SOURCE_USER environment variable
	// DATA_SOURCE_NAME environment variable

	rDBConnStringSKey := api.Spec.Database.DBConnectionString.Key
	rDBConnStringSName := api.Spec.Database.DBConnectionString.SecretName

	if rDBConnStringSKey == "" { // overwrite
		rDBConnStringSKey = DefaultDBConnectionStringKey
	}
	dbConnectionString := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			Key:                  rDBConnStringSKey,
			LocalObjectReference: corev1.LocalObjectReference{Name: rDBConnStringSName},
			Optional:             &optional,
		}}
	e = append(e, corev1.EnvVar{Name: EnvVarDataSourceName, ValueFrom: dbConnectionString})

	// DB PASSWORD environment variable
	// DATA_SOURCE_PASSWORD environment variable
	if rDBPasswordKey == "" { // overwrite
		rDBPasswordKey = DefaultDBPasswordKey
	}
	dbPassword := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			Key:                  rDBPasswordKey,
			LocalObjectReference: corev1.LocalObjectReference{Name: rDBPasswordName},
			Optional:             &optional,
		}}

	e = append(e, corev1.EnvVar{Name: EnvVarDataSourcePassword, ValueFrom: dbPassword})

	// TNS_ADMIN environment variable
	if secretName := api.Spec.Database.DBWallet.SecretName; secretName != "" {
		e = append(e, corev1.EnvVar{
			Name:  "TNS_ADMIN",
			Value: DefaultDBWalletMountPath,
		})
	}

	return e
}

func GetExporterReplicas(api *apiv1.DatabaseObserver) int32 {
	if rc := api.Spec.Replicas; rc != 0 {
		return rc
	}
	return int32(DefaultReplicaCount)
}

func GetExporterImage(api *apiv1.DatabaseObserver) string {
	if img := api.Spec.Exporter.ExporterImage; img != "" {
		return img
	}
	return DefaultExporterImage

}
