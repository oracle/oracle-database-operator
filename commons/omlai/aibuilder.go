package commons

import (
	"context"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/omlai/v4" // TODO
)

func buildLabelsForPrivateAi(instance *privateaiv4.PrivateAi, label, name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":       "PrivateAi-" + instance.Name,
		"app.kubernetes.io/name":           instance.Name,
		"app.kubernetes.io/component":      getLabelForPrivateAI(instance),
		"app.kubernetes.io/managed-by":     "Oracle-Database-Operator",
		"app.kubernetes.io/offline-status": "false",
	}
}

func getLabelForPrivateAI(instance *privateaiv4.PrivateAi) string {
	return "Oml-" + "PrivateAi-" + instance.Name
}

func BuildDeploySetForPrivateAI(instance *privateaiv4.PrivateAi) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta:   buildTypeMetaForPrivateAI(),
		ObjectMeta: buildObjectMetaForPrivateAI(instance),
		Spec:       *buildDeploymentSpecForPrivateAI(instance),
	}
}

func buildTypeMetaForPrivateAI() metav1.TypeMeta {
	return metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	}
}

func buildObjectMetaForPrivateAI(instance *privateaiv4.PrivateAi) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            instance.Name,
		Namespace:       instance.Namespace,
		OwnerReferences: getOwnerRefPrivateAI(instance),
		Labels:          buildLabelsForPrivateAi(instance, "privateai", instance.Name),
	}
}

func buildDeploymentSpecForPrivateAI(instance *privateaiv4.PrivateAi) *appsv1.DeploymentSpec {
	var replicas int32 = 1
	if instance.Spec.Replicas > 0 {
		replicas = instance.Spec.Replicas
	} else {
		replicas = 1
	}

	return &appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: buildLabelsForPrivateAi(instance, "privateai", instance.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: buildLabelsForPrivateAi(instance, "privateai", instance.Name),
			},
			Spec: *buildPodSpecForPrivateAI(instance),
		},
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}

func buildPodSpecForPrivateAI(instance *privateaiv4.PrivateAi) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		// SecurityContext: &corev1.PodSecurityContext{FsGroup: int64(2001), RunAsUser: int64(2001), RunAsGroup(2001)},
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup:    int64Ptr(2001),
			RunAsUser:  int64Ptr(2001),
			RunAsGroup: int64Ptr(2001),
		},
		Containers: buildContainerSpecForPrivateAI(instance),
		Volumes:    buildVolumeSpecForPrivateAI(instance),
	}

	if len(instance.Spec.PaiImagePullSecret) > 0 {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: instance.Spec.PaiImagePullSecret},
		}
	}

	// Add NodeSelector if provided ?? TODO
	// if len(paiSpec.NodeSelector) > 0 {
	//     spec.NodeSelector = paiSpec.NodeSelector
	// }

	return spec
}

func buildContainerSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.Container {
	container := corev1.Container{
		Name:            instance.Name,
		Image:           instance.Spec.PaiImage,
		Resources:       corev1.ResourceRequirements{},
		VolumeMounts:    buildVolumeMountSpecForPrivateAI(instance),
		Env:             buildEnvVarsForPrivateAI(instance, instance.Spec.EnvVars),
		ImagePullPolicy: corev1.PullIfNotPresent,
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"sh", "-c", "! /bin/test -f /tmp/unhealthy"},
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		},
	}
	var appPort int32
	if instance.Spec.Resources != nil {
		container.Resources = *instance.Spec.Resources
	}
	if instance.Spec.PaiHTTPEnabled {
		container.Env = append(container.Env, corev1.EnvVar{Name: "OML_HTTP_ENABLED", Value: "true"}, corev1.EnvVar{Name: "OML_HTTPS_ENABLED", Value: "false"})
		appPort = instance.Spec.PaiHTTPPort

	} else {
		// HTTPS
		container.Env = append(container.Env, corev1.EnvVar{Name: "OML_HTTP_ENABLED", Value: "false"}, corev1.EnvVar{Name: "OML_HTTPS_ENABLED", Value: "true"})
		appPort = instance.Spec.PaiHTTPSPort
	}

	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(int(appPort)),
			},
		},
		InitialDelaySeconds: 60,
		TimeoutSeconds:      3,
		SuccessThreshold:    1,
		FailureThreshold:    3,
		PeriodSeconds:       30,
	}
	container.Ports = []corev1.ContainerPort{
		{
			ContainerPort: appPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	return []corev1.Container{container}
}

func buildVolumeSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.Volume {
	var vols []corev1.Volume
	if instance.Spec.PaiSecret != nil && instance.Spec.PaiSecret.Name != "" {
		vols = append(vols, corev1.Volume{
			Name: instance.Name + "secret-vol",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.PaiSecret.Name},
			},
		})
	}

	if instance.Spec.PaiConfigFile != nil && instance.Spec.PaiConfigFile.Name != "" {
		vols = append(vols, corev1.Volume{
			Name: instance.Name + "configmap-vol",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.PaiConfigFile.Name}},
			},
		})
	}

	if instance.Spec.StorageClass != "" {
		vols = append(vols, corev1.Volume{
			Name:         instance.Name + "-oradata-vol4",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: instance.Name + "-oradata-vol4"}},
		})
	}

	vols = append(vols, corev1.Volume{
		Name: instance.Name + "-logs-vol",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	return vols
}

func buildVolumeMountSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.VolumeMount {
	var vms []corev1.VolumeMount

	// Main data PVC mount
	if instance.Spec.StorageClass != "" {
		vms = append(vms, corev1.VolumeMount{
			Name:      instance.Name + "-oradata-vol4",
			MountPath: piDataMount,
		})
	}

	if instance.Spec.PaiSecret != nil {
		vms = append(vms, corev1.VolumeMount{
			Name:      instance.Name + "secret-vol",
			MountPath: instance.Spec.PaiSecret.MountLocation,
			ReadOnly:  true,
		})
	}

	if instance.Spec.PaiConfigFile != nil {
		vms = append(vms, corev1.VolumeMount{
			Name:      instance.Name + "configmap-vol",
			MountPath: instance.Spec.PaiConfigFile.MountLocation,
			ReadOnly:  true,
		})
	}

	// NEW: Logs mount
	logMount := defaultLogMount
	if instance.Spec.PaiLogLocation != "" {
		logMount = instance.Spec.PaiLogLocation
	}

	vms = append(vms, corev1.VolumeMount{
		Name:      instance.Name + "-logs-vol",
		MountPath: logMount,
	})

	return vms
}

func VolumeClaimTemplatesForPrivateAi(instance *privateaiv4.PrivateAi) []corev1.PersistentVolumeClaim {

	var claims []corev1.PersistentVolumeClaim

	claims = []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            instance.Name + "-oradata-vol4",
				Namespace:       instance.Namespace,
				OwnerReferences: getOwnerRef(instance),
				Labels:          buildLabelsForPrivateAi(instance, "privateai", instance.Name),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: &instance.Spec.StorageClass,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(strconv.FormatInt(int64(instance.Spec.StorageSizeInGb), 10) + "Gi"),
					},
				},
			},
		},
	}

	//	if len(instance.Spec.PvcList) != 0 {

	//claims = append(claims, slices.Collect(maps.Values(instance.Spec.PvcList)))
	//	}

	return claims
}

func buildEnvVarsForPrivateAI(instance *privateaiv4.PrivateAi, envVars []privateaiv4.EnvironmentVariable) []corev1.EnvVar {
	var envs []corev1.EnvVar
	var omlConfigFileFlag bool
	var omlSecretFlag bool

	for _, ev := range envVars {
		if ev.Name == "OML_CONFIG_FILE" {
			omlConfigFileFlag = true
		}
		if ev.Name == "OML_SECRETS_MOUNTPOINT" {
			omlSecretFlag = true
		}
		envs = append(envs, corev1.EnvVar{
			Name:  ev.Name,
			Value: ev.Value,
		})
	}

	if !omlConfigFileFlag {
		if instance.Spec.PaiConfigFile.MountLocation != "" && instance.Spec.PaiConfigFile.Name != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  "OML_CONFIG_FILE",
				Value: instance.Spec.PaiConfigFile.MountLocation + "/" + "config.json",
			})
		}
	}

	if !omlSecretFlag {
		if instance.Spec.PaiSecret.Name != "" && instance.Spec.PaiSecret.MountLocation != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  "OML_SECRETS_MOUNTPOINT",
				Value: instance.Spec.PaiSecret.MountLocation,
			})
		}
	}

	return envs
}

func getOwnerRefPrivateAI(instance *privateaiv4.PrivateAi) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(instance, privateaiv4.GroupVersion.WithKind("PrivateAI")),
	}
}

func BuildServiceDefForPrivateAi(instance *privateaiv4.PrivateAi, svctype string) *corev1.Service {
	//service := &corev1.Service{}
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForPrivateAi(instance, svctype),
		Spec:       corev1.ServiceSpec{},
	}

	// Check if user want External Svc on each replica pod
	if svctype == "external" {
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		service.Spec.Selector = buildLabelsForPrivateAi(instance, "privateai", instance.Name)
	}

	if svctype == "local" {
		service.Spec.ClusterIP = corev1.ClusterIPNone
		service.Spec.Selector = buildLabelsForPrivateAi(instance, "privateai", instance.Name)
	}

	// build Service Ports Specs to be exposed. If the PortMappings is not set then default ports will be exposed.
	service.Spec.Ports = buildSvcPortsDef(instance)
	if instance.Spec.PaiLBIP != "" {
		service.Spec.LoadBalancerIP = instance.Spec.PaiLBIP
	}
	return service
}

// Function to build Service ObjectMeta
func buildSvcObjectMetaForPrivateAi(instance *privateaiv4.PrivateAi, svctype string) metav1.ObjectMeta {
	// building objectMeta
	var svcName string
	if svctype == "local" {
		svcName = instance.Name
	}

	if svctype == "external" {
		svcName = instance.Name + "-svc" // consistent single svc name
	}

	objmeta := metav1.ObjectMeta{
		Name:            svcName,
		Namespace:       instance.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildSvcLabelsForPrivateAi(instance, svctype, instance.Name),
	}
	return objmeta
}

func buildSvcLabelsForPrivateAi(instance *privateaiv4.PrivateAi, svctype, name string) map[string]string {
	labelMap := buildLabelsForPrivateAi(instance, "privateai", instance.Name)
	labelMap["app.kubernetes.io/servicetype"] = svctype

	return labelMap
}

// Update Section
func ManageReplicas(
	r client.Reader,
	instance *privateaiv4.PrivateAi,
	kClient client.Client,
	Config *rest.Config,
	deploy *appsv1.Deployment,
	podList *corev1.PodList,
	ctx context.Context,
	req ctrl.Request,
	logger logr.Logger,
) (ctrl.Result, error) {
	var desired int32 = 1
	if instance.Spec.Replicas > 0 {
		desired = instance.Spec.Replicas
	}

	current := *deploy.Spec.Replicas

	if deploy.Spec.Replicas != nil && current != desired {
		LogMessages("DEBUG", "Deployment replicas mismatch. Updating deployment...", nil, instance, logger)

		newDeploy := BuildDeploySetForPrivateAI(instance)
		newDeploy.Spec.Replicas = &desired
		err := kClient.Update(context.Background(), newDeploy)
		if err != nil {
			LogMessages("ERROR", "Failed to update Deployment with new replica count", err, instance, logger)
			return ctrl.Result{}, err
		}
	}

	// Re-mark pods based on diff
	diff := current - desired

	for i := range podList.Items {
		pod := &podList.Items[i]

		if diff > 0 {
			// Mark pod as unhealthy
			touchCmd := []string{"/bin/touch", "/tmp/unhealthy"}
			_, err := ExecCommand(r, Config, pod.Name, pod.Namespace, pod.Spec.Containers[0].Name, ctx, req, false, touchCmd)
			if err != nil {
				LogMessages("ERROR", "Failed to mark pod as unhealthy", err, instance, logger)
				return ctrl.Result{}, err
			}
			diff--
		} else {
			// Heal pod if previously unready
			removeCmd := []string{"rm", "-f", "/tmp/unhealthy"}
			_, err := ExecCommand(r, Config, pod.Name, pod.Namespace, pod.Spec.Containers[0].Name, ctx, req, false, removeCmd)
			if err != nil {
				LogMessages("ERROR", "Failed to heal pod back to ready state", err, instance, logger)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// Update Section
func UpdateDeploySetForPrivateAI(
	instance *privateaiv4.PrivateAi,
	paiSpec privateaiv4.PrivateAiSpec,
	kClient client.Client,
	Config *rest.Config,
	deploy *appsv1.Deployment,
	pod *corev1.Pod,
	logger logr.Logger,
) (ctrl.Result, error) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == deploy.Name {
			contRes := pod.Spec.Containers[i].Resources
			paiRes := paiSpec.Resources
			if !reflect.DeepEqual(contRes, paiRes) {
				LogMessages("DEBUG", "Container resources have changed. Updating deployment...", nil, instance, logger)

				// Update the deployment with new spec
				err := kClient.Update(context.Background(), BuildDeploySetForPrivateAI(instance))
				if err != nil {
					LogMessages("ERROR", "Failed to update deployment with new spec", err, instance, logger)
					return ctrl.Result{}, err
				}
			}
		}
	}

	return ctrl.Result{}, nil
}
