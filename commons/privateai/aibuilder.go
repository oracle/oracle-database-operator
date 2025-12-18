package commons

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
)

const (
	restartAnnotationKey = "kubectl.kubernetes.io/restartedAt"
	envHTTPEnabled       = "PRIVATE_AI_HTTP_ENABLED"
	envHTTPSEnabled      = "PRIVATE_AI_HTTPS_ENABLED"
	envAuthEnabled       = "PRIVATE_AI_AUTHENTICATION_ENABLED"
	envConfigFile        = "PRIVATE_AI_CONFIG_FILE"
	envSecretsMount      = "PRIVATE_AI_SECRETS_MOUNTPOINT"
)

// BuildDeploySetForPrivateAI produces a deployment definition suitable for the
// provided PrivateAi custom resource.
func BuildDeploySetForPrivateAI(instance *privateaiv4.PrivateAi) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta:   buildTypeMetaForPrivateAI(),
		ObjectMeta: buildObjectMetaForPrivateAI(instance),
		Spec:       *buildDeploymentSpecForPrivateAI(instance),
	}
}

func buildTypeMetaForPrivateAI() metav1.TypeMeta {
	return metav1.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"}
}

func buildObjectMetaForPrivateAI(instance *privateaiv4.PrivateAi) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            instance.Name,
		Namespace:       instance.Namespace,
		OwnerReferences: getOwnerRefPrivateAI(instance),
		Labels:          buildLabelsForPrivateAi(instance),
	}
}

func buildLabelsForPrivateAi(instance *privateaiv4.PrivateAi) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":       fmt.Sprintf("PrivateAi-%s", instance.Name),
		"app.kubernetes.io/name":           instance.Name,
		"app.kubernetes.io/component":      getComponentLabel(instance),
		"app.kubernetes.io/managed-by":     "Oracle-Database-Operator",
		"app.kubernetes.io/offline-status": "false",
	}
}

func getComponentLabel(instance *privateaiv4.PrivateAi) string {
	return "Oml-PrivateAi-" + instance.Name
}

func buildDeploymentSpecForPrivateAI(instance *privateaiv4.PrivateAi) *appsv1.DeploymentSpec {
	replicas := replicasOrDefault(instance.Spec.Replicas)
	strategy := appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: maxUnavailableFor(replicas)},
			MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	labels := buildLabelsForPrivateAi(instance)
	annotations := map[string]string{restartAnnotationKey: time.Now().UTC().Format(time.RFC3339)}

	return &appsv1.DeploymentSpec{
		Replicas:             pointer.Int32(replicas),
		RevisionHistoryLimit: pointer.Int32(0),
		Strategy:             strategy,
		Selector:             &metav1.LabelSelector{MatchLabels: labels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations},
			Spec:       *buildPodSpecForPrivateAI(instance),
		},
	}
}

func replicasOrDefault(requested int32) int32 {
	if requested > 0 {
		return requested
	}
	return 1
}

func maxUnavailableFor(replicas int32) int32 {
	if replicas > 1 {
		return 1
	}
	return 0
}

func buildPodSpecForPrivateAI(instance *privateaiv4.PrivateAi) *corev1.PodSpec {
	podSpec := &corev1.PodSpec{
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup:    int64Ptr(2001),
			RunAsUser:  int64Ptr(2001),
			RunAsGroup: int64Ptr(2001),
		},
		InitContainers: buildInitContainerSpecForPrivateAI(instance),
		Containers:     buildContainerSpecForPrivateAI(instance),
		Volumes:        buildVolumeSpecForPrivateAI(instance),
	}

	if len(instance.Spec.WorkerNodes) > 0 {
		podSpec.Affinity = getNodeAffinity(instance)
	}
	podSpec.TopologySpreadConstraints = buildTopologySpreadConstraintsForPrivateAI(instance)

	if instance.Spec.PaiImagePullSecret != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: instance.Spec.PaiImagePullSecret}}
	}

	return podSpec
}

// Init container: run as root, chown all pvcList mount paths
func buildInitContainerSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.Container {
	entries := orderedPVCEntries(instance.Spec.PvcList)
	if len(entries) == 0 {
		return nil
	}

	cmds := make([]string, 0, len(entries))
	for _, entry := range entries {
		cmds = append(cmds, fmt.Sprintf("chown -R 2001:2001 %q || true", entry.mountPath))
	}

	privileged := true
	rootUser := int64(0)

	return []corev1.Container{ //nolint:gomnd // explicit security context
		{
			Name:            fmt.Sprintf("%s-init", instance.Name),
			Image:           instance.Spec.PaiImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/sh", "-c", strings.Join(cmds, " && ")},
			SecurityContext: &corev1.SecurityContext{Privileged: &privileged, RunAsUser: &rootUser, RunAsGroup: &rootUser},
			VolumeMounts:    pvcVolumeMounts(instance, true),
		},
	}
}

func orderedPVCEntries(pvcs map[string]string) []pvcEntry {
	if len(pvcs) == 0 {
		return nil
	}
	entries := make([]pvcEntry, 0, len(pvcs))
	for claim, path := range pvcs {
		if path == "" {
			continue
		}
		entries = append(entries, pvcEntry{claimName: claim, mountPath: path})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].claimName < entries[j].claimName })
	return entries
}

type pvcEntry struct {
	claimName string
	mountPath string
}

func pvcVolumeMounts(instance *privateaiv4.PrivateAi, initContainer bool) []corev1.VolumeMount {
	entries := orderedPVCEntries(instance.Spec.PvcList)
	if len(entries) == 0 {
		return nil
	}

	mounts := make([]corev1.VolumeMount, 0, len(entries))
	for _, entry := range entries {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("%s-%s-vol", instance.Name, entry.claimName),
			MountPath: entry.mountPath,
			ReadOnly:  false,
		})
	}

	if !initContainer {
		return mounts
	}

	result := make([]corev1.VolumeMount, len(mounts))
	copy(result, mounts)
	return result
}

func buildContainerSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.Container {
	if instance.Spec.PaiImage == "" {
		return nil
	}

	port, scheme, httpEnabled, httpsEnabled := resolveServicePort(&instance.Spec)

	container := corev1.Container{
		Name:            instance.Name,
		Image:           instance.Spec.PaiImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             buildEnvVarsForPrivateAI(instance, instance.Spec.EnvVars, httpEnabled, httpsEnabled),
		Resources:       corev1.ResourceRequirements{},
		VolumeMounts:    buildVolumeMountSpecForPrivateAI(instance),
		Ports: []corev1.ContainerPort{{
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		}},
		StartupProbe: newHTTPProbe("/health", port, scheme, corev1.Probe{FailureThreshold: 30, PeriodSeconds: 30}),
		LivenessProbe: newHTTPProbe("/health", port, scheme, corev1.Probe{
			InitialDelaySeconds: 30,
			TimeoutSeconds:      3,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			PeriodSeconds:       30,
		}),
	}

	if instance.Spec.Resources != nil {
		container.Resources = *instance.Spec.Resources
	}

	return []corev1.Container{container}
}

func resolveServicePort(spec *privateaiv4.PrivateAiSpec) (int32, corev1.URIScheme, bool, bool) {
	httpEnabled := boolFromString(spec.PaiHTTPEnabled)
	httpsEnabled := boolFromString(spec.PaiHTTPSEnabled)

	if !httpEnabled && !httpsEnabled {
		// Preserve legacy behaviour – prefer HTTPS when port available.
		httpsEnabled = spec.PaiHTTPSPort > 0
		httpEnabled = !httpsEnabled
	}

	if httpsEnabled && spec.PaiHTTPSPort > 0 {
		return spec.PaiHTTPSPort, corev1.URISchemeHTTPS, httpEnabled, httpsEnabled
	}

	if httpEnabled && spec.PaiHTTPPort > 0 {
		return spec.PaiHTTPPort, corev1.URISchemeHTTP, httpEnabled, httpsEnabled
	}

	// Fall back to whichever port is set.
	if spec.PaiHTTPSPort > 0 {
		return spec.PaiHTTPSPort, corev1.URISchemeHTTPS, httpEnabled, true
	}
	return spec.PaiHTTPPort, corev1.URISchemeHTTP, true, httpsEnabled
}

func newHTTPProbe(path string, port int32, scheme corev1.URIScheme, base corev1.Probe) *corev1.Probe {
	probe := base
	probe.ProbeHandler = corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path:   path,
			Port:   intstr.FromInt(int(port)),
			Scheme: scheme,
		},
	}
	return &probe
}

func buildVolumeSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.Volume {
	volumes := make([]corev1.Volume, 0, 4)

	if instance.Spec.PaiSecret != nil && instance.Spec.PaiSecret.Name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: fmt.Sprintf("%ssecret-vol", instance.Name),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: instance.Spec.PaiSecret.Name},
			},
		})
	}

	if instance.Spec.PaiConfigFile != nil && instance.Spec.PaiConfigFile.Name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: fmt.Sprintf("%sconfigmap-vol", instance.Name),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Spec.PaiConfigFile.Name}},
			},
		})
	}

	if instance.Spec.StorageClass != "" {
		volumes = append(volumes, corev1.Volume{
			Name: fmt.Sprintf("%s-oradata-vol4", instance.Name),
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: fmt.Sprintf("%s-oradata-vol4", instance.Name)},
			},
		})
	}

	for _, entry := range orderedPVCEntries(instance.Spec.PvcList) {
		volumes = append(volumes, corev1.Volume{
			Name: fmt.Sprintf("%s-%s-vol", instance.Name, entry.claimName),
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: entry.claimName},
			},
		})
	}

	volumes = append(volumes, corev1.Volume{
		Name:         fmt.Sprintf("%s-logs-vol", instance.Name),
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})

	return volumes
}

func buildVolumeMountSpecForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.VolumeMount {
	var mounts []corev1.VolumeMount

	if instance.Spec.StorageClass != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("%s-oradata-vol4", instance.Name),
			MountPath: piDataMount,
		})
	}

	if instance.Spec.PaiSecret != nil && instance.Spec.PaiSecret.Name != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("%ssecret-vol", instance.Name),
			MountPath: instance.Spec.PaiSecret.MountLocation,
			ReadOnly:  true,
		})
	}

	if instance.Spec.PaiConfigFile != nil && instance.Spec.PaiConfigFile.Name != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("%sconfigmap-vol", instance.Name),
			MountPath: instance.Spec.PaiConfigFile.MountLocation,
			ReadOnly:  true,
		})
	}

	mounts = append(mounts, pvcVolumeMounts(instance, false)...)

	logMount := defaultLogMount
	if instance.Spec.PaiLogLocation != "" {
		logMount = instance.Spec.PaiLogLocation
	}

	mounts = append(mounts, corev1.VolumeMount{
		Name:      fmt.Sprintf("%s-logs-vol", instance.Name),
		MountPath: logMount,
	})

	return mounts
}

// VolumeClaimTemplatesForPrivateAi returns the PersistentVolumeClaim templates
// required by the PrivateAi deployment when storage is requested.
func VolumeClaimTemplatesForPrivateAi(instance *privateaiv4.PrivateAi) []corev1.PersistentVolumeClaim {
	if instance.Spec.StorageClass == "" {
		return nil
	}

	quantity := resource.MustParse(fmt.Sprintf("%dGi", instance.Spec.StorageSizeInGb))

	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-oradata-vol4", instance.Name),
				Namespace:       instance.Namespace,
				OwnerReferences: getOwnerRef(instance),
				Labels:          buildLabelsForPrivateAi(instance),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: pointer.String(instance.Spec.StorageClass),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: quantity},
				},
			},
		},
	}
}

func buildEnvVarsForPrivateAI(
	instance *privateaiv4.PrivateAi,
	custom []privateaiv4.EnvironmentVariable,
	httpEnabled, httpsEnabled bool,
) []corev1.EnvVar {
	// Preserve order: custom vars first, then inferred ones when missing.
	envVars := make([]corev1.EnvVar, 0, len(custom)+4)
	seen := map[string]bool{}

	for _, envVar := range custom {
		if envVar.Name == "" {
			continue
		}
		envVars = append(envVars, corev1.EnvVar{Name: envVar.Name, Value: envVar.Value})
		seen[envVar.Name] = true
	}

	if instance.Spec.PaiConfigFile != nil && instance.Spec.PaiConfigFile.Name != "" && instance.Spec.PaiConfigFile.MountLocation != "" {
		ensureEnvVar(&envVars, seen, envConfigFile, instance.Spec.PaiConfigFile.MountLocation+"/config.json")
	}

	if instance.Spec.PaiSecret != nil && instance.Spec.PaiSecret.Name != "" && instance.Spec.PaiSecret.MountLocation != "" {
		ensureEnvVar(&envVars, seen, envSecretsMount, instance.Spec.PaiSecret.MountLocation)
	}

	ensureEnvVar(&envVars, seen, envHTTPEnabled, strconv.FormatBool(httpEnabled))
	ensureEnvVar(&envVars, seen, envHTTPSEnabled, strconv.FormatBool(httpsEnabled))
	ensureEnvVar(&envVars, seen, envAuthEnabled, strconv.FormatBool(boolFromString(instance.Spec.PaiEnableAuthentication)))

	return envVars
}

func ensureEnvVar(envs *[]corev1.EnvVar, seen map[string]bool, name, value string) {
	if name == "" || seen[name] {
		return
	}
	*envs = append(*envs, corev1.EnvVar{Name: name, Value: value})
	seen[name] = true
}

func getOwnerRefPrivateAI(instance *privateaiv4.PrivateAi) []metav1.OwnerReference {
	return []metav1.OwnerReference{*metav1.NewControllerRef(instance, privateaiv4.GroupVersion.WithKind("PrivateAI"))}
}

// BuildServiceDefForPrivateAi constructs a Kubernetes Service definition for
// the PrivateAi instance respecting the requested service type.
func BuildServiceDefForPrivateAi(instance *privateaiv4.PrivateAi, svcType string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: buildSvcObjectMetaForPrivateAi(instance, svcType),
		Spec:       corev1.ServiceSpec{Selector: buildLabelsForPrivateAi(instance)},
	}

	switch svcType {
	case "external":
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
		if instance.Spec.PaiLBExternalTrafficPolicy == "local" {
			service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
		} else {
			service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
		}
		service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
		augmentExternalService(service, instance)
	case "local":
		service.Spec.ClusterIP = corev1.ClusterIPNone
	}

	service.Spec.Ports = buildSvcPortsDef(instance)
	if instance.Spec.PaiLBIP != "" {
		service.Spec.LoadBalancerIP = instance.Spec.PaiLBIP
	}

	return service
}

func augmentExternalService(service *corev1.Service, instance *privateaiv4.PrivateAi) {
	plStatus := boolFromString(instance.Spec.PaiInternalLB)
	if !plStatus {
		return
	}

	if service.ObjectMeta.Annotations == nil {
		service.ObjectMeta.Annotations = map[string]string{}
	}

	for k, v := range instance.Spec.PailbAnnotation {
		service.ObjectMeta.Annotations[k] = v
	}

	if _, ok := service.ObjectMeta.Annotations["oci.oraclecloud.com/load-balancer-type"]; !ok {
		service.ObjectMeta.Annotations["oci.oraclecloud.com/load-balancer-type"] = "lb"
	}
	if _, ok := service.ObjectMeta.Annotations["service.beta.kubernetes.io/oci-load-balancer-internal"]; !ok {
		service.ObjectMeta.Annotations["service.beta.kubernetes.io/oci-load-balancer-internal"] = "true"
	}
}

func buildSvcObjectMetaForPrivateAi(instance *privateaiv4.PrivateAi, svcType string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:            GetSvcName(instance.Name, svcType),
		Namespace:       instance.Namespace,
		OwnerReferences: getOwnerRef(instance),
		Labels:          buildSvcLabelsForPrivateAi(instance, svcType),
	}
}

func buildSvcLabelsForPrivateAi(instance *privateaiv4.PrivateAi, svcType string) map[string]string {
	labels := buildLabelsForPrivateAi(instance)
	labels["app.kubernetes.io/servicetype"] = svcType
	return labels
}

// Update Section
// ManageReplicas reconciles the Deployment replica count with the desired
// specification and updates the PrivateAi status accordingly.
func ManageReplicas(
	r client.Reader,
	instance *privateaiv4.PrivateAi,
	kClient client.Client,
	config *rest.Config,
	deploy *appsv1.Deployment,
	podList *corev1.PodList,
	ctx context.Context,
	req ctrl.Request,
	logger logr.Logger,
) (ctrl.Result, error) {
	desired := replicasOrDefault(instance.Spec.Replicas)

	if deploy.Spec.Replicas != nil && *deploy.Spec.Replicas == desired {
		return ctrl.Result{}, nil
	}

	logger.Info("Deployment replicas mismatch. Updating deployment...")
	instance.Status.Status = privateaiv4.StatusUpdating
	if err := kClient.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	updated := deploy.DeepCopy()
	updated.Spec.Replicas = pointer.Int32(desired)
	if err := kClient.Update(context.Background(), updated); err != nil {
		LogMessages("ERROR", "Failed to update Deployment with new replica count", err, instance, logger)
		instance.Status.Status = privateaiv4.StatusError
		_ = kClient.Status().Update(ctx, instance)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// UpdateSvcForPrivateAI reconciles the Service definition and updates the
// PrivateAi status when changes are detected.
func UpdateSvcForPrivateAI(
	instance *privateaiv4.PrivateAi,
	paiSpec privateaiv4.PrivateAiSpec,
	kClient client.Client,
	config *rest.Config,
	newSvc *corev1.Service,
	oldSvc *corev1.Service,
	logger logr.Logger,
) (ctrl.Result, error) {
	_ = paiSpec

	if servicesEqual(newSvc, oldSvc) {
		return ctrl.Result{}, nil
	}

	LogMessages("INFO", "Svc definition change detected ...", nil, instance, logger)
	instance.Status.Status = privateaiv4.StatusUpdating
	if err := kClient.Status().Update(context.Background(), instance); err != nil {
		return ctrl.Result{}, err
	}

	if err := kClient.Update(context.Background(), newSvc); err != nil {
		LogMessages("ERROR", "Failed to update service spec", err, instance, logger)
		instance.Status.Status = privateaiv4.StatusError
		_ = kClient.Status().Update(context.Background(), instance)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func servicesEqual(a, b *corev1.Service) bool {
	return reflect.DeepEqual(a.ObjectMeta.Annotations, b.ObjectMeta.Annotations) &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Spec.LoadBalancerIP, b.Spec.LoadBalancerIP) &&
		reflect.DeepEqual(a.Spec.Ports, b.Spec.Ports)
}

// UpdateDeploySetForPrivateAI reconciles the running deployment with the
// desired container configuration defined in the custom resource.
func UpdateDeploySetForPrivateAI(
	instance *privateaiv4.PrivateAi,
	paiSpec privateaiv4.PrivateAiSpec,
	kClient client.Client,
	config *rest.Config,
	deploy *appsv1.Deployment,
	pod *corev1.Pod,
	logger logr.Logger,
) (ctrl.Result, error) {
	_ = config

	var needsUpdate bool

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Name != deploy.Name {
			continue
		}

		if paiSpec.Resources != nil && !reflect.DeepEqual(&container.Resources, paiSpec.Resources) {
			LogMessages("INFO", "Container resources have changed. Updating deployment...", nil, instance, logger)
			needsUpdate = true
		}

		if container.Image != paiSpec.PaiImage {
			LogMessages("INFO", "Container image changed. Updating deployment...", nil, instance, logger)
			needsUpdate = true
		}
	}

	if !needsUpdate {
		return ctrl.Result{}, nil
	}

	instance.Status.Status = privateaiv4.StatusUpdating
	if err := kClient.Status().Update(context.Background(), instance); err != nil {
		return ctrl.Result{}, err
	}

	if err := kClient.Update(context.Background(), BuildDeploySetForPrivateAI(instance)); err != nil {
		LogMessages("ERROR", "Failed to update deployment with new spec", err, instance, logger)
		instance.Status.Status = privateaiv4.StatusError
		_ = kClient.Status().Update(context.Background(), instance)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// UpdateRestartedAtAnnotation refreshes the restart timestamp annotation on the
// deployment pod template when Kubernetes has recorded a restart request.
func UpdateRestartedAtAnnotation(
	r client.Reader,
	instance *privateaiv4.PrivateAi,
	kClient client.Client,
	config *rest.Config,
	deploy *appsv1.Deployment,
	ctx context.Context,
	req ctrl.Request,
	logger logr.Logger,
) error {
	_ = r
	_ = config
	_ = req
	_ = logger

	depCopy := deploy.DeepCopy()
	annotations := depCopy.Spec.Template.Annotations
	if annotations == nil {
		return nil
	}

	if _, ok := annotations[restartAnnotationKey]; !ok {
		return nil
	}

	if depCopy.Spec.Template.Annotations == nil {
		depCopy.Spec.Template.Annotations = map[string]string{}
	}
	depCopy.Spec.Template.Annotations[restartAnnotationKey] = time.Now().Format(time.RFC3339)

	return kClient.Patch(context.Background(), depCopy, client.MergeFrom(deploy))
}

func buildTopologySpreadConstraintsForPrivateAI(instance *privateaiv4.PrivateAi) []corev1.TopologySpreadConstraint {
	return []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: buildLabelsForPrivateAi(instance),
			},
		},
	}
}

func getNodeAffinity(instance *privateaiv4.PrivateAi) *corev1.Affinity {
	term := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "kubernetes.io/hostname",
				Operator: corev1.NodeSelectorOpIn,
				Values:   instance.Spec.WorkerNodes,
			},
		},
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{term}},
		},
	}
}

func boolFromString(value string) bool {
	result, err := strconv.ParseBool(value)
	return err == nil && result
}

func int64Ptr(value int64) *int64 {
	return &value
}
