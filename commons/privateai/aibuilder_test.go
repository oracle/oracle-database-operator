package commons

import (
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	corev1 "k8s.io/api/core/v1"
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
