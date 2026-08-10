// Package orautil provides small Oracle-specific string normalization helpers.
package orautil

import "strings"

// EnsurePlusPrefix guarantees ASM disk-group names start with '+'.
func EnsurePlusPrefix(name string) string {
	if name == "" {
		return ""
	}
	if !strings.HasPrefix(name, "+") {
		return "+" + name
	}
	return name
}

// NormalizeOracleMemoryUnit converts Gi/Mi style memory units to Oracle-friendly G/M.
func NormalizeOracleMemoryUnit(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.ReplaceAll(s, "GI", "G")
	s = strings.ReplaceAll(s, "MI", "M")
	return s
}
