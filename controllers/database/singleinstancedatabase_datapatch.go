package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	dbapi "github.com/oracle/oracle-database-operator/apis/database/v4"
	dbcommons "github.com/oracle/oracle-database-operator/commons/database"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const sidbOracleHomePatchInventoryCommand = `set -o pipefail
"$ORACLE_HOME/OPatch/opatch" lspatches |
  awk -F ';' '/^[[:space:]]*[0-9]+;/ { gsub(/^[[:space:]]+|[[:space:]]+$/, "", $1); print $1 }' |
  LC_ALL=C sort -u`

const sidbSQLPatchFingerprintSQL = `whenever oserror exit failure
whenever sqlerror exit failure
set heading off feedback off pages 0 verify off echo off
select patch_id || ':' || patch_uid || ':' || action || ':' || status || ':' || target_version
from (
  select patch_id, patch_uid, target_version, action, status,
         row_number() over (partition by patch_id, patch_uid order by action_time desc) as latest_action
  from dba_registry_sqlpatch
)
where latest_action = 1
order by patch_id, patch_uid;
exit success`

func (r *SingleInstanceDatabaseReconciler) getSIDBOracleHomePatchFingerprint(readyPod corev1.Pod,
	ctx context.Context, req ctrl.Request) (string, error) {
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", sidbOracleHomePatchInventoryCommand)
	if err != nil {
		return "", fmt.Errorf("read Oracle Home patch inventory: %w", err)
	}
	return sidbPatchFingerprint(out), nil
}

func (r *SingleInstanceDatabaseReconciler) getSIDBSQLPatchFingerprint(readyPod corev1.Pod,
	ctx context.Context, req ctrl.Request) (string, error) {
	command := fmt.Sprintf("echo -e \"%s\" | sqlplus -s / as sysdba", sidbSQLPatchFingerprintSQL)
	out, err := dbcommons.ExecCommand(r, r.Config, readyPod.Name, readyPod.Namespace, "",
		ctx, req, false, "bash", "-c", command)
	if err != nil {
		return "", fmt.Errorf("read database SQL patch inventory: %w", err)
	}
	return sidbPatchFingerprint(out), nil
}

func sidbPatchFingerprint(output string) string {
	values := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			values[value] = struct{}{}
		}
	}
	patches := make([]string, 0, len(values))
	for value := range values {
		patches = append(patches, value)
	}
	sort.Strings(patches)
	sum := sha256.Sum256([]byte(strings.Join(patches, "\n")))
	return hex.EncodeToString(sum[:])
}

func sidbDatapatchIsCurrent(previous *dbapi.SingleInstanceDatabaseDatapatchStatus,
	oracleHomePatchFingerprint, sqlPatchFingerprint string) bool {
	return previous != nil &&
		previous.OracleHomePatchFingerprint != "" &&
		previous.SQLPatchFingerprint != "" &&
		previous.OracleHomePatchFingerprint == oracleHomePatchFingerprint &&
		previous.SQLPatchFingerprint == sqlPatchFingerprint
}

func recordSIDBDatapatchStatus(sidb *dbapi.SingleInstanceDatabase, readyPod corev1.Pod,
	oracleHomePatchFingerprint, sqlPatchFingerprint string) {
	sidb.Status.Datapatch = &dbapi.SingleInstanceDatabaseDatapatchStatus{
		Image:                      sidb.Spec.Image.PullFrom,
		ImageID:                    sidbPodImageID(readyPod, sidb.Name),
		OracleHomePatchFingerprint: oracleHomePatchFingerprint,
		SQLPatchFingerprint:        sqlPatchFingerprint,
	}
}

func sidbPodImageID(pod corev1.Pod, containerName string) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName && status.ImageID != "" {
			return status.ImageID
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.ImageID != "" {
			return status.ImageID
		}
	}
	return ""
}
