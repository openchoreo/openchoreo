# Changelog

All notable changes to OpenChoreo are documented in this file.

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
