# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI

from src.api import agent_router, report_router
from src.auth import check_oauth2_connection, get_oauth2_auth
from src.auth.dependencies import _load_auth_config
from src.clients import MCPClient, get_model, get_report_backend
from src.config import settings
from src.logging_config import setup_logging
from src.mcp_server import drain_background_tasks, make_mcp_app, mcp_server

load_dotenv()
setup_logging()

logger = logging.getLogger(__name__)

if settings.tls_insecure_skip_verify:
    logger.warning("TLS certificate verification disabled")


@asynccontextmanager
async def lifespan(_app: FastAPI):
    logger.info("Starting up: Testing LLM connection...")
    try:
        model = get_model()
        test_response = await model.ainvoke("Hello")
        logger.info("LLM test successful: %s", test_response.content[:50])
    except Exception as e:
        logger.error("LLM initialization failed: %s", e)
        raise RuntimeError(f"LLM initialization failed: {e}") from e

    logger.info("Initializing report backend...")
    try:
        report_backend = get_report_backend()
        await report_backend.initialize()
    except Exception as e:
        logger.error("Report backend initialization failed: %s", e)
        raise RuntimeError(f"Report backend initialization failed: {e}") from e

    logger.info("Testing OAuth2 token endpoint...")
    try:
        await check_oauth2_connection()
        logger.info("OAuth2 connection successful")
    except Exception as e:
        logger.error("OAuth2 initialization failed: %s", e)
        raise RuntimeError(f"OAuth2 initialization failed: {e}") from e

    logger.info("Loading auth config...")
    try:
        _load_auth_config()
        logger.info("Auth config loaded successfully")
    except Exception as e:
        logger.error("Auth config loading failed: %s", e)
        raise RuntimeError(f"Auth config loading failed: {e}") from e

    logger.info("Testing MCP connections...")
    try:
        mcp_client = MCPClient(auth=get_oauth2_auth())
        tools = await mcp_client.get_tools()
        logger.info("MCP connection successful: loaded %d tools", len(tools))
    except Exception as e:
        logger.error("MCP initialization failed: %s", e)
        raise RuntimeError(f"MCP initialization failed: {e}") from e

    # Enter the FastMCP streamable-HTTP session manager so the /mcp sub-app
    # can serve requests. Without this, requests to /mcp 500 with
    # "Task group not initialized".
    #
    # The try/finally below guarantees cleanup runs even if an exception
    # propagates out of the yield (e.g. uvicorn aborts the lifespan on a
    # signal). Without it, an exceptional shutdown would skip
    # drain_background_tasks and report_backend.close() — leaving
    # in-flight analyses with reports stuck in 'pending' and leaking the
    # connection pool until process exit.
    #
    # Cleanup order matters: session_manager exits first (stops accepting
    # new MCP requests), then we drain in-flight tasks, then close the
    # backend they were writing to.
    try:
        async with mcp_server.session_manager.run():
            logger.info("MCP server (streamable HTTP) ready at /mcp")
            yield
    finally:
        logger.info("Shutting down...")
        # Wait for any in-flight analyze_runtime_state tasks to finish
        # writing their RCA report before we close the report backend
        # out from under them. Bounded so a stuck task can't block
        # shutdown past Kubernetes' grace period.
        try:
            await drain_background_tasks(timeout=30.0)
        except Exception as e:  # noqa: BLE001
            # Don't let a drain failure prevent the backend close.
            logger.error("drain_background_tasks failed: %s", e, exc_info=True)
        try:
            await report_backend.close()
        except Exception as e:  # noqa: BLE001
            logger.error("report_backend.close failed: %s", e, exc_info=True)


app = FastAPI(lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None, strict_content_type=False)

# Configure CORS if allowed origins are specified
if settings.cors_allowed_origins:
    from starlette.middleware.cors import CORSMiddleware

    origins = [o.strip() for o in settings.cors_allowed_origins.split(",") if o.strip()]
    if origins:
        app.add_middleware(
            CORSMiddleware,  # type: ignore[arg-type]
            allow_origins=origins,
            allow_methods=["GET", "POST", "PUT", "DELETE", "OPTIONS"],
            allow_headers=["Content-Type", "Authorization"],
            max_age=3600,
        )
        logger.info("CORS enabled for origins: %s", origins)


@app.get("/health")
async def health():
    return {"status": "healthy"}


app.include_router(agent_router)
app.include_router(report_router)

# Mount the MCP server (streamable HTTP) so other agents — e.g. the
# control-plane assistant-agent — can call list_rca_reports /
# get_rca_report / analyze_runtime_state via MCP. The mounted app
# enforces JWT auth on every request via its own middleware.
app.mount("/mcp", make_mcp_app())
