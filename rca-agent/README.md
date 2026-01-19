# RCA Agent for OpenChoreo

An autonomous Root Cause Analysis agent that investigates alerts using observability data and LLMs.

## Architecture

### Overview

The RCA agent receives alerts via a REST API, then uses a **ReAct (Reasoning + Acting)** loop to investigate by querying observability data through MCP servers. It produces a structured `RCAReport` with root causes, evidence, and recommendations.

### MCP Integration

The agent connects to two MCP (Model Context Protocol) servers:
- **Observability MCP** – provides tools for traces, logs, and metrics
- **OpenChoreo MCP** – provides tools for projects, environments, and components

### Tool Filtering

Only a whitelisted set of read-only tools are exposed to the agent:
- `get_traces`, `get_component_logs`, `get_project_logs`, `get_component_resource_metrics`
- `list_environments`, `list_projects`, `list_components`

This prevents unintended modifications and limits the agent's scope to investigation.

### Middleware

**OutputProcessorMiddleware** intercepts tool responses and transforms raw data before it reaches the LLM—computing metric statistics, detecting anomalies, building trace hierarchies, and grouping logs by component.

### Request Flow

1. `POST /analyze` queues analysis as a background task
2. Agent executes a ReAct loop, calling MCP tools and processing responses through middleware
3. Produces an `RCAReport` stored in OpenSearch

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
