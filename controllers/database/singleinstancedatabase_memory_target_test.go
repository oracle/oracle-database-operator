package controllers

import "testing"

func TestSIDBMemoryTargetMB(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "gib", value: "20Gi", want: 20 * 1024},
		{name: "gb", value: "20GB", want: 19074},
		{name: "space", value: "50 GB", want: 47684},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := sidbMemoryTargetMB(test.value)
			if err != nil || got != test.want {
				t.Fatalf("sidbMemoryTargetMB(%q) = %d, %v; want %d", test.value, got, err, test.want)
			}
		})
	}
}

func TestSIDBMemoryTargetMBRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0Gi", "invalid", "-1Gi"} {
		if got, err := sidbMemoryTargetMB(value); err == nil || got != 0 {
			t.Errorf("sidbMemoryTargetMB(%q) = %d, %v; want an error", value, got, err)
		}
	}
}
