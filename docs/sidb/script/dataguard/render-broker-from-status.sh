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

cat <<'EOF'
# Review this generated manifest before applying it.
# Relationships are inferred from member roles: the PRIMARY is paired with each standby.
# Members inherit topology.defaults.adminSecretRef. Add a member-level adminSecretRef only when a member uses a different password Secret.
# Local SIDB endpoints use <service>.<namespace>.svc.cluster.local. Replace external or placeholder hosts with a broker-reachable hostname.
# Missing TCPS endpoint ports default to 2484. Existing endpoint ports are preserved.
# Replace placeholder Secret names or keys, and verify the TCPS client wallet before applying.
EOF

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
          spec: (
            $dg.renderedBrokerSpec.spec
            | .topology as $topology
            | .topology.members |= map(
                . as $member
                | if ($topology.defaults.adminSecretRef // null) != null
                    and (
                      ($member.adminSecretRef.secretName // "")
                      ==
                      ($topology.defaults.adminSecretRef.secretName // "")
                    )
                    and (
                      ($member.adminSecretRef.secretKey // "")
                      ==
                      ($topology.defaults.adminSecretRef.secretKey // "")
                    )
                  then
                    del(.adminSecretRef)
                  else
                    .
                  end

                | if .localRef != null then
                    (.localRef.namespace // $namespace) as $member_namespace
                    | (.localRef.name // "REPLACE_WITH_SIDB_SERVICE") as $service
                    | .endpoints |= map(
                        .host = (
                          $service
                          + "."
                          + $member_namespace
                          + ".svc.cluster.local"
                        )
                      )
                  else
                    .endpoints |= map(
                        if ((.host // "") | length) == 0 then
                          .host = "<REPLACE_WITH_REACHABLE_HOST>"
                        else
                          .
                        end
                      )
                  end

                | .endpoints |= map(
                    if ((.port // 0) == 0)
                       and ((.protocol // "" | ascii_upcase) == "TCPS")
                    then
                      .port = 2484
                    else
                      .
                    end
                  )
              )
          )
        }
      end
  '