# MongoDB

> **Relevant source files**
> * [charts/hdx-oss-v2/templates/hyperdx-deployment.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/templates/hyperdx-deployment.yaml)
> * [charts/hdx-oss-v2/values.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml)

## Purpose and Scope

This document provides technical details about MongoDB deployment and configuration in the HyperDX Helm chart. MongoDB serves as the metadata storage layer for HyperDX, storing application configuration, user settings, alerts, dashboards, and other metadata. For information about the main telemetry data storage (logs, traces, metrics), see [ClickHouse Database](/hyperdxio/helm-charts/5.2-clickhouse-database). For information about how the HyperDX application connects to MongoDB, see [HyperDX Application](/hyperdxio/helm-charts/5.1-hyperdx-application).

Sources: [charts/hdx-oss-v2/values.yaml L256-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L287)

## MongoDB Role in HyperDX

MongoDB is one of two primary storage systems in HyperDX, serving a complementary but distinct role from ClickHouse:

```mermaid
flowchart TD

API["hyperdx-app<br>API: Port 8000<br>UI: Port 3000"]
MongoDB["mongodb<br>Port 27017<br>Database: hyperdx"]
ClickHouse["clickhouse<br>HTTP: 8123<br>Native: 9000"]
Metadata["Metadata:<br>- User accounts<br>- Dashboards<br>- Alert rules<br>- Saved searches<br>- Team configuration<br>- API keys"]
Telemetry["Telemetry Data:<br>- Logs (otel_logs)<br>- Traces (otel_traces)<br>- Metrics (otel_metrics_*)<br>- Sessions (hyperdx_sessions)"]

API --> MongoDB
API --> ClickHouse
MongoDB --> Metadata
ClickHouse --> Telemetry

subgraph subGraph3 ["ClickHouse Data Types"]
    Telemetry
end

subgraph subGraph2 ["MongoDB Data Types"]
    Metadata
end

subgraph subGraph1 ["Storage Layer"]
    MongoDB
    ClickHouse
end

subgraph subGraph0 ["HyperDX Application Layer"]
    API
end
```

**MongoDB Storage Responsibility**: Application metadata and configuration
**ClickHouse Storage Responsibility**: High-volume telemetry data

Sources: [charts/hdx-oss-v2/values.yaml L61](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L61-L61)

 [charts/hdx-oss-v2/values.yaml L256-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L287)

## Deployment Architecture

MongoDB is deployed as a single-replica StatefulSet-like Deployment with persistent storage. The deployment creates three primary Kubernetes resources:

```mermaid
flowchart TD

Deployment["Deployment<br>Name: {fullname}-mongodb<br>Replicas: 1<br>Image: mongo:5.0.14-focal"]
Service["Service<br>Name: {fullname}-mongodb<br>Type: ClusterIP<br>Port: 27017"]
PVC["PersistentVolumeClaim<br>Name: {fullname}-mongodb-data<br>Size: 10Gi (default)<br>StorageClass: global.storageClassName"]
Container["mongodb container<br>Port: 27017<br>Image: mongo:5.0.14-focal"]
Volume["Volume Mount:<br>/data/db<br>from PVC"]
Probes["Health Checks:<br>- livenessProbe<br>- readinessProbe"]
InitContainer["initContainer:<br>wait-for-mongodb<br>busybox nc -z check"]
AppContainer["app container<br>Connects via mongoUri"]

Deployment --> Container
Volume --> PVC
Service --> Container
InitContainer --> Service
AppContainer --> Service

subgraph subGraph2 ["HyperDX App"]
    InitContainer
    AppContainer
    InitContainer --> AppContainer
end

subgraph subGraph1 ["Pod Configuration"]
    Container
    Volume
    Probes
    Container --> Volume
    Container --> Probes
end

subgraph subGraph0 ["Kubernetes Resources"]
    Deployment
    Service
    PVC
end
```

**Key Resource Names** (using Helm template function `include "hdx-oss.fullname" .`):

* Deployment: `{fullname}-mongodb`
* Service: `{fullname}-mongodb`
* PVC: `{fullname}-mongodb-data`

Sources: [charts/hdx-oss-v2/values.yaml L256-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L287)

 [charts/hdx-oss-v2/templates/hyperdx-deployment.yaml L50-L56](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/templates/hyperdx-deployment.yaml#L50-L56)

## Configuration Options

MongoDB configuration is defined in the `mongodb` section of `values.yaml`:

| Configuration Path | Default Value | Purpose |
| --- | --- | --- |
| `mongodb.enabled` | `true` | Enable/disable MongoDB deployment |
| `mongodb.image` | `mongo:5.0.14-focal` | MongoDB container image |
| `mongodb.port` | `27017` | MongoDB service port |
| `mongodb.persistence.enabled` | `true` | Enable persistent storage |
| `mongodb.persistence.dataSize` | `10Gi` | Size of data PVC |
| `mongodb.nodeSelector` | `{}` | Node selection constraints |
| `mongodb.tolerations` | `[]` | Pod tolerations |
| `mongodb.livenessProbe.enabled` | `true` | Enable liveness probe |
| `mongodb.livenessProbe.initialDelaySeconds` | `10` | Initial delay before probing |
| `mongodb.livenessProbe.periodSeconds` | `30` | Probe frequency |
| `mongodb.livenessProbe.timeoutSeconds` | `5` | Probe timeout |
| `mongodb.livenessProbe.failureThreshold` | `3` | Failures before restart |
| `mongodb.readinessProbe.enabled` | `true` | Enable readiness probe |
| `mongodb.readinessProbe.initialDelaySeconds` | `1` | Initial delay before probing |
| `mongodb.readinessProbe.periodSeconds` | `10` | Probe frequency |
| `mongodb.readinessProbe.timeoutSeconds` | `5` | Probe timeout |
| `mongodb.readinessProbe.failureThreshold` | `3` | Failures before unready |

### Example Configuration

```yaml
mongodb:
  enabled: true
  image: "mongo:5.0.14-focal"
  port: 27017
  persistence:
    enabled: true
    dataSize: 20Gi  # Increase storage for production
  nodeSelector:
    disk-type: ssd  # Schedule on SSD nodes
  tolerations:
    - key: "database"
      operator: "Equal"
      value: "mongodb"
      effect: "NoSchedule"
```

Sources: [charts/hdx-oss-v2/values.yaml L256-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L287)

## Persistence Configuration

MongoDB uses a PersistentVolumeClaim to maintain data across pod restarts and redeployments:

```mermaid
flowchart TD

Values["values.yaml<br>mongodb.persistence.enabled: true<br>mongodb.persistence.dataSize: 10Gi<br>global.storageClassName: local-path"]
Template["Helm Template<br>mongodb-pvc.yaml"]
PVC["PersistentVolumeClaim<br>Name: {fullname}-mongodb-data<br>Storage: 10Gi<br>AccessMode: ReadWriteOnce"]
PV["PersistentVolume<br>Provisioned by StorageClass"]
Pod["MongoDB Pod<br>Container: mongodb"]
Mount["Volume Mount:<br>/data/db"]
Data["MongoDB Data Files:<br>- Database files<br>- Collections<br>- Indexes<br>- Journal"]

subgraph subGraph0 ["Persistence Flow"]
    Values
    Template
    PVC
    PV
    Pod
    Mount
    Data
    Values --> Template
    Template --> PVC
    PVC --> PV
    Pod --> PVC
    PVC --> Mount
    Mount --> Data
end
```

**Persistence Behavior**:

* When `mongodb.persistence.enabled: true`: Data persists across pod restarts
* When `mongodb.persistence.enabled: false`: Data is ephemeral (lost on pod restart)
* PVC retention: Controlled by `global.keepPVC` flag (default: `false`) * `keepPVC: true`: PVC retained when helm release is uninstalled * `keepPVC: false`: PVC deleted when helm release is uninstalled

**Storage Class**: Uses `global.storageClassName` (default: `"local-path"`)

Sources: [charts/hdx-oss-v2/values.yaml L273-L275](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L273-L275)

 [charts/hdx-oss-v2/values.yaml L10-L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L10-L12)

## Health Checks

MongoDB deployment includes Kubernetes health probes to ensure availability:

```mermaid
flowchart TD

ReadyConfig["Configuration:<br>initialDelaySeconds: 1<br>periodSeconds: 10<br>timeoutSeconds: 5<br>failureThreshold: 3"]
ReadyExec["Execution:<br>TCP socket check<br>Port: 27017"]
ReadyAction["Action on Failure:<br>Remove from Service endpoints"]
LiveConfig["Configuration:<br>initialDelaySeconds: 10<br>periodSeconds: 30<br>timeoutSeconds: 5<br>failureThreshold: 3"]
LiveExec["Execution:<br>TCP socket check<br>Port: 27017"]
LiveAction["Action on Failure:<br>Restart container"]

subgraph subGraph1 ["Readiness Probe"]
    ReadyConfig
    ReadyExec
    ReadyAction
    ReadyConfig --> ReadyExec
    ReadyExec --> ReadyAction
end

subgraph subGraph0 ["Liveness Probe"]
    LiveConfig
    LiveExec
    LiveAction
    LiveConfig --> LiveExec
    LiveExec --> LiveAction
end
```

**Probe Configuration Details**:

| Probe Type | Initial Delay | Period | Timeout | Failure Threshold | Action on Failure |
| --- | --- | --- | --- | --- | --- |
| Liveness | 10s | 30s | 5s | 3 | Restart container |
| Readiness | 1s | 10s | 5s | 3 | Remove from endpoints |

**Purpose**:

* **Liveness Probe**: Detects and recovers from deadlocked MongoDB processes
* **Readiness Probe**: Ensures MongoDB is ready to accept connections before routing traffic

Sources: [charts/hdx-oss-v2/values.yaml L276-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L276-L287)

## Connection from HyperDX Application

The HyperDX application establishes a connection to MongoDB through a multi-stage initialization process:

```mermaid
flowchart TD

PodStart["Pod Start"]
InitContainer["initContainer:<br>wait-for-mongodb<br>Image: busybox<br>Command: nc -z check"]
MainContainer["Main app container<br>Image: hyperdx/hyperdx"]
DNSLookup["Kubernetes DNS Lookup:<br>{fullname}-mongodb.{namespace}.svc.cluster.local"]
Service["Service:<br>{fullname}-mongodb<br>Port: 27017"]
MongoDBPod["MongoDB Pod"]
MongoURI["Environment Variable:<br>MONGO_URI=mongodb://{fullname}-mongodb:27017/hyperdx"]
AppConfig["ConfigMap:<br>{fullname}-app-config<br>Referenced via envFrom"]

InitContainer --> DNSLookup
MongoDBPod --> InitContainer
MainContainer --> AppConfig
MainContainer --> Service

subgraph subGraph2 ["Connection Configuration"]
    MongoURI
    AppConfig
    AppConfig --> MongoURI
end

subgraph subGraph1 ["MongoDB Service Discovery"]
    DNSLookup
    Service
    MongoDBPod
    DNSLookup --> Service
    Service --> MongoDBPod
end

subgraph subGraph0 ["HyperDX Pod Initialization"]
    PodStart
    InitContainer
    MainContainer
    PodStart --> InitContainer
    InitContainer --> MainContainer
end
```

**Connection URI Format**:

```
mongodb://{fullname}-mongodb:27017/hyperdx
```

Where:

* `{fullname}`: Helm release name (e.g., `hdx-oss-v2` or custom release name)
* Database name: `hyperdx`
* No authentication by default (internal cluster communication only)

**Init Container Implementation**:

```yaml
initContainers:
  - name: wait-for-mongodb
    image: busybox@sha256:1fcf5df59121b92d61e066df1788e8df0cc35623f5d62d9679a41e163b6a0cdb
    imagePullPolicy: IfNotPresent
    command: ['sh', '-c', 'until nc -z {fullname}-mongodb 27017; do echo waiting for mongodb; sleep 2; done;']
```

This ensures the HyperDX application does not start until MongoDB is ready to accept connections.

Sources: [charts/hdx-oss-v2/values.yaml L19-L22](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L19-L22)

 [charts/hdx-oss-v2/values.yaml L61](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L61-L61)

 [charts/hdx-oss-v2/templates/hyperdx-deployment.yaml L50-L56](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/templates/hyperdx-deployment.yaml#L50-L56)

## Node Scheduling and Placement

MongoDB supports Kubernetes scheduling constraints for controlling pod placement:

```mermaid
flowchart TD

NodeSelector["nodeSelector:<br>Key-value pairs<br>for node selection"]
Tolerations["tolerations:<br>Allow scheduling<br>on tainted nodes"]
SSD["Schedule on SSD nodes:<br>nodeSelector:<br>  disk-type: ssd"]
DedicatedDB["Schedule on dedicated DB nodes:<br>tolerations:<br>- key: database<br>  value: mongodb<br>  effect: NoSchedule"]
Region["Schedule in specific region:<br>nodeSelector:<br>  topology.kubernetes.io/region: us-west-1"]

NodeSelector --> SSD
NodeSelector --> Region
Tolerations --> DedicatedDB

subgraph subGraph1 ["Example Use Cases"]
    SSD
    DedicatedDB
    Region
end

subgraph subGraph0 ["Pod Scheduling Configuration"]
    NodeSelector
    Tolerations
end
```

**Configuration Example**:

```yaml
mongodb:
  nodeSelector:
    disk-type: ssd
    kubernetes.io/os: linux
  tolerations:
    - key: "database"
      operator: "Equal"
      value: "mongodb"
      effect: "NoSchedule"
```

Sources: [charts/hdx-oss-v2/values.yaml L260-L272](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L260-L272)

## Disabling MongoDB for External Use

MongoDB can be disabled if you want to use an external MongoDB instance:

```mermaid
flowchart TD

Disabled["mongodb.enabled: false"]
NoDeploy["No MongoDB resources created"]
CustomURI["mongoUri:<br>mongodb://external-mongo.example.com:27017/hyperdx"]
ExternalMongo["External MongoDB Instance<br>Managed outside Helm chart"]
Enabled["mongodb.enabled: true"]
Deploy["MongoDB Deployment created"]
Service["MongoDB Service created"]
PVC["MongoDB PVC created"]
URI["mongoUri:<br>mongodb://{fullname}-mongodb:27017/hyperdx"]

subgraph subGraph1 ["External MongoDB"]
    Disabled
    NoDeploy
    CustomURI
    ExternalMongo
    Disabled --> NoDeploy
    NoDeploy --> CustomURI
    CustomURI --> ExternalMongo
end

subgraph subGraph0 ["Internal MongoDB (Default)"]
    Enabled
    Deploy
    Service
    PVC
    URI
    Enabled --> Deploy
    Deploy --> Service
    Service --> PVC
    Deploy --> URI
end
```

**To use an external MongoDB**:

1. Set `mongodb.enabled: false` in `values.yaml`
2. Override `hyperdx.mongoUri` with your external MongoDB connection string

```yaml
mongodb:
  enabled: false

hyperdx:
  mongoUri: "mongodb://external-mongo.example.com:27017/hyperdx"
  # Or for authenticated connection:
  # mongoUri: "mongodb://username:password@external-mongo.example.com:27017/hyperdx?authSource=admin"
```

**External MongoDB Requirements**:

* Must be accessible from the Kubernetes cluster
* Requires database named `hyperdx` (or adjust URI accordingly)
* Network connectivity must allow access from HyperDX pods
* Authentication credentials should be provided in the URI if required

For minimal deployment scenarios with external MongoDB, see [Minimal Deployment](/hyperdxio/helm-charts/4.4-minimal-deployment).

Sources: [charts/hdx-oss-v2/values.yaml L259](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L259-L259)

 [charts/hdx-oss-v2/values.yaml L61](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L61-L61)

## Service Configuration

MongoDB is exposed within the cluster via a ClusterIP service:

```mermaid
flowchart TD

ServiceConfig["Service:<br>Name: {fullname}-mongodb<br>Type: ClusterIP<br>Port: 27017<br>TargetPort: 27017<br>Protocol: TCP"]
Selector["Selector:<br>app: {fullname}-mongodb"]
DNS["DNS Resolution:<br>{fullname}-mongodb<br>{fullname}-mongodb.{namespace}<br>{fullname}-mongodb.{namespace}.svc.cluster.local"]
InternalOnly["Internal Access Only<br>No external exposure"]
AppAccess["HyperDX App:<br>Connects via DNS name"]
NoIngress["No Ingress support<br>MongoDB is cluster-internal"]

DNS --> AppAccess
ServiceConfig --> InternalOnly

subgraph subGraph1 ["Access Patterns"]
    InternalOnly
    AppAccess
    NoIngress
    InternalOnly --> NoIngress
end

subgraph subGraph0 ["Service Details"]
    ServiceConfig
    Selector
    DNS
    ServiceConfig --> Selector
    ServiceConfig --> DNS
end
```

**Service Type**: `ClusterIP` (internal only, no external exposure)

**Port**: `27017` (standard MongoDB port)

**DNS Names Available**:

* Short: `{fullname}-mongodb`
* Namespaced: `{fullname}-mongodb.{namespace}`
* FQDN: `{fullname}-mongodb.{namespace}.svc.cluster.local`

MongoDB is intentionally not exposed outside the cluster for security. All access is through internal Kubernetes networking.

Sources: [charts/hdx-oss-v2/values.yaml L256-L258](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L258)

## Resource Management

MongoDB pod resource allocation can be configured (though defaults are not set in the chart):

```yaml
mongodb:
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"
```

**Considerations for Production**:

* MongoDB memory usage depends on working set size and indexes
* CPU requirements depend on query patterns and write volume
* Consider the size of metadata (typically much smaller than telemetry data in ClickHouse)
* For high-availability scenarios, consider using an external MongoDB replica set

Sources: [charts/hdx-oss-v2/values.yaml L256-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L287)