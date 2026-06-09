# Linter Custom Step — Spectral

This sample demonstrates how to add an API linting step to an OpenChoreo CI workflow using [Spectral](https://stoplight.io/open-source/spectral), an open-source linter for OpenAPI and AsyncAPI specifications.

## Files

| File | Description |
|------|-------------|
| `spectral-lint.yaml` | `ClusterWorkflowTemplate` that runs Spectral against an API spec file |
| `dockerfile-builder-linter.yaml` | `ClusterWorkflow` that extends `dockerfile-builder` with a Spectral lint step |
| `.spectral.yaml` | Sample ruleset that augments Spectral's built-in OAS rules with the OWASP API Security Top 10 ruleset |

## Pipeline

The `dockerfile-builder-linter` workflow runs these steps in order:

```
checkout-source → spectral-lint → build-image → publish-image → generate-workload-cr
```

If `spectral-lint` fails, the pipeline stops and the build is not triggered.

## Parameters

The `dockerfile-builder-linter` workflow accepts all the same parameters as `dockerfile-builder`, plus a `spectral` block:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `spectral.apiSpecPath` | `./openapi.yaml` | Path to the OpenAPI or AsyncAPI spec file of the component, relative to the repository root |
| `spectral.rulesetPath` | `""` | Path to a custom Spectral ruleset file, relative to the repository root. |

## Applying the Samples

### Step 1 — Apply the ClusterWorkflowTemplate

The `ClusterWorkflowTemplate` defines the reusable lint step and must be applied before the `ClusterWorkflow` that references it:

```bash
kubectl apply -f spectral-lint.yaml
```

Verify it was created:

```bash
kubectl get clusterworkflowtemplate spectral-lint
```

### Step 2 — Apply the ClusterWorkflow

```bash
kubectl apply -f dockerfile-builder-linter.yaml
```

Verify it was created:

```bash
kubectl get clusterworkflow dockerfile-builder-linter
```

### Step 3 — Add the Workflow to the Allowed List for your Component Type

`ClusterComponentType` resources have an explicit `allowedWorkflows` list. The new workflow will not appear in the UI dropdown or be selectable when creating a component until it is added to the relevant component type.

Check which component type you are using (e.g. `service`, `web-application`, `worker`) and patch it:

```bash
kubectl patch clustercomponenttype service --type=json -p='[
  {
    "op": "add",
    "path": "/spec/allowedWorkflows/-",
    "value": {"kind": "ClusterWorkflow", "name": "dockerfile-builder-linter"}
  }
]'
```

Verify it was added:

```bash
kubectl get clustercomponenttype service -o jsonpath='{.spec.allowedWorkflows[*].name}'
```

Repeat for any other component types (`web-application`, `worker`, etc.) that should also use this workflow.

### Step 4 — Create or Update your Component

#### Option A — New component via the UI

1. In the Backstage portal, navigate to your project and click **Create Component**
2. Select your component type (e.g. `service`)
3. In the **CI Workflow** dropdown, select `dockerfile-builder-linter` — this appears once Step 3 is complete
4. Fill in the repository details and the `spectral` parameters (`apiSpecPath`, `rulesetPath`)
5. Submit — the component is created
6. Trigger a build.

#### Option B — Updating an existing component via YAML

In your `Component` manifest, change `workflow.name` from `dockerfile-builder` to `dockerfile-builder-linter` and add the `spectral` parameters block:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-service
  namespace: default
spec:
  owner:
    projectName: default
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder-linter
    parameters:
      repository:
        url: "https://github.com/my-org/my-repo"
        revision:
          branch: "main"
      docker:
        context: "."
        filePath: "./Dockerfile"
      spectral:
        apiSpecPath: ./openapi.yaml
        rulesetPath: ./.spectral.yaml   # omit to use Spectral defaults
```

Apply it:

```bash
kubectl apply -f my-service.yaml
```

### Step 5 — Trigger a WorkflowRun

Create a `WorkflowRun` to test the pipeline:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-service-build-01
  labels:
    openchoreo.dev/project: "default"
    openchoreo.dev/component: "my-service"
spec:
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder-linter
    parameters:
      repository:
        url: "https://github.com/my-org/my-repo"
        revision:
          branch: "main"
      docker:
        context: "."
        filePath: "./Dockerfile"
      spectral:
        apiSpecPath: ./openapi.yaml
        rulesetPath: ./.spectral.yaml
```

Apply it:

```bash
kubectl apply -f workflowrun.yaml
```

Watch the steps progress in the Argo Workflows UI or via kubectl:

```bash
kubectl get workflowrun my-service-build-01 -w
```

If the `spectral-lint` step fails, check the logs to see which rules were violated:

```bash
kubectl logs -l workflows.argoproj.io/workflow=my-service-build-01 -c main
```

## Using a Custom Ruleset

Spectral supports custom rulesets defined in `.spectral.yaml` (or `.json`, `.js`). To use one from your repository, set `spectral.rulesetPath` to its path relative to the repo root:

```yaml
spectral:
  apiSpecPath: ./api/openapi.yaml
  rulesetPath: ./.spectral.yaml
```

### Augmenting with the OWASP API Security Top 10

The sample `.spectral.yaml` in this directory extends both the built-in OAS ruleset and the [OWASP API Security Top 10 ruleset](https://github.com/stoplightio/spectral-owasp-ruleset), so all rules from both are applied:

```yaml
extends:
  - "spectral:oas"
  - "@stoplight/spectral-owasp-ruleset"
```

Copy this file into your repository root and set `spectral.rulesetPath` to `./.spectral.yaml`. The lint step installs `@stoplight/spectral-owasp-ruleset` automatically before running, so no additional setup is needed.

You can also selectively turn off individual rules:

```yaml
extends:
  - "spectral:oas"
  - "@stoplight/spectral-owasp-ruleset"
rules:
  info-contact: off
```
