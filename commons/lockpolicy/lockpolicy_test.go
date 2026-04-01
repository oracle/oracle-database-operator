package lockpolicy

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsControllerUpdateLocked(t *testing.T) {
	conds := []metav1.Condition{
		{
			Type:               DefaultReconcilingConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             DefaultUpdateLockReason,
			ObservedGeneration: 7,
			Message:            "in progress",
		},
	}
	locked, gen, msg := IsControllerUpdateLocked(conds, DefaultReconcilingConditionType, DefaultUpdateLockReason)
	if !locked || gen != 7 || msg != "in progress" {
		t.Fatalf("unexpected lock result: locked=%v gen=%d msg=%q", locked, gen, msg)
	}
}

func TestIsControllerUpdateLockedNotLocked(t *testing.T) {
	conds := []metav1.Condition{
		{
			Type:   DefaultReconcilingConditionType,
			Status: metav1.ConditionFalse,
			Reason: DefaultUpdateLockReason,
		},
	}
	locked, _, _ := IsControllerUpdateLocked(conds, DefaultReconcilingConditionType, DefaultUpdateLockReason)
	if locked {
		t.Fatalf("expected unlocked")
	}
}

func TestIsUpdateLockOverrideEnabled(t *testing.T) {
	ok, _ := IsUpdateLockOverrideEnabled(map[string]string{
		DefaultOverrideAnnotation: "TrUe",
	}, DefaultOverrideAnnotation)
	if !ok {
		t.Fatalf("expected override enabled")
	}
	ok, _ = IsUpdateLockOverrideEnabled(map[string]string{
		DefaultOverrideAnnotation: "false",
	}, DefaultOverrideAnnotation)
	if ok {
		t.Fatalf("expected override disabled")
	}
}
