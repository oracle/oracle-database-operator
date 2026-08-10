#!/bin/bash
set -euo pipefail

access_log_dir="/opt/oracle/sa/log/global"

cleanup() {
	if [[ -n "${tail_pid:-}" ]]; then
		kill "${tail_pid}" 2>/dev/null || true
		wait "${tail_pid}" 2>/dev/null || true
	fi
}

trap cleanup EXIT
trap 'trap - EXIT; cleanup; exit 0' INT TERM

date +"%Y-%m-%d %H:%M:%S"
echo "=================== ORDS access log forwarder start ==================="
echo "Watching ${access_log_dir}/ords_*.log"

if [[ ! -d "${access_log_dir}" ]]; then
	echo "ERROR: access log directory does not exist: ${access_log_dir}" >&2
	exit 1
fi

if [[ ! -r "${access_log_dir}" ]]; then
	echo "ERROR: access log directory is not readable: ${access_log_dir}" >&2
	exit 1
fi

current_log=""

while true; do
	newest_log=""
	for candidate_log in "${access_log_dir}"/ords_*.log; do
		[[ -f "${candidate_log}" ]] || continue
		newest_log="${candidate_log}"
	done

	if [[ -z "${newest_log}" ]]; then
		sleep 5
		continue
	fi

	if [[ "${newest_log}" != "${current_log}" ]]; then
		if [[ -n "${tail_pid:-}" ]]; then
			kill "${tail_pid}" 2>/dev/null || true
			wait "${tail_pid}" 2>/dev/null || true
		fi

		current_log="${newest_log}"
		echo "=================== Forwarding ORDS access log: ${current_log} ==================="
		tail -n +1 -F "${current_log}" &
		tail_pid="$!"
	fi

	if [[ -n "${tail_pid:-}" ]] && ! kill -0 "${tail_pid}" 2>/dev/null; then
		echo "WARNING: tail process stopped unexpectedly, restarting"
		wait "${tail_pid}" 2>/dev/null || true
		tail_pid=""
		current_log=""
	fi

	sleep 10
done
