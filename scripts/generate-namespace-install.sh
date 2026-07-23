#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Generate namespace-scoped Oracle Database Operator install manifests.

Usage:
  scripts/generate-namespace-install.sh <watch-namespace[,watch-namespace...]>
  scripts/generate-namespace-install.sh --watch-namespaces <list> [--operator-namespace <namespace>] [--output-dir <dir>]

Examples:
  scripts/generate-namespace-install.sh oracle-database-operator-system,shns
  scripts/generate-namespace-install.sh --watch-namespaces default,pdbnamespace,cdbnamespace --output-dir dist/install

The script writes:
  dist/install/oracle-database-operator-rbac.yaml
  dist/install/oracle-database-operator-system.yaml

Note:
  This script generates namespace-scoped watch access only. It does not grant
  cluster-scoped permissions for resources such as PersistentVolumes,
  StorageClasses, or Nodes. Apply the optional RBAC manifests for those
  resources only when the selected controller feature requires them.
EOF
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
watch_namespaces=""
operator_namespace="oracle-database-operator-system"
output_dir="${repo_root}/dist/install"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --watch-namespaces)
      [[ $# -ge 2 ]] || { echo "ERROR: --watch-namespaces requires a value" >&2; exit 1; }
      watch_namespaces="$2"
      shift 2
      ;;
    --operator-namespace)
      [[ $# -ge 2 ]] || { echo "ERROR: --operator-namespace requires a value" >&2; exit 1; }
      operator_namespace="$2"
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 ]] || { echo "ERROR: --output-dir requires a value" >&2; exit 1; }
      output_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "ERROR: Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -n "${watch_namespaces}" ]]; then
        echo "ERROR: Watch namespaces were provided more than once" >&2
        usage >&2
        exit 1
      fi
      watch_namespaces="$1"
      shift
      ;;
  esac
done

if [[ -z "${watch_namespaces}" ]]; then
  echo "ERROR: Provide at least one watch namespace" >&2
  usage >&2
  exit 1
fi

validate_namespace() {
  local namespace="$1"
  if [[ ${#namespace} -gt 63 || ! "${namespace}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    echo "ERROR: Invalid Kubernetes namespace: ${namespace}" >&2
    exit 1
  fi
}

validate_namespace "${operator_namespace}"

watch_namespaces="${watch_namespaces//[[:space:]]/}"
IFS=',' read -r -a requested_namespaces <<< "${watch_namespaces}"

declare -A seen_namespaces=()
namespaces=()
for namespace in "${requested_namespaces[@]}"; do
  if [[ -z "${namespace}" ]]; then
    echo "ERROR: Watch namespace list contains an empty value" >&2
    exit 1
  fi
  validate_namespace "${namespace}"
  if [[ -z "${seen_namespaces[${namespace}]+x}" ]]; then
    namespaces+=("${namespace}")
    seen_namespaces["${namespace}"]=1
  fi
done

watch_csv="$(IFS=','; echo "${namespaces[*]}")"

source_system="${repo_root}/oracle-database-operator-system.yaml"
source_rbac="${repo_root}/oracle-database-operator-rbac.yaml"

[[ -f "${source_system}" ]] || { echo "ERROR: Missing ${source_system}" >&2; exit 1; }
[[ -f "${source_rbac}" ]] || { echo "ERROR: Missing ${source_rbac}" >&2; exit 1; }

mkdir -p "${output_dir}"

system_out="${output_dir}/oracle-database-operator-system.yaml"
rbac_out="${output_dir}/oracle-database-operator-rbac.yaml"

awk -v watch="${watch_csv}" '
  function starts_with(value, prefix) {
    return substr(value, 1, length(prefix)) == prefix
  }
  {
    if ($0 ~ /^[[:space:]]*- name: WATCH_NAMESPACE[[:space:]]*$/) {
      print
      match($0, /^[[:space:]]*/)
      name_indent = substr($0, RSTART, RLENGTH)
      value_indent = name_indent "  "
      print value_indent "value: \"" watch "\""
      skipping = 1
      mode = ""
      next
    }

    if (skipping) {
      if (mode == "" && $0 ~ "^" value_indent "valueFrom:[[:space:]]*$") {
        mode = "valueFrom"
        next
      }
      if (mode == "" && $0 ~ "^" value_indent "value:") {
        skipping = 0
        next
      }
      if (mode == "valueFrom" && starts_with($0, value_indent "  ")) {
        next
      }
      skipping = 0
    }

    print
  }
' "${source_system}" > "${system_out}"

if ! grep -Fq "value: \"${watch_csv}\"" "${system_out}"; then
  echo "ERROR: Failed to set WATCH_NAMESPACE in ${system_out}" >&2
  exit 1
fi

cp "${source_rbac}" "${rbac_out}"

for namespace in "${namespaces[@]}"; do
  if [[ "${namespace}" == "${operator_namespace}" ]]; then
    continue
  fi

  cat >> "${rbac_out}" <<EOF
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: oracle-database-operator-manager-rolebinding
  namespace: ${namespace}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: oracle-database-operator-manager-role
subjects:
- kind: ServiceAccount
  name: oracle-database-operator-controller-manager
  namespace: ${operator_namespace}
EOF
done

cat <<EOF
Generated namespace-scoped install manifests:
  ${rbac_out}
  ${system_out}

WATCH_NAMESPACE=${watch_csv}
Operator namespace=${operator_namespace}

Apply with:
  kubectl apply -f ${rbac_out}
  kubectl apply -f ${system_out}

Note:
  Cluster-scoped resources such as PersistentVolumes, StorageClasses, and Nodes
  are not namespace-scoped by WATCH_NAMESPACE. Apply optional RBAC manifests
  such as rbac/persistent-volume-rbac.yaml, rbac/storage-class-rbac.yaml,
  rbac/node-rbac.yaml, or docs/rac/rbac/pv-rbac.yaml only when required.
EOF
