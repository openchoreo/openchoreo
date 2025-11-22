# OpenChoreo RCA Agent

AI Root Cause Analysis agent addon for OpenChoreo Observability Plane.

## Prerequisites

- OpenChoreo Observability Plane must be installed in the target namespace
- OpenChoreo API must be enabled in the control plane
- LLM API credentials (Anthropic, OpenAI, Azure OpenAI, or Google GenAI)

## Installation

### Quick Start

### Example installation with OpenAI

```bash
helm install rca-agent install/helm/openchoreo-observability-plane/addons/rca \
  --namespace openchoreo-observability-plane \
  --set rcaService.llm.apiKey="sk-proj-your-api-key" \
  --set rcaService.llm.provider="openai" \
  --set rcaService.llm.modelName="gpt-5"
```

### Installation with External Secrets

Only the LLM API key should be configured in the secret key vault

```bash
# Ensure your ClusterSecretStore exists and contains the LLM API key
helm install rca-agent install/helm/openchoreo-observability-plane/addons/rca \
  --namespace openchoreo-observability-plane \
  --set rcaService.llm.externalSecret.enabled=true \
  --set rcaService.llm.externalSecret.secretStoreRef="prod-vault-store" \
  --set rcaService.llm.externalSecret.apiKeyRef.key="llm/anthropic" \
  --set rcaService.llm.externalSecret.apiKeyRef.property="apiKey" \
  --set rcaService.llm.provider="anthropic" \
  --set rcaService.llm.modelName="claude-sonnet-4-5"
```

## Configuration

### LLM Provider Configuration

The RCA agent supports four LLM providers:

| Provider | Value | Required Fields |
|----------|-------|----------------|
| Anthropic Claude | `anthropic` | `apiKey`, `modelName` |
| OpenAI | `openai` | `apiKey`, `modelName` |
| Azure OpenAI | `azureopenai` | `apiKey`, `modelName`, `azure.endpoint`, `azure.apiVersion`, `azure.deployment` |
| Google GenAI | `googlegenai` | `apiKey`, `modelName` |

### Verify Configuration

```bash
# Check ConfigMap
kubectl get configmap -n openchoreo-observability-plane -l app.kubernetes.io/name=openchoreo-observer-rca -o yaml

# Check Secret (values are base64 encoded)
kubectl get secret -n openchoreo-observability-plane -l app.kubernetes.io/name=openchoreo-observer-rca -o yaml
```

## Advanced Configuration

For advanced configuration options, create a custom `values.yaml` file:

```bash
helm install rca-agent install/helm/openchoreo-observability-plane/addons/rca \
  --namespace openchoreo-observability-plane \
  --values custom-values.yaml
```

See `values.yaml` for all available configuration options.
