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

func TestSIDBWebhookStandbyRejectsTrueCacheSpec(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.CreateAs = "standby"
	sidb.Spec.StandbyConfig = &SingleInstanceDatabaseStandbyConfig{
		PrimaryDatabaseRef: "primary-db",
	}
	sidb.Spec.TrueCache = &SingleInstanceDatabaseTrueCacheSpec{
		BlobConfigMapRef: "tc-blob",
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error for trueCache on standby")
	}
}

func TestSIDBWebhookRestoreObjectStoreRequiresDBID(t *testing.T) {
	sidb := sidbWebhookValidBaseSpec()
	sidb.Spec.Restore = &SingleInstanceDatabaseRestoreSpec{
		ObjectStore: &SingleInstanceDatabaseRestoreObjectStoreSpec{
			OCIConfig:  &SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
			PrivateKey: &SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
		},
	}

	if errs := validateSingleInstanceDatabaseSpec(sidb); len(errs) == 0 {
		t.Fatalf("expected validation error when restore.objectStore.backupIdentity.dbid is missing")
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
