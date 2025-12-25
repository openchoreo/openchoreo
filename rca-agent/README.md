# RCA Agent for OpenChoreo

## Local Development

The following instructions are for local development. For deploying on OpenChoreo, refer docs.

### Prerequisites

- **[uv](https://docs.astral.sh/uv/)** package manager
- **LLM API access** - We recommend the latest models from OpenAI or Anthropic for best results
- **OpenSearch cluster** - Port-forward the OpenSearch service to your local machine
- **MCP servers** - Port-forward both the OpenChoreo MCP server and OpenChoreo Observability MCP server to your local machine

```

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
