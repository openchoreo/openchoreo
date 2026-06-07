# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Typed wire-format events for the /chat ndjson stream.

The chat endpoint streams one JSON object per line. Historically these
events were assembled inline as anonymous dicts (``emit({"type": ...})``)
in agent.py — the exact shape lived in code comments, the frontend
consumed it from memory, and shape drift between the two had to be
caught by manual testing.

This module makes the wire format explicit:

  - Each event is a Pydantic ``BaseModel`` with a literal ``type`` field
    that discriminates the union. Adding a new event = adding a class +
    extending the union.
  - ``emit(event)`` serialises to a newline-terminated JSON line — the
    one place that decides exactly what bytes leave the server.
  - The frontend's TypeScript ``StreamEvent`` type can be regenerated
    from these models (or hand-mirrored) with confidence that fields and
    field-names match.

Backward compatibility: the JSON shape is unchanged. Existing frontends
that already understand ``{"type": "message_chunk", ...}`` etc. continue
to work — this is a server-side typing exercise, not a wire change.
"""

from __future__ import annotations

from typing import Annotated, Any, Literal, Union  # noqa: UP007 — Pydantic discriminated unions don't accept PEP-604 `X | Y` here

from pydantic import BaseModel, Field


class _StreamEventBase(BaseModel):
    """Forbid extra keys so a typo (`messsage`, `actoins`) fails fast in
    tests instead of silently shipping a confusing event to the UI."""

    model_config = {"extra": "forbid"}


class MessageChunkEvent(_StreamEventBase):
    """A delta of the assistant's user-facing message. Concatenate in
    order to reconstruct the full message (also surfaced on the final
    DoneEvent)."""

    type: Literal["message_chunk"] = "message_chunk"
    content: str


class ToolCallEvent(_StreamEventBase):
    """The agent is starting a tool call. Surfaced to the UI so it can
    show "Looking at builds…" / "Reading workflow run…" indicators while
    the tool is in flight. ``args`` is the partial JSON the model has
    streamed so far — it may still be truncated when this event fires."""

    type: Literal["tool_call"] = "tool_call"
    tool: str
    activeForm: str
    args: Any = ""


class DoneEvent(_StreamEventBase):
    """Terminal event for a successful turn. ``message`` is the full
    user-facing message (not a delta).

    ``recovery`` is set when this turn went through the recursion-limit
    fallback path: ``"model"`` means the tool-less fallback LLM
    answered, ``"stub"`` means even the fallback failed and the user is
    seeing a canned apology. Absent on normal (non-recovery) turns so
    existing clients that ignore the field keep working unchanged."""

    type: Literal["done"] = "done"
    message: str
    recovery: Literal["model", "stub"] | None = None


class ErrorEvent(_StreamEventBase):
    """Terminal event for a failed turn. ``message`` is human-readable;
    the request id is embedded for support-handoff."""

    type: Literal["error"] = "error"
    message: str


# Union the frontend / consumers can branch on. Discriminator is `type`
# so pydantic generates an O(1) dispatch.
StreamEvent = Annotated[
    Union[MessageChunkEvent, ToolCallEvent, DoneEvent, ErrorEvent],
    Field(discriminator="type"),
]


def emit(event: StreamEvent | dict[str, Any]) -> str:
    """Serialise an event to a newline-terminated JSON line.

    Accepts either a typed event (preferred) or a raw dict (transitional
    — call sites still using ``emit({"type": "..."})`` keep working
    while they're migrated). The dict path runs through the same
    discriminated-union validator so a malformed event raises locally
    rather than corrupting the stream.
    """
    if isinstance(event, dict):
        # Validate the dict against the union; on success we can serialise
        # the validated model so unknown fields don't slip through.
        from pydantic import TypeAdapter

        validated = TypeAdapter(StreamEvent).validate_python(event)
        # ``exclude_none=True`` keeps optional fields (e.g. DoneEvent.recovery)
        # out of the wire format when unset, so non-recovery turns serialise
        # to byte-identical JSON as before this field was added.
        return validated.model_dump_json(exclude_none=True) + "\n"
    return event.model_dump_json(exclude_none=True) + "\n"
