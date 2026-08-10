package orautil

import "testing"

func TestEnsurePlusPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "already prefixed", in: "+DATA", want: "+DATA"},
		{name: "needs prefix", in: "DATA", want: "+DATA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnsurePlusPrefix(tt.in); got != tt.want {
				t.Fatalf("EnsurePlusPrefix(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeOracleMemoryUnit(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "2Gi", want: "2G"},
		{in: " 1024Mi ", want: "1024M"},
		{in: "4G", want: "4G"},
	}
	for _, tt := range tests {
		if got := NormalizeOracleMemoryUnit(tt.in); got != tt.want {
			t.Fatalf("NormalizeOracleMemoryUnit(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}
