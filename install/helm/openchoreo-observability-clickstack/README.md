# OpenChoreo Observability Plane - ClickStack

A standalone Helm chart for deploying ClickStack (ClickHouse + HyperDX) as the observability backend for OpenChoreo.

## Overview

ClickStack provides a high-performance, cost-effective observability solution using:

- **ClickHouse** - Columnar database optimized for analytical queries
- **HyperDX** - Modern observability UI with advanced correlation and analysis
- **OpenTelemetry Collector** - OTLP-native data ingestion
- **MongoDB** - Metadata storage for HyperDX

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner support in the underlying infrastructure (or disable persistence)
- At least 4 CPU cores and 8GB RAM available in your cluster

## Installation

### Quick Start

```bash
# Add HyperDX Helm repository
helm repo add hyperdxio https://hyperdxio.github.io/helm-charts/
helm repo update

# Install ClickStack
helm install openchoreo-observability-clickstack \
  ./install/helm/openchoreo-observability-clickstack \
  --namespace openchoreo-observability-clickstack \
  --create-namespace
```

### Custom Configuration

Create a custom values file:

```yaml
# custom-values.yaml
global:
  storageClassName: "standard"  # Your StorageClass

hyperdx:
  hyperdx:
    apiKey: "your-secure-api-key"
    frontendUrl: "https://hyperdx.example.com"

    ingress:
      enabled: true
      ingressClassName: nginx
      host: "hyperdx.example.com"
      tls:
        enabled: true
        secretName: "hyperdx-tls"

  clickhouse:
    persistence:
      data:
        size: 50Gi  # Adjust based on your data volume
      logs:
        size: 10Gi

    config:
      users:
        appUserPassword: "secure-password"  # Set a strong password
```

Install with custom values:

```bash
helm install openchoreo-observability-clickstack \
  ./install/helm/openchoreo-observability-clickstack \
  --namespace openchoreo-observability-clickstack \
  --create-namespace \
  -f custom-values.yaml
```

### Minimal Installation (No Persistence)

For testing without persistent storage:

```yaml
# minimal-values.yaml
hyperdx:
  clickhouse:
    persistence:
      enabled: false

  mongodb:
    persistence:
      enabled: false
```

```bash
helm install openchoreo-observability-clickstack \
  ./install/helm/openchoreo-observability-clickstack \
  --namespace openchoreo-observability-clickstack \
  --create-namespace \
  -f minimal-values.yaml
```

## Configuration

See [values.yaml](./values.yaml) for full configuration options.

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.storageClassName` | StorageClass for PVCs | `"standard"` |
| `hyperdx.enabled` | Enable HyperDX stack | `true` |
| `hyperdx.hyperdx.apiKey` | HyperDX API key | `"2c04a8a1-c7a4-445c-9919-ebd01289c6bd"` |
| `hyperdx.hyperdx.frontendUrl` | Frontend URL | `"http://localhost:3000"` |
| `hyperdx.hyperdx.ingress.enabled` | Enable ingress | `false` |
| `hyperdx.clickhouse.persistence.enabled` | Enable ClickHouse persistence | `true` |
| `hyperdx.clickhouse.persistence.data.size` | ClickHouse data volume size | `10Gi` |
| `hyperdx.mongodb.persistence.enabled` | Enable MongoDB persistence | `true` |
| `hyperdx.mongodb.persistence.size` | MongoDB volume size | `5Gi` |

## Access HyperDX UI

### Port Forward (Development)

```bash
kubectl port-forward -n openchoreo-observability-clickstack \
  svc/openchoreo-observability-clickstack-hyperdx-app 3000:3000
```

Open http://localhost:3000 in your browser.

### Ingress (Production)

Enable ingress in values.yaml and configure your domain:

```yaml
hyperdx:
  hyperdx:
    ingress:
      enabled: true
      ingressClassName: nginx
      host: "hyperdx.example.com"
      tls:
        enabled: true
        secretName: "hyperdx-tls"
```

## Integration with OpenChoreo Observability Plane

To integrate HyperDX with the existing OpenChoreo Observability Plane (OpenSearch/Fluent Bit), you can configure Fluent Bit to "dual-ship" logs to both OpenSearch and HyperDX.

### 1. Deploy ClickStack

Follow the installation instructions above to deploy the `openchoreo-observability-clickstack` chart.

### 2. Configure Fluent Bit in OpenChoreo Observability Plane

Update your `openchoreo-observability-clickstack` values to include an additional output for HyperDX.

Add the following to your `values.yaml` for `openchoreo-observability-clickstack`:

```yaml
fluentBit:
  config:
    outputs: |
      [OUTPUT]
          Name opensearch
          Host opensearch
          Generate_ID On
          HTTP_Passwd admin
          HTTP_User admin
          Logstash_Format On
          Logstash_DateFormat %Y-%m-%d
          Logstash_Prefix container-logs
          Match kube.*
          Port 9200
          Replace_Dots On
          Suppress_Type_Name On
          tls On
          tls.verify Off

      # Add this section for HyperDX integration
      [OUTPUT]
          Name http
          Match kube.*
          Host openchoreo-observability-clickstack-hyperdx-otel-collector
          Port 4318
          URI /v1/logs
          Format json
          Json_Date_Key time
          Json_Date_Format iso8601
          Header x-api-key 2c04a8a1-c7a4-445c-9919-ebd01289c6bd
```

> **Note**: Replace `2c04a8a1-c7a4-445c-9919-ebd01289c6bd` with your actual API key if you changed it.

### 3. Upgrade OpenChoreo Observability Plane

Apply the changes:

```bash
helm upgrade openchoreo-observability-clickstack \
  ./install/helm/openchoreo-observability-clickstack \
  --namespace openchoreo-observability-clickstack \
  -f your-values.yaml
```

## Sending Telemetry Data Directly

Configure your applications to send OTLP data to the OTEL Collector:

```bash
# Environment variables for OpenTelemetry SDK
OTEL_EXPORTER_OTLP_ENDPOINT=http://openchoreo-observability-clickstack-hyperdx-otel-collector:4318
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
```

### Endpoints

- **OTLP gRPC**: `openchoreo-observability-clickstack-hyperdx-otel-collector:4317`
- **OTLP HTTP**: `openchoreo-observability-clickstack-hyperdx-otel-collector:4318`

## Querying ClickHouse

### Using clickhouse-client

```bash
# Show tables
kubectl exec -n openchoreo-observability-clickstack \
  statefulset/openchoreo-observability-clickstack-hdx-oss-v2-clickhouse -- \
  clickhouse-client --query "SHOW TABLES"

# Count logs
kubectl exec -n openchoreo-observability-clickstack \
  statefulset/openchoreo-observability-clickstack-hdx-oss-v2-clickhouse -- \
  clickhouse-client --query "SELECT count() FROM otel_logs"

# View recent logs from dp-* namespaces
kubectl exec -n openchoreo-observability-clickstack \
  statefulset/openchoreo-observability-clickstack-hdx-oss-v2-clickhouse -- \
  clickhouse-client --query "
    SELECT
      Timestamp,
      ResourceAttributes['k8s.namespace.name'] as namespace,
      ResourceAttributes['k8s.pod.name'] as pod,
      Body
    FROM otel_logs
    WHERE namespace LIKE 'dp-%'
    ORDER BY Timestamp DESC
    LIMIT 10
  "
```

## Uninstallation

```bash
# Uninstall the chart
helm uninstall openchoreo-observability-clickstack -n openchoreo-observability-clickstack

# Delete PVCs (if you want to remove data)
kubectl delete pvc -n openchoreo-observability-clickstack -l app.kubernetes.io/instance=openchoreo-observability-clickstack
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n openchoreo-observability-clickstack
```

### View Logs

```bash
# HyperDX App
kubectl logs -n openchoreo-observability-clickstack -l app=hyperdx-app

# ClickHouse
kubectl logs -n openchoreo-observability-clickstack -l app=clickhouse

# OTEL Collector
kubectl logs -n openchoreo-observability-clickstack -l app=otel-collector

# MongoDB
kubectl logs -n openchoreo-observability-clickstack -l app=mongodb
```

### Common Issues

#### PVCs Stuck in Pending

**Problem**: PersistentVolumeClaims are not binding.

**Solution**:
- Check if StorageClass exists: `kubectl get storageclass`
- Install local-path-provisioner for local clusters
- Or disable persistence in values.yaml

#### OTEL Collector Restarting

**Problem**: OTEL Collector pod keeps restarting.

**Solution**:
- Check HyperDX App is running (OTEL Collector depends on it)
- View OTEL Collector logs for specific errors
- Ensure ClickHouse and MongoDB are healthy

## Documentation

For more information, see:

- [HyperDX Documentation](https://hyperdx.io/docs)

## Support

For issues and questions:

- GitHub Issues: https://github.com/openchoreo/openchoreo/issues
- Documentation: https://github.com/openchoreo/openchoreo/tree/main/docs
