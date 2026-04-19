package v4

import (
	"context"
	"strings"
	"testing"

	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sidbWebhookValidBaseSpec() *SingleInstanceDatabase {
	return &SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-test", Namespace: "default"},
		Spec: SingleInstanceDatabaseSpec{
			CreateAs: "primary",
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

func TestSIDBWebhookAllowsNewExternalNodePortConfig(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		External: &SingleInstanceDatabaseExternalService{
			Type: SingleInstanceDatabaseExternalServiceTypeNodePort,
			Annotations: map[string]string{
				"service.beta.kubernetes.io/oci-load-balancer-internal": "true",
			},
			TCP: &SingleInstanceDatabaseExternalServicePort{Enabled: true},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for services.external nodeport config, got: %v", errs)
	}
}

func TestSIDBWebhookRejectsExternalTCPSWithoutDatabaseTCPS(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		External: &SingleInstanceDatabaseExternalService{
			Type: SingleInstanceDatabaseExternalServiceTypeLoadBalancer,
			TCPS: &SingleInstanceDatabaseExternalServicePort{
				Enabled: true,
				Port:    2484,
			},
		},
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
	sidb.Spec.Resources = SingleInstanceDatabaseResources{
		Requests: &SingleInstanceDatabaseResource{Cpu: "1", Memory: "1Gi"},
	}
	sidb.Spec.Persistence.Size = "100Gi"
	sidb.Spec.Persistence.StorageClass = "oci-bv"
	sidb.Spec.Persistence.AccessMode = "ReadWriteOnce"

	warnings := sidbDeprecatedFieldWarnings(sidb)
	expectedWarnings := []string{
		"spec.loadBalancer is deprecated; use spec.services.external.type",
		"spec.listenerPort is deprecated; use spec.services.external.tcp",
		"spec.tcpsListenerPort is deprecated; use spec.services.external.tcps",
		"spec.serviceAnnotations is deprecated; use spec.services.external.annotations",
		"spec.enableTCPS is deprecated; use spec.security.tcps.enabled",
		"spec.tcpsCertRenewInterval is deprecated; use spec.security.tcps.certRenewInterval",
		"spec.tcpsTlsSecret is deprecated; use spec.security.tcps.tlsSecret",
		"spec.adminPassword is deprecated; use spec.security.secrets.admin",
		"spec.resources is deprecated; use spec.resourceRequirements",
		"spec.persistence.size is deprecated; use spec.persistence.oradata.size",
		"spec.persistence.storageClass is deprecated; use spec.persistence.oradata.storageClass",
		"spec.persistence.accessMode is deprecated; use spec.persistence.oradata.accessMode",
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

func TestSIDBWebhookAllowsLegacyTcpsListenerPortOutsideNodePortRange(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Security = &SingleInstanceDatabaseSecurity{
		TCPS: &SingleInstanceDatabaseSecurityTCPS{Enabled: true},
	}
	sidb.Spec.TcpsListenerPort = 2484

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected legacy tcpsListenerPort 2484 to be accepted, got: %v", errs)
	}
}

func TestSIDBWebhookAllowsExternalTCPSWhenLegacyListenerPortImpliesEnablement(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.TcpsListenerPort = 2484
	sidb.Spec.Services = &SingleInstanceDatabaseServices{
		External: &SingleInstanceDatabaseExternalService{
			Type: SingleInstanceDatabaseExternalServiceTypeLoadBalancer,
			TCPS: &SingleInstanceDatabaseExternalServicePort{
				Enabled: true,
				Port:    2484,
			},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected legacy spec.tcpsListenerPort to satisfy tcps enablement, got: %v", errs)
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

func TestSIDBWebhookTrueCachePrimaryAllowsGeneratePathWhenEnabled(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateEnabled: true,
		GeneratePath:    "/opt/oracle/truecache/blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when primary enables trueCache generation with a path, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCachePrimaryAllowsTmpGeneratePath(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "primary"
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		GenerateEnabled: true,
		GeneratePath:    "/tmp/tc_config_blob.tar.gz",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when primary sets generateEnabled=true with /tmp trueCache generatePath, got: %v", errs)
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
		GenerateEnabled: true,
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for generateEnabled on truecache")
	}
}

func TestSIDBWebhookTrueCacheModeAllowsBlobConfigMapRef(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
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
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
		BlobConfigMapKey: "tc-config",
		BlobMountPath:    "/mnt/truecache",
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
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		TrueCacheServices: []string{"svc1", "svc2"},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets nested trueCacheServices, got: %v", errs)
	}
}

func TestSIDBWebhookTrueCacheModeAllowsLegacyTrueCacheServices(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "truecache"
	sidb.Spec.PrimarySource = &SingleInstanceDatabasePrimarySource{
		DatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCacheServices = []string{"svc1", "svc2"}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors when truecache sets legacy trueCacheServices, got: %v", errs)
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
	sidb.Spec.Image.PullPolicy = &invalid

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for invalid pullPolicy")
	}
}

func TestSIDBWebhookAcceptsValidPullPolicy(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	valid := corev1.PullAlways
	sidb.Spec.Image.PullFrom = "example.com/repo/image:tag"
	sidb.Spec.Image.PullPolicy = &valid

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) != 0 {
		t.Fatalf("expected no validation errors for valid pullPolicy, got: %v", errs)
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
