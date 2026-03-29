package v4

import (
	"context"
	"strings"
	"testing"
)

func hasErrContaining(errs []error, want string) bool {
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Error()), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

func TestValidateShardOperationRules(t *testing.T) {
	tests := []struct {
		name         string
		spec         ShardingDatabaseSpec
		wantErr      string
		wantDeployAs string
	}{
		{
			name: "user mode rejects shardGroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardGroup: "sg1",
					ShardSpace: "ss1",
				}},
			},
			wantErr: "does not allow shardGroup",
		},
		{
			name: "system mode rejects direct shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardGroup: "sg1",
					ShardSpace: "ss1",
				}},
			},
			wantErr: "cannot use shardSpace directly",
		},
		{
			name: "composite mode requires shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardGroup: "sg1",
				}},
			},
			wantErr: "requires shardSpace",
		},
		{
			name: "native replication rejects deployAs",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "NATIVE",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardSpace: "ss1",
					DeployAs:   "PRIMARY",
				}},
			},
			wantErr: "not supported for NATIVE replication",
		},
		{
			name: "system mode forbids deployAs with shardGroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardGroup: "sg1",
					DeployAs:   "PRIMARY",
				}},
			},
			wantErr: "cannot be combined with shardGroup",
		},
		{
			name: "user DG without explicit primary is rejected",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardSpace: "ss1",
				}},
			},
			wantErr: "requires exactly one PRIMARY shard per shardSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardOperationRules()

			if tt.wantErr != "" {
				if errList == nil || len(errList) == 0 {
					t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
				}
				errs := make([]error, 0, len(errList))
				for _, e := range errList {
					errs = append(errs, e)
				}
				if !hasErrContaining(errs, tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, errs)
				}
				return
			}

			if errList != nil && len(errList) > 0 {
				t.Fatalf("expected no errors, got: %v", errList)
			}

			if tt.wantDeployAs != "" {
				if got := cr.Spec.Shard[0].DeployAs; got != tt.wantDeployAs {
					t.Fatalf("expected deployAs default %q, got %q", tt.wantDeployAs, got)
				}
			}
		})
	}
}

func TestValidateShardOperationRulesUserPrimaryConstraints(t *testing.T) {
	tests := []struct {
		name    string
		spec    ShardingDatabaseSpec
		wantErr string
	}{
		{
			name: "user DG rejects multiple primaries in same shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "PRIMARY"},
				},
			},
			wantErr: "at most one PRIMARY shard per shardSpace",
		},
		{
			name: "user DG rejects shardSpace without primary",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "STANDBY"},
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "ACTIVE_STANDBY"},
				},
			},
			wantErr: "requires exactly one PRIMARY shard per shardSpace",
		},
		{
			name: "user DG allows multiple standbys with one primary in shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "STANDBY", ShardRegion: "ashburn"},
					{Name: "shard3", ShardSpace: "ss1", DeployAs: "ACTIVE_STANDBY", ShardRegion: "chicago"},
				},
			},
		},
		{
			name: "user DG allows standby-only when standbyConfig provides external primary",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "asb1", ShardSpace: "ss1", DeployAs: "STANDBY", ShardRegion: "ashburn"},
					{Name: "asb2", ShardSpace: "ss1", DeployAs: "ACTIVE_STANDBY", ShardRegion: "chicago"},
				},
				ShardInfo: []ShardingDetails{{
					ShardPreFixName: "asb",
					ShardSpaceDetails: &ShardSpaceSpec{
						Name: "ss1",
					},
					StandbyConfig: &StandbyConfig{
						SourceType:            "ConnectString",
						StandbyPerPrimary:     2,
						PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL"},
					},
				}},
			},
		},
		{
			name: "user DG rejects local primary when standbyConfig external primary is set",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "asb1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "asb2", ShardSpace: "ss1", DeployAs: "STANDBY"},
				},
				ShardInfo: []ShardingDetails{{
					ShardPreFixName: "asb",
					ShardSpaceDetails: &ShardSpaceSpec{
						Name: "ss1",
					},
					StandbyConfig: &StandbyConfig{
						SourceType:            "ConnectString",
						StandbyPerPrimary:     2,
						PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL"},
					},
				}},
			},
			wantErr: "uses standbyConfig primary source; do not set local deployAs=PRIMARY",
		},
		{
			name: "user DG rejects standby shard without shardRegion",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "STANDBY"},
				},
			},
			wantErr: "requires shardRegion",
		},
		{
			name: "user DG rejects duplicate standby shardRegion in same shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "STANDBY", ShardRegion: "ashburn"},
					{Name: "shard3", ShardSpace: "ss1", DeployAs: "ACTIVE_STANDBY", ShardRegion: "ashburn"},
				},
			},
			wantErr: "standby regions must be unique per shardSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardOperationRules()

			if tt.wantErr != "" {
				if errList == nil || len(errList) == 0 {
					t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
				}
				errs := make([]error, 0, len(errList))
				for _, e := range errList {
					errs = append(errs, e)
				}
				if !hasErrContaining(errs, tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, errs)
				}
				return
			}

			if errList != nil && len(errList) > 0 {
				t.Fatalf("expected no validation errors, got: %v", errList)
			}
		})
	}
}

func TestValidateShardInfoSystemPrimaryStandbyConstraints(t *testing.T) {
	tests := []struct {
		name    string
		spec    ShardingDatabaseSpec
		wantErr string
	}{
		{
			name: "system DG allows one primary group and one standby group within primary cardinality",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        3,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "sba",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
					},
				},
			},
		},
		{
			name: "system DG rejects standbyConfig with standbyPerPrimary greater than one",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "sba",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:            "ConnectString",
							StandbyPerPrimary:     2,
							PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL"},
						},
					},
				},
			},
			wantErr: "at most one standby per primary",
		},
		{
			name: "system DG rejects multiple standby shardgroups for one primary shardgroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        3,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "sba",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
					},
					{
						ShardPreFixName: "sbc",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-chicago",
							Region:   "chicago",
							DeployAs: "ACTIVE_STANDBY",
						},
					},
				},
			},
			wantErr: "only one standby shardGroup can be mapped to a primary shardGroup",
		},
		{
			name: "system DG rejects multiple primary shardgroups",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm1",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary-a",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "prm2",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary-b",
							Region:   "ashburn",
							DeployAs: "PRIMARY",
						},
					},
				},
			},
			wantErr: "exactly one shardGroup must be PRIMARY",
		},
		{
			name: "system DG rejects standby cardinality greater than primary cardinality",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "sba",
						ShardNum:        3,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
					},
				},
			},
			wantErr: "has 3 standby databases but primary shardGroup SG-PRIMARY has only 2 primary databases",
		},
		{
			name: "system DG rejects standby databases in primary shardgroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "std",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "STANDBY",
						},
					},
				},
			},
			wantErr: "PRIMARY shardGroup must not contain standby databases",
		},
		{
			name: "system DG rejects duplicate region across shardgroups",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "std",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-standby",
							Region:   "phoenix",
							DeployAs: "STANDBY",
						},
					},
				},
			},
			wantErr: "region PHOENIX is already used by shardGroup SG-PRIMARY",
		},
		{
			name: "system DG rejects duplicate primary connect strings in standbyConfig",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "std",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:            "ConnectString",
							StandbyPerPrimary:     1,
							PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL", " //phx-primary:1521/PHX_DGMGRL "},
						},
					},
				},
			},
			wantErr: "Duplicate value",
		},
		{
			name: "system DG rejects duplicate primary database refs in standbyConfig",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "prm",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-primary",
							Region:   "phoenix",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "std",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-ashburn",
							Region:   "ashburn",
							DeployAs: "STANDBY",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:        "PrimaryDatabaseRef",
							StandbyPerPrimary: 1,
							PrimaryDatabaseRefs: []PrimaryDatabaseCRRef{
								{Name: "primary-db", Namespace: "shns"},
								{Name: "primary-db", Namespace: "shns"},
							},
						},
					},
				},
			},
			wantErr: "Duplicate value",
		},
		{
			name: "user DG allows single primary source in shardInfo standbyConfig per shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "us1",
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name: "ss1",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:            "ConnectString",
							StandbyPerPrimary:     1,
							PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL"},
						},
					},
				},
			},
		},
		{
			name: "user DG allows exactly one primary replica in shardInfo shardspace deployAs",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "up1",
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "PRIMARY",
						},
					},
					{
						ShardPreFixName: "us1",
						ShardNum:        2,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "STANDBY",
						},
					},
				},
			},
		},
		{
			name: "user DG rejects shardInfo with multiple primary replicas in shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "up1",
						ShardNum:        2,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "PRIMARY",
						},
					},
				},
			},
			wantErr: "requires exactly one PRIMARY shard per shardSpace",
		},
		{
			name: "user DG rejects shardInfo primary deployAs when standbyConfig external primary exists",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "up1",
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "PRIMARY",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:            "ConnectString",
							StandbyPerPrimary:     1,
							PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL"},
						},
					},
				},
			},
			wantErr: "uses standbyConfig primary source; do not set shardSpaceDetails.deployAs=PRIMARY",
		},
		{
			name: "user DG rejects multiple primary sources in shardInfo standbyConfig per shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "us1",
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name: "ss1",
						},
						StandbyConfig: &StandbyConfig{
							SourceType:            "ConnectString",
							StandbyPerPrimary:     1,
							PrimaryConnectStrings: []string{"//phx-primary:1521/PHX_DGMGRL", "//ash-primary:1521/ASH_DGMGRL"},
						},
					},
				},
			},
			wantErr: "at most one primary source per shardSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardInfo()

			if tt.wantErr != "" {
				if errList == nil || len(errList) == 0 {
					t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
				}
				errs := make([]error, 0, len(errList))
				for _, e := range errList {
					errs = append(errs, e)
				}
				if !hasErrContaining(errs, tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, errs)
				}
				return
			}

			if errList != nil && len(errList) > 0 {
				t.Fatalf("expected no validation errors, got: %v", errList)
			}
		})
	}
}

func TestBuildDesiredShardSpecMapsUserShardSpaceDeployAs(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ShardingType:    "USER",
			ReplicationType: "DG",
			ShardInfo: []ShardingDetails{
				{
					ShardPreFixName: "u",
					ShardNum:        1,
					ShardSpaceDetails: &ShardSpaceSpec{
						Name:     "ss1",
						DeployAs: "PRIMARY",
					},
				},
			},
		},
	}

	desired := cr.buildDesiredShardSpec()
	if len(desired) != 1 {
		t.Fatalf("expected 1 generated shard, got %d", len(desired))
	}
	if got := desired[0].ShardSpace; got != "ss1" {
		t.Fatalf("expected shardSpace ss1, got %q", got)
	}
	if got := desired[0].DeployAs; got != "PRIMARY" {
		t.Fatalf("expected deployAs PRIMARY from shardSpaceDetails, got %q", got)
	}
}

func TestValidateShardInfoCompositeMappingConstraints(t *testing.T) {
	tests := []struct {
		name    string
		spec    ShardingDatabaseSpec
		wantErr string
	}{
		{
			name: "composite allows one primary group per shardspace and multiple standby groups",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "a1p",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a1",
							Region:   "dc1",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
					{
						ShardPreFixName: "a2s",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a2",
							Region:   "dc1",
							DeployAs: "ACTIVE_STANDBY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
					{
						ShardPreFixName: "a3s",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a3",
							Region:   "dc3",
							DeployAs: "ACTIVE_STANDBY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
				},
			},
		},
		{
			name: "composite rejects shardspace with two primary groups",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "a1p",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a1",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
					{
						ShardPreFixName: "a2p",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a2",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
				},
			},
			wantErr: "exactly one PRIMARY shardGroup",
		},
		{
			name: "composite rejects standby cardinality greater than primary cardinality in shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "a1p",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a1",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
					{
						ShardPreFixName: "a2s",
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "shardgroup_a2",
							DeployAs: "ACTIVE_STANDBY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "shardspace_a"},
					},
				},
			},
			wantErr: "has 2 standby databases but primary shardGroup has only 1 primary databases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardInfo()

			if tt.wantErr != "" {
				if errList == nil || len(errList) == 0 {
					t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
				}
				errs := make([]error, 0, len(errList))
				for _, e := range errList {
					errs = append(errs, e)
				}
				if !hasErrContaining(errs, tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, errs)
				}
				return
			}

			if errList != nil && len(errList) > 0 {
				t.Fatalf("expected no validation errors, got: %v", errList)
			}
		})
	}
}

func TestNormalizeReplicationType(t *testing.T) {
	cr := &ShardingDatabase{Spec: ShardingDatabaseSpec{ReplicationType: "raftreplicatin"}}
	if got := normalizeReplicationType(&cr.Spec); got != replNative {
		t.Fatalf("expected %q, got %q", replNative, got)
	}
}

func TestValidateUpdateRejectsReplicationTypeChange(t *testing.T) {
	oldCR := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ReplicationType: "DG",
			ShardingType:    "SYSTEM",
			ShardInfo: []ShardingDetails{{
				ShardGroupDetails: &ShardGroupSpec{Name: "sg1"},
			}},
		},
	}
	newCR := oldCR.DeepCopy()
	newCR.Spec.ReplicationType = "NATIVE"

	_, err := (&ShardingDatabase{}).ValidateUpdate(context.Background(), oldCR, newCR)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "replicationtype is immutable") {
		t.Fatalf("expected replicationType immutability error, got: %v", err)
	}
}

func TestValidateUpdateRejectsShardingTypeChange(t *testing.T) {
	oldCR := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ReplicationType: "DG",
			ShardingType:    "USER",
			ShardInfo: []ShardingDetails{{
				ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
			}},
		},
	}
	newCR := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ReplicationType: "DG",
			ShardingType:    "COMPOSITE",
			ShardInfo: []ShardingDetails{{
				ShardGroupDetails: &ShardGroupSpec{Name: "sg1"},
				ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
			}},
		},
	}

	_, err := (&ShardingDatabase{}).ValidateUpdate(context.Background(), oldCR, newCR)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "shardingtype is immutable") {
		t.Fatalf("expected shardingType immutability error, got: %v", err)
	}
}

func TestValidateCatalogAdvancedParamsUserNativeRepFactorRejected(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ReplicationType: "NATIVE",
			ShardingType:    "USER",
			Catalog: []CatalogSpec{{
				Name:      "cat1",
				Repl:      "NATIVE",
				Sharding:  "USER",
				RepFactor: 2,
			}},
		},
	}

	errList := cr.validateCatalogAdvancedParams()
	if errList == nil || len(errList) == 0 {
		t.Fatalf("expected validation error for USER+NATIVE repFactor, got none")
	}

	errs := make([]error, 0, len(errList))
	for _, e := range errList {
		errs = append(errs, e)
	}
	if !hasErrContaining(errs, "repFactor is not applicable for USER sharding catalog") {
		t.Fatalf("expected USER repFactor error, got: %v", errs)
	}
}
