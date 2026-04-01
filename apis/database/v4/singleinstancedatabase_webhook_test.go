package v4

import (
	"context"
	"strings"
	"testing"

	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSIDBWebhookValidateUpdateRejectsSpecChangeWhenLockedWithoutOverride(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Sid = "NEWSID"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected validate update to fail for locked spec change")
	}
	if !strings.Contains(err.Error(), "spec updates are blocked while controller operation is in progress") {
		t.Fatalf("expected lock rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsSpecChangeWhenLockedWithOverride(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Sid = "NEWSID"
	newObj.SetAnnotations(map[string]string{lockpolicy.DefaultOverrideAnnotation: "true"})

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected validate update to pass with override, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsMetadataOnlyChangeWhenLocked(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.SetAnnotations(map[string]string{"team": "db"})

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected metadata-only update to pass while locked, got: %v", err)
	}
}
