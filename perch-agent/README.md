# OpenChoreo Perch Agent

A control-plane copilot service that consumes the openchoreo MCP server and answers user questions over a streaming NDJSON chat endpoint. Mutating actions are proposed for user confirmation before execution.

## Status

V1 — backend only. The Backstage chat-view plugin is a follow-up workstream. Validate the service with `curl` against `/api/v1alpha1/perch-agent/chat` and `/execute`.

## Endpoints

- `POST /api/v1alpha1/perch-agent/chat` — NDJSON `StreamEvent` stream
- `POST /api/v1alpha1/perch-agent/execute` — Approve a previously-proposed action by `action_id`
- `GET  /health`

## Running locally

```bash
uv sync
uv run uvicorn src.main:app --port 8080
```

Required env vars:

- `PERCH_MODEL_NAME` — e.g. `openai:gpt-4o-mini`
- `PERCH_LLM_API_KEY`
- `JWT_JWKS_URL`, `JWT_ISSUER`, `JWT_AUDIENCE` — point at your IDP
- `OPENCHOREO_API_URL` — defaults to the in-cluster URL

See `src/config.py` for the full list.
