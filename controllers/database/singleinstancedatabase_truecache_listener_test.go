package controllers

import (
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestSetupListenerForDGOnDatabaseSkipsTrueCache(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
		},
	}

	if err := SetupListenerForDGOnDatabase(nil, sidb, corev1.Pod{}, nil, ctrl.Request{}); err != nil {
		t.Fatalf("expected truecache listener setup to be skipped, got error: %v", err)
	}
}
