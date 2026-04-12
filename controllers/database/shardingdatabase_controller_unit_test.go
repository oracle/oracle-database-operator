package controllers

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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

func TestShardingUnit_SyncDataguardPreviewStatusUserDG(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "shdb", Namespace: "ns1"},
		Spec: databasev4.ShardingDatabaseSpec{
			ReplicationType:   "DG",
			ShardingType:      "USER",
			DbImage:           "oracle/sharding-db:23ai",
			DbImagePullSecret: "db-pull-secret",
			DbSecret: &databasev4.SecretDetails{
				Name: "shard-db-secret",
				DbAdmin: databasev4.PasswordSecretConfig{
					PasswordKey: "oracle_pwd",
				},
			},
			Dataguard:         &databasev4.DataguardProducerSpec{Mode: databasev4.DataguardProducerModePreview},
			Shard: []databasev4.ShardSpec{
				{Name: "primary1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
				{Name: "standby1", ShardSpace: "ss1", DeployAs: "STANDBY"},
			},
		},
	}
	status := &databasev4.ShardingDatabaseStatus{}

	r := &ShardingDatabaseReconciler{}
	r.syncShardingDataguardPreviewStatus(inst, status)

	if status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if status.Dataguard.Phase != dataguardPreviewPhaseReady {
		t.Fatalf("expected preview phase %q, got %q", dataguardPreviewPhaseReady, status.Dataguard.Phase)
	}
	if !status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be true")
	}
	if status.Dataguard.Topology == nil || len(status.Dataguard.Topology.Pairs) != 1 {
		t.Fatalf("expected one topology pair, got %#v", status.Dataguard.Topology)
	}
	if len(status.Dataguard.Members) != 2 {
		t.Fatalf("expected two member statuses, got %#v", status.Dataguard.Members)
	}
	if status.Dataguard.PublishedTopologyHash == "" {
		t.Fatalf("expected topology hash to be set")
	}
	if status.Dataguard.TopologyHash == "" {
		t.Fatalf("expected topologyHash to be set")
	}
	if status.Dataguard.LastPublishedTime == nil {
		t.Fatalf("expected lastPublishedTime to be set")
	}
	if status.Dataguard.Execution == nil || status.Dataguard.Execution.Image != "oracle/sharding-db:23ai" {
		t.Fatalf("expected sharding execution image to be published, got %#v", status.Dataguard.Execution)
	}
	if status.Dataguard.RenderedBrokerSpec == nil {
		t.Fatalf("expected renderedBrokerSpec to be published")
	}
	if status.Dataguard.RenderedBrokerSpec.Name != "shdb-dg" {
		t.Fatalf("unexpected rendered broker name: %#v", status.Dataguard.RenderedBrokerSpec)
	}
	if status.Dataguard.RenderedBrokerSpec.Namespace != "ns1" {
		t.Fatalf("unexpected rendered broker namespace: %#v", status.Dataguard.RenderedBrokerSpec)
	}
	if status.Dataguard.RenderedBrokerSpec.Spec == nil || status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology, got %#v", status.Dataguard.RenderedBrokerSpec)
	}
	for _, member := range status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members {
		if member.AdminSecretRef == nil {
			t.Fatalf("expected adminSecretRef for member %#v", member)
		}
		if member.AdminSecretRef.SecretName != "shard-db-secret" || member.AdminSecretRef.SecretKey != "oracle_pwd" {
			t.Fatalf("unexpected adminSecretRef for member %#v", member)
		}
	}
}

func TestShardingUnit_SyncDataguardPreviewStatusExternalPrimaryRequiresUserInput(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "shdb", Namespace: "ns1"},
		Spec: databasev4.ShardingDatabaseSpec{
			ReplicationType: "DG",
			ShardingType:    "USER",
			DbImage:         "oracle/sharding-db:23ai",
			DbSecret: &databasev4.SecretDetails{
				Name: "shard-db-secret",
				DbAdmin: databasev4.PasswordSecretConfig{
					PasswordKey: "oracle_pwd",
				},
			},
			Dataguard: &databasev4.DataguardProducerSpec{Mode: databasev4.DataguardProducerModePreview},
			Shard: []databasev4.ShardSpec{
				{Name: "standby1", ShardSpace: "ss1", DeployAs: "STANDBY"},
			},
			ShardInfo: []databasev4.ShardingDetails{{
				ShardPreFixName: "standby",
				StandbyConfig: &databasev4.StandbyConfig{
					StandbyPerPrimary: 1,
					PrimarySources: []databasev4.StandbyPrimarySource{{
						Details: &databasev4.PrimaryEndpointRef{
							Host:    "external-primary",
							Port:    1521,
							CdbName: "PRIM",
						},
					}},
				},
			}},
		},
	}
	status := &databasev4.ShardingDatabaseStatus{}

	r := &ShardingDatabaseReconciler{}
	r.syncShardingDataguardPreviewStatus(inst, status)

	if status.Dataguard == nil {
		t.Fatalf("expected dataguard preview status to be populated")
	}
	if status.Dataguard.Phase != dataguardPreviewPhaseWaitingForUserInput {
		t.Fatalf("expected preview phase %q, got %q", dataguardPreviewPhaseWaitingForUserInput, status.Dataguard.Phase)
	}
	if status.Dataguard.ReadyForBroker {
		t.Fatalf("expected readyForBroker to be false")
	}
	if status.Dataguard.RenderedBrokerSpec == nil || status.Dataguard.RenderedBrokerSpec.Spec == nil || status.Dataguard.RenderedBrokerSpec.Spec.Topology == nil {
		t.Fatalf("expected rendered broker spec topology to be published")
	}
	if status.Dataguard.RenderedBrokerSpec.Ready {
		t.Fatalf("expected rendered broker spec to be marked not ready")
	}
	condition := meta.FindStatusCondition(status.Dataguard.Conditions, "TopologyPreviewReady")
	if condition == nil {
		t.Fatalf("expected TopologyPreviewReady condition to be set")
	}
	if condition.Reason != "WaitingForUserInput" {
		t.Fatalf("expected WaitingForUserInput condition reason, got %#v", condition)
	}
	if !strings.Contains(condition.Message, "adminSecretRef.secretName") {
		t.Fatalf("expected condition message to explain adminSecretRef update, got %#v", condition)
	}
	members := status.Dataguard.RenderedBrokerSpec.Spec.Topology.Members
	if len(members) != 2 {
		t.Fatalf("expected two topology members, got %#v", members)
	}
	var primaryMember *databasev4.DataguardTopologyMember
	var standbyMember *databasev4.DataguardTopologyMember
	for i := range members {
		switch members[i].Role {
		case "PRIMARY":
			primaryMember = &members[i]
		case "PHYSICAL_STANDBY":
			standbyMember = &members[i]
		}
	}
	if primaryMember == nil || standbyMember == nil {
		t.Fatalf("expected primary and standby members, got %#v", members)
	}
	if primaryMember.AdminSecretRef == nil {
		t.Fatalf("expected placeholder adminSecretRef for external primary")
	}
	if primaryMember.AdminSecretRef.SecretName != dataguardPreviewExternalSecretPlaceholder || primaryMember.AdminSecretRef.SecretKey != dataguardPreviewExternalSecretKey {
		t.Fatalf("unexpected placeholder adminSecretRef %#v", primaryMember.AdminSecretRef)
	}
	if standbyMember.AdminSecretRef == nil {
		t.Fatalf("expected local standby adminSecretRef")
	}
	if standbyMember.AdminSecretRef.SecretName != "shard-db-secret" || standbyMember.AdminSecretRef.SecretKey != "oracle_pwd" {
		t.Fatalf("unexpected standby adminSecretRef %#v", standbyMember.AdminSecretRef)
	}
}

func TestShardingUnit_SyncDataguardPreviewStatusDisabledClearsStatus(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		Spec: databasev4.ShardingDatabaseSpec{
			ReplicationType: "DG",
			Dataguard: &databasev4.DataguardProducerSpec{
				Mode: databasev4.DataguardProducerModeDisabled,
			},
		},
	}
	status := &databasev4.ShardingDatabaseStatus{
		Dataguard: &databasev4.ShardingDataguardStatus{Phase: dataguardPreviewPhaseReady},
	}

	r := &ShardingDatabaseReconciler{}
	r.syncShardingDataguardPreviewStatus(inst, status)

	if status.Dataguard != nil {
		t.Fatalf("expected dataguard status to be cleared when preview is disabled")
	}
}

func TestShardingUnit_BuildPrimaryIdentitiesUsesStandbyPrimarySources(t *testing.T) {
	inst := &databasev4.ShardingDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shdb",
			Namespace: "shns",
		},
	}

	t.Run("databaseRef", func(t *testing.T) {
		cfg := &databasev4.StandbyConfig{
			PrimarySources: []databasev4.StandbyPrimarySource{{
				DatabaseRef: &databasev4.PrimaryDatabaseCRRef{Name: "primary-db"},
			}},
		}
		ids := buildPrimaryIdentities(inst, cfg)
		if len(ids) != 1 || ids[0].Key != "shns/primary-db" || ids[0].Source != "PrimaryDatabaseRef" {
			t.Fatalf("unexpected primary identities for databaseRef: %#v", ids)
		}
	})

	t.Run("details", func(t *testing.T) {
		cfg := &databasev4.StandbyConfig{
			PrimarySources: []databasev4.StandbyPrimarySource{{
				Details: &databasev4.PrimaryEndpointRef{
					Host:    "primary-host",
					Port:    1522,
					CdbName: "primdb",
				},
			}},
		}
		ids := buildPrimaryIdentities(inst, cfg)
		if len(ids) != 1 {
			t.Fatalf("expected one primary identity, got %#v", ids)
		}
		if ids[0].Source != "Endpoint" {
			t.Fatalf("expected endpoint source, got %#v", ids[0])
		}
		if ids[0].Connect != "//primary-host:1522/PRIMDB_DGMGRL" {
			t.Fatalf("unexpected endpoint connect string: %#v", ids[0])
		}
	})

	t.Run("multiple", func(t *testing.T) {
		cfg := &databasev4.StandbyConfig{
			PrimarySources: []databasev4.StandbyPrimarySource{
				{ConnectString: "//phx-primary:1521/PHX_DGMGRL"},
				{ConnectString: "//ash-primary:1521/ASH_DGMGRL"},
			},
		}
		ids := buildPrimaryIdentities(inst, cfg)
		if len(ids) != 2 {
			t.Fatalf("expected two primary identities, got %#v", ids)
		}
	})
}

func TestShardingUnit_StandbyConfigPrimaryCountControllerSupportsPrimarySources(t *testing.T) {
	cfg := &databasev4.StandbyConfig{
		PrimarySources: []databasev4.StandbyPrimarySource{
			{ConnectString: "//phx-primary:1521/PHX_DGMGRL"},
			{ConnectString: "//ash-primary:1521/ASH_DGMGRL"},
		},
	}

	if got := standbyConfigPrimaryCountController(cfg); got != 2 {
		t.Fatalf("expected primary source count 2, got %d", got)
	}
	if got := standbyConfigDerivedShardCountController(&databasev4.StandbyConfig{
		StandbyPerPrimary: 2,
		PrimarySources: []databasev4.StandbyPrimarySource{
			{ConnectString: "//phx-primary:1521/PHX_DGMGRL"},
			{ConnectString: "//ash-primary:1521/ASH_DGMGRL"},
		},
	}); got != 4 {
		t.Fatalf("expected derived shard count 4, got %d", got)
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

func TestShardingUnit_ValidatePrimaryTopologyConstraint_UserDGOrderIndependent(t *testing.T) {
	r := &ShardingDatabaseReconciler{}
	inst := &databasev4.ShardingDatabase{
		Spec: databasev4.ShardingDatabaseSpec{
			ShardingType:    "USER",
			ReplicationType: "DG",
			Shard: []databasev4.ShardSpec{
				{Name: "s1", ShardSpace: "ss1", DeployAs: "STANDBY", ShardRegion: "ashburn"},
				{Name: "p1", ShardSpace: "ss1", DeployAs: "PRIMARY"},
			},
		},
	}

	if err := r.validatePrimaryTopologyConstraint(inst); err != nil {
		t.Fatalf("expected standby-before-primary ordering to pass, got %v", err)
	}
}

func TestShardingUnit_ValidatePrimaryTopologyConstraint_CompositeNativeRules(t *testing.T) {
	tests := []struct {
		name    string
		inst    *databasev4.ShardingDatabase
		wantErr string
	}{
		{
			name: "allows composite native with unique region per shardgroup and one readwrite database",
			inst: &databasev4.ShardingDatabase{
				Spec: databasev4.ShardingDatabaseSpec{
					ShardingType:    "COMPOSITE",
					ReplicationType: "NATIVE",
					ShardGroup: []databasev4.ShardGroupSpec{
						{Name: "sg-rw", ShardSpace: "ss1", RuMode: "READWRITE"},
						{Name: "sg-ro", ShardSpace: "ss1", RuMode: "READONLY"},
					},
					Shard: []databasev4.ShardSpec{
						{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
						{Name: "ro1", ShardGroup: "sg-ro", ShardSpace: "ss1", ShardRegion: "iad"},
						{Name: "ro2", ShardGroup: "sg-ro", ShardSpace: "ss1", ShardRegion: "iad"},
					},
				},
			},
		},
		{
			name: "rejects duplicate region across shardgroups in same shardspace",
			inst: &databasev4.ShardingDatabase{
				Spec: databasev4.ShardingDatabaseSpec{
					ShardingType:    "COMPOSITE",
					ReplicationType: "NATIVE",
					ShardGroup: []databasev4.ShardGroupSpec{
						{Name: "sg-rw", ShardSpace: "ss1", RuMode: "READWRITE"},
						{Name: "sg-ro", ShardSpace: "ss1", RuMode: "READONLY"},
					},
					Shard: []databasev4.ShardSpec{
						{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
						{Name: "ro1", ShardGroup: "sg-ro", ShardSpace: "ss1", ShardRegion: "phx"},
					},
				},
			},
			wantErr: "already used by shardGroup",
		},
		{
			name: "rejects missing ru_mode for composite native shardgroup",
			inst: &databasev4.ShardingDatabase{
				Spec: databasev4.ShardingDatabaseSpec{
					ShardingType:    "COMPOSITE",
					ReplicationType: "NATIVE",
					ShardGroup: []databasev4.ShardGroupSpec{
						{Name: "sg-rw", ShardSpace: "ss1"},
					},
					Shard: []databasev4.ShardSpec{
						{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
					},
				},
			},
			wantErr: "requires ru_mode",
		},
		{
			name: "rejects more than one readwrite database in same shardgroup",
			inst: &databasev4.ShardingDatabase{
				Spec: databasev4.ShardingDatabaseSpec{
					ShardingType:    "COMPOSITE",
					ReplicationType: "NATIVE",
					ShardGroup: []databasev4.ShardGroupSpec{
						{Name: "sg-rw", ShardSpace: "ss1", RuMode: "READWRITE"},
					},
					Shard: []databasev4.ShardSpec{
						{Name: "rw1", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
						{Name: "rw2", ShardGroup: "sg-rw", ShardSpace: "ss1", ShardRegion: "phx"},
					},
				},
			},
			wantErr: "allows at most one READWRITE database",
		},
	}

	r := &ShardingDatabaseReconciler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.validatePrimaryTopologyConstraint(tt.inst)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
