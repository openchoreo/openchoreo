# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ``Agent.create`` and ``CloseMCPCallback``.

``create`` is exercised with the LangChain ``create_agent`` factory, the MCP
client, and template rendering all mocked at their import-site boundaries, so we
assert on *our* wiring: MCP-tool filtering by allowed names, appending tool
factories, attaching the cleanup + usage callbacks, and closing the MCP client
when agent construction fails.
"""

from contextlib import contextmanager
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import httpx
import pytest
from langchain_core.callbacks import UsageMetadataCallbackHandler

from src.agent.agent import Agent, CloseMCPCallback
from src.agent.middleware import LoggingMiddleware
from src.models import FinOpsReport

AUTH = httpx.BasicAuth("user", "pass")


def _tool(name: str):
    tool = MagicMock()
    tool.name = name
    return tool


@contextmanager
def _patched_create(*, mcp_tools, get_tools_error=None):
    """Patch the create-time boundaries; yield (mcp_instance, fake_agent, configured)."""
    mcp_instance = AsyncMock()
    if get_tools_error is not None:
        mcp_instance.get_tools = AsyncMock(side_effect=get_tools_error)
    else:
        mcp_instance.get_tools = AsyncMock(return_value=mcp_tools)
    mcp_instance.close = AsyncMock()

    fake_agent = MagicMock()
    configured = MagicMock(name="configured_runnable")
    fake_agent.with_config = MagicMock(return_value=configured)

    with (
        patch("src.agent.agent.MCPClient", return_value=mcp_instance),
        patch("src.agent.agent.create_agent", return_value=fake_agent) as create_agent_mock,
        patch("src.agent.agent.ProviderStrategy"),
        patch("src.agent.agent.render", return_value="system prompt"),
    ):
        fake_agent.create_agent_mock = create_agent_mock
        yield mcp_instance, fake_agent, configured


def _agent(**overrides):
    kwargs = {
        "template": "prompts/test.j2",
        "middleware": [],
        "response_format": FinOpsReport,
        "recursion_limit": 5,
    }
    kwargs.update(overrides)
    return Agent(**kwargs)


@pytest.mark.asyncio
async def test_create_filters_mcp_tools_and_appends_factories():
    agent = _agent(
        middleware=[LoggingMiddleware],
        tool_factories=[lambda _auth: _tool("extra_tool")],
        allowed_mcp_tools={"keep_me"},
    )
    mcp_tools = [_tool("keep_me"), _tool("drop_me")]

    with _patched_create(mcp_tools=mcp_tools) as (_mcp, fake_agent, configured):
        runnable, logging_mw = await agent.create(auth=AUTH)

    passed_tools = fake_agent.create_agent_mock.call_args.kwargs["tools"]
    assert [t.name for t in passed_tools] == ["keep_me", "extra_tool"]
    assert runnable is configured
    assert isinstance(logging_mw, LoggingMiddleware)


@pytest.mark.asyncio
async def test_create_passes_all_tools_when_no_allowlist():
    agent = _agent(allowed_mcp_tools=None)
    mcp_tools = [_tool("a"), _tool("b")]

    with _patched_create(mcp_tools=mcp_tools) as (_mcp, fake_agent, _configured):
        _, logging_mw = await agent.create(auth=AUTH)

    passed_tools = fake_agent.create_agent_mock.call_args.kwargs["tools"]
    assert [t.name for t in passed_tools] == ["a", "b"]
    assert logging_mw is None  # no LoggingMiddleware configured


@pytest.mark.asyncio
async def test_create_attaches_cleanup_and_usage_callbacks():
    agent = _agent(recursion_limit=7)
    usage_cb = UsageMetadataCallbackHandler()

    with _patched_create(mcp_tools=[]) as (_mcp, fake_agent, _configured):
        await agent.create(auth=AUTH, usage_callback=usage_cb)

    config = fake_agent.with_config.call_args.args[0]
    assert config["recursion_limit"] == 7
    callbacks = config["callbacks"]
    assert any(isinstance(c, CloseMCPCallback) for c in callbacks)
    assert usage_cb in callbacks


@pytest.mark.asyncio
async def test_create_attaches_only_cleanup_callback_without_usage():
    agent = _agent()

    with _patched_create(mcp_tools=[]) as (_mcp, fake_agent, _configured):
        await agent.create(auth=AUTH)

    callbacks = fake_agent.with_config.call_args.args[0]["callbacks"]
    assert len(callbacks) == 1
    assert isinstance(callbacks[0], CloseMCPCallback)


@pytest.mark.asyncio
async def test_create_closes_mcp_client_on_failure():
    agent = _agent()

    with (
        _patched_create(mcp_tools=[], get_tools_error=RuntimeError("boom")) as (mcp, _, _),
        pytest.raises(RuntimeError, match="boom"),
    ):
        await agent.create(auth=AUTH)

    mcp.close.assert_awaited_once()


# ----------------------------------------------------------- CloseMCPCallback


@pytest.mark.asyncio
async def test_close_mcp_callback_closes_once_across_calls():
    mcp = AsyncMock()
    cb = CloseMCPCallback(mcp)

    await cb.on_chain_end({}, run_id=uuid4())
    await cb.on_chain_end({}, run_id=uuid4())

    mcp.close.assert_awaited_once()


@pytest.mark.asyncio
async def test_close_mcp_callback_closes_on_chain_error():
    mcp = AsyncMock()
    cb = CloseMCPCallback(mcp)

    await cb.on_chain_error(RuntimeError("y"), run_id=uuid4())

    mcp.close.assert_awaited_once()


@pytest.mark.asyncio
async def test_close_mcp_callback_swallows_close_errors():
    mcp = AsyncMock()
    mcp.close = AsyncMock(side_effect=RuntimeError("cleanup failed"))
    cb = CloseMCPCallback(mcp)

    # Must not raise.
    await cb.on_chain_end({}, run_id=uuid4())
