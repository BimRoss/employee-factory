#!/usr/bin/env bash
set -euo pipefail

# Clones cursor-rules, renders Alex persona, applies ConfigMap, restarts deployment.
# Required env: GITHUB_TOKEN, NAMESPACE (default employee-factory), EMPLOYEE (default alex)
# Optional: CURSOR_RULES_REPO (default https://github.com/bimross/cursor-rules.git)

NAMESPACE="${NAMESPACE:-employee-factory}"
EMPLOYEE="${EMPLOYEE:-alex}"
REPO="${CURSOR_RULES_REPO:-https://github.com/bimross/cursor-rules.git}"
WORKDIR="${WORKDIR:-/tmp/cursor-rules}"
OUT="/tmp/${EMPLOYEE}.persona.md"
CM_NAME="employee-factory-${EMPLOYEE}-persona"
DEPLOY_NAME="employee-factory-${EMPLOYEE}"

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN is required" >&2
  exit 1
fi

rm -rf "${WORKDIR}"
CLONE_URL="${REPO/https:\/\//https:\/\/x-access-token:${GITHUB_TOKEN}@}"
git clone --depth 1 "${CLONE_URL}" "${WORKDIR}"

python3 "${WORKDIR}/scripts/render-employee-persona.py" \
  --repo-root "${WORKDIR}" \
  --employee "${EMPLOYEE}" \
  -o "${OUT}"

SHA="$(git -C "${WORKDIR}" rev-parse HEAD)"
kubectl -n "${NAMESPACE}" create configmap "${CM_NAME}" \
  --from-file="persona.md=${OUT}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NAMESPACE}" annotate configmap "${CM_NAME}" \
  "employee-factory/cursor-rules-sha=${SHA}" \
  "employee-factory/synced-at=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --overwrite

kubectl -n "${NAMESPACE}" rollout restart "deployment/${DEPLOY_NAME}" || true

echo "persona sync complete sha=${SHA}"
