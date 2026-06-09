# Copyright 2026 The OpenChoreo Authors
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
    # Server names are kept separate from connection config to avoid
    # CodeQL taint-tracking false positives (py/clear-text-logging-sensitive-data).
    _SERVER_NAMES: list[str] = ["observability", "opencost"]

    def __init__(self, auth: httpx.Auth) -> None:
        self._servers: dict[str, StreamableHttpConnection] = {
            "observability": {
                "transport": "streamable_http",
                "url": settings.observability_mcp_server_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            },
            "opencost": {
                "transport": "streamable_http",
                "url": settings.opencost_mcp_server_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            },
        }

        self._client = MultiServerMCPClient(self._servers)
        logger.debug("Initialized MCP client with servers: observability, opencost")

    async def get_tools(self) -> list[BaseTool]:
        try:
            tools = await self._client.get_tools()
        except Exception as e:
            failed_names = await self._identify_unreachable_servers()
            if failed_names:
                server_names = ", ".join(failed_names)
                msg = f"Failed to connect to MCP server(s): {server_names}"
            else:
                msg = "Failed to fetch tools from MCP servers"
            logger.error(msg)
            # Log exception type only to avoid exposing sensitive data in exception message
            logger.debug("Exception type: %s", type(e).__name__)
            raise RuntimeError(msg) from e

        return tools

    async def close(self) -> None:
        """Close the MCP client and cleanup resources."""
        try:
            await self._client.cleanup()
        except Exception as e:
            # Log exception type only to avoid exposing sensitive data
            logger.warning("Error during MCP client cleanup: %s", type(e).__name__)

    async def _identify_unreachable_servers(self) -> list[str]:
        """
        Check which MCP servers are unreachable via a simple HTTP request.

        Returns:
            List of unreachable server names.
        """
        # Map server names to URLs without going through self._servers (which
        # contains auth data and triggers CodeQL taint-tracking alerts).
        server_urls: dict[str, str] = {
            "observability": settings.observability_mcp_server_url,
            "opencost": settings.opencost_mcp_server_url,
        }
        unreachable: list[str] = []
        for name in self._SERVER_NAMES:
            url = server_urls[name]
            try:
                async with httpx.AsyncClient(
                    verify=not settings.tls_insecure_skip_verify, timeout=5.0
                ) as client:
                    await client.get(url)
            except Exception:
                unreachable.append(name)
        return unreachable
