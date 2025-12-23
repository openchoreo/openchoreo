# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.core.mcp import MCP_CONFIG
from src.core.template_manager import render


def get_system_prompt(tools: list) -> str:
    """
    Generate the RCA agent system prompt based on available tools.

    Args:
        tools: List of available tools

    Returns:
        Rendered system prompt
    """
    context = {
        "observability_tools": [
            tool for tool in tools if tool.name in MCP_CONFIG["observability"]["allowed_tools"]
        ],
        "openchoreo_tools": [
            tool for tool in tools if tool.name in MCP_CONFIG["openchoreo"]["allowed_tools"]
        ],
    }

    return render("prompts/system_prompt.j2", context)
