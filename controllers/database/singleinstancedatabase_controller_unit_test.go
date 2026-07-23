//nolint:staticcheck // unit tests intentionally assert legacy requeue behavior.
package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	lockpolicy "github.com/oracle/oracle-database-operator/commons/lockpolicy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSIDBUnit_CleanupManagedSingleInstanceDatabasePVCsDeletesManagedClaims(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node1", Namespace: "default", UID: types.UID("tck8node1")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Oradata: &dbapi.SingleInstanceDatabasePersistenceOradata{
					Size:         "60Gi",
					StorageClass: "oci-bv",
					AccessMode:   "ReadWriteOnce",
				},
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					Size:             "10Gi",
					StorageClass:     "oci-bv",
					AccessMode:       "ReadWriteOnce",
					RecoveryAreaSize: "8Gi",
				},
			},
		},
	}

	oradataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node1", Namespace: "default"},
	}
	fraPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: getFraClaimName(sidb), Namespace: "default"},
	}
	if err := ctrl.SetControllerReference(sidb, oradataPVC, scheme); err != nil {
		t.Fatalf("failed to set oradata pvc controller reference: %v", err)
	}
	if err := ctrl.SetControllerReference(sidb, fraPVC, scheme); err != nil {
		t.Fatalf("failed to set FRA pvc controller reference: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, oradataPVC, fraPVC).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	pending, err := reconciler.cleanupManagedSingleInstanceDatabasePVCs(context.Background(), sidb)
	if err != nil {
		t.Fatalf("cleanupManagedSingleInstanceDatabasePVCs returned error: %v", err)
	}
	if !pending {
		t.Fatalf("expected cleanupManagedSingleInstanceDatabasePVCs to report pending deletion")
	}

	gotOradata := &corev1.PersistentVolumeClaim{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "tck8node1", Namespace: "default"}, gotOradata); err != nil {
		if client.IgnoreNotFound(err) != nil {
			t.Fatalf("failed to get managed oradata pvc after cleanup call: %v", err)
		}
		return
	}
	if gotOradata.DeletionTimestamp == nil {
		t.Fatalf("expected managed oradata pvc to have deletion timestamp set when PVC is still present")
	}
}

func TestSIDBUnit_BuildContainerResourcesUsesDeprecatedResourceRequirementsFallback(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	}

	got := buildSIDBContainerResources(sidb)
	if cpu := got.Requests.Cpu(); cpu == nil || !cpu.Equal(resource.MustParse("1")) {
		t.Fatalf("expected deprecated cpu request fallback, got %v", cpu)
	}
	if mem := got.Requests.Memory(); mem == nil || !mem.Equal(resource.MustParse("2Gi")) {
		t.Fatalf("expected deprecated memory request fallback, got %v", mem)
	}
	if cpu := got.Limits.Cpu(); cpu == nil || !cpu.Equal(resource.MustParse("2")) {
		t.Fatalf("expected deprecated cpu limit fallback, got %v", cpu)
	}
	if mem := got.Limits.Memory(); mem == nil || !mem.Equal(resource.MustParse("4Gi")) {
		t.Fatalf("expected deprecated memory limit fallback, got %v", mem)
	}
}

func TestSIDBUnit_BuildContainerResourcesPrefersResourcesOverDeprecatedResourceRequirements(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("3"),
				},
			},
			ResourceRequirements: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
	}

	got := buildSIDBContainerResources(sidb)
	if cpu := got.Requests.Cpu(); cpu == nil || !cpu.Equal(resource.MustParse("3")) {
		t.Fatalf("expected spec.resources to take precedence, got %v", cpu)
	}
}

func TestSIDBUnit_CleanupManagedSingleInstanceDatabasePVCsSkipsUnownedClaims(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node2", Namespace: "default", UID: types.UID("tck8node2")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Oradata: &dbapi.SingleInstanceDatabasePersistenceOradata{
					Size:         "60Gi",
					StorageClass: "oci-bv",
					AccessMode:   "ReadWriteOnce",
				},
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					Size:             "10Gi",
					StorageClass:     "oci-bv",
					AccessMode:       "ReadWriteOnce",
					RecoveryAreaSize: "8Gi",
				},
			},
		},
	}

	oradataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node2", Namespace: "default"},
	}
	fraPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: getFraClaimName(sidb), Namespace: "default"},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, oradataPVC, fraPVC).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	pending, err := reconciler.cleanupManagedSingleInstanceDatabasePVCs(context.Background(), sidb)
	if err != nil {
		t.Fatalf("cleanupManagedSingleInstanceDatabasePVCs returned error: %v", err)
	}
	if pending {
		t.Fatalf("expected cleanupManagedSingleInstanceDatabasePVCs to skip unowned PVCs")
	}

	gotOradata := &corev1.PersistentVolumeClaim{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "tck8node2", Namespace: "default"}, gotOradata); err != nil {
		t.Fatalf("failed to get unowned oradata pvc after cleanup call: %v", err)
	}
	if gotOradata.DeletionTimestamp != nil {
		t.Fatalf("expected unowned oradata pvc to be left untouched")
	}
}

func TestSIDBUnit_CreateOrReplacePVCforDatafilesVolRejectsUnownedManagedPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-owned-check", Namespace: "default", UID: types.UID("sidb-owned-check")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Oradata: &dbapi.SingleInstanceDatabasePersistenceOradata{
					Size:         "60Gi",
					StorageClass: "oci-bv",
					AccessMode:   "ReadWriteOnce",
				},
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: sidb.Name, Namespace: sidb.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: func() *string { s := "oci-bv"; return &s }(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("60Gi"),
				},
			},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, pvc).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := reconciler.createOrReplacePVCforDatafilesVol(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(sidb)}, sidb)
	if err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("expected unowned managed datafiles pvc to be rejected, got err: %v", err)
	}
}

func TestSIDBUnit_CreateOrReplacePVCforFRAVolRejectsUnownedManagedPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-owned-fra-check", Namespace: "default", UID: types.UID("sidb-owned-fra-check")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					Size:             "10Gi",
					StorageClass:     "oci-bv",
					AccessMode:       "ReadWriteOnce",
					RecoveryAreaSize: "8Gi",
				},
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: getFraClaimName(sidb), Namespace: sidb.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: func() *string { s := "oci-bv"; return &s }(),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, pvc).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := reconciler.createOrReplacePVCforFRAVol(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(sidb)}, sidb)
	if err == nil || !strings.Contains(err.Error(), "not controlled") {
		t.Fatalf("expected unowned managed FRA pvc to be rejected, got err: %v", err)
	}
}

func TestSIDBUnit_DeleteGeneratedDataguardClientWalletSecretDeletesManagedSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-wallet", Namespace: "default", UID: types.UID("sidb-wallet")},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getGeneratedDataguardClientWalletSecretName(sidb),
			Namespace: sidb.Namespace,
			Labels: map[string]string{
				"database.oracle.com/managed-by":         "singleinstancedatabase-controller",
				"database.oracle.com/tcps-client-wallet": sidb.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(sidb, secret, scheme); err != nil {
		t.Fatalf("failed to set wallet secret controller reference: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, secret).Build(),
		Log:    logr.Discard(),
		Scheme: scheme,
	}

	if err := reconciler.deleteGeneratedDataguardClientWalletSecret(sidb, context.Background()); err != nil {
		t.Fatalf("deleteGeneratedDataguardClientWalletSecret returned error: %v", err)
	}

	got := &corev1.Secret{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, got); err != nil {
		if client.IgnoreNotFound(err) != nil {
			t.Fatalf("failed to get generated wallet secret after cleanup call: %v", err)
		}
		return
	}
	if got.DeletionTimestamp == nil {
		t.Fatalf("expected generated wallet secret to have deletion timestamp set when secret is still present")
	}
}

func TestSIDBUnit_DeleteGeneratedDataguardClientWalletSecretSkipsUnownedSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-wallet-unowned", Namespace: "default", UID: types.UID("sidb-wallet-unowned")},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getGeneratedDataguardClientWalletSecretName(sidb),
			Namespace: sidb.Namespace,
			Labels: map[string]string{
				"database.oracle.com/managed-by":         "singleinstancedatabase-controller",
				"database.oracle.com/tcps-client-wallet": sidb.Name,
			},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, secret).Build(),
		Log:    logr.Discard(),
		Scheme: scheme,
	}

	if err := reconciler.deleteGeneratedDataguardClientWalletSecret(sidb, context.Background()); err != nil {
		t.Fatalf("deleteGeneratedDataguardClientWalletSecret returned error: %v", err)
	}

	got := &corev1.Secret{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, got); err != nil {
		t.Fatalf("failed to get unmanaged wallet secret after cleanup call: %v", err)
	}
	if got.DeletionTimestamp != nil {
		t.Fatalf("expected unmanaged wallet secret to be left untouched")
	}
}

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

func TestSIDBUnit_GetPrimaryDatabaseCDBConnectStringRewritesPDBServiceToDBName(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "custom-host:1521/primary_service",
				DBName:        "ORCLPRD",
			},
		},
	}

	got := GetPrimaryDatabaseCDBConnectString(sidb, nil)
	if got != "custom-host:1521/ORCLPRD" {
		t.Fatalf("expected root-capable connect string derived from dbName, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseCDBConnectStringKeepsMatchingService(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "custom-host:1521/ORCLPRD",
				DBName:        "ORCLPRD",
			},
		},
	}

	got := GetPrimaryDatabaseCDBConnectString(sidb, nil)
	if got != "custom-host:1521/ORCLPRD" {
		t.Fatalf("expected unchanged connect string when service already matches dbName, got %q", got)
	}
}

func TestSIDBUnit_GetPrimaryDatabaseCDBConnectStringKeepsRACDomainService(t *testing.T) {
	// OCI RAC SCAN services are domain-qualified and must not be rewritten to
	// short DB_NAME (DB0515 is not registered on the SCAN listener).
	const want = "racdb26260-scan.okeprivsubnet.k8stest.oraclevcn.com:1521/DB0515_qw6_iad.okeprivsubnet.k8stest.oraclevcn.com"
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: want,
				DBName:        "DB0515",
			},
		},
	}

	got := GetPrimaryDatabaseCDBConnectString(sidb, nil)
	if got != want {
		t.Fatalf("expected RAC service connect string preserved, got %q", got)
	}
	if name := GetPrimaryDatabaseName(sidb, nil); name != "DB0515" {
		t.Fatalf("expected PRIMARY_DB_NAME DB0515, got %q", name)
	}
}

func TestSIDBUnit_ShouldPublishNotReadyExternalService(t *testing.T) {
	tests := []struct {
		name string
		sidb *dbapi.SingleInstanceDatabase
		want bool
	}{
		{
			name: "truecache publishes not ready endpoints",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "truecache"},
			},
			want: true,
		},
		{
			name: "primary keeps default ready-only behavior",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "primary"},
			},
			want: false,
		},
		{
			name: "nil sidb stays disabled",
			sidb: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		if got := shouldPublishNotReadyExternalService(tt.sidb); got != tt.want {
			t.Fatalf("%s: expected %t, got %t", tt.name, tt.want, got)
		}
	}
}

func TestSIDBUnit_ShouldEnsureStandbyManagedRecovery(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "TRUE_CACHE", want: false},
		{role: "PHYSICAL_STANDBY", want: true},
		{role: "SNAPSHOT_STANDBY", want: false},
		{role: "", want: false},
	}

	for _, tt := range tests {
		if got := shouldEnsureStandbyManagedRecovery(tt.role); got != tt.want {
			t.Fatalf("role %q: expected %t, got %t", tt.role, tt.want, got)
		}
	}
}

func TestSIDBUnit_ShouldVerifyStandbyManagedRecovery(t *testing.T) {
	standby := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "standby"},
	}
	if !shouldVerifyStandbyManagedRecovery(standby, "PHYSICAL_STANDBY") {
		t.Fatalf("expected physical standby to require managed recovery verification")
	}
	if shouldVerifyStandbyManagedRecovery(standby, "PRIMARY") {
		t.Fatalf("expected primary role to skip standby managed recovery verification")
	}
	primary := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "primary"},
	}
	if shouldVerifyStandbyManagedRecovery(primary, "PHYSICAL_STANDBY") {
		t.Fatalf("expected primary SIDB spec to skip standby managed recovery verification")
	}
}

func TestSIDBUnit_NormalizeDatabaseRole(t *testing.T) {
	tests := map[string]string{
		"PHYSICAL STANDBY": "PHYSICAL_STANDBY",
		"physical_standby": "PHYSICAL_STANDBY",
		"Snapshot-Standby": "SNAPSHOT_STANDBY",
		"PRIMARY":          "PRIMARY",
	}
	for input, want := range tests {
		if got := normalizeDatabaseRole(input); got != want {
			t.Fatalf("role %q: expected %q, got %q", input, want, got)
		}
	}
}

func TestSIDBUnit_SnapshotStandbyConversionNeeded(t *testing.T) {
	tests := []struct {
		name                 string
		convertToSnapshot    bool
		liveRole             string
		wantConversionNeeded bool
	}{
		{name: "physical to snapshot", convertToSnapshot: true, liveRole: "PHYSICAL STANDBY", wantConversionNeeded: true},
		{name: "snapshot already desired", convertToSnapshot: true, liveRole: "SNAPSHOT_STANDBY", wantConversionNeeded: false},
		{name: "snapshot to physical", convertToSnapshot: false, liveRole: "SNAPSHOT STANDBY", wantConversionNeeded: true},
		{name: "physical already desired", convertToSnapshot: false, liveRole: "PHYSICAL_STANDBY", wantConversionNeeded: false},
		{name: "primary", convertToSnapshot: false, liveRole: "PRIMARY", wantConversionNeeded: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := snapshotStandbyConversionNeeded(test.convertToSnapshot, test.liveRole); got != test.wantConversionNeeded {
				t.Fatalf("expected conversion needed=%t, got %t", test.wantConversionNeeded, got)
			}
		})
	}
}

func TestSIDBUnit_StandbyManagedRecoveryOpenModeHelpers(t *testing.T) {
	tests := []struct {
		mode          string
		wantSupported bool
		wantStart     bool
	}{
		{mode: "MOUNTED", wantSupported: true, wantStart: true},
		{mode: "READ ONLY", wantSupported: true, wantStart: true},
		{mode: "READ_ONLY_WITH_APPLY", wantSupported: true, wantStart: false},
		{mode: "READ ONLY WITH APPLY", wantSupported: true, wantStart: false},
		{mode: "OPEN", wantSupported: false, wantStart: false},
	}
	for _, tt := range tests {
		if got := standbyManagedRecoveryOpenModeSupported(tt.mode); got != tt.wantSupported {
			t.Fatalf("mode %q: expected supported=%t, got %t", tt.mode, tt.wantSupported, got)
		}
		if got := standbyManagedRecoveryShouldStart(tt.mode); got != tt.wantStart {
			t.Fatalf("mode %q: expected start=%t, got %t", tt.mode, tt.wantStart, got)
		}
	}
}

func TestSIDBUnit_SnapshotStandbyConversionOpenModeAllowed(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{mode: "MOUNTED", want: true},
		{mode: "READ ONLY", want: true},
		{mode: "READ ONLY WITH APPLY", want: true},
		{mode: "READ_WRITE", want: false},
	}
	for _, tt := range tests {
		if got := snapshotStandbyConversionOpenModeAllowed(tt.mode); got != tt.want {
			t.Fatalf("open mode %q: expected %t, got %t", tt.mode, tt.want, got)
		}
	}
}

func TestSIDBUnit_StandbyManagedRecoveryActive(t *testing.T) {
	out := `
PROCESS   STATUS
--------- ------------
MRP0      APPLYING_LOG
`
	if !standbyManagedRecoveryActive(out) {
		t.Fatalf("expected MRP output to be treated as active")
	}
	if standbyManagedRecoveryActive("PROCESS STATUS\nRFS IDLE\n") {
		t.Fatalf("expected output without MRP to be treated as inactive")
	}
}

func TestSIDBUnit_ContainsOracleOrBrokerError(t *testing.T) {
	if !containsOracleOrBrokerError("ORA-16627: operation disallowed") {
		t.Fatalf("expected ORA output to be treated as an error")
	}
	if !containsOracleOrBrokerError("DGM-17016: failed to retrieve status") {
		t.Fatalf("expected DGM output to be treated as an error")
	}
	if containsOracleOrBrokerError("Database converted successfully") {
		t.Fatalf("expected success output to avoid error classification")
	}
}

func TestSIDBUnit_ShouldResolveSIDBOEMExpressURL(t *testing.T) {
	tests := []struct {
		name string
		sidb *dbapi.SingleInstanceDatabase
		want bool
	}{
		{
			name: "nil sidb defaults to resolve",
			sidb: nil,
			want: true,
		},
		{
			name: "primary resolves",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "primary"},
			},
			want: true,
		},
		{
			name: "truecache spec skips probe",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "truecache"},
			},
			want: false,
		},
		{
			name: "true cache role skips probe",
			sidb: &dbapi.SingleInstanceDatabase{
				Status: dbapi.SingleInstanceDatabaseStatus{Role: "TRUE_CACHE"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		if got := shouldResolveSIDBOEMExpressURL(tt.sidb); got != tt.want {
			t.Fatalf("%s: expected %t, got %t", tt.name, tt.want, got)
		}
	}
}

func TestSIDBUnit_ShouldEnableStandbyFlashback(t *testing.T) {
	tests := []struct {
		name string
		sidb *dbapi.SingleInstanceDatabase
		want bool
	}{
		{
			name: "nil sidb defaults to enabled",
			sidb: nil,
			want: true,
		},
		{
			name: "physical standby keeps flashback flow",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "standby"},
				Status: dbapi.SingleInstanceDatabaseStatus{
					Role: "PHYSICAL_STANDBY",
				},
			},
			want: true,
		},
		{
			name: "truecache spec skips flashback flow",
			sidb: &dbapi.SingleInstanceDatabase{
				Spec: dbapi.SingleInstanceDatabaseSpec{CreateAs: "truecache"},
			},
			want: false,
		},
		{
			name: "true cache role skips flashback flow",
			sidb: &dbapi.SingleInstanceDatabase{
				Status: dbapi.SingleInstanceDatabaseStatus{Role: "TRUE_CACHE"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		if got := shouldEnableStandbyFlashback(tt.sidb); got != tt.want {
			t.Fatalf("%s: expected %t, got %t", tt.name, tt.want, got)
		}
	}
}

func TestSIDBUnit_ValidateRejectsReplicasGreaterThanOne(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-replicas", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 2,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Oradata: &dbapi.SingleInstanceDatabasePersistenceOradata{
					PvcName: "existing-oradata-pvc",
				},
			},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	got, err := reconciler.validate(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-replicas"}},
	)
	if err == nil {
		t.Fatalf("expected validation error for replicas > 1")
	}
	if got != requeueN {
		t.Fatalf("expected no requeue for invalid replicas, got %#v", got)
	}
	if !strings.Contains(err.Error(), "multi-replica SIDB is not supported") {
		t.Fatalf("expected replicas validation error, got %v", err)
	}
}

func TestSIDBUnit_ValidateTrueCacheUsesDBCredentialsWalletSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node1", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Edition:  "enterprise",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "phx.ocir.io/intsanjaysingh/db-repo/oracle/database:truecache-23.26.1-ee-patch",
			},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "racdb26260-scan.okeprivsubnet.k8stest.oraclevcn.com:1521/DB0515_qw6_iad.okeprivsubnet.k8stest.oraclevcn.com",
			},
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				DBCredentialsWallet: &dbapi.SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
					SecretName: "primary-db-cred-wallet",
					MountPath:  "/u01/app/oracle/db_wallet",
				},
				TrueCacheServices:         []string{"DB0515_PDB1:tcokeprim.example.com:tcokenodes.example.com"},
				AutoTCServiceRegistration: true,
			},
		},
	}

	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-admin-secret", Namespace: "default"},
		Data: map[string][]byte{
			"oracle_pwd": []byte("secret"),
		},
	}

	sidb.Spec.Security = &dbapi.SingleInstanceDatabaseSecurity{
		Secrets: &dbapi.SingleInstanceDatabaseSecrets{
			Admin: &dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "db-admin-secret",
				SecretKey:  "oracle_pwd",
			},
		},
	}
	sidb.Spec.TrueCache.DBCredentialsWallet = nil

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(adminSecret).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	got, err := reconciler.validate(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "tck8node1"}},
	)
	if err != nil {
		t.Fatalf("expected truecache validation to use admin secret, got err: %v", err)
	}
	if got != requeueN {
		t.Fatalf("expected no requeue for valid truecache admin secret, got %#v", got)
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

func TestSIDBUnit_AutoTCServiceRegistrationEnabledUsesExplicitTrueCacheFlag(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				AutoTCServiceRegistration: true,
			},
		},
	}

	if !autoTCServiceRegistrationEnabled(sidb) {
		t.Fatalf("expected AUTO_TC_SVC_REGISTRATION to be enabled when trueCache.autoTCServiceRegistration=true")
	}
}

func TestSIDBUnit_AutoTCServiceRegistrationDisabledByDefault(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{},
		},
	}

	if autoTCServiceRegistrationEnabled(sidb) {
		t.Fatalf("expected AUTO_TC_SVC_REGISTRATION to stay disabled by default")
	}
}

func TestSIDBUnit_GetPrimarySchedulerCredentialName(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				PrimarySchedulerCredentialName: " TC_ORACLE_OS_CRED ",
			},
		},
	}

	if got := getPrimarySchedulerCredentialName(sidb); got != "TC_ORACLE_OS_CRED" {
		t.Fatalf("expected trimmed primary scheduler credential name, got %q", got)
	}
}

func TestSIDBUnit_BuildTrueCacheContainerEnvIncludesDBCredentialsWalletDir(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "truecache", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "orcltc",
			CreateAs: "truecache",
			Edition:  "enterprise",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "10.0.2.101:1521/primary_service",
				DBName:        "ORCLPRD",
			},
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				BlobMountPath:                  "/stage/tc_config_blob.tar.gz",
				TrueCacheServices:              []string{"APPPDB1:tpdb_primary:tpdb_cache"},
				TruedbUniqueName:               "truecache_tc",
				PrimarySchedulerCredentialName: "TC_ORACLE_OS_CRED",
				DBCredentialsWallet: &dbapi.SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
					SecretName: "primary-db-cred-wallet",
					MountPath:  "/u01/app/oracle/db_wallet",
				},
			},
			InitParams: &dbapi.SingleInstanceDatabaseInitParams{
				SgaTarget:          16384,
				PgaAggregateTarget: 4096,
				Processes:          1200,
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				Secrets: &dbapi.SingleInstanceDatabaseSecrets{
					TDE: &dbapi.SingleInstanceDatabasePasswordSecret{
						SecretName: "tde-wallet-secret",
						SecretKey:  "tde_wallet_pwd",
					},
				},
			},
		},
	}

	envs := buildTrueCacheContainerEnv(
		sidb,
		nil,
		"/opt/oracle/oradata/dbconfig/ORCLTC/.wallet",
		"/run/secrets",
		"oracle_pwd",
		"truecache.internal.example.com",
	)

	values := map[string]string{}
	for _, env := range envs {
		values[env.Name] = env.Value
	}

	if got := values["TRUE_CACHE_DB_CREDENTIAL_WALLET_DIR"]; got != "/u01/app/oracle/db_wallet" {
		t.Fatalf("expected TRUE_CACHE_DB_CREDENTIAL_WALLET_DIR to be populated for truecache main container, got %q", got)
	}
	if got := values["SECRET_VOLUME"]; got != "" {
		t.Fatalf("expected SECRET_VOLUME to stay unset for manual truecache wallet flow, got %q", got)
	}
	if got := values["PASSWORD_FILE"]; got != "" {
		t.Fatalf("expected PASSWORD_FILE to stay unset for truecache wallet flow, got %q", got)
	}
	if got := values["PRIMARY_DB_CONN_STR"]; got != "10.0.2.101:1521/ORCLPRD" {
		t.Fatalf("expected truecache env to use root-capable primary connect string, got %q", got)
	}
	if got := values["PRIMARY_TC_SERVICE_CREDENTIAL_NAME"]; got != "TC_ORACLE_OS_CRED" {
		t.Fatalf("expected truecache env to include PRIMARY_TC_SERVICE_CREDENTIAL_NAME, got %q", got)
	}
	if got := values["INIT_SGA_SIZE"]; got != "16384" {
		t.Fatalf("expected truecache env to include INIT_SGA_SIZE, got %q", got)
	}
	if got := values["INIT_PGA_SIZE"]; got != "4096" {
		t.Fatalf("expected truecache env to include INIT_PGA_SIZE, got %q", got)
	}
	if got := values["INIT_PROCESSES"]; got != "1200" {
		t.Fatalf("expected truecache env to include INIT_PROCESSES, got %q", got)
	}
}

func TestSIDBUnit_ShouldMountAdminPasswordSecretForTrueCache(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				Secrets: &dbapi.SingleInstanceDatabaseSecrets{
					Admin: &dbapi.SingleInstanceDatabaseAdminPassword{
						SecretName: "db-admin-secret",
						SecretKey:  "oracle_pwd",
					},
				},
			},
		},
	}

	if !shouldMountAdminPasswordSecret(sidb) {
		t.Fatalf("expected truecache with security.secrets.admin to mount the admin password secret")
	}
}

func TestSIDBUnit_CloneAndStandbyExposePasswordFileFromAdminSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIM",
		},
	}

	tests := []struct {
		name     string
		createAs string
		cloneRef *dbapi.SingleInstanceDatabase
		dgRef    *dbapi.SingleInstanceDatabase
	}{
		{name: "clone", createAs: "clone", cloneRef: primary},
		{name: "standby", createAs: "standby", dgRef: primary},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sidb := &dbapi.SingleInstanceDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "sidb-" + tc.name, Namespace: "ns1"},
				Spec: dbapi.SingleInstanceDatabaseSpec{
					CreateAs: tc.createAs,
					Sid:      "ORCL",
					Edition:  "enterprise",
					Image: dbapi.SingleInstanceDatabaseImage{
						PullFrom: "container-registry.oracle.com/database/enterprise:latest",
					},
					AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
						SecretName: "sidb-admin",
						SecretKey:  "sys_pwd",
						MountPath:  "/mnt/admin-secret",
					},
				},
			}

			pod, err := reconciler.instantiatePodSpec(sidb, tc.cloneRef, tc.dgRef, false)
			if err != nil {
				t.Fatalf("instantiatePodSpec returned err: %v", err)
			}

			envs := map[string]string{}
			for _, env := range pod.Spec.Containers[0].Env {
				envs[env.Name] = env.Value
			}
			if got := envs["SECRET_VOLUME"]; got != "/mnt/admin-secret" {
				t.Fatalf("expected SECRET_VOLUME to point at admin secret mount root, got %q", got)
			}
			if got := envs["ORACLE_PWD_SECRET_NAME"]; got != "sys_pwd" {
				t.Fatalf("expected ORACLE_PWD_SECRET_NAME to use admin secret key, got %q", got)
			}
			if got := envs["PASSWORD_FILE"]; got != "sys_pwd" {
				t.Fatalf("expected PASSWORD_FILE to use admin secret key for image startup script, got %q", got)
			}

			var adminMount *corev1.VolumeMount
			for i := range pod.Spec.Containers[0].VolumeMounts {
				if pod.Spec.Containers[0].VolumeMounts[i].Name == "oracle-pwd-vol" {
					adminMount = &pod.Spec.Containers[0].VolumeMounts[i]
					break
				}
			}
			if adminMount == nil {
				t.Fatalf("expected clone/standby pod to mount oracle-pwd-vol")
			}
			if adminMount.MountPath != "/mnt/admin-secret/sys_pwd" || adminMount.SubPath != "sys_pwd" || !adminMount.ReadOnly {
				t.Fatalf("unexpected admin password mount: %#v", *adminMount)
			}
		})
	}
}

func TestSIDBUnit_SIDBPodContainerRunningRequiresRunningMainContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "sidb-standby"}},
		},
	}

	if !sidbPodSpecHasContainer(pod, "sidb-standby") {
		t.Fatalf("expected pod spec to contain sidb-standby")
	}
	if sidbPodSpecHasContainer(pod, "missing") {
		t.Fatalf("did not expect pod spec to contain missing container")
	}
	if sidbPodContainerRunning(pod, "sidb-standby") {
		t.Fatalf("container without status must not be treated as running")
	}
	if got := sidbPodContainerState(pod, "sidb-standby"); got != "not reported" {
		t.Fatalf("unexpected missing status state: %q", got)
	}

	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "sidb-standby",
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"},
		},
	}}
	if sidbPodContainerRunning(pod, "sidb-standby") {
		t.Fatalf("waiting container must not be treated as running")
	}
	if got := sidbPodContainerState(pod, "sidb-standby"); got != "waiting: ContainerCreating" {
		t.Fatalf("unexpected waiting state: %q", got)
	}

	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1},
	}
	if sidbPodContainerRunning(pod, "sidb-standby") {
		t.Fatalf("terminated container must not be treated as running")
	}
	if got := sidbPodContainerState(pod, "sidb-standby"); got != "terminated: Error exitCode=1" {
		t.Fatalf("unexpected terminated state: %q", got)
	}

	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{},
	}
	if !sidbPodContainerRunning(pod, "sidb-standby") {
		t.Fatalf("running container should be treated as running")
	}
	if got := sidbPodContainerState(pod, "sidb-standby"); got != "running" {
		t.Fatalf("unexpected running state: %q", got)
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

func TestSIDBUnit_GetPrimaryDatabasePdbNameFromConnectStringSource(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary-host:1522/primdb",
				Pdbname:       "appPdb1",
			},
		},
	}

	if got := GetPrimaryDatabasePdbName(sidb, nil); got != "APPPDB1" {
		t.Fatalf("expected primary pdb name from connect string source, got %q", got)
	}
	if got := ShouldCreatePDBFromPrimary(sidb, nil); got != "true" {
		t.Fatalf("expected create_pdb to be true for connect string source with pdbName, got %q", got)
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

func TestSIDBUnit_WalletAlreadySeeded(t *testing.T) {
	if walletAlreadySeeded("") {
		t.Fatalf("expected empty output to mean wallet is not seeded")
	}
	if walletAlreadySeeded("   \n\t") {
		t.Fatalf("expected whitespace output to mean wallet is not seeded")
	}
	if !walletAlreadySeeded("present\n") {
		t.Fatalf("expected non-empty output to mean wallet already exists")
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusForStandby(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0", PullSecrets: "pull-secret"},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIM",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "primary-admin",
			},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, primary)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected preview phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true")
	}
	if sidb.Status.Dataguard.PrimaryMemberName != "primary-db" {
		t.Fatalf("expected primary member name primary-db, got %q", sidb.Status.Dataguard.PrimaryMemberName)
	}
	if sidb.Status.Dataguard.MemberName != "sidb-standby" {
		t.Fatalf("expected member name sidb-standby, got %q", sidb.Status.Dataguard.MemberName)
	}
	if sidb.Status.Dataguard.Role != "PHYSICAL_STANDBY" {
		t.Fatalf("expected standby member role, got %q", sidb.Status.Dataguard.Role)
	}
	if sidb.Status.Dataguard.PublishedTopologyHash == "" {
		t.Fatalf("expected published topology hash to be set")
	}
	if sidb.Status.Dataguard.TopologyHash == "" {
		t.Fatalf("expected topology hash to be set")
	}
	if sidb.Status.Dataguard.LastPublishedTime == nil {
		t.Fatalf("expected lastPublishedTime to be set")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec == nil {
		t.Fatalf("expected renderedBrokerSpec to be published")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Name != "sidb-standby-dg" {
		t.Fatalf("unexpected rendered broker name: %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Namespace != "ns1" {
		t.Fatalf("unexpected rendered broker namespace: %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology, got %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution.Image == "" {
		t.Fatalf("expected rendered broker execution image to be published")
	}
	if !sidb.Status.Dataguard.RenderedBrokerSpec.Ready {
		t.Fatalf("expected rendered broker spec to be marked ready")
	}
	gotMembers := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	if len(gotMembers) != 2 {
		t.Fatalf("expected two rendered broker members, got %#v", gotMembers)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.AdminSecretRef == nil {
		t.Fatalf("expected topology defaults adminSecretRef to be published")
	}
	if defaults.AdminSecretRef.SecretName != "standby-admin" || defaults.AdminSecretRef.SecretKey != "oracle_pwd" {
		t.Fatalf("unexpected topology defaults adminSecretRef %#v", defaults.AdminSecretRef)
	}
	for _, member := range gotMembers {
		switch member.Role {
		case "PRIMARY":
			if member.AdminSecretRef == nil || member.AdminSecretRef.SecretName != "primary-admin" || member.AdminSecretRef.SecretKey != "oracle_pwd" {
				t.Fatalf("expected primary override adminSecretRef to be published for member %#v", member)
			}
		case "PHYSICAL_STANDBY":
			if member.AdminSecretRef != nil {
				t.Fatalf("expected standby member to inherit adminSecretRef from topology defaults, got %#v", member)
			}
		}
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusExternalPrimaryPublishesReadyPreviewWithPlaceholder(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0"},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true when placeholder values are published")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology to be published")
	}
	if !sidb.Status.Dataguard.RenderedBrokerSpec.Ready {
		t.Fatalf("expected rendered broker spec to be marked ready")
	}
	condition := meta.FindStatusCondition(sidb.Status.Dataguard.Conditions, "TopologyPreviewReady")
	if condition == nil {
		t.Fatalf("expected TopologyPreviewReady condition to be set")
	}
	if condition.Reason != "PreviewReady" {
		t.Fatalf("expected PreviewReady condition reason, got %#v", condition)
	}
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("expected TopologyPreviewReady condition status true, got %#v", condition)
	}
	if !strings.Contains(condition.Message, "topology.defaults.adminSecretRef") {
		t.Fatalf("expected condition message to explain topology default admin secret usage, got %#v", condition)
	}
	members := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	if len(members) != 2 {
		t.Fatalf("expected two topology members, got %#v", members)
	}
	var externalPrimary *dbapi.DataguardTopologyMember
	for i := range members {
		if members[i].Role == "PRIMARY" {
			externalPrimary = &members[i]
			break
		}
	}
	if externalPrimary == nil {
		t.Fatalf("expected primary member in rendered topology, got %#v", members)
	}
	if externalPrimary.AdminSecretRef != nil {
		t.Fatalf("expected external primary member to inherit adminSecretRef from topology defaults, got %#v", externalPrimary.AdminSecretRef)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.AdminSecretRef == nil {
		t.Fatalf("expected topology defaults adminSecretRef to be published")
	}
	if defaults.AdminSecretRef.SecretName != "standby-admin" || defaults.AdminSecretRef.SecretKey != "oracle_pwd" {
		t.Fatalf("unexpected topology defaults adminSecretRef %#v", defaults.AdminSecretRef)
	}
	if externalPrimary.TCPS != nil {
		t.Fatalf("did not expect primary tcps block when standby tcps is disabled, got %#v", externalPrimary.TCPS)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusExternalPrimaryInfersTCPSPlaceholders(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			Image: dbapi.SingleInstanceDatabaseImage{PullFrom: "oracle/db:19.3.0"},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled: true,
				},
			},
			TcpsListenerPort: 2484,
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseReady, sidb.Status.Dataguard.Phase)
	}
	if !sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true")
	}
	condition := meta.FindStatusCondition(sidb.Status.Dataguard.Conditions, "TopologyPreviewReady")
	if condition == nil {
		t.Fatalf("expected TopologyPreviewReady condition to be set")
	}
	if !strings.Contains(condition.Message, "topology.defaults.tcps.clientWalletSecret") {
		t.Fatalf("expected condition message to mention topology tcps defaults, got %#v", condition)
	}
	members := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	var externalPrimary *dbapi.DataguardTopologyMember
	for i := range members {
		if members[i].Role == "PRIMARY" {
			externalPrimary = &members[i]
			break
		}
	}
	if externalPrimary == nil {
		t.Fatalf("expected primary member in rendered topology, got %#v", members)
	}
	if externalPrimary.TCPS == nil || !externalPrimary.TCPS.Enabled {
		t.Fatalf("expected inferred primary tcps block, got %#v", externalPrimary.TCPS)
	}
	if externalPrimary.TCPS.ClientWalletSecret != "" {
		t.Fatalf("expected external primary to inherit tcps wallet secret from topology defaults, got %#v", externalPrimary.TCPS)
	}
	defaults := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Topology.Defaults
	if defaults == nil || defaults.TCPS == nil {
		t.Fatalf("expected topology tcps defaults to be published")
	}
	if defaults.TCPS.ClientWalletSecret != dataguardPreviewSharedWalletPlaceholder {
		t.Fatalf("unexpected topology tcps wallet default %#v", defaults.TCPS)
	}
	var tcpsEndpoint *dbapi.DataguardEndpointSpec
	for i := range externalPrimary.Endpoints {
		if externalPrimary.Endpoints[i].Protocol == "TCPS" {
			tcpsEndpoint = &externalPrimary.Endpoints[i]
			break
		}
	}
	if tcpsEndpoint == nil {
		t.Fatalf("expected primary tcps endpoint in rendered topology, got %#v", externalPrimary.Endpoints)
	}
	if tcpsEndpoint.Port != 2484 || tcpsEndpoint.ServiceName != "PRIM" {
		t.Fatalf("unexpected inferred primary tcps endpoint: %#v", tcpsEndpoint)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusDefaultsRenderedExecutionAuthWalletWithoutImage(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBY",
			CreateAs: "standby",
			AdminPassword: dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "standby-admin",
			},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "external-primary:1521/PRIM",
			},
			Dataguard: &dbapi.DataguardProducerSpec{},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			CreatedAs: "standby",
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil || sidb.Status.Dataguard.RenderedBrokerSpec == nil || sidb.Status.Dataguard.RenderedBrokerSpec.Spec == nil {
		t.Fatalf("expected rendered broker spec to be published, got %#v", sidb.Status.Dataguard)
	}
	execution := sidb.Status.Dataguard.RenderedBrokerSpec.Spec.Execution
	if execution == nil {
		t.Fatalf("expected rendered broker execution to be published even without image")
	}
	if execution.Image != "" {
		t.Fatalf("expected no execution image, got %q", execution.Image)
	}
	if execution.AuthWallet == nil || !execution.AuthWallet.Enabled {
		t.Fatalf("expected default auth wallet to be enabled, got %#v", execution.AuthWallet)
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusDisabledClearsStatus(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Dataguard: &dbapi.DataguardProducerSpec{
				Mode: dbapi.DataguardProducerModeDisabled,
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			Dataguard: &dbapi.ProducerDataguardStatus{Phase: dataguardPreviewPhaseReady},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard != nil {
		t.Fatalf("expected dataguard status to be cleared when mode is disabled")
	}
}

func TestSIDBUnit_SyncDataguardPreviewStatusTrueCacheIsNotApplicable(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-truecache", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:       "TC01",
			CreateAs:  "truecache",
			Dataguard: &dbapi.DataguardProducerSpec{Mode: dbapi.DataguardProducerModePreview},
		},
	}

	syncSIDBDataguardPreviewStatus(sidb, nil)

	if sidb.Status.Dataguard == nil {
		t.Fatalf("expected dataguard status to be populated for not-applicable truecache")
	}
	if sidb.Status.Dataguard.Phase != dataguardPreviewPhaseNotApplicable {
		t.Fatalf("expected phase %q, got %q", dataguardPreviewPhaseNotApplicable, sidb.Status.Dataguard.Phase)
	}
	if sidb.Status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be false for truecache")
	}
	if sidb.Status.Dataguard.RenderedBrokerSpec != nil {
		t.Fatalf("expected no rendered broker spec for truecache, got %#v", sidb.Status.Dataguard.RenderedBrokerSpec)
	}
}

func TestSIDBUnit_BuildSIDBPreviewTCPSConfigUsesOverrideWalletSecret(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBY",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled:            true,
					TlsSecret:          "server-tls",
					ClientWalletSecret: "custom-client-wallet",
				},
			},
		},
	}

	tcps := buildSIDBPreviewTCPSConfig(sidb)
	if tcps == nil {
		t.Fatalf("expected TCPS config")
	}
	if tcps.ClientWalletSecret != "custom-client-wallet" {
		t.Fatalf("expected custom client wallet secret, got %q", tcps.ClientWalletSecret)
	}
}

func TestSIDBUnit_BuildSIDBPreviewTCPSConfigUsesGeneratedWalletSecretWhenEnabled(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBY",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
					Enabled:   true,
					TlsSecret: "server-tls",
				},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			IsTcpsEnabled:   true,
			ClientWalletLoc: "/opt/oracle/oradata/clientWallet/STBY",
		},
	}

	tcps := buildSIDBPreviewTCPSConfig(sidb)
	if tcps == nil {
		t.Fatalf("expected TCPS config")
	}
	if tcps.ClientWalletSecret != "sidb-standby-dg-client-wallet" {
		t.Fatalf("expected generated client wallet secret, got %q", tcps.ClientWalletSecret)
	}
}

func TestSIDBUnit_BuildAutomaticPrimaryTNSAliases(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Sid:      "STBYDB",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "primary-db",
			},
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
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

	aliases, names := buildAutomaticStandbyPeerTNSAliases(sidb, primary)
	expectedNames := []string{"ORCLCDB", "ORCLCDBTCPS", "ORCLCDBTCPS_DGMGRL", "ORCLCDB_DGMGRL", "STBYDB", "STBYDBTCPS", "STBYDBTCPS_DGMGRL", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected generated alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["STBYDB"]; got.Host != "sidb-standby" || got.Port != 1521 || got.ServiceName != "STBYDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected base standby self alias: %#v", got)
	}
	if got := aliases["STBYDB_DGMGRL"]; got.Host != "sidb-standby" || got.Port != 1521 || got.ServiceName != "STBYDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected standby self dgmgrl alias: %#v", got)
	}
	if got := aliases["STBYDBTCPS"]; got.Host != "sidb-standby" || got.Port != 2484 || got.ServiceName != "STBYDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected standby self tcps alias: %#v", got)
	}
	if got := aliases["STBYDBTCPS_DGMGRL"]; got.Host != "sidb-standby" || got.Port != 2484 || got.ServiceName != "STBYDB_DGMGRL" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected standby self tcps dgmgrl alias: %#v", got)
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

func TestSIDBUnit_BuildMissingTNSAliasesCommandSeparatesPrimaryAndStandbyChecks(t *testing.T) {
	cmd := buildMissingTNSAliasesCommand("/opt/oracle/product/19c/dbhome_1/network/admin/tnsnames.ora", []string{
		"PRIMDB",
		"PRIMDBTCPS",
		"STBYDB",
		"STBYDBTCPS",
	})

	if cmd == "" {
		t.Fatalf("expected verification command")
	}
	if strings.Contains(cmd, "fi if") {
		t.Fatalf("verification command must separate shell if blocks, got: %s", cmd)
	}
	for _, want := range []string{"PRIMDB", "PRIMDBTCPS", "STBYDB", "STBYDBTCPS"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("expected command to verify %s, got: %s", want, cmd)
		}
	}
	if got := strings.Count(cmd, "; if ! grep"); got != 3 {
		t.Fatalf("expected semicolon separators between alias checks, got %d in: %s", got, cmd)
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
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{
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

	aliases, names := buildStandbyManagedTNSAliases(sidb, nil)
	expectedNames := []string{"DATAGUARD", "PRIMDB", "PRIMDBTCPS"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected managed alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["PRIMDB"]; got.Host != "primary-host" || got.Port != 1521 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected protected PRIMDB alias: %#v", got)
	}
	if got := aliases["PRIMDBTCPS"]; got.Host != "primary-host" || got.Port != 2484 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS || got.SSLServerDN != "" {
		t.Fatalf("unexpected protected PRIMDBTCPS alias: %#v", got)
	}
	if _, exists := aliases["PRIMDB_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDB_DGMGRL alias for truecache")
	}
	if _, exists := aliases["PRIMDBTCPS_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDBTCPS_DGMGRL alias for truecache")
	}
	if got := aliases["DATAGUARD"]; got.Host != "dg-host" || got.ServiceName != "DATAGUARD" {
		t.Fatalf("unexpected appended DATAGUARD alias: %#v", got)
	}
}

func TestSIDBUnit_BuildManagedTNSAliasesForTrueCacheTCPOnlySkipsDGMGRL(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				Details: &dbapi.SingleInstanceDatabasePrimaryDetails{
					Host: "primary-host",
					Sid:  "PRIMDB",
				},
			},
		},
	}

	aliases, names := buildStandbyManagedTNSAliases(sidb, nil)
	expectedNames := []string{"PRIMDB"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected managed alias names: got %v want %v", names, expectedNames)
	}

	if got := aliases["PRIMDB"]; got.Host != "primary-host" || got.Port != 1521 || got.ServiceName != "PRIMDB" || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected PRIMDB alias for TCP-only truecache: %#v", got)
	}
	if _, exists := aliases["PRIMDBTCPS"]; exists {
		t.Fatalf("did not expect PRIMDBTCPS alias when TCPS is disabled")
	}
	if _, exists := aliases["PRIMDB_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDB_DGMGRL alias for truecache")
	}
	if _, exists := aliases["PRIMDBTCPS_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDBTCPS_DGMGRL alias for truecache")
	}
}

func TestSIDBUnit_BuildLegacySingleStandbyPrimaryPeerTNSAliasesAppliesOverridesWithoutAppendingExtras(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{
					Enabled: true,
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "STBYDB",
					Host:        "override-standby",
					Port:        1525,
					ServiceName: "stby_service",
					Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS,
					SSLServerDN: "CN=standby",
				},
				{
					Name:        "EXTRA_ALIAS",
					Host:        "extra-host",
					ServiceName: "EXTRA_SERVICE",
				},
			},
		},
	}
	standby := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBYDB",
		},
	}

	aliases, names := buildLegacySingleStandbyPrimaryPeerTNSAliases(primary, standby)
	expectedNames := []string{"PRIMDB", "PRIMDB_DGMGRL", "STBYDB", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected primary peer alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["PRIMDB"]; got.Host != "sidb-primary" ||
		got.Port != 1521 ||
		got.ServiceName != "PRIMDB" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected primary self alias: %#v", got)
	}
	got := aliases["STBYDB"]
	if got.Host != "sidb-standby" ||
		got.Port != 1521 ||
		got.ServiceName != "STBYDB" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected standby peer alias: %#v", got)
	}
	if got := aliases["STBYDB_DGMGRL"]; got.Host != "sidb-standby" ||
		got.Port != 1521 ||
		got.ServiceName != "STBYDB_DGMGRL" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected standby peer dgmgrl alias: %#v", got)
	}
	if got := aliases["PRIMDB_DGMGRL"]; got.Host != "sidb-primary" ||
		got.Port != 1521 ||
		got.ServiceName != "PRIMDB_DGMGRL" ||
		got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP ||
		got.SSLServerDN != "" {
		t.Fatalf("unexpected protected primary self dgmgrl alias: %#v", got)
	}
	if _, exists := aliases["EXTRA_ALIAS"]; exists {
		t.Fatalf("did not expect extra alias to be appended on primary peer path")
	}
}

func TestSIDBUnit_BuildLegacySingleStandbyPrimaryPeerTNSAliasesSkipsDGMGRLWhenPrimaryPrereqsDisabled(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
		},
	}
	standby := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "STBYDB",
		},
	}

	aliases, names := buildLegacySingleStandbyPrimaryPeerTNSAliases(primary, standby)
	expectedNames := []string{"PRIMDB", "STBYDB"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected primary peer alias names when prereqs disabled: got %v want %v", names, expectedNames)
	}
	if _, exists := aliases["PRIMDB_DGMGRL"]; exists {
		t.Fatalf("did not expect PRIMDB_DGMGRL alias when primary dataguard prereqs are disabled")
	}
	if _, exists := aliases["STBYDB_DGMGRL"]; exists {
		t.Fatalf("did not expect STBYDB_DGMGRL alias when primary dataguard prereqs are disabled")
	}
}

func TestSIDBUnit_BuildPrimaryPeerTNSAliasesForConfiguredStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
				StandbySources: []dbapi.DataguardStandbySourceSpec{
					{
						DBUniqueName: "STBYA",
						Host:         "stbya.example",
						TCPSEnabled:  true,
						TCPPort:      1621,
					},
					{
						DBUniqueName: "STBYB",
						Host:         "stbyb.example",
					},
				},
			},
			TNSAliases: []dbapi.SingleInstanceDatabaseTNSAlias{
				{
					Name:        "STBYA",
					Host:        "ignored-host",
					Port:        9999,
					ServiceName: "IGNORED",
				},
			},
		},
	}

	aliases, names := buildPrimaryPeerTNSAliasesForTargets(primary, configuredSIDBPrimaryPeerAliasTargets(primary), true)
	expectedNames := []string{
		"PRIMDB",
		"PRIMDBTCPS",
		"PRIMDBTCPS_DGMGRL",
		"PRIMDB_DGMGRL",
		"STBYA",
		"STBYATCPS",
		"STBYATCPS_DGMGRL",
		"STBYA_DGMGRL",
		"STBYB",
		"STBYB_DGMGRL",
	}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected configured primary peer alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["STBYA"]; got.Host != "stbya.example" || got.Port != 1621 || got.ServiceName != "STBYA" {
		t.Fatalf("unexpected STBYA alias: %#v", got)
	}
	if got := aliases["STBYATCPS"]; got.Host != "stbya.example" || got.Port != 2484 || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS {
		t.Fatalf("unexpected STBYATCPS alias: %#v", got)
	}
	if got := aliases["STBYB"]; got.Host != "stbyb.example" || got.Port != 1521 || got.Protocol != dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP {
		t.Fatalf("unexpected STBYB alias: %#v", got)
	}
	if _, exists := aliases["STBYBTCPS"]; exists {
		t.Fatalf("did not expect STBYBTCPS alias when configured standby does not enable TCPS")
	}
}

func TestSIDBUnit_SIDBPrimaryUsesExplicitStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Dataguard: &dbapi.DataguardProducerSpec{
				StandbySources: []dbapi.DataguardStandbySourceSpec{{DBUniqueName: "STBYDB", Host: "sidb-standby"}},
			},
		},
	}
	if !sidbPrimaryUsesExplicitStandbySources(primary) {
		t.Fatalf("expected primary with standbySources to use explicit standby source management")
	}

	standby := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "standby",
			Dataguard: &dbapi.DataguardProducerSpec{
				StandbySources: []dbapi.DataguardStandbySourceSpec{{DBUniqueName: "STBYDB", Host: "sidb-standby"}},
			},
		},
	}
	if sidbPrimaryUsesExplicitStandbySources(standby) {
		t.Fatalf("did not expect non-primary SIDB to use explicit standby source management")
	}
}

func TestSIDBUnit_ResolvePrimaryPeerTNSAliasesUsesConfiguredStandbySources(t *testing.T) {
	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			Sid:      "PRIMDB",
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Dataguard: &dbapi.DataguardProducerSpec{
				Prereqs: &dbapi.DataguardPrereqsSpec{Enabled: true},
				StandbySources: []dbapi.DataguardStandbySourceSpec{
					{DBUniqueName: "STBYDB", Host: "sidb-standby.shns.svc.cluster.local", TCPSEnabled: true},
				},
			},
		},
	}

	aliases, names, source, err := resolvePrimaryPeerTNSAliases(&SingleInstanceDatabaseReconciler{}, primary, nil, context.Background())
	if err != nil {
		t.Fatalf("resolvePrimaryPeerTNSAliases() error = %v", err)
	}
	if source != "spec.dataguard.standbySources" {
		t.Fatalf("unexpected alias source: got %q", source)
	}
	expectedNames := []string{"PRIMDB", "PRIMDBTCPS", "PRIMDBTCPS_DGMGRL", "PRIMDB_DGMGRL", "STBYDB", "STBYDBTCPS", "STBYDBTCPS_DGMGRL", "STBYDB_DGMGRL"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected alias names: got %v want %v", names, expectedNames)
	}
	if got := aliases["STBYDBTCPS"]; got.Host != "sidb-standby.shns.svc.cluster.local" || got.Port != 2484 {
		t.Fatalf("unexpected STBYDBTCPS alias: %#v", got)
	}
}

func TestSIDBUnit_DiscoverLegacyPrimaryPeerAliasTargetsIncludesAllStandbysForPrimary(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	primary := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-primary", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "PRIMDB",
		},
	}
	standbyOne := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby-a", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "STBYA",
			CreateAs: "standby",
			Security: &dbapi.SingleInstanceDatabaseSecurity{TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true}},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				DatabaseRef: "sidb-primary",
			},
		},
	}
	standbyTwo := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby-b", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:                "STBYB",
			CreateAs:           "standby",
			PrimaryDatabaseRef: "sidb-primary",
		},
	}
	unrelated := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-other", Namespace: "shns"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "OTHER",
			CreateAs: "primary",
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(primary, standbyOne, standbyTwo, unrelated).Build(),
	}

	targets, err := discoverLegacyPrimaryPeerAliasTargets(reconciler, primary, nil, context.Background())
	if err != nil {
		t.Fatalf("discoverLegacyPrimaryPeerAliasTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 standby targets, got %d: %#v", len(targets), targets)
	}
	if targets[0].DBUniqueName != "STBYA" || targets[0].Host != "sidb-standby-a" || !targets[0].TCPSEnabled {
		t.Fatalf("unexpected first discovered standby target: %#v", targets[0])
	}
	if targets[1].DBUniqueName != "STBYB" || targets[1].Host != "sidb-standby-b" || targets[1].TCPSEnabled {
		t.Fatalf("unexpected second discovered standby target: %#v", targets[1])
	}
}

func TestSIDBUnit_ComputeTNSAliasesHashChangesWhenAliasSetChanges(t *testing.T) {
	baseAliases := map[string]dbapi.SingleInstanceDatabaseTNSAlias{
		"PRIMDB": {
			Name:        "PRIMDB",
			Host:        "sidb-primary",
			Port:        1521,
			ServiceName: "PRIMDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
		"STBYDB": {
			Name:        "STBYDB",
			Host:        "sidb-standby",
			Port:        1521,
			ServiceName: "STBYDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCP,
		},
	}
	baseNames := []string{"PRIMDB", "STBYDB"}

	hash1 := computeTNSAliasesHash(baseAliases, baseNames)
	hash2 := computeTNSAliasesHash(baseAliases, baseNames)
	if hash1 == "" || hash1 != hash2 {
		t.Fatalf("expected stable non-empty alias hash, got %q and %q", hash1, hash2)
	}

	changedAliases := map[string]dbapi.SingleInstanceDatabaseTNSAlias{
		"PRIMDB": baseAliases["PRIMDB"],
		"STBYDB": {
			Name:        "STBYDB",
			Host:        "sidb-standby.shns.svc.cluster.local",
			Port:        2484,
			ServiceName: "STBYDB",
			Protocol:    dbapi.SingleInstanceDatabaseTNSAliasProtocolTCPS,
		},
	}
	hash3 := computeTNSAliasesHash(changedAliases, baseNames)
	if hash1 == hash3 {
		t.Fatalf("expected alias hash to change when desired primary peer aliases change")
	}
}

func resolvedSIDBEndpointConfigForTest(t *testing.T, sidb *dbapi.SingleInstanceDatabase, name dbapi.SingleInstanceDatabaseServiceEndpointName) sidbResolvedServiceEndpointConfig {
	t.Helper()
	for _, cfg := range resolveServiceEndpointConfigs(sidb) {
		if cfg.Name == name {
			return cfg
		}
	}
	t.Fatalf("endpoint config %q not found", name)
	return sidbResolvedServiceEndpointConfig{}
}

func TestSIDBUnit_ResolveServiceEndpointConfigUsesLoadBalancerDefaults(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					TCP:  &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
					TCPS: &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	if cfg.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected load balancer type, got %q", cfg.Type)
	}
	if cfg.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyCluster {
		t.Fatalf("expected default external traffic policy Cluster, got %q", cfg.ExternalTrafficPolicy)
	}
	if !cfg.TCPEnabled || cfg.TCPServicePort != dbcommons.CONTAINER_LISTENER_PORT {
		t.Fatalf("expected default tcp load balancer port %d, got %#v", dbcommons.CONTAINER_LISTENER_PORT, cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSServicePort != dbcommons.CONTAINER_TCPS_PORT {
		t.Fatalf("expected default tcps load balancer port %d, got %#v", dbcommons.CONTAINER_TCPS_PORT, cfg)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigKeepsClusterTCPDefault(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameCluster,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeClusterIP,
					TCP:  &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true, Port: 1522},
					TCPS: &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true, Port: 2485},
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameCluster)
	if cfg.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("expected clusterip type, got %q", cfg.Type)
	}
	if cfg.ExternalTrafficPolicy != "" {
		t.Fatalf("expected no external traffic policy for clusterip service, got %q", cfg.ExternalTrafficPolicy)
	}
	if !cfg.TCPEnabled || cfg.TCPServicePort != dbcommons.CONTAINER_LISTENER_PORT || cfg.TCPNodePort != 0 {
		t.Fatalf("expected cluster tcp default port %d, got %#v", dbcommons.CONTAINER_LISTENER_PORT, cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSServicePort != 2485 || cfg.TCPSNodePort != 0 {
		t.Fatalf("expected explicit tcps clusterip port 2485, got %#v", cfg)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigUsesRequestedExternalTrafficPolicy(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name:                  dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type:                  dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
					TCP:                   &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	if cfg.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("expected external traffic policy Local, got %q", cfg.ExternalTrafficPolicy)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigCarriesKeepFlag(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name:   dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type:   dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					IsKeep: true,
					TCP:    &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	if !cfg.IsKeep {
		t.Fatalf("expected endpoint service keep flag to propagate, got %#v", cfg)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigUsesLegacyExternalService(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-cman-peer", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Services: &dbapi.SingleInstanceDatabaseServices{
				External: &dbapi.SingleInstanceDatabaseServiceEndpoint{
					Type:                  dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					IsKeep:                true,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
					Annotations: map[string]string{
						externalDNSHostnameAnnotation: "sidb-cman-peer.internal.example.com",
					},
					TCP: &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true, Port: 1521},
				},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	if cfg.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected legacy external service to resolve as LoadBalancer, got %q", cfg.Type)
	}
	if cfg.ServiceName != "sidb-cman-peer-lb" {
		t.Fatalf("expected legacy external service to use managed loadbalancer service name, got %q", cfg.ServiceName)
	}
	if !cfg.IsKeep {
		t.Fatalf("expected legacy external service keep flag to propagate, got %#v", cfg)
	}
	if cfg.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("expected external traffic policy Local, got %q", cfg.ExternalTrafficPolicy)
	}
	if cfg.TCPServicePort != 1521 {
		t.Fatalf("expected legacy external tcp port 1521, got %#v", cfg)
	}

	annotations := desiredSIDBServiceAnnotations(sidb, &cfg)
	if annotations[externalDNSHostnameAnnotation] != "sidb-cman-peer.internal.example.com" {
		t.Fatalf("expected legacy external annotations to be applied, got %#v", annotations)
	}
}

func TestSIDBUnit_CreateOrReplaceSVCCreatesLegacyExternalLoadBalancerService(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-cman-peer", Namespace: "default", UID: types.UID("sidb-cman-peer")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "ORCLCMN",
			Pdbname:  "APPPDB2",
			CreateAs: "primary",
			Services: &dbapi.SingleInstanceDatabaseServices{
				External: &dbapi.SingleInstanceDatabaseServiceEndpoint{
					Type:                  dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyLocal,
					Annotations: map[string]string{
						externalDNSHostnameAnnotation: "sidb-cman-peer.internal.example.com",
					},
					TCP: &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true, Port: 1521},
				},
			},
		},
	}
	clusterCfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameCluster)
	clusterSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sidb.Name,
			Namespace:   sidb.Namespace,
			Labels:      map[string]string{"app": sidb.Name},
			Annotations: desiredSIDBServiceAnnotations(sidb, &clusterCfg),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 map[string]string{"app": sidb.Name},
			Ports:                    desiredSIDBServiceEndpointPorts(clusterCfg),
			PublishNotReadyAddresses: true,
			Type:                     corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(sidb, clusterSvc, scheme); err != nil {
		t.Fatalf("failed to set cluster service owner reference: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, clusterSvc).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	got, err := reconciler.createOrReplaceSVC(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sidb.Name, Namespace: sidb.Namespace}}, sidb)
	if err != nil {
		t.Fatalf("createOrReplaceSVC returned error: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after creating legacy external loadbalancer service, got %#v", got)
	}

	lbSvc := &corev1.Service{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "sidb-cman-peer-lb", Namespace: "default"}, lbSvc); err != nil {
		t.Fatalf("expected legacy external loadbalancer service to be created: %v", err)
	}
	if lbSvc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected LoadBalancer service, got %q", lbSvc.Spec.Type)
	}
	if lbSvc.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("expected external traffic policy Local, got %q", lbSvc.Spec.ExternalTrafficPolicy)
	}
	if lbSvc.Annotations[externalDNSHostnameAnnotation] != "sidb-cman-peer.internal.example.com" {
		t.Fatalf("expected external DNS annotation on service, got %#v", lbSvc.Annotations)
	}
	listenerPort := servicePortByName(lbSvc.Spec.Ports, "listener")
	if listenerPort == nil || listenerPort.Port != 1521 || listenerPort.TargetPort.IntVal != 1521 {
		t.Fatalf("expected listener port 1521 to target 1521, got %#v", lbSvc.Spec.Ports)
	}
}

func TestSIDBUnit_CreateOrReplaceSVCRepairsEndpointServiceOwnerReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node1", Namespace: "default", UID: types.UID("tck8node1")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "TCK8DB1",
			CreateAs: "truecache",
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					TCP:  &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
				}},
			},
		},
	}
	clusterCfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameCluster)

	clusterSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sidb.Name,
			Namespace:   sidb.Namespace,
			Labels:      map[string]string{"app": sidb.Name},
			Annotations: desiredSIDBServiceAnnotations(sidb, &clusterCfg),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 map[string]string{"app": sidb.Name},
			Ports:                    desiredSIDBServiceEndpointPorts(clusterCfg),
			PublishNotReadyAddresses: true,
			Type:                     corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(sidb, clusterSvc, scheme); err != nil {
		t.Fatalf("failed to set cluster service owner reference: %v", err)
	}

	externalCfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	extSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        externalCfg.ServiceName,
			Namespace:   sidb.Namespace,
			Labels:      map[string]string{"app": sidb.Name},
			Annotations: desiredSIDBServiceAnnotations(sidb, &externalCfg),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 map[string]string{"app": sidb.Name},
			Ports:                    desiredSIDBServiceEndpointPorts(externalCfg),
			PublishNotReadyAddresses: shouldPublishNotReadyExternalService(sidb),
			Type:                     externalCfg.Type,
			ExternalTrafficPolicy:    externalCfg.ExternalTrafficPolicy,
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, clusterSvc, extSvc).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	got, err := reconciler.createOrReplaceSVC(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sidb.Name, Namespace: sidb.Namespace}}, sidb)
	if err != nil {
		t.Fatalf("createOrReplaceSVC returned error: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after repairing endpoint service owner references, got %#v", got)
	}

	updatedExtSvc := &corev1.Service{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: extSvc.Name, Namespace: extSvc.Namespace}, updatedExtSvc); err != nil {
		t.Fatalf("failed to get endpoint service: %v", err)
	}
	if len(updatedExtSvc.OwnerReferences) != 1 {
		t.Fatalf("expected endpoint service owner reference to be restored, got %#v", updatedExtSvc.OwnerReferences)
	}
	if updatedExtSvc.OwnerReferences[0].UID != sidb.UID {
		t.Fatalf("expected owner UID %q, got %#v", sidb.UID, updatedExtSvc.OwnerReferences)
	}
	if updatedExtSvc.OwnerReferences[0].Controller == nil || !*updatedExtSvc.OwnerReferences[0].Controller {
		t.Fatalf("expected restored owner reference to be controller, got %#v", updatedExtSvc.OwnerReferences)
	}
}

func TestSIDBUnit_CreateOrReplaceSVCClearsEndpointServiceOwnerReferencesWhenKeepEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tck8node1", Namespace: "default", UID: types.UID("tck8node1")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "TCK8DB1",
			CreateAs: "truecache",
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name:   dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type:   dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					IsKeep: true,
					TCP:    &dbapi.SingleInstanceDatabaseServiceEndpointPort{Enabled: true},
				}},
			},
		},
	}
	clusterCfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameCluster)

	clusterSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sidb.Name,
			Namespace:   sidb.Namespace,
			Labels:      map[string]string{"app": sidb.Name},
			Annotations: desiredSIDBServiceAnnotations(sidb, &clusterCfg),
		},
		Spec: corev1.ServiceSpec{
			Selector:                 map[string]string{"app": sidb.Name},
			Ports:                    desiredSIDBServiceEndpointPorts(clusterCfg),
			PublishNotReadyAddresses: true,
			Type:                     corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(sidb, clusterSvc, scheme); err != nil {
		t.Fatalf("failed to set cluster service owner reference: %v", err)
	}

	externalCfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	controller := true
	extSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        externalCfg.ServiceName,
			Namespace:   sidb.Namespace,
			Labels:      map[string]string{"app": sidb.Name},
			Annotations: desiredSIDBServiceAnnotations(sidb, &externalCfg),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: dbapi.GroupVersion.String(),
				Kind:       "SingleInstanceDatabase",
				Name:       sidb.Name,
				UID:        sidb.UID,
				Controller: &controller,
			}},
		},
		Spec: corev1.ServiceSpec{
			Selector:                 map[string]string{"app": sidb.Name},
			Ports:                    desiredSIDBServiceEndpointPorts(externalCfg),
			PublishNotReadyAddresses: shouldPublishNotReadyExternalService(sidb),
			Type:                     externalCfg.Type,
			ExternalTrafficPolicy:    externalCfg.ExternalTrafficPolicy,
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb, clusterSvc, extSvc).Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	got, err := reconciler.createOrReplaceSVC(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: sidb.Name, Namespace: sidb.Namespace}}, sidb)
	if err != nil {
		t.Fatalf("createOrReplaceSVC returned error: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after clearing endpoint service owner references, got %#v", got)
	}

	updatedExtSvc := &corev1.Service{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: extSvc.Name, Namespace: extSvc.Namespace}, updatedExtSvc); err != nil {
		t.Fatalf("failed to get endpoint service: %v", err)
	}
	if len(updatedExtSvc.OwnerReferences) != 0 {
		t.Fatalf("expected endpoint service owner references to be cleared when keep is enabled, got %#v", updatedExtSvc.OwnerReferences)
	}
}

func TestSIDBUnit_DesiredSIDBServiceAnnotationsUsesEndpointOverrides(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ServiceAnnotations: map[string]string{
				"legacy": "value",
				"shared": "legacy",
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					Annotations: map[string]string{
						"shared": "external",
						"new":    "value",
					},
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	got := desiredSIDBServiceAnnotations(sidb, &cfg)
	want := map[string]string{
		"legacy": "value",
		"shared": "external",
		"new":    "value",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected endpoint annotations: got %#v want %#v", got, want)
	}
}

func TestSIDBUnit_DesiredSIDBServiceAnnotationsKeepsLegacyFallbackForEndpointService(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ServiceAnnotations: map[string]string{
				"legacy": "value",
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
				}},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer)
	got := desiredSIDBServiceAnnotations(sidb, &cfg)
	want := map[string]string{"legacy": "value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected legacy fallback annotations: got %#v want %#v", got, want)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigRetainsLegacyCompatibility(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ListenerPort:     32001,
			TcpsListenerPort: 32002,
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameNodePort)
	if cfg.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("expected nodeport type, got %q", cfg.Type)
	}
	if !cfg.TCPEnabled || cfg.TCPNodePort != 32001 {
		t.Fatalf("expected legacy tcp nodeport 32001, got %#v", cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSNodePort != 32002 {
		t.Fatalf("expected legacy tcps nodeport 32002, got %#v", cfg)
	}
}

func TestSIDBUnit_ResolveServiceEndpointConfigTreatsLegacyNonNodePortAsServicePort(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			ListenerPort:     1522,
			TcpsListenerPort: 2484,
			Security: &dbapi.SingleInstanceDatabaseSecurity{
				TCPS: &dbapi.SingleInstanceDatabaseSecurityTCPS{Enabled: true},
			},
		},
	}

	cfg := resolvedSIDBEndpointConfigForTest(t, sidb, dbapi.SingleInstanceDatabaseServiceEndpointNameNodePort)
	if cfg.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("expected nodeport type, got %q", cfg.Type)
	}
	if !cfg.TCPEnabled || cfg.TCPServicePort != 1522 || cfg.TCPNodePort != 0 {
		t.Fatalf("expected legacy tcp port 1522 with auto nodePort, got %#v", cfg)
	}
	if !cfg.TCPSEnabled || cfg.TCPSServicePort != 2484 || cfg.TCPSNodePort != 0 {
		t.Fatalf("expected legacy tcps port 2484 with auto nodePort, got %#v", cfg)
	}
}

func TestSIDBUnit_HasExpectedTCPSListenerEndpointRequiresContainerTCPSPort(t *testing.T) {
	listenerStatus := `
Listening Endpoints Summary...
  (DESCRIPTION=(ADDRESS=(PROTOCOL=ipc)(KEY=EXTPROC1)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=0.0.0.0)(PORT=1521)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=0.0.0.0)(PORT=2484)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=sidb-standby)(PORT=5500))(Security=(my_wallet_directory=/opt/oracle/admin/STBYDB/xdb_wallet))(Presentation=HTTP)(Session=RAW))
`
	if !hasExpectedTCPSListenerEndpoint(listenerStatus, dbcommons.CONTAINER_TCPS_PORT) {
		t.Fatalf("expected tcps listener endpoint on port %d to be detected", dbcommons.CONTAINER_TCPS_PORT)
	}
}

func TestSIDBUnit_HasExpectedTCPSListenerEndpointIgnoresXDBTCPSOnly(t *testing.T) {
	listenerStatus := `
Listening Endpoints Summary...
  (DESCRIPTION=(ADDRESS=(PROTOCOL=ipc)(KEY=EXTPROC1)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=0.0.0.0)(PORT=1521)))
  (DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=sidb-standby)(PORT=5500))(Security=(my_wallet_directory=/opt/oracle/admin/STBYDB/xdb_wallet))(Presentation=HTTP)(Session=RAW))
`
	if hasExpectedTCPSListenerEndpoint(listenerStatus, dbcommons.CONTAINER_TCPS_PORT) {
		t.Fatalf("did not expect xdb-only tcps endpoint to satisfy listener validation")
	}
}

func TestSIDBUnit_GetTcpsEnabledTreatsLegacyListenerPortAsEnabled(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TcpsListenerPort: 2484,
		},
	}

	if !getTcpsEnabled(sidb) {
		t.Fatalf("expected deprecated spec.tcpsListenerPort to imply tcps enabled")
	}

	sidb = &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			TcpsListenerPort: 32002,
		},
	}

	if !getTcpsEnabled(sidb) {
		t.Fatalf("expected deprecated spec.tcpsListenerPort to imply tcps enabled")
	}
}

func TestSIDBUnit_AllowTrueCacheBlobGenerationWithoutTCPS(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "primary",
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				GenerateBlob: true,
			},
		},
	}
	if !allowTrueCacheBlobGenerationWithoutTCPS(sidb) {
		t.Fatalf("expected primary truecache blob generation to bypass tcps blocking")
	}

	sidb.Spec.TrueCache.GenerateBlob = false
	sidb.Spec.TrueCache.GenerateEnabled = true
	if !allowTrueCacheBlobGenerationWithoutTCPS(sidb) {
		t.Fatalf("expected deprecated generateEnabled to continue bypassing tcps blocking")
	}

	sidb.Spec.TrueCache.GenerateEnabled = false
	if allowTrueCacheBlobGenerationWithoutTCPS(sidb) {
		t.Fatalf("did not expect bypass when blob generation is disabled")
	}

	sidb.Spec.TrueCache.GenerateBlob = true
	sidb.Spec.CreateAs = "truecache"
	if allowTrueCacheBlobGenerationWithoutTCPS(sidb) {
		t.Fatalf("did not expect bypass for non-primary sidb")
	}
}

func TestSIDBUnit_TrueCacheBlobFeatureFlags(t *testing.T) {
	tc := &dbapi.SingleInstanceDatabaseTrueCacheSpec{}
	if tc.BlobGenerationEnabled() {
		t.Fatalf("did not expect blob generation to be enabled by default")
	}
	if tc.BlobConfigMapCreationEnabled() {
		t.Fatalf("did not expect configmap creation to be enabled by default")
	}

	tc.GenerateBlob = true
	if !tc.BlobGenerationEnabled() {
		t.Fatalf("expected generateBlob=true to enable blob generation")
	}
	if tc.BlobConfigMapCreationEnabled() {
		t.Fatalf("did not expect generateBlob=true alone to enable configmap creation")
	}

	tc.GenerateBlob = false
	tc.CreateConfigMap = true
	if tc.BlobGenerationEnabled() {
		t.Fatalf("did not expect createConfigMap=true alone to enable blob generation")
	}
	if !tc.BlobConfigMapCreationEnabled() {
		t.Fatalf("expected createConfigMap=true to enable configmap creation")
	}

	tc.CreateConfigMap = false
	tc.GenerateEnabled = true
	if !tc.BlobGenerationEnabled() || !tc.BlobConfigMapCreationEnabled() {
		t.Fatalf("expected deprecated generateEnabled=true to enable both behaviors")
	}
}

func TestSIDBUnit_ResolveTrueCacheBlobGenerationPathsForFileTarget(t *testing.T) {
	generationPath, materializedPath := resolveTrueCacheBlobGenerationPaths("/tmp/tc_config_blob.tar.gz")

	if generationPath != "/tmp/tc_config_blob.tar.gz.dir" {
		t.Fatalf("unexpected generation path: %q", generationPath)
	}
	if materializedPath != "/tmp/tc_config_blob.tar.gz" {
		t.Fatalf("unexpected materialized path: %q", materializedPath)
	}
}

func TestSIDBUnit_ResolveTrueCacheBlobGenerationPathsForDirectoryTarget(t *testing.T) {
	generationPath, materializedPath := resolveTrueCacheBlobGenerationPaths("/tmp/tc_blob_output")

	if generationPath != "/tmp/tc_blob_output" {
		t.Fatalf("unexpected generation path: %q", generationPath)
	}
	if materializedPath != "" {
		t.Fatalf("expected no materialized path for directory target, got %q", materializedPath)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobResolveCommandForFileTarget(t *testing.T) {
	cmd := buildTrueCacheBlobResolveCommand("/tmp/tc_config_blob.tar.gz.dir", "/tmp/tc_config_blob.tar.gz")

	if !strings.Contains(cmd, "[ -f '/tmp/tc_config_blob.tar.gz' ]") {
		t.Fatalf("expected file existence check in resolve command, got %q", cmd)
	}
	if !strings.Contains(cmd, "[ -d '/tmp/tc_config_blob.tar.gz.dir' ]") {
		t.Fatalf("expected staging directory check in resolve command, got %q", cmd)
	}
	if !strings.Contains(cmd, "'/tmp/tc_config_blob.tar.gz.dir'/*.tar.gz") {
		t.Fatalf("expected staging directory scan in resolve command, got %q", cmd)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobResolveCommandForDirectoryTarget(t *testing.T) {
	cmd := buildTrueCacheBlobResolveCommand("/tmp/tc_blob_output", "")

	if !strings.Contains(cmd, "[ -d '/tmp/tc_blob_output' ]") {
		t.Fatalf("expected directory existence check in resolve command, got %q", cmd)
	}
	if !strings.Contains(cmd, "'/tmp/tc_blob_output'/*.tar.gz") {
		t.Fatalf("expected directory tarball scan in resolve command, got %q", cmd)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobGenerateCommandForFileTarget(t *testing.T) {
	cmd := buildTrueCacheBlobGenerateCommand("/tmp/tc_config_blob.tar.gz.dir", "/tmp/tc_config_blob.tar.gz", "/etc/secrets/tde", "ORCLPRD")

	if !strings.Contains(cmd, "rm -rf '/tmp/tc_config_blob.tar.gz.dir' '/tmp/tc_config_blob.tar.gz'") {
		t.Fatalf("expected staging directory and file cleanup in generate command, got %q", cmd)
	}
	if !strings.Contains(cmd, "mkdir -p '/tmp/tc_config_blob.tar.gz.dir'") {
		t.Fatalf("expected staging directory creation in generate command, got %q", cmd)
	}
	if !strings.Contains(cmd, "-trueCacheBlobLocation '/tmp/tc_config_blob.tar.gz.dir'") {
		t.Fatalf("expected staging directory output path in generate command, got %q", cmd)
	}
	if !strings.Contains(cmd, "cp \"$latest\" '/tmp/tc_config_blob.tar.gz'") {
		t.Fatalf("expected materialization copy in generate command, got %q", cmd)
	}
	if strings.Contains(cmd, "cat '/etc/secrets/tde' |") {
		t.Fatalf("did not expect password file to be piped to dbca, got %q", cmd)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobGenerateCommandForDirectoryTarget(t *testing.T) {
	cmd := buildTrueCacheBlobGenerateCommand("/tmp/tc_blob_output", "", "/etc/secrets/tde", "ORCLPRD")

	if !strings.Contains(cmd, "rm -rf '/tmp/tc_blob_output'") {
		t.Fatalf("expected directory cleanup in generate command, got %q", cmd)
	}
	if !strings.Contains(cmd, "mkdir -p '/tmp/tc_blob_output'") {
		t.Fatalf("expected directory creation in generate command, got %q", cmd)
	}
	if !strings.Contains(cmd, "-trueCacheBlobLocation '/tmp/tc_blob_output'") {
		t.Fatalf("expected directory output path in generate command, got %q", cmd)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobExistsCommandForFileTarget(t *testing.T) {
	cmd := buildTrueCacheBlobExistsCommand("/tmp/tc_config_blob.tar.gz.dir", "/tmp/tc_config_blob.tar.gz")

	if !strings.Contains(cmd, "[ -f '/tmp/tc_config_blob.tar.gz' ]") {
		t.Fatalf("expected file existence check in exists command, got %q", cmd)
	}
	if strings.Contains(cmd, ".tar.gz.dir/*.tar.gz") {
		t.Fatalf("did not expect staging directory scan in file existence command, got %q", cmd)
	}
}

func TestSIDBUnit_BuildTrueCacheBlobExistsCommandForDirectoryTarget(t *testing.T) {
	cmd := buildTrueCacheBlobExistsCommand("/tmp/tc_blob_output", "")

	if !strings.Contains(cmd, "[ -d '/tmp/tc_blob_output' ]") {
		t.Fatalf("expected directory existence check in exists command, got %q", cmd)
	}
	if !strings.Contains(cmd, "'/tmp/tc_blob_output'/*.tar.gz") {
		t.Fatalf("expected tarball scan in directory existence command, got %q", cmd)
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

func TestSIDBUnit_PodHasDesiredAdditionalPVCs(t *testing.T) {
	desired := []dbapi.AdditionalPVCSpec{
		{PvcName: "scripts-pvc", MountPath: "/mnt/scripts"},
		{PvcName: "backup-pvc", MountPath: "/mnt/backup"},
	}
	basePod := corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "additional-pvc-0",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "scripts-pvc"},
					},
				},
				{
					Name: "additional-pvc-1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "backup-pvc"},
					},
				},
			},
			Containers: []corev1.Container{{
				Name: "db",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "additional-pvc-0", MountPath: "/mnt/scripts"},
					{Name: "additional-pvc-1", MountPath: "/mnt/backup"},
				},
			}},
		},
	}

	if !podHasDesiredAdditionalPVCs(basePod, desired) {
		t.Fatalf("expected pod additionalPVC mounts to match desired spec")
	}

	claimDriftPod := basePod.DeepCopy()
	claimDriftPod.Spec.Volumes[1].PersistentVolumeClaim.ClaimName = "different-backup-pvc"
	if podHasDesiredAdditionalPVCs(*claimDriftPod, desired) {
		t.Fatalf("expected claim-name drift to be detected")
	}

	extraMountPod := basePod.DeepCopy()
	extraMountPod.Spec.Volumes = append(extraMountPod.Spec.Volumes, corev1.Volume{
		Name: "additional-pvc-2",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "logs-pvc"},
		},
	})
	extraMountPod.Spec.Containers[0].VolumeMounts = append(extraMountPod.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: "additional-pvc-2", MountPath: "/mnt/logs"})
	if podHasDesiredAdditionalPVCs(*extraMountPod, desired) {
		t.Fatalf("expected stale additionalPVC mounts to be detected")
	}
}

func TestSIDBUnit_PodHasDesiredCustomScriptsPVC(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name: "custom-scripts-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "sidb-scripts-new"},
				},
			}},
		},
	}

	if !podHasDesiredCustomScriptsPVC(pod, "sidb-scripts-new") {
		t.Fatalf("expected pod custom-scripts PVC to match desired claim")
	}
	if podHasDesiredCustomScriptsPVC(pod, "sidb-scripts-old") {
		t.Fatalf("expected mismatched custom-scripts PVC claim to be detected")
	}
	if podHasDesiredCustomScriptsPVC(pod, "") {
		t.Fatalf("expected dedicated scripts PVC to be rejected when no dedicated claim is desired")
	}

	emptyDirPod := corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name:         "custom-scripts-vol",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}},
		},
	}
	if !podHasDesiredCustomScriptsPVC(emptyDirPod, "") {
		t.Fatalf("expected emptyDir scripts volume to match no dedicated scripts claim")
	}
}

func TestSIDBUnit_PodHasDesiredSIDBScriptMountsExplicitScripts(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Scripts: &dbapi.SingleInstanceDatabaseScriptsSpec{
				Setup:   &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "setup-pvc"},
				Startup: &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "startup-pvc"},
			},
		},
	}
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "scripts-setup-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "setup-pvc"},
					},
				},
				{
					Name: "scripts-startup-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "startup-pvc"},
					},
				},
			},
			Containers: []corev1.Container{{
				Name: "db",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "scripts-setup-vol", MountPath: "/opt/oracle/scripts/setup/"},
					{Name: "scripts-startup-vol", MountPath: "/opt/oracle/scripts/startup/"},
				},
			}},
		},
	}

	if !podHasDesiredSIDBScriptMounts(pod, sidb) {
		t.Fatalf("expected explicit spec.scripts mounts to match desired state")
	}

	driftPod := pod.DeepCopy()
	driftPod.Spec.Volumes[1].PersistentVolumeClaim.ClaimName = "old-startup-pvc"
	if podHasDesiredSIDBScriptMounts(*driftPod, sidb) {
		t.Fatalf("expected startup PVC drift to be detected")
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

func TestSIDBUnit_ValidateRestoreSpecRefsObjectStoreOpcInstallerZipBinaryData(t *testing.T) {
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
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "opc-installer-zipfile", Namespace: "ns1"},
				BinaryData: map[string][]byte{"opc_installer_zipfile": []byte("zip-bytes")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "sshkeysecret", Namespace: "ns1"},
				Data:       map[string][]byte{"oci_api_key.pem": []byte("key")},
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
					OCIConfig:       &dbapi.SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "ociconfig", Key: "oci.env"},
					PrivateKey:      &dbapi.SingleInstanceDatabaseSecretKeyRef{SecretName: "sshkeysecret", Key: "oci_api_key.pem"},
					OpcInstallerZip: &dbapi.SingleInstanceDatabaseConfigMapKeyRef{ConfigMapName: "opc-installer-zipfile", Key: "opc_installer_zipfile"},
					BackupIdentity:  &dbapi.SingleInstanceDatabaseBackupIdentity{DBID: "1234567890"},
				},
			},
		},
	}
	if err := ValidateRestoreSpecRefs(reconciler, sidb, ctx); err != nil {
		t.Fatalf("expected binaryData-backed opcInstallerZip ref to validate, got err: %v", err)
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
	got, err := getFraRecoveryAreaSize(sidb)
	if err != nil {
		t.Fatalf("expected FRA recovery area size normalization to succeed, got err: %v", err)
	}
	if got != "120G" {
		t.Fatalf("expected FRA recovery area size to normalize from fra.size, got %q", got)
	}
}

func TestSIDBUnit_GetFraRecoveryAreaSizeRejectsInvalidLiteral(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					RecoveryAreaSize: "50G scope=both; alter system set open_cursors=9999; --",
				},
			},
		},
	}

	if _, err := getFraRecoveryAreaSize(sidb); err == nil {
		t.Fatalf("expected invalid FRA recovery area size to be rejected")
	}
}

func TestSIDBUnit_PodHasDesiredSIDBFraMount(t *testing.T) {
	sidb := &dbapi.SingleInstanceDatabase{
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					PvcName:          "fra-pvc",
					RecoveryAreaSize: "100Gi",
					MountPath:        "/u02/fra",
				},
			},
		},
	}

	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{
				Name: "fra-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "fra-pvc"},
				},
			}},
			Containers: []corev1.Container{{
				Name: "db",
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "fra-vol",
					MountPath: "/u02/fra",
				}},
			}},
		},
	}

	if !podHasDesiredSIDBFraMount(pod, sidb) {
		t.Fatalf("expected FRA mount to match desired state")
	}

	driftPod := pod.DeepCopy()
	driftPod.Spec.Containers[0].VolumeMounts[0].MountPath = "/u03/fra"
	if podHasDesiredSIDBFraMount(*driftPod, sidb) {
		t.Fatalf("expected FRA mount path drift to be detected")
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

func TestSIDBUnit_InstantiatePodSpecDefaultsCapabilitiesToSysNice(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-default-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{Add: []corev1.Capability{"SYS_NICE"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected default capabilities %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiatePodSpecDoesNotMountDefaultShm(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-default-shm", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == sidbShmVolumeName {
			t.Fatalf("did not expect %s volume when shmSize is omitted", sidbShmVolumeName)
		}
	}
	for _, mount := range pod.Spec.Containers[0].VolumeMounts {
		if mount.Name == sidbShmVolumeName || mount.MountPath == sidbShmMountPath {
			t.Fatalf("did not expect %s mount when shmSize is omitted, got %#v", sidbShmMountPath, pod.Spec.Containers[0].VolumeMounts)
		}
	}
}

func TestSIDBUnit_InstantiatePodSpecUsesExplicitShmSize(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-custom-shm", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:     "ORCLCDB",
			ShmSize: "8Gi",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	for i := range pod.Spec.Volumes {
		volume := &pod.Spec.Volumes[i]
		if volume.Name != sidbShmVolumeName {
			continue
		}
		if volume.EmptyDir == nil || volume.EmptyDir.SizeLimit == nil || volume.EmptyDir.SizeLimit.Cmp(resource.MustParse("8Gi")) != 0 {
			t.Fatalf("expected explicit shm size 8Gi, got %#v", volume.EmptyDir)
		}
		return
	}
	t.Fatalf("expected %s volume, got %#v", sidbShmVolumeName, pod.Spec.Volumes)
}

func TestSIDBUnit_InstantiatePodSpecAllowsEmptyCapabilitiesOverride(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-empty-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:          "ORCLCDB",
			Capabilities: &corev1.Capabilities{},
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected empty capability override %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiatePodSpecUsesExplicitCapabilitiesOverride(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-custom-caps", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid: "ORCLCDB",
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{"NET_RAW"},
				Drop: []corev1.Capability{"ALL"},
			},
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	got := pod.Spec.Containers[0].SecurityContext.Capabilities
	want := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_RAW"},
		Drop: []corev1.Capability{"ALL"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit capability override %#v, got %#v", want, got)
	}
}

func TestSIDBUnit_InstantiatePodSpecUsesExplicitScriptsPVCs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-explicit-scripts", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Sid:      "ORCLCDB",
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Scripts: &dbapi.SingleInstanceDatabaseScriptsSpec{
				Setup:   &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "setup-pvc"},
				Startup: &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "startup-pvc"},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	volumeClaims := map[string]string{}
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			volumeClaims[volume.Name] = volume.PersistentVolumeClaim.ClaimName
		}
	}
	if volumeClaims["scripts-setup-vol"] != "setup-pvc" {
		t.Fatalf("expected scripts-setup-vol to use setup-pvc, got %q", volumeClaims["scripts-setup-vol"])
	}
	if volumeClaims["scripts-startup-vol"] != "startup-pvc" {
		t.Fatalf("expected scripts-startup-vol to use startup-pvc, got %q", volumeClaims["scripts-startup-vol"])
	}

	setupFound := false
	startupFound := false
	for _, mount := range pod.Spec.Containers[0].VolumeMounts {
		if mount.Name == "scripts-setup-vol" && mount.MountPath == "/opt/oracle/scripts/setup/" {
			setupFound = true
		}
		if mount.Name == "scripts-startup-vol" && mount.MountPath == "/opt/oracle/scripts/startup/" {
			startupFound = true
		}
	}
	if !setupFound || !startupFound {
		t.Fatalf("expected explicit scripts volume mounts in pod spec, setup=%t startup=%t", setupFound, startupFound)
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

func TestSIDBUnit_InstantiateTrueCachePodSpecPrefersExternalDNSHostname(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "truecache-nlb",
					Namespace: "ns1",
					Annotations: map[string]string{
						externalDNSHostnameAnnotation: "truecache-production.internal.example.com",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{"app": "tc1"},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{IP: "10.2.1.55"}},
					},
				},
			},
		).Build(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	var oracleHostname string
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "ORACLE_HOSTNAME" {
			oracleHostname = env.Value
			break
		}
	}
	if oracleHostname != "truecache-production.internal.example.com" {
		t.Fatalf("expected ORACLE_HOSTNAME to prefer external DNS hostname over load balancer ingress IP, got %q", oracleHostname)
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecSkipsLegacyAdminPasswordVolume(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
				ConnectString: "primary.example.com:1521/ORCLPRD",
			},
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				DBCredentialsWallet: &dbapi.SingleInstanceDatabaseTrueCacheDBCredentialsWallet{
					SecretName: "primary-db-cred-wallet",
					MountPath:  "/u01/app/oracle/db_wallet",
				},
				TrueCacheServices: []string{"APPPDB1:tpdb_primary:tpdb_cache"},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	for _, volume := range pod.Spec.Volumes {
		if volume.Name == "oracle-pwd-vol" {
			t.Fatalf("expected wallet-only truecache pod spec to skip oracle-pwd-vol, got %#v", volume)
		}
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecPrefersExternalDNSHostnameFromNodePortService(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "truecache-nodeport",
					Namespace: "ns1",
					Annotations: map[string]string{
						externalDNSHostnameAnnotation: "truecache-production.internal.example.com",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeNodePort,
					Selector: map[string]string{"app": "tc1"},
				},
			},
		).Build(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	var oracleHostname string
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "ORACLE_HOSTNAME" {
			oracleHostname = env.Value
			break
		}
	}
	if oracleHostname != "truecache-production.internal.example.com" {
		t.Fatalf("expected ORACLE_HOSTNAME to use external DNS hostname from NodePort service, got %q", oracleHostname)
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecPrefersSpecExternalDNSHostnameBeforeServiceExists(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Services: &dbapi.SingleInstanceDatabaseServices{
				Endpoints: []dbapi.SingleInstanceDatabaseServiceEndpoint{{
					Name: dbapi.SingleInstanceDatabaseServiceEndpointNameLoadBalancer,
					Type: dbapi.SingleInstanceDatabaseServiceEndpointTypeLoadBalancer,
					Annotations: map[string]string{
						externalDNSHostnameAnnotation: "truecache-production.internal.example.com",
					},
				}},
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	var oracleHostname string
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "ORACLE_HOSTNAME" {
			oracleHostname = env.Value
			break
		}
	}
	if oracleHostname != "truecache-production.internal.example.com" {
		t.Fatalf("expected ORACLE_HOSTNAME to use spec external DNS hostname before service creation, got %q", oracleHostname)
	}
}

func TestSIDBUnit_InstantiateTrueCachePodSpecFallsBackToClusterServiceHostname(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "tc1", Namespace: "ns1"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Sid:      "ORCLCDB",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
	}

	pod, err := reconciler.instantiatePodSpec(sidb, nil, nil, false)
	if err != nil {
		t.Fatalf("instantiatePodSpec returned err: %v", err)
	}

	var oracleHostname string
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "ORACLE_HOSTNAME" {
			oracleHostname = env.Value
			break
		}
	}
	if oracleHostname != "tc1.ns1.svc.cluster.local" {
		t.Fatalf("expected ORACLE_HOSTNAME to fall back to cluster service hostname, got %q", oracleHostname)
	}
}

func TestSIDBUnit_InstantiatePodSpecPrefersSeparateNodeFromLocalPrimary(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard(), Scheme: scheme}

	t.Run("standby local primary adds anti-affinity and preserves nodeSelector", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-standby", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "standby",
				Sid:      "STBY",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					DatabaseRef: "primary-db",
				},
				NodeSelector: map[string]string{"db-role": "ha"},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if got := pod.Spec.NodeSelector["db-role"]; got != "ha" {
			t.Fatalf("expected nodeSelector to be preserved, got %q", got)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected pod anti-affinity to be configured")
		}
		terms := pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(terms) == 0 {
			t.Fatalf("expected preferred anti-affinity terms")
		}
		last := terms[len(terms)-1]
		if last.Weight != 100 {
			t.Fatalf("unexpected anti-affinity weight: %d", last.Weight)
		}
		values := last.PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
		if len(values) != 1 || values[0] != "primary-db" {
			t.Fatalf("unexpected anti-affinity target values: %#v", values)
		}
		if last.PodAffinityTerm.TopologyKey != "kubernetes.io/hostname" {
			t.Fatalf("unexpected topology key: %q", last.PodAffinityTerm.TopologyKey)
		}
	})

	t.Run("truecache local primary adds anti-affinity", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-tc", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "truecache",
				Sid:      "TCDB",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					DatabaseRef: "primary-db",
				},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected truecache pod anti-affinity to be configured")
		}
		terms := pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		if len(terms) == 0 {
			t.Fatalf("expected preferred anti-affinity terms for truecache")
		}
		values := terms[len(terms)-1].PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
		if len(values) != 1 || values[0] != "primary-db" {
			t.Fatalf("unexpected truecache anti-affinity target values: %#v", values)
		}
	})

	t.Run("external primary source does not add anti-affinity", func(t *testing.T) {
		sidb := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "sidb-ext", Namespace: "ns1"},
			Spec: dbapi.SingleInstanceDatabaseSpec{
				CreateAs: "standby",
				Sid:      "STBY",
				Image: dbapi.SingleInstanceDatabaseImage{
					PullFrom: "container-registry.oracle.com/database/free:latest",
				},
				PrimarySource: &dbapi.SingleInstanceDatabasePrimarySource{
					ConnectString: "primary-host:1521/PRIM",
				},
			},
		}
		primary := &dbapi.SingleInstanceDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "primary-db", Namespace: "ns1"},
		}

		pod, err := reconciler.instantiatePodSpec(sidb, nil, primary, false)
		if err != nil {
			t.Fatalf("instantiatePodSpec returned err: %v", err)
		}
		if pod.Spec.Affinity == nil || pod.Spec.Affinity.PodAntiAffinity == nil {
			t.Fatalf("expected base pod anti-affinity to exist")
		}
		for _, term := range pod.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			if term.PodAffinityTerm.LabelSelector == nil || len(term.PodAffinityTerm.LabelSelector.MatchExpressions) == 0 {
				continue
			}
			values := term.PodAffinityTerm.LabelSelector.MatchExpressions[0].Values
			if len(values) == 1 && values[0] == "primary-db" {
				t.Fatalf("did not expect external primary anti-affinity term to target primary-db")
			}
		}
	})
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

func TestSIDBUnit_RunSIDBPhaseTreatsRequeueAfterAsRequeue(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}

	got, err := reconciler.runSIDBPhase(context.Background(), ctrl.Request{}, "unit_test_phase", func() (ctrl.Result, error) {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue || got.RequeueAfter != 30*time.Second {
		t.Fatalf("expected requeue with delay, got %#v", got)
	}
}

func TestSIDBUnit_EnsureTrueCacheBlobSourceReadyMissingConfigMapRequeues(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "truecache-production", Namespace: "default", Generation: 3},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				BlobConfigMapRef: "orcl-production-truecache-blob",
			},
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

	got, err := reconciler.ensureTrueCacheBlobSourceReady(
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "truecache-production"}},
		sidb,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.RequeueAfter != 30*time.Second {
		t.Fatalf("expected requeue after 30s, got %#v", got)
	}

	updated := &dbapi.SingleInstanceDatabase{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: sidb.Name, Namespace: sidb.Namespace}, updated); err != nil {
		t.Fatalf("failed to fetch updated sidb: %v", err)
	}
	if updated.Status.Status != dbcommons.StatusPending {
		t.Fatalf("expected pending status, got %q", updated.Status.Status)
	}
	condition := meta.FindStatusCondition(updated.Status.Conditions, "TrueCacheBlobSourceReady")
	if condition == nil {
		t.Fatalf("expected TrueCacheBlobSourceReady condition")
	}
	if condition.Status != metav1.ConditionFalse || condition.Reason != "WaitingForBlobConfigMap" {
		t.Fatalf("unexpected condition: %#v", condition)
	}
}

func TestSIDBUnit_CreateOrReplacePodsBlocksTrueCacheWithoutBlobConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "truecache-production", Namespace: "default", Generation: 4},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Replicas: 1,
			TrueCache: &dbapi.SingleInstanceDatabaseTrueCacheSpec{
				BlobConfigMapRef: "orcl-production-truecache-blob",
			},
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

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "truecache-production"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue || got.RequeueAfter != 30*time.Second {
		t.Fatalf("expected requeue after 30s, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected no pods to be created, got %d", len(pods.Items))
	}
}

func TestSIDBUnit_CreateOrReplacePodsRollsPodsForAdditionalPVCDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-additional-pvc", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				AdditionalPVCs: []dbapi.AdditionalPVCSpec{{
					PvcName:   "new-scripts-pvc",
					MountPath: "/mnt/scripts",
				}},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DatafilesCreated: "true",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sidb-additional-pvc-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sidb-additional-pvc",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "sidb-additional-pvc",
				Image: "container-registry.oracle.com/database/free:latest",
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "additional-pvc-0",
					MountPath: "/mnt/scripts",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "additional-pvc-0",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "old-scripts-pvc",
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "sidb-additional-pvc",
				Ready: true,
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, pod).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-additional-pvc"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after deleting pod for additionalPVC drift, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected stale pod to be deleted for additionalPVC drift, got %d pod(s)", len(pods.Items))
	}
}

func TestSIDBUnit_CreateOrReplacePodsRollsPodsForCustomScriptsPVCDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-custom-scripts", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				ScriptsVolumeName: "db-vol-patch",
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DatafilesCreated: "true",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sidb-custom-scripts-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sidb-custom-scripts",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "sidb-custom-scripts",
				Image: "container-registry.oracle.com/database/free:latest",
			}},
			Volumes: []corev1.Volume{{
				Name: "custom-scripts-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "sidb-custom-scripts-db-vol",
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "sidb-custom-scripts",
				Ready: true,
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, pod).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-custom-scripts"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after deleting pod for scriptsVolumeName drift, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected stale pod to be deleted for scriptsVolumeName drift, got %d pod(s)", len(pods.Items))
	}
}

func TestSIDBUnit_CreateOrReplacePodsRollsPodsForExplicitScriptsDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-explicit-scripts", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Scripts: &dbapi.SingleInstanceDatabaseScriptsSpec{
				Setup:   &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "setup-pvc"},
				Startup: &dbapi.SingleInstanceDatabaseScriptLocation{PvcName: "startup-pvc"},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DatafilesCreated: "true",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sidb-explicit-scripts-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sidb-explicit-scripts",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "sidb-explicit-scripts",
				Image: "container-registry.oracle.com/database/free:latest",
				VolumeMounts: []corev1.VolumeMount{
					{Name: "scripts-setup-vol", MountPath: "/opt/oracle/scripts/setup/"},
					{Name: "scripts-startup-vol", MountPath: "/opt/oracle/scripts/startup/"},
				},
			}},
			Volumes: []corev1.Volume{
				{
					Name: "scripts-setup-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "setup-pvc"},
					},
				},
				{
					Name: "scripts-startup-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "old-startup-pvc"},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "sidb-explicit-scripts",
				Ready: true,
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, pod).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-explicit-scripts"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after deleting pod for explicit scripts drift, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected stale pod to be deleted for explicit scripts drift, got %d pod(s)", len(pods.Items))
	}
}

func TestSIDBUnit_CreateOrReplacePodsRollsPodsForFraDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-fra", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Fra: &dbapi.SingleInstanceDatabasePersistenceFra{
					PvcName:          "fra-pvc",
					RecoveryAreaSize: "100Gi",
					MountPath:        "/u02/fra",
				},
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DatafilesCreated: "true",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sidb-fra-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sidb-fra",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "sidb-fra",
				Image: "container-registry.oracle.com/database/free:latest",
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "fra-vol",
					MountPath: "/opt/oracle/oradata/fast_recovery_area",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "fra-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "fra-pvc",
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "sidb-fra",
				Ready: true,
			}},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, pod).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-fra"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after deleting pod for FRA drift, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 0 {
		t.Fatalf("expected stale pod to be deleted for FRA drift, got %d pod(s)", len(pods.Items))
	}
}

func TestSIDBUnit_CreateOrReplacePodsWaitsForTerminatingPodBeforeReplacement(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-recovery", Namespace: "default"},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Replicas: 1,
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/free:latest",
			},
		},
		Status: dbapi.SingleInstanceDatabaseStatus{
			DatafilesCreated: "true",
		},
	}

	now := metav1.Now()
	terminatingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sidb-recovery-oldpod",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{"database.oracle.com/test-finalizer"},
			Labels: map[string]string{
				"app": "sidb-recovery",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "sidb-recovery",
				Image: "container-registry.oracle.com/database/free:latest",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, terminatingPod).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePods(
		sidb,
		nil,
		nil,
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-recovery"}},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue while waiting for terminating pod, got %#v", got)
	}

	pods := &corev1.PodList{}
	if err := reconciler.List(context.Background(), pods); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(pods.Items) != 1 {
		t.Fatalf("expected no replacement pod while old pod is terminating, got %d pod(s)", len(pods.Items))
	}
	if pods.Items[0].Name != terminatingPod.Name {
		t.Fatalf("expected only terminating pod to remain, got %q", pods.Items[0].Name)
	}
}

func TestSIDBUnit_CreateOrReplacePVCforCustomScriptsVolDeletesStalePVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb-custom-scripts", Namespace: "default", UID: types.UID("sidb-custom-scripts")},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				ScriptsVolumeName: "db-vol-patch",
			},
		},
	}

	stalePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sidb-custom-scripts-db-vol",
			Namespace: "default",
			Labels: map[string]string{
				"app": "sidb-custom-scripts",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         dbapi.GroupVersion.String(),
				Kind:               "SingleInstanceDatabase",
				Name:               "sidb-custom-scripts",
				UID:                sidb.UID,
				Controller:         func() *bool { b := true; return &b }(),
				BlockOwnerDeletion: func() *bool { b := true; return &b }(),
			}},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "db-vol",
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sidb, stalePVC).
			Build(),
		Log: logr.Discard(),
	}

	got, err := reconciler.createOrReplacePVCforCustomScriptsVol(
		context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "sidb-custom-scripts"}},
		sidb,
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Requeue {
		t.Fatalf("expected requeue after deleting stale scripts PVC, got %#v", got)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err = reconciler.Get(context.Background(), types.NamespacedName{Name: stalePVC.Name, Namespace: stalePVC.Namespace}, pvc)
	if err == nil {
		t.Fatalf("expected stale scripts PVC to be deleted")
	}
}

func TestSIDBUnit_PhaseConnectStringGate(t *testing.T) {
	reconciler := &SingleInstanceDatabaseReconciler{Log: logr.Discard()}
	pending := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1"},
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:     dbcommons.ValueUnavailable,
			TcpsConnectString: dbcommons.ValueUnavailable,
		},
	}
	res, err := reconciler.phaseConnectStringGate(pending)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Requeue {
		t.Fatalf("expected requeue when both connect strings are unavailable")
	}

	notEnabled := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "sidb1"},
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:     dbcommons.ValueUnavailable,
			TcpsConnectString: dbcommons.ValueNotEnabled,
		},
	}
	res, err = reconciler.phaseConnectStringGate(notEnabled)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Requeue {
		t.Fatalf("expected requeue when tcp is unavailable and tcps is not enabled")
	}

	ready := &dbapi.SingleInstanceDatabase{Status: dbapi.SingleInstanceDatabaseStatus{ConnectString: "host:1521/ORCLCDB"}}
	res, err = reconciler.phaseConnectStringGate(ready)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available connect string, got %#v", res)
	}

	clusterOnly := &dbapi.SingleInstanceDatabase{
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:        dbcommons.ValueUnavailable,
			ClusterConnectString: "sidb.testcase:1521/ORCLCDB",
			TcpsConnectString:    dbcommons.ValueUnavailable,
		},
	}
	res, err = reconciler.phaseConnectStringGate(clusterOnly)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available cluster connect string, got %#v", res)
	}

	tcpsOnly := &dbapi.SingleInstanceDatabase{
		Status: dbapi.SingleInstanceDatabaseStatus{
			ConnectString:     dbcommons.ValueUnavailable,
			TcpsConnectString: "host:2484/ORCLCDB",
		},
	}
	res, err = reconciler.phaseConnectStringGate(tcpsOnly)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res != requeueN {
		t.Fatalf("expected no requeue for available tcps connect string, got %#v", res)
	}
}

func TestSIDBUnit_UnresolvedSIDBTCPSConnectStringValue(t *testing.T) {
	if got := unresolvedSIDBTCPSConnectStringValue(false); got != dbcommons.ValueNotEnabled {
		t.Fatalf("expected tcps-disabled status %q, got %q", dbcommons.ValueNotEnabled, got)
	}
	if got := unresolvedSIDBTCPSConnectStringValue(true); got != dbcommons.ValueUnavailable {
		t.Fatalf("expected tcps-enabled pending status %q, got %q", dbcommons.ValueUnavailable, got)
	}
}

func TestSIDBUnit_ParseSQLPlusSingleInt(t *testing.T) {
	got, err := parseSQLPlusSingleInt("\n  5500\n")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 5500 {
		t.Fatalf("expected 5500, got %d", got)
	}

	if _, err := parseSQLPlusSingleInt("\n\n"); err == nil {
		t.Fatalf("expected error for empty sqlplus output")
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

func TestSIDBUnit_ReconcileDeletingSIDBStopsAfterManageDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}

	now := metav1.Now()
	sidb := &dbapi.SingleInstanceDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sidb-delete",
			Namespace:         "ns1",
			DeletionTimestamp: &now,
			Finalizers:        []string{singleInstanceDatabaseFinalizer},
		},
		Spec: dbapi.SingleInstanceDatabaseSpec{
			CreateAs: "truecache",
			Edition:  "enterprise",
			Sid:      "ORCLTC",
			Image: dbapi.SingleInstanceDatabaseImage{
				PullFrom: "container-registry.oracle.com/database/enterprise:latest",
			},
			Persistence: dbapi.SingleInstanceDatabasePersistence{
				Oradata: &dbapi.SingleInstanceDatabasePersistenceOradata{
					PvcName: "existing-oradata-pvc",
				},
			},
		},
	}

	reconciler := &SingleInstanceDatabaseReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&dbapi.SingleInstanceDatabase{}).
			WithObjects(sidb).
			Build(),
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "sidb-delete"},
	})
	if err != nil {
		t.Fatalf("expected deleting SIDB reconcile to exit cleanly, got: %v", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Fatalf("expected deleting SIDB reconcile not to requeue, got: %#v", res)
	}

	podList := &corev1.PodList{}
	if err := reconciler.List(context.Background(), podList, client.InNamespace("ns1")); err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}
	if len(podList.Items) != 0 {
		t.Fatalf("expected no pods to be created for deleting SIDB, found %d", len(podList.Items))
	}
}
