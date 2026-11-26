# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A check edit reply that asks instead of proposing a rule, read for an A2UI ask card.

Two kinds of reply: an open question in the Step 1 format ("What I
understood: … what should change instead?"), shown with the saved rule in
plain words and a small editor for each number or yes/no condition; and
options written as a numbered list in words ("Do you mean: 1. … 2. …"),
shown as one button each. Every answer is a plain sentence sent as the next
message, so the agent proposes the change and the change card asks as
usual. The caller checks first that the reply proposes no rule.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

from src.agent.rule_view import (
    Leaf,
    current_conditions,
    fact_labels,
    humanize_fact,
    rule_lines,
    understood,
    value_words,
)
from src.agent.words import first_upper, lower_first

_SUMMARY_MAX = 160
KEEP_MESSAGE = "Keep the rule as it is"
KEEP_LABEL = "Keep the current rule"
# What a number condition's editor offers, by operator.
COMPARISONS = {
    "greaterThanInclusive": "at least",
    "greaterThan": "more than",
    "equal": "exactly",
    "lessThanInclusive": "at most",
    "lessThan": "less than",
}

_OPTION_RE = re.compile(r"^[ \t]*([1-9])[.)][ \t]+(.+)$", re.MULTILINE)
# An indented line under an option: more of it, or a sub-point ("   - …").
_CONTINUATION_RE = re.compile(r"^[ \t]+([-*•][ \t]+)?(\S.*)$")
# A closing line after the options that tells the user how to answer
# ("Reply with 1 or 2.", "Or tell me something else."); it isn't part of one.
_FOOTER_RE = re.compile(
    r"^[ \t]*(?:please[ \t]+)?(?:reply|answer|respond|choose|pick|say|type|tell[ \t]+me|"
    r"or[ \t]+(?:reply|tell|type|say|pick|choose))\b",
    re.IGNORECASE,
)
# A numbered list is a question only when the reply asks the user to pick.
_PICK_RE = re.compile(r"\b(?:reply with|do you mean|which|choose|pick)\b", re.IGNORECASE)
# "(tell me the exact number)": the option needs a number from the user.
_NEEDS_VALUE_RE = re.compile(r"\s*\((?:tell me|say|give me|which)\b[^)]*\)", re.IGNORECASE)
_TRAILING_RE = re.compile(r"(?:[\s,;:?.]|\bor\b)+$", re.IGNORECASE)
_CURRENT_RULE_LINE_RE = re.compile(r"^.*Current rule\**[ \t]*:.*$", re.IGNORECASE | re.MULTILINE)


@dataclass
class Editor:
    """A number or yes/no condition the user can change in place."""

    label: str
    # number: a whole number with a comparison; choice: Yes / No
    kind: str
    value: Any
    # The message sent, with {value} for the new value in words.
    template: str
    comparison: str | None = None
    # The message that removes it; None for the rule's only condition.
    remove: str | None = None


@dataclass
class AskOption:
    """One of the reply's numbered options."""

    number: int
    label: str
    # What a click sends. An option that needs a number has a template
    # with {value} instead, and the number its field starts at.
    message: str
    value: int | None = None


@dataclass
class AskView:
    title: str
    summary: str | None
    current: list[tuple[str, str]] = field(default_factory=list)
    editors: list[Editor] = field(default_factory=list)
    options: list[AskOption] = field(default_factory=list)
    placeholder: str = "Or type the change you want"
    keep: str = KEEP_LABEL


def _plain(text: str) -> str:
    return re.sub(r"\s+", " ", re.sub(r"[`*]", "", text)).strip()


def _short(text: str | None) -> str | None:
    """The text, or only its first sentence when it is long."""
    if not text or len(text) <= _SUMMARY_MAX:
        return text or None
    return re.split(r"(?<=[.!?])\s+", text, maxsplit=1)[0]


def _is_whole_number(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _editors(leaves: list[Leaf], labels: dict[str, str]) -> list[Editor]:
    editors: list[Editor] = []
    for fact, operator, value in leaves:
        name = lower_first(labels.get(fact) or humanize_fact(fact))
        label, template = first_upper(name), f"Change {name} to {{value}}"
        remove = f"Remove the condition: {name}" if len(leaves) > 1 else None
        if isinstance(value, bool) and operator in ("equal", "notEqual"):
            editors.append(
                Editor(label, "choice", value_words(operator, value), template, remove=remove),
            )
        elif _is_whole_number(value) and operator in COMPARISONS:
            editors.append(
                Editor(label, "number", value, template, COMPARISONS[operator], remove),
            )
    return editors


def edit_view(scope: dict[str, Any], summary: str | None) -> AskView | None:
    """The input card for the saved rule: each condition in plain words, an
    editor per number or yes/no value and a text box. None when the rule
    can't be shown exactly."""
    current = current_conditions(scope)
    lines = rule_lines(scope.get("current_rule") or "", scope)
    if current is None or lines is None:
        return None
    editors = _editors(current, fact_labels(scope))
    numbers = [e.value for e in editors if e.kind == "number"]
    return AskView(
        title="What should change?",
        summary=_short(summary),
        current=lines,
        editors=editors,
        placeholder=(
            f'Or type it, e.g. "make it {numbers[0] + 1}"'
            if numbers
            else "Or type the change you want"
        ),
    )


# A list item: "- …", "* …", "1. …". A reply that lists ideas or options the
# form can't show keeps its own card, so they aren't hidden.
_LIST_ITEM_RE = re.compile(r"^[ \t]*(?:[-*•]|\d+[.)])[ \t]+\S", re.MULTILINE)


def _open_question(message: str, scope: dict[str, Any]) -> AskView | None:
    said = understood(message)
    if not said or "?" not in message or _LIST_ITEM_RE.search(message):
        return None
    return edit_view(scope, said)


def _numbered_items(message: str) -> list[tuple[str, str]] | None:
    """Each numbered option with the indented lines under it (a wrapped line,
    or a sub-point), as (number, text). None when an unindented line that
    isn't a closing line ("Reply with 1 or 2.") sits between two options or
    after the last: which option it belongs to can't be told, and a button
    without it would send half the answer."""
    items: list[tuple[str, str]] = []
    in_item = stray = closed = False
    for line in message.splitlines():
        option = _OPTION_RE.match(line)
        if option:
            if stray or closed:
                return None
            items.append((option[1], option[2]))
            in_item = True
        elif not line.strip():
            continue
        elif in_item and (more := _CONTINUATION_RE.match(line)):
            number, text = items[-1]
            joiner = "; " if more[1] else " "
            items[-1] = (number, f"{text.rstrip()}{joiner}{more[2]}")
        elif items:
            in_item = False
            # A closing question ("Which one do you want?") ends the list too:
            # an option's own wrapped text doesn't end in one.
            if _FOOTER_RE.match(line) or line.rstrip().endswith("?"):
                closed = True
            else:
                stray = True
    return None if stray else items


def _options(message: str, scope: dict[str, Any]) -> AskView | None:
    found = _numbered_items(message)
    if found is None:
        return None
    numbers = [int(n) for n, _ in found]
    if len(found) < 2 or numbers != list(range(1, len(found) + 1)) or not _PICK_RE.search(message):
        return None
    whole = [v for _, _, v in current_conditions(scope) or [] if _is_whole_number(v)]
    start = whole[0] if len(whole) == 1 else 1

    options: list[AskOption] = []
    for number, text in zip(numbers, (t for _, t in found), strict=True):
        text = _TRAILING_RE.sub("", _plain(text))
        if not text:
            return None
        if _NEEDS_VALUE_RE.search(text) and "number" in text.lower():
            label = _TRAILING_RE.sub("", _NEEDS_VALUE_RE.sub("", text))
            options.append(AskOption(number, first_upper(label), f"{label}: {{value}}", start))
        else:
            options.append(AskOption(number, first_upper(text), text))

    # What comes before the list, without its "Do you mean:" lead-in.
    intro = message[: _OPTION_RE.search(message).start()]
    intro = re.sub(r"\s*[^.!?]*:$", "", _plain(_CURRENT_RULE_LINE_RE.sub("", intro)))
    return AskView(
        title="Which change do you mean?",
        summary=_short(understood(message) or intro),
        options=options,
        placeholder="Or type something else",
    )


def ask_view(message: str, scope: dict[str, Any]) -> AskView | None:
    """The ask card for an edit reply that asks instead of proposing, or None."""
    return _options(message, scope) or _open_question(message, scope)
