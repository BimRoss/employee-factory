#!/usr/bin/env bash
set -euo pipefail

# Apply employee-factory Kubernetes runtime secret from a local .env (kubectl apply).
#
# Secret SECRET_NAME (default employee-factory-<EMPLOYEE_ID>-runtime)
# Keys: LLM_API_KEY, SLACK_BOT_TOKEN, SLACK_APP_TOKEN; optional LLM_MODEL
# With EMPLOYEE_ID set, also reads {ID}_CHUTES_KEY, {ID}_MODEL, and {ID}_SLACK_* (e.g. GARTH_CHUTES_KEY).
#
# Usage:
#   ./scripts/update-runtime-secrets.sh
#   EMPLOYEE_ID=tim ./scripts/update-runtime-secrets.sh
#   ENV_FILE=/path/.env NAMESPACE=employee-factory ./scripts/update-runtime-secrets.sh
#
# Kubeconfig: if KUBECONFIG is unset, defaults to ~/.kube/config/grant-admin.yaml when present.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -z "${KUBECONFIG:-}" ]]; then
  _bimross_kube="${HOME}/.kube/config/grant-admin.yaml"
  if [[ -f "${_bimross_kube}" ]]; then
    export KUBECONFIG="${_bimross_kube}"
  fi
fi

ENV_FILE="${ENV_FILE:-${ROOT}/.env}"
NAMESPACE="${NAMESPACE:-employee-factory}"
# Preserve target employee if set before sourcing .env (CLI overrides .env's EMPLOYEE_ID).
_cli_employee_set=
if [[ -n "${EMPLOYEE_ID+x}" ]]; then
  _cli_employee_set=1
  _cli_employee="${EMPLOYEE_ID}"
fi
_cli_secret_set=
if [[ -n "${SECRET_NAME+x}" ]]; then
  _cli_secret_set=1
  _cli_secret="${SECRET_NAME}"
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a

if [[ -n "${_cli_employee_set}" ]]; then
  EMPLOYEE_ID="${_cli_employee}"
else
  EMPLOYEE_ID="${EMPLOYEE_ID:-alex}"
fi
if [[ -n "${_cli_secret_set}" ]]; then
  SECRET_NAME="${_cli_secret}"
else
  SECRET_NAME="employee-factory-${EMPLOYEE_ID}-runtime"
fi

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

EMP_PREFIX="$(echo "${EMPLOYEE_ID}" | tr '[:lower:]-' '[:upper:]_')"
CHUTES_VAR="${EMP_PREFIX}_CHUTES_KEY"
BOT_VAR="${EMP_PREFIX}_SLACK_BOT_TOKEN"
APP_VAR="${EMP_PREFIX}_SLACK_APP_TOKEN"

LLM_KEY="${LLM_API_KEY:-}"
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${!CHUTES_VAR:-}"
fi
if [[ -z "${LLM_KEY}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  LLM_KEY="${ALEX_CHUTES_KEY:-}"
fi

BOT="${SLACK_BOT_TOKEN:-}"
if [[ -z "${BOT}" ]]; then
  BOT="${!BOT_VAR:-}"
fi
if [[ -z "${BOT}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  BOT="${ALEX_SLACK_BOT_TOKEN:-}"
fi

APP="${SLACK_APP_TOKEN:-}"
if [[ -z "${APP}" ]]; then
  APP="${!APP_VAR:-}"
fi
if [[ -z "${APP}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  APP="${ALEX_SLACK_APP_TOKEN:-}"
fi

if [[ -z "${LLM_KEY}" || -z "${BOT}" || -z "${APP}" ]]; then
  echo "need LLM_API_KEY or ${EMP_PREFIX}_CHUTES_KEY, and Slack tokens (${EMP_PREFIX}_SLACK_* or SLACK_*)" >&2
  exit 1
fi

EMP_MODEL_VAR="$(echo "${EMPLOYEE_ID}" | tr '[:lower:]-' '[:upper:]_')_MODEL"
MODEL_VAL="${LLM_MODEL:-}"
if [[ -z "${MODEL_VAL}" ]]; then
  MODEL_VAL="${!EMP_MODEL_VAR:-}"
fi

secret_args=(
  --namespace "${NAMESPACE}"
  --from-literal=LLM_API_KEY="${LLM_KEY}"
  --from-literal=SLACK_BOT_TOKEN="${BOT}"
  --from-literal=SLACK_APP_TOKEN="${APP}"
)
if [[ -n "${MODEL_VAL}" ]]; then
  secret_args+=(--from-literal=LLM_MODEL="${MODEL_VAL}")
fi

kubectl create secret generic "${SECRET_NAME}" \
  "${secret_args[@]}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "applied secret ${SECRET_NAME} in ${NAMESPACE}"
