#!/usr/bin/env bash
# rac/cleanup: delete the test-owned RacDatabase and its managed resources.
set -Eeuo pipefail

namespace="${RAC_E2E_NAMESPACE:-rac-ns}"
name="${RAC_CLEANUP_NAME:-${RAC_TEST17_NAME:-racdbprov-sample}}"
timeout_value="${RAC_CLEANUP_TIMEOUT:-30m}"

fail() {
  echo "rac/cleanup: ERROR: $*" >&2
  exit 2
}

timeout_seconds() {
  local value="$1"
  case "$value" in
    *h) echo $(( ${value%h} * 3600 )) ;;
    *m) echo $(( ${value%m} * 60 )) ;;
    *s) echo $(( ${value%s} )) ;;
    *) echo "$value" ;;
  esac
}

case "$namespace" in
  ""|default|kube-system|kube-public|kube-node-lease)
    fail "refusing to clean protected or empty namespace: ${namespace:-<empty>}"
    ;;
esac
[ -n "$name" ] || fail "RAC cleanup name is empty"
command -v kubectl >/dev/null 2>&1 || fail "kubectl is required"
kubectl get namespace "$namespace" >/dev/null 2>&1 || {
  echo "rac/cleanup: namespace does not exist; already clean: $namespace"
  exit 0
}

echo "rac/cleanup: deleting RacDatabase $namespace/$name"
if kubectl get racdatabase "$name" -n "$namespace" >/dev/null 2>&1; then
  kubectl delete racdatabase "$name" -n "$namespace" \
    --ignore-not-found=true --wait=true --timeout="$timeout_value" ||
    fail "RacDatabase deletion did not complete: $namespace/$name"
else
  echo "rac/cleanup: RacDatabase is already absent; checking for orphaned managed resources"
fi

# The controller removes these objects as part of the RacDatabase finalizer.
# The label cleanup is deliberately limited to the configured test name so an
# explicit RAC cleanup cannot remove another RAC database in the namespace.
delete_labeled() {
  local resource="$1"
  local selector="$2"
  local items

  items="$(kubectl get "$resource" -n "$namespace" -l "$selector" \
    -o name --ignore-not-found=true 2>/dev/null || true)"
  [ -n "$items" ] || return 0
  echo "rac/cleanup: deleting remaining $resource resources for $selector"
  kubectl delete "$resource" -n "$namespace" -l "$selector" \
    --ignore-not-found=true --wait=true --timeout="$timeout_value" ||
    fail "failed to delete $resource resources selected by $selector"
}

wait_for_no_labeled() {
  local resource="$1"
  local selector="$2"
  local deadline=$((SECONDS + $(timeout_seconds "$timeout_value")))
  local items

  while (( SECONDS < deadline )); do
    items="$(kubectl get "$resource" -n "$namespace" -l "$selector" \
      -o name --ignore-not-found=true 2>/dev/null || true)"
    [ -z "$items" ] && return 0
    echo "rac/cleanup: waiting for $resource resources selected by $selector: $items"
    sleep 5
  done
  echo "rac/cleanup: resources still present after $timeout_value: $resource $selector" >&2
  kubectl get "$resource" -n "$namespace" -l "$selector" -o wide >&2 || true
  return 1
}

# Delete any remaining controller-owned objects only after the RacDatabase
# finalizer has completed (or when the CR was already absent from a prior run).
delete_labeled statefulsets "database.oracle.com/name=$name"
delete_labeled services "database.oracle.com/name=$name"
delete_labeled configmaps "database.oracle.com/name=$name"
delete_labeled pods "database.oracle.com/name=$name"
delete_labeled persistentvolumeclaims "database.oracle.com/name=$name"
delete_labeled persistentvolumeclaims "app.kubernetes.io/instance=$name"

wait_for_no_labeled statefulsets "database.oracle.com/name=$name"
wait_for_no_labeled services "database.oracle.com/name=$name"
wait_for_no_labeled configmaps "database.oracle.com/name=$name"
wait_for_no_labeled pods "database.oracle.com/name=$name"
wait_for_no_labeled persistentvolumeclaims "database.oracle.com/name=$name"
wait_for_no_labeled persistentvolumeclaims "app.kubernetes.io/instance=$name"

echo "rac/cleanup: PASS — removed test-owned RAC resources for $namespace/$name"
