# Go Greeter Service (Manual Deploy with occ CLI)

## Overview

This sample demonstrates how to deploy a Go REST service in OpenChoreo using the `occ` CLI with manual control over each stage: build, deploy, and promote.

The component is created with `autoDeploy: false`, so builds and deployments must be triggered explicitly.

The service exposes one REST endpoint.

Exposed REST endpoints:

### Greet a user

**Endpoint:** `/greeter/greet`
**Method:** `GET`
**Functionality:** Sends a greeting to the user.

The source code is available at:
https://github.com/openchoreo/sample-workloads/tree/main/service-go-greeter

## Prerequisites

- A running OpenChoreo cluster (see [Getting Started](https://openchoreo.dev/docs/getting-started/quick-start-guide/))
- The `occ` CLI installed and configured (see [CLI Installation](https://openchoreo.dev/docs/developer-guide/cli-installation/))
- Logged in via `occ login`
- [`yq`](https://github.com/mikefarah/yq#install) installed (used to extract the invoke URL in Step 5)

## Step 1: Apply the Component

The following command will create the component in OpenChoreo with `autoDeploy: false`. This registers the component but does not trigger a build or deployment automatically.

```bash
occ apply -f samples/occ-cli/go-greeter-manual-deploy/greeter-service.yaml
```

Verify the component was created:

```bash
occ component list
```

## Step 2: Trigger a Workflow Run (Build)

Run the component's configured workflow to build the container image:

```bash
occ component workflow run greeter-service
```

## Step 3: Monitor the Build

List workflow runs for the component:

```bash
occ component workflowrun list greeter-service
```

Follow the build logs:

```bash
occ component workflowrun logs greeter-service
```

> [!NOTE]
> The Docker build typically takes 3-5 minutes depending on network speed and system resources.

## Step 4: Deploy to Development

Once the build succeeds, deploy the component to the root environment (development):

```bash
occ component deploy greeter-service
```

Check the release bindings:

```bash
occ releasebinding list
```

## Step 5: Test the Application

Wait for the release binding status to show `Ready` before proceeding:

```bash
occ releasebinding list
```

Get the invoke URL from the release binding status and test the endpoint:

```bash
INVOKE_URL=$(occ releasebinding get greeter-service-development | yq '.status.endpoints[0].externalURLs.http | .scheme + "://" + .host + ":" + (.port | tostring) + .path')
```

```bash
curl "$INVOKE_URL/greeter/greet"
```

```bash
curl "$INVOKE_URL/greeter/greet?name=Alice"
```

## Step 6: Check Component Logs

Verify the component is running by checking its runtime logs:

```bash
occ component logs greeter-service
```

```bash
# Logs from a specific environment
occ component logs greeter-service --env development
```

## Step 7: Promote to Staging

Promote the component to the next environment:

```bash
occ component deploy greeter-service --to staging
```

Check the release binding status:

```bash
occ releasebinding list
```

## Step 8: Promote to Production

```bash
occ component deploy greeter-service --to production --set spec.componentTypeEnvironmentConfigs.replicas=2
```

## Clean Up

Remove all resources:

```bash
occ component delete greeter-service
```
