# Personality OAuth Scope Map

Canonical map for non-Slack OAuth scopes requested by each personality runtime in `employee-factory`.

Use this file when:
- adding a new external tool for an employee persona
- rotating refresh tokens
- reviewing least-privilege scope drift

Slack app scopes are versioned in `slack-factory` manifests and are intentionally not duplicated here.

## Google OAuth scopes

| Personality | Feature | Env gate | Scopes |
| --- | --- | --- | --- |
| joanne | Gmail send-email tool | `JOANNE_EMAIL_ENABLED=true` | `https://www.googleapis.com/auth/gmail.send` |
| joanne | Google Docs create-doc tool | `JOANNE_GOOGLE_DOCS_ENABLED=true` | `https://www.googleapis.com/auth/documents` |

## Update protocol

When adding/changing scopes:
1. Update this map in the same PR as code/config changes.
2. Update `.env.example` and README setup steps.
3. Re-consent and rotate refresh token if new scopes are required.
4. Verify runtime behavior from Slack and confirm success logs.
