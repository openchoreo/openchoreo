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
