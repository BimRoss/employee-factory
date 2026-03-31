#!/usr/bin/env bash
set -euo pipefail

# Apply employee-factory Kubernetes runtime secret from a local .env (kubectl apply).
#
# Secret SECRET_NAME (default employee-factory-alex-runtime)
# Keys: LLM_API_KEY, SLACK_BOT_TOKEN, SLACK_APP_TOKEN; optional LLM_MODEL
#
# Usage:
#   ./scripts/update-runtime-secrets.sh
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
SECRET_NAME="${SECRET_NAME:-employee-factory-alex-runtime}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

LLM_KEY="${LLM_API_KEY:-${ALEX_CHUTES_KEY:-}}"
BOT="${SLACK_BOT_TOKEN:-${ALEX_SLACK_BOT_TOKEN:-}}"
APP="${SLACK_APP_TOKEN:-${ALEX_SLACK_APP_TOKEN:-}}"

if [[ -z "${LLM_KEY}" || -z "${BOT}" || -z "${APP}" ]]; then
  echo "need LLM_API_KEY (or ALEX_CHUTES_KEY) and Slack tokens for runtime secret" >&2
  exit 1
fi

EMPLOYEE_ID="${EMPLOYEE_ID:-alex}"
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
