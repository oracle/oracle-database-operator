package v4

import (
	"context"
	"strings"
	"testing"
)

func validDataguardBrokerTopology() *DataguardBroker {
	return &DataguardBroker{
		Spec: DataguardBrokerSpec{
			Topology: &DataguardTopologySpec{
				Policy: &DataguardPolicySpec{
					ProtectionMode: "MaxPerformance",
					TransportMode:  "ASYNC",
				},
				Members: []DataguardTopologyMember{
					{
						Name:         "primary-a",
						Role:         "PRIMARY",
						DBUniqueName: "primary_a",
						LocalRef: &DataguardLocalRef{
							Kind: "SingleInstanceDatabase",
							Name: "primary-a",
						},
						Endpoints: []DataguardEndpointSpec{{
							Protocol:    "TCP",
							Host:        "primary-a",
							Port:        1521,
							ServiceName: "PRIM",
						}},
					},
					{
						Name:         "standby-a",
						Role:         "PHYSICAL_STANDBY",
						DBUniqueName: "standby_a",
						Endpoints: []DataguardEndpointSpec{{
							Protocol:    "TCP",
							Host:        "standby-a",
							Port:        1521,
							ServiceName: "STBY",
						}},
					},
				},
				Pairs: []DataguardTopologyPair{{
					Primary: "primary-a",
					Standby: "standby-a",
				}},
			},
		},
	}
}

func TestDataguardBrokerWebhookValidateCreateAllowsLegacySpec(t *testing.T) {
	obj := &DataguardBroker{
		Spec: DataguardBrokerSpec{
			PrimaryDatabaseRef:  "primary-a",
			StandbyDatabaseRefs: []string{"standby-a"},
			ProtectionMode:      "MaxPerformance",
		},
	}

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err != nil {
		t.Fatalf("expected legacy dataguardbroker spec to validate, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateRejectsMixedTopologyAndLegacy(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.PrimaryDatabaseRef = "primary-a"

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected mixed topology and legacy fields to be rejected")
	}
	if !strings.Contains(err.Error(), "cannot be used with spec.topology") {
		t.Fatalf("expected mixed topology rejection, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateRejectsInvalidTopology(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Members[1].Endpoints[0].Protocol = "UDP"

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected invalid topology to be rejected")
	}
	if !strings.Contains(err.Error(), "must be TCP or TCPS") {
		t.Fatalf("expected endpoint protocol validation error, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateRejectsTrueCacheRole(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Members[1].Role = "TRUECACHE"

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected TRUECACHE topology member role to be rejected")
	}
	if !strings.Contains(err.Error(), "must be PRIMARY, PHYSICAL_STANDBY, or SNAPSHOT_STANDBY") {
		t.Fatalf("expected TRUECACHE role validation error, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateRejectsExternalTopologyWithoutExecution(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Members[1].LocalRef = nil

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err == nil {
		t.Fatalf("expected topology-native runtime requirement to be enforced")
	}
	if !strings.Contains(err.Error(), "spec.execution is required") {
		t.Fatalf("expected execution requirement error, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateAllowsExternalTopologyWithExecution(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Members[1].LocalRef = nil
	obj.Spec.Topology.Members[1].AdminSecretRef = &DataguardSecretRef{
		SecretName: "standby-admin",
		SecretKey:  "password",
	}
	obj.Spec.Execution = &DataguardExecutionSpec{
		Image:            "container-registry.oracle.com/database/enterprise:19.3.0",
		ImagePullSecrets: []string{"pull-secret"},
		WalletMountPath:  "/opt/oracle/dg-wallet",
		TNSAdminPath:     "/opt/oracle/dg-net",
	}

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err != nil {
		t.Fatalf("expected external topology with execution to validate, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateAllowsTopologyDefaultsForExternalMember(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Defaults = &DataguardTopologyDefaults{
		AdminSecretRef: &DataguardSecretRef{
			SecretName: "shared-admin",
			SecretKey:  "oracle_pwd",
		},
	}
	obj.Spec.Topology.Members[1].LocalRef = nil
	obj.Spec.Execution = &DataguardExecutionSpec{
		Image: "container-registry.oracle.com/database/enterprise:19.3.0",
	}

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err != nil {
		t.Fatalf("expected topology defaults to satisfy external member secret validation, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateCreateAllowsTopologyDefaultWallet(t *testing.T) {
	obj := validDataguardBrokerTopology()
	obj.Spec.Topology.Defaults = &DataguardTopologyDefaults{
		AdminSecretRef: &DataguardSecretRef{
			SecretName: "shared-admin",
			SecretKey:  "oracle_pwd",
		},
		TCPS: &DataguardTopologyTCPSDefaults{
			ClientWalletSecret: "shared-dg-wallet",
		},
	}
	obj.Spec.Topology.Members[0].TCPS = &DataguardTCPSConfig{Enabled: true}
	obj.Spec.Topology.Members[0].Endpoints = []DataguardEndpointSpec{{
		Protocol:    "TCPS",
		Host:        "primary-a",
		Port:        2484,
		ServiceName: "PRIM",
	}}
	obj.Spec.Topology.Members[1].LocalRef = nil
	obj.Spec.Execution = &DataguardExecutionSpec{
		Image: "container-registry.oracle.com/database/enterprise:19.3.0",
	}

	_, err := (&DataguardBroker{}).ValidateCreate(context.Background(), obj)
	if err != nil {
		t.Fatalf("expected topology default wallet secret to satisfy TCPS validation, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateUpdateRejectsTopologyChangeWhenLocked(t *testing.T) {
	oldObj := validDataguardBrokerTopology()
	oldObj.Status.Status = "CREATING"

	newObj := oldObj.DeepCopy()
	newObj.Spec.Topology.Members[1].Endpoints[0].ServiceName = "STBY_NEW"

	_, err := (&DataguardBroker{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected topology update to be rejected after reconcile start")
	}
	if !strings.Contains(err.Error(), "spec.topology cannot be changed") {
		t.Fatalf("expected topology immutability error, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateUpdateRejectsExecutionChangeWhenLocked(t *testing.T) {
	oldObj := validDataguardBrokerTopology()
	oldObj.Spec.Execution = &DataguardExecutionSpec{
		Image: "oracle-db:19.3.0",
		AuthWallet: &DataguardAuthWalletSpec{
			Enabled:      true,
			RebuildToken: "token-1",
		},
	}
	oldObj.Spec.Topology.Members[1].AdminSecretRef = &DataguardSecretRef{
		SecretName: "standby-admin",
		SecretKey:  "password",
	}
	oldObj.Status.ObservedTopologyHash = "abc123"

	newObj := oldObj.DeepCopy()
	newObj.Spec.Execution.Image = "oracle-db:21.3.0"

	_, err := (&DataguardBroker{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err == nil {
		t.Fatalf("expected execution update to be rejected after reconcile start")
	}
	if !strings.Contains(err.Error(), "spec.execution cannot be changed") {
		t.Fatalf("expected execution immutability error, got: %v", err)
	}
}

func TestDataguardBrokerWebhookValidateUpdateAllowsAuthWalletRebuildTokenChangeWhenLocked(t *testing.T) {
	oldObj := validDataguardBrokerTopology()
	oldObj.Spec.Execution = &DataguardExecutionSpec{
		Image: "oracle-db:19.3.0",
		AuthWallet: &DataguardAuthWalletSpec{
			Enabled:      true,
			RebuildToken: "token-1",
		},
	}
	oldObj.Spec.Topology.Members[1].AdminSecretRef = &DataguardSecretRef{
		SecretName: "standby-admin",
		SecretKey:  "password",
	}
	oldObj.Status.ObservedTopologyHash = "abc123"

	newObj := oldObj.DeepCopy()
	newObj.Spec.Execution.AuthWallet.RebuildToken = "token-2"

	_, err := (&DataguardBroker{}).ValidateUpdate(context.Background(), oldObj, newObj)
	if err != nil {
		t.Fatalf("expected auth wallet rebuild token update to validate, got: %v", err)
	}
}
