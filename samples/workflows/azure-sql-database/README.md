# Azure SQL Database Create — Generic Workflow Sample

This sample demonstrates a self-service Generic Workflow that lets a developer quickly spin up an Azure SQL Database for feature development or testing — without involving the platform team. Fill in a few parameters, trigger the workflow, and get a ready-to-use connection string at the end.

The database uses the Basic SKU (5 DTUs) — sized for dev/test, not production. The workflow clones this repository at runtime to execute the Terraform files in `terraform/`. Terraform state is stored in Azure Blob Storage so the same resources can be updated or destroyed later.

---

## Pipeline Overview

```
WorkflowRun
    │
    ▼
[clone-step]    — git clone the configured repo/branch to get terraform/
    │
    ▼
[setup-step]    — create the storage account and blob container for tfstate if they do not already exist
    │
    ▼
[init-step]     — terraform init with azurerm backend
    │
    ▼
[plan-step]     — terraform plan (dry run, visible in logs)
    │
    ▼
[apply-step]    — terraform apply -auto-approve; save outputs to shared volume
    │
    ▼
[report-step]   — print server FQDN, port, database, username, connection string
```

---

## Infrastructure Provisioned

| Resource | Details |
|----------|---------|
| `azurerm_resource_group` | **Pre-existing** — looked up by name, not created by this workflow |
| `azurerm_mssql_server` | Azure SQL Server (version 12.0) |
| `azurerm_mssql_database` | SQL Database with Basic SKU (5 DTUs) |
| `azurerm_mssql_firewall_rule` | Allows all IP addresses inbound (0.0.0.0 – 255.255.255.255) |

The server firewall is **open to all IPs** so you can connect directly from your workstation to verify the database. For production use, restrict the firewall rules to your application's IP range and remove the `AllowAll` rule in `terraform/main.tf`.

---

## Prerequisites

### 1. Azure AD App Registration

Register an application in Microsoft Entra ID (Azure AD) and create a client secret:

1. Go to **Azure Portal → Microsoft Entra ID → App registrations → New registration**.
2. Give it a name (e.g. `openchoreo-workflow`) and register.
3. Note the **Application (client) ID** and **Directory (tenant) ID** from the Overview page.
4. Go to **Certificates & secrets → New client secret**, add a secret, and copy the **Value** (this is your client secret — it is only shown once).

Assign the required roles to the app's service principal:

```bash
# Contributor — lets Terraform create/manage Azure resources
az role assignment create \
  --assignee <application-client-id> \
  --role "Contributor" \
  --scope /subscriptions/<your-subscription-id>

# Storage Blob Data Contributor — lets Terraform store state in Azure Blob Storage
az role assignment create \
  --assignee <application-client-id> \
  --role "Storage Blob Data Contributor" \
  --scope /subscriptions/<your-subscription-id>
```

### 2. Kubernetes Secret

Create the secret in the workflow execution namespace — `workflows-<namespace>` where `<namespace>` is the namespace your `WorkflowRun` is applied in (e.g. `workflows-default`).

The secret holds five values:
- `clientId` — Application (client) ID from the App Registration
- `clientSecret` — Client secret value from the App Registration
- `tenantId` — Directory (tenant) ID from the App Registration
- `subscriptionId` — Azure subscription ID
- `adminPassword` — administrator password for the SQL Server

```bash
kubectl create secret generic azure-sql-credentials \
  --from-literal=clientId=<your-client-id> \
  --from-literal=clientSecret=<your-client-secret> \
  --from-literal=tenantId=<your-tenant-id> \
  --from-literal=subscriptionId=<your-subscription-id> \
  --from-literal=adminPassword=<your-admin-password> \
  --namespace=workflows-default
```

> The admin password is stored in a Kubernetes Secret and injected into the workflow as an environment variable. It is **not** passed as a plain workflow parameter.

---

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `git.repoUrl` | No | `https://github.com/openchoreo/openchoreo.git` | Git repository URL (HTTPS) that contains the Terraform files. Override to use your own repo. |
| `git.branch` | No | `main` | Branch or tag to check out |
| `git.tfPath` | No | `samples/workflows/azure-sql-database/terraform` | Relative path inside the cloned repo to the directory containing the Terraform files |
| `azure.location` | No | `eastus` | Azure region for the Terraform state storage account (SQL Server location is inherited from the resource group) |
| `azure.credentialsSecret` | Yes | `azure-sql-credentials` | Name of the Kubernetes Secret (see Prerequisites) |
| `tfState.resourceGroup` | Yes | `openchoreo-tfstate-rg` | Azure Resource Group for the Terraform state storage account. Created automatically on first run. |
| `tfState.storageAccount` | Yes | `ocstfstate` | Azure Storage Account name for Terraform state (must be globally unique, 3-24 lowercase letters/numbers). Created automatically on first run. |
| `server.name` | Yes | `my-app-sqlserver` | Globally unique name for the Azure SQL Server (e.g. `my-app-sqlserver`). Also used as the Terraform state key prefix. |
| `server.resourceGroupName` | Yes | `my-app-rg` | Azure Resource Group for the SQL Server and database |
| `db.name` | Yes | `myappdb` | Name of the SQL Database to create |
| `db.adminUsername` | Yes | `sqladmin` | Administrator login for the SQL Server |
| `db.sku` | No | `Basic` | SKU name for the database (`Basic`, `S0`, `GP_S_Gen5_1`, etc.) |

---

## How to Run

```bash
# 1. Apply the ClusterWorkflowTemplate and Workflow CRs
kubectl apply -f azure-sql-database-create.yaml
```

Edit the `WorkflowRun` section at the bottom of the file with your values, then apply, or create a separate `WorkflowRun`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-app-db-run
spec:
  workflow:
    name: azure-sql-database-create
    parameters:
      git:
        repoUrl: "https://github.com/openchoreo/openchoreo.git"
        branch: "main"
        tfPath: "samples/workflows/azure-sql-database/terraform"
      azure:
        location: "eastus"
        credentialsSecret: "azure-sql-credentials"
      tfState:
        resourceGroup: "openchoreo-tfstate-rg"
        storageAccount: "ocstfstate"
      server:
        name: "my-app-sqlserver"
        resourceGroupName: "my-app-rg"
      db:
        name: "myappdb"
        adminUsername: "sqladmin"
        sku: "Basic"
```

---

## Example Output

```
=================================================
  Azure SQL Database Created
=================================================
Server FQDN:       my-app-sqlserver.database.windows.net
Port:              1433
Database:          myappdb
Admin Username:    sqladmin
Server Resource ID: /subscriptions/.../Microsoft.Sql/servers/my-app-sqlserver
DB Resource ID:    /subscriptions/.../Microsoft.Sql/servers/my-app-sqlserver/databases/myappdb
-------------------------------------------------
Connection String (template):
  Server=tcp:my-app-sqlserver.database.windows.net,1433;Initial Catalog=myappdb;User ID=sqladmin;Password=<password>;Encrypt=True;TrustServerCertificate=False;

NOTE: Password is stored in the 'azure-sql-credentials'
      Kubernetes Secret under the 'adminPassword' key.
      Retrieve it with:
      kubectl get secret azure-sql-credentials \
        -o jsonpath='{.data.adminPassword}' | base64 -d
=================================================

NOTE: The server firewall allows connections from all IP addresses.
```

---

## Deleting the Instance

Use the companion `azure-sql-database-delete` workflow to destroy the SQL Server, database, firewall rules, and resource group. It reads the same Terraform state file from Azure Blob Storage, so no manual cleanup is needed.

```bash
# Apply the delete ClusterWorkflowTemplate and Workflow CRs
kubectl apply -f azure-sql-database-delete.yaml
```

Edit the `WorkflowRun` section at the bottom of `azure-sql-database-delete.yaml` with the **same parameter values used when creating the instance**, then apply:

```bash
kubectl apply -f azure-sql-database-delete.yaml
```

> **Important:** `server.name`, `tfState.storageAccount`, `tfState.resourceGroup`, and `azure.location` must exactly match the values used during creation — Terraform uses these to locate the correct state file.

---

## Terraform State

State is stored at:
```
Azure Storage Account: <tfState.storageAccount>
Container:             tfstate
Blob:                  azuresql/<server.name>/terraform.tfstate
```

Each `server.name` gets its own isolated state key, so running the workflow for a different server does not affect existing instances.
