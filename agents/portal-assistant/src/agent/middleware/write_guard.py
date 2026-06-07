# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.agent.tool_registry import is_mutating

logger = logging.getLogger(__name__)


class WriteGuardMiddleware(AgentMiddleware):
    """Hard-refuse every mutating tool call.

    The agent is read-only: the build/runtime cases only ever need to
    inspect state, and the approval/execute flow that used to surface
    proposed writes has been removed. Mutating tools are filtered out of
    the catalog at build time (see ``builder._filter_tools_for_case``),
    so the model normally cannot see them. This guard is a backstop for
    the case where a tool's mutating-ness is misclassified or a new MCP
    tool slips into the catalog before its classification is updated.
    """

    def __init__(self, tools_by_name: dict[str, object]) -> None:
        super().__init__()
        self._tools_by_name = tools_by_name

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        name = request.tool_call.get("name", "")

        tool = self._tools_by_name.get(name)
        if tool is None or not is_mutating(tool):
            return await handler(request)

        logger.info(
            "WriteGuard: refusing mutating tool '%s' — agent is read-only",
            name,
        )
        return ToolMessage(
            content=(
                f"WriteGuard: tool '{name}' is a mutating operation. This "
                "assistant is read-only and cannot perform writes. Answer "
                "the user's question using read tools only, then call "
                "ChatResponse. If the user is asking to make a change, "
                "explain that you can only read state and direct them to "
                "the OpenChoreo UI or CLI for write operations."
            ),
            tool_call_id=request.tool_call.get("id", ""),
            name=name,
            status="error",
        )
