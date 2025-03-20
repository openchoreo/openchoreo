# GitHub Issue Reporter Task

This sample demonstrates Choreo's powerful application configuration capability provided via ConfigurationGroups using 
a Scheduled Task that simulates an integration with GitHub to report issues.

## What is a ConfigurationGroup?

A ConfigurationGroup is a Choreo resource abstraction that allows you to define a set of configurations that can be 
shared by multiple applications across various environments. This simplifies configuration management, especially when 
applications share common configuration values.

## What does this application do?

This application is a task designed to simulate (without actual connections) GitHub issue reporting via email notifications.

This application will simulate the following steps:

1. Log that it is connecting to GitHub to fetch the issues.
2. Log that it is connecting to the MySQL database and store the processed issues.
3. Log that it is sending an email notification to the configured email address.

### Required environment variables

**GitHub connection details**

- GITHUB_REPOSITORY: Full url of the GitHub repository
- GITHUB_TOKEN: GitHub personal access token

**MySQL connection details**

- MYSQL_HOST: Hostname of the MySQL database
- MYSQL_PORT: Port of the MySQL database
- MYSQL_USER: Username of the MySQL database
- MYSQL_PASSWORD: Password of the MySQL database
- MYSQL_DATABASE: Database name of the MySQL database

**SMTP connection details for sending the email**

- EMAIL_HOST: Hostname of the email server
- EMAIL_PORT: Port of the email server
- EMAIL_SENDER: Email address of the sender
- EMAIL_PASSWORD: Password of the email server
- EMAIL_TO: Email address of the recipient

## Pre-requisites

- Kubernetes cluster with Choreo installed
- The `choreoctl` and `kubectl` CLI tools installed

## Add secret values to the vault

The Choreo data plane comes with a built-in HashiCorp Vault that can be used to store sensitive information such as
passwords, API keys, and tokens. For this sample, we will add the necessary secret values to the vault.

1. Ensure the Choreo Vault is running.

    ```shell
    kubectl -n choreo-system get pods  | grep vault
    ```

   This should provide an output similar to the following:

    ```
    choreo-vault-0                                                   1/1     Running   0              44h
    choreo-vault-csi-provider-j8pld                                  1/1     Running   0              44h
    ```

2. The following command will add all the necessary secret values to the vault in a single command.

    ```shell
    kubectl -n choreo-system exec -it choreo-vault-0 -- sh -c "vault kv put --mount=secret dev/github/pat value=gh_dev_token && vault kv put --mount=secret stg/github/pat value=gh_stg_token && vault kv put --mount=secret prod/github/pat value=gh_prod_token && vault kv put --mount=secret dev/mysql/password value=mysql_dev_password && vault kv put --mount=secret stg/mysql/password value=mysql_stg_password && vault kv put --mount=secret prod/mysql/password value=mysql_prod_password && vault kv put --mount=secret dev/email/no-reply/password value=email_non_prod_password && vault kv put --mount=secret prod/email/no-reply/password value=email_prod_password"
    ```

If you want to add the secret values one by one or modify the values, here are the individual commands:

- GitHub Personal Access Token
    ```shell
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret dev/github/pat value=gh_dev_token
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret stg/github/pat value=gh_stg_token
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret prod/github/pat value=gh_prod_token
    ```

- MySQL Password
    ```shell
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret dev/mysql/password value=mysql_dev_password
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret stg/mysql/password value=mysql_stg_password
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret prod/mysql/password value=mysql_prod_password
    ```
- Email Password
    ```shell
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret dev/email/no-reply/password value=email_non_prod_password
    kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv put --mount=secret prod/email/no-reply/password value=email_prod_password
    ```

> [!NOTE] 
> - The provided secret format is just for clear separation of the secrets. You can use any key format that suits your
  requirement.
> - The value should be in the format `value=<secret_value>`. This means we store only one key-value pair in each secret.
  This is to keep the compatibility with other key-value stores and provide a clear separation of the secrets.

## Deploy the ConfigurationGroups and the Task

For simplicity, this sample provides a [single YAML](github-issue-reporter.yaml) file that contains the both the
ConfigurationGroups and the Task.
Additionally, the Task is scheduled to run every minute.


1. Run the following command to deploy the ConfigurationGroups and the Task in one go.

    ```shell
    choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/github-issue-reporter-task/github-issue-reporter.yaml
    ```

2. Run the following command to see if the Task is deployed successfully in each environment.

    For convenience, let's create an alias for the `choreoctl` with the organization, project and component before proceeding.
    
    ```shell
    alias choreoctl='choreoctl --organization default-org --project default-project --component github-issue-reporter'
    ```

    - Development
        ```shell
        choreoctl get deployments --environment=development
        ```
      The output should be similar to the following:
        ```
        NAME                     ARTIFACT                       ENVIRONMENT   STATUS                    AGE   COMPONENT               PROJECT           ORGANIZATION
        development-deployment   github-issue-reporter-latest   development   Ready (DeploymentReady)   12m   github-issue-reporter   default-project   default-org
        ```
    - Staging
        ```shell
        choreoctl get deployments --environment=staging
        ```
      The output should be similar to the following:
        ```
        NAME                 ARTIFACT                       ENVIRONMENT   STATUS                    AGE   COMPONENT               PROJECT           ORGANIZATION
        staging-deployment   github-issue-reporter-latest   staging       Ready (DeploymentReady)   20h   github-issue-reporter   default-project   default-org
        ```
    - Production
        ```shell
        choreoctl get deployments --environment=production
        ```
      The output should be similar to the following:
        ```
        NAME                    ARTIFACT                       ENVIRONMENT   STATUS                    AGE   COMPONENT               PROJECT           ORGANIZATION
        production-deployment   github-issue-reporter-latest   production    Ready (DeploymentReady)   20h   github-issue-reporter   default-project   default-org
        ```

### View the logs

After waiting for a minute for the Task to run, you can check the logs to see if the Task is running as expected with the
environment specific configurations.

- Check the logs for the Development environment
    ```shell
    choreoctl logs --type=deployment --environment=development --deployment=development-deployment
    ```
    You should see a similar output as below:
    ```
    === Pod: github-issue-reporter-github-issue-reporter-c5d0fdde-290359rkk2 ===
    Starting task at 2025-03-16T06:38:02Z
    ----------------------------------------------------------------------------------------
    DEMO: This is for demonstration purposes only. DO NOT log secrets from your application.
    ----------------------------------------------------------------------------------------
    Fetched 2 issues from GitHub repository.
    Connecting to GitHub repository: https://github.com/choreo-idp/choreo with token: gh_dev_token
    Connecting to MySQL at dev-mysql.internal:3306 with user: dev-sql-user and password: mysql_dev_password to database: github-issue-reporter
    Processing issues and storing them in the database...
    Issues have been processed and stored in the database successfully.
    Sending email via smtp-test.internal:587 from no-reply@example.com (password: email_non_prod_password) to john@example.com
    Email with the issue summary has been sent.
    Completed task at 2025-03-16T06:38:02Z
    ```
- Check the logs for the Staging environment
    ```shell
    choreoctl logs --type=deployment --environment=staging --deployment=staging-deployment
    ```
    You should see a similar output as below:
    ```
    === Pod: github-issue-reporter-github-issue-reporter-c5d0fdde-29035jgv8s ===
    Starting task at 2025-03-16T06:41:03Z
    ----------------------------------------------------------------------------------------
    DEMO: This is for demonstration purposes only. DO NOT log secrets from your application.
    ----------------------------------------------------------------------------------------
    Fetched 2 issues from GitHub repository.
    Connecting to GitHub repository: https://github.com/choreo-idp/choreo with token: gh_stg_token
    Connecting to MySQL at stg-mysql.internal:3306 with user: stg-sql-user and password: mysql_stg_password to database: github-issue-reporter
    Processing issues and storing them in the database...
    Issues have been processed and stored in the database successfully.
    Sending email via smtp-test.internal:587 from no-reply@example.com (password: email_non_prod_password) to john@example.com
    Email with the issue summary has been sent.
    Completed task at 2025-03-16T06:41:03Z
    ```

- Check the logs for the Production environment
    ```shell
    choreoctl logs --type=deployment --environment=production --deployment=production-deployment
    ```
    You should see a similar output as below:
    ```
    === Pod: github-issue-reporter-github-issue-reporter-c5d0fdde-290352vsxp ===
    Starting task at 2025-03-16T06:41:05Z
    ----------------------------------------------------------------------------------------
    DEMO: This is for demonstration purposes only. DO NOT log secrets from your application.
    ----------------------------------------------------------------------------------------
    Fetched 2 issues from GitHub repository.
    Connecting to GitHub repository: https://github.com/choreo-idp/choreo with token: gh_prod_token
    Connecting to MySQL at prod-mysql.internal:3306 with user: prod-sql-user and password: mysql_prod_password to database: github-issue-reporter
    Processing issues and storing them in the database...
    Issues have been processed and stored in the database successfully.
    Sending email via smtp.internal:587 from no-reply@example.com (password: email_prod_password) to max@example.com,john@example.com
    Email with the issue summary has been sent.
    Completed task at 2025-03-16T06:41:05Z
    ```

Notice that the logs show the environment specific configurations being used for each environment.

If you don't see any logs or have different output, please refer to the [Troubleshoot](#troubleshoot) section.

## Clean up

To clean up the resources created by this sample, you can run the following commands:

- Unalias the choreoctl alias if you have created one
    ```shell
    unalias choreoctl
    ```
- Delete the resources created by the sample
    ```shell
    choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/github-issue-reporter-task/github-issue-reporter.yaml
    ```
- Delete the secrets from the vault
    ```shell
    kubectl -n choreo-system exec -it choreo-vault-0 -- sh -c "vault kv destroy -mount=secret -versions=1 dev/github/pat && vault kv metadata delete -mount=secret dev/github/pat && vault kv destroy -mount=secret -versions=1 stg/github/pat && vault kv metadata delete -mount=secret stg/github/pat && vault kv destroy -mount=secret -versions=1 prod/github/pat && vault kv metadata delete -mount=secret prod/github/pat && vault kv destroy -mount=secret -versions=1 dev/mysql/password && vault kv metadata delete -mount=secret dev/mysql/password && vault kv destroy -mount=secret -versions=1 stg/mysql/password && vault kv metadata delete -mount=secret stg/mysql/password && vault kv destroy -mount=secret -versions=1 prod/mysql/password && vault kv metadata delete -mount=secret prod/mysql/password && vault kv destroy -mount=secret -versions=1 dev/email/no-reply/password && vault kv metadata delete -mount=secret dev/email/no-reply/password && vault kv destroy -mount=secret -versions=1 prod/email/no-reply/password && vault kv metadata delete -mount=secret prod/email/no-reply/password"
    ```

> [!TIP]
> #### Troubleshoot
> 
> - Log output: `Error: no deployment pods found for component 'github-issue-reporter' in environment '<environment>''`
>    - The Task might not have run yet. Wait for a minute and try again.
> 
> - Log output: `failed to get log stream: container "main" in pod "<pod-name>" is waiting to start: ContainerCreating`
>    - This could indicate that the secret is not created in the data plane. Please ensure that the secret is created in the
>      vault as mentioned in the [Adding the secret values to the vault](#adding-the-secret-values-to-the-vault) section.
>    - To verify the secret, you can run the following command for each secret in the environment:
>      ```shell
>      kubectl -n choreo-system exec -it choreo-vault-0 -- vault kv get --mount=secret dev/github/pat
>      ```
>    - Run the following command to verify the vault operator can retrieve the secret value.
>      ```shell
>      kubectl -n choreo-system logs -l app.kubernetes.io/name=choreo-vault-csi-provider --since=10m -f | grep 404 -C 10
>      ```
>      If you see a log similar to the following, it means the secret is not found in the vault.
>      ```
>      2025-03-16T07:17:10.920Z [INFO]  server: Finished unary gRPC call: grpc.method=/v1alpha1.CSIDriverProvider/Mount grpc.time=2.718397ms grpc.code=Unknown
>       err=
>        | error making mount request: couldn't read secret "pat": error requesting secret: Error making API request.
>        |
>        | URL: GET http://choreo-vault:8200/v1/secret/data/dev/github/pat`
>        | Code: 404. Errors:
>        |
>      ```
> 
> If any of the above does not resolve the issue, please contact us via the [Discord channel](https://discord.gg/HYCgUacN) for further assistance.
