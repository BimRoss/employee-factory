#!/usr/bin/env bash
set -euo pipefail

# Apply runtime Secret with canonical keys from a local .env file.
# Usage:
#   ENV_FILE=.env NAMESPACE=employee-factory SECRET_NAME=employee-factory-alex-runtime ./scripts/push-runtime-secrets.sh
#
# After sourcing .env, uses:
#   LLM_API_KEY, SLACK_BOT_TOKEN, SLACK_APP_TOKEN
# or Alex-local fallbacks:
#   ALEX_CHUTES_KEY, ALEX_SLACK_BOT_TOKEN, ALEX_SLACK_APP_TOKEN
#
# Requires: kubectl configured for the target cluster.
#
# Kubeconfig: if KUBECONFIG is unset, defaults to ~/.kube/config/grant-admin.yaml when
# that file exists (BimRoss layout: ~/.kube/config may be a directory of split configs).

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

LLM_KEY="${LLM_API_KEY:-${ALEX_CHUTES_KEY:-}}"
BOT="${SLACK_BOT_TOKEN:-${ALEX_SLACK_BOT_TOKEN:-}}"
APP="${SLACK_APP_TOKEN:-${ALEX_SLACK_APP_TOKEN:-}}"

if [[ -z "${LLM_KEY}" || -z "${BOT}" || -z "${APP}" ]]; then
  echo "need LLM_API_KEY (or ALEX_CHUTES_KEY) and Slack tokens" >&2
  exit 1
fi

kubectl create secret generic "${SECRET_NAME}" \
  --namespace "${NAMESPACE}" \
  --from-literal=LLM_API_KEY="${LLM_KEY}" \
  --from-literal=SLACK_BOT_TOKEN="${BOT}" \
  --from-literal=SLACK_APP_TOKEN="${APP}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "applied secret ${SECRET_NAME} in ${NAMESPACE}"
