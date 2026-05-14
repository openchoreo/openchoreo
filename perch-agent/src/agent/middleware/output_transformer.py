# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Compress observability tool responses before they hit the LLM.

The observability MCP returns large JSON blobs (50-row log batches,
trace lists with 10+ fields per trace, span trees with full attribute
maps). Feeding those verbatim into the prompt context wastes input
tokens — a 50-row ``query_component_logs`` response is ~11 k tokens of
mostly-irrelevant JSON envelope (timestamps as ISO strings, UUIDs,
pod/container names, indexer internals).

This middleware intercepts every tool response and, for the three
observability shapes that dominate runtime_debug turns, re-renders the
content as a compact markdown table via Jinja2 templates in
``src/templates/middleware/``. Benchmarked compression on realistic
payloads (cl100k_base / gpt-4o tokenizer):

    query_component_logs n=50:   10 981 →  3 937 tokens  (−64 %)
    query_traces         n=30:    2 937 →  1 585 tokens  (−46 %)
    query_trace_spans    n=25:    4 154 →  1 203 tokens  (−71 %)

The templates render the log message text verbatim — embedded
``trace_id=…`` tokens survive the round-trip so the prompt's
logs→traces pivot keeps working. The single ``test_output_transformer``
test pins this invariant.

This is the trimmed version of rca-agent's ``output_transformer.py``:
- Metric processors removed (perch's chat doesn't analyse CPU/memory
  pressure — that's the SRE Agent's job, reachable as a separate MCP).
- ``numpy`` dependency dropped (saved ~80 MB on the runtime image).
- ``TOOLS`` enum dropped (perch has no equivalent; we match on raw
  tool-name strings instead).
"""

import json
import logging
from collections.abc import Awaitable, Callable
from typing import Any

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.template_manager import render

logger = logging.getLogger(__name__)


# MCP tool names this middleware transforms. Anything else passes through
# untouched. Kept as plain strings (not an enum) because perch doesn't
# have an internal tool registry — tool names arrive verbatim from the
# upstream MCP servers.
_TOOL_LOGS = "query_component_logs"
_TOOL_TRACES = "query_traces"
_TOOL_TRACE_SPANS = "query_trace_spans"


def _unwrap_envelope(content: dict[str, Any], key: str) -> Any:
    """Locate a payload by key, allowing for one layer of application wrapping.

    Different MCP tools wrap their results inconsistently — sometimes
    the response is ``{"logs": [...]}``, sometimes ``{"data": {"logs":
    [...]}}``, sometimes ``{"result": {"logs": [...]}}``. We probe the
    common shapes in order and surface whichever contains the key.

    Returns ``None`` only when no wrapper variant produces a usable
    value, so the caller can disambiguate "tool returned nothing" from
    "tool returned a shape we don't understand". Critical: without
    this, an unknown wrapper silently degrades to ``"No logs found"``
    and the LLM loops re-querying for data it can never see — the
    exact failure mode that motivated this helper.
    """
    direct = content.get(key)
    if direct is not None:
        return direct
    for wrapper in ("data", "result", "response"):
        inner = content.get(wrapper)
        if isinstance(inner, dict):
            value = inner.get(key)
            if value is not None:
                return value
    return None


def _process_logs(content: dict[str, Any]) -> str:
    """Render a ``LogsQueryResponse`` as a per-component markdown table.

    Groups rows by ``metadata.componentName`` so the prompt's
    "name which component started it" instruction has obvious
    structure to lean on. Drops indexer-internal metadata (uids,
    container names, pod namespace) — the LLM doesn't read those.
    """
    try:
        logs = _unwrap_envelope(content, "logs") or []
        if not logs:
            return "No logs found"

        first_metadata = logs[0].get("metadata", {}) or {}

        logs_by_component: dict[str, dict[str, Any]] = {}
        for log in logs:
            log_metadata = log.get("metadata", {}) or {}
            component_name = log_metadata.get("componentName", "unknown")
            if component_name not in logs_by_component:
                logs_by_component[component_name] = {
                    "componentName": component_name,
                    "logs": [],
                }
            logs_by_component[component_name]["logs"].append(log)

        context = {
            "namespaceName": first_metadata.get("namespaceName", "N/A"),
            "projectName": first_metadata.get("projectName", "N/A"),
            "environmentName": first_metadata.get("environmentName", "N/A"),
            "components": list(logs_by_component.values()),
        }
        return render("middleware/logs.j2", context)
    except Exception as e:  # noqa: BLE001
        logger.error("Error processing logs: %s", e, exc_info=True)
        # Fall back to JSON so the LLM still sees something rather than
        # an empty tool result.
        return json.dumps(content)


def _process_traces(content: dict[str, Any]) -> str:
    """Render a trace list as a markdown table.

    The template surfaces ``traceId`` so the model can pass it back
    into a ``query_trace_spans`` follow-up. No reshaping beyond that.
    """
    try:
        traces = _unwrap_envelope(content, "traces") or []
        if not traces:
            return "No traces found"
        total = _unwrap_envelope(content, "total")
        if not isinstance(total, int):
            total = len(traces)
        context = {"traces": traces, "total": total}
        return render("middleware/traces.j2", context)
    except Exception as e:  # noqa: BLE001
        logger.error("Error processing traces: %s", e, exc_info=True)
        return json.dumps(content)


def _build_span_tree(spans: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Flatten span list into a depth-annotated DFS pre-order traversal.

    The template renders each entry as one indented line; depth becomes
    leading whitespace so the LLM can see parent/child relationships
    without us having to fabricate a nested structure.
    """
    span_map = {span["spanId"]: span for span in spans}

    # A span is a root if it has no parent OR its parent isn't in the
    # returned batch (a common case when the query truncates a trace
    # mid-tree). Treating orphans as roots preserves them in the output.
    root_spans = [
        span["spanId"]
        for span in spans
        if not span.get("parentSpanId") or span.get("parentSpanId") not in span_map
    ]

    result: list[dict[str, Any]] = []

    def walk(span_id: str, depth: int = 0) -> None:
        if span_id not in span_map:
            return
        span = span_map[span_id]
        attrs = span.get("attributes", {}) or {}
        resource_attrs = span.get("resourceAttributes", {}) or {}

        # Drop the internal ``data_stream`` indexer attribute — never
        # actionable to the LLM. Everything else passes through.
        relevant_attrs = {k: v for k, v in attrs.items() if k != "data_stream"}

        result.append(
            {
                "spanId": span["spanId"],
                "spanName": span["spanName"],
                "serviceName": resource_attrs.get("service.name", "unknown"),
                "component": resource_attrs.get("openchoreo.dev/component"),
                "project": resource_attrs.get("openchoreo.dev/project"),
                "namespace": resource_attrs.get("openchoreo.dev/namespace"),
                "durationNs": span.get("durationNs", 0),
                "startTime": span.get("startTime"),
                "depth": depth,
                "attributes": relevant_attrs,
            }
        )

        children = [s for s in spans if s.get("parentSpanId") == span_id]
        children.sort(key=lambda s: s.get("startTime", ""))
        for child in children:
            walk(child["spanId"], depth + 1)

    root_spans.sort(key=lambda sid: span_map[sid].get("startTime", ""))
    for root_id in root_spans:
        walk(root_id)
    return result


def _process_trace_spans(content: dict[str, Any]) -> str:
    try:
        spans = _unwrap_envelope(content, "spans") or []
        if not spans:
            return "No spans found"
        span_tree = _build_span_tree(spans)
        total = _unwrap_envelope(content, "total")
        if not isinstance(total, int):
            total = len(spans)
        context = {"spans": span_tree, "total": total}
        return render("middleware/trace_spans.j2", context)
    except Exception as e:  # noqa: BLE001
        logger.error("Error processing trace spans: %s", e, exc_info=True)
        return json.dumps(content)


_PROCESSORS: dict[str, Callable[[dict[str, Any]], str]] = {
    _TOOL_LOGS: _process_logs,
    _TOOL_TRACES: _process_traces,
    _TOOL_TRACE_SPANS: _process_trace_spans,
}


def _get_processor(tool_name: str | None) -> Callable[[dict[str, Any]], str] | None:
    if tool_name is None:
        return None
    return _PROCESSORS.get(tool_name)


def _extract_content(content: Any) -> dict[str, Any] | None:
    """Pull a JSON dict out of an MCP-shaped tool response.

    The langchain-mcp-adapters layer delivers tool results in one of
    three shapes depending on the upstream MCP server and adapter
    version:

      1. **dict** — the adapter parsed structured JSON for us.
      2. **str** — the raw JSON text, unparsed. This is what the
         openchoreo observer MCP actually returns today; without this
         branch the middleware silently passes through every
         observability call and the "60% token savings" the benchmark
         predicted never materialise in production. The
         ``passthrough … shape=str`` INFO log in the middleware was
         the breadcrumb that surfaced this in the cluster.
      3. **list of content blocks** — e.g. ``[{"type":"text","text":
         "{...}"}]``. The wire shape the raw MCP protocol uses; some
         adapters preserve it.

    Returns ``None`` for anything else so the middleware passes the
    tool message through untouched rather than corrupting it.
    """
    if isinstance(content, dict):
        return content
    if isinstance(content, str):
        try:
            parsed = json.loads(content)
        except (json.JSONDecodeError, TypeError):
            return None
        return parsed if isinstance(parsed, dict) else None
    if isinstance(content, list):
        for block in content:
            if not isinstance(block, dict) or block.get("type") != "text":
                continue
            try:
                parsed = json.loads(block.get("text", ""))
            except (json.JSONDecodeError, TypeError):
                continue
            if isinstance(parsed, dict):
                return parsed
    return None


def _to_mcp_content(text: str) -> list[dict[str, str]]:
    """Re-wrap a rendered string in the MCP text-block envelope.

    LangChain's ``ToolMessage`` accepts either a string or a list of
    content blocks. We use the list form so the downstream agent sees
    the same shape the MCP client would have produced — keeps any
    parallel handling (e.g. logging middleware that inspects
    ``content[0]["text"]``) working unchanged.
    """
    return [{"type": "text", "text": text}]


class OutputTransformerMiddleware(AgentMiddleware):
    """LangGraph middleware: compress observability tool responses.

    Sits BEFORE WriteGuardMiddleware in the chain so refusals from
    WriteGuard land in the model's prompt unmodified, and AFTER
    EmptyResultGuard so empty short-circuits don't pay the render
    cost. See ``src/agent/builder.py`` for the assembled order.

    On any unexpected failure, the original tool message is returned
    untouched — the LLM is more useful with raw JSON than nothing.
    """

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        result = await handler(request)

        # WriteGuard / EmptyResultGuard / ToolError variants may return
        # a Command instead of a ToolMessage — pass those through.
        if not isinstance(result, ToolMessage):
            return result

        tool_name = request.tool_call.get("name")
        processor = _get_processor(tool_name)
        if processor is None:
            # Not one of the three observability shapes — leave it alone.
            return result

        content = _extract_content(result.content)
        if content is None:
            # Visible as INFO because this is the silent-degradation
            # failure mode: the middleware was *expected* to handle
            # this tool but couldn't, so the model is about to receive
            # raw JSON it isn't tuned for. Log a short prefix of the
            # content so we can recognise unknown adapter shapes in
            # production without dumping full responses to disk.
            raw = result.content
            if isinstance(raw, (str, bytes)):
                prefix = raw[:300] if isinstance(raw, str) else raw[:300].decode("utf-8", "replace")
            else:
                # For list/dict/anything else, repr is bounded by Python and
                # gives us a glanceable shape without re-serialising the
                # whole thing.
                prefix = repr(raw)[:300]
            logger.info(
                "OutputTransformerMiddleware passthrough tool=%s reason=unparseable_content shape=%s prefix=%s",
                tool_name,
                type(result.content).__name__,
                prefix,
            )
            return result

        try:
            processed_text = processor(content)
        except Exception as e:  # noqa: BLE001
            logger.error(
                "OutputTransformerMiddleware processor for %s raised: %s",
                tool_name, e, exc_info=True,
            )
            return result

        # INFO-level outcome so we can tell from production logs whether
        # the middleware is actually compressing real responses or
        # silently degrading to "No logs found" / "No traces found"
        # (which sends the model into a retry loop — the regression we
        # are debugging right now). Cheap to keep on: one short line
        # per observability tool call. The ``empty`` flag fires when
        # the processor returned a "No X found" sentinel — useful for
        # spotting shape mismatches without scraping the rendered text.
        is_empty = processed_text in {"No logs found", "No traces found", "No spans found"}
        logger.info(
            "OutputTransformerMiddleware tool=%s rendered_len=%d empty=%s",
            tool_name,
            len(processed_text),
            is_empty,
        )
        # Detailed before/after size only at DEBUG, and guarded so the
        # ``json.dumps(content)`` re-serialisation doesn't run at INFO.
        if logger.isEnabledFor(logging.DEBUG):
            logger.debug(
                "OutputTransformerMiddleware compressed %s: %d → %d chars",
                tool_name,
                len(json.dumps(content)),
                len(processed_text),
            )

        return ToolMessage(  # type: ignore[no-matching-overload]
            content=_to_mcp_content(processed_text),
            tool_call_id=result.tool_call_id,
            name=result.name,
        )
