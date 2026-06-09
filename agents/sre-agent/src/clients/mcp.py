# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

import httpx
from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.config import settings

logger = logging.getLogger(__name__)


def _httpx_client_factory(
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        verify=not settings.tls_insecure_skip_verify,
    )


class MCPClient:
    def __init__(self, auth: httpx.Auth) -> None:
        obs_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.observer_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }
        oc_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.openchoreo_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }

        self._client = MultiServerMCPClient(
            {
                "observability": obs_connection,
                "openchoreo": oc_connection,
            }
        )
        logger.debug("Initialized MCP client with servers: observability, openchoreo")

    async def get_tools(self) -> list[BaseTool]:
        try:
            tools = await self._client.get_tools()
        except Exception as e:
            logger.error("Failed to fetch tools from MCP client: %s", e, exc_info=True)
            raise RuntimeError(f"Failed to fetch tools from MCP client: {e}") from e

        return tools
