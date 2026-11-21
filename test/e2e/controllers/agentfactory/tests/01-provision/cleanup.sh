#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

export AGENTFACTORY_CASE_ID=test01

exec "$script_dir/../common/cleanup.sh"
