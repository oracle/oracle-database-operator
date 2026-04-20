package commons

import (
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveServicePortDefaultsToHTTPSWhenUnset(t *testing.T) {
	spec := &privateaiv4.PrivateAiSpec{}

	port, scheme, httpEnabled, httpsEnabled := resolveServicePort(spec)

	if port != 8443 {
		t.Fatalf("expected fallback port 8443, got %d", port)
	}
	if scheme != corev1.URISchemeHTTPS {
		t.Fatalf("expected HTTPS scheme, got %q", scheme)
	}
	if httpEnabled {
		t.Fatalf("expected HTTP to be disabled in fallback mode")
	}
	if !httpsEnabled {
		t.Fatalf("expected HTTPS to be enabled in fallback mode")
	}
}

func TestResolveServicePortDefaultsHTTPPortWhenHTTPEnabled(t *testing.T) {
	spec := &privateaiv4.PrivateAiSpec{
		PaiHTTPEnabled: "true",
	}

	port, scheme, httpEnabled, httpsEnabled := resolveServicePort(spec)

	if port != 8080 {
		t.Fatalf("expected fallback port 8080, got %d", port)
	}
	if scheme != corev1.URISchemeHTTP {
		t.Fatalf("expected HTTP scheme, got %q", scheme)
	}
	if !httpEnabled {
		t.Fatalf("expected HTTP to be enabled")
	}
	if httpsEnabled {
		t.Fatalf("expected HTTPS to be disabled")
	}
}

func TestBuildVolumeSpecForPrivateAI_UsesProjectedVolumeForSharedSecretMount(t *testing.T) {
	instance := &privateaiv4.PrivateAi{
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				Secret: &privateaiv4.PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
				},
				TLS: &privateaiv4.PaiTLSSpec{
					SecretName:    "tls-secret",
					MountLocation: "/run/secrets",
				},
			},
		},
	}

	volumes := buildVolumeSpecForPrivateAI(instance)
	if len(volumes) != 2 {
		t.Fatalf("expected projected secret volume plus log volume, got %d volumes", len(volumes))
	}
	projected := volumes[0].Projected
	if projected == nil {
		t.Fatalf("expected first volume to be projected")
	}
	if len(projected.Sources) != 2 {
		t.Fatalf("expected 2 projected sources, got %d", len(projected.Sources))
	}
	if projected.Sources[0].Secret == nil || projected.Sources[0].Secret.Name != "auth-secret" {
		t.Fatalf("expected first projected source to be auth-secret, got %#v", projected.Sources[0].Secret)
	}
	if projected.Sources[1].Secret == nil || projected.Sources[1].Secret.Name != "tls-secret" {
		t.Fatalf("expected second projected source to be tls-secret, got %#v", projected.Sources[1].Secret)
	}
}

func TestBuildVolumeMountSpecForPrivateAI_UsesSingleMountForSharedSecretMount(t *testing.T) {
	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample"},
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				Secret: &privateaiv4.PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
				},
				TLS: &privateaiv4.PaiTLSSpec{
					SecretName:    "tls-secret",
					MountLocation: "/run/secrets",
				},
			},
		},
	}

	mounts := buildVolumeMountSpecForPrivateAI(instance)
	if len(mounts) != 2 {
		t.Fatalf("expected shared secret mount plus log mount, got %d mounts", len(mounts))
	}
	if mounts[0].Name != "pai-sample-secrets-vol" {
		t.Fatalf("expected shared secret mount name, got %q", mounts[0].Name)
	}
	if mounts[0].MountPath != "/run/secrets" {
		t.Fatalf("expected shared mount path /run/secrets, got %q", mounts[0].MountPath)
	}
}
