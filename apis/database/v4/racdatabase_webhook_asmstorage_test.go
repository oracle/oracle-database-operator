package v4

import "testing"

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
