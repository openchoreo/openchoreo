# OpenChoreo Perch Agent

A read-only control-plane copilot service that consumes the openchoreo MCP server and answers user questions over a streaming NDJSON chat endpoint. Mutating tools are filtered out of the catalog and hard-refused by `WriteGuardMiddleware`; the agent can only inspect state, not change it.

## Status

V1 — backend only. The Backstage chat-view plugin is a follow-up workstream. Validate the service with `curl` against `/api/v1alpha1/portal-assistant/chat`.

## Endpoints

- `POST /api/v1alpha1/portal-assistant/chat` — NDJSON `StreamEvent` stream
- `POST /api/v1alpha1/portal-assistant/warmup` — Pre-warm the per-user MCP tool cache
- `POST /api/v1alpha1/portal-assistant/chat/confirm` — Save the check change on a confirm card the user clicked
- `GET  /health`

## Running locally

```bash
uv sync
uv run uvicorn src.main:app --port 8080
```

Required env vars:

- `PERCH_MODEL_NAME` — e.g. `openai:gpt-5`
- `PERCH_LLM_API_KEY`
- `JWT_JWKS_URL`, `JWT_ISSUER`, `JWT_AUDIENCE` — point at your IDP
- `OPENCHOREO_API_URL` — defaults to the in-cluster URL

See `src/config.py` for the full list.

## Check authoring (optional)

Check authoring chats use the check tools that Backstage's `mcp-actions` backend serves
(see `docs/scoring-compliance/check-authoring-mcp.md` in openchoreo/backstage-plugins).

- `BACKSTAGE_MCP_URL` — e.g. `http://backstage:7007/api/mcp-actions/v1/check-authoring`. Empty turns check authoring's tools off.
- `PORTAL_ASSISTANT_ACTION_SECRET` — signs confirm cards. Set the same value on every replica;
  when empty, each process uses its own random secret, so a card only works on the replica that made it.
  A card can be confirmed once, but the record of used cards is kept in each process: with several
  replicas sharing the secret, the same card could be confirmed once per replica within its 5 minutes.
  For an edit that saves the same change again; for a new check it could create a second copy. The chart
  runs one replica by default; with more, route a user's requests to one replica (sticky sessions).
- `BACKSTAGE_MCP_TOKEN` — optional static token from Backstage's `backend.auth.externalAccess`, used
  only when a chat has no token issued for the signed-in user. Backstage treats it as a service, which
  may read every check and entity's facts, so leave it unset unless you need that fallback.

With the Helm chart, set them through `portalAssistant.extraEnvs`, using a `secretKeyRef` for the two secrets.
