package controllers

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testShardStatefulSet(name string, cpu string, runAsUser int64, capsAdd []corev1.Capability) *appsv1.StatefulSet {
	res := corev1.ResourceList{}
	if cpu != "" {
		res[corev1.ResourceCPU] = resource.MustParse(cpu)
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &runAsUser,
					},
					Containers: []corev1.Container{
						{
							Name: name,
							Resources: corev1.ResourceRequirements{
								Limits: res,
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: capsAdd,
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestShardingUnit_StatefulSetNeedsNonShapeShardTemplateRecreate(t *testing.T) {
	t.Run("resources drift", func(t *testing.T) {
		curr := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"NET_ADMIN"})
		want := testShardStatefulSet("shard1", "2", 54321, []corev1.Capability{"NET_ADMIN"})
		if !statefulSetNeedsNonShapeShardTemplateRecreate(curr, want) {
			t.Fatalf("expected non-shape drift when resources changed")
		}
	})

	t.Run("pod security context drift", func(t *testing.T) {
		curr := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"NET_ADMIN"})
		want := testShardStatefulSet("shard1", "1", 54322, []corev1.Capability{"NET_ADMIN"})
		if !statefulSetNeedsNonShapeShardTemplateRecreate(curr, want) {
			t.Fatalf("expected non-shape drift when pod security context changed")
		}
	})

	t.Run("container capabilities drift", func(t *testing.T) {
		curr := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"NET_ADMIN"})
		want := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"SYS_NICE"})
		if !statefulSetNeedsNonShapeShardTemplateRecreate(curr, want) {
			t.Fatalf("expected non-shape drift when container capabilities changed")
		}
	})

	t.Run("no drift", func(t *testing.T) {
		curr := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"NET_ADMIN"})
		want := testShardStatefulSet("shard1", "1", 54321, []corev1.Capability{"NET_ADMIN"})
		if statefulSetNeedsNonShapeShardTemplateRecreate(curr, want) {
			t.Fatalf("expected no non-shape drift for identical templates")
		}
	})
}

func TestShardingUnit_OrderedNonShapeTemplateRollTargets(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		Spec: databasev4.ShardingDatabaseSpec{
			Catalog: []databasev4.CatalogSpec{
				{Name: "catalog1"},
			},
			Gsm: []databasev4.GsmSpec{
				{Name: "gsm1"},
			},
			Shard: []databasev4.ShardSpec{
				{Name: "shard10"},
				{Name: "shard2"},
				{Name: "shard1"},
				{Name: "shard9", IsDelete: "enable"},
			},
		},
	}

	targets := orderedNonShapeTemplateRollTargets(inst, map[string]bool{
		"CATALOG|catalog1": true,
		"SHARD|shard2":     true,
	})
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[0].kind != "GSM" || targets[0].name != "gsm1" {
		t.Fatalf("expected first target gsm1, got %s/%s", targets[0].kind, targets[0].name)
	}
	if targets[1].kind != "SHARD" || targets[1].name != "shard1" {
		t.Fatalf("expected second target shard1, got %s/%s", targets[1].kind, targets[1].name)
	}
	if targets[2].kind != "SHARD" || targets[2].name != "shard10" {
		t.Fatalf("expected third target shard10, got %s/%s", targets[2].kind, targets[2].name)
	}
}

func TestShardingUnit_StandbyWalletResolutionUsesLongestPrefix(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
		},
		Spec: databasev4.ShardingDatabaseSpec{
			ShardInfo: []databasev4.ShardingDetails{
				{
					ShardPreFixName: "shard1",
					StandbyConfig: &databasev4.StandbyConfig{
						TDEWallet: &databasev4.TDEWalletConfig{
							SecretRef:  "wallet-short",
							ZipFileKey: "short.zip",
						},
					},
				},
				{
					ShardPreFixName: "shard10",
					StandbyConfig: &databasev4.StandbyConfig{
						TDEWallet: &databasev4.TDEWalletConfig{
							SecretRef:  "wallet-long",
							ZipFileKey: "long.zip",
						},
					},
				},
			},
		},
	}

	r := &ShardingDatabaseReconciler{}
	shard := databasev4.ShardSpec{Name: "shard101"}
	if got := r.resolveStandbyWalletSecretRef(inst, shard); got != "wallet-long" {
		t.Fatalf("expected wallet-long, got %q", got)
	}
	if got := r.resolveStandbyWalletZipFileKey(inst, shard); got != "long.zip" {
		t.Fatalf("expected long.zip, got %q", got)
	}
}

func TestShardingUnit_ValidateStandbyWalletSecretRefUsesLongestPrefix(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wallet-long",
			Namespace: "shns",
		},
		Data: map[string][]byte{
			"long.zip": []byte("wallet-bytes"),
		},
	}

	r := &ShardingDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
	}

	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
		},
		Spec: databasev4.ShardingDatabaseSpec{
			ShardInfo: []databasev4.ShardingDetails{
				{
					ShardPreFixName: "shard1",
					StandbyConfig: &databasev4.StandbyConfig{
						TDEWallet: &databasev4.TDEWalletConfig{
							SecretRef:  "wallet-short",
							ZipFileKey: "short.zip",
						},
					},
				},
				{
					ShardPreFixName: "shard10",
					StandbyConfig: &databasev4.StandbyConfig{
						TDEWallet: &databasev4.TDEWalletConfig{
							SecretRef:  "wallet-long",
							ZipFileKey: "long.zip",
						},
					},
				},
			},
		},
	}

	if err := r.validateStandbyWalletSecretRef(inst, databasev4.ShardSpec{Name: "shard101"}); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
	if err := r.validateStandbyWalletSecretRef(inst, databasev4.ShardSpec{Name: "shard11"}); err == nil {
		t.Fatalf("expected validation failure for shard11")
	} else if !strings.Contains(err.Error(), "wallet-short") {
		t.Fatalf("expected wallet-short lookup failure, got %v", err)
	}
}

func TestShardingUnit_ParseAndSerializeImportedTDEShards(t *testing.T) {
	parsed := parseImportedTDEShards(" shard2,Shard1,, shard2 ")
	if !parsed["shard1"] || !parsed["shard2"] || len(parsed) != 2 {
		t.Fatalf("unexpected parsed set: %#v", parsed)
	}
	serialized := serializeImportedTDEShards(parsed)
	if serialized != "shard1,shard2" {
		t.Fatalf("unexpected serialized output: %q", serialized)
	}
}

func TestShardingUnit_EnsureTDEExportImportMarkers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := databasev4.AddToScheme(scheme); err != nil {
		t.Fatalf("add databasev4 scheme: %v", err)
	}

	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
		},
		Spec: databasev4.ShardingDatabaseSpec{
			IsTdeWallet: "enable",
			DbSecret: &databasev4.SecretDetails{
				Name: "db-secret",
				DbAdmin: databasev4.PasswordSecretConfig{
					PasswordKey: "dbpwd",
				},
				TDE: &databasev4.PasswordSecretConfig{
					PasswordKey: "tdepwd",
				},
			},
			Catalog: []databasev4.CatalogSpec{
				{Name: "catalog1"},
			},
		},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-secret",
			Namespace: "shns",
		},
		Data: map[string][]byte{
			"dbpwd":  []byte("admin"),
			"tdepwd": []byte("tde"),
		},
	}

	exportCalls := 0
	importCalls := 0
	origExport := exportTDEKeyFn
	origImport := importTDEKeyFn
	exportTDEKeyFn = func(_ string, _ string, _ *databasev4.ShardingDatabase, _ *rest.Config, _ logr.Logger) error {
		exportCalls++
		return nil
	}
	importTDEKeyFn = func(_ string, _ string, _ *databasev4.ShardingDatabase, _ *rest.Config, _ logr.Logger) error {
		importCalls++
		return nil
	}
	t.Cleanup(func() {
		exportTDEKeyFn = origExport
		importTDEKeyFn = origImport
	})

	r := &ShardingDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst, sec).Build(),
	}

	if err := r.ensureTDEKeysExported(context.Background(), inst); err != nil {
		t.Fatalf("ensureTDEKeysExported failed: %v", err)
	}
	if exportCalls != 1 {
		t.Fatalf("expected one export call, got %d", exportCalls)
	}

	updated := &databasev4.ShardingDatabase{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "shdb", Namespace: "shns"}, updated); err != nil {
		t.Fatalf("get updated cr: %v", err)
	}
	if !strings.EqualFold(updated.Annotations[tdeKeyExportedAnnotation], "true") {
		t.Fatalf("expected exported annotation to be true, got %q", updated.Annotations[tdeKeyExportedAnnotation])
	}

	if err := r.ensureTDEKeysImportedForShard(context.Background(), updated, databasev4.ShardSpec{Name: "shard1"}); err != nil {
		t.Fatalf("ensureTDEKeysImportedForShard failed: %v", err)
	}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "shdb", Namespace: "shns"}, updated); err != nil {
		t.Fatalf("get updated cr after import: %v", err)
	}
	if importCalls != 1 {
		t.Fatalf("expected one import call, got %d", importCalls)
	}
	if got := updated.Annotations[tdeKeyImportedShardsAnnotation]; got != "shard1" {
		t.Fatalf("expected imported shards annotation shard1, got %q", got)
	}

	if err := r.ensureTDEKeysImportedForShard(context.Background(), updated, databasev4.ShardSpec{Name: "shard1"}); err != nil {
		t.Fatalf("ensureTDEKeysImportedForShard second call failed: %v", err)
	}
	if importCalls != 1 {
		t.Fatalf("expected import to be idempotent, got %d calls", importCalls)
	}

	if err := r.ensureTDEKeysImportedForShard(context.Background(), updated, databasev4.ShardSpec{Name: "shard2", DeployAs: "STANDBY"}); err != nil {
		t.Fatalf("ensureTDEKeysImportedForShard standby call failed: %v", err)
	}
	if importCalls != 1 {
		t.Fatalf("expected standby import to be skipped, got %d calls", importCalls)
	}
}

func TestShardingUnit_PruneImportedTDEShardsAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := databasev4.AddToScheme(scheme); err != nil {
		t.Fatalf("add databasev4 scheme: %v", err)
	}

	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
			Annotations: map[string]string{
				tdeKeyImportedShardsAnnotation: "shard1,shard2,shard3",
			},
		},
		Spec: databasev4.ShardingDatabaseSpec{
			Shard: []databasev4.ShardSpec{
				{Name: "shard1"},
				{Name: "shard2", IsDelete: "enable"},
			},
		},
	}

	r := &ShardingDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst).Build(),
	}
	if err := r.pruneImportedTDEShardsAnnotation(context.Background(), inst); err != nil {
		t.Fatalf("pruneImportedTDEShardsAnnotation failed: %v", err)
	}

	updated := &databasev4.ShardingDatabase{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "shdb", Namespace: "shns"}, updated); err != nil {
		t.Fatalf("get updated cr: %v", err)
	}
	if got := updated.Annotations[tdeKeyImportedShardsAnnotation]; got != "shard1" {
		t.Fatalf("expected only shard1 to remain, got %q", got)
	}
}

func TestShardingUnit_OrderedManualTDERefreshTargets(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		Spec: databasev4.ShardingDatabaseSpec{
			Shard: []databasev4.ShardSpec{
				{Name: "s2", DeployAs: "STANDBY"},
				{Name: "s1", DeployAs: "PRIMARY"},
				{Name: "s3", DeployAs: "ACTIVE_STANDBY"},
				{Name: "s4", DeployAs: "PRIMARY", IsDelete: "enable"},
			},
		},
	}
	got := orderedManualTDERefreshTargets(inst)
	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(got))
	}
	if got[0] != "s1" || got[1] != "s2" || got[2] != "s3" {
		t.Fatalf("unexpected target order: %#v", got)
	}
}

func TestShardingUnit_PhaseManualTDERefresh_AllShardsAndClearAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := databasev4.AddToScheme(scheme); err != nil {
		t.Fatalf("add databasev4 scheme: %v", err)
	}

	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
			Annotations: map[string]string{
				tdeKeyRefreshAnnotation: "token-1",
			},
		},
		Spec: databasev4.ShardingDatabaseSpec{
			IsTdeWallet: "enable",
			Catalog: []databasev4.CatalogSpec{
				{Name: "catalog1"},
			},
			Shard: []databasev4.ShardSpec{
				{Name: "p1", DeployAs: "PRIMARY"},
				{Name: "s1", DeployAs: "STANDBY"},
			},
		},
	}

	exportCalls := 0
	importCalls := []string{}
	origExport := exportTDEKeyFn
	origImport := importTDEKeyFn
	exportTDEKeyFn = func(_ string, _ string, _ *databasev4.ShardingDatabase, _ *rest.Config, _ logr.Logger) error {
		exportCalls++
		return nil
	}
	importTDEKeyFn = func(pod string, _ string, _ *databasev4.ShardingDatabase, _ *rest.Config, _ logr.Logger) error {
		importCalls = append(importCalls, pod)
		return nil
	}
	t.Cleanup(func() {
		exportTDEKeyFn = origExport
		importTDEKeyFn = origImport
	})

	r := &ShardingDatabaseReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(inst).Build(),
	}
	st := &databasev4.ShardingDatabaseStatus{}

	for i := 0; i < 5; i++ {
		pr, handled := r.phaseManualTDERefresh(context.Background(), inst, st)
		if !handled {
			t.Fatalf("expected refresh phase to handle token")
		}
		if !pr.wait {
			t.Fatalf("expected wait=true while processing refresh")
		}
		_ = r.Get(context.Background(), types.NamespacedName{Name: "shdb", Namespace: "shns"}, inst)
		if _, ok := inst.Annotations[tdeKeyRefreshAnnotation]; !ok {
			break
		}
	}

	if exportCalls != 1 {
		t.Fatalf("expected one export call, got %d", exportCalls)
	}
	if len(importCalls) != 2 {
		t.Fatalf("expected two import calls for primary+standby, got %d (%#v)", len(importCalls), importCalls)
	}
	if importCalls[0] != "p1-0" || importCalls[1] != "s1-0" {
		t.Fatalf("unexpected import call order: %#v", importCalls)
	}
	if st.TDEKeyRefresh == nil || st.TDEKeyRefresh.Phase != "Succeeded" {
		t.Fatalf("expected succeeded refresh status, got %#v", st.TDEKeyRefresh)
	}
	if _, ok := inst.Annotations[tdeKeyRefreshAnnotation]; ok {
		t.Fatalf("expected refresh annotation to be cleared")
	}
}
