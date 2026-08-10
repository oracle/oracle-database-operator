package commons

import (
	"fmt"
	"strings"
	"testing"
)

func TestDisableFSFOCMDTerminatesStopObserverCommand(t *testing.T) {
	script := fmt.Sprintf(DisableFSFOCMD, "sidb-standby-dg")

	if !strings.Contains(script, "STOP OBSERVER sidb-standby-dg;\nDISABLE FAST_START FAILOVER;") {
		t.Fatalf("expected STOP OBSERVER and DISABLE FAST_START FAILOVER to be separate DGMGRL commands, got %q", script)
	}
	if strings.Contains(script, "sidb-standby-dg DISABLE") {
		t.Fatalf("STOP OBSERVER command is missing a terminator before DISABLE FAST_START FAILOVER: %q", script)
	}
}

func TestGetORDSStatusUsesUnmappedLandingEndpoint(t *testing.T) {
	statusCommand := fmt.Sprintf(GetORDSStatus, ORDSDefaultHTTPPort)

	if !strings.Contains(statusCommand, "/ords/_/landing") {
		t.Fatalf("expected ORDS status command to use landing endpoint, got %q", statusCommand)
	}
	if strings.Contains(statusCommand, "/db-api/") {
		t.Fatalf("ORDS status command must not require a database mapping, got %q", statusCommand)
	}
	for _, fragment := range []string{"-sSkfL", "-o /dev/null", "-w 'HTTP %{http_code}\\n'"} {
		if !strings.Contains(statusCommand, fragment) {
			t.Fatalf("expected ORDS status command to contain %q, got %q", fragment, statusCommand)
		}
	}
}
