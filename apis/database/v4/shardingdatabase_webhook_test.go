package v4

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func hasErrContaining(errs []error, want string) bool {
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Error()), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

func TestShardingWebhookDefaultSetsDataguardModePreview(t *testing.T) {
	cr := &ShardingDatabase{}

	if err := cr.Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if cr.Spec.Dataguard == nil {
		t.Fatalf("expected dataguard spec to be defaulted")
	}
	if cr.Spec.Dataguard.Mode != DataguardProducerModePreview {
		t.Fatalf("expected dataguard mode %q, got %q", DataguardProducerModePreview, cr.Spec.Dataguard.Mode)
	}
}

func TestShardingWebhookRejectsManagedDataguardMode(t *testing.T) {
	errs := validateDataguardProducerSpec(field.NewPath("spec").Child("dataguard"), &DataguardProducerSpec{
		Mode: DataguardProducerModeManaged,
	})
	if len(errs) == 0 {
		t.Fatalf("expected validation error for managed dataguard mode")
	}
}

func TestShardingDeprecatedFieldWarnings(t *testing.T) {
	spec := &ShardingDatabaseSpec{
		IsDataGuard:               true,
		GsmShardSpace:             []ShardSpaceSpec{{Name: "legacy-ss"}},
		GsmShardGroup:             []ShardGroupSpec{{Name: "legacy-sg"}},
		ShardRegion:               []string{"phx"},
		ShardBuddyRegion:          "iad",
		IsTdeWallet:               "enable",
		TdeWalletPvc:              "tde-pvc",
		TdeWalletPvcMountLocation: "/mnt/tde",
		Shard: []ShardSpec{{
			Name:          "sh1",
			PvcName:       "legacy-pvc",
			PvAnnotations: map[string]string{"a": "b"},
		}},
		Catalog: []CatalogSpec{{
			Name:          "cat1",
			PvcName:       "legacy-cat-pvc",
			PvMatchLabels: map[string]string{"c": "d"},
		}},
		Gsm: []GsmSpec{{
			Name:    "gsm1",
			PvcName: "legacy-gsm-pvc",
		}},
		ShardGroup: []ShardGroupSpec{{
			Name:             "sg1",
			ShardGroupName:   "legacy-sg",
			LegacyShardSpace: "SS1",
		}},
		ShardSpace: []ShardSpaceSpec{{
			Name:           "ss1",
			ShardSpaceName: "legacy-ss",
			Chnuks:         8,
			ProtectionMode: "MAXPERFORMANCE",
		}},
		Service: []GsmServiceSpec{{
			Name:        "svc1",
			PrferredAll: "pool1",
		}},
		GsmService: []GsmServiceSpec{{
			Name:        "svc2",
			PrferredAll: "pool2",
		}},
		ShardInfo: []ShardingDetails{{
			ShardPreFixName: "sh",
			Replicas:        2,
			ShardGroupDetails: &ShardGroupSpec{
				Name:             "sgi",
				ShardGroupName:   "legacy-sgi",
				LegacyShardSpace: "SS2",
			},
			ShardSpaceDetails: &ShardSpaceSpec{
				Name:           "ssi",
				ShardSpaceName: "legacy-ssi",
				Chnuks:         4,
				ProtectionMode: "MAXAVAILABILITY",
			},
		}},
	}

	warnings := deprecatedShardingFieldWarnings(spec)
	expectedWarnings := []string{
		"spec.isDataGuard is deprecated and retained only for backward compatibility",
		"spec.gsmShardSpace is deprecated; use spec.shardSpace",
		"spec.gsmShardGroup is deprecated; use spec.shardGroup",
		"spec.shardRegion is deprecated; use spec.region",
		"spec.shardBuddyRegion is deprecated; use spec.region[].buddy",
		"spec.isTdeWallet is deprecated; use spec.tdeWallet.isEnabled",
		"spec.tdeWalletPvc is deprecated; use spec.tdeWallet.pvcName",
		"spec.tdeWalletPvcMountLocation is deprecated; use spec.tdeWallet.mountPath",
		"spec.shard[0] uses deprecated pvcName/pvAnnotations/pvMatchLabels fields; these fields are ignored by the operator. Use additionalPVCs instead.",
		"spec.catalog[0] uses deprecated pvcName/pvAnnotations/pvMatchLabels fields; these fields are ignored by the operator. Use additionalPVCs instead.",
		"spec.gsm[0] uses deprecated pvcName/pvAnnotations/pvMatchLabels fields; these fields are ignored by the operator. Use additionalPVCs instead.",
		"spec.shardGroup[0].shardGroupName is deprecated; use spec.shardGroup[0].name",
		"spec.shardGroup[0].ShardSpace is deprecated; use spec.shardGroup[0].shardSpace",
		"spec.shardSpace[0].shardSpaceName is deprecated; use spec.shardSpace[0].name",
		"spec.shardSpace[0].Chnuks is deprecated; use spec.shardSpace[0].chunks",
		"spec.shardSpace[0].protectionMode is deprecated; use spec.shardSpace[0].protectMode",
		"spec.service[0].prferredAll is deprecated; use spec.service[0].preferredAll",
		"spec.gsmService[0].prferredAll is deprecated; use spec.gsmService[0].preferredAll",
		"spec.shardInfo[0].replicas is deprecated; use spec.shardInfo[0].shardNum",
		"spec.shardInfo[0].shardGroupDetails.shardGroupName is deprecated; use spec.shardInfo[0].shardGroupDetails.name",
		"spec.shardInfo[0].shardGroupDetails.ShardSpace is deprecated; use spec.shardInfo[0].shardGroupDetails.shardSpace",
		"spec.shardInfo[0].shardSpaceDetails.shardSpaceName is deprecated; use spec.shardInfo[0].shardSpaceDetails.name",
		"spec.shardInfo[0].shardSpaceDetails.Chnuks is deprecated; use spec.shardInfo[0].shardSpaceDetails.chunks",
		"spec.shardInfo[0].shardSpaceDetails.protectionMode is deprecated; use spec.shardInfo[0].shardSpaceDetails.protectMode",
	}

	if len(warnings) != len(expectedWarnings) {
		t.Fatalf("expected %d warnings, got %#v", len(expectedWarnings), warnings)
	}
	for _, expected := range expectedWarnings {
		found := false
		for _, warning := range warnings {
			if warning == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected warning %q, got %#v", expected, warnings)
		}
	}
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
			wantErr: "not supported with RAFT/NATIVE replication",
		},
		{
			name: "system mode allows deployAs with shardGroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardGroup: "sg1",
					DeployAs:   "PRIMARY",
				}},
			},
		},
		{
			name: "user mode normalizes lowercase deployAs in shard",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{{
					Name:       "shard1",
					ShardSpace: "ss1",
					DeployAs:   "primary",
				}},
			},
			wantDeployAs: "PRIMARY",
		},
		{
			name: "composite mode rejects reusing shardGroup name across shardSpaces in shard",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "s1", ShardGroup: "sg1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "s2", ShardGroup: "sg1", ShardSpace: "ss2", DeployAs: "PRIMARY"},
				},
			},
			wantErr: "must be unique across shardSpaces",
		},
		{
			name: "composite mode rejects reusing shardGroup name across shardSpaces in shardInfo",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "a",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg1",
							Region:   "phx",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
					{
						ShardPreFixName: "b",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg1",
							Region:   "iad",
							DeployAs: "PRIMARY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss2"},
					},
				},
				Shard: []ShardSpec{
					{Name: "a1", ShardGroup: "sg1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
					{Name: "b1", ShardGroup: "sg1", ShardSpace: "ss2", DeployAs: "PRIMARY"},
				},
			},
			wantErr: "must be unique across shardSpaces",
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
		{
			name: "composite native shard operations allow one readwrite shardgroup with multiple members",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardGroup: []ShardGroupSpec{
					{Name: "sg-rw", ShardSpace: "ss1", RuMode: "READWRITE"},
				},
				Shard: []ShardSpec{
					{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
					{Name: "rw2", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
					{Name: "rw3", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
				},
			},
		},
		{
			name: "composite native shard operations reject readonly shardgroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardGroup: []ShardGroupSpec{
					{Name: "sg-rw", ShardSpace: "ss1", RuMode: "READWRITE"},
					{Name: "sg-ro", ShardSpace: "ss1", RuMode: "READONLY"},
				},
				Shard: []ShardSpec{
					{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
					{Name: "ro1", ShardGroup: "sg-ro", ShardSpace: "ss1", ShardRegion: "iad"},
				},
			},
			wantErr: "currently supports only READWRITE shardGroups",
		},
		{
			name: "composite native shard operations reject multiple shardgroups in one shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardGroup: []ShardGroupSpec{
					{Name: "sg-rw-1", ShardSpace: "ss1", RuMode: "READWRITE"},
					{Name: "sg-rw-2", ShardSpace: "ss1", RuMode: "READWRITE"},
				},
				Shard: []ShardSpec{
					{Name: "rw1", ShardGroup: "sg-rw-1", ShardSpace: "ss1", ShardRegion: "phx"},
					{Name: "rw2", ShardGroup: "sg-rw-2", ShardSpace: "ss1", ShardRegion: "iad"},
				},
			},
			wantErr: "currently supports at most one shardGroup per shardSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardOperationRules()

			if tt.wantErr != "" {
				if len(errList) == 0 {
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

			if len(errList) > 0 {
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
			name: "user DG allows standby before primary in shardSpace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				Shard: []ShardSpec{
					{Name: "shard2", ShardSpace: "ss1", DeployAs: "STANDBY", ShardRegion: "ashburn"},
					{Name: "shard1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
				},
			},
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
			name: "user sharding rejects RAFT/NATIVE replication",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "NATIVE",
				Shard: []ShardSpec{
					{Name: "shard1", ShardSpace: "ss1"},
				},
			},
			wantErr: "not supported with RAFT/NATIVE replication",
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
						StandbyPerPrimary: 2,
						PrimarySources: []StandbyPrimarySource{{
							ConnectString: "//phx-primary:1521/PHX_DGMGRL",
						}},
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
						StandbyPerPrimary: 2,
						PrimarySources: []StandbyPrimarySource{{
							ConnectString: "//phx-primary:1521/PHX_DGMGRL",
						}},
					},
				}},
			},
			wantErr: "uses standbyConfig.primarySources; do not set local deployAs=PRIMARY",
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
				if len(errList) == 0 {
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

			if len(errList) > 0 {
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
			name: "system DG allows primary shardgroup without standby shardgroup",
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
				},
			},
		},
		{
			name: "system DG allows one primary group and one standby group with 1:1 total mapping",
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
						ShardNum:        3,
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
							StandbyPerPrimary: 2,
							PrimarySources: []StandbyPrimarySource{{
								ConnectString: "//phx-primary:1521/PHX_DGMGRL",
							}},
						},
					},
				},
			},
			wantErr: "does not support standbyPerPrimary",
		},
		{
			name: "system DG rejects standby totals that do not match primary total across standby shardgroups",
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
			wantErr: "total standby databases across standby shardGroups must match primary shardGroup",
		},
		{
			name: "system DG allows multiple standby shardgroups when total standby equals primary total",
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
						ShardNum:        2,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-chicago",
							Region:   "chicago",
							DeployAs: "ACTIVE_STANDBY",
						},
					},
				},
			},
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
			name: "system DG rejects duplicate primary sources in standbyConfig",
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
							PrimarySources: []StandbyPrimarySource{
								{ConnectString: "//phx-primary:1521/PHX_DGMGRL"},
								{ConnectString: " //phx-primary:1521/PHX_DGMGRL "},
							},
						},
					},
				},
			},
			wantErr: "Duplicate value",
		},
		{
			name: "system DG rejects mixed source modes in one primarySources item",
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
							PrimarySources: []StandbyPrimarySource{{
								DatabaseRef:   &PrimaryDatabaseCRRef{Name: "primary-db", Namespace: "shns"},
								ConnectString: "//phx-primary:1521/PHX_DGMGRL",
							}},
						},
					},
				},
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "system NATIVE allows shardGroup ru_mode when deployAs is unset",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "sn1",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-native",
							Region: "phx",
							RuMode: "READWRITE",
						},
					},
				},
			},
		},
		{
			name: "system DG rejects shardGroup ru_mode",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "sd1",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-dg",
							Region: "phx",
							RuMode: "READWRITE",
						},
					},
				},
			},
			wantErr: "ru_mode is only supported for NATIVE replication",
		},
		{
			name: "system NATIVE rejects shardGroup deployAs with ru_mode",
			spec: ShardingDatabaseSpec{
				ShardingType:    "SYSTEM",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "sx1",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:     "sg-x",
							Region:   "phx",
							DeployAs: "PRIMARY",
							RuMode:   "READWRITE",
						},
					},
				},
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "user DG allows single primary source in standbyConfig.primarySources per shardspace",
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
							StandbyPerPrimary: 1,
							PrimarySources: []StandbyPrimarySource{{
								ConnectString: "//phx-primary:1521/PHX_DGMGRL",
							}},
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
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "STANDBY",
						},
					},
				},
			},
		},
		{
			name: "user DG allows shardInfo standby when shardNum is greater than one",
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
			name: "user DG rejects shardInfo standby without primary in shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "USER",
				ReplicationType: "DG",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "us1",
						ShardNum:        1,
						ShardSpaceDetails: &ShardSpaceSpec{
							Name:     "ss1",
							DeployAs: "STANDBY",
						},
					},
				},
			},
			wantErr: "requires at least one PRIMARY shard",
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
							StandbyPerPrimary: 1,
							PrimarySources: []StandbyPrimarySource{{
								ConnectString: "//phx-primary:1521/PHX_DGMGRL",
							}},
						},
					},
				},
			},
			wantErr: "uses standbyConfig.primarySources; do not set shardSpaceDetails.deployAs=PRIMARY",
		},
		{
			name: "user DG allows multiple primary sources in shardInfo standbyConfig per shardspace",
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
							StandbyPerPrimary: 1,
							PrimarySources: []StandbyPrimarySource{
								{ConnectString: "//phx-primary:1521/PHX_DGMGRL"},
								{ConnectString: "//ash-primary:1521/ASH_DGMGRL"},
							},
						},
					},
				},
			},
		},
		{
			name: "user DG rejects empty primarySources item",
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
							PrimarySources: []StandbyPrimarySource{{}},
						},
					},
				},
			},
			wantErr: "set exactly one of databaseRef, connectString, or details",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardInfo()

			if tt.wantErr != "" {
				if len(errList) == 0 {
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

			if len(errList) > 0 {
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

func TestValidateShardInfoNormalizesDeployAsCaseForShardSpace(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ShardingType:    "USER",
			ReplicationType: "DG",
			ShardInfo: []ShardingDetails{
				{
					ShardPreFixName: "u1",
					ShardNum:        1,
					ShardSpaceDetails: &ShardSpaceSpec{
						Name:     "ss1",
						DeployAs: "primary",
					},
				},
			},
		},
	}

	if errList := cr.validateShardInfo(); len(errList) > 0 {
		t.Fatalf("expected no validation errors, got: %v", errList)
	}
	if got := cr.Spec.ShardInfo[0].ShardSpaceDetails.DeployAs; got != "PRIMARY" {
		t.Fatalf("expected shardSpaceDetails.deployAs to be normalized to PRIMARY, got %q", got)
	}
}

func TestValidateShardInfoNormalizesDeployAsCaseForShardGroup(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			ShardingType:    "SYSTEM",
			ReplicationType: "DG",
			ShardInfo: []ShardingDetails{
				{
					ShardPreFixName: "s1",
					ShardNum:        1,
					ShardGroupDetails: &ShardGroupSpec{
						Name:     "sg1",
						Region:   "phx",
						DeployAs: "primary",
					},
				},
				{
					ShardPreFixName: "s2",
					ShardNum:        1,
					ShardGroupDetails: &ShardGroupSpec{
						Name:     "sg2",
						Region:   "iad",
						DeployAs: "standby",
					},
				},
			},
		},
	}

	if errList := cr.validateShardInfo(); len(errList) > 0 {
		t.Fatalf("expected no validation errors, got: %v", errList)
	}
	if got := cr.Spec.ShardInfo[0].ShardGroupDetails.DeployAs; got != "PRIMARY" {
		t.Fatalf("expected shardGroupDetails.deployAs to be normalized to PRIMARY, got %q", got)
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
				if len(errList) == 0 {
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

			if len(errList) > 0 {
				t.Fatalf("expected no validation errors, got: %v", errList)
			}
		})
	}
}

func TestValidateShardInfoCompositeNativeRules(t *testing.T) {
	tests := []struct {
		name    string
		spec    ShardingDatabaseSpec
		wantErr string
	}{
		{
			name: "composite native allows one readwrite shardgroup with multiple members",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "rw",
						ShardNum:        3,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-rw",
							Region: "phx",
							RuMode: "READWRITE",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
				},
			},
		},
		{
			name: "composite native rejects readonly shardgroup",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "rw",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-rw",
							Region: "phx",
							RuMode: "READWRITE",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
					{
						ShardPreFixName: "ro",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-ro",
							Region: "iad",
							RuMode: "READONLY",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
				},
			},
			wantErr: "currently supports only READWRITE shardGroups",
		},
		{
			name: "composite native rejects missing ru_mode",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "rw",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-rw",
							Region: "phx",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
				},
			},
			wantErr: "requires shardGroupDetails.ru_mode",
		},
		{
			name: "composite native rejects multiple shardgroups in one shardspace",
			spec: ShardingDatabaseSpec{
				ShardingType:    "COMPOSITE",
				ReplicationType: "NATIVE",
				ShardInfo: []ShardingDetails{
					{
						ShardPreFixName: "rw1",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-rw-1",
							Region: "phx",
							RuMode: "READWRITE",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
					{
						ShardPreFixName: "rw2",
						ShardNum:        1,
						ShardGroupDetails: &ShardGroupSpec{
							Name:   "sg-rw-2",
							Region: "iad",
							RuMode: "READWRITE",
						},
						ShardSpaceDetails: &ShardSpaceSpec{Name: "ss1"},
					},
				},
			},
			wantErr: "currently supports at most one shardGroup per shardSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ShardingDatabase{Spec: tt.spec}
			errList := cr.validateShardInfo()

			if tt.wantErr != "" {
				if len(errList) == 0 {
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

			if len(errList) > 0 {
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

func validShardingWebhookDGObject() *ShardingDatabase {
	return &ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "gdd1", Namespace: "ns1"},
		Spec: ShardingDatabaseSpec{
			ReplicationType: "DG",
			ShardingType:    "USER",
			EnableTCPS:      false,
			ShardInfo: []ShardingDetails{{
				ShardPreFixName: "sh",
				ShardNum:        1,
				ShardSpaceDetails: &ShardSpaceSpec{
					Name:     "ss1",
					DeployAs: "STANDBY",
				},
				StandbyConfig: &StandbyConfig{
					PrimarySources: []StandbyPrimarySource{{
						ConnectString: "primary-a:1521/PRIM",
					}},
				},
			}},
		},
	}
}

func TestValidateUpdateRejectsShardingDataguardTopologyChangeWhenLocked(t *testing.T) {
	oldCR := validShardingWebhookDGObject()
	oldCR.Status.Dataguard = &ShardingDataguardStatus{TopologyLocked: true}

	newCR := oldCR.DeepCopy()
	newCR.Spec.ShardInfo[0].StandbyConfig.PrimarySources[0].ConnectString = "primary-b:1521/PRIM"

	_, err := (&ShardingDatabase{}).ValidateUpdate(context.Background(), oldCR, newCR)
	if err == nil {
		t.Fatalf("expected topology update to be rejected when dataguard topology is locked")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "topology-defining fields cannot be changed") {
		t.Fatalf("expected dataguard topology lock rejection, got: %v", err)
	}
}

func TestValidateUpdateAllowsNonTopologyShardingChangeWhenDataguardLocked(t *testing.T) {
	oldCR := validShardingWebhookDGObject()
	oldCR.Status.Dataguard = &ShardingDataguardStatus{TopologyLocked: true}

	newCR := oldCR.DeepCopy()
	newCR.Spec.IsDebug = true

	_, err := (&ShardingDatabase{}).ValidateUpdate(context.Background(), oldCR, newCR)
	if err != nil {
		t.Fatalf("expected non-topology update to pass when dataguard topology is locked, got: %v", err)
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
	if len(errList) == 0 {
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

func TestDefaultAppliesGsmResourcesTemplate(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			GsmResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			Gsm: []GsmSpec{
				{Name: "gsm1"},
				{
					Name: "gsm2",
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	if err := cr.Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if cr.Spec.Gsm[0].Resources == nil {
		t.Fatalf("expected gsm[0].resources to be defaulted from spec.gsmResources")
	}
	if got := cr.Spec.Gsm[0].Resources.Limits.Cpu().String(); got != "2" {
		t.Fatalf("expected gsm[0] cpu limit 2, got %s", got)
	}
	if got := cr.Spec.Gsm[0].Resources.Limits.Memory().String(); got != "4Gi" {
		t.Fatalf("expected gsm[0] memory limit 4Gi, got %s", got)
	}
	if cr.Spec.Gsm[1].Resources == nil {
		t.Fatalf("expected gsm[1].resources to remain set")
	}
	if got := cr.Spec.Gsm[1].Resources.Limits.Cpu().String(); got != "1" {
		t.Fatalf("expected gsm[1] cpu limit to remain 1, got %s", got)
	}
}

func TestDefaultAppliesInlineGsmResourcesTemplate(t *testing.T) {
	pullAlways := corev1.PullAlways
	pullIfNotPresent := corev1.PullIfNotPresent
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			Gsm: []GsmSpec{
				{
					StorageSizeInGb:  50,
					ImagePulllPolicy: &pullAlways,
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
				{Name: "gsm1"},
				{
					Name:             "gsm2",
					StorageSizeInGb:  60,
					ImagePulllPolicy: &pullIfNotPresent,
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	if err := cr.Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if len(cr.Spec.Gsm) != 2 {
		t.Fatalf("expected inline default entry to be removed from spec.gsm, got len=%d", len(cr.Spec.Gsm))
	}
	if cr.Spec.Gsm[0].Name != "gsm1" || cr.Spec.Gsm[0].Resources == nil {
		t.Fatalf("expected gsm1 to receive inline default resources")
	}
	if cr.Spec.Gsm[0].StorageSizeInGb != 50 {
		t.Fatalf("expected gsm1 storageSizeInGb 50, got %d", cr.Spec.Gsm[0].StorageSizeInGb)
	}
	if cr.Spec.Gsm[0].ImagePulllPolicy == nil || *cr.Spec.Gsm[0].ImagePulllPolicy != corev1.PullAlways {
		t.Fatalf("expected gsm1 imagePullPolicy Always, got %v", cr.Spec.Gsm[0].ImagePulllPolicy)
	}
	if got := cr.Spec.Gsm[0].Resources.Limits.Cpu().String(); got != "2" {
		t.Fatalf("expected gsm1 cpu limit 2, got %s", got)
	}
	if got := cr.Spec.Gsm[0].Resources.Limits.Memory().String(); got != "4Gi" {
		t.Fatalf("expected gsm1 memory limit 4Gi, got %s", got)
	}
	if cr.Spec.Gsm[1].Name != "gsm2" || cr.Spec.Gsm[1].Resources == nil {
		t.Fatalf("expected gsm2 resources to remain set")
	}
	if cr.Spec.Gsm[1].StorageSizeInGb != 60 {
		t.Fatalf("expected gsm2 storageSizeInGb to remain 60, got %d", cr.Spec.Gsm[1].StorageSizeInGb)
	}
	if cr.Spec.Gsm[1].ImagePulllPolicy == nil || *cr.Spec.Gsm[1].ImagePulllPolicy != corev1.PullIfNotPresent {
		t.Fatalf("expected gsm2 imagePullPolicy to remain IfNotPresent, got %v", cr.Spec.Gsm[1].ImagePulllPolicy)
	}
	if got := cr.Spec.Gsm[1].Resources.Limits.Cpu().String(); got != "1" {
		t.Fatalf("expected gsm2 cpu limit to remain 1, got %s", got)
	}
}

func TestDefaultMaterializesGsmInfoWrapper(t *testing.T) {
	pullAlways := corev1.PullAlways
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			GsmResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
			GsmInfo: &GsmInfo{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2"),
					},
				},
				StorageSizeInGb:  55,
				ImagePulllPolicy: &pullAlways,
				EnvVars: []EnvironmentVariable{
					{Name: "A", Value: "1"},
					{Name: "B", Value: "2"},
				},
				ServiceAnnotations: map[string]string{
					"tier": "common",
				},
				Gsm: []GsmSpec{
					{
						Name: "gsm1",
						EnvVars: []EnvironmentVariable{
							{Name: "B", Value: "22"},
							{Name: "C", Value: "3"},
						},
						ServiceAnnotations: map[string]string{
							"tier": "item",
						},
					},
				},
			},
		},
	}

	if err := cr.Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}

	if len(cr.Spec.Gsm) != 1 {
		t.Fatalf("expected one gsm entry from gsmInfo.gsm, got %d", len(cr.Spec.Gsm))
	}
	if cr.Spec.Gsm[0].Resources == nil || cr.Spec.Gsm[0].Resources.Limits.Cpu().String() != "2" {
		t.Fatalf("expected gsm resources to come from gsmInfo.resources")
	}
	if cr.Spec.Gsm[0].StorageSizeInGb != 55 {
		t.Fatalf("expected gsm storageSizeInGb 55, got %d", cr.Spec.Gsm[0].StorageSizeInGb)
	}
	if cr.Spec.Gsm[0].ImagePulllPolicy == nil || *cr.Spec.Gsm[0].ImagePulllPolicy != corev1.PullAlways {
		t.Fatalf("expected gsm imagePullPolicy Always, got %v", cr.Spec.Gsm[0].ImagePulllPolicy)
	}
	if got := envVarValue(cr.Spec.Gsm[0].EnvVars, "A"); got != "1" {
		t.Fatalf("expected env A=1, got %q", got)
	}
	if got := envVarValue(cr.Spec.Gsm[0].EnvVars, "B"); got != "22" {
		t.Fatalf("expected env B=22 from item override, got %q", got)
	}
	if got := envVarValue(cr.Spec.Gsm[0].EnvVars, "C"); got != "3" {
		t.Fatalf("expected env C=3, got %q", got)
	}
	if got := cr.Spec.Gsm[0].ServiceAnnotations["tier"]; got != "item" {
		t.Fatalf("expected serviceAnnotations tier=item, got %q", got)
	}
}

func envVarValue(envs []EnvironmentVariable, key string) string {
	for _, e := range envs {
		if e.Name == key {
			return e.Value
		}
	}
	return ""
}

func TestDefaultDbSecretGsmWalletDefaults(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			DbSecret: &SecretDetails{
				Name: "db-secret",
				DbAdmin: PasswordSecretConfig{
					PasswordKey: "oracle_pwd",
				},
			},
		},
	}

	if err := cr.Default(context.Background(), cr); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if got := strings.ToLower(strings.TrimSpace(cr.Spec.DbSecret.UseGsmWallet)); got != "true" {
		t.Fatalf("expected dbSecret.useGsmWallet default true, got %q", cr.Spec.DbSecret.UseGsmWallet)
	}
	if got := strings.TrimSpace(cr.Spec.DbSecret.GsmWalletRoot); got != DefaultGsmWalletRoot {
		t.Fatalf("expected dbSecret.gsmWalletRoot default %q, got %q", DefaultGsmWalletRoot, cr.Spec.DbSecret.GsmWalletRoot)
	}
}

func TestValidateDbSecretConfigUseGsmWalletValidation(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			DbSecret: &SecretDetails{
				Name:         "db-secret",
				UseGsmWallet: "maybe",
				DbAdmin: PasswordSecretConfig{
					PasswordKey: "oracle_pwd",
				},
			},
		},
	}

	errList := cr.validateDbSecretConfig()
	if len(errList) == 0 {
		t.Fatalf("expected validation error for invalid useGsmWallet")
	}
	errs := make([]error, 0, len(errList))
	for _, e := range errList {
		errs = append(errs, e)
	}
	if !hasErrContaining(errs, "usegsmwallet must be") {
		t.Fatalf("expected useGsmWallet validation error, got: %v", errs)
	}
}

func TestValidateComputeSizingPathConfigMutuallyExclusive(t *testing.T) {
	cr := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			Catalog: []CatalogSpec{
				{
					Name:      "catalog1",
					Shape:     "kodb4",
					Resources: &corev1.ResourceRequirements{},
				},
			},
			ShardInfo: []ShardingDetails{
				{
					ShardPreFixName: "shard",
					Shape:           "kodb4",
					Resources:       &corev1.ResourceRequirements{},
				},
			},
		},
	}

	errList := cr.validateComputeSizingPathConfig()
	if len(errList) == 0 {
		t.Fatalf("expected validation error for mutually exclusive shape/resources")
	}
	errs := make([]error, 0, len(errList))
	for _, e := range errList {
		errs = append(errs, e)
	}
	if !hasErrContaining(errs, "mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got: %v", errs)
	}
}

func TestValidateComputeSizingPathUpdateImmutable(t *testing.T) {
	oldCR := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			Catalog: []CatalogSpec{
				{Name: "catalog1", Shape: "kodb4"},
			},
			ShardInfo: []ShardingDetails{
				{ShardPreFixName: "shard", Shape: "kodb4"},
			},
		},
	}
	newCR := &ShardingDatabase{
		Spec: ShardingDatabaseSpec{
			Catalog: []CatalogSpec{
				{Name: "catalog1", Resources: &corev1.ResourceRequirements{}},
			},
			ShardInfo: []ShardingDetails{
				{ShardPreFixName: "shard", Resources: &corev1.ResourceRequirements{}},
			},
		},
	}

	errList := newCR.validateComputeSizingPathUpdate(oldCR)
	if len(errList) == 0 {
		t.Fatalf("expected immutable sizing path error")
	}
	errs := make([]error, 0, len(errList))
	for _, e := range errList {
		errs = append(errs, e)
	}
	if !hasErrContaining(errs, "immutable") {
		t.Fatalf("expected immutable error, got: %v", errs)
	}
}
