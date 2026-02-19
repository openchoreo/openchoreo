# Getting Started Resources

Default resources for OpenChoreo. Apply these after installing the control plane to set up a working environment with projects, environments, component types, and build workflows.

## Quick Start

Apply all resources with a single command:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/getting-started/all.yaml
```

Or if you have cloned the repository:

```bash
kubectl apply -f samples/getting-started/all.yaml
```

## Verify Installation

```bash
kubectl get project,environment,deploymentpipeline,componenttype,componentworkflow,trait -n default
```

Expected output:

```
NAME                                   AGE
project.openchoreo.dev/default         10s

NAME                                       AGE
environment.openchoreo.dev/development     10s
environment.openchoreo.dev/production      10s
environment.openchoreo.dev/staging         10s

NAME                                           AGE
deploymentpipeline.openchoreo.dev/default      10s

NAME                                        AGE
componenttype.openchoreo.dev/scheduled-task    10s
componenttype.openchoreo.dev/service           10s
componenttype.openchoreo.dev/web-application   10s
componenttype.openchoreo.dev/worker            10s

NAME                                                   AGE
componentworkflow.openchoreo.dev/ballerina-buildpack   10s
componentworkflow.openchoreo.dev/docker                10s
componentworkflow.openchoreo.dev/google-cloud-buildpacks   10s
componentworkflow.openchoreo.dev/react                 10s

NAME                                        AGE
trait.openchoreo.dev/api-configuration      10s
```

## What Gets Created

### Project and Pipeline

- **default** project with a deployment pipeline that promotes through development -> staging -> production

### Environments

| Name | DNS Prefix | Production |
|------|------------|------------|
| development | dev | No |
| staging | staging | No |
| production | prod | Yes |

### Component Types

| Name | Workload Type | Build Workflows | Validation |
|------|---------------|-----------------|------------|
| worker | Deployment | docker, google-cloud-buildpacks | No endpoints |
| service | Deployment | docker, google-cloud-buildpacks, ballerina-buildpack | At least 1 endpoint |
| web-application | Deployment | react, docker | HTTP/REST endpoint required |
| scheduled-task | CronJob | docker, google-cloud-buildpacks | - |

### Component Workflows

| Name | Description |
|------|-------------|
| docker | Build using Dockerfile |
| react | Build React web applications |
| ballerina-buildpack | Build Ballerina applications |
| google-cloud-buildpacks | Build using Google Cloud Buildpacks |

### Traits

| Name | Description |
|------|-------------|
| api-configuration | Configure API gateway routing and policies |

## Individual Files

For applying resources selectively:

```
getting-started/
├── all.yaml                    # Combined manifest
├── project.yaml                # Default project
├── environments.yaml           # Development, staging, production
├── deployment-pipeline.yaml    # Promotion pipeline
├── component-types/
│   ├── worker.yaml
│   ├── service.yaml
│   ├── webapp.yaml
│   └── scheduled-task.yaml
├── component-workflows/
│   ├── docker.yaml
│   ├── react.yaml
│   ├── ballerina-buildpack.yaml
│   └── google-cloud-buildpacks.yaml
└── component-traits/
    └── api-management.yaml
```

## Customization

These are default configurations. You can:

1. Modify the YAML files before applying
2. Apply individual files instead of `all.yaml`
3. Create your own resources using these as templates

See the [Platform Configuration](../platform-config/) samples for more customization examples.
