# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import re
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

logger = logging.getLogger(__name__)


# Tool errors that originate from MCP servers / SDK exceptions occasionally
# carry partial secrets (Bearer headers, KEY=… values) or stack traces
# pointing at internal IPs. The error string is fed back into the LLM
# context (which then shows up in the user-facing reply), so we redact
# obvious patterns and bound the length before forwarding.
_REDACT_PATTERNS: tuple[re.Pattern[str], ...] = (
    re.compile(r"(?i)bearer\s+[A-Za-z0-9\-._~+/=]+"),
    re.compile(r"(?i)\b(authorization|token|api[_-]?key|secret|password)\s*[:=]\s*\S+"),
    re.compile(r"(?i)(KEY|TOKEN|SECRET|PASSWORD|PWD)=\S+"),
)
_MAX_ERROR_CHARS = 800


def _scrub(text: str) -> str:
    for pat in _REDACT_PATTERNS:
        text = pat.sub("[REDACTED]", text)
    if len(text) > _MAX_ERROR_CHARS:
        text = text[:_MAX_ERROR_CHARS] + f"… [truncated, {len(text)} chars]"
    return text


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
            scrubbed = _scrub(str(e))
            logger.warning("Tool '%s' error: %s", tool_name, scrubbed)
            return ToolMessage(
                content=f"Error: {scrubbed}",
                tool_call_id=request.tool_call.get("id", ""),
                name=tool_name,
                status="error",
            )
