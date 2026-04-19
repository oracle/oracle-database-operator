package network

import (
	"context"
	"strings"
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	deploy := buildTrafficManagerDeployment(inst, "config-hash", "tls-hash")
	got := deploy.Spec.Template.Annotations["network.oracle.com/tls-secret-hash"]
	if got != "tls-hash" {
		t.Fatalf("expected TLS secret hash annotation, got %q", got)
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
