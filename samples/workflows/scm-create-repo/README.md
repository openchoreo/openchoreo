# SCM Create Repository — Generic Workflow Sample

This sample demonstrates Generic Workflows that create a new source code repository via a cloud SCM provider's API. Each provider has its own self-contained YAML file that includes all three required resources: the Argo `ClusterWorkflowTemplate`, the OpenChoreo `Workflow`, and a sample `WorkflowRun`.

The workflows have no dependency on any Component and can be triggered manually.

---

## Available Providers

| File | Provider | Description |
|------|----------|-------------|
| [`github-create-repo.yaml`](./github-create-repo.yaml) | GitHub | Creates a repository under a GitHub organization |
| [`codecommit-create-repo.yaml`](./codecommit-create-repo.yaml) | AWS CodeCommit | Creates a repository in an AWS CodeCommit region |

---

## Pipeline Overview

Both workflows follow the same two-step pattern:

```
WorkflowRun
    │
    ▼
[create-step]  — calls the provider API → response.json (shared volume)
    │
    ▼
[report-step]  — prints repository details (name, clone URLs, etc.)
```

---

## GitHub

### Prerequisites

The workflow authenticates with GitHub using a Personal Access Token (PAT). The token must have the **`repo`** scope (for private repositories) or **`public_repo`** scope (for public repositories). If creating repositories under an organization, the token also needs the **`admin:org`** scope.

The secret must be created in the workflow plane namespace where Argo executes the workflow steps — `workflows-<namespace>` (e.g. `workflows-default` if your `WorkflowRun` is in the `default` namespace):

```bash
kubectl create secret generic github-token \
  --from-literal=token=<your-github-pat> \
  --namespace=workflows-default
```

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `github.owner` | Yes | — | GitHub organization or user that will own the repository |
| `github.tokenSecret` | Yes | — | Name of the Kubernetes Secret containing the PAT under the key `token` (e.g. `github-token`) |
| `repo.name` | Yes | — | Name of the repository to create |
| `repo.description` | No | `""` | Short description of the repository |
| `repo.visibility` | No | `private` | Repository visibility: `public` or `private` |
| `repo.autoInit` | No | `"true"` | Initialize the repository with an empty README: `"true"` or `"false"` |
| `repo.gitignoreTemplate` | No | `""` | Gitignore template to apply (e.g. `Go`, `Python`, `Node`) |
| `repo.licenseTemplate` | No | `""` | License template to apply (e.g. `mit`, `apache-2.0`) |

### How to Run

```bash
# 1. Apply all three resources (ClusterWorkflowTemplate, Workflow, and WorkflowRun)
kubectl apply -f github-create-repo.yaml
```

To customise the run, edit the `WorkflowRun` section at the bottom of `github-create-repo.yaml` before applying, or create a separate `WorkflowRun`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-github-repo-run
spec:
  workflow:
    name: scm-github-create-repo
    parameters:
      github:
        owner: "my-org"
        tokenSecret: "github-token"
      repo:
        name: "my-service"
        description: "Backend service for the payments platform"
        visibility: "private"
        autoInit: "true"
        gitignoreTemplate: "Go"
        licenseTemplate: "apache-2.0"
```

### Example Output

```
=================================
  GitHub Repository Created
=================================
Name:           my-org/my-service
URL:            https://github.com/my-org/my-service
Clone (HTTPS):  https://github.com/my-org/my-service.git
Clone (SSH):    git@github.com:my-org/my-service.git
Visibility:     private
Default Branch: main
Description:    Backend service for the payments platform
Created At:     2026-03-06T10:00:00Z
=================================
```

### Common Gitignore Templates

| Language / Framework | Value |
|----------------------|-------|
| Go | `Go` |
| Python | `Python` |
| Node.js | `Node` |
| Java | `Java` |
| Rust | `Rust` |

For the full list see the [GitHub gitignore templates repository](https://github.com/github/gitignore).

### Common License Templates

| License | Value |
|---------|-------|
| MIT | `mit` |
| Apache 2.0 | `apache-2.0` |
| GNU GPLv3 | `gpl-3.0` |
| BSD 2-Clause | `bsd-2-clause` |

For the full list see the [GitHub license API](https://docs.github.com/en/rest/licenses/licenses).

---

## AWS CodeCommit

### Prerequisites

The workflow authenticates with AWS using an IAM user's access key. The IAM user must have the **`codecommit:CreateRepository`** permission.

Create the secret in the workflow plane namespace:

```bash
kubectl create secret generic aws-credentials \
  --from-literal=accessKeyId=<your-access-key-id> \
  --from-literal=secretAccessKey=<your-secret-access-key> \
  --namespace=workflows-default
```

### Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `aws.region` | Yes | — | AWS region where the repository will be created (e.g. `us-east-1`) |
| `aws.credentialsSecret` | Yes | — | Name of the Kubernetes Secret containing `accessKeyId` and `secretAccessKey` keys (e.g. `aws-credentials`) |
| `repo.name` | Yes | — | Name of the CodeCommit repository to create |
| `repo.description` | No | `""` | Short description of the repository |

### How to Run

```bash
# 1. Apply all three resources (ClusterWorkflowTemplate, Workflow, and WorkflowRun)
kubectl apply -f codecommit-create-repo.yaml
```

To customise the run, edit the `WorkflowRun` section at the bottom of `codecommit-create-repo.yaml` before applying, or create a separate `WorkflowRun`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-codecommit-repo-run
spec:
  workflow:
    name: scm-codecommit-create-repo
    parameters:
      aws:
        region: "us-east-1"
        credentialsSecret: "aws-credentials"
      repo:
        name: "my-service"
        description: "Backend service for the payments platform"
```

### Example Output

```
=================================
  CodeCommit Repository Created
=================================
Name:          my-service
Description:   Backend service for the payments platform
Repository ID: a1b2c3d4-5678-90ab-cdef-EXAMPLE11111
ARN:           arn:aws:codecommit:us-east-1:111122223333:my-service
Clone (HTTPS): https://git-codecommit.us-east-1.amazonaws.com/v1/repos/my-service
Clone (SSH):   ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/my-service
Account ID:    111122223333
=================================
```

---

## Triggering a Run via the OpenChoreo Console

Each YAML file in this sample includes a `WorkflowRun` that can be applied directly with `kubectl`. Alternatively, runs can be triggered from the OpenChoreo Console:

1. Open the **OpenChoreo Console** and navigate to the **Catalog** and filter workflows
2. Find the workflow by name (e.g. `scm-github-create-repo` or `scm-codecommit-create-repo`) — it will be listed under the **Generic** type
3. Select the workflow and go to the **Runs** tab
4. Click **New Run** and fill in the parameter values in the YAML editor
5. Submit to trigger the execution
