#!/usr/bin/env bash

# Shared configuration and helpers for the numbered SIDB E2E cases.
# This file is sourced by the common lifecycle scripts and case wrappers.

e2e_workload_health_file="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}/.gitlab/ci/e2e-workload-health.sh"
# shellcheck disable=SC1090
source "$e2e_workload_health_file"
unset e2e_workload_health_file

sidb_case_label() {
  printf 'SIDB %s' "${SIDB_CASE_ID:-test01}"
}

sidb_case_init_config() {
  : "${SIDB_CASE_ID:=test01}"
  : "${SIDB_CASE_NAME:=01-provision}"
  : "${SIDB_CASE_NAMESPACE:=${SIDB_E2E_NAMESPACE:-${E2E_NAMESPACE:-sidb-ns}}}"
  : "${SIDB_TEST_NAME:=sidb-e2e-primary}"
  : "${SIDB_TEST_SID:=ORCL1}"
  : "${SIDB_TEST_PDB:=ORCLPDB1}"
  : "${SIDB_EDITION:=enterprise}"
  : "${SIDB_CHARSET:=AL32UTF8}"
  : "${SIDB_STORAGE_SIZE:=100Gi}"
  : "${SIDB_ACCESS_MODE:=ReadWriteOnce}"
  : "${SIDB_IMAGE_PULL_POLICY:=Always}"
  : "${SIDB_IMAGE:?Set SIDB_IMAGE as an environment variable}"
  : "${SIDB_STORAGE_CLASS:=oci-bv}"
  : "${SIDB_ADMIN_SECRET:=sidb-e2e-admin}"
  : "${SIDB_ADMIN_SECRET_KEY:=oracle_pwd}"
  : "${SIDB_TDE_SECRET:=}"
  : "${SIDB_TDE_SECRET_KEY:=tde_wallet_pwd}"
  : "${SIDB_TCPS_TLS_SECRET:=}"
  : "${SIDB_REPLICAS:=1}"
  : "${SIDB_TIMEOUT_SECONDS:=1500}"
  : "${SIDB_UNHEALTHY_GRACE_SECONDS:=600}"
  : "${SIDB_POLL_INTERVAL:=20}"
  : "${SIDB_CLEANUP_TIMEOUT_SECONDS:=300}"
  : "${SIDB_CASE_CLEANUP:=true}"
  : "${SIDB_CASE_NAMESPACE_CREATED:=false}"
  : "${SIDB_CASE_PIPELINE:=${CI_PIPELINE_ID:-local}}"
  : "${SIDB_CASE_RESULTS_DIR:=${OPERATOR_LOG_DIR:-${SIDB_E2E_RESULTS_DIR:-test/e2e/controllers/sidb/results}}/sidb/${SIDB_CASE_NAME}-${SIDB_CASE_NAMESPACE}}"
  : "${SIDB_CASE_MANIFEST:=${SIDB_CASE_DIR:-$(dirname "${BASH_SOURCE[0]}")}/test01.yaml}"
  : "${SIDB_CASE_ENV_FILE:=${TMPDIR:-/tmp}/sidb-ns-${SIDB_CASE_ID}.env}"
  : "${SIDB_RENDERED_MANIFEST:=${SIDB_CASE_RESULTS_DIR}/${SIDB_CASE_ID}-rendered.yaml}"

  export SIDB_CASE_ID SIDB_CASE_NAME SIDB_CASE_NAMESPACE SIDB_TEST_NAME
  export SIDB_TEST_SID SIDB_TEST_PDB SIDB_EDITION SIDB_CHARSET SIDB_STORAGE_SIZE
  export SIDB_ACCESS_MODE SIDB_IMAGE_PULL_POLICY SIDB_ADMIN_SECRET SIDB_ADMIN_SECRET_KEY
  export SIDB_TDE_SECRET SIDB_TDE_SECRET_KEY SIDB_TCPS_TLS_SECRET
  export SIDB_REPLICAS SIDB_TIMEOUT_SECONDS SIDB_UNHEALTHY_GRACE_SECONDS SIDB_POLL_INTERVAL SIDB_CLEANUP_TIMEOUT_SECONDS
  export SIDB_CASE_CLEANUP SIDB_CASE_NAMESPACE_CREATED SIDB_CASE_PIPELINE SIDB_CASE_RESULTS_DIR
  export SIDB_CASE_MANIFEST SIDB_CASE_ENV_FILE SIDB_RENDERED_MANIFEST
}

sidb_case_load_env() {
  local env_file="${SIDB_CASE_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-${SIDB_CASE_ID:-test01}.env}"
  if [ -f "$env_file" ]; then
    # shellcheck disable=SC1090
    source "$env_file"
  fi
}

sidb_case_kubectl() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}


# Only newly created credentials belong to automatic E2E pre-cleanup. Existing
# caller-provided Secrets must not acquire cleanup ownership merely by reuse.
sidb_case_label_new_secret_manifest() {
  local secret_name="$1" existing labels
  sidb_case_require_command jq || return
  existing="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$secret_name" \
    --ignore-not-found -o json </dev/null)" || return
  if [ -n "$existing" ]; then
    labels="$(jq -c '.metadata.labels // {}' <<< "$existing")" || return
    # Preserve earlier ownership on an already-owned Secret without adopting
    # an external Secret or removing its labels through three-way apply.
    jq --argjson labels "$labels" '
      if $labels == {} then .
      else .metadata.labels = ($labels + (.metadata.labels // {})) end'
    return
  fi
  jq --arg test_id "$SIDB_CASE_ID" --arg pipeline "$SIDB_CASE_PIPELINE" '
    .metadata.labels = ((.metadata.labels // {}) + {
      "e2e.oracle.com/owner": "operator-e2e",
      "e2e.oracle.com/controller": "sidb",
      "e2e.oracle.com/test-id": $test_id,
      "e2e.oracle.com/pipeline": $pipeline
    })'
}

# Build comma-separated subjectAltName DNS entries for one hostname and its
# in-cluster Service DNS forms.
sidb_case_tcps_san_for_name() {
  local name="$1"
  printf 'DNS:%s,DNS:%s.%s,DNS:%s.%s.svc,DNS:%s.%s.svc.cluster.local' \
    "$name" "$name" "$SIDB_CASE_NAMESPACE" "$name" "$SIDB_CASE_NAMESPACE" \
    "$name" "$SIDB_CASE_NAMESPACE"
}

sidb_case_create_tcps_tls_secret() {
  local temporary_dir certificate_file key_file san extra_name

  [ -n "${SIDB_TCPS_TLS_SECRET:-}" ] || return 0
  mkdir -p "$SIDB_CASE_RESULTS_DIR/secret-manifests"
  if sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_TCPS_TLS_SECRET" >/dev/null 2>&1; then
    sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_TCPS_TLS_SECRET" -o yaml \
      > "$SIDB_CASE_RESULTS_DIR/secret-manifests/tcps-tls-secret-existing.yaml"
    return 0
  fi

  sidb_case_require_command openssl
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/sidb-tcps.XXXXXX")"
  certificate_file="$temporary_dir/tls.crt"
  key_file="$temporary_dir/tls.key"
  # Primary Service DNS always. Optional SIDB_TCPS_TLS_EXTRA_DNS_NAMES (comma-
  # separated short names) add more SANs so one shared TLS Secret can cover
  # Data Guard primary+standby without regenerating the cert after primary TCPS.
  san="$(sidb_case_tcps_san_for_name "$SIDB_TEST_NAME")"
  if [ -n "${SIDB_TCPS_TLS_EXTRA_DNS_NAMES:-}" ]; then
    local old_ifs="$IFS"
    IFS=','
    # shellcheck disable=SC2086
    set -- ${SIDB_TCPS_TLS_EXTRA_DNS_NAMES}
    IFS="$old_ifs"
    for extra_name in "$@"; do
      extra_name="${extra_name#"${extra_name%%[![:space:]]*}"}"
      extra_name="${extra_name%"${extra_name##*[![:space:]]}"}"
      [ -n "$extra_name" ] || continue
      [ "$extra_name" = "$SIDB_TEST_NAME" ] && continue
      san="${san},$(sidb_case_tcps_san_for_name "$extra_name")"
    done
  fi

  if ! openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -subj "/CN=${SIDB_TEST_NAME}" \
    -addext "subjectAltName=${san}" \
    -keyout "$key_file" -out "$certificate_file" >/dev/null 2>&1; then
    rm -rf -- "$temporary_dir"
    echo "$(sidb_case_label): failed to generate the TCPS test certificate" >&2
    return 1
  fi

  if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" create secret tls "$SIDB_TCPS_TLS_SECRET" \
    --cert="$certificate_file" --key="$key_file" \
    --dry-run=client -o json |
    sidb_case_label_new_secret_manifest "$SIDB_TCPS_TLS_SECRET" |
    tee "$SIDB_CASE_RESULTS_DIR/secret-manifests/tcps-tls-secret.yaml" |
    sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f - >/dev/null; then
    rm -rf -- "$temporary_dir"
    echo "$(sidb_case_label): failed to create TCPS TLS Secret $SIDB_TCPS_TLS_SECRET" >&2
    return 1
  fi
  echo "$(sidb_case_label): created TCPS TLS Secret $SIDB_TCPS_TLS_SECRET with SAN=${san}"
  rm -rf -- "$temporary_dir"
}

sidb_case_require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "$(sidb_case_label): required command is unavailable: $1" >&2
    return 2
  }
}

sidb_case_validate_scalar() {
  local name="$1" value="$2"
  case "$value" in
    *$'\n'*|*$'\r'*|*'"'*)
      echo "$(sidb_case_label): $name contains an unsupported newline or quote" >&2
      return 2
      ;;
  esac
}

sidb_case_validate_config() {
  sidb_case_require_command kubectl
  [ -n "${SIDB_IMAGE:-}" ] || {
    echo "$(sidb_case_label): SIDB_IMAGE is required" >&2
    return 2
  }
  [ -n "${SIDB_STORAGE_CLASS:-}" ] || {
    echo "$(sidb_case_label): SIDB_STORAGE_CLASS is required" >&2
    return 2
  }

  if ! [[ "$SIDB_CASE_NAMESPACE" =~ ^[a-z0-9][a-z0-9.-]*$ ]]; then
    echo "$(sidb_case_label): invalid Kubernetes namespace: $SIDB_CASE_NAMESPACE" >&2
    return 2
  fi
  if ! [[ "$SIDB_TEST_NAME" =~ ^[a-z0-9][a-z0-9.-]*$ ]]; then
    echo "$(sidb_case_label): invalid Kubernetes resource name: $SIDB_TEST_NAME" >&2
    return 2
  fi
  if ! [[ "$SIDB_TEST_SID" =~ ^[a-zA-Z0-9]+$ ]]; then
    echo "$(sidb_case_label): SIDB_TEST_SID must contain only letters and digits: $SIDB_TEST_SID" >&2
    return 2
  fi
  [ "${#SIDB_TEST_SID}" -le 12 ] || {
    echo "$(sidb_case_label): SIDB_TEST_SID is longer than 12 characters" >&2
    return 2
  }

  sidb_case_validate_scalar SIDB_TEST_PDB "$SIDB_TEST_PDB"
  sidb_case_validate_scalar SIDB_IMAGE "$SIDB_IMAGE"
  sidb_case_validate_scalar SIDB_EDITION "$SIDB_EDITION"
  sidb_case_validate_scalar SIDB_CHARSET "$SIDB_CHARSET"
  sidb_case_validate_scalar SIDB_STORAGE_SIZE "$SIDB_STORAGE_SIZE"
  sidb_case_validate_scalar SIDB_ACCESS_MODE "$SIDB_ACCESS_MODE"
  sidb_case_validate_scalar SIDB_IMAGE_PULL_POLICY "$SIDB_IMAGE_PULL_POLICY"
  case "$SIDB_IMAGE_PULL_POLICY" in
    Always|IfNotPresent|Never) ;;
    *)
      echo "$(sidb_case_label): SIDB_IMAGE_PULL_POLICY must be Always, IfNotPresent, or Never" >&2
      return 2
      ;;
  esac
  sidb_case_validate_scalar SIDB_ADMIN_SECRET "$SIDB_ADMIN_SECRET"
  sidb_case_validate_scalar SIDB_ADMIN_SECRET_KEY "$SIDB_ADMIN_SECRET_KEY"
  sidb_case_validate_scalar SIDB_TDE_SECRET "$SIDB_TDE_SECRET"
  sidb_case_validate_scalar SIDB_TDE_SECRET_KEY "$SIDB_TDE_SECRET_KEY"
  sidb_case_validate_scalar SIDB_TCPS_TLS_SECRET "$SIDB_TCPS_TLS_SECRET"
  if [ -n "${SIDB_TDE_SECRET:-}" ]; then
    [ -n "${SIDB_TDE_PASSWORD:-}" ] || {
      echo "$(sidb_case_label): SIDB_TDE_PASSWORD is required when SIDB_TDE_SECRET is set" >&2
      return 2
    }
    sidb_case_validate_scalar SIDB_TDE_PASSWORD "$SIDB_TDE_PASSWORD"
  fi
  sidb_case_validate_scalar SIDB_CASE_PIPELINE "$SIDB_CASE_PIPELINE"
  sidb_case_validate_scalar SIDB_STORAGE_CLASS "$SIDB_STORAGE_CLASS"
  sidb_case_validate_scalar SIDB_PULL_SECRET "${SIDB_PULL_SECRET:-}"
  sidb_case_validate_scalar SIDB_ADMIN_PASSWORD "${SIDB_ADMIN_PASSWORD:-}"
}

sidb_case_write_env() {
  local env_file="${SIDB_CASE_ENV_FILE}"
  mkdir -p "$(dirname "$env_file")"
  umask 077
  {
    printf 'export SIDB_CASE_ID=%q\n' "$SIDB_CASE_ID"
    printf 'export SIDB_CASE_NAME=%q\n' "$SIDB_CASE_NAME"
    printf 'export SIDB_CASE_NAMESPACE=%q\n' "$SIDB_CASE_NAMESPACE"
    printf 'export SIDB_TEST_NAME=%q\n' "$SIDB_TEST_NAME"
    printf 'export SIDB_TEST_SID=%q\n' "$SIDB_TEST_SID"
    printf 'export SIDB_TEST_PDB=%q\n' "$SIDB_TEST_PDB"
    printf 'export SIDB_EDITION=%q\n' "$SIDB_EDITION"
    printf 'export SIDB_CHARSET=%q\n' "$SIDB_CHARSET"
    printf 'export SIDB_STORAGE_SIZE=%q\n' "$SIDB_STORAGE_SIZE"
    printf 'export SIDB_ACCESS_MODE=%q\n' "$SIDB_ACCESS_MODE"
    printf 'export SIDB_IMAGE=%q\n' "$SIDB_IMAGE"
    printf 'export SIDB_STORAGE_CLASS=%q\n' "$SIDB_STORAGE_CLASS"
    printf 'export SIDB_PULL_SECRET=%q\n' "${SIDB_PULL_SECRET:-}"
    printf 'export SIDB_IMAGE_PULL_POLICY=%q\n' "$SIDB_IMAGE_PULL_POLICY"
    printf 'export SIDB_ADMIN_SECRET=%q\n' "$SIDB_ADMIN_SECRET"
    printf 'export SIDB_ADMIN_SECRET_KEY=%q\n' "$SIDB_ADMIN_SECRET_KEY"
    printf 'export SIDB_ADMIN_PASSWORD=%q\n' "$SIDB_ADMIN_PASSWORD"
    printf 'export SIDB_TDE_SECRET=%q\n' "$SIDB_TDE_SECRET"
    printf 'export SIDB_TDE_SECRET_KEY=%q\n' "$SIDB_TDE_SECRET_KEY"
    printf 'export SIDB_TCPS_TLS_SECRET=%q\n' "$SIDB_TCPS_TLS_SECRET"
    printf 'export SIDB_REPLICAS=%q\n' "$SIDB_REPLICAS"
    printf 'export SIDB_TIMEOUT_SECONDS=%q\n' "$SIDB_TIMEOUT_SECONDS"
    printf 'export SIDB_UNHEALTHY_GRACE_SECONDS=%q\n' "$SIDB_UNHEALTHY_GRACE_SECONDS"
    printf 'export SIDB_POLL_INTERVAL=%q\n' "$SIDB_POLL_INTERVAL"
    printf 'export SIDB_CLEANUP_TIMEOUT_SECONDS=%q\n' "$SIDB_CLEANUP_TIMEOUT_SECONDS"
    printf 'export SIDB_CASE_CLEANUP=%q\n' "$SIDB_CASE_CLEANUP"
    printf 'export SIDB_CASE_NAMESPACE_CREATED=%q\n' "$SIDB_CASE_NAMESPACE_CREATED"
    printf 'export SIDB_CASE_PIPELINE=%q\n' "$SIDB_CASE_PIPELINE"
    printf 'export SIDB_CASE_RESULTS_DIR=%q\n' "$SIDB_CASE_RESULTS_DIR"
    printf 'export SIDB_CASE_MANIFEST=%q\n' "$SIDB_CASE_MANIFEST"
    printf 'export SIDB_CASE_ENV_FILE=%q\n' "$SIDB_CASE_ENV_FILE"
    printf 'export SIDB_RENDERED_MANIFEST=%q\n' "$SIDB_RENDERED_MANIFEST"
    printf 'export KUBE_CONTEXT_NAME=%q\n' "${KUBE_CONTEXT_NAME:-}"
    printf 'export KUBECONFIG=%q\n' "${KUBECONFIG:-}"
  } > "$env_file"
  chmod 600 "$env_file"
}


# Omit standby TDE secretKey for Oracle Database 19c images.
# 19c standby needs walletZipFileKey only; secretKey is for 23ai+.
# Override with SIDB_OMIT_STANDBY_TDE_SECRET_KEY=true|false.
sidb_case_image_is_19c() {
  local img="${SIDB_IMAGE:-}"
  local ver="${SIDB_IMAGE_VERSION:-}"
  local tag="${img##*:}"
  if [ -n "$ver" ]; then
    case "$ver" in
      19|19.*|19c|19C) return 0 ;;
    esac
  fi
  case "$tag" in
    19.*|19-*|19) return 0 ;;
  esac
  # OCR validation tag family for 19c SIDB images
  case "$tag" in
    128786|128786-*) return 0 ;;
  esac
  return 1
}

sidb_case_omit_standby_tde_secret_key() {
  case "${SIDB_OMIT_STANDBY_TDE_SECRET_KEY:-}" in
    true|TRUE|yes|YES|1) return 0 ;;
    false|FALSE|no|NO|0) return 1 ;;
  esac
  sidb_case_image_is_19c
}

sidb_case_render_manifest() {
  local source_file="$1" destination_file="$2" line
  mkdir -p "$(dirname "$destination_file")"
  : > "$destination_file"
  while IFS= read -r line || [ -n "$line" ]; do
    if [ -z "${SIDB_PULL_SECRET:-}" ] && [ "$line" = '    pullSecrets: "__SIDB_PULL_SECRET__"' ]; then
      continue
    fi
    # Standby-only: drop TDE password key for 19c (keep walletZipFileKey).
    if [ "${SIDB_RENDER_OMIT_TDE_SECRET_KEY:-}" = "true" ] && [[ "$line" == *'secretKey: "__SIDB_TDE_SECRET_KEY__"'* ]]; then
      echo "sidb_case_render_manifest: omitting standby tde.secretKey for 19c image" >&2
      continue
    fi
    line="${line//__SIDB_CASE_NAMESPACE__/$SIDB_CASE_NAMESPACE}"
    line="${line//__SIDB_TEST_NAME__/$SIDB_TEST_NAME}"
    line="${line//__SIDB_PRIMARY_NAME__/${SIDB_DG_PRIMARY_NAME:-$SIDB_TEST_NAME}}"
    line="${line//__SIDB_TEST_SID__/$SIDB_TEST_SID}"
    line="${line//__SIDB_TEST_PDB__/$SIDB_TEST_PDB}"
    line="${line//__SIDB_IMAGE__/$SIDB_IMAGE}"
    line="${line//__SIDB_PULL_SECRET__/${SIDB_PULL_SECRET:-}}"
    line="${line//__SIDB_IMAGE_PULL_POLICY__/$SIDB_IMAGE_PULL_POLICY}"
    line="${line//__SIDB_ADMIN_SECRET__/$SIDB_ADMIN_SECRET}"
    line="${line//__SIDB_ADMIN_SECRET_KEY__/$SIDB_ADMIN_SECRET_KEY}"
    line="${line//__SIDB_TDE_SECRET__/${SIDB_TDE_SECRET:-}}"
    line="${line//__SIDB_TDE_SECRET_KEY__/${SIDB_TDE_SECRET_KEY:-}}"
    line="${line//__SIDB_TCPS_TLS_SECRET__/${SIDB_TCPS_TLS_SECRET:-}}"
    line="${line//__SIDB_EDITION__/$SIDB_EDITION}"
    line="${line//__SIDB_CHARSET__/$SIDB_CHARSET}"
    line="${line//__SIDB_STORAGE_SIZE__/$SIDB_STORAGE_SIZE}"
    line="${line//__SIDB_STORAGE_CLASS__/$SIDB_STORAGE_CLASS}"
    line="${line//__SIDB_ACCESS_MODE__/$SIDB_ACCESS_MODE}"
    line="${line//__SIDB_REPLICAS__/$SIDB_REPLICAS}"
    line="${line//__SIDB_CASE_PIPELINE__/$SIDB_CASE_PIPELINE}"
    printf '%s\n' "$line" >> "$destination_file"
  done < "$source_file"
}

sidb_case_snapshot() {
  local reason="${1:-snapshot}"
  local snapshot_dir="${SIDB_CASE_RESULTS_DIR}/snapshots/${reason}"
  mkdir -p "$snapshot_dir"
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o yaml > "$snapshot_dir/sidb.yaml" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods -o wide > "$snapshot_dir/pods.txt" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pvc -o wide > "$snapshot_dir/pvc.txt" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc -o wide > "$snapshot_dir/services.txt" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get events --sort-by=.lastTimestamp > "$snapshot_dir/events.txt" 2>&1 || true
  echo "$(sidb_case_label) diagnostics: $snapshot_dir"
}

sidb_case_collect_yaml() {
  local namespace="$1" output_dir="$2"
  mkdir -p "$output_dir"

  sidb_case_kubectl -n "$namespace" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o yaml > "$output_dir/sidb.yaml" 2>&1 || true
  sidb_case_kubectl -n "$namespace" get all \
    -o yaml > "$output_dir/all-namespaced-resources.yaml" 2>&1 || true
  sidb_case_kubectl -n "$namespace" get pvc \
    -o yaml > "$output_dir/persistent-volume-claims.yaml" 2>&1 || true
  sidb_case_kubectl -n "$namespace" get endpoints \
    -o yaml > "$output_dir/endpoints.yaml" 2>&1 || true
  sidb_case_kubectl -n "$namespace" get events \
    -o yaml > "$output_dir/events.yaml" 2>&1 || true

  if [ -n "${OPERATOR_NAMESPACE:-}" ] && [ -n "${OPERATOR_DEPLOYMENT:-}" ]; then
    sidb_case_kubectl -n "$OPERATOR_NAMESPACE" get deployment "$OPERATOR_DEPLOYMENT" \
      -o yaml > "$output_dir/operator-deployment.yaml" 2>&1 || true
    sidb_case_kubectl -n "$OPERATOR_NAMESPACE" get pods \
      -o yaml > "$output_dir/operator-pods.yaml" 2>&1 || true
  fi
}

sidb_case_collect_pod_logs() {
  local namespace="$1" output_dir="$2" pod container safe_name pod_names containers
  mkdir -p "$output_dir"
  pod_names="$(sidb_case_kubectl -n "$namespace" get pods \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"

  while IFS= read -r pod; do
    [ -n "$pod" ] || continue
    local pod_dir="$output_dir/$pod"
    mkdir -p "$pod_dir"
    sidb_case_kubectl -n "$namespace" get pod "$pod" -o yaml \
      > "$pod_dir/pod.yaml" 2>&1 || true
    containers="$(sidb_case_kubectl -n "$namespace" get pod "$pod" \
      -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}{range .spec.containers[*]}{.name}{"\n"}{end}' 2>/dev/null || true)"

    while IFS= read -r container; do
      [ -n "$container" ] || continue
      safe_name="${container//[^A-Za-z0-9_.-]/_}"
      sidb_case_kubectl -n "$namespace" logs "$pod" -c "$container" --timestamps \
        > "$pod_dir/${safe_name}.log" 2>&1 || true
      sidb_case_kubectl -n "$namespace" logs "$pod" -c "$container" \
        --previous --timestamps > "$pod_dir/${safe_name}-previous.log" 2>&1 || true
    done <<< "$containers"
  done <<< "$pod_names"
}

sidb_case_collect_alert_logs() {
  local namespace="$1" output_dir="$2" pod container safe_name alert_path safe_path
  local pod_names containers alert_paths found=false
  mkdir -p "$output_dir"
  pod_names="$(sidb_case_kubectl -n "$namespace" get pods \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"

  while IFS= read -r pod; do
    [ -n "$pod" ] || continue
    containers="$(sidb_case_kubectl -n "$namespace" get pod "$pod" \
      -o jsonpath='{range .spec.containers[*]}{.name}{"\n"}{end}' 2>/dev/null || true)"
    while IFS= read -r container; do
      [ -n "$container" ] || continue
      safe_name="${container//[^A-Za-z0-9_.-]/_}"
      alert_paths="$(sidb_case_kubectl -n "$namespace" exec "$pod" -c "$container" -- \
        sh -c 'for root in /opt/oracle /u01/app/oracle /oracle /home/oracle; do
          if [ -d "$root" ]; then
            find "$root" -type f \( -name "alert.log" -o -name "alert_*.log" \) -print 2>/dev/null
          fi
        done' 2>/dev/null | sort -u || true)"
      while IFS= read -r alert_path; do
        [ -n "$alert_path" ] || continue
        found=true
        safe_path="${alert_path#/}"
        safe_path="${safe_path//\//_}"
        safe_path="${safe_path//[^A-Za-z0-9_.-]/_}"
        sidb_case_kubectl -n "$namespace" exec "$pod" -c "$container" -- \
          sh -c 'cat "$1"' sh "$alert_path" \
          > "$output_dir/${pod}-${safe_name}-${safe_path}" 2>&1 || true
      done <<< "$alert_paths"
    done <<< "$containers"
  done <<< "$pod_names"

  if [ "$found" != true ]; then
    printf '%s\n' 'No alert.log or alert_*.log file was found in the inspected Oracle roots.' \
      > "$output_dir/alert-log-not-found.txt"
  fi
}

sidb_case_collect_diagnostics() {
  local reason="${1:-final}"
  local diagnostics_dir="$SIDB_CASE_RESULTS_DIR/diagnostics"
  local marker="$diagnostics_dir/.complete"

  if [ -f "$marker" ]; then
    echo "$(sidb_case_label) diagnostics already collected: $diagnostics_dir"
    return 0
  fi

  mkdir -p "$diagnostics_dir"
  sidb_case_snapshot "$reason"
  sidb_case_collect_yaml "$SIDB_CASE_NAMESPACE" "$diagnostics_dir/kubernetes-yaml"
  sidb_case_collect_pod_logs "$SIDB_CASE_NAMESPACE" "$diagnostics_dir/database-pod-logs"
  sidb_case_collect_alert_logs "$SIDB_CASE_NAMESPACE" "$diagnostics_dir/alert-logs"

  if [ -n "${OPERATOR_NAMESPACE:-}" ]; then
    sidb_case_collect_pod_logs "$OPERATOR_NAMESPACE" "$diagnostics_dir/operator-pod-logs"
  fi

  {
    printf 'context=%s\n' "${KUBE_CONTEXT_NAME:-<default>}"
    printf 'namespace=%s\n' "$SIDB_CASE_NAMESPACE"
    printf 'sidb=%s\n' "$SIDB_TEST_NAME"
    printf 'image=%s\n' "$SIDB_IMAGE"
    printf 'reason=%s\n' "$reason"
    printf 'secret-data=not-collected\n'
  } > "$diagnostics_dir/run-info.txt"
  touch "$marker"
  echo "$(sidb_case_label) diagnostics collected: $diagnostics_dir"
}

sidb_case_setup_exit_snapshot() {
  local exit_code=$?
  if [ "$exit_code" -ne 0 ]; then
    sidb_case_collect_diagnostics "${SIDB_CASE_STAGE:-failure}" || true
  fi
  return "$exit_code"
}

sidb_case_assert_image_usage() {
  local spec_policy pod pod_policy image_id expected_digest
  spec_policy="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.image.imagePullPolicy}')"
  [ "$spec_policy" = "$SIDB_IMAGE_PULL_POLICY" ] || {
    echo "$(sidb_case_label): CR imagePullPolicy is '$spec_policy', expected '$SIDB_IMAGE_PULL_POLICY'" >&2
    return 1
  }
  expected_digest=""
  [[ "$SIDB_IMAGE" == *@sha256:* ]] && expected_digest="${SIDB_IMAGE##*@}"
  for pod in "$@"; do
    pod_policy="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.spec.containers[0].imagePullPolicy}')"
    [ "$pod_policy" = "$SIDB_IMAGE_PULL_POLICY" ] || {
      echo "$(sidb_case_label): Pod $pod imagePullPolicy is '$pod_policy', expected '$SIDB_IMAGE_PULL_POLICY'" >&2
      return 1
    }
    image_id="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.status.containerStatuses[0].imageID}')"
    [ -n "$image_id" ] || {
      echo "$(sidb_case_label): Pod $pod has no resolved imageID" >&2
      return 1
    }
    if [ -n "$expected_digest" ] && [[ "$image_id" != *"$expected_digest" ]]; then
      echo "$(sidb_case_label): Pod $pod imageID '$image_id' does not match '$expected_digest'" >&2
      return 1
    fi
  done
}
