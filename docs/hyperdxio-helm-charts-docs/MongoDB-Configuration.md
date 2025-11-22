# MongoDB Configuration

> **Relevant source files**
> * [charts/hdx-oss-v2/values.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml)

This document describes the MongoDB configuration options available in the HyperDX Helm chart. MongoDB serves as the metadata storage layer for HyperDX, storing application configuration, user settings, and other persistent metadata. While ClickHouse handles the high-volume telemetry data (logs, traces, metrics), MongoDB manages the operational metadata that drives the HyperDX application.

For information about ClickHouse configuration (telemetry data storage), see [ClickHouse Configuration](/hyperdxio/helm-charts/3.3-clickhouse-configuration). For MongoDB deployment architecture details, see [MongoDB](/hyperdxio/helm-charts/5.4-mongodb).

## Configuration Overview

MongoDB configuration is managed under the `mongodb` key in [values.yaml L256-L288](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L256-L288)

 The MongoDB instance is deployed as a StatefulSet within the Kubernetes cluster and is accessed by the HyperDX application via an internal ClusterIP service.

```mermaid
flowchart TD

MongoConfig["mongodb:<br>enabled, image, port<br>persistence, probes"]
HyperDXConfig["hyperdx:<br>mongoUri"]
InitConfig["hyperdx.waitForMongodb:<br>image, pullPolicy"]
GlobalConfig["global:<br>storageClassName<br>keepPVC"]
Deployment["Deployment<br>hdx-oss-v2-mongodb"]
Service["Service<br>hdx-oss-v2-mongodb<br>Port 27017"]
PVC["PersistentVolumeClaim<br>mongodb-data"]
InitContainer["Init Container<br>wait-for-mongodb"]
MongoDBPod["MongoDB Pod<br>mongo:5.0.14-focal<br>Database: hyperdx"]
AppPod["HyperDX App Pod<br>Connects to MongoDB"]

MongoConfig --> Deployment
MongoConfig --> Service
MongoConfig --> PVC
HyperDXConfig --> AppPod
InitConfig --> InitContainer
GlobalConfig --> PVC
Deployment --> MongoDBPod
Service --> MongoDBPod
PVC --> MongoDBPod
InitContainer --> AppPod
AppPod -->|"mongoUri connection"| Service

subgraph subGraph2 ["Runtime Components"]
    MongoDBPod
    AppPod
end

subgraph subGraph1 ["Generated Resources"]
    Deployment
    Service
    PVC
    InitContainer
end

subgraph subGraph0 ["values.yaml Configuration"]
    MongoConfig
    HyperDXConfig
    InitConfig
    GlobalConfig
end
```

**MongoDB Configuration Flow**

Sources: [charts/hdx-oss-v2/values.yaml L256-L288](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L256-L288)

## Core Settings

### Image Configuration

The MongoDB image and version are specified in [values.yaml L257](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L257-L257)

:

```yaml
mongodb:
  image: "mongo:5.0.14-focal"
```

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.image` | `mongo:5.0.14-focal` | MongoDB Docker image with tag |

The image uses the official MongoDB 5.0.14 release with the focal (Ubuntu 20.04) base. This version provides a stable platform for metadata storage while maintaining compatibility with HyperDX application requirements.

Sources: [charts/hdx-oss-v2/values.yaml L257](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L257-L257)

### Port Configuration

MongoDB listens on the standard port defined in [values.yaml L258](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L258-L258)

:

```yaml
mongodb:
  port: 27017
```

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.port` | `27017` | MongoDB service port |

This port is used both for the Kubernetes service definition and in the connection URI template at [values.yaml L61](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L61-L61)

Sources: [charts/hdx-oss-v2/values.yaml L61-L258](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L61-L258)

### Enabling/Disabling MongoDB

The `enabled` flag controls whether MongoDB is deployed by the chart, as specified in [values.yaml L259](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L259-L259)

:

```yaml
mongodb:
  enabled: true
```

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.enabled` | `true` | Deploy MongoDB instance |

When set to `false`, MongoDB resources are not created, allowing use of an external MongoDB instance. In this case, update `hyperdx.mongoUri` to point to your external MongoDB server.

For details on external MongoDB deployment scenarios, see [Minimal Deployment](/hyperdxio/helm-charts/4.4-minimal-deployment).

Sources: [charts/hdx-oss-v2/values.yaml L259](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L259-L259)

## Persistence Configuration

MongoDB persistence is configured under the `persistence` key in [values.yaml L273-L275](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L273-L275)

:

```yaml
mongodb:
  persistence:
    enabled: true
    dataSize: 10Gi
```

### Persistence Settings

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.persistence.enabled` | `true` | Enable persistent storage |
| `mongodb.persistence.dataSize` | `10Gi` | PersistentVolumeClaim size |

When persistence is enabled, a PersistentVolumeClaim is created using the storage class specified in `global.storageClassName` (see [Global Configuration](/hyperdxio/helm-charts/3.1-global-configuration)). The claim is mounted at the MongoDB data directory to ensure metadata survives pod restarts.

The `global.keepPVC` setting at [values.yaml L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L12-L12)

 controls whether the PVC is retained when the Helm release is uninstalled:

```yaml
global:
  keepPVC: false  # Set to true to preserve data on uninstall
```

```mermaid
flowchart TD

PersistenceEnabled["persistence.enabled: true"]
DataSize["persistence.dataSize: 10Gi"]
StorageClass["global.storageClassName"]
KeepPVC["global.keepPVC"]
PVC["PersistentVolumeClaim<br>mongodb-data<br>10Gi"]
PV["PersistentVolume<br>Provisioned by StorageClass"]
Container["MongoDB Container"]
DataMount["Volume Mount<br>/data/db"]

PersistenceEnabled -->|"if true"| PVC
DataSize --> PVC
StorageClass --> PVC
KeepPVC -->|"controls deletion"| PVC
PV --> DataMount

subgraph subGraph2 ["MongoDB Pod"]
    Container
    DataMount
    DataMount --> Container
end

subgraph subGraph1 ["Kubernetes Resources"]
    PVC
    PV
    PVC --> PV
end

subgraph subGraph0 ["Persistence Configuration"]
    PersistenceEnabled
    DataSize
    StorageClass
    KeepPVC
end
```

**MongoDB Persistence Architecture**

Sources: [charts/hdx-oss-v2/values.yaml L10-L275](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L10-L275)

## Health Probes

MongoDB health is monitored through Kubernetes liveness and readiness probes configured in [values.yaml L276-L288](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L276-L288)

### Liveness Probe

The liveness probe determines if MongoDB needs to be restarted:

```yaml
mongodb:
  livenessProbe:
    enabled: true
    initialDelaySeconds: 10
    periodSeconds: 30
    timeoutSeconds: 5
    failureThreshold: 3
```

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.livenessProbe.enabled` | `true` | Enable liveness probe |
| `mongodb.livenessProbe.initialDelaySeconds` | `10` | Delay before first probe |
| `mongodb.livenessProbe.periodSeconds` | `30` | Probe interval |
| `mongodb.livenessProbe.timeoutSeconds` | `5` | Probe timeout |
| `mongodb.livenessProbe.failureThreshold` | `3` | Consecutive failures before restart |

### Readiness Probe

The readiness probe determines if MongoDB is ready to accept connections:

```yaml
mongodb:
  readinessProbe:
    enabled: true
    initialDelaySeconds: 1
    periodSeconds: 10
    timeoutSeconds: 5
    failureThreshold: 3
```

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `mongodb.readinessProbe.enabled` | `true` | Enable readiness probe |
| `mongodb.readinessProbe.initialDelaySeconds` | `1` | Delay before first probe |
| `mongodb.readinessProbe.periodSeconds` | `10` | Probe interval |
| `mongodb.readinessProbe.timeoutSeconds` | `5` | Probe timeout |
| `mongodb.readinessProbe.failureThreshold` | `3` | Consecutive failures before marking unready |

The readiness probe has a shorter initial delay (1 second vs 10 seconds) and more frequent checks (every 10 seconds vs 30 seconds) compared to the liveness probe, allowing faster detection of when MongoDB becomes available.

Sources: [charts/hdx-oss-v2/values.yaml L276-L288](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L276-L288)

## Node Scheduling

MongoDB pods can be scheduled to specific nodes using `nodeSelector` and `tolerations` configured in [values.yaml L261-L272](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L261-L272)

### Node Selector

```css
mongodb:
  nodeSelector: {}
    # Example:
    # kubernetes.io/os: linux
    # node-role.kubernetes.io/worker: "true"
```

Use `nodeSelector` to constrain MongoDB pods to nodes with specific labels. This is useful for directing MongoDB to nodes with appropriate storage or performance characteristics.

### Tolerations

```yaml
mongodb:
  tolerations: []
    # Example:
    # - key: "key1"
    #   operator: "Equal"
    #   value: "value1"
    #   effect: "NoSchedule"
```

Tolerations allow MongoDB pods to schedule on nodes with matching taints. This enables deployment on dedicated nodes or nodes with special characteristics.

Example configuration for dedicated storage nodes:

```yaml
mongodb:
  nodeSelector:
    storage-tier: "high-performance"
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "storage"
      effect: "NoSchedule"
```

Sources: [charts/hdx-oss-v2/values.yaml L261-L272](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L261-L272)

## Connection Configuration

The HyperDX application connects to MongoDB using the URI specified in [values.yaml L61](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L61-L61)

:

```yaml
hyperdx:
  mongoUri: mongodb://{{ include "hdx-oss.fullname" . }}-mongodb:{{ .Values.mongodb.port }}/hyperdx
```

### Connection URI Structure

```mermaid
flowchart TD

Protocol["mongodb://"]
Hostname["include hdx-oss.fullname"]
Suffix["-mongodb"]
Port["Values.mongodb.port"]
Database["/hyperdx"]
FullHostname["-mongodb"]
ServicePort["27017"]
DatabaseName["hyperdx"]
Service["Service:<br>-mongodb<br>Port: 27017<br>Type: ClusterIP"]

Protocol --> FullHostname
Hostname --> FullHostname
Suffix --> FullHostname
Port --> ServicePort
Database --> DatabaseName
FullHostname --> Service
ServicePort --> Service

subgraph subGraph2 ["Kubernetes Service"]
    Service
end

subgraph subGraph1 ["Runtime Resolution"]
    FullHostname
    ServicePort
    DatabaseName
end

subgraph subGraph0 ["URI Template"]
    Protocol
    Hostname
    Suffix
    Port
    Database
end
```

**MongoDB Connection URI Resolution**

The URI components:

* **Protocol**: `mongodb://` - Standard MongoDB connection protocol
* **Hostname**: Templated to `<release-name>-mongodb` using Helm template functions
* **Port**: References `mongodb.port` (default: 27017)
* **Database**: `hyperdx` - The database name used by HyperDX

The service name is resolved through Kubernetes DNS to the internal ClusterIP service, ensuring the connection remains within the cluster network.

### External MongoDB Configuration

To use an external MongoDB instance, set `mongodb.enabled: false` and override the `mongoUri`:

```yaml
mongodb:
  enabled: false

hyperdx:
  mongoUri: "mongodb://external-mongo.example.com:27017/hyperdx"
```

For authenticated connections:

```yaml
hyperdx:
  mongoUri: "mongodb://username:password@external-mongo.example.com:27017/hyperdx?authSource=admin"
```

Sources: [charts/hdx-oss-v2/values.yaml L61-L259](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L61-L259)

## Dependency Management

The HyperDX application pod includes an init container that waits for MongoDB to become available before starting, configured in [values.yaml L19-L22](https://github.com/hyperdxio/helm-charts/blob/845dd482/values.yaml#L19-L22)

:

```yaml
hyperdx:
  waitForMongodb:
    image: "busybox@sha256:1fcf5df59121b92d61e066df1788e8df0cc35623f5d62d9679a41e163b6a0cdb"
    pullPolicy: IfNotPresent
```

### Wait Init Container

| Configuration Key | Default Value | Description |
| --- | --- | --- |
| `hyperdx.waitForMongodb.image` | `busybox@sha256:1fc...` | Init container image |
| `hyperdx.waitForMongodb.pullPolicy` | `IfNotPresent` | Image pull policy |

The init container uses a lightweight busybox image (specified by SHA256 digest for security and immutability) to perform a network connectivity check to MongoDB before allowing the main HyperDX application container to start.

```mermaid
flowchart TD

InitStart["Init Container Start"]
WaitCheck["Check MongoDB Connection<br>busybox image<br>TCP connection test"]
WaitSuccess["MongoDB<br>Reachable?"]
WaitRetry["Wait and Retry"]
MainStart["Main Container Start<br>HyperDX Application"]
MongoService["Service: -mongodb<br>Port: 27017"]
MongoPod["MongoDB Pod<br>Status: Running<br>Readiness: True"]

WaitCheck -->|"tests connection"| MongoService

subgraph subGraph1 ["MongoDB Service"]
    MongoService
    MongoPod
    MongoService --> MongoPod
end

subgraph subGraph0 ["HyperDX Pod Startup Sequence"]
    InitStart
    WaitCheck
    WaitSuccess
    WaitRetry
    MainStart
    InitStart --> WaitCheck
    WaitCheck --> WaitSuccess
    WaitSuccess -->|"No"| WaitRetry
    WaitRetry --> WaitCheck
    WaitSuccess -->|"Yes"| MainStart
end
```

**MongoDB Dependency Wait Flow**

This pattern ensures that the HyperDX application does not start until MongoDB is ready to accept connections, preventing connection errors during startup and reducing the need for application-level retry logic.

Sources: [charts/hdx-oss-v2/values.yaml L19-L22](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L19-L22)

## Configuration Examples

### Production Configuration with High Availability

```yaml
mongodb:
  enabled: true
  image: "mongo:5.0.14-focal"
  port: 27017
  persistence:
    enabled: true
    dataSize: 50Gi
  nodeSelector:
    node-role.kubernetes.io/database: "true"
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "database"
      effect: "NoSchedule"
  livenessProbe:
    enabled: true
    initialDelaySeconds: 15
    periodSeconds: 30
    timeoutSeconds: 5
    failureThreshold: 3
  readinessProbe:
    enabled: true
    initialDelaySeconds: 5
    periodSeconds: 10
    timeoutSeconds: 5
    failureThreshold: 3

global:
  storageClassName: "fast-ssd"
  keepPVC: true
```

### Development Configuration with Minimal Resources

```yaml
mongodb:
  enabled: true
  image: "mongo:5.0.14-focal"
  port: 27017
  persistence:
    enabled: false  # Use ephemeral storage for development
  livenessProbe:
    enabled: false  # Disable for faster iterations
  readinessProbe:
    enabled: false

global:
  storageClassName: "local-path"
  keepPVC: false
```

### External MongoDB Configuration

```yaml
mongodb:
  enabled: false

hyperdx:
  mongoUri: "mongodb://mongo-user:mongo-pass@mongo.production.svc.cluster.local:27017/hyperdx?authSource=admin&replicaSet=rs0"
```

Sources: [charts/hdx-oss-v2/values.yaml L10-L288](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L10-L288)