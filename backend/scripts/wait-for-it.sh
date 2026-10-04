#!/usr/bin/env bash
set -e

HOSTPORT="$1"
shift
CMD=("$@")

HOST="${HOSTPORT%%:*}"
PORT="${HOSTPORT##*:}"

echo "Waiting for ${HOST}:${PORT}..."
until (echo > /dev/tcp/"${HOST}"/"${PORT}") >/dev/null 2>&1; do
  sleep 1
done

echo "${HOST}:${PORT} is available - starting service"
exec "${CMD[@]}"
