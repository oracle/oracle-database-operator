package privateai

import (
	"context"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateReconcileStatus_KeepsUpdatingWhileRolloutInProgress(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding apps scheme: %v", err)
	}

	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-rollout", Namespace: "default"},
		Spec:       privateaiv4.PrivateAiSpec{Replicas: 1},
		Status: privateaiv4.PrivateAiStatus{
			Status: privateaiv4.StatusUpdating,
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pai-rollout",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			UpdatedReplicas:    0,
			ReadyReplicas:      0,
			AvailableReplicas:  0,
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithStatusSubresource(&privateaiv4.PrivateAi{}).
		WithObjects(inst, deploy).
		Build()

	r := &PrivateAiReconciler{Client: baseClient, Scheme: sch}
	state := &reconcileState{completed: true}
	if err := r.updateReconcileStatus(inst, context.Background(), requeueN, nil, state); err != nil {
		t.Fatalf("updateReconcileStatus failed: %v", err)
	}

	fetched := &privateaiv4.PrivateAi{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, fetched); err != nil {
		t.Fatalf("failed fetching updated privateai: %v", err)
	}
	if fetched.Status.Status != privateaiv4.StatusUpdating {
		t.Fatalf("expected status %q during rollout, got %q", privateaiv4.StatusUpdating, fetched.Status.Status)
	}
}

func TestUpdateReconcileStatus_SetsHealthyAfterRolloutSettles(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}
	if err := appsv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding apps scheme: %v", err)
	}

	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{Name: "pai-ready", Namespace: "default"},
		Spec:       privateaiv4.PrivateAiSpec{Replicas: 1},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pai-ready",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
			AvailableReplicas:  1,
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithStatusSubresource(&privateaiv4.PrivateAi{}).
		WithObjects(inst, deploy).
		Build()

	r := &PrivateAiReconciler{Client: baseClient, Scheme: sch}
	state := &reconcileState{completed: true}
	if err := r.updateReconcileStatus(inst, context.Background(), requeueN, nil, state); err != nil {
		t.Fatalf("updateReconcileStatus failed: %v", err)
	}

	fetched := &privateaiv4.PrivateAi{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, fetched); err != nil {
		t.Fatalf("failed fetching updated privateai: %v", err)
	}
	if fetched.Status.Status != privateaiv4.StatusReady {
		t.Fatalf("expected status %q after rollout settled, got %q", privateaiv4.StatusReady, fetched.Status.Status)
	}
}
