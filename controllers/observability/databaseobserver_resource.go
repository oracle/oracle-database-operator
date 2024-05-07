package controllers

import (
	apiv1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	constants "github.com/oracle/oracle-database-operator/commons/observability"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

/*
This handler file contains all the methods that
retrieve/find and create all related resources
on Kubernetes.
*/

type ObservabilityDeploymentResource struct{}
type ObservabilityServiceResource struct{}
type ObservabilityServiceMonitorResource struct{}

type ObserverResource interface {
	generate(*apiv1.DatabaseObserver, *runtime.Scheme) (*unstructured.Unstructured, error)
	identify() (string, string, schema.GroupVersionKind)
}

func (resource *ObservabilityDeploymentResource) generate(api *apiv1.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := constants.DefaultExporterDeploymentPrefix + api.Name
	rContainerName := constants.DefaultExporterContainerName
	rContainerImage := constants.GetExporterImage(api)
	rVolumes := constants.GetExporterDeploymentVolumes(api)
	rVolumeMounts := constants.GetExporterDeploymentVolumeMounts(api)
	rSelectors := constants.GetExporterSelector(api)
	rReplicas := constants.GetExporterReplicas(api)
	rEnvs := constants.GetExporterEnvs(api)

	rPort := []corev1.ContainerPort{
		{ContainerPort: 8080},
	}

	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Namespace: api.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: rSelectors,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: rSelectors,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           rContainerImage,
						ImagePullPolicy: corev1.PullAlways,
						Name:            rContainerName,
						Env:             rEnvs,
						VolumeMounts:    rVolumeMounts,
						Ports:           rPort,
					}},
					RestartPolicy: corev1.RestartPolicyAlways,
					Volumes:       rVolumes,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(api, obj, scheme); err != nil {
		return nil, err
	}

	var u = &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}

func (resource *ObservabilityServiceResource) generate(api *apiv1.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rServiceName := "obs-svc-" + api.Name
	rLabels := constants.GetExporterLabels(api)
	rPort := constants.GetExporterServicePort(api)
	rSelector := constants.GetExporterSelector(api)

	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rServiceName,
			Labels:    rLabels,
			Namespace: api.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     "ClusterIP",
			Selector: rSelector,
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       rPort,
					TargetPort: intstr.FromInt32(constants.DefaultServiceTargetPort),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(api, obj, scheme); err != nil {
		return nil, err
	}

	var u = &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}

func (resource *ObservabilityServiceMonitorResource) generate(api *apiv1.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := constants.DefaultServiceMonitorPrefix + api.Name
	rLabels := constants.GetExporterLabels(api)
	rSelector := constants.GetExporterSelector(api)
	rPort := constants.GetExporterServiceMonitorPort(api)
	rInterval := "20s"

	obj := &monitorv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Labels:    rLabels,
			Namespace: api.Namespace,
		},
		Spec: monitorv1.ServiceMonitorSpec{
			Endpoints: []monitorv1.Endpoint{{
				Interval: monitorv1.Duration(rInterval),
				Port:     rPort,
			}},
			Selector: metav1.LabelSelector{
				MatchLabels: rSelector,
			},
		},
	}

	// set reference
	if e := controllerutil.SetControllerReference(api, obj, scheme); e != nil {
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
