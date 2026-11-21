package v4

import "testing"

func TestOracleRestartValidateAsmStorageRejectsShellMetacharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		group AsmDiskGroupDetails
	}{
		{
			name: "disk group name",
			group: AsmDiskGroupDetails{
				Name:  "DATA;touch /tmp/oraclerestart-poc",
				Disks: []string{"data1"},
			},
		},
		{
			name: "disk name",
			group: AsmDiskGroupDetails{
				Name:  "DATA",
				Disks: []string{"data1;touch /tmp/oraclerestart-poc"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cr := &OracleRestart{Spec: OracleRestartSpec{
				AsmStorageDetails: []AsmDiskGroupDetails{tt.group},
			}}
			if errs := cr.validateAsmStorage(); len(errs) == 0 {
				t.Fatalf("expected shell metacharacters to be rejected")
			}
		})
	}
}
