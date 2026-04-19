package v1alpha1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDataguardBrokerValidateUpdateAllowsDeletionWithLegacyStatusMismatch(t *testing.T) {
	timestamp := metav1.Now()
	oldObj := &DataguardBroker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dg",
			Namespace: "default",
		},
		Spec: DataguardBrokerSpec{
			PrimaryDatabaseRef:  "legacy-primary",
			StandbyDatabaseRefs: []string{"legacy-standby"},
			ProtectionMode:      "MaxPerformance",
		},
		Status: DataguardBrokerStatus{
			PrimaryDatabaseRef: "resolved-primary",
			ProtectionMode:     "MaxPerformance",
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.DeletionTimestamp = &timestamp
	newObj.Spec.PrimaryDatabaseRef = ""

	_, err := (&DataguardBroker{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected deletion update to be allowed, got error: %v", err)
	}
}
