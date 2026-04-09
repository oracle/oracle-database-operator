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
		Spec:       SingleInstanceDatabaseSpec{CreateAs: "primary"},
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
