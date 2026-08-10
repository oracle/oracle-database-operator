package naming

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestAsmNames(t *testing.T) {
	pvc := AsmPVCName("/dev/mapper/asm1", "MyDB", 63)
	pv := AsmPVName("/dev/mapper/asm1", "MyDB", 63)
	if pvc == "" || pv == "" {
		t.Fatalf("expected non-empty asm names")
	}
	if len(pvc) > 63 || len(pv) > 63 {
		t.Fatalf("expected names <= 63 chars")
	}

	sum := sha256.Sum256([]byte("/dev/mapper/asm1"))
	prefix := hex.EncodeToString(sum[:])[:8]
	if !strings.HasPrefix(pvc, "asm-pvc-"+prefix+"-") {
		t.Fatalf("expected PVC name to use SHA-256 prefix %q, got %q", prefix, pvc)
	}
	if !strings.HasPrefix(pv, "asm-pv-"+prefix+"-") {
		t.Fatalf("expected PV name to use SHA-256 prefix %q, got %q", prefix, pv)
	}
}

func TestSanitizeK8sName(t *testing.T) {
	got := SanitizeK8sName("My_DB.Name", 63)
	if got != "my-db-name" {
		t.Fatalf("unexpected sanitize output: %q", got)
	}
}

func TestShortHashDeterministicAndTruncated(t *testing.T) {
	a := ShortHash("disk-path-1", 8)
	b := ShortHash("disk-path-1", 8)
	if a != b {
		t.Fatalf("expected deterministic hash, got %q and %q", a, b)
	}
	if len(a) != 8 {
		t.Fatalf("expected length 8, got %d (%q)", len(a), a)
	}
	full := ShortHash("disk-path-1", 0)
	if len(full) != 64 {
		t.Fatalf("expected full SHA-256 hex length 64, got %d", len(full))
	}
	if !strings.HasPrefix(full, a) {
		t.Fatalf("truncated hash %q is not prefix of full hash %q", a, full)
	}
}
