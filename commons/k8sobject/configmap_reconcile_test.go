package k8sobjects

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureConfigMapExistsCreates(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).Build()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-a", Namespace: "ns1"}}

	created, err := EnsureConfigMapExists(context.Background(), cl, "ns1", cm)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}
}

func TestEnsureConfigMapEnvfileUpdate(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-b", Namespace: "ns1"},
		Data:       map[string]string{"envfile": "A=1"},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-b", Namespace: "ns1"},
		Data:       map[string]string{"envfile": "A=2"},
	}

	changed, err := EnsureConfigMapEnvfile(context.Background(), cl, "ns1", desired)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	got := &corev1.ConfigMap{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cm-b", Namespace: "ns1"}, got); err != nil {
		t.Fatalf("failed to fetch configmap: %v", err)
	}
	if got.Data["envfile"] != "A=2" {
		t.Fatalf("expected envfile updated, got %q", got.Data["envfile"])
	}
}

func TestEnsureConfigMapEnvfileNoChange(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-c", Namespace: "ns1"},
		Data:       map[string]string{"envfile": "A=1"},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-c", Namespace: "ns1"},
		Data:       map[string]string{"envfile": "A=1"},
	}

	changed, err := EnsureConfigMapEnvfile(context.Background(), cl, "ns1", desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false")
	}
}
