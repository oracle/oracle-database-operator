package v4

import (
	"context"
	"strings"
	"testing"
)

func TestPrivateAIValidate_AllowsSharedAuthAndTLSMountPath(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			PaiEnableAuthentication: "true",
			Security: &PrivateAiSecuritySpec{
				Secret: &PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
				},
				TLS: &PaiTLSSpec{
					SecretName:    "tls-secret",
					MountLocation: "/run/secrets",
				},
			},
		},
	}

	if _, err := validator.validate(context.Background(), obj); err != nil {
		t.Fatalf("expected shared mount path to be allowed, got error: %v", err)
	}
}

func TestPrivateAIValidate_RejectsRelativeTLSMountPath(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Security: &PrivateAiSecuritySpec{
				TLS: &PaiTLSSpec{
					SecretName:    "tls-secret",
					MountLocation: "relative/path",
				},
			},
		},
	}

	_, err := validator.validate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected validation error for relative TLS mount path")
	}
	if !strings.Contains(err.Error(), "spec.security.tls.mountLocation must be an absolute path") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
