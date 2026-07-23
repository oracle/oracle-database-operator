package controllers

import (
	"reflect"
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestOracleRestDataServicePodUsesORDSImageCommand(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add database scheme: %v", err)
	}
	reconciler := &OracleRestDataServiceReconciler{Scheme: scheme}
	ords := &dbapi.OracleRestDataService{
		Spec: dbapi.OracleRestDataServiceSpec{
			Image: dbapi.OracleRestDataServiceImage{
				PullFrom: "phx.ocir.io/example/ords:latest",
			},
		},
	}
	database := &dbapi.SingleInstanceDatabase{}

	pod := reconciler.instantiatePodSpec(ords, database, ctrl.Request{})
	container := pod.Spec.Containers[0]

	if want := []string{"/usr/bin/ords"}; !reflect.DeepEqual(container.Command, want) {
		t.Fatalf("ORDS command = %v, want %v", container.Command, want)
	}
	if want := []string{"--config", "/etc/ords/config", "serve", "--port", "8080"}; !reflect.DeepEqual(container.Args, want) {
		t.Fatalf("ORDS args = %v, want %v", container.Args, want)
	}
}
