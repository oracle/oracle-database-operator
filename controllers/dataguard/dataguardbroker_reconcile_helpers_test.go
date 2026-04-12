package controllers

import (
	"context"
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveDataguardBrokerDesiredSpecLegacy(t *testing.T) {
	broker := &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			PrimaryDatabaseRef:  "primary-db",
			StandbyDatabaseRefs: []string{"standby-a", "standby-b"},
			ProtectionMode:      "MaxPerformance",
			FastStartFailover:   true,
			LoadBalancer:        true,
			ServiceAnnotations: map[string]string{
				"shape": "lb",
			},
		},
	}

	got := resolveDataguardBrokerDesiredSpec(broker)
	if got.Path != dataguardBrokerPathLegacy {
		t.Fatalf("expected legacy path, got %q", got.Path)
	}
	if got.PrimaryDatabaseRef != "primary-db" {
		t.Fatalf("expected primary-db, got %q", got.PrimaryDatabaseRef)
	}
	if len(got.StandbyDatabaseRefs) != 2 {
		t.Fatalf("expected 2 standby refs, got %d", len(got.StandbyDatabaseRefs))
	}
	if !got.FastStartFailover {
		t.Fatalf("expected FSFO to be enabled")
	}
	if !got.SupportsLegacyExecution {
		t.Fatalf("expected legacy spec to support execution")
	}
	if got.ServiceAnnotations["shape"] != "lb" {
		t.Fatalf("expected service annotation to be copied")
	}
}

func TestResolveDataguardBrokerDesiredSpecTopologyProjectsLocalSIDBMembers(t *testing.T) {
	broker := &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			Topology: &dbapi.DataguardTopologySpec{
				Policy: &dbapi.DataguardPolicySpec{
					ProtectionMode:    "MaxAvailability",
					FastStartFailover: true,
				},
				Members: []dbapi.DataguardTopologyMember{
					{
						Name:         "primary-a",
						Role:         "PRIMARY",
						DBUniqueName: "PRIMARYA",
						LocalRef: &dbapi.DataguardLocalRef{
							Kind: "SingleInstanceDatabase",
							Name: "primary-db",
						},
						Endpoints: []dbapi.DataguardEndpointSpec{{
							Protocol:    "TCP",
							Host:        "primary-db",
							Port:        1521,
							ServiceName: "PRIMARYA_DGMGRL",
						}},
					},
					{
						Name:         "standby-a",
						Role:         "PHYSICAL_STANDBY",
						DBUniqueName: "STBYA",
						LocalRef: &dbapi.DataguardLocalRef{
							Kind: "SingleInstanceDatabase",
							Name: "standby-db",
						},
						Endpoints: []dbapi.DataguardEndpointSpec{{
							Protocol:    "TCP",
							Host:        "standby-db",
							Port:        1521,
							ServiceName: "STBYA_DGMGRL",
						}},
					},
				},
				Pairs: []dbapi.DataguardTopologyPair{{
					Primary: "primary-a",
					Standby: "standby-a",
					Type:    "PHYSICAL",
				}},
			},
		},
	}

	got := resolveDataguardBrokerDesiredSpec(broker)
	if got.Path != dataguardBrokerPathTopology {
		t.Fatalf("expected topology path, got %q", got.Path)
	}
	if !got.SupportsLegacyExecution {
		t.Fatalf("expected projected topology to support legacy execution, message=%q", got.CompatibilityMessage)
	}
	if got.PrimaryDatabaseRef != "primary-db" {
		t.Fatalf("expected primary-db, got %q", got.PrimaryDatabaseRef)
	}
	if len(got.StandbyDatabaseRefs) != 1 || got.StandbyDatabaseRefs[0] != "standby-db" {
		t.Fatalf("unexpected standby refs: %#v", got.StandbyDatabaseRefs)
	}
	if got.ProtectionMode != "MaxAvailability" {
		t.Fatalf("expected MaxAvailability, got %q", got.ProtectionMode)
	}
	if !got.FastStartFailover {
		t.Fatalf("expected FSFO true")
	}
	if got.TopologyHash == "" {
		t.Fatalf("expected topology hash to be set")
	}
	if len(got.ResolvedMembers) != 2 {
		t.Fatalf("expected resolved members, got %d", len(got.ResolvedMembers))
	}
	if len(got.ObservedPairs) != 1 || got.ObservedPairs[0].State != "Resolved" {
		t.Fatalf("unexpected observed pairs: %#v", got.ObservedPairs)
	}
}

func TestResolveDataguardBrokerDesiredSpecTopologyMarksTCPSAsPendingNativeExecution(t *testing.T) {
	broker := &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			Topology: &dbapi.DataguardTopologySpec{
				Members: []dbapi.DataguardTopologyMember{
					{
						Name: "primary-a",
						Role: "PRIMARY",
						LocalRef: &dbapi.DataguardLocalRef{
							Kind: "SingleInstanceDatabase",
							Name: "primary-db",
						},
						Endpoints: []dbapi.DataguardEndpointSpec{{
							Protocol:    "TCPS",
							Host:        "primary-db",
							Port:        2484,
							ServiceName: "PRIMARYA_DGMGRL",
						}},
						TCPS: &dbapi.DataguardTCPSConfig{Enabled: true},
					},
					{
						Name: "standby-a",
						Role: "PHYSICAL_STANDBY",
						LocalRef: &dbapi.DataguardLocalRef{
							Kind: "SingleInstanceDatabase",
							Name: "standby-db",
						},
						Endpoints: []dbapi.DataguardEndpointSpec{{
							Protocol:    "TCP",
							Host:        "standby-db",
							Port:        1521,
							ServiceName: "STBYA_DGMGRL",
						}},
					},
				},
				Pairs: []dbapi.DataguardTopologyPair{{
					Primary: "primary-a",
					Standby: "standby-a",
				}},
			},
		},
	}

	got := resolveDataguardBrokerDesiredSpec(broker)
	if got.SupportsLegacyExecution {
		t.Fatalf("expected TCPS topology to require topology-native execution")
	}
	if got.CompatibilityMessage == "" {
		t.Fatalf("expected compatibility message")
	}
}

func TestResolveDataguardBrokerExecutionRuntimeFromSIDBProducerStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{}
	sidb.Namespace = "ns1"
	sidb.Name = "sidb-standby"
	sidb.Status.Dataguard = &dbapi.ProducerDataguardStatus{
		Execution: &dbapi.DataguardExecutionStatus{
			Image:            "oracle/db:19.3.0",
			ImagePullSecrets: []string{"pull-secret"},
		},
	}

	reconciler := &DataguardBrokerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb).Build(),
	}
	broker := &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			Topology: &dbapi.DataguardTopologySpec{
				SourceRef: &dbapi.DataguardSourceRef{
					Kind:      "SingleInstanceDatabase",
					Namespace: "ns1",
					Name:      "sidb-standby",
				},
				Members: []dbapi.DataguardTopologyMember{{
					Name: "primary-a",
					Role: "PRIMARY",
				}},
				Pairs: []dbapi.DataguardTopologyPair{{
					Primary: "primary-a",
					Standby: "standby-a",
				}},
			},
		},
	}

	got, ready, message, err := resolveDataguardBrokerExecutionRuntime(context.Background(), reconciler, broker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Fatalf("expected runtime to be ready, message=%q", message)
	}
	if got == nil || got.Image != "oracle/db:19.3.0" {
		t.Fatalf("expected execution image from producer status, got %#v", got)
	}
	if got.Source == "" {
		t.Fatalf("expected runtime source to be recorded")
	}
}

func TestResolveDataguardBrokerExecutionRuntimeRequiresTCPSWalletSecret(t *testing.T) {
	reconciler := &DataguardBrokerReconciler{}
	broker := &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			Topology: &dbapi.DataguardTopologySpec{
				Members: []dbapi.DataguardTopologyMember{
					{
						Name: "primary-a",
						Role: "PRIMARY",
						TCPS: &dbapi.DataguardTCPSConfig{Enabled: true},
					},
				},
				Pairs: []dbapi.DataguardTopologyPair{{
					Primary: "primary-a",
					Standby: "standby-a",
				}},
			},
		},
	}

	got, ready, message, err := resolveDataguardBrokerExecutionRuntime(context.Background(), reconciler, broker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatalf("expected runtime to be blocked on missing wallet secret, got %#v", got)
	}
	if message == "" {
		t.Fatalf("expected missing wallet message")
	}
}

func TestResolveDataguardTopologyMemberAdminSecretRefUsesGroupedSIDBSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add dbapi scheme: %v", err)
	}

	sidb := &dbapi.SingleInstanceDatabase{}
	sidb.Namespace = "ns1"
	sidb.Name = "sidb-standby"
	sidb.Spec.Security = &dbapi.SingleInstanceDatabaseSecurity{
		Secrets: &dbapi.SingleInstanceDatabaseSecrets{
			Admin: &dbapi.SingleInstanceDatabaseAdminPassword{
				SecretName: "grouped-admin",
				SecretKey:  "oracle_pwd",
			},
		},
	}

	reconciler := &DataguardBrokerReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(sidb).Build(),
	}
	broker := &dbapi.DataguardBroker{}
	broker.Namespace = "ns1"
	member := &dbapi.DataguardTopologyMember{
		Name: "standby-a",
		LocalRef: &dbapi.DataguardLocalRef{
			Kind:      "SingleInstanceDatabase",
			Namespace: "ns1",
			Name:      "sidb-standby",
		},
	}

	secretName, secretKey, secretNamespace, err := resolveDataguardTopologyMemberAdminSecretRef(context.Background(), reconciler, broker, member)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretName != "grouped-admin" || secretKey != "oracle_pwd" || secretNamespace != "ns1" {
		t.Fatalf("unexpected resolved secret ref: %q/%q in %q", secretName, secretKey, secretNamespace)
	}
}
