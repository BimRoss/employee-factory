#!/usr/bin/env bash
set -euo pipefail

# Apply employee-factory Kubernetes secrets from a local .env (kubectl apply).
#
# 1) Runtime: Secret SECRET_NAME (default employee-factory-alex-runtime)
#    Keys: LLM_API_KEY, SLACK_BOT_TOKEN, SLACK_APP_TOKEN; optional LLM_MODEL
# 2) Persona sync: Secret GIT_SECRET_NAME (default employee-factory-persona-sync-git)
#    Key: GITHUB_TOKEN (from your .env — see below)
#
# Usage:
#   ./scripts/update-runtime-secrets.sh
#   ENV_FILE=/path/.env NAMESPACE=employee-factory ./scripts/update-runtime-secrets.sh
#
# .env variables:
#   Required for runtime: LLM_API_KEY or ALEX_CHUTES_KEY; SLACK_* or ALEX_SLACK_*
#   Optional model: LLM_MODEL or {EMPLOYEE_ID}_MODEL (e.g. ALEX_MODEL)
#   Persona sync (optional): CURSOR_RULES_GITHUB_TOKEN (preferred) or GITHUB_TOKEN
#     -> stored in-cluster as secret ...-persona-sync-git, key GITHUB_TOKEN (CronJob expects this)
#
# Flags (optional):
#   SKIP_RUNTIME_SECRET=1   only apply git secret
#   SKIP_GIT_SECRET=1       only apply runtime secret
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
GIT_SECRET_NAME="${GIT_SECRET_NAME:-employee-factory-persona-sync-git}"
SKIP_RUNTIME_SECRET="${SKIP_RUNTIME_SECRET:-0}"
SKIP_GIT_SECRET="${SKIP_GIT_SECRET:-0}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

if [[ "${SKIP_RUNTIME_SECRET}" != "1" ]]; then
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
fi

if [[ "${SKIP_GIT_SECRET}" != "1" ]]; then
  GIT_TOKEN="${CURSOR_RULES_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"
  if [[ -z "${GIT_TOKEN}" ]]; then
    echo "note: CURSOR_RULES_GITHUB_TOKEN (or GITHUB_TOKEN) unset; skipped ${GIT_SECRET_NAME}" >&2
  else
    kubectl create secret generic "${GIT_SECRET_NAME}" \
      --namespace "${NAMESPACE}" \
      --from-literal=GITHUB_TOKEN="${GIT_TOKEN}" \
      --dry-run=client -o yaml | kubectl apply -f -

    echo "applied secret ${GIT_SECRET_NAME} (GITHUB_TOKEN) in ${NAMESPACE}"
  fi
fi
