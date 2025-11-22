# Component Deep Dives

> **Relevant source files**
> * [README.md](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md)
> * [charts/hdx-oss-v2/values.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml)

This section provides detailed technical documentation of each major component in the HyperDX Helm chart deployment. Each component is examined at the implementation level, covering deployment templates, configuration mechanisms, resource definitions, and inter-component dependencies.

For step-by-step installation and basic configuration, see [Getting Started](/hyperdxio/helm-charts/2-getting-started). For comprehensive configuration options, see [Configuration Reference](/hyperdxio/helm-charts/3-configuration-reference). For deployment patterns, see [Deployment Scenarios](/hyperdxio/helm-charts/4-deployment-scenarios).

## Component Overview

The HyperDX Helm chart deploys five primary components, each implemented as separate Kubernetes resources with distinct configuration requirements:

| Component | Primary Function | Kubernetes Resource Type | Template Path |
| --- | --- | --- | --- |
| HyperDX Application | UI, API, OpAMP server | Deployment | `templates/deployment.yaml` |
| ClickHouse | Telemetry data storage | StatefulSet/Deployment | `templates/clickhouse-*.yaml` |
| OpenTelemetry Collector | Telemetry ingestion & processing | Deployment | `templates/otel-collector-*.yaml` |
| MongoDB | Metadata storage | Deployment | `templates/mongodb-*.yaml` |
| Scheduled Tasks | Background processing | CronJob | `templates/cronjob-*.yaml` |

Each component can be enabled or disabled independently through the `enabled` flag in [values.yaml L259-L467](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L259-L467)

 supporting flexible deployment scenarios from full-stack to minimal configurations.

## Component Architecture Mapping

The following diagram maps natural language component names to their actual Kubernetes resource implementations:

```mermaid
flowchart TD

TaskCron["CronJob<br>{{ include 'hdx-oss.fullname' . }}-task-checkAlerts"]
MongoDep["Deployment<br>{{ include 'hdx-oss.fullname' . }}-mongodb"]
MongoPVC["PVC<br>{{ include 'hdx-oss.fullname' . }}-mongodb-data"]
MongoSvc["Service<br>{{ include 'hdx-oss.fullname' . }}-mongodb"]
OTELDep["Deployment<br>{{ include 'hdx-oss.fullname' . }}-otel-collector"]
OTELConfigMap["ConfigMap<br>{{ include 'hdx-oss.fullname' . }}-otel-custom-config"]
OTELSvc["Service<br>{{ include 'hdx-oss.fullname' . }}-otel-collector"]
CHDep["Deployment<br>{{ include 'hdx-oss.fullname' . }}-clickhouse"]
CHConfigMap["ConfigMap<br>{{ include 'hdx-oss.fullname' . }}-clickhouse-config"]
CHUsersMap["ConfigMap<br>{{ include 'hdx-oss.fullname' . }}-clickhouse-users"]
CHDataPVC["PVC<br>{{ include 'hdx-oss.fullname' . }}-clickhouse-data"]
CHLogPVC["PVC<br>{{ include 'hdx-oss.fullname' . }}-clickhouse-logs"]
CHSvc["Service<br>{{ include 'hdx-oss.fullname' . }}-clickhouse"]
HyperdxDep["Deployment<br>{{ include 'hdx-oss.fullname' . }}-app"]
HyperdxConfigMap["ConfigMap<br>{{ include 'hdx-oss.fullname' . }}-app-config"]
HyperdxSecret["Secret<br>{{ include 'hdx-oss.fullname' . }}-app-secrets"]
HyperdxSvc["Service<br>{{ include 'hdx-oss.fullname' . }}-app"]
HyperdxIngress["Ingress<br>{{ include 'hdx-oss.fullname' . }}-app-ingress"]

subgraph subGraph4 ["Scheduled Tasks Component"]
    TaskCron
end

subgraph subGraph3 ["MongoDB Component"]
    MongoDep
    MongoPVC
    MongoSvc
    MongoDep --> MongoPVC
    MongoDep --> MongoSvc
end

subgraph subGraph2 ["OTEL Collector Component"]
    OTELDep
    OTELConfigMap
    OTELSvc
    OTELDep --> OTELConfigMap
    OTELDep --> OTELSvc
end

subgraph subGraph1 ["ClickHouse Component"]
    CHDep
    CHConfigMap
    CHUsersMap
    CHDataPVC
    CHLogPVC
    CHSvc
    CHDep --> CHConfigMap
    CHDep --> CHUsersMap
    CHDep --> CHDataPVC
    CHDep --> CHLogPVC
    CHDep --> CHSvc
end

subgraph subGraph0 ["HyperDX Application Component"]
    HyperdxDep
    HyperdxConfigMap
    HyperdxSecret
    HyperdxSvc
    HyperdxIngress
    HyperdxDep --> HyperdxConfigMap
    HyperdxDep --> HyperdxSecret
    HyperdxDep --> HyperdxSvc
    HyperdxIngress --> HyperdxSvc
end
```

**Sources:** [values.yaml L1-L476](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L1-L476)

## Component Port Architecture

Each component exposes specific ports for different protocols and purposes:

```mermaid
flowchart TD

MongoTCP["MongoDB Protocol<br>Port 27017<br>mongodb.port"]
OTELGrpc["OTLP gRPC<br>Port 4317<br>otel.grpcPort"]
OTELHttp["OTLP HTTP<br>Port 4318<br>otel.httpPort"]
OTELFluentd["Fluentd Forward<br>Port 24225<br>otel.nativePort"]
OTELHealth["Health/Metrics<br>Port 8888<br>otel.healthPort"]
OTELExtension["Extension<br>Port 13133<br>otel.port"]
CHHTTP["HTTP Interface<br>Port 8123<br>clickhouse.port"]
CHNative["Native TCP<br>Port 9000<br>clickhouse.nativePort"]
CHProm["Prometheus<br>Port 9363<br>clickhouse.prometheus.port"]
AppUI["UI<br>Port 3000<br>hyperdx.appPort"]
AppAPI["API<br>Port 8000<br>hyperdx.apiPort"]
AppOpAMP["OpAMP<br>Port 4320<br>hyperdx.opampPort"]

subgraph subGraph3 ["MongoDB Port"]
    MongoTCP
end

subgraph subGraph2 ["OTEL Collector Ports"]
    OTELGrpc
    OTELHttp
    OTELFluentd
    OTELHealth
    OTELExtension
end

subgraph subGraph1 ["ClickHouse Ports"]
    CHHTTP
    CHNative
    CHProm
end

subgraph subGraph0 ["HyperDX Application Ports"]
    AppUI
    AppAPI
    AppOpAMP
end
```

**Sources:** [values.yaml L49-L404](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L49-L404)

## Configuration Injection Mechanisms

Components receive configuration through multiple injection patterns, each serving different security and flexibility requirements:

```mermaid
flowchart TD

ValuesYAML["values.yaml<br>User configuration"]
HelmDefaults["Chart defaults<br>Built-in values"]
ExternalSecret["External Secret<br>existingConfigSecret"]
AppConfigMap["app-config<br>envFrom injection<br>defaultConnections, defaultSources"]
CHConfigMap["clickhouse-config<br>volume mount<br>/etc/clickhouse-server/config.d/"]
CHUsersMap["clickhouse-users<br>volume mount<br>/etc/clickhouse-server/users.d/"]
OTELConfigMap["otel-custom-config<br>volume mount<br>/etc/otelcol-contrib/"]
AppSecrets["app-secrets<br>env injection<br>API_KEY, CLICKHOUSE_PASSWORD"]
EnvVars["Environment Variables<br>Direct env injection"]
EnvFromCM["Environment from ConfigMap<br>envFrom configMapRef"]
EnvFromSecret["Environment from Secret<br>env valueFrom secretKeyRef"]
VolumeMounts["Volume Mounts<br>Configuration files"]

ValuesYAML --> AppConfigMap
ValuesYAML --> CHConfigMap
ValuesYAML --> CHUsersMap
ValuesYAML --> OTELConfigMap
ValuesYAML --> AppSecrets
HelmDefaults --> AppConfigMap
ExternalSecret -->|"Optional"| AppConfigMap
AppConfigMap --> EnvFromCM
AppSecrets --> EnvFromSecret
ValuesYAML --> EnvVars
CHConfigMap --> VolumeMounts
CHUsersMap --> VolumeMounts
OTELConfigMap --> VolumeMounts

subgraph subGraph3 ["Container Environment"]
    EnvVars
    EnvFromCM
    EnvFromSecret
    VolumeMounts
end

subgraph subGraph2 ["Generated Secrets"]
    AppSecrets
end

subgraph subGraph1 ["Generated ConfigMaps"]
    AppConfigMap
    CHConfigMap
    CHUsersMap
    OTELConfigMap
end

subgraph subGraph0 ["Configuration Sources"]
    ValuesYAML
    HelmDefaults
    ExternalSecret
end
```

**Sources:** [values.yaml L72-L436](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L72-L436)

## Inter-Component Communication Patterns

Components communicate using Kubernetes service discovery with specific connection patterns:

```mermaid
flowchart TD

AppEnv["Environment Variables<br>MONGO_URI: mongodb://...-mongodb:27017<br>HDX_OTEL_ENDPOINT: http://...-otel-collector:4318"]
AppAPI["API Service<br>Queries ClickHouse HTTP:8123"]
AppOpAMP["OpAMP Service<br>Configures OTEL Collector:4320"]
OTELEnv["Environment Variables<br>OPAMP_SERVER_URL<br>CLICKHOUSE_ADDR<br>CLICKHOUSE_USER<br>CLICKHOUSE_PASSWORD<br>CLICKHOUSE_DATABASE"]
OTELExporter["ClickHouse Exporter<br>Native Protocol TCP:9000"]
OTELOpAMP["OpAMP Client<br>Connects to OpAMP:4320"]
CHConfig["config.xml<br>listen_host: ::<br>Access control via clusterCidrs"]
CHUsers["users.xml<br>app user: HTTP queries<br>otelcollector user: Native inserts"]
MongoListener["Listen on 0.0.0.0:27017"]
TaskEnv["Environment Variables<br>API_HOST: http://...-app:8000"]
TaskScript["node /app/out/task-check-alerts/src/index.js<br>Connects to API"]

AppEnv --> MongoListener
AppEnv --> OTELExporter
AppAPI --> CHConfig
AppOpAMP --> OTELOpAMP
OTELEnv --> CHConfig
OTELExporter --> CHUsers
OTELOpAMP --> AppOpAMP
TaskEnv --> AppAPI

subgraph subGraph4 ["Task CronJob Container"]
    TaskEnv
    TaskScript
end

subgraph subGraph3 ["MongoDB Container"]
    MongoListener
end

subgraph subGraph2 ["ClickHouse Container"]
    CHConfig
    CHUsers
end

subgraph subGraph1 ["OTEL Collector Container"]
    OTELEnv
    OTELExporter
    OTELOpAMP
end

subgraph subGraph0 ["HyperDX Application Container"]
    AppEnv
    AppAPI
    AppOpAMP
end
```

**Sources:** [values.yaml L60-L446](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L60-L446)

## Component Enablement and Conditional Rendering

Each component uses conditional template rendering based on the `enabled` flag:

```mermaid
flowchart TD

ValuesEnabled["values.yaml<br>.enabled flags"]
HyperdxCheck["{{- if .Values.hyperdx.enabled }}"]
CHCheck["{{- if .Values.clickhouse.enabled }}"]
OTELCheck["{{- if .Values.otel.enabled }}"]
MongoCheck["{{- if .Values.mongodb.enabled }}"]
TasksCheck["{{- if .Values.tasks.enabled }}"]
HyperdxRes["Deployment, Service, ConfigMap<br>Secret, Ingress"]
CHRes["Deployment, Service, ConfigMaps<br>PVCs"]
OTELRes["Deployment, Service<br>ConfigMap"]
MongoRes["Deployment, Service<br>PVC"]
TaskRes["CronJob"]
ExternalCH["External ClickHouse config<br>hyperdx.defaultConnections<br>otel.clickhouseEndpoint"]
ExternalOTEL["External OTEL config<br>hyperdx.otelExporterEndpoint"]

ValuesEnabled --> HyperdxCheck
ValuesEnabled --> CHCheck
ValuesEnabled --> OTELCheck
ValuesEnabled --> MongoCheck
ValuesEnabled --> TasksCheck
HyperdxCheck -->|"enabled: true"| HyperdxRes
CHCheck -->|"enabled: true"| CHRes
CHCheck -->|"enabled: false"| ExternalCH
OTELCheck -->|"enabled: true"| OTELRes
OTELCheck -->|"enabled: false"| ExternalOTEL
MongoCheck -->|"enabled: true"| MongoRes
TasksCheck -->|"enabled: true"| TaskRes

subgraph subGraph2 ["Configuration Adjustments"]
    ExternalCH
    ExternalOTEL
end

subgraph subGraph1 ["Rendered Resources"]
    HyperdxRes
    CHRes
    OTELRes
    MongoRes
    TaskRes
end

subgraph subGraph0 ["Template Conditionals"]
    HyperdxCheck
    CHCheck
    OTELCheck
    MongoCheck
    TasksCheck
end
```

**Sources:** [values.yaml L259-L467](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L259-L467)

## Container Image Configuration

Each component uses specific container images with configurable repositories and tags:

| Component | Image Repository | Tag Source | Pull Policy |
| --- | --- | --- | --- |
| HyperDX Application | `docker.hyperdx.io/hyperdx/hyperdx` | `hyperdx.image.tag` or Chart `appVersion` | `hyperdx.image.pullPolicy` |
| HyperDX Init (MongoDB wait) | `busybox@sha256:1fcf5d...` | Pinned digest | `hyperdx.waitForMongodb.pullPolicy` |
| ClickHouse | `clickhouse/clickhouse-server:25.7-alpine` | Fixed in values | N/A |
| OpenTelemetry Collector | `docker.hyperdx.io/hyperdx/hyperdx-otel-collector` | `otel.image.tag` or Chart `appVersion` | `otel.image.pullPolicy` |
| MongoDB | `mongo:5.0.14-focal` | Fixed in values | N/A |

**Sources:** [values.yaml L15-L372](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L15-L372)

## Health Probe Configuration

All deployable components implement Kubernetes health probes with consistent configuration patterns:

```mermaid
flowchart TD

LivenessProbe["Liveness Probe<br>Restart container if failing"]
ReadinessProbe["Readiness Probe<br>Remove from service endpoints"]
StartupProbe["Startup Probe<br>Delay other probes until ready"]
HyperdxLive["HTTP GET /:3000<br>initialDelaySeconds: 10<br>periodSeconds: 30"]
HyperdxReady["HTTP GET /:3000<br>initialDelaySeconds: 1<br>periodSeconds: 10"]
CHLive["HTTP GET /:8123/ping<br>initialDelaySeconds: 10<br>periodSeconds: 30"]
CHReady["HTTP GET /:8123/ping<br>initialDelaySeconds: 1<br>periodSeconds: 10"]
CHStartup["HTTP GET /:8123/ping<br>initialDelaySeconds: 5<br>failureThreshold: 30"]
OTELLive["HTTP GET /:13133<br>initialDelaySeconds: 10<br>periodSeconds: 30"]
OTELReady["HTTP GET /:13133<br>initialDelaySeconds: 5<br>periodSeconds: 10"]
MongoLive["Exec: mongosh --eval db.adminCommand('ping')<br>initialDelaySeconds: 10<br>periodSeconds: 30"]
MongoReady["Exec: mongosh --eval db.adminCommand('ping')<br>initialDelaySeconds: 1<br>periodSeconds: 10"]

LivenessProbe --> HyperdxLive
LivenessProbe --> CHLive
LivenessProbe --> OTELLive
LivenessProbe --> MongoLive
ReadinessProbe --> HyperdxReady
ReadinessProbe --> CHReady
ReadinessProbe --> OTELReady
ReadinessProbe --> MongoReady
StartupProbe --> CHStartup

subgraph MongoDB ["MongoDB"]
    MongoLive
    MongoReady
end

subgraph subGraph3 ["OTEL Collector"]
    OTELLive
    OTELReady
end

subgraph ClickHouse ["ClickHouse"]
    CHLive
    CHReady
    CHStartup
end

subgraph subGraph1 ["HyperDX Application"]
    HyperdxLive
    HyperdxReady
end

subgraph subGraph0 ["Probe Types"]
    LivenessProbe
    ReadinessProbe
    StartupProbe
end
```

**Sources:** [values.yaml L23-L464](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L23-L464)

## Persistence Architecture

Storage components implement persistent volume claims with configurable sizes and storage classes:

```mermaid
flowchart TD

StorageClass["global.storageClassName: local-path<br>global.keepPVC: false"]
CHDataPVC["PVC: ...-clickhouse-data<br>Size: clickhouse.persistence.dataSize: 10Gi<br>Mount: /var/lib/clickhouse"]
CHLogPVC["PVC: ...-clickhouse-logs<br>Size: clickhouse.persistence.logSize: 5Gi<br>Mount: /var/log/clickhouse-server"]
CHEnabled["Enabled: clickhouse.persistence.enabled: true"]
MongoPVC["PVC: ...-mongodb-data<br>Size: mongodb.persistence.dataSize: 10Gi<br>Mount: /data/db"]
MongoEnabled["Enabled: mongodb.persistence.enabled: true"]
KeepPVCTrue["keepPVC: true<br>Annotation: helm.sh/resource-policy: keep"]
KeepPVCFalse["keepPVC: false<br>PVCs deleted with release"]

StorageClass --> CHDataPVC
StorageClass --> CHLogPVC
StorageClass --> MongoPVC
StorageClass --> KeepPVCTrue
StorageClass --> KeepPVCFalse

subgraph subGraph3 ["Helm Uninstall Behavior"]
    KeepPVCTrue
    KeepPVCFalse
end

subgraph subGraph2 ["MongoDB Storage"]
    MongoPVC
    MongoEnabled
    MongoEnabled --> MongoPVC
end

subgraph subGraph1 ["ClickHouse Storage"]
    CHDataPVC
    CHLogPVC
    CHEnabled
    CHEnabled --> CHDataPVC
    CHEnabled --> CHLogPVC
end

subgraph subGraph0 ["Global Configuration"]
    StorageClass
end
```

**Sources:** [values.yaml L10-L349](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L10-L349)

## Node Scheduling Configuration

All deployable components support node affinity, tolerations, and resource constraints:

| Component | Node Selector Config | Tolerations Config | Resources Config |
| --- | --- | --- | --- |
| HyperDX Application | `hyperdx.nodeSelector` | `hyperdx.tolerations` | Not exposed in values |
| ClickHouse | `clickhouse.nodeSelector` | `clickhouse.tolerations` | `clickhouse.resources` |
| OpenTelemetry Collector | `otel.nodeSelector` | `otel.tolerations` | `otel.resources` |
| MongoDB | `mongodb.nodeSelector` | `mongodb.tolerations` | Not exposed in values |

**Example configuration:**

```yaml
clickhouse:
  nodeSelector:
    kubernetes.io/os: linux
    node-role.kubernetes.io/worker: "true"
  tolerations:
    - key: "key1"
      operator: "Equal"
      value: "value1"
      effect: "NoSchedule"
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"
```

**Sources:** [values.yaml L35-L399](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L35-L399)

## Service Discovery and DNS

Components use Kubernetes service discovery with predictable naming patterns:

```mermaid
flowchart TD

FullName["{{ include 'hdx-oss.fullname' . }}<br>Formula: release-name + chart-name"]
AppFQDN["{{ include 'hdx-oss.fullname' . }}-app.namespace.svc.cluster.local<br>Ports: 3000, 8000, 4320"]
CHFQDN["{{ include 'hdx-oss.fullname' . }}-clickhouse.namespace.svc.cluster.local<br>Ports: 8123, 9000, 9363"]
OTELFQDN["{{ include 'hdx-oss.fullname' . }}-otel-collector.namespace.svc.cluster.local<br>Ports: 4317, 4318, 24225, 8888, 13133"]
MongoFQDN["{{ include 'hdx-oss.fullname' . }}-mongodb.namespace.svc.cluster.local<br>Port: 27017"]
MongoURI["hyperdx.mongoUri:<br>mongodb://{{ include 'hdx-oss.fullname' . }}-mongodb:{{ .Values.mongodb.port }}/hyperdx"]
OTELEndpoint["hyperdx.otelExporterEndpoint:<br>http://{{ include 'hdx-oss.fullname' . }}-otel-collector:{{ .Values.otel.httpPort }}"]
CHConnection["hyperdx.defaultConnections:<br>host: http://{{ include 'hdx-oss.fullname' . }}-clickhouse:8123"]

FullName --> AppFQDN
FullName --> CHFQDN
FullName --> OTELFQDN
FullName --> MongoFQDN
MongoFQDN --> MongoURI
OTELFQDN --> OTELEndpoint
CHFQDN --> CHConnection

subgraph subGraph2 ["Configuration References"]
    MongoURI
    OTELEndpoint
    CHConnection
end

subgraph subGraph1 ["Service FQDNs"]
    AppFQDN
    CHFQDN
    OTELFQDN
    MongoFQDN
end

subgraph subGraph0 ["Service Naming Pattern"]
    FullName
end
```

**Sources:** [values.yaml L60-L100](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L60-L100)

## Component Dependencies and Initialization

The HyperDX application deployment includes an init container to ensure MongoDB availability before starting:

```mermaid
flowchart TD

InitContainer["Init Container: wait-for-mongodb<br>Image: busybox@sha256:1fcf5d...<br>Command: until nc -z mongodb-service 27017"]
MongoReady["MongoDB Service Ready<br>TCP connection successful"]
AppStarts["HyperDX Application Container Starts<br>Image: hyperdx/hyperdx"]
MongoURI["MONGO_URI available<br>Connection string configured"]
APIReady["API server starts<br>Port 8000"]
UIReady["UI server starts<br>Port 3000"]
OpAMPReady["OpAMP server starts<br>Port 4320"]

InitContainer --> MongoReady
AppStarts --> MongoURI

subgraph subGraph2 ["Environment Available"]
    MongoURI
    APIReady
    UIReady
    OpAMPReady
    MongoURI --> APIReady
    APIReady --> UIReady
    APIReady --> OpAMPReady
end

subgraph subGraph1 ["Main Container Start"]
    MongoReady
    AppStarts
    MongoReady --> AppStarts
end

subgraph subGraph0 ["Deployment Initialization"]
    InitContainer
end
```

**Sources:** [values.yaml L19-L22](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L19-L22)

## Default Configuration vs External Configuration

The chart supports two configuration patterns for connections and sources:

```mermaid
flowchart TD

UseExisting["hyperdx.useExistingConfigSecret: false"]
InlineConnections["hyperdx.defaultConnections<br>JSON array in values.yaml<br>Lines 92-101"]
InlineSources["hyperdx.defaultSources<br>JSON array in values.yaml<br>Lines 104-202"]
InlineConfigMap["ConfigMap: app-config<br>HDX_DEFAULT_CONNECTIONS<br>HDX_DEFAULT_SOURCES"]
InlineInjection["envFrom configMapRef<br>Injected as env vars"]
ExternalFlag["hyperdx.useExistingConfigSecret: true<br>hyperdx.existingConfigSecret: 'my-secret'"]
ExternalKeys["existingConfigConnectionsKey: 'connections.json'<br>existingConfigSourcesKey: 'sources.json'"]
ExternalSecret["Existing Secret<br>connections.json key<br>sources.json key"]
ExternalVolume["Volume mount from secret<br>Mounted to container"]

UseExisting -->|"false"| InlineConnections
UseExisting -->|"false"| InlineSources
UseExisting -->|"true"| ExternalFlag

subgraph subGraph2 ["External Secret Pattern"]
    ExternalFlag
    ExternalKeys
    ExternalSecret
    ExternalVolume
    ExternalFlag --> ExternalKeys
    ExternalKeys --> ExternalSecret
    ExternalSecret --> ExternalVolume
end

subgraph subGraph1 ["Inline Configuration Pattern (Default)"]
    InlineConnections
    InlineSources
    InlineConfigMap
    InlineInjection
    InlineConnections --> InlineConfigMap
    InlineSources --> InlineConfigMap
    InlineConfigMap --> InlineInjection
end

subgraph subGraph0 ["Configuration Mode Selection"]
    UseExisting
end
```

**Sources:** [values.yaml L77-L202](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L77-L202)

## Component-Specific Details

For detailed technical documentation of individual components, see the following pages:

* **[HyperDX Application](/hyperdxio/helm-charts/5.1-hyperdx-application)**: Deployment structure, multi-container architecture (UI, API, OpAMP), environment configuration, ingress setup
* **[ClickHouse Database](/hyperdxio/helm-charts/5.2-clickhouse-database)**: Persistence configuration, `config.xml` and `users.xml` structure, network access control via `clusterCidrs`, performance tuning
* **[OpenTelemetry Collector](/hyperdxio/helm-charts/5.3-opentelemetry-collector)**: Port architecture, custom config injection via `customConfig`, OpAMP integration, ClickHouse exporter configuration
* **[MongoDB](/hyperdxio/helm-charts/5.4-mongodb)**: Initialization, persistence setup, connection strings
* **[Scheduled Tasks System](/hyperdxio/helm-charts/5.5-scheduled-tasks-system)**: CronJob implementation, version-specific command paths, environment configuration for tasks

**Sources:** [values.yaml L1-L476](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L1-L476)