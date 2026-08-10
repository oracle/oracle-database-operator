#!/usr/bin/env bash
set -uo pipefail

BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INVALID_DIR="${BASE_DIR}/invalid"
NAMESPACE="${1:-default}"
DRY_RUN="${DRY_RUN:-false}"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "ERROR: kubectl not found in PATH" >&2
  exit 2
fi

if [[ ! -d "${INVALID_DIR}" ]]; then
  echo "ERROR: invalid directory not found: ${INVALID_DIR}" >&2
  exit 2
fi

echo "Running invalid webhook cases"
echo "Namespace: ${NAMESPACE}"
echo "Directory: ${INVALID_DIR}"
echo

pass_count=0
fail_count=0

mapfile -t files < <(find "${INVALID_DIR}" -maxdepth 1 -type f -name '*.yaml' | sort)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "No invalid YAML files found."
  exit 1
fi

for file in "${files[@]}"; do
  name="$(basename "${file}")"

  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "[DRY-RUN] ${name}"
    ((pass_count++))
    continue
  fi

  output="$(kubectl -n "${NAMESPACE}" apply -f "${file}" 2>&1)"
  rc=$?

  if [[ ${rc} -ne 0 ]]; then
    echo "[PASS] ${name} (rejected as expected)"
    ((pass_count++))
  else
    echo "[FAIL] ${name} (unexpectedly accepted)"
    echo "       ${output}" | sed 's/^/       /'
    ((fail_count++))

    obj_kind="$(awk '/^kind:/{print $2; exit}' "${file}")"
    obj_name="$(awk '/^  name:/{print $2; exit}' "${file}")"
    if [[ -n "${obj_kind}" && -n "${obj_name}" ]]; then
      kubectl -n "${NAMESPACE}" delete "${obj_kind,,}" "${obj_name}" --ignore-not-found >/dev/null 2>&1 || true
    fi
  fi

done

echo
echo "Summary: ${pass_count} passed, ${fail_count} failed, total ${#files[@]}"

if [[ ${fail_count} -gt 0 ]]; then
  exit 1
fi
