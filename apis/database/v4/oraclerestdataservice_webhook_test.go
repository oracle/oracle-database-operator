package v4

import (
	"context"
	"strings"
	"testing"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
)

func TestOracleRestDataServiceDefaultLegacySecrets(t *testing.T) {
	ords := &OracleRestDataService{}
	ords.Spec.DatabaseRef = "sidb"
	ords.Spec.AdminPassword.SecretName = "db-admin-secret"
	ords.Spec.OrdsPassword.SecretName = "ords-secret"

	if err := (&OracleRestDataService{}).Default(context.Background(), ords); err != nil {
		t.Fatalf("Default() error = %v", err)
	}

	if ords.Spec.Replicas != 1 {
		t.Fatalf("expected replicas default 1, got %d", ords.Spec.Replicas)
	}
	if ords.Spec.HTTPPort != dbcommons.ORDSDefaultHTTPPort {
		t.Fatalf("expected httpPort default %d, got %d", dbcommons.ORDSDefaultHTTPPort, ords.Spec.HTTPPort)
	}
	if ords.Spec.AdminPassword.SecretKey != DefaultOracleRestDataServiceSecretKey {
		t.Fatalf("expected admin secretKey default %q, got %q", DefaultOracleRestDataServiceSecretKey, ords.Spec.AdminPassword.SecretKey)
	}
	if ords.Spec.OrdsPassword.SecretKey != DefaultOracleRestDataServiceSecretKey {
		t.Fatalf("expected ORDS secretKey default %q, got %q", DefaultOracleRestDataServiceSecretKey, ords.Spec.OrdsPassword.SecretKey)
	}
	if ords.Spec.AdminPassword.KeepSecret == nil || !*ords.Spec.AdminPassword.KeepSecret {
		t.Fatalf("expected admin keepSecret default true")
	}
	if ords.Spec.OrdsPassword.KeepSecret == nil || !*ords.Spec.OrdsPassword.KeepSecret {
		t.Fatalf("expected ORDS keepSecret default true")
	}
}

func TestResolveOracleRestDataServiceSecretsPrefersGroupedFields(t *testing.T) {
	ords := &OracleRestDataService{}
	ords.Spec.AdminPassword.SecretName = "legacy-admin"
	ords.Spec.OrdsPassword.SecretName = "legacy-ords"
	ords.Spec.Security = &OracleRestDataServiceSecurity{
		Secrets: &OracleRestDataServiceSecrets{
			DatabaseAdmin: &OracleRestDataServicePassword{
				SecretName: "grouped-admin",
				SecretKey:  "admin-key",
			},
			OrdsPublicUser: &OracleRestDataServicePassword{
				SecretName: "grouped-ords",
				SecretKey:  "ords-key",
			},
		},
	}

	adminName, adminKey, adminKeep, ok := ResolveOracleRestDataServiceAdminSecretRef(ords)
	if !ok || adminName != "grouped-admin" || adminKey != "admin-key" || !adminKeep {
		t.Fatalf("unexpected admin ref: name=%q key=%q keep=%v ok=%v", adminName, adminKey, adminKeep, ok)
	}

	ordsName, ordsKey, ordsKeep, ok := ResolveOracleRestDataServiceOrdsSecretRef(ords)
	if !ok || ordsName != "grouped-ords" || ordsKey != "ords-key" || !ordsKeep {
		t.Fatalf("unexpected ORDS ref: name=%q key=%q keep=%v ok=%v", ordsName, ordsKey, ordsKeep, ok)
	}
}

func TestOracleRestDataServiceValidateRequiresEffectiveSecrets(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	ords := &OracleRestDataService{}
	ords.Name = "ords-sample"
	ords.Namespace = "default"
	ords.Spec.DatabaseRef = "sidb-sample"
	ords.Spec.HTTPPort = dbcommons.ORDSDefaultHTTPPort
	ords.Spec.Image.PullFrom = "container-registry.oracle.com/database/ords-developer:latest"

	_, err := (&OracleRestDataService{}).ValidateCreate(context.Background(), ords)
	if err == nil {
		t.Fatalf("expected validation error for missing password secret references")
	}
}

func TestOracleRestDataServiceValidateCreateWarnsForLegacySecrets(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	ords := validOracleRestDataServiceForWebhookTest()
	ords.Spec.AdminPassword.SecretName = "legacy-admin"
	ords.Spec.OrdsPassword.SecretName = "legacy-ords"

	// Match admission order: mutate/default before validate.
	if err := (&OracleRestDataService{}).Default(context.Background(), ords); err != nil {
		t.Fatalf("Default() error = %v", err)
	}

	warnings, err := (&OracleRestDataService{}).ValidateCreate(context.Background(), ords)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	assertOracleRestDataServiceWarning(t, warnings, "spec.adminPassword is deprecated; use spec.security.secrets.databaseAdmin")
	assertOracleRestDataServiceWarning(t, warnings, "spec.ordsPassword is deprecated; use spec.security.secrets.ordsPublicUser")
}

func TestOracleRestDataServiceValidateCreateWarnsGroupedSecretsTakePrecedence(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	ords := validOracleRestDataServiceForWebhookTest()
	ords.Spec.AdminPassword.SecretName = "legacy-admin"
	ords.Spec.OrdsPassword.SecretName = "legacy-ords"
	ords.Spec.Security = &OracleRestDataServiceSecurity{
		Secrets: &OracleRestDataServiceSecrets{
			DatabaseAdmin: &OracleRestDataServicePassword{
				SecretName: "grouped-admin",
			},
			OrdsPublicUser: &OracleRestDataServicePassword{
				SecretName: "grouped-ords",
			},
		},
	}

	if err := (&OracleRestDataService{}).Default(context.Background(), ords); err != nil {
		t.Fatalf("Default() error = %v", err)
	}

	warnings, err := (&OracleRestDataService{}).ValidateCreate(context.Background(), ords)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	assertOracleRestDataServiceWarning(t, warnings, "spec.adminPassword is deprecated; use spec.security.secrets.databaseAdmin")
	assertOracleRestDataServiceWarning(t, warnings, "spec.security.secrets.databaseAdmin takes precedence over deprecated spec.adminPassword")
	assertOracleRestDataServiceWarning(t, warnings, "spec.ordsPassword is deprecated; use spec.security.secrets.ordsPublicUser")
	assertOracleRestDataServiceWarning(t, warnings, "spec.security.secrets.ordsPublicUser takes precedence over deprecated spec.ordsPassword")
}

func TestOracleRestDataServiceValidateUpdateReturnsWarningsWithErrors(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	oldOrds := validOracleRestDataServiceForWebhookTest()
	oldOrds.Status.DatabaseRef = "sidb-sample"

	newOrds := validOracleRestDataServiceForWebhookTest()
	newOrds.Spec.DatabaseRef = "other-sidb"
	newOrds.Spec.AdminPassword.SecretName = "legacy-admin"
	newOrds.Spec.OrdsPassword.SecretName = "legacy-ords"

	if err := (&OracleRestDataService{}).Default(context.Background(), newOrds); err != nil {
		t.Fatalf("Default() error = %v", err)
	}

	warnings, err := (&OracleRestDataService{}).ValidateUpdate(context.Background(), oldOrds, newOrds)
	if err == nil {
		t.Fatalf("expected immutable databaseRef validation error")
	}
	assertOracleRestDataServiceWarning(t, warnings, "spec.adminPassword is deprecated; use spec.security.secrets.databaseAdmin")
	assertOracleRestDataServiceWarning(t, warnings, "spec.ordsPassword is deprecated; use spec.security.secrets.ordsPublicUser")
}

func TestOracleRestDataServiceValidateRejectsInvalidHTTPPort(t *testing.T) {
	t.Setenv("WATCH_NAMESPACE", "")
	ords := validOracleRestDataServiceForWebhookTest()
	ords.Spec.HTTPPort = 70000

	_, err := (&OracleRestDataService{}).ValidateCreate(context.Background(), ords)
	if err == nil {
		t.Fatalf("expected validation error for invalid httpPort")
	}
	if !strings.Contains(err.Error(), "httpPort") {
		t.Fatalf("expected httpPort validation error, got %v", err)
	}
}

func validOracleRestDataServiceForWebhookTest() *OracleRestDataService {
	ords := &OracleRestDataService{}
	ords.Name = "ords-sample"
	ords.Namespace = "default"
	ords.Spec.DatabaseRef = "sidb-sample"
	ords.Spec.Replicas = 1
	ords.Spec.HTTPPort = dbcommons.ORDSDefaultHTTPPort
	ords.Spec.Image.PullFrom = "container-registry.oracle.com/database/ords-developer:latest"
	ords.Spec.AdminPassword.SecretName = "db-admin-secret"
	ords.Spec.OrdsPassword.SecretName = "ords-secret"
	return ords
}

func assertOracleRestDataServiceWarning(t *testing.T, warnings []string, expected string) {
	t.Helper()
	for _, warning := range warnings {
		if strings.Contains(warning, expected) {
			return
		}
	}
	t.Fatalf("expected warning %q in %v", expected, warnings)
}
