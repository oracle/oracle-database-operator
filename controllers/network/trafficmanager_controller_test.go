package network

import (
	"context"
	"strings"
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
