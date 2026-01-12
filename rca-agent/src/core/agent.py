# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from deepagents.middleware import FilesystemMiddleware
from langchain.agents import create_agent
from langchain.agents.middleware import TodoListMiddleware
from langchain.agents.structured_output import ToolStrategy
from langchain_core.callbacks import BaseCallbackHandler
from langchain_core.runnables import RunnableConfig

from src.core.config import settings
from src.core.mcp import MCPClient
from src.core.middleware import LoggingMiddleware, OutputProcessorMiddleware, TimingMiddleware
from src.core.models.rca_report import RCAReport
from src.core.prompts.system_prompt import get_system_prompt

logger = logging.getLogger(__name__)

async def create_rca_agent(model, usage_callback: BaseCallbackHandler | None = None):
    mcp_client = MCPClient()
    tools = await mcp_client.get_tools()

    prompt = get_system_prompt(tools)

    middleware = [TimingMiddleware(), OutputProcessorMiddleware(), TodoListMiddleware()]

    if settings.debug:
        middleware.append(LoggingMiddleware())
    if settings.use_filesystem:
        middleware.append(FilesystemMiddleware())

    config: RunnableConfig = {"recursion_limit": 200}
    if usage_callback is not None:
        config["callbacks"] = [usage_callback]

    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=prompt,
        response_format=ToolStrategy(RCAReport),
        middleware=middleware,
    ).with_config(config)

    logger.info("Created RCA agent with %d tools: %s", len(tools), [tool.name for tool in tools])

    return agent
