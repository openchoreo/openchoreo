# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the incremental JSON stream parser."""

from src.agent.stream_parser import ChatResponseParser, parse_partial_json


def test_parse_partial_json_returns_dict_for_complete_object():
    assert parse_partial_json('{"message": "hi"}') == {"message": "hi"}


def test_parse_partial_json_completes_trailing_string():
    parsed = parse_partial_json('{"message": "hel')
    assert parsed == {"message": "hel"}


def test_parse_partial_json_returns_none_for_non_object():
    assert parse_partial_json("[1, 2, 3]") is None


def test_parse_partial_json_returns_none_for_garbage():
    assert parse_partial_json("not json at all %%%") is None


def test_parse_partial_json_strips_dangling_unicode_escape():
    # A chunk cut mid-escape ("\\u00") must not blow up the parser.
    assert parse_partial_json('{"message": "x\\u00') == {"message": "x"}


def test_push_streams_message_deltas():
    p = ChatResponseParser()
    assert p.push('{"message": "') is None
    assert p.push("hel") == "hel"
    assert p.push('lo"}') == "lo"
    assert p.message == "hello"


def test_push_returns_none_when_message_does_not_grow():
    p = ChatResponseParser()
    p.push('{"message": "done"}')
    assert p.push(" ") is None


def test_push_captures_actions():
    p = ChatResponseParser()
    p.push('{"message": "ok", "actions": [{"id": 1}]}')
    assert p.actions == [{"id": 1}]


def test_push_returns_none_for_unparseable_buffer():
    p = ChatResponseParser()
    assert p.push("@@@") is None
    assert p.message == ""
