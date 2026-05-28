# GitHub Actions Integration

OpenChoreo integrates with GitHub Actions as an external CI platform. Once enabled, the
Backstage developer portal can read `workflow_run` data for any Component that carries
the `github.com/project-slug` annotation, and (in a follow-up release) workflows running
on GitHub Actions can register a new Workload back into OpenChoreo at the end of a build.

This document covers the **portal-side wiring** that ships in the `openchoreo-control-plane`
Helm chart. The **deployment bridge** (reusable workflow + OIDC validation) is tracked in
the linked follow-up and will be documented in this file when it lands.

> Related: [#3551](https://github.com/openchoreo/openchoreo/issues/3551) — follow-up to
> [#1788](https://github.com/openchoreo/openchoreo/issues/1788).

## Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                  PLATFORM ENGINEER (one-time setup)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│  1. Provision a GitHub PAT (or App installation token) with `repo`          │
│     and `actions:read` scopes.                                              │
│  2. Add it to the Backstage credentials Secret under `github-actions-token`.│
│  3. Enable the integration via Helm values.                                 │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DEVELOPER WORKFLOW                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  Annotate the Component:                                                    │
│    metadata:                                                                │
│      annotations:                                                           │
│        github.com/project-slug: <org>/<repo>                                │
│                                                                              │
│  The Component page in Backstage now surfaces a "GitHub Actions" tab        │
│  showing recent workflow_runs for that repository.                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 1. Provision a GitHub token

The Backstage GitHub integration needs an API token with read access to the repositories
whose workflow_runs you want to surface.

- **For github.com, lightweight setup:** create a [fine-grained personal access token](https://github.com/settings/personal-access-tokens/new)
  scoped to the relevant repositories with `Actions: Read` and `Metadata: Read` permissions.
- **For org-wide / production setup:** create a [GitHub App](https://docs.github.com/en/apps/creating-github-apps),
  install it on the org, and pass the installation token. App tokens scale better and don't
  expire with the user that created them.
- **For GitHub Enterprise Server:** create the token on your GHES instance, then set both
  `host` and `apiBaseUrl` in the Helm values.

## 2. Store the token

Add the token to the existing `backstage-secrets` Secret used by Backstage:

```bash
kubectl -n openchoreo-control-plane patch secret backstage-secrets \
  --type='json' \
  -p='[{"op":"add","path":"/data/github-actions-token","value":"'"$(printf %s "$GITHUB_TOKEN" | base64 | tr -d '\n')"'"}]'
```

If you create the Secret from scratch (k3d local-dev path), include the key directly:

```bash
kubectl create secret generic backstage-secrets \
  -n openchoreo-control-plane \
  --from-literal=backend-secret="$(head -c 32 /dev/urandom | base64)" \
  --from-literal=client-secret="backstage-portal-secret" \
  --from-literal=jenkins-api-key="placeholder-not-in-use" \
  --from-literal=github-actions-token="$GITHUB_TOKEN"
```

The Secret name is taken from `.Values.backstage.secretName`. The key must literally be
`github-actions-token`.

## 3. Enable the integration

### Public GitHub (github.com)

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --namespace openchoreo-control-plane \
  --set backstage.externalCI.githubActions.enabled=true
```

The defaults (`host: github.com`, empty `apiBaseUrl`) are sufficient.

### GitHub Enterprise Server

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --namespace openchoreo-control-plane \
  --set backstage.externalCI.githubActions.enabled=true \
  --set backstage.externalCI.githubActions.host=ghe.example.com \
  --set backstage.externalCI.githubActions.apiBaseUrl=https://ghe.example.com/api/v3
```

## 4. Annotate Components

For any Component whose workflows you want to see in Backstage, set the
`github.com/project-slug` annotation:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-service
  annotations:
    github.com/project-slug: my-org/my-service
spec:
  # ...
```

This is the same annotation that the upstream [`@backstage/plugin-github-actions`](https://www.npmjs.com/package/@backstage/plugin-github-actions)
plugin reads, so any existing Backstage docs that reference it apply unchanged.

## What ships in this release

| Surface                                | Status                                    |
| -------------------------------------- | ----------------------------------------- |
| Helm value `externalCI.githubActions`  | ✅ in this release                         |
| `GITHUB_HOST`, `GITHUB_API_BASE_URL`, `GITHUB_TOKEN` env vars injected into Backstage | ✅ in this release                         |
| `app-config.ci.yaml` `integrations.github` block | ✅ in this release                         |
| Reusable workflow → Workload bridge    | ✅ in this release (`samples/github-actions/`) |
| GitHub OIDC token validation           | ✅ in this release                         |
| Project-level allow-list (`spec.externalCI.githubActions`) | ✅ in this release                         |
| Component-creation wizard option       | 🚧 tracked in `openchoreo/backstage-plugins` |

## Deployment bridge (GitHub Actions → Workload)

Beyond the read-only Backstage tab covered above, openchoreo-api can accept
`POST /workloads` requests authenticated via GitHub Actions OIDC tokens. A
reusable workflow at [`samples/github-actions/register-workload.yml`](../../samples/github-actions/register-workload.yml)
wraps the API call so callers do not need to embed the curl + JSON
themselves.

### 1. Turn on OIDC verification

In your openchoreo-api configuration (typically passed via Helm values for
the `openchoreoApi` chart subsection, or via env vars):

```yaml
security:
  enabled: true
  authentication:
    github_oidc:
      enabled: true
      issuer: https://token.actions.githubusercontent.com   # default
      audience: https://github.com/my-org                   # MUST match the caller's oidc_audience input
```

The middleware performs an OIDC discovery request against `issuer` at
process startup; misconfigured values surface as a startup error rather
than a runtime 401.

### 2. Opt your Project in to a repository

Without a Project-level allow-list, OIDC-authenticated requests are
rejected with `403`. Edit (or create) the target Project CR:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: my-project
  namespace: default
spec:
  deploymentPipelineRef:
    name: default
  externalCI:
    githubActions:
      allowedRepositories:
        - my-org/my-service
      # Optional: also restrict ref and/or workflow file. Empty = any.
      allowedRefs:
        - refs/heads/main
      allowedJobWorkflowRefs:
        - my-org/my-service/.github/workflows/deploy.yml@refs/heads/main
```

`AllowedRepositories` is mandatory; `AllowedRefs` and
`AllowedJobWorkflowRefs` are optional defence-in-depth. The strongest
control is `AllowedJobWorkflowRefs` because GitHub's `job_workflow_ref`
claim is immutable for the duration of a workflow run.

### 3. Call the reusable workflow from the developer's repo

```yaml
jobs:
  register-workload:
    needs: build
    uses: openchoreo/openchoreo/.github/workflows/register-workload.yml@main
    with:
      project: my-project
      component: my-service
      environment: dev
      image: ${{ needs.build.outputs.image }}
      openchoreo_api_url: https://api.openchoreo.example.com
      oidc_audience: https://github.com/my-org
```

A full caller example lives at [`samples/github-actions/example-usage.yml`](../../samples/github-actions/example-usage.yml).

### 4. Workload annotations

The resulting Workload CR carries:

| Annotation | Value |
| --- | --- |
| `ci.openchoreo.dev/ci-platform` | `github-actions` |
| `github.com/repository` | `owner/repo` |
| `github.com/ref` | `refs/heads/main` |
| `github.com/sha` | commit SHA that triggered the run |
| `github.com/run-id` | GitHub workflow run ID |
| `github.com/run-attempt` | GitHub workflow run attempt |
| `github.com/workflow` | workflow display name |
| `github.com/workflow-ref` | mutable workflow ref |
| `github.com/job-workflow-ref` | immutable workflow ref (use for trust) |

`ci.openchoreo.dev/ci-platform` is a platform-owned discriminator that
authorization policies and dashboards can key off without parsing the
`github.com/*` keys. The `github.com/*` keys mirror the names GitHub uses
in the OIDC token so any reader can correlate a Workload back to the
originating workflow run.

### 5. Where the reusable workflow lives

The workflow ships at `samples/github-actions/register-workload.yml` in
this repository. A future release may move it to a dedicated
[`openchoreo/actions`](https://github.com/openchoreo) repository so callers
can pin a SemVer tag (`@v1`); the input shape will stay backwards
compatible. See [#3551](https://github.com/openchoreo/openchoreo/issues/3551).

## Troubleshooting

**The "GitHub Actions" tab shows no runs even though the workflow ran.**
Verify the Component annotation matches the repository slug exactly (`<org>/<repo>`,
case-sensitive). Then check the Backstage pod logs for `Bad credentials` or `403`
responses against the GitHub API; if you see these, the `github-actions-token` Secret
key is missing or the token lacks `Actions: Read` permission.

**The Backstage pod fails to start with "secret key not found".**
The `github-actions-token` Secret key is declared `optional: true` in the Deployment so
the pod will still start without it; if you see this error, your cluster may be running
an older chart version. Upgrade with `helm upgrade` after pulling the latest chart.

**GitHub Enterprise Server users see `Cannot find host` errors.**
Both `host` and `apiBaseUrl` must be set. The plugin uses `host` for `git` URLs and
`apiBaseUrl` for API calls; setting only one will cause the other to fall back to
github.com defaults.
