package v4

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sidbWebhookValidBaseSpec() *SingleInstanceDatabase {
	return &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-test", Namespace: "default"},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Edition:  "enterprise",
			Image:    SingleInstanceDatabaseImage{},
		},
	}
}

func TestSIDBWebhookDefaultSetsDataguardModePreview(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()

	if err := (&SingleInstanceDatabase{}).Default(context.Background(), sidb); err != nil {
		t.Fatalf("expected default to succeed, got: %v", err)
	}
	if sidb.Spec.Dataguard == nil {
		t.Fatalf("expected dataguard spec to be defaulted")
	}
	if sidb.Spec.Dataguard.Mode != DataguardProducerModePreview {
		t.Fatalf("expected dataguard mode %q, got %q", DataguardProducerModePreview, sidb.Spec.Dataguard.Mode)
	}
}

func TestSIDBWebhookRejectsManagedDataguardMode(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Dataguard = &DataguardProducerSpec{Mode: DataguardProducerModeManaged}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for managed dataguard mode")
	}
}

func TestSIDBWebhookRejectsTrueCacheWithoutAdminSecret(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache admin secret is missing")
	}
}

func TestSIDBWebhookAllowsTrueCacheAdminSecretAlongsideDBCredentialsWallet(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected truecache admin secret plus dbCredentialsWallet to validate, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsTrueCacheWithoutDBCredentialsWallet(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected truecache validation to allow missing dbCredentialsWallet when admin secret is set, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsTrueCacheAutoRegistrationWithDBCredentialsWallet(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		AutoTCServiceRegistration: true,
		TrueCacheServices:         []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected wallet-backed truecache auto registration spec to validate, got: %v", errs)
	}
}

func TestSIDBWebhookDefaultDoesNotSynthesizeLegacyAdminPasswordForTrueCache(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		AutoTCServiceRegistration: true,
		TrueCacheServices:         []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if err := (&SingleInstanceDatabase{}).Default(context.Background(), sidb); err != nil {
		t.Fatalf("expected default to succeed, got: %v", err)
	}
	if sidbHasLegacyAdminPasswordSpec(sidb) {
		t.Fatalf("expected defaulting to leave legacy adminPassword unset for truecache, got %#v", sidb.Spec.AdminPassword)
	}
	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected defaulted wallet-backed truecache spec to validate, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheIgnoresDefaultedLegacyAdminPasswordSecretKey(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary.example.com:1521/ORCLPRD"}
	sidb.Spec.AdminPassword.SecretKey = "oracle_pwd"
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		AutoTCServiceRegistration: true,
		TrueCacheServices:         []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if sidbHasLegacyAdminPasswordSpec(sidb) {
		t.Fatalf("expected secretKey-only adminPassword stub to be ignored for truecache, got %#v", sidb.Spec.AdminPassword)
	}
	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected truecache validation to ignore secretKey-only adminPassword stub, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsDataguardPrereqsOverrides(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Dataguard = &DataguardProducerSpec{
		Mode: DataguardProducerModePreview,
		Prereqs: &DataguardPrereqsSpec{
			Enabled:         true,
			BrokerConfigDir: "/opt/oracle/oradata/dbconfig/ORCLCDB",
			StandbyRedoSize: "512M",
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for dataguard prereqs overrides, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsRelativeDataguardPrereqsBrokerConfigDir(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Dataguard = &DataguardProducerSpec{
		Prereqs: &DataguardPrereqsSpec{
			Enabled:         true,
			BrokerConfigDir: "relative/path",
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for relative dataguard prereqs brokerConfigDir")
	}
}

func TestSIDBWebhookRejectsUnsafeTNSAliasName(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.TNSAliases = []SingleInstanceDatabaseTNSAlias{
		{
			Name:        "APP'; touch /tmp/pwned; #",
			Host:        "db.example.com",
			Port:        1521,
			ServiceName: "ORCLPDB1",
			Protocol:    SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
	}

	errs := validateSingleInstanceDatabaseSpec(sidb)
	if len(errs) == 0 {
		t.Fatalf("expected validation error for unsafe TNS alias name")
	}
	if !strings.Contains(errs.ToAggregate().Error(), "tnsAliases") {
		t.Fatalf("expected TNS alias validation error, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsShellSafeTNSAliasName(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.TNSAliases = []SingleInstanceDatabaseTNSAlias{
		{
			Name:        "app_alias-1.prod",
			Host:        "db.example.com",
			Port:        1521,
			ServiceName: "ORCLPDB1",
			Protocol:    SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected shell-safe TNS alias name to validate, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsDataguardStandbySourcesOnPrimary(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Dataguard = &DataguardProducerSpec{
		StandbySources: []DataguardStandbySourceSpec{
			{
				DBUniqueName: "STBYDB",
				Host:         "sidb-standby.shns.svc.cluster.local",
				TCPSEnabled:  true,
				TCPPort:      1521,
			},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for dataguard standbySources on primary, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsDataguardStandbySourcesOnStandby(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{DatabaseRef: "sidb-primary"}
	sidb.Spec.Dataguard = &DataguardProducerSpec{
		StandbySources: []DataguardStandbySourceSpec{
			{
				DBUniqueName: "STBYDB",
				Host:         "sidb-standby.shns.svc.cluster.local",
			},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for dataguard standbySources on standby")
	}
}

func TestSIDBWebhookRejectsClientWalletSecretWhenTCPSDisabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		TCPS: &SingleInstanceDatabaseSecurityTCPS{
			ClientWalletSecret: "dg-client-wallet",
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when clientWalletSecret is set but TCPS is disabled")
	}
}

func TestSIDBWebhookAllowsEndpointNodePortConfig(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name:                  SingleInstanceDatabaseServiceEndpointNameNodePort,
			Type:                  SingleInstanceDatabaseServiceEndpointTypeNodePort,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			Annotations: map[string]string{
				"service.beta.kubernetes.io/oci-load-balancer-internal": "true",
			},
			TCP: &SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for services.endpoints nodeport config, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsEndpointClusterIPConfig(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name: SingleInstanceDatabaseServiceEndpointNameCluster,
			Type: SingleInstanceDatabaseServiceEndpointTypeClusterIP,
			TCP: &SingleInstanceDatabaseServiceEndpointPort{
				Enabled: true,
				Port:    1521,
			},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for services.endpoints cluster config, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsEndpointTypeNameMismatch(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name: SingleInstanceDatabaseServiceEndpointNameCluster,
			Type: SingleInstanceDatabaseServiceEndpointTypeNodePort,
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when endpoint type does not match name")
	}
}

func TestSIDBWebhookRejectsExternalTrafficPolicyWhenClusterIP(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name:                  SingleInstanceDatabaseServiceEndpointNameCluster,
			Type:                  SingleInstanceDatabaseServiceEndpointTypeClusterIP,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
			TCP:                   &SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when externalTrafficPolicy is set for cluster endpoint")
	}
}

func TestSIDBWebhookRejectsClusterIPNodePortOverride(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name: SingleInstanceDatabaseServiceEndpointNameCluster,
			Type: SingleInstanceDatabaseServiceEndpointTypeClusterIP,
			TCP: &SingleInstanceDatabaseServiceEndpointPort{
				Enabled:  true,
				NodePort: 32001,
			},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when nodePort is set for cluster endpoint")
	}
}

func TestSIDBWebhookRejectsExternalTCPSWithoutDatabaseTCPS(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name: SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
			Type: SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
			TCPS: &SingleInstanceDatabaseServiceEndpointPort{
				Enabled: true,
				Port:    2484,
			},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when tcps service is enabled without tcps database config")
	}
}

func TestSIDBDeprecatedFieldWarnings(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.LoadBalancer = true
	sidb.Spec.ListenerPort = 32001
	sidb.Spec.TcpsListenerPort = 32002
	sidb.Spec.ServiceAnnotations = map[string]string{
		"service.beta.kubernetes.io/oci-load-balancer-internal": "true",
	}
	sidb.Spec.EnableTCPS = true
	sidb.Spec.TcpsCertRenewInterval = "48h"
	sidb.Spec.TcpsTlsSecret = "legacy-tls"
	sidb.Spec.AdminPassword = SingleInstanceDatabaseAdminPassword{
		SecretName: "sidb-admin",
	}
	sidb.Spec.Persistence.Size = "100Gi"
	sidb.Spec.Persistence.StorageClass = "oci-bv"
	sidb.Spec.Persistence.AccessMode = "ReadWriteOnce"
	sidb.Spec.Persistence.ScriptsVolumeName = "legacy-scripts-pv"
	validPolicy := corev1.PullAlways
	sidb.Spec.Image.PullPolicy = &validPolicy
	sidb.Spec.ResourceRequirements = &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("1"),
		},
	}

	warnings := sidbDeprecatedFieldWarnings(sidb)
	expectedWarnings := []string{
		"spec.loadBalancer is deprecated; use spec.services.endpoints",
		"spec.listenerPort is deprecated; use spec.services.endpoints.tcp",
		"spec.tcpsListenerPort is deprecated; use spec.services.endpoints.tcps",
		"spec.serviceAnnotations is deprecated; use spec.services.endpoints.annotations",
		"spec.enableTCPS is deprecated; use spec.security.tcps.enabled",
		"spec.tcpsCertRenewInterval is deprecated; certificate renewal is managed by the TLS Secret owner",
		"spec.tcpsTlsSecret is deprecated; use spec.security.tcps.tlsSecret",
		"spec.adminPassword is deprecated; use spec.security.secrets.admin",
		"spec.persistence.size is deprecated; use spec.persistence.oradata.size",
		"spec.persistence.storageClass is deprecated; use spec.persistence.oradata.storageClass",
		"spec.persistence.accessMode is deprecated; use spec.persistence.oradata.accessMode",
		"spec.persistence.scriptsVolumeName is deprecated; use spec.scripts.setup/spec.scripts.startup pvcName references",
		"spec.image.pullPolicy is deprecated; use spec.image.imagePullPolicy",
		"spec.resourceRequirements is deprecated; use spec.resources",
	}
	if len(warnings) != len(expectedWarnings) {
		t.Fatalf("expected %d deprecation warnings, got %#v", len(expectedWarnings), warnings)
	}
	for _, expected := range expectedWarnings {
		found := false
		for _, warning := range warnings {
			if warning == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected warning %q, got %#v", expected, warnings)
		}
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "primaryDatabaseRef") {
			t.Fatalf("did not expect warning for spec.primaryDatabaseRef, got %#v", warnings)
		}
	}
}

func TestSIDBWebhookDefaultTrimsScriptsPVCNames(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Scripts = &SingleInstanceDatabaseScriptsSpec{
		Setup:   &SingleInstanceDatabaseScriptLocation{PvcName: "  setup-pvc  "},
		Startup: &SingleInstanceDatabaseScriptLocation{PvcName: "  startup-pvc  "},
	}

	if err := (&SingleInstanceDatabase{}).Default(context.Background(), sidb); err != nil {
		t.Fatalf("expected default to succeed, got: %v", err)
	}
	if sidb.Spec.Scripts.Setup == nil || sidb.Spec.Scripts.Setup.PvcName != "setup-pvc" {
		t.Fatalf("expected setup pvc name to be trimmed, got %#v", sidb.Spec.Scripts.Setup)
	}
	if sidb.Spec.Scripts.Startup == nil || sidb.Spec.Scripts.Startup.PvcName != "startup-pvc" {
		t.Fatalf("expected startup pvc name to be trimmed, got %#v", sidb.Spec.Scripts.Startup)
	}
}

func TestSIDBWebhookValidateUpdateAllowsDeleteWithStatusDrift(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "truecache",
			Namespace:  "default",
			Finalizers: []string{"database.oracle.com/singleinstancedatabasefinalizer"},
		},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Edition:  "enterprise",
			Image: SingleInstanceDatabaseImage{
				PullFrom: "example.com/db:latest",
			},
			InitParams: &SingleInstanceDatabaseInitParams{
				SgaTarget:          7376,
				PgaAggregateTarget: 2458,
				CpuCount:           2,
				Processes:          360,
			},
			PrimarySource: &SingleInstanceDatabasePrimarySource{ConnectString: "10.0.2.7:1521/ORCLPRD"},
		},
		Status: SingleInstanceDatabaseStatus{
			Role:      "TRUE_CACHE",
			CreatedAs: "truecache",
			InitParams: SingleInstanceDatabaseInitParams{
				SgaTarget:          0,
				PgaAggregateTarget: 0,
				CpuCount:           0,
				Processes:          0,
			},
		},
	}

	newObj := oldObj.DeepCopy()
	newObj.DeletionTimestamp = &now
	newObj.Finalizers = nil
	newObj.Status.Status = "Deleting"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected delete-time update with unchanged spec to pass, got: %v", err)
	}
}

func TestSIDBWebhookRejectsMixedLegacyAndExplicitScripts(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Persistence.ScriptsVolumeName = "legacy-scripts-pv"
	sidb.Spec.Scripts = &SingleInstanceDatabaseScriptsSpec{
		Setup: &SingleInstanceDatabaseScriptLocation{PvcName: "setup-pvc"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for mixed legacy and explicit scripts config")
	}
}

func TestSIDBWebhookRejectsEmptyScriptsSpec(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Scripts = &SingleInstanceDatabaseScriptsSpec{}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when spec.scripts has no pvcName entries")
	}
}

func TestSIDBWebhookAllowsExplicitSetupOrStartupScripts(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Scripts = &SingleInstanceDatabaseScriptsSpec{
		Startup: &SingleInstanceDatabaseScriptLocation{PvcName: "startup-pvc"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected explicit startup scripts config to be accepted, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsReplicasGreaterThanOneWithoutPVCName(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Replicas = 2
	sidb.Spec.Persistence.Oradata = &SingleInstanceDatabasePersistenceOradata{
		Size:         "100Gi",
		StorageClass: "oci-bv",
		AccessMode:   "ReadWriteOnce",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for replicas > 1 without pvcName")
	}
}

func TestSIDBWebhookRejectsReplicasGreaterThanOneWithPVCName(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Replicas = 2
	sidb.Spec.Persistence.Oradata = &SingleInstanceDatabasePersistenceOradata{
		PvcName: "existing-oradata-pvc",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for replicas > 1 with pvcName")
	}
}

func TestSIDBResourcesBackwardCompatibleWithReleasedWireShape(t *testing.T) {
	raw := []byte(`{
		"spec": {
			"createAs": "primary",
			"edition": "enterprise",
			"image": {},
			"resources": {
				"requests": {
					"cpu": "1",
					"memory": "2Gi"
				},
				"limits": {
					"cpu": "2",
					"memory": "4Gi"
				}
			}
		}
	}`)

	var sidb SingleInstanceDatabase
	if err := json.Unmarshal(raw, &sidb); err != nil {
		t.Fatalf("expected released resources payload to unmarshal, got: %v", err)
	}
	if sidb.Spec.Resources == nil {
		t.Fatalf("expected resources to be populated from released wire shape")
	}
	if got := sidb.Spec.Resources.Requests.Cpu(); got == nil || !got.Equal(resource.MustParse("1")) {
		t.Fatalf("expected cpu request 1, got: %v", got)
	}
	if got := sidb.Spec.Resources.Requests.Memory(); got == nil || !got.Equal(resource.MustParse("2Gi")) {
		t.Fatalf("expected memory request 2Gi, got: %v", got)
	}
	if got := sidb.Spec.Resources.Limits.Cpu(); got == nil || !got.Equal(resource.MustParse("2")) {
		t.Fatalf("expected cpu limit 2, got: %v", got)
	}
	if got := sidb.Spec.Resources.Limits.Memory(); got == nil || !got.Equal(resource.MustParse("4Gi")) {
		t.Fatalf("expected memory limit 4Gi, got: %v", got)
	}
}

func TestSIDBResourceRequirementsDeprecatedWireShape(t *testing.T) {
	raw := []byte(`{
		"spec": {
			"createAs": "primary",
			"edition": "enterprise",
			"image": {},
			"resourceRequirements": {
				"requests": {
					"cpu": "1",
					"memory": "2Gi"
				},
				"limits": {
					"cpu": "2",
					"memory": "4Gi"
				}
			}
		}
	}`)

	var sidb SingleInstanceDatabase
	if err := json.Unmarshal(raw, &sidb); err != nil {
		t.Fatalf("expected deprecated resourceRequirements payload to unmarshal, got: %v", err)
	}
	if sidb.Spec.ResourceRequirements == nil {
		t.Fatalf("expected resourceRequirements to be populated from deprecated wire shape")
	}
	if got := sidb.Spec.ResourceRequirements.Requests.Cpu(); got == nil || !got.Equal(resource.MustParse("1")) {
		t.Fatalf("expected cpu request 1, got: %v", got)
	}
	if got := sidb.Spec.ResourceRequirements.Requests.Memory(); got == nil || !got.Equal(resource.MustParse("2Gi")) {
		t.Fatalf("expected memory request 2Gi, got: %v", got)
	}
	if got := sidb.Spec.ResourceRequirements.Limits.Cpu(); got == nil || !got.Equal(resource.MustParse("2")) {
		t.Fatalf("expected cpu limit 2, got: %v", got)
	}
	if got := sidb.Spec.ResourceRequirements.Limits.Memory(); got == nil || !got.Equal(resource.MustParse("4Gi")) {
		t.Fatalf("expected memory limit 4Gi, got: %v", got)
	}
	if warnings := sidbDeprecatedFieldWarnings(&sidb); !containsAdmissionWarning(warnings, "spec.resourceRequirements is deprecated; use spec.resources") {
		t.Fatalf("expected deprecated resourceRequirements warning, got %#v", warnings)
	}
}

func containsAdmissionWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func TestSIDBWebhookAllowsLegacyTcpsListenerPortOutsideNodePortRange(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		TCPS: &SingleInstanceDatabaseSecurityTCPS{Enabled: true},
	}
	sidb.Spec.TcpsListenerPort = 2484
	sidb.Spec.TcpsTlsSecret = "legacy-tls"

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected legacy tcpsListenerPort 2484 to be accepted, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsExternalTCPSWhenLegacyListenerPortImpliesEnablement(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.TcpsListenerPort = 2484
	sidb.Spec.TcpsTlsSecret = "legacy-tls"
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		Endpoints: []SingleInstanceDatabaseServiceEndpoint{{
			Name: SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
			Type: SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
			TCPS: &SingleInstanceDatabaseServiceEndpointPort{
				Enabled: true,
				Port:    2484,
			},
		}},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected legacy spec.tcpsListenerPort to satisfy tcps enablement, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsTCPSEnabledWithoutTLSSecret(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{TCPS: &SingleInstanceDatabaseSecurityTCPS{Enabled: true}}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatal("expected validation error when TCPS is enabled without tlsSecret")
	}
}

func TestSIDBWebhookAllowsTLSSecretOnlyWhenTCPSIsEnabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{TCPS: &SingleInstanceDatabaseSecurityTCPS{TlsSecret: "sidb-tls"}}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatal("expected validation error when tlsSecret is set but TCPS is disabled")
	}
}

func TestSIDBWebhookValidateUpdateAllowsExistingSelfSignedTCPSUntilMigrated(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Security = &SingleInstanceDatabaseSecurity{TCPS: &SingleInstanceDatabaseSecurityTCPS{Enabled: true, CertRenewInterval: "48h"}}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Services = &SingleInstanceDatabaseServices{}

	if _, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj); err != nil {
		t.Fatalf("expected existing self-signed TCPS resource to remain updateable until migrated, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsRemovingTLSSecret(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Security = &SingleInstanceDatabaseSecurity{TCPS: &SingleInstanceDatabaseSecurityTCPS{Enabled: true, TlsSecret: "sidb-tls"}}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Security.TCPS.TlsSecret = ""

	if _, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj); err == nil {
		t.Fatal("expected update removing tlsSecret while TCPS remains enabled to be rejected")
	}
}

func TestSIDBWebhookValidateUpdateRejectsSpecChangeWhenLockedWithoutOverride(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Sid = "NEWSID"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected validate update to fail for locked spec change")
	}
	if !strings.Contains(err.Error(), "spec updates are blocked while controller operation is in progress") {
		t.Fatalf("expected lock rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsSpecChangeWhenLockedWithOverride(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Sid = "NEWSID"
	newObj.SetAnnotations(map[string]string{lockpolicy.DefaultOverrideAnnotation: "true"})

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected validate update to pass with override, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsMetadataOnlyChangeWhenLocked(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 3},
		Spec:       SingleInstanceDatabaseSpec{Sid: "ORCLCDB"},
		Status: SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 2,
				Message:            "controller operation in progress",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.SetAnnotations(map[string]string{"team": "db"})

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected metadata-only update to pass while locked, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsFinalizerRemovalDuringDeleteForLegacySpec(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sidb-delete",
			Namespace:         "ns1",
			DeletionTimestamp: &now,
			Finalizers:        []string{"database.oracle.com/singleinstancedatabasefinalizer"},
		},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Edition:  "enterprise",
			Image:    SingleInstanceDatabaseImage{},
			Persistence: SingleInstanceDatabasePersistence{
				Oradata: &SingleInstanceDatabasePersistenceOradata{
					Size: "10Gi",
				},
			},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Finalizers = nil

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected finalizer removal during delete to pass, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsStandbyPrimarySourceChangeAfterDatafilesCreated(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			PrimarySource: &SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-a",
			},
		},
		Status: SingleInstanceDatabaseStatus{
			CreatedAs:        "standby",
			DatafilesCreated: "true",
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{DatabaseRef: "primary-b"}

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected standby primary source update to be rejected")
	}
	if !strings.Contains(err.Error(), "primary source of a standby database cannot be changed") {
		t.Fatalf("expected standby lock rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsTrueCachePrimarySourceChangeAfterBlobReady(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-tc", Namespace: "ns1"},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			PrimarySource: &SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-a",
			},
		},
		Status: SingleInstanceDatabaseStatus{
			CreatedAs: "truecache",
			Conditions: []metav1.Condition{{
				Type:   "TrueCacheBlobSourceReady",
				Status: metav1.ConditionTrue,
				Reason: "BlobConfigMapReady",
			}},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{ConnectString: "primary-b:1521/PRIM"}

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected truecache primary source update to be rejected")
	}
	if !strings.Contains(err.Error(), "primary source of a truecache database cannot be changed") {
		t.Fatalf("expected truecache lock rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsPrimarySourceChangeWhenDataguardTopologyLocked(t *testing.T) {
	oldObj := &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			PrimarySource: &SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-a",
			},
		},
		Status: SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
			Dataguard: &ProducerDataguardStatus{
				TopologyLocked: true,
			},
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{DatabaseRef: "primary-b"}

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected primary source update to be rejected when dataguard topology is locked")
	}
	if !strings.Contains(err.Error(), "dataguard topology is locked") {
		t.Fatalf("expected dataguard lock rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateAllowsAddingFra(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		Size:             "100Gi",
		StorageClass:     "oci-bv",
		AccessMode:       "ReadWriteOnce",
		RecoveryAreaSize: "100Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected adding FRA to be allowed, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsRemovingFra(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		PvcName:          "fra-pvc",
		RecoveryAreaSize: "100Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra = nil

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected removing FRA to be rejected")
	}
	if !strings.Contains(err.Error(), "cannot be removed once configured") {
		t.Fatalf("expected FRA removal rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsChangingFraPVCName(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		PvcName:          "fra-pvc-a",
		RecoveryAreaSize: "100Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra.PvcName = "fra-pvc-b"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected changing FRA pvcName to be rejected")
	}
}

func TestSIDBWebhookValidateUpdateAllowsManagedFraExpansion(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		Size:             "100Gi",
		StorageClass:     "oci-bv",
		AccessMode:       "ReadWriteOnce",
		RecoveryAreaSize: "80Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra.Size = "120Gi"
	newObj.Spec.Persistence.Fra.RecoveryAreaSize = "90Gi"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected managed FRA expansion to be allowed, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsManagedFraShrink(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		Size:             "120Gi",
		StorageClass:     "oci-bv",
		AccessMode:       "ReadWriteOnce",
		RecoveryAreaSize: "100Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra.Size = "100Gi"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected managed FRA shrink to be rejected")
	}
	if !strings.Contains(err.Error(), "cannot be decreased once managed FRA is configured") {
		t.Fatalf("expected FRA shrink rejection message, got: %v", err)
	}
}

func TestSIDBWebhookValidateUpdateRejectsChangingFraMountPath(t *testing.T) {
	oldObj := sidbWebhookValidBaseSpec()
	oldObj.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		PvcName:          "fra-pvc",
		RecoveryAreaSize: "100Gi",
		MountPath:        "/opt/oracle/oradata/fast_recovery_area",
	}
	newObj := oldObj.DeepCopy()
	newObj.Spec.Persistence.Fra.MountPath = "/u02/fra"

	_, err := (&SingleInstanceDatabase{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected changing FRA mountPath to be rejected")
	}
}

func TestSIDBWebhookRejectsInvalidFraRecoveryAreaSizeLiteral(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Persistence.Fra = &SingleInstanceDatabasePersistenceFra{
		PvcName:          "fra-pvc",
		RecoveryAreaSize: "50G scope=both sid='*'; alter system set open_cursors=9999 scope=spfile; --",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for invalid FRA recoveryAreaSize literal")
	}
}

func TestSIDBWebhookTrueCachePrimaryWithoutTrueCacheFieldsPasses(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = nil
	sidb.Spec.TrueCacheServices = nil

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when primary has no trueCache fields, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsGenerateEnabledOnly(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateEnabled: true,
		GeneratePath:    "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsGenerateBlobOnly(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateBlob: true,
		GeneratePath: "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for generateBlob=true, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsCreateConfigMapOnly(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		CreateConfigMap: true,
		GeneratePath:    "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for createConfigMap=true, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsGeneratePathWhenEnabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateBlob: true,
		GeneratePath: "/opt/oracle/truecache/blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when primary enables trueCache generation with a path, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsTmpGeneratePath(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		CreateConfigMap: true,
		GeneratePath:    "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when primary sets createConfigMap=true with /tmp trueCache generatePath, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsGeneratePathWhenDisabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateEnabled: false,
		GeneratePath:    "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets generatePath without generateEnabled=true")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsBlobConfigMapRef(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets trueCache.blobConfigMapRef")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsBlobConfigMapKey(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapKey: "tc-config",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets trueCache.blobConfigMapKey")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsBlobMountPath(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobMountPath: "/mnt/truecache",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets trueCache.blobMountPath")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsNestedTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		TrueCacheServices: []string{"svc1"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets trueCache.trueCacheServices")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsLegacyTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCacheServices = []string{"svc1"}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets legacy trueCacheServices")
	}
}

func TestSIDBWebhookTrueCachePrimaryRejectsConsumerFields(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for blobConfigMapRef on primary")
	}
}

func TestSIDBWebhookTrueCacheModeRejectsGenerateFields(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateBlob: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for generateBlob on truecache")
	}
}

func TestSIDBWebhookTrueCacheModeAllowsBlobConfigMapRef(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets blobConfigMapRef, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeAllowsConsumerBlobFields(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
		BlobConfigMapKey: "tc-config",
		BlobMountPath:    "/mnt/truecache",
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets blobConfigMapRef, blobConfigMapKey, and blobMountPath, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeAllowsNestedTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"svc1", "svc2"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets nested trueCacheServices, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeAllowsAutoTCServiceRegistration(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		AutoTCServiceRegistration: true,
		TrueCacheServices:         []string{"svc1"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache enables autoTCServiceRegistration with trueCacheServices, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeAllowsLegacyTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
	}
	sidb.Spec.TrueCacheServices = []string{"svc1", "svc2"}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets legacy trueCacheServices, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeRejectsAutoTCServiceRegistrationWithoutServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		AutoTCServiceRegistration: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache enables autoTCServiceRegistration without trueCacheServices")
	}
}

func TestSIDBWebhookTrueCacheModeRejectsGenerateEnabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateEnabled: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache sets generateEnabled=true")
	}
}

func TestSIDBWebhookTrueCacheModeRejectsCreateConfigMap(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		CreateConfigMap: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache sets createConfigMap=true")
	}
}

func TestSIDBWebhookTrueCacheModeRejectsGeneratePath(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GeneratePath: "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache sets generatePath")
	}
}

func TestSIDBWebhookStandbyWithoutTrueCacheFieldsPasses(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = nil
	sidb.Spec.TrueCacheServices = nil

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when standby has no trueCache fields, got: %v", errs)
	}
}

func TestSIDBWebhookStandbyRejectsNestedTrueCacheField(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when standby sets a nested trueCache field")
	}
}

func TestSIDBWebhookPrimaryRejectsAutoTCServiceRegistration(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		AutoTCServiceRegistration: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary sets autoTCServiceRegistration")
	}
}

func TestSIDBWebhookStandbyRejectsLegacyTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCacheServices = []string{"svc1"}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when standby sets legacy trueCacheServices")
	}
}

func TestSIDBWebhookStandbyRejectsTrueCacheSpec(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for trueCache on standby")
	}
}

func TestSIDBWebhookCloneWithoutTrueCacheFieldsPasses(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "clone"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = nil
	sidb.Spec.TrueCacheServices = nil

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when clone has no trueCache fields, got: %v", errs)
	}
}

func TestSIDBWebhookCloneRejectsNestedTrueCacheField(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "clone"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when clone sets a nested trueCache field")
	}
}

func TestSIDBWebhookCloneRejectsLegacyTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "clone"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCacheServices = []string{"svc1"}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when clone sets legacy trueCacheServices")
	}
}

func TestSIDBWebhookPrimarySourceRejectsMixedFields(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef:   "primary-db",
		ConnectString: "primary-host:1521/PRIM",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primarySource mixes mutually exclusive fields")
	}
}

func TestSIDBWebhookPrimarySourceAllowsConnectStringWithPdbName(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.Edition = "enterprise"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		ConnectString: "primary-host:1521/PRIM",
		Pdbname:       "PRIMPDB",
	}
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		Secrets: &SingleInstanceDatabaseSecrets{
			Admin: &SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		DBCredentialsWallet: &SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
			SecretName: "primary-db-cred-wallet",
		},
		TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation error when primarySource sets connectString with pdbName, got: %v", errs)
	}
}

func TestSIDBWebhookPrimarySourceRejectsPdbNameWithoutConnectString(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.Edition = "enterprise"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		Pdbname: "PRIMPDB",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primarySource.pdbName is set without connectString")
	}
}

func TestSIDBWebhookPrimarySourceRejectsDeprecatedPrimaryDatabaseRefMix(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.PrimaryDatabaseRef = "legacy-primary-db"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when deprecated primaryDatabaseRef is mixed with primarySource")
	}
}

func TestSIDBWebhookTrueCacheRequiresPrimarySource(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when truecache omits primary source")
	}
}

func TestSIDBWebhookTrueCacheRejectsNonEnterpriseEdition(t *testing.T) {
	testCases := []string{"free", "standard", "express", ""}

	for _, edition := range testCases {
		t.Run(edition, func(t *testing.T) {
			sidb := sidbWebhookValidBaseSpec()
			sidb.Spec.CreateAs = "truecache"
			sidb.Spec.Edition = edition
			sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			}

			if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
				t.Fatalf("expected validation error when truecache uses edition %q", edition)
			}
		})
	}
}

func TestSIDBWebhookPrimaryRejectsPrimarySource(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when primary uses primarySource")
	}
}

func TestSIDBWebhookRestoreObjectStoreRequiresDBID(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		ObjectStore: &SingleInstanceDatabaseRestoreObjectStoreSpec{
			OCIConfig:       &SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
			PrivateKey:      &SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
			OpcInstallerZip: &SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociinstaller", Key: "opc_installer.zip"},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when restore.objectStore.backupIdentity.dbid is missing")
	}
}

func TestSIDBWebhookRestoreObjectStoreRequiresOpcInstallerZipOrEnv(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		ObjectStore: &SingleInstanceDatabaseRestoreObjectStoreSpec{
			OCIConfig:  &SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
			PrivateKey: &SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
			BackupIdentity: &SingleInstanceDatabaseBackupIdentity{
				DBID: "1234567890",
			},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when restore.objectStore.opcInstallerZip is missing")
	}
}

func TestSIDBWebhookRestoreObjectStoreAllowsOpcInstallerZipEnvOverride(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		ObjectStore: &SingleInstanceDatabaseRestoreObjectStoreSpec{
			OCIConfig:  &SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
			PrivateKey: &SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
			BackupIdentity: &SingleInstanceDatabaseBackupIdentity{
				DBID: "1234567890",
			},
		},
	}
	sidb.Spec.EnvVars = []corev1.EnvVar{{Name: "OPC_INSTALL_ZIP", Value: "/opt/oracle/oci/opc/oci_installer.zip"}}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors with OPC_INSTALL_ZIP env override, got: %v", errs)
	}
}

func TestSIDBWebhookRestoreFileSystemRequiresDBIDEnvVar(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		FileSystem: &SingleInstanceDatabaseRestoreFileSystemSpec{
			BackupPath: "/mnt/backup",
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when DBID env var is missing for restore.fileSystem")
	}
}

func TestSIDBWebhookRestoreFileSystemWithDBIDEnvVarPasses(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		FileSystem: &SingleInstanceDatabaseRestoreFileSystemSpec{
			BackupPath: "/mnt/backup",
		},
	}
	sidb.Spec.EnvVars = []corev1.EnvVar{{Name: "DBID", Value: "1234567890"}}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsInvalidPullPolicy(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	invalid := corev1.PullPolicy("Sometimes")
	sidb.Spec.Image.PullFrom = "example.com/repo/image:tag"
	sidb.Spec.Image.ImagePullPolicy = &invalid

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for invalid imagePullPolicy")
	}
}

func TestSIDBWebhookAcceptsValidPullPolicy(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	valid := corev1.PullAlways
	sidb.Spec.Image.PullFrom = "example.com/repo/image:tag"
	sidb.Spec.Image.ImagePullPolicy = &valid

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for valid imagePullPolicy, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsConflictingImagePullPolicyFields(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Image.PullFrom = "example.com/repo/image:tag"
	std := corev1.PullAlways
	legacy := corev1.PullNever
	sidb.Spec.Image.ImagePullPolicy = &std
	sidb.Spec.Image.PullPolicy = &legacy

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for conflicting image pull policy fields")
	}
}

func TestResolveSIDBAdminSecretRefPrefersGroupedField(t *testing.T) {
	sidb := &SingleInstanceDatabase{
		Spec: SingleInstanceDatabaseSpec{
			AdminPassword: SingleInstanceDatabaseAdminPassword{
				SecretName: "legacy-admin",
				SecretKey:  "legacy-key",
			},
			Security: &SingleInstanceDatabaseSecurity{
				Secrets: &SingleInstanceDatabaseSecrets{
					Admin: &SingleInstanceDatabaseAdminPassword{
						SecretName: "grouped-admin",
						SecretKey:  "grouped-key",
					},
				},
			},
		},
	}

	secretName, secretKey, ok := ResolveSIDBAdminSecretRef(sidb)
	if !ok {
		t.Fatalf("expected grouped secret metadata to resolve")
	}
	if secretName != "grouped-admin" || secretKey != "grouped-key" {
		t.Fatalf("unexpected resolved grouped secret ref: %q/%q", secretName, secretKey)
	}
}

func TestResolveSIDBAdminSecretRefFallsBackToLegacyField(t *testing.T) {
	sidb := &SingleInstanceDatabase{
		Spec: SingleInstanceDatabaseSpec{
			AdminPassword: SingleInstanceDatabaseAdminPassword{
				SecretName: "legacy-admin",
			},
		},
	}

	secretName, secretKey, ok := ResolveSIDBAdminSecretRef(sidb)
	if !ok {
		t.Fatalf("expected legacy secret metadata to resolve")
	}
	if secretName != "legacy-admin" || secretKey != DefaultSIDBAdminSecretKey {
		t.Fatalf("unexpected resolved legacy secret ref: %q/%q", secretName, secretKey)
	}
}
