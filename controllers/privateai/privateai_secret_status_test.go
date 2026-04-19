package privateai

import (
	"context"
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureSecretSetsStatusFields(t *testing.T) {
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

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "paisecret", Namespace: "pai"},
		Data: map[string][]byte{
			"api-key":  []byte("topsecret"),
			"cert.pem": []byte("certdata"),
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			PaiSecret: &privateaiv4.PaiSecretSpec{Name: "paisecret", MountLocation: "/privateai/ssl"},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(secret).Build(),
		Scheme: sch,
	}

	if _, err := r.ensureSecret(context.Background(), ctrl.Request{}, inst); err != nil {
		t.Fatalf("ensureSecret returned error: %v", err)
	}

	if inst.Status.PaiSecret.Name != "paisecret" {
		t.Fatalf("expected secret name to be recorded, got %q", inst.Status.PaiSecret.Name)
	}
	if !inst.Status.PaiSecret.HasAPIKey {
		t.Fatalf("expected hasAPIKey=true")
	}
	if !inst.Status.PaiSecret.HasCertPem {
		t.Fatalf("expected hasCertPem=true")
	}
	if inst.Status.PaiSecret.APIKey != "paisecret" || inst.Status.PaiSecret.Certpem != "paisecret" {
		t.Fatalf("expected deprecated compatibility fields to retain secret name")
	}
}

func TestPopulateTrafficManagerAccessStatus(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := networkv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding network scheme: %v", err)
	}

	tm := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-nginx", Namespace: "pai"},
		Status: networkv4.TrafficManagerStatus{
			ExternalService:  "pai-nginx-external",
			ExternalEndpoint: "https://141.148.67.224",
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			TrafficManager: &privateaiv4.TrafficManagerRefSpec{
				Ref:       "pai-nginx",
				RoutePath: "/pai-sample/v1/",
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(tm).Build(),
		Scheme: sch,
	}

	r.populateTrafficManagerAccessStatus(context.Background(), inst.Namespace, inst)

	if inst.Status.TrafficManager.ServiceName != "pai-nginx-external" {
		t.Fatalf("expected traffic manager service name, got %q", inst.Status.TrafficManager.ServiceName)
	}
	if inst.Status.TrafficManager.Endpoint != "https://141.148.67.224" {
		t.Fatalf("expected traffic manager endpoint, got %q", inst.Status.TrafficManager.Endpoint)
	}
	if inst.Status.TrafficManager.PublicURL != "https://141.148.67.224/pai-sample/v1/" {
		t.Fatalf("expected traffic manager public URL, got %q", inst.Status.TrafficManager.PublicURL)
	}
}

func TestPopulateTrafficManagerAccessStatus_NormalizesEndpointAndDefaultRoute(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := networkv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding network scheme: %v", err)
	}

	tm := &networkv4.TrafficManager{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-nginx", Namespace: "pai"},
		Spec: networkv4.TrafficManagerSpec{
			Security: networkv4.TrafficManagerSecuritySpec{
				TLS: networkv4.TrafficManagerTLSSpec{Enabled: true},
			},
		},
		Status: networkv4.TrafficManagerStatus{
			ExternalEndpoint: "141.148.67.224",
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			TrafficManager: &privateaiv4.TrafficManagerRefSpec{
				Ref: "pai-nginx",
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(tm).Build(),
		Scheme: sch,
	}
	inst.Status.TrafficManager.RoutePath = resolvedTrafficManagerRoutePath(inst)
	r.populateTrafficManagerAccessStatus(context.Background(), inst.Namespace, inst)

	if inst.Status.TrafficManager.Endpoint != "https://141.148.67.224" {
		t.Fatalf("expected normalized endpoint, got %q", inst.Status.TrafficManager.Endpoint)
	}
	if inst.Status.TrafficManager.PublicURL != "https://141.148.67.224/pai-sample/v1/" {
		t.Fatalf("expected default-route public URL, got %q", inst.Status.TrafficManager.PublicURL)
	}
}
