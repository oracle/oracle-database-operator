package controllers

import (
	"context"
	apiv1 "github.com/oracle/oracle-database-operator/apis/observability/v1alpha1"
	"github.com/oracle/oracle-database-operator/commons/observability"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

/*
This handler file contains all the methods that
retrieve/find and create all related resources
on Kubernetes.
*/

type ObservabilityConfigResource struct{}
type ObservabilityDeploymentResource struct{}
type ObservabilityServiceResource struct{}
type ObservabilityGrafanaJsonResource struct{}
type ObservabilityServiceMonitorResource struct{}

type ObservabilityResource interface {
	generate(*apiv1.DatabaseObserver, *runtime.Scheme) (*unstructured.Unstructured, error)
	identify() (string, string, schema.GroupVersionKind)
}

func (r *DatabaseObserverReconciler) isExporterDeploymentReady(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) bool {

	// get latest deployment
	dep := &appsv1.Deployment{}
	rName := observability.DefaultExporterDeploymentPrefix + api.Name

	//  defer update for status changes below
	defer r.updateStatusAndRetrieve(api, ctx, req)
	if err := r.Get(context.TODO(), types.NamespacedName{Name: rName, Namespace: api.Namespace}, dep); err != nil {
		r.Log.Error(err, "Failed to fetch deployment for validating readiness")
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersDeployValidation,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve deployment for validation of observability exporter deployment readiness",
		})
		return false
	}

	labels := dep.Spec.Template.Labels
	cLabels := client.MatchingLabels{}
	for k, v := range labels {
		cLabels[k] = v
	}

	pods := &corev1.PodList{}
	if err := r.List(context.TODO(), pods, []client.ListOption{cLabels}...); err != nil {
		r.Log.Error(err, "Failed to fetch list of observability exporter pods")
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersDeployValidation,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve pods for validation of observability exporter deployment readiness",
		})
		return false
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			r.Log.Info("Observability exporter pod(s) found but not ready")
			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    observability.PhaseExportersDeployValidation,
				Status:  metav1.ConditionFalse,
				Reason:  observability.ReasonValidationFailure,
				Message: "Validation of observability exporter deployment readiness failed",
			})
			return false
		}
	}

	meta.RemoveStatusCondition(&api.Status.Conditions, observability.PhaseExportersDeployValidation)
	return true
}

func (r *DatabaseObserverReconciler) compareOrUpdateDeployment(or ObservabilityResource, api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) error {
	var updated bool

	// get desired
	desiredObj, genErr := or.generate(api, r.Scheme)
	if genErr != nil {
		r.Log.WithName(observability.LogExportersDeploy).Error(genErr, "Failed to generate deployment")
		return genErr
	}
	desiredDeployment := &appsv1.Deployment{}
	if err := r.Scheme.Convert(desiredObj, desiredDeployment, nil); err != nil {
		r.Log.WithName(observability.LogExportersDeploy).Error(genErr, "Failed to convert generated deployment")
		return err
	}

	// defer for updates to status below
	defer r.updateStatusAndRetrieve(api, ctx, req)
	// retrieve existing deployment
	foundDeployment := &appsv1.Deployment{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundDeployment); err != nil {
		r.Log.WithName(observability.LogExportersDeploy).Error(genErr, "Failed to retrieve deployment")
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersDeployUpdate,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve deployment: " + desiredObj.GetName(),
		})
		return err
	}

	// check containerImage
	foundContainerImage := foundDeployment.Spec.Template.Spec.Containers[0].Image
	desiredContainerImage := desiredDeployment.Spec.Template.Spec.Containers[0].Image
	if foundContainerImage != desiredContainerImage {
		r.Log.WithName(observability.LogExportersDeploy).Info("Updating deployment container image")
		foundDeployment.Spec.Template.Spec.Containers[0].Image = desiredContainerImage
		updated = true
	}

	// check config-volume
	var foundConfigmapName, desiredConfigmapName string
	desiredVolumes := desiredDeployment.Spec.Template.Spec.Volumes
	for _, v := range desiredVolumes {
		if v.Name == "config-volume" {
			desiredConfigmapName = v.ConfigMap.Name
		}
	}

	foundVolumes := foundDeployment.Spec.Template.Spec.Volumes
	for _, v := range foundVolumes {
		if v.Name == "config-volume" {
			foundConfigmapName = v.ConfigMap.Name
		}
	}

	if desiredConfigmapName != foundConfigmapName {
		r.Log.WithName(observability.LogExportersDeploy).Info("Updating deployment volumes with new configuration configmap")
		foundDeployment.Spec.Template.Spec.Volumes = observability.GetExporterDeploymentVolumes(api)
		updated = true
	}

	// make the update
	if updated {
		if err := r.Update(context.TODO(), foundDeployment); err != nil {
			r.Log.WithName(observability.LogExportersDeploy).Error(err, "Failed to update deployment", "name", desiredObj.GetName())
			r.Recorder.Event(api, corev1.EventTypeWarning, observability.ReasonUpdateFailure, "Failed to update deployment: "+desiredObj.GetName())
			return err
		}

		if desiredConfigmapName != foundConfigmapName { // if configmap was updated
			api.Status.ExporterConfig = desiredConfigmapName
		}
		r.Recorder.Event(api, corev1.EventTypeNormal, observability.ReasonUpdateSuccess, "Succeeded updating deployment: "+desiredObj.GetName())
	}

	meta.RemoveStatusCondition(&api.Status.Conditions, observability.PhaseExportersDeployUpdate) // Clear any conditions
	return nil
}

func (r *DatabaseObserverReconciler) manageConfigmap(api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// retrieve spec information related to resource,
	cName := api.Spec.Exporter.ExporterConfig.Configmap.Name
	rName := observability.DefaultExporterConfigmapPrefix + api.Name
	rKey := observability.DefaultConfigurationConfigmapKey
	metricsConfigData := observability.DefaultConfig

	var nameToFind string
	if cName != "" {
		nameToFind = cName
	} else {
		nameToFind = rName
	}

	// generate
	crOwnedConfigmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Namespace: api.Namespace,
		},
		Data: map[string]string{
			rKey: metricsConfigData,
		},
	}

	// defer for updates to status below
	defer r.updateStatusAndRetrieve(api, ctx, req)

	// set CR as owner
	if err := controllerutil.SetControllerReference(api, crOwnedConfigmap, r.Scheme); err != nil {
		r.Log.WithName(observability.LogExportersConfigMap).Error(err, "Failed to set controller reference for configmap", "name", nameToFind)
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersConfigMap,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonSetFailure,
			Message: "Failed to set owner reference for configmap",
		})
		return ctrl.Result{}, err
	}

	// find resource
	foundConfigmap := &corev1.ConfigMap{}
	getErr := r.Get(ctx, types.NamespacedName{Name: nameToFind, Namespace: req.Namespace}, foundConfigmap)

	// if resource not found and custom configmap was not provided, create resource
	if getErr != nil && errors.IsNotFound(getErr) && cName == "" {
		r.Log.WithName(observability.LogExportersConfigMap).Info("Creating exporter configuration configmap resource", "name", nameToFind)
		if err := r.Create(context.TODO(), crOwnedConfigmap); err != nil { // create
			r.Log.WithName(observability.LogExportersConfigMap).Error(err, "Failed to create configmap", "name", nameToFind)
			r.Recorder.Event(api, corev1.EventTypeWarning, observability.ReasonCreateFailure, "Failed creating configmap: "+rName)

			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    observability.PhaseExportersConfigMap,
				Status:  metav1.ConditionFalse,
				Reason:  observability.ReasonCreateFailure,
				Message: "Failed to create configmap " + nameToFind,
			})
			return ctrl.Result{}, err
		}
		r.Recorder.Event(api, corev1.EventTypeNormal, observability.ReasonCreateSuccess, "Succeeded creating configmap: "+rName)

	} else if getErr != nil && errors.IsNotFound(getErr) && cName != "" { // if custom configmap was not found
		r.Log.WithName(observability.LogExportersConfigMap).Error(getErr, "Failed to retrieve custom configmap", "name", cName)
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersConfigMap,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve custom configmap " + nameToFind,
		})
		return ctrl.Result{}, getErr

	} else if getErr != nil { // if an error occurred
		r.Log.WithName(observability.LogExportersConfigMap).Error(getErr, "Failed to retrieve due to some error", "name", nameToFind)
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    observability.PhaseExportersConfigMap,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve configmap: " + nameToFind,
		})
		return ctrl.Result{}, getErr

	}

	// refresh cr before changes
	if err := r.Get(context.TODO(), req.NamespacedName, api); err != nil {
		r.Log.Error(err, "Failed to fetch updated CR")
	}

	// create event if exporter config needs to be updated
	if getErr == nil && cName != "" && api.Status.ExporterConfig != nameToFind {
		r.Recorder.Event(api, corev1.EventTypeNormal, observability.ReasonRetrieveSuccess, "Succeeded retrieving custom configmap: "+nameToFind)
	}

	api.Status.ExporterConfig = nameToFind
	meta.RemoveStatusCondition(&api.Status.Conditions, observability.PhaseExportersConfigMap) // Clear any conditions
	return ctrl.Result{}, nil

}
func (r *DatabaseObserverReconciler) manageResource(or ObservabilityResource, api *apiv1.DatabaseObserver, ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	resourceType, logName, gvk := or.identify()

	// generate desired object based on api.Spec
	desiredObj, genErr := or.generate(api, r.Scheme)
	if genErr != nil {
		r.Log.WithName(logName).Error(genErr, "Failed to generate "+gvk.Kind)
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    resourceType,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonSetFailure,
			Message: "Failed to generate " + gvk.Kind,
		})
		return ctrl.Result{}, genErr
	}

	// retrieve object if it exists
	foundObj := &unstructured.Unstructured{}
	foundObj.SetGroupVersionKind(gvk)
	getErr := r.Get(context.TODO(), types.NamespacedName{Name: desiredObj.GetName(), Namespace: req.Namespace}, foundObj)

	// defer for updates to status below
	defer r.updateStatusAndRetrieve(api, ctx, req)

	// if resource not found, create resource then return
	if getErr != nil && errors.IsNotFound(getErr) {
		r.Log.WithName(logName).Info("Creating resource "+gvk.Kind, "name", desiredObj.GetName())

		if err := r.Create(context.TODO(), desiredObj); err != nil { // create
			r.Log.WithName(logName).Error(err, "Failed to create "+gvk.Kind, "name", desiredObj.GetName())
			r.Recorder.Event(api, corev1.EventTypeWarning, observability.ReasonCreateFailure, "Failed creating resource "+gvk.Kind+": "+desiredObj.GetName())
			meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
				Type:    resourceType,
				Status:  metav1.ConditionFalse,
				Reason:  observability.ReasonCreateFailure,
				Message: "Failed to create " + gvk.Kind + ": " + desiredObj.GetName(),
			})
			return ctrl.Result{}, err
		}

		r.Recorder.Event(api, corev1.EventTypeNormal, observability.ReasonCreateSuccess, "Succeeded creating "+gvk.Kind+": "+desiredObj.GetName())

	} else if getErr != nil { // if an error occurred
		r.Log.WithName(logName).Error(getErr, "Failed to retrieve "+desiredObj.GetKind(), "name", desiredObj.GetName())
		meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
			Type:    resourceType,
			Status:  metav1.ConditionFalse,
			Reason:  observability.ReasonRetrieveFailure,
			Message: "Failed to retrieve " + gvk.Kind + ": " + desiredObj.GetName(),
		})
		return ctrl.Result{}, getErr
	}

	// if it does exist, if it's not the same, update with details from desired object
	meta.RemoveStatusCondition(&api.Status.Conditions, resourceType) // Clear any conditions
	return ctrl.Result{}, nil
}

func (resource *ObservabilityDeploymentResource) generate(api *apiv1.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := observability.DefaultExporterDeploymentPrefix + api.Name
	rContainerName := observability.DefaultExporterContainerName
	rContainerImage := observability.GetExporterImage(api)
	rVolumes := observability.GetExporterDeploymentVolumes(api)
	rVolumeMounts := observability.GetExporterDeploymentVolumeMounts(api)
	rSelectors := observability.GetExporterSelector(api)
	rReplicas := observability.GetExporterReplicas(api)
	rEnvs := observability.GetExporterEnvs(api)

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
	rLabels := observability.GetExporterLabels(api)
	rPort := observability.GetExporterServicePort(api)
	rSelector := observability.GetExporterSelector(api)

	obj := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rServiceName,
			Labels:    rLabels,
			Namespace: api.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     "NodePort",
			Selector: rSelector,
			Ports: []corev1.ServicePort{
				{
					Name: "metrics",
					Port: rPort,
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
	rName := observability.DefaultServicemonitorPrefix + api.Name
	rLabels := observability.GetExporterLabels(api)
	rSelector := observability.GetExporterSelector(api)
	rPort := observability.GetExporterServiceMonitorPort(api)
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

	if err := controllerutil.SetControllerReference(api, obj, scheme); err != nil {
		return nil, err
	}

	var u = &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}
func (resource *ObservabilityGrafanaJsonResource) generate(api *apiv1.DatabaseObserver, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	rName := observability.DefaultGrafanaConfigMapNamePrefix + api.Name
	grafanaJSON, err := observability.GetGrafanaJSONData(api)
	if err != nil {
		return nil, err
	}

	obj := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rName,
			Namespace: api.Namespace,
		},
		Data: map[string]string{observability.DefaultGrafanaConfigmapKey: string(grafanaJSON)},
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

func (resource *ObservabilityDeploymentResource) identify() (string, string, schema.GroupVersionKind) {
	return observability.PhaseExportersDeploy, observability.LogExportersDeploy, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
}
func (resource *ObservabilityServiceResource) identify() (string, string, schema.GroupVersionKind) {
	return observability.PhaseExportersSVC, observability.LogExportersSVC, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	}
}
func (resource *ObservabilityServiceMonitorResource) identify() (string, string, schema.GroupVersionKind) {
	return observability.PhaseExportersServiceMonitor, observability.LogExportersServiceMonitor, schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}
}
func (resource *ObservabilityGrafanaJsonResource) identify() (string, string, schema.GroupVersionKind) {
	return observability.PhaseExportersGrafanaConfigMap, observability.LogExportersGrafanaConfigMap, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
}
