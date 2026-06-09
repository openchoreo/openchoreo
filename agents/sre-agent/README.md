# RCA Agent for OpenChoreo

An autonomous Root Cause Analysis agent that investigates alerts using observability data and LLMs.

## Architecture

### Overview

The RCA agent receives alerts via a REST API, then uses a **ReAct (Reasoning + Acting)** loop to investigate by querying observability data through MCP servers. It produces a structured RCA report with root causes, evidence, and recommendations.

When the experimental auto-remediation feature is enabled, a remediation agent runs after the RCA agent to review, revise, and apply recommended actions.

### MCP Integration

The agent connects to two MCP (Model Context Protocol) servers:
- **Observability MCP** – provides tools for traces, logs, and metrics
- **OpenChoreo MCP** – provides tools for projects, environments, and components

Only a whitelisted set of read-only tools are exposed to the agent, preventing unintended modifications and limiting scope to investigation.

### Middleware

Output middleware intercepts tool responses and transforms raw data before it reaches the LLM — computing metric statistics, detecting anomalies, building trace hierarchies, and grouping logs by component.

### API

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/agent/rca` | Queue an RCA analysis as a background task |
| `POST /api/v1/agent/chat` | Streaming chat for follow-up questions about a report |
| `GET /api/v1/rca-reports/projects/{id}` | List reports by project |
| `GET /api/v1/rca-reports/alerts/{id}` | Get a report by alert |

## Local Development

The following instructions are for local development. For deploying on OpenChoreo, refer docs.

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
| `CONTROL_PLANE_URL` | OpenChoreo control plane URL (MCP URL derived from this) |
| `OBSERVER_MCP_URL` | OpenChoreo Observability MCP server URL |
| `REMED_AGENT` | Enable remediation agent (default: false) |

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
fastapi dev src/main.py
```
