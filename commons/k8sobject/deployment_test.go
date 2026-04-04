package k8sobjects

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileDeploymentCreate(t *testing.T) {
	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}

	cl := fake.NewClientBuilder().WithScheme(sch).Build()
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns-a"},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep-a"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep-a"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
				},
			},
		},
	}

	found, result, err := ReconcileDeployment(context.Background(), cl, "ns-a", desired, nil)
	if err != nil {
		t.Fatalf("reconcile create failed: %v", err)
	}
	if !result.Created || result.Updated {
		t.Fatalf("expected created=true and updated=false, got %+v", result)
	}
	if found == nil || found.Name != "dep-a" {
		t.Fatalf("expected created deployment to be returned")
	}
}

func TestReconcileDeploymentUpdateViaCallback(t *testing.T) {
	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add appsv1 scheme: %v", err)
	}

	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep-b", Namespace: "ns-b"},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dep-b"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dep-b"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "busybox:1"}},
				},
			},
		},
	}
	desired := existing.DeepCopy()
	desired.Spec.Template.Spec.Containers[0].Image = "busybox:2"

	cl := fake.NewClientBuilder().WithScheme(sch).WithObjects(existing).Build()
	_, result, err := ReconcileDeployment(context.Background(), cl, "ns-b", desired, func(found, desired *appsv1.Deployment) bool {
		if found.Spec.Template.Spec.Containers[0].Image == desired.Spec.Template.Spec.Containers[0].Image {
			return false
		}
		found.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
		return true
	})
	if err != nil {
		t.Fatalf("reconcile update failed: %v", err)
	}
	if result.Created || !result.Updated {
		t.Fatalf("expected created=false and updated=true, got %+v", result)
	}

	updated := &appsv1.Deployment{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "dep-b", Namespace: "ns-b"}, updated); err != nil {
		t.Fatalf("failed to fetch updated deployment: %v", err)
	}
	if got := updated.Spec.Template.Spec.Containers[0].Image; got != "busybox:2" {
		t.Fatalf("expected image busybox:2, got %s", got)
	}
}
