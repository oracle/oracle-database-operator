package v4

import (
	"strings"
	"testing"
)

func TestOracleRestartValidateUpdateTdeSecretRejectsRemovalWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &OracleRestart{}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret removal to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "cannot be removed") {
		t.Fatalf("expected removal error, got: %v", errs)
	}
}

func TestOracleRestartValidateUpdateTdeSecretRejectsNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
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

func TestOracleRestartValidateUpdateTdeSecretRejectsKeyFileNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
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

func TestOracleRestartValidateUpdateTdeSecretRejectsPwdFileNameChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
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

func TestOracleRestartValidateUpdateTdeSecretRejectsKeyChangeWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:      "tde-wallet",
				SecretKey: "tdekey",
			},
		},
	}
	newCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:      "tde-wallet",
				SecretKey: "other-tdekey",
			},
		},
	}

	errs := newCr.validateUpdateTdeSecret(oldCr)
	if len(errs) == 0 {
		t.Fatal("expected TDE secret key change to be rejected")
	}
	if !strings.Contains(errs[0].Error(), "key cannot be changed") {
		t.Fatalf("expected key immutability error, got: %v", errs)
	}
}

func TestOracleRestartValidateUpdateTdeSecretAllowsUnchangedSecretWithoutStatusSnapshot(t *testing.T) {
	t.Parallel()

	oldCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "tde-wallet",
				KeyFileName: "ewallet.p12",
				PwdFileName: "pwd.txt",
			},
		},
	}
	newCr := &OracleRestart{
		Spec: OracleRestartSpec{
			TdeWalletSecret: &OracleRestartDbPwdSecretDetails{
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

func TestOracleRestartValidateDbSecretAllowsKeyFileNameWithoutPwdFileNameForBase64(t *testing.T) {
	t.Parallel()

	cr := &OracleRestart{
		Spec: OracleRestartSpec{
			DbSecret: &OracleRestartDbPwdSecretDetails{
				Name:           "db-user-pass-pkutl",
				KeyFileName:    "key.pem",
				EncryptionType: "base64",
			},
		},
	}

	if errs := cr.validateDbSecret(); len(errs) != 0 {
		t.Fatalf("expected validation to pass, got: %v", errs)
	}
}

func TestOracleRestartValidateDbSecretAllowsSecretKeyWithoutPwdFileName(t *testing.T) {
	t.Parallel()

	cr := &OracleRestart{
		Spec: OracleRestartSpec{
			DbSecret: &OracleRestartDbPwdSecretDetails{
				Name:      "db-user-pass-pkutl",
				SecretKey: "key.pem",
			},
		},
	}

	if errs := cr.validateDbSecret(); len(errs) != 0 {
		t.Fatalf("expected validation to pass, got: %v", errs)
	}
}

func TestOracleRestartValidateDbSecretRejectsPwdFileNameWithoutKeyOrKeyFileName(t *testing.T) {
	t.Parallel()

	cr := &OracleRestart{
		Spec: OracleRestartSpec{
			DbSecret: &OracleRestartDbPwdSecretDetails{
				Name:        "db-user-pass-pkutl",
				PwdFileName: "pwdfile.enc",
			},
		},
	}

	errs := cr.validateDbSecret()
	if len(errs) == 0 {
		t.Fatal("expected validation error, got none")
	}
	if !strings.Contains(errs[0].Error(), "KeyFileName") {
		t.Fatalf("expected KeyFileName validation error, got: %v", errs)
	}
}
