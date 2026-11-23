package privateai

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentFactoryOutboundTLSHTTPClientVerifiesServerIdentity(t *testing.T) {
	const serverName = "agentfactory.paf.svc"

	client := agentFactoryOutboundTLSHTTPClient(serverName)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected cloned HTTP transport, got %T", client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLS client configuration")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected TLS server identity verification to remain enabled")
	}
	if transport.TLSClientConfig.ServerName != serverName {
		t.Fatalf("expected TLS ServerName %q, got %q", serverName, transport.TLSClientConfig.ServerName)
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected minimum TLS version %d, got %d", tls.VersionTLS12, transport.TLSClientConfig.MinVersion)
	}
}

func TestRegisterAgentFactoryOutboundCABundleIsIdempotent(t *testing.T) {
	const (
		username = "admin@example.com"
		password = "admin-password"
		bundle   = "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n"
	)
	certificateRequests := 0
	resetRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case agentFactoryOutboundTLSLoginPath:
			gotUsername, gotPassword, ok := request.BasicAuth()
			if !ok || gotUsername != username || gotPassword != password {
				t.Error("Agent Factory login did not receive administrator Basic Auth")
			}
			http.SetCookie(writer, &http.Cookie{Name: "agent_factory_session", Value: "test-session", Path: "/"})
			writer.WriteHeader(http.StatusOK)
		case agentFactoryOutboundTLSCertificatesPath:
			if request.Method != http.MethodPost {
				t.Errorf("expected POST certificate request, got %s", request.Method)
			}
			if !strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/form-data;") {
				t.Errorf("expected multipart certificate request, got %q", request.Header.Get("Content-Type"))
			}
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("failed parsing certificate request: %v", err)
				return
			}
			file, _, err := request.FormFile("certificate")
			if err != nil {
				t.Errorf("certificate form field missing: %v", err)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				t.Errorf("failed reading certificate form field: %v", err)
				return
			}
			if string(data) != bundle {
				t.Errorf("unexpected certificate bundle %q", string(data))
			}
			certificateRequests++
			if certificateRequests == 1 {
				writer.WriteHeader(http.StatusCreated)
			} else {
				writer.WriteHeader(http.StatusOK)
			}
		case "/agentFactory/v1/certs/reset":
			resetRequests++
			writer.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := server.Client()
	for i := 0; i < 2; i++ {
		if err := registerAgentFactoryOutboundCABundle(context.Background(), client, server.URL, username, password, []byte(bundle)); err != nil {
			t.Fatalf("certificate registration attempt %d failed: %v", i+1, err)
		}
	}
	if certificateRequests != 2 {
		t.Fatalf("expected two idempotent certificate requests, got %d", certificateRequests)
	}
	if resetRequests != 0 {
		t.Fatalf("expected no global certificate reset request, got %d", resetRequests)
	}
}
