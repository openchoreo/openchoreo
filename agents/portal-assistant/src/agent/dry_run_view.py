# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A dry-run's result, read for the A2UI test card.

Read from dry_run_check's own call and result — the component, the rule it
ran and the fact values it compared — never from the model's words.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from typing import Any

from src.agent.rule_view import fact_labels, humanize_fact, rule_lines
from src.agent.words import lower_first

# The statuses that are an answer; "invalid" comes back as a tool error.
_RESULTS = ("passed", "failed", "not-applicable")
_VALUE_MAX = 120
# "… but didn't report githubA, githubB." in a missing-fact-values reason.
_MISSING_RE = re.compile(r"didn't report (.+?)\.?$")
# What the check / the entity didn't match, after the reason's ": " (the
# entity ref's own colon, "component:default/x", has no space after it).
_AFTER_COLON_RE = re.compile(r":\s+(.+?)\.?$")


@dataclass
class DryRun:
    status: str
    entity_ref: str | None = None
    fact_values: dict[str, Any] = field(default_factory=dict)
    # (code, message) — why a check doesn't apply.
    reasons: list[tuple[str, str]] = field(default_factory=list)
    # The conditions it ran, as JSON.
    rule: str | None = None
    # The retrievers it ran with; None when the call didn't say.
    fact_ids: list[str] | None = None


def _json(raw: Any) -> Any:
    if isinstance(raw, list):
        raw = " ".join(b.get("text", "") if isinstance(b, dict) else str(b) for b in raw)
    if isinstance(raw, dict):
        return raw
    try:
        return json.loads(raw or "")
    except TypeError, ValueError:
        return None


def read_dry_run(args: Any, content: Any) -> DryRun | None:
    """A dry_run_check call's result, or None when it gave none (an error)."""
    payload = _json(content)
    if not isinstance(payload, dict) or payload.get("status") not in _RESULTS:
        return None
    call = _json(args)
    call = call if isinstance(call, dict) else {}
    check = call.get("check") if isinstance(call.get("check"), dict) else {}
    rule = (
        (check.get("rule") or {}).get("conditions") if isinstance(check.get("rule"), dict) else None
    )
    values = payload.get("factValues")
    fact_ids = check.get("factIds")
    return DryRun(
        status=payload["status"],
        entity_ref=call.get("entityRef") if isinstance(call.get("entityRef"), str) else None,
        fact_values=values if isinstance(values, dict) else {},
        reasons=[
            (str(r.get("code") or ""), r["message"])
            for r in payload.get("reasons") or []
            if isinstance(r, dict) and isinstance(r.get("message"), str) and r["message"]
        ],
        rule=rule if isinstance(rule, str) else None,
        fact_ids=fact_ids
        if isinstance(fact_ids, list) and all(isinstance(i, str) for i in fact_ids)
        else None,
    )


def entity_name(entity_ref: str | None) -> tuple[str, str | None]:
    """ "component:default/order-api" → ("order-api", "Component")."""
    if not entity_ref:
        return "the component", None
    kind, _, rest = entity_ref.rpartition(":") if ":" in entity_ref else ("", "", entity_ref)
    return rest.rsplit("/", 1)[-1], kind.capitalize() or None


def actual_words(value: Any) -> str:
    """A fact's actual value in plain words; null means it wasn't collected."""
    if value is None:
        text = "not collected"
    elif isinstance(value, bool):
        text = "Yes" if value else "No"
    elif isinstance(value, list):
        text = ", ".join(actual_words(v) for v in value) or "none"
    elif isinstance(value, dict):
        text = json.dumps(value, separators=(",", ":"))
    else:
        text = str(value)
    return text if len(text) <= _VALUE_MAX else text[: _VALUE_MAX - 1] + "…"


def _label(fact: str, scope: dict[str, Any]) -> str:
    return fact_labels(scope).get(fact) or humanize_fact(fact)


def missing_facts(run: DryRun) -> list[str]:
    """The facts the rule needs that weren't reported for the component."""
    found: list[str] = []
    for code, message in run.reasons:
        match = _MISSING_RE.search(message) if code == "missing-fact-values" else None
        found += [f.strip() for f in match[1].split(",")] if match else []
    return [f for f in dict.fromkeys(found) if f and f not in run.fact_values]


def actual_lines(run: DryRun, scope: dict[str, Any]) -> list[tuple[str, str]]:
    """Each fact the rule compared, with its actual value, in plain words —
    and each one it needed that wasn't reported."""
    return [
        *((_label(fact, scope), actual_words(value)) for fact, value in run.fact_values.items()),
        *((_label(fact, scope), "not reported") for fact in missing_facts(run)),
    ]


def reason_words(run: DryRun, scope: dict[str, Any]) -> list[str]:
    """Why the check doesn't apply, in plain words, read from each reason's
    code. A code this doesn't know keeps the tool's own message."""
    name, _ = entity_name(run.entity_ref)
    words: list[str] = []
    for code, message in run.reasons:
        after = _AFTER_COLON_RE.search(message)
        if code == "missing-fact-values" and missing_facts(run):
            facts = ", ".join(lower_first(_label(f, scope)) for f in missing_facts(run))
            words.append(f"The facts collected for {name} have no value for {facts}.")
        elif code == "no-facts":
            words.append(
                f"No facts have been collected for {name} yet: the fact retriever hasn't run "
                "since it appeared, or something it needs, such as an annotation or "
                "credentials, is missing.",
            )
        elif code == "retriever-not-covering" and after:
            words.append(f"These facts aren't collected for {name}: {after[1]}.")
        elif code == "entity-filtered" and after:
            words.append(f'The check\'s "Applies to" filter leaves out {name}: {after[1]}.')
        elif code == "no-result":
            words.append(f"The check gave no result for {name}.")
        else:
            words.append(message)
    return list(dict.fromkeys(words))


def needed_lines(run: DryRun, scope: dict[str, Any]) -> list[tuple[str, str]]:
    """The rule it ran, in plain words; empty when it can't be shown exactly."""
    return rule_lines(run.rule or "", scope) or []


# json-rules-engine's built-in operators: the fact's value, the rule's value.
_OPERATORS = {
    "equal": lambda a, b: a == b,
    "notEqual": lambda a, b: a != b,
    "greaterThan": lambda a, b: a > b,
    "greaterThanInclusive": lambda a, b: a >= b,
    "lessThan": lambda a, b: a < b,
    "lessThanInclusive": lambda a, b: a <= b,
    "in": lambda a, b: a in b,
    "notIn": lambda a, b: a not in b,
    "contains": lambda a, b: b in a,
    "doesNotContain": lambda a, b: b not in a,
}


def _leaves(node: Any, negated: bool = False, required: bool = True):
    """Each condition of a rule, whether a `not` turns it around, and whether
    the rule needs it met (only "all of these" above it): a condition that is
    one of several choices can fail while the rule passes."""
    if not isinstance(node, dict):
        return
    for key in ("all", "any"):
        children = node.get(key)
        children = children if isinstance(children, list) else []
        # `not` turns "all" into "any" of the opposites, and back.
        choice = (key == "any") != negated and len(children) > 1
        for child in children:
            yield from _leaves(child, negated, required and not choice)
    if "not" in node:
        yield from _leaves(node["not"], not negated, required)
    if isinstance(node.get("fact"), str):
        yield node, negated, required


def _follows(value: Any, leaf: dict[str, Any], negated: bool) -> bool | None:
    """Whether a fact's value meets one condition; None when it can't be told."""
    operator = _OPERATORS.get(leaf.get("operator"))
    if operator is None or value is None:
        return None
    try:
        met = bool(operator(value, leaf.get("value")))
    except TypeError:
        return None
    return not met if negated else met


def fact_results(
    run: DryRun,
    scope: dict[str, Any],
) -> list[tuple[str, str, bool | None, str | None]]:
    """Each fact the rule compared: its label, the component's value in plain
    words, whether that follows the rule (None when it can't be told), and
    what the rule wants when it doesn't."""
    wants = dict(needed_lines(run, scope))
    checks: dict[str, list[bool | None]] = {}
    for leaf, negated, required in _leaves(_json(run.rule or "")):
        fact = leaf["fact"]
        # One of several choices: the fact alone can't say whether it follows.
        follows = _follows(run.fact_values.get(fact), leaf, negated) if required else None
        checks.setdefault(fact, []).append(follows)
    rows: list[tuple[str, str, bool | None, str | None]] = []
    for fact, value in run.fact_values.items():
        results = checks.get(fact) or [None]
        follows = None if None in results else all(results)
        label = _label(fact, scope)
        rows.append(
            (label, actual_words(value), follows, wants.get(label) if follows is False else None)
        )
    for fact in missing_facts(run):
        label = _label(fact, scope)
        rows.append((label, "not reported", False, wants.get(label)))
    return rows
