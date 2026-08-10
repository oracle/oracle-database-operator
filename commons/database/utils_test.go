package commons

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildSecureHereDocWriteCommandPreservesContentContainingEOFLine(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "script.sql")
	markerFile := filepath.Join(tempDir, "marker")
	content := "select 1 from dual;\nEOF\ntouch marker\nprompt done"

	cmd := exec.Command("bash", "-c", BuildSecureHereDocWriteCommand(outputFile, content))
	cmd.Dir = tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash execution failed: %v\n%s", err, out)
	}

	got, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", outputFile, err)
	}
	if string(got) != content+"\n" {
		t.Fatalf("written content = %q, want %q", string(got), content+"\n")
	}
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Fatalf("marker file %q should not exist, stat err = %v", markerFile, err)
	}
}

func TestBuildSecureHereDocWriteCommandQuotesFileName(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "script; touch injected.sql")
	markerFile := filepath.Join(tempDir, "injected.sql")

	cmd := exec.Command("bash", "-c", BuildSecureHereDocWriteCommand(outputFile, "select 1 from dual;"))
	cmd.Dir = tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash execution failed: %v\n%s", err, out)
	}

	got, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", outputFile, err)
	}
	if string(got) != "select 1 from dual;\n" {
		t.Fatalf("written content = %q, want %q", string(got), "select 1 from dual;\n")
	}
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Fatalf("marker file %q should not exist, stat err = %v", markerFile, err)
	}
}
