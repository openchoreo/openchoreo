# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the agent middleware: logging, tool-error handling, output transform."""

import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from langchain_core.messages import ToolMessage

from src.agent.middleware import (
    LoggingMiddleware,
    OutputTransformerMiddleware,
    ToolErrorHandlerMiddleware,
)
from src.agent.middleware import output_transformer as ot
from src.agent.tool_registry import TOOLS


def _request(name="query_resource_metrics", call_id="call-1"):
    return SimpleNamespace(tool_call={"name": name, "args": {}, "id": call_id})


# ------------------------------------------------------------- logging


def test_logging_summary_empty_is_none():
    assert LoggingMiddleware().tool_call_summary() is None


def test_logging_summary_serializes_calls():
    mw = LoggingMiddleware()
    mw.tool_calls.append({"name": "query_traces", "args": {}, "elapsed": 0.1})
    summary = mw.tool_call_summary()
    assert summary is not None and "query_traces" in summary


@pytest.mark.asyncio
async def test_logging_records_and_passes_through():
    mw = LoggingMiddleware()
    msg = ToolMessage(content="ok", tool_call_id="call-1")
    handler = AsyncMock(return_value=msg)
    result = await mw.awrap_tool_call(_request(), handler)
    assert result is msg
    assert mw.tool_call_count == 1
    assert mw.tool_calls[0]["name"] == "query_resource_metrics"


# --------------------------------------------------- tool error handler


@pytest.mark.asyncio
async def test_error_handler_returns_error_tool_message():
    mw = ToolErrorHandlerMiddleware()
    handler = AsyncMock(side_effect=RuntimeError("boom"))
    result = await mw.awrap_tool_call(_request(call_id="abc"), handler)
    assert isinstance(result, ToolMessage)
    assert result.status == "error"
    assert "boom" in result.content
    assert result.tool_call_id == "abc"
    assert result.name == "query_resource_metrics"


@pytest.mark.asyncio
async def test_error_handler_passes_through_success():
    mw = ToolErrorHandlerMiddleware()
    msg = ToolMessage(content="ok", tool_call_id="call-1", status="success")
    handler = AsyncMock(return_value=msg)
    assert await mw.awrap_tool_call(_request(), handler) is msg


# ---------------------------------------------------- output transformer


def test_extract_content_accepts_dict():
    assert ot._extract_content({"a": 1}) == {"a": 1}


def test_extract_content_unpacks_mcp_text_block():
    blocks = [{"type": "text", "text": json.dumps({"logs": []})}]
    assert ot._extract_content(blocks) == {"logs": []}


def test_extract_content_returns_none_for_unparseable_block():
    assert ot._extract_content([{"type": "text", "text": "not json"}]) is None


def test_extract_content_returns_none_for_scalar():
    assert ot._extract_content("plain string") is None


def test_process_logs_empty_returns_sentinel():
    assert ot._process_logs({"logs": []}) == "No logs found"


def test_process_metrics_empty_returns_sentinel():
    assert ot._process_metrics({}) == "No metrics data available"


def test_process_traces_empty_returns_sentinel():
    assert ot._process_traces({"traces": []}) == "No traces found"


def test_process_trace_spans_empty_returns_sentinel():
    assert ot._process_trace_spans({"spans": []}) == "No spans found"


def test_get_processor_unknown_tool_falls_back_to_json():
    processor = ot.get_processor("not-a-known-tool")
    assert processor({"x": 1}) == json.dumps({"x": 1})


def test_get_processor_routes_known_tools():
    assert ot.get_processor(TOOLS.QUERY_COMPONENT_LOGS) is ot._process_logs
    assert ot.get_processor(TOOLS.QUERY_RESOURCE_METRICS) is ot._process_metrics


@pytest.mark.asyncio
async def test_transformer_passes_through_non_tool_message():
    mw = OutputTransformerMiddleware()
    sentinel = object()
    handler = AsyncMock(return_value=sentinel)
    assert await mw.awrap_tool_call(_request(), handler) is sentinel


@pytest.mark.asyncio
async def test_transformer_passes_through_unextractable_content():
    mw = OutputTransformerMiddleware()
    msg = ToolMessage(content="plain string", tool_call_id="call-1")
    handler = AsyncMock(return_value=msg)
    assert await mw.awrap_tool_call(_request(), handler) is msg


@pytest.mark.asyncio
async def test_transformer_json_encodes_unknown_tool_output():
    mw = OutputTransformerMiddleware()
    block = [{"type": "text", "text": json.dumps({"hello": "world"})}]
    msg = ToolMessage(content=block, tool_call_id="call-1", name="some_tool")
    handler = AsyncMock(return_value=msg)
    result = await mw.awrap_tool_call(_request(name="some_tool"), handler)
    assert result.content == [{"type": "text", "text": json.dumps({"hello": "world"})}]
