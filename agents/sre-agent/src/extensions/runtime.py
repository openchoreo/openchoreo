# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Any

from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient

from src.config import settings
from src.extensions.config import ExternalServer, load_extensions
from src.extensions.skills import create_load_skill_tool

logger = logging.getLogger(__name__)


def _root_cause(error: BaseException) -> str:
    while isinstance(error, BaseExceptionGroup) and error.exceptions:
        error = error.exceptions[0]
    return f"{type(error).__name__}: {error}"


async def _load_external_tools(servers: tuple[ExternalServer, ...]) -> list[BaseTool]:
    if not servers:
        return []

    client = MultiServerMCPClient(
        {server.name: server.connection for server in servers},
        tool_name_prefix=True,
    )

    tools: list[BaseTool] = []
    for server in servers:
        try:
            loaded = await client.get_tools(server_name=server.name)
        except Exception as e:
            logger.error("Skipping MCP server %s: %s", server.name, _root_cause(e))
            logger.debug("MCP server %s failed", server.name, exc_info=True)
            continue
        if not loaded:
            logger.warning("MCP server %s is reachable but exposes no tools", server.name)
        else:
            logger.info(
                "Loaded %d tools from MCP server %s: %s",
                len(loaded),
                server.name,
                [t.name for t in loaded],
            )
        tools.extend(loaded)
    return tools


async def apply_extensions(agent_name: str) -> tuple[list[BaseTool], dict[str, Any]]:
    extensions = load_extensions(settings.extensions_dir, agent_name)

    external_tools = await _load_external_tools(extensions.servers)
    tools = list(external_tools)
    if extensions.skills:
        tools.append(create_load_skill_tool(extensions.skills))

    return tools, {
        "external_tools": external_tools,
        "skills": extensions.skills,
        "extra_context": extensions.context,
    }
