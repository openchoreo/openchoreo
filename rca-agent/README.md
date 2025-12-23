# RCA Agent for OpenChoreo

## Local Development

The following instructions are for local development. For deploying on OpenChoreo, refer to the [Deploying on OpenChoreo](#deploying-on-openchoreo) section.

### Prerequisites

- **[uv](https://docs.astral.sh/uv/)** package manager
- **LLM API access** - We recommend the latest models from OpenAI or Anthropic for best results
- **OpenSearch cluster** - Port-forward the OpenSearch service to your local machine
- **MCP servers** - Port-forward both the OpenChoreo MCP server and OpenChoreo Observability MCP server to your local machine

### Configuration

| Variable | Description |
|----------|-------------|
| `RCA_LLM_API_KEY` | API key for your LLM provider |
| `RCA_MODEL_NAME` | Model name to use for analysis |
| `OPENSEARCH_ADDRESS` | OpenSearch cluster address |
| `OPENSEARCH_USERNAME` | OpenSearch username |
| `OPENSEARCH_PASSWORD` | OpenSearch password |
| `MCP_SERVER_URL` | OpenChoreo MCP server URL |
| `OBSERVABILITY_MCP_SERVER_URL` | OpenChoreo Observability MCP server URL |

### Authentication (Optional)

If authentication is enabled, create an OAuth2 client credentials grant and configure:

| Variable | Description |
|----------|-------------|
| `OAUTH_TOKEN_URL` | OAuth2 token endpoint URL |
| `OAUTH_CLIENT_ID` | OAuth2 client ID |
| `OAUTH_CLIENT_SECRET` | OAuth2 client secret |

### Setup

Install dependencies:

```bash
uv sync
```

Run the development server:

```bash
uvicorn src.main:app --reload
```

## Deploying on OpenChoreo

The RCA agent is deployed as part of the Observability Plane. You can enable it during initial installation or by upgrading an existing deployment.

### Configuration Flags

```bash
--set rca.enabled=true
--set rca.llm.modelName="gpt-5"
--set rca.llm.apiKey="your-api-key"
--set rca.opensearch.username="admin"
--set rca.opensearch.password="ThisIsTheOpenSearchPassword1"
```

> **Note:** The OpenSearch credentials shown above are the defaults. Modify them accordingly for your environment.

> **Note:** For single cluster deployments, values for MCP URLs, OpenSearch URLs, and OAuth URLs should work out of the box. For multi-cluster deployments, you will need to configure these manually based on your domain. For example:
>
> ```bash
> --set rca.openchoreoMcpUrl="https://api.openchoreo.localhost:8080/mcp"
> --set rca.oauth.tokenUrl="https://thunder.openchoreo.localhost:8443/oauth2/token"
> ```
>
> Replace `openchoreo.localhost` with your actual domain.

### Using Existing Kubernetes Secrets

Instead of passing secrets via Helm values, you can reference pre-existing Kubernetes secrets:

```bash
--set rca.enabled=true
--set rca.llm.modelName="gpt-5"
--set rca.llm.existingSecret="my-llm-secret"
--set rca.opensearch.existingSecret="my-opensearch-secret"
--set rca.oauth.existingSecret="my-oauth-secret"
```

**Expected secret formats:**

| Secret Type | Required Keys |
|-------------|---------------|
| LLM | `RCA_LLM_API_KEY` |
| OpenSearch | `username`, `password` |
| OAuth | `client-secret` |

### Upgrading an Existing Installation (k3d)

If you followed the local k3d setup and have the Observability Plane installed, you can enable the RCA agent by running the following from the root of this repository:

```bash
helm upgrade openchoreo-observability-plane \
  install/helm/openchoreo-observability-plane \
  --kube-context k3d-openchoreo \
  --namespace openchoreo-observability-plane \
  --set rca.enabled=true \
  --set rca.llm.modelName="gpt-5" \
  --set rca.llm.apiKey="sk-proj-your-openai-api-key" \
  --set rca.opensearch.username="admin" \
  --set rca.opensearch.password="ThisIsTheOpenSearchPassword1"
```
