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
# pointing at internal IPs / k8s service hostnames. The error string is fed
# back into the LLM context (which then shows up in the user-facing reply),
# so we redact obvious patterns and bound the length before forwarding.
#
# Port-suffix handling: ":<port>" is *only* matched as a tail on the IP /
# hostname patterns. A bare ":\d+" regex would mangle benign error text
# (e.g. "error 500: not found", timestamps, "line 5:10").
_REDACT_PATTERNS: tuple[re.Pattern[str], ...] = (
    # Credentials / secrets.
    re.compile(r"(?i)bearer\s+[A-Za-z0-9\-._~+/=]+"),
    re.compile(r"(?i)\b(authorization|token|api[_-]?key|secret|password)\s*[:=]\s*\S+"),
    re.compile(r"(?i)(KEY|TOKEN|SECRET|PASSWORD|PWD)=\S+"),
    # Internal endpoints. Each IP / hostname pattern accepts an optional
    # ":<port>" tail so "10.0.0.5:8080" is redacted whole.
    # URLs containing .svc — match first so the URL is redacted as one unit
    # before the bare-hostname pattern below would partial-match it.
    re.compile(r"\bhttps?://[^\s/]*\.svc[^\s]*"),
    # K8s service DNS: "<name>.<ns>.svc" (optionally ".cluster.local") with optional port.
    re.compile(r"\b[a-z0-9][a-z0-9.-]*\.svc(?:\.cluster\.local)?(?::\d+)?\b"),
    # RFC1918 private IPv4 ranges with optional :port.
    re.compile(r"\b10\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?\b"),
    re.compile(r"\b192\.168\.\d{1,3}\.\d{1,3}(?::\d+)?\b"),
    re.compile(r"\b172\.(?:1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3}(?::\d+)?\b"),
    # Loopback (127.0.0.0/8 + literal "localhost") with optional :port.
    re.compile(r"\b127\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?\b"),
    re.compile(r"(?i)\blocalhost(?::\d+)?\b"),
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
