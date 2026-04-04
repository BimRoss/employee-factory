#!/usr/bin/env bash
set -euo pipefail

# Apply employee-factory Kubernetes runtime secret from a local .env (kubectl apply).
# Direction: local .env → cluster Secret only. Does not read the cluster or modify .env.
#
# Secret SECRET_NAME (default employee-factory-<EMPLOYEE_ID>-runtime)
# Keys: LLM_API_KEY, SLACK_BOT_TOKEN, SLACK_APP_TOKEN; optional SLACK_USER_TOKEN,
# MULTIAGENT_BOT_USER_IDS, LLM_MODEL, Joanne Google OAuth keys, and Ross ops proxy keys.
# LLM key resolution (first non-empty): LLM_API_KEY, OPENROUTER_API_KEY, OPENROUTER_KEY, then
# {ID}_OPENROUTER_API_KEY / {ID}_OPENROUTER_KEY / {ID}_CHUTES_KEY, ALEX_* fallbacks, {ID}_MODEL, and {ID}_SLACK_*.
#
# Usage:
#   ./scripts/update-runtime-secrets.sh
#   EMPLOYEE_ID=tim ./scripts/update-runtime-secrets.sh
#   ENV_FILE=/path/.env NAMESPACE=employee-factory ./scripts/update-runtime-secrets.sh
#
# Kubeconfig: if KUBECONFIG is unset, defaults to ~/.kube/config/admin.yaml when present.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

resolve_bot_user_id() {
  local token="$1"
  if [[ -z "${token}" ]]; then
    return 1
  fi
  python3 - "${token}" <<'PY'
import json
import sys
import urllib.request

token = sys.argv[1]
req = urllib.request.Request(
    "https://slack.com/api/auth.test",
    method="POST",
    headers={"Authorization": f"Bearer {token}"},
)
try:
    with urllib.request.urlopen(req, timeout=10) as resp:
        payload = json.loads(resp.read().decode())
except Exception:
    sys.exit(1)

if not payload.get("ok"):
    sys.exit(1)

uid = (payload.get("user_id") or "").strip()
if not uid:
    sys.exit(1)

print(uid)
PY
}

build_multiagent_bot_user_ids() {
  local order_raw="${MULTIAGENT_ORDER:-ross,tim,alex,garth,joanne}"
  local -a pairs=()
  local part key token_var token user_id

  IFS=',' read -r -a _order <<< "${order_raw}"
  for part in "${_order[@]}"; do
    key="$(echo "${part}" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
    if [[ -z "${key}" ]]; then
      continue
    fi
    token_var="$(echo "${key}" | tr '[:lower:]-' '[:upper:]_')_SLACK_BOT_TOKEN"
    token="${!token_var:-}"
    if [[ -z "${token}" ]]; then
      echo "warning: cannot auto-resolve ${key} bot id; missing ${token_var}" >&2
      return 1
    fi
    if ! user_id="$(resolve_bot_user_id "${token}")"; then
      echo "warning: cannot auto-resolve ${key} bot id from ${token_var}" >&2
      return 1
    fi
    pairs+=("${key}:${user_id}")
  done

  if [[ "${#pairs[@]}" -eq 0 ]]; then
    return 1
  fi

  local joined
  joined="$(IFS=,; echo "${pairs[*]}")"
  echo "${joined}"
}

if [[ -z "${KUBECONFIG:-}" ]]; then
  _bimross_kube="${HOME}/.kube/config/admin.yaml"
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
OPENROUTER_API_VAR="${EMP_PREFIX}_OPENROUTER_API_KEY"
OPENROUTER_KEY_VAR="${EMP_PREFIX}_OPENROUTER_KEY"
CHUTES_VAR="${EMP_PREFIX}_CHUTES_KEY"
BOT_VAR="${EMP_PREFIX}_SLACK_BOT_TOKEN"
APP_VAR="${EMP_PREFIX}_SLACK_APP_TOKEN"
USER_VAR="${EMP_PREFIX}_SLACK_USER_TOKEN"
GOOGLE_CLIENT_ID_VAR="${EMP_PREFIX}_GOOGLE_CLIENT_ID"
GOOGLE_CLIENT_SECRET_VAR="${EMP_PREFIX}_GOOGLE_CLIENT_SECRET"
GOOGLE_REFRESH_TOKEN_VAR="${EMP_PREFIX}_GOOGLE_REFRESH_TOKEN"
GOOGLE_SENDER_EMAIL_VAR="${EMP_PREFIX}_GOOGLE_SENDER_EMAIL"
GOOGLE_SENDER_NAME_VAR="${EMP_PREFIX}_GOOGLE_SENDER_NAME"
JOANNE_EMAIL_ENABLED_VAR="${EMP_PREFIX}_EMAIL_ENABLED"
JOANNE_GOOGLE_DOCS_ENABLED_VAR="${EMP_PREFIX}_GOOGLE_DOCS_ENABLED"
ROSS_OPS_ENABLED_VAR="${EMP_PREFIX}_OPS_ENABLED"
ROSS_OPS_LOG_ONLY_VAR="${EMP_PREFIX}_OPS_LOG_ONLY"
ROSS_OPS_PROXY_URL_VAR="${EMP_PREFIX}_OPS_PROXY_URL"
ROSS_OPS_PROXY_TOKEN_VAR="${EMP_PREFIX}_OPS_PROXY_TOKEN"
ROSS_OPS_DEFAULT_NAMESPACE_VAR="${EMP_PREFIX}_OPS_DEFAULT_NAMESPACE"
ROSS_OPS_ALLOWED_NAMESPACES_VAR="${EMP_PREFIX}_OPS_ALLOWED_NAMESPACES"
ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAR="${EMP_PREFIX}_OPS_ALLOWED_REDIS_PREFIXES"
OPS_PROXY_WAITLIST_PREFIXES_VAR="${EMP_PREFIX}_OPS_WAITLIST_PREFIXES"

LLM_KEY="${LLM_API_KEY:-}"
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${OPENROUTER_API_KEY:-}"
fi
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${OPENROUTER_KEY:-}"
fi
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${!OPENROUTER_API_VAR:-}"
fi
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${!OPENROUTER_KEY_VAR:-}"
fi
if [[ -z "${LLM_KEY}" ]]; then
  LLM_KEY="${!CHUTES_VAR:-}"
fi
if [[ -z "${LLM_KEY}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  LLM_KEY="${ALEX_OPENROUTER_API_KEY:-}"
fi
if [[ -z "${LLM_KEY}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  LLM_KEY="${ALEX_OPENROUTER_KEY:-}"
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

USER="${SLACK_USER_TOKEN:-}"
if [[ -z "${USER}" ]]; then
  USER="${!USER_VAR:-}"
fi
if [[ -z "${USER}" && "${EMPLOYEE_ID}" == "alex" ]]; then
  USER="${ALEX_SLACK_USER_TOKEN:-}"
fi

JOANNE_EMAIL_ENABLED_VAL="${JOANNE_EMAIL_ENABLED:-}"
if [[ -z "${JOANNE_EMAIL_ENABLED_VAL}" ]]; then
  JOANNE_EMAIL_ENABLED_VAL="${!JOANNE_EMAIL_ENABLED_VAR:-}"
fi
JOANNE_GOOGLE_DOCS_ENABLED_VAL="${JOANNE_GOOGLE_DOCS_ENABLED:-}"
if [[ -z "${JOANNE_GOOGLE_DOCS_ENABLED_VAL}" ]]; then
  JOANNE_GOOGLE_DOCS_ENABLED_VAL="${!JOANNE_GOOGLE_DOCS_ENABLED_VAR:-}"
fi
GOOGLE_CLIENT_ID_VAL="${GOOGLE_CLIENT_ID:-}"
if [[ -z "${GOOGLE_CLIENT_ID_VAL}" ]]; then
  GOOGLE_CLIENT_ID_VAL="${!GOOGLE_CLIENT_ID_VAR:-}"
fi
GOOGLE_CLIENT_SECRET_VAL="${GOOGLE_CLIENT_SECRET:-}"
if [[ -z "${GOOGLE_CLIENT_SECRET_VAL}" ]]; then
  GOOGLE_CLIENT_SECRET_VAL="${!GOOGLE_CLIENT_SECRET_VAR:-}"
fi
GOOGLE_REFRESH_TOKEN_VAL="${GOOGLE_REFRESH_TOKEN:-}"
if [[ -z "${GOOGLE_REFRESH_TOKEN_VAL}" ]]; then
  GOOGLE_REFRESH_TOKEN_VAL="${!GOOGLE_REFRESH_TOKEN_VAR:-}"
fi
GOOGLE_SENDER_EMAIL_VAL="${GOOGLE_SENDER_EMAIL:-}"
if [[ -z "${GOOGLE_SENDER_EMAIL_VAL}" ]]; then
  GOOGLE_SENDER_EMAIL_VAL="${!GOOGLE_SENDER_EMAIL_VAR:-}"
fi
GOOGLE_SENDER_NAME_VAL="${GOOGLE_SENDER_NAME:-}"
if [[ -z "${GOOGLE_SENDER_NAME_VAL}" ]]; then
  GOOGLE_SENDER_NAME_VAL="${!GOOGLE_SENDER_NAME_VAR:-}"
fi
ROSS_OPS_ENABLED_VAL="${ROSS_OPS_ENABLED:-}"
if [[ -z "${ROSS_OPS_ENABLED_VAL}" ]]; then
  ROSS_OPS_ENABLED_VAL="${!ROSS_OPS_ENABLED_VAR:-}"
fi
ROSS_OPS_LOG_ONLY_VAL="${ROSS_OPS_LOG_ONLY:-}"
if [[ -z "${ROSS_OPS_LOG_ONLY_VAL}" ]]; then
  ROSS_OPS_LOG_ONLY_VAL="${!ROSS_OPS_LOG_ONLY_VAR:-}"
fi
ROSS_OPS_PROXY_URL_VAL="${ROSS_OPS_PROXY_URL:-}"
if [[ -z "${ROSS_OPS_PROXY_URL_VAL}" ]]; then
  ROSS_OPS_PROXY_URL_VAL="${!ROSS_OPS_PROXY_URL_VAR:-}"
fi
ROSS_OPS_PROXY_TOKEN_VAL="${ROSS_OPS_PROXY_TOKEN:-}"
if [[ -z "${ROSS_OPS_PROXY_TOKEN_VAL}" ]]; then
  ROSS_OPS_PROXY_TOKEN_VAL="${!ROSS_OPS_PROXY_TOKEN_VAR:-}"
fi
ROSS_OPS_DEFAULT_NAMESPACE_VAL="${ROSS_OPS_DEFAULT_NAMESPACE:-}"
if [[ -z "${ROSS_OPS_DEFAULT_NAMESPACE_VAL}" ]]; then
  ROSS_OPS_DEFAULT_NAMESPACE_VAL="${!ROSS_OPS_DEFAULT_NAMESPACE_VAR:-}"
fi
ROSS_OPS_ALLOWED_NAMESPACES_VAL="${ROSS_OPS_ALLOWED_NAMESPACES:-}"
if [[ -z "${ROSS_OPS_ALLOWED_NAMESPACES_VAL}" ]]; then
  ROSS_OPS_ALLOWED_NAMESPACES_VAL="${!ROSS_OPS_ALLOWED_NAMESPACES_VAR:-}"
fi
ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAL="${ROSS_OPS_ALLOWED_REDIS_PREFIXES:-}"
if [[ -z "${ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAL}" ]]; then
  ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAL="${!ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAR:-}"
fi
OPS_PROXY_WAITLIST_PREFIXES_VAL="${OPS_PROXY_WAITLIST_PREFIXES:-}"
if [[ -z "${OPS_PROXY_WAITLIST_PREFIXES_VAL}" ]]; then
  OPS_PROXY_WAITLIST_PREFIXES_VAL="${!OPS_PROXY_WAITLIST_PREFIXES_VAR:-}"
fi

if [[ -z "${LLM_KEY}" || -z "${BOT}" || -z "${APP}" ]]; then
  echo "need LLM_API_KEY, OPENROUTER_API_KEY, or OPENROUTER_KEY (or ${EMP_PREFIX}_OPENROUTER_* / ${EMP_PREFIX}_CHUTES_KEY), and Slack tokens (${EMP_PREFIX}_SLACK_* or SLACK_*)" >&2
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
if [[ -n "${USER}" ]]; then
  secret_args+=(--from-literal=SLACK_USER_TOKEN="${USER}")
fi
if [[ -n "${JOANNE_EMAIL_ENABLED_VAL}" ]]; then
  secret_args+=(--from-literal=JOANNE_EMAIL_ENABLED="${JOANNE_EMAIL_ENABLED_VAL}")
fi
if [[ -n "${JOANNE_GOOGLE_DOCS_ENABLED_VAL}" ]]; then
  secret_args+=(--from-literal=JOANNE_GOOGLE_DOCS_ENABLED="${JOANNE_GOOGLE_DOCS_ENABLED_VAL}")
fi
if [[ -n "${GOOGLE_CLIENT_ID_VAL}" ]]; then
  secret_args+=(--from-literal=GOOGLE_CLIENT_ID="${GOOGLE_CLIENT_ID_VAL}")
fi
if [[ -n "${GOOGLE_CLIENT_SECRET_VAL}" ]]; then
  secret_args+=(--from-literal=GOOGLE_CLIENT_SECRET="${GOOGLE_CLIENT_SECRET_VAL}")
fi
if [[ -n "${GOOGLE_REFRESH_TOKEN_VAL}" ]]; then
  secret_args+=(--from-literal=GOOGLE_REFRESH_TOKEN="${GOOGLE_REFRESH_TOKEN_VAL}")
fi
if [[ -n "${GOOGLE_SENDER_EMAIL_VAL}" ]]; then
  secret_args+=(--from-literal=GOOGLE_SENDER_EMAIL="${GOOGLE_SENDER_EMAIL_VAL}")
fi
if [[ -n "${GOOGLE_SENDER_NAME_VAL}" ]]; then
  secret_args+=(--from-literal=GOOGLE_SENDER_NAME="${GOOGLE_SENDER_NAME_VAL}")
fi
if [[ -n "${ROSS_OPS_ENABLED_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_ENABLED="${ROSS_OPS_ENABLED_VAL}")
fi
if [[ -n "${ROSS_OPS_LOG_ONLY_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_LOG_ONLY="${ROSS_OPS_LOG_ONLY_VAL}")
fi
if [[ -n "${ROSS_OPS_PROXY_URL_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_PROXY_URL="${ROSS_OPS_PROXY_URL_VAL}")
fi
if [[ -n "${ROSS_OPS_PROXY_TOKEN_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_PROXY_TOKEN="${ROSS_OPS_PROXY_TOKEN_VAL}")
  secret_args+=(--from-literal=OPS_PROXY_AUTH_TOKEN="${ROSS_OPS_PROXY_TOKEN_VAL}")
fi
if [[ -n "${ROSS_OPS_DEFAULT_NAMESPACE_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_DEFAULT_NAMESPACE="${ROSS_OPS_DEFAULT_NAMESPACE_VAL}")
fi
if [[ -n "${ROSS_OPS_ALLOWED_NAMESPACES_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_ALLOWED_NAMESPACES="${ROSS_OPS_ALLOWED_NAMESPACES_VAL}")
fi
if [[ -n "${ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAL}" ]]; then
  secret_args+=(--from-literal=ROSS_OPS_ALLOWED_REDIS_PREFIXES="${ROSS_OPS_ALLOWED_REDIS_PREFIXES_VAL}")
fi
if [[ -n "${OPS_PROXY_WAITLIST_PREFIXES_VAL}" ]]; then
  secret_args+=(--from-literal=OPS_PROXY_WAITLIST_PREFIXES="${OPS_PROXY_WAITLIST_PREFIXES_VAL}")
fi

MULTIAGENT_IDS="${MULTIAGENT_BOT_USER_IDS:-}"
if [[ -z "${MULTIAGENT_IDS}" ]]; then
  MULTIAGENT_IDS="$(build_multiagent_bot_user_ids || true)"
fi
if [[ -n "${MULTIAGENT_IDS}" ]]; then
  secret_args+=(--from-literal=MULTIAGENT_BOT_USER_IDS="${MULTIAGENT_IDS}")
fi

kubectl create secret generic "${SECRET_NAME}" \
  "${secret_args[@]}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "applied secret ${SECRET_NAME} in ${NAMESPACE}"
