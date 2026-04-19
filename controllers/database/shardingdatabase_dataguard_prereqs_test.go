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
