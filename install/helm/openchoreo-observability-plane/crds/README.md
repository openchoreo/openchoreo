# OpenSearch Custom Resource Definitions (CRDs)

This directory contains the Custom Resource Definitions (CRDs) for the OpenSearch Operator from [Opster](https://github.com/Opster/opensearch-k8s-operator). These CRDs are explicitly included here rather than using the opensearch-operator subchart's `installCRDs: true` option.

## Why CRDs are Included Here

OpenChoreo Observability Plane Helm Chart installs opensearch-operator, and opensearch-cluster subcharts using a single Helm install. The opensearch-cluster sub-chart is dependant on several CRDs from the opensearch-operator subchart. Since both subcharts are installed together, the CRDs need to be installed here rather than in the opensearch-operator subchart.

## Installation Behavior

### Standard Mode (opensearch-operator.enabled: true)
- CRDs are installed as part of the Helm chart deployment
- CRDs are managed by Helm and will be updated/removed with the chart
- opensearch-operator subchart has `installCRDs: false` to prevent conflicts

### Minimal Mode (opensearch-operator.enabled: false)
- CRDs are not installed (conditional installation via Helm templates)
- No operator dependency, so CRDs are not needed

### Manual Installation
If needed, CRDs can be manually installed:
```bash
kubectl apply -f .
```

## Version

These CRDs are from opensearch-operator version 2.8.0, as specified in the parent chart's dependencies. The CRDs will be updated as the opensearch-operator version is updated.
