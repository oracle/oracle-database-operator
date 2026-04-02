package k8sobjects

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsurePersistentVolumeCreateAndValidate(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).Build()

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-a"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				Local: &corev1.LocalVolumeSource{Path: "/dev/a"},
			},
		},
	}

	name, created, err := EnsurePersistentVolume(context.Background(), cl, pv)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !created || name != "pv-a" {
		t.Fatalf("expected created pv-a, got created=%v name=%q", created, name)
	}

	name, created, err = EnsurePersistentVolume(context.Background(), cl, pv)
	if err != nil {
		t.Fatalf("validate existing failed: %v", err)
	}
	if created || name != "pv-a" {
		t.Fatalf("expected existing pv-a, got created=%v name=%q", created, name)
	}
}

func TestEnsurePersistentVolumeRejectsLocalPathMismatch(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	existing := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-b"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				Local: &corev1.LocalVolumeSource{Path: "/dev/old"},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()

	desired := existing.DeepCopy()
	desired.Spec.PersistentVolumeSource.Local = &corev1.LocalVolumeSource{Path: "/dev/new"}
	_, _, err := EnsurePersistentVolume(context.Background(), cl, desired)
	if err == nil {
		t.Fatalf("expected local-path mismatch error")
	}
}

func TestEnsurePersistentVolumeClaimCreateAndGet(t *testing.T) {
	sch := runtime.NewScheme()
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register scheme: %v", err)
	}
	cl := fake.NewClientBuilder().WithScheme(sch).Build()

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-a", Namespace: "ns1"},
	}
	name, created, err := EnsurePersistentVolumeClaim(context.Background(), cl, pvc)
	if err != nil {
		t.Fatalf("create pvc failed: %v", err)
	}
	if !created || name != "pvc-a" {
		t.Fatalf("expected created pvc-a, got created=%v name=%q", created, name)
	}

	name, created, err = EnsurePersistentVolumeClaim(context.Background(), cl, pvc)
	if err != nil {
		t.Fatalf("get pvc failed: %v", err)
	}
	if created || name != "pvc-a" {
		t.Fatalf("expected existing pvc-a, got created=%v name=%q", created, name)
	}
}
