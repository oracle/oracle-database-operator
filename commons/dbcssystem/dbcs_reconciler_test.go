package common

import (
	"strings"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/database"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestValidateDbcsAdminPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		password    string
		wantErrText string
	}{
		{
			name:     "accepts OCI compatible password",
			password: "WElcome_23ai_2026",
		},
		{
			name:        "rejects too few special characters",
			password:    "WElcome_23ai2026",
			wantErrText: "password must contain at least 2 uppercase, 2 lowercase, 2 numbers, and 2 special characters",
		},
		{
			name:        "rejects unsupported special character",
			password:    "WElcome!23ai_2026",
			wantErrText: "password contains unsupported character '!'",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := databasev4.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to register database api scheme: %v", err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to register core scheme: %v", err)
			}

			dbcs := &databasev4.DbcsSystem{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dbcssystem-create",
					Namespace: "default",
				},
				Spec: databasev4.DbcsSystemSpec{
					DbSystem: &databasev4.DbSystemDetails{
						DbAdminPasswordSecret: "admin-password",
					},
				},
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admin-password",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"admin-password": []byte(tc.password + "\n"),
				},
			}

			cl := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(dbcs, secret).
				Build()

			err := validateDbcsAdminPassword(cl, dbcs)
			if tc.wantErrText == "" {
				if err != nil {
					t.Fatalf("validateDbcsAdminPassword() unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validateDbcsAdminPassword() expected error containing %q", tc.wantErrText)
			}
			if !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("validateDbcsAdminPassword() error %q does not contain %q", err.Error(), tc.wantErrText)
			}
		})
	}
}

func TestRedactLaunchDbSystemDetailsForLogDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	adminPassword := "WElcome_23ai_2026"
	tdePassword := "TDe_23ai_2026"
	original := database.LaunchDbSystemDetails{
		DbHome: &database.CreateDbHomeDetails{
			Database: &database.CreateDatabaseDetails{
				AdminPassword:     &adminPassword,
				TdeWalletPassword: &tdePassword,
			},
		},
	}

	redacted := redactLaunchDbSystemDetailsForLog(original)

	if got := *original.DbHome.Database.AdminPassword; got != adminPassword {
		t.Fatalf("original admin password mutated: got %q want %q", got, adminPassword)
	}
	if got := *original.DbHome.Database.TdeWalletPassword; got != tdePassword {
		t.Fatalf("original TDE wallet password mutated: got %q want %q", got, tdePassword)
	}
	if got := *redacted.DbHome.Database.AdminPassword; got != "<redacted>" {
		t.Fatalf("redacted admin password mismatch: got %q", got)
	}
	if got := *redacted.DbHome.Database.TdeWalletPassword; got != "<redacted>" {
		t.Fatalf("redacted TDE wallet password mismatch: got %q", got)
	}
}
