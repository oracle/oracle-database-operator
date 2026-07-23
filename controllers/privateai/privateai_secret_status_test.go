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
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "paisecret",
			Namespace: "pai",
			Labels: map[string]string{
				"team": "ml",
			},
		},
		Data: map[string][]byte{
			"api-key":  []byte("topsecret"),
			"cert.pem": []byte("certdata"),
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				Secret: &privateaiv4.PaiSecretSpec{Name: "paisecret", MountLocation: "/privateai/auth"},
			},
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

	fetchedSecret := &corev1.Secret{}
	if err := r.Client.Get(context.Background(), client.ObjectKey{Name: "paisecret", Namespace: "pai"}, fetchedSecret); err != nil {
		t.Fatalf("failed fetching secret: %v", err)
	}
	if fetchedSecret.Labels["team"] != "ml" {
		t.Fatalf("expected referenced secret labels to be preserved, got %#v", fetchedSecret.Labels)
	}
	if _, ok := fetchedSecret.Labels["app.kubernetes.io/privateai-resource-name"]; ok {
		t.Fatalf("expected ensureSecret not to add PrivateAI labels to referenced secret, got %#v", fetchedSecret.Labels)
	}
}

func TestEnsureConfigMapDoesNotPatchReferencedLabels(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
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
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Configuration: &privateaiv4.PrivateAiConfigurationSpec{
				ConfigFile: &privateaiv4.PaiConfigMap{Name: "model-config", MountLocation: "/privateai/config"},
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(configMap).Build(),
		Scheme: sch,
	}

	if _, err := r.ensureConfigMap(context.Background(), ctrl.Request{}, inst); err != nil {
		t.Fatalf("ensureConfigMap returned error: %v", err)
	}
	if inst.Status.PaiConfigMap.Name != "model-config" {
		t.Fatalf("expected configMap name to be recorded, got %q", inst.Status.PaiConfigMap.Name)
	}

	fetchedConfigMap := &corev1.ConfigMap{}
	if err := r.Client.Get(context.Background(), client.ObjectKey{Name: "model-config", Namespace: "pai"}, fetchedConfigMap); err != nil {
		t.Fatalf("failed fetching ConfigMap: %v", err)
	}
	if fetchedConfigMap.Labels["team"] != "ml" {
		t.Fatalf("expected referenced ConfigMap labels to be preserved, got %#v", fetchedConfigMap.Labels)
	}
	if _, ok := fetchedConfigMap.Labels["app.kubernetes.io/privateai-resource-name"]; ok {
		t.Fatalf("expected ensureConfigMap not to add PrivateAI labels to referenced ConfigMap, got %#v", fetchedConfigMap.Labels)
	}
}

func TestEnsureTLSSecretSetsStatusFields(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample-tls", Namespace: "pai", ResourceVersion: "7"},
		Data: map[string][]byte{
			"tls.crt": []byte("crt"),
			"tls.key": []byte("key"),
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				TLS: &privateaiv4.PaiTLSSpec{SecretName: "pai-sample-tls", MountLocation: "/privateai/tls"},
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(secret).Build(),
		Scheme: sch,
	}

	if _, err := r.ensureTLSSecret(context.Background(), ctrl.Request{}, inst); err != nil {
		t.Fatalf("ensureTLSSecret returned error: %v", err)
	}

	if inst.Status.TLSSecret.Name != "pai-sample-tls" {
		t.Fatalf("expected tls secret name to be recorded, got %q", inst.Status.TLSSecret.Name)
	}
	if inst.Status.TLSSecret.ResourceVersion != "7" {
		t.Fatalf("expected tls secret resource version 7, got %q", inst.Status.TLSSecret.ResourceVersion)
	}
	if len(inst.Spec.Security.TLS.Items) != 2 {
		t.Fatalf("expected inferred TLS items for tls.crt and tls.key, got %#v", inst.Spec.Security.TLS.Items)
	}
	if inst.Spec.Security.TLS.Items[0].Key != "tls.crt" || inst.Spec.Security.TLS.Items[0].Path != "cert.pem" {
		t.Fatalf("expected tls.crt to map to cert.pem, got %#v", inst.Spec.Security.TLS.Items[0])
	}
	if inst.Spec.Security.TLS.Items[1].Key != "tls.key" || inst.Spec.Security.TLS.Items[1].Path != "key.pem" {
		t.Fatalf("expected tls.key to map to key.pem, got %#v", inst.Spec.Security.TLS.Items[1])
	}
}

func TestEnsureTLSSecretPreservesExplicitItems(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding core scheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample-tls", Namespace: "pai", ResourceVersion: "8"},
		Data: map[string][]byte{
			"tls.crt":      []byte("crt"),
			"tls.key":      []byte("key"),
			"keystore.p12": []byte("ks"),
		},
	}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-sample", Namespace: "pai"},
		Spec: privateaiv4.PrivateAiSpec{
			Security: &privateaiv4.PrivateAiSecuritySpec{
				TLS: &privateaiv4.PaiTLSSpec{
					SecretName:    "pai-sample-tls",
					MountLocation: "/privateai/tls",
					Items: []privateaiv4.SecretMountItem{
						{Key: "tls.crt", Path: "tls.crt"},
					},
				},
			},
		},
	}

	r := &PrivateAiReconciler{
		Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(secret).Build(),
		Scheme: sch,
	}

	if _, err := r.ensureTLSSecret(context.Background(), ctrl.Request{}, inst); err != nil {
		t.Fatalf("ensureTLSSecret returned error: %v", err)
	}

	if len(inst.Spec.Security.TLS.Items) != 1 {
		t.Fatalf("expected explicit TLS items to be preserved, got %#v", inst.Spec.Security.TLS.Items)
	}
	if inst.Spec.Security.TLS.Items[0].Path != "tls.crt" {
		t.Fatalf("expected explicit TLS path to remain unchanged, got %#v", inst.Spec.Security.TLS.Items[0])
	}
}

func TestInferDefaultTLSSecretItemsIncludesOptionalKeystoreWhenPresent(t *testing.T) {
	items := inferDefaultTLSSecretItems(map[string][]byte{
		"tls.crt":      []byte("crt"),
		"tls.key":      []byte("key"),
		"keystore.p12": []byte("ks"),
	})

	if len(items) != 3 {
		t.Fatalf("expected 3 inferred TLS items, got %#v", items)
	}
	if items[2].Key != "keystore.p12" || items[2].Path != "keystore" {
		t.Fatalf("expected keystore.p12 to map to keystore, got %#v", items[2])
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
