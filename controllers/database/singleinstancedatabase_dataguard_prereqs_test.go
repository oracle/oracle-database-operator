package controllers

import (
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldRunDataguardPrereqsDisabled(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{}
	if shouldRunDataguardPrereqs(sidb) {
		t.Fatalf("expected disabled prereqs to skip execution")
	}
}

func TestShouldRunDataguardPrereqsFirstRun(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb", Namespace: "ns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
			},
		},
	}
	if !shouldRunDataguardPrereqs(sidb) {
		t.Fatalf("expected initial prereqs run to be required")
	}
}

func TestShouldRunDataguardPrereqsSkipAfterSuccess(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb", Namespace: "ns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true, StandbyRedoSize: "512M"},
			},
		},
	}
	sidb.Status.DataguardPrereqsHash = dataguardPrereqsDesiredHash(sidb)
	sidb.Status.Conditions = []metav1.Condition{{
		Type:   sidbConditionDataguardPrereqsReady,
		Status: metav1.ConditionTrue,
	}}
	if shouldRunDataguardPrereqs(sidb) {
		t.Fatalf("expected successful prereqs state to skip execution")
	}
}

func TestShouldRunDataguardPrereqsRerunTokenChange(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "sidb",
			Namespace:   "ns",
			Annotations: map[string]string{sidbDataguardPrereqsRerunAnnotation: "run-2"},
		},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DataguardPrereqsRerunToken: "run-1",
		},
	}
	sidb.Status.DataguardPrereqsHash = dataguardPrereqsDesiredHash(sidb)
	sidb.Status.Conditions = []metav1.Condition{{
		Type:   sidbConditionDataguardPrereqsReady,
		Status: metav1.ConditionTrue,
	}}
	if !shouldRunDataguardPrereqs(sidb) {
		t.Fatalf("expected rerun token change to force execution")
	}
}
