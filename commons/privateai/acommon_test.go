package commons

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type captureLogSink struct {
	messages []string
}

func (s *captureLogSink) Init(logr.RuntimeInfo) {}

func (s *captureLogSink) Enabled(int) bool { return true }

func (s *captureLogSink) Info(_ int, msg string, _ ...interface{}) {
	s.messages = append(s.messages, msg)
}

func (s *captureLogSink) Error(err error, msg string, _ ...interface{}) {
	if err != nil {
		msg += " " + err.Error()
	}
	s.messages = append(s.messages, msg)
}

func (s *captureLogSink) WithValues(_ ...interface{}) logr.LogSink { return s }

func (s *captureLogSink) WithName(_ string) logr.LogSink { return s }

func TestReadSecretRedactsSecretValuesInDebugLogs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	apiKey := "super-secret-api-key"
	certPem := "private-cert-material"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-secret", Namespace: "pai"},
		Data: map[string][]byte{
			"api-key":  []byte(apiKey),
			"cert.pem": []byte(certPem),
		},
	}
	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec:       privateaiv4.PrivateAiSpec{IsDebug: true},
	}
	kClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	logSink := &captureLogSink{}

	gotAPIKey, gotCertPem := ReadSecret("auth-secret", instance, kClient, logr.New(logSink))
	if gotAPIKey != apiKey || gotCertPem != certPem {
		t.Fatalf("expected ReadSecret to return secret values, got apiKey=%q certPem=%q", gotAPIKey, gotCertPem)
	}

	logs := strings.Join(logSink.messages, "\n")
	if strings.Contains(logs, apiKey) || strings.Contains(logs, certPem) {
		t.Fatalf("expected debug logs to redact secret values, got %q", logs)
	}
	if !strings.Contains(logs, "api-key") || !strings.Contains(logs, "cert.pem") || !strings.Contains(logs, "[REDACTED]") {
		t.Fatalf("expected debug logs to keep non-sensitive key diagnostics, got %q", logs)
	}
}

func TestPatchSecretPreservesExistingLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auth-secret",
			Namespace: "pai",
			Labels: map[string]string{
				"team": "ml",
			},
		},
	}
	kClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	if err := PatchSecret("auth-secret", instance, kClient, logr.Discard()); err != nil {
		t.Fatalf("PatchSecret returned error: %v", err)
	}

	got := &corev1.Secret{}
	if err := kClient.Get(context.Background(), client.ObjectKey{Name: "auth-secret", Namespace: "pai"}, got); err != nil {
		t.Fatalf("failed fetching patched secret: %v", err)
	}
	if got.Labels["team"] != "ml" {
		t.Fatalf("expected existing label to be preserved, got labels %#v", got.Labels)
	}
	if got.Labels[privateAIResourceNameLabel] != "PrivateAi-pai-sample" {
		t.Fatalf("expected PrivateAI resource label to be added, got labels %#v", got.Labels)
	}
}

func TestPatchConfigMapPreservesExistingLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-config",
			Namespace: "pai",
			Labels: map[string]string{
				"team": "ml",
			},
		},
	}
	kClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()

	if err := PatchConfigMap("model-config", instance, kClient, logr.Discard()); err != nil {
		t.Fatalf("PatchConfigMap returned error: %v", err)
	}

	got := &corev1.ConfigMap{}
	if err := kClient.Get(context.Background(), client.ObjectKey{Name: "model-config", Namespace: "pai"}, got); err != nil {
		t.Fatalf("failed fetching patched ConfigMap: %v", err)
	}
	if got.Labels["team"] != "ml" {
		t.Fatalf("expected existing label to be preserved, got labels %#v", got.Labels)
	}
	if got.Labels[privateAIResourceNameLabel] != "PrivateAi-pai-sample" {
		t.Fatalf("expected PrivateAI resource label to be added, got labels %#v", got.Labels)
	}
}

func TestPatchSecretCorrectsPrivateAIResourceLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	instance := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auth-secret",
			Namespace: "pai",
			Labels: map[string]string{
				privateAIResourceNameLabel: "PrivateAi-old",
			},
		},
	}
	kClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	if err := PatchSecret("auth-secret", instance, kClient, logr.Discard()); err != nil {
		t.Fatalf("PatchSecret returned error: %v", err)
	}

	got := &corev1.Secret{}
	if err := kClient.Get(context.Background(), client.ObjectKey{Name: "auth-secret", Namespace: "pai"}, got); err != nil {
		t.Fatalf("failed fetching patched secret: %v", err)
	}
	if got.Labels[privateAIResourceNameLabel] != "PrivateAi-pai-sample" {
		t.Fatalf("expected PrivateAI resource label to be corrected, got labels %#v", got.Labels)
	}
}
