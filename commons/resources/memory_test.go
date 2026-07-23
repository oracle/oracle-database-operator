package resources

import (
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

func TestCalculateOracleSysctls(t *testing.T) {
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
	if len(sysctls) == 0 {
		t.Fatalf("expected sysctls")
	}
}

func TestValidateSgaPgaSafety(t *testing.T) {
	oneGi := int64(1024 * 1024 * 1024)

	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 12*oneGi, 0, 0.80); err != nil {
		t.Fatalf("expected non-hugepages budget to pass, got err: %v", err)
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 10*oneGi, 0, 0.80); err == nil {
		t.Fatalf("expected non-hugepages budget failure")
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 3*oneGi, 8*oneGi, 0.80); err != nil {
		t.Fatalf("expected hugepages budget to pass, got err: %v", err)
	}
	if err := ValidateSgaPgaSafety(8*oneGi, 2*oneGi, 2*oneGi, 8*oneGi, 0.80); err == nil {
		t.Fatalf("expected hugepages regular-memory budget failure")
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
