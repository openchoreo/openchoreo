# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

import httpx
from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.core.auth import get_oauth2_auth
from src.core.config import settings
from src.core.constants import obs_tools, oc_tools

logger = logging.getLogger(__name__)

OBS_MCP_TOOLS = (
    obs_tools.GET_TRACES,
    obs_tools.GET_COMPONENT_LOGS,
    obs_tools.GET_PROJECT_LOGS,
    obs_tools.GET_COMPONENT_RESOURCE_METRICS,
)

OC_MCP_TOOLS = (
    oc_tools.LIST_ENVIRONMENTS,
    oc_tools.LIST_NAMESPACES,
    oc_tools.LIST_PROJECTS,
    oc_tools.LIST_COMPONENTS,
)

ALLOWED_TOOLS: frozenset[str] = frozenset(OBS_MCP_TOOLS + OC_MCP_TOOLS)


def _insecure_httpx_client_factory(
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        verify=False,
    )


class MCPClient:
    def __init__(self) -> None:
        oauth_auth = get_oauth2_auth()

        obs_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.observer_mcp_url,
        }
        oc_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.openchoreo_mcp_url,
        }

        if oauth_auth:
            obs_connection["auth"] = oauth_auth
            oc_connection["auth"] = oauth_auth

        if settings.tls_insecure_skip_verify:
            obs_connection["httpx_client_factory"] = _insecure_httpx_client_factory
            oc_connection["httpx_client_factory"] = _insecure_httpx_client_factory

        self._client = MultiServerMCPClient(
            {
                "observability": obs_connection,
                "openchoreo": oc_connection,
            }
        )
        logger.debug("Initialized MCP client with servers: observability, openchoreo")

    async def get_tools(self) -> list[BaseTool]:
        try:
            available_tools = await self._client.get_tools()
        except Exception as e:
            logger.error("Failed to fetch tools from MCP client: %s", e, exc_info=True)
            raise RuntimeError(f"Failed to fetch tools from MCP client: {e}") from e

        filtered_tools = [tool for tool in available_tools if tool.name in ALLOWED_TOOLS]

        logger.debug(
            "Filtered to %d allowed tools: %s",
            len(filtered_tools),
            [tool.name for tool in filtered_tools],
        )

        return filtered_tools
