# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import time
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware, ModelRequest, ModelResponse
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langchain_core.messages import AIMessage, HumanMessage
from langgraph.types import Command

logger = logging.getLogger(__name__)


class LoggingMiddleware(AgentMiddleware):
    def __init__(self) -> None:
        super().__init__()
        self.model_call_count: int = 0
        self.tool_call_count: int = 0

    async def awrap_model_call(
        self,
        request: ModelRequest,
        handler: Callable[[ModelRequest], Awaitable[ModelResponse]],
    ) -> ModelResponse:
        # Find last AI message and get everything after it
        last_ai_idx = -1
        for i in range(len(request.messages) - 1, -1, -1):
            if isinstance(request.messages[i], AIMessage):
                last_ai_idx = i
                break

        new_messages = request.messages[last_ai_idx + 1 :]

        for message in new_messages:
            if isinstance(message, HumanMessage):
                logger.debug("Human message: %s", message.content)

        self.model_call_count += 1
        logger.debug("Starting model call #%d", self.model_call_count)

        start_time = time.time()
        result = await handler(request)
        elapsed = time.time() - start_time

        logger.info("Model call #%d completed in %.2fs", self.model_call_count, elapsed)

        ai_message = result.result[0]

        if isinstance(ai_message, AIMessage) and ai_message.tool_calls:
            for tool_call in ai_message.tool_calls:
                logger.debug(
                    "Tool call: %s with args: %s", tool_call.get("name"), tool_call.get("args")
                )

        return result

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        tool_name = request.tool_call.get("name")
        tool_args = request.tool_call.get("args")
        start_time = time.time()

        result = await handler(request)

        elapsed = time.time() - start_time
        self.tool_call_count += 1

        if isinstance(result, ToolMessage) and result.content:
            if isinstance(result.content, str):
                content_len = len(result.content)
            elif isinstance(result.content, list):
                content_len = sum(len(b.get("text", "")) for b in result.content if isinstance(b, dict))
            else:
                content_len = 0
        else:
            content_len = 0
        logger.info(
            "Tool '%s' (#%d) took %.2fs, result: %d chars",
            tool_name,
            self.tool_call_count,
            elapsed,
            content_len,
        )
        if isinstance(result, ToolMessage):
            logger.debug("Tool '%s' result content: %s", tool_name, result.content)
        logger.debug("Tool '%s' args: %s", tool_name, tool_args)

        return result
