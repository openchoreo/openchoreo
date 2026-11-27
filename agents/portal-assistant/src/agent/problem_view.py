# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A check tool that failed at the end of a turn, read for an A2UI notice card.

Built from the tool's own error, never from the model's words. Known errors
get plain words; any other keeps the tool's text, scrubbed here of tokens
and internal addresses: not every error comes through
ToolErrorHandlerMiddleware (a test run by code, or a tool's error result).
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from src.agent.dry_run_view import entity_name
from src.agent.middleware.tool_error_handler import _scrub
from src.agent.reply_view import NoticeView
from src.clients.backstage_tools import (
    DRY_RUN_CHECK,
    GET_CHECK,
    GET_ENTITY_FACTS,
    LIST_CHECKS,
    LIST_FACT_SCHEMAS,
    VALIDATE_CHECK,
)

_TEXT_MAX = 300

# What the user was waiting for, by tool.
_TITLES = {
    DRY_RUN_CHECK: "Couldn't test the check",
    VALIDATE_CHECK: "Couldn't check the rule",
    GET_CHECK: "Couldn't load the check",
    LIST_CHECKS: "Couldn't list the checks",
    GET_ENTITY_FACTS: "Couldn't read the component's facts",
    LIST_FACT_SCHEMAS: "Couldn't list the facts",
}

_NO_ENTITY_RE = re.compile(r"No entity (\S+?) in the catalog", re.IGNORECASE)
_NO_PERMISSION_RE = re.compile(
    r"permission|not allowed|forbidden|unauthori[sz]ed|signed-in user|\b40[13]\b",
    re.IGNORECASE,
)
_UNAVAILABLE_RE = re.compile(
    r"not ready|isn't available|unavailable|timed? ?out|ECONNREFUSED|\b50[234]\b",
    re.IGNORECASE,
)
_NOT_EVALUATED_RE = re.compile(r"could not be evaluated:\s*(.+)", re.IGNORECASE | re.DOTALL)


@dataclass
class ToolError:
    tool: str
    text: str


def _clean(text: str) -> str:
    text = re.sub(r"^\s*Error(?: executing tool \S+)?:\s*", "", _scrub(text or ""))
    text = " ".join(text.split())
    return text if len(text) <= _TEXT_MAX else text[: _TEXT_MAX - 1] + "…"


def problem_view(error: ToolError) -> NoticeView:
    """The notice card for a tool that failed, in plain words."""
    text = _clean(error.text)
    no_entity = _NO_ENTITY_RE.search(text)
    not_evaluated = _NOT_EVALUATED_RE.search(text)
    if no_entity:
        name, _ = entity_name(no_entity[1])
        summary = f"{name} isn't in the catalog. Check the component's name."
    elif _NO_PERMISSION_RE.search(text):
        summary = "You don't have permission to do this. Ask an admin for access to manage checks."
    elif _UNAVAILABLE_RE.search(text):
        summary = "The check service isn't available right now. Try again in a moment."
    elif not_evaluated:
        summary = f"The rule couldn't be run: {not_evaluated[1].rstrip('.')}."
    else:
        summary = text or "The tool gave no answer."
    return NoticeView(
        tone="warn",
        title=_TITLES.get(error.tool, "Something went wrong"),
        summary=summary,
        note="Or say what to do next in the chat.",
    )


def failed_test_note(error: ToolError) -> str | None:
    """The save card's note when the test of a draft failed, in the notice's
    words ("Couldn't test the check: … isn't in the catalog."). None for the
    failure of any other tool."""
    if error.tool != DRY_RUN_CHECK:
        return None
    view = problem_view(error)
    return f"{view.title}: {view.summary}"
