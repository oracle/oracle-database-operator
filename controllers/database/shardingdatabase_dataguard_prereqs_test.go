package controllers

import (
	"testing"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldRunShardingDataguardPrereqsDisabled(t *testing.T) {
	inst := &databasev4.ShardingDatabase{}
	if shouldRunShardingDataguardPrereqs(inst, "shard1") {
		t.Fatalf("expected disabled prereqs to skip execution")
	}
}

func TestShouldRunShardingDataguardPrereqsFirstRun(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "gdd", Namespace: "ns"},
		Spec: databasev4.ShardingDatabaseSpec{
			Dataguard: &databasev4.DataguardProducerSpec{
				Prereqs: &databasev4.DataguardPrereqsSpec{Enabled: true},
			},
		},
	}
	if !shouldRunShardingDataguardPrereqs(inst, "shard1") {
		t.Fatalf("expected initial sharding prereqs run to be required")
	}
}

func TestShouldRunShardingDataguardPrereqsSkipAfterSuccess(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "gdd", Namespace: "ns"},
		Spec: databasev4.ShardingDatabaseSpec{
			Dataguard: &databasev4.DataguardProducerSpec{
				Prereqs: &databasev4.DataguardPrereqsSpec{Enabled: true, StandbyRedoSize: "512M"},
			},
		},
		Status: databasev4.ShardingDatabaseStatus{
			DataguardPrereqsHash:       map[string]string{},
			DataguardPrereqsRerunToken: map[string]string{"shard1": ""},
		},
	}
	inst.Status.DataguardPrereqsHash["shard1"] = shardingDataguardPrereqsDesiredHash(inst)
	if shouldRunShardingDataguardPrereqs(inst, "shard1") {
		t.Fatalf("expected successful sharding prereqs state to skip execution")
	}
}

func TestShouldRunShardingDataguardPrereqsRerunTokenChange(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "gdd",
			Namespace:   "ns",
			Annotations: map[string]string{shardingDataguardPrereqsRerunAnnotation: "run-2"},
		},
		Spec: databasev4.ShardingDatabaseSpec{
			Dataguard: &databasev4.DataguardProducerSpec{
				Prereqs: &databasev4.DataguardPrereqsSpec{Enabled: true},
			},
		},
		Status: databasev4.ShardingDatabaseStatus{
			DataguardPrereqsHash:       map[string]string{},
			DataguardPrereqsRerunToken: map[string]string{"shard1": "run-1"},
		},
	}
	inst.Status.DataguardPrereqsHash["shard1"] = shardingDataguardPrereqsDesiredHash(inst)
	if !shouldRunShardingDataguardPrereqs(inst, "shard1") {
		t.Fatalf("expected rerun token change to force sharding prereqs execution")
	}
}

func TestIsManagedPrimaryShard(t *testing.T) {
	inst := &databasev4.ShardingDatabase{}

	tests := []struct {
		name  string
		shard databasev4.ShardSpec
		want  bool
	}{
		{
			name:  "primary shard included",
			shard: databasev4.ShardSpec{Name: "pri1", DeployAs: "PRIMARY"},
			want:  true,
		},
		{
			name:  "default deployAs treated as primary",
			shard: databasev4.ShardSpec{Name: "pri2"},
			want:  true,
		},
		{
			name:  "standby shard excluded",
			shard: databasev4.ShardSpec{Name: "std1", DeployAs: "STANDBY"},
			want:  false,
		},
		{
			name:  "active standby shard excluded",
			shard: databasev4.ShardSpec{Name: "astd1", DeployAs: "ACTIVE_STANDBY"},
			want:  false,
		},
		{
			name:  "deleted shard excluded",
			shard: databasev4.ShardSpec{Name: "pri3", DeployAs: "PRIMARY", IsDelete: "enable"},
			want:  false,
		},
	}

	for _, tt := range tests {
		if got := isManagedPrimaryShard(inst, tt.shard); got != tt.want {
			t.Fatalf("%s: expected %t, got %t", tt.name, tt.want, got)
		}
	}
}
