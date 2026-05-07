# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the typed StreamEvent wire-format.

The discriminated union is the contract between the agent and the
frontend. These tests pin field names and types — a typo in agent.py's
emit() call (e.g. ``messsage`` instead of ``message``) raises locally
instead of corrupting the ndjson stream.
"""
import json

import pytest
from pydantic import ValidationError

from src.agent.stream_events import (
    ActionsEvent,
    DoneEvent,
    ErrorEvent,
    MessageChunkEvent,
    emit,
)


def test_message_chunk_serialises_with_correct_keys():
    line = emit(MessageChunkEvent(content="hello"))
    payload = json.loads(line.rstrip("\n"))
    assert payload == {"type": "message_chunk", "content": "hello"}


def test_actions_event_round_trips():
    line = emit(ActionsEvent(actions=[{"action_id": "a1", "tool": "create_x"}]))
    payload = json.loads(line.rstrip("\n"))
    assert payload["type"] == "actions"
    assert payload["actions"][0]["tool"] == "create_x"


def test_done_event_includes_final_message():
    line = emit(DoneEvent(message="all set"))
    payload = json.loads(line.rstrip("\n"))
    assert payload == {"type": "done", "message": "all set"}


def test_error_event_includes_message():
    line = emit(ErrorEvent(message="boom"))
    payload = json.loads(line.rstrip("\n"))
    assert payload == {"type": "error", "message": "boom"}


def test_dict_path_validates_against_union():
    # Existing call sites in agent.py still pass dicts; they should be
    # validated against the union and serialised the same way.
    line = emit({"type": "message_chunk", "content": "hi"})
    assert json.loads(line.rstrip("\n")) == {
        "type": "message_chunk",
        "content": "hi",
    }


def test_unknown_type_is_rejected():
    with pytest.raises(ValidationError):
        emit({"type": "totally_made_up", "message": "x"})


def test_extra_field_is_rejected():
    # ``extra="forbid"`` on the base model — protects against typos.
    with pytest.raises(ValidationError):
        emit({"type": "message_chunk", "content": "hi", "messsage": "typo"})


def test_missing_required_field_is_rejected():
    with pytest.raises(ValidationError):
        emit({"type": "message_chunk"})  # no `content`


def test_emit_terminates_with_newline():
    # ndjson framing.
    line = emit(MessageChunkEvent(content="x"))
    assert line.endswith("\n")
