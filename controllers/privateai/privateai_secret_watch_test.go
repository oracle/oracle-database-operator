package privateai

import (
	"context"
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
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
	if err := networkv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding network scheme: %v", err)
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

func TestPrivateAiRequestsForConfigMap(t *testing.T) {
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
			Configuration: &privateaiv4.PrivateAiConfigurationSpec{
				ConfigFile: &privateaiv4.PaiConfigMap{Name: "model-config", MountLocation: "/privateai/config"},
			},
		},
	}
	legacyReferenced := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-b", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			PaiConfigFile: &privateaiv4.PaiConfigMap{Name: "legacy-config", MountLocation: "/privateai/config"},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(referenced, legacyReferenced).
			Build(),
		Scheme: sch,
	}

	requests := r.privateAiRequestsForConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "model-config", Namespace: "pai"},
	})
	if len(requests) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(requests))
	}
	expected := types.NamespacedName{Name: "pai-a", Namespace: "pai"}
	if requests[0].NamespacedName != expected {
		t.Fatalf("expected request %v, got %v", expected, requests[0].NamespacedName)
	}

	legacyRequests := r.privateAiRequestsForConfigMap(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-config", Namespace: "pai"},
	})
	expectedLegacy := types.NamespacedName{Name: "pai-b", Namespace: "pai"}
	if len(legacyRequests) != 1 || legacyRequests[0].NamespacedName != expectedLegacy {
		t.Fatalf("expected legacy configmap request %v, got %#v", expectedLegacy, legacyRequests)
	}
}

func TestPrivateAiRequestsForTrafficManager(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := networkv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding network scheme: %v", err)
	}

	referenced := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-a", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Networking: &privateaiv4.PrivateAiNetworkingSpec{
				TrafficManager: &privateaiv4.TrafficManagerRefSpec{
					Ref:       "pai-nginx",
					RoutePath: "/pai-a/v1/",
				},
			},
		},
	}
	legacyReferenced := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-b", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			TrafficManager: &privateaiv4.TrafficManagerRefSpec{
				Ref: "legacy-nginx",
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(sch).
			WithObjects(referenced, legacyReferenced).
			Build(),
		Scheme: sch,
	}

	requests := r.privateAiRequestsForTrafficManager(context.Background(), &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-nginx", Namespace: "pai"},
	})
	if len(requests) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(requests))
	}
	expected := types.NamespacedName{Name: "pai-a", Namespace: "pai"}
	if requests[0].NamespacedName != expected {
		t.Fatalf("expected request %v, got %v", expected, requests[0].NamespacedName)
	}

	legacyRequests := r.privateAiRequestsForTrafficManager(context.Background(), &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-nginx", Namespace: "pai"},
	})
	expectedLegacy := types.NamespacedName{Name: "pai-b", Namespace: "pai"}
	if len(legacyRequests) != 1 || legacyRequests[0].NamespacedName != expectedLegacy {
		t.Fatalf("expected legacy traffic manager request %v, got %#v", expectedLegacy, legacyRequests)
	}
}
