# employee-factory

Go + [Cogito](https://github.com/mudler/cogito) workers that act as **BimRoss “employees”**: Slack (Socket Mode), pluggable OpenAI-compatible LLMs (e.g. **Chutes**), and personas rendered from [`cursor-rules`](../cursor-rules) (`personas/<id>.manifest` + `scripts/render-employee-persona.py`).

## Local run

1. Copy `.env.example` to `.env` and set `ALEX_CHUTES_KEY`, Slack tokens, and `LLM_MODEL`.
2. Create a local persona file (e.g. `persona.local.md`) and set `PERSONA_PATH`.
3. `go run ./cmd/employee-factory`

Health: `GET /health`, `GET /readyz` on `HTTP_ADDR` (default `:8080`).

## Kubernetes

Manifests live in [`rancher-admin`](../rancher-admin/admin/apps/employee-factory/):

- Apply namespace, config, persona ConfigMap, NetworkPolicy, RBAC, Deployment, Service, CronJob.
- Create `employee-factory-alex-runtime` secret (LLM + Slack keys). Use `scripts/push-runtime-secrets.sh`. If `KUBECONFIG` is unset, the script defaults to `~/.kube/config/grant-admin.yaml` when present (so a directory-only `~/.kube/config` layout still works). Override with `KUBECONFIG=/path/to/kubeconfig` if needed.
- Create `employee-factory-persona-sync-git` secret with key `GITHUB_TOKEN` (read access to `bimross/cursor-rules`).

## CI/CD

Workflow `.github/workflows/employee-factory-images.yml` builds and pushes:

- `geeemoney/employee-factory`
- `geeemoney/employee-factory-persona-sync`

On `v*` tags, **`gitops-release`** bumps image tags in `bimross/rancher-admin` (requires `RANCHER_ADMIN_REPO_TOKEN` and Docker Hub secrets).

## Environment resolution

The binary accepts **canonical** env vars (`LLM_API_KEY`, `SLACK_*`) or **Alex-local** names (`ALEX_CHUTES_KEY`, etc.). See `internal/config`.
