# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import time
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware, ModelRequest, ModelResponse
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

logger = logging.getLogger(__name__)


class TimingMiddleware(AgentMiddleware):
    """Track execution time for model calls and tool calls."""

    def __init__(self) -> None:
        super().__init__()
        self.model_call_count: int = 0
        self.tool_call_count: int = 0

    async def awrap_model_call(
        self,
        request: ModelRequest,
        handler: Callable[[ModelRequest], Awaitable[ModelResponse]],
    ) -> ModelResponse:
        start_time = time.time()
        result = await handler(request)
        elapsed = time.time() - start_time

        self.model_call_count += 1
        logger.info("Model call #%d took %.2fs", self.model_call_count, elapsed)

        return result

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        tool_name = request.tool_call.get("name")
        start_time = time.time()

        result = await handler(request)

        elapsed = time.time() - start_time
        self.tool_call_count += 1
        logger.info("Tool '%s' (#%d) took %.2fs", tool_name, self.tool_call_count, elapsed)

        return result
