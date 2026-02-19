# GitHub Issue Reporter Scheduled Task (From Image)

## Overview

This sample demonstrates how to deploy a GitHub Issue Reporter as a scheduled task in OpenChoreo from a pre-built container image. The scheduled task runs periodically to report GitHub issues and integrates with MySQL database and email notifications.

The scheduled task is deployed from the pre-built image:
`ghcr.io/openchoreo/samples/github-issue-reporter:latest`

Features:
- **GitHub Integration**: Connects to GitHub repositories to fetch issue data
- **Database Storage**: Stores issue data in MySQL database
- **Email Notifications**: Sends email reports about GitHub issues
- **Configurable Schedule**: Runs every minute (configurable via CronJob schedule)

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-image/issue-reporter-schedule-task/github-issue-reporter.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 2: Monitor the Scheduled Task

Check the status of the scheduled task and its job executions:

```bash
# View the CronJob created by the scheduled task
kubectl get cronjob -A -l openchoreo.dev/component=github-issue-reporter

# Monitor job executions
kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter

# View logs from the latest job pod
POD_NAMESPACE=$(kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter -o jsonpath='{.items[0].metadata.namespace}')
JOB_NAME=$(kubectl get jobs -n $POD_NAMESPACE -l openchoreo.dev/component=github-issue-reporter --sort-by=.metadata.creationTimestamp --no-headers | tail -1 | awk '{print $1}')
POD_NAME=$(kubectl get pods -n $POD_NAMESPACE -l job-name=$JOB_NAME -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n $POD_NAMESPACE $POD_NAME --tail=50
```

## Configuration

The scheduled task requires several environment variables to be configured:

### GitHub Configuration
- `GITHUB_REPOSITORY`: Target GitHub repository URL
- `GITHUB_TOKEN`: GitHub personal access token for API access

### MySQL Configuration  
- `MYSQL_HOST`: MySQL server hostname
- `MYSQL_PORT`: MySQL server port (default: 3306)
- `MYSQL_USER`: Database username
- `MYSQL_PASSWORD`: Database password
- `MYSQL_DATABASE`: Database name

### Email Configuration
- `EMAIL_HOST`: SMTP server hostname
- `EMAIL_PORT`: SMTP server port (default: 587)
- `EMAIL_SENDER`: Email sender address
- `EMAIL_PASSWORD`: Email account password
- `EMAIL_TO`: Email recipient address

> [!IMPORTANT]
> The current configuration uses hardcoded values for demonstration. In production, use Kubernetes secrets or external secret management systems.

## Troubleshooting

If the scheduled task is not working correctly:

1. **Check the ReleaseBinding status:**
   ```bash
   kubectl get releasebinding github-issue-reporter-development -n default -o yaml
   ```

2. **Check the ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding github-issue-reporter-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify the CronJob is created:**
   ```bash
   kubectl get cronjob -A -l openchoreo.dev/component=github-issue-reporter -o yaml
   ```

4. **Check recent job executions:**
   ```bash
   kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter --sort-by=.metadata.creationTimestamp
   ```

5. **View job logs for debugging:**
   ```bash
   # Get the latest job name
   JOB_NAME=$(kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

   # Get the namespace of the job
   JOB_NAMESPACE=$(kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.namespace}')

   # View logs
   kubectl logs -n $JOB_NAMESPACE job/$JOB_NAME
   ```

6. **Check for failed jobs:**
   ```bash
   kubectl get jobs -A -l openchoreo.dev/component=github-issue-reporter --field-selector status.successful!=1
   ```

## Schedule Configuration

The task is configured to run every minute (`*/1 * * * *`) for testing purposes. To modify the schedule:

1. Edit the `Component` resource in the YAML file
2. Update the `parameters.schedule` field with a valid cron expression
3. Apply the changes using `kubectl apply -f github-issue-reporter.yaml`

Example schedules:
- `0 */6 * * *` - Every 6 hours
- `0 9 * * 1-5` - Every weekday at 9 AM
- `0 0 */3 * *` - Every 3 days at midnight

You can also override the schedule for specific environments by modifying the `ReleaseBinding` resource and adding schedule to the `componentTypeEnvOverrides` section (as shown in the sample YAML where the development environment overrides the schedule to run every 5 minutes).

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-image/issue-reporter-schedule-task/github-issue-reporter.yaml
```
