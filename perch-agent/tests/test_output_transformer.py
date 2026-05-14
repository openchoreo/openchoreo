# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Unit tests for OutputTransformerMiddleware.

The middleware is a token-saving optimisation, but it also has a
load-bearing UX invariant: the runtime_debug prompt instructs the LLM
to scan rendered log lines for ``trace_id=…`` tokens and pivot to
``query_trace_spans``. If a future "let's drop more fields" change
silently strips trace_ids from the rendered output, that pivot breaks
without the smoke test noticing — the agent just stops finding traces.

These tests pin:
  1. Each known tool is reshaped (logs / traces / trace_spans).
  2. Unknown tools pass through untouched.
  3. trace_id tokens embedded in log messages survive the round-trip.
  4. Empty responses produce the canned "no rows" strings.
  5. A processor exception falls back to raw JSON rather than corrupting
     the tool message — defensive promise the orchestrator relies on.
"""
import json

import pytest
from langchain.messages import ToolMessage

from src.agent.middleware.output_transformer import (
    OutputTransformerMiddleware,
    _extract_content,
    _process_logs,
    _process_traces,
    _process_trace_spans,
    _unwrap_envelope,
)


# ── Helpers ────────────────────────────────────────────────────────


def _logs_payload(rows: list[dict]) -> dict:
    return {"logs": rows, "total": len(rows)}


def _tool_message(payload: dict, tool_call_id: str = "tc-1") -> ToolMessage:
    """Mimic MCP's wire shape: a single text block carrying JSON.

    The middleware accepts either a raw dict or this list-of-blocks
    form; we use the latter because it's the shape langchain-mcp
    actually delivers.
    """
    return ToolMessage(
        content=[{"type": "text", "text": json.dumps(payload)}],
        tool_call_id=tool_call_id,
        name="dummy",
    )


# ── Processor-level tests ──────────────────────────────────────────


def test_process_logs_preserves_trace_ids_verbatim():
    """If trace_ids stop surviving the render, the logs→traces pivot
    in the runtime_debug prompt breaks. This is the load-bearing
    invariant — guard it explicitly."""
    rows = [
        {
            "timestamp": "2026-05-14T10:42:11Z",
            "level": "ERROR",
            "log": "boom trace_id=0123456789abcdef0123456789abcdef status=500",
            "metadata": {"componentName": "order-api", "namespaceName": "default"},
        },
        {
            "timestamp": "2026-05-14T10:42:13Z",
            "level": "ERROR",
            "log": "downstream failure traceparent=00-deadbeefcafebabefeedfacedeadbeef-0123456789abcdef-01",
            "metadata": {"componentName": "billing-svc"},
        },
    ]
    out = _process_logs(_logs_payload(rows))
    assert "0123456789abcdef0123456789abcdef" in out
    assert "deadbeefcafebabefeedfacedeadbeef" in out


def test_process_logs_groups_by_component():
    """The prompt's "name which component started the failure" only
    works if components are surfaced in section headers — the LLM
    leans on the structure, not on its own grouping. Pin that."""
    rows = [
        {
            "log": "a", "level": "ERROR",
            "metadata": {"componentName": "alpha", "namespaceName": "ns"},
        },
        {
            "log": "b", "level": "WARN",
            "metadata": {"componentName": "beta", "namespaceName": "ns"},
        },
    ]
    out = _process_logs(_logs_payload(rows))
    assert "**Component:** alpha" in out
    assert "**Component:** beta" in out


def test_process_logs_empty_returns_canonical_string():
    assert _process_logs({"logs": []}) == "No logs found"


def test_process_logs_unwraps_data_envelope():
    """The regression we just deployed: when the observer MCP returns
    ``{"data": {"logs": [...]}}`` instead of the bare LogsQueryResponse
    shape we expected, my old code looked at the top level, saw no
    ``logs`` key, and returned "No logs found" — sending the model
    into a loop. _unwrap_envelope was added precisely for this; pin it
    so a future "let's simplify the extractor" change can't silently
    reintroduce the loop."""
    wrapped = {
        "data": {
            "logs": [
                {
                    "timestamp": "2026-05-14T10:42:11Z",
                    "level": "ERROR",
                    "log": "boom trace_id=cafebabe in flight",
                    "metadata": {"componentName": "demo-svc", "namespaceName": "default"},
                },
            ],
        }
    }
    out = _process_logs(wrapped)
    assert "No logs found" not in out
    assert "cafebabe" in out
    assert "demo-svc" in out


def test_process_logs_unwraps_result_envelope():
    """Same as above but with the other common wrapper key — some MCP
    SDKs use ``result`` instead of ``data``."""
    wrapped = {"result": {"logs": [{"log": "x", "metadata": {"componentName": "c"}}]}}
    out = _process_logs(wrapped)
    assert "No logs found" not in out
    assert "**Component:** c" in out


def test_unwrap_envelope_prefers_direct_key():
    """If the key is at the top level, never look at wrappers — protects
    against pathological responses like ``{"logs": [...], "data": {"logs": [other]}}``
    where one is real data and the other is debug context."""
    real = [{"log": "real"}]
    other = [{"log": "shadow"}]
    out = _unwrap_envelope({"logs": real, "data": {"logs": other}}, "logs")
    assert out is real


def test_unwrap_envelope_returns_none_when_missing_everywhere():
    """Differentiates 'tool returned empty list' from 'tool returned a
    shape we don't understand'. The middleware uses ``or []`` after,
    so the visible behaviour is identical at the processor level, but
    a future caller that wants to log shape misses (we do, in the
    middleware) needs the None signal."""
    assert _unwrap_envelope({"unrelated": 1}, "logs") is None
    assert _unwrap_envelope({"data": "not-a-dict"}, "logs") is None


def test_process_traces_renders_trace_ids_in_table():
    """The model uses the rendered traceId column to issue a follow-up
    ``query_trace_spans`` call — strip it from the template and the
    follow-up never fires."""
    payload = {
        "traces": [
            {
                "traceId": "abc123def456",
                "traceName": "POST /api/orders",
                "spanCount": 7,
                "durationNs": 51_300_000,
                "startTime": "2026-05-14T10:42:10Z",
            },
        ],
        "total": 1,
    }
    out = _process_traces(payload)
    assert "abc123def456" in out
    assert "POST /api/orders" in out


def test_process_trace_spans_renders_tree_with_depth():
    """Indentation encodes parent/child to the LLM — without it the
    span tree degrades to a flat list and call-graph reasoning falls
    apart. Pin that the second-level span is indented."""
    payload = {
        "spans": [
            {
                "spanId": "root", "parentSpanId": None,
                "spanName": "POST /api/orders",
                "durationNs": 50_000_000, "startTime": "2026-05-14T10:42:10Z",
                "attributes": {"http.method": "POST", "data_stream": "internal"},
                "resourceAttributes": {"service.name": "order-api"},
            },
            {
                "spanId": "child", "parentSpanId": "root",
                "spanName": "db.query",
                "durationNs": 20_000_000, "startTime": "2026-05-14T10:42:10.5Z",
                "attributes": {},
                "resourceAttributes": {"service.name": "order-api"},
            },
        ],
        "total": 2,
    }
    out = _process_trace_spans(payload)
    # Root has no leading indent; child has 2 spaces (depth=1, "  " * 1).
    assert "POST /api/orders" in out
    assert "  db.query" in out
    # The internal ``data_stream`` attribute should NOT survive — it's
    # not actionable for the LLM and would just inflate tokens.
    assert "data_stream" not in out


def test_process_logs_falls_back_to_json_on_template_failure(monkeypatch):
    """If the template ever raises, the processor must return SOMETHING
    the LLM can read — raw JSON dump is the documented fallback. Without
    this we'd silently return the empty string and the chat would
    surface "I have no data" mid-investigation."""
    from src.agent.middleware import output_transformer

    def boom(*_args, **_kwargs):
        raise RuntimeError("template exploded")

    monkeypatch.setattr(output_transformer, "render", boom)
    out = _process_logs({"logs": [{"log": "x", "metadata": {"componentName": "c"}}]})
    parsed = json.loads(out)
    assert parsed["logs"][0]["log"] == "x"


# ── _extract_content tests ─────────────────────────────────────────


def test_extract_content_accepts_raw_dict():
    assert _extract_content({"logs": []}) == {"logs": []}


def test_extract_content_unpacks_mcp_text_blocks():
    payload = {"logs": [{"log": "x"}], "total": 1}
    blocks = [{"type": "text", "text": json.dumps(payload)}]
    assert _extract_content(blocks) == payload


def test_extract_content_parses_raw_string():
    """langchain-mcp-adapters in production delivers ``result.content``
    as a plain JSON string for openchoreo's observer MCP — not a dict,
    not the list-of-blocks shape. Discovered via INFO instrumentation
    showing ``passthrough … shape=str`` for every tool call. Without
    this branch the middleware silently no-ops on all observability
    calls in the cluster, and the benchmark's 60% token savings never
    materialise."""
    payload = {"logs": [{"log": "real-prod-shape", "metadata": {}}], "total": 1}
    out = _extract_content(json.dumps(payload))
    assert out == payload


def test_extract_content_returns_none_for_unparseable_string():
    """A non-JSON string (e.g. a plain error message from a failing
    tool) must NOT crash or corrupt the response — pass through."""
    assert _extract_content("oops, tool exploded") is None


def test_extract_content_returns_none_for_non_object_string():
    """If the JSON parses but isn't an object (e.g. ``"some text"`` or
    a number), the processors can't extract logs/traces from it.
    Returning None lets the middleware pass through cleanly."""
    assert _extract_content('"just a string"') is None
    assert _extract_content("42") is None


def test_extract_content_returns_none_for_unparseable_shape():
    # Non-JSON text inside a content block — middleware must pass through
    # rather than corrupt the tool response.
    blocks = [{"type": "text", "text": "this is not json"}]
    assert _extract_content(blocks) is None


# ── End-to-end middleware tests ────────────────────────────────────


@pytest.mark.asyncio
async def test_middleware_compresses_known_tool():
    """The handler returns a raw MCP-shaped ToolMessage; the middleware
    must replace its content with a rendered markdown table."""
    mw = OutputTransformerMiddleware()
    payload = _logs_payload(
        [
            {
                "log": "ERROR with trace_id=cafebabe in flight",
                "level": "ERROR",
                "timestamp": "2026-05-14T10:42:11Z",
                "metadata": {"componentName": "demo-svc", "namespaceName": "default"},
            },
        ]
    )

    async def handler(_request):
        return _tool_message(payload)

    request = type("ReqStub", (), {"tool_call": {"name": "query_component_logs"}})()
    result = await mw.awrap_tool_call(request, handler)
    assert isinstance(result, ToolMessage)
    rendered = result.content[0]["text"]
    assert "Timestamp | Log Level | Message" in rendered
    assert "cafebabe" in rendered, "trace_id must survive the middleware"


@pytest.mark.asyncio
async def test_middleware_passes_through_unknown_tool():
    """Anything not in the {logs, traces, trace_spans} set must be
    forwarded untouched — the middleware is opt-in by tool name, not
    a global JSON-to-markdown filter."""
    mw = OutputTransformerMiddleware()
    payload = {"components": [{"name": "x"}]}
    original = _tool_message(payload)

    async def handler(_request):
        return original

    request = type("ReqStub", (), {"tool_call": {"name": "list_components"}})()
    result = await mw.awrap_tool_call(request, handler)
    # ToolMessage content unchanged.
    assert result is original


@pytest.mark.asyncio
async def test_middleware_passes_through_non_tool_message_result():
    """If a prior middleware (WriteGuard / EmptyResultGuard) replaced
    the tool result with a Command, this middleware must not try to
    reshape it — Commands have no ``content`` field and the cast would
    explode."""
    mw = OutputTransformerMiddleware()

    sentinel = object()

    async def handler(_request):
        return sentinel  # type: ignore[return-value]

    request = type("ReqStub", (), {"tool_call": {"name": "query_component_logs"}})()
    result = await mw.awrap_tool_call(request, handler)
    assert result is sentinel
