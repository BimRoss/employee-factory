# employee-factory

Go + [Cogito](https://github.com/mudler/cogito) workers that act as **BimRoss “employees”**: Slack (Socket Mode), pluggable OpenAI-compatible LLMs (e.g. **Chutes**), and personas rendered from [`cursor-rules`](../cursor-rules) (`personas/<id>.manifest` + `scripts/render-employee-persona.py`).

## Local run

1. Copy `.env.example` to `.env` and set `ALEX_CHUTES_KEY` and Slack tokens. Model: `LLM_MODEL` (canonical), or `ALEX_MODEL` / `{EMPLOYEE_ID}_MODEL` for per-employee brains; if all unset, defaults to `unsloth/Llama-3.2-1B-Instruct` on Chutes.
2. Create a local persona file (e.g. `persona.local.md`) and set `PERSONA_PATH`.
3. `go run ./cmd/employee-factory`

Health: `GET /health`, `GET /readyz` on `HTTP_ADDR` (default `:8080`).

## Kubernetes

Manifests live in [`rancher-admin`](../rancher-admin/admin/apps/employee-factory/):

- Apply namespace, config, persona ConfigMap, NetworkPolicy, RBAC, Deployment, Service, CronJob (or rely on `scripts/update-runtime-secrets.sh` to create the namespace).
- Secrets: run **`./scripts/update-runtime-secrets.sh`** from a filled-in `.env`. It applies **`employee-factory-alex-runtime`** (LLM + Slack; optional `LLM_MODEL` / `{EMPLOYEE_ID}_MODEL`) and **`employee-factory-persona-sync-git`** with **`GITHUB_TOKEN`** sourced from **`CURSOR_RULES_GITHUB_TOKEN`** (or `GITHUB_TOKEN`) for the persona CronJob clone of `bimross/cursor-rules`. If `KUBECONFIG` is unset, the script defaults to `~/.kube/config/grant-admin.yaml` when present. Override with `KUBECONFIG=/path/to/kubeconfig` if needed. Use `SKIP_GIT_SECRET=1` or `SKIP_RUNTIME_SECRET=1` to update only one.
- **Docker Hub pull**: manifests use **`imagePullSecrets: dockerhub-pull`**. That secret must exist in namespace **`employee-factory`** (Kubernetes secrets are per-namespace). From [`rancher-admin`](../rancher-admin), run **`./scripts/sync-employee-factory-pull-secret.sh`** once to copy `dockerhub-pull` from `subnet-signal`. Without it, pods show **ImagePullBackOff** for `geeemoney/*` images.

## CI/CD

Workflow `.github/workflows/employee-factory-images.yml` builds and pushes:

- `geeemoney/employee-factory`
- `geeemoney/employee-factory-persona-sync`

**GitHub Actions → Secrets** (repository):

| Secret | Purpose |
|--------|---------|
| `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` | Push images on `v*` tags |
| `RANCHER_ADMIN_REPO_TOKEN` | **`gitops-release` only**: clone + push to `bimross/rancher-admin` to bump image tags under `admin/apps/employee-factory/`. Same pattern as subnet-signal / twitter-worker. **Do not** use `github.token` for that checkout. |

On **`v*`** tags, job **`gitops-release`** runs after both images build, strips the leading `v` for the semver tag (e.g. `v0.0.2` → `0.0.2`), updates `alex-deployment.yaml` and `persona-sync-cronjob.yaml`, commits, and pushes to **`master`** on rancher-admin. Fleet then deploys the new tags.

The **running employee-factory pod** does not talk to the rancher-admin Git repo; only **CI** does. Persona sync uses **`CURSOR_RULES_GITHUB_TOKEN`** (see Kubernetes section) for `cursor-rules`, which is separate.

If you tagged a release **before** this workflow (or `RANCHER_ADMIN_REPO_TOKEN`) existed, **`gitops-release`** may have been skipped or failed—fix secrets, merge workflow to default branch, then **re-run the failed workflow** or tag **`v0.0.3`** (or bump manifests in rancher-admin manually once).

## Environment resolution

The binary accepts **canonical** env vars (`LLM_API_KEY`, `SLACK_*`) or **Alex-local** names (`ALEX_CHUTES_KEY`, etc.). See `internal/config`.
