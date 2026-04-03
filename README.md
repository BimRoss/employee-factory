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
- `MULTIAGENT_CHATTER_CAP` applies a global runtime cap across all handoff-style mention probabilities (default `0.25`) to keep banter low as squad size grows.
- **`MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY`** (default `0.35`, then capped by `MULTIAGENT_CHATTER_CAP`) controls broadcast mention intensity.
- **Deterministic branch mode**: with `MULTIAGENT_BROADCAST_BRANCHING_ENABLED=true`, each `<!everyone>` trigger deterministically flips into branch mode using `MULTIAGENT_BROADCAST_BRANCHING_PROBABILITY` (default `0.5`). Branch mode uses `MULTIAGENT_BROADCAST_BRANCHING_HANDOFF_PROBABILITY` (default `0.6`, then capped by `MULTIAGENT_CHATTER_CAP`) to keep cross-agent follow-ons balanced while preserving stable ordering across pods.

## Plain `#general` auto-reply (single random agent)

When a plain (no-bot-mention, no channel-wide summon) message is posted in `#general`, the squad can auto-reply as one agent:

- Enable with `MULTIAGENT_GENERAL_AUTO_REPLY_ENABLED=true`.
- Gate the channel with `SLACK_GENERAL_CHANNEL_ID`.
- Trigger user is `CHAT_ALLOWED_USER_ID` (Grant-only by policy).
- Trigger chance is deterministic per message via `MULTIAGENT_GENERAL_AUTO_REPLY_PROBABILITY` (default `0.4`).
- Winner selection is deterministic across pods from message timestamp + `MULTIAGENT_ORDER` + optional `MULTIAGENT_SHUFFLE_SECRET`, so exactly one bot responds.

## Availability/signoff router

`employee-factory` supports a deterministic ingress router for operator availability cues.

- `ROUTER_AVAILABILITY_ENABLED=true` enforces async-safe behavior for cues like `step away`, `afk`, `back later`, `sign off`, and `go to bed`.
- `ROUTER_LOG_ONLY=true` records `router_decision` traces without suppressing normal reply paths.
- Router policy is checked at Slack ingress and again pre-LLM in channel/thread/multi-agent paths so edge event shapes do not bypass safety.
- Enforced behavior posts one concise acknowledgment and suppresses additional asks/mentions for that message path.

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
2. In [`rancher-admin`](https://github.com/bimross/rancher-admin), bump the `geeemoney/employee-factory` image tag on all five deployments under `admin/apps/employee-factory/` (`alex`, `tim`, `ross`, `garth`, `joanne`) to match the new tag.
3. Commit to `rancher-admin` `master`; Fleet applies the new image and the updated `employee-factory-config` (retry/fallback env) from the same repo.

## Runtime secret sync (kubectl path)

- `scripts/update-runtime-secrets.sh` syncs one employee runtime Secret from local `.env` (`EMPLOYEE_ID=alex|tim|ross|garth|joanne`).
- `scripts/update-all-runtime-secrets.sh` syncs all five runtime Secrets in one pass.
- Synced keys include `LLM_API_KEY`, or `OPENROUTER_API_KEY` / `OPENROUTER_KEY` (written into Secret as `LLM_API_KEY`), `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`, optional `SLACK_USER_TOKEN`, optional `LLM_MODEL`, and `MULTIAGENT_BOT_USER_IDS`.
- `MULTIAGENT_BOT_USER_IDS` auto-resolves from each bot token via Slack `auth.test` (order from `MULTIAGENT_ORDER`, default `ross,tim,alex,garth,joanne`) unless explicitly provided.

## Joanne Gmail send-email tooling (first vertical slice)

Joanne can now send Gmail on command from Slack when OAuth runtime config is present.

### Required env/runtime keys (Joanne only)

- `JOANNE_EMAIL_ENABLED=true`
- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`
- `GOOGLE_REFRESH_TOKEN`
- `GOOGLE_SENDER_EMAIL`

These are loaded from the `employee-factory-joanne-runtime` Secret (same secret flow as Slack and LLM keys).

### Google setup (least privilege)

1. Enable Gmail API on your Google Cloud project.
2. Create OAuth client credentials for the Joanne mailbox flow.
3. Grant scope `https://www.googleapis.com/auth/gmail.send`.
4. Generate/store a refresh token for the Joanne account.

Do not use password-based auth for this path.

### Command contract (first pass)

- Trigger intent: message includes "send email" / "send an email" / "draft email".
- Optional explicit fields:
  - `to: name@example.com`
  - `subject: ...`
  - `instruction: ...` (Joanne drafts body in her voice)
  - `body: ...` (direct body override)
- If `to` is omitted, recipient defaults to the requesting Slack user's profile email (`users.info`).

### Cluster-first E2E runbook

1. Populate `.env` with the five Joanne Google keys above.
2. Sync Joanne runtime secret:
   - `EMPLOYEE_ID=joanne ./scripts/update-runtime-secrets.sh`
3. Restart or rollout Joanne deployment if needed:
   - `kubectl -n employee-factory rollout restart deploy/employee-factory-joanne`
   - `kubectl -n employee-factory rollout status deploy/employee-factory-joanne`
4. In Slack, ask Joanne to send an email (with `instruction:` or `body:`).
5. Verify:
   - Slack confirmation from Joanne
   - Recipient inbox received message
   - Pod logs show `joanne_email: send success`
