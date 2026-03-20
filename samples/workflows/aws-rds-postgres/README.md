# AWS RDS PostgreSQL Create — Generic Workflow Sample

This sample demonstrates a self-service Generic Workflow that lets a developer quickly spin up a throwaway AWS RDS PostgreSQL instance for feature development or testing — without involving the platform team. Fill in a few parameters, trigger the workflow, and get a ready-to-use connection string at the end.

The instance is a minimal, publicly accessible `db.t3.micro` (free-tier eligible) — sized for dev/test, not production. The workflow clones this repository at runtime to execute the Terraform files in `terraform/`. Terraform state is stored in S3 so the same instance can be updated or destroyed later.

---

## Pipeline Overview

```
WorkflowRun
    │
    ▼
[clone-step]    — git clone the configured repo/branch to get terraform/
    │
    ▼
[setup-step]    — create the S3 state bucket if it does not already exist
    │
    ▼
[init-step]     — terraform init with S3 backend
    │
    ▼
[plan-step]     — terraform plan (dry run, visible in logs)
    │
    ▼
[apply-step]    — terraform apply -auto-approve; save outputs to shared volume
    │
    ▼
[report-step]   — print host, port, database, username, password, connection string
```

---

## Infrastructure Provisioned

| Resource | Details |
|----------|---------|
| `aws_db_instance` | PostgreSQL `db.t3.micro` (free-tier eligible), 20 GiB gp2 (AWS minimum), single-AZ |
| `aws_db_subnet_group` | Uses subnets from the default VPC |
| `aws_security_group` | Allows port 5432 inbound from `0.0.0.0/0` |

The instance is **publicly accessible** so you can connect directly from your workstation to verify it. For production use, set `publicly_accessible = false` in `terraform/main.tf` and restrict the security group `cidr_blocks` to your application's IP range.

---

## Prerequisites

### 1. IAM permissions

The IAM user needs two policy statements. The S3 bucket for Terraform state is created automatically by the workflow on first run — no manual bucket creation needed.

**RDS, EC2, and S3:**
```json
{
  "Effect": "Allow",
  "Action": [
    "rds:CreateDBInstance",
    "rds:DeleteDBInstance",
    "rds:DescribeDBInstances",
    "rds:AddTagsToResource",
    "rds:ListTagsForResource",
    "rds:CreateDBSubnetGroup",
    "rds:DescribeDBSubnetGroups",
    "rds:DeleteDBSubnetGroup",
    "ec2:DescribeVpcs",
    "ec2:DescribeVpcAttribute",
    "ec2:DescribeSubnets",
    "ec2:DescribeSecurityGroups",
    "ec2:DescribeNetworkInterfaces",
    "ec2:CreateSecurityGroup",
    "ec2:AuthorizeSecurityGroupIngress",
    "ec2:AuthorizeSecurityGroupEgress",
    "ec2:RevokeSecurityGroupEgress",
    "ec2:DeleteSecurityGroup",
    "ec2:CreateTags",
    "s3:CreateBucket",
    "s3:GetObject",
    "s3:PutObject",
    "s3:ListBucket",
    "s3:DeleteObject"
  ],
  "Resource": "*"
}
```

### 2. Kubernetes Secret

Create the secret in the workflow execution namespace — `workflows-<namespace>` where `<namespace>` is the namespace your `WorkflowRun` is applied in (e.g. `workflows-default`).

The secret holds three values:
- `accessKeyId` — AWS access key ID
- `secretAccessKey` — AWS secret access key
- `dbPassword` — master password for the PostgreSQL instance

```bash
kubectl create secret generic aws-rds-credentials \
  --from-literal=accessKeyId=<your-access-key-id> \
  --from-literal=secretAccessKey=<your-secret-access-key> \
  --from-literal=dbPassword=<your-db-password> \
  --namespace=workflows-default
```

> The DB password is stored in a Kubernetes Secret and injected into the workflow as an environment variable. It is **not** passed as a plain workflow parameter.

---

## Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `git.repoUrl` | No | `https://github.com/openchoreo/openchoreo.git` | Git repository URL (HTTPS) that contains the Terraform files. Override to use your own repo. |
| `git.branch` | No | `main` | Branch or tag to check out |
| `git.tfPath` | No | `samples/workflows/aws-rds-postgres/terraform` | Relative path inside the cloned repo to the directory containing the Terraform files |
| `aws.region` | No | `us-east-1` | AWS region for the RDS instance |
| `aws.credentialsSecret` | Yes | — | Name of the Kubernetes Secret (see Prerequisites) |
| `tfState.s3Bucket` | Yes | — | S3 bucket name for Terraform state. Created automatically on first run if it does not exist. |
| `db.identifier` | Yes | — | Unique RDS instance identifier (e.g. `my-app-db`). Also used as the S3 state key prefix. |
| `db.name` | Yes | — | Initial database name inside the instance |
| `db.username` | Yes | — | Master username |
| `db.engineVersion` | No | `"16"` | PostgreSQL major version (`"16"`, `"15"`, etc.) |

---

## How to Run

```bash
# 1. Apply the ClusterWorkflowTemplate and Workflow CRs
kubectl apply -f aws-rds-postgres-create.yaml
```

Edit the `WorkflowRun` section at the bottom of the file with your values, then apply, or create a separate `WorkflowRun`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-app-db-run
spec:
  workflow:
    name: aws-rds-postgres-create
    parameters:
      # Override these to point to your own repo/branch/terraform path.
      git:
        repoUrl: "https://github.com/openchoreo/openchoreo.git"
        branch: "main"
        tfPath: "samples/workflows/aws-rds-postgres/terraform"
      aws:
        region: "us-east-1"
        credentialsSecret: "aws-rds-credentials"
      tfState:
        s3Bucket: "openchoreo-rds-tfstate"
      db:
        identifier: "my-app-db"
        name: "myappdb"
        username: "dbadmin"
        engineVersion: "16"
```

---

## Example Output

```
=================================================
  AWS RDS PostgreSQL Instance Created
=================================================
Host:              my-app-db.xxxxxxxxxxxx.us-east-1.rds.amazonaws.com
Port:              5432
Database:          myappdb
Username:          dbadmin
ARN:               arn:aws:rds:us-east-1:111122223333:db:my-app-db
-------------------------------------------------
Connection String (template):
  postgresql://dbadmin:<password>@my-app-db.xxxxxxxxxxxx.us-east-1.rds.amazonaws.com:5432/myappdb

NOTE: Password is stored in the 'aws-rds-credentials'
      Kubernetes Secret under the 'dbPassword' key.
      Retrieve it with:
      kubectl get secret aws-rds-credentials \
        -o jsonpath='{.data.dbPassword}' | base64 -d
=================================================

NOTE: The instance is publicly accessible on port 5432.
```

---

## Deleting the Instance

Use the companion `aws-rds-postgres-delete` workflow to destroy the instance and all associated AWS resources (security group, subnet group). It reads the same Terraform state file from S3, so no manual cleanup is needed.

```bash
# Apply the delete ClusterWorkflowTemplate and Workflow CRs
kubectl apply -f aws-rds-postgres-delete.yaml
```

Edit the `WorkflowRun` section at the bottom of `aws-rds-postgres-delete.yaml` with the **same parameter values used when creating the instance**, then apply:

```bash
kubectl apply -f aws-rds-postgres-delete.yaml
```

> **Important:** `db.identifier`, `tfState.s3Bucket`, and `aws.region` must exactly match the values used during creation — Terraform uses these to locate the correct state file.

---

## Terraform State

State is stored at:
```
s3://<tfState.s3Bucket>/rds/<db.identifier>/terraform.tfstate
```

Each `db.identifier` gets its own isolated state key, so running the workflow for a different identifier does not affect existing instances.
