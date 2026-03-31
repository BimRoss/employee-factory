# employee-factory

Go + [Cogito](https://github.com/mudler/cogito) workers that act as **BimRoss “employees”**: Slack (Socket Mode), pluggable OpenAI-compatible LLMs (e.g. **Chutes**), and personas rendered from [`cursor-rules`](../cursor-rules) by `scripts/render-employee-persona.py`, which concatenates **`.cursor/rules/{employee}-*.mdc`** (e.g. all `alex-*.mdc` when `EMPLOYEE=alex`). See [`cursor-rules/personas/README.md`](../cursor-rules/personas/README.md).

## Local run

1. Copy `.env.example` to `.env` and set `ALEX_CHUTES_KEY` and Slack tokens. Model: `LLM_MODEL` (canonical), or `ALEX_MODEL` / `{EMPLOYEE_ID}_MODEL` for per-employee brains; if all unset, defaults to `unsloth/Llama-3.2-1B-Instruct` on Chutes.
2. Build a local persona from `cursor-rules` (compact Slack-oriented render):  
   `python3 ../cursor-rules/scripts/render-employee-persona.py --repo-root ../cursor-rules --employee alex --compact -o persona.local.md`  
   Then set `PERSONA_PATH` (e.g. `persona.local.md`).
3. `go run ./cmd/employee-factory`

Health: `GET /health`, `GET /readyz` on `HTTP_ADDR` (default `:8080`).

### Slack length, thread context, and models

- Default **`LLM_MAX_TOKENS`** is **1024** (ceiling so replies rarely cut off mid-sentence; brevity comes from the Slack system suffix, not a tiny cap). Lower only if you need hard cost limits.
- **`LLM_TEMPERATURE`** (default `0.55`) and optional **`LLM_TOP_P`** tune sampling.
- The bot appends **Slack reply rules** after `persona.md` via a fixed suffix. Persona text is truncated first if **`LLM_SYSTEM_MAX_RUNES`** is exceeded; the suffix is never dropped.
- In **channels**, a top-level @mention is answered on the **main channel timeline** (not threaded under your message). Replies use a thread only when Slack already sent `thread_ts` (for example you used “Reply in thread”).
- In **threads** (channel or DM), the bot loads recent messages with **`conversations.replies`** (up to **`LLM_THREAD_MAX_MESSAGES`**, trimmed to **`LLM_THREAD_MAX_RUNES`**) and prepends them to the user message—no extra LLM call.
- In **linear DMs / MPIMs** (no `thread_ts`), it loads prior turns with **`conversations.history`** before the current message—same limits and prepended format—so you do not have to thread every reply in a DM.
- For **Alex** (`EMPLOYEE_ID=alex` or empty), optional deterministic **keyword hints** (`LLM_ALEX_HINTS=true`) nudge the model toward the right framework; disable with `LLM_ALEX_HINTS=0` if you want zero hinting.
- **1B** models are cheap but weak at long system prompts. For production, use a stronger **Instruct** model on Chutes ([LLM list](https://chutes.ai/app?type=llm)).

### Persona privacy (production)

- Production persona text comes from the **`geeemoney/cursor-rules`** image (built from [`bimross/cursor-rules`](../cursor-rules): committed **`.cursor/personas/alex-personality.md`**). Do **not** bake in gitignored **`local-context.mdc`**, **`.cursor/rules/private/**`**, or **`.cursor/businesses/**`**—keep private overlays Cursor-only.

### Manual QA

- See [`docs/BASELINE_PROMPTS.md`](docs/BASELINE_PROMPTS.md) for quick before/after prompts when changing models or prompts.

## Kubernetes

Manifests live in [`rancher-admin`](../rancher-admin/admin/apps/employee-factory/):

- Apply namespace, config, NetworkPolicy, Deployment, Service (or rely on `scripts/update-runtime-secrets.sh` to create the namespace).
- **Persona:** the Deployment uses an **initContainer** image **`geeemoney/cursor-rules:<semver>`** that carries [`bimross/cursor-rules`](../cursor-rules). It copies **`.cursor/personas/alex-personality.md`** into a shared volume; the app container reads **`/config/persona.md`** as today. New Alex brain ships when you **release `cursor-rules`** (see that repo’s workflow)—not via a CronJob.
- Secrets: run **`./scripts/update-runtime-secrets.sh`** from a filled-in `.env`. It applies **`employee-factory-alex-runtime`** (LLM + Slack; optional `LLM_MODEL` / `{EMPLOYEE_ID}_MODEL`). If `KUBECONFIG` is unset, the script defaults to `~/.kube/config/grant-admin.yaml` when present.
- **Docker Hub pull**: manifests use **`imagePullSecrets: dockerhub-pull`**. That secret must exist in namespace **`employee-factory`**. From [`rancher-admin`](../rancher-admin), run **`./scripts/sync-employee-factory-pull-secret.sh`** once to copy `dockerhub-pull` from `subnet-signal`. Without it, pods show **ImagePullBackOff** for `geeemoney/*` images.

## CI/CD

Workflow `.github/workflows/employee-factory-images.yml` builds and pushes **`geeemoney/employee-factory`** on `v*` tags.

**GitHub Actions → Secrets** (repository):

| Secret | Purpose |
|--------|---------|
| `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` | Push images on `v*` tags |
| `RANCHER_ADMIN_REPO_TOKEN` | **`gitops-release` only**: clone + push to `bimross/rancher-admin` to bump **`geeemoney/employee-factory`** in `alex-deployment.yaml`. Same pattern as subnet-signal / twitter-worker. **Do not** use `github.token` for that checkout. |

On **`v*`** tags, **`gitops-release`** bumps **`geeemoney/employee-factory`** only. The **`geeemoney/cursor-rules`** image (persona bundle) is released from the **`cursor-rules`** repo.

If you tagged a release **before** this workflow (or `RANCHER_ADMIN_REPO_TOKEN`) existed, **`gitops-release`** may have been skipped or failed—fix secrets, merge workflow to default branch, then **re-run the failed workflow** or tag a new patch (or bump manifests in rancher-admin manually once).

## Environment resolution

The binary accepts **canonical** env vars (`LLM_API_KEY`, `SLACK_*`) or **Alex-local** names (`ALEX_CHUTES_KEY`, etc.). See `internal/config`.
