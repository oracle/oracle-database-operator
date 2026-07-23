package privateai

import (
	"context"
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateReconcileStatus_SetsAndReleasesUpdateLockCondition(t *testing.T) {
	sch := runtime.NewScheme()
	if err := privateaiv4.AddToScheme(sch); err != nil {
		t.Fatalf("failed adding privateai scheme: %v", err)
	}

	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pai-lock-cond",
			Namespace:  "default",
			Generation: 3,
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithStatusSubresource(&privateaiv4.PrivateAi{}).
		WithObjects(inst).
		Build()
	counting := &countingClient{
		Client: baseClient,
		statusWriter: &countingStatusWriter{
			StatusWriter: baseClient.Status(),
		},
	}
	r := &PrivateAiReconciler{Client: counting, Scheme: sch}

	lockState := &reconcileState{
		updateLock:    true,
		updateLockMsg: "controller update lock: image rollout in progress",
	}
	if err := r.updateReconcileStatus(inst, context.Background(), requeueN, nil, lockState); err != nil {
		t.Fatalf("failed setting lock condition: %v", err)
	}

	fetched := &privateaiv4.PrivateAi{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, fetched); err != nil {
		t.Fatalf("failed fetching object after first status update: %v", err)
	}
	lockCond := lockpolicy.FindStatusCondition(fetched.Status.Conditions, lockpolicy.DefaultReconcilingConditionType)
	if lockCond == nil {
		t.Fatalf("expected reconciling lock condition to be present")
	}
	if lockCond.Status != metav1.ConditionTrue || lockCond.Reason != lockpolicy.DefaultUpdateLockReason {
		t.Fatalf("expected active lock condition, got status=%s reason=%s", lockCond.Status, lockCond.Reason)
	}

	releaseState := &reconcileState{completed: true}
	if err := r.updateReconcileStatus(fetched, context.Background(), requeueN, nil, releaseState); err != nil {
		t.Fatalf("failed releasing lock condition: %v", err)
	}

	if err := baseClient.Get(context.Background(), types.NamespacedName{Name: inst.Name, Namespace: inst.Namespace}, fetched); err != nil {
		t.Fatalf("failed fetching object after release status update: %v", err)
	}
	releasedCond := lockpolicy.FindStatusCondition(fetched.Status.Conditions, lockpolicy.DefaultReconcilingConditionType)
	if releasedCond == nil {
		t.Fatalf("expected reconciling condition to exist after release")
	}
	if releasedCond.Status != metav1.ConditionFalse || releasedCond.Reason != "UpdateSettled" {
		t.Fatalf("expected released lock condition, got status=%s reason=%s", releasedCond.Status, releasedCond.Reason)
	}
}
