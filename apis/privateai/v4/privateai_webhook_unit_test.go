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

func TestPrivateAIValidate_RejectsMissingItemKey(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Security: &PrivateAiSecuritySpec{
				Secret: &PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
					Items:         []SecretMountItem{{Path: "api-key"}},
				},
			},
		},
	}

	_, err := validator.validate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected validation error for missing item key")
	}
	if !strings.Contains(err.Error(), "spec.security.secret.items[0].key must be set") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestPrivateAIValidate_RejectsSharedMountItemPathCollision(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Security: &PrivateAiSecuritySpec{
				Secret: &PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
					Items: []SecretMountItem{
						{Key: "keystore"},
					},
				},
				TLS: &PaiTLSSpec{
					SecretName:    "tls-secret",
					MountLocation: "/run/secrets",
					Items: []SecretMountItem{
						{Key: "keystore.p12", Path: "keystore"},
					},
				},
			},
		},
	}

	_, err := validator.validate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected validation error for shared-mount path collision")
	}
	if !strings.Contains(err.Error(), `cannot resolve to the same mounted path "keystore"`) {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestPrivateAIDefault_RejectsBothNewListenersEnabled(t *testing.T) {
	validator := &PrivateAi{}
	trueValue := true
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Networking: &PrivateAiNetworkingSpec{
				Listeners: &PrivateAiListenersSpec{
					HTTP:  &PrivateAiListenerSpec{Enabled: &trueValue},
					HTTPS: &PrivateAiListenerSpec{Enabled: &trueValue},
				},
			},
		},
	}

	err := validator.Default(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected defaulting error when both new listeners are enabled")
	}
	if !strings.Contains(err.Error(), "spec.networking.listeners.http.enabled") {
		t.Fatalf("expected new listener field names in error, got: %v", err)
	}
}

func TestDeprecatedPrivateAIFieldWarnings_UseGroupedFieldNames(t *testing.T) {
	warnings := deprecatedPrivateAIFieldWarnings(&PrivateAiSpec{
		IsExternalSvc:   "true",
		PaiLBPort:       443,
		PailbAnnotation: map[string]string{"k": "v"},
		PortMappings: []PaiPortMapping{
			{Port: 443, TargetPort: 8443},
		},
	})

	joined := strings.Join(warnings, "\n")
	expected := []string{
		"spec.networking.service.external.enabled",
		"spec.networking.service.external.port",
		"spec.networking.service.external.annotations",
		"spec.networking.service.portMappings",
	}

	for _, want := range expected {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected warning to mention %q, got warnings: %v", want, warnings)
		}
	}
}

func TestPrivateAIValidate_UsesGroupedAuthFieldInError(t *testing.T) {
	validator := &privateAiValidator{}
	trueValue := true
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Security: &PrivateAiSecuritySpec{
				AuthEnabled: &trueValue,
			},
		},
	}

	_, err := validator.validate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected validation error when auth is enabled without a secret")
	}
	if !strings.Contains(err.Error(), "spec.security.authEnabled=true") {
		t.Fatalf("expected grouped auth field in error, got: %v", err)
	}
}

func TestPrivateAIValidate_IgnoresDeprecatedPaiSecretWhenNewSecretIsPresent(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			Security: &PrivateAiSecuritySpec{
				Secret: &PaiSecretSpec{
					Name:          "auth-secret",
					MountLocation: "/run/secrets",
				},
			},
			PaiSecret: &PaiSecretSpec{
				Name:          "legacy-auth",
				MountLocation: "relative/path",
			},
		},
	}

	if _, err := validator.validate(context.Background(), obj); err != nil {
		t.Fatalf("expected deprecated paiSecret to be ignored when spec.security.secret is present, got error: %v", err)
	}
}

func TestPrivateAIValidate_UsesLegacyFieldNameForLegacyPaiSecretItemErrors(t *testing.T) {
	validator := &privateAiValidator{}
	obj := &PrivateAi{
		Spec: PrivateAiSpec{
			PaiSecret: &PaiSecretSpec{
				Name:          "legacy-auth",
				MountLocation: "/run/secrets",
				Items:         []SecretMountItem{{Path: "api-key"}},
			},
		},
	}

	_, err := validator.validate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected validation error for legacy paiSecret item")
	}
	if !strings.Contains(err.Error(), "paiSecret.items[0].key must be set") {
		t.Fatalf("expected legacy paiSecret field name in error, got: %v", err)
	}
}
