package k8sobjects

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMergeServicePortsWithAssignedNodePortsByNamePortProtocol(t *testing.T) {
	existing := []corev1.ServicePort{
		{Name: "sql", Port: 1521, Protocol: corev1.ProtocolTCP, NodePort: 30001},
	}
	desired := []corev1.ServicePort{
		{Name: "sql", Port: 1521, Protocol: corev1.ProtocolTCP},
		{Name: "sql", Port: 1522, Protocol: corev1.ProtocolTCP},
	}

	got := MergeServicePortsWithAssignedNodePortsByNamePortProtocol(existing, desired)
	if got[0].NodePort != 30001 {
		t.Fatalf("expected nodeport to be preserved for exact key, got %d", got[0].NodePort)
	}
	if got[1].NodePort != 0 {
		t.Fatalf("expected non-matching key to remain 0, got %d", got[1].NodePort)
	}
}

func TestMergeServicePortsWithAssignedNodePortByName(t *testing.T) {
	existing := []corev1.ServicePort{
		{Name: "sql", NodePort: 30001},
	}
	desired := []corev1.ServicePort{
		{Name: "sql"},
		{Name: "new"},
	}

	got := MergeServicePortsWithAssignedNodePortByName(existing, desired)
	if got[0].NodePort != 30001 {
		t.Fatalf("expected name-based nodeport preserve, got %d", got[0].NodePort)
	}
	if got[1].NodePort != 0 {
		t.Fatalf("expected unmatched name nodeport 0, got %d", got[1].NodePort)
	}
}
