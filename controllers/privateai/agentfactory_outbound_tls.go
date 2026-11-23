/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package privateai

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	aicommons "github.com/oracle/oracle-database-operator/commons/privateai"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	agentFactoryOutboundTLSMaxCertificateSize = 5 << 20
	agentFactoryOutboundTLSLoginPath          = "/agentFactory/v1/loginValidation"
	agentFactoryOutboundTLSCertificatesPath   = "/agentFactory/v1/certs"
)

// reconcileAgentFactoryOutboundTLSImport registers the configured CA bundle
// with Agent Factory after the workload is ready. The Agent Factory API skips
// certificates it already owns, so retrying this operation is safe.
func (r *AgentFactoryReconciler) reconcileAgentFactoryOutboundTLSImport(ctx context.Context, inst *privateaiv4.AgentFactory) error {
	if inst.Spec.Security == nil || inst.Spec.Security.OutboundTLS == nil || inst.Spec.Security.OutboundTLS.CABundle == nil {
		return nil
	}

	status := &inst.Status.OutboundTLS
	if status.CABundleResourceVersion == "" {
		return r.setAgentFactoryOutboundTLSFailure(inst, "ImportPending", "outbound CA bundle has not been validated")
	}
	if status.CABundleImportedResourceVersion == status.CABundleResourceVersion {
		r.setAgentFactoryCondition(inst, metav1.Condition{
			Type:               agentFactoryOutboundTLSReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Imported",
			Message:            "Outbound CA bundle is registered with Agent Factory",
			ObservedGeneration: inst.Generation,
		})
		return nil
	}

	bundle := inst.Spec.Security.OutboundTLS.CABundle
	caSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: strings.TrimSpace(bundle.SecretName), Namespace: inst.Namespace}, caSecret); err != nil {
		return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleSecretUnavailable", fmt.Sprintf("outbound CA Secret %q is unavailable: %v", bundle.SecretName, err))
	}
	key := agentFactoryOutboundCABundleKey(bundle)
	caBundle, ok := caSecret.Data[key]
	if !ok {
		return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleKeyMissing", fmt.Sprintf("outbound CA Secret %q must contain key %q", caSecret.Name, key))
	}
	if len(caBundle) > agentFactoryOutboundTLSMaxCertificateSize {
		return r.setAgentFactoryOutboundTLSFailure(inst, "CABundleTooLarge", fmt.Sprintf("outbound CA Secret %q key %q exceeds the 5 MiB Agent Factory API limit", caSecret.Name, key))
	}

	secretSpec := agentFactorySecretSpec(inst)
	runtimeSecret := &corev1.Secret{}
	if secretSpec == nil || strings.TrimSpace(secretSpec.Name) == "" {
		return r.setAgentFactoryOutboundTLSFailure(inst, "AdminCredentialsUnavailable", "AgentFactory runtime Secret is required to register outbound certificates")
	}
	if err := r.Get(ctx, types.NamespacedName{Name: strings.TrimSpace(secretSpec.Name), Namespace: inst.Namespace}, runtimeSecret); err != nil {
		return r.setAgentFactoryOutboundTLSFailure(inst, "AdminCredentialsUnavailable", fmt.Sprintf("AgentFactory runtime Secret %q is unavailable: %v", secretSpec.Name, err))
	}
	password := runtimeSecret.Data[agentFactoryUIAdminPasswordKey(secretSpec)]
	if len(password) == 0 || strings.TrimSpace(inst.Spec.Configuration.UIAdminUsername) == "" {
		return r.setAgentFactoryOutboundTLSFailure(inst, "AdminCredentialsUnavailable", "AgentFactory UI administrator credentials are unavailable for outbound certificate registration")
	}

	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: aicommons.AgentFactoryClusterServiceName(inst), Namespace: inst.Namespace}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return r.setAgentFactoryOutboundTLSFailure(inst, "ImportServiceUnavailable", fmt.Sprintf("AgentFactory cluster Service %q is required for outbound certificate registration", aicommons.AgentFactoryClusterServiceName(inst)))
		}
		return r.setAgentFactoryOutboundTLSFailure(inst, "ImportServiceUnavailable", fmt.Sprintf("failed to get AgentFactory cluster Service: %v", err))
	}
	if len(service.Spec.Ports) == 0 || service.Spec.Ports[0].Port == 0 {
		return r.setAgentFactoryOutboundTLSFailure(inst, "ImportServiceUnavailable", fmt.Sprintf("AgentFactory cluster Service %q has no usable port", service.Name))
	}
	serviceHost := fmt.Sprintf("%s.%s.svc", service.Name, service.Namespace)
	baseURL := fmt.Sprintf("https://%s:%d", serviceHost, service.Spec.Ports[0].Port)
	if err := registerAgentFactoryOutboundCABundle(ctx, agentFactoryOutboundTLSHTTPClient(serviceHost), baseURL, strings.TrimSpace(inst.Spec.Configuration.UIAdminUsername), string(password), caBundle); err != nil {
		return r.setAgentFactoryOutboundTLSFailure(inst, "ImportFailed", fmt.Sprintf("failed to register outbound CA bundle with Agent Factory: %v", err))
	}

	status.CABundleImportedResourceVersion = status.CABundleResourceVersion
	r.setAgentFactoryCondition(inst, metav1.Condition{
		Type:               agentFactoryOutboundTLSReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Imported",
		Message:            "Outbound CA bundle is registered with Agent Factory",
		ObservedGeneration: inst.Generation,
	})
	return nil
}

// agentFactoryOutboundTLSHTTPClient uses the in-cluster Agent Factory Service.
// Agent Factory's in-cluster endpoint is addressed by its Service DNS name.
// Keep normal system certificate and hostname verification enabled for this
// authenticated administrative API call.
func agentFactoryOutboundTLSHTTPClient(serverName string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

func registerAgentFactoryOutboundCABundle(ctx context.Context, httpClient *http.Client, baseURL, username, password string, caBundle []byte) error {
	if httpClient == nil {
		return fmt.Errorf("HTTP client is nil")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create Agent Factory session cookie jar: %w", err)
	}
	clientCopy := *httpClient
	clientCopy.Jar = jar

	loginRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+agentFactoryOutboundTLSLoginPath, nil)
	if err != nil {
		return fmt.Errorf("create Agent Factory login request: %w", err)
	}
	loginRequest.SetBasicAuth(username, password)
	loginRequest.Header.Set("Origin", baseURL)
	loginResponse, err := clientCopy.Do(loginRequest)
	if err != nil {
		return fmt.Errorf("Agent Factory login request: %w", err)
	}
	defer func() { _ = loginResponse.Body.Close() }()
	if loginResponse.StatusCode < http.StatusOK || loginResponse.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Agent Factory login returned HTTP %d", loginResponse.StatusCode)
	}
	loginURL, err := http.NewRequest(http.MethodGet, baseURL+agentFactoryOutboundTLSLoginPath, nil)
	if err != nil {
		return fmt.Errorf("create Agent Factory session URL: %w", err)
	}
	if !hasAgentFactorySessionCookie(jar.Cookies(loginURL.URL)) {
		return fmt.Errorf("Agent Factory login did not return an administrator session cookie")
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("certificate", "agentfactory-outbound-ca.crt")
	if err != nil {
		return fmt.Errorf("create Agent Factory certificate form: %w", err)
	}
	if _, err := part.Write(caBundle); err != nil {
		return fmt.Errorf("write Agent Factory certificate form: %w", err)
	}
	if err := form.Close(); err != nil {
		return fmt.Errorf("close Agent Factory certificate form: %w", err)
	}

	certificateRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+agentFactoryOutboundTLSCertificatesPath, &body)
	if err != nil {
		return fmt.Errorf("create Agent Factory certificate request: %w", err)
	}
	certificateRequest.Header.Set("Content-Type", form.FormDataContentType())
	certificateRequest.Header.Set("Origin", baseURL)
	certificateResponse, err := clientCopy.Do(certificateRequest)
	if err != nil {
		return fmt.Errorf("Agent Factory certificate request: %w", err)
	}
	defer func() { _ = certificateResponse.Body.Close() }()
	if certificateResponse.StatusCode != http.StatusOK && certificateResponse.StatusCode != http.StatusCreated {
		return fmt.Errorf("Agent Factory certificate registration returned HTTP %d", certificateResponse.StatusCode)
	}
	return nil
}

func hasAgentFactorySessionCookie(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.Name == "agent_factory_session" && cookie.Value != "" {
			return true
		}
	}
	return false
}
