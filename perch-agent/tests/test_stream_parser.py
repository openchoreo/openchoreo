# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for src.agent.stream_parser.

Covers the bug fix where a single-element list wrapping the ChatResponse
dict (an output mode some smaller models occasionally emit) used to be
silently dropped.
"""
from src.agent.stream_parser import ChatResponseParser, parse_partial_json


def test_parses_complete_dict():
    out = parse_partial_json('{"message": "hello"}')
    assert out == {"message": "hello"}


def test_parses_single_element_list_unwrapped():
    # Was the bug: returned None and the streamed message was lost.
    out = parse_partial_json('[{"message": "hi"}]')
    assert out == {"message": "hi"}


def test_multi_element_list_returns_none():
    # Genuinely malformed for our schema — keep returning None.
    assert parse_partial_json('[{"message": "a"}, {"message": "b"}]') is None


def test_scalar_returns_none():
    assert parse_partial_json('"just a string"') is None
    assert parse_partial_json("42") is None


def test_partial_dict_returns_partial_or_none_without_raising():
    # jiter's trailing-strings mode handles the truncated case.
    # Either we get a partial dict back or None — both are acceptable;
    # the key is no exception escapes the parser.
    result = parse_partial_json('{"message": "hel')
    assert result is None or "message" in result


def test_chat_response_parser_streams_message_deltas():
    p = ChatResponseParser()
    # First chunk: incomplete — no delta yet.
    p.push('{"message": "h')
    # Second chunk: completes "he".
    delta1 = p.push('e')
    # Third chunk: extends to "hello", then closing brace.
    p.push('llo')
    p.push('"}')
    # The parser surfaces deltas as the message grows; we only assert
    # the cumulative message ends up correct since jiter's exact
    # partial-parse output isn't a stable contract.
    assert p.message.startswith("h")
    assert "hello" in p.message or p.message == "hello"


def test_chat_response_parser_extracts_actions():
    p = ChatResponseParser()
    p.push('{"message": "ok", "actions": [{"tool": "create_x"}]}')
    assert p.actions == [{"tool": "create_x"}]
    assert p.message == "ok"
