# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for agent wiring (``Agent.create``) and orchestration
(``run_analysis``, ``stream_chat``). The LLM, MCP, and backend are mocked."""

import json
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

import src.agent.agent as agent_module
from src.agent.agent import Agent, run_analysis, stream_chat
from src.agent.middleware import LoggingMiddleware
from src.helpers import AlertScope
from src.models import RCAReport
from tests.factories import make_rca_report, make_remediation_result

AUTH = httpx.BasicAuth("user", "pass")
SCOPE = AlertScope(
    namespace="ns",
    project="p",
    project_uid="proj-uid",
    environment="dev",
    environment_uid="env-uid",
    component="c",
    component_uid="comp-uid",
)


def _tool(name):
    return SimpleNamespace(name=name)


def _make_agent(**overrides):
    kwargs = {
        "template": "prompts/x.j2",
        "tools": {"query_traces"},
        "middleware": [LoggingMiddleware],
        "response_format": RCAReport,
        "recursion_limit": 42,
    }
    kwargs.update(overrides)
    return Agent(**kwargs)


# ----------------------------------------------------------- Agent.create


@pytest.mark.asyncio
async def test_create_filters_mcp_tools_and_appends_factories():
    captured = {}
    fake_agent = MagicMock()
    fake_agent.with_config.return_value = "CONFIGURED"

    def fake_create_agent(**kwargs):
        captured.update(kwargs)
        return fake_agent

    agent_obj = _make_agent(tool_factories=[lambda auth: _tool("list_components")])

    with (
        patch("src.agent.agent.MCPClient") as mcp_cls,
        patch("src.agent.agent.create_agent", fake_create_agent),
        patch("src.agent.agent.render", lambda *a, **k: "PROMPT"),
    ):
        mcp_cls.return_value.get_tools = AsyncMock(
            return_value=[_tool("query_traces"), _tool("query_logs")]
        )
        runnable, logging_mw = await agent_obj.create(auth=AUTH)

    names = [t.name for t in captured["tools"]]
    assert "query_traces" in names  # in the allowlist
    assert "query_logs" not in names  # filtered out
    assert "list_components" in names  # factory appended
    assert captured["system_prompt"] == "PROMPT"
    assert runnable == "CONFIGURED"
    assert isinstance(logging_mw, LoggingMiddleware)
    fake_agent.with_config.assert_called_once_with({"recursion_limit": 42})


@pytest.mark.asyncio
async def test_create_attaches_usage_callback():
    fake_agent = MagicMock()
    fake_agent.with_config.return_value = "CONFIGURED"
    cb = object()
    agent_obj = _make_agent(tools=set())

    with (
        patch("src.agent.agent.create_agent", return_value=fake_agent),
        patch("src.agent.agent.render", lambda *a, **k: "PROMPT"),
        patch("src.agent.agent.MCPClient") as mcp_cls,
    ):
        await agent_obj.create(auth=AUTH, usage_callback=cb)

    mcp_cls.assert_not_called()  # no MCP fetch when tools is empty
    fake_agent.with_config.assert_called_once_with({"recursion_limit": 42, "callbacks": [cb]})


# ------------------------------------------------------------ run_analysis


def _patched_run(rca_report, backend, *, remed_result=None, remed_enabled=False):
    fake_rca = MagicMock()
    fake_rca.ainvoke = AsyncMock(return_value={"structured_response": rca_report})
    fake_remed = MagicMock()
    fake_remed.ainvoke = AsyncMock(
        return_value={"structured_response": remed_result or make_remediation_result()}
    )
    return (
        patch.object(agent_module.RCA_AGENT, "create", AsyncMock(return_value=(fake_rca, None))),
        patch.object(
            agent_module.REMED_AGENT, "create", AsyncMock(return_value=(fake_remed, None))
        ),
        patch("src.agent.agent.get_report_backend", return_value=backend),
        patch("src.agent.agent.get_oauth2_auth", return_value=MagicMock()),
        patch("src.agent.agent.render", return_value="CONTENT"),
        patch.object(agent_module.settings, "remed_agent", remed_enabled),
    )


@pytest.mark.asyncio
async def test_run_analysis_completes_and_upserts():
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock(return_value={"result": "created"})
    patches = _patched_run(make_rca_report(), backend)

    with patches[0], patches[1], patches[2], patches[3], patches[4], patches[5]:
        await run_analysis(report_id="r1", alert_id="a1", alert={"x": 1}, scope=SCOPE)

    backend.upsert_rca_report.assert_awaited_once()
    kw = backend.upsert_rca_report.await_args.kwargs
    assert kw["status"] == "completed"
    assert kw["report_id"] == "r1"
    assert kw["project_uid"] == "proj-uid"
    assert kw["report"]["summary"]


@pytest.mark.asyncio
async def test_run_analysis_marks_failed_on_timeout():
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock()
    fake_rca = MagicMock()
    fake_rca.ainvoke = AsyncMock(side_effect=TimeoutError())

    with (
        patch.object(agent_module.RCA_AGENT, "create", AsyncMock(return_value=(fake_rca, None))),
        patch("src.agent.agent.get_report_backend", return_value=backend),
        patch("src.agent.agent.get_oauth2_auth", return_value=MagicMock()),
        patch("src.agent.agent.render", return_value="CONTENT"),
    ):
        await run_analysis(report_id="r1", alert_id="a1", alert={}, scope=SCOPE)

    kw = backend.upsert_rca_report.await_args.kwargs
    assert kw["status"] == "failed"
    assert "timed out" in kw["summary"]


@pytest.mark.asyncio
async def test_run_analysis_marks_failed_on_error():
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock()
    fake_rca = MagicMock()
    fake_rca.ainvoke = AsyncMock(side_effect=RuntimeError("boom"))

    with (
        patch.object(agent_module.RCA_AGENT, "create", AsyncMock(return_value=(fake_rca, None))),
        patch("src.agent.agent.get_report_backend", return_value=backend),
        patch("src.agent.agent.get_oauth2_auth", return_value=MagicMock()),
        patch("src.agent.agent.render", return_value="CONTENT"),
    ):
        await run_analysis(report_id="r1", alert_id="a1", alert={}, scope=SCOPE)

    kw = backend.upsert_rca_report.await_args.kwargs
    assert kw["status"] == "failed"
    assert "failed" in kw["summary"].lower()


@pytest.mark.asyncio
async def test_run_analysis_enriches_with_remediation_when_enabled():
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock(return_value={"result": "created"})
    remed = make_remediation_result()
    patches = _patched_run(make_rca_report(), backend, remed_result=remed, remed_enabled=True)

    with patches[0], patches[1], patches[2], patches[3], patches[4], patches[5]:
        await run_analysis(report_id="r1", alert_id="a1", alert={}, scope=SCOPE)

    saved = backend.upsert_rca_report.await_args.kwargs["report"]
    actions = saved["result"]["recommendations"]["recommended_actions"]
    assert actions[0]["description"] == remed.recommended_actions[0].description


# ------------------------------------------------------------- stream_chat


@pytest.mark.asyncio
async def test_stream_chat_emits_error_on_failure():
    with patch.object(
        agent_module.CHAT_AGENT, "create", AsyncMock(side_effect=RuntimeError("boom"))
    ):
        events = [
            json.loads(line)
            async for line in stream_chat(messages=[{"role": "user", "content": "hi"}], token="t")
        ]

    assert len(events) == 1
    assert events[0]["type"] == "error"


@pytest.mark.asyncio
async def test_stream_chat_streams_message_and_done():
    payload = '{"message": "Hello"}'

    class FakeChunk:
        content = payload
        content_blocks = [{"type": "text", "text": payload}]

    async def fake_astream(*args, **kwargs):
        yield (FakeChunk(), {})

    fake_agent = MagicMock()
    fake_agent.astream = fake_astream

    with patch.object(
        agent_module.CHAT_AGENT, "create", AsyncMock(return_value=(fake_agent, None))
    ):
        events = [
            json.loads(line)
            async for line in stream_chat(messages=[{"role": "user", "content": "hi"}], token="t")
        ]

    types = [e["type"] for e in events]
    assert "message_chunk" in types
    assert events[-1]["type"] == "done"
    assert events[-1]["message"] == "Hello"
