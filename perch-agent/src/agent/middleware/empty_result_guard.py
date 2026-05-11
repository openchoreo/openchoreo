# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

logger = logging.getLogger(__name__)


# Names of observability/query tools whose empty results indicate "no data
# in the requested window" rather than "wrong arguments". Repeatedly
# retrying these with new windows almost never finds data on the next try
# — the right behavior is to summarize what's missing and stop.
#
# Add a tool here only when its empty result is information ("nothing is
# happening") rather than a hint to retry differently.
_EMPTY_GUARD_TOOLS: frozenset[str] = frozenset({
    # observer plane
    "query_component_logs",
    "query_workflow_logs",
    "query_alerts",
    "query_incidents",
    "query_resource_metrics",
    "query_http_metrics",
    "query_traces",
    "query_trace_spans",
    # rca-agent (also empty-prone — may have no analyses yet)
    "list_rca_reports",
})

# After this many consecutive empty results from any tool in the guard set,
# refuse the next query_* call and force the agent to summarize. 2 lets the
# model try one widening retry (which the prompt explicitly allows) before
# the guard bites.
_EMPTY_THRESHOLD = 2


def _looks_empty(content: object) -> bool:
    """Heuristic: does this tool result indicate no data?

    Conservative — better to let one extra call through than to gag a tool
    that genuinely returned data. The observer MCP wraps every response in
    a thin envelope (``{"queryStatus": "success", "data": {...}, "metadata":
    {...}}``), so the bytes-on-the-wire for an EMPTY response is consistently
    around 250 bytes. A real response (even a single log line) blows past
    400 bytes immediately. Combined with substring matching for the common
    JSON shapes, this catches empties without false-positiving real data.
    """
    if content is None:
        return True
    if isinstance(content, (dict, list)):
        # Normalize to canonical JSON so the double-quoted markers below
        # match — str(dict) renders single quotes, which would never hit
        # patterns like '"items":[]'.
        try:
            text = json.dumps(content, separators=(",", ":"), ensure_ascii=False).strip()
        except (TypeError, ValueError):
            text = str(content).strip()
    else:
        text = str(content).strip()
    if not text:
        return True
    lowered = text.lower()
    # Trivial no-data sentinels
    if lowered in {"[]", "{}", "null", "none", '""'}:
        return True
    # Long results almost certainly contain data — skip the substring
    # checks to avoid false positives on noisy responses that happen to
    # include the word "empty" inside a payload.
    if len(text) > 800:
        return False
    # Common JSON shapes from observer + openchoreo MCP tools
    if any(
        marker in lowered
        for marker in (
            '"items":[]',
            '"items": []',
            '"entries":[]',
            '"entries": []',
            '"results":[]',
            '"results": []',
            '"data":[]',
            '"data": []',
            '"logs":[]',
            '"logs": []',
            '"reports":[]',
            '"reports": []',
            # Numeric markers MUST be anchored with a terminator (, or })
            # so '"total":0' doesn't false-match '"total":0.5'.
            '"totalcount":0,',
            '"totalcount":0}',
            '"totalcount": 0,',
            '"totalcount": 0}',
            '"total":0,',
            '"total":0}',
            '"total": 0,',
            '"total": 0}',
        )
    ):
        return True
    # Plain-English no-data signals
    if any(
        phrase in lowered
        for phrase in (
            "no results",
            "no data",
            "no logs",
            "no alerts",
            "no incidents",
            "no metrics",
            "no traces",
            "not found",
        )
    ):
        return True
    # Emptiness is determined only by the marker / sentinel checks above.
    # A pure length-based fallback misclassifies legitimately short tool
    # outputs (e.g. a single-row query result) as empty.
    return False


class EmptyResultGuardMiddleware(AgentMiddleware):
    """Stops the agent from looping on empty observability queries.

    Observability tools (query_component_logs, query_alerts, etc.) frequently
    return empty results when the time window is wrong or the component has
    no recent activity. Without this guard, gpt-4o (and smaller) tends to
    retry with adjusted windows 3-6 times per turn, blowing the per-turn
    latency budget for no useful gain.

    Behavior:
    - The first N consecutive empty results from guarded tools are passed
      through unchanged — the agent gets to see them and either retry once
      or summarize.
    - The (N+1)th attempt to call ANY guarded tool is short-circuited with
      a ``ToolMessage`` instructing the agent to call ``ChatResponse`` next
      with what it has.
    - A single non-empty result resets the counter (a successful call means
      the agent found its data and shouldn't be penalized later).
    """

    def __init__(self, threshold: int = _EMPTY_THRESHOLD) -> None:
        super().__init__()
        # bool is a subclass of int — exclude it so True/False don't sneak in.
        if not isinstance(threshold, int) or isinstance(threshold, bool):
            raise TypeError(f"threshold must be int, got {type(threshold).__name__}")
        if threshold <= 0:
            raise ValueError(f"threshold must be > 0, got {threshold}")
        self._threshold = threshold
        self._consecutive_empty = 0

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        name = request.tool_call.get("name", "")

        # ChatResponse is the structured-output terminator — never block it.
        if name == "ChatResponse":
            return await handler(request)

        # Only guard the empty-prone observability tools; other tools pass
        # through transparently regardless of empty-result count.
        if name not in _EMPTY_GUARD_TOOLS:
            return await handler(request)

        # Refuse before the call when we've already hit the threshold —
        # avoids the round-trip to the MCP server entirely.
        if self._consecutive_empty >= self._threshold:
            logger.info(
                "EmptyResultGuard: refusing %s after %d consecutive empty results",
                name,
                self._consecutive_empty,
            )
            return ToolMessage(
                content=(
                    "EmptyResultGuard: previous observability queries returned no "
                    "data. STOP iterating. Your VERY NEXT action MUST be "
                    "ChatResponse with a short message explaining that no relevant "
                    "data was found in the time window, and suggest the user check "
                    "whether the component is running and logging is enabled. Do "
                    "NOT call any more query_* / list_rca_reports tools this turn."
                ),
                tool_call_id=request.tool_call.get("id", ""),
                name=name,
                status="error",
            )

        result = await handler(request)
        if isinstance(result, ToolMessage):
            if _looks_empty(result.content):
                self._consecutive_empty += 1
                logger.info(
                    "EmptyResultGuard: %s returned empty (%d consecutive)",
                    name,
                    self._consecutive_empty,
                )
            else:
                self._consecutive_empty = 0
        return result
