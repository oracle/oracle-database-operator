#!/usr/bin/env bash
# Render a DataguardBroker manifest from SIDB (or sharding/rac) status.dataguard.renderedBrokerSpec.
#
# Single-member:
#   render-dg-broker-from-status.sh sidb <standby-name> [namespace] [broker-name]
#
# Multi-member (union across clusters / standbys):
#   render-dg-broker-from-status.sh sidb \
#     --broker-name sidb-multi-dg \
#     --namespace default \
#     --kubeconfig ash=/path/to/ashburn.kubeconfig \
#     --kubeconfig mum=/path/to/mumbai.kubeconfig \
#     --member ash:sidb-sample:lb=sidb-sample-lb \
#     --member ash:sidb-standby1:lb=sidb-standby1-lb \
#     --member mum:sidb-standby2:lb=sidb-standby2-lb \
#     --member mum:sidb-standby3:lb=sidb-standby3-lb \
#     --source-ref mum:sidb-standby3
#
# Member suffix options (optional, colon-separated after name):
#   lb=<svc>          set every endpoint host to the Service EXTERNAL-IP (or hostname)
#   host=<ip>         set every endpoint host (preserve ports from status)
#   host=<ip>:<port>  set every endpoint host and port (TCP-only / CMAN NodePort cases)
#   ns=<namespace>    override namespace for this member (default: --namespace)
#
# Requires: kubectl, jq. Optional: yq (python-yq) to emit YAML instead of JSON.

set -euo pipefail

usage() {
  cat <<'EOF' >&2
Usage:
  render-dg-broker-from-status.sh <sidb|sharding|rac> <name> [namespace] [broker-name]

  render-dg-broker-from-status.sh <sidb|sharding|rac> \
    --broker-name <name> \
    --namespace <ns> \
    --kubeconfig <alias>=<path> ... \
    --member <alias>:<cr-name>[:lb=<svc>|:host=<ip>[:port]|:ns=<ns>] ... \
    --source-ref <alias>:<cr-name>

Single mode reads one CR's status.dataguard.renderedBrokerSpec (current KUBECONFIG).
Multi mode unions members from each --member CR, applies endpoint overrides, and
sets topology.sourceRef / localRef for the --source-ref cluster.
EOF
  exit 1
}

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

kind="${1:-}"
[[ -n "$kind" ]] || usage
shift

case "$kind" in
  sidb) resource="singleinstancedatabase" ;;
  sharding) resource="shardingdatabase" ;;
  rac) resource="racdatabase" ;;
  -h|--help) usage ;;
  *)
    echo "unsupported kind: $kind" >&2
    usage
    ;;
esac

broker_name=""
namespace="default"
source_ref=""
declare -A KUBECONFIGS=()
MEMBERS=()

# Positional single-mode args collected if no --member flags appear.
positional=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --broker-name)
      broker_name="${2:?--broker-name requires a value}"
      shift 2
      ;;
    --namespace)
      namespace="${2:?--namespace requires a value}"
      shift 2
      ;;
    --kubeconfig)
      kv="${2:?--kubeconfig requires alias=path}"
      shift 2
      alias="${kv%%=*}"
      path="${kv#*=}"
      if [[ -z "$alias" || -z "$path" || "$alias" == "$kv" ]]; then
        echo "ERROR: --kubeconfig must be alias=/path/to/kubeconfig" >&2
        exit 1
      fi
      if [[ ! -f "$path" ]]; then
        echo "ERROR: kubeconfig not found for alias '$alias': $path" >&2
        exit 1
      fi
      KUBECONFIGS["$alias"]="$path"
      ;;
    --member)
      MEMBERS+=("${2:?--member requires alias:name[:options]}")
      shift 2
      ;;
    --source-ref)
      source_ref="${2:?--source-ref requires alias:name}"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    --*)
      echo "ERROR: unknown option: $1" >&2
      usage
      ;;
    *)
      positional+=("$1")
      shift
      ;;
  esac
done

emit_header() {
  cat <<'EOF'
# Review this generated manifest before applying it.
# Relationships are inferred from member roles: the PRIMARY is paired with each standby.
# Members inherit topology.defaults.adminSecretRef. Add a member-level adminSecretRef only when a member uses a different password Secret.
# Local SIDB endpoints use <service>.<namespace>.svc.cluster.local unless overridden with lb= or host=.
# Missing TCPS endpoint ports default to 2484. Existing endpoint ports are preserved.
# Replace placeholder Secret names or keys, and verify the TCPS client wallet before applying.
EOF
}

emit_doc() {
  # Prefer YAML when yq is available; otherwise JSON (kubectl accepts both).
  if command -v yq >/dev/null 2>&1; then
    if yq -y '.' >/dev/null 2>&1 <<<"{}"; then
      yq -y '.'
    elif yq -oy . >/dev/null 2>&1 <<<"{}"; then
      yq -oy .
    elif yq -P . >/dev/null 2>&1 <<<"{}"; then
      yq -P .
    else
      cat
    fi
  else
    cat
  fi
}

normalize_member_json() {
  # stdin: full CR json
  # args: broker_name namespace
  local bn="$1"
  local ns="$2"

  jq -e --arg broker_name "$bn" --arg namespace "$ns" '
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

            # Materialize each rendered specs effective adminSecretRef
            # before members from multiple specs are merged.
            | (.topology.defaults.adminSecretRef // null) as $default_secret

            | .topology.members |= map(
                if (.adminSecretRef // null) == null
                   and $default_secret != null
                then
                  .adminSecretRef = $default_secret
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
}

# Remove member-level adminSecretRef only when it matches
# the FINAL topology default.
strip_redundant_admin_secret() {
  jq '
    .spec.topology as $topology
    | .spec.topology.members |= map(
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
      )
  '
}

lb_ingress() {
  local kcfg="$1" ns="$2" svc="$3"
  local ip host

  ip=$(kubectl --kubeconfig="$kcfg" -n "$ns" get svc "$svc" \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)

  if [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "$ip"
    return 0
  fi

  host=$(kubectl --kubeconfig="$kcfg" -n "$ns" get svc "$svc" \
    -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)

  if [[ -n "$host" ]]; then
    echo "$host"
    return 0
  fi

  echo "ERROR: Service $ns/$svc has no LoadBalancer EXTERNAL-IP/hostname yet" >&2
  exit 1
}

parse_member_spec() {
  # Sets: m_alias m_name m_ns m_lb m_host m_port
  local spec="$1"
  local default_ns="$2"

  m_alias=""
  m_name=""
  m_ns="$default_ns"
  m_lb=""
  m_host=""
  m_port=""

  IFS=':' read -r m_alias m_name rest <<<"$spec"

  if [[ -z "$m_alias" || -z "$m_name" ]]; then
    echo "ERROR: --member must be alias:cr-name[:options] (got: $spec)" >&2
    exit 1
  fi

  m_alias="${spec%%:*}"
  local after_alias="${spec#*:}"
  m_name="${after_alias%%:*}"

  local opts=""
  if [[ "$after_alias" == *:* ]]; then
    opts="${after_alias#*:}"
  fi

  if [[ -z "$m_alias" || -z "$m_name" ]]; then
    echo "ERROR: --member must be alias:cr-name[:options] (got: $spec)" >&2
    exit 1
  fi

  if [[ -n "$opts" ]]; then
    local remaining="$opts"

    while [[ -n "$remaining" ]]; do
      local token=""

      if [[ "$remaining" == ns=* ]]; then
        token="${remaining%%:*}"
        m_ns="${token#ns=}"
        if [[ "$remaining" == *:* ]]; then
          remaining="${remaining#*:}"
        else
          remaining=""
        fi

      elif [[ "$remaining" == lb=* ]]; then
        token="${remaining%%:*}"
        m_lb="${token#lb=}"
        if [[ "$remaining" == *:* ]]; then
          remaining="${remaining#*:}"
        else
          remaining=""
        fi

      elif [[ "$remaining" == host=* ]]; then
        local hostpart="${remaining#host=}"

        if [[ "$hostpart" =~ ^([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+):([0-9]+)(:.*)?$ ]]; then
          m_host="${BASH_REMATCH[1]}"
          m_port="${BASH_REMATCH[2]}"
          remaining="${BASH_REMATCH[3]#:}"
        elif [[ "$hostpart" =~ ^([^:]+)(:.*)?$ ]]; then
          m_host="${BASH_REMATCH[1]}"
          remaining="${BASH_REMATCH[2]#:}"
        else
          echo "ERROR: invalid host= in --member $spec" >&2
          exit 1
        fi

      else
        echo "ERROR: unknown option in --member $spec (near: $remaining)" >&2
        exit 1
      fi
    done
  fi
}

# For single-SIDB rendering, resolve an endpoint-based PRIMARY only when
# its service DNS and DB_UNIQUE_NAME match a real SIDB in this namespace.
# External connectString primaries are left unchanged.
enrich_local_sidb_primary() {
  local ns="$1"
  local doc host suffix local_name primary

  doc=$(cat)
  host=$(printf '%s\n' "$doc" | jq -r '
    [.spec.topology.members[]?
     | select(.role == "PRIMARY" and .localRef == null)
     | .endpoints[]?.host] | unique
    | if length == 1 then .[0] else empty end
  ')

  suffix=".$ns.svc.cluster.local"
  if [[ -z "$host" || "$host" != *"$suffix" ]]; then
    printf '%s\n' "$doc"
    return
  fi

  local_name="${host%"$suffix"}"
  if [[ ! "$local_name" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    printf '%s\n' "$doc"
    return
  fi

  if ! primary=$(kubectl get singleinstancedatabase "$local_name" -n "$ns" -o json 2>/dev/null); then
    printf '%s\n' "$doc"
    return
  fi

  printf '%s\n' "$doc" | jq --arg ns "$ns" --arg host "$host" \
    --arg name "$local_name" --argjson primary "$primary" '
    .spec.topology.members |= map(
      if .role == "PRIMARY"
         and .localRef == null
         and ((.dbUniqueName // "" | ascii_upcase) == ($primary.spec.sid // "" | ascii_upcase))
         and (($primary.status.role // $primary.spec.createAs // "" | ascii_upcase) == "PRIMARY")
         and any(.endpoints[]?; .host == $host)
      then
        .name = $name
        | .localRef = {
            apiVersion: "database.oracle.com/v4",
            kind: "SingleInstanceDatabase",
            name: $name,
            namespace: $ns
          }
        | if .tcps.enabled == true and ($primary.status.clientWalletSecret // "") != "" then
            .tcps.clientWalletSecret = $primary.status.clientWalletSecret
          else
            .
          end
      else
        .
      end
    )
  '
}

# ---------- single-member mode ----------
if [[ ${#MEMBERS[@]} -eq 0 ]]; then
  name="${positional[0]:-}"
  [[ -n "$name" ]] || usage

  namespace="${positional[1]:-$namespace}"
  broker_name="${positional[2]:-${broker_name:-${name}-dg}}"

  emit_header

  if [[ "$kind" == "sidb" ]]; then
    kubectl get "$resource" "$name" -n "$namespace" -o json \
      | normalize_member_json "$broker_name" "$namespace" \
      | enrich_local_sidb_primary "$namespace" \
      | strip_redundant_admin_secret \
      | emit_doc
  else
    kubectl get "$resource" "$name" -n "$namespace" -o json \
      | normalize_member_json "$broker_name" "$namespace" \
      | strip_redundant_admin_secret \
      | emit_doc
  fi

  exit 0
fi

# ---------- multi-member mode ----------
if [[ ${#positional[@]} -gt 0 ]]; then
  echo "ERROR: unexpected positional args in multi-member mode: ${positional[*]}" >&2
  usage
fi

if [[ -z "$broker_name" ]]; then
  broker_name="sidb-multi-dg"
fi

if [[ -z "$source_ref" ]]; then
  echo "ERROR: --source-ref alias:name is required in multi-member mode" >&2
  exit 1
fi

src_alias="${source_ref%%:*}"
src_name="${source_ref#*:}"

if [[ -z "$src_alias" || -z "$src_name" || "$src_alias" == "$source_ref" ]]; then
  echo "ERROR: --source-ref must be alias:cr-name" >&2
  exit 1
fi

src_ns="$namespace"

if [[ ${#KUBECONFIGS[@]} -eq 0 ]]; then
  if [[ -n "${KUBECONFIG:-}" && -f "${KUBECONFIG}" ]]; then
    KUBECONFIGS["default"]="$KUBECONFIG"
  else
    echo "ERROR: provide at least one --kubeconfig alias=path" >&2
    exit 1
  fi
fi

if [[ -z "${KUBECONFIGS[$src_alias]+x}" ]]; then
  echo "ERROR: --source-ref alias '$src_alias' has no --kubeconfig" >&2
  exit 1
fi

TMPROOT=$(mktemp -d)
trap 'rm -rf "$TMPROOT"' EXIT

overrides_file="$TMPROOT/overrides.jsonl"
: >"$overrides_file"

render_files=()
base_file=""

echo "Rendering multi-member DataguardBroker '$broker_name' (namespace $namespace)" >&2

for spec in "${MEMBERS[@]}"; do
  parse_member_spec "$spec" "$namespace"

  if [[ -z "${KUBECONFIGS[$m_alias]+x}" ]]; then
    echo "ERROR: --member alias '$m_alias' has no --kubeconfig" >&2
    exit 1
  fi

  kcfg="${KUBECONFIGS[$m_alias]}"

  if [[ -n "$m_lb" && -n "$m_host" ]]; then
    echo "ERROR: --member $spec: use either lb= or host=, not both" >&2
    exit 1
  fi

  cr_json="$TMPROOT/${m_alias}_${m_name}.cr.json"

  kubectl --kubeconfig="$kcfg" \
    get "$resource" "$m_name" -n "$m_ns" -o json >"$cr_json"

  dbun=$(jq -r '.spec.sid // .status.sid // empty' "$cr_json")

  if [[ -z "$dbun" ]]; then
    echo "ERROR: $m_alias/$m_ns/$m_name has no spec.sid/status.sid" >&2
    exit 1
  fi

  create_as=$(jq -r '.spec.createAs // empty' "$cr_json")
  status_role=$(jq -r '.status.role // empty' "$cr_json")

  case "$create_as" in
    primary)
      role="PRIMARY"
      ;;
    standby)
      role="PHYSICAL_STANDBY"
      ;;
    *)
      case "$status_role" in
        PRIMARY)
          role="PRIMARY"
          ;;
        PHYSICAL_STANDBY|PHYSICAL\ STANDBY)
          role="PHYSICAL_STANDBY"
          ;;
        *)
          role="PHYSICAL_STANDBY"
          ;;
      esac
      ;;
  esac

  ov_host=""
  ov_port=""

  if [[ -n "$m_lb" ]]; then
    ov_host=$(lb_ingress "$kcfg" "$m_ns" "$m_lb")
    echo "  $m_alias/$m_name ($dbun): LB $m_ns/$m_lb -> $ov_host" >&2

  elif [[ -n "$m_host" ]]; then
    ov_host="$m_host"
    ov_port="$m_port"

    if [[ -n "$ov_port" ]]; then
      echo "  $m_alias/$m_name ($dbun): host $ov_host port $ov_port" >&2
    else
      echo "  $m_alias/$m_name ($dbun): host $ov_host (ports from status)" >&2
    fi

  else
    echo "  $m_alias/$m_name ($dbun): endpoints from status (ClusterIP DNS if localRef)" >&2
  fi

  out="$TMPROOT/${m_alias}_${m_name}.json"
  rendered=false

  if normalize_member_json "$broker_name" "$m_ns" \
      <"$cr_json" >"$out" \
      2>"$TMPROOT/${m_alias}_${m_name}.err"; then

    render_files+=("$out")
    rendered=true

    if [[ "$m_alias" == "$src_alias" && "$m_name" == "$src_name" ]]; then
      base_file="$out"
      src_ns="$m_ns"
    fi
  else
    echo "  note: no renderedBrokerSpec for $m_alias/$m_name ($(tr '\n' ' ' <"$TMPROOT/${m_alias}_${m_name}.err")) — will synthesize/merge by sid=$dbun" >&2
    rm -f "$out"
  fi

  svc_name=$(jq -r --arg p "${ov_port:-1521}" '
      (.status.connectString // "") as $c
      | if ($c | test(":[0-9]+/")) then
          ($c | capture(":(?<port>[0-9]+)/").port)
        else
          $p
        end
    ' "$cr_json" 2>/dev/null || echo "${ov_port:-1521}")

  jq -nc \
    --arg alias "$m_alias" \
    --arg name "$m_name" \
    --arg ns "$m_ns" \
    --arg dbun "$dbun" \
    --arg role "$role" \
    --arg host "$ov_host" \
    --arg port "${ov_port:-}" \
    --arg default_port "$svc_name" \
    --argjson is_local "$([[ "$m_alias" == "$src_alias" ]] && echo true || echo false)" \
    --argjson rendered "$rendered" \
    '{
      alias:$alias,
      name:$name,
      namespace:$ns,
      dbUniqueName:$dbun,
      role:$role,
      host:$host,
      port:$port,
      default_port:$default_port,
      is_local:$is_local,
      rendered:$rendered
    }' >>"$overrides_file"
done

if [[ -z "$base_file" ]]; then
  echo "  fetching --source-ref $src_alias/$src_name for Broker base spec" >&2

  base_file="$TMPROOT/source_ref_base.json"

  if ! kubectl \
      --kubeconfig="${KUBECONFIGS[$src_alias]}" \
      get "$resource" "$src_name" -n "$src_ns" -o json \
      | normalize_member_json "$broker_name" "$src_ns" >"$base_file"; then

    jq -n \
      --arg bn "$broker_name" \
      --arg ns "$namespace" '
      {
        apiVersion:"database.oracle.com/v4",
        kind:"DataguardBroker",
        metadata:{
          name:$bn,
          namespace:$ns
        },
        spec:{
          execution:{
            authWallet:{
              enabled:true
            }
          },
          topology:{
            defaults:{},
            members:[]
          }
        }
      }
    ' >"$base_file"

    echo "  warning: source-ref has no renderedBrokerSpec; using skeleton (set execution.image manually if needed)" >&2
  fi
fi

if [[ ${#render_files[@]} -eq 0 ]]; then
  echo "ERROR: none of the --member CRs published status.dataguard.renderedBrokerSpec (readyForBroker)" >&2
  exit 1
fi

final_json="$TMPROOT/final.json"

jq -s \
  --arg broker_name "$broker_name" \
  --arg namespace "$namespace" \
  --arg src_name "$src_name" \
  --arg src_ns "$src_ns" \
  --slurpfile overrides <(jq -s '.' "$overrides_file") \
  '
    . as $all
    | ($all[0]) as $base

    | (
        reduce $all[] as $doc ({};
          reduce ($doc.spec.topology.members // [])[] as $m (.;
            ($m.dbUniqueName // $m.name) as $key
            | if .[$key] == null then
                .[$key] = $m
              else
                .
              end
          )
        )
      ) as $by_key

    | (
        reduce $overrides[0][] as $o ({};
          .[$o.dbUniqueName] = $o
        )
      ) as $ov_by_dbun

    | (
        reduce $overrides[0][] as $o ($by_key;
          if .[$o.dbUniqueName] == null then
            .[$o.dbUniqueName] = {
              dbUniqueName: $o.dbUniqueName,
              name: $o.name,
              role: $o.role,
              endpoints: [{
                name: "tcp",
                host: (
                  if (($o.host // "") | length) > 0 then
                    $o.host
                  else
                    "<REPLACE_WITH_REACHABLE_HOST>"
                  end
                ),
                port: (
                  if (($o.port // "") | length) > 0 then
                    ($o.port | tonumber)
                  else
                    (($o.default_port // "1521") | tonumber)
                  end
                ),
                protocol: "TCP",
                serviceName: $o.dbUniqueName
              }]
            }
          else
            .
          end
        )
      ) as $by_key2

    | (
        $by_key2
        | to_entries
        | map(
            .key as $dbun
            | .value
            | ($ov_by_dbun[$dbun] // null) as $ov

            | if $ov == null then
                .
              else
                .name = $ov.name
                | .role = ($ov.role // .role)
                | .dbUniqueName = $ov.dbUniqueName

                | if (($ov.host // "") | length) > 0 then
                    .endpoints |= map(
                      .host = $ov.host
                      | if (($ov.port // "") | length) > 0 then
                          .port = ($ov.port | tonumber)
                        else
                          .
                        end
                    )
                  else
                    .
                  end

                | if $ov.is_local == true then
                    .localRef = {
                      apiVersion: "database.oracle.com/v4",
                      kind: "SingleInstanceDatabase",
                      name: $ov.name,
                      namespace: $ov.namespace
                    }
                  else
                    del(.localRef)
                  end
              end
          )

        | sort_by(
            if (.role // "") == "PRIMARY" then
              "0"
            else
              "1-" + (.dbUniqueName // .name // "")
            end
          )
      ) as $members

    # A valid merged Broker topology must have exactly one PRIMARY.
    | (
        [
          $members[]
          | select((.role // "" | ascii_upcase) == "PRIMARY")
        ]
        | length
      ) as $primary_count

    | if $primary_count != 1 then
        error(
          "expected exactly one PRIMARY after merging members; found "
          + ($primary_count | tostring)
        )
      else
        .
      end

    | (
        reduce $all[] as $doc (null;
          if . != null then
            .
          elif (($doc.spec.execution.image // "") | length) > 0 then
            $doc.spec.execution
          else
            .
          end
        )
      ) as $exec

    | $base
    | .metadata.name = $broker_name
    | .metadata.namespace = $namespace

    | if $exec != null then
        .spec.execution = $exec
      else
        .
      end

    | .spec.topology.members = $members

    # Only remove member adminSecretRef after all members have been
    # merged under the final topology defaults.
    | .spec.topology as $topology
    | .spec.topology.members |= map(
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
      )

    | .spec.topology.sourceKind = "SingleInstanceDatabase"

    | .spec.topology.sourceRef = {
        apiVersion: "database.oracle.com/v4",
        kind: "SingleInstanceDatabase",
        name: $src_name,
        namespace: $src_ns
      }
  ' "$base_file" "${render_files[@]}" >"$final_json"

member_count=$(jq '.spec.topology.members | length' "$final_json")

echo "Wrote DataguardBroker '$broker_name' with $member_count members (stdout)" >&2

emit_header
emit_doc <"$final_json"