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

    yield

    logger.info("Shutting down...")
    await report_backend.close()


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
