package main

import "testing"

func TestParseFlagsFromArgs_DefaultsLeaderElectionEnabled(t *testing.T) {
	metricsAddr, enableLeaderElection, err := parseFlagsFromArgs(nil)
	if err != nil {
		t.Fatalf("parseFlagsFromArgs returned error: %v", err)
	}
	if metricsAddr != ":8443" {
		t.Fatalf("expected default metrics addr :8443, got %q", metricsAddr)
	}
	if !enableLeaderElection {
		t.Fatal("expected leader election to default to enabled")
	}
}

func TestParseFlagsFromArgs_AllowsExplicitLeaderElectionDisable(t *testing.T) {
	_, enableLeaderElection, err := parseFlagsFromArgs([]string{"--enable-leader-election=false"})
	if err != nil {
		t.Fatalf("parseFlagsFromArgs returned error: %v", err)
	}
	if enableLeaderElection {
		t.Fatal("expected leader election to be disabled when explicitly requested")
	}
}
