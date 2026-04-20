package controllers

import (
	"testing"

	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAsmDiskGroupModeHelpers(t *testing.T) {
	t.Parallel()

	rawDG := racdb.AsmDiskGroupDetails{Name: "DATA", Type: racdb.DbDataDiskDg, Disks: []string{"/dev/raw1"}}
	scDG := racdb.AsmDiskGroupDetails{Name: "RECO", Type: racdb.DbRecoveryDiskDg, Disks: []string{"reco1"}, StorageClass: "fast-sc"}

	if got := effectiveAsmStorageClassForDG(rawDG, ""); got != "" {
		t.Fatalf("expected raw dg effective storage class to be empty, got %q", got)
	}
	if !isRawAsmDiskGroup(rawDG, "") {
		t.Fatalf("expected raw dg to be treated as raw")
	}
	if got := effectiveAsmStorageClassForDG(scDG, ""); got != "fast-sc" {
		t.Fatalf("expected per-dg storage class to win, got %q", got)
	}
	if isRawAsmDiskGroup(scDG, "") {
		t.Fatalf("expected storageClass dg to be treated as non-raw")
	}
	if got := effectiveAsmStorageClassForDG(rawDG, "global-sc"); got != "global-sc" {
		t.Fatalf("expected global storage class fallback, got %q", got)
	}
}

func TestHasAnyRawAsmDiskGroup(t *testing.T) {
	t.Parallel()

	specAllSC := &racdb.RacDatabaseSpec{
		StorageClass: "global-sc",
		AsmStorageDetails: []racdb.AsmDiskGroupDetails{
			{Name: "DATA", Type: racdb.DbDataDiskDg, Disks: []string{"d1"}},
		},
	}
	if hasAnyRawAsmDiskGroup(specAllSC) {
		t.Fatalf("expected no raw disk groups when global storageClass is set")
	}

	specMixed := &racdb.RacDatabaseSpec{
		AsmStorageDetails: []racdb.AsmDiskGroupDetails{
			{Name: "DATA", Type: racdb.DbDataDiskDg, Disks: []string{"/dev/raw1"}},
			{Name: "RECO", Type: racdb.DbRecoveryDiskDg, Disks: []string{"reco1"}, StorageClass: "fast-sc"},
		},
	}
	if !hasAnyRawAsmDiskGroup(specMixed) {
		t.Fatalf("expected mixed spec to report raw disk groups present")
	}
}

func TestHasAnyRawAsmDiskGroup_IgnoresDefaultedAliasGroupsWithoutDisks(t *testing.T) {
	t.Parallel()

	spec := &racdb.RacDatabaseSpec{
		AsmStorageDetails: []racdb.AsmDiskGroupDetails{
			{
				Name:               "DATA",
				Type:               racdb.CrsAsmDiskDg,
				Disks:              []string{"asm-disk1", "asm-disk2"},
				StorageClass:       "oci-bv",
				AsmStorageSizeInGb: 50,
			},
			{
				Name: "+DATA",
				Type: racdb.DbDataDiskDg,
			},
			{
				Name: "+DATA",
				Type: racdb.DbRecoveryDiskDg,
			},
		},
	}

	if hasAnyRawAsmDiskGroup(spec) {
		t.Fatalf("expected empty defaulted alias groups to be ignored for raw disk detection")
	}
}

func TestNormalizeRacStatusConditionsDeduplicatesByType(t *testing.T) {
	t.Parallel()

	conditions := []metav1.Condition{
		{Type: string(racdb.RacCrdReconcileWaitingState), Message: "old"},
		{Type: string(racdb.RacCrdReconcileErrorState), Message: "err"},
		{Type: string(racdb.RacCrdReconcileWaitingState), Message: "new"},
	}

	got := normalizeRacStatusConditions(conditions)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique conditions, got %d: %#v", len(got), got)
	}
	if got[0].Type != string(racdb.RacCrdReconcileWaitingState) || got[0].Message != "new" {
		t.Fatalf("expected last ReconcileWaiting condition to win, got %#v", got[0])
	}
	if got[1].Type != string(racdb.RacCrdReconcileErrorState) {
		t.Fatalf("expected ReconcileError condition to be preserved, got %#v", got[1])
	}
}
