package naming

import "testing"

func TestAsmNames(t *testing.T) {
	pvc := AsmPVCName("/dev/mapper/asm1", "MyDB", 63)
	pv := AsmPVName("/dev/mapper/asm1", "MyDB", 63)
	if pvc == "" || pv == "" {
		t.Fatalf("expected non-empty asm names")
	}
	if len(pvc) > 63 || len(pv) > 63 {
		t.Fatalf("expected names <= 63 chars")
	}
}

func TestSanitizeK8sName(t *testing.T) {
	got := SanitizeK8sName("My_DB.Name", 63)
	if got != "my-db-name" {
		t.Fatalf("unexpected sanitize output: %q", got)
	}
}
