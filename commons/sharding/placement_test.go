package commons

import (
	"testing"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPodSpecForShardAppliesPlacementPreferences(t *testing.T) {
	instance := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "shdb"},
		Spec: databasev4.ShardingDatabaseSpec{
			DbSecret: &databasev4.SecretDetails{Name: "db-secret"},
		},
	}

	spec, err := buildPodSpecForShard(instance, databasev4.ShardSpec{
		Name:  "sh1",
		Nodes: []string{" worker-a ", "worker-b", "worker-a"},
		NodeSelector: map[string]string{
			"node-role.kubernetes.io/db": "true",
		},
	})
	if err != nil {
		t.Fatalf("buildPodSpecForShard() error = %v", err)
	}
	if got := spec.NodeSelector["node-role.kubernetes.io/db"]; got != "true" {
		t.Fatalf("expected nodeSelector to be preserved, got %q", got)
	}
	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil || spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatalf("expected node affinity to be configured")
	}
	values := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values
	if len(values) != 2 || values[0] != "worker-a" || values[1] != "worker-b" {
		t.Fatalf("expected normalized node list [worker-a worker-b], got %v", values)
	}
	if spec.Affinity.PodAntiAffinity == nil || len(spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Fatalf("expected one preferred pod anti-affinity rule")
	}
	term := spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0]
	if term.PodAffinityTerm.TopologyKey != corev1.LabelHostname {
		t.Fatalf("expected topology key %q, got %q", corev1.LabelHostname, term.PodAffinityTerm.TopologyKey)
	}
}

func TestBuildPodSpecForGsmAppliesPlacementPreferences(t *testing.T) {
	instance := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "shdb"},
		Spec: databasev4.ShardingDatabaseSpec{
			DbSecret: &databasev4.SecretDetails{Name: "db-secret"},
			GsmImage: "example/gsm:latest",
			Gsm: []databasev4.GsmSpec{{
				Name: "gsm1",
			}},
		},
	}

	spec := buildPodSpecForGsm(instance, databasev4.GsmSpec{
		Name:  "gsm1",
		Nodes: []string{"worker-x", "worker-y"},
	})
	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil || spec.Affinity.PodAntiAffinity == nil {
		t.Fatalf("expected gsm placement affinity to be configured")
	}
	values := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values
	if len(values) != 2 || values[0] != "worker-x" || values[1] != "worker-y" {
		t.Fatalf("expected gsm node list [worker-x worker-y], got %v", values)
	}
	if len(spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) != 1 {
		t.Fatalf("expected one preferred anti-affinity rule for gsm")
	}
}
