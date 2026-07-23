package commons

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	"k8s.io/client-go/rest"
)

// -----------------------------------------------------------------------------
// DGMGRL service/connect helpers
// -----------------------------------------------------------------------------

// BuildDgmgrlServiceName returns the canonical DG broker service name: <DB_UNIQUE_NAME>_DGMGRL.
func BuildDgmgrlServiceName(dbUnique string) string {
	base := strings.ToUpper(strings.TrimSpace(dbUnique))
	return base + "_DGMGRL"
}

// BuildDgmgrlConnectIdentifier returns canonical easy connect identifier.
// Format: //<pod>-0.<svc>.<ns>.svc.cluster.local:1521/<DB_UNIQUE_NAME>_DGMGRL
func BuildDgmgrlConnectIdentifier(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) string {
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shardName, shardName, instance.Namespace)
	return fmt.Sprintf("//%s:1521/%s", host, BuildDgmgrlServiceName(dbUniqueName))
}

// BuildDgmgrlConnectIdentifiers returns canonical connect identifiers.
// Keeping slice form because existing controller flow uses []string.
func BuildDgmgrlConnectIdentifiers(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) []string {
	return []string{
		BuildDgmgrlConnectIdentifier(instance, shardName, dbUniqueName),
	}
}

// BuildDgmgrlStaticConnectIdentifier returns broker StaticConnectIdentifier.
// We use host FQDN + canonical _DGMGRL service name.
func BuildDgmgrlStaticConnectIdentifier(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) string {
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shardName, shardName, instance.Namespace)
	svc := BuildDgmgrlServiceName(dbUniqueName)
	inst := strings.ToUpper(strings.TrimSpace(dbUniqueName))

	return fmt.Sprintf(
		"(DESCRIPTION=(ADDRESS=(PROTOCOL=tcp)(HOST=%s)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=%s)(INSTANCE_NAME=%s)(SERVER=DEDICATED)))",
		host, svc, inst,
	)
}

// -----------------------------------------------------------------------------
// DG broker parameter + start helper (must run on EACH DB: primary + standby)
// -----------------------------------------------------------------------------

// EnsureDgBrokerFilesAndStart prepares broker config files and ensures dg_broker_start is enabled.
func EnsureDgBrokerFilesAndStart(
	podName string,
	dbUnique string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	dbUnique = strings.ToUpper(strings.TrimSpace(dbUnique))
	if dbUnique == "" {
		return fmt.Errorf("dbUnique is empty")
	}

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
set -euo pipefail

mkdir -p /opt/oracle/oradata/dbconfig/%[1]s
chown oracle:oinstall /opt/oracle/oradata/dbconfig/%[1]s || true
chmod 775 /opt/oracle/oradata/dbconfig/%[1]s || true

sqlplus -s / as sysdba <<'EOF'
whenever sqlerror exit 1
set echo on
set pages 0 feedback on verify off heading on

select status from v$instance;

begin
  execute immediate q'[alter system set dg_broker_start=false scope=both sid='*']';
exception when others then
  begin
    execute immediate q'[alter system set dg_broker_start=false scope=memory sid='*']';
  exception when others then null;
  end;
end;
/

alter system set dg_broker_config_file1='/opt/oracle/oradata/dbconfig/%[1]s/dr1%[1]s.dat' scope=both sid='*';
alter system set dg_broker_config_file2='/opt/oracle/oradata/dbconfig/%[1]s/dr2%[1]s.dat' scope=both sid='*';

alter system set dg_broker_start=true scope=both sid='*';

show parameter dg_broker_start
show parameter dg_broker_config_file

exit
EOF
`, dbUnique)}

	stdout, stderr, err := ExecCommand(podName, cmd, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR",
			"EnsureDgBrokerFilesAndStart failed on "+podName+" stdout="+stdout+" stderr="+stderr,
			err, instance, log)
		return err
	}

	LogMessages("INFO", "Ensured DG broker files + started broker on "+podName, nil, instance, log)
	return nil
}

// -----------------------------------------------------------------------------
// DG broker config steps
// -----------------------------------------------------------------------------

// CreateDgBrokerConfigTryConnects creates/validates DG broker config trying each primary connect identifier.
func CreateDgBrokerConfigTryConnects(
	primaryPod string,
	cfgName string,
	primaryDbUniqueName string,
	primaryConnects []string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	for _, c := range primaryConnects {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}

		if err := validateDgmgrlConnectIdentifier(c); err != nil {
			LogMessages("INFO", "Skipping unsafe DG broker primary connect identifier: "+err.Error(), nil, instance, log)
			continue
		}

		script := fmt.Sprintf(`create configuration %s as primary database is %s connect identifier is "%s";
show configuration;`, safeIdent(cfgName), safeIdent(primaryDbUniqueName), c)

		stdout, stderr, err := runDgmgrlScriptInPod(primaryPod, script, instance, kubeConfig, log)
		if err == nil {
			LogMessages("INFO", "Created/verified DG broker config "+cfgName+" using connect "+c, nil, instance, log)
			return nil
		}

		if looksLikeAlreadyExists(stdout, stderr) {
			LogMessages("INFO", "DG config already exists; continuing. "+cfgName, nil, instance, log)
			return nil
		}

		LogMessages("INFO", "CreateDgBrokerConfig failed with connect "+c+"; trying next. stdout="+stdout+" stderr="+stderr, nil, instance, log)
	}

	return fmt.Errorf("CreateDgBrokerConfig failed for all connect identifiers")
}

// AddStandbyToDgBrokerConfigTryConnects adds/validates standby in broker config trying each standby connect identifier.
func AddStandbyToDgBrokerConfigTryConnects(
	primaryPod string,
	standbyDbUniqueName string,
	standbyConnects []string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	for _, c := range standbyConnects {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}

		if err := validateDgmgrlConnectIdentifier(c); err != nil {
			LogMessages("INFO", "Skipping unsafe DG broker standby connect identifier: "+err.Error(), nil, instance, log)
			continue
		}

		script := fmt.Sprintf(`add database %s as connect identifier is "%s" maintained as physical;
show configuration;`, safeIdent(standbyDbUniqueName), c)

		stdout, stderr, err := runDgmgrlScriptInPod(primaryPod, script, instance, kubeConfig, log)
		if err == nil {
			LogMessages("INFO", "Added/verified standby "+standbyDbUniqueName+" using connect "+c, nil, instance, log)
			return nil
		}

		if looksLikeAlreadyExists(stdout, stderr) {
			LogMessages("INFO", "Standby already present; continuing. "+standbyDbUniqueName, nil, instance, log)
			return nil
		}

		LogMessages("INFO", "Add standby failed with connect "+c+"; trying next. stdout="+stdout+" stderr="+stderr, nil, instance, log)
	}

	return fmt.Errorf("AddStandbyToDgBrokerConfig failed for all connect identifiers")
}

// EnableAndValidateDgBroker enables DG broker configuration and validates resulting broker state.
func EnableAndValidateDgBroker(
	primaryPod string,
	cfgName string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {

	stdout, stderr, err := runDgmgrlScriptInPod(primaryPod, "enable configuration;\nshow configuration;", instance, kubeConfig, log)
	if err != nil {
		LogMessages("ERROR", "EnableAndValidateDgBroker failed stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "Enabled/validated DG broker config "+cfgName, nil, instance, log)
	return nil
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func safeIdent(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "")
}

func looksLikeAlreadyExists(stdout, stderr string) bool {
	x := strings.ToLower(stdout + " " + stderr)
	return strings.Contains(x, "already") ||
		strings.Contains(x, "exists") ||
		strings.Contains(x, "ora-165") ||
		strings.Contains(x, "ora-166")
}

func dgmgrlPodCommand() []string {
	return []string{"dgmgrl", "-silent", "/"}
}

func buildDgmgrlScript(script string) string {
	return strings.TrimSpace(script) + "\nexit\n"
}

func runDgmgrlScriptInPod(
	podName string,
	script string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) (string, string, error) {
	return ExecCommandWithInput(podName, dgmgrlPodCommand(), buildDgmgrlScript(script), kubeConfig, instance, log)
}

func firstValidDgmgrlConnectIdentifier(connects []string) (string, error) {
	var lastErr error
	for _, connect := range connects {
		connect = strings.TrimSpace(connect)
		if connect == "" {
			continue
		}
		if err := validateDgmgrlConnectIdentifier(connect); err != nil {
			lastErr = err
			continue
		}
		return connect, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("connect identifier is empty")
}

func validateDgmgrlConnectIdentifier(connect string) error {
	connect = strings.TrimSpace(connect)
	if connect == "" {
		return fmt.Errorf("connect identifier is empty")
	}
	for _, r := range connect {
		switch r {
		case '\n', '\r', '"', ';':
			return fmt.Errorf("connect identifier contains unsupported DGMGRL metacharacter %q", r)
		}
		if r == '\'' {
			return fmt.Errorf("connect identifier contains unsupported DGMGRL metacharacter %q", r)
		}
		if unicode.IsControl(r) {
			return fmt.Errorf("connect identifier contains unsupported control character %q", r)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// SQL helpers
// -----------------------------------------------------------------------------

// RunStandbyDatabasePrerequisitesSQL executes standby prerequisite SQL in the target pod.
func RunStandbyDatabasePrerequisitesSQL(
	podName string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	sql := shardingStandbyDatabasePrerequisitesSQL()
	if sql == "" {
		return fmt.Errorf("StandbyDatabasePrerequisitesSQL is empty")
	}

	return runSQLPlusScriptInPod("RunStandbyDatabasePrerequisitesSQL", podName, sql, instance, kubeConfig, log)
}

func shardingStandbyDatabasePrerequisitesSQL() string {
	lines := strings.Split(dbcommons.StandbyDatabasePrerequisitesSQL, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "dg_broker_config_file") ||
			strings.Contains(lower, "dg_broker_start") ||
			strings.Contains(lower, "add standby logfile") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// RunSQLPlusInPod executes arbitrary SQL in the target pod with SQL*Plus.
func RunSQLPlusInPod(
	podName string,
	sql string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return fmt.Errorf("sql is empty")
	}

	return runSQLPlusScriptInPod("RunSQLPlusInPod", podName, sql, instance, kubeConfig, log)
}

func sqlPlusPodCommand() []string {
	return strings.Fields(dbcommons.SQLPlusCLI)
}

func buildSQLPlusScript(sql string) string {
	return fmt.Sprintf("whenever sqlerror exit 1\n%s\nexit\n", strings.TrimSpace(sqlPlusStdinSQL(sql)))
}

func sqlPlusStdinSQL(sql string) string {
	return strings.ReplaceAll(sql, `\$`, `$`)
}

func runSQLPlusScriptInPod(
	operation string,
	podName string,
	sql string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	script := buildSQLPlusScript(sql)
	stdout, stderr, err := ExecCommandWithInput(podName, sqlPlusPodCommand(), script, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", operation+" failed on "+podName+
			" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", operation+" succeeded on "+podName, nil, instance, log)
	return nil
}

// EnableArchiveLogInPod enables ARCHIVELOG mode in the target pod.
func EnableArchiveLogInPod(
	podName string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	cmdStr := fmt.Sprintf(dbcommons.ArchiveLogTrueCMD, dbcommons.SQLPlusCLI)

	cmd := []string{"bash", "-lc", cmdStr}
	stdout, stderr, err := ExecCommand(podName, cmd, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "EnableArchiveLogInPod failed on "+podName+
			" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "EnableArchiveLogInPod succeeded on "+podName, nil, instance, log)
	return nil
}

// ExecShellInPod executes a shell command in the target pod.
func ExecShellInPod(
	podName string,
	shellCmd string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {
	cmd := []string{"bash", "-lc", shellCmd}
	stdout, stderr, err := ExecCommand(podName, cmd, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "ExecShellInPod failed on "+podName+" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	return nil
}

// -----------------------------------------------------------------------------
// Broker property helpers
// -----------------------------------------------------------------------------

// SetDgBrokerConnectIdentifiers sets DG and static connect identifiers for primary and standby in broker.
func SetDgBrokerConnectIdentifiers(
	primaryPod string,
	primaryShardName string,
	primaryDbUnique string,
	primaryConnects []string,
	standbyShardName string,
	standbyDbUnique string,
	standbyConnects []string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {

	if len(primaryConnects) == 0 {
		return fmt.Errorf("primary connect identifiers are empty for %s", primaryDbUnique)
	}
	if len(standbyConnects) == 0 {
		return fmt.Errorf("standby connect identifiers are empty for %s", standbyDbUnique)
	}

	primaryConn, err := firstValidDgmgrlConnectIdentifier(primaryConnects)
	if err != nil {
		return fmt.Errorf("primary connect identifier is unsafe: %w", err)
	}
	standbyConn, err := firstValidDgmgrlConnectIdentifier(standbyConnects)
	if err != nil {
		return fmt.Errorf("standby connect identifier is unsafe: %w", err)
	}

	primaryStatic := BuildDgmgrlStaticConnectIdentifier(instance, primaryShardName, primaryDbUnique)
	standbyStatic := BuildDgmgrlStaticConnectIdentifier(instance, standbyShardName, standbyDbUnique)

	script := fmt.Sprintf(`edit database %s set property DGConnectIdentifier='%s';
edit database %s set property StaticConnectIdentifier='%s';

edit database %s set property DGConnectIdentifier='%s';
edit database %s set property StaticConnectIdentifier='%s';

show database verbose %s;
show database verbose %s;`,
		safeIdent(primaryDbUnique), primaryConn,
		safeIdent(primaryDbUnique), primaryStatic,

		safeIdent(standbyDbUnique), standbyConn,
		safeIdent(standbyDbUnique), standbyStatic,

		safeIdent(primaryDbUnique),
		safeIdent(standbyDbUnique),
	)

	stdout, stderr, err := runDgmgrlScriptInPod(primaryPod, script, instance, kubeConfig, log)
	if err != nil {
		LogMessages("ERROR",
			"SetDgBrokerConnectIdentifiers failed stdout="+stdout+" stderr="+stderr,
			err, instance, log)
		return err
	}

	LogMessages("INFO", "Set DG broker connect identifiers for "+primaryDbUnique+" and "+standbyDbUnique, nil, instance, log)
	return nil
}

// -----------------------------------------------------------------------------
// SRL / apply helpers
// -----------------------------------------------------------------------------

// EnsureStandbyRedoLogsForShards validates and creates missing standby redo logs on both sides.
func EnsureStandbyRedoLogsForShards(
	primaryPod string,
	standbyPod string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {

	primarySQL := standbyRedoLogSummarySQL()
	if err := RunSQLPlusInPod(primaryPod, primarySQL, instance, kubeConfig, log); err != nil {
		return err
	}

	standbySQL := standbyRedoLogSummarySQL()
	if err := RunSQLPlusInPod(standbyPod, standbySQL, instance, kubeConfig, log); err != nil {
		return err
	}

	if err := RunSQLPlusInPod(primaryPod, ensureStandbyRedoLogsSQL(false), instance, kubeConfig, log); err != nil {
		return err
	}

	if err := RunSQLPlusInPod(standbyPod, ensureStandbyRedoLogsSQL(true), instance, kubeConfig, log); err != nil {
		return err
	}

	LogMessages("INFO", "Ensured standby redo logs on primary and standby", nil, instance, log)
	return nil
}

func standbyRedoLogSummarySQL() string {
	return `
set pages 200 lines 200
prompt === ONLINE REDO (v$log) ===
select thread#, count(*) online_groups, round(max(bytes)/1024/1024) mb
from v$log group by thread# order by thread#;

select group#, thread#, round(bytes/1024/1024) mb, status
from v$log order by group#;

prompt === STANDBY REDO (v$standby_log) ===
select thread#, count(*) srl_groups, round(nvl(max(bytes),0)/1024/1024) mb
from v$standby_log group by thread# order by thread#;

select group#, thread#, round(bytes/1024/1024) mb, status
from v$standby_log order by group#;
`
}

func ensureStandbyRedoLogsSQL(cancelApply bool) string {
	cancelApplySQL := ""
	if cancelApply {
		cancelApplySQL = `
      begin
        execute immediate 'alter database recover managed standby database cancel';
      exception when others then
        null;
      end;`
	}
	return fmt.Sprintf(`
set serveroutput on
declare
  l_online_groups number := 0;
  l_standby_groups number := 0;
  l_missing_groups number := 0;
begin
  select count(*) into l_online_groups from v$log where thread# = 1;
  select count(*) into l_standby_groups from v$standby_log where thread# = 1;
  l_missing_groups := greatest((l_online_groups + 1) - l_standby_groups, 0);

  if l_missing_groups > 0 then%s
    for i in 1..l_missing_groups loop
      execute immediate 'alter database add standby logfile thread 1 size 200M';
    end loop;
  end if;

  dbms_output.put_line('online_groups=' || l_online_groups ||
                       ' standby_groups=' || l_standby_groups ||
                       ' added_standby_groups=' || l_missing_groups);
end;
/
`, cancelApplySQL)
}

// RestartStandbyApplyAndForceRedo restarts standby apply and forces redo generation on primary.
func RestartStandbyApplyAndForceRedo(
	primaryPod string,
	standbyPod string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {

	startApplySQL := `
alter database recover managed standby database using current logfile disconnect from session;
`
	if err := RunSQLPlusInPod(standbyPod, startApplySQL, instance, kubeConfig, log); err != nil {
		return err
	}

	forceRedoSQL := `
alter system archive log current;
alter system archive log current;
`
	if err := RunSQLPlusInPod(primaryPod, forceRedoSQL, instance, kubeConfig, log); err != nil {
		return err
	}

	verifyApplySQL := `
set pages 200 lines 200
select process, status, thread#, sequence# from v$managed_standby order by process;
`
	if err := RunSQLPlusInPod(standbyPod, verifyApplySQL, instance, kubeConfig, log); err != nil {
		return err
	}

	LogMessages("INFO", "Restarted standby apply and forced redo shipping", nil, instance, log)
	return nil
}

// -----------------------------------------------------------------------------
// Redo transport helper
// -----------------------------------------------------------------------------

// ConfigurePrimaryRedoTransport configures primary log_archive_dest_2 to ship redo to standby.
func ConfigurePrimaryRedoTransport(
	primaryPod string,
	standbyShardName string,
	standbyDbUniqueName string,
	instance *databasev4.ShardingDatabase,
	kubeConfig *rest.Config,
	log logr.Logger,
) error {

	standbyConn := BuildDgmgrlConnectIdentifier(instance, standbyShardName, standbyDbUniqueName)

	logArchiveDest2SQL := fmt.Sprintf(
		"alter system set log_archive_dest_2='service=\"%s\" async valid_for=(online_logfiles,primary_role) db_unique_name=%s' scope=both sid='*';",
		standbyConn,
		strings.ToUpper(strings.TrimSpace(standbyDbUniqueName)),
	)

	enableDest2SQL := "alter system set log_archive_dest_state_2=enable scope=both sid='*';"

	switchLogSQL := `
alter system archive log current;
alter system archive log current;
`

	if err := RunSQLPlusInPod(primaryPod, logArchiveDest2SQL, instance, kubeConfig, log); err != nil {
		return err
	}

	if err := RunSQLPlusInPod(primaryPod, enableDest2SQL, instance, kubeConfig, log); err != nil {
		return err
	}

	if err := RunSQLPlusInPod(primaryPod, switchLogSQL, instance, kubeConfig, log); err != nil {
		return err
	}

	LogMessages("INFO", "Configured primary redo transport to standby "+standbyDbUniqueName, nil, instance, log)
	return nil
}
