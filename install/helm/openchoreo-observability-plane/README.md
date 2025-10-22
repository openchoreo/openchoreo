# OpenChoreo Observability Plane

The OpenChoreo Observability Plane provides comprehensive logging and monitoring capabilities for the OpenChoreo platform. It includes OpenSearch for log storage and analysis, OpenSearch Dashboards for visualization, and the OpenChoreo Observer service for log retrieval and processing.

## Architecture Components

- **OpenSearch**: Distributed search and analytics engine for log storage
- **OpenSearch Dashboards**: Web interface for data visualization and exploration
- **OpenChoreo Observer**: Service that collects and forwards logs to OpenSearch
- **OpenSearch Operator** (Standard mode only): Kubernetes operator for managing OpenSearch clusters

## Installation Modes

The OpenChoreo Observability Plane supports three installation modes to accommodate different deployment scenarios and resource requirements:

### 1. Legacy Mode (Default)

**Use Case**: Development environments, single-node clusters, or minimal resource deployments.

**Components**:
- Single-replica OpenSearch instance deployed as StatefulSet
- Single-replica OpenSearch Dashboards
- OpenChoreo Observer service
- Basic authentication with default credentials

**Characteristics**:
- Minimal resource requirements
- Single-node OpenSearch deployment
- No high availability
- Security disabled for simplicity
- Fixed NodePort (30920) for external access

### 2. Minimal Mode

**Use Case**: This mode is intended to be replace the legacy mode in the future.

### 3. Standard Mode

**Use Case**: Production environments requiring high availability, scalability, and enterprise features.

**Components**:
- OpenSearch Operator for cluster management
- Multi-node OpenSearch cluster with dedicated master and data nodes
- OpenSearch Dashboards with operator management
- Enhanced security configuration

**Characteristics**:
- High availability with multiple replicas
- Separate master (3 replicas) and data (2 replicas) node pools
- Operator-managed lifecycle and scaling
- Enhanced security with TLS and authentication
- Production-ready configuration

## Installation

### Deploy Legacy Mode (Default)

```bash
# Create namespace
kubectl create namespace openchoreo-observability-plane

# Install with default values (Legacy mode)
helm install openchoreo-observability-plane . \
  --namespace openchoreo-observability-plane
```

### Deploy Minimal Mode

```bash
# Create namespace
kubectl create namespace openchoreo-observability-plane

# Install with minimal mode values
helm install openchoreo-observability-plane . \
  --namespace openchoreo-observability-plane \
  --values values-minimal.yaml
```

### Deploy Standard Mode

```bash
# Create namespace
kubectl create namespace openchoreo-observability-plane

# Install the OpenSearch operator chart
helm install opensearch-operator opensearch-operator/opensearch-operator \
  --namespace openchoreo-observability-plane \
  --values opensearch-operator/values.yaml

# Install with standard mode values
helm install openchoreo-observability-plane . \
  --namespace openchoreo-observability-plane \
  --values values-standard.yaml
```

## Accessing Services

### Legacy/Minimal Mode

After installation, services are accessible via NodePort:

```bash
# Get node IP
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')

# Access OpenSearch
curl http://$NODE_IP:30920

# Access OpenSearch Dashboards
kubectl port-forward service/opensearch-dashboard 5601:5601
# Then open http://localhost:5601
```

### Standard Mode

Services are managed by the operator and typically accessed via port-forwarding or ingress:

```bash
# Port-forward OpenSearch
kubectl port-forward service/opensearch 9200:9200

# Port-forward OpenSearch Dashboards
kubectl port-forward service/opensearch-dashboards-service 5601:5601
```

## Monitoring and Troubleshooting

### Check Installation Status

```bash
# Check all pods
kubectl get pods -n openchoreo-observability-plane

# Check services
kubectl get services -n openchoreo-observability-plane

# Check OpenSearch cluster health (Legacy/Minimal)
kubectl exec -it opensearch-0 -n openchoreo-observability-plane -- curl -s http://localhost:9200/_cluster/health

# Check OpenSearch cluster status (Standard mode)
kubectl get opensearchcluster -n openchoreo-observability-plane
```

### Common Issues

1. **Pod stuck in Pending**: Check resource availability and storage class
2. **OpenSearch not ready**: Verify memory limits and node resources
3. **Dashboard connection issues**: Ensure OpenSearch is healthy first
4. **Observer not collecting logs**: Check service account permissions and network policies

### Logs

```bash
# Observer logs
kubectl logs -f deployment/observer -n openchoreo-observability-plane

# OpenSearch logs (Legacy/Minimal)
kubectl logs -f opensearch-0 -n openchoreo-observability-plane

# OpenSearch Dashboards logs
kubectl logs -f deployment/opensearch-dashboard -n openchoreo-observability-plane
```

## Upgrading

### Upgrade Process

```bash
# Update Helm repository
helm repo update

# Upgrade installation
helm upgrade openchoreo-observability-plane . \
  --namespace openchoreo-observability-plane \
  --values <your-values-file>
```

## Support

For issues and questions:
- GitHub Issues: [OpenChoreo Issues](https://github.com/openchoreo/openchoreo/issues)
- Documentation: [OpenChoreo Docs](https://openchoreo.dev)
- Community: [OpenChoreo Community](https://github.com/openchoreo/openchoreo/discussions)
