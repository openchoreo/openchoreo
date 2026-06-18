# Workflow-template contract tests

Contract tests for the build/CI YAMLs in `samples/getting-started/`:

- `workflow-templates/*.yaml`: reusable Argo `ClusterWorkflowTemplate`s whose
  logic lives in embedded shell scripts.
- `ci-workflows/*.yaml`: OpenChoreo `ClusterWorkflow`s that pass component
  parameters into those Argo workflow templates.

These tests load the YAML and assert on the contracts each file must preserve,
so an edit that breaks a supported feature fails a test that names the broken
feature.

Run them (part of the unit tier, no cluster needed):

```bash
go test ./test/workflowtemplates/
```

## Two layers

| File | Layer | What it does |
|------|-------|--------------|
| `static_test.go` | Static | Asserts the extracted script contains the required constructs. Robust against reformatting; catches feature deletion. Covers all 9 templates. |
| `ci_workflows_test.go` | Static | Asserts CI `ClusterWorkflow`s pass repository/build/image/secret parameters into the Argo steps and template refs correctly. Covers all 4 CI workflows. |
| `checkout_behavior_test.go` | Behavioral | Substitutes the Argo `{{...}}` placeholders, stubs `git`/`ssh-keygen` on `PATH`, and runs the real `checkout-source` script with `sh` to prove the URL/credential transformation logic. |
| `build_publish_behavior_test.go` | Behavioral | Runs the real build and publish scripts with temp mounts plus stubbed `podman`, `pack`, and `jq` to prove path validation, build command construction, tar handoff, registry auth, and cloud/k3d push behavior. |
| `generate_workload_behavior_test.go` | Behavioral | Runs the real workload-generation script with stubbed `occ`, `curl`, `yq`, and `jq` to prove source-vs-generated descriptors, OAuth handling, create/update branching, image-only merge, and WorkflowRun annotations. |
| `helper_test.go` | — | YAML loader + script/mount/param extraction helpers. |

The behavioral tests `t.Skip()` if `sh` is unavailable. Tests that exercise
scripts using `echo -n` normalize it to busybox semantics via a shell-function
shim, so they behave the same on macOS/dash and on CI.

## Test cases

### Static — `static_test.go` (covers all 9 templates)

| Test | What it guards |
|------|----------------|
| `TestAllTemplates_ParseAndShape` | Every template parses, is a `ClusterWorkflowTemplate`, has the expected `metadata.name`, and its script starts with `set -e`. |
| `TestCheckoutSource_AuthAndProviderContract` | The `checkout-source` auth/provider contract, via sub-tests: ssh-config lists github/gitlab/bitbucket/codecommit with `StrictHostKeyChecking no`; auth-type detection by secret file (ssh-privatekey / password / ssh-key-id); private-key validation; the host-agnostic https→ssh rewrite; codecommit key-id injection; basic-auth percent-encoding + credential helper; branch and commit checkout modes; the missing-repo / missing-branch-and-commit guards. |
| `TestCheckoutSource_SecretWiring` | `git-secret` mounts at `/etc/secrets/git-secret`, workspace at `/mnt/vol`, and the secret volume is `optional` (so public repos work without it). |
| `TestBuildTemplates_SharedContract` | All 4 build templates: produce `/mnt/vol/app-image.tar`, keep a path-validation guard, and plumb `build-env` JSON → `--env` (skipping empty/`[]`). |
| `TestContainerfileBuild_Specifics` | `podman build` with dockerfile-path/docker-context, `--build-arg` handling, and `podman save` to the tar. |
| `TestBuildpackTemplates_Specifics` | `pack build` flags (`--builder`/`--run-image`/`--pull-policy always`/`--docker-host inherit`), builder/run images pinned by `@sha256:` digest, and the podman-socket + image-exists readiness waits. |
| `TestPublishImage_Cloud` | Pushes to `ttl.sh` with `--tls-verify=true`, loads/tags the tar, writes `/tmp/image.txt`, guards the missing tar, and has the `--authfile` branch. |
| `TestPublishImage_K3d` | k3d variant pushes to the local registry with `--tls-verify=false` and never uses `--tls-verify=true`. |
| `TestPublishImage_SecretWiring` | `registry-push-secret` mounts at `/etc/secrets/registry-push-secret` and is `optional` (anonymous push), for both variants. |
| `TestGenerateWorkload_SharedContract` | `occ workload create`, source-vs-auto descriptor branch, YAML→JSON strip of apiVersion/kind, the OAuth client-credentials call, the 409 create/conflict state machine, the image-only merge, and the WorkflowRun annotation keys. |
| `TestGenerateWorkload_Cloud` | Cloud variant: `oauth-token-url` default is `https://…`, uses `curl -sk`, no Host-header routing. |
| `TestGenerateWorkload_K3d` | k3d variant: `oauth-token-url` default is `http://…`, sends `Host:` headers for oauth + API, uses `curl -s` (no `-k`). |

### Static — `ci_workflows_test.go` (covers all 4 CI workflows)

| Test | What it guards |
|------|----------------|
| `TestCIWorkflows_ParseAndShape` | Every CI workflow parses as `ClusterWorkflow`, is labelled as a component workflow, targets the default workflow plane, and renders an Argo `Workflow` using `workflow-sa` and `build-workflow`. |
| `TestCIWorkflows_RepositoryParametersFeedCheckout` | `repository.url` is required, branch defaults to `main`, commit defaults to empty, and `git-repo`/`branch`/`commit`/`app-path`/`git-secret` workflow parameters reference repository/workflow metadata. |
| `TestCIWorkflows_RunTemplateStepHandoffs` | The Argo step chain calls checkout, build, publish, and generate-workload templates; build/publish receive checkout git revision; generate-workload receives the published image and workflow run name. |
| `TestCIWorkflows_BuildParameters` | Build env, docker context/file path, build args, image name/tag, and registry push secret reference the workflow parameter surface expected by the templates. |
| `TestCIWorkflows_SecretResources` | Git and registry secret resources are generated with names matching the workflow parameters; git credentials are only rendered when `repository.secretRef` is configured. |

### Behavioral — `checkout_behavior_test.go` (runs the real `checkout-source` script)

| Test | What it proves |
|------|----------------|
| `TestCheckout_SSH_HTTPSToSSHRewrite` | github / gitlab / bitbucket https URLs are rewritten to `git@host:path.git` for SSH. |
| `TestCheckout_SSH_CodeCommitKeyIDInjection` | A codecommit ssh URL gets the SSH key id injected: `ssh://<KEYID>@…`. |
| `TestCheckout_SSH_CodeCommitNotRewrittenToScpSyntax` | A codecommit https URL is **not** scp-rewritten (left as https). |
| `TestCheckout_BasicAuth_PercentEncodesCredentials` | `my-user` / `p@ss:w0rd` → `https://my-user:p%40ss%3Aw0rd@…`. |
| `TestCheckout_BasicAuth_DefaultsUsernameToGit` | With only a password, the username defaults to `git`. |
| `TestCheckout_SSHTakesPrecedenceOverBasic` | When both an ssh key and a password exist, the SSH path wins. |
| `TestCheckout_PublicRepo_NoSecret` | No secret → URL untouched, public-repo path taken. |
| `TestCheckout_SecretPresentButUnrecognized_Fails` | A secret with no recognized keys exits 1 with a clear message. |
| `TestCheckout_MissingRepo_Fails` | Empty repo URL exits 1. |
| `TestCheckout_MissingBranchAndCommit_Fails` | Neither branch nor commit exits 1. |
| `TestCheckout_ByBranch_UsesSingleBranchClone` | Branch checkout uses `clone --single-branch --branch <name>`. |
| `TestCheckout_ByCommit_UsesNoCheckoutCloneAndFetch` | Commit checkout uses `clone --no-checkout` then `checkout <sha>`. |

### Behavioral — `build_publish_behavior_test.go`

| Test | What it proves |
|------|----------------|
| `TestContainerfileBuild_Behavior` | Dockerfile builds validate source paths, transform `build-env`/`build-args` JSON into `--env`/`--build-arg`, invoke `podman build`, and save `/mnt/vol/app-image.tar`. |
| `TestContainerfileBuild_MissingDockerfileFailsBeforeBuild` | A missing Dockerfile fails before `podman build` is invoked. |
| `TestBuildpackBuilds_Behavior` | Ballerina, GCP, and Paketo buildpacks start/wait for podman, call `pack build` with digest-pinned builder/run images, pass env flags, wait for the image, and save the tar handoff. |
| `TestBuildpackBuild_MissingAppPathFailsBeforePack` | A missing app path fails before `pack build` is invoked. |
| `TestPublishImage_BehaviorWithAndWithoutAuth` | Cloud and k3d publish variants push to the correct registry/TLS mode, optionally pass `--authfile`, and write `/tmp/image.txt`. |
| `TestPublishImage_LoadsAndTagsBuildOutput` | Cloud and k3d publish variants load `/mnt/vol/app-image.tar` and tag the local image with the target registry endpoint. |
| `TestPublishImage_MissingTarFailsBeforePush` | A missing `/mnt/vol/app-image.tar` fails before any push. |

### Behavioral — `generate_workload_behavior_test.go`

| Test | What it proves |
|------|----------------|
| `TestGenerateWorkload_CreateFromSourceBehavior` | A source `workload.yaml` drives `occ workload create --descriptor`, strips `apiVersion`/`kind`, requests OAuth with scope, posts the workload, and annotates the WorkflowRun with `workload-from-source=true`. |
| `TestGenerateWorkload_AutoGeneratedConflictMergesOnlyImage` | On 409 for an autogenerated workload, the script GETs the existing workload, updates only `.spec.container.image`, preserves other fields, PUTs the merged payload, and annotates `workload-from-source=false`. |
| `TestGenerateWorkload_EmptyOAuthTokenFailsBeforeCreate` | An OAuth response without `access_token` exits before calling the workload API. |

### Helpers — `helper_test.go`

No tests. Provides the YAML loader and extractors used by the suites:
`scriptForTemplate` (extract by Argo template name), CI workflow loading,
`mountPath`, `secretVolumeOptional`, and `inputParamDefault`.

## Scope

These tests guard the **existing** supported providers and features against
regression — they assert that github/gitlab/bitbucket/codecommit and the
current auth/build/publish/workload flows keep working. They intentionally do
**not** assert that a brand-new git provider works out of the box.
