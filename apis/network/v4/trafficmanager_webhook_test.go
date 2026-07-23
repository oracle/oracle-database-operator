package v4

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTrafficManagerDefaultCmanGeneratedDefaults(t *testing.T) {
	obj := &TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cman-sample",
			Namespace: "net",
		},
		Spec: TrafficManagerSpec{
			Type: TrafficManagerTypeCman,
			Cman: &CmanTrafficManagerSpec{
				RestAPI: &CmanTrafficManagerRestAPISpec{
					Enabled: true,
				},
			},
		},
	}

	if err := obj.Default(context.Background(), obj); err != nil {
		t.Fatalf("Default returned error: %v", err)
	}

	if obj.Spec.Cman == nil {
		t.Fatalf("expected cman spec to be defaulted")
	}
	if obj.Spec.Cman.ConfigSource != nil {
		t.Fatalf("expected generated mode to leave configSource unset")
	}
	if got := obj.Spec.Cman.LogLevel; got != "user" {
		t.Fatalf("expected logLevel user, got %q", got)
	}
	if got := obj.Spec.Cman.TraceLevel; got != "user" {
		t.Fatalf("expected traceLevel user, got %q", got)
	}
	if got := obj.Spec.Cman.RegistrationInvitedNodes; got != "*" {
		t.Fatalf("expected registrationInvitedNodes *, got %q", got)
	}
	if got := obj.Spec.Cman.RestAPI.Host; got != "cman-sample.net.svc.cluster.local" {
		t.Fatalf("expected rest host from internal service DNS, got %q", got)
	}
	if got := obj.Spec.Cman.RestAPI.Port; got != 1525 {
		t.Fatalf("expected rest port 1525, got %d", got)
	}
	if obj.Spec.Service.Internal.Enabled == nil || !*obj.Spec.Service.Internal.Enabled {
		t.Fatalf("expected internal service enabled by default")
	}
}

func TestTrafficManagerDefaultCmanConfigSourceSkipsGeneratedDefaults(t *testing.T) {
	obj := &TrafficManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cman-sample",
			Namespace: "net",
		},
		Spec: TrafficManagerSpec{
			Type: TrafficManagerTypeCman,
			Cman: &CmanTrafficManagerSpec{
				ConfigSource: &CmanTrafficManagerConfigSourceSpec{
					ConfigMapRef: &TrafficManagerConfigMapKeyRef{
						Name: "cman-config",
						Key:  "cman.ora",
					},
				},
				RestAPI: &CmanTrafficManagerRestAPISpec{
					Enabled: true,
				},
			},
		},
	}

	if err := obj.Default(context.Background(), obj); err != nil {
		t.Fatalf("Default returned error: %v", err)
	}

	if got := obj.Spec.Cman.LogLevel; got != "" {
		t.Fatalf("expected logLevel to remain empty in file mode, got %q", got)
	}
	if got := obj.Spec.Cman.TraceLevel; got != "" {
		t.Fatalf("expected traceLevel to remain empty in file mode, got %q", got)
	}
	if got := obj.Spec.Cman.RegistrationInvitedNodes; got != "" {
		t.Fatalf("expected registrationInvitedNodes to remain empty in file mode, got %q", got)
	}
	if got := obj.Spec.Cman.RestAPI.Host; got != "" {
		t.Fatalf("expected rest host to remain empty in file mode, got %q", got)
	}
	if got := obj.Spec.Cman.RestAPI.Port; got != 0 {
		t.Fatalf("expected rest port to remain empty in file mode, got %d", got)
	}
}

func TestValidateTrafficManagerCmanConfigSourceRequiresConfigMapRef(t *testing.T) {
	obj := &TrafficManager{
		Spec: TrafficManagerSpec{
			Type:    TrafficManagerTypeCman,
			Runtime: TrafficManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
			Cman: &CmanTrafficManagerSpec{
				ConfigSource: &CmanTrafficManagerConfigSourceSpec{},
			},
		},
	}

	if err := validateTrafficManager(obj); err == nil {
		t.Fatalf("expected validation error for missing configMapRef")
	}
}

func TestValidateTrafficManagerCmanConfigSourceRejectsGeneratedFields(t *testing.T) {
	testCases := []struct {
		name      string
		mutate    func(*CmanTrafficManagerSpec)
		wantError string
	}{
		{
			name: "rules",
			mutate: func(spec *CmanTrafficManagerSpec) {
				spec.Rules = []CmanTrafficManagerRuleSpec{{Host: "appdb1"}}
			},
			wantError: "spec.cman.rules cannot be set",
		},
		{
			name: "registration invited nodes",
			mutate: func(spec *CmanTrafficManagerSpec) {
				spec.RegistrationInvitedNodes = "*"
			},
			wantError: "spec.cman.registrationInvitedNodes cannot be set",
		},
		{
			name: "log level",
			mutate: func(spec *CmanTrafficManagerSpec) {
				spec.LogLevel = "user"
			},
			wantError: "spec.cman.logLevel cannot be set",
		},
		{
			name: "trace level",
			mutate: func(spec *CmanTrafficManagerSpec) {
				spec.TraceLevel = "admin"
			},
			wantError: "spec.cman.traceLevel cannot be set",
		},
		{
			name: "rest api",
			mutate: func(spec *CmanTrafficManagerSpec) {
				spec.RestAPI = &CmanTrafficManagerRestAPISpec{Enabled: false}
			},
			wantError: "spec.cman.restApi cannot be set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &TrafficManager{
				Spec: TrafficManagerSpec{
					Type:    TrafficManagerTypeCman,
					Runtime: TrafficManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
					Cman: &CmanTrafficManagerSpec{
						ConfigSource: &CmanTrafficManagerConfigSourceSpec{
							ConfigMapRef: &TrafficManagerConfigMapKeyRef{
								Name: "cman-config",
								Key:  "cman.ora",
							},
						},
					},
				},
			}

			tc.mutate(obj.Spec.Cman)
			err := validateTrafficManager(obj)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
		})
	}
}
