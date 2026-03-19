# Changelog

All notable changes to OpenChoreo are documented in this file.

## v1.0.0-rc.2

Changes since [v1.0.0-rc.1](https://github.com/openchoreo/openchoreo/releases/tag/v1.0.0-rc.1).

### Breaking Changes

- **(CRD)** `REST` endpoint type removed from Workload. Use `HTTP` instead. ([#2785](https://github.com/openchoreo/openchoreo/pull/2785))
- **(Controller)** NetworkPolicy now allows ingress only from pods with the `openchoreo.dev/system-component` label, tightening data plane project boundary. ([#2876](https://github.com/openchoreo/openchoreo/pull/2876))
- **(Controller)** `connections` renamed to `dependencies` in CEL template context.
  `${connections.*}` becomes `${dependencies.*}` and `toContainerEnv()` becomes
  `toContainerEnvs()`. ([#2912](https://github.com/openchoreo/openchoreo/pull/2912))
- **(CLI)** `occ component scaffold` flags restructured: `--type` renamed to `--componenttype`,
  `--traits` and `--workflow` now resolve namespace-scoped resources only. Use
  `--clustercomponenttype`, `--clustertraits`, and `--clusterworkflow` for cluster-scoped
  resources. ([#2759](https://github.com/openchoreo/openchoreo/pull/2759))

### Features

- **(Controller)** `${workflowplane.secretStore}` context variable in Workflow and ClusterWorkflow
  CEL templates. ([#2902](https://github.com/openchoreo/openchoreo/pull/2902))
- **(MCP)** `query_alerts` and `query_incidents` tools for querying fired alerts and incidents. ([#2751](https://github.com/openchoreo/openchoreo/pull/2751))
- **(MCP)** `create_release_binding` tool for deploying a component release to an environment. ([#2794](https://github.com/openchoreo/openchoreo/pull/2794))
- **(Backstage UI)** Resource definition tab for Component and Project entities. ([#423](https://github.com/openchoreo/backstage-plugins/pull/423), [#453](https://github.com/openchoreo/backstage-plugins/pull/453))
- **(Backstage UI)** Template quick-create from search with type-specific icons. ([#454](https://github.com/openchoreo/backstage-plugins/pull/454))

### Enhancements

- **(CRD)** Admission webhook validation for Workflow and ClusterWorkflow resources. ([#2847](https://github.com/openchoreo/openchoreo/pull/2847))
- **(CRD)** OpenAPI v3 JSON Schema validation for ComponentType, ClusterComponentType, Trait, and ClusterTrait schema fields. ([#2852](https://github.com/openchoreo/openchoreo/pull/2852))
- **(CRD)** Validation rejects resource templates with hardcoded `metadata.namespace`; only `${metadata.namespace}` allowed. ([#2832](https://github.com/openchoreo/openchoreo/pull/2832))
- **(CRD)** Typed return types for CEL helper functions, enabling validation of field access errors in `forEach` loops. ([#2850](https://github.com/openchoreo/openchoreo/pull/2850))
- **(CRD)** Improved workload generation workflow template. ([#2819](https://github.com/openchoreo/openchoreo/pull/2819))
- **(API)** `deletionTimestamp` and `deletionGracePeriodSeconds` fields added to ObjectMeta schema. ([#2853](https://github.com/openchoreo/openchoreo/pull/2853))
- **(MCP)** Component dependencies exposed in `get_component_release` and `get_release_binding` tools. ([#2835](https://github.com/openchoreo/openchoreo/pull/2835))
- **(MCP)** Platform engineer toolset enabled by default. ([#2747](https://github.com/openchoreo/openchoreo/pull/2747))
- **(CLI)** Mixed-scope resources in `occ component scaffold` with `--clustercomponenttype`, `--clustertraits`, and `--clusterworkflow` flags. ([#2759](https://github.com/openchoreo/openchoreo/pull/2759))
- **(CLI)** Short flags `-n` (namespace), `-p` (project), `-c` (component) for common options. ([#2833](https://github.com/openchoreo/openchoreo/pull/2833))
- **(Helm)** Backstage authentication `redirectFlow` toggle for login mode. ([#2834](https://github.com/openchoreo/openchoreo/pull/2834))
- **(Helm)** Removed OpenTelemetry Collector, Fluent Bit, and unused observability plane sub-chart dependencies from Helm charts. ([#2878](https://github.com/openchoreo/openchoreo/pull/2878), [#2809](https://github.com/openchoreo/openchoreo/pull/2809), [#2737](https://github.com/openchoreo/openchoreo/pull/2737))
- **(Helm)** Thunder bumped to v0.28.0 with improved user attribute handling. ([#2776](https://github.com/openchoreo/openchoreo/pull/2776))
- **(Backstage UI)** Deployment pipeline selector in project creation and project overview page. ([#419](https://github.com/openchoreo/backstage-plugins/pull/419), [#420](https://github.com/openchoreo/backstage-plugins/pull/420))
- **(Backstage UI)** Endpoint editor improvements: schema type auto-derived from endpoint type, schema-aware placeholders, and API entities created only for endpoints with schemas. ([#428](https://github.com/openchoreo/backstage-plugins/pull/428))
- **(Backstage UI)** Entity ownership resolution from `backstage.io/owner` CR annotations. ([#434](https://github.com/openchoreo/backstage-plugins/pull/434))
- **(Backstage UI)** Workload details (container images, repository, revision) in workflow run details view. ([#436](https://github.com/openchoreo/backstage-plugins/pull/436))
- **(Backstage UI)** Deletion-marked visual indicators on catalog graph nodes. ([#437](https://github.com/openchoreo/backstage-plugins/pull/437))
- **(Backstage UI)** Workflow run retention time (TTL) display. ([#445](https://github.com/openchoreo/backstage-plugins/pull/445))
- **(Backstage UI)** Edit functionality in role binding detail dialog. ([#447](https://github.com/openchoreo/backstage-plugins/pull/447))
- **(Backstage UI)** Overhauled search with Cmd+K/Ctrl+K shortcut, kind icons, and custom filters. ([#448](https://github.com/openchoreo/backstage-plugins/pull/448))
- **(Backstage UI)** Custom recently visited card with kind-aware colors and icons. ([#449](https://github.com/openchoreo/backstage-plugins/pull/449))
- **(Backstage UI)** "Cluster" role type chip in namespace role binding views. ([#421](https://github.com/openchoreo/backstage-plugins/pull/421))
- **(Backstage UI)** Improved array field display and description styling in scaffolder forms. ([#418](https://github.com/openchoreo/backstage-plugins/pull/418))
- **(Backstage UI)** Scaffolder review step rewritten with template-aware structured sections. ([#426](https://github.com/openchoreo/backstage-plugins/pull/426))
- **(Backstage UI)** Default role templates updated with platformEngineer and SRE roles. ([#425](https://github.com/openchoreo/backstage-plugins/pull/425))
- **(Backstage UI)** Default log sort order changed to ascending (oldest first). ([#442](https://github.com/openchoreo/backstage-plugins/pull/442))
- **(Backstage UI)** Log entry copy button always visible instead of hover-only. ([#446](https://github.com/openchoreo/backstage-plugins/pull/446))
- **(Backstage UI)** Sign-in flow switched from popup to redirect-based OAuth by default. ([#440](https://github.com/openchoreo/backstage-plugins/pull/440))
- **(Backstage UI)** Separate view permissions for alerts and incidents. ([#462](https://github.com/openchoreo/backstage-plugins/pull/462))
- **(Backstage UI)** Git secrets management UI improved with search, redesigned toolbar, and improved dialog layouts. ([#465](https://github.com/openchoreo/backstage-plugins/pull/465))
- **(Backstage UI)** "Invoke URLs" renamed to "Endpoint URLs" with clickable links. ([#467](https://github.com/openchoreo/backstage-plugins/pull/467))
- **(Backstage UI)** Default or first namespace pre-selected during resource creation. ([#457](https://github.com/openchoreo/backstage-plugins/pull/457))
- **(Backstage UI)** Helper texts for environment variables and file mounts in Workload Editor. ([#455](https://github.com/openchoreo/backstage-plugins/pull/455))
- **(Backstage UI)** Keyboard shortcut hint shown in search modal with improved auto-focus. ([#451](https://github.com/openchoreo/backstage-plugins/pull/451))
- **(Backstage UI)** "promotesTo/promotedBy" relationship labels renamed to "deploysTo/deployedBy". ([#456](https://github.com/openchoreo/backstage-plugins/pull/456))
- **(Backstage UI)** "OpenChoreo Catalog" page title renamed to "Catalog". ([#459](https://github.com/openchoreo/backstage-plugins/pull/459))
- **(Backstage UI)** Deletion badge and disabled navigation for entities marked for deletion. ([#463](https://github.com/openchoreo/backstage-plugins/pull/463))

### Bug Fixes

- **(Controller)** Environment finalizer stuck in infinite retry when deleting namespace with ClusterDataPlane reference fixed. ([#2769](https://github.com/openchoreo/openchoreo/pull/2769))
- **(Controller)** ReleaseBindings not deleted during environment deletion fixed. ([#2881](https://github.com/openchoreo/openchoreo/pull/2881))
- **(Controller)** DataPlane and ClusterDataPlane status not persisted when cluster-gateway unreachable fixed. ([#2778](https://github.com/openchoreo/openchoreo/pull/2778))
- **(Controller)** Trait schema cache collisions when Trait and ClusterTrait share the same name fixed. ([#2842](https://github.com/openchoreo/openchoreo/pull/2842))
- **(Controller)** Workflow run deletion failing for ClusterWorkflow references fixed. ([#2875](https://github.com/openchoreo/openchoreo/pull/2875))
- **(Controller)** Intermittent agent disconnects when HA (multiple replicas) enabled fixed. ([#2828](https://github.com/openchoreo/openchoreo/pull/2828))
- **(Controller)** Default component type descriptions corrected. ([#2732](https://github.com/openchoreo/openchoreo/pull/2732))
- **(CLI)** Server error messages now shown instead of bare HTTP status codes. ([#2738](https://github.com/openchoreo/openchoreo/pull/2738))
- **(CLI)** Descriptive error shown for missing positional arguments instead of generic message. ([#2780](https://github.com/openchoreo/openchoreo/pull/2780))
- **(Helm)** Missing default role permissions for component CRUD, secret references, and observability management fixed. ([#2761](https://github.com/openchoreo/openchoreo/pull/2761))
- **(Helm)** Missing Helm values for quick-start observability MCP server fixed. ([#2745](https://github.com/openchoreo/openchoreo/pull/2745))
- **(Helm)** Missing `workload:view` permission in workload-publisher role fixed. ([#2904](https://github.com/openchoreo/openchoreo/pull/2904))
- **(Observability)** Observer API `total` field in logs, traces, and spans responses returning item count instead of total result count fixed. ([#2911](https://github.com/openchoreo/openchoreo/pull/2911))
- **(Install)** `check-status.sh` script updated to use correct component labels and namespaces. ([#2820](https://github.com/openchoreo/openchoreo/pull/2820))
- **(Backstage UI)** Deny policies ignored in permission evaluation and catalog visibility fixed. ([#439](https://github.com/openchoreo/backstage-plugins/pull/439))
- **(Backstage UI)** Role deletion with active bindings now prevented with clear error. ([#424](https://github.com/openchoreo/backstage-plugins/pull/424))
- **(Backstage UI)** Display names not appearing consistently in breadcrumb navigation fixed. ([#438](https://github.com/openchoreo/backstage-plugins/pull/438))
- **(Backstage UI)** Endpoints tab incorrectly showing for CronJob/Job types fixed. ([#443](https://github.com/openchoreo/backstage-plugins/pull/443))
- **(Backstage UI)** Environment overrides using latest workload instead of component release snapshot fixed. ([#430](https://github.com/openchoreo/backstage-plugins/pull/430))
- **(Backstage UI)** Traces waterfall view span color fixed. ([#468](https://github.com/openchoreo/backstage-plugins/pull/468))
- **(Backstage UI)** Cluster workflow creation failing with template fixed. ([#460](https://github.com/openchoreo/backstage-plugins/pull/460))
- **(Backstage UI)** Microsoft Entra JWT claims for profile extraction during sign-in fixed. ([#458](https://github.com/openchoreo/backstage-plugins/pull/458))
- **(Backstage UI)** Row height overflow and horizontal scroll in project components card fixed. ([#452](https://github.com/openchoreo/backstage-plugins/pull/452))

## v1.0.0-rc.1

Changes since [v0.17.0](https://github.com/openchoreo/openchoreo/releases/tag/v0.17.0).

### Breaking Changes

- **(CRD)** BuildPlane renamed to WorkflowPlane. `BuildPlane` → `WorkflowPlane`,
  `ClusterBuildPlane` → `ClusterWorkflowPlane`. ([#2574](https://github.com/openchoreo/openchoreo/pull/2574))
- **(CRD)** Schema definition changed to openAPIV3Schema. ComponentTypes, Traits, and Workflows now
  use standard OpenAPI v3 schema format. The old inline schema syntax is no longer
  supported. ([#2547](https://github.com/openchoreo/openchoreo/pull/2547), [#2539](https://github.com/openchoreo/openchoreo/pull/2539), [#2679](https://github.com/openchoreo/openchoreo/pull/2679))
  ```yaml
  # Before
  schema:
    parameters:
      replicas: "integer | default=1"

  # After
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
  ```
- **(CRD)** Connections renamed to Dependencies in Workload. The `connections` field is now
  `dependencies` with a restructured spec. ([#2510](https://github.com/openchoreo/openchoreo/pull/2510), [#2531](https://github.com/openchoreo/openchoreo/pull/2531))
- **(CRD)** `AuthzClusterRole` renamed to `ClusterAuthzRole`. CRD names follow Kubernetes naming
  conventions with `Cluster` prefix. ([#2606](https://github.com/openchoreo/openchoreo/pull/2606))
  ```yaml
  # Before
  kind: AuthzClusterRole
  # After
  kind: ClusterAuthzRole
  ```
- **(CRD)** `AuthzClusterRoleBinding` renamed to `ClusterAuthzRoleBinding`. ([#2606](https://github.com/openchoreo/openchoreo/pull/2606))
  ```yaml
  # Before
  kind: AuthzClusterRoleBinding
  # After
  kind: ClusterAuthzRoleBinding
  ```
- **(CRD)** `secretRef` renamed to `secretKeyRef` across CRDs. Affects DataPlane, ClusterDataPlane,
  and Workload resources. ([#2642](https://github.com/openchoreo/openchoreo/pull/2642))
- **(CRD)** `traitOverrides` renamed to `traitEnvironmentConfigs` in ReleaseBinding. Also
  `componentTypeEnvOverrides` → `componentTypeEnvironmentConfigs`. ([#2607](https://github.com/openchoreo/openchoreo/pull/2607))
- **(CRD)** `targetPath` renamed to `scope` in AuthzRoleBinding. ([#2580](https://github.com/openchoreo/openchoreo/pull/2580))
- **(CRD)** Role binding specs support multiple role mappings. Single `roleRef` + `targetPath`
  replaced with `roleMappings` array in both `AuthzRoleBinding` and
  `ClusterAuthzRoleBinding`. ([#2568](https://github.com/openchoreo/openchoreo/pull/2568))
  ```yaml
  # Before
  spec:
    roleRef:
      kind: AuthzRole
      name: developer
    targetPath:
      project: my-project

  # After
  spec:
    roleMappings:
      - roleRef:
          kind: AuthzRole
          name: developer
        scope:
          project: my-project
  ```
- **(CRD)** Removed approval fields from DeploymentPipeline. `requiresApproval` and
  `isManualApprovalRequired` removed from `spec.promotionPaths[].targetEnvironmentRefs[]`.
  `sourceEnvironmentRef` changed from string to object with `kind` and `name`
  fields. ([#2651](https://github.com/openchoreo/openchoreo/pull/2651))
- **(CRD)** DeploymentPipelineRef changed from string to object. Now requires `kind` and `name`
  fields. ([#2461](https://github.com/openchoreo/openchoreo/pull/2461))
  ```yaml
  # Before
  deploymentPipelineRef: my-pipeline

  # After
  deploymentPipelineRef:
    kind: DeploymentPipeline
    name: my-pipeline
  ```
- **(CRD)** DeploymentPipeline environment references changed to objects. `kind` field added to
  `spec.promotionPaths[].targetEnvironmentRefs[]`. `spec.promotionPaths[].sourceEnvironmentRef`
  changed from string to object with `kind` and `name`
  fields. ([#2544](https://github.com/openchoreo/openchoreo/pull/2544), [#2594](https://github.com/openchoreo/openchoreo/pull/2594))
- **(CRD)** ComponentRelease traits changed from map to array. Traits now include `kind`, `name`,
  and `spec` fields. ComponentType reference also includes `kind` and `name`. ([#2694](https://github.com/openchoreo/openchoreo/pull/2694), [#2687](https://github.com/openchoreo/openchoreo/pull/2687))
- **(CRD)** Default workflow ref kind changed to ClusterWorkflow. Component and WorkflowRun workflow
  references now default to `ClusterWorkflow` kind instead of `Workflow`. ([#2667](https://github.com/openchoreo/openchoreo/pull/2667))
- **(CRD)** ObservabilityAlertRule restructured. Removed `enableAiRootCauseAnalysis` and
  `notificationChannel` fields. Replaced with `actions` struct containing `notifications` and
  optional `incident` configuration. ([#2470](https://github.com/openchoreo/openchoreo/pull/2470))
- **(CRD)** `imagePullSecretRefs` removed from DataPlane and ClusterDataPlane. ([#2297](https://github.com/openchoreo/openchoreo/pull/2297))
- **(CRD)** `openchoreo.dev/controlplane-namespace` label renamed to
  `openchoreo.dev/control-plane`. ([#2576](https://github.com/openchoreo/openchoreo/pull/2576), [#2668](https://github.com/openchoreo/openchoreo/pull/2668), [#2681](https://github.com/openchoreo/openchoreo/pull/2681))
- **(CRD)** Workflow authz role renamed to `workload-publisher`. ([#2497](https://github.com/openchoreo/openchoreo/pull/2497))
- **(CRD)** Deprecated CRDs removed: ConfigurationGroup, Build, GitCommitRequest,
  DeploymentTrack. ([#2297](https://github.com/openchoreo/openchoreo/pull/2297))
- **(CRD)** Release CRD renamed to RenderedRelease. All references to `Release` resources must be
  updated to `RenderedRelease`. ([#2484](https://github.com/openchoreo/openchoreo/pull/2484))
- **(CRD)** `suspend` state removed from ReleaseBinding. ([#2706](https://github.com/openchoreo/openchoreo/pull/2706))
- **(CRD)** Workflow ref is now immutable in WorkflowRun. Workflow kind and name cannot be changed
  after creation. ([#2657](https://github.com/openchoreo/openchoreo/pull/2657))
- **(CRD)** `api-configuration` trait removed from default resources. ([#2684](https://github.com/openchoreo/openchoreo/pull/2684))
- **(API)** Legacy API routes removed. The `/legacy` prefix fallback and migration router have been
  removed. All clients must use the current API routes. ([#2588](https://github.com/openchoreo/openchoreo/pull/2588))
- **(API)** Deploy and promote endpoints removed. Use ReleaseBinding CRUD operations
  instead. ([#2569](https://github.com/openchoreo/openchoreo/pull/2569))
- **(API)** Legacy release resource endpoints removed. ([#2551](https://github.com/openchoreo/openchoreo/pull/2551))
- **(API)** ListActions API returns `ActionInfo` objects. Previously returned plain string arrays,
  now returns objects with `name` and `lowestScope` fields. ([#2602](https://github.com/openchoreo/openchoreo/pull/2602))
- **(API)** `releases` renamed to `renderedReleases` in resource tree API response. ([#2500](https://github.com/openchoreo/openchoreo/pull/2500))
- **(API)** Observer URL and RCA agent URL endpoints removed. ([#2466](https://github.com/openchoreo/openchoreo/pull/2466))
- **(API)** User types endpoint path updated to include authentication prefix. ([#2524](https://github.com/openchoreo/openchoreo/pull/2524))
- **(API)** `user_types` renamed to `subject_types` in observer auth config. ([#2534](https://github.com/openchoreo/openchoreo/pull/2534))
- **(API)** Legacy auto-build HTTP handler removed. ([#2518](https://github.com/openchoreo/openchoreo/pull/2518))
- **(API)** `component:deploy` permission action removed from action registry. ([#2597](https://github.com/openchoreo/openchoreo/pull/2597))
- **(CLI)** `--set` flag uses Helm-style paths. Override paths now follow Helm
  conventions. ([#2537](https://github.com/openchoreo/openchoreo/pull/2537))
- **(Helm)** Build plane chart renamed to `openchoreo-workflow-plane`. The `openchoreo-build-plane`
  chart no longer exists. ([#2574](https://github.com/openchoreo/openchoreo/pull/2574))
- **(Helm)** WSO2 API platform chart removed from data plane Helm chart. Now available as a
  community module. ([#2646](https://github.com/openchoreo/openchoreo/pull/2646))
- **(Helm)** Default observability module charts removed from observability-plane Helm chart. Now
  available as community modules. ([#2675](https://github.com/openchoreo/openchoreo/pull/2675))
- **(Helm)** Observer role renamed to `observer-resource-reader`. ([#2661](https://github.com/openchoreo/openchoreo/pull/2661))
- **(Backstage UI)** Build plane renamed to workflow plane throughout the UI. ([#375](https://github.com/openchoreo/backstage-plugins/pull/375))

### Features

- **(CRD)** ClusterWorkflow resource added. Cluster-scoped version of Workflow for shared workflow definitions. ([#2465](https://github.com/openchoreo/openchoreo/pull/2465))
- **(CRD)** Cluster-scoped resources (ClusterComponentType, ClusterTrait, ClusterWorkflow) used as default platform resources. ([#2532](https://github.com/openchoreo/openchoreo/pull/2532))
- **(CRD)** Cluster trait validation support. ([#2486](https://github.com/openchoreo/openchoreo/pull/2486))
- **(CRD)** Scope field in ClusterAuthzRoleBinding for scoped access control. ([#2591](https://github.com/openchoreo/openchoreo/pull/2591))
- **(CRD)** Component workflow validation in WorkflowRun. ([#2667](https://github.com/openchoreo/openchoreo/pull/2667))
- **(API)** ComponentRelease create and delete endpoints. ([#2620](https://github.com/openchoreo/openchoreo/pull/2620))
- **(API)** `labelSelector` query parameter for all list APIs. ([#2560](https://github.com/openchoreo/openchoreo/pull/2560))
- **(API)** `apiVersion` and `kind` fields in all CRD API responses. ([#2456](https://github.com/openchoreo/openchoreo/pull/2456))
- **(API)** Alerts, incidents querying and incident update endpoints. ([#2527](https://github.com/openchoreo/openchoreo/pull/2527), [#2550](https://github.com/openchoreo/openchoreo/pull/2550))
- **(API)** Alert and incident entry storage for observability plane. ([#2509](https://github.com/openchoreo/openchoreo/pull/2509))
- **(API)** CRUD endpoints for cluster workflows. ([#2489](https://github.com/openchoreo/openchoreo/pull/2489))
- **(API)** Git secret creation with cluster workflow plane support. ([#2700](https://github.com/openchoreo/openchoreo/pull/2700))
- **(CLI)** ClusterWorkflow commands: apply, list, get, delete, run, logs. ([#2494](https://github.com/openchoreo/openchoreo/pull/2494), [#2653](https://github.com/openchoreo/openchoreo/pull/2653), [#2674](https://github.com/openchoreo/openchoreo/pull/2674))
- **(CLI)** ClusterWorkflowPlane list, get, and delete commands. ([#2691](https://github.com/openchoreo/openchoreo/pull/2691))
- **(CLI)** ClusterDataPlane and ClusterObservabilityPlane commands. ([#2680](https://github.com/openchoreo/openchoreo/pull/2680))
- **(CLI)** Workflow delete command. ([#2601](https://github.com/openchoreo/openchoreo/pull/2601))
- **(CLI)** `--tail` flag for `component logs` command. ([#2533](https://github.com/openchoreo/openchoreo/pull/2533))
- **(Helm)** Default authorization roles (admin, developer, sre, platform-engineer) shipped in Helm values. ([#2268](https://github.com/openchoreo/openchoreo/pull/2268))
- **(MCP)** Manage cluster workflows. ([#2489](https://github.com/openchoreo/openchoreo/pull/2489))
- **(MCP)** Platform engineering tools for managing ComponentTypes, Traits, ComponentReleases, DeploymentPipelines, and cluster plane resources. ([#2599](https://github.com/openchoreo/openchoreo/pull/2599))
- **(Observability)** Configurable JWT auth for RCA agent and refined remediation model. ([#2614](https://github.com/openchoreo/openchoreo/pull/2614))
- **(Samples)** AWS RDS PostgreSQL create/delete generic workflow sample. ([#2577](https://github.com/openchoreo/openchoreo/pull/2577))
- **(Backstage UI)** Alerts and incidents support in observability plugin with incident indicators on environment cards. ([#361](https://github.com/openchoreo/backstage-plugins/pull/361), [#344](https://github.com/openchoreo/backstage-plugins/pull/344), [#371](https://github.com/openchoreo/backstage-plugins/pull/371))
- **(Backstage UI)** Cluster-scoped resource support: ClusterWorkflow page, ClusterDataPlane entities, and dynamic kind filtering. ([#364](https://github.com/openchoreo/backstage-plugins/pull/364), [#382](https://github.com/openchoreo/backstage-plugins/pull/382), [#370](https://github.com/openchoreo/backstage-plugins/pull/370), [#345](https://github.com/openchoreo/backstage-plugins/pull/345))
- **(Backstage UI)** Permission-based access control: proactive permission checks, permission filtering, 403 error handling, and permission gates for traces and RCA. ([#359](https://github.com/openchoreo/backstage-plugins/pull/359), [#358](https://github.com/openchoreo/backstage-plugins/pull/358), [#351](https://github.com/openchoreo/backstage-plugins/pull/351), [#356](https://github.com/openchoreo/backstage-plugins/pull/356), [#363](https://github.com/openchoreo/backstage-plugins/pull/363), [#366](https://github.com/openchoreo/backstage-plugins/pull/366), [#386](https://github.com/openchoreo/backstage-plugins/pull/386))
- **(Backstage UI)** Access control views updated with cluster role and mapping permissions support. ([#380](https://github.com/openchoreo/backstage-plugins/pull/380), [#378](https://github.com/openchoreo/backstage-plugins/pull/378), [#385](https://github.com/openchoreo/backstage-plugins/pull/385))
- **(Backstage UI)** Delete support for all OpenChoreo resource types. ([#377](https://github.com/openchoreo/backstage-plugins/pull/377))
- **(Backstage UI)** DeploymentPipeline scaffolder template with permission checks. ([#383](https://github.com/openchoreo/backstage-plugins/pull/383))
- **(Backstage UI)** Component trait configs moved to deploy page with revamped configure and deploy wizard. ([#362](https://github.com/openchoreo/backstage-plugins/pull/362))
- **(Backstage UI)** Hierarchy-aware navigation breadcrumbs. ([#367](https://github.com/openchoreo/backstage-plugins/pull/367))
- **(Backstage UI)** Context-aware create button on catalog page. ([#393](https://github.com/openchoreo/backstage-plugins/pull/393))
- **(Backstage UI)** Form/YAML toggle for workflow trigger with custom build parameters support. ([#350](https://github.com/openchoreo/backstage-plugins/pull/350), [#399](https://github.com/openchoreo/backstage-plugins/pull/399))
- **(Backstage UI)** `displayName` field added to YAML editors and templates in scaffolder. ([#400](https://github.com/openchoreo/backstage-plugins/pull/400))
- **(Backstage UI)** Skeleton loaders added to create page during template loading. ([#405](https://github.com/openchoreo/backstage-plugins/pull/405))
- **(Backstage UI)** DeploymentStatusCard replacing ProductionOverviewCard. ([#395](https://github.com/openchoreo/backstage-plugins/pull/395))
- **(Backstage UI)** Deselect-all toggle for kind and project filters. ([#372](https://github.com/openchoreo/backstage-plugins/pull/372))
- **(Backstage UI)** Catalog listing external icon opens in new tab. ([#391](https://github.com/openchoreo/backstage-plugins/pull/391))
- **(Backstage UI)** Workflow plane linked to workflow with project mapping removed. ([#389](https://github.com/openchoreo/backstage-plugins/pull/389))
- **(Backstage UI)** ObservabilityProjectRuntimeLogs component. ([#335](https://github.com/openchoreo/backstage-plugins/pull/335))
- **(Backstage UI)** EntityRelationWarning made collapsible with platform kinds filtered. ([#360](https://github.com/openchoreo/backstage-plugins/pull/360))
- **(Backstage UI)** Auth providers section removed from settings. ([#342](https://github.com/openchoreo/backstage-plugins/pull/342))
- **(Backstage UI)** WorkflowRun management support for cluster-scoped workflows, including view, trigger, and logs. ([#392](https://github.com/openchoreo/backstage-plugins/pull/392), [#376](https://github.com/openchoreo/backstage-plugins/pull/376))

### Enhancements

- **(CRD)** Workflow annotations replaced with labels and schema extensions. ([#2616](https://github.com/openchoreo/openchoreo/pull/2616))
- **(CRD)** Paketo buildpack builder introduced for build workflows. ([#2557](https://github.com/openchoreo/openchoreo/pull/2557))
- **(CLI)** Command aliases improved for consistency. ([#2565](https://github.com/openchoreo/openchoreo/pull/2565))
- **(CLI)** `--set` value typing and escaped-dot handling hardened. ([#2541](https://github.com/openchoreo/openchoreo/pull/2541))
- **(CLI)** Command descriptions updated for cluster authz role and binding commands. ([#2615](https://github.com/openchoreo/openchoreo/pull/2615))
- **(Helm)** External secret references used in observability plane. ([#2554](https://github.com/openchoreo/openchoreo/pull/2554))
- **(Helm)** Thunder v0.24.0 → v0.26.0 ([#2593](https://github.com/openchoreo/openchoreo/pull/2593))
- **(Observability)** RCA agent report backend switched from OpenSearch to SQLite (default) with PostgreSQL support. ([#2468](https://github.com/openchoreo/openchoreo/pull/2468))
- **(Backstage UI)** RCA quick fixes refactored to singular change field and file card UI. ([#374](https://github.com/openchoreo/backstage-plugins/pull/374))
- **(Backstage UI)** Environment overview cards (hosted environments, linked planes, pipelines, deployed components) made clickable with consistent styling. ([#390](https://github.com/openchoreo/backstage-plugins/pull/390))
- **(Backstage UI)** Cell diagram and platform graph panning, zoom, and positioning improved. ([#336](https://github.com/openchoreo/backstage-plugins/pull/336), [#337](https://github.com/openchoreo/backstage-plugins/pull/337))
- **(Backstage UI)** Scaffolder review step shows full path for selected scope. ([#334](https://github.com/openchoreo/backstage-plugins/pull/334))
- **(Backstage UI)** API page cards reordered with duplicate namespace filter removed. ([#388](https://github.com/openchoreo/backstage-plugins/pull/388))
- **(Backstage UI)** Component creation steps reordered. ([#410](https://github.com/openchoreo/backstage-plugins/pull/410))
- **(Backstage UI)** More resource YAML fields (uid, creationTimestamp, generation, status) shown in Definition Tab editor. ([#408](https://github.com/openchoreo/backstage-plugins/pull/408))
- **(Backstage UI)** Git secret creation scoped to workflow plane with improved secret filtering. ([#406](https://github.com/openchoreo/backstage-plugins/pull/406))

### Bug Fixes

- **(CRD)** Missing types in workflow schemas fixed. ([#2521](https://github.com/openchoreo/openchoreo/pull/2521))
- **(API)** Component validation removed from update path. ([#2631](https://github.com/openchoreo/openchoreo/pull/2631))
- **(API)** Generate release endpoint fixed to handle embedded traits. ([#2579](https://github.com/openchoreo/openchoreo/pull/2579))
- **(API)** Alerting endpoints in observer fixed. ([#2508](https://github.com/openchoreo/openchoreo/pull/2508))
- **(API)** Release generation action check fixed. ([#2688](https://github.com/openchoreo/openchoreo/pull/2688))
- **(API)** Metadata object no longer sent in workflow logs response. ([#2455](https://github.com/openchoreo/openchoreo/pull/2455))
- **(API)** Component schema updated in OpenAPI spec. ([#2649](https://github.com/openchoreo/openchoreo/pull/2649))
- **(API)** Missing workflow kind ref in WorkflowRun OpenAPI spec fixed. ([#2641](https://github.com/openchoreo/openchoreo/pull/2641))
- **(API)** Builds skipped for non-matching branches on webhook events. ([#2519](https://github.com/openchoreo/openchoreo/pull/2519))
- **(Controller)** Workflow kind passed correctly when starting component workflow run. ([#2669](https://github.com/openchoreo/openchoreo/pull/2669))
- **(Controller)** ReleaseBinding Ready condition always set and CrashLoopBackOff detected. ([#2698](https://github.com/openchoreo/openchoreo/pull/2698))
- **(Controller)** Cluster-agent 401 errors after ~1 hour on clusters with bound ServiceAccount tokens (e.g., AKS) fixed. ([#2722](https://github.com/openchoreo/openchoreo/pull/2722))
- **(CLI)** WorkflowRun list no longer always shows Pending status. ([#2654](https://github.com/openchoreo/openchoreo/pull/2654))
- **(CLI)** WorkflowRun creation fixed. ([#2634](https://github.com/openchoreo/openchoreo/pull/2634))
- **(CLI)** Scaffold command fixed to use cluster-scoped resources. ([#2696](https://github.com/openchoreo/openchoreo/pull/2696))
- **(CLI)** Existing release binding handled in deploy flow. ([#2678](https://github.com/openchoreo/openchoreo/pull/2678))
- **(Helm)** Missing property field in external secrets fixed. ([#2452](https://github.com/openchoreo/openchoreo/pull/2452))
- **(Helm)** Missing ClusterWorkflow view permission in backstage catalog reader role fixed. ([#2570](https://github.com/openchoreo/openchoreo/pull/2570))
- **(Helm)** Missing ClusterWorkflow permission in cluster role fixed. ([#2566](https://github.com/openchoreo/openchoreo/pull/2566))
- **(Helm)** Missing ClusterRoles RBAC and ClusterTrait kind in embedded traits sample fixed. ([#2714](https://github.com/openchoreo/openchoreo/pull/2714))
- **(Observability)** Observer and tracing adapter integration fixed. ([#2467](https://github.com/openchoreo/openchoreo/pull/2467))
- **(Observability)** Alert rule GET response fixed to return full alert rule instead of only rule IDs. ([#2552](https://github.com/openchoreo/openchoreo/pull/2552))
- **(Install)** Multiple quick-start and installation fixes. ([#2664](https://github.com/openchoreo/openchoreo/pull/2664), [#2672](https://github.com/openchoreo/openchoreo/pull/2672), [#2685](https://github.com/openchoreo/openchoreo/pull/2685), [#2689](https://github.com/openchoreo/openchoreo/pull/2689), [#2690](https://github.com/openchoreo/openchoreo/pull/2690), [#2692](https://github.com/openchoreo/openchoreo/pull/2692), [#2693](https://github.com/openchoreo/openchoreo/pull/2693))
- **(Backstage UI)** Workflow parameters and type extraction fixed. ([#394](https://github.com/openchoreo/backstage-plugins/pull/394))
- **(Backstage UI)** Workflow kind propagated correctly through build trigger flow. ([#381](https://github.com/openchoreo/backstage-plugins/pull/381))
- **(Backstage UI)** Workflow status handling and UI components fixed. ([#387](https://github.com/openchoreo/backstage-plugins/pull/387))
- **(Backstage UI)** Project dropdown conditionally shown only when namespace is selected. ([#398](https://github.com/openchoreo/backstage-plugins/pull/398))
- **(Backstage UI)** In-app navigation triggered for links in CompactEntityHeader. ([#396](https://github.com/openchoreo/backstage-plugins/pull/396))
- **(Backstage UI)** Release binding status handling and conditions display improved. ([#349](https://github.com/openchoreo/backstage-plugins/pull/349))
- **(Backstage UI)** Glitch effect in traits configuration during component creation wizard fixed. ([#339](https://github.com/openchoreo/backstage-plugins/pull/339))
- **(Backstage UI)** Permission check no longer restricted to default namespace, fixing resource
  visibility in other namespaces. ([#343](https://github.com/openchoreo/backstage-plugins/pull/343))
- **(Backstage UI)** Project-specific deployment pipeline fetched instead of first in namespace. ([#365](https://github.com/openchoreo/backstage-plugins/pull/365))
- **(Backstage UI)** Cluster-scoped kinds restored when cluster scope toggled back on in platform overview. ([#404](https://github.com/openchoreo/backstage-plugins/pull/404))
- **(Backstage UI)** Project-level deny policy now correctly disables UI actions over namespace-level allow. ([#409](https://github.com/openchoreo/backstage-plugins/pull/409))
- **(Backstage UI)** Deployment status uses Ready condition as single source of truth, fixing
  incorrect status display. ([#407](https://github.com/openchoreo/backstage-plugins/pull/407))
- **(Backstage UI)** Cluster workflows not showing in component types fixed. ([#414](https://github.com/openchoreo/backstage-plugins/pull/414))
- **(Backstage UI)** Catalog table column misalignment between header and data rows fixed. ([#413](https://github.com/openchoreo/backstage-plugins/pull/413))
- **(Backstage UI)** Platform Overview scope defaults to first available namespace. ([#412](https://github.com/openchoreo/backstage-plugins/pull/412))
- **(Backstage UI)** Missing deploymentpipeline template in production config fixed. ([#411](https://github.com/openchoreo/backstage-plugins/pull/411))
- **(Backstage UI)** Observability configuration in BuildEvents component fixed. ([#415](https://github.com/openchoreo/backstage-plugins/pull/415))

---

For changes in earlier versions, see [GitHub Releases](https://github.com/openchoreo/openchoreo/releases).
