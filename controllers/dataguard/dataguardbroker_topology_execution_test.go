package controllers

import (
	"strings"
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestBuildDataguardTopologyTNSAliasesIncludesBrokerStaticAlias(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		DBUniqueName:    "PRIMDB",
		Alias:           "PRIMDB",
		StaticAlias:     "PRIMDB_DGMGRL",
		WalletDirectory: "/opt/oracle/dg-wallet/primdb-wallet",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCPS",
			Host:        "sidb-primary.shns.svc.cluster.local",
			Port:        2484,
			ServiceName: "PRIMDB",
		},
	}

	aliases := buildDataguardTopologyTNSAliases(member)
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}

	if !strings.Contains(aliases[0], "PRIMDB =") || !strings.Contains(aliases[0], "(SERVICE_NAME = PRIMDB)") {
		t.Fatalf("expected normal alias to target PRIMDB service, got:\n%s", aliases[0])
	}
	if !strings.Contains(aliases[1], "PRIMDB_DGMGRL =") || !strings.Contains(aliases[1], "(SERVICE_NAME = PRIMDB_DGMGRL)") {
		t.Fatalf("expected static alias to target PRIMDB_DGMGRL service, got:\n%s", aliases[1])
	}
	if !strings.Contains(aliases[1], "(MY_WALLET_DIRECTORY = /opt/oracle/dg-wallet/primdb-wallet)") {
		t.Fatalf("expected TCPS alias to keep MY_WALLET_DIRECTORY, got:\n%s", aliases[1])
	}
}

func TestBuildDataguardStaticConnectIdentifierReturnsStaticAlias(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		StaticAlias: "STBYDB_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "sidb-standby",
			Port:        1521,
			ServiceName: "STBYDB",
		},
	}

	got := buildDataguardStaticConnectIdentifier(member)
	if got != "STBYDB_DGMGRL" {
		t.Fatalf("expected static alias, got %q", got)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptUsesManagedAliases(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName: "PRIMDB",
				Alias:        "PRIMDB",
				StaticAlias:  "PRIMDB_DGMGRL",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCPS",
					Host:        "sidb-primary.shns.svc.cluster.local",
					Port:        2484,
					ServiceName: "PRIMDB",
				},
			},
			"STBYDB": {
				DBUniqueName: "STBYDB",
				Alias:        "STBYDB",
				StaticAlias:  "STBYDB_DGMGRL",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCPS",
					Host:        "sidb-standby.shns.svc.cluster.local",
					Port:        2484,
					ServiceName: "STBYDB",
				},
			},
		},
	}

	currentMembers := map[string]string{
		"STBYDB": "PHYSICAL_STANDBY",
		"PRIMDB": "PRIMARY",
	}

	script := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers)
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDB';") {
		t.Fatalf("expected primary DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY STATICCONNECTIDENTIFIER='PRIMDB_DGMGRL';") {
		t.Fatalf("expected primary static connect refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY DGConnectIdentifier='STBYDB';") {
		t.Fatalf("expected standby DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY STATICCONNECTIDENTIFIER='STBYDB_DGMGRL';") {
		t.Fatalf("expected standby static connect refresh, got:\n%s", script)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptSkipsUnknownMembers(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName: "PRIMDB",
				Alias:        "PRIMDB",
				StaticAlias:  "PRIMDB_DGMGRL",
			},
		},
	}

	currentMembers := map[string]string{
		"PRIMDB":  "PRIMARY",
		"OTHERDB": "PHYSICAL_STANDBY",
	}

	script := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers)
	if strings.Contains(script, "OTHERDB") {
		t.Fatalf("expected unknown members to be skipped, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDB';") {
		t.Fatalf("expected known member refresh to remain, got:\n%s", script)
	}
}
