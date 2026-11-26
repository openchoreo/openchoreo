# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from pydantic import Field, model_validator

from common.models.base import BaseModel


class ChatResponse(BaseModel):
    """Return your final answer to the user. Call once when you have enough;
    do not call other tools after. Set ``message`` to the user-facing reply.

    ``fix_prompt`` is an optional SECOND, paste-ready prompt the drawer
    surfaces behind a Copy button. Recipient depends on ``case_type``:
    ``build_failure`` → coding bot on the user's repo (no kubectl/MCP);
    ``runtime_debug`` → AI agent with OpenChoreo MCP + kubectl. The
    per-case-type sections of the system prompt carry the decision rules
    and the exact shape. Leave ``None`` whenever those rules say so
    (empty logs, not-found, no actionable handoff).
    """

    message: str
    fix_prompt: str | None = None


# The card a check authoring reply is shown as (CheckAuthoringResponse.card).
# Every field has a default so a partly filled card never fails the whole
# reply: code checks the card before it is shown (src/agent/a2ui.py).


class _CardPart(BaseModel):
    """A part of the card: a field the model sent as null gets its default
    (the model often writes ``"sections": null`` for none)."""

    @model_validator(mode="before")
    @classmethod
    def _null_is_default(cls, data: Any) -> Any:
        if isinstance(data, dict):
            return {key: value for key, value in data.items() if value is not None}
        return data


class CardLine(_CardPart):
    label: str = Field(default="", description="What the line is about, in plain words.")
    value: str = Field(default="", description="Its value in plain words, e.g. '2 or more'.")


class CardSection(_CardPart):
    label: str = Field(default="", description="A short heading, e.g. 'Facts it uses'.")
    lines: list[CardLine] = Field(default_factory=list)


class CardChoice(_CardPart):
    label: str = Field(default="", description="What the button shows: a few words.")
    message: str = Field(
        default="",
        description="The full sentence sent as the user's reply when clicked.",
    )


class ReplyCard(_CardPart):
    """How the portal shows a check authoring reply that has no rule block.
    It is shown instead of ``message``, so it must say everything the user
    needs. Plain words only: no fact names, operators, backticks or Markdown,
    unless the system prompt says the user knows checks well."""

    title: str = Field(default="", description="2-6 words naming what the reply is about.")
    summary: str = Field(
        default="",
        description="One or two short sentences: the answer, or the question.",
    )
    sections: list[CardSection] = Field(default_factory=list, description="Up to 5.")
    choices: list[CardChoice] = Field(
        default_factory=list,
        description="Up to 2: the two most useful next steps or answers to click.",
    )
    ask: str | None = Field(
        default=None,
        description="When the user must type an answer: the text box's hint. Otherwise null.",
    )


class CheckAuthoringResponse(ChatResponse):
    """Return your final answer to the user. Call once when you have enough;
    do not call other tools after. Set ``message`` to the reply, and
    ``card`` as "Reply card" in the system prompt says."""

    card: ReplyCard | None = None


# The names the final answer can arrive under (see orchestrator._yield_chunks).
RESPONSE_NAMES = frozenset({"ChatResponse", "CheckAuthoringResponse"})
