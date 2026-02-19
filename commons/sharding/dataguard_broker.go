package commons

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"
)

// BuildDgmgrlConnectIdentifier matches your observed style:
//
//	sshard2-0.sshard2.<ns>.svc.cluster.local:1521/SSHARD2_DGMGRL
func BuildDgmgrlConnectIdentifier(instance *databasev4.ShardingDatabase, shardName string, dbUniqueName string) string {
	svc := strings.ToUpper(strings.TrimSpace(dbUniqueName)) + "_DGMGRL"
	host := fmt.Sprintf("%s-0.%s.%s.svc.cluster.local", shardName, shardName, instance.Namespace)
	return fmt.Sprintf("%s:1521/%s", host, svc)
}

// EnableDgBrokerStart enables dg_broker_start=true inside DB.
func EnableDgBrokerStart(
	podName string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	cmd := []string{"bash", "-lc", `
sqlplus -s / as sysdba <<'EOF'
whenever sqlerror exit 1
alter system set dg_broker_start=true scope=both sid='*';
exit
EOF
`}
	stdout, stderr, err := ExecCommand(podName, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		LogMessages("ERROR", "EnableDgBrokerStart failed on "+podName+" stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "Enabled dg_broker_start on "+podName, nil, instance, log)
	return nil
}

// CreateDgBrokerConfig creates broker config on PRIMARY.
func CreateDgBrokerConfig(
	primaryPod string,
	cfgName string,
	primaryDbUniqueName string,
	primaryConnectIdentifier string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
dgmgrl -silent / <<'EOF'
create configuration %s as primary database is %s connect identifier is "%s";
show configuration;
exit
EOF
`, safeIdent(cfgName), safeIdent(primaryDbUniqueName), strings.TrimSpace(primaryConnectIdentifier))}

	stdout, stderr, err := ExecCommand(primaryPod, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		// idempotency: allow "already exists"
		if looksLikeAlreadyExists(stdout, stderr) {
			LogMessages("INFO", "DG config already exists; continuing. "+cfgName, nil, instance, log)
			return nil
		}
		LogMessages("ERROR", "CreateDgBrokerConfig failed stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "Created/verified DG broker config "+cfgName+" on "+primaryPod, nil, instance, log)
	return nil
}

// AddStandbyToDgBrokerConfig adds standby DB (run on primary).
func AddStandbyToDgBrokerConfig(
	primaryPod string,
	standbyDbUniqueName string,
	standbyConnectIdentifier string,
	instance *databasev4.ShardingDatabase,
	kubeClient kubernetes.Interface,
	kubeConfig clientcmd.ClientConfig,
	log logr.Logger,
) error {

	cmd := []string{"bash", "-lc", fmt.Sprintf(`
dgmgrl -silent / <<'EOF'
add database %s as connect identifier is "%s" maintained as physical;
show configuration;
exit
EOF
`, safeIdent(standbyDbUniqueName), strings.TrimSpace(standbyConnectIdentifier))}

	stdout, stderr, err := ExecCommand(primaryPod, cmd, kubeClient, kubeConfig, instance, log)
	if err != nil {
		// idempotency: allow already present
		if looksLikeAlreadyExists(stdout, stderr) {
			LogMessages("INFO", "Standby already present in DG config; continuing. "+standbyDbUniqueName, nil, instance, log)
			return nil
		}
		LogMessages("ERROR", "AddStandbyToDgBrokerConfig failed stdout="+stdout+" stderr="+stderr, err, instance, log)
		return err
	}
	LogMessages("INFO", "Added/verified standby "+standbyDbUniqueName+" in broker config via "+primaryPod, nil, instance, log)
	return nil
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
