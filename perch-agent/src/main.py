# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI

from src.agent.tool_registry import log_classification_summary
from src.api import agent_router
from src.auth.bearer import BearerTokenAuth
from src.auth.dependencies import _load_auth_config, get_authz_client
from src.auth.jwt import get_jwt_validator
from src.clients import MCPClient, get_model
from src.config import settings
from src.logging_config import setup_logging
from src.template_manager import preload as preload_templates

load_dotenv()
setup_logging()

logger = logging.getLogger(__name__)

if settings.jwks_url_tls_insecure_skip_verify:
    logger.warning("TLS certificate verification disabled for JWKS URL")
if settings.uid_resolver_tls_insecure_skip_verify:
    logger.warning(
        "TLS certificate verification disabled for UID resolver / internal API calls"
    )

if settings.jwt_insecure_allow_unverified:
    logger.warning(
        "JWT_INSECURE_ALLOW_UNVERIFIED is set — JWT signature, issuer, and "
        "audience checks may be skipped. This is a DEV-ONLY setting; production "
        "deployments MUST leave it False."
    )


@asynccontextmanager
async def lifespan(_app: FastAPI):
    # Fail-fast config validation: a runtime LLM 401 is a confusing user
    # experience; surface a clear startup error when the API key isn't set.
    if not settings.perch_llm_api_key:
        raise RuntimeError(
            "PERCH_LLM_API_KEY is not set — refusing to start. "
            "Set the env var (or values.yaml llm.apiKeySecret) and redeploy."
        )
    if not settings.perch_model_name:
        raise RuntimeError(
            "PERCH_MODEL_NAME is not set — refusing to start. "
            "Set the env var (or values.yaml llm.modelName) and redeploy."
        )

    # Preload prompt templates so a missing/broken Jinja2 file fails the
    # deploy instead of emitting a 500 on the first chat.
    logger.info("Preloading prompt templates...")
    try:
        preload_templates(["prompts/perch_prompt.j2"])
    except Exception as e:
        logger.error("Prompt template preload failed: %s", e)
        raise RuntimeError(f"Prompt template preload failed: {e}") from e

    logger.info("Starting up: testing LLM connection...")
    try:
        # Minimal probe: we only need to verify the endpoint is reachable
        # and the API key is valid — we don't need a useful response.
        # "Reply with exactly: Pong" steers the model to a 1-token answer;
        # max_tokens=5 caps billing even if a model ignores the prompt
        # framing (reasoning models occasionally do). Both knobs together
        # turn the per-pod-startup probe from ~50 tokens down to ~2.
        model = get_model()
        test_response = await model.ainvoke(
            "Reply with exactly: Pong",
            max_tokens=5,
        )
        logger.info("LLM test successful: %s", str(test_response.content)[:50])
    except Exception as e:
        logger.error("LLM initialization failed: %s", e)
        raise RuntimeError(f"LLM initialization failed: {e}") from e

    logger.info("Loading auth config...")
    try:
        _load_auth_config()
    except Exception as e:
        logger.error("Auth config loading failed: %s", e)
        raise RuntimeError(f"Auth config loading failed: {e}") from e

    # Construct auth singletons eagerly so the first request burst doesn't
    # race on lazy init, and so we have a stable reference to close on
    # shutdown. JWT validator may be DisabledJWTValidator when the
    # dev-only allow-unverified flag is set; that's fine.
    logger.info("Initialising auth singletons...")
    get_jwt_validator()
    authz_client = get_authz_client()

    # MCP availability check uses an empty bearer — we only verify the endpoint
    # responds. Actual tool listing always uses the user's bearer at request time.
    logger.info(
        "Probing MCP servers (openchoreo=%s, observability=%s)...",
        settings.openchoreo_mcp_url,
        settings.observer_mcp_url,
    )
    try:
        probe = MCPClient(auth=BearerTokenAuth(""))
        tools = await probe.get_tools()
        logger.info("MCP probe succeeded (%d tools)", len(tools))
        # Surface mutating-vs-read split so ops can spot a misclassified
        # write tool before a user discovers it. Logged as INFO once at
        # startup; tool catalog is stable for the lifetime of the pod.
        log_classification_summary(tools)
    except Exception as e:  # noqa: BLE001 — non-fatal at startup
        logger.warning("MCP probe failed (will retry on first request): %s", e)

    logger.info(
        "Startup complete (max_concurrent_chats=%d)",
        settings.max_concurrent_chats,
    )

    yield

    logger.info("Shutting down...")
    # Close the authz HTTP pool so in-flight requests drain cleanly before
    # the worker exits.
    await authz_client.close()


app = FastAPI(
    lifespan=lifespan,
    docs_url=None,
    redoc_url=None,
    openapi_url=None,
    strict_content_type=False,
)

if settings.cors_allowed_origins:
    from starlette.middleware.cors import CORSMiddleware

    origins = [o.strip() for o in settings.cors_allowed_origins.split(",") if o.strip()]
    if origins:
        app.add_middleware(
            CORSMiddleware,  # type: ignore[arg-type]
            allow_origins=origins,
            allow_methods=["GET", "POST", "OPTIONS"],
            allow_headers=["Content-Type", "Authorization"],
            max_age=3600,
        )
        logger.info("CORS enabled for origins: %s", origins)


@app.get("/health")
async def health():
    return {"status": "healthy"}


app.include_router(agent_router)
