# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the agent middleware (truncation + logging summary)."""

from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

import pytest
from langchain_core.messages import ToolMessage

from src.agent.middleware import LoggingMiddleware, ToolResultTruncationMiddleware


def _tool_request(name: str = "query_resource_metrics"):
    # The middleware only reads request.tool_call.get("name").
    return SimpleNamespace(tool_call={"name": name})


@pytest.mark.asyncio
async def test_truncation_truncates_oversized_string_result():
    mw = ToolResultTruncationMiddleware()
    big = "x" * 20_000
    handler = AsyncMock(return_value=ToolMessage(content=big, tool_call_id="1"))

    with patch("src.agent.middleware.tool_result_truncation.settings.tool_result_max_chars", 8000):
        result = await mw.awrap_tool_call(_tool_request(), handler)

    assert isinstance(result, ToolMessage)
    assert len(result.content) <= 8000
    assert "truncated" in result.content


@pytest.mark.asyncio
async def test_truncation_passes_through_small_result():
    mw = ToolResultTruncationMiddleware()
    msg = ToolMessage(content="small", tool_call_id="1")
    handler = AsyncMock(return_value=msg)

    with patch("src.agent.middleware.tool_result_truncation.settings.tool_result_max_chars", 8000):
        result = await mw.awrap_tool_call(_tool_request(), handler)

    assert result.content == "small"


@pytest.mark.asyncio
async def test_truncation_ignores_non_tool_message_result():
    mw = ToolResultTruncationMiddleware()
    sentinel = object()
    handler = AsyncMock(return_value=sentinel)
    result = await mw.awrap_tool_call(_tool_request(), handler)
    assert result is sentinel


def test_logging_tool_call_summary_empty_is_none():
    mw = LoggingMiddleware()
    assert mw.tool_call_summary() is None


def test_logging_tool_call_summary_serializes_calls():
    mw = LoggingMiddleware()
    mw.tool_calls.append({"name": "get_cloud_costs", "args": {"x": 1}, "elapsed": 0.1})
    summary = mw.tool_call_summary()
    assert summary is not None
    assert "get_cloud_costs" in summary
