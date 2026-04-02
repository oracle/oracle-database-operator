package netutil

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPreferredNodeIP(t *testing.T) {
	node := &corev1.Node{
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeExternalIP, Address: "34.1.1.1"},
			},
		},
	}
	if got := PreferredNodeIP(node); got != "34.1.1.1" {
		t.Fatalf("expected external IP, got %q", got)
	}
}

func TestParseHostsContent(t *testing.T) {
	content := "# comment\n10.0.0.1 a\n\n10.0.0.2 b\n"
	if got := ParseHostsContent(content); got != "10.0.0.2 b" {
		t.Fatalf("unexpected parsed hosts content: %q", got)
	}
}
