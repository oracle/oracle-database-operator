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
