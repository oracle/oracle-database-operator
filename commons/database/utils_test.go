package commons

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseSIDBInitParamValueSupportsOracleMemoryUnits(t *testing.T) {
	t.Parallel()

	tests := map[string]int{"2G": 2048, "14g": 14336, "2048M": 2048, "320": 320}
	for input, want := range tests {
		got, err := parseSIDBInitParamValue(input)
		if err != nil {
			t.Errorf("parseSIDBInitParamValue(%q) returned error: %v", input, err)
		} else if got != want {
			t.Errorf("parseSIDBInitParamValue(%q) = %d, want %d", input, got, want)
		}
	}
}

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

func TestParseSIDBInitParamValueNormalizesOracleMemoryUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "gigabytes", value: "2G", want: 2048},
		{name: "lowercase gigabytes", value: "14g", want: 14336},
		{name: "megabytes", value: "2048M", want: 2048},
		{name: "plain megabytes", value: "1024", want: 1024},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSIDBInitParamValue(test.value)
			if err != nil {
				t.Fatalf("parseSIDBInitParamValue(%q) failed: %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("parseSIDBInitParamValue(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

func TestParseSIDBInitParamValueRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "2.5G", "not-a-number"} {
		if _, err := parseSIDBInitParamValue(value); err == nil {
			t.Errorf("parseSIDBInitParamValue(%q) returned no error", value)
		}
	}
}
