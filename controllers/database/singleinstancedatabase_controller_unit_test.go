package controllers

import (
	"context"
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

func TestSIDBUnit_GetPrimaryDatabaseConnectStringPrefersStandbyConfig(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimaryDatabaseRef: "primary-db",
			StandbyConfig: &dbapi.SingleInstanceDatabaseStandbyConfig{
				PrimaryConnectString: "custom-host:1521/CDB1",
			},
		},
	}
	got := GetPrimaryDatabaseConnectString(sidb, &dbapi.SingleInstanceDatabase{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}, Spec: dbapi.SingleInstanceDatabaseSpec{Sid: "IGN"}})
	if got != "custom-host:1521/CDB1" {
		t.Fatalf("expected standbyConfig primary connect string, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseConnectStringFromPrimaryDetails(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			StandbyConfig: &dbapi.SingleInstanceDatabaseStandbyConfig{
				PrimaryDetails: &dbapi.SingleInstanceDatabasePrimaryDetails{
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
