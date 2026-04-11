#!/usr/bin/env bash
set -euo pipefail

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi
if ! command -v yq >/dev/null 2>&1; then
  echo "yq is required" >&2
  exit 1
fi

kind="${1:?kind required: sidb|sharding|rac}"
name="${2:?resource name required}"
namespace="${3:-default}"
broker_name="${4:-${name}-dg}"

case "$kind" in
  sidb)
    resource="singleinstancedatabase"
    ;;
  sharding)
    resource="shardingdatabase"
    ;;
  rac)
    resource="racdatabase"
    ;;
  *)
    echo "unsupported kind: $kind" >&2
    exit 1
    ;;
esac

kubectl get "$resource" "$name" -n "$namespace" -o json \
| jq -e --arg broker_name "$broker_name" --arg namespace "$namespace" '
    .status.dataguard as $dg
    | if $dg == null then
        error("status.dataguard is missing")
      elif ($dg.readyForBroker // false) != true then
        error("status.dataguard.readyForBroker is not true")
      elif ($dg.renderedBrokerSpec.spec // null) == null then
        error("status.dataguard.renderedBrokerSpec.spec is missing")
      else
        {
          apiVersion: "database.oracle.com/v4",
          kind: "DataguardBroker",
          metadata: {
            name: ($dg.renderedBrokerSpec.name // $broker_name),
            namespace: ($dg.renderedBrokerSpec.namespace // $namespace)
          },
          spec: $dg.renderedBrokerSpec.spec
        }
      end
  ' | yq -P
