# samples/github-actions

End-to-end example of integrating a GitHub Actions build pipeline with
OpenChoreo using GitHub's OIDC token (no long-lived secrets).

## Files

| File | Purpose |
| --- | --- |
| `register-workload.yml` | Reusable workflow (`workflow_call`) that POSTs a Workload to openchoreo-api. Authenticates via GitHub OIDC. |
| `example-usage.yml` | Drop-in example showing how a caller repository invokes the reusable workflow. |

## How it works

1. The caller's `build` job produces a container image and emits the image reference as a job output.
2. The caller invokes `register-workload.yml` via `uses:`, passing the project, component, environment, image, and the OIDC audience.
3. `register-workload.yml` requests a GitHub OIDC token for the configured audience.
4. The workflow POSTs a `Workload` CR to `POST /api/v1/namespaces/{ns}/workloads` on the OpenChoreo API.
5. openchoreo-api verifies the token against `https://token.actions.githubusercontent.com/.well-known/openid-configuration`, validates the `aud`, `repository`, `ref`, and `job_workflow_ref` claims against the target Project's allow-list, and creates the Workload with `github.com/*` annotations carrying the originating workflow run.

```text
┌──────────────────────────────┐      ┌──────────────────────────────────┐
│ Caller repo workflow         │      │ openchoreo-api                   │
│  • build & push image        │      │  • discovers token.actions.gh    │
│  • uses: register-workload   │      │    well-known + JWKS             │
│      with: image, project,   │ ─────▶  • verifies iss / aud / sig     │
│            component, env    │ POST │  • looks up Project CR          │
│                              │      │  • checks AllowedRepositories,  │
│                              │      │    AllowedRefs,                 │
│                              │      │    AllowedJobWorkflowRefs       │
│                              │      │  • creates Workload + stamps    │
│                              │      │    github.com/* annotations     │
└──────────────────────────────┘      └──────────────────────────────────┘
```

## Platform prerequisites

The platform engineer must have:

- Enabled the GitHub OIDC verifier on openchoreo-api by setting
  `security.authentication.github_oidc.enabled=true` and
  `security.authentication.github_oidc.audience=<your audience>`.
- Created the target Project with a `spec.externalCI.githubActions.allowedRepositories`
  allow-list that includes the caller's `owner/repo`. Example:

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
        # Optional: pin to a single branch
        allowedRefs:
          - refs/heads/main
        # Optional: pin to a single workflow (immutable, strongest trust)
        allowedJobWorkflowRefs:
          - my-org/my-service/.github/workflows/deploy.yml@refs/heads/main
  ```

The OIDC audience MUST match between the platform configuration and the
caller's `oidc_audience` input. A common convention is
`https://github.com/<your-github-org>`.

## Future location

The reusable workflow ships in this repository for the initial release.
A future release may extract it to a dedicated
[`openchoreo/actions`](https://github.com/openchoreo) companion repository
so callers can pin a SemVer tag (`@v1`). The input shape will stay
backwards compatible; only the `uses:` ref will change.

See [#3551](https://github.com/openchoreo/openchoreo/issues/3551) for the
tracking issue.

## Troubleshooting

**`401 unauthorized: invalid GitHub Actions OIDC token`** — the `aud` claim
in the token does not match the audience configured on openchoreo-api.
Check that `oidc_audience` in the caller's `with:` matches
`security.authentication.github_oidc.audience` exactly.

**`403 forbidden`** — the token was accepted but the Project's allow-list
rejected it. Verify that the caller's repository (`owner/repo`), ref, and
(if configured) `job_workflow_ref` appear in
`spec.externalCI.githubActions` on the target Project CR.

**`404 not found` on `POST /workloads`** — the `openchoreo_api_url` input is
wrong, or the namespace path segment does not exist. Check the API URL and
that the target namespace was created.

**`actions/github-script` step fails with `Bad credentials`** — the caller
workflow is missing `permissions: id-token: write`. The reusable workflow
declares this for its own job, but the caller must also grant it at the
workflow or job level if it overrides default permissions.
