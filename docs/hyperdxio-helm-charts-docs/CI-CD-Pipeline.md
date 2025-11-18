# CI/CD Pipeline

> **Relevant source files**
> * [.github/workflows/chart-test.yml](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml)
> * [.github/workflows/release.yml](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml)
> * [charts/hdx-oss-v2/tests/helpers_test.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/tests/helpers_test.yaml)
> * [package.json](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json)
> * [scripts/smoke-test.sh](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh)

## Purpose and Scope

This document describes the continuous integration and continuous deployment (CI/CD) pipeline for the HyperDX Helm Charts repository. The pipeline automates testing, versioning, and publishing of Helm charts using GitHub Actions workflows. The system uses the Changesets tool for semantic versioning and the Helm Chart Releaser for distribution via GitHub Pages.

For information about the chart versioning strategy and metadata, see [Chart Metadata and Packaging](/hyperdxio/helm-charts/9.5-chart-metadata-and-packaging). For details on the testing methodology, see [Testing](/hyperdxio/helm-charts/9.2-testing). For the overall release management process, see [Release Management](/hyperdxio/helm-charts/9.3-release-management).

## Pipeline Architecture

The CI/CD pipeline consists of three primary workflows that work together to ensure code quality and automate releases:

```mermaid
flowchart TD

Push["Push to main"]
PR["Pull Request"]
Schedule["Nightly Schedule<br>2 AM UTC"]
Manual["workflow_dispatch"]
UnitTest["chart-test.yml<br>Helm Unit Tests<br>helm unittest"]
IntegrationTest["chart-test.yml<br>Integration Tests<br>Kind Cluster + Full Deploy"]
SmokeTest["scripts/smoke-test.sh<br>Smoke Tests<br>Endpoint + Data Validation"]
ReleaseWorkflow["release.yml<br>Release Workflow"]
ChangesetAction["changesets/action@v1<br>Version Management"]
VersionScript["scripts/update-chart-versions.js<br>Chart.yaml Sync"]
ChartReleaser["helm/Unsupported markdown: link<br>Publish to GitHub Pages"]
ReleasePR["Release PR<br>Version Bump"]
GitHubRelease["GitHub Release<br>Tagged Version"]
HelmRepo["Helm Repository<br>index.yaml on gh-pages"]
Changelog["CHANGELOG.md<br>Version History"]

Push --> UnitTest
PR --> UnitTest
Schedule --> UnitTest
Manual --> UnitTest
SmokeTest --> ReleaseWorkflow
ChangesetAction --> ReleasePR
ChangesetAction --> Changelog
ReleasePR --> ChartReleaser
ChartReleaser --> GitHubRelease
ChartReleaser -->|"Merged"| HelmRepo

subgraph Outputs ["Outputs"]
    ReleasePR
    GitHubRelease
    HelmRepo
    Changelog
end

subgraph subGraph2 ["Release Phase"]
    ReleaseWorkflow
    ChangesetAction
    VersionScript
    ChartReleaser
    ReleaseWorkflow --> ChangesetAction
    ChangesetAction --> VersionScript
end

subgraph subGraph1 ["Testing Phase"]
    UnitTest
    IntegrationTest
    SmokeTest
    UnitTest --> IntegrationTest
    IntegrationTest --> SmokeTest
end

subgraph Triggers ["Triggers"]
    Push
    PR
    Schedule
    Manual
end
```

**Sources:** [.github/workflows/chart-test.yml L1-L184](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L1-L184)

 [.github/workflows/release.yml L1-L51](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L1-L51)

 [package.json L1-L19](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json#L1-L19)

## Testing Workflows

### Chart Test Workflow

The `chart-test.yml` workflow provides comprehensive testing of the Helm chart across multiple stages. It is triggered by pushes to main, pull requests, nightly schedules, and manual invocations.

```mermaid
flowchart TD

Checkout["actions/checkout@v4<br>Fetch Repository"]
SetupHelm["azure/setup-helm@v3<br>Helm 3.12.0"]
NightlyUpdate["Update appVersion<br>to 2-nightly<br>if schedule trigger"]
KindConfig["Create kind-config.yaml<br>Port mappings:<br>30000:3000, 30001:4318"]
CreateCluster["helm/kind-action@v1<br>Create cluster:<br>hyperdx-test"]
StorageProvisioner["Install local-path-provisioner<br>Set as default StorageClass"]
HelmUnitTest["helm unittest<br>charts/hdx-oss-v2<br>Run template tests"]
ChartDeploy["helm install hyperdx-test<br>with test-values.yaml<br>Reduced resources for CI"]
MongoBootstrap["kubectl exec mongodb<br>Insert test team<br>apiKey: test-api-key-for-ci"]
VerifyDeploy["kubectl wait<br>for condition=Ready<br>timeout=600s"]
SmokeTests["./scripts/smoke-test.sh<br>Comprehensive endpoint tests"]
CollectLogs["kubectl logs<br>Collect on failure"]
Uninstall["helm uninstall<br>kind delete cluster"]

NightlyUpdate --> KindConfig
StorageProvisioner --> HelmUnitTest
SmokeTests --> Uninstall
SmokeTests --> CollectLogs

subgraph Cleanup ["Cleanup"]
    CollectLogs
    Uninstall
    CollectLogs --> Uninstall
end

subgraph subGraph2 ["Test Execution"]
    HelmUnitTest
    ChartDeploy
    MongoBootstrap
    VerifyDeploy
    SmokeTests
    HelmUnitTest --> ChartDeploy
    ChartDeploy --> MongoBootstrap
    MongoBootstrap --> VerifyDeploy
    VerifyDeploy --> SmokeTests
end

subgraph subGraph1 ["Infrastructure Setup"]
    KindConfig
    CreateCluster
    StorageProvisioner
    KindConfig --> CreateCluster
    CreateCluster --> StorageProvisioner
end

subgraph subGraph0 ["Setup Phase"]
    Checkout
    SetupHelm
    NightlyUpdate
    Checkout --> SetupHelm
    SetupHelm --> NightlyUpdate
end
```

**Key configuration details:**

| Configuration | Value | Purpose |
| --- | --- | --- |
| Kind cluster name | `hyperdx-test` | Test cluster identifier |
| Port mappings | 30000→3000, 30001→4318 | Expose UI and OTEL endpoints |
| Storage class | `local-path` | PVC provisioning in Kind |
| Test API key | `test-api-key-for-ci` | Authentication for test telemetry |
| Helm timeout | 5m | Chart installation timeout |
| Pod wait timeout | 600s | Maximum time for pods to become ready |

**Sources:** [.github/workflows/chart-test.yml L16-L184](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L16-L184)

### Unit Tests

The workflow runs Helm unit tests using the `helm-unittest` plugin, which validates template rendering with various values configurations:

```markdown
# Executed at .github/workflows/chart-test.yml:64-67
helm plugin install https://github.com/helm-unittest/helm-unittest.git
helm unittest charts/hdx-oss-v2
```

Test files are located at `charts/hdx-oss-v2/tests/` and validate helper templates, resource generation, and configuration injection patterns.

**Sources:** [.github/workflows/chart-test.yml L64-L67](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L64-L67)

 [charts/hdx-oss-v2/tests/helpers_test.yaml L1-L50](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/tests/helpers_test.yaml#L1-L50)

### Integration Tests

The integration test phase deploys a complete HyperDX stack to a Kind Kubernetes cluster with optimized test values:

```yaml
# Test values used for CI deployment
hyperdx:
  apiKey: "test-api-key-for-ci"
  frontendUrl: "http://localhost:3000"
  replicas: 1
  service:
    type: NodePort
    nodePort: 30000

clickhouse:
  persistence:
    enabled: true
    dataSize: 2Gi
    logSize: 1Gi

mongodb:
  persistence:
    enabled: true
    dataSize: 2Gi

otel:
  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "256Mi"
      cpu: "200m"
```

The test includes MongoDB bootstrapping to create a test team, which is required for the OpAMP server to configure collectors properly.

**Sources:** [.github/workflows/chart-test.yml L69-L138](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L69-L138)

### Smoke Tests

The smoke test script validates the deployed system through multiple checks:

```mermaid
flowchart TD

PodCheck["Pod Status Check<br>kubectl wait<br>condition=Ready"]
UITest["UI Endpoint Test<br>curl localhost:3000<br>expect HTTP 200"]
MetricsTest["OTEL Metrics Test<br>curl localhost:8888/metrics<br>expect HTTP 200"]
IngestTest["Data Ingestion Test<br>POST /v1/logs<br>POST /v1/traces"]
CHTest["ClickHouse Test<br>clickhouse-client<br>SELECT 1"]
MongoTest["MongoDB Test<br>mongosh<br>ismaster command"]
DataVerify["Data Verification<br>Query otel_logs<br>Query otel_traces"]

subgraph subGraph0 ["Smoke Test Stages"]
    PodCheck
    UITest
    MetricsTest
    IngestTest
    CHTest
    MongoTest
    DataVerify
    PodCheck --> UITest
    UITest --> MetricsTest
    MetricsTest --> IngestTest
    IngestTest --> CHTest
    CHTest --> MongoTest
    MongoTest --> DataVerify
end
```

The smoke test script uses `kubectl port-forward` to access internal services and validates end-to-end telemetry flow by sending test OTLP data and querying ClickHouse for ingested records.

**Sources:** [scripts/smoke-test.sh L1-L202](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L1-L202)

 [.github/workflows/chart-test.yml L152-L156](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L152-L156)

## Release Workflow

The `release.yml` workflow automates chart publishing and is triggered only after successful test workflow completion on the main branch.

```mermaid
flowchart TD

TestSuccess["workflow_run completed<br>Workflows:<br>- Helm Chart Tests<br>- Helm Chart Integration Test"]
BranchCheck["Branch: main<br>Conclusion: success"]
Concurrency["concurrency:<br>workflow-ref<br>Prevent parallel releases"]
Checkout["actions/checkout@v2<br>fetch-depth: 0<br>Full history for tags"]
GitConfig["Configure Git<br>user: GITHUB_ACTOR<br>email: noreply"]
NodeSetup["setup-node@v3<br>Node.js 20<br>corepack enable"]
YarnInstall["yarn install<br>Install dependencies:<br>@changesets/cli, js-yaml"]
ChangesetPR["changesets/action@v1<br>version: yarn run version<br>publish: yarn run release"]
ChartRelease["helm/Unsupported markdown: link<br>If no changesets pending"]
VersionPR["Release PR<br>chore(release): bump version"]
PublishAction["changeset publish<br>Create Git tags"]
HelmPackage["Package charts<br>Publish to gh-pages<br>Update index.yaml"]

BranchCheck --> Concurrency
Concurrency --> Checkout
ChangesetPR --> VersionPR
VersionPR --> ChartRelease
ChartRelease --> PublishAction
ChartRelease --> HelmPackage

subgraph Outputs ["Outputs"]
    VersionPR
    PublishAction
    HelmPackage
end

subgraph subGraph2 ["Release Job Steps"]
    Checkout
    GitConfig
    NodeSetup
    YarnInstall
    ChangesetPR
    ChartRelease
    Checkout --> GitConfig
    GitConfig --> NodeSetup
    NodeSetup --> YarnInstall
    YarnInstall --> ChangesetPR
    ChangesetPR --> ChartRelease
end

subgraph subGraph1 ["Concurrency Control"]
    Concurrency
end

subgraph subGraph0 ["Workflow Triggers"]
    TestSuccess
    BranchCheck
    TestSuccess --> BranchCheck
end
```

**Sources:** [.github/workflows/release.yml L1-L51](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L1-L51)

### Workflow Configuration

| Setting | Value | Purpose |
| --- | --- | --- |
| Trigger | `workflow_run` | Run after test workflows complete |
| Condition | `conclusion == 'success'` | Only proceed if tests pass |
| Branch | `main` | Only release from main branch |
| Concurrency group | `${{ github.workflow }}-${{ github.ref }}` | Prevent parallel releases |
| Permissions | `contents: write``pull-requests: write` | Create releases and PRs |
| Node version | 20 | Required for changesets CLI |

**Sources:** [.github/workflows/release.yml L2-L15](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L2-L15)

## Version Management with Changesets

The pipeline uses the Changesets tool for semantic versioning. This provides a structured approach to version bumping and changelog generation.

```mermaid
flowchart TD

ChangesetFile["Developer creates<br>.changeset/*.md<br>Describes changes"]
CommitPush["Commit and push<br>to feature branch"]
DetectChangesets["changesets/action@v1<br>Detect pending changesets"]
RunVersion["Execute: yarn run version<br>Command: changeset version"]
UpdateVersions["Update package.json version<br>Generate CHANGELOG.md"]
RunScript["Execute: npm run update-chart-versions<br>Script: update-chart-versions.js"]
UpdateChart["Update Chart.yaml:<br>- version<br>- appVersion"]
CreatePR["Create Release PR<br>Title: Release HyperDX Helm Charts<br>Commit: chore(release): bump version"]
RunPublish["Execute: yarn run release<br>Command: changeset publish"]
CreateTags["Create Git tags<br>Push to GitHub"]

CommitPush --> DetectChangesets
UpdateChart --> CreatePR
CreatePR --> RunPublish

subgraph Post-Merge ["Post-Merge"]
    RunPublish
    CreateTags
    RunPublish --> CreateTags
end

subgraph subGraph2 ["PR Creation"]
    CreatePR
end

subgraph subGraph1 ["Changesets Action"]
    DetectChangesets
    RunVersion
    UpdateVersions
    RunScript
    UpdateChart
    DetectChangesets --> RunVersion
    RunVersion --> UpdateVersions
    UpdateVersions --> RunScript
    RunScript --> UpdateChart
end

subgraph subGraph0 ["Developer Workflow"]
    ChangesetFile
    CommitPush
    ChangesetFile --> CommitPush
end
```

**Sources:** [.github/workflows/release.yml L36-L45](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L36-L45)

 [package.json L9-L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json#L9-L12)

### Version Script

The `update-chart-versions.js` script synchronizes the Helm chart version with the package.json version:

```sql
// Executed by: npm run update-chart-versions
// Script location: scripts/update-chart-versions.js
// Purpose: Keep Chart.yaml version in sync with package.json
```

This script reads the version from `package.json`, parses `charts/hdx-oss-v2/Chart.yaml` using `js-yaml`, updates the `version` field, and writes the file back.

**Sources:** [package.json L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json#L12-L12)

### Package.json Scripts

The npm scripts coordinate the version management process:

| Script | Command | Purpose |
| --- | --- | --- |
| `version` | `changeset version && npm run update-chart-versions` | Bump versions and sync Chart.yaml |
| `release` | `changeset publish` | Create Git tags and publish |
| `update-chart-versions` | `node scripts/update-chart-versions.js` | Sync Chart.yaml with package.json |

**Sources:** [package.json L9-L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json#L9-L12)

## Chart Publishing with Chart Releaser

After a release PR is merged and there are no pending changesets, the workflow publishes charts using the Helm Chart Releaser action.

```mermaid
flowchart TD

Condition["Check:<br>steps.changesets.outputs.hasChangesets == 'false'"]
Action["helm/Unsupported markdown: link<br>env: CR_TOKEN"]
Scan["Scan charts directory<br>Find modified charts"]
Package["helm package<br>Create .tgz archives"]
GHRelease["Create GitHub Release<br>Upload chart archives<br>Tag: chart-version"]
GHPages["Checkout gh-pages branch<br>Update index.yaml<br>Add new chart version"]
Push["Push to gh-pages<br>Publish Helm repository"]
ReleaseAsset["GitHub Release<br>Chart .tgz file<br>Release notes"]
HelmIndex["Helm Repository<br>index.yaml<br>Chart metadata"]

Action --> Scan
GHRelease --> ReleaseAsset
Push --> HelmIndex

subgraph subGraph2 ["Published Artifacts"]
    ReleaseAsset
    HelmIndex
end

subgraph subGraph1 ["Chart Releaser Tasks"]
    Scan
    Package
    GHRelease
    GHPages
    Push
    Scan --> Package
    Package --> GHRelease
    GHRelease --> GHPages
    GHPages --> Push
end

subgraph subGraph0 ["Chart Releaser Process"]
    Condition
    Action
    Condition --> Action
end
```

The Chart Releaser action automates:

* Packaging changed charts into `.tgz` archives
* Creating GitHub Releases with semantic version tags
* Updating the Helm repository `index.yaml` on the `gh-pages` branch
* Publishing charts for consumption via `helm repo add`

**Sources:** [.github/workflows/release.yml L46-L50](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L46-L50)

### Chart Releaser Configuration

| Setting | Value | Purpose |
| --- | --- | --- |
| Action version | `v1.7.0` | Helm Chart Releaser version |
| Token | `${{ secrets.GITHUB_TOKEN }}` | Authenticate with GitHub API |
| Condition | `hasChangesets == 'false'` | Only run after versions are bumped |

**Sources:** [.github/workflows/release.yml L47-L50](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L47-L50)

## Nightly Builds

The testing workflow includes special handling for nightly scheduled builds to test against the latest development versions:

```mermaid
flowchart TD

ScheduleTrigger["Schedule Trigger<br>cron: '0 2 * * *'<br>2 AM UTC daily"]
CheckTrigger["Check:<br>github.event_name == 'schedule'"]
UpdateAppVersion["sed command<br>Update Chart.yaml<br>appVersion: 2-nightly"]
ContinueTest["Continue with<br>normal test workflow"]

ScheduleTrigger --> CheckTrigger
CheckTrigger --> UpdateAppVersion
UpdateAppVersion --> ContinueTest
```

This ensures the chart is tested against the `2-nightly` Docker image tag, which contains the latest development build of the HyperDX application.

**Sources:** [.github/workflows/chart-test.yml L11-L35](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L11-L35)

## Workflow Permissions

The workflows require specific GitHub permissions to perform their operations:

| Workflow | Permission | Purpose |
| --- | --- | --- |
| `chart-test.yml` | Default | Read repository, run tests |
| `release.yml` | `contents: write` | Create releases, push tags |
| `release.yml` | `pull-requests: write` | Create and update PRs |

**Sources:** [.github/workflows/release.yml L12-L14](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L12-L14)

## Environment Variables and Secrets

The CI/CD pipeline uses the following environment variables and secrets:

| Name | Type | Usage | Source |
| --- | --- | --- | --- |
| `GITHUB_TOKEN` | Secret | Authenticate GitHub API | Automatically provided by GitHub Actions |
| `GITHUB_ACTOR` | Variable | Git commit author | Automatically provided by GitHub Actions |
| `CR_TOKEN` | Environment | Chart Releaser authentication | Set from `GITHUB_TOKEN` |
| `RELEASE_NAME` | Variable | Helm release name in tests | Set to `hyperdx-test` |
| `NAMESPACE` | Variable | Kubernetes namespace | Set to `default` |

**Sources:** [.github/workflows/release.yml L45-L50](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L45-L50)

 [scripts/smoke-test.sh L5-L6](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L5-L6)

## Failure Handling

The testing workflow includes comprehensive failure handling to aid debugging:

```markdown
# Log collection on test failure
# .github/workflows/chart-test.yml:158-177

echo "=== Pod Status ==="
kubectl get pods -o wide

echo "=== Events ==="
kubectl get events --sort-by=.metadata.creationTimestamp

echo "=== HyperDX App Logs ==="
kubectl logs -l app=app --tail=100

echo "=== ClickHouse Logs ==="
kubectl logs -l app=clickhouse --tail=100

echo "=== MongoDB Logs ==="
kubectl logs -l app=mongodb --tail=100

echo "=== OTEL Collector Logs ==="
kubectl logs -l app=otel-collector --tail=100
```

The workflow always cleans up resources, regardless of success or failure, to prevent resource leaks in the CI environment.

**Sources:** [.github/workflows/chart-test.yml L158-L183](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L158-L183)

## Summary of CI/CD Components

| Component | File/Action | Purpose |
| --- | --- | --- |
| Test Workflow | `.github/workflows/chart-test.yml` | Run unit, integration, and smoke tests |
| Release Workflow | `.github/workflows/release.yml` | Automate version bumping and publishing |
| Changesets CLI | `@changesets/cli` | Semantic versioning and changelog |
| Version Sync Script | `scripts/update-chart-versions.js` | Keep Chart.yaml synchronized |
| Smoke Test Script | `scripts/smoke-test.sh` | Validate deployed system |
| Chart Releaser | `helm/chart-releaser-action` | Publish charts to GitHub Pages |
| Helm Unit Test | `helm-unittest` plugin | Validate Helm templates |
| Unit Test Specs | `charts/hdx-oss-v2/tests/*.yaml` | Template validation tests |

**Sources:** [.github/workflows/chart-test.yml L1-L184](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L1-L184)

 [.github/workflows/release.yml L1-L51](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/release.yml#L1-L51)

 [package.json L1-L19](https://github.com/hyperdxio/helm-charts/blob/845dd482/package.json#L1-L19)

 [scripts/smoke-test.sh L1-L202](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L1-L202)

 [charts/hdx-oss-v2/tests/helpers_test.yaml L1-L50](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/tests/helpers_test.yaml#L1-L50)