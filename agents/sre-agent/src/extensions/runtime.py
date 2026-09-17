# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from dataclasses import dataclass, field
from typing import Any

from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient

from src.config import settings
from src.extensions.config import (
    ExtensionConfigError,
    Extensions,
    ExternalServer,
    Skill,
    load_extensions,
)
from src.extensions.skills import create_load_skill_tool

logger = logging.getLogger(__name__)

# Discovery holds a concurrency slot, so keep it under the adapter's read timeout.
_DISCOVERY_TIMEOUT_SECONDS = 10


@dataclass(frozen=True)
class LoadedExtensions:
    tools: list[BaseTool] = field(default_factory=list)
    external_tools: list[BaseTool] = field(default_factory=list)
    skills: tuple[Skill, ...] = ()
    context: str = ""

    def prompt_context(self) -> dict[str, Any]:
        return {
            "external_tools": self.external_tools,
            "skills": self.skills,
            "extra_context": self.context,
        }


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
            loaded = await asyncio.wait_for(
                client.get_tools(server_name=server.name), _DISCOVERY_TIMEOUT_SECONDS
            )
        except TimeoutError:
            logger.error(
                "Skipping MCP server %s: did not respond within %ds",
                server.name,
                _DISCOVERY_TIMEOUT_SECONDS,
            )
            continue
        except Exception as e:
            logger.error("Skipping MCP server %s: %s", server.name, _root_cause(e))
            logger.debug("MCP server %s failed", server.name, exc_info=True)
            continue
        if loaded:
            logger.info(
                "Loaded %d tools from MCP server %s: %s",
                len(loaded),
                server.name,
                [t.name for t in loaded],
            )
        else:
            logger.warning("MCP server %s is reachable but exposes no tools", server.name)
        tools.extend(loaded)
    return tools


def read_extensions(agent_name: str) -> Extensions:
    try:
        return load_extensions(settings.extensions_dir, agent_name)
    except (ExtensionConfigError, OSError) as e:
        logger.error("Ignoring extensions for %s: %s", agent_name, e)
        return Extensions()


async def apply_extensions(agent_name: str) -> LoadedExtensions:
    extensions = read_extensions(agent_name)

    external_tools = await _load_external_tools(extensions.servers)
    tools = list(external_tools)
    if extensions.skills:
        tools.append(create_load_skill_tool(extensions.skills))

    return LoadedExtensions(
        tools=tools,
        external_tools=external_tools,
        skills=extensions.skills,
        context=extensions.context,
    )
