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

type recordingWarningHandler struct {
	messages []string
}

func (h *recordingWarningHandler) HandleWarningHeader(_ int, _, message string) {
	h.messages = append(h.messages, message)
}

func TestOperatorWarningHandlerSuppressesOracleRestDataServiceDeprecation(t *testing.T) {
	delegate := &recordingWarningHandler{}
	handler := operatorWarningHandler{delegate: delegate}

	handler.HandleWarningHeader(299, "", "OracleRestDataService is deprecated; use the OrdsSrvs controller instead.")
	if len(delegate.messages) != 0 {
		t.Fatalf("expected OracleRestDataService deprecation warning to be suppressed, got %v", delegate.messages)
	}

	handler.HandleWarningHeader(299, "", "some other API warning")
	if len(delegate.messages) != 1 || delegate.messages[0] != "some other API warning" {
		t.Fatalf("expected other API warnings to be delegated, got %v", delegate.messages)
	}
}
