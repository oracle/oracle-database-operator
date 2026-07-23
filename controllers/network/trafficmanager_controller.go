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
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
	log := logf.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	inst := &networkv4.TrafficManager{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found")
			return trafficManagerNoRequeue, nil
		}
		log.Error(err, "failed to fetch TrafficManager")
		return trafficManagerRequeue, err
	}
	log.Info("reconcile started", "type", inst.Spec.Type)
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		return r.reconcileCman(ctx, log, inst)
	}

	backends, err := r.listAssociatedBackends(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to discover associated backends")
		return trafficManagerRequeue, err
	}
	log.Info("resolved associated backends",
		"backendCount", len(backends),
		"backends", describeAssociatedBackends(backends),
		"frontendTLSEnabled", inst.Spec.Security.TLS.Enabled,
		"backendTLSVerificationEnabled", backendTLSVerificationEnabled(inst),
		"backendTrustSecret", backendTLSTrustSecretName(inst),
	)

	configData, err := buildManagedNginxConfig(inst, backends)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to build managed nginx config")
		return trafficManagerRequeue, err
	}
	configChecksum := checksumString(configData)
	tlsChecksum, err := r.resolveTLSSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to resolve frontend TLS secret checksum", "secretName", strings.TrimSpace(inst.Spec.Security.TLS.SecretName))
		return trafficManagerRequeue, err
	}
	backendTLSChecksum, err := r.resolveBackendTLSSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to resolve backend TLS trust secret checksum", "secretName", backendTLSTrustSecretName(inst))
		return trafficManagerRequeue, err
	}
	log.Info("rendered managed nginx config",
		"configMapName", trafficManagerConfigMapName(inst),
		"configChecksum", configChecksum,
		"frontendTLSChecksumPresent", tlsChecksum != "",
		"backendTLSChecksumPresent", backendTLSChecksum != "",
	)

	configMap := buildTrafficManagerConfigMap(inst, configData)
	if err := controllerutil.SetControllerReference(inst, configMap, r.Scheme); err != nil {
		log.Error(err, "failed to set controller reference on configmap", "configMap", configMap.Name)
		return trafficManagerNoRequeue, err
	}
	if err := r.applyConfigMap(ctx, inst, configMap); err != nil {
		log.Error(err, "failed to apply configmap", "configMap", configMap.Name)
		return trafficManagerNoRequeue, err
	}
	log.Info("configmap reconciled", "configMap", configMap.Name)

	deploy := buildTrafficManagerDeployment(inst, configChecksum, tlsChecksum, backendTLSChecksum)
	if err := controllerutil.SetControllerReference(inst, deploy, r.Scheme); err != nil {
		log.Error(err, "failed to set controller reference on deployment", "deployment", deploy.Name)
		return trafficManagerNoRequeue, err
	}
	foundDeploy, depResult, err := k8sobjects.ReconcileDeployment(ctx, r.Client, inst.Namespace, deploy, syncTrafficManagerDeployment)
	if err != nil {
		log.Error(err, "failed to reconcile deployment", "deployment", deploy.Name)
		return trafficManagerNoRequeue, err
	}
	log.Info("deployment reconciled",
		"deployment", deploy.Name,
		"created", depResult.Created,
		"updated", depResult.Updated,
		"readyReplicas", foundDeploy.Status.ReadyReplicas,
		"desiredReplicas", ptr.Deref(deploy.Spec.Replicas, int32(0)),
	)

	if trafficManagerServiceEnabled(inst.Spec.Service.Internal.Enabled, true) {
		if err := r.ensureService(ctx, inst, "internal"); err != nil {
			log.Error(err, "failed to reconcile internal service", "service", trafficManagerInternalServiceName(inst))
			return trafficManagerNoRequeue, err
		}
		log.Info("internal service reconciled", "service", trafficManagerInternalServiceName(inst))
	} else if err := deleteServiceIfExists(ctx, r.Client, inst, trafficManagerInternalServiceName(inst)); err != nil {
		log.Error(err, "failed to delete internal service", "service", trafficManagerInternalServiceName(inst))
		return trafficManagerNoRequeue, err
	} else {
		log.Info("internal service disabled", "service", trafficManagerInternalServiceName(inst))
	}
	if trafficManagerServiceEnabled(inst.Spec.Service.External.Enabled, false) {
		if err := r.ensureService(ctx, inst, "external"); err != nil {
			log.Error(err, "failed to reconcile external service", "service", trafficManagerExternalServiceName(inst))
			return trafficManagerNoRequeue, err
		}
		log.Info("external service reconciled", "service", trafficManagerExternalServiceName(inst))
	} else if err := deleteServiceIfExists(ctx, r.Client, inst, trafficManagerExternalServiceName(inst)); err != nil {
		log.Error(err, "failed to delete external service", "service", trafficManagerExternalServiceName(inst))
		return trafficManagerNoRequeue, err
	} else {
		log.Info("external service disabled", "service", trafficManagerExternalServiceName(inst))
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
		log.Error(err, "failed to update TrafficManager status")
		return trafficManagerNoRequeue, err
	}
	log.Info("status updated",
		"status", inst.Status.Status,
		"readyReplicas", inst.Status.ReadyReplicas,
		"internalService", inst.Status.InternalService,
		"externalService", inst.Status.ExternalService,
		"externalEndpoint", inst.Status.ExternalEndpoint,
		"routeCount", len(inst.Status.Nginx.Routes),
	)
	if depResult.Created {
		log.Info("reconcile completed", "requeueAfter", trafficManagerRequeue.RequeueAfter.String(), "reason", "deployment created")
		return trafficManagerRequeue, nil
	}
	if depResult.Updated {
		log.Info("reconcile completed", "requeueAfter", trafficManagerRequeue.RequeueAfter.String(), "reason", "deployment updated")
		return trafficManagerRequeue, nil
	}
	result := ctrl.Result{RequeueAfter: 30 * time.Second}
	log.Info("reconcile completed", "requeueAfter", result.RequeueAfter.String())
	return result, nil
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
				switch item.Spec.Type {
				case networkv4.TrafficManagerTypeNginx:
					if strings.TrimSpace(item.Spec.Security.TLS.SecretName) != secret.Name &&
						backendTLSTrustSecretName(item) != secret.Name {
						continue
					}
				case networkv4.TrafficManagerTypeCman:
					if !trafficManagerReferencesSecret(item, secret.Name) {
						continue
					}
				default:
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
		if err := privateaiv4.ValidateTrafficManagerRoutePath(path); err != nil {
			return nil, fmt.Errorf("invalid traffic manager route path for backend %s: %w", item.Name, err)
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

func (r *TrafficManagerReconciler) applyConfigMap(ctx context.Context, inst *networkv4.TrafficManager, cm *corev1.ConfigMap) error {
	existing := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Client.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(existing, inst) {
		return apierrors.NewConflict(
			schema.GroupResource{Resource: "configmaps"},
			existing.Name,
			fmt.Errorf("configmap is not controlled by TrafficManager %s/%s", inst.Namespace, inst.Name),
		)
	}
	existing.Data = cm.Data
	existing.Labels = cm.Labels
	existing.OwnerReferences = cm.OwnerReferences
	return r.Client.Update(ctx, existing)
}

func (r *TrafficManagerReconciler) reconcileCman(ctx context.Context, log logr.Logger, inst *networkv4.TrafficManager) (ctrl.Result, error) {
	configChecksum, err := r.resolveCmanConfigChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to resolve CMAN config source")
		return trafficManagerRequeue, err
	}
	restSecretChecksum, err := r.resolveCmanRestSecretChecksum(ctx, inst)
	if err != nil {
		inst.Status.Status = privateaiv4.StatusError
		_ = r.Status().Update(ctx, inst)
		log.Error(err, "failed to resolve CMAN REST secrets")
		return trafficManagerRequeue, err
	}

	deploy := buildTrafficManagerDeployment(inst, configChecksum, restSecretChecksum, "")
	if err := controllerutil.SetControllerReference(inst, deploy, r.Scheme); err != nil {
		log.Error(err, "failed to set controller reference on deployment", "deployment", deploy.Name)
		return trafficManagerNoRequeue, err
	}
	foundDeploy, depResult, err := k8sobjects.ReconcileDeployment(ctx, r.Client, inst.Namespace, deploy, syncTrafficManagerDeployment)
	if err != nil {
		log.Error(err, "failed to reconcile deployment", "deployment", deploy.Name)
		return trafficManagerNoRequeue, err
	}
	log.Info("deployment reconciled",
		"deployment", deploy.Name,
		"created", depResult.Created,
		"updated", depResult.Updated,
		"readyReplicas", foundDeploy.Status.ReadyReplicas,
		"desiredReplicas", ptr.Deref(deploy.Spec.Replicas, int32(0)),
	)

	if trafficManagerServiceEnabled(inst.Spec.Service.Internal.Enabled, true) {
		if err := r.ensureService(ctx, inst, "internal"); err != nil {
			log.Error(err, "failed to reconcile internal service", "service", trafficManagerInternalServiceName(inst))
			return trafficManagerNoRequeue, err
		}
	} else if err := deleteServiceIfExists(ctx, r.Client, inst, trafficManagerInternalServiceName(inst)); err != nil {
		log.Error(err, "failed to delete internal service", "service", trafficManagerInternalServiceName(inst))
		return trafficManagerNoRequeue, err
	}
	if trafficManagerServiceEnabled(inst.Spec.Service.External.Enabled, false) {
		if err := r.ensureService(ctx, inst, "external"); err != nil {
			log.Error(err, "failed to reconcile external service", "service", trafficManagerExternalServiceName(inst))
			return trafficManagerNoRequeue, err
		}
	} else if err := deleteServiceIfExists(ctx, r.Client, inst, trafficManagerExternalServiceName(inst)); err != nil {
		log.Error(err, "failed to delete external service", "service", trafficManagerExternalServiceName(inst))
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
	inst.Status.Cman = &networkv4.CmanTrafficManagerStatus{
		ConfigMode: cmanConfigMode(inst),
		RestHost:   cmanRestHost(inst),
	}
	if err := r.Status().Update(ctx, inst); err != nil {
		log.Error(err, "failed to update TrafficManager status")
		return trafficManagerNoRequeue, err
	}
	if depResult.Created || depResult.Updated {
		return trafficManagerRequeue, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
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
		builder.WriteString("            proxy_ssl_protocols TLSv1.2 TLSv1.3;\n")
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
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		return buildCmanTrafficManagerDeployment(inst, configChecksum, tlsChecksum)
	}
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
		VolumeMounts:    volumeMounts,
		Env:             buildTrafficManagerEnvVars(inst),
		SecurityContext: inst.Spec.Runtime.ContainerSecurityContext,
	}
	if inst.Spec.Runtime.Resources != nil {
		container.Resources = *inst.Spec.Runtime.Resources
	}
	podSpec := corev1.PodSpec{
		ServiceAccountName: inst.Spec.Runtime.ServiceAccountName,
		SecurityContext:    inst.Spec.Runtime.PodSecurityContext,
		Containers:         []corev1.Container{container},
		Volumes:            volumes,
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

func buildCmanTrafficManagerDeployment(inst *networkv4.TrafficManager, configChecksum, restSecretChecksum string) *appsv1.Deployment {
	labels := trafficManagerLabels(inst)
	annotations := map[string]string{"network.oracle.com/config-hash": configChecksum}
	if restSecretChecksum != "" {
		annotations["network.oracle.com/cman-rest-secret-hash"] = restSecretChecksum
	}
	volumeMounts, volumes := buildCmanTrafficManagerVolumes(inst)
	container := corev1.Container{
		Name:            trafficManagerDeploymentName(inst),
		Image:           inst.Spec.Runtime.Image,
		ImagePullPolicy: imagePullPolicyOrDefault(inst.Spec.Runtime.ImagePullPolicy),
		Ports:           buildTrafficManagerContainerPorts(inst),
		Env:             buildTrafficManagerEnvVars(inst),
		VolumeMounts:    volumeMounts,
		SecurityContext: inst.Spec.Runtime.ContainerSecurityContext,
	}
	if inst.Spec.Runtime.Resources != nil {
		container.Resources = *inst.Spec.Runtime.Resources
	}
	podSpec := corev1.PodSpec{
		ServiceAccountName: inst.Spec.Runtime.ServiceAccountName,
		SecurityContext:    inst.Spec.Runtime.PodSecurityContext,
		Containers:         []corev1.Container{container},
		Volumes:            volumes,
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
	if inst.Spec.Type != networkv4.TrafficManagerTypeCman && spec.Port == 0 {
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
	if inst.Spec.Type != networkv4.TrafficManagerTypeCman && spec.TargetPort == 0 {
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
			Ports:    buildTrafficManagerServicePorts(inst, serviceKind, spec),
		},
	}
	if serviceKind == "external" {
		if spec.ExternalTrafficPolicy != "" {
			service.Spec.ExternalTrafficPolicy = spec.ExternalTrafficPolicy
		}
		if spec.LoadBalancerIP != "" {
			service.Spec.LoadBalancerIP = spec.LoadBalancerIP
		}
		if spec.LoadBalancerClass != nil && strings.TrimSpace(*spec.LoadBalancerClass) != "" {
			service.Spec.LoadBalancerClass = spec.LoadBalancerClass
		}
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

func describeAssociatedBackends(backends []associatedBackend) []string {
	out := make([]string, 0, len(backends))
	for _, backend := range backends {
		out = append(out, fmt.Sprintf("%s:%s->%s:%d(%s)",
			backend.Name,
			backend.Path,
			backend.ServiceName,
			backend.ServicePort,
			backendScheme(backend.UseHTTPS),
		))
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
	out := make([]corev1.EnvVar, 0, len(inst.Spec.Runtime.EnvVars)+12)
	for _, e := range inst.Spec.Runtime.EnvVars {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		out = append(out, corev1.EnvVar{Name: e.Name, Value: e.Value})
	}
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		out = append(out, buildCmanEnvVars(inst)...)
	}
	return out
}

func buildCmanEnvVars(inst *networkv4.TrafficManager) []corev1.EnvVar {
	publicHostname := trafficManagerInternalServiceName(inst)
	domain := fmt.Sprintf("%s.svc.cluster.local", inst.Namespace)
	out := []corev1.EnvVar{
		{Name: "PUBLIC_HOSTNAME", Value: publicHostname},
		{Name: "DOMAIN", Value: domain},
		{Name: "PORT", Value: strconv.Itoa(int(trafficManagerPrimaryPort(inst)))},
	}
	if cmanUsesFileConfig(inst) {
		return append(out, corev1.EnvVar{Name: "USER_CMAN_FILE", Value: cmanUserConfigFilePath})
	}
	if inst.Spec.Cman == nil {
		return out
	}
	if strings.TrimSpace(inst.Spec.Cman.LogLevel) != "" {
		out = append(out, corev1.EnvVar{Name: "LOG_LEVEL", Value: inst.Spec.Cman.LogLevel})
	}
	if strings.TrimSpace(inst.Spec.Cman.TraceLevel) != "" {
		out = append(out, corev1.EnvVar{Name: "TRACE_LEVEL", Value: inst.Spec.Cman.TraceLevel})
	}
	if strings.TrimSpace(inst.Spec.Cman.RegistrationInvitedNodes) != "" {
		out = append(out, corev1.EnvVar{Name: "REGISTRATION_INVITED_NODES", Value: inst.Spec.Cman.RegistrationInvitedNodes})
	}
	if details := cmanDBHostDetails(inst); details != "" {
		out = append(out, corev1.EnvVar{Name: "DB_HOSTDETAILS", Value: details})
	}
	if restEnabled(inst) {
		out = append(out,
			corev1.EnvVar{Name: "ENABLE_REST_API", Value: "true"},
			corev1.EnvVar{Name: "REST_HOST", Value: cmanRestHost(inst)},
			corev1.EnvVar{Name: "REST_PORT", Value: strconv.Itoa(int(cmanRestPort(inst)))},
			corev1.EnvVar{Name: "REST_PWD_FILE", Value: cmanRestPasswordFilePath},
			corev1.EnvVar{Name: "REST_PWD_KEY", Value: cmanRestPrivateKeyFilePath},
		)
	}
	return out
}

func buildTrafficManagerServicePorts(inst *networkv4.TrafficManager, serviceKind string, spec networkv4.TrafficManagerServiceEndpointSpec) []corev1.ServicePort {
	portSpecs := trafficManagerServicePortSpecs(inst, serviceKind, spec)
	ports := make([]corev1.ServicePort, 0, len(portSpecs))
	for _, portSpec := range portSpecs {
		name := portSpec.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("tcp-%d", portSpec.Port)
		}
		protocol := portSpec.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		servicePort := corev1.ServicePort{
			Name:       name,
			Protocol:   protocol,
			Port:       portSpec.Port,
			TargetPort: intstr.FromInt(int(portSpec.TargetPort)),
		}
		if portSpec.NodePort > 0 {
			servicePort.NodePort = portSpec.NodePort
		}
		ports = append(ports, servicePort)
	}
	return ports
}

func trafficManagerServicePortSpecs(inst *networkv4.TrafficManager, serviceKind string, spec networkv4.TrafficManagerServiceEndpointSpec) []networkv4.TrafficManagerServicePortSpec {
	if len(spec.Ports) > 0 {
		out := make([]networkv4.TrafficManagerServicePortSpec, 0, len(spec.Ports))
		for _, port := range spec.Ports {
			if port.Port <= 0 && port.TargetPort <= 0 {
				continue
			}
			if port.Port <= 0 {
				port.Port = port.TargetPort
			}
			if port.TargetPort <= 0 {
				port.TargetPort = port.Port
			}
			out = append(out, port)
		}
		if len(out) > 0 {
			return out
		}
	}
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		mainPort := spec.Port
		if mainPort == 0 {
			mainPort = trafficManagerPrimaryPort(inst)
		}
		targetPort := spec.TargetPort
		if targetPort == 0 {
			targetPort = mainPort
		}
		out := []networkv4.TrafficManagerServicePortSpec{{
			Name:       "cman",
			Port:       mainPort,
			TargetPort: targetPort,
			Protocol:   corev1.ProtocolTCP,
		}}
		if restEnabled(inst) {
			out = append(out, networkv4.TrafficManagerServicePortSpec{
				Name:       "rest",
				Port:       cmanRestPort(inst),
				TargetPort: cmanRestPort(inst),
				Protocol:   corev1.ProtocolTCP,
			})
		}
		return out
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
	return []networkv4.TrafficManagerServicePortSpec{{
		Port:       spec.Port,
		TargetPort: spec.TargetPort,
		Protocol:   corev1.ProtocolTCP,
	}}
}

func buildTrafficManagerContainerPorts(inst *networkv4.TrafficManager) []corev1.ContainerPort {
	seen := map[int32]struct{}{}
	appendPorts := func(portSpecs []networkv4.TrafficManagerServicePortSpec, out []corev1.ContainerPort) []corev1.ContainerPort {
		for _, portSpec := range portSpecs {
			targetPort := portSpec.TargetPort
			if targetPort <= 0 {
				targetPort = portSpec.Port
			}
			if targetPort <= 0 {
				continue
			}
			if _, ok := seen[targetPort]; ok {
				continue
			}
			seen[targetPort] = struct{}{}
			protocol := portSpec.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}
			out = append(out, corev1.ContainerPort{ContainerPort: targetPort, Protocol: protocol})
		}
		return out
	}

	out := make([]corev1.ContainerPort, 0)
	out = appendPorts(trafficManagerServicePortSpecs(inst, "internal", inst.Spec.Service.Internal), out)
	out = appendPorts(trafficManagerServicePortSpecs(inst, "external", inst.Spec.Service.External), out)
	if len(out) == 0 {
		out = append(out, corev1.ContainerPort{ContainerPort: trafficManagerContainerPort(inst), Protocol: corev1.ProtocolTCP})
	}
	return out
}

func buildCmanTrafficManagerVolumes(inst *networkv4.TrafficManager) ([]corev1.VolumeMount, []corev1.Volume) {
	volumeMounts := make([]corev1.VolumeMount, 0)
	volumes := make([]corev1.Volume, 0)

	if cmanUsesFileConfig(inst) && inst.Spec.Cman != nil && inst.Spec.Cman.ConfigSource != nil {
		if ref := inst.Spec.Cman.ConfigSource.ConfigMapRef; ref != nil {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "cman-user-config",
				MountPath: cmanUserConfigFilePath,
				SubPath:   ref.Key,
				ReadOnly:  true,
			})
			volumes = append(volumes, corev1.Volume{
				Name: "cman-user-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
						Items:                []corev1.KeyToPath{{Key: ref.Key, Path: ref.Key}},
					},
				},
			})
		}
	}

	if restEnabled(inst) && inst.Spec.Cman != nil && inst.Spec.Cman.RestAPI != nil {
		sources := make([]corev1.VolumeProjection, 0, 2)
		if ref := inst.Spec.Cman.RestAPI.PasswordSecretRef; ref != nil {
			sources = append(sources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
					Items:                []corev1.KeyToPath{{Key: ref.Key, Path: cmanRestPasswordFileName}},
				},
			})
		}
		if ref := inst.Spec.Cman.RestAPI.PrivateKeySecretRef; ref != nil {
			sources = append(sources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
					Items:                []corev1.KeyToPath{{Key: ref.Key, Path: cmanRestPrivateKeyFileName}},
				},
			})
		}
		if len(sources) > 0 {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "cman-rest-secrets",
				MountPath: cmanRestSecretsMountDir,
				ReadOnly:  true,
			})
			volumes = append(volumes, corev1.Volume{
				Name: "cman-rest-secrets",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{Sources: sources},
				},
			})
		}
	}

	return volumeMounts, volumes
}

func cmanDBHostDetails(inst *networkv4.TrafficManager) string {
	if inst.Spec.Cman == nil || len(inst.Spec.Cman.Rules) == 0 {
		return ""
	}
	parts := make([]string, 0, len(inst.Spec.Cman.Rules))
	for _, rule := range inst.Spec.Cman.Rules {
		ruleParts := []string{fmt.Sprintf("HOST=%s", rule.Host)}
		if strings.TrimSpace(rule.IP) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("IP=%s", rule.IP))
		}
		if strings.TrimSpace(rule.Src) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_SRC=%s", rule.Src))
		}
		if strings.TrimSpace(rule.Dst) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_DST=%s", rule.Dst))
		}
		if strings.TrimSpace(rule.Srv) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_SRV=%s", rule.Srv))
		}
		if strings.TrimSpace(rule.Action) != "" {
			ruleParts = append(ruleParts, fmt.Sprintf("RULE_ACT=%s", rule.Action))
		}
		parts = append(parts, strings.Join(ruleParts, ":"))
	}
	return strings.Join(parts, ",")
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

const (
	cmanUserConfigFilePath     = "/opt/oracle/cman-config/cman.ora"
	cmanRestSecretsMountDir    = "/run/secrets/cman-rest"
	cmanRestPasswordFileName   = "RESTpwdsecret"
	cmanRestPrivateKeyFileName = "RESTkeysecret"
)

var (
	cmanRestPasswordFilePath   = filepath.Join(cmanRestSecretsMountDir, cmanRestPasswordFileName)
	cmanRestPrivateKeyFilePath = filepath.Join(cmanRestSecretsMountDir, cmanRestPrivateKeyFileName)
)

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
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		if cmanUsesFileConfig(inst) {
			return "File"
		}
		return "Generated"
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
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		return trafficManagerPrimaryPort(inst)
	}
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

func trafficManagerPrimaryPort(inst *networkv4.TrafficManager) int32 {
	for _, port := range inst.Spec.Service.Internal.Ports {
		if port.TargetPort > 0 {
			return port.TargetPort
		}
		if port.Port > 0 {
			return port.Port
		}
	}
	if inst.Spec.Service.Internal.TargetPort > 0 {
		return inst.Spec.Service.Internal.TargetPort
	}
	if inst.Spec.Service.Internal.Port > 0 {
		return inst.Spec.Service.Internal.Port
	}
	for _, port := range inst.Spec.Service.External.Ports {
		if port.TargetPort > 0 {
			return port.TargetPort
		}
		if port.Port > 0 {
			return port.Port
		}
	}
	if inst.Spec.Service.External.TargetPort > 0 {
		return inst.Spec.Service.External.TargetPort
	}
	if inst.Spec.Service.External.Port > 0 {
		return inst.Spec.Service.External.Port
	}
	if inst.Spec.Type == networkv4.TrafficManagerTypeCman {
		return 1521
	}
	return trafficManagerContainerPort(inst)
}

func cmanConfigMode(inst *networkv4.TrafficManager) string {
	if cmanUsesFileConfig(inst) {
		return "file"
	}
	return "generated"
}

func cmanUsesFileConfig(inst *networkv4.TrafficManager) bool {
	return inst != nil &&
		inst.Spec.Cman != nil &&
		inst.Spec.Cman.ConfigSource != nil
}

func restEnabled(inst *networkv4.TrafficManager) bool {
	return inst != nil &&
		inst.Spec.Cman != nil &&
		inst.Spec.Cman.RestAPI != nil &&
		inst.Spec.Cman.RestAPI.Enabled &&
		!cmanUsesFileConfig(inst)
}

func cmanRestHost(inst *networkv4.TrafficManager) string {
	if !restEnabled(inst) {
		return ""
	}
	if host := strings.TrimSpace(inst.Spec.Cman.RestAPI.Host); host != "" {
		return host
	}
	return fmt.Sprintf("%s.%s.svc.cluster.local", trafficManagerInternalServiceName(inst), inst.Namespace)
}

func cmanRestPort(inst *networkv4.TrafficManager) int32 {
	if !restEnabled(inst) || inst.Spec.Cman == nil || inst.Spec.Cman.RestAPI == nil || inst.Spec.Cman.RestAPI.Port == 0 {
		return 1525
	}
	return inst.Spec.Cman.RestAPI.Port
}

func trafficManagerReferencesSecret(inst *networkv4.TrafficManager, secretName string) bool {
	if inst == nil || strings.TrimSpace(secretName) == "" {
		return false
	}
	if inst.Spec.Type == networkv4.TrafficManagerTypeNginx {
		return strings.TrimSpace(inst.Spec.Security.TLS.SecretName) == secretName ||
			backendTLSTrustSecretName(inst) == secretName
	}
	if inst.Spec.Type != networkv4.TrafficManagerTypeCman || inst.Spec.Cman == nil {
		return false
	}
	if inst.Spec.Cman.RestAPI != nil {
		if inst.Spec.Cman.RestAPI.PasswordSecretRef != nil &&
			strings.TrimSpace(inst.Spec.Cman.RestAPI.PasswordSecretRef.Name) == secretName {
			return true
		}
		if inst.Spec.Cman.RestAPI.PrivateKeySecretRef != nil &&
			strings.TrimSpace(inst.Spec.Cman.RestAPI.PrivateKeySecretRef.Name) == secretName {
			return true
		}
	}
	return false
}

func (r *TrafficManagerReconciler) resolveCmanConfigChecksum(ctx context.Context, inst *networkv4.TrafficManager) (string, error) {
	if !cmanUsesFileConfig(inst) {
		return checksumString(strings.Join(flattenEnvVars(buildCmanEnvVars(inst)), "\n")), nil
	}
	if inst.Spec.Cman == nil || inst.Spec.Cman.ConfigSource == nil {
		return checksumString(""), nil
	}
	if ref := inst.Spec.Cman.ConfigSource.ConfigMapRef; ref != nil {
		cm := &corev1.ConfigMap{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, cm); err != nil {
			return "", fmt.Errorf("failed to get configMap %s: %w", ref.Name, err)
		}
		value, ok := cm.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("configMap %s is missing key %s", ref.Name, ref.Key)
		}
		return checksumString(value), nil
	}
	return checksumString(""), nil
}

func (r *TrafficManagerReconciler) resolveCmanRestSecretChecksum(ctx context.Context, inst *networkv4.TrafficManager) (string, error) {
	if !restEnabled(inst) || inst.Spec.Cman == nil || inst.Spec.Cman.RestAPI == nil {
		return "", nil
	}
	payload := make([]byte, 0)
	for _, ref := range []*networkv4.TrafficManagerSecretKeyRef{
		inst.Spec.Cman.RestAPI.PasswordSecretRef,
		inst.Spec.Cman.RestAPI.PrivateKeySecretRef,
	} {
		if ref == nil {
			continue
		}
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: inst.Namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to get secret %s: %w", ref.Name, err)
		}
		value, ok := secret.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("secret %s is missing key %s", ref.Name, ref.Key)
		}
		payload = append(payload, value...)
	}
	if len(payload) == 0 {
		return "", nil
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func flattenEnvVars(envs []corev1.EnvVar) []string {
	out := make([]string, 0, len(envs))
	for _, env := range envs {
		out = append(out, env.Name+"="+env.Value)
	}
	sort.Strings(out)
	return out
}

func parseBoolFlag(flag string) bool {
	val, err := strconv.ParseBool(flag)
	if err != nil {
		return false
	}
	return val
}

func deleteServiceIfExists(ctx context.Context, c client.Client, inst *networkv4.TrafficManager, name string) error {
	if inst == nil || name == "" {
		return nil
	}
	svc := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !metav1.IsControlledBy(svc, inst) {
		return apierrors.NewConflict(
			schema.GroupResource{Resource: "services"},
			svc.Name,
			fmt.Errorf("service is not controlled by TrafficManager %s/%s", inst.Namespace, inst.Name),
		)
	}
	uid := svc.UID
	if err := c.Delete(ctx, svc, client.Preconditions{UID: &uid}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
