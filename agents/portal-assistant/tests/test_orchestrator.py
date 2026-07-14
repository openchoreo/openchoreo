# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for src.agent.orchestrator helpers.

``_stamp_current_time`` hands the model a trustworthy "now" by prefixing the
latest *user* turn with the server clock. These tests pin the contract: it
finds the latest user turn wherever it sits (not just the final message),
leaves later turns untouched, and never mutates the caller's input.
"""
import copy
from datetime import UTC, datetime

import pytest

from src.agent import orchestrator
from src.agent.orchestrator import _stamp_current_time

_FIXED = datetime(2026, 7, 14, 12, 0, 0, tzinfo=UTC)
_PREFIX = "[current server time: 2026-07-14T12:00:00+00:00]\n"


@pytest.fixture(autouse=True)
def _freeze_clock(monkeypatch):
    """Pin the server clock so the stamped prefix is deterministic."""
    monkeypatch.setattr(orchestrator, "get_current_utc", lambda: _FIXED)


def test_empty_input_returns_unchanged():
    assert _stamp_current_time([]) == []


def test_stamps_trailing_user_turn():
    out = _stamp_current_time([{"role": "user", "content": "is it fixed?"}])
    assert out == [{"role": "user", "content": f"{_PREFIX}is it fixed?"}]


def test_stamps_latest_user_turn_when_assistant_trails():
    # Latest user turn is "second" (index 1); the trailing assistant turn must
    # NOT suppress the stamp, and both other turns stay byte-for-byte identical.
    msgs = [
        {"role": "user", "content": "first"},
        {"role": "user", "content": "second"},
        {"role": "assistant", "content": "answer"},
    ]
    assert _stamp_current_time(msgs) == [
        {"role": "user", "content": "first"},
        {"role": "user", "content": f"{_PREFIX}second"},
        {"role": "assistant", "content": "answer"},
    ]


def test_no_user_turn_returns_unchanged():
    msgs = [{"role": "assistant", "content": "hi"}]
    assert _stamp_current_time(msgs) == msgs


def test_does_not_mutate_input():
    # Trailing assistant turn ensures the stamped user turn is not the last
    # element, exercising the copy-not-mutate path on an interior message.
    msgs = [
        {"role": "user", "content": "second"},
        {"role": "assistant", "content": "answer"},
    ]
    before = copy.deepcopy(msgs)
    _stamp_current_time(msgs)
    assert msgs == before


async def test_recovery_fallback_receives_stamped_user_turn(monkeypatch):
    """On a recursion-limit failure the fallback must get the SAME stamped
    "now" anchor as the primary path, not the raw unstamped messages."""
    captured: dict[str, object] = {}

    async def _fake_build_agent(token, scope, *, user_sub=""):
        return object(), [], {}

    async def _boom(agent, messages, parser):
        raise RuntimeError("Recursion limit of 50 reached")
        yield  # unreachable; makes this an async generator

    class _Recovery:
        text = "recovered answer"
        source = "model"  # DoneEvent.recovery is Literal["model", "stub"]

    async def _fake_recover(*, messages, scope):
        captured["messages"] = messages
        return _Recovery()

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _boom)
    monkeypatch.setattr(orchestrator, "recover_with_fallback", _fake_recover)

    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": "is it fixed?"}],
            token="t",
            user_sub="u",
            scope=None,
        )
    ]

    # The fallback received the stamped latest user turn — same anchor the
    # primary streaming path would have used.
    assert captured["messages"][-1]["content"] == f"{_PREFIX}is it fixed?"
    # And the recovered answer still reached the client.
    assert any("recovered answer" in ev for ev in events)
