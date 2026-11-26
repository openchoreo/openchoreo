# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Check authoring replies that draft no rule, read for an A2UI notice card.

The prompt gives these replies fixed wording: "No available fact measures
…", "No fact retriever collects … for …" and "Did you mean it must be …".
Code recognises that wording and keeps only facts that exist in the
forwarded facts; a reply in other words gives None and is shown as text.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

from src.agent.rule_view import fact_labels, humanize_fact
from src.agent.words import first_upper

_MAX_PICKS = 4

_NO_FACT_RE = re.compile(
    r"No available fact measures (.+?),? so I can(?:'|’)?t build this check",
    re.IGNORECASE,
)
# `fact` (what it measures); the model sometimes leaves the backticks off, and
# any other word before a bracket is dropped later, as it isn't a known fact.
_FACT_WITH_WORDS_RE = re.compile(r"`?(?<![\w.])([A-Za-z][\w.]*)`?\s*\(([^()\n]+)\)")
_OTHER_OPTIONS_RE = re.compile(r"^[ \t]*\**Other options\**[ \t]*:\**[ \t]*(.+)$", re.I | re.M)
_FOR_EXAMPLE_RE = re.compile(r"\(for example:?\s*([^()\n]+)\)", re.IGNORECASE)
_CODE_RE = re.compile(r"`([^`\n]+)`")

_NO_KIND_RE = re.compile(
    r"No fact retriever collects `([^`\n]+)` for `?([^`\n,]+?)`?,? so I can(?:'|’)?t build "
    r"this check\.?\s*It can be checked on `?([^`\n.]+?)`?\.",
    re.IGNORECASE,
)

_WHICH_RE = re.compile(r"Did you mean it (must(?: not)? be `[^`\n]+`.*?)\?", re.IGNORECASE)
_MUST_RE = re.compile(r"(must(?: not)? be) `([^`\n]+)`", re.IGNORECASE)
_CAN_BE_RE = re.compile(r"`([A-Za-z][\w.]*)` can be ((?:`[^`\n]+`(?:,|\s|or|and)*)+)")


@dataclass
class Pick:
    """A clickable answer: what it shows and the message it sends."""

    label: str
    message: str


@dataclass
class NoticeView:
    # warn: the check can't be built; ask: a value is needed first
    tone: str
    title: str
    summary: str
    subtitle: str | None = None
    picks_label: str | None = None
    picks: list[Pick] = field(default_factory=list)
    # Checks the user could write instead, in words to read, not to click.
    ideas_label: str | None = None
    ideas: list[str] = field(default_factory=list)
    # Asked after the ideas; the user types the answer in the message box.
    question: str | None = None
    chips_label: str | None = None
    chips: list[Pick] = field(default_factory=list)
    buttons: list[Pick] = field(default_factory=list)
    note: str | None = None
    # Placeholder of a box to type your own answer; None for no box.
    ask: str | None = None


def _facts(scope: dict[str, Any]) -> set[str]:
    return {
        f["name"]
        for r in scope.get("available_fact_retrievers") or []
        for f in r.get("facts") or []
        if f.get("name")
    }


def _plain(text: str) -> str:
    return re.sub(r"[`*]", "", text).strip().rstrip(".")


def _idea(words: str) -> str:
    """ "the repo must have a CODEOWNERS file" → "The repo must have a
    CODEOWNERS file"; "whether a CODEOWNERS file exists" → "A CODEOWNERS
    file exists"."""
    words = _plain(words)
    return first_upper(re.sub(r"^whether\s+", "", words, flags=re.IGNORECASE))


def _other_options(message: str) -> list[str]:
    """What else the user may have meant: an "Other options:" line, or the
    reply's "(for example: a, b, or c)"."""
    line = _OTHER_OPTIONS_RE.search(message)
    example = _FOR_EXAMPLE_RE.search(message)
    text = line[1] if line else example[1] if example else ""
    options = _CODE_RE.findall(text) or re.split(r",\s*(?:or\s+)?|\s+or\s+|;\s*", text)
    return [_plain(o) for o in options if _plain(o)]


def _no_fact(message: str, scope: dict[str, Any]) -> NoticeView | None:
    match = _NO_FACT_RE.search(message)
    if not match:
        return None
    known = _facts(scope)
    ideas: list[str] = []
    for fact, words in _FACT_WITH_WORDS_RE.findall(message):
        if fact in known and _idea(words) not in ideas:
            ideas.append(_idea(words))
    chips = [Pick(first_upper(option), f"I mean: {option}") for option in _other_options(message)]
    subject = _plain(match[1])
    return NoticeView(
        tone="warn",
        title="Can't build this check yet",
        subtitle="No fact measures this",
        summary=f"There's no fact for {subject}.",
        ideas_label="Checks you could write instead" if ideas else None,
        ideas=ideas[:_MAX_PICKS],
        question="Would you like to write one of these, or something else?"
        if ideas
        else "What would you like to check instead?",
        chips_label="Or did you mean" if chips else None,
        chips=chips[:3],
        # "Tell me something else" only next to answers to click: otherwise
        # the question already asks the user to type.
        ask="Or describe a different check" if chips else None,
    )


def lacks_alternatives(message: str, scope: dict[str, Any]) -> bool:
    """A "no fact fits" reply that offers the user nothing to pick from.

    True when the reply says no fact measures the request, facts exist, and
    the card would have neither checks to write instead nor options to ask
    "did you mean" about: it would show only the bare question.
    """
    if not _facts(scope):
        return False
    view = _no_fact(message, scope)
    return bool(view and not view.ideas and not view.chips)


def _kind_words(kind: str) -> str:
    kind = _plain(kind)
    return first_upper(kind if kind.lower().endswith("s") else f"{kind}s")


def _no_kind(message: str, scope: dict[str, Any]) -> NoticeView | None:
    match = _NO_KIND_RE.search(message)
    if not match or match[1] not in _facts(scope):
        return None
    fact, asked, other = match[1], _kind_words(match[2]), _kind_words(match[3])
    label = fact_labels(scope).get(fact) or humanize_fact(fact)
    return NoticeView(
        tone="warn",
        title=f"Can't check this for {asked}",
        subtitle=label,
        summary=f"{label} is only collected for {other}, so a check on {asked} would never "
        "have a result.",
        buttons=[
            Pick(f"Check {other} instead", f"Yes, check {other} instead"),
            Pick("No", "No"),
        ],
    )


def _which_value(message: str, scope: dict[str, Any]) -> NoticeView | None:
    question = _WHICH_RE.search(message)
    can_be = _CAN_BE_RE.search(message)
    if not question or not can_be or can_be[1] not in _facts(scope):
        return None
    fact = can_be[1]
    label = fact_labels(scope).get(fact) or humanize_fact(fact)
    values = _CODE_RE.findall(can_be[2])
    chips = [
        Pick(f"{first_upper(must.lower())} {value}", f"It {must.lower()} {value}")
        for must, value in _MUST_RE.findall(question[1])
    ]
    if not chips:
        return None
    listed = ", ".join(values[:-1]) + f" or {values[-1]}" if len(values) > 1 else "".join(values)
    return NoticeView(
        tone="ask",
        title="Which value should pass?",
        subtitle=label,
        summary=f"{label} can be {listed}." if values else f"Which {label.lower()} should pass?",
        # No "Pick one" above the values, whatever their number.
        chips=chips,
        ask="Or type another value",
    )


def notice_view(message: str, scope: dict[str, Any]) -> NoticeView | None:
    """The notice card for a reply that drafts no rule, or None."""
    for read in (_no_fact, _no_kind, _which_value):
        view = read(message, scope)
        if view:
            return view
    return None
