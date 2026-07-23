package controllers

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
)

func TestDataguardBrokerOutputErrorDetectsBrokerErrors(t *testing.T) {
	if err := dataguardBrokerOutputError("Connected to \"PRIMDB\"\nORA-16532: Oracle Data Guard broker configuration does not exist"); err == nil {
		t.Fatalf("expected ORA-16532 to be treated as a DGMGRL error")
	}
	if err := dataguardBrokerOutputError("Connected to \"STBYDB\"\nConfiguration SUCCESS"); err != nil {
		t.Fatalf("did not expect a broker error: %v", err)
	}
	if err := dataguardBrokerOutputError(
		`Connected to "STBYDB"
	ORA-16596: Member is not part of the Oracle Data Guard broker configuration.`,
	); err == nil {
		t.Fatalf("expected ORA-16596 to be treated as a DGMGRL error")
	}
}

func TestDataguardBrokerConfigurationPreviouslyObserved(t *testing.T) {
	if dataguardBrokerConfigurationPreviouslyObserved(&dbapi.DataguardBroker{}) {
		t.Fatalf("did not expect an empty broker status to be treated as observed")
	}
	broker := &dbapi.DataguardBroker{}
	broker.Status.ResolvedMembers = []dbapi.DataguardResolvedMemberStatus{{
		Name:  "sidb-standby",
		Phase: "Resolved",
	}}
	if dataguardBrokerConfigurationPreviouslyObserved(broker) {
		t.Fatalf("did not expect resolved desired members alone to be treated as observed")
	}
	broker.Status.PrimaryDatabase = "PRIMDB"
	if !dataguardBrokerConfigurationPreviouslyObserved(broker) {
		t.Fatalf("expected populated broker status to be treated as observed")
	}
}

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

	aliases, err := buildDataguardTopologyTNSAliases(member)
	if err != nil {
		t.Fatalf("expected TNS aliases to build, got %v", err)
	}
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

func TestBuildDataguardTopologyTNSAliasesRejectsUnsafeServiceName(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Name:         "primary-a",
		DBUniqueName: "PRIMDB",
		Alias:        "PRIMDB",
		StaticAlias:  "PRIMDB_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "primary-a",
			Port:        1521,
			ServiceName: "PRIMDB)(ADDRESS=(PROTOCOL=TCP)(HOST=evil)(PORT=1521",
		},
	}

	if aliases, err := buildDataguardTopologyTNSAliases(member); err == nil {
		t.Fatalf("expected unsafe service name to be rejected, got %v", aliases)
	} else if !strings.Contains(err.Error(), "invalid TNS service name") {
		t.Fatalf("expected invalid TNS service name error, got %v", err)
	}
}

func TestBuildDataguardTopologyTNSAliasesRejectsUnsafeHost(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Name:         "primary-a",
		DBUniqueName: "PRIMDB",
		Alias:        "PRIMDB",
		StaticAlias:  "PRIMDB_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "primary-a)(ADDRESS=(PROTOCOL=TCP)(HOST=evil)(PORT=1521",
			Port:        1521,
			ServiceName: "PRIMDB",
		},
	}

	if aliases, err := buildDataguardTopologyTNSAliases(member); err == nil {
		t.Fatalf("expected unsafe host to be rejected, got %v", aliases)
	} else if !strings.Contains(err.Error(), "invalid TNS host") {
		t.Fatalf("expected invalid TNS host error, got %v", err)
	}
}

func TestBuildDataguardTopologyTNSAliasesRejectsUnsafeSSLServerDN(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Name:         "primary-a",
		DBUniqueName: "PRIMDB",
		Alias:        "PRIMDBTCPS",
		StaticAlias:  "PRIMDBTCPS_DGMGRL",
		SSLServerDN:  "CN=primary-a\n__CODex_EOF__\ntouch /tmp/dg-poc",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCPS",
			Host:        "primary-a",
			Port:        2484,
			ServiceName: "PRIMDB",
		},
	}

	if aliases, err := buildDataguardTopologyTNSAliases(member); err == nil {
		t.Fatalf("expected unsafe sslServerDN to be rejected, got %v", aliases)
	} else if !strings.Contains(err.Error(), "invalid TNS sslServerDN") {
		t.Fatalf("expected invalid TNS sslServerDN error, got %v", err)
	}
}

func TestBuildDataguardTopologyTNSAliasesAllowsDottedServiceName(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Name:         "primary-a",
		DBUniqueName: "PRIMDB",
		Alias:        "PRIMDB",
		StaticAlias:  "PRIMDB_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "primary-a",
			Port:        1521,
			ServiceName: "primdb.example.com",
		},
	}

	aliases, err := buildDataguardTopologyTNSAliases(member)
	if err != nil {
		t.Fatalf("expected dotted service name to be accepted, got %v", err)
	}
	if !strings.Contains(aliases[0], "(SERVICE_NAME = PRIMDB.EXAMPLE.COM)") {
		t.Fatalf("expected dotted service name in TNS alias, got:\n%s", aliases[0])
	}
}

func TestBuildDataguardWriteFileCommandEncodesTNSContent(t *testing.T) {
	content := "PRIM =\n(DESCRIPTION =\n  (ADDRESS = (PROTOCOL = TCP)(HOST = primary-a\n__CODex_EOF__\ntouch /tmp/dg-poc)(PORT = 1521))\n)\n"
	command := buildDataguardWriteFileCommand("/opt/oracle/dg-net/tnsnames.ora", content)
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	if strings.Contains(command, "__CODex_EOF__") || strings.Contains(command, "touch /tmp/dg-poc") {
		t.Fatalf("expected generated command to avoid raw TNS content, got:\n%s", command)
	}
	if !strings.Contains(command, encoded) {
		t.Fatalf("expected generated command to contain base64 encoded content, got:\n%s", command)
	}
	if !strings.Contains(command, "base64 -d >") {
		t.Fatalf("expected generated command to decode content into file, got:\n%s", command)
	}
}

func TestBuildDataguardStdinWriteFileCommandDoesNotEmbedContent(t *testing.T) {
	command := buildDataguardStdinWriteFileCommand("/tmp/dg secret/admin.sql")

	if strings.Contains(command, "oracle-password") || strings.Contains(command, "sys/") {
		t.Fatalf("expected stdin write command to avoid secret content, got:\n%s", command)
	}
	if !strings.Contains(command, "umask 177") || !strings.Contains(command, "cat > '/tmp/dg secret/admin.sql'") {
		t.Fatalf("expected stdin write command to create protected file from stdin, got:\n%s", command)
	}
}

func TestBuildDataguardAdminPasswordWriteCommandDoesNotEmbedSecret(t *testing.T) {
	secret := "good\nEOF\ntouch /tmp/dg-poc\ncat <<'EOF'"
	command := buildDataguardStdinWriteFileCommand("admin.pwd")

	if strings.Contains(command, secret) || strings.Contains(command, "touch /tmp/dg-poc") || strings.Contains(command, "\nEOF\n") {
		t.Fatalf("expected admin password write command to avoid raw secret content, got:\n%s", command)
	}
	if !strings.Contains(command, "umask 177") || !strings.Contains(command, "cat > 'admin.pwd'") {
		t.Fatalf("expected admin password write command to create protected file from stdin, got:\n%s", command)
	}
}

func TestBuildDataguardRunnerDGMGRLScriptKeepsPasswordOutOfCommand(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Alias:         "PRIMDB",
		AdminPassword: "oracle-password",
	}
	script := buildDataguardRunnerDGMGRLScript(member, "SHOW CONFIGURATION;\n")
	command := mirrorDataguardRunnerCommandToContainerLogs(
		fmt.Sprintf("dgmgrl -silent @%s", shellQuote("/tmp/dgmgrl-topology.cmd")),
		fmt.Sprintf("rm -f %s", shellQuote("/tmp/dgmgrl-topology.cmd")),
	)

	if !strings.Contains(script, `CONNECT sys/"oracle-password"@PRIMDB;`) {
		t.Fatalf("expected DGMGRL script to contain connect command, got:\n%s", script)
	}
	if strings.Contains(command, "oracle-password") || strings.Contains(command, `sys/"`) {
		t.Fatalf("expected DGMGRL command argv text to avoid password descriptor, got:\n%s", command)
	}
	if !strings.Contains(command, "dgmgrl -silent @'/tmp/dgmgrl-topology.cmd'") {
		t.Fatalf("expected DGMGRL command to execute script file without connect argv, got:\n%s", command)
	}
}

func TestBuildDataguardConfigurationMembersSQLScriptKeepsPasswordOutOfCommand(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Alias:         "PRIMDB",
		AdminPassword: "oracle-password",
	}
	script := buildDataguardConfigurationMembersSQLScript(member)
	command := mirrorDataguardRunnerCommandToContainerLogs(
		fmt.Sprintf("sqlplus -s /nolog @%s", shellQuote("/tmp/dg-broker-members.sql")),
		fmt.Sprintf("rm -f %s", shellQuote("/tmp/dg-broker-members.sql")),
	)

	if !strings.Contains(script, `connect sys/"oracle-password"@PRIMDB as sysdba`) {
		t.Fatalf("expected SQLPlus script to contain connect command, got:\n%s", script)
	}
	if strings.Contains(command, "oracle-password") || strings.Contains(command, `sys/"`) {
		t.Fatalf("expected SQLPlus command argv text to avoid password descriptor, got:\n%s", command)
	}
	if !strings.Contains(command, "sqlplus -s /nolog @'/tmp/dg-broker-members.sql'") {
		t.Fatalf("expected SQLPlus command to use /nolog and script file, got:\n%s", command)
	}
}

func TestBuildDataguardObserverStartupCommandQuotesDGMGRLArguments(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		Runtime: &dataguardBrokerExecutionRuntime{
			TNSAdminPath: "/tmp/tns; touch /tmp/path-poc",
		},
		Members: []*dataguardTopologyResolvedMember{{
			DBUniqueName: "PRIMDB",
			Alias:        "PRIMDB",
			StaticAlias:  "PRIMDB_DGMGRL",
			Endpoint: dbapi.DataguardEndpointSpec{
				Protocol:    "TCP",
				Host:        "primary-a",
				Port:        1521,
				ServiceName: "PRIM",
			},
		}},
	}
	currentPrimary := &dataguardTopologyResolvedMember{
		Alias:         "PRIMDB; touch /tmp/alias-poc",
		AdminPassword: "oracle-password",
	}

	command, err := buildDataguardObserverStartupCommand(state, currentPrimary, "dg-observer; touch /tmp/observer-poc")
	if err != nil {
		t.Fatalf("expected observer startup command to build, got %v", err)
	}

	if !strings.Contains(command, "nohup dgmgrl -echo 'sys@PRIMDB; touch /tmp/alias-poc' @'/tmp/fsfo-observer.cmd' < /tmp/admin.pwd > '/tmp/fsfo_observer_stdout.log' 2>&1 &") {
		t.Fatalf("expected observer startup command to run dgmgrl with nohup in the background, got:\n%s", command)
	}
	expectedStopCommand := base64.StdEncoding.EncodeToString([]byte("STOP OBSERVER dg-observer; touch /tmp/observer-poc;\n"))
	if !strings.Contains(command, "printf %s '"+expectedStopCommand+"' | base64 -d > '/tmp/fsfo-observer-stop.cmd'") {
		t.Fatalf("expected observer startup command to stop stale observer registration before startup, got:\n%s", command)
	}
	if !strings.Contains(command, "dgmgrl -echo 'sys@PRIMDB; touch /tmp/alias-poc' @'/tmp/fsfo-observer-stop.cmd' < /tmp/admin.pwd > '/tmp/fsfo_observer_stop_stdout.log' 2>&1 || true") {
		t.Fatalf("expected observer startup command to tolerate stale observer cleanup failures, got:\n%s", command)
	}
	if !strings.Contains(command, "sleep 3") || !strings.Contains(command, "ps -ef | grep -i '[d]gmgrl'") || !strings.Contains(command, "tail -50 '/tmp/fsfo_observer_stdout.log'") {
		t.Fatalf("expected observer startup command to emit startup diagnostics, got:\n%s", command)
	}
	if !strings.Contains(command, "wait \"${observer_pid}\"") {
		t.Fatalf("expected observer pod to wait on the background dgmgrl process, got:\n%s", command)
	}
	if strings.Contains(command, "__CODex_TNS__") || strings.Contains(command, "__CODex_SQLNET__") || strings.Contains(command, "__CODex_PWD__") || strings.Contains(command, "<<") {
		t.Fatalf("expected observer startup command to avoid heredoc markers, got:\n%s", command)
	}
	if !strings.Contains(command, "base64 -d > '/tmp/tns; touch /tmp/path-poc/tnsnames.ora'") {
		t.Fatalf("expected TNS admin path to be shell-quoted in file write, got:\n%s", command)
	}
}

func TestBuildDataguardTopologyDisableFSFOScriptStopsTopologyObserver(t *testing.T) {
	script := buildDataguardTopologyDisableFSFOScript("dg-prod")

	if !strings.Contains(script, "STOP OBSERVER dg-prod-observer;\nDISABLE FAST_START FAILOVER;") {
		t.Fatalf("expected topology FSFO disable script to stop the topology observer before disabling FSFO, got %q", script)
	}
	if strings.Contains(script, "STOP OBSERVER dg-prod;\n") {
		t.Fatalf("expected topology FSFO disable script to use the observer pod name, got %q", script)
	}
}

func TestBuildDataguardStaticConnectIdentifierReturnsDescriptor(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		DBUniqueName: "STBYDB",
		StaticAlias:  "STBYDBTCPS_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "sidb-standby",
			Port:        1521,
			ServiceName: "STBYDB",
		},
	}

	got, err := buildDataguardStaticConnectIdentifier(member)
	if err != nil {
		t.Fatalf("expected static descriptor to validate, got %v", err)
	}
	if got != "'(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=sidb-standby)(PORT=1521))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=STBYDB_DGMGRL)))'" {
		t.Fatalf("expected static descriptor, got %q", got)
	}
}

func TestBuildDataguardStaticConnectIdentifierRejectsUnsafeHost(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Name:         "standby-a",
		DBUniqueName: "STBYDB",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "sidb-standby)(ADDRESS=(PROTOCOL=TCP)(HOST=evil)(PORT=1521",
			Port:        1521,
			ServiceName: "STBYDB",
		},
	}

	if got, err := buildDataguardStaticConnectIdentifier(member); err == nil {
		t.Fatalf("expected unsafe static descriptor to be rejected, got %q", got)
	} else if !strings.Contains(err.Error(), "invalid static connect identifier") {
		t.Fatalf("expected invalid static connect identifier error, got %v", err)
	}
}

func TestBuildDataguardDGMGRLConnectIdentifierPrefersResolvedConnectString(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Alias:         "STBYDBTCPS",
		ConnectString: "sidb-standby.shns.svc.cluster.local:2484/STBYDB",
	}

	got, err := dataguardDGMGRLConnectIdentifier(member)
	if err != nil {
		t.Fatalf("expected connect identifier to build, got %v", err)
	}
	if got != "'sidb-standby.shns.svc.cluster.local:2484/STBYDB'" {
		t.Fatalf("expected resolved connect string literal, got %q", got)
	}
}

func TestBuildDataguardDGMGRLConnectIdentifierUsesAliasForTCPS(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Alias:         "STBYDBTCPS",
		ConnectString: "sidb-standby.shns.svc.cluster.local:2484/STBYDB",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol: "TCPS",
		},
	}

	got, err := dataguardDGMGRLConnectIdentifier(member)
	if err != nil {
		t.Fatalf("expected connect identifier to build, got %v", err)
	}
	if got != "'STBYDBTCPS'" {
		t.Fatalf("expected TCPS alias literal, got %q", got)
	}
}

func TestBuildDataguardDGMGRLConnectIdentifierRejectsUnsafeConnectString(t *testing.T) {
	member := &dataguardTopologyResolvedMember{
		Alias:         "STBYDB",
		ConnectString: "sidb-standby:1521/STBYDB'; DISABLE CONFIGURATION; --",
	}

	if got, err := dataguardDGMGRLConnectIdentifier(member); err == nil {
		t.Fatalf("expected unsafe connect string to be rejected, got %q", got)
	} else if !strings.Contains(err.Error(), "unsafe DGMGRL string literal") {
		t.Fatalf("expected unsafe DGMGRL string literal error, got %v", err)
	}
}

func TestBuildDataguardTopologyCreateConfigurationScriptUsesResolvedConnectStrings(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		Primary: &dataguardTopologyResolvedMember{
			DBUniqueName:   "PRIMDB",
			Alias:          "PRIMDB",
			ConnectString:  "sidb-primary.shns.svc.cluster.local:1521/PRIMDB",
			StaticAlias:    "PRIMDB_DGMGRL",
			Role:           "PRIMARY",
			ResourceName:   "sidb-primary",
			AdminSecretKey: "oracle_pwd",
		},
		DesiredStandbys: []*dataguardTopologyResolvedMember{{
			DBUniqueName:  "STBYDB",
			Alias:         "STBYDB",
			ConnectString: "sidb-standby.shns.svc.cluster.local:1521/STBYDB",
			StaticAlias:   "STBYDB_DGMGRL",
			Role:          "PHYSICAL_STANDBY",
		}},
		DesiredPhysicalMembers: []*dataguardTopologyResolvedMember{
			{
				DBUniqueName:  "PRIMDB",
				Alias:         "PRIMDB",
				ConnectString: "sidb-primary.shns.svc.cluster.local:1521/PRIMDB",
				StaticAlias:   "PRIMDB_DGMGRL",
				Role:          "PRIMARY",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCP",
					Host:        "sidb-primary.shns.svc.cluster.local",
					Port:        1521,
					ServiceName: "PRIMDB",
				},
			},
			{
				DBUniqueName:  "STBYDB",
				Alias:         "STBYDB",
				ConnectString: "sidb-standby.shns.svc.cluster.local:1521/STBYDB",
				StaticAlias:   "STBYDB_DGMGRL",
				Role:          "PHYSICAL_STANDBY",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCP",
					Host:        "sidb-standby.shns.svc.cluster.local",
					Port:        1521,
					ServiceName: "STBYDB",
				},
			},
		},
	}

	script, err := buildDataguardTopologyCreateConfigurationScript(&dataguardBrokerDesiredSpec{}, state)
	if err != nil {
		t.Fatalf("expected create configuration script to build, got %v", err)
	}
	if !strings.Contains(script, "CREATE CONFIGURATION dg_config AS PRIMARY DATABASE IS PRIMDB CONNECT IDENTIFIER IS 'sidb-primary.shns.svc.cluster.local:1521/PRIMDB';") {
		t.Fatalf("expected primary create configuration connect string, got:\n%s", script)
	}
	if !strings.Contains(script, "ADD DATABASE STBYDB AS CONNECT IDENTIFIER IS 'sidb-standby.shns.svc.cluster.local:1521/STBYDB';") {
		t.Fatalf("expected standby add database connect string, got:\n%s", script)
	}
}

func TestBuildDataguardTopologyAddDatabaseScriptUsesResolvedConnectStrings(t *testing.T) {
	currentPrimary := &dataguardTopologyResolvedMember{
		DBUniqueName:  "PRIMDB",
		Alias:         "PRIMDB",
		ConnectString: "sidb-primary.shns.svc.cluster.local:1521/PRIMDB",
	}
	missing := []*dataguardTopologyResolvedMember{{
		DBUniqueName:  "STBYDB",
		Alias:         "STBYDB",
		ConnectString: "sidb-standby.shns.svc.cluster.local:1521/STBYDB",
		StaticAlias:   "STBYDB_DGMGRL",
		Endpoint: dbapi.DataguardEndpointSpec{
			Protocol:    "TCP",
			Host:        "sidb-standby.shns.svc.cluster.local",
			Port:        1521,
			ServiceName: "STBYDB",
		},
	}}

	script, err := buildDataguardTopologyAddDatabaseScript(&dataguardBrokerDesiredSpec{}, currentPrimary, missing)
	if err != nil {
		t.Fatalf("expected add database script to build, got %v", err)
	}
	if !strings.Contains(script, "ADD DATABASE STBYDB AS CONNECT IDENTIFIER IS 'sidb-standby.shns.svc.cluster.local:1521/STBYDB';") {
		t.Fatalf("expected add database connect string, got:\n%s", script)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptUsesResolvedConnectStrings(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName:  "PRIMDB",
				Alias:         "PRIMDBTCPS",
				ConnectString: "sidb-primary.shns.svc.cluster.local:2484/PRIMDB",
				StaticAlias:   "PRIMDBTCPS_DGMGRL",
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCPS",
					Host:        "sidb-primary.shns.svc.cluster.local",
					Port:        2484,
					ServiceName: "PRIMDB",
				},
			},
			"STBYDB": {
				DBUniqueName:  "STBYDB",
				Alias:         "STBYDBTCPS",
				ConnectString: "sidb-standby.shns.svc.cluster.local:2484/STBYDB",
				StaticAlias:   "STBYDBTCPS_DGMGRL",
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

	script, err := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers)
	if err != nil {
		t.Fatalf("expected refresh script to build, got %v", err)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDBTCPS';") {
		t.Fatalf("expected primary TCPS alias DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=sidb-primary.shns.svc.cluster.local)(PORT=2484))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=PRIMDB_DGMGRL))(SECURITY=(SSL_SERVER_DN_MATCH=NO)))';") {
		t.Fatalf("expected primary static connect refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY DGConnectIdentifier='STBYDBTCPS';") {
		t.Fatalf("expected standby TCPS alias DGConnectIdentifier refresh, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE STBYDB SET PROPERTY STATICCONNECTIDENTIFIER='(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=sidb-standby.shns.svc.cluster.local)(PORT=2484))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=STBYDB_DGMGRL))(SECURITY=(SSL_SERVER_DN_MATCH=NO)))';") {
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
				Endpoint: dbapi.DataguardEndpointSpec{
					Protocol:    "TCPS",
					Host:        "sidb-primary.shns.svc.cluster.local",
					Port:        2484,
					ServiceName: "PRIMDB",
				},
			},
		},
	}

	currentMembers := map[string]string{
		"PRIMDB":  "PRIMARY",
		"OTHERDB": "PHYSICAL_STANDBY",
	}

	script, err := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers)
	if err != nil {
		t.Fatalf("expected refresh script to build, got %v", err)
	}
	if strings.Contains(script, "OTHERDB") {
		t.Fatalf("expected unknown members to be skipped, got:\n%s", script)
	}
	if !strings.Contains(script, "EDIT DATABASE PRIMDB SET PROPERTY DGConnectIdentifier='PRIMDBTCPS';") {
		t.Fatalf("expected known member refresh to remain, got:\n%s", script)
	}
}

func TestBuildDataguardTopologyRefreshConnectIdentifiersScriptRejectsUnsafeAlias(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		MembersByDBUniqueName: map[string]*dataguardTopologyResolvedMember{
			"PRIMDB": {
				DBUniqueName: "PRIMDB",
				Alias:        "PRIMDB'; DISABLE CONFIGURATION; --",
				StaticAlias:  "PRIMDB_DGMGRL",
			},
		},
	}

	currentMembers := map[string]string{
		"PRIMDB": "PRIMARY",
	}

	if script, err := buildDataguardTopologyRefreshConnectIdentifiersScript(state, currentMembers); err == nil {
		t.Fatalf("expected unsafe connect identifier to be rejected, got:\n%s", script)
	} else if !strings.Contains(err.Error(), "unsafe DGMGRL identifier") {
		t.Fatalf("expected unsafe DGMGRL identifier error, got %v", err)
	}
}

func TestDataguardTopologyMissingStandbysReturnsOnlyUnconfiguredStandbys(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		DesiredStandbys: []*dataguardTopologyResolvedMember{
			{DBUniqueName: "STBYB"},
			{DBUniqueName: "STBYA"},
			nil,
			{DBUniqueName: "  "},
		},
	}

	currentMembers := map[string]string{
		"PRIMDB": "PRIMARY",
		"STBYA":  "PHYSICAL_STANDBY",
	}

	missing := dataguardTopologyMissingStandbys(state, currentMembers)
	if len(missing) != 1 || missing[0] != "STBYB" {
		t.Fatalf("expected only STBYB to be missing, got %#v", missing)
	}
}

func TestIsDataguardTopologyLocalMemberNotReady(t *testing.T) {
	err := fmt.Errorf("%w: local member sidb-standby does not have a ready database pod yet", errDataguardTopologyLocalMemberNotReady)
	if !isDataguardTopologyLocalMemberNotReady(err) {
		t.Fatalf("expected local-member-not-ready error to be detected")
	}
	if isDataguardTopologyLocalMemberNotReady(fmt.Errorf("some other error")) {
		t.Fatalf("did not expect unrelated error to be detected")
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

func TestResolveDataguardTopologyMemberRejectsUnsafeDBUniqueName(t *testing.T) {
	member := &dbapi.DataguardTopologyMember{
		Name:         "primary-db",
		Role:         "PRIMARY",
		DBUniqueName: "PRIMDB' scope=both sid='*'; shutdown immediate; --",
		Endpoints: []dbapi.DataguardEndpointSpec{{
			Protocol:    "TCP",
			Host:        "primary-tcp",
			Port:        1521,
			ServiceName: "PRIMDB",
		}},
	}

	if _, err := resolveDataguardTopologyMember(nil, nil, &dbapi.DataguardBroker{}, &dataguardBrokerExecutionRuntime{}, member, false); err == nil {
		t.Fatalf("expected unsafe dbUniqueName to be rejected")
	} else if !strings.Contains(err.Error(), "invalid DB_UNIQUE_NAME") {
		t.Fatalf("expected DB_UNIQUE_NAME validation error, got %v", err)
	}
}

func TestResolveDataguardTopologyMemberRejectsUnsafeHost(t *testing.T) {
	member := &dbapi.DataguardTopologyMember{
		Name:         "primary-db",
		Role:         "PRIMARY",
		DBUniqueName: "PRIMDB",
		Endpoints: []dbapi.DataguardEndpointSpec{{
			Protocol:    "TCP",
			Host:        "primary-tcp)(ADDRESS=(PROTOCOL=TCP)(HOST=evil)(PORT=1521",
			Port:        1521,
			ServiceName: "PRIMDB",
		}},
	}

	if _, err := resolveDataguardTopologyMember(nil, nil, &dbapi.DataguardBroker{}, &dataguardBrokerExecutionRuntime{}, member, false); err == nil {
		t.Fatalf("expected unsafe host to be rejected")
	} else if !strings.Contains(err.Error(), "invalid host") {
		t.Fatalf("expected host validation error, got %v", err)
	}
}

func TestResolveDataguardTopologyMemberRejectsUnsafeSSLServerDN(t *testing.T) {
	member := &dbapi.DataguardTopologyMember{
		Name:         "primary-db",
		Role:         "PRIMARY",
		DBUniqueName: "PRIMDB",
		TCPS:         &dbapi.DataguardTCPSConfig{Enabled: true},
		Endpoints: []dbapi.DataguardEndpointSpec{{
			Protocol:    "TCPS",
			Host:        "primary-tcps",
			Port:        2484,
			ServiceName: "PRIMDB",
			SSLServerDN: "CN=primary-a\n__CODex_EOF__\ntouch /tmp/dg-poc",
		}},
	}

	if _, err := resolveDataguardTopologyMember(nil, nil, &dbapi.DataguardBroker{}, &dataguardBrokerExecutionRuntime{}, member, false); err == nil {
		t.Fatalf("expected unsafe sslServerDN to be rejected")
	} else if !strings.Contains(err.Error(), "invalid sslServerDN") {
		t.Fatalf("expected sslServerDN validation error, got %v", err)
	}
}

func TestBuildDataguardTopologyLogArchiveConfigValueSortsAndDeduplicatesMembers(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		Members: []*dataguardTopologyResolvedMember{
			{DBUniqueName: "stbydb"},
			{DBUniqueName: "PRIMDB"},
			{DBUniqueName: "STBYDB"},
			nil,
			{DBUniqueName: "  "},
		},
	}

	got, err := buildDataguardTopologyLogArchiveConfigValue(state)
	if err != nil {
		t.Fatalf("expected DG config value to build, got %v", err)
	}
	if got != "PRIMDB,STBYDB" {
		t.Fatalf("expected sorted unique DG config value, got %q", got)
	}
}

func TestBuildDataguardTopologyLogArchiveConfigValueRejectsUnsafeDBUniqueName(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		Members: []*dataguardTopologyResolvedMember{
			{
				Name:         "primary-db",
				DBUniqueName: "PRIMDB' scope=both sid='*'; shutdown immediate; --",
			},
		},
	}

	if got, err := buildDataguardTopologyLogArchiveConfigValue(state); err == nil {
		t.Fatalf("expected unsafe DB_UNIQUE_NAME to be rejected, got %q", got)
	} else if !strings.Contains(err.Error(), "invalid log_archive_config DB_UNIQUE_NAME") {
		t.Fatalf("expected log_archive_config DB_UNIQUE_NAME validation error, got %v", err)
	}
}

func TestBuildDataguardTopologyLogArchiveConfigSQLIncludesShowParameter(t *testing.T) {
	state := &dataguardTopologyResolvedState{
		Members: []*dataguardTopologyResolvedMember{
			{DBUniqueName: "PRIMDB"},
			{DBUniqueName: "STBYDB"},
		},
	}

	sql, err := buildDataguardTopologyLogArchiveConfigSQL(state)
	if err != nil {
		t.Fatalf("expected log_archive_config SQL to build, got %v", err)
	}
	if !strings.Contains(sql, "ALTER SYSTEM SET log_archive_config='dg_config=(PRIMDB,STBYDB)' scope=both sid='*';") {
		t.Fatalf("expected log_archive_config alter system SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "SHOW PARAMETER log_archive_config;") {
		t.Fatalf("expected show parameter verification SQL, got:\n%s", sql)
	}
}

func TestParseDataguardTopologyObservedConfiguration(t *testing.T) {
	out := `Connected to "PRIMDB"

Configuration - dg_config

  Protection Mode: MaxAvailability
  Members:
  stbydb - Primary database
    primdb - Physical standby database

Fast-Start Failover:  Disabled

Configuration Status:
SUCCESS   (status updated 23 seconds ago)
`

	observed := parseDataguardTopologyObservedConfiguration(out)
	if observed.ProtectionMode != "MaxAvailability" {
		t.Fatalf("expected MaxAvailability, got %q", observed.ProtectionMode)
	}
	if observed.ConfigurationStatus != "SUCCESS   (status updated 23 seconds ago)" {
		t.Fatalf("unexpected configuration status %q", observed.ConfigurationStatus)
	}
}

func TestDataguardTopologyConfigurationStatusReady(t *testing.T) {
	if !dataguardTopologyConfigurationStatusReady("SUCCESS   (status updated 23 seconds ago)") {
		t.Fatalf("expected SUCCESS status with suffix to be ready")
	}
	if dataguardTopologyConfigurationStatusReady("WARNING   (status updated 23 seconds ago)") {
		t.Fatalf("expected WARNING status to be not ready")
	}
}

func TestObservedDataguardTopologyProtectionModeFallsBack(t *testing.T) {
	if got := observedDataguardTopologyProtectionMode(&dataguardTopologyObservedConfiguration{ProtectionMode: "MaxAvailability"}, &dataguardBrokerDesiredSpec{ProtectionMode: "MaxPerformance"}); got != "MaxAvailability" {
		t.Fatalf("expected observed mode, got %q", got)
	}
	if got := observedDataguardTopologyProtectionMode(nil, &dataguardBrokerDesiredSpec{ProtectionMode: "MaxPerformance"}); got != "MaxPerformance" {
		t.Fatalf("expected desired fallback, got %q", got)
	}
	if got := observedDataguardTopologyProtectionMode(nil, nil); got != "MaxPerformance" {
		t.Fatalf("expected default fallback, got %q", got)
	}
}

func TestNormalizeTopologyMemberRoleAcceptsBrokerFormatting(t *testing.T) {
	if got := normalizeTopologyMemberRole("PHYSICAL STANDBY"); got != "PHYSICAL_STANDBY" {
		t.Fatalf("expected PHYSICAL_STANDBY, got %q", got)
	}
	if got := normalizeTopologyMemberRole("snapshot-standby"); got != "SNAPSHOT_STANDBY" {
		t.Fatalf("expected SNAPSHOT_STANDBY, got %q", got)
	}
	if got := normalizeTopologyMemberRole("primary"); got != "PRIMARY" {
		t.Fatalf("expected PRIMARY, got %q", got)
	}
}
