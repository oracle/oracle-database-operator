package k8sobjects

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileStatefulSetCreateAndUpdate(t *testing.T) {
	sch := runtime.NewScheme()
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	cl := fake.NewClientBuilder().WithScheme(sch).Build()
	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sfs-a", Namespace: "ns1"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "a"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "img:v1"}}},
			},
		},
	}

	res, err := ReconcileStatefulSet(context.Background(), cl, "ns1", desired, nil)
	if err != nil || !res.Created {
		t.Fatalf("expected created statefulset, got res=%+v err=%v", res, err)
	}

	desired2 := desired.DeepCopy()
	desired2.Spec.Template.Spec.Containers[0].Image = "img:v2"
	res, err = ReconcileStatefulSet(context.Background(), cl, "ns1", desired2, func(found, desired *appsv1.StatefulSet) bool {
		if found.Spec.Template.Spec.Containers[0].Image == desired.Spec.Template.Spec.Containers[0].Image {
			return false
		}
		found.Spec.Template = desired.Spec.Template
		return true
	})
	if err != nil || !res.Updated {
		t.Fatalf("expected updated statefulset, got res=%+v err=%v", res, err)
	}

	got := &appsv1.StatefulSet{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "sfs-a", Namespace: "ns1"}, got); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "img:v2" {
		t.Fatalf("expected image update, got %s", got.Spec.Template.Spec.Containers[0].Image)
	}
}
