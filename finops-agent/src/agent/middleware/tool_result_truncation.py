# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.config import settings

logger = logging.getLogger(__name__)

_TRUNCATION_NOTICE = (
    "\n\n[NOTE: Result truncated from {original_len} to {max_len} characters. "
    "The data above is a representative subset. Work with the available data.]"
)


class ToolResultTruncationMiddleware(AgentMiddleware):
    """Truncates tool results that exceed a configurable character limit.

    This prevents large tool responses (e.g. verbose time-series data) from
    bloating the conversation context and causing progressively slower
    model calls.
    """

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        result = await handler(request)

        if not isinstance(result, ToolMessage) or not isinstance(result.content, str):
            return result

        max_len = settings.tool_result_max_chars
        if len(result.content) <= max_len:
            return result

        original_len = len(result.content)
        tool_name = request.tool_call.get("name")
        notice_text = _TRUNCATION_NOTICE.format(
            original_len=original_len, max_len=max_len
        )
        content_max_len = max_len - len(notice_text)
        truncated = result.content[:content_max_len] + notice_text

        logger.info(
            "Tool '%s' result truncated from %d to %d chars",
            tool_name,
            original_len,
            max_len,
        )

        result.content = truncated
        return result
