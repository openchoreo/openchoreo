# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.core.auth import get_oauth2_auth
from src.core.config import settings
from src.core.constants import obs_tools, oc_tools

logger = logging.getLogger(__name__)

# MCP Server registry and configuration
MCP_CONFIG: dict[str, dict] = {
    "observability": {
        "env_url_key": "mcp_observability_url",
        "allowed_tools": [
            obs_tools.GET_TRACES,
            obs_tools.GET_COMPONENT_LOGS,
            obs_tools.GET_PROJECT_LOGS,
            obs_tools.GET_COMPONENT_RESOURCE_METRICS,
        ],
        "requires_auth": True,
    },
    "openchoreo": {
        "env_url_key": "mcp_openchoreo_url",
        "allowed_tools": [
            oc_tools.LIST_ENVIRONMENTS,
            oc_tools.LIST_ORGANIZATIONS,
            oc_tools.LIST_PROJECTS,
            oc_tools.LIST_COMPONENTS,
        ],
        "requires_auth": True,
    },
}


class MCPClient:
    def __init__(self):
        oauth_auth = get_oauth2_auth()

        mcp_config = {}
        for name, config in MCP_CONFIG.items():
            connection: StreamableHttpConnection = {
                "transport": "streamable_http",
                "url": getattr(settings, config["env_url_key"]),
            }
            if config.get("requires_auth") and oauth_auth:
                connection["auth"] = oauth_auth
            mcp_config[name] = connection

        self._client = MultiServerMCPClient(mcp_config)
        logger.info("Initialized MCP client with servers: %s", list(mcp_config.keys()))

    async def get_tools(self) -> list[BaseTool]:
        try:
            available_tools = await self._client.get_tools()
        except Exception as e:
            logger.error("Failed to fetch tools from MCP client: %s", e, exc_info=True)
            raise RuntimeError(f"Failed to fetch tools from MCP client: {e}") from e

        allowed_tools = [tool for config in MCP_CONFIG.values() for tool in config["allowed_tools"]]
        filtered_tools = [tool for tool in available_tools if tool.name in allowed_tools]

        logger.info(
            "Filtered to %d allowed tools: %s",
            len(filtered_tools),
            [tool.name for tool in filtered_tools],
        )

        return filtered_tools
