# employee-factory

**BimRoss treats the company as code.** This is the **Slack worker**: the process that turns a persona (from [`cursor-rules`](https://github.com/bimross/cursor-rules)) and a Slack app (from [`slack-factory`](https://github.com/bimross/slack-factory)) into something that actually answers in Slack.

The repo is the source of truth for behavior—read the code and `.env.example` if you are running or extending it.

## LLM resilience (Chutes)

Chat completions retry on transient provider errors (`429`, `502`, `503`, and Chutes “no instances available”) with exponential backoff, then optionally try `LLM_FALLBACK_MODEL` on the same `LLM_BASE_URL` and API key (typically a smaller model). Configure via `LLM_MAX_RETRIES`, `LLM_RETRY_BACKOFF_MS`, and `LLM_FALLBACK_MODEL` (see `.env.example`). No extra secrets.

## Multi-agent Slack (`<!everyone>`)

For `@everyone` triggers (no individual bot `@mention`), each squad pod receives the same Events API message and runs a shared **multi-agent session**:

- **Turn order** is a **pseudorandom permutation** of `MULTIAGENT_ORDER`, deterministic per trigger from `SHA-256(anchor message timestamp + NUL + comma-joined order + NUL + optional secret)`. Every pod must use the same `MULTIAGENT_ORDER` and the same optional `MULTIAGENT_SHUFFLE_SECRET` so they agree on who posts first, second, etc.
- **Coordination** is **not** Redis: each bot polls `conversations.history` until prior squad messages match the expected slot prefix, then calls the LLM and posts (same as before).
- **`MULTIAGENT_BROADCAST_ROUNDS`** (default `1`) is how many full passes over that shuffled order to run per trigger (`1` ⇒ each agent replies once).
- **`MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY`** (default `0.5`) controls per-reply chance to include exactly one other-agent `@mention`, which creates organic follow-on turns without hardcoding message counts.
- **Deterministic branch mode**: with `MULTIAGENT_BROADCAST_BRANCHING_ENABLED=true`, each `<!everyone>` trigger deterministically flips into branch mode using `MULTIAGENT_BROADCAST_BRANCHING_PROBABILITY` (default `0.5`). Branch mode uses `MULTIAGENT_BROADCAST_BRANCHING_HANDOFF_PROBABILITY` (default `1.0`) to produce richer cross-agent follow-ons while keeping ordering stable across pods.

### Turn quality policy (runtime-enforced prompt block)

During multi-agent sessions, each slot now receives an explicit policy block before LLM generation:

- **Role lanes** are reinforced per employee (`ross` execution/risk, `alex` GTM/revenue, `tim` tradeoffs/experiments, `garth` synthesis/checklists).
- **Novelty guard**: each agent is instructed to add one *new* angle instead of repeating previous bot lines.
- **Single closer pattern**: non-final slots are instructed not to provide the final merged answer; the final slot is instructed to provide the closing recommendation.
- **Close without ping-pong**: final slot suppresses handoff mentions by forcing handoff probability to `0` for that slot only.

### Never-sliced outbound safety

Before posting, replies pass a completion gate that checks for:

- likely clipped tails
- prompt/rule artifact leakage (for example internal labels/slugs)

If flagged, the bot runs one repair rewrite pass and only then posts. If repair still fails, it posts a short complete fallback sentence instead of an empty or broken message.

## Deploying to the admin cluster (Fleet)

Phase 1 adds only ConfigMap keys; runtime Secrets stay the same.

1. Merge this repo to the default branch and cut a semver tag so CI builds and pushes `geeemoney/employee-factory:<version>`.
2. In [`rancher-admin`](https://github.com/bimross/rancher-admin), bump the `geeemoney/employee-factory` image tag on all four deployments under `admin/apps/employee-factory/` (`alex`, `tim`, `ross`, `garth`) to match the new tag.
3. Commit to `rancher-admin` `master`; Fleet applies the new image and the updated `employee-factory-config` (retry/fallback env) from the same repo.

## Runtime secret sync (kubectl path)

- `scripts/update-runtime-secrets.sh` syncs one employee runtime Secret from local `.env` (`EMPLOYEE_ID=alex|tim|ross|garth`).
- `scripts/update-all-runtime-secrets.sh` syncs all four runtime Secrets in one pass.
- Synced keys include `LLM_API_KEY`, `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, optional `SLACK_USER_TOKEN`, optional `LLM_MODEL`, and `MULTIAGENT_BOT_USER_IDS`.
- `MULTIAGENT_BOT_USER_IDS` auto-resolves from each bot token via Slack `auth.test` (order from `MULTIAGENT_ORDER`, default `ross,tim,alex,garth`) unless explicitly provided.
