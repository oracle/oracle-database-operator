package controllers

import (
	api "github.com/oracle/oracle-database-operator/apis/observability/v4"
	k8sutil "github.com/oracle/oracle-database-operator/commons/k8sutil"
	constants "github.com/oracle/oracle-database-operator/commons/observability"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

/*
This handler file contains all the methods that
retrieve/find and create all related resources
on Kubernetes.
*/

// ObservabilityDeploymentResource manages the exporter Deployment desired state.
type ObservabilityDeploymentResource struct{}

// ObservabilityServiceResource manages the exporter Service desired state.
type ObservabilityServiceResource struct{}

// ObservabilityServiceMonitorResource manages the exporter ServiceMonitor desired state.
type ObservabilityServiceMonitorResource struct{}

// ObserverResource describes a reconcilable child resource in DatabaseObserver flows.
type ObserverResource interface {
	generate(*api.DatabaseObserver, *runtime.Scheme) (*unstructured.Unstructured, error)
	identify() (string, string, schema.GroupVersionKind)
}

// getServiceMonitorEndpoints builds generated ServiceMonitor endpoints from
// the DatabaseObserver ServiceMonitor policy allowlist.
func getServiceMonitorEndpoints(a *api.DatabaseObserver) []monitorv1.Endpoint {
	if len(a.Spec.ServiceMonitor.Endpoints) == 0 {
		return []monitorv1.Endpoint{{
			Port:     constants.DefaultPrometheusPort,
			Interval: "20s",
		}}
	}

	endpoints := make([]monitorv1.Endpoint, 0, len(a.Spec.ServiceMonitor.Endpoints))
	for _, endpoint := range a.Spec.ServiceMonitor.Endpoints {
		allowedEndpoint := monitorv1.Endpoint{
			Port:          endpoint.Port,
			Path:          endpoint.Path,
			Interval:      endpoint.Interval,
			ScrapeTimeout: endpoint.ScrapeTimeout,
		}

		if endpoint.Scheme != nil {
			scheme := *endpoint.Scheme
			allowedEndpoint.Scheme = &scheme
		}

		if endpoint.Params != nil {
			// Copy map and slice values so the generated object has independent state.
			allowedEndpoint.Params = make(map[string][]string, len(endpoint.Params))
			for key, values := range endpoint.Params {
				allowedEndpoint.Params[key] = append([]string(nil), values...)
			}
		}

		if endpoint.RelabelConfigs != nil {
			allowedEndpoint.RelabelConfigs = make([]monitorv1.RelabelConfig, len(endpoint.RelabelConfigs))
			for i := range endpoint.RelabelConfigs {
				endpoint.RelabelConfigs[i].DeepCopyInto(&allowedEndpoint.RelabelConfigs[i])
			}
		}

		if endpoint.MetricRelabelConfigs != nil {
			allowedEndpoint.MetricRelabelConfigs = make([]monitorv1.RelabelConfig, len(endpoint.MetricRelabelConfigs))
			for i := range endpoint.MetricRelabelConfigs {
				endpoint.MetricRelabelConfigs[i].DeepCopyInto(&allowedEndpoint.MetricRelabelConfigs[i])
			}
		}

		endpoints = append(endpoints, allowedEndpoint)
	}

	return endpoints
}

func (resource *ObservabilityDeploymentResource) generate(a *api.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := a.Name
	rContainerName := constants.DefaultExporterContainerName
	rContainerImage := constants.GetExporterImage(a)
	rArgs := constants.GetExporterArgs(a)
	rCommands := constants.GetExporterCommands(a)
	rVolumes := constants.GetExporterDeploymentVolumes(a)
	rVolumeMounts := constants.GetExporterDeploymentVolumeMounts(a)

	rReplicas := constants.GetExporterReplicas(a)
	rEnvs := constants.GetExporterEnvs(a)

	rLabels := constants.GetLabels(a, a.Spec.Deployment.Labels)
	rPodLabels := constants.GetLabels(a, a.Spec.Deployment.DeploymentPodTemplate.Labels)
	rSelector := constants.GetSelectorLabel(a)

	rDeploymentSecurityContext := constants.GetExporterDeploymentSecurityContext(a)
	rPodSecurityContext := constants.GetExporterPodSecurityContext(a)

	rPort := []corev1.ContainerPort{
		{ContainerPort: constants.DefaultAppPort},
	}

	// exporterContainer
	rContainers := make([]corev1.Container, 1)
	rContainers[0] = corev1.Container{
		Image:           rContainerImage,
		ImagePullPolicy: corev1.PullAlways,
		Name:            rContainerName,
		Env:             rEnvs,
		VolumeMounts:    rVolumeMounts,
		Ports:           rPort,
		Args:            rArgs,
		Command:         rCommands,
		SecurityContext: rDeploymentSecurityContext,
	}

	constants.AddSidecarContainers(a, &rContainers)
	constants.AddSidecarVolumes(a, &rVolumes)

	// additionalContainers

	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Namespace: a.Namespace,
			Labels:    rLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: rSelector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: rPodLabels,
				},
				Spec: corev1.PodSpec{
					Containers:                   rContainers,
					RestartPolicy:                corev1.RestartPolicyAlways,
					Volumes:                      rVolumes,
					SecurityContext:              rPodSecurityContext,
					AutomountServiceAccountToken: k8sutil.BoolOrDefault(a.Spec.AutomountServiceAccountToken, false),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(a, obj, scheme); err != nil {
		return nil, err
	}

	var u = &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}

func (resource *ObservabilityServiceResource) generate(a *api.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rServiceName := a.Name
	rLabels := constants.GetLabels(a, a.Spec.Service.Labels)
	rSelector := constants.GetSelectorLabel(a)
	rPorts := constants.GetExporterServicePort(a)

	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rServiceName,
			Labels:    rLabels,
			Namespace: a.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     constants.DefaultServiceType,
			Selector: rSelector,
			Ports:    rPorts,
		},
	}

	if err := controllerutil.SetControllerReference(a, obj, scheme); err != nil {
		return nil, err
	}

	var u = &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}

func (resource *ObservabilityServiceMonitorResource) generate(a *api.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := a.Name
	rEndpoints := getServiceMonitorEndpoints(a)

	rSelector := constants.GetSelectorLabel(a)
	rLabels := constants.GetLabels(a, a.Spec.ServiceMonitor.Labels)

	smSpec := monitorv1.ServiceMonitorSpec{
		Endpoints: rEndpoints,
		Selector: metav1.LabelSelector{
			MatchLabels: rSelector,
		},
	}

	obj := &monitorv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Labels:    rLabels,
			Namespace: a.Namespace,
		},
		Spec: smSpec,
	}

	// set reference
	if e := controllerutil.SetControllerReference(a, obj, scheme); e != nil {
		return nil, e
	}

	// convert
	var u = &unstructured.Unstructured{}
	if e := scheme.Convert(obj, u, nil); e != nil {
		return nil, e
	}

	return u, nil
}

func (resource *ObservabilityDeploymentResource) identify() (string, string, schema.GroupVersionKind) {
	return constants.IsExporterDeploymentReady, constants.LogExportersDeploy, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
}

func (resource *ObservabilityServiceResource) identify() (string, string, schema.GroupVersionKind) {
	return constants.IsExporterServiceReady, constants.LogExportersSVC, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	}
}

func (resource *ObservabilityServiceMonitorResource) identify() (string, string, schema.GroupVersionKind) {
	return constants.IsExporterServiceMonitorReady, constants.LogExportersServiceMonitor, schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}
}
