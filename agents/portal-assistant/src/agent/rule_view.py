# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A check edit's confirmation question in plain words, for the A2UI card.

Code, not the model, decides what the card shows: the current rule comes
from the saved check (``scope["current_rule"]``) and each proposal from the
backtick notation the prompt asks the model to use
(`` `fact` `operator` `value` AND … ``). A rule this can't show exactly —
nested groups, NOT, a line it can't read — gives None, and the reply keeps
plain Yes / No buttons instead, or numbered choices a card of their own
(numbered_choices).
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from typing import Any

from src.agent.check_reply import unwrap

# One condition: fact, operator, value.
Leaf = tuple[str, str, Any]

_PROPOSAL_RE = re.compile(
    r"^[ \t]*(?:([1-9])[.)][ \t]*)?\**[ \t]*Proposed rule\**[ \t]*:?[ \t]*\**[ \t]*(.+)$",
    re.IGNORECASE | re.MULTILINE,
)
_LEAF_RE = re.compile(r"`([^`\n]+)`[ \t]+`([^`\n]+)`[ \t]+`([^`\n]+)`")
_OPERATORS = (
    r"(equal|notEqual|lessThan|lessThanInclusive|greaterThan|greaterThanInclusive|"
    r"in|notIn|contains|doesNotContain)"
)
# The same condition written other ways by the model: in one pair of backticks,
# `githubVisibility in ["private","internal"]`, or with a bare value,
# `githubVisibility` `in` ["private", "internal"].
_ONE_SPAN_LEAF_RE = re.compile(r"`([A-Za-z][\w.]*)[ \t]+" + _OPERATORS + r"[ \t]+([^`\n]+)`")
_BARE_VALUE_LEAF_RE = re.compile(
    r"`([A-Za-z][\w.]*)`[ \t]+`" + _OPERATORS + r"`[ \t]+"
    r"(\[[^\]\n]*\]|\"[^\"\n]*\"|-?\d+(?:\.\d+)?|[A-Za-z][\w.-]*)"
)
_LABEL_MAX = 48
_UNDERSTOOD_RE = re.compile(
    r"^[ \t]*\**What I understood\**[ \t]*:?[ \t]*\**[ \t]*(.+)$",
    re.IGNORECASE | re.MULTILINE,
)
_CHOICE_RE = re.compile(r"^[ \t]*([1-9])[.)][ \t]+(.+)$", re.MULTILINE)
_PROPOSED_LABEL_RE = re.compile(
    r"^\**[ \t]*Proposed rule\**[ \t]*:?[ \t]*\**[ \t]*",
    re.IGNORECASE,
)
# The dash between a proposal's rule and the words naming it.
_NAMED_RE = re.compile(r"\s+[—–]\s+|\s+-\s+(?=[^`]*$)")


@dataclass
class ChangeRow:
    """One condition of the card: what it is now and what it becomes."""

    label: str
    # changed, same, added, removed, or pick (differs between the options)
    state: str
    before: str | None = None
    after: str | None = None


@dataclass
class Option:
    """One of the numbered choices, in plain words."""

    number: int
    value: str


@dataclass
class ChangeView:
    rows: list[ChangeRow]
    options: list[Option] = field(default_factory=list)
    # The reply's "What I understood" sentence, shown at the top of the card.
    summary: str | None = None

    @property
    def current(self) -> list[tuple[str, str]]:
        """Each condition of the rule as it is now: (label, value in words)."""
        return [(row.label, row.before) for row in self.rows if row.before is not None]

    @property
    def changes(self) -> list[ChangeRow]:
        """Only the conditions that change."""
        return [row for row in self.rows if row.state != "same"]

    @property
    def clear(self) -> bool:
        """Whether each choice reads as one value: the choices differ in one
        condition at most. Choices that differ in more ("Yes; 5 or more" vs
        "Removed; 5 or more") are shown as the input card instead."""
        return sum(row.state == "pick" for row in self.rows) <= 1


def _value(raw: str) -> Any:
    try:
        return json.loads(raw)
    except ValueError:
        return raw.strip()


def _leaves(text: str) -> list[Leaf] | None:
    """The conditions on one proposal line, or None if they aren't a flat AND / OR."""
    text = _NAMED_RE.split(text, maxsplit=1)[0]
    matches = (
        list(_LEAF_RE.finditer(text))
        or list(_ONE_SPAN_LEAF_RE.finditer(text))
        or list(_BARE_VALUE_LEAF_RE.finditer(text))
    )
    if not matches:
        return None
    joiners = {
        text[a.end() : b.start()].strip().upper()
        for a, b in zip(matches, matches[1:], strict=False)
    }
    # Rows read as "all of these"; an OR rule is shown exactly instead.
    if not joiners <= {"AND"}:
        return None
    # Anything around the conditions, like NOT ( … ), changes their meaning.
    outside = text[: matches[0].start()] + text[matches[-1].end() :]
    if outside.strip(" \t*.,;:"):
        return None
    return [(m[1].strip(), m[2].strip(), _value(m[3])) for m in matches]


def _proposals(message: str) -> dict[int, list[Leaf]] | None:
    """Each proposal in the reply, by choice number (1 for a single one)."""
    found: dict[int, list[Leaf]] = {}
    for match in _PROPOSAL_RE.finditer(message):
        leaves = _leaves(match[2])
        if leaves is None:
            return None
        found[int(match[1] or 1)] = leaves
    return found or None


def _flat(rule_json: str | None) -> list[Leaf] | None:
    """A rule's conditions when it is one flat "all" group (or a single "any")."""
    try:
        rule = unwrap(json.loads(rule_json or ""))
    except TypeError, ValueError:
        return None
    if not isinstance(rule, dict) or len(rule) != 1:
        return None
    ((joiner, group),) = rule.items()
    if joiner not in ("all", "any") or not isinstance(group, list):
        return None
    if joiner == "any" and len(group) > 1:
        return None  # rows read as "all of these"
    if not all(isinstance(c, dict) and {"fact", "operator"} <= set(c) for c in group):
        return None
    return [(c["fact"], c["operator"], c.get("value")) for c in group]


def current_conditions(scope: dict[str, Any]) -> list[Leaf] | None:
    """The saved rule's conditions, or None when they aren't a flat AND / OR."""
    return _flat(scope.get("current_rule"))


def _fmt(value: Any) -> str:
    if isinstance(value, list):
        return ", ".join(_fmt(v) for v in value)
    if isinstance(value, bool):
        return "yes" if value else "no"
    return str(value)


def value_words(operator: str, value: Any) -> str:
    """A condition's operator and value in plain words, e.g. "3 or more"."""
    v = _fmt(value)
    if isinstance(value, bool) and operator in ("equal", "notEqual"):
        return "Yes" if value == (operator == "equal") else "No"
    whole = isinstance(value, int | float) and not isinstance(value, bool)
    words = {
        "equal": f"exactly {v}" if whole else v,
        "notEqual": f"not {v}",
        "greaterThanInclusive": f"{v} or more",
        "greaterThan": f"more than {v}",
        "lessThanInclusive": f"{v} or less",
        "lessThan": f"less than {v}",
        "in": f"one of {v}",
        "notIn": f"none of {v}",
        "contains": f"contains {v}",
        "doesNotContain": f"doesn't contain {v}",
    }
    return words.get(operator, f"{operator} {v}")


def understood(message: str) -> str | None:
    """The reply's "What I understood" sentence, without Markdown.

    A closing lead-in to the choices ("Tell me which you want (reply with
    the number):") is left out: the card shows the choices itself.
    """
    match = _UNDERSTOOD_RE.search(message)
    text = re.sub(r"[`*]", "", match[1]).strip() if match else ""
    text = re.sub(r"(?<=[.!?])\s+[^.!?]*:$", "", text).rstrip(":").strip()
    return text[:1].upper() + text[1:] or None


def numbered_choices(message: str) -> list[tuple[int, str]]:
    """The reply's numbered choices as (number, label), for a card when
    change_view can't show them: the few words naming each choice after its
    dash, or else its rule as written, without Markdown."""
    found: dict[int, str] = {}
    for number, text in _CHOICE_RE.findall(message):
        rule, *named = _NAMED_RE.split(_PROPOSED_LABEL_RE.sub("", text), maxsplit=1)
        words = re.sub(r"[`*]", "", named[0]).strip() if named else ""
        label = words[:1].upper() + words[1:] or re.sub(r"[`*]", "", rule).strip()
        if label:
            found.setdefault(int(number), label)
    return sorted(found.items())


def fact_labels(scope: dict[str, Any]) -> dict[str, str]:
    """Fact name → its short description, from the facts forwarded with the chat."""
    labels: dict[str, str] = {}
    for retriever in scope.get("available_fact_retrievers") or []:
        for fact in retriever.get("facts") or []:
            text = (fact.get("description") or "").split(":")[0].strip().rstrip(".")
            if fact.get("name") and text and len(text) <= _LABEL_MAX:
                labels[fact["name"]] = text
    return labels


def humanize_fact(fact: str) -> str:
    """ "githubBranchProtected" → "Branch protected"."""
    words = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", " ", fact).split()
    if len(words) > 1 and words[0].lower() == "github":
        words = words[1:]
    return " ".join(words).capitalize()


def change_view(message: str, scope: dict[str, Any]) -> ChangeView | None:
    """The card for an edit question, or None when it can't be shown exactly."""
    current = current_conditions(scope)
    proposals = _proposals(message)
    if current is None or proposals is None:
        return None
    return _view(current, proposals, scope, understood(message))


def saved_change_view(
    current_rule: str | None,
    new_rule: str,
    scope: dict[str, Any],
) -> ChangeView | None:
    """The change a confirm card saves, read from its signed rules."""
    current, new = _flat(current_rule), _flat(new_rule)
    if current is None or new is None:
        return None
    return _view(current, {1: new}, scope, None)


def rule_lines(rule: str, scope: dict[str, Any]) -> list[tuple[str, str]] | None:
    """A new check's rule as (label, value in words), or None if it can't be shown exactly."""
    leaves = _flat(rule)
    if leaves is None or len({fact for fact, _, _ in leaves}) != len(leaves):
        return None
    labels = fact_labels(scope)
    return [(labels.get(f) or humanize_fact(f), value_words(op, v)) for f, op, v in leaves]


def _view(
    current: list[Leaf],
    proposals: dict[int, list[Leaf]],
    scope: dict[str, Any],
    summary: str | None,
) -> ChangeView | None:
    all_leaves = [current, *proposals.values()]
    rules = [{f: (op, v) for f, op, v in leaves} for leaves in all_leaves]
    if any(len(rule) != len(leaves) for rule, leaves in zip(rules, all_leaves, strict=True)):
        return None  # the same fact twice: rows would hide one
    now, *options = rules
    labels = fact_labels(scope)

    facts = list(now) + [f for rule in options for f in rule if f not in now]
    rows: list[ChangeRow] = []
    picked: list[str] = []
    for fact in dict.fromkeys(facts):
        label = labels.get(fact) or humanize_fact(fact)
        before = value_words(*now[fact]) if fact in now else None
        afters = [rule.get(fact) for rule in options]
        # Compared, not hashed: an `in` value is a list.
        if len(options) > 1 and any(after != afters[0] for after in afters):
            picked.append(fact)
            rows.append(ChangeRow(label, "pick", before))
            continue
        after = afters[0]
        if after == now.get(fact):
            rows.append(ChangeRow(label, "same", before))
        elif after is None:
            rows.append(ChangeRow(label, "removed", before))
        elif before is None:
            rows.append(ChangeRow(label, "added", after=value_words(*after)))
        else:
            rows.append(ChangeRow(label, "changed", before, value_words(*after)))

    if len(options) > 1 and not picked:
        # Numbered choices that don't differ: "yes" wouldn't approve them,
        # so keep the plain choice buttons.
        return None
    choices: list[Option] = []
    if picked:
        for number, rule in zip(proposals, options, strict=True):
            # Without the condition: "removed" when the rule has it now, else
            # "not added" (another choice adds it).
            words = [
                value_words(*rule[f]) if f in rule else "removed" if f in now else "not added"
                for f in picked
            ]
            choices.append(Option(number, "; ".join(words)))
    return ChangeView(rows, choices, summary)
