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
