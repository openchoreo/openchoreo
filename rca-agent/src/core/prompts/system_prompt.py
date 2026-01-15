# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from langchain_core.tools import BaseTool

from src.core.mcp import OBS_MCP_TOOLS, OC_MCP_TOOLS
from src.core.template_manager import render


def get_system_prompt(tools: list[BaseTool]) -> str:
    """
    Generate the RCA agent system prompt based on available tools.

    Args:
        tools: List of available tools

    Returns:
        Rendered system prompt
    """
    context = {
        "observability_tools": [tool for tool in tools if tool.name in OBS_MCP_TOOLS],
        "openchoreo_tools": [tool for tool in tools if tool.name in OC_MCP_TOOLS],
    }

    return render("prompts/system_prompt.j2", context)
