# Slack squad bot user IDs (BimRoss)

These are the **Slack bot user IDs** (`auth.test` / Bot User) used for multi-agent sequencing, `@name` → `<@U…>` substitution, and mention parsing—not OAuth client IDs.

## Canonical source

Update IDs in **one place** and mirror here:

1. **`bimross/rancher-admin`**: `admin/apps/employee-factory/configmap.yaml`  
   Keys: `ROSS_SLACK_BOT_ID`, `TIM_SLACK_BOT_ID`, `ALEX_SLACK_BOT_ID`, `GARTH_SLACK_BOT_ID`, plus `MULTIAGENT_ORDER`, `MULTIAGENT_BROADCAST_ROUNDS`, and optional `MULTIAGENT_SHUFFLE_SECRET` (same value on all pods if set).

2. **Fleet / cluster**: ConfigMap `employee-factory-config` in namespace `employee-factory` (all four deployments use `envFrom` this ConfigMap).

## Current values (keep in sync with GitOps)

| Employee | Env key               | Slack user ID |
|----------|------------------------|---------------|
| Ross     | `ROSS_SLACK_BOT_ID`    | `U0APX108QE7` |
| Tim      | `TIM_SLACK_BOT_ID`     | `U0AQ10R2H8E` |
| Alex     | `ALEX_SLACK_BOT_ID`    | `U0APSMH05B5` |
| Garth    | `GARTH_SLACK_BOT_ID`   | `U0GARTH00000` (placeholder—replace with real ID after Slack app install) |

Optional single-line equivalent:

`MULTIAGENT_BOT_USER_IDS=ross:U0APX108QE7,tim:U0AQ10R2H8E,alex:U0APSMH05B5,garth:<REAL_GARTH_ID>`

## Verify on cluster

```bash
kubectl -n employee-factory get configmap employee-factory-config \
  -o jsonpath='{.data.ROSS_SLACK_BOT_ID}{"\n"}{.data.TIM_SLACK_BOT_ID}{"\n"}{.data.ALEX_SLACK_BOT_ID}{"\n"}{.data.GARTH_SLACK_BOT_ID}{"\n"}'
```

## Tests

`internal/slackbot/slack_format_test.go` uses sample IDs in mention-substitution tests—update that file if canonical squad IDs change.
