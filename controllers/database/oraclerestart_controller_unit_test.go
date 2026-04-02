package controllers

import (
	"context"
	"testing"

	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestMergeOracleRestartServicePortsWithAssignedNodePorts(t *testing.T) {
	existing := []corev1.ServicePort{
		{Name: "sql", NodePort: 30001},
		{Name: "em", NodePort: 30002},
	}
	desired := []corev1.ServicePort{
		{Name: "sql"},
		{Name: "em", NodePort: 32002},
		{Name: "new"},
	}

	got := mergeOracleRestartServicePortsWithAssignedNodePorts(existing, desired)

	if len(got) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(got))
	}
	if got[0].NodePort != 30001 {
		t.Fatalf("expected sql nodePort to be preserved as 30001, got %d", got[0].NodePort)
	}
	if got[1].NodePort != 32002 {
		t.Fatalf("expected explicit desired nodePort 32002 to be preserved, got %d", got[1].NodePort)
	}
	if got[2].NodePort != 0 {
		t.Fatalf("expected unmatched port nodePort to remain 0, got %d", got[2].NodePort)
	}
}

func TestCheckOracleRestartState(t *testing.T) {
	tests := []struct {
		name    string
		obj     *oraclerestartdb.OracleRestart
		wantErr bool
	}{
		{
			name: "restricted by failed status",
			obj: &oraclerestartdb.OracleRestart{
				Status: oraclerestartdb.OracleRestartStatus{
					State: string(oraclerestartdb.OracleRestartFailedState),
				},
			},
			wantErr: true,
		},
		{
			name: "restricted by spec isFailed",
			obj: &oraclerestartdb.OracleRestart{
				Spec: oraclerestartdb.OracleRestartSpec{IsFailed: true},
			},
			wantErr: true,
		},
		{
			name: "restricted by spec isManual",
			obj: &oraclerestartdb.OracleRestart{
				Spec: oraclerestartdb.OracleRestartSpec{IsManual: true},
			},
			wantErr: true,
		},
		{
			name: "allowed pending state",
			obj: &oraclerestartdb.OracleRestart{
				Status: oraclerestartdb.OracleRestartStatus{
					State: string(oraclerestartdb.OracleRestartPendingState),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkOracleRestartState(tt.obj)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestOracleRestartPhaseValidationAndDefaults_RestrictedStateRequeues(t *testing.T) {
	r := &OracleRestartReconciler{}
	obj := &oraclerestartdb.OracleRestart{
		Spec: oraclerestartdb.OracleRestartSpec{
			ConfigParams: &oraclerestartdb.InitParams{},
		},
		Status: oraclerestartdb.OracleRestartStatus{
			State: string(oraclerestartdb.OracleRestartFailedState),
		},
	}
	resultQ := ctrl.Result{Requeue: true}

	cName, fName, result, err, earlyExit := r.oracleRestartPhaseValidationAndDefaults(
		context.Background(),
		ctrl.Request{},
		obj,
		nil,
		true,
		resultQ,
	)

	if !earlyExit {
		t.Fatalf("expected earlyExit=true for restricted state")
	}
	if err != nil {
		t.Fatalf("expected nil error for restricted requeue path, got %v", err)
	}
	if !result.Requeue {
		t.Fatalf("expected requeue result for restricted state")
	}
	if cName != "" || fName != "" {
		t.Fatalf("expected empty response file names, got cName=%q fName=%q", cName, fName)
	}
}
