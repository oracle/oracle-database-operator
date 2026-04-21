package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	k8sobjects "github.com/oracle/oracle-database-operator/commons/k8sobject"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var trafficManagerRequeue = ctrl.Result{Requeue: true, RequeueAfter: 25 * time.Second}
var trafficManagerNoRequeue = ctrl.Result{}

type TrafficManagerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Config   *rest.Config
	Recorder record.EventRecorder
}

type associatedBackend struct {
	Name        string
	Path        string
	ServiceName string
	ServicePort int32
	UseHTTPS    bool
}

// +kubebuilder:rbac:groups=network.oracle.com,resources=trafficmanagers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=network.oracle.com,resources=trafficmanagers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=privateai.oracle.com,resources=privateais,verbs=get;list;watch

func (r *TrafficManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	inst := &networkv4.TrafficManager{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if apierrors.IsNotFound(err) {
			return trafficManagerNoRequeue, nil
		}
		return trafficManagerRequeue, err
	}

	backends, err := r.listAssociatedBackends(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		return trafficManagerRequeue, err
	}

	configData, err := buildManagedNginxConfig(inst, backends)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		return trafficManagerRequeue, err
	}
	configChecksum := checksumString(configData)
	tlsChecksum, err := r.resolveTLSSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		return trafficManagerRequeue, err
	}
	backendTLSChecksum, err := r.resolveBackendTLSSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		return trafficManagerRequeue, err
	}

	configMap := buildTrafficManagerConfigMap(inst, configData)
	if err := controllerutil.SetControllerReference(inst, configMap, r.Scheme); err != nil {
		return trafficManagerNoRequeue, err
	}
	if err := r.applyConfigMap(ctx, configMap); err != nil {
		return trafficManagerNoRequeue, err
	}

	deploy := buildTrafficManagerDeployment(inst, configChecksum, tlsChecksum, backendTLSChecksum)
	if err := controllerutil.SetControllerReference(inst, deploy, r.Scheme); err != nil {
		return trafficManagerNoRequeue, err
	}
	foundDeploy, depResult, err := k8sobjects.ReconcileDeployment(ctx, r.Client, inst.Namespace, deploy, syncTrafficManagerDeployment)
	if err != nil {
		return trafficManagerNoRequeue, err
	}

	if trafficManagerServiceEnabled(inst.Spec.Service.Internal.Enabled, true) {
		if err := r.ensureService(ctx, inst, "internal"); err != nil {
			return trafficManagerNoRequeue, err
		}
	} else if err := deleteServiceIfExists(ctx, r.Client, inst.Namespace, trafficManagerInternalServiceName(inst)); err != nil {
		return trafficManagerNoRequeue, err
	}
	if trafficManagerServiceEnabled(inst.Spec.Service.External.Enabled, false) {
		if err := r.ensureService(ctx, inst, "external"); err != nil {
			return trafficManagerNoRequeue, err
		}
	} else if err := deleteServiceIfExists(ctx, r.Client, inst.Namespace, trafficManagerExternalServiceName(inst)); err != nil {
		return trafficManagerNoRequeue, err
	}

	inst.Status.Status = privateaiv4.StatusReady
	inst.Status.Type = string(inst.Spec.Type)
	inst.Status.ReadyReplicas = foundDeploy.Status.ReadyReplicas
	inst.Status.InternalService = ""
	inst.Status.ExternalService = ""
	inst.Status.ExternalEndpoint = ""
	if trafficManagerServiceEnabled(inst.Spec.Service.Internal.Enabled, true) {
		inst.Status.InternalService = trafficManagerInternalServiceName(inst)
	}
	if trafficManagerServiceEnabled(inst.Spec.Service.External.Enabled, false) {
		inst.Status.ExternalService = trafficManagerExternalServiceName(inst)
		existing := &corev1.Service{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: trafficManagerExternalServiceName(inst), Namespace: inst.Namespace}, existing); err == nil &&
			len(existing.Status.LoadBalancer.Ingress) > 0 {
			ingress := existing.Status.LoadBalancer.Ingress[0]
			if ingress.IP != "" {
				inst.Status.ExternalEndpoint = ingress.IP
			} else {
				inst.Status.ExternalEndpoint = ingress.Hostname
			}
		}
	}
	if inst.Status.ExternalEndpoint != "" {
		inst.Status.ExternalEndpoint = trafficManagerURLBase(inst, inst.Status.ExternalEndpoint)
	}
	inst.Status.Nginx = &networkv4.NginxTrafficManagerStatus{
		ConfigMapName:      configMap.Name,
		AssociatedBackends: backendNames(backends),
		BackendCount:       int32(len(backends)),
		ConfigMode:         trafficManagerConfigMode(inst),
		TLSEnabled:         inst.Spec.Security.TLS.Enabled,
		TLSSecretName:      strings.TrimSpace(inst.Spec.Security.TLS.SecretName),
		BackendTLSEnabled:  backendTLSVerificationEnabled(inst),
		BackendTrustSecret: backendTLSTrustSecretName(inst),
		Routes:             buildNginxRouteStatuses(inst, backends, inst.Status.ExternalEndpoint),
	}
	if err := r.Status().Update(ctx, inst); err != nil {
		return trafficManagerNoRequeue, err
	}
	if depResult.Created {
		return trafficManagerRequeue, nil
	}
	if depResult.Updated {
		return trafficManagerRequeue, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *TrafficManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv4.TrafficManager{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&privateaiv4.PrivateAi{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			privateAi, ok := obj.(*privateaiv4.PrivateAi)
			if !ok {
				return nil
			}
			trafficManager := privateaiv4.EffectiveTrafficManager(&privateAi.Spec)
			if trafficManager == nil {
				return nil
			}
			if ref := strings.TrimSpace(trafficManager.Ref); ref != "" {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: ref, Namespace: privateAi.Namespace}}}
			}
			return nil
		})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return nil
			}
			list := &networkv4.TrafficManagerList{}
			if err := mgr.GetClient().List(ctx, list, client.InNamespace(secret.Namespace)); err != nil {
				return nil
			}
			requests := make([]reconcile.Request, 0)
			for i := range list.Items {
				item := &list.Items[i]
				if item.Spec.Type != networkv4.TrafficManagerTypeNginx {
					continue
				}
				if strings.TrimSpace(item.Spec.Security.TLS.SecretName) != secret.Name &&
					backendTLSTrustSecretName(item) != secret.Name {
					continue
				}
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: item.Name, Namespace: item.Namespace},
				})
			}
			return requests
		})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *TrafficManagerReconciler) listAssociatedBackends(ctx context.Context, inst *networkv4.TrafficManager) ([]associatedBackend, error) {
	list := &privateaiv4.PrivateAiList{}
	if err := r.List(ctx, list, client.InNamespace(inst.Namespace)); err != nil {
		return nil, err
	}
	backends := make([]associatedBackend, 0)
	seenPaths := map[string]string{}
	for i := range list.Items {
		item := &list.Items[i]
		trafficManager := privateaiv4.EffectiveTrafficManager(&item.Spec)
		if trafficManager == nil || strings.TrimSpace(trafficManager.Ref) != inst.Name {
			continue
		}
		path := strings.TrimSpace(trafficManager.RoutePath)
		if path == "" {
			path = fmt.Sprintf("/%s/v1/", strings.ToLower(strings.TrimSpace(item.Name)))
		}
		if other, exists := seenPaths[path]; exists {
			return nil, fmt.Errorf("duplicate traffic manager route path %q for backends %s and %s", path, other, item.Name)
		}
		seenPaths[path] = item.Name
		port := backendServicePort(item)
		backends = append(backends, associatedBackend{
			Name:        item.Name,
			Path:        path,
			ServiceName: backendServiceDNS(item),
			ServicePort: port,
			UseHTTPS:    !parseBoolFlag(item.Spec.PaiHTTPEnabled),
		})
	}
	sort.Slice(backends, func(i, j int) bool { return backends[i].Path < backends[j].Path })
	return backends, nil
}

func (r *TrafficManagerReconciler) applyConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	existing := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Client.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	existing.Data = cm.Data
	existing.Labels = cm.Labels
	existing.OwnerReferences = cm.OwnerReferences
	return r.Client.Update(ctx, existing)
}

func (r *TrafficManagerReconciler) ensureService(ctx context.Context, inst *networkv4.TrafficManager, serviceKind string) error {
	svc := buildTrafficManagerService(inst, serviceKind)
	if err := controllerutil.SetControllerReference(inst, svc, r.Scheme); err != nil {
		return err
	}
	_, err := k8sobjects.EnsureService(ctx, r.Client, inst.Namespace, svc, k8sobjects.ServiceSyncOptions{
		SyncOwnerReferences:    true,
		SyncLoadBalancerFields: true,
	})
	return err
}

func buildManagedNginxConfig(inst *networkv4.TrafficManager, backends []associatedBackend) (string, error) {
	var builder strings.Builder
	builder.WriteString("events {}\n")
	builder.WriteString("http {\n")
	builder.WriteString("    server {\n")
	if inst.Spec.Security.TLS.Enabled {
		builder.WriteString(fmt.Sprintf("        listen %d ssl;\n", trafficManagerContainerPort(inst)))
		builder.WriteString("        ssl_certificate /etc/nginx/tls/tls.crt;\n")
		builder.WriteString("        ssl_certificate_key /etc/nginx/tls/tls.key;\n")
		builder.WriteString("        ssl_protocols TLSv1.2 TLSv1.3;\n")
	} else {
		builder.WriteString(fmt.Sprintf("        listen %d;\n", trafficManagerContainerPort(inst)))
	}
	builder.WriteString("        location = /healthz { return 200 \"ok\"; }\n")
	if len(backends) == 0 {
		builder.WriteString("        location / { return 503; }\n")
	} else {
		for _, backend := range backends {
			if err := appendBackendLocation(&builder, inst, backend); err != nil {
				return "", err
			}
		}
	}
	builder.WriteString("    }\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}

func appendBackendLocation(builder *strings.Builder, inst *networkv4.TrafficManager, backend associatedBackend) error {
	pathExpr := regexp.QuoteMeta(strings.TrimSpace(backend.Path))
	if pathExpr == "" {
		return fmt.Errorf("backend %s has empty route path", backend.Name)
	}
	scheme := "https"
	if !backend.UseHTTPS {
		scheme = "http"
	}
	builder.WriteString(fmt.Sprintf("        location %s {\n", backend.Path))
	builder.WriteString(fmt.Sprintf("            rewrite ^%s?(.*)$ /v1/$1 break;\n", pathExpr))
	builder.WriteString(fmt.Sprintf("            proxy_pass %s://%s:%d;\n", scheme, backend.ServiceName, backend.ServicePort))
	if backend.UseHTTPS {
		builder.WriteString("            proxy_ssl_server_name on;\n")
		builder.WriteString(fmt.Sprintf("            proxy_ssl_name %s;\n", backend.ServiceName))
		if backendTLSVerificationEnabled(inst) {
			builder.WriteString(fmt.Sprintf("            proxy_ssl_trusted_certificate %s;\n", backendTLSFilePath(inst)))
			builder.WriteString("            proxy_ssl_verify on;\n")
		} else {
			builder.WriteString("            proxy_ssl_verify off;\n")
		}
	}
	builder.WriteString(fmt.Sprintf("            proxy_set_header Host %s;\n", backend.ServiceName))
	builder.WriteString("            proxy_set_header Authorization $http_authorization;\n")
	builder.WriteString("            proxy_set_header Content-Type $content_type;\n")
	builder.WriteString("            proxy_set_header Accept $http_accept;\n")
	builder.WriteString("            proxy_set_header X-Real-IP $remote_addr;\n")
	builder.WriteString("            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	builder.WriteString("            proxy_http_version 1.1;\n")
	builder.WriteString("            proxy_set_header Connection \"\";\n")
	builder.WriteString("            proxy_read_timeout 600s;\n")
	builder.WriteString("            proxy_send_timeout 600s;\n")
	builder.WriteString("        }\n")
	return nil
}

func buildTrafficManagerConfigMap(inst *networkv4.TrafficManager, config string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trafficManagerConfigMapName(inst),
			Namespace: inst.Namespace,
			Labels:    trafficManagerLabels(inst),
		},
		Data: map[string]string{"nginx.conf": config},
	}
}

func buildTrafficManagerDeployment(inst *networkv4.TrafficManager, configChecksum, tlsChecksum, backendTLSChecksum string) *appsv1.Deployment {
	labels := trafficManagerLabels(inst)
	configMountDir := trafficManagerConfigMountLocation(inst)
	configMountPath := path.Join(configMountDir, "nginx.conf")
	annotations := map[string]string{"network.oracle.com/config-hash": configChecksum}
	if tlsChecksum != "" {
		annotations["network.oracle.com/tls-secret-hash"] = tlsChecksum
	}
	if backendTLSChecksum != "" {
		annotations["network.oracle.com/backend-tls-secret-hash"] = backendTLSChecksum
	}
	volumeMounts := []corev1.VolumeMount{{
		Name:      "traffic-manager-config",
		MountPath: configMountPath,
		SubPath:   "nginx.conf",
		ReadOnly:  true,
	}}
	volumes := []corev1.Volume{{
		Name: "traffic-manager-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: trafficManagerConfigMapName(inst)},
				Items:                []corev1.KeyToPath{{Key: "nginx.conf", Path: "nginx.conf"}},
			},
		},
	}}
	if inst.Spec.Security.TLS.Enabled && strings.TrimSpace(inst.Spec.Security.TLS.SecretName) != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "traffic-manager-tls",
			MountPath: inst.Spec.Security.TLS.MountLocation,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "traffic-manager-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: inst.Spec.Security.TLS.SecretName},
			},
		})
	}
	if trustSecretName := backendTLSTrustSecretName(inst); trustSecretName != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "traffic-manager-backend-tls",
			MountPath: backendTLSMountLocation(inst),
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "traffic-manager-backend-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: trustSecretName},
			},
		})
	}
	container := corev1.Container{
		Name:            trafficManagerDeploymentName(inst),
		Image:           inst.Spec.Runtime.Image,
		ImagePullPolicy: imagePullPolicyOrDefault(inst.Spec.Runtime.ImagePullPolicy),
		Command:         []string{"nginx", "-g", "daemon off;"},
		Ports: []corev1.ContainerPort{{
			ContainerPort: trafficManagerContainerPort(inst),
			Protocol:      corev1.ProtocolTCP,
		}},
		VolumeMounts: volumeMounts,
		Env:          buildTrafficManagerEnvVars(inst),
	}
	if inst.Spec.Runtime.Resources != nil {
		container.Resources = *inst.Spec.Runtime.Resources
	}
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		Volumes:    volumes,
	}
	if len(inst.Spec.Runtime.ImagePullSecrets) > 0 {
		podSpec.ImagePullSecrets = make([]corev1.LocalObjectReference, 0, len(inst.Spec.Runtime.ImagePullSecrets))
		for _, name := range inst.Spec.Runtime.ImagePullSecrets {
			if strings.TrimSpace(name) == "" {
				continue
			}
			podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
		}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trafficManagerDeploymentName(inst),
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(inst.Spec.Runtime.Replicas),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: podSpec,
			},
		},
	}
}

func syncTrafficManagerDeployment(found, desired *appsv1.Deployment) bool {
	if found == nil || desired == nil {
		return false
	}
	updated := false

	if !reflect.DeepEqual(found.Labels, desired.Labels) {
		found.Labels = desired.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.OwnerReferences, desired.OwnerReferences) {
		found.OwnerReferences = desired.OwnerReferences
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Replicas, desired.Spec.Replicas) {
		found.Spec.Replicas = desired.Spec.Replicas
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Selector, desired.Spec.Selector) {
		found.Spec.Selector = desired.Spec.Selector
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Labels, desired.Spec.Template.Labels) {
		found.Spec.Template.Labels = desired.Spec.Template.Labels
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Annotations, desired.Spec.Template.Annotations) {
		found.Spec.Template.Annotations = desired.Spec.Template.Annotations
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) {
		found.Spec.Template.Spec.Volumes = desired.Spec.Template.Spec.Volumes
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.ImagePullSecrets, desired.Spec.Template.Spec.ImagePullSecrets) {
		found.Spec.Template.Spec.ImagePullSecrets = desired.Spec.Template.Spec.ImagePullSecrets
		updated = true
	}
	if !reflect.DeepEqual(found.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		found.Spec.Template.Spec.Containers = desired.Spec.Template.Spec.Containers
		updated = true
	}

	return updated
}

func buildTrafficManagerService(inst *networkv4.TrafficManager, serviceKind string) *corev1.Service {
	spec := inst.Spec.Service.Internal
	name := trafficManagerInternalServiceName(inst)
	svcType := corev1.ServiceTypeClusterIP
	if serviceKind == "external" {
		spec = inst.Spec.Service.External
		name = trafficManagerExternalServiceName(inst)
		if spec.ServiceType == "" {
			svcType = corev1.ServiceTypeLoadBalancer
		} else {
			svcType = spec.ServiceType
		}
	}
	if spec.Port == 0 {
		if serviceKind == "external" {
			if inst.Spec.Security.TLS.Enabled {
				spec.Port = 443
			} else {
				spec.Port = 80
			}
		} else {
			spec.Port = trafficManagerContainerPort(inst)
		}
	}
	if spec.TargetPort == 0 {
		spec.TargetPort = trafficManagerContainerPort(inst)
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   inst.Namespace,
			Labels:      trafficManagerLabels(inst),
			Annotations: map[string]string{},
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: trafficManagerLabels(inst),
			Ports: []corev1.ServicePort{{
				Name:       fmt.Sprintf("tcp-%d", spec.Port),
				Protocol:   corev1.ProtocolTCP,
				Port:       spec.Port,
				TargetPort: intstr.FromInt(int(spec.TargetPort)),
			}},
		},
	}
	for k, v := range spec.Annotations {
		service.Annotations[k] = v
	}
	return service
}

func backendNames(backends []associatedBackend) []string {
	out := make([]string, 0, len(backends))
	for _, backend := range backends {
		out = append(out, backend.Name)
	}
	return out
}

func backendServicePort(inst *privateaiv4.PrivateAi) int32 {
	for _, pm := range inst.Spec.PaiService.PortMappings {
		if pm.Port > 0 {
			return pm.Port
		}
	}
	if parseBoolFlag(inst.Spec.PaiHTTPEnabled) {
		if inst.Spec.PaiHTTPPort > 0 {
			return inst.Spec.PaiHTTPPort
		}
		return 8080
	}
	if inst.Spec.PaiHTTPSPort > 0 {
		return inst.Spec.PaiHTTPSPort
	}
	return 8443
}

func buildTrafficManagerEnvVars(inst *networkv4.TrafficManager) []corev1.EnvVar {
	if len(inst.Spec.Runtime.EnvVars) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, 0, len(inst.Spec.Runtime.EnvVars))
	for _, e := range inst.Spec.Runtime.EnvVars {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		out = append(out, corev1.EnvVar{Name: e.Name, Value: e.Value})
	}
	return out
}

func backendTLSTrustSecretName(inst *networkv4.TrafficManager) string {
	if inst == nil || inst.Spec.Security.BackendTLS == nil {
		return ""
	}
	return strings.TrimSpace(inst.Spec.Security.BackendTLS.TrustSecretName)
}

func backendTLSMountLocation(inst *networkv4.TrafficManager) string {
	if inst == nil || inst.Spec.Security.BackendTLS == nil || strings.TrimSpace(inst.Spec.Security.BackendTLS.MountLocation) == "" {
		return "/etc/nginx/backend-ca"
	}
	return strings.TrimSpace(inst.Spec.Security.BackendTLS.MountLocation)
}

func backendTLSTrustFileName(inst *networkv4.TrafficManager) string {
	if inst == nil || inst.Spec.Security.BackendTLS == nil || strings.TrimSpace(inst.Spec.Security.BackendTLS.TrustFileName) == "" {
		return "ca.crt"
	}
	return strings.TrimSpace(inst.Spec.Security.BackendTLS.TrustFileName)
}

func backendTLSFilePath(inst *networkv4.TrafficManager) string {
	return filepath.Join(backendTLSMountLocation(inst), backendTLSTrustFileName(inst))
}

func backendTLSVerificationEnabled(inst *networkv4.TrafficManager) bool {
	if inst == nil || inst.Spec.Security.BackendTLS == nil {
		return false
	}
	if inst.Spec.Security.BackendTLS.Verify == nil {
		return true
	}
	return *inst.Spec.Security.BackendTLS.Verify
}

func trafficManagerServiceEnabled(enabled *bool, defaultValue bool) bool {
	if enabled == nil {
		return defaultValue
	}
	return *enabled
}

func trafficManagerLabels(inst *networkv4.TrafficManager) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       inst.Name,
		"app.kubernetes.io/component":  "traffic-manager",
		"app.kubernetes.io/managed-by": "Oracle-Database-Operator",
	}
}

func trafficManagerConfigMapName(inst *networkv4.TrafficManager) string {
	if inst.Spec.Nginx != nil && inst.Spec.Nginx.Config != nil && strings.TrimSpace(inst.Spec.Nginx.Config.ConfigMapName) != "" {
		return strings.TrimSpace(inst.Spec.Nginx.Config.ConfigMapName)
	}
	return inst.Name + "-nginx"
}

func trafficManagerConfigMountLocation(inst *networkv4.TrafficManager) string {
	if inst.Spec.Nginx != nil && inst.Spec.Nginx.Config != nil && strings.TrimSpace(inst.Spec.Nginx.Config.MountLocation) != "" {
		return strings.TrimSpace(inst.Spec.Nginx.Config.MountLocation)
	}
	return "/etc/nginx"
}

func trafficManagerDeploymentName(inst *networkv4.TrafficManager) string {
	return inst.Name
}

func trafficManagerInternalServiceName(inst *networkv4.TrafficManager) string {
	return inst.Name
}

func trafficManagerExternalServiceName(inst *networkv4.TrafficManager) string {
	return inst.Name + "-ext"
}

func backendServiceDNS(inst *privateaiv4.PrivateAi) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", inst.Name, inst.Namespace)
}

func checksumString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func buildNginxRouteStatuses(inst *networkv4.TrafficManager, backends []associatedBackend, externalEndpoint string) []networkv4.NginxRouteStatus {
	routes := make([]networkv4.NginxRouteStatus, 0, len(backends))
	for _, backend := range backends {
		backendURL := fmt.Sprintf("%s://%s:%d", backendScheme(backend.UseHTTPS), backend.ServiceName, backend.ServicePort)
		publicURL := ""
		if externalEndpoint != "" {
			publicURL = strings.TrimRight(externalEndpoint, "/") + backend.Path
		}
		routes = append(routes, networkv4.NginxRouteStatus{
			Path:           backend.Path,
			BackendName:    backend.Name,
			BackendService: backend.ServiceName,
			BackendURL:     backendURL,
			PublicURL:      publicURL,
		})
	}
	return routes
}

func (r *TrafficManagerReconciler) resolveTLSSecretChecksum(ctx context.Context, inst *networkv4.TrafficManager) (string, error) {
	if !inst.Spec.Security.TLS.Enabled {
		return "", nil
	}
	secretName := strings.TrimSpace(inst.Spec.Security.TLS.SecretName)
	if secretName == "" {
		return "", fmt.Errorf("spec.security.tls.secretName must be set when TLS is enabled")
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: inst.Namespace}, secret); err != nil {
		return "", fmt.Errorf("failed to get TLS secret %s: %w", secretName, err)
	}
	crt, ok := secret.Data["tls.crt"]
	if !ok || len(crt) == 0 {
		return "", fmt.Errorf("TLS secret %s is missing tls.crt", secretName)
	}
	key, ok := secret.Data["tls.key"]
	if !ok || len(key) == 0 {
		return "", fmt.Errorf("TLS secret %s is missing tls.key", secretName)
	}
	payload := make([]byte, 0, len(crt)+len(key))
	payload = append(payload, crt...)
	payload = append(payload, key...)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (r *TrafficManagerReconciler) resolveBackendTLSSecretChecksum(ctx context.Context, inst *networkv4.TrafficManager) (string, error) {
	secretName := backendTLSTrustSecretName(inst)
	if secretName == "" {
		return "", nil
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: inst.Namespace}, secret); err != nil {
		return "", fmt.Errorf("failed to get backend TLS trust secret %s: %w", secretName, err)
	}
	trustFileName := backendTLSTrustFileName(inst)
	trustFile, ok := secret.Data[trustFileName]
	if !ok || len(trustFile) == 0 {
		return "", fmt.Errorf("backend TLS trust secret %s is missing %s", secretName, trustFileName)
	}
	sum := sha256.Sum256(trustFile)
	return hex.EncodeToString(sum[:]), nil
}

func imagePullPolicyOrDefault(policy corev1.PullPolicy) corev1.PullPolicy {
	if policy == "" {
		return corev1.PullIfNotPresent
	}
	return policy
}

func trafficManagerURLBase(inst *networkv4.TrafficManager, host string) string {
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", backendScheme(inst.Spec.Security.TLS.Enabled), host)
}

func trafficManagerConfigMode(inst *networkv4.TrafficManager) string {
	if inst.Spec.Type == networkv4.TrafficManagerTypeNginx {
		return "Managed"
	}
	return ""
}

func backendScheme(useHTTPS bool) string {
	if useHTTPS {
		return "https"
	}
	return "http"
}

func trafficManagerContainerPort(inst *networkv4.TrafficManager) int32 {
	if inst.Spec.Service.Internal.TargetPort > 0 {
		return inst.Spec.Service.Internal.TargetPort
	}
	if inst.Spec.Service.External.TargetPort > 0 {
		return inst.Spec.Service.External.TargetPort
	}
	if inst.Spec.Security.TLS.Enabled {
		return 8443
	}
	return 8080
}

func parseBoolFlag(flag string) bool {
	val, err := strconv.ParseBool(flag)
	if err != nil {
		return false
	}
	return val
}

func deleteServiceIfExists(ctx context.Context, c client.Client, namespace, name string) error {
	if name == "" {
		return nil
	}
	svc := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := c.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
