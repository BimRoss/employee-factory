# employee-factory

**BimRoss treats the company as code.** This is the **Slack worker**: the process that turns a persona (from [`cursor-rules`](https://github.com/bimross/cursor-rules)) and a Slack app (from [`slack-factory`](https://github.com/bimross/slack-factory)) into something that actually answers in Slack.

The repo is the source of truth for behavior—read the code and `.env.example` if you are running or extending it.

## LLM resilience (Chutes)

Chat completions retry on transient provider errors (`429`, `502`, `503`, and Chutes “no instances available”) with exponential backoff, then optionally try `LLM_FALLBACK_MODEL` on the same `LLM_BASE_URL` and API key (typically a smaller model). Configure via `LLM_MAX_RETRIES`, `LLM_RETRY_BACKOFF_MS`, and `LLM_FALLBACK_MODEL` (see `.env.example`). No extra secrets.

## Multi-agent Slack (`<!everyone>` / `<!channel>`)

For channel-wide triggers (no individual bot `@mention`), each squad pod receives the same Events API message and runs a shared **multi-agent session**:

- **Turn order** is a **pseudorandom permutation** of `MULTIAGENT_ORDER`, deterministic per trigger from `SHA-256(anchor message timestamp + NUL + comma-joined order + NUL + optional secret)`. Every pod must use the same `MULTIAGENT_ORDER` and the same optional `MULTIAGENT_SHUFFLE_SECRET` so they agree on who posts first, second, etc.
- **Coordination** is **not** Redis: each bot polls `conversations.history` until prior squad messages match the expected slot prefix, then calls the LLM and posts (same as before).
- **`MULTIAGENT_BROADCAST_ROUNDS`** (default `1`) is how many full passes over that shuffled order to run per trigger (`1` ⇒ each agent replies once). Raise it for longer “jam” threads.

## Deploying to the admin cluster (Fleet)

Phase 1 adds only ConfigMap keys; runtime Secrets stay the same.

1. Merge this repo to the default branch and cut a semver tag so CI builds and pushes `geeemoney/employee-factory:<version>`.
2. In [`rancher-admin`](https://github.com/bimross/rancher-admin), bump the `geeemoney/employee-factory` image tag on all four deployments under `admin/apps/employee-factory/` (`alex`, `tim`, `ross`, `garth`) to match the new tag.
3. Commit to `rancher-admin` `master`; Fleet applies the new image and the updated `employee-factory-config` (retry/fallback env) from the same repo.
