package privateai

import (
	"context"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPrivateAiRequestsForSecret(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	referenced := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-a", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				Secret: &privateaiv4.PaiSecretSpec{Name: "auth-secret", MountLocation: "/privateai/auth"},
				TLS:    &privateaiv4.PaiTLSSpec{SecretName: "tls-secret", MountLocation: "/privateai/tls"},
			},
		},
	}
	otherSecret := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-b", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			PaiSecret: &privateaiv4.PaiSecretSpec{Name: "other-secret", MountLocation: "/privateai/ssl"},
		},
	}
	otherNamespace := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-c", Namespace: "other"},
		Spec: privateaiv4.PrivateAiSpec{
			PaiSecret: &privateaiv4.PaiSecretSpec{Name: "tls-secret", MountLocation: "/privateai/ssl"},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(referenced, otherSecret, otherNamespace).
			Build(),
		Scheme: sch,
	}

	requests := r.privateAiRequestsForSecret(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-secret", Namespace: "pai"},
	})

	if len(requests) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(requests))
	}
	expected := types.NamespacedName{Name: "pai-a", Namespace: "pai"}
	if requests[0].NamespacedName != expected {
		t.Fatalf("expected request %v, got %v", expected, requests[0].NamespacedName)
	}

	authRequests := r.privateAiRequestsForSecret(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-secret", Namespace: "pai"},
	})
	if len(authRequests) != 1 || authRequests[0].NamespacedName != expected {
		t.Fatalf("expected auth secret request %v, got %#v", expected, authRequests)
	}
}
