# employee-factory

**BimRoss treats the company as code.** This is the **Slack worker**: the process that turns a persona (from [`cursor-rules`](https://github.com/bimross/cursor-rules)) and a Slack app (from [`slack-factory`](https://github.com/bimross/slack-factory)) into something that actually answers in Slack.

The repo is the source of truth for behavior—read the code and `.env.example` if you are running or extending it.

## LLM resilience (Chutes)

Chat completions retry on transient provider errors (`429`, `502`, `503`, and Chutes “no instances available”) with exponential backoff, then optionally try `LLM_FALLBACK_MODEL` on the same `LLM_BASE_URL` and API key (typically a smaller model). Configure via `LLM_MAX_RETRIES`, `LLM_RETRY_BACKOFF_MS`, and `LLM_FALLBACK_MODEL` (see `.env.example`). No extra secrets.

## Deploying to the admin cluster (Fleet)

Phase 1 adds only ConfigMap keys; runtime Secrets stay the same.

1. Merge this repo to the default branch and cut a semver tag so CI builds and pushes `geeemoney/employee-factory:<version>`.
2. In [`rancher-admin`](https://github.com/bimross/rancher-admin), bump the `geeemoney/employee-factory` image tag on all four deployments under `admin/apps/employee-factory/` (`alex`, `tim`, `ross`, `garth`) to match the new tag.
3. Commit to `rancher-admin` `master`; Fleet applies the new image and the updated `employee-factory-config` (retry/fallback env) from the same repo.
