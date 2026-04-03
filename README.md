# employee-factory

**BimRoss treats the company as code.** This is the **Slack worker**: the process that turns a persona (from [`cursor-rules`](https://github.com/bimross/cursor-rules)) and a Slack app (from [`slack-factory`](https://github.com/bimross/slack-factory)) into something that actually answers in Slack.

The repo is the source of truth for behavior—read the code and `.env.example` if you are running or extending it.

## Company-channel runtime (first pass)

This repo now includes a first-pass contract for "one Slack channel = one company runtime".

- Configure `COMPANY_CHANNELS_JSON` as a JSON array of channel contracts (`company_slug`, `channel_id`, optional metadata).
- Keep `COMPANY_CHANNELS_ENFORCE=false` during migration to preserve existing behavior.
- Set `COMPANY_CHANNELS_ENFORCE=true` to ignore events from channels not declared in `COMPANY_CHANNELS_JSON`.

This is intentionally lightweight in v1: it establishes channel identity + allowlist behavior without forcing a multi-tenant rewrite yet.

## LLM model and resilience (OpenRouter)

Default model is `google/gemini-2.0-flash-001` via OpenRouter (`LLM_BASE_URL` defaults to `https://openrouter.ai/api/v1`; override via `LLM_MODEL` when needed). Chat completions retry on transient provider errors (`429`, `502`, `503`, plus temporary-capacity responses) with exponential backoff. `LLM_FALLBACK_MODEL` is optional and can be left empty for single-model behavior (recommended for this Slack flow). Each completion call is bounded by `LLM_REPLY_TIMEOUT_SEC` so a stalled provider request cannot block Slack event handling indefinitely. Configure via `LLM_MAX_RETRIES`, `LLM_RETRY_BACKOFF_MS`, `LLM_REPLY_TIMEOUT_SEC`, and `LLM_FALLBACK_MODEL` (see `.env.example`).

## Multi-agent Slack (`<!everyone>` / `<!channel>`)

For channel-wide summons (`@everyone` or `@channel`, Slack tokens `<!everyone>` / `<!channel>`), each squad pod receives the same Events API message and runs a shared **multi-agent session**:

- **Turn order** is a **pseudorandom permutation** of `MULTIAGENT_ORDER`, deterministic per trigger from `SHA-256(anchor message timestamp + NUL + comma-joined order + NUL + optional secret)`. Every pod must use the same `MULTIAGENT_ORDER` and the same optional `MULTIAGENT_SHUFFLE_SECRET` so they agree on who posts first, second, etc.
- **Mixed summons precedence:** if a message includes both channel-wide summon + explicit bot mention(s), broadcast wins and the full squad turn runs.
- **Coordination** is **not** Redis: each bot polls `conversations.history` until prior squad messages match the expected slot prefix, then calls the LLM and posts (same as before).
- **`MULTIAGENT_BROADCAST_ROUNDS`** (default `1`) is how many full passes over that shuffled order to run per trigger (`1` ⇒ each agent replies once).
- Per-reply handoff chance is sampled inside `MULTIAGENT_HANDOFF_MIN_PROBABILITY..MULTIAGENT_HANDOFF_MAX_PROBABILITY` (defaults `0.25..0.75`) so cross-agent mentions feel organic.
- **`MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY`** (default `0.35`) controls broadcast mention intensity, but each reply uses bounded randomness before deciding the final `@mention`.
- **Deterministic branch mode**: with `MULTIAGENT_BROADCAST_BRANCHING_ENABLED=true`, each `<!everyone>` trigger deterministically flips into branch mode using `MULTIAGENT_BROADCAST_BRANCHING_PROBABILITY` (default `0.5`). Branch mode uses `MULTIAGENT_BROADCAST_BRANCHING_HANDOFF_PROBABILITY` (default `0.6`) to keep cross-agent follow-ons balanced while preserving stable ordering across pods.

## Plain `#general` auto-reply (single random agent)

When a plain (no-bot-mention, no channel-wide summon) message is posted in `#general`, the squad can auto-reply as one agent:

- Enable with `MULTIAGENT_GENERAL_AUTO_REPLY_ENABLED=true`.
- Gate the channel with `SLACK_GENERAL_CHANNEL_ID`.
- Trigger user is `CHAT_ALLOWED_USER_ID` (Grant-only by policy).
- Trigger chance is deterministic per message via `MULTIAGENT_GENERAL_AUTO_REPLY_PROBABILITY` (default `0.4`).
- Winner selection is deterministic across pods from message timestamp + `MULTIAGENT_ORDER` + optional `MULTIAGENT_SHUFFLE_SECRET`, so exactly one bot responds.

### Turn quality policy (runtime-enforced prompt block)

During multi-agent sessions, each slot now receives an explicit policy block before LLM generation:

- **Role lanes** are reinforced per employee (`ross` execution/risk, `alex` GTM/revenue, `tim` tradeoffs/experiments, `garth` synthesis/checklists).
- **Novelty guard**: each agent is instructed to add one *new* angle instead of repeating previous bot lines.
- **Single closer pattern**: non-final slots are instructed not to provide the final merged answer; the final slot is instructed to provide the closing recommendation.
- **Close without ping-pong**: final slot suppresses handoff mentions by forcing handoff probability to `0` for that slot only.

### Recency-weighted context

Context blocks now include explicit recency weights so the newest message dominates while still preserving short memory:

- `LLM_CONTEXT_WEIGHT_DECAY` (default `0.5`)
- `LLM_CONTEXT_WEIGHT_WINDOW` (default `3`)

This keeps conversations evolving naturally without drifting too far off the latest user intent.

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
- Synced keys include `LLM_API_KEY`, or `OPENROUTER_API_KEY` / `OPENROUTER_KEY` (written into Secret as `LLM_API_KEY`), `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, optional `SLACK_USER_TOKEN`, optional `LLM_MODEL`, and `MULTIAGENT_BOT_USER_IDS`.
- `MULTIAGENT_BOT_USER_IDS` auto-resolves from each bot token via Slack `auth.test` (order from `MULTIAGENT_ORDER`, default `ross,tim,alex,garth`) unless explicitly provided.
