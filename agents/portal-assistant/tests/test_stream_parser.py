# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for src.agent.stream_parser.

The parser extracts the ``message`` field from a streamed
``{"message": "..."}`` payload. The agent is read-only — there is no
``actions`` field to extract.
"""
import pytest

from src.agent.stream_parser import (
    MAX_BUFFER_SIZE,
    ChatResponseParser,
    StreamBufferOverflowError,
)


def test_chat_response_parser_streams_message_deltas():
    p = ChatResponseParser()
    # First chunk: incomplete — no delta yet.
    p.push('{"message": "h')
    # Second chunk: completes "he".
    p.push("e")
    # Third chunk: extends to "hello", then closing brace.
    p.push("llo")
    p.push('"}')
    # The parser surfaces deltas as the message grows; we only assert
    # the cumulative message ends up correct since exact partial-parse
    # output isn't a stable contract.
    assert p.message.startswith("h")
    assert "hello" in p.message or p.message == "hello"


def test_chat_response_parser_handles_complete_payload():
    p = ChatResponseParser()
    p.push('{"message": "ok"}')
    assert p.message == "ok"


def test_chat_response_parser_handles_escapes():
    p = ChatResponseParser()
    p.push('{"message": "line1\\nline2"}')
    assert p.message == "line1\nline2"


def test_set_message_overrides_streamed():
    p = ChatResponseParser()
    p.push('{"message": "partial')
    p.set_message("final answer")
    assert p.message == "final answer"


def test_push_raises_when_buffer_exceeds_max(monkeypatch):
    # Lower the cap so the test doesn't have to allocate 1 MiB.
    monkeypatch.setattr("src.agent.stream_parser.MAX_BUFFER_SIZE", 64)
    p = ChatResponseParser()
    # First push is well under the cap and parses normally.
    p.push('{"message": "ok"}')
    assert p.message == "ok"
    # A subsequent oversized push overflows and raises.
    with pytest.raises(StreamBufferOverflowError):
        p.push("x" * 200)


def test_max_buffer_size_constant_is_exposed():
    # Pin the public constant so callers can reason about the cap.
    assert isinstance(MAX_BUFFER_SIZE, int) and MAX_BUFFER_SIZE > 0


def test_pop_delta_returns_only_new_content():
    """``pop_delta`` is the contract that enables incremental
    ``message_chunk`` emission: each call must return ONLY content
    appended since the previous call, not the cumulative prefix.
    Re-emitting the prefix every chunk would inflate the wire by N²
    and the drawer would render the full message N times concatenated."""
    p = ChatResponseParser()
    p.push('{"message": "Hello')
    first = p.pop_delta()
    assert first == "Hello"

    p.push(', world')
    second = p.pop_delta()
    assert second == ", world"

    p.push('"}')
    third = p.pop_delta()
    # Closing brace completed the JSON; no new message content arrived.
    assert third == ""


def test_pop_delta_after_set_message_returns_full_message():
    """When the structured-response recovery path replaces the parsed
    message wholesale (placeholder → final diagnosis), ``pop_delta``
    can't compute an incremental diff — it returns the FULL new
    message so the consumer can re-render. The drawer's terminal
    ``done`` event then clears the streaming buffer anyway, so the
    user sees the final message exactly once in the timeline."""
    p = ChatResponseParser()
    p.push('{"message": "placeholder"}')
    _ = p.pop_delta()
    p.set_message("final answer")
    delta = p.pop_delta()
    assert delta == "final answer"


def test_pop_delta_resets_emitted_pointer():
    """After ``pop_delta`` returns content, the same call again with
    no new pushes must return the empty string. Without this the
    orchestrator would re-emit the last chunk indefinitely."""
    p = ChatResponseParser()
    p.push('{"message": "x"}')
    assert p.pop_delta() == "x"
    assert p.pop_delta() == ""
