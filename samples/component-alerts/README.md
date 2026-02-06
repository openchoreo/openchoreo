## Component Alerts Samples

This sample builds on top of the `gcp-microservices-demo` in `samples/gcp-microservices-demo` and shows how to:

- Attach observability alert rules to existing components as traits
- Configure email and webhook notification channels
- Create controlled failure scenarios to fire those alerts

### Prerequisites

- A running OpenChoreo control plane, data plane, and observability plane
- `kubectl` configured to talk to the cluster where OpenChoreo is installed

### Architecture Overview

This sample uses a simple, role-based workflow:

- **Platform Engineers (one-time setup)**:
  - Deploy the `observability-alert-rule` trait once per OpenChoreo installation
  - Allow the `observability-alert-rule` trait in the desired `ComponentType`. This is already done for you in the default component types `deployment/service` and `deployment/web-application`
  - Configure notification channels once per environment (email, webhook, etc.)

- **Developers (once per component)**:
  - Attach `observability-alert-rule` traits to components
  - The same alert rules are reused as components are promoted across environments

- **Per-environment tuning (by Platform Engineers)**:
  - Use `ReleaseBinding` `traitOverrides` to enable/disable alerts, toggle AI analysis, and choose notification channels per environment

### Files in this folder

- `alert-rule-trait.yaml`: The `Trait` definition for `observability-alert-rule` that enables attaching alert rules to components. This trait is typically installed as part of the OpenChoreo control plane, but is included here for reference. **One-time setup by Platform Engineers.**
- `alert-notification-channels.yaml`: Two `ObservabilityAlertsNotificationChannel`s (one email, one webhook) plus their backing `Secret`s for the `development` environment. **One-time setup per environment by Platform Engineers.**
- `components-with-alert-rules.yaml`: `Component` definitions for `frontend`, `recommendation`, and `cart` microservices with alert rules attached as `observability-alert-rule` traits. **Defined once by Developers - alert rules propagate through environments.**
- `failure-scenario-setup.yaml`: `ReleaseBinding`s that deliberately misconfigure the system (env var overrides, CPU / memory limit overrides) to cause alerts to fire. Also demonstrates environment-specific alert customization via `traitOverrides`.
- `trigger-alerts.sh`: Simple script that drives traffic to the `frontend` to trigger the configured alerts.

---

## One-Time Setup (Platform Engineers)

### Step 1: Deploy Alert Rule Trait (One-Time Setup)

Platform Engineers deploy this trait once per OpenChoreo installation so developers can attach alert rules to any component.

The trait is typically installed as part of the OpenChoreo control plane by default unless you have disabled the default resources. If it's not available, you can deploy it manually:

```bash
kubectl apply -f samples/component-alerts/alert-rule-trait.yaml
kubectl get trait observability-alert-rule -n default
```

### Step 2: Configure Notification Channels (One-Time Setup)

Platform Engineers configure notification channels once per environment (email/webhook) and reuse them across all alert rules in that environment.

1. Edit `samples/component-alerts/alert-notification-channels.yaml` and replace the placeholder values:
   - SMTP credentials (host, port, username, password, from/to email addresses)
   - Webhook URL and custom headers (if required)
   - **Email template** (optional): For emails that require specific formatting, you can provide a `template` using CEL expressions. The sample includes a template. If not provided, the raw alertDetails object will be sent.
   - **Webhook payload template** (optional): For webhooks that require specific JSON formats (like Slack), you can provide a `payloadTemplate` using CEL expressions. The sample includes a Slack-compatible template. If not provided, the raw alertDetails object will be sent.

2. Apply the notification channels:
   ```bash
   kubectl apply -f samples/component-alerts/alert-notification-channels.yaml
   ```

3. The notification channels are bounded to the environment. In this sample they are created for the `development` environment. The first notification channel created will be marked as the default notification channel for the environment by OpenChoreo.

---

## Developer Workflow

### Step 3: Deploy the gcp-microservices-demo Sample

Deploy the `gcp-microservices-demo` sample to setup the project and components:
- See `samples/gcp-microservices-demo/README.md` for exact commands.

### Step 4: Define Alert Rules for Components (One-Time per Component)

Developers attach alert rules as traits to components once. These rules automatically propagate to all environments as the component is promoted. Platform Engineers can still customize behavior per environment via `ReleaseBinding` `traitOverrides` (enable/disable, AI analysis, notification channel selection).

This step attaches alert-rule traits to the existing components:

- **Frontend**: Log-based alert that triggers when "rpc error: code = Unavailable" appears more than 5 times within 5 minutes
- **Recommendation**: Metric-based alert that triggers when CPU usage exceeds 80% for 5 minutes
- **Cart**: Metric-based alert that triggers when memory usage exceeds 70% for 2 minutes

Apply the components with alert rules:
```bash
kubectl apply -f samples/component-alerts/components-with-alert-rules.yaml
```

**Note**: The alert rules defined here don't specify notification channels in the trait parameters. Notification channels are configured per environment via `ReleaseBinding` `traitOverrides`. If you want to connect these alert rules to specific notification channels, you can use the `ReleaseBinding` resources in the next section with `traitOverrides` that reference the notification channel names (e.g., `email-notification-channel-development` or `webhook-notification-channel-development`). If not specified, the default notification channel for the environment will be used.

---

## Testing and Verification

### Step 5: Configure Failure Scenarios (Optional - for Testing)

This step creates `ReleaseBinding`s that:
- **Misconfigure resources** to trigger alerts:
  - `frontend`: Override `PRODUCT_CATALOG_SERVICE_ADDR` to `http://localhost:8080` (invalid endpoint) via `workloadOverrides`
  - `recommendation`: Lower CPU limits to `10m` (very restrictive) via `componentTypeEnvOverrides`
  - `cart`: Lower memory requests to `100Mi` and limits to `150Mi` (restrictive) via `componentTypeEnvOverrides`
- **Configure alert rule behavior per environment** via `traitOverrides`:
  - **Frontend alert** (`frontend-rpc-unavailable-error-log-alert`):
    - `enabled: true` - Alert rule is enabled (default is `true`)
    - `enableAiRootCauseAnalysis: false` - AI root cause analysis disabled (default is `false`)
    - `notificationChannel: "email-notification-channel-development"` - Uses email notification channel
  - **Recommendation alert** (`recommendation-high-cpu-alert`):
    - `enabled: true` - Alert rule is enabled
    - `enableAiRootCauseAnalysis: true` - AI root cause analysis enabled
    - `notificationChannel: "webhook-notification-channel-development"` - Uses webhook notification channel
  - **Cart alert** (`cartservice-high-memory-alert`):
    - No `traitOverrides` specified - Uses default values (enabled, no AI analysis, default notification channel for environment)

Apply the failure scenario setup:
```bash
kubectl apply -f samples/component-alerts/failure-scenario-setup.yaml
```

> **Note**: `traitOverrides` let Platform Engineers customize alert rule behavior per environment (enable/disable, AI analysis, notification channel) without changing the component definition. If `traitOverrides` are not specified for an alert rule, it uses the defaults (enabled, no AI analysis, default notification channel for the environment).

### Step 6: Trigger Alerts

Deploy the `trigger-alerts` script to trigger the alerts:
- From the repo root:
  ```bash
  chmod +x samples/component-alerts/trigger-alerts.sh
  ./samples/component-alerts/trigger-alerts.sh
  ```
- This continuously calls the frontend URL to generate load and surface the misconfigurations.

### Step 7: Verify Alerts

Verify the alerts received to configured notification channels:
- For the email channel, check the inbox configured in `alert-notification-channels.yaml`.
- For the webhook channel, check the target endpoint and/or the OpenChoreo observer logs.

---

## Cleanup

Stop the trigger script (Ctrl+C).

Delete the sample resources in reverse order if desired:
```bash
kubectl delete -f samples/component-alerts/failure-scenario-setup.yaml
kubectl delete -f samples/component-alerts/components-with-alert-rules.yaml
kubectl delete -f samples/component-alerts/alert-notification-channels.yaml
# Optionally delete the gcp-microservices-demo resources as well
```

**Note**: If you manually deployed the trait, you can remove it:
```bash
kubectl delete -f samples/component-alerts/alert-rule-trait.yaml
```

---

## Troubleshooting

### Webhook returns 400 Bad Request (e.g., Slack)

**Problem**: Some webhook endpoints (like Slack) require a specific JSON payload format, but OpenChoreo sends the raw alert details object by default.

**Root Cause**:
- OpenChoreo sends by default: `{"ruleName": "...", "description": "...", "severity": "...", ...}`
- Slack expects: `{"text": "...", "blocks": [...]}`

**Solution**: Use the `payloadTemplate` feature in `webhookConfig` to transform the payload format.

**How to use payload templates**:

1. **Add `payloadTemplate` to your webhook configuration**:
   ```yaml
   webhookConfig:
     url: https://hooks.slack.com/services/...
     payloadTemplate: |
       {
         "text": "Alert: ${alertName}",
         "blocks": [
           {
             "type": "section",
             "text": {
               "type": "mrkdwn",
               "text": "*${alertDescription}*\nSeverity: ${alertSeverity}"
             }
           }
         ]
       }
   ```

2. **Available CEL expression variables**:
   - `${alertName}` - Alert rule name
   - `${alertDescription}` - Alert description
   - `${alertSeverity}` - Alert severity (info, warning, critical)
   - `${alertValue}` - Current alert value
   - `${alertThreshold}` - Alert threshold
   - `${alertType}` - Alert type (log, metric)
   - `${component}` - Component name
   - `${project}` - Project name
   - `${environment}` - Environment name
   - `${alertTimestamp}` - Alert timestamp
   - `${componentId}` - Component UID
   - `${projectId}` - Project UID
   - `${environmentId}` - Environment UID
   - `${alertAIRootCauseAnalysisEnabled}` - Whether AI root cause analysis is enabled

3. **Example templates**:
   - See `alert-notification-channels.yaml` for a complete Slack template example
   - For other services, customize the JSON structure as needed
