package v4

import (
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
			name: "user DG defaults deployAs to PRIMARY",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardSpace: "ss1",
				}},
			},
			wantDeployAs: "PRIMARY",
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

func TestNormalizeReplicationType(t *testing.T) {
	cr := &ShardingDatabase{Spec: ShardingDatabaseSpec{ReplicationType: "raftreplicatin"}}
	if got := normalizeReplicationType(&cr.Spec); got != replNative {
		t.Fatalf("expected %q, got %q", replNative, got)
	}
}
