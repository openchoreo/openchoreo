# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from contextlib import asynccontextmanager

from dotenv import load_dotenv
from fastapi import FastAPI

from src.api import router
from src.core.llm import get_model
from src.core.mcp import MCPClient
from src.core.opensearch import get_opensearch_client
from src.logging_config import setup_logging

load_dotenv()
setup_logging()

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(_app: FastAPI):
    logger.info("Starting up: Testing LLM connection...")
    try:
        model = get_model()
        test_response = await model.ainvoke("Hello")
        logger.info("LLM test successful: %s", test_response.content[:50])
    except Exception as e:
        logger.error("LLM initialization failed: %s", e, exc_info=True)
        raise RuntimeError(f"LLM initialization failed: {e}") from e

    logger.info("Testing OpenSearch connection...")
    try:
        opensearch_client = get_opensearch_client()
        if opensearch_client.check_connection():
            logger.info("OpenSearch connection successful")
        else:
            logger.error("OpenSearch connection check failed")
            raise RuntimeError("OpenSearch connection check failed")
    except RuntimeError:
        raise
    except Exception as e:
        logger.error("OpenSearch initialization failed: %s", e, exc_info=True)
        raise RuntimeError(f"OpenSearch initialization failed: {e}") from e

    logger.info("Testing MCP connections...")
    try:
        mcp_client = MCPClient()
        tools = await mcp_client.get_tools()
        logger.info("MCP connection successful: loaded %d tools", len(tools))
    except Exception as e:
        logger.error("MCP initialization failed: %s", e, exc_info=True)
        raise RuntimeError(f"MCP initialization failed: {e}") from e

    yield

    logger.info("Shutting down...")


app = FastAPI(lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)

app.include_router(router)
