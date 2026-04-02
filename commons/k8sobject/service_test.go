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

func TestEnsureServiceCreatesWhenMissing(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register corev1 scheme: %v", err)
	}

	cl := fake.NewClientBuilder().WithScheme(sch).Build()
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "ns1"},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "x"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	changed, err := EnsureService(context.Background(), cl, "ns1", desired, ServiceSyncOptions{
		NodePortMerge:          NodePortMergeByName,
		SyncLoadBalancerFields: true,
	})
	if err != nil {
		t.Fatalf("EnsureService create failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true on create")
	}
}

func TestEnsureServiceUpdatesAndPreservesNodePort(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register corev1 scheme: %v", err)
	}

	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "ns1", Labels: map[string]string{"old": "1"}},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "old"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521, NodePort: 30001}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "ns1", Labels: map[string]string{"new": "1"}},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "new"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	changed, err := EnsureService(context.Background(), cl, "ns1", desired, ServiceSyncOptions{
		NodePortMerge:          NodePortMergeByName,
		SyncLoadBalancerFields: true,
	})
	if err != nil {
		t.Fatalf("EnsureService update failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true on update")
	}

	got := &corev1.Service{}
	if err := cl.Get(context.Background(), types.NamespacedName{Namespace: "ns1", Name: "svc-b"}, got); err != nil {
		t.Fatalf("failed to fetch updated service: %v", err)
	}
	if got.Spec.Ports[0].NodePort != 30001 {
		t.Fatalf("expected nodePort preserved, got %d", got.Spec.Ports[0].NodePort)
	}
	if got.Labels["new"] != "1" {
		t.Fatalf("expected labels to be updated")
	}
}
