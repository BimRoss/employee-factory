#!/usr/bin/env bash
set -euo pipefail

# Canonical name for BimRoss "push runtime secrets to admin cluster" scripts.
# Delegates to update-runtime-secrets.sh (employee-factory Slack + LLM keys).

exec "$(cd "$(dirname "$0")" && pwd)/update-runtime-secrets.sh" "$@"
