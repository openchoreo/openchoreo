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

from typing import (  # noqa: UP007 — Pydantic discriminated unions don't accept PEP-604 `X | Y` here
    Annotated,
    Any,
    Literal,
    Union,
)

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
    existing clients that ignore the field keep working unchanged.

    ``fix_prompt`` carries the model's paste-ready prompt for an
    external coding bot (CodeRabbit / Cursor / Claude Code). Populated
    only for ``build_failure`` turns where the agent identified a
    concrete code-level root cause; absent for everything else (infra
    issues, empty logs, runtime_debug, the generic FAB). Frontends use
    its presence to gate the per-message "Copy as prompt" affordance —
    they never synthesise the prompt themselves."""

    type: Literal["done"] = "done"
    message: str
    recovery: Literal["model", "stub"] | None = None
    fix_prompt: str | None = None


class CheckTextChange(_StreamEventBase):
    """A suggested change to one text field of the check."""

    field: Literal["name", "description"]
    label: str
    old: str | None = None
    new: str


class ConfirmationRequiredEvent(_StreamEventBase):
    """A check change ready to save, sent before ``done`` in check_authoring:
    an edit's Step 2 (``kind="update"``) or a drafted new check
    (``kind="create"``). The drawer shows it as a confirm card; nothing is
    saved until the user picks a button there, which sends ``token`` back to
    the confirm endpoint. ``token`` is signed, so the saved change is
    exactly the one described here.

    Edits carry ``check_id``, ``status``, ``current_rule`` and
    ``text_changes``; new checks carry ``description``, ``fact_ids``,
    ``category``, ``tier`` and ``applies_to``. ``valid`` and ``problems`` come from
    Backstage's validator; a card with problems shows them and offers no
    save buttons. A card first offers a test on a component, and the save
    buttons come with the next card: ``tested`` is set when the change was
    tested in this turn, ``test_skipped`` when the user skipped the test or
    asked to save; the card then offers only to save. ``example`` is set
    for a beginner's practice check (the welcome's example): once tested,
    its card offers what to do next instead of saving."""

    type: Literal["confirmation_required"] = "confirmation_required"
    token: str
    kind: Literal["update", "create"]
    check_id: str | None = None
    check_name: str
    status: Literal["draft", "published"] | None = None
    current_rule: str | None = None
    new_rule: str
    text_changes: list[CheckTextChange] = []
    description: str | None = None
    fact_ids: list[str] = []
    category: str | None = None
    tier: str | None = None
    # Entity kinds a new check will apply to; empty when not limited to any.
    applies_to: list[str] = []
    valid: bool
    problems: list[str] = []
    # None (left off the wire) unless tested, so other cards are unchanged.
    tested: bool | None = None
    test_skipped: bool | None = None
    example: bool | None = None


class A2UIEvent(_StreamEventBase):
    """UI for the reply, sent before ``done``: A2UI v1.0 messages
    (``createSurface`` …) built by code — see ``src/agent/a2ui.py``. The
    drawer renders them under the reply. Today: the answer buttons of a
    check edit's confirmation question; a click sends the button's
    ``userMessage`` as the next chat message."""

    type: Literal["a2ui"] = "a2ui"
    messages: list[dict[str, Any]]


class DryRunLine(_StreamEventBase):
    """A fact and its value, in plain words."""

    label: str
    value: str


class DryRunSection(_StreamEventBase):
    label: str
    lines: list[DryRunLine]


class DryRunResultEvent(_StreamEventBase):
    """A dry-run-check result, sent as soon as it comes back so the drawer
    can show it while the reply is still being written. The words are the
    ones its test card shows at the end (``src/agent/a2ui.py``): ``title``
    is the component, ``subtitle`` its kind, and ``sections`` what the rule
    needs and what the component has."""

    type: Literal["dry_run_result"] = "dry_run_result"
    status: Literal["passed", "failed", "not-applicable"]
    title: str
    subtitle: str | None = None
    summary: str
    sections: list[DryRunSection] = []


class ErrorEvent(_StreamEventBase):
    """Terminal event for a failed turn. ``message`` is human-readable;
    the request id is embedded for support-handoff."""

    type: Literal["error"] = "error"
    message: str


# Union the frontend / consumers can branch on. Discriminator is `type`
# so pydantic generates an O(1) dispatch.
StreamEvent = Annotated[
    Union[
        MessageChunkEvent,
        ToolCallEvent,
        DoneEvent,
        ConfirmationRequiredEvent,
        A2UIEvent,
        DryRunResultEvent,
        ErrorEvent,
    ],
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
