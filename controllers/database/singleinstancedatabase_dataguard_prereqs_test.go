package controllers

import (
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigureStandbyFALServerSQLQuotesPrimaryConnectString(t *testing.T) {
	sql := standbyFALServerSQL("primary.example:1521/ORCL'PRD")
	if sql != "ALTER SYSTEM SET fal_server='primary.example:1521/ORCL''PRD' SCOPE=BOTH SID='*';" {
		t.Fatalf("unexpected FAL_SERVER SQL: %q", sql)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

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

func TestDataguardBrokerFSFOEnabled(t *testing.T) {
	tests := []struct {
		name   string
		broker *dbapi.DataguardBroker
		want   bool
	}{
		{name: "disabled", broker: &dbapi.DataguardBroker{}, want: false},
		{name: "legacy spec", broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{FastStartFailover: true}}, want: true},
		{name: "observed status", broker: &dbapi.DataguardBroker{Status: dbapi.DataguardBrokerStatus{FastStartFailover: "true"}}, want: true},
		{name: "topology policy", broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{Topology: &dbapi.DataguardTopologySpec{Policy: &dbapi.DataguardPolicySpec{FastStartFailover: boolPtr(true)}}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataguardBrokerFSFOEnabled(tt.broker); got != tt.want {
				t.Fatalf("dataguardBrokerFSFOEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDataguardBrokerReferencesSIDB(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby3", Namespace: "default"},
		Status:     dbapi.SingleInstanceDatabaseStatus{Sid: "ORCLSTBY3"},
	}
	tests := []struct {
		name   string
		broker *dbapi.DataguardBroker
		want   bool
	}{
		{
			name: "legacy standby",
			broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{
				StandbyDatabaseRefs: []string{"sidb-standby3"},
			}},
			want: true,
		},
		{
			name: "topology member identifiers",
			broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{
				Topology: &dbapi.DataguardTopologySpec{Members: []dbapi.DataguardTopologyMember{{
					Name:         "sidb-standby3",
					DBUniqueName: "ORCLSTBY3",
				}}},
			}},
			want: true,
		},
		{
			name: "topology local ref",
			broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{
				Topology: &dbapi.DataguardTopologySpec{Members: []dbapi.DataguardTopologyMember{{
					LocalRef: &dbapi.DataguardLocalRef{Name: "sidb-standby3", Namespace: "default"},
				}}},
			}},
			want: true,
		},
		{
			name:   "unrelated broker",
			broker: &dbapi.DataguardBroker{Spec: dbapi.DataguardBrokerSpec{PrimaryDatabaseRef: "other-primary"}},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataguardBrokerReferencesSIDB(tt.broker, sidb); got != tt.want {
				t.Fatalf("dataguardBrokerReferencesSIDB() = %v, want %v", got, tt.want)
			}
		})
	}
}
