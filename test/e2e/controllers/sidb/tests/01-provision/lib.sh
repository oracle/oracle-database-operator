#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/../common/lib.sh"

# TLS Secret used when enable-tcps.sh patches TCPS onto the ready primary.
# Day-0 test01.yaml stays TCP-only; prerequisites create this Secret early so
# the later patch can mount it without a separate secret stage.
export SIDB_TCPS_TLS_SECRET="${SIDB_TCPS_TLS_SECRET:-sidb-e2e-tcps-tls-test01}"
