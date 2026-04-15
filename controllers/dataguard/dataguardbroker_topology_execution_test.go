package controllers

import (
	"strings"
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestBuildDataguardTopologyTNSAliasesIncludesBrokerStaticAlias(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		DBUniqueName:    "PRIMDB",
		Alias:           "PRIMDBTCPS",
		StaticAlias:     "PRIMDBTCPS_DGMGRL",
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

	if !strings.Contains(aliases[0], "PRIMDBTCPS =") || !strings.Contains(aliases[0], "(SERVICE_NAME = PRIMDB)") {
		t.Fatalf("expected normal alias to target PRIMDB service, got:\n%s", aliases[0])
	}
	if !strings.Contains(aliases[1], "PRIMDBTCPS_DGMGRL =") || !strings.Contains(aliases[1], "(SERVICE_NAME = PRIMDB_DGMGRL)") {
		t.Fatalf("expected static alias to target PRIMDB_DGMGRL service, got:\n%s", aliases[1])
	}
	if !strings.Contains(aliases[1], "(MY_WALLET_DIRECTORY = /opt/oracle/dg-wallet/primdb-wallet)") {
		t.Fatalf("expected TCPS alias to keep MY_WALLET_DIRECTORY, got:\n%s", aliases[1])
	}
}

func TestBuildDataguardStaticConnectIdentifierReturnsStaticAlias(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		StaticAlias: "STBYDBTCPS_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCPS",
			Host:        "sidb-standby",
			Port:        2484,
			ServiceName: "STBYDB",
		},
	}

	got := buildDataguardStaticConnectIdentifier(member)
	if got != "STBYDBTCPS_DGMGRL" {
		t.Fatalf("expected static alias, got %q", got)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptUsesManagedAliases(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName: "PRIMDB",
				Alias:        "PRIMDBTCPS",
				StaticAlias:  "PRIMDBTCPS_DGMGRL",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCPS",
					Host:        "sidb-primary.shns.svc.cluster.local",
					Port:        2484,
					ServiceName: "PRIMDB",
				},
			},
			"STBYDB": {
				DBUniqueName: "STBYDB",
				Alias:        "STBYDBTCPS",
				StaticAlias:  "STBYDBTCPS_DGMGRL",
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
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDBTCPS';") {
		t.Fatalf("expected primary DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY STATICCONNECTIDENTIFIER='PRIMDBTCPS_DGMGRL';") {
		t.Fatalf("expected primary static connect refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY DGConnectIdentifier='STBYDBTCPS';") {
		t.Fatalf("expected standby DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY STATICCONNECTIDENTIFIER='STBYDBTCPS_DGMGRL';") {
		t.Fatalf("expected standby static connect refresh, got:\n%s", script)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptSkipsUnknownMembers(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName: "PRIMDB",
				Alias:        "PRIMDBTCPS",
				StaticAlias:  "PRIMDBTCPS_DGMGRL",
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
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDBTCPS';") {
		t.Fatalf("expected known member refresh to remain, got:\n%s", script)
	}
}

func TestResolveDataguardTopologyMemberUsesTCPSAliasAndCanonicalPort(t *testing.T) {
	member := &dbapi.DataguardTopologyMember{
		Name:         "primary-db",
		Role:         "PRIMARY",
		DBUniqueName: "PRIMDB",
		Endpoints: []dbapi.DataguardEndpointSpec{
			{Protocol: "TCP", Host: "primary-tcp", Port: 1521, ServiceName: "PRIMDB"},
			{Protocol: "TCPS", Host: "primary-tcps", Port: 1521, ServiceName: "PRIMDB"},
		},
		TCPS: &dbapi.DataguardTCPSConfig{
			Enabled:            true,
			ClientWalletSecret: "primdb-wallet",
		},
	}

	resolved, err := resolveDataguardTopologyMember(nil, nil, &dbapi.DataguardBroker{
		Spec: dbapi.DataguardBrokerSpec{
			Topology: &dbapi.DataguardTopologySpec{
				Members: []dbapi.DataguardTopologyMember{*member},
			},
		},
	}, &dataguardBrokerExecutionRuntime{WalletMountPath: "/wallet"}, member, false)
	if err != nil {
		t.Fatalf("resolveDataguardTopologyMember returned error: %v", err)
	}
	if resolved.Alias != "PRIMDBTCPS" {
		t.Fatalf("expected TCPS alias PRIMDBTCPS, got %q", resolved.Alias)
	}
	if resolved.StaticAlias != "PRIMDBTCPS_DGMGRL" {
		t.Fatalf("expected TCPS static alias, got %q", resolved.StaticAlias)
	}
	if resolved.Endpoint.Protocol != "TCPS" {
		t.Fatalf("expected canonical TCPS protocol, got %q", resolved.Endpoint.Protocol)
	}
	if resolved.Endpoint.Port != 2484 {
		t.Fatalf("expected canonical TCPS port 2484, got %d", resolved.Endpoint.Port)
	}
	if resolved.Endpoint.Host != "primary-tcps" {
		t.Fatalf("expected TCPS endpoint host, got %q", resolved.Endpoint.Host)
	}
}

func TestResolveDataguardTopologyMemberUsesTCPAliasAndCanonicalPort(t *testing.T) {
	member := &dbapi.DataguardTopologyMember{
		Name:         "standby-db",
		Role:         "PHYSICAL_STANDBY",
		DBUniqueName: "STBYDB",
		Endpoints: []dbapi.DataguardEndpointSpec{
			{Protocol: "TCP", Host: "standby-tcp", Port: 2484, ServiceName: "STBYDB"},
			{Protocol: "TCPS", Host: "standby-tcps", Port: 2484, ServiceName: "STBYDB"},
		},
	}

	resolved, err := resolveDataguardTopologyMember(nil, nil, &dbapi.DataguardBroker{}, &dataguardBrokerExecutionRuntime{}, member, false)
	if err != nil {
		t.Fatalf("resolveDataguardTopologyMember returned error: %v", err)
	}
	if resolved.Alias != "STBYDB" {
		t.Fatalf("expected TCP alias STBYDB, got %q", resolved.Alias)
	}
	if resolved.StaticAlias != "STBYDB_DGMGRL" {
		t.Fatalf("expected TCP static alias, got %q", resolved.StaticAlias)
	}
	if resolved.Endpoint.Protocol != "TCP" {
		t.Fatalf("expected canonical TCP protocol, got %q", resolved.Endpoint.Protocol)
	}
	if resolved.Endpoint.Port != 1521 {
		t.Fatalf("expected canonical TCP port 1521, got %d", resolved.Endpoint.Port)
	}
	if resolved.Endpoint.Host != "standby-tcp" {
		t.Fatalf("expected TCP endpoint host, got %q", resolved.Endpoint.Host)
	}
}
