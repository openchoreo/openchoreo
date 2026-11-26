# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A saved check like the new one a chat just drafted.

Before a drafted new check gets its save card, code looks for a saved check
with the same rule, or a similar one, as the signed-in user: ``list_checks``,
then ``get_check`` for the checks that use the same retriever or have the
same name. Read tools only. When one is found, the reply's card recommends
editing that check instead and says why, and still offers to create a
separate check: the user decides. Choosing that gets the draft's save card,
unchanged (``card_for_separate_check``).

Any failure means no lookup: the save card shows as before.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from dataclasses import dataclass
from typing import Any

from common.auth.bearer import BearerTokenAuth
from src.agent.check_reply import unwrap
from src.agent.confirm_card import build_confirmation
from src.agent.rule_view import rule_lines
from src.clients.mcp import backstage_tools_as_user, call_backstage_tool

logger = logging.getLogger(__name__)

# Saved checks read with get_check per lookup, at most: same name first,
# then published ones.
_MAX_READS = 20

# "Create a separate check" on the card: what it sends, naming the check the
# user chose not to edit, so a later draft in the chat doesn't offer it again.
_SEPARATE = "Create a separate check instead of editing {name}"
_SEPARATE_RE = re.compile(r"Create a separate check instead of editing (.+?)[.!]*", re.IGNORECASE)


@dataclass(frozen=True)
class ExistingCheck:
    """A saved check like the drafted one.

    ``match``: ``"same"`` (the same conditions, in any order) or
    ``"similar"``. ``reason`` of a similar one: ``"values"`` (the same facts
    with other values or comparisons) or ``"name"`` (the same name only).
    ``rule``: its conditions, as JSON.
    """

    match: str
    reason: str
    id: str
    name: str
    description: str
    fact_ids: list[str]
    status: str
    rule: str


@dataclass(frozen=True)
class ExistingCheckView:
    """What the card says about the check it found, in plain words."""

    title: str
    subtitle: str
    summary: str
    question: str
    edit_label: str
    # Sent once the chat is about that check; None: nothing is sent, the
    # drawer shows its "Edit this check" intro.
    edit_message: str | None
    separate_message: str
    next_step: str
    rule_label: str
    rule: list[tuple[str, str]]
    draft: list[tuple[str, str]]
    check: dict[str, Any]


def _key(node: Any) -> str:
    """Conditions as text that ignores the order of keys and of list items."""
    if isinstance(node, dict):
        return "{" + ",".join(f"{json.dumps(k)}:{_key(v)}" for k, v in sorted(node.items())) + "}"
    if isinstance(node, list):
        return "[" + ",".join(sorted(_key(v) for v in node)) + "]"
    return json.dumps(node)


def _facts(node: Any) -> frozenset[str]:
    """The facts a rule tests."""
    if isinstance(node, dict):
        found = {node["fact"]} if isinstance(node.get("fact"), str) else set()
        return frozenset(found.union(*(_facts(v) for v in node.values())))
    if isinstance(node, list):
        return frozenset().union(*(_facts(v) for v in node))
    return frozenset()


def _name(text: str | None) -> str:
    return " ".join((text or "").split()).lower()


def _declined(messages: list[dict[str, str]]) -> set[str]:
    """The checks the user chose not to edit in this chat, by name."""
    names = set()
    for message in messages:
        if message.get("role") != "user":
            continue
        match = _SEPARATE_RE.fullmatch((message.get("content") or "").strip())
        if match:
            names.add(_name(match.group(1)))
    return names


def _compare(draft: Any, draft_name: str, check: dict[str, Any]) -> ExistingCheck | None:
    rule = unwrap(check.get("rule"))
    if isinstance(rule, str):
        try:
            rule = json.loads(rule)
        except ValueError:
            return None
    if not isinstance(rule, dict):
        return None
    if _key(rule) == _key(draft):
        match, reason = "same", "rule"
    elif _facts(rule) and _facts(rule) == _facts(draft):
        match, reason = "similar", "values"
    elif _name(check.get("name")) == draft_name:
        match, reason = "similar", "name"
    else:
        return None
    return ExistingCheck(
        match=match,
        reason=reason,
        id=check["id"],
        name=check.get("name") or "",
        description=check.get("description") or "",
        fact_ids=list(check.get("factIds") or []),
        status=check.get("status") or "draft",
        rule=json.dumps(rule, separators=(",", ":")),
    )


async def find_existing_check(
    confirmation: dict[str, Any],
    messages: list[dict[str, str]],
    *,
    backstage_token: str | None,
) -> ExistingCheck | None:
    """The saved check most like the new check on ``confirmation``, or None.

    The same rule before a similar one, then a published check before a
    draft. Checks the user already chose not to edit in this chat are left
    out.
    """
    if confirmation.get("kind") != "create" or not backstage_token:
        return None
    try:
        draft = unwrap(json.loads(confirmation["new_rule"]))
    except KeyError, TypeError, ValueError:
        return None
    draft_name = _name(confirmation.get("check_name"))
    fact_ids = set(confirmation.get("fact_ids") or [])
    declined = _declined(messages)
    try:
        tools = await backstage_tools_as_user(BearerTokenAuth(backstage_token))
        listed = (await call_backstage_tool(tools, "list_checks", {}))["checks"]
        likely = [
            c
            for c in listed
            if _name(c.get("name")) not in declined
            and (_name(c.get("name")) == draft_name or fact_ids & set(c.get("factIds") or []))
        ]
        likely.sort(
            key=lambda c: (_name(c.get("name")) != draft_name, c.get("status") != "published"),
        )
        if len(likely) > _MAX_READS:
            logger.info("Existing checks: reading %d of %d likely ones", _MAX_READS, len(likely))
        read = await asyncio.gather(
            *(
                call_backstage_tool(tools, "get_check", {"id": c["id"]})
                for c in likely[:_MAX_READS]
            ),
            return_exceptions=True,
        )
    except Exception as exc:  # noqa: BLE001 — no lookup; the save card shows as before
        logger.warning("No lookup of existing checks: %s", exc)
        return None
    found = [
        existing
        for result in read
        if isinstance(result, dict)
        and (existing := _compare(draft, draft_name, result.get("check") or {}))
    ]
    if not found:
        logger.info("Existing checks: none like the draft (%d read)", len(read))
        return None
    found.sort(key=lambda e: (e.match != "same", e.status != "published"))
    best = found[0]
    logger.info(
        "Existing check like the draft: %s (%s, %s, %s)",
        best.id,
        best.match,
        best.reason,
        best.status,
    )
    return best


def _change_message(confirmation: dict[str, Any], scope: dict[str, Any]) -> str:
    """What "Change it to my rule" sends once the chat is about that check."""
    lines = rule_lines(confirmation.get("new_rule") or "", scope)
    if not lines:
        return "Change this check to the new rule you drafted"
    return "Change this check to my new rule ({})".format(
        ", ".join(f"{label}: {value}" for label, value in lines),
    )


def existing_check_view(
    found: ExistingCheck,
    confirmation: dict[str, Any],
    scope: dict[str, Any],
) -> ExistingCheckView:
    """The card's words: what was found, why editing it is recommended, and
    what happens next."""
    name = found.name
    draft = found.status != "published"
    if found.match == "same":
        title = "This check is already a draft" if draft else "This check already exists"
        summary = (
            f"{name} is a saved draft with this rule. Drafts aren't in any score yet."
            if draft
            else f"{name} already checks this rule. It's published, so it's in every score."
        ) + " Editing it keeps one check for this rule, not two that test the same thing."
        question = "Would you like to edit it, or create a separate check?"
        edit_label = "Edit the draft" if draft else "Edit this check"
        edit_message = None
        next_step = (
            f"Editing moves this chat to {name}: I can explain it, improve it or test it "
            "on a service."
        )
        rule_label = "What it checks"
    else:
        title = (
            "A check with this name exists"
            if found.reason == "name"
            else "A similar draft exists"
            if draft
            else "A similar check exists"
        )
        if found.reason == "name":
            summary = (
                f"A check named {name} already exists, but it checks something else. "
                "If it's meant to cover this, changing it keeps one check. If not, create "
                "a separate check and give it another name."
            )
        else:
            summary = (
                f"{name} is a draft that checks the same facts with other values. It isn't "
                "in any score yet, so it's safe to change."
                if draft
                else f"{name} checks the same facts with other values. It's published, so a "
                "change updates it for every service."
            ) + " Changing it keeps one check for this, not two that disagree."
        question = "Would you like to change it to your rule, or create a separate check?"
        edit_label = "Change it to my rule"
        edit_message = _change_message(confirmation, scope)
        next_step = (
            f"Changing moves this chat to {name}. I'll show you the change before anything "
            "is saved."
        )
        rule_label = "What it checks now"
    return ExistingCheckView(
        title=title,
        subtitle=f"{name} · {'Draft' if draft else 'Published'}",
        summary=summary,
        question=question,
        edit_label=edit_label,
        edit_message=edit_message,
        separate_message=_SEPARATE.format(name=name),
        next_step=next_step,
        rule_label=rule_label,
        rule=rule_lines(found.rule, scope) or [],
        draft=(
            []
            if found.match == "same"
            else rule_lines(confirmation.get("new_rule") or "", scope) or []
        ),
        # The drawer moves the chat to this check with these, like after a save.
        check={
            "checkId": found.id,
            "name": name,
            "description": found.description,
            "rule": found.rule,
            "factIds": found.fact_ids,
            "status": found.status,
        },
    )


async def card_for_separate_check(
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
    *,
    user_sub: str,
    backstage_token: str | None,
) -> tuple[dict[str, Any], str] | None:
    """The drafted check's save card and the draft, when the user just chose
    "Create a separate check" on the card that found a check like it, or None.

    Built by code from the previous reply (the draft), like "save this": no
    model call, and the card is the one the draft would have had, test first.
    """
    if not messages or messages[-1].get("role") != "user" or (scope or {}).get("check_id"):
        return None
    if not _SEPARATE_RE.fullmatch((messages[-1].get("content") or "").strip()):
        return None
    draft = next(
        (m.get("content") or "" for m in reversed(messages[:-1]) if m.get("role") == "assistant"),
        "",
    )
    if not draft:
        return None
    card = await build_confirmation(
        draft,
        scope,
        user_sub=user_sub,
        backstage_token=backstage_token,
    )
    return (card, draft) if card else None
