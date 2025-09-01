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
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/from-image/issue-reporter-schedule-task/github-issue-reporter.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 2: Monitor the Scheduled Task

Check the status of the scheduled task and its job executions:

```bash
# Check the scheduled task status
kubectl get scheduledtaskbinding github-issue-reporter -o yaml

# View the CronJob created by the scheduled task
kubectl get cronjob -A -l component-name=github-issue-reporter

# Monitor job executions
kubectl get jobs -A -l component-name=github-issue-reporter

# View logs from the latest job
kubectl logs -n $(kubectl get pods -A -l app.kubernetes.io/component=issue-reporter-schedule-task -o jsonpath='{.items[0].metadata.namespace}') -l app.kubernetes.io/component=issue-reporter-schedule-task --tail=50
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

1. **Check the scheduled task status:**
   ```bash
   kubectl describe scheduledtaskbinding github-issue-reporter
   ```

2. **Verify the CronJob is created:**
   ```bash
   kubectl get cronjob -A -l component-name=github-issue-reporter -o yaml
   ```

3. **Check recent job executions:**
   ```bash
   kubectl get jobs -A -l component-name=github-issue-reporter --sort-by=.metadata.creationTimestamp
   ```

4. **View job logs for debugging:**
   ```bash
   # Get the latest job name
   JOB_NAME=$(kubectl get jobs -A -l component-name=github-issue-reporter --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')
   
   # Get the namespace of the job
   JOB_NAMESPACE=$(kubectl get jobs -A -l component-name=github-issue-reporter --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.namespace}')
   
   # View logs
   kubectl logs -n $JOB_NAMESPACE job/$JOB_NAME
   ```

5. **Check for failed jobs:**
   ```bash
   kubectl get jobs -A -l component-name=github-issue-reporter --field-selector status.successful!=1
   ```

## Schedule Configuration

The task is configured to run every minute (`*/1 * * * *`) for testing purposes. To modify the schedule:

1. Edit the `ScheduledTaskClass` resource
2. Update the `cronJobTemplate.schedule` field with a valid cron expression
3. Apply the changes

Example schedules:
- `0 */6 * * *` - Every 6 hours
- `0 9 * * 1-5` - Every weekday at 9 AM
- `0 0 */3 * *` - Every 3 days at midnight

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/from-image/issue-reporter-schedule-task/github-issue-reporter.yaml
```
