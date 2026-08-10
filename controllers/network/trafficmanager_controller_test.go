package network

import (
	"context"
	"strings"
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildManagedNginxConfigTLSProtocols(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
		Spec: networkv4.TrafficManagerSpec{
			Security: networkv4.TrafficManagerSecuritySpec{
				TLS: networkv4.TrafficManagerTLSSpec{Enabled: true},
			},
		},
	}

	cfg, err := buildManagedNginxConfig(inst, nil)
	if err != nil {
		t.Fatalf("buildManagedNginxConfig returned error: %v", err)
	}
	if !strings.Contains(cfg, "ssl_protocols TLSv1.2 TLSv1.3;") {
		t.Fatalf("expected TLS protocol restriction in config, got:\n%s", cfg)
	}
}

func TestBuildTrafficManagerDeploymentIncludesTLSSecretHashAnnotation(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
	}

	deploy := buildTrafficManagerDeployment(inst, "config-hash", "tls-hash", "")
	got := deploy.Spec.Template.Annotations["network.oracle.com/tls-secret-hash"]
	if got != "tls-hash" {
		t.Fatalf("expected TLS secret hash annotation, got %q", got)
	}
}

func TestBuildTrafficManagerDeploymentIncludesBackendTLSSecretHashAnnotation(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
	}

	deploy := buildTrafficManagerDeployment(inst, "config-hash", "", "backend-tls-hash")
	got := deploy.Spec.Template.Annotations["network.oracle.com/backend-tls-secret-hash"]
	if got != "backend-tls-hash" {
		t.Fatalf("expected backend TLS secret hash annotation, got %q", got)
	}
}

func TestDeleteServiceIfExistsPreservesUnownedService(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm",
			Namespace: "ns",
			UID:       types.UID("tm-uid"),
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "tm-ext", Namespace: "ns"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst, svc).Build()

	err := deleteServiceIfExists(context.Background(), client, inst, "tm-ext")
	if err == nil {
		t.Fatalf("expected ownership conflict for unowned service")
	}
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}

	preserved := &corev1.Service{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "tm-ext", Namespace: "ns"}, preserved); err != nil {
		t.Fatalf("expected unowned service to be preserved, got error: %v", err)
	}
}

func TestDeleteServiceIfExistsDeletesOwnedService(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "network.oracle.com/v4",
			Kind:       "TrafficManager",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm",
			Namespace: "ns",
			UID:       types.UID("tm-uid"),
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm-ext",
			Namespace: "ns",
			UID:       types.UID("svc-uid"),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "network.oracle.com/v4",
				Kind:       "TrafficManager",
				Name:       "tm",
				UID:        types.UID("tm-uid"),
				Controller: ptr.To(true),
			}},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst, svc).Build()

	if err := deleteServiceIfExists(context.Background(), client, inst, "tm-ext"); err != nil {
		t.Fatalf("deleteServiceIfExists returned error: %v", err)
	}

	deleted := &corev1.Service{}
	err := client.Get(context.Background(), types.NamespacedName{Name: "tm-ext", Namespace: "ns"}, deleted)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected owned service to be deleted, got error: %v", err)
	}
}

func TestApplyConfigMapPreservesUnownedConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm",
			Namespace: "ns",
			UID:       types.UID("tm-uid"),
		},
	}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-nginx-config",
			Namespace: "ns",
			Labels:    map[string]string{"owner": "other"},
		},
		Data: map[string]string{"sentinel": "keep"},
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-nginx-config",
			Namespace: "ns",
			Labels:    trafficManagerLabels(inst),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "network.oracle.com/v4",
				Kind:       "TrafficManager",
				Name:       "tm",
				UID:        types.UID("tm-uid"),
				Controller: ptr.To(true),
			}},
		},
		Data: map[string]string{"nginx.conf": "generated"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst, existing).Build()
	r := &TrafficManagerReconciler{Client: client}

	err := r.applyConfigMap(context.Background(), inst, desired)
	if err == nil {
		t.Fatalf("expected ownership conflict for unowned configmap")
	}
	if !apierrors.IsConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}

	preserved := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "shared-nginx-config", Namespace: "ns"}, preserved); err != nil {
		t.Fatalf("expected configmap to be preserved, got error: %v", err)
	}
	if got := preserved.Data["sentinel"]; got != "keep" {
		t.Fatalf("expected sentinel data to be preserved, got %q", got)
	}
	if got := preserved.Labels["owner"]; got != "other" {
		t.Fatalf("expected original label to be preserved, got %q", got)
	}
	if len(preserved.OwnerReferences) != 0 {
		t.Fatalf("expected owner references to remain empty, got %#v", preserved.OwnerReferences)
	}
}

func TestApplyConfigMapUpdatesOwnedConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: "network.oracle.com/v4",
		Kind:       "TrafficManager",
		Name:       "tm",
		UID:        types.UID("tm-uid"),
		Controller: ptr.To(true),
	}
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm",
			Namespace: "ns",
			UID:       types.UID("tm-uid"),
		},
	}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "tm-nginx",
			Namespace:       "ns",
			Labels:          map[string]string{"old": "label"},
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Data: map[string]string{"nginx.conf": "old"},
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "tm-nginx",
			Namespace:       "ns",
			Labels:          trafficManagerLabels(inst),
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Data: map[string]string{"nginx.conf": "generated"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst, existing).Build()
	r := &TrafficManagerReconciler{Client: client}

	if err := r.applyConfigMap(context.Background(), inst, desired); err != nil {
		t.Fatalf("applyConfigMap returned error: %v", err)
	}

	updated := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "tm-nginx", Namespace: "ns"}, updated); err != nil {
		t.Fatalf("expected configmap to exist, got error: %v", err)
	}
	if got := updated.Data["nginx.conf"]; got != "generated" {
		t.Fatalf("expected generated config, got %q", got)
	}
	if got := updated.Labels["app.kubernetes.io/component"]; got != "traffic-manager" {
		t.Fatalf("expected traffic-manager label, got %q", got)
	}
	if !metav1.IsControlledBy(updated, inst) {
		t.Fatalf("expected configmap to remain controlled by TrafficManager")
	}
}

func TestApplyConfigMapCreatesMissingConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm",
			Namespace: "ns",
			UID:       types.UID("tm-uid"),
		},
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tm-nginx",
			Namespace: "ns",
			Labels:    trafficManagerLabels(inst),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "network.oracle.com/v4",
				Kind:       "TrafficManager",
				Name:       "tm",
				UID:        types.UID("tm-uid"),
				Controller: ptr.To(true),
			}},
		},
		Data: map[string]string{"nginx.conf": "generated"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst).Build()
	r := &TrafficManagerReconciler{Client: client}

	if err := r.applyConfigMap(context.Background(), inst, desired); err != nil {
		t.Fatalf("applyConfigMap returned error: %v", err)
	}

	created := &corev1.ConfigMap{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "tm-nginx", Namespace: "ns"}, created); err != nil {
		t.Fatalf("expected configmap to be created, got error: %v", err)
	}
	if got := created.Data["nginx.conf"]; got != "generated" {
		t.Fatalf("expected generated config, got %q", got)
	}
	if !metav1.IsControlledBy(created, inst) {
		t.Fatalf("expected created configmap to be controlled by TrafficManager")
	}
}

func TestResolveTLSSecretChecksum(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
		Spec: networkv4.TrafficManagerSpec{
			Security: networkv4.TrafficManagerSecuritySpec{
				TLS: networkv4.TrafficManagerTLSSpec{
					Enabled:    true,
					SecretName: "tls-secret",
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-secret", Namespace: "ns"},
		Data: map[string][]byte{
			"tls.crt": []byte("crt-data"),
			"tls.key": []byte("key-data"),
		},
	}

	r := &TrafficManagerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	sum1, err := r.resolveTLSSecretChecksum(context.Background(), inst)
	if err != nil {
		t.Fatalf("resolveTLSSecretChecksum returned error: %v", err)
	}
	if sum1 == "" {
		t.Fatalf("expected non-empty checksum")
	}

	secret.Data["tls.crt"] = []byte("crt-data-updated")
	r.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	sum2, err := r.resolveTLSSecretChecksum(context.Background(), inst)
	if err != nil {
		t.Fatalf("resolveTLSSecretChecksum returned error after update: %v", err)
	}
	if sum1 == sum2 {
		t.Fatalf("expected checksum to change when TLS secret data changes")
	}
}

func TestBuildNginxRouteStatuses(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
		Spec: networkv4.TrafficManagerSpec{
			Type: networkv4.TrafficManagerTypeNginx,
			Security: networkv4.TrafficManagerSecuritySpec{
				TLS: networkv4.TrafficManagerTLSSpec{Enabled: true},
			},
		},
	}
	backends := []associatedBackend{{
		Name:        "pai-a",
		Path:        "/pai-a/v1/",
		ServiceName: "pai-a-local.ns.svc.cluster.local",
		ServicePort: 8443,
		UseHTTPS:    true,
	}}

	routes := buildNginxRouteStatuses(inst, backends, "https://141.148.67.224")
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].BackendURL != "https://pai-a-local.ns.svc.cluster.local:8443" {
		t.Fatalf("unexpected backend URL %q", routes[0].BackendURL)
	}
	if routes[0].PublicURL != "https://141.148.67.224/pai-a/v1/" {
		t.Fatalf("unexpected public URL %q", routes[0].PublicURL)
	}
	if got := trafficManagerConfigMode(inst); got != "Managed" {
		t.Fatalf("expected config mode Managed, got %q", got)
	}
}

func TestListAssociatedBackendsUsesEffectiveTrafficManagerSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := privateaiv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add privateai scheme: %v", err)
	}

	tm := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-nginx", Namespace: "pai"},
	}
	privateAI := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Networking: &privateaiv4.PrivateAiNetworkingSpec{
				TrafficManager: &privateaiv4.TrafficManagerRefSpec{
					Ref:       "pai-nginx",
					RoutePath: "/pai-sample/v1/",
				},
			},
		},
	}

	r := &TrafficManagerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tm, privateAI).Build(),
	}

	backends, err := r.listAssociatedBackends(context.Background(), tm)
	if err != nil {
		t.Fatalf("listAssociatedBackends returned error: %v", err)
	}
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Name != "pai-sample" {
		t.Fatalf("unexpected backend name %q", backends[0].Name)
	}
	if backends[0].Path != "/pai-sample/v1/" {
		t.Fatalf("unexpected backend path %q", backends[0].Path)
	}
	if backends[0].ServiceName != "pai-sample.pai.svc.cluster.local" {
		t.Fatalf("unexpected backend service %q", backends[0].ServiceName)
	}
	if backends[0].ServicePort != 8443 {
		t.Fatalf("unexpected backend service port %d", backends[0].ServicePort)
	}
	if !backends[0].UseHTTPS {
		t.Fatalf("expected backend to default to HTTPS")
	}
}

func TestListAssociatedBackendsRejectsUnsafeRoutePath(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := privateaiv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add privateai scheme: %v", err)
	}

	tm := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-nginx", Namespace: "pai"},
	}
	privateAI := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Networking: &privateaiv4.PrivateAiNetworkingSpec{
				TrafficManager: &privateaiv4.TrafficManagerRefSpec{
					Ref:       "pai-nginx",
					RoutePath: "/pai/v1/;return 200;/",
				},
			},
		},
	}

	r := &TrafficManagerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tm, privateAI).Build(),
	}

	if _, err := r.listAssociatedBackends(context.Background(), tm); err == nil {
		t.Fatalf("expected unsafe route path to be rejected")
	}
}

func TestBuildManagedNginxConfigBackendTLSVerificationEnabled(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
		Spec: networkv4.TrafficManagerSpec{
			Type: networkv4.TrafficManagerTypeNginx,
			Security: networkv4.TrafficManagerSecuritySpec{
				BackendTLS: &networkv4.TrafficManagerBackendTLSSpec{
					TrustSecretName: "backend-ca",
				},
			},
		},
	}
	backends := []associatedBackend{{
		Name:        "pai-a",
		Path:        "/pai-a/v1/",
		ServiceName: "pai-a.ns.svc.cluster.local",
		ServicePort: 8443,
		UseHTTPS:    true,
	}}

	cfg, err := buildManagedNginxConfig(inst, backends)
	if err != nil {
		t.Fatalf("buildManagedNginxConfig returned error: %v", err)
	}
	if !strings.Contains(cfg, "proxy_ssl_trusted_certificate /etc/nginx/backend-ca/ca.crt;") {
		t.Fatalf("expected backend trust file reference in config, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "proxy_ssl_protocols TLSv1.2 TLSv1.3;") {
		t.Fatalf("expected backend TLS protocols in config, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "proxy_ssl_verify on;") {
		t.Fatalf("expected backend TLS verification on, got:\n%s", cfg)
	}
}

func TestBuildManagedNginxConfigBackendTLSAbsentKeepsVerificationOff(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
	}
	backends := []associatedBackend{{
		Name:        "pai-a",
		Path:        "/pai-a/v1/",
		ServiceName: "pai-a.ns.svc.cluster.local",
		ServicePort: 8443,
		UseHTTPS:    true,
	}}

	cfg, err := buildManagedNginxConfig(inst, backends)
	if err != nil {
		t.Fatalf("buildManagedNginxConfig returned error: %v", err)
	}
	if !strings.Contains(cfg, "proxy_ssl_protocols TLSv1.2 TLSv1.3;") {
		t.Fatalf("expected backend TLS protocols in config, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "proxy_ssl_verify off;") {
		t.Fatalf("expected backend TLS verification off when backendTLS is absent, got:\n%s", cfg)
	}
}

func TestResolveBackendTLSSecretChecksum(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := networkv4.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add network scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns"},
		Spec: networkv4.TrafficManagerSpec{
			Security: networkv4.TrafficManagerSecuritySpec{
				BackendTLS: &networkv4.TrafficManagerBackendTLSSpec{
					TrustSecretName: "backend-ca",
					TrustFileName:   "ca-bundle.crt",
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backend-ca", Namespace: "ns"},
		Data: map[string][]byte{
			"ca-bundle.crt": []byte("ca-data"),
		},
	}

	r := &TrafficManagerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	sum1, err := r.resolveBackendTLSSecretChecksum(context.Background(), inst)
	if err != nil {
		t.Fatalf("resolveBackendTLSSecretChecksum returned error: %v", err)
	}
	if sum1 == "" {
		t.Fatalf("expected non-empty backend TLS checksum")
	}

	secret.Data["ca-bundle.crt"] = []byte("ca-data-updated")
	r.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	sum2, err := r.resolveBackendTLSSecretChecksum(context.Background(), inst)
	if err != nil {
		t.Fatalf("resolveBackendTLSSecretChecksum returned error after update: %v", err)
	}
	if sum1 == sum2 {
		t.Fatalf("expected backend TLS checksum to change when trust secret data changes")
	}
}

func TestBuildTrafficManagerEnvVarsCmanFileModeSkipsGeneratedSettings(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.TrafficManagerSpec{
			Type: networkv4.TrafficManagerTypeCman,
			Cman: &networkv4.CmanTrafficManagerSpec{
				ConfigSource: &networkv4.CmanTrafficManagerConfigSourceSpec{
					ConfigMapRef: &networkv4.TrafficManagerConfigMapKeyRef{
						Name: "cman-config",
						Key:  "cman.ora",
					},
				},
				LogLevel:                 "admin",
				TraceLevel:               "support",
				RegistrationInvitedNodes: "db1",
				RestAPI: &networkv4.CmanTrafficManagerRestAPISpec{
					Enabled: true,
				},
			},
		},
	}

	envs := buildTrafficManagerEnvVars(inst)
	got := map[string]string{}
	for _, env := range envs {
		got[env.Name] = env.Value
	}
	if got["USER_CMAN_FILE"] != cmanUserConfigFilePath {
		t.Fatalf("expected USER_CMAN_FILE %q, got %q", cmanUserConfigFilePath, got["USER_CMAN_FILE"])
	}
	if got["PUBLIC_HOSTNAME"] != "cman" {
		t.Fatalf("expected PUBLIC_HOSTNAME cman, got %q", got["PUBLIC_HOSTNAME"])
	}
	if got["DOMAIN"] != "net.svc.cluster.local" {
		t.Fatalf("expected DOMAIN net.svc.cluster.local, got %q", got["DOMAIN"])
	}
	for _, forbidden := range []string{"LOG_LEVEL", "TRACE_LEVEL", "REGISTRATION_INVITED_NODES", "REST_HOST", "REST_PORT", "ENABLE_REST_API"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("did not expect %s in file mode envs", forbidden)
		}
	}
}

func TestBuildTrafficManagerServiceCmanGeneratedAddsRestPort(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.TrafficManagerSpec{
			Type: networkv4.TrafficManagerTypeCman,
			Cman: &networkv4.CmanTrafficManagerSpec{
				RestAPI: &networkv4.CmanTrafficManagerRestAPISpec{
					Enabled: true,
				},
			},
			Service: networkv4.TrafficManagerServiceSpec{
				External: networkv4.TrafficManagerServiceEndpointSpec{
					Enabled: ptr.To(true),
				},
			},
		},
	}

	svc := buildTrafficManagerService(inst, "external")
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected external CMAN service type LoadBalancer, got %s", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("expected 2 service ports, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Name != "cman" || svc.Spec.Ports[0].Port != 1521 {
		t.Fatalf("unexpected primary CMAN service port: %#v", svc.Spec.Ports[0])
	}
	if svc.Spec.Ports[1].Name != "rest" || svc.Spec.Ports[1].Port != 1525 {
		t.Fatalf("unexpected REST service port: %#v", svc.Spec.Ports[1])
	}
}

func TestBuildTrafficManagerDeploymentCmanAddsConfigAndSecurityContext(t *testing.T) {
	inst := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.TrafficManagerSpec{
			Type: networkv4.TrafficManagerTypeCman,
			Runtime: networkv4.TrafficManagerRuntimeSpec{
				Image:                    "oracle/cman:23.8.0",
				ServiceAccountName:       "cman-sa",
				PodSecurityContext:       &corev1.PodSecurityContext{FSGroup: ptr.To[int64](54321)},
				ContainerSecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)},
			},
			Cman: &networkv4.CmanTrafficManagerSpec{
				ConfigSource: &networkv4.CmanTrafficManagerConfigSourceSpec{
					ConfigMapRef: &networkv4.TrafficManagerConfigMapKeyRef{
						Name: "cman-config",
						Key:  "cman.ora",
					},
				},
			},
		},
	}

	deploy := buildTrafficManagerDeployment(inst, "cfg-hash", "", "")
	if deploy.Spec.Template.Spec.ServiceAccountName != "cman-sa" {
		t.Fatalf("expected service account cman-sa, got %q", deploy.Spec.Template.Spec.ServiceAccountName)
	}
	if deploy.Spec.Template.Spec.SecurityContext == nil || ptr.Deref(deploy.Spec.Template.Spec.SecurityContext.FSGroup, 0) != 54321 {
		t.Fatalf("expected pod security context fsGroup 54321")
	}
	container := deploy.Spec.Template.Spec.Containers[0]
	if container.SecurityContext == nil || ptr.Deref(container.SecurityContext.AllowPrivilegeEscalation, true) {
		t.Fatalf("expected container security context allowPrivilegeEscalation=false")
	}
	foundUserConfig := false
	for _, env := range container.Env {
		if env.Name == "USER_CMAN_FILE" && env.Value == cmanUserConfigFilePath {
			foundUserConfig = true
			break
		}
	}
	if !foundUserConfig {
		t.Fatalf("expected USER_CMAN_FILE env var in CMAN deployment")
	}
}

func TestSyncTrafficManagerDeploymentUpdatesTemplate(t *testing.T) {
	found := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns", Labels: map[string]string{"old": "label"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "tm"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "tm"},
					Annotations: map[string]string{"network.oracle.com/config-hash": "old"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "tm",
						Image: "nginx:old",
					}},
				},
			},
		},
	}
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "tm", Namespace: "ns", Labels: map[string]string{"new": "label"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "tm"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "tm"},
					Annotations: map[string]string{"network.oracle.com/config-hash": "new"},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name: "traffic-manager-backend-tls",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "backend-ca"},
						},
					}},
					Containers: []corev1.Container{{
						Name:  "tm",
						Image: "nginx:new",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "traffic-manager-backend-tls",
							MountPath: "/etc/nginx/backend-ca",
						}},
					}},
				},
			},
		},
	}

	if !syncTrafficManagerDeployment(found, desired) {
		t.Fatalf("expected deployment sync to report update")
	}
	if got := found.Spec.Template.Annotations["network.oracle.com/config-hash"]; got != "new" {
		t.Fatalf("expected updated config hash annotation, got %q", got)
	}
	if got := found.Spec.Template.Spec.Containers[0].Image; got != "nginx:new" {
		t.Fatalf("expected updated container image, got %q", got)
	}
	if len(found.Spec.Template.Spec.Volumes) != 1 || found.Spec.Template.Spec.Volumes[0].Name != "traffic-manager-backend-tls" {
		t.Fatalf("expected updated backend TLS volume, got %#v", found.Spec.Template.Spec.Volumes)
	}
	if found.Spec.Replicas == nil || *found.Spec.Replicas != 2 {
		t.Fatalf("expected updated replicas, got %#v", found.Spec.Replicas)
	}
}
