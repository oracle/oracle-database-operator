package diskcheck

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCheckDaemonSetReadyAndDiskValidation_Ready(t *testing.T) {
	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "disk-check-daemonset", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			NumberReady:            1,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(ds).Build()
	kube := k8sfake.NewSimpleClientset()

	ready, invalidDevice, err := CheckDaemonSetReadyAndDiskValidation(
		context.Background(), cl, kube, "ns1", "disk-check-daemonset", "app=disk-check",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected ready=true")
	}
	if invalidDevice {
		t.Fatalf("expected invalidDevice=false")
	}
}

func TestCheckDaemonSetReadyAndDiskValidation_NotReadyNoPods(t *testing.T) {
	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "disk-check-daemonset", Namespace: "ns1"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 1,
			NumberReady:            0,
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(ds).Build()
	kube := k8sfake.NewSimpleClientset()

	ready, invalidDevice, err := CheckDaemonSetReadyAndDiskValidation(
		context.Background(), cl, kube, "ns1", "disk-check-daemonset", "app=disk-check",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected ready=false")
	}
	if invalidDevice {
		t.Fatalf("expected invalidDevice=false")
	}
}
