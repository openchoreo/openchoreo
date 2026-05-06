# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

logger = logging.getLogger(__name__)


class ToolErrorHandlerMiddleware(AgentMiddleware):
    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        try:
            return await handler(request)
        except Exception as e:
            tool_name = request.tool_call.get("name")
            logger.error("Tool '%s' failed: %s", tool_name, type(e).__name__)
            return ToolMessage(
                content="An error occurred while executing the tool",
                tool_call_id=request.tool_call.get("id", ""),
                name=tool_name,
                status="error",
            )
