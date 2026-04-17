package commons

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMergeCapabilitiesWithDefaultsKeepsDefaultsWhenUserCapsOmitted(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, nil)
	if !reflect.DeepEqual(got, defaultCaps) {
		t.Fatalf("expected defaults to be preserved, got %#v", got)
	}
}

func TestMergeCapabilitiesWithDefaultsDisablesDefaultsForExplicitEmptyObject(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, &corev1.Capabilities{})
	want := &corev1.Capabilities{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit empty capabilities to disable defaults, got %#v", got)
	}
}

func TestMergeCapabilitiesWithDefaultsMergesNonEmptyUserCaps(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}
	userCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_RAW"},
		Drop: []corev1.Capability{"CHOWN"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, userCaps)
	want := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE", "NET_RAW"},
		Drop: []corev1.Capability{"ALL", "CHOWN"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected merged capabilities %#v, got %#v", want, got)
	}
}
