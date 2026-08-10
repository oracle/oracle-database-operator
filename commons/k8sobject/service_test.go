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

func serviceOwnerRef(uid string) metav1.OwnerReference {
	controller := true
	return metav1.OwnerReference{
		APIVersion: "database.oracle.com/v4",
		Kind:       "ShardingDatabase",
		Name:       "sharddb",
		UID:        types.UID(uid),
		Controller: &controller,
	}
}

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
		ObjectMeta: metav1.ObjectMeta{
			Name:            "svc-b",
			Namespace:       "ns1",
			Labels:          map[string]string{"old": "1"},
			OwnerReferences: []metav1.OwnerReference{serviceOwnerRef("owner-1")},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "old"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521, NodePort: 30001}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "svc-b",
			Namespace:       "ns1",
			Labels:          map[string]string{"new": "1"},
			OwnerReferences: []metav1.OwnerReference{serviceOwnerRef("owner-1")},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "new"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521}},
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	changed, err := EnsureService(context.Background(), cl, "ns1", desired, ServiceSyncOptions{
		NodePortMerge:          NodePortMergeByName,
		SyncOwnerReferences:    true,
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

func TestEnsureServiceRefusesUnownedExistingServiceWhenSyncingOwner(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register corev1 scheme: %v", err)
	}

	existing := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "victim-svc", Namespace: "ns1", Labels: map[string]string{"app": "victim"}},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "victim"},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80}},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "victim-svc",
			Namespace:       "ns1",
			Labels:          map[string]string{"app": "gsm"},
			OwnerReferences: []metav1.OwnerReference{serviceOwnerRef("owner-1")},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "gsm"},
			Ports:    []corev1.ServicePort{{Name: "sql", Port: 1521}},
			Type:     corev1.ServiceTypeLoadBalancer,
		},
	}

	changed, err := EnsureService(context.Background(), cl, "ns1", desired, ServiceSyncOptions{
		SyncOwnerReferences: true,
	})
	if err == nil {
		t.Fatalf("expected ownership error")
	}
	if changed {
		t.Fatalf("expected changed=false when ownership validation fails")
	}

	got := &corev1.Service{}
	if err := cl.Get(context.Background(), types.NamespacedName{Namespace: "ns1", Name: "victim-svc"}, got); err != nil {
		t.Fatalf("failed to fetch service: %v", err)
	}
	if got.Spec.Selector["app"] != "victim" || got.Spec.Ports[0].Port != 80 || got.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("expected existing service to remain unchanged, got selector=%v ports=%v type=%s",
			got.Spec.Selector, got.Spec.Ports, got.Spec.Type)
	}
}
