#!/usr/bin/env bash
set -euo pipefail

PROFILE="${PROFILE:-mumbai}"

SHARED_REGION="${SHARED_REGION:-us-ashburn-1}"
SHARED_ZONE_ID="${SHARED_ZONE_ID:-ocid1.dns-zone.oc1.iad.replace-me}"

LEGACY_REGION="${LEGACY_REGION:-ap-mumbai-1}"
LEGACY_ZONE_ID="${LEGACY_ZONE_ID:-}"

MODE="check"
TARGET="both"

usage() {
  cat <<'EOF'
Usage:
  ./docs/sidb/external-dns/cleanup-dns.sh [--check|--apply] [primary|truecache|both]

Examples:
  ./docs/sidb/external-dns/cleanup-dns.sh
  ./docs/sidb/external-dns/cleanup-dns.sh --check primary
  ./docs/sidb/external-dns/cleanup-dns.sh --apply truecache
  ./docs/sidb/external-dns/cleanup-dns.sh --apply both

What it does:
  - Checks or deletes stale A records in the shared private zone
  - Also checks or deletes the matching ExternalDNS TXT owner records:
    - a-orcl-production.internal.example.com
    - a-truecache-production.internal.example.com
  - Optionally checks or deletes the same records from a legacy local zone
    when LEGACY_ZONE_ID is set during a migration from separate zones

Targets:
  primary    -> orcl-production.internal.example.com
  truecache  -> truecache-production.internal.example.com
  both       -> both names

Defaults:
  --check both

Important:
  - Use the shared zone that ExternalDNS currently publishes into.
  - A deleted record will be recreated if the annotated *-ext Service still exists
    and ExternalDNS is still running.
EOF
}

while (($#)); do
  case "$1" in
    --check)
      MODE="check"
      ;;
    --apply)
      MODE="apply"
      ;;
    primary|truecache|both)
      TARGET="$1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

domains_for_target() {
  case "$1" in
    primary)
      printf '%s\n' "orcl-production.internal.example.com"
      ;;
    truecache)
      printf '%s\n' "truecache-production.internal.example.com"
      ;;
    both)
      printf '%s\n' \
        "orcl-production.internal.example.com" \
        "truecache-production.internal.example.com"
      ;;
    *)
      echo "Unsupported target: $1" >&2
      exit 1
      ;;
  esac
}

rrset_get() {
  local region="$1"
  local zone_id="$2"
  local domain="$3"
  local rtype="$4"

  oci --profile "${PROFILE}" --region "${region}" dns record rrset get \
    --zone-name-or-id "${zone_id}" \
    --domain "${domain}" \
    --rtype "${rtype}" \
    --scope PRIVATE
}

rrset_delete() {
  local region="$1"
  local zone_id="$2"
  local domain="$3"
  local rtype="$4"

  oci --profile "${PROFILE}" --region "${region}" dns record rrset delete \
    --zone-name-or-id "${zone_id}" \
    --domain "${domain}" \
    --rtype "${rtype}" \
    --scope PRIVATE \
    --force
}

handle_rrset() {
  local zone_name="$1"
  local region="$2"
  local zone_id="$3"
  local domain="$4"
  local rtype="$5"

  echo
  echo "[${zone_name}] ${rtype} ${domain}"

  if rrset_get "${region}" "${zone_id}" "${domain}" "${rtype}" >"${TMPFILE}" 2>/dev/null; then
    cat "${TMPFILE}"
    if [[ "${MODE}" == "apply" ]]; then
      rrset_delete "${region}" "${zone_id}" "${domain}" "${rtype}" >"${TMPFILE}" 2>/dev/null
      echo "deleted ${rtype} ${domain} from ${zone_name}"
    fi
  else
    echo "not present"
  fi
}

TMPFILE="$(mktemp /tmp/cleanup-dns.XXXXXX)"
trap 'rm -f "${TMPFILE}"' EXIT

echo "mode=${MODE} target=${TARGET} profile=${PROFILE}"

while IFS= read -r domain; do
  txt_domain="a-${domain}"

  handle_rrset "shared-zone" "${SHARED_REGION}" "${SHARED_ZONE_ID}" "${domain}" "A"
  handle_rrset "shared-zone" "${SHARED_REGION}" "${SHARED_ZONE_ID}" "${txt_domain}" "TXT"

  if [[ -n "${LEGACY_ZONE_ID}" ]]; then
    handle_rrset "legacy-local-zone" "${LEGACY_REGION}" "${LEGACY_ZONE_ID}" "${domain}" "A"
    handle_rrset "legacy-local-zone" "${LEGACY_REGION}" "${LEGACY_ZONE_ID}" "${txt_domain}" "TXT"
  fi
done < <(domains_for_target "${TARGET}")

if [[ "${MODE}" == "check" ]]; then
  echo
  echo "check complete"
else
  echo
  echo "cleanup complete"
fi
