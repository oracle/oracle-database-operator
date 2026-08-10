package v1alpha1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSingleInstanceDatabaseValidateUpdateAllowsDeleteWithUnchangedSpec(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "truecache",
			Namespace:  "default",
			Finalizers: []string{"database.oracle.com/singleinstancedatabasefinalizer"},
		},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Edition:  "enterprise",
			Image: SingleInstanceDatabaseImage{
				PullFrom: "example.com/db:latest",
			},
			InitParams: &SingleInstanceDatabaseInitParams{
				SgaTarget:          7376,
				PgaAggregateTarget: 2458,
				Processes:          360,
			},
		},
		Status: SingleInstanceDatabaseStatus{
			Role:      "TRUE_CACHE",
			CreatedAs: "truecache",
			InitParams: SingleInstanceDatabaseInitParams{
				SgaTarget:          6,
				PgaAggregateTarget: 2,
				CpuCount:           8,
				Processes:          360,
			},
		},
	}

	newObj := oldObj.DeepCopy()
	newObj.DeletionTimestamp = &now
	newObj.Finalizers = nil
	newObj.Status.Status = "Deleting"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected delete-time update with unchanged spec to pass, got: %v", err)
	}
}
