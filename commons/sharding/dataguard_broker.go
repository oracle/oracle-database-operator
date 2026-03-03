package commons

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// -----------------------------------------------------------------------------
// DGMGRL service/connect helpers
// -----------------------------------------------------------------------------
// Canonical service name: <DB_UNIQUE_NAME>_DGMGRL
func BuildDgmgrlServiceName(dbUnique string) string {
	base := strings.ToUpper(strings.TrimSpace(dbUnique))
	return base + "_DGMGRL"
}

// Legacy/typo service name some images/scripts expose: <DB_UNIQUE_NAME>_DGMRL
func BuildDgmrLServiceName(dbUnique string) string {
	base := strings.ToUpper(strings.TrimSpace(dbUnique))
	return base + "_DGMRL"
}

// BuildDgmgrlConnectIdentifier returns canonical connect identifier.
//
//	<shard>-0.<shard>.<ns>.svc.cluster.local:1521/<DB_UNIQUE_NAME>_DGMGRL
func BuildDgmgrlConnectIdentifier(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) string {
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shardName, shardName, instance.Namespace)
	return fmt.Sprintf("%s:1521/%s", host, BuildDgmgrlServiceName(dbUniqueName))
}

// BuildDgmgrlConnectIdentifiers tries canonical first, then legacy/typo.
// Recommended for robustness while keeping customer YAML canonical.
func BuildDgmgrlConnectIdentifiers(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) []string {
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shardName, shardName, instance.Namespace)
	base := strings.ToUpper(strings.TrimSpace(dbUniqueName))
	if base == "" {
		base = strings.ToUpper(strings.TrimSpace(shardName))
	}

	// prefer correct _DGMGRL service, add old typo fallback just in case
	svc1 := fmt.Sprintf("%s_DGMGRL", base)
	svc2 := fmt.Sprintf("%s_DGMRL", base) // fallback (typo seen in some setups)

	return []string{
		fmt.Sprintf("//%s:1521/%s", host, svc1),
		fmt.Sprintf("%s:1521/%s", host, svc1),
		fmt.Sprintf("//%s:1521/%s", host, svc2),
		fmt.Sprintf("%s:1521/%s", host, svc2),
	}
}

// -----------------------------------------------------------------------------
// DG broker parameter + start helper (must run on EACH DB: primary + standby)
// -----------------------------------------------------------------------------
// Ensures:
// - dg_broker_start is toggled OFF (so we can change broker files)
// - dg_broker_config_file1/2 point to per-DB location under dbconfig/<DB_UNIQUE_NAME>
// - dg_broker_start is ON again
//
// This is the proven sequence you tested manually (avoids ORA-16573/ORA-16604).
func EnsureDgBrokerFilesAndStart(
	podName string,
	dbUnique string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
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

-- NOMOUNT-safe readiness check
select status from v$instance;

-- Stop broker so file params can be changed (avoid ORA-16573/ORA-16604)
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

	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
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
// DG broker config steps (TryConnects variants only; no duplicates)
// -----------------------------------------------------------------------------
// CreateDgBrokerConfigTryConnects creates broker configuration on PRIMARY.
// It tries connect identifiers in order (DGMGRL then DGMRL fallback).
func CreateDgBrokerConfigTryConnects(
	primaryPod string,
	cfgName string,
	primaryDbUniqueName string,
	primaryConnects []string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {
	for _, c := range primaryConnects {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}

		cmd := []string{"bash", "-lc", fmt.Sprintf(`
dgmgrl -silent / <<'EOF'
create configuration %s as primary database is %s connect identifier is "%s";
show configuration;
exit
EOF
`, safeIdent(cfgName), safeIdent(primaryDbUniqueName), c)}

		stdout, stderr, err := ExecCommand(primaryPod, cmd, kubeClient, kubeConfig, instance, log)
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

// AddStandbyToDgBrokerConfigTryConnects adds standby to existing broker config (run on primary).
// It tries connect identifiers in order (DGMGRL then DGMRL fallback).
func AddStandbyToDgBrokerConfigTryConnects(
	primaryPod string,
	standbyDbUniqueName string,
	standbyConnects []string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {
	for _, c := range standbyConnects {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}

		cmd := []string{"bash", "-lc", fmt.Sprintf(`
dgmgrl -silent / <<'EOF'
add database %s as connect identifier is "%s" maintained as physical;
show configuration;
exit
EOF
`, safeIdent(standbyDbUniqueName), c)}

		stdout, stderr, err := ExecCommand(primaryPod, cmd, kubeClient, kubeConfig, instance, log)
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

// EnableAndValidateDgBroker enables config and prints status.
func EnableAndValidateDgBroker(
	primaryPod string,
	cfgName string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	cmd := []string{"bash", "-lc", `
dgmgrl -silent / <<'EOF'
enable configuration;
show configuration;
exit
EOF
`}
	stdout, stderr, err := ExecCommand(primaryPod, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "EnableAndValidateDgBroker failed stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "Enabled/validated DG broker config "+cfgName, nil, instance, log)
	return nil
}

// ---------- helpers ----------
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

// RunStandbyDatabasePrerequisitesSQL runs the same prereq SQL used by SIDB flow
// (dbcommons.StandbyDatabasePrerequisitesSQL) inside the given pod.
func RunStandbyDatabasePrerequisitesSQL(
	podName string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	sql := strings.TrimSpace(dbcommons.StandbyDatabasePrerequisitesSQL)
	if sql == "" {
		return fmt.Errorf("StandbyDatabasePrerequisitesSQL is empty")
	}

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
%s -s / as sysdba <<'EOF'
whenever sqlerror exit 1
%s
exit
EOF
`, dbcommons.SQLPlusCLI, sql)}

	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "RunStandbyDatabasePrerequisitesSQL failed on "+podName+
			" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}

	LogMessages("INFO", "RunStandbyDatabasePrerequisitesSQL succeeded on "+podName, nil, instance, log)
	return nil
}

func RunSQLPlusInPod(
	podName string,
	sql string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return fmt.Errorf("sql is empty")
	}

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
%s -s / as sysdba <<'EOF'
whenever sqlerror exit 1
%s
exit
EOF
`, dbcommons.SQLPlusCLI, sql)}

	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "RunSQLPlusInPod failed on "+podName+
			" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "RunSQLPlusInPod succeeded on "+podName, nil, instance, log)
	return nil
}

func EnableArchiveLogInPod(
	podName string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	// ArchiveLogTrueCMD expects SQLPlusCLI inserted using %s
	cmdStr := fmt.Sprintf(dbcommons.ArchiveLogTrueCMD, dbcommons.SQLPlusCLI)

	cmd := []string{"bash", "-lc", cmdStr}
	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "EnableArchiveLogInPod failed on "+podName+
			" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "EnableArchiveLogInPod succeeded on "+podName, nil, instance, log)
	return nil
}

// ExecShellInPod runs a shell command in the given pod and returns error on failure.
func ExecShellInPod(
	podName string,
	shellCmd string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {
	cmd := []string{"bash", "-lc", shellCmd}
	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "ExecShellInPod failed on "+podName+" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	return nil
}
