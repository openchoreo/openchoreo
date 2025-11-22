# Operations and Maintenance

> **Relevant source files**
> * [CHANGELOG.md](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md)
> * [README.md](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md)
> * [charts/hdx-oss-v2/values.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml)

This document covers day-to-day operational procedures, monitoring, and maintenance tasks for HyperDX deployments. It provides guidance on ensuring system health, managing resources, handling data persistence, and troubleshooting common issues.

For detailed configuration of individual components, see [Component Deep Dives](/hyperdxio/helm-charts/5-component-deep-dives). For deployment and initial setup procedures, see [Getting Started](/hyperdxio/helm-charts/2-getting-started). For network and security configuration, see [Networking and Security](/hyperdxio/helm-charts/7-networking-and-security).

## Operational Overview

HyperDX deployments consist of multiple components that require ongoing monitoring and maintenance. The Helm chart provides built-in health checks, resource management, and persistence mechanisms to ensure reliable operation.

```mermaid
flowchart TD

HealthChecks["Health Check System<br>Liveness/Readiness/Startup Probes"]
Metrics["Metrics Collection<br>Prometheus on :9363"]
Logs["Log Aggregation<br>kubectl logs"]
HyperDXApp["hyperdx-app Deployment<br>livenessProbe.enabled: true<br>readinessProbe.enabled: true"]
ClickHouse["clickhouse Deployment<br>livenessProbe, readinessProbe<br>startupProbe for initialization"]
MongoDB["mongodb Deployment<br>livenessProbe, readinessProbe"]
OTEL["otel-collector Deployment<br>livenessProbe, readinessProbe"]
CHData["ClickHouse PVCs<br>clickhouse-data: 10Gi<br>clickhouse-logs: 5Gi"]
MongoData["MongoDB PVC<br>mongodb-data: 10Gi"]
KeepPVC["global.keepPVC: false<br>Retention policy"]
Replicas["Replica Configuration<br>hyperdx.replicas: 1<br>otel.replicas: 1"]
Resources["Resource Limits<br>clickhouse.resources<br>otel.resources"]

HealthChecks --> HyperDXApp
HealthChecks --> ClickHouse
HealthChecks --> MongoDB
HealthChecks --> OTEL
Metrics --> ClickHouse
Logs --> HyperDXApp
Logs --> ClickHouse
Logs --> OTEL
HyperDXApp -->|"uses"| CHData
HyperDXApp -->|"uses"| MongoData
ClickHouse -->|"stores in"| CHData
MongoDB -->|"stores in"| MongoData
Resources -->|"limits"| ClickHouse
Resources -->|"limits"| OTEL
Replicas -->|"scales"| HyperDXApp
Replicas -->|"scales"| OTEL

subgraph subGraph3 ["Resource Management"]
    Replicas
    Resources
end

subgraph subGraph2 ["Persistence Layer"]
    CHData
    MongoData
    KeepPVC
    KeepPVC -->|"controls"| CHData
    KeepPVC -->|"controls"| MongoData
end

subgraph subGraph1 ["Deployment Components"]
    HyperDXApp
    ClickHouse
    MongoDB
    OTEL
end

subgraph subGraph0 ["Operational Monitoring Layer"]
    HealthChecks
    Metrics
    Logs
end
```

**Sources:** [charts/hdx-oss-v2/values.yaml L23-L34](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L23-L34)

 [charts/hdx-oss-v2/values.yaml L276-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L276-L287)

 [charts/hdx-oss-v2/values.yaml L303-L320](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L303-L320)

 [charts/hdx-oss-v2/values.yaml L453-L464](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L453-L464)

## Health Monitoring System

### Kubernetes Health Probes

Each component has configurable health probes that Kubernetes uses to determine pod health and readiness:

| Component | Liveness Probe | Readiness Probe | Startup Probe |
| --- | --- | --- | --- |
| **hyperdx-app** | Enabled by defaultinitialDelay: 10speriod: 30stimeout: 5sfailureThreshold: 3 | Enabled by defaultinitialDelay: 1speriod: 10stimeout: 5sfailureThreshold: 3 | Not configured |
| **clickhouse** | Enabled by defaultinitialDelay: 10speriod: 30stimeout: 5sfailureThreshold: 3 | Enabled by defaultinitialDelay: 1speriod: 10stimeout: 5sfailureThreshold: 3 | Enabled by defaultinitialDelay: 5speriod: 10sfailureThreshold: 30 |
| **mongodb** | Enabled by defaultinitialDelay: 10speriod: 30stimeout: 5sfailureThreshold: 3 | Enabled by defaultinitialDelay: 1speriod: 10stimeout: 5sfailureThreshold: 3 | Not configured |
| **otel-collector** | Enabled by defaultinitialDelay: 10speriod: 30stimeout: 5sfailureThreshold: 3 | Enabled by defaultinitialDelay: 5speriod: 10stimeout: 5sfailureThreshold: 3 | Not configured |

**Probe Configuration Locations:**

* HyperDX: [charts/hdx-oss-v2/values.yaml L23-L34](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L23-L34)
* ClickHouse: [charts/hdx-oss-v2/values.yaml L303-L320](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L303-L320)
* MongoDB: [charts/hdx-oss-v2/values.yaml L276-L287](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L276-L287)
* OTEL Collector: [charts/hdx-oss-v2/values.yaml L453-L464](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L453-L464)

### Health Check Endpoints

```mermaid
flowchart TD

MongoLiveness["mongosh ping<br>Port: 27017"]
K8s["Kubernetes Kubelet"]
AppLiveness["/health endpoint<br>Port: 8000"]
AppReadiness["/ready endpoint<br>Port: 8000"]
CHLiveness["HTTP GET :8123<br>clickhouse ping"]
CHReadiness["HTTP GET :8123<br>connection test"]
CHStartup["HTTP GET :8123<br>initialization check"]
OTELHealth["Health Check Extension<br>Port: 8888"]
MongoReadiness["mongosh connection test<br>Port: 27017"]

subgraph subGraph4 ["Health Check Flow"]
    K8s
    K8s -->|"probe"| AppLiveness
    K8s -->|"probe"| AppReadiness
    K8s -->|"probe"| CHLiveness
    K8s -->|"probe"| CHReadiness
    K8s -->|"probe"| CHStartup
    K8s -->|"probe"| OTELHealth
    K8s -->|"probe"| MongoLiveness
    K8s -->|"probe"| MongoReadiness

subgraph subGraph3 ["MongoDB Pod"]
    MongoLiveness
    MongoReadiness
end

subgraph subGraph2 ["OTEL Collector Pod"]
    OTELHealth
end

subgraph subGraph1 ["ClickHouse Pod"]
    CHLiveness
    CHReadiness
    CHStartup
end

subgraph subGraph0 ["HyperDX App Pod"]
    AppLiveness
    AppReadiness
end
end
```

**Sources:** [charts/hdx-oss-v2/values.yaml L23-L34](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L23-L34)

 [charts/hdx-oss-v2/values.yaml L303-L320](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L303-L320)

 [charts/hdx-oss-v2/values.yaml L453-L464](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L453-L464)

### Checking Pod Health

Monitor pod health using kubectl commands:

```html
# Check pod status across all components
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2

# Watch pod health in real-time
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2 -w

# Detailed pod status with conditions
kubectl describe pod <pod-name>

# Check specific component health
kubectl get pods -l app.kubernetes.io/component=app
kubectl get pods -l app.kubernetes.io/component=clickhouse
kubectl get pods -l app.kubernetes.io/component=mongodb
kubectl get pods -l app.kubernetes.io/component=otel-collector
```

**Sources:** [README.md L626-L631](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L626-L631)

## Metrics and Observability

### Prometheus Metrics Export

ClickHouse exposes Prometheus metrics for monitoring database performance:

```mermaid
flowchart TD

CH["ClickHouse Server<br>clickhouse/clickhouse-server:25.7"]
PrometheusExporter["Prometheus Exporter<br>Port: 9363<br>Endpoint: /metrics"]
Config["clickhouse.prometheus.enabled: true<br>clickhouse.prometheus.port: 9363"]
Prometheus["Prometheus Server<br>Scrape Target"]
Grafana["Grafana Dashboard<br>Visualization"]

PrometheusExporter -->|"scrape"| Prometheus

subgraph subGraph1 ["Monitoring Stack"]
    Prometheus
    Grafana
    Prometheus -->|"query"| Grafana
end

subgraph subGraph0 ["ClickHouse Metrics System"]
    CH
    PrometheusExporter
    Config
    Config -->|"configures"| PrometheusExporter
    CH -->|"exposes"| PrometheusExporter
end
```

**Configuration:**

```yaml
clickhouse:
  prometheus:
    enabled: true    # Enable Prometheus metrics
    port: 9363      # Metrics endpoint port
    endpoint: "/metrics"
```

**Sources:** [charts/hdx-oss-v2/values.yaml L350-L353](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L350-L353)

### OTEL Collector Metrics

The OTEL Collector exposes its own metrics on the health port:

```yaml
otel:
  healthPort: 8888  # Metrics and health endpoint
```

Access collector metrics:

```markdown
# Forward port to access metrics locally
kubectl port-forward svc/my-hyperdx-hdx-oss-v2-otel-collector 8888:8888

# Query metrics endpoint
curl http://localhost:8888/metrics
```

**Sources:** [charts/hdx-oss-v2/values.yaml L404](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L404-L404)

## Resource Management

### Replica Configuration

Control the number of replicas for scalable components:

```yaml
hyperdx:
  replicas: 1  # HyperDX application instances

otel:
  replicas: 1  # OTEL Collector instances
```

**Scaling operations:**

```markdown
# Scale HyperDX application
kubectl scale deployment my-hyperdx-hdx-oss-v2-app --replicas=3

# Scale OTEL Collector
kubectl scale deployment my-hyperdx-hdx-oss-v2-otel-collector --replicas=2

# Check current replica count
kubectl get deployment -l app.kubernetes.io/name=hdx-oss-v2
```

**Sources:** [charts/hdx-oss-v2/values.yaml L241](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L241-L241)

 [charts/hdx-oss-v2/values.yaml L373](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L373-L373)

### Resource Limits and Requests

Configure CPU and memory allocation for components:

```mermaid
flowchart TD

Values["values.yaml<br>Resource Specifications"]
CHRequests["Requests<br>Memory: 512Mi<br>CPU: 500m"]
CHLimits["Limits<br>Memory: 2Gi<br>CPU: 2000m"]
OTELRequests["Requests<br>Memory: 127Mi<br>CPU: 100m"]
OTELLimits["Limits<br>Memory: 256Mi<br>CPU: 200m"]
TaskRequests["Requests<br>Memory: 128Mi<br>CPU: 100m"]
TaskLimits["Limits<br>Memory: 256Mi<br>CPU: 200m"]
Scheduler["Kube-Scheduler<br>Resource-based placement"]

CHRequests --> Scheduler
OTELRequests --> Scheduler
TaskRequests --> Scheduler

subgraph subGraph4 ["Kubernetes Scheduler"]
    Scheduler
end

subgraph subGraph3 ["Resource Configuration"]
    Values
    Values --> CHRequests
    Values --> CHLimits
    Values --> OTELRequests
    Values --> OTELLimits
    Values --> TaskRequests
    Values --> TaskLimits

subgraph subGraph2 ["CronJob Resources"]
    TaskRequests
    TaskLimits
end

subgraph subGraph1 ["OTEL Collector Resources"]
    OTELRequests
    OTELLimits
end

subgraph subGraph0 ["ClickHouse Resources"]
    CHRequests
    CHLimits
end
end
```

**Example resource configuration:**

```yaml
clickhouse:
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "2000m"

otel:
  resources:
    requests:
      memory: "127Mi"
      cpu: "100m"
    limits:
      memory: "256Mi"
      cpu: "200m"

tasks:
  checkAlerts:
    resources:
      requests:
        memory: "128Mi"
        cpu: "100m"
      limits:
        memory: "256Mi"
        cpu: "200m"
```

**Sources:** [charts/hdx-oss-v2/values.yaml L294-L302](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L294-L302)

 [charts/hdx-oss-v2/values.yaml L374-L382](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L374-L382)

 [charts/hdx-oss-v2/values.yaml L470-L476](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L470-L476)

### Monitoring Resource Usage

```html
# Check resource usage across pods
kubectl top pods -l app.kubernetes.io/name=hdx-oss-v2

# Detailed resource requests/limits
kubectl describe pod <pod-name> | grep -A 5 "Limits:"

# Node resource availability
kubectl top nodes
```

**Sources:** [README.md L626-L631](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L626-L631)

## Persistence and Data Management

### Persistent Volume Configuration

```mermaid
flowchart TD

Global["global.storageClassName: local-path<br>global.keepPVC: false"]
CHPersist["clickhouse.persistence.enabled: true<br>dataSize: 10Gi<br>logSize: 5Gi"]
CHDataPVC["PVC: clickhouse-data<br>10Gi"]
CHLogPVC["PVC: clickhouse-logs<br>5Gi"]
MongoPersist["mongodb.persistence.enabled: true<br>dataSize: 10Gi"]
MongoDataPVC["PVC: mongodb-data<br>10Gi"]
KeepPVC["Keep PVC on Uninstall?"]
Delete["Delete PVC"]
Retain["Retain PVC"]

Global --> KeepPVC

subgraph subGraph3 ["Lifecycle Management"]
    KeepPVC
    Delete
    Retain
    KeepPVC -->|"keepPVC: false"| Delete
    KeepPVC -->|"keepPVC: true"| Retain
end

subgraph subGraph2 ["Storage Configuration"]
    Global
    Global --> CHPersist
    Global --> MongoPersist

subgraph subGraph1 ["MongoDB Storage"]
    MongoPersist
    MongoDataPVC
    MongoPersist --> MongoDataPVC
end

subgraph subGraph0 ["ClickHouse Storage"]
    CHPersist
    CHDataPVC
    CHLogPVC
    CHPersist --> CHDataPVC
    CHPersist --> CHLogPVC
end
end
```

**Configuration details:**

```python
global:
  storageClassName: "local-path"  # Storage class for all PVCs
  keepPVC: false                  # Delete PVCs on helm uninstall

clickhouse:
  persistence:
    enabled: true
    dataSize: 10Gi   # ClickHouse data volume
    logSize: 5Gi     # ClickHouse log volume

mongodb:
  persistence:
    enabled: true
    dataSize: 10Gi   # MongoDB data volume
```

**Sources:** [charts/hdx-oss-v2/values.yaml L10-L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L10-L12)

 [charts/hdx-oss-v2/values.yaml L346-L349](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L346-L349)

 [charts/hdx-oss-v2/values.yaml L273-L275](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L273-L275)

### Managing Persistent Volumes

```python
# List all PVCs for the deployment
kubectl get pvc -l app.kubernetes.io/name=hdx-oss-v2

# Check PVC status and capacity
kubectl describe pvc <pvc-name>

# Check actual disk usage in pods
kubectl exec -it <clickhouse-pod> -- df -h /var/lib/clickhouse
kubectl exec -it <mongodb-pod> -- df -h /data/db

# Resize a PVC (if storage class supports it)
kubectl patch pvc <pvc-name> -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
```

### Backup Strategies

```mermaid
flowchart TD

CHData["ClickHouse Data<br>/var/lib/clickhouse"]
MongoData["MongoDB Data<br>/data/db"]
ConfigMaps["ConfigMaps<br>app-config<br>clickhouse-config"]
Secrets["Secrets<br>app-secrets"]
VolumeSnapshot["Volume Snapshots<br>CSI Driver"]
PodExec["kubectl exec<br>Native tools"]
PVCClone["PVC Cloning<br>Storage class feature"]
CHBackup["clickhouse-backup<br>or clickhouse-client"]
MongoDump["mongodump<br>MongoDB native"]
HelmValues["Helm values backup<br>helm get values"]

CHData --> VolumeSnapshot
CHData --> PodExec
MongoData --> VolumeSnapshot
MongoData --> PodExec
PodExec --> CHBackup
PodExec --> MongoDump
ConfigMaps --> HelmValues
Secrets --> HelmValues

subgraph subGraph2 ["Backup Tools"]
    CHBackup
    MongoDump
    HelmValues
end

subgraph subGraph1 ["Backup Methods"]
    VolumeSnapshot
    PodExec
    PVCClone
end

subgraph subGraph0 ["Data Sources"]
    CHData
    MongoData
    ConfigMaps
    Secrets
end
```

**ClickHouse backup example:**

```sql
# Backup using clickhouse-client
kubectl exec -it <clickhouse-pod> -- clickhouse-client --query="BACKUP DATABASE default TO Disk('backups', 'backup-$(date +%Y%m%d).zip')"

# Export specific tables
kubectl exec -it <clickhouse-pod> -- clickhouse-client --query="SELECT * FROM default.otel_logs FORMAT Native" > otel_logs_backup.native

# Create volume snapshot (if CSI driver supports it)
kubectl create -f clickhouse-snapshot.yaml
```

**MongoDB backup example:**

```markdown
# MongoDB dump
kubectl exec -it <mongodb-pod> -- mongodump --archive=/data/backup-$(date +%Y%m%d).gz --gzip --db hyperdx

# Copy backup to local machine
kubectl cp <mongodb-pod>:/data/backup-20240101.gz ./mongodb-backup-20240101.gz
```

**Helm configuration backup:**

```markdown
# Backup current Helm values
helm get values my-hyperdx > hyperdx-values-backup.yaml

# Backup all Kubernetes resources
kubectl get all,pvc,configmap,secret -l app.kubernetes.io/name=hdx-oss-v2 -o yaml > hyperdx-k8s-backup.yaml
```

**Sources:** [charts/hdx-oss-v2/values.yaml L346-L349](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L346-L349)

 [charts/hdx-oss-v2/values.yaml L273-L275](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L273-L275)

## Upgrade Procedures

### Pre-Upgrade Checklist

```mermaid
flowchart TD

PreUpgrade["Pre-Upgrade Phase"]
Backup["Unsupported markdown: list"]
CheckVersion["Unsupported markdown: list"]
ReviewChangelog["Unsupported markdown: list"]
TestEnv["Unsupported markdown: list"]
Upgrade["Helm Upgrade Execution"]
VerifyPods["Unsupported markdown: list"]
CheckLogs["Unsupported markdown: list"]
HealthCheck["Unsupported markdown: list"]
Rollback["Rollback if Issues<br>helm rollback"]

PreUpgrade --> Backup
Backup --> CheckVersion
CheckVersion --> ReviewChangelog
ReviewChangelog --> TestEnv
TestEnv --> Upgrade
Upgrade --> VerifyPods
VerifyPods --> CheckLogs
CheckLogs --> HealthCheck
HealthCheck -->|"Issues Found"| Rollback
```

**Sources:** [CHANGELOG.md L1-L154](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L1-L154)

 [README.md L502-L516](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L502-L516)

### Standard Upgrade Process

```sql
# 1. Update Helm repository
helm repo update hyperdx

# 2. Check available versions
helm search repo hyperdx/hdx-oss-v2 --versions

# 3. Review changes in target version
helm show readme hyperdx/hdx-oss-v2 --version <target-version>

# 4. Backup current configuration
helm get values my-hyperdx > pre-upgrade-values.yaml
kubectl get pvc -l app.kubernetes.io/name=hdx-oss-v2 > pre-upgrade-pvcs.yaml

# 5. Perform upgrade
helm upgrade my-hyperdx hyperdx/hdx-oss-v2 \
  -f values.yaml \
  --version <target-version>

# 6. Monitor rollout
kubectl rollout status deployment/my-hyperdx-hdx-oss-v2-app
kubectl rollout status deployment/my-hyperdx-hdx-oss-v2-clickhouse
kubectl rollout status deployment/my-hyperdx-hdx-oss-v2-otel-collector

# 7. Verify pods are running
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2

# 8. Check application logs for errors
kubectl logs -l app.kubernetes.io/component=app --tail=100
```

**Sources:** [README.md L504-L516](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L504-L516)

### Version-Specific Upgrade Notes

#### ClickHouse 25.7 Upgrade (Chart v0.8.0+)

The chart implements a safe ClickHouse upgrade process with controlled termination:

```yaml
clickhouse:
  terminationGracePeriodSeconds: 90  # Allow time for graceful shutdown
```

**Important considerations:**

* ClickHouse v25.7 upgrade includes breaking changes
* Ensure 90-second grace period for clean shutdown
* Monitor ClickHouse logs during upgrade for any migration issues

**Sources:** [CHANGELOG.md L28-L34](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L28-L34)

 [charts/hdx-oss-v2/values.yaml L293](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L293-L293)

#### Alert CronJob Path Changes (Chart v0.8.3+)

Starting with chart v0.8.3, alert checking uses updated command paths for newer HyperDX versions:

```markdown
# Check which version-specific command is being used
kubectl describe cronjob my-hyperdx-hdx-oss-v2-task-checkalerts | grep command

# Verify cronjob is running correctly after upgrade
kubectl get jobs -l app.kubernetes.io/component=task-checkalerts
```

**Sources:** [CHANGELOG.md L9-L13](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L9-L13)

 [CHANGELOG.md L5-L7](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L5-L7)

### Rollback Procedure

```markdown
# List release history
helm history my-hyperdx

# Rollback to previous release
helm rollback my-hyperdx <revision-number>

# Rollback to previous release (automatic revision detection)
helm rollback my-hyperdx

# Verify rollback
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2
helm list
```

**Sources:** [README.md L504-L516](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L504-L516)

## Scheduled Tasks Management

### CronJob Configuration

The chart includes a scheduled task system for background operations:

```mermaid
flowchart TD

Enabled["tasks.enabled: false<br>Default: in-process tasks"]
Schedule["tasks.checkAlerts.schedule<br>*/1 * * * *"]
Resources["tasks.checkAlerts.resources<br>CPU/Memory limits"]
InProcess["In-Process Mode<br>RUN_SCHEDULED_TASKS_EXTERNALLY=false<br>Tasks run within app pods"]
CronJob["CronJob Mode<br>RUN_SCHEDULED_TASKS_EXTERNALLY=true<br>Separate CronJob pods"]
CheckAlerts["checkAlerts Task<br>Alert evaluation<br>Notification dispatch"]
Command["Version-specific command<br>v2.0.2+: npx tsx dist/...<br>Earlier: node dist/..."]

Enabled -->|"false"| InProcess
Enabled -->|"true"| CronJob
CronJob --> Schedule
CronJob --> Resources
CronJob --> CheckAlerts

subgraph subGraph2 ["Task Operations"]
    CheckAlerts
    Command
    CheckAlerts --> Command
end

subgraph subGraph1 ["Execution Modes"]
    InProcess
    CronJob
end

subgraph subGraph0 ["Task Configuration"]
    Enabled
    Schedule
    Resources
end
```

**Configuration options:**

```yaml
tasks:
  enabled: false  # Set to true for separate CronJob execution
  checkAlerts:
    schedule: "*/1 * * * *"  # Every minute
    resources:
      requests:
        memory: "128Mi"
        cpu: "100m"
      limits:
        memory: "256Mi"
        cpu: "200m"
```

**When to use CronJob mode (`tasks.enabled: true`):**

* Heavy alert processing workload
* Need isolated resource allocation for tasks
* Want independent scaling of task execution
* Require separate monitoring of task performance

**When to use in-process mode (`tasks.enabled: false`):**

* Default for most deployments
* Simpler operational model
* Lower resource overhead
* Adequate for typical alert volumes

**Sources:** [charts/hdx-oss-v2/values.yaml L466-L476](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L466-L476)

 [CHANGELOG.md L83-L86](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L83-L86)

### Monitoring Scheduled Tasks

```python
# List CronJobs
kubectl get cronjobs -l app.kubernetes.io/name=hdx-oss-v2

# View CronJob details and schedule
kubectl describe cronjob my-hyperdx-hdx-oss-v2-task-checkalerts

# List completed and active jobs
kubectl get jobs -l app.kubernetes.io/component=task-checkalerts

# Check logs from latest job execution
kubectl logs -l job-name=<job-name> --tail=100

# Manually trigger a job execution
kubectl create job manual-alert-check-$(date +%s) \
  --from=cronjob/my-hyperdx-hdx-oss-v2-task-checkalerts
```

**Sources:** [README.md L324-L332](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L324-L332)

## Log Collection and Analysis

### Component Log Locations

```mermaid
flowchart TD

AppLogs["HyperDX App Logs<br>stdout/stderr<br>hyperdx.logLevel: info"]
CHLogs["ClickHouse Logs<br>/var/log/clickhouse-server<br>PVC: clickhouse-logs"]
OTELLogs["OTEL Collector Logs<br>stdout/stderr<br>Configuration errors"]
MongoLogs["MongoDB Logs<br>stdout/stderr<br>Connection logs"]
TaskLogs["CronJob Logs<br>Job pod logs<br>Task execution"]
Kubectl["kubectl logs<br>Direct pod logs"]
SidecarAgent["Sidecar Agent<br>Log forwarding"]
HostPath["Host Path Volume<br>Node-level logs"]
LogAggregator["Log Aggregator<br>Fluentd/Fluent Bit<br>Forward to OTEL"]
HyperDXIngestion["HyperDX Self-Ingestion<br>OTEL Collector :24225"]

AppLogs --> Kubectl
CHLogs --> Kubectl
OTELLogs --> Kubectl
MongoLogs --> Kubectl
TaskLogs --> Kubectl
AppLogs --> LogAggregator
CHLogs --> SidecarAgent
OTELLogs --> LogAggregator
SidecarAgent --> HyperDXIngestion

subgraph Aggregation ["Aggregation"]
    LogAggregator
    HyperDXIngestion
    LogAggregator --> HyperDXIngestion
end

subgraph subGraph1 ["Collection Methods"]
    Kubectl
    SidecarAgent
    HostPath
end

subgraph subGraph0 ["Log Sources"]
    AppLogs
    CHLogs
    OTELLogs
    MongoLogs
    TaskLogs
end
```

**Collecting logs:**

```python
# Stream logs from HyperDX application
kubectl logs -f deployment/my-hyperdx-hdx-oss-v2-app

# Get logs from all app pods
kubectl logs -l app.kubernetes.io/component=app --tail=100

# ClickHouse logs (stdout)
kubectl logs -l app.kubernetes.io/component=clickhouse --tail=100

# ClickHouse logs (from volume)
kubectl exec -it <clickhouse-pod> -- tail -f /var/log/clickhouse-server/clickhouse-server.log

# OTEL Collector logs
kubectl logs -l app.kubernetes.io/component=otel-collector --tail=100

# MongoDB logs
kubectl logs -l app.kubernetes.io/component=mongodb --tail=100

# CronJob execution logs
kubectl logs -l app.kubernetes.io/component=task-checkalerts --tail=100

# Export logs to file
kubectl logs deployment/my-hyperdx-hdx-oss-v2-app > app-logs.txt
```

**Sources:** [charts/hdx-oss-v2/values.yaml L57](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L57-L57)

 [README.md L626-L631](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L626-L631)

## Common Operational Issues

### Pod Restart Loops

**Symptoms:**

* Pod shows `CrashLoopBackOff` status
* Restart count incrementing

**Diagnosis:**

```html
# Check pod status
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2

# View recent events
kubectl describe pod <pod-name>

# Check container logs
kubectl logs <pod-name> --previous
```

**Common causes:**

1. **Failed health checks:** Probe timeouts too short
2. **Resource limits:** OOMKilled events
3. **Configuration errors:** Invalid environment variables
4. **Dependency issues:** MongoDB/ClickHouse not ready

**Resolution:**

```yaml
# Increase probe delays and timeouts
hyperdx:
  livenessProbe:
    initialDelaySeconds: 30
    timeoutSeconds: 10
    failureThreshold: 5
```

**Sources:** [charts/hdx-oss-v2/values.yaml L23-L34](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L23-L34)

### Init Container Failures

**Symptoms:**

* Pod stuck in `Init:Error` or `Init:CrashLoopBackOff`
* HyperDX app pod waiting for MongoDB

**Diagnosis:**

```html
# Check init container logs
kubectl logs <pod-name> -c wait-for-mongodb

# Verify MongoDB service is accessible
kubectl get svc my-hyperdx-hdx-oss-v2-mongodb
kubectl get pods -l app.kubernetes.io/component=mongodb
```

**Configuration:**

```yaml
hyperdx:
  waitForMongodb:
    image: "busybox@sha256:1fcf5df..."
    pullPolicy: IfNotPresent
```

**Sources:** [charts/hdx-oss-v2/values.yaml L19-L22](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L19-L22)

 [CHANGELOG.md L36-L37](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L36-L37)

### ClickHouse Connection Issues

**Symptoms:**

* HyperDX app cannot query data
* "Connection refused" errors in logs

**Diagnosis:**

```python
# Check ClickHouse service
kubectl get svc my-hyperdx-hdx-oss-v2-clickhouse

# Test ClickHouse connectivity from app pod
kubectl exec -it <app-pod> -- curl http://my-hyperdx-hdx-oss-v2-clickhouse:8123/ping

# Check ClickHouse CIDR configuration
kubectl get configmap my-hyperdx-hdx-oss-v2-clickhouse-config -o yaml
```

**Network CIDR configuration:**

```yaml
clickhouse:
  config:
    clusterCidrs:
      - "10.0.0.0/8"      # Kubernetes pod network
      - "172.16.0.0/12"   # Docker network
      - "192.168.0.0/16"  # Development networks
```

**Sources:** [charts/hdx-oss-v2/values.yaml L359-L366](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L359-L366)

 [README.md L528-L618](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L528-L618)

### OTEL Collector OpAMP Connection Failures

**Symptoms:**

* OTEL Collector logs show "connection refused" to OpAMP server
* Particularly common on GKE with LoadBalancer services

**Diagnosis:**

```markdown
# Check OTEL Collector logs
kubectl logs -l app.kubernetes.io/component=otel-collector --tail=50

# Verify OpAMP server URL configuration
kubectl get configmap my-hyperdx-hdx-oss-v2-otel-custom-config -o yaml
```

**GKE-specific resolution:**

```yaml
otel:
  # Use FQDN instead of service name to avoid external IP resolution
  opampServerUrl: "http://my-hyperdx-hdx-oss-v2-app.default.svc.cluster.local:4320"
```

**Sources:** [README.md L528-L549](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L528-L549)

 [charts/hdx-oss-v2/values.yaml L437-L440](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L437-L440)

### Persistent Volume Issues

**Symptoms:**

* Pods stuck in `Pending` with `FailedScheduling` events
* "no persistent volumes available" errors

**Diagnosis:**

```html
# Check PVC status
kubectl get pvc -l app.kubernetes.io/name=hdx-oss-v2

# Describe PVC for events
kubectl describe pvc <pvc-name>

# Check storage class
kubectl get storageclass
```

**Resolution:**

```python
# Set appropriate storage class for your cluster
global:
  storageClassName: "standard"  # AWS: gp2, GKE: standard, AKS: default
```

**Sources:** [charts/hdx-oss-v2/values.yaml L10](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L10-L10)

## Maintenance Windows

### Planned Maintenance Procedure

```mermaid
flowchart TD

Start["Maintenance Window Start"]
Notify["Unsupported markdown: list"]
Backup["Unsupported markdown: list"]
ScaleDown["Unsupported markdown: list"]
Maintenance["Perform Maintenance<br>Upgrades, patches, config changes"]
ScaleUp["Unsupported markdown: list"]
HealthCheck["Unsupported markdown: list"]
SmokeTest["Unsupported markdown: list"]
Complete["Maintenance Complete<br>Resume normal operations"]

Start --> Notify
Notify --> Backup
Backup --> ScaleDown
ScaleDown --> Maintenance
Maintenance --> ScaleUp
ScaleUp --> HealthCheck
HealthCheck --> SmokeTest
SmokeTest --> Complete
```

**Maintenance commands:**

```markdown
# 1. Backup current state
helm get values my-hyperdx > pre-maintenance-values.yaml
kubectl get all,pvc,configmap,secret -l app.kubernetes.io/name=hdx-oss-v2 -o yaml > pre-maintenance-state.yaml

# 2. Scale down OTEL collector to pause ingestion
kubectl scale deployment my-hyperdx-hdx-oss-v2-otel-collector --replicas=0

# 3. Perform maintenance (upgrade, patch, etc.)
helm upgrade my-hyperdx hyperdx/hdx-oss-v2 -f values.yaml

# 4. Scale up OTEL collector
kubectl scale deployment my-hyperdx-hdx-oss-v2-otel-collector --replicas=1

# 5. Verify all pods are healthy
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2

# 6. Run smoke tests
curl -I https://hyperdx.yourdomain.com
```

### Zero-Downtime Considerations

For production deployments requiring minimal downtime:

```sql
# Configure pod disruption budgets
hyperdx:
  podDisruptionBudget:
    enabled: true
    minAvailable: 1

# Scale to multiple replicas
hyperdx:
  replicas: 3

otel:
  replicas: 2

# Configure rolling update strategy (in templates)
# maxUnavailable: 1, maxSurge: 1
```

**Sources:** [charts/hdx-oss-v2/values.yaml L243-L244](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L243-L244)

## Node Scheduling and Affinity

### Node Selector Configuration

Direct pods to specific nodes based on labels:

```yaml
hyperdx:
  nodeSelector:
    kubernetes.io/os: linux
    node-role.kubernetes.io/worker: "true"
    disk-type: ssd  # Custom label for high-performance storage

clickhouse:
  nodeSelector:
    kubernetes.io/os: linux
    storage: high-performance

mongodb:
  nodeSelector:
    kubernetes.io/os: linux

otel:
  nodeSelector:
    kubernetes.io/os: linux
```

**Applying node labels:**

```markdown
# Label nodes for specific workloads
kubectl label nodes <node-name> disk-type=ssd
kubectl label nodes <node-name> storage=high-performance

# Verify node labels
kubectl get nodes --show-labels
```

**Sources:** [charts/hdx-oss-v2/values.yaml L36-L40](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L36-L40)

 [charts/hdx-oss-v2/values.yaml L261-L265](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L261-L265)

 [charts/hdx-oss-v2/values.yaml L322-L327](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L322-L327)

 [charts/hdx-oss-v2/values.yaml L388-L392](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L388-L392)

### Tolerations for Tainted Nodes

Allow pods to schedule on tainted nodes:

```yaml
hyperdx:
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "hyperdx"
      effect: "NoSchedule"
    - key: "high-memory"
      operator: "Exists"
      effect: "NoSchedule"

clickhouse:
  tolerations:
    - key: "database"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"
```

**Applying taints:**

```markdown
# Taint nodes for dedicated workloads
kubectl taint nodes <node-name> dedicated=hyperdx:NoSchedule

# Remove taint
kubectl taint nodes <node-name> dedicated=hyperdx:NoSchedule-
```

**Sources:** [charts/hdx-oss-v2/values.yaml L41-L47](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L41-L47)

 [charts/hdx-oss-v2/values.yaml L266-L272](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L266-L272)

 [charts/hdx-oss-v2/values.yaml L328-L334](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L328-L334)

 [charts/hdx-oss-v2/values.yaml L393-L399](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L393-L399)

## Service Configuration

### Service Type and Annotations

Configure Kubernetes service types and cloud provider-specific annotations:

```css
hyperdx:
  service:
    type: ClusterIP  # Default: internal only
    annotations:
      # AWS ELB annotations
      service.beta.kubernetes.io/aws-load-balancer-internal: "true"
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      
      # GCP Load Balancer annotations
      cloud.google.com/load-balancer-type: "Internal"
      
      # Azure Load Balancer annotations
      service.beta.kubernetes.io/azure-load-balancer-internal: "true"

clickhouse:
  service:
    type: ClusterIP  # Always ClusterIP for security
    annotations: {}
```

**Service types:**

* `ClusterIP` (default): Internal cluster access only - **recommended for security**
* `LoadBalancer`: External cloud load balancer - **use with caution**
* `NodePort`: Access via node IP:port - **not recommended for production**

**Sources:** [charts/hdx-oss-v2/values.yaml L247-L254](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L247-L254)

 [charts/hdx-oss-v2/values.yaml L337-L344](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L337-L344)

## Uninstallation and Cleanup

### Standard Uninstall

```markdown
# Uninstall Helm release
helm uninstall my-hyperdx

# Verify pods are terminating
kubectl get pods -l app.kubernetes.io/name=hdx-oss-v2 -w
```

### PVC Retention Policy

By default, PVCs are deleted on uninstall. To preserve data:

```yaml
global:
  keepPVC: true  # Retain PVCs when uninstalling
```

**Manual PVC cleanup:**

```sql
# List remaining PVCs
kubectl get pvc -l app.kubernetes.io/name=hdx-oss-v2

# Delete specific PVC
kubectl delete pvc <pvc-name>

# Delete all PVCs for the deployment
kubectl delete pvc -l app.kubernetes.io/name=hdx-oss-v2
```

**Sources:** [charts/hdx-oss-v2/values.yaml L12](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/values.yaml#L12-L12)

 [README.md L519-L526](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L519-L526)

### Complete Cleanup Procedure

```sql
# 1. Backup data if needed
kubectl get all,pvc,configmap,secret -l app.kubernetes.io/name=hdx-oss-v2 -o yaml > final-backup.yaml

# 2. Uninstall Helm release
helm uninstall my-hyperdx

# 3. Delete persistent volumes (if keepPVC was enabled)
kubectl delete pvc -l app.kubernetes.io/name=hdx-oss-v2

# 4. Delete any orphaned resources
kubectl delete all -l app.kubernetes.io/name=hdx-oss-v2

# 5. Clean up secrets (if manually created)
kubectl delete secret hyperdx-external-config
kubectl delete secret hyperdx-tls

# 6. Verify all resources are removed
kubectl get all,pvc,configmap,secret -l app.kubernetes.io/name=hdx-oss-v2
```

**Sources:** [README.md L519-L526](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L519-L526)

---

For detailed component-specific troubleshooting and health checks, see [Health Checks and Monitoring](/hyperdxio/helm-charts/8.1-health-checks-and-monitoring). For resource tuning and optimization, see [Resource Management](/hyperdxio/helm-charts/8.2-resource-management). For backup strategies and data recovery, see [Persistence and Backups](/hyperdxio/helm-charts/8.3-persistence-and-backups). For detailed troubleshooting scenarios, see [Troubleshooting](/hyperdxio/helm-charts/8.4-troubleshooting).