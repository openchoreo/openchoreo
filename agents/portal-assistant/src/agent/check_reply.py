# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Read the parts of a check_authoring reply that the confirm card needs.

The model writes the rule as a fenced JSON block, followed for an edit by a
"Suggested related changes" list for the check's Name and Description.
"""

from __future__ import annotations

import json
import logging
import re
from typing import Any

logger = logging.getLogger(__name__)

RULE_FENCE_RE = re.compile(r"```(?:json)?\s*([\s\S]*?)```", re.IGNORECASE)
_SUGGESTION_RE = re.compile(
    r"^[ \t]*(?:[-*]|\d+[.)])?[ \t]*\**(Name|Description)\**"
    r"(?:[ \t]*\(new\))?\**[ \t]*:[ \t]*\**[ \t]*(.+?)[ \t]*$",
    re.IGNORECASE | re.MULTILINE,
)
_ARROW_RE = re.compile(r"\s+(?:→|->)\s+")
_CODE_SPAN_RE = re.compile(r"`([^`\n]+)`")
# Left in a suggested text only when the line couldn't be split into the
# current and the suggested text.
_UNREAD_RE = re.compile(r"`|→|->|⟶|=>")
# The lines of a drafted reply that the save card shows: the headings around
# the rule, the check's fields and the suggested-change bullets.
_DRAFT_LINE_RE = re.compile(
    r"^[ \t]*(?:[-*][ \t]*)?\**[ \t]*(?:"
    r"(?:Edited rule|Suggested related changes)\**[ \t]*:?\**[ \t]*$"
    r"|Rule \(json-rules-engine conditions\)\**[ \t]*:?\**[ \t]*$"
    r"|(?:Name|Description|(?:Suggested )?Fact Retriever|Suggested Category"
    r"|Suggested Maturity Tier)\**(?:[ \t]*\(new\))?\**[ \t]*:.*$"
    r"|None [—–-] the name and description still match.*$"
    r")\n?",
    re.IGNORECASE | re.MULTILINE,
)
# The rule block, with the line that leads into it ("Here's the change:").
_LEAD_IN_AND_RULE_RE = re.compile(
    r"(?:^[^\n]*:\**[ \t]*\n\s*)?```(?:json)?\s*[\s\S]*?```",
    re.IGNORECASE | re.MULTILINE,
)
# A line that is only a format heading from the prompt, e.g.
# "Step 1 — confirm the change" or "**Step 2 - the edited rule**".
_STEP_LABEL_RE = re.compile(
    r"^[ \t]*(?:#+[ \t]*)?\**[ \t]*Step[ \t]*[12][ \t]*[—–:-][^\n]*?\**[ \t]*$\n?",
    re.IGNORECASE | re.MULTILINE,
)
# A "Tested on" summary the model writes anyway (the prompt doesn't ask for
# one): its heading line and the bullets under it.
_TEST_SUMMARY_RE = re.compile(
    r"^[ \t]*\**Tested on\**[^\n]*(?:\n[ \t]*[-*•][^\n]*)*\n?",
    re.IGNORECASE | re.MULTILINE,
)

# A line the model writes about a test that failed, e.g. "Dry-run error: …".
_DRY_RUN_LINE_RE = re.compile(r"^[ \t]*\**Dry-?run\b[^\n]*\n?", re.IGNORECASE | re.MULTILINE)


def unwrap(node: Any) -> Any:
    """The conditions of ``{"conditions": ...}``; anything else as is."""
    if isinstance(node, dict) and set(node) == {"conditions"}:
        return node["conditions"]
    return node


def clean(value: str) -> str:
    return value.strip().strip('`"*').strip()


def strip_step_labels(message: str) -> str:
    """``message`` without the prompt's Step 1 / Step 2 heading lines."""
    return _STEP_LABEL_RE.sub("", message).strip()


def reply_words(message: str) -> str | None:
    """A drafted reply's own words, without the rule block, the check's
    fields and the suggested changes, which the save card shows. Without any
    test result in the model's words too ("Tested on …", "Dry-run error: …"):
    only code reports a test, from the tool's own result. None when nothing
    else is left."""
    text = _DRAFT_LINE_RE.sub("", _LEAD_IN_AND_RULE_RE.sub("", message))
    text = _DRY_RUN_LINE_RE.sub("", _TEST_SUMMARY_RE.sub("", text))
    text = re.sub(r"\n{3,}", "\n\n", text).strip()
    return text or None


# A guard the model sometimes adds to a new check's rule, which nobody asked for.
_RESOLVED_FACT = "githubRepositoryResolved"


def _is_resolved_guard(condition: Any) -> bool:
    return (
        isinstance(condition, dict)
        and condition.get("fact") == _RESOLVED_FACT
        and condition.get("operator") == "equal"
        and condition.get("value") is True
    )


def without_unrequested_guard(message: str, requests: list[str]) -> str:
    """The reply with the repository-resolved guard taken out of its rule.

    Only a ``githubRepositoryResolved = true`` condition inside a top-level
    ``all`` with other conditions, and only when none of the user's
    ``requests`` mention "resolved". Any other reply or rule is returned as it
    is, and so is a rule that would be left empty.
    """
    if any("resolved" in request.lower() for request in requests):
        return message
    fence = RULE_FENCE_RE.search(message)
    if not fence:
        return message
    try:
        rule = json.loads(fence.group(1))
    except ValueError:
        return message
    wrapped = isinstance(rule, dict) and set(rule) == {"conditions"}
    conditions = rule["conditions"] if wrapped else rule
    if not isinstance(conditions, dict) or not isinstance(conditions.get("all"), list):
        return message
    kept = [c for c in conditions["all"] if not _is_resolved_guard(c)]
    if not kept or len(kept) == len(conditions["all"]):
        return message
    conditions = {**conditions, "all": kept}
    text = json.dumps({"conditions": conditions} if wrapped else conditions, indent=2)
    return message[: fence.start(1)] + text + "\n" + message[fence.end(1) :]


def parse_suggestions(tail: str) -> dict[str, tuple[str | None, str]]:
    """The model's suggestions after the rule block, keyed by field label."""
    found: dict[str, tuple[str | None, str]] = {}
    for match in _SUGGESTION_RE.finditer(tail):
        label = match.group(1).capitalize()
        value = match.group(2)
        spans = _CODE_SPAN_RE.findall(value)
        if spans and value.lstrip("*").startswith("`"):
            # `current` → `suggested`, read from the backticks so the arrow
            # (or its absence) and any note after them don't matter.
            old, new = (
                (clean(spans[0]), clean(spans[1])) if len(spans) > 1 else (None, clean(spans[0]))
            )
        else:
            parts = _ARROW_RE.split(value)
            old = clean(parts[0]) if len(parts) > 1 else None
            new = clean(re.sub(r"[`\"*]+[.,;:!]*$", "", parts[-1]))
        if _UNREAD_RE.search(new):
            # Better no suggestion than the current and suggested text glued
            # together on the card.
            logger.warning("Suggestion for %s skipped: the new text couldn't be read", label)
            continue
        if new:
            found[label] = (old, new)
    return found
