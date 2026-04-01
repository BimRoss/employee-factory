#!/usr/bin/env bash
set -euo pipefail

# Apply runtime secrets for every employee from a shared .env file.
# Usage:
#   ./scripts/update-all-runtime-secrets.sh
#   ENV_FILE=/path/.env ./scripts/update-all-runtime-secrets.sh
#   NAMESPACE=employee-factory ./scripts/update-all-runtime-secrets.sh

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${ROOT}/.env}"
NAMESPACE="${NAMESPACE:-employee-factory}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

for employee in alex tim ross garth; do
  echo "==> syncing ${employee}"
  ENV_FILE="${ENV_FILE}" NAMESPACE="${NAMESPACE}" EMPLOYEE_ID="${employee}" \
    "${ROOT}/scripts/update-runtime-secrets.sh"
done

echo "synced all employee runtime secrets in ${NAMESPACE}"
