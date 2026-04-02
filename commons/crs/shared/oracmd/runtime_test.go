package oracmd

import "testing"

func TestRuntimeConstants(t *testing.T) {
	if ScriptMount == "" || Python3Cmd == "" || DBUser == "" || GIUser == "" {
		t.Fatalf("runtime constants must not be empty")
	}
}
