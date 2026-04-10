//nolint:staticcheck // unit tests intentionally assert legacy requeue behavior.
package controllers

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSIDBUnit_GetPrimaryDatabaseConnectStringPrefersPrimarySource(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimaryDatabaseRef: "primary-db",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "custom-host:1521/CDB1",
			},
		},
	}
	got := GetPrimaryDatabaseConnectString(sidb, &dbapi.SingleInstanceDatabase{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}, Spec: dbapi.SingleInstanceDatabaseSpec{Sid: "IGN"}})
	if got != "custom-host:1521/CDB1" {
		t.Fatalf("expected primarySource connect string, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseConnectStringFromPrimaryDetails(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
					Host: "external-primary",
					Port: 1522,
					Sid:  "PRIM",
				},
			},
		},
	}
	got := GetPrimaryDatabaseConnectString(sidb, nil)
	if got != "external-primary:1522/PRIM" {
		t.Fatalf("expected connect string from primaryDetails, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseInfoFromConnectString(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary-host:1522/primdb",
			},
		},
	}

	if got := GetPrimaryDatabaseHost(sidb, nil); got != "primary-host" {
		t.Fatalf("expected primary host from connect string, got %q", got)
	}
	if got := GetPrimaryDatabasePort(sidb); got != 1522 {
		t.Fatalf("expected primary port from connect string, got %d", got)
	}
	if got := GetPrimaryDatabaseSid(sidb, nil); got != "PRIMDB" {
		t.Fatalf("expected primary sid from connect string, got %q", got)
	}
	if got := GetPrimaryDatabaseDisplayName(sidb, nil); got != "primary-host" {
		t.Fatalf("expected primary display name from connect string, got %q", got)
	}
}

func TestSIDBUnit_IsLocalPrimaryDatabaseSource(t *testing.T) {
	local := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
		},
	}
	if !isLocalPrimaryDatabaseSource(local) {
		t.Fatalf("expected databaseRef source to be treated as local")
	}

	connectString := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary-host:1521/PRIM",
			},
		},
	}
	if isLocalPrimaryDatabaseSource(connectString) {
		t.Fatalf("expected connect string source to be treated as external")
	}
}

func TestSIDBUnit_BuildAutomaticPrimaryTNSAliases(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseTCPS{
					Enabled: true,
				},
			},
		},
	}
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "primary-db"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
		},
	}

	aliases, names := buildAutomaticPrimaryTNSAliases(sidb, primary)
	expectedNames := []string{"ORCLCDB", "ORCLCDBTCPS", "ORCLCDBTCPS_DGMGRL", "ORCLCDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected generated alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["ORCLCDB"]; got.Host != "primary-db" || got.Port != 1521 || got.ServiceName != "ORCLCDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected base primary alias: %#v", got)
	}
	if got := aliases["ORCLCDB_DGMGRL"]; got.Host != "primary-db" || got.Port != 1521 || got.ServiceName != "ORCLCDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected dgmgrl alias: %#v", got)
	}
	if got := aliases["ORCLCDBTCPS"]; got.Host != "primary-db" || got.Port != 2484 || got.ServiceName != "ORCLCDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected tcps alias: %#v", got)
	}
	if got := aliases["ORCLCDBTCPS_DGMGRL"]; got.Host != "primary-db" || got.Port != 2484 || got.ServiceName != "ORCLCDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected tcps dgmgrl alias: %#v", got)
	}
}

func TestSIDBUnit_BuildManagedTNSAliasesAppliesOverridesAndAppendsExtras(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
					Host: "primary-host",
					Sid:  "PRIMDB",
				},
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseTCPS{
					Enabled: true,
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "PRIMDB",
					Host:        "override-host",
					Port:        1525,
					ServiceName: "override_svc",
				},
				{
					Name:        "PRIMDBTCPS",
					Host:        "secure-host",
					ServiceName: "primdb",
					SSLServerDN: "CN=primary",
				},
				{
					Name:        "DATAGUARD",
					Host:        "dg-host",
					ServiceName: "DATAGUARD",
				},
			},
		},
	}

	aliases, names := buildManagedTNSAliases(sidb, nil)
	expectedNames := []string{"DATAGUARD", "PRIMDB", "PRIMDBTCPS", "PRIMDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected managed alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["PRIMDB"]; got.Host != "override-host" || got.Port != 1525 || got.ServiceName != "OVERRIDE_SVC" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected overridden PRIMDB alias: %#v", got)
	}
	if got := aliases["PRIMDBTCPS"]; got.Host != "secure-host" || got.Port != 2484 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS || got.SSLServerDN != "CN=primary" {
		t.Fatalf("unexpected merged PRIMDBTCPS alias: %#v", got)
	}
	if got := aliases["PRIMDB_DGMGRL"]; got.Host != "primary-host" || got.Port != 1521 || got.ServiceName != "PRIMDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected generated PRIMDB_DGMGRL alias: %#v", got)
	}
	if _, exists := aliases["PRIMDBTCPS_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDBTCPS_DGMGRL alias for truecache")
	}
	if got := aliases["DATAGUARD"]; got.Host != "dg-host" || got.ServiceName != "DATAGUARD" {
		t.Fatalf("unexpected appended DATAGUARD alias: %#v", got)
	}
}

func TestSIDBUnit_GetStandbyWalletDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{}

	if got := GetStandbyWalletMountPath(sidb); got != "/mnt/standby-wallet" {
		t.Fatalf("unexpected default standby wallet mount path: %q", got)
	}
	if got := GetStandbyTDEWalletRoot(sidb); got != "/opt/oracle/oradata/dbconfig/${ORACLE_SID}/.wallet" {
		t.Fatalf("unexpected default standby wallet root: %q", got)
	}
}

func TestSIDBUnit_ValidateStandbyWalletSecretRef(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	ctx := context.Background()
	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "wallet-secret", Namespace: "ns1"},
				Data:       map[string][]byte{"wallet.zip": []byte("zip-bytes")},
			},
		).Build(),
		Log: logr.Discard(),
	}

	t.Run("missing secret", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName: "does-not-exist",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err == nil {
			t.Fatalf("expected error for missing wallet secret")
		}
	})

	t.Run("zip key missing", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName:       "wallet-secret",
							WalletZipFileKey: "missing.zip",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err == nil {
			t.Fatalf("expected error for missing wallet zip key")
		}
	})

	t.Run("valid secret and zip key", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				Security: &dbapi.SingleInstanceDatabaseSecurity{
					Secrets: &dbapi.SingleInstanceDatabaseSecrets{
						TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
							SecretName:       "wallet-secret",
							WalletZipFileKey: "wallet.zip",
						},
					},
				},
			},
		}
		if err := ValidateStandbyWalletSecretRef(reconciler, sidb, ctx); err != nil {
			t.Fatalf("expected valid wallet secret ref, got err: %v", err)
		}
	})
}

func TestSIDBUnit_GetRestoreCatalogStartWithDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Restore: &dbapi.SingleInstanceDatabaseRestoreSpec{
				FileSystem: &dbapi.SingleInstanceDatabaseRestoreFileSystemSpec{
					BackupPath: "/mnt/backup",
				},
			},
		},
	}
	if got := getRestoreCatalogStartWith(sidb); got != "/mnt/backup" {
		t.Fatalf("expected catalogStartWith default to backupPath, got %q", got)
	}
}

func TestSIDBUnit_IsRestoreFSPathVolumeBacked(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Size: "10Gi",
				AdditionalPVCs: []dbapi.AdditionalPVCSpec{
					{
						MountPath: "/mnt/backup",
						PvcName:   "backup-pvc",
					},
				},
			},
		},
	}
	if !isRestoreFSPathVolumeBacked(sidb, "/opt/oracle/oradata/rman") {
		t.Fatalf("expected /opt/oracle/oradata path to be treated as volume-backed when persistence is enabled")
	}
	if !isRestoreFSPathVolumeBacked(sidb, "/mnt/backup/full") {
		t.Fatalf("expected additionalPVC mount path to be treated as volume-backed")
	}
	if isRestoreFSPathVolumeBacked(sidb, "/tmp/random") {
		t.Fatalf("expected unrelated path to be treated as non volume-backed")
	}
}

func TestSIDBUnit_ValidateRestoreSpecRefsObjectStore(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	ctx := context.Background()
	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ociconfig", Namespace: "ns1"},
				Data:       map[string]string{"oci.env": "DBID=123"},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sshkeysecret", Namespace: "ns1"},
				Data:       map[string][]byte{"oci_api_key.pem": []byte("key")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sourcedbtde", Namespace: "ns1"},
				Data:       map[string][]byte{"source-wallet.tar.gz": []byte("wallet")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sourcedbwalletpwd", Namespace: "ns1"},
				Data:       map[string][]byte{"wallet_pwd": []byte("pwd")},
			},
		).Build(),
		Log: logr.Discard(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Restore: &dbapi.SingleInstanceDatabaseRestoreSpec{
				ObjectStore: &dbapi.SingleInstanceDatabaseRestoreObjectStoreSpec{
					OCIConfig:        &dbapi.SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
					PrivateKey:       &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
					SourceDBWallet:   &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sourcedbtde", Key: "source-wallet.tar.gz"},
					SourceDBWalletPw: &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sourcedbwalletpwd", Key: "wallet_pwd"},
				},
			},
		},
	}
	if err := ValidateRestoreSpecRefs(reconciler, sidb, ctx); err != nil {
		t.Fatalf("expected restore refs to validate, got err: %v", err)
	}
}

func TestSIDBUnit_FraMountPathAndRecoverySizeDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					Size: "120Gi",
				},
			},
		},
	}
	if got := getFraMountPath(sidb); got != "/opt/oracle/oradata/fast_recovery_area" {
		t.Fatalf("unexpected FRA mount path default: %q", got)
	}
	if got := getFraRecoveryAreaSize(sidb); got != "120Gi" {
		t.Fatalf("expected FRA recovery area size to default from fra.size, got %q", got)
	}
}

func TestSIDBUnit_InstantiatePVCSpecMalformedVolumeClaimAnnotationDoesNotPanic(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Size:                  "10Gi",
				AccessMode:            "ReadWriteOnce",
				StorageClass:          "standard",
				VolumeClaimAnnotation: "malformed",
			},
		},
	}
	pvc := reconciler.instantiatePVCSpec(sidb)
	if pvc == nil {
		t.Fatalf("expected pvc to be created")
	}
	if len(pvc.Annotations) != 0 {
		t.Fatalf("expected malformed annotation to be ignored, got annotations: %v", pvc.Annotations)
	}
}

func TestSIDBUnit_InstantiatePodSpecCopiesHostAliases(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			HostAliases: []corev1.HostAlias{
				{
					IP:        "10.10.10.10",
					Hostnames: []string{"database.example.com", "db-alias.example.com"},
				},
				{
					IP:        "10.10.10.11",
					Hostnames: []string{"analytics.example.com"},
				},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}
	if len(pod.Spec.HostAliases) != len(sidb.Spec.HostAliases) {
		t.Fatalf("expected %d host aliases, got %d", len(sidb.Spec.HostAliases), len(pod.Spec.HostAliases))
	}
	if pod.Spec.HostAliases[0].IP != "10.10.10.10" || len(pod.Spec.HostAliases[0].Hostnames) != 2 {
		t.Fatalf("unexpected first host alias: %#v", pod.Spec.HostAliases[0])
	}
	if pod.Spec.HostAliases[1].IP != "10.10.10.11" || len(pod.Spec.HostAliases[1].Hostnames) != 1 || pod.Spec.HostAliases[1].Hostnames[0] != "analytics.example.com" {
		t.Fatalf("unexpected second host alias: %#v", pod.Spec.HostAliases[1])
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecCopiesSIDBHostAliases(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			HostAliases: []corev1.HostAlias{
				{
					IP:        "10.10.10.20",
					Hostnames: []string{"primary.example.com", "primary-vip.example.com"},
				},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}
	if len(pod.Spec.HostAliases) != 1 {
		t.Fatalf("expected 1 host alias, got %d", len(pod.Spec.HostAliases))
	}
	if pod.Spec.HostAliases[0].IP != "10.10.10.20" {
		t.Fatalf("expected sidb host alias to be copied to truecache pod, got %#v", pod.Spec.HostAliases[0])
	}
	if len(pod.Spec.HostAliases[0].Hostnames) != 2 || pod.Spec.HostAliases[0].Hostnames[0] != "primary.example.com" || pod.Spec.HostAliases[0].Hostnames[1] != "primary-vip.example.com" {
		t.Fatalf("unexpected truecache pod host alias hostnames: %#v", pod.Spec.HostAliases[0])
	}
}

func TestSIDBUnit_PhaseScheduleFutureRequeueIsPerContext(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}
	phaseCtx := &sidbPhaseContext{
		futureRequeue: ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Minute},
	}

	got, err := reconciler.phaseScheduleFutureRequeue(phaseCtx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue || got.RequeueAfter != 30*time.Minute {
		t.Fatalf("unexpected scheduled result: %#v", got)
	}
	if phaseCtx.futureRequeue != requeueN {
		t.Fatalf("expected future requeue to be reset on context")
	}

	got2, err := reconciler.phaseScheduleFutureRequeue(phaseCtx)
	if err != nil {
		t.Fatalf("unexpected err on second call: %v", err)
	}
	if got2 != requeueN {
		t.Fatalf("expected no requeue after reset, got %#v", got2)
	}
}

func TestSIDBUnit_PhaseConnectStringGate(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}
	pending := &dbapi.SingleInstanceDatabase{ObjectMeta: metav1.ObjectMeta{Name: "sidb1"}, Status: dbapi.SingleInstanceDatabaseStatus{ConnectString: dbcommons.ValueUnavailable}}
	res, err := reconciler.phaseConnectStringGate(pending)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Requeue {
		t.Fatalf("expected requeue when connect string is unavailable")
	}

	ready := &dbapi.SingleInstanceDatabase{Status: dbapi.SingleInstanceDatabaseStatus{ConnectString: "host:1521/ORCLCDB"}}
	res, err = reconciler.phaseConnectStringGate(ready)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available connect string, got %#v", res)
	}
}

func TestSIDBUnit_ReconcileBlockedByUpdateLockRequeues(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1", Namespace: "ns1", Generation: 5},
		Status: dbapi.SingleInstanceDatabaseStatus{
			Conditions: []metav1.Condition{{
				Type:               lockpolicy.DefaultReconcilingConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             lockpolicy.DefaultUpdateLockReason,
				ObservedGeneration: 4,
				Message:            "controller lock active",
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&dbapi.SingleInstanceDatabase{}).
			WithObjects(sidb).
			Build(),
		Log: logr.Discard(),
	}

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "sidb1"},
	})
	if err != nil {
		t.Fatalf("expected no error while lock-gated, got: %v", err)
	}
	if !res.Requeue || res.RequeueAfter != 30*time.Second {
		t.Fatalf("expected lock-gated requeue after 30s, got: %#v", res)
	}
}
