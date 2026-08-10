package commons

import (
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
)

func TestMergeCapabilitiesWithDefaultsKeepsDefaultsWhenUserCapsOmitted(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, nil)
	if !reflect.DeepEqual(got, defaultCaps) {
		t.Fatalf("expected defaults to be preserved, got %#v", got)
	}
}

func TestMergeCapabilitiesWithDefaultsDisablesDefaultsForExplicitEmptyObject(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, &corev1.Capabilities{})
	want := &corev1.Capabilities{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit empty capabilities to disable defaults, got %#v", got)
	}
}

func TestMergeCapabilitiesWithDefaultsMergesNonEmptyUserCaps(t *testing.T) {
	defaultCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE"},
		Drop: []corev1.Capability{"ALL"},
	}
	userCaps := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_RAW"},
		Drop: []corev1.Capability{"CHOWN"},
	}

	got := mergeCapabilitiesWithDefaults(defaultCaps, userCaps)
	want := &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_ADMIN", "SYS_NICE", "NET_RAW"},
		Drop: []corev1.Capability{"ALL", "CHOWN"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected merged capabilities %#v, got %#v", want, got)
	}
}

func TestShellQuoteKeepsShellSubstitutionsLiteral(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash not available: %v", err)
	}

	payload := "prod$(printf INJECTED) `uname` 'quoted value'"
	out, err := exec.Command("bash", "-lc", "printf '%s' "+shellQuote(payload)).Output()
	if err != nil {
		t.Fatalf("bash command failed: %v", err)
	}
	if string(out) != payload {
		t.Fatalf("expected shell payload to stay literal %q, got %q", payload, string(out))
	}
}

func TestSQLPlusPodCommandStreamsSQLInsteadOfEmbeddingHereDoc(t *testing.T) {
	sql := "select 1 from dual;\nEOF\ntouch /tmp/runsqlplus_probe\n--"
	cmd := sqlPlusPodCommand()
	joined := strings.Join(cmd, " ")

	if want := []string{"sqlplus", "-s", "/", "as", "sysdba"}; !reflect.DeepEqual(cmd, want) {
		t.Fatalf("expected sqlplus argv %#v, got %#v", want, cmd)
	}
	if len(cmd) == 0 || strings.Contains(joined, "bash") || strings.Contains(joined, "<<EOF") {
		t.Fatalf("expected direct sqlplus argv without shell heredoc, got %#v", cmd)
	}
	if strings.Contains(cmd[0], " ") {
		t.Fatalf("expected sqlplus executable as argv[0], got %#v", cmd)
	}
	if strings.Contains(joined, "runsqlplus_probe") || strings.Contains(joined, "EOF") {
		t.Fatalf("expected sql payload to stay out of pod command argv, got %#v", cmd)
	}

	script := buildSQLPlusScript(sql)
	if !strings.Contains(script, "runsqlplus_probe") || !strings.Contains(script, "\nEOF\n") {
		t.Fatalf("expected SQL payload to be carried only in stdin script, got %q", script)
	}
	if strings.Contains(script, "<<EOF") {
		t.Fatalf("expected stdin script to avoid shell heredoc syntax, got %q", script)
	}
}

func TestBuildSQLPlusScriptUnescapesShellEscapedViews(t *testing.T) {
	for name, sql := range map[string]string{
		"force logging": dbcommons.ForceLoggingTrueSQL,
		"flashback":     dbcommons.FlashBackTrueSQL,
	} {
		script := buildSQLPlusScript(sql)
		if strings.Contains(script, `v\$database`) {
			t.Fatalf("%s: expected SQLPlus stdin script to unescape shell-only view references, got %q", name, script)
		}
		if !strings.Contains(script, "v$database") {
			t.Fatalf("%s: expected SQLPlus stdin script to keep database view references, got %q", name, script)
		}
	}
}

func TestShardingStandbyDatabasePrerequisitesSQLExcludesBrokerSetup(t *testing.T) {
	sql := strings.ToLower(shardingStandbyDatabasePrerequisitesSQL())

	for _, disallowed := range []string{"dg_broker_config_file", "dg_broker_start", "add standby logfile"} {
		if strings.Contains(sql, disallowed) {
			t.Fatalf("expected sharding standby prerequisite SQL to exclude %q, got %q", disallowed, sql)
		}
	}
	for _, required := range []string{"db_create_file_dest", "db_create_online_log_dest_1", "standby_file_management"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("expected sharding standby prerequisite SQL to keep %q, got %q", required, sql)
		}
	}
}

func TestEnsureStandbyRedoLogsSQLAddsOnlyMissingGroups(t *testing.T) {
	primarySQL := strings.ToLower(ensureStandbyRedoLogsSQL(false))
	standbySQL := strings.ToLower(ensureStandbyRedoLogsSQL(true))

	for _, sql := range []string{primarySQL, standbySQL} {
		for _, required := range []string{
			"select count(*) into l_online_groups from v$log where thread# = 1",
			"select count(*) into l_standby_groups from v$standby_log where thread# = 1",
			"l_missing_groups := greatest((l_online_groups + 1) - l_standby_groups, 0)",
			"if l_missing_groups > 0 then",
			"for i in 1..l_missing_groups loop",
			"alter database add standby logfile thread 1 size 200m",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("expected guarded SRL SQL to include %q, got %q", required, sql)
			}
		}
	}

	if strings.Contains(primarySQL, "recover managed standby database cancel") {
		t.Fatalf("expected primary SRL SQL not to cancel managed recovery, got %q", primarySQL)
	}
	if !strings.Contains(standbySQL, "recover managed standby database cancel") {
		t.Fatalf("expected standby SRL SQL to cancel managed recovery before adding missing SRLs, got %q", standbySQL)
	}
}

func TestResetPasswordCommandFallsBackToPython3(t *testing.T) {
	cmd := getResetPasswordCmd()
	joined := strings.Join(cmd, " ")

	if len(cmd) != 3 || cmd[0] != "/bin/bash" || cmd[1] != "-c" {
		t.Fatalf("expected reset password to run through bash fallback, got %#v", cmd)
	}
	if !strings.Contains(joined, "command -v python") || !strings.Contains(joined, "command -v python3") {
		t.Fatalf("expected reset password command to discover python or python3, got %#v", cmd)
	}
	if !strings.Contains(joined, "/opt/oracle/scripts/sharding/cmdExec") ||
		!strings.Contains(joined, "/opt/oracle/scripts/sharding/main.py") ||
		!strings.Contains(joined, "--resetpassword=true") {
		t.Fatalf("expected reset password command to invoke sharding reset script, got %#v", cmd)
	}
}

func TestDgmgrlPodCommandStreamsScriptInsteadOfEmbeddingHereDoc(t *testing.T) {
	connect := "//primary:1521/PRIM_DGMGRL\nEOF\ntouch /tmp/dgmgrl_probe"
	cmd := dgmgrlPodCommand()
	joined := strings.Join(cmd, " ")

	if len(cmd) == 0 || strings.Contains(joined, "bash") || strings.Contains(joined, "<<EOF") {
		t.Fatalf("expected direct dgmgrl argv without shell heredoc, got %#v", cmd)
	}
	if strings.Contains(joined, "dgmgrl_probe") || strings.Contains(joined, "EOF") {
		t.Fatalf("expected DGMGRL payload to stay out of pod command argv, got %#v", cmd)
	}

	script := buildDgmgrlScript("show configuration;\n" + connect)
	if !strings.Contains(script, "dgmgrl_probe") || !strings.Contains(script, "\nEOF\n") {
		t.Fatalf("expected DGMGRL payload to be carried only in stdin script, got %q", script)
	}
	if strings.Contains(script, "<<EOF") {
		t.Fatalf("expected stdin script to avoid shell heredoc syntax, got %q", script)
	}
}

func TestValidateDgmgrlConnectIdentifierRejectsCommandSeparators(t *testing.T) {
	badValues := []string{
		"//primary:1521/PRIM_DGMGRL\nEOF\ntouch /tmp/dgmgrl_probe",
		"//primary:1521/PRIM_DGMGRL\"; show configuration; --",
		"//primary:1521/PRIM_DGMGRL'; show configuration; --",
		"//primary:1521/PRIM_DGMGRL; show configuration",
	}
	for _, value := range badValues {
		if err := validateDgmgrlConnectIdentifier(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}

	if err := validateDgmgrlConnectIdentifier("//primary.example.com:1521/PRIM_DGMGRL"); err != nil {
		t.Fatalf("expected normal connect identifier to be allowed: %v", err)
	}

	got, err := firstValidDgmgrlConnectIdentifier([]string{badValues[0], "//primary.example.com:1521/PRIM_DGMGRL"})
	if err != nil || got != "//primary.example.com:1521/PRIM_DGMGRL" {
		t.Fatalf("expected unsafe value to be skipped in favor of safe fallback, got %q err %v", got, err)
	}
}

func TestGetOnlineShardCmdQuotesShardParamsForShell(t *testing.T) {
	payload := "shard_group=prod$(printf INJECTED);connect=`id`"

	cmd := getOnlineShardCmd(payload)
	if len(cmd) != 3 || cmd[0] != "/bin/bash" || cmd[1] != "-lc" {
		t.Fatalf("expected bash -lc GSM command, got %#v", cmd)
	}

	expectedArg := shellQuote("--checkonlineshard=" + strconv.Quote(payload))
	if !strings.Contains(cmd[2], expectedArg) {
		t.Fatalf("expected online shard arg to be shell-quoted as %q, got %q", expectedArg, cmd[2])
	}
	if strings.Contains(cmd[2], ` "--checkonlineshard=`) {
		t.Fatalf("expected online shard arg not to use double-quoted shell word, got %q", cmd[2])
	}
}

func TestGetShardAddCmdQuotesShardParamsForShell(t *testing.T) {
	payload := "shard_group=prod$(printf INJECTED);connect=`id`;pwd='secret'"

	cmd := getShardAddCmd(payload)
	if len(cmd) != 3 || cmd[0] != "/bin/bash" || cmd[1] != "-lc" {
		t.Fatalf("expected bash -lc GSM command, got %#v", cmd)
	}

	expectedArg := shellQuote("--addshard=" + strconv.Quote(payload))
	if !strings.Contains(cmd[2], expectedArg) {
		t.Fatalf("expected add shard arg to be shell-quoted as %q, got %q", expectedArg, cmd[2])
	}
	if strings.Contains(cmd[2], ` "--addshard=`) {
		t.Fatalf("expected add shard arg not to use double-quoted shell word, got %q", cmd[2])
	}
}

func TestGetGsmExecShellTargetQuotesEnvAndArgs(t *testing.T) {
	payload := "value$(printf INJECTED)`id`'quoted'"
	cmd := getGsmExecShellTarget("/tmp/scripts", map[string]string{
		"ADD_SSPACE_PARAMS": payload,
	}, "--addshard="+strconv.Quote(payload))

	expectedEnv := shellQuote("ADD_SSPACE_PARAMS=" + payload)
	if !strings.Contains(cmd, expectedEnv) {
		t.Fatalf("expected env assignment to be shell-quoted as %q, got %q", expectedEnv, cmd)
	}

	expectedArg := shellQuote("--addshard=" + strconv.Quote(payload))
	if !strings.Contains(cmd, expectedArg) {
		t.Fatalf("expected command arg to be shell-quoted as %q, got %q", expectedArg, cmd)
	}

	if strings.Contains(cmd, `ADD_SSPACE_PARAMS="`) || strings.Contains(cmd, `"--addshard=`) {
		t.Fatalf("expected command target not to use double-quoted shell words, got %q", cmd)
	}
}

func TestGetShardSpaceAddCmdQuotesEnvValueForShell(t *testing.T) {
	payload := "shard_space=space$(printf INJECTED);region=`id`"

	cmd := getShardSpaceAddCmd(payload)
	if len(cmd) != 3 || cmd[0] != "/bin/bash" || cmd[1] != "-lc" {
		t.Fatalf("expected bash -lc GSM command, got %#v", cmd)
	}

	expectedEnv := shellQuote("ADD_SSPACE_PARAMS=" + payload)
	if !strings.Contains(cmd[2], expectedEnv) {
		t.Fatalf("expected env assignment to be shell-quoted as %q, got %q", expectedEnv, cmd[2])
	}
	if strings.Contains(cmd[2], `ADD_SSPACE_PARAMS="`) {
		t.Fatalf("expected env assignment not to use double-quoted shell word, got %q", cmd[2])
	}
}
