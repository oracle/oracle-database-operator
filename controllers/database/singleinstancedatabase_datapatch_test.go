package controllers

import (
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
)

func TestSIDBPatchFingerprintIsOrderIndependent(t *testing.T) {
	first := sidbPatchFingerprint("35943157\n35926646\n35943157\n")
	second := sidbPatchFingerprint("35926646\n35943157\n")
	if first != second {
		t.Fatalf("expected equivalent patch inventories to have the same fingerprint: %q != %q", first, second)
	}
	if first == sidbPatchFingerprint("35943157\n") {
		t.Fatal("expected different patch inventories to have different fingerprints")
	}
}

func TestSIDBDatapatchIsCurrent(t *testing.T) {
	previous := &dbapi.SingleInstanceDatabaseDatapatchStatus{
		OracleHomePatchFingerprint: "oracle-home",
		SQLPatchFingerprint:        "sql-patch",
	}
	for _, test := range []struct {
		name      string
		previous  *dbapi.SingleInstanceDatabaseDatapatchStatus
		inventory string
		sql       string
		want      bool
	}{
		{name: "matching fingerprints", previous: previous, inventory: "oracle-home", sql: "sql-patch", want: true},
		{name: "new oracle home patch", previous: previous, inventory: "new-oracle-home", sql: "sql-patch", want: false},
		{name: "database SQL patch changed", previous: previous, inventory: "oracle-home", sql: "new-sql-patch", want: false},
		{name: "no prior datapatch status", inventory: "oracle-home", sql: "sql-patch", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sidbDatapatchIsCurrent(test.previous, test.inventory, test.sql); got != test.want {
				t.Fatalf("sidbDatapatchIsCurrent() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRecordSIDBDatapatchStatus(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{}
	sidb.Name = "sidb-sample"
	sidb.Spec.Image.PullFrom = "container-registry.oracle.com/database:23.26"
	pod := corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
		{Name: "sidecar", ImageID: "sidecar@sha256:1"},
		{Name: "sidb-sample", ImageID: "database@sha256:2"},
	}}}

	recordSIDBDatapatchStatus(sidb, pod, "oracle-home", "sql-patch")

	if sidb.Status.Datapatch == nil {
		t.Fatal("expected datapatch status to be recorded")
	}
	if got := sidb.Status.Datapatch.ImageID; got != "database@sha256:2" {
		t.Fatalf("ImageID = %q, want database image ID", got)
	}
	if got := sidb.Status.Datapatch.OracleHomePatchFingerprint; got != "oracle-home" {
		t.Fatalf("OracleHomePatchFingerprint = %q, want oracle-home", got)
	}
}
