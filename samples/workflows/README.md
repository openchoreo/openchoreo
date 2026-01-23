# Generic Workflows

This directory contains Generic Workflow definitions that can be used independently of Components. Unlike ComponentWorkflows (which are tied to Component builds), Generic Workflows provide a flexible way to run any type of workflow directly.

## Use Cases

Generic Workflows are useful for:
- **Infrastructure Provisioning** - Terraform, Pulumi, or cloud resource automation
- **Data Processing (ETL)** - Extract, transform, and load pipelines
- **End-to-End Testing** - Integration and acceptance test suites
- **Package Publishing** - Publishing libraries to npm, PyPI, Maven, etc.
- **Docker Builds** - Container image builds not tied to a Component
- **Scheduled Jobs** - Recurring tasks and maintenance operations

## Sample Files

This directory includes a Docker build workflow as an example:

| File | Kind | Description |
|------|------|-------------|
| `cluster-workflow-template-docker-build.yaml` | ClusterWorkflowTemplate | Argo Workflows template with the actual execution steps (clone, build, push) |
| `workflow-docker-build.yaml` | Workflow | OpenChoreo CR that defines the parameter schema and references the template |
| `workflow-run-docker-build.yaml` | WorkflowRun | Triggers an execution with specific parameter values |

## About This Sample

This sample implements a Docker build pipeline that clones a Git repository, builds a container image using Podman, and pushes it to an internal registry. The workflow accepts parameters for the repository URL, branch, commit, Dockerfile path, and build context - allowing you to build images from any repository without modifying the workflow definition.

The Workflow CR (`workflow-docker-build.yaml`) defines the parameter schema and acts as a reusable template. Each time you need to build an image, you create a WorkflowRun with specific parameter values (repository, branch, etc.). The controller then generates an Argo Workflow in the Build Plane that executes the actual steps defined in the ClusterWorkflowTemplate.

This decouples the "what to build" (WorkflowRun parameters) from "how to build" (ClusterWorkflowTemplate steps), making the workflow reusable across different repositories and triggerable on-demand without being tied to any specific Component.

## How to Run

Deploy the resources in order:

```bash
# 1. Deploy the ClusterWorkflowTemplate to the Build Plane
kubectl apply -f cluster-workflow-template-docker-build.yaml

# 2. Deploy the Workflow CR to the Control Plane
kubectl apply -f workflow-docker-build.yaml

# 3. Trigger an execution by creating a WorkflowRun
kubectl apply -f workflow-run-docker-build.yaml
```
