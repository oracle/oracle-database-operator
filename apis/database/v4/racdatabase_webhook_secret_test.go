package v4

import (
	"strings"
	"testing"
)

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

func TestValidateDbSecretRejectsPwdFileNameWithoutKeyFileName(t *testing.T) {
	t.Parallel()

	cr := &RacDatabase{
		Spec: RacDatabaseSpec{
			DbSecret: &RacDbPwdSecretDetails{
				Name:        "db-user-pass",
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
