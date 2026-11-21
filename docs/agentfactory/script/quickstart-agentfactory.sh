#!/usr/bin/env bash
#
# Quick prereq helper for AgentFactory with a SIDB-backed database.
#
# Expected flow:
# 1. User creates SIDB.
# 2. This script checks/creates the AgentFactory DB users in the SIDB PDB.
# 3. This script creates the AgentFactory runtime Secret if it does not exist.
# 4. Optionally writes AgentFactory YAML when --output-yaml is provided.

set -euo pipefail

NS="paf"
SIDB_NS=""
SIDB_NAME="sidb-sample"
SIDB_POD=""

AGENT_FACTORY_NAME="agentfactory-sample"
AGENT_FACTORY_IMAGE="container-registry.oracle.com/database/applied-ai:latest"
AGENT_FACTORY_SECRET="agentfactory-runtime-secret"
AGENT_FACTORY_STORAGE_CLASS="oci-bv"
AGENT_FACTORY_PVC_SIZE="100Gi"
AGENT_FACTORY_PVC_ACCESS_MODE="ReadWriteOnce"
OUTPUT_YAML=""

PDB_NAME="prodpdb"
DB_USER="appuser"
DB_PASSWORD=""
DB_HOST=""
DB_PORT="1521"
DB_SERVICE="prodpdb"
SKIP_DB_USER_SETUP="false"

UI_ADMIN_USERNAME="admin@example.com"
UI_ADMIN_PASSWORD=""

usage() {
  cat <<'USAGE'
Usage:
  quickstart-agentfactory.sh [options]

This script is a prereq helper for an existing SIDB and AgentFactory YAML:
  1. Finds SIDB connection details from the SIDB CR.
  2. Checks whether the AgentFactory DB users exist in the target PDB.
  3. Creates the DB users and grants only when the users are missing.
  4. Creates the runtime Secret if it does not already exist.
  5. Writes AgentFactory YAML only when --output-yaml is provided.

Required:
  --db-password PASSWORD           Password for appuser and AAI_RO_appuser
  --ui-admin-password PASSWORD     Agent Factory UI admin password

Common options:
  --namespace NAMESPACE            AgentFactory namespace. Default: paf
  --sidb-name NAME                 SIDB CR resource name. Default: sidb-sample
  --sidb-namespace NAMESPACE       SIDB namespace. Default: --namespace
  --sidb-pod POD_NAME              SIDB pod for sqlplus.
  --pdb-name NAME                  PDB container name. Default: prodpdb
  --db-user USER                   Runtime DB user. Default: appuser
  --agentfactory-secret NAME       Secret name. Default: agentfactory-runtime-secret
  --agentfactory-image IMAGE       Image for rendered YAML
  --agentfactory-name NAME         AgentFactory name. Default: agentfactory-sample
  --ui-admin-username USERNAME     Default: admin@example.com
  --output-yaml FILE               Write AgentFactory YAML to FILE
  --skip-db-user-setup             Skip DB users and only ensure the Secret

Storage options:
  --storage-class NAME             StorageClass for /mount PVC. Default: oci-bv
  --pvc-size SIZE                  PVC size. Default: 100Gi
  --pvc-access-mode MODE           PVC access mode. Default: ReadWriteOnce

Examples:
  # Prepare DB users and Secret only:
  ./quickstart-agentfactory.sh \
    --namespace paf \
    --sidb-namespace <Namespace for the SIDB> \
    --sidb-name sidb-sample \
    --db-user appuser \
    --db-password '<db-password>' \
    --ui-admin-password '<ui-admin-password>'

  # Prepare DB users and Secret, then write AgentFactory YAML:
  ./quickstart-agentfactory.sh \
    --namespace paf \
    --sidb-namespace <Namespace for the SIDB> \
    --sidb-name sidb-sample \
    --storage-class oci-bv \
    --pvc-size 100Gi \
    --db-user appuser \
    --db-password '<db-password>' \
    --ui-admin-password '<ui-admin-password>' \
    --output-yaml agentfactory-sample.yaml
USAGE
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

validate_sql_identifier() {
  local name="$1"
  local value="$2"
  [[ "${value}" =~ ^[A-Za-z][A-Za-z0-9_$#]*$ ]] || die "${name} must be a simple Oracle identifier: ${value}"
}

parse_connect_string() {
  local connect_string="$1"
  local host_port=""

  [[ -n "${connect_string}" ]] || die "SIDB status.clusterConnectString is empty"
  [[ "${connect_string}" == */* ]] || die "cannot parse SIDB connect string: ${connect_string}"

  host_port="${connect_string%%/*}"
  DB_HOST="${host_port%:*}"
  DB_PORT="${host_port##*:}"
  DB_SERVICE="${connect_string#*/}"

  [[ -n "${DB_HOST}" && -n "${DB_PORT}" && -n "${DB_SERVICE}" ]] || die "cannot parse SIDB connect string: ${connect_string}"
}

ensure_runtime_secret() {
  if kubectl -n "${NS}" get secret "${AGENT_FACTORY_SECRET}" >/dev/null 2>&1; then
    echo "Secret ${NS}/${AGENT_FACTORY_SECRET} already exists; leaving it unchanged"
    return
  fi

  echo "Creating Secret ${NS}/${AGENT_FACTORY_SECRET}"
  kubectl -n "${NS}" create secret generic "${AGENT_FACTORY_SECRET}" \
    --from-literal=paf_ui_admin_password="${UI_ADMIN_PASSWORD}" \
    --from-literal=paf_db_password="${DB_PASSWORD}"
}

ensure_database_users() {
  if [[ "${SKIP_DB_USER_SETUP}" == "true" ]]; then
    echo "Skipping database user setup"
    return
  fi

  echo "Checking AgentFactory database users in PDB ${PDB_NAME}"
  kubectl -n "${SIDB_NS}" exec -i "${SIDB_POD}" -- sqlplus / as sysdba <<SQL
whenever sqlerror exit sql.sqlcode
whenever oserror exit 2
set echo on
set serveroutput on size unlimited
set feedback on
set heading on
set pages 100

prompt Current container before switch:
show con_name

alter session set container=${PDB_NAME};

prompt Current container after switch:
show con_name

declare
  l_count number := 0;
begin
  dbms_output.put_line('Checking user ${DB_USER}');
  select count(*)
    into l_count
    from dba_users
   where username = upper('${DB_USER}');

  if l_count = 0 then
    dbms_output.put_line('Creating user ${DB_USER}');
    execute immediate 'CREATE USER ${DB_USER} IDENTIFIED BY "${DB_PASSWORD}" DEFAULT TABLESPACE USERS QUOTA unlimited ON USERS';
    execute immediate 'grant create session, create table, create sequence, create trigger, create type, create procedure, create view, create synonym to ${DB_USER}';
    execute immediate 'grant read, write on directory DATA_PUMP_DIR to ${DB_USER}';
    execute immediate 'grant select on V_\$PARAMETER to ${DB_USER}';
  else
    dbms_output.put_line('User ${DB_USER} already exists; skipping schema user changes');
  end if;
end;
/

declare
  l_count number := 0;
begin
  dbms_output.put_line('Checking user AAI_RO_${DB_USER}');
  select count(*)
    into l_count
    from dba_users
   where username = upper('AAI_RO_${DB_USER}');
  if l_count = 0 then
    dbms_output.put_line('Creating user AAI_RO_${DB_USER}');
    execute immediate 'CREATE USER AAI_RO_${DB_USER} IDENTIFIED BY "${DB_PASSWORD}" ACCOUNT UNLOCK';
    execute immediate 'grant create session to AAI_RO_${DB_USER}';
  else
    dbms_output.put_line('User AAI_RO_${DB_USER} already exists; skipping read-only user changes');
  end if;
end;
/

select username
  from dba_users
 where username in (upper('${DB_USER}'), upper('AAI_RO_${DB_USER}'))
 order by username;

exit
SQL
}

render_agentfactory_yaml() {
  cat <<YAML
apiVersion: privateai.oracle.com/v4
kind: AgentFactory
metadata:
  name: ${AGENT_FACTORY_NAME}
  namespace: ${NS}
spec:
  runtime:
    image:
      name: ${AGENT_FACTORY_IMAGE}
    replicas: 1
    resources:
      requests:
        cpu: "2"
        memory: 8Gi
      limits:
        cpu: "4"
        memory: 16Gi
  configuration:
    uiAdminUsername: ${UI_ADMIN_USERNAME}
  database:
    connectionType: Details
    username: ${DB_USER}
    details:
      protocol: TCP
      host: ${DB_HOST}
      port: ${DB_PORT}
      serviceName: ${PDB_NAME}
    airGapped: false
  security:
    secret:
      name: ${AGENT_FACTORY_SECRET}
      uiAdminPasswordKey: paf_ui_admin_password
      dbPasswordKey: paf_db_password
  storage:
    volumes:
      - name: app-mount
        mountPath: /mount
        source:
          pvc:
            storageClass: ${AGENT_FACTORY_STORAGE_CLASS}
            size: ${AGENT_FACTORY_PVC_SIZE}
            accessModes:
              - ${AGENT_FACTORY_PVC_ACCESS_MODE}
            deletePvcOnDelete: false
YAML
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace) NS="$2"; shift 2 ;;
    --sidb-namespace) SIDB_NS="$2"; shift 2 ;;
    --sidb-name) SIDB_NAME="$2"; shift 2 ;;
    --sidb-pod) SIDB_POD="$2"; shift 2 ;;
    --agentfactory-name) AGENT_FACTORY_NAME="$2"; shift 2 ;;
    --agentfactory-image) AGENT_FACTORY_IMAGE="$2"; shift 2 ;;
    --agentfactory-secret) AGENT_FACTORY_SECRET="$2"; shift 2 ;;
    --storage-class) AGENT_FACTORY_STORAGE_CLASS="$2"; shift 2 ;;
    --pvc-size) AGENT_FACTORY_PVC_SIZE="$2"; shift 2 ;;
    --pvc-access-mode) AGENT_FACTORY_PVC_ACCESS_MODE="$2"; shift 2 ;;
    --pdb-name) PDB_NAME="$2"; shift 2 ;;
    --db-user) DB_USER="$2"; shift 2 ;;
    --db-password) DB_PASSWORD="$2"; shift 2 ;;
    --ui-admin-username) UI_ADMIN_USERNAME="$2"; shift 2 ;;
    --ui-admin-password) UI_ADMIN_PASSWORD="$2"; shift 2 ;;
    --output-yaml) OUTPUT_YAML="$2"; shift 2 ;;
    --skip-db-user-setup) SKIP_DB_USER_SETUP="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

require_cmd kubectl

[[ -n "${DB_PASSWORD}" ]] || die "--db-password is required"
[[ -n "${UI_ADMIN_PASSWORD}" ]] || die "--ui-admin-password is required"
#[[ -n "${SIDB_POD}" ]] || die "--sidb-pod is required"

if [[ "${DB_PASSWORD}" == *\"* || "${UI_ADMIN_PASSWORD}" == *\"* ]]; then
  die "password values must not contain double quote characters for this quick start script"
fi

validate_sql_identifier "--pdb-name" "${PDB_NAME}"
validate_sql_identifier "--db-user" "${DB_USER}"

if [[ -z "${SIDB_NS}" ]]; then
  SIDB_NS="${NS}"
fi

if [[ -z "${SIDB_POD}" ]]; then
  SIDB_POD=$(kubectl -n $SIDB_NS get pod -l app=$SIDB_NAME -o jsonpath='{.items[0].metadata.name}')
fi

kubectl get namespace "${NS}" >/dev/null 2>&1 || kubectl create namespace "${NS}"

kubectl -n "${SIDB_NS}" get singleinstancedatabase "${SIDB_NAME}" >/dev/null 2>&1 ||
  die "SIDB ${SIDB_NS}/${SIDB_NAME} was not found; create SIDB first or pass the correct --sidb-name/--sidb-namespace"

echo "Waiting for SIDB ${SIDB_NS}/${SIDB_NAME} to be Ready"
kubectl -n "${SIDB_NS}" wait singleinstancedatabase "${SIDB_NAME}" \
  --for=jsonpath='{.status.status}'=Healthy \
  --timeout=60m

CONNECT_STRING="$(kubectl -n "${SIDB_NS}" get singleinstancedatabase "${SIDB_NAME}" \
  -o jsonpath='{.status.clusterConnectString}')"
parse_connect_string "${CONNECT_STRING}"

echo "Using database host=${DB_HOST} port=${DB_PORT} service=${PDB_NAME}"

ensure_database_users
ensure_runtime_secret

if [[ -n "${OUTPUT_YAML}" ]]; then
  render_agentfactory_yaml > "${OUTPUT_YAML}"
  echo "Wrote AgentFactory YAML to ${OUTPUT_YAML}"
else
  echo "Prerequisites are ready. Pass --output-yaml <file> to write the AgentFactory YAML."
fi
