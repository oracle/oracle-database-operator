package v4

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateAsmStorageAllowsMixedRawAndStorageClassDGs(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ConfigParams: &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:       "DATA",
					Type:       DbDataDiskDg,
					Disks:      []string{"/dev/raw1"},
					Redundancy: "EXTERNAL",
				},
				{
					Name:               "RECO",
					Type:               DbRecoveryDiskDg,
					Disks:              []string{"reco1"},
					StorageClass:       "fast-sc",
					AsmStorageSizeInGb: 200,
				},
			},
		},
	}

	if errs := cr.validateAsmStorage(); len(errs) != 0 {
		t.Fatalf("expected mixed raw/storageClass disk groups to pass validation, got: %v", errs)
	}
}

func TestValidateAsmStorageRequiresSizeWhenStorageClassConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec RacDatabaseSpec
	}{
		{
			name: "per-dg storageClass requires size",
			spec: RacDatabaseSpec{
				ConfigParams: &RacInitParams{},
				AsmStorageDetails: []AsmDiskGroupDetails{
					{
						Name:         "DATA",
						Type:         DbDataDiskDg,
						Disks:        []string{"data1"},
						StorageClass: "fast-sc",
					},
				},
			},
		},
		{
			name: "global storageClass fallback requires size",
			spec: RacDatabaseSpec{
				ConfigParams: &RacInitParams{},
				StorageClass: "fast-sc",
				AsmStorageDetails: []AsmDiskGroupDetails{
					{
						Name:  "DATA",
						Type:  DbDataDiskDg,
						Disks: []string{"data1"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := &RacDatabase{Spec: tt.spec}
			errs := cr.validateAsmStorage()
			if len(errs) == 0 {
				t.Fatalf("expected validation errors, got none")
			}
		})
	}
}

func TestRacDatabaseDefaultSetsAsmAccessModeFromTopology(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 2},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AsmStorageSizeInGb: 50,
				},
				{
					Name:  "RECO",
					Type:  DbRecoveryDiskDg,
					Disks: []string{"/dev/raw1"},
				},
			},
		},
	}

	if err := (&RacDatabase{}).Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if got := cr.Spec.AsmStorageDetails[0].AccessMode; got != "ReadWriteMany" {
		t.Fatalf("expected storageClass-backed multi-node accessMode=ReadWriteMany, got %q", got)
	}
	if got := cr.Spec.AsmStorageDetails[1].AccessMode; got != "ReadWriteMany" {
		t.Fatalf("expected raw ASM accessMode=ReadWriteMany, got %q", got)
	}
}

func TestRacDatabaseDefaultPreservesExplicitAsmAccessMode(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 3},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "ReadWriteOnce",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if err := (&RacDatabase{}).Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if got := cr.Spec.AsmStorageDetails[0].AccessMode; got != "ReadWriteOnce" {
		t.Fatalf("expected explicit accessMode to be preserved, got %q", got)
	}
}

func TestRacDatabaseDefaultDoesNotBackfillAsmAccessModeOnUpdate(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Now(),
		},
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 1},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if err := (&RacDatabase{}).Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if got := cr.Spec.AsmStorageDetails[0].AccessMode; got != "" {
		t.Fatalf("expected accessMode to remain optional on update defaulting, got %q", got)
	}
}
func TestValidateAsmStorageRejectsInvalidAccessMode(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ConfigParams: &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "Nope",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if errs := cr.validateAsmStorage(); len(errs) == 0 {
		t.Fatalf("expected invalid accessMode to fail validation")
	}
}

func TestValidateAsmStorageRejectsReadWriteOnceForMultiNodeRac(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 2},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "ReadWriteOnce",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if errs := cr.validateAsmStorage(); len(errs) == 0 {
		t.Fatalf("expected ReadWriteOnce to fail validation for multi-node RAC")
	}
}

func TestValidateAsmStorageAllowsReadWriteOnceForSingleNodeRac(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 1},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "ReadWriteOnce",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if errs := cr.validateAsmStorage(); len(errs) != 0 {
		t.Fatalf("expected ReadWriteOnce to be allowed for single-node RAC, got: %v", errs)
	}
}

func TestValidateUpdateAsmStorageRejectsAccessModeChange(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 2},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "ReadWriteMany",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}
	newCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			ClusterDetails: &RacClusterDetailSpec{NodeCount: 1},
			ConfigParams:   &RacInitParams{},
			AsmStorageDetails: []AsmDiskGroupDetails{
				{
					Name:               "DATA",
					Type:               DbDataDiskDg,
					Disks:              []string{"data1"},
					StorageClass:       "fast-sc",
					AccessMode:         "ReadWriteOnce",
					AsmStorageSizeInGb: 50,
				},
			},
		},
	}

	if errs := newCr.validateUpdateAsmStorage(oldCr); len(errs) == 0 {
		t.Fatalf("expected accessMode update to be rejected")
	}
}
