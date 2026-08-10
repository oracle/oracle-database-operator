package v4

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateUpdateTdeSecretRejectsRemovalWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &RacDatabase{}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret removal to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "cannot be removed") {
		t.Fatalf("expected removal error, got: %v", errs)
	}
}

func TestValidateUpdateTdeSecretRejectsNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "other-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret name change to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "name cannot be changed") {
		t.Fatalf("expected name immutability error, got: %v", errs)
	}
}

func TestValidateUpdateTdeSecretRejectsKeyFileNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "other-wallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret key file name change to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "KeyFileName cannot be changed") {
		t.Fatalf("expected key file immutability error, got: %v", errs)
	}
}

func TestValidateUpdateTdeSecretRejectsPwdFileNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "other-pwd.txt",
			},
		},
	}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret password file name change to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "PwdFileName cannot be changed") {
		t.Fatalf("expected password file immutability error, got: %v", errs)
	}
}

func TestValidateUpdateTdeSecretAllowsUnchangedSecretWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &RacDatabase{
		Spec: RacDatabaseSpec{
			TdeWalletSecret: &RacDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}

	if errs := newCr.validateUpdateTdeSecret(oldCr); len(errs) != 0 {
		t.Fatalf("expected unchanged TDE secret to pass, got: %v", errs)
	}
}

func TestValidateDbSecretAllowsKeyFileNameWithoutPwdFileName(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			DbSecret: &RacDbPwdSecretDetails{
				Name:        "db-user-pass",
				KeyFileName: "key.pem",
			},
		},
	}

	if errs := cr.validateDbSecret(); len(errs) != 0 {
		t.Fatalf("expected validation to pass, got: %v", errs)
	}
}

func TestValidateDbSecretAllowsPwdFileNameWithoutKeyFileName(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			DbSecret: &RacDbPwdSecretDetails{
				Name:        "db-user-pass",
				PwdFileName: "pwdfile",
			},
		},
	}

	if errs := cr.validateDbSecret(); len(errs) != 0 {
		t.Fatalf("expected validation to pass, got: %v", errs)
	}
}

func TestValidateMinMemoryLimitEnforcedByDefault(t *testing.T) {
	t.Parallel()

	errs := validateMinMemoryLimit(
		nil,
		&corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
		16*1024*1024*1024,
		field.NewPath("spec"),
	)
	if len(errs) == 0 {
		t.Fatal("expected min memory validation error, got none")
	}
}

func TestValidateMinMemoryLimitSkippedInDevMode(t *testing.T) {
	t.Parallel()

	errs := validateMinMemoryLimit(
		[]corev1.EnvVar{{Name: racWebhookDevModeEnvVar, Value: "true"}},
		&corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
		16*1024*1024*1024,
		field.NewPath("spec"),
	)
	if len(errs) != 0 {
		t.Fatalf("expected dev mode to skip min memory validation, got: %v", errs)
	}
}
