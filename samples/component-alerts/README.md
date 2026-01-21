## Component Alerts Samples

This sample builds on top of the `gcp-microservices-demo` in `samples/gcp-microservices-demo` and shows how to:

- Attach observability alert rules to existing components as traits
- Configure email and webhook notification channels
- Create controlled failure scenarios to fire those alerts

### Prerequisites

- A running OpenChoreo control plane, data plane, and observability plane
- `kubectl` configured to talk to the cluster where OpenChoreo is installed

### Files in this folder

- `alert-rule-trait.yaml`: the `Trait` definition for `observability-alert-rule` that enables attaching alert rules to components. This trait is typically installed as part of the OpenChoreo control plane, but is included here for reference.
- `alert-notification-channels.yaml`: two `ObservabilityAlertsNotificationChannel`s (one email, one webhook) plus their backing `Secret`s for the `development` environment.
- `components-with-alert-rules.yaml`: `Component` definitions for `frontend`, `recommendation`, and `cart` microservices with alert rules attached as `observability-alert-rule` traits.
- `failure-scenario-setup.yaml`: `ReleaseBinding`s that deliberately misconfigure the system (env var overrides, CPU / memory limit overrides) to cause alerts to fire.
- `trigger-alerts.sh`: simple script that drives traffic to the `frontend` to trigger the configured alerts.

### Order of execution

1. Deploy the `gcp-microservices-demo` sample to setup the project and components
   - See `samples/gcp-microservices-demo/README.md` for exact commands.

2. Ensure the `observability-alert-rule` trait is available
   - The trait is typically installed as part of the OpenChoreo control plane by default unless you have disabled the default resources.
   - If it's not available, you can deploy it manually:
     ```bash
     kubectl apply -f samples/component-alerts/alert-rule-trait.yaml
     ```
   - Verify it exists:
     ```bash
     kubectl get trait observability-alert-rule -n default
     ```

3. Deploy the `alert-notification-channels` to setup the notification channels
   - Edit `samples/component-alerts/alert-notification-channels.yaml` and replace the placeholder values:
     - SMTP credentials (host, port, username, password, from/to email addresses)
     - Webhook URL and custom headers (if required)
     - **Webhook payload template** (optional): For webhooks that require specific JSON formats (like Slack), you can provide a `payloadTemplate` using CEL expressions. The sample includes a Slack-compatible template. If not provided, the raw alertDetails object will be sent.
   - Then apply:
     ```bash
     kubectl apply -f samples/component-alerts/alert-notification-channels.yaml
     ```
   - Note: The notification channels are bounded to the environment. Channels in this sample are specified for the `development` environment. The first notitification channel created will be marked as the default notification channel for the environment by OpenChoreo.

4. Deploy the `components-with-alert-rules` to setup the alert rules for frontend, recommendation and cart components
   - This attaches alert-rule traits to the existing components:
     - **Frontend**: Log-based alert that triggers when "rpc error: code = Unavailable" appears more than 5 times within 5 minutes
     - **Recommendation**: Metric-based alert that triggers when CPU usage exceeds 80% for 5 minutes
     - **Cart**: Metric-based alert that triggers when memory usage exceeds 70% for 2 minutes
   - Apply:
     ```bash
     kubectl apply -f samples/component-alerts/components-with-alert-rules.yaml
     ```
   - **Note**: The alert rules defined here don't specify notification channels in the trait parameters. Notification channels are configured per environment via `ReleaseBinding` `traitOverrides`. If you want to connect these alert rules to specific notification channels, you can use the `ReleaseBinding` resources in step 5 with `traitOverrides` that reference the notification channel names (e.g., `email-notification-channel-development` or `webhook-notification-channel-development`). If not specified, the default notification channel for the environment will be used.

5. Deploy the `failure-scenario-setup` to setup the failure scenario for frontend, recommendation and cart components
   - This creates `ReleaseBinding`s that:
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
   - Apply:
     ```bash
     kubectl apply -f samples/component-alerts/failure-scenario-setup.yaml
     ```
   - **Note**: The `traitOverrides` allow you to customize alert rule behavior per environment. You can enable/disable alerts, toggle AI root cause analysis, and specify which notification channel to use. If `traitOverrides` are not specified for an alert rule, it will use the defaults (enabled, no AI analysis, default notification channel for the environment).

6. Deploy the `trigger-alerts` script to trigger the alerts
   - From the repo root:
     ```bash
     chmod +x samples/component-alerts/trigger-alerts.sh
     ./samples/component-alerts/trigger-alerts.sh
     ```
   - This continuously calls the frontend URL to generate load and surface the misconfigurations.

7. Verify the alerts received to configured notification channels
   - For the email channel, check the inbox configured in `alert-notification-channels.yaml`.
   - For the webhook channel, check the target endpoint and/or the OpenChoreo observer logs.

8. Cleanup the resources
   - Stop the trigger script (Ctrl+C).
   - Delete the sample resources in reverse order if desired:
     ```bash
     kubectl delete -f samples/component-alerts/failure-scenario-setup.yaml
     kubectl delete -f samples/component-alerts/components-with-alert-rules.yaml
     kubectl delete -f samples/component-alerts/alert-notification-channels.yaml
     # Optionally delete the gcp-microservices-demo resources as well
     ```
   - Note: If you manually deployed the trait, you can remove it:
     ```bash
     kubectl delete -f samples/component-alerts/alert-rule-trait.yaml
     ```

### Troubleshooting

#### Webhook returns 400 Bad Request (e.g., Slack)

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
