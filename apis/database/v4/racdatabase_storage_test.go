package v4

import (
	"strings"
	"testing"
)

func TestResolveSwStorageMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    *RacDatabaseSpec
		want    RacSwStorageMode
		wantErr string
	}{
		{name: "nil spec defaults to none", spec: nil, want: RacSwStorageNone},
		{name: "no software storage fields defaults to none", spec: &RacDatabaseSpec{}, want: RacSwStorageNone},
		{
			name: "host path mode",
			spec: &RacDatabaseSpec{ClusterDetails: &RacClusterDetailSpec{RacHostSwLocation: "/u01/stage"}},
			want: RacSwStorageHostPath,
		},
		{name: "existing pvc prefix mode", spec: &RacDatabaseSpec{RacSwPrefix: "racsw-"}, want: RacSwStorageExistingPVC},
		{name: "storage class mode", spec: &RacDatabaseSpec{StorageClass: "fast-sc"}, want: RacSwStorageStorageClass},
		{
			name: "host path and storage class are mutually exclusive",
			spec: &RacDatabaseSpec{
				ClusterDetails: &RacClusterDetailSpec{RacHostSwLocation: "/u01/stage"},
				StorageClass:   "fast-sc",
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "prefix and storage class are mutually exclusive",
			spec: &RacDatabaseSpec{
				RacSwPrefix:  "racsw-",
				StorageClass: "fast-sc",
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.spec.ResolveSwStorageMode()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveSwStorageMode=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveSwStageMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *RacInitParams
		want    RacSwStageMode
		wantErr string
	}{
		{name: "nil config defaults to none", cfg: nil, want: RacSwStageNone},
		{name: "no stage settings defaults to none", cfg: &RacInitParams{}, want: RacSwStageNone},
		{
			name: "stage pvc mode",
			cfg: &RacInitParams{
				SwStagePvc:              "stage-pvc",
				SwStagePvcMountLocation: "/u01/swstage",
			},
			want: RacSwStageExistingPVC,
		},
		{
			name: "host stage mode",
			cfg: &RacInitParams{
				HostSwStageLocation: "/u01/stage",
			},
			want: RacSwStageHostPath,
		},
		{
			name: "missing stage pvc is rejected",
			cfg: &RacInitParams{
				SwStagePvcMountLocation: "/u01/swstage",
			},
			wantErr: "swStagePvc must be specified",
		},
		{
			name: "missing stage mount is rejected",
			cfg: &RacInitParams{
				SwStagePvc: "stage-pvc",
			},
			wantErr: "swStagePvcMountLocation must be specified",
		},
		{
			name: "host and pvc staging are mutually exclusive",
			cfg: &RacInitParams{
				SwStagePvc:              "stage-pvc",
				SwStagePvcMountLocation: "/u01/swstage",
				HostSwStageLocation:     "/u01/stage",
			},
			wantErr: "cannot be used when swStagePvc is specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.cfg.ResolveSwStageMode()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveSwStageMode=%q, want %q", got, tt.want)
			}
		})
	}
}
