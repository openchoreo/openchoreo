# Troubleshooting

> **Relevant source files**
> * [.github/workflows/chart-test.yml](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml)
> * [CHANGELOG.md](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md)
> * [README.md](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md)
> * [scripts/smoke-test.sh](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh)

## Purpose and Scope

This page provides diagnostic procedures and solutions for common issues encountered when deploying and operating HyperDX via the Helm chart. It covers pod failures, connectivity problems, data ingestion issues, and component-specific troubleshooting. For monitoring and health checks configuration, see [Health Checks and Monitoring](/hyperdxio/helm-charts/8.1-health-checks-and-monitoring). For resource-related performance issues, see [Resource Management](/hyperdxio/helm-charts/8.2-resource-management). For data persistence problems, see [Persistence and Backups](/hyperdxio/helm-charts/8.3-persistence-and-backups).

---

## Diagnostic Workflow

```mermaid
flowchart TD

Start["Issue Detected"]
CheckPods["Check Pod Status<br>kubectl get pods"]
PodsRunning["All Pods<br>Running?"]
CheckLogs["Collect Pod Logs<br>kubectl logs"]
CheckEvents["Check Cluster Events<br>kubectl get events"]
CheckServices["Verify Services<br>kubectl get svc"]
CheckIngress["Check Ingress<br>kubectl get ingress"]
TestConnectivity["Port Forward & Test<br>kubectl port-forward"]
ComponentIssue["Identify Failed Component:<br>app, clickhouse, mongodb,<br>otel-collector"]
NetworkIssue["Network/Ingress Issue"]
DataIssue["Data Flow Issue"]
Resolution["Apply Fix & Verify"]

Start --> CheckPods
CheckPods --> PodsRunning
PodsRunning --> CheckLogs
PodsRunning --> CheckServices
CheckLogs --> CheckEvents
CheckEvents --> ComponentIssue
CheckServices --> CheckIngress
CheckIngress --> TestConnectivity
TestConnectivity --> NetworkIssue
CheckServices --> DataIssue
ComponentIssue --> Resolution
NetworkIssue --> Resolution
DataIssue --> Resolution
```

**Sources:** [.github/workflows/chart-test.yml L159-L177](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L159-L177)

 [scripts/smoke-test.sh L54-L59](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L54-L59)

---

## Pod Startup Failures

### Checking Pod Status

```html
# List all pods in the release
kubectl get pods -l app.kubernetes.io/instance=my-hyperdx

# Get detailed pod information
kubectl describe pod <pod-name>

# Check pod events
kubectl get events --sort-by=.metadata.creationTimestamp | grep <pod-name>
```

**Sources:** [.github/workflows/chart-test.yml L161-L162](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L161-L162)

 [scripts/smoke-test.sh L54-L59](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L54-L59)

### Common Pod Failure States

| State | Symptom | Common Causes | Resolution |
| --- | --- | --- | --- |
| `ImagePullBackOff` | Cannot pull container image | Registry auth issues, invalid image tag | Verify `imageRegistry`, check `imagePullSecrets` in values.yaml |
| `CrashLoopBackOff` | Container repeatedly crashes | Missing configuration, resource limits, dependency unavailable | Check logs with `kubectl logs <pod-name>`, verify environment variables |
| `Pending` | Pod not scheduled | Insufficient cluster resources, PVC binding issues | Check node resources, verify storage class exists |
| `Init:Error` | Init container failed | Volume mount issues, connectivity problems | Check init container logs: `kubectl logs <pod-name> -c <init-container>` |

### Init Container Failures

The HyperDX app deployment includes an init container that waits for ClickHouse and MongoDB:

```mermaid
flowchart TD

InitContainer["Init Container:<br>busybox@sha256:..."]
WaitCH["Wait for ClickHouse<br>Port 8123"]
WaitMongo["Wait for MongoDB<br>Port 27017"]
AppStart["Start hyperdx-app<br>Container"]

InitContainer --> WaitCH
InitContainer --> WaitMongo
WaitCH --> AppStart
WaitMongo --> AppStart
```

**Diagnostic commands:**

```markdown
# Check init container logs
kubectl logs deployment/my-hyperdx-hdx-oss-v2-app -c wait-for-dependencies

# Verify ClickHouse is reachable
kubectl exec deployment/my-hyperdx-hdx-oss-v2-app -c wait-for-dependencies -- nc -zv my-hyperdx-hdx-oss-v2-clickhouse 8123

# Verify MongoDB is reachable
kubectl exec deployment/my-hyperdx-hdx-oss-v2-app -c wait-for-dependencies -- nc -zv my-hyperdx-hdx-oss-v2-mongodb 27017
```

**Sources:** [charts/hdx-oss-v2/templates/hyperdx-deployment.yaml](https://github.com/hyperdxio/helm-charts/blob/845dd482/charts/hdx-oss-v2/templates/hyperdx-deployment.yaml)

 [CHANGELOG.md L37](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L37-L37)

---

## Component-Specific Issues

### HyperDX Application Issues

#### API Key Not Configured

**Symptom:** OTEL collector shows authentication errors or data not appearing in UI.

**Diagnosis:**

```markdown
# Check if API key is set in configmap
kubectl get configmap my-hyperdx-hdx-oss-v2-app-config -o jsonpath='{.data.API_KEY}'

# Check secret
kubectl get secret my-hyperdx-hdx-oss-v2-app-secrets -o jsonpath='{.data.API_KEY}' | base64 -d
```

**Resolution:**

```sql
# Update API key via Helm upgrade
helm upgrade my-hyperdx hyperdx/hdx-oss-v2 --set hyperdx.apiKey="your-api-key"

# Restart pods to pick up new configuration
kubectl rollout restart deployment/my-hyperdx-hdx-oss-v2-app
kubectl rollout restart deployment/my-hyperdx-hdx-oss-v2-otel-collector
```

**Sources:** [README.md L239-L273](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L239-L273)

#### Frontend URL Misconfiguration

**Symptom:** Redirects fail, cookies not set, or CORS errors in browser console.

**Diagnosis:**

```markdown
# Check configured frontend URL
kubectl get configmap my-hyperdx-hdx-oss-v2-app-config -o jsonpath='{.data.FRONTEND_URL}'

# Check ingress host
kubectl get ingress my-hyperdx-hdx-oss-v2-app-ingress -o jsonpath='{.spec.rules[0].host}'
```

**Resolution:** Ensure `hyperdx.frontendUrl` matches the ingress host including protocol:

```yaml
hyperdx:
  frontendUrl: "https://hyperdx.yourdomain.com"
  ingress:
    enabled: true
    host: "hyperdx.yourdomain.com"
```

**Sources:** [README.md L351-L365](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L351-L365)

 [CHANGELOG.md L58-L64](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L58-L64)

---

### ClickHouse Issues

#### Connection Refused Errors

**Symptom:** Application logs show `connection refused` when connecting to ClickHouse.

```mermaid
flowchart TD

App["hyperdx-app Pod"]
CHSvc["clickhouse Service<br>ClusterIP"]
CHPod["clickhouse Pod"]
CheckDNS["Verify DNS:<br>nslookup clickhouse"]
CheckPort["Verify Port:<br>nc -zv clickhouse 8123"]
CheckLogs["Check ClickHouse Logs"]

App --> CHSvc
CHSvc --> CHPod
App --> CheckDNS
CheckDNS --> CheckPort
CheckPort --> CheckLogs
```

**Diagnosis:**

```python
# Verify ClickHouse pod is running
kubectl get pods -l app=clickhouse

# Check ClickHouse logs for startup errors
kubectl logs -l app=clickhouse --tail=100

# Test ClickHouse connectivity from another pod
kubectl run -it --rm debug --image=busybox --restart=Never -- sh
# Inside the debug pod:
nc -zv my-hyperdx-hdx-oss-v2-clickhouse 8123
```

**Common causes:**

* Pod not ready (check readiness probe)
* Network policy blocking access
* ClickHouse configuration errors
* Insufficient resources causing restart

**Sources:** [.github/workflows/chart-test.yml L164-L169](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L164-L169)

 [scripts/smoke-test.sh L163-L169](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L163-L169)

#### Data Not Persisting

**Symptom:** Data lost after ClickHouse pod restart.

**Diagnosis:**

```markdown
# Check if PVC is bound
kubectl get pvc -l app=clickhouse

# Check PVC mount in pod
kubectl describe pod -l app=clickhouse | grep -A 5 "Mounts:"

# Verify persistence is enabled
helm get values my-hyperdx | grep -A 5 "clickhouse.persistence"
```

**Resolution:**

```python
clickhouse:
  persistence:
    enabled: true
    dataSize: 50Gi
    logSize: 10Gi
    storageClassName: standard  # Verify this storage class exists
```

**Sources:** [README.md L82-L90](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L82-L90)

#### Network Access Control Issues

**Symptom:** OTEL collector cannot write to ClickHouse, errors about IP not allowed.

**Diagnosis:**

```markdown
# Check ClickHouse config for clusterCidrs
kubectl get configmap my-hyperdx-hdx-oss-v2-clickhouse-config -o yaml | grep -A 5 "clusterCidrs"

# Get pod CIDR range
kubectl cluster-info dump | grep -m 1 cluster-cidr
```

**Resolution:** Configure correct CIDR for your cloud provider:

```yaml
clickhouse:
  config:
    clusterCidrs:
      - "10.244.0.0/16"  # Example for standard k8s
      - "10.0.0.0/8"      # Broader range for compatibility
```

**Sources:** [README.md L553-L609](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L553-L609)

---

### MongoDB Issues

#### Authentication Failures

**Symptom:** HyperDX app logs show MongoDB authentication errors.

**Diagnosis:**

```markdown
# Check MongoDB is running
kubectl get pods -l app=mongodb

# Test MongoDB connection
kubectl exec -it deployment/my-hyperdx-hdx-oss-v2-mongodb -- mongosh --eval "db.adminCommand('ismaster')"

# Check connection string in app config
kubectl get configmap my-hyperdx-hdx-oss-v2-app-config -o jsonpath='{.data.MONGO_URI}'
```

**Resolution for external MongoDB:**

```yaml
mongodb:
  enabled: false

hyperdx:
  mongoUri: "mongodb://username:password@your-mongo-host:27017/hyperdx?authSource=admin"
```

**Sources:** [scripts/smoke-test.sh L171-L177](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L171-L177)

 [CHANGELOG.md L145](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L145-L145)

#### Team Bootstrap Issues

**Symptom:** Cannot login after fresh install, no team exists in database.

**Diagnosis:**

```markdown
# Check if any teams exist
kubectl exec deployment/my-hyperdx-hdx-oss-v2-mongodb -- mongosh hyperdx --eval "db.teams.find().pretty()"
```

**Resolution:**

```sql
# Create test team manually
kubectl exec deployment/my-hyperdx-hdx-oss-v2-mongodb -- mongosh hyperdx --eval "
  db.teams.insertOne({
    name: 'Default Team',
    apiKey: 'your-api-key',
    collectorAuthenticationEnforced: false,
    createdAt: new Date(),
    updatedAt: new Date()
  })
"
```

**Sources:** [.github/workflows/chart-test.yml L109-L137](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L109-L137)

---

### OpenTelemetry Collector Issues

#### OTLP Endpoint Not Accepting Data

**Symptom:** Applications cannot send telemetry, connection timeouts or refused.

```mermaid
flowchart TD

ExtApp["External Application"]
Ingress["Ingress<br>additionalIngresses"]
OTELSvc["otel-collector Service<br>Port 4318 HTTP<br>Port 4317 gRPC"]
OTELPod["otel-collector Pod"]
CheckIngress["Check Ingress<br>kubectl get ingress"]
CheckSvc["Check Service<br>kubectl get svc"]
PortForward["Port Forward Test<br>kubectl port-forward"]

ExtApp --> Ingress
Ingress --> OTELSvc
OTELSvc --> OTELPod
ExtApp --> CheckIngress
CheckIngress --> CheckSvc
CheckSvc --> PortForward
```

**Diagnosis:**

```css
# Check OTEL collector is running
kubectl get pods -l app=otel-collector

# Check OTEL collector logs
kubectl logs -l app=otel-collector --tail=50

# Test OTLP HTTP endpoint locally
kubectl port-forward service/my-hyperdx-hdx-oss-v2-otel-collector 4318:4318

# Send test data
curl -X POST http://localhost:4318/v1/logs \
  -H "Content-Type: application/json" \
  -d '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"test"}}]}]}]}'
```

**Sources:** [scripts/smoke-test.sh L85-L126](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L85-L126)

 [README.md L436-L472](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L436-L472)

#### OpAMP Connection Failures (GKE Specific)

**Symptom:** OTEL collector logs show `dial tcp <EXTERNAL-IP>:4320: connect: connection refused`.

**Root cause:** GKE LoadBalancer causes DNS resolution to external IPs instead of cluster-internal IPs.

**Diagnosis:**

```markdown
# Check OpAMP server URL in OTEL collector env
kubectl get deployment my-hyperdx-hdx-oss-v2-otel-collector -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="OPAMP_SERVER_URL")].value}'

# Check if URL resolves to external IP (bad)
kubectl exec deployment/my-hyperdx-hdx-oss-v2-otel-collector -- nslookup my-hyperdx-hdx-oss-v2-app
```

**Resolution:**

```yaml
otel:
  opampServerUrl: "http://my-hyperdx-hdx-oss-v2-app.default.svc.cluster.local:4320"
```

**Sources:** [README.md L531-L549](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L531-L549)

 [CHANGELOG.md L58](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L58-L58)

#### Custom Configuration Not Applied

**Symptom:** Custom OTEL collector config not taking effect.

**Diagnosis:**

```markdown
# Check if custom config is mounted
kubectl get configmap my-hyperdx-hdx-oss-v2-otel-custom-config -o yaml

# Verify OTEL_CUSTOM_CONFIG_PATH env variable
kubectl get deployment my-hyperdx-hdx-oss-v2-otel-collector -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="OTEL_CUSTOM_CONFIG_PATH")].value}'
```

**Resolution:**

```yaml
otel:
  customConfig: |
    receivers:
      otlp:
        protocols:
          http:
            endpoint: 0.0.0.0:4318
    processors:
      batch:
        timeout: 10s
```

**Sources:** [CHANGELOG.md L51](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L51-L51)

 [README.md L189-L197](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L189-L197)

---

## Network and Connectivity Issues

### Ingress Not Working

#### UI Returns 404 or HTML for JS Assets

**Symptom:** Browser shows blank page, console errors like `Unexpected token '<'`.

**Root cause:** Ingress not properly rewriting paths, serving HTML for JavaScript files.

**Diagnosis:**

```markdown
# Check ingress configuration
kubectl describe ingress my-hyperdx-hdx-oss-v2-app-ingress

# Test asset URL
curl -I https://hyperdx.yourdomain.com/_next/static/chunks/main.js
# Should return: Content-Type: application/javascript
# Bad response: Content-Type: text/html
```

**Resolution:** Ensure proper path rewrite configuration:

```yaml
hyperdx:
  ingress:
    enabled: true
    host: "hyperdx.yourdomain.com"
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: /$1
      nginx.ingress.kubernetes.io/use-regex: "true"
    path: "/(.*)$"
    pathType: ImplementationSpecific
```

**Sources:** [README.md L389-L431](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L389-L431)

 [CHANGELOG.md L122](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L122-L122)

### TLS Certificate Issues

**Symptom:** Browser shows certificate errors or HTTPS not working.

**Diagnosis:**

```css
# Check if TLS secret exists
kubectl get secret hyperdx-tls

# Check ingress TLS configuration
kubectl get ingress my-hyperdx-hdx-oss-v2-app-ingress -o jsonpath='{.spec.tls}'

# Test TLS handshake
openssl s_client -connect hyperdx.yourdomain.com:443 -servername hyperdx.yourdomain.com
```

**Resolution:**

```sql
# Create TLS secret if missing
kubectl create secret tls hyperdx-tls \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key

# Update values
helm upgrade my-hyperdx hyperdx/hdx-oss-v2 -f values.yaml
```

**Sources:** [README.md L369-L388](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L369-L388)

### Service Discovery Issues

```mermaid
flowchart TD

AppPod["hyperdx-app Pod"]
DNS["Kubernetes DNS<br>coredns"]
ShortName["Service Name:<br>clickhouse"]
FQDN["FQDN:<br>my-hyperdx-hdx-oss-v2-clickhouse<br>.default.svc.cluster.local"]
CHSvc["clickhouse Service<br>ClusterIP"]
CHPod["clickhouse Pod"]
TestDNS["Test: nslookup<br>kubectl exec -- nslookup"]
TestPort["Test: nc -zv<br>kubectl exec -- nc"]

AppPod --> DNS
DNS --> ShortName
DNS --> FQDN
ShortName --> CHSvc
FQDN --> CHSvc
CHSvc --> CHPod
AppPod --> TestDNS
TestDNS --> TestPort
```

**Diagnosis:**

```python
# Test DNS resolution from app pod
kubectl exec deployment/my-hyperdx-hdx-oss-v2-app -- nslookup my-hyperdx-hdx-oss-v2-clickhouse

# Test with FQDN
kubectl exec deployment/my-hyperdx-hdx-oss-v2-app -- nslookup my-hyperdx-hdx-oss-v2-clickhouse.default.svc.cluster.local

# Check CoreDNS logs
kubectl logs -n kube-system -l k8s-app=kube-dns
```

**Sources:** [README.md L531-L549](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L531-L549)

---

## Data Flow Issues

### No Data Appearing in UI

```mermaid
flowchart TD

Source["Telemetry Source"]
OTEL["OTEL Collector<br>:4318, :4317"]
CH["ClickHouse<br>otel_logs, otel_traces"]
API["HyperDX API"]
UI["HyperDX UI"]
CheckOTEL["Check OTEL Logs"]
CheckCH["Query ClickHouse"]
CheckAPI["Check API Logs"]

Source --> OTEL
OTEL --> CH
API --> CH
UI --> API
OTEL --> CheckOTEL
CH --> CheckCH
API --> CheckAPI
```

**Diagnosis at each stage:**

**1. OTEL Collector receiving data:**

```markdown
# Check OTEL metrics
kubectl port-forward service/my-hyperdx-hdx-oss-v2-otel-collector 8888:8888
curl http://localhost:8888/metrics | grep otelcol_receiver_accepted

# Check OTEL logs
kubectl logs -l app=otel-collector --tail=100 | grep -i error
```

**2. ClickHouse storing data:**

```sql
# Query ClickHouse directly
kubectl exec deployment/my-hyperdx-hdx-oss-v2-clickhouse -- clickhouse-client --query "
  SELECT count() FROM default.otel_logs;
  SELECT count() FROM default.otel_traces;
  SELECT count() FROM default.otel_metrics_sum;
"

# Check for recent data
kubectl exec deployment/my-hyperdx-hdx-oss-v2-clickhouse -- clickhouse-client --query "
  SELECT * FROM default.otel_logs ORDER BY Timestamp DESC LIMIT 5;
"
```

**3. API querying data:**

```markdown
# Check API logs for query errors
kubectl logs deployment/my-hyperdx-hdx-oss-v2-app | grep -i "clickhouse\|query"

# Verify defaultConnections configuration
kubectl get configmap my-hyperdx-hdx-oss-v2-app-config -o jsonpath='{.data.HDX_DEFAULT_CONNECTIONS}'
```

**Sources:** [scripts/smoke-test.sh L85-L194](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L85-L194)

 [README.md L86-L117](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L86-L117)

### API Key Mismatch

**Symptom:** Data sent to OTEL collector but not associated with any team.

**Diagnosis:**

```css
# Check API key in app config
kubectl get secret my-hyperdx-hdx-oss-v2-app-secrets -o jsonpath='{.data.API_KEY}' | base64 -d

# Check API key in MongoDB teams collection
kubectl exec deployment/my-hyperdx-hdx-oss-v2-mongodb -- mongosh hyperdx --eval "
  db.teams.find({}, {name: 1, apiKey: 1}).pretty()
"

# Check OTEL collector config for API key
kubectl logs -l app=otel-collector | grep -i "api.*key\|authorization"
```

**Resolution:** Ensure API key matches between configuration and team in MongoDB.

**Sources:** [.github/workflows/chart-test.yml L109-L137](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L109-L137)

 [README.md L239-L273](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L239-L273)

---

## Scheduled Task Issues

### CronJob Not Running

**Symptom:** Alerts not firing, scheduled tasks not executing.

```mermaid
flowchart TD

CronJob["CronJob:<br>task-checkAlerts<br>schedule: */1 * * * *"]
Job["Job Pod"]
AppImage["hyperdx-app Image<br>Tag: 2.7.1"]
Command["Command Path"]
V2["Version < 2.0.2<br>yarn task:checkAlerts"]
V2New["Version >= 2.0.2<br>node build/tasks/src/tasks/index.js<br>--name checkAlerts"]

CronJob --> Job
Job --> AppImage
Job --> Command
Command --> V2
Command --> V2New
```

**Diagnosis:**

```markdown
# Check if cronjob exists
kubectl get cronjob

# Check cronjob schedule and status
kubectl describe cronjob my-hyperdx-hdx-oss-v2-task-checkalerts

# Check recent jobs
kubectl get jobs | grep checkalerts

# Check job logs
kubectl logs job/my-hyperdx-hdx-oss-v2-task-checkalerts-<timestamp>
```

**Common issues:**

**1. Wrong command for version (fixed in 0.8.4):**

Before chart version 0.8.4, the cronjob template didn't adapt the command path for newer app versions:

```css
# Check current command
kubectl get cronjob my-hyperdx-hdx-oss-v2-task-checkalerts -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].command}'

# Expected for appVersion >= 2.0.2:
# ["node", "build/tasks/src/tasks/index.js", "--name", "checkAlerts"]

# Expected for appVersion < 2.0.2:
# ["yarn", "task:checkAlerts"]
```

**Resolution:** Upgrade to chart version 0.8.4 or later.

**2. Environment variable misconfiguration:**

```markdown
# Check RUN_SCHEDULED_TASKS_EXTERNALLY is set
kubectl get deployment my-hyperdx-hdx-oss-v2-app -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="RUN_SCHEDULED_TASKS_EXTERNALLY")].value}'
# Should be "true" when tasks.enabled is true
```

**3. Resource constraints:**

```markdown
# Check if job pods are pending
kubectl get pods | grep checkalerts

# Check resource requests
kubectl describe cronjob my-hyperdx-hdx-oss-v2-task-checkalerts | grep -A 5 "Limits\|Requests"
```

**Sources:** [CHANGELOG.md L1-L14](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L1-L14)

 [CHANGELOG.md L86-L93](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L86-L93)

 [README.md L324-L333](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L324-L333)

---

## Log Collection for Support

### Comprehensive Log Collection

When reporting issues, collect comprehensive diagnostic information:

```php
#!/bin/bash
# Comprehensive diagnostic collection script

RELEASE_NAME="my-hyperdx"
NAMESPACE="default"
OUTPUT_DIR="hyperdx-diagnostics-$(date +%Y%m%d-%H%M%S)"

mkdir -p "$OUTPUT_DIR"

echo "Collecting HyperDX diagnostics..."

# Cluster info
kubectl cluster-info > "$OUTPUT_DIR/cluster-info.txt"
kubectl version > "$OUTPUT_DIR/kubectl-version.txt"

# Pod status
kubectl get pods -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o wide > "$OUTPUT_DIR/pods-status.txt"
kubectl describe pods -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE > "$OUTPUT_DIR/pods-describe.txt"

# Events
kubectl get events -n $NAMESPACE --sort-by=.metadata.creationTimestamp > "$OUTPUT_DIR/events.txt"

# Services and ingress
kubectl get services -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o yaml > "$OUTPUT_DIR/services.yaml"
kubectl get ingress -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o yaml > "$OUTPUT_DIR/ingress.yaml"

# ConfigMaps and Secrets (redacted)
kubectl get configmaps -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o yaml > "$OUTPUT_DIR/configmaps.yaml"
kubectl get secrets -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' > "$OUTPUT_DIR/secrets-list.txt"

# PVCs
kubectl get pvc -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o yaml > "$OUTPUT_DIR/pvcs.yaml"

# Component logs
kubectl logs -l app=app -n $NAMESPACE --tail=500 > "$OUTPUT_DIR/app-logs.txt" 2>&1
kubectl logs -l app=clickhouse -n $NAMESPACE --tail=500 > "$OUTPUT_DIR/clickhouse-logs.txt" 2>&1
kubectl logs -l app=mongodb -n $NAMESPACE --tail=500 > "$OUTPUT_DIR/mongodb-logs.txt" 2>&1
kubectl logs -l app=otel-collector -n $NAMESPACE --tail=500 > "$OUTPUT_DIR/otel-logs.txt" 2>&1

# Helm values
helm get values $RELEASE_NAME -n $NAMESPACE > "$OUTPUT_DIR/helm-values.yaml"
helm get manifest $RELEASE_NAME -n $NAMESPACE > "$OUTPUT_DIR/helm-manifest.yaml"

# CronJobs
kubectl get cronjobs -l app.kubernetes.io/instance=$RELEASE_NAME -n $NAMESPACE -o yaml > "$OUTPUT_DIR/cronjobs.yaml" 2>&1

echo "Diagnostics collected in $OUTPUT_DIR/"
echo "Create archive: tar czf $OUTPUT_DIR.tar.gz $OUTPUT_DIR/"
```

**Sources:** [.github/workflows/chart-test.yml L158-L177](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L158-L177)

### Per-Component Log Commands

| Component | Log Command | Key Log Patterns |
| --- | --- | --- |
| HyperDX App | `kubectl logs -l app=app --tail=100` | `ERROR`, `MongoDB`, `ClickHouse`, `ECONNREFUSED` |
| ClickHouse | `kubectl logs -l app=clickhouse --tail=100` | `Exception`, `Cannot bind`, `Data directory` |
| MongoDB | `kubectl logs -l app=mongodb --tail=100` | `error`, `unauthorized`, `connection` |
| OTEL Collector | `kubectl logs -l app=otel-collector --tail=100` | `connection refused`, `OpAMP`, `authentication` |
| CronJobs | `kubectl logs job/<job-name>` | `task:checkAlerts`, `error`, `MongoDB` |

**Sources:** [.github/workflows/chart-test.yml L167-L177](https://github.com/hyperdxio/helm-charts/blob/845dd482/.github/workflows/chart-test.yml#L167-L177)

---

## Health Check Verification

### Kubernetes Probes

```html
# Check liveness probe configuration
kubectl get deployment my-hyperdx-hdx-oss-v2-app -o jsonpath='{.spec.template.spec.containers[0].livenessProbe}'

# Check readiness probe configuration  
kubectl get deployment my-hyperdx-hdx-oss-v2-app -o jsonpath='{.spec.template.spec.containers[0].readinessProbe}'

# View probe history in events
kubectl get events --field-selector involvedObject.name=<pod-name> | grep -i probe
```

**Sources:** [CHANGELOG.md L76](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L76-L76)

### Manual Health Checks

```sql
# Test UI endpoint
kubectl port-forward service/my-hyperdx-hdx-oss-v2-app 3000:3000
curl -I http://localhost:3000

# Test API health endpoint
curl -I http://localhost:8000/health

# Test OTEL metrics endpoint
kubectl port-forward service/my-hyperdx-hdx-oss-v2-otel-collector 8888:8888
curl http://localhost:8888/metrics

# Test ClickHouse
kubectl exec deployment/my-hyperdx-hdx-oss-v2-clickhouse -- clickhouse-client --query "SELECT 1"

# Test MongoDB
kubectl exec deployment/my-hyperdx-hdx-oss-v2-mongodb -- mongosh --eval "db.adminCommand('ping')"
```

**Sources:** [scripts/smoke-test.sh L62-L83](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L62-L83)

 [scripts/smoke-test.sh L163-L177](https://github.com/hyperdxio/helm-charts/blob/845dd482/scripts/smoke-test.sh#L163-L177)

---

## Version-Specific Issues

### Upgrading from 0.8.3 or Earlier

**Issue:** CronJob fails after upgrade due to incorrect command path.

**Fixed in:** Chart version 0.8.4

**Workaround for older versions:**

```sql
# Manually update cronjob
kubectl edit cronjob my-hyperdx-hdx-oss-v2-task-checkalerts

# Change command to:
command: ["node", "build/tasks/src/tasks/index.js", "--name", "checkAlerts"]
```

**Sources:** [CHANGELOG.md L1-L14](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L1-L14)

### ClickHouse Version Upgrade (0.8.0)

**Issue:** ClickHouse upgraded to v25.7 requires careful migration.

**Steps for safe upgrade:**

```markdown
# 1. Backup data before upgrade
kubectl exec deployment/my-hyperdx-hdx-oss-v2-clickhouse -- clickhouse-client --query "BACKUP DATABASE default TO Disk('default', 'backup.zip')"

# 2. Perform Helm upgrade
helm upgrade my-hyperdx hyperdx/hdx-oss-v2 --version 0.8.0

# 3. Verify ClickHouse is running with new version
kubectl get pods -l app=clickhouse -o jsonpath='{.items[0].spec.containers[0].image}'

# 4. Check tables are accessible
kubectl exec deployment/my-hyperdx-hdx-oss-v2-clickhouse -- clickhouse-client --query "SHOW TABLES FROM default"
```

**Sources:** [CHANGELOG.md L28-L37](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L28-L37)

### Environment Variable Name Change (0.6.8)

**Issue:** `CRON_IN_APP_DISABLED` renamed to `RUN_SCHEDULED_TASKS_EXTERNALLY`.

**Impact:** Older configurations may have unexpected cron behavior.

**Resolution:**

```sql
# Update values.yaml
# Old (no longer works):
# hyperdx:
#   env:
#     CRON_IN_APP_DISABLED: "true"

# New:
tasks:
  enabled: true  # This sets RUN_SCHEDULED_TASKS_EXTERNALLY=true
```

**Sources:** [CHANGELOG.md L86](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L86-L86)

---

## Quick Reference: Common Fix Commands

| Problem | Quick Fix |
| --- | --- |
| Pod not starting | `kubectl describe pod <pod-name>` then check events |
| Image pull fail | `kubectl get secret` verify pull secrets exist |
| Config not applied | `kubectl rollout restart deployment/<name>` |
| API key not working | `helm upgrade --set hyperdx.apiKey=<key>` then restart |
| Ingress 404 | Check path regex and rewrite annotations |
| No data in UI | Verify API key, check OTEL logs, query ClickHouse |
| CronJob not running | Check command matches appVersion, verify schedule |
| ClickHouse connection fail | Verify clusterCidrs, check DNS resolution |
| OpAMP connection fail (GKE) | Use FQDN: `<service>.<ns>.svc.cluster.local` |
| MongoDB connection fail | Check MONGO_URI, verify pod is running |

**Sources:** [README.md L625-L631](https://github.com/hyperdxio/helm-charts/blob/845dd482/README.md#L625-L631)

 [CHANGELOG.md L1-L154](https://github.com/hyperdxio/helm-charts/blob/845dd482/CHANGELOG.md#L1-L154)

---

## Getting Help

When reporting issues, always include:

1. **Environment information:** * Kubernetes version: `kubectl version` * Cloud provider (GKE, EKS, AKS, on-prem) * Helm chart version: `helm list` * App version from Chart.yaml
2. **Configuration:** * Sanitized `helm get values` output * Custom values used during install
3. **Diagnostics:** * Pod status: `kubectl get pods` * Events: `kubectl get events` * Relevant logs from affected components
4. **Steps to reproduce:** * Deployment method and values * Actions that triggered the issue * Expected vs actual behavior

For more detailed monitoring and observability setup, see [Health Checks and Monitoring](/hyperdxio/helm-charts/8.1-health-checks-and-monitoring).