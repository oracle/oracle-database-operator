package privateai

import (
	"testing"

	privateaiv4 "github.com/oracle/oracle-database-operator/apis/privateai/v4"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldBlockForUpdateLock_BlocksNewerGenerationWithoutOverride(t *testing.T) {
	r := &PrivateAiReconciler{}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 5,
		},
		Status: privateaiv4.PrivateAiStatus{
			Conditions: []metav1.Condition{
				{
					Type:               lockpolicy.DefaultReconcilingConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             lockpolicy.DefaultUpdateLockReason,
					ObservedGeneration: 4,
					Message:            "update in progress",
				},
			},
		},
	}

	blocked, _, err := r.shouldBlockForUpdateLock(inst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatalf("expected lock to block newer generation")
	}
}

func TestShouldBlockForUpdateLock_AllowsOverride(t *testing.T) {
	r := &PrivateAiReconciler{}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 5,
			Annotations: map[string]string{
				lockpolicy.DefaultOverrideAnnotation: "true",
			},
		},
		Status: privateaiv4.PrivateAiStatus{
			Conditions: []metav1.Condition{
				{
					Type:               lockpolicy.DefaultReconcilingConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             lockpolicy.DefaultUpdateLockReason,
					ObservedGeneration: 4,
					Message:            "update in progress",
				},
			},
		},
	}

	blocked, _, err := r.shouldBlockForUpdateLock(inst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatalf("expected lock override to allow reconcile")
	}
}

func TestShouldBlockForUpdateLock_AllowsSameGeneration(t *testing.T) {
	r := &PrivateAiReconciler{}
	inst := &privateaiv4.PrivateAi{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 4,
		},
		Status: privateaiv4.PrivateAiStatus{
			Conditions: []metav1.Condition{
				{
					Type:               lockpolicy.DefaultReconcilingConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             lockpolicy.DefaultUpdateLockReason,
					ObservedGeneration: 4,
					Message:            "update in progress",
				},
			},
		},
	}

	blocked, _, err := r.shouldBlockForUpdateLock(inst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatalf("expected same-generation reconcile to proceed")
	}
}
