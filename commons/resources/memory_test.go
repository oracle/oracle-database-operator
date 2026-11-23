package resources

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseBytesOrMemory(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"2Gi", 2 * 1024 * 1024 * 1024},
		{"512M", 512 * 1024 * 1024},
	}
	for _, tc := range cases {
		got, err := ParseBytesOrMemory(tc.in)
		if err != nil {
			t.Fatalf("ParseBytesOrMemory(%q) unexpected err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseBytesOrMemory(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
}

func TestExtractMemoryAndHugePagesBytes(t *testing.T) {
	res := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory:                resource.MustParse("16Gi"),
			corev1.ResourceName("hugepages-2Mi"): resource.MustParse("8Gi"),
		},
	}
	mem, huge := ExtractMemoryAndHugePagesBytes(res)
	if mem != 16*1024*1024*1024 {
		t.Fatalf("mem=%d want %d", mem, int64(16*1024*1024*1024))
	}
	if huge != 8*1024*1024*1024 {
		t.Fatalf("huge=%d want %d", huge, int64(8*1024*1024*1024))
	}
}

func TestCalculateOracleSysctlsDoesNotGenerateRegularMemoryDefaults(t *testing.T) {
	sysctls, err := CalculateOracleSysctls(
		8*1024*1024*1024,
		2*1024*1024*1024,
		32*1024*1024*1024,
		0,
		0,
		0,
	)
	if err != nil {
		t.Fatalf("CalculateOracleSysctls unexpected err: %v", err)
	}
	if len(sysctls) != 0 {
		t.Fatalf("did not expect automatic regular-memory sysctls, got %#v", sysctls)
	}
}

func TestCalculateOracleSysctlsPreservesExplicitShmmax(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)
	wantedShmmax := int64(32) * oneGi

	sysctls, err := CalculateOracleSysctls(8*oneGi, 2*oneGi, 12*oneGi, 0, wantedShmmax, 0)
	if err != nil {
		t.Fatalf("CalculateOracleSysctls unexpected err: %v", err)
	}
	for _, sysctl := range sysctls {
		if sysctl.Name == "kernel.shmmax" && sysctl.Value == fmt.Sprintf("%d", wantedShmmax) {
			return
		}
	}
	t.Fatalf("expected explicit shmmax %d to be preserved, got %#v", wantedShmmax, sysctls)
}

func TestValidateSgaPgaSafety(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 12*oneGi, 0, 0.80); err != nil {
		t.Fatalf("expected non-hugepages budget to pass, got err: %v", err)
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 10*oneGi, 0, 0.80); err == nil {
		t.Fatalf("expected non-hugepages budget failure")
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 4*oneGi, 8*oneGi, 0.80); err != nil {
		t.Fatalf("expected hugepages budget to pass, got err: %v", err)
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 2*oneGi, 8*oneGi, 0.80); err == nil {
		t.Fatalf("expected hugepages regular-memory budget failure")
	}
}

func TestCalculateOracleSysctlsAllowsHugePagesBackedSga(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	if _, err := CalculateOracleSysctls(
		8*oneGi,
		2*oneGi,
		4*oneGi,
		8*oneGi,
		0,
		0,
	); err != nil {
		t.Fatalf("expected HugePages-backed SGA to be independent of regular memory, got err: %v", err)
	}
}

func TestCalculateOracleSysctlsSetsShmmaxToHugePages(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)
	hugePages := int64(8) * oneGi

	sysctls, err := CalculateOracleSysctls(8*oneGi, 2*oneGi, 4*oneGi, hugePages, 0, 0)
	if err != nil {
		t.Fatalf("CalculateOracleSysctls unexpected err: %v", err)
	}
	for _, sysctl := range sysctls {
		if sysctl.Name == "kernel.shmmax" && sysctl.Value == fmt.Sprintf("%d", hugePages) {
			return
		}
	}
	t.Fatalf("expected shmmax to equal HugePages allocation %d, got %#v", hugePages, sysctls)
}

func TestCalculateOracleSysctlsRejectsMemoryBelowFourGiB(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	if _, err := CalculateOracleSysctls(
		1*oneGi,
		1*oneGi,
		3*oneGi,
		0,
		0,
		0,
	); err == nil {
		t.Fatalf("expected regular container memory below 4GiB to fail")
	}
}

func TestCalculateOracleSysctlsOverheadChecks(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	if _, err := CalculateOracleSysctls(
		8*oneGi,
		2*oneGi,
		10*oneGi,
		0,
		0,
		0,
	); err == nil {
		t.Fatalf("expected non-hugepages overhead validation failure")
	}

	if _, err := CalculateOracleSysctls(
		8*oneGi,
		2*oneGi,
		2*oneGi,
		8*oneGi,
		0,
		0,
	); err == nil {
		t.Fatalf("expected hugepages regular-memory overhead validation failure")
	}
}

func TestValidateSgaPgaSafetyDerivedFallback(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	// With mem=16Gi and missing SGA/PGA:
	// derived SGA=8Gi, derived PGA=2Gi, overhead=1Gi.
	if err := ValidateSgaPgaSafety(0, 0, 16*oneGi, 8*oneGi, 0.80); err != nil {
		t.Fatalf("expected derived hugepages fallback to pass, got err: %v", err)
	}
	if err := ValidateSgaPgaSafety(0, 0, 16*oneGi, 4*oneGi, 0.80); err == nil {
		t.Fatalf("expected derived hugepages >= SGA failure")
	}
}

func TestCalculateOracleSysctlsDerivedFallback(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	// Missing SGA/PGA should derive from container memory and enforce hugepages >= derived SGA.
	if _, err := CalculateOracleSysctls(
		0,
		0,
		16*oneGi,
		4*oneGi,
		0,
		0,
	); err == nil {
		t.Fatalf("expected hugepages < derived SGA failure")
	}

	if _, err := CalculateOracleSysctls(
		0,
		0,
		16*oneGi,
		8*oneGi,
		0,
		0,
	); err != nil {
		t.Fatalf("expected derived fallback to pass, got err: %v", err)
	}
}
