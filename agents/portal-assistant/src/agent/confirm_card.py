# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""The confirm card for a check_authoring reply.

The reply holds everything a save needs. For an edit's Step 2: the edited
rule and the suggested Name / Description. For a new check:
Name, Description, the retriever, category and tier, and the rule.
``build_confirmation`` reads them from the final message — code, not the
model, decides what the card offers — validates the rule through Backstage
as the signed-in user, and signs the change so the confirm endpoint saves
exactly what the card showed.

Any failure means no card: the reply still arrives, and the user can copy
the rule into the form as before.
"""

from __future__ import annotations

import contextlib
import json
import logging
import re
import uuid
from typing import Any

from common.auth.bearer import BearerTokenAuth
from src.agent.check_reply import RULE_FENCE_RE, clean, parse_suggestions, unwrap
from src.agent.pending_actions import sign
from src.agent.rule_view import same_rules
from src.agent.user_action import SKIP_TEST, decision_of, is_read_as_text
from src.clients.backstage_tools import GET_CHECK, LIST_FACT_SCHEMAS, VALIDATE_CHECK
from src.clients.mcp import backstage_tools_as_user, call_backstage_tool

logger = logging.getLogger(__name__)

# Suggestion label → (action / event field name).
_TEXT_FIELDS = {"Name": "name", "Description": "description"}

# The labelled lines before the rule in a new-check reply.
_HEAD_RE = re.compile(
    r"^[ \t]*\**(Name|Description|(?:Suggested )?Fact Retriever|Suggested Category"
    r"|Suggested Maturity Tier)\**[ \t]*:[ \t]*\**[ \t]*(.+?)[ \t]*$",
    re.IGNORECASE | re.MULTILINE,
)
# Tier names → metadata.rank, as in the scorecards plugin's TIER_LABEL.
_TIER_RANK = {"bronze": 1, "silver": 2, "gold": 3}

# A short request to save what was just drafted, and nothing else: "save
# this", "publish it", "yes, save and publish". Anything longer ("save it but
# rename it to …") goes to the model.
_SAVE_REQUEST_RE = re.compile(
    r"(?:(?:yes|ok|okay|sure|great|perfect|looks good)[,.!]?\s+)?(?:please\s+)?"
    r"(?:go ahead and\s+)?(?:save|apply|publish|unpublish|update)"
    r"(?:\s+(?:it|this|that|the (?:check|change|changes|rule|edit)))?"
    r"(?:\s+(?:as (?:a )?draft|and publish(?: it)?|now|please))*[.!]*",
    re.IGNORECASE,
)

# The save card's "Skip the test": straight to saving, without a test.
_SKIP_TEST_RE = re.compile(
    r"(?:skip (?:the )?test(?:ing)?|(?:save|continue) (?:it )?without (?:a )?test(?:ing)?|"
    r"no test(?:ing)?(?: needed)?)[.!]*",
    re.IGNORECASE,
)

SAVE_REQUEST_REPLY = "Here's the change, ready to save — pick an option on the card below."


def is_save_request(text: str) -> bool:
    """Whether typed ``text`` only asks to save, or to skip the test."""
    text = text.strip()
    return bool(_SAVE_REQUEST_RE.fullmatch(text) or _SKIP_TEST_RE.fullmatch(text))


def asks_to_save(message: dict[str, Any] | None) -> bool:
    """Whether the message only asks to save or to skip the test: a save
    card's Skip the test, or those words typed."""
    if decision_of(message) == SKIP_TEST:
        return True
    return is_read_as_text(message) and is_save_request((message or {}).get("content") or "")


def rule_fix_note(problems: list[str]) -> str:
    """Asks the model to fix the problems the validator found in its rule."""
    return "\n".join(
        [
            f"[Automatic check — not written by the user] {VALIDATE_CHECK} found problems in the "
            "rule you wrote:",
            *(f"- {problem}" for problem in problems),
            "Fix exactly those conditions — keep the rule's meaning — and write your reply "
            "again in the same format.",
        ]
    )


def _edited_rule(message: str) -> Any | None:
    """The conditions of the edited rule in the reply, or None."""
    fence = RULE_FENCE_RE.search(message)
    if not fence:
        return None
    try:
        rule = unwrap(json.loads(fence.group(1)))
    except ValueError:
        return None
    # Exactly one root: with both "all" and "any" the engine reads only "any".
    if not isinstance(rule, dict) or len(rule) != 1 or next(iter(rule)) not in ("all", "any", "not"):
        return None
    return rule


def is_saved_rule(message: str, scope: dict[str, Any] | None) -> bool:
    """Whether the reply's rule is the check's saved rule (``current_rule``)."""
    saved = (scope or {}).get("current_rule")
    if not saved:
        return False
    try:
        saved_rule = unwrap(json.loads(saved))
    except ValueError:
        return False
    rule = _edited_rule(message)
    return rule is not None and same_rules(rule, saved_rule)


def same_rule(first: str, second: str) -> bool:
    """Whether two replies show the same rule (both have one)."""
    rule = _edited_rule(first)
    return rule is not None and same_rules(rule, _edited_rule(second))


def check_baseline(check: dict[str, Any]) -> dict[str, Any]:
    """A saved check's rule, retrievers, name and description, as an edit
    card signs them: update-check sends all four, so the save compares them
    with the check then and never replaces a change someone made after the
    card (see confirm_actions)."""
    conditions = unwrap(check.get("rule"))
    if isinstance(conditions, str):
        with contextlib.suppress(ValueError):
            conditions = json.loads(conditions)
    return {
        "rule": conditions,
        "factIds": list(check.get("factIds") or []),
        "name": check.get("name") or "",
        "description": check.get("description") or "",
    }


def same_baseline(now: dict[str, Any], signed: dict[str, Any]) -> bool:
    """Whether the check ``now`` is still the one the card signed, whatever
    the retrievers' order."""
    same_retrievers = sorted(now["factIds"]) == sorted(signed["factIds"])
    same_texts = all(now[key] == signed[key] for key in ("name", "description"))
    return same_rules(now["rule"], signed["rule"]) and same_retrievers and same_texts


def _has_condition(rule: Any) -> bool:
    """Whether the rule tests at least one fact: ``{"all": []}`` tests none."""
    if isinstance(rule, dict):
        # "condition" is a json-rules-engine reference to a named condition.
        if {"fact", "condition"} & set(rule):
            return True
        return any(_has_condition(v) for v in rule.values())
    if isinstance(rule, list):
        return any(_has_condition(v) for v in rule)
    return False


def _rule_facts(rule: Any) -> list[str]:
    """The facts a rule reads, in order, each once."""
    facts: list[str] = []
    if isinstance(rule, dict):
        if isinstance(rule.get("fact"), str):
            facts.append(rule["fact"])
        for value in rule.values():
            facts += _rule_facts(value)
    elif isinstance(rule, list):
        for value in rule:
            facts += _rule_facts(value)
    return list(dict.fromkeys(facts))


# Keys of a list-fact-schemas schema that aren't facts.
_SCHEMA_KEYS = {"id", "version", "entityFilter"}


async def fact_ids_for_rule(tools: Any, rule: Any, fact_ids: list[str]) -> list[str]:
    """``fact_ids`` and, after them, the retriever of each fact in ``rule``
    that none of them collects ("also require no open Code Scanning alerts"
    on a check that reads only repository facts). Validation, the test and
    the save all use this list, so they agree. When the fact list can't be
    read, ``fact_ids`` as they are: the validator then says what's missing."""
    facts = _rule_facts(rule)
    if not facts:
        return list(fact_ids)
    try:
        output = await call_backstage_tool(tools, LIST_FACT_SCHEMAS, {"ids": []})
    except Exception as exc:  # noqa: BLE001 — the validator says what's missing
        logger.warning("Fact schemas unavailable; keeping retrievers %s: %s", fact_ids, exc)
        return list(fact_ids)
    owners: dict[str, set[str]] = {}
    for schema in output.get("schemas") or []:
        if isinstance(schema, dict) and isinstance(schema.get("id"), str):
            owners[schema["id"]] = set(schema) - _SCHEMA_KEYS
    result = list(fact_ids)
    for fact in facts:
        if any(fact in owners.get(retriever, ()) for retriever in result):
            continue
        owner = next((r for r, collected in owners.items() if fact in collected), None)
        if owner:
            logger.info("Rule reads %s: adding retriever %s", fact, owner)
            result.append(owner)
    return result


def _text_changes(message: str, check: dict[str, Any]) -> list[dict[str, Any]]:
    """The suggested text changes after the rule, with the saved text as old.

    ``check`` is the check as get-check returned it, not the drawer's copy:
    the card must show what the save changes, also when someone else
    renamed the check since the drawer opened."""
    fence = RULE_FENCE_RE.search(message)
    suggestions = parse_suggestions(message[fence.end() :]) if fence else {}
    saved = {
        "name": check.get("name"),
        "description": check.get("description"),
    }
    changes = []
    for label, field in _TEXT_FIELDS.items():
        if label not in suggestions:
            continue
        new = suggestions[label][1]
        if new != (saved[field] or ""):
            changes.append(
                {"field": field, "label": label, "old": saved[field] or None, "new": new}
            )
    return changes


# The welcome's practice example (see check_authoring._is_example): its rule and
# the words of its Description are fixed, like the example's button.
_EXAMPLE_RULE = {"all": [{"fact": "githubBranchProtected", "operator": "equal", "value": True}]}
_EXAMPLE_NAME = "Default branch must be protected"
_EXAMPLE_DESCRIPTION = "Fails if the default branch is not protected"
_DESCRIPTION_LINE_RE = re.compile(
    r"^([ \t]*\**Description\**[ \t]*:[ \t]*).+$",
    re.IGNORECASE | re.MULTILINE,
)


def with_example_description(message: str) -> str:
    """The reply with its Description set to the example's exact words.

    Only when the reply's rule is the practice example's rule, so a different
    rule never gets this Description; any other reply is returned as it is.
    """
    fence = RULE_FENCE_RE.search(message)
    if not fence or _edited_rule(message) != _EXAMPLE_RULE:
        return message
    head = _DESCRIPTION_LINE_RE.sub(
        lambda m: m.group(1) + _EXAMPLE_DESCRIPTION,
        message[: fence.start()],
    )
    return head + message[fence.start() :]


def check_to_test(reply: str | None, scope: dict[str, Any]) -> dict[str, Any] | None:
    """dry-run-check's ``check`` for a test: the rule in ``reply`` (a drafted
    check or an edited rule that isn't saved yet), or else (``reply`` None)
    the saved check in the scope. None when it can't be read, e.g. a draft
    without a retriever."""
    if reply is not None:
        rule = _edited_rule(reply)
    else:
        try:
            rule = unwrap(json.loads(scope.get("current_rule") or ""))
        except ValueError:
            rule = None
    if not isinstance(rule, dict):
        return None
    if scope.get("check_id"):
        fields: dict[str, Any] | None = {
            "name": scope.get("check_name") or "",
            "description": scope.get("check_description") or "",
            "fact_ids": scope.get("selected_fact_ids") or [],
        }
    else:
        fields = _new_check_fields(reply or "", scope)
    if not fields or not fields["name"] or not fields["fact_ids"]:
        return None
    return {
        "name": fields["name"],
        "description": fields["description"],
        "factIds": fields["fact_ids"],
        "rule": {"conditions": rule},
    }


def example_draft(scope: dict[str, Any]) -> str | None:
    """The practice example drafted by code, in the form the model drafts a
    check, or None when no installed retriever has the example's fact."""
    (condition,) = _EXAMPLE_RULE["all"]
    retriever = next(
        (
            r.get("id")
            for r in scope.get("available_fact_retrievers") or []
            if any(f.get("name") == condition["fact"] for f in r.get("facts") or [])
        ),
        None,
    )
    if not retriever:
        return None
    rule = json.dumps({"conditions": _EXAMPLE_RULE})
    return (
        f"Name: {_EXAMPLE_NAME}\n\n"
        f"Description: {_EXAMPLE_DESCRIPTION}\n\n"
        f"Suggested Fact Retriever: {retriever}\n\n"
        "Suggested Category: Security\n\n"
        "Suggested Maturity Tier: Bronze\n\n"
        f"Rule (json-rules-engine conditions)\n```json\n{rule}\n```"
    )


def _new_check_fields(message: str, scope: dict[str, Any]) -> dict[str, Any] | None:
    """Name, description, retriever, category and tier of a drafted check."""
    fence = RULE_FENCE_RE.search(message)
    head = message[: fence.start()] if fence else message
    lines = {m.group(1).lower(): clean(m.group(2)) for m in _HEAD_RE.finditer(head)}
    name = lines.get("name")
    retriever = lines.get("fact retriever") or lines.get("suggested fact retriever")
    if not retriever or retriever.lower().startswith("no relevant"):
        selected = scope.get("selected_fact_ids") or []
        retriever = selected[0] if selected else None
    if not name or not retriever:
        return None
    tier = (lines.get("suggested maturity tier") or "").capitalize()
    return {
        "name": name,
        "description": lines.get("description") or "",
        "fact_ids": [retriever],
        "category": lines.get("suggested category") or None,
        "tier": tier if tier.lower() in _TIER_RANK else None,
    }


async def build_confirmation(
    message: str,
    scope: dict[str, Any] | None,
    *,
    user_sub: str,
    backstage_token: str | None,
) -> dict[str, Any] | None:
    """A ``confirmation_required`` event for the reply, or None.

    Only in check_authoring, with the user's Backstage token and a rule in
    the reply: an edit's Step 2, or a drafted new check.
    """
    if not scope or scope.get("case_type") != "check_authoring":
        return None
    if not backstage_token or not user_sub:
        return None
    rule = _edited_rule(message)
    if rule is None:
        return None
    # A rule that tests nothing would pass or fail every entity alike; it
    # came from a reply that wasn't a real draft (e.g. a facts question).
    if not _has_condition(rule):
        logger.info("No confirm card: the rule tests no fact")
        return None
    if scope.get("check_id"):
        return await _edit_card(message, scope, rule, user_sub, backstage_token)
    return await _new_check_card(message, scope, rule, user_sub, backstage_token)


async def card_for_save_request(
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
    *,
    user_sub: str,
    backstage_token: str | None,
    practice: bool = False,
    reply: str | None = None,
) -> dict[str, Any] | None:
    """The confirm card again, ready to save, when the user just asks to
    save or to skip the test, or None. ``practice``: the draft is the welcome's
    example, so its card has the example's Description, as before.

    Built from the previous assistant reply (the drafted check or edited
    rule the user is answering), or from ``reply`` when the caller found the
    one to save, so "save this" gets the card without a model call. The card
    skips its test step (``test_skipped``): it offers only the save buttons.
    None when the latest turn says more than that, or the reply has no rule.
    """
    if not messages or messages[-1].get("role") != "user":
        return None
    if not asks_to_save(messages[-1]):
        return None
    if reply is None:
        previous = next(
            (m for m in reversed(messages[:-1]) if m.get("role") == "assistant"),
            None,
        )
        if previous is None:
            return None
        reply = previous.get("content") or ""
    card = await build_confirmation(
        with_example_description(reply) if practice else reply,
        scope,
        user_sub=user_sub,
        backstage_token=backstage_token,
    )
    if not card:
        return None
    # The example's card says it is the example, skipped test or not.
    return {**card, "test_skipped": True, **({"example": True} if practice else {})}


async def _new_check_card(
    message: str,
    scope: dict[str, Any],
    rule: Any,
    user_sub: str,
    backstage_token: str,
) -> dict[str, Any] | None:
    fields = _new_check_fields(message, scope)
    if fields is None:
        logger.info("No confirm card for the new check: no name or fact retriever in the reply")
        return None
    conditions = rule
    try:
        tools = await backstage_tools_as_user(BearerTokenAuth(backstage_token))
        fact_ids = await fact_ids_for_rule(tools, rule, fields["fact_ids"])
        validation = await call_backstage_tool(
            tools,
            VALIDATE_CHECK,
            {"factIds": fact_ids, "rule": {"conditions": conditions}},
        )
    except Exception as exc:  # noqa: BLE001 — no card; the reply still arrives
        logger.warning("No confirm card for the new check: %s", exc)
        return None

    metadata: dict[str, Any] = {}
    if fields["category"]:
        metadata["category"] = fields["category"]
    if fields["tier"]:
        metadata["rank"] = _TIER_RANK[fields["tier"].lower()]
    action = {
        "kind": "create",
        # The new check's id: a card confirmed again, after a restart or on
        # another replica, is refused instead of saving a second check.
        "check_id": str(uuid.uuid4()),
        # create-check's input; category and tier go to the check's metadata.
        "check": {
            "name": fields["name"],
            "description": fields["description"],
            "factIds": fact_ids,
            "rule": {"conditions": conditions},
        },
        "metadata": metadata,
    }
    logger.info("Confirm card for a new check: valid=%s", validation["valid"])
    return {
        "type": "confirmation_required",
        # Only a valid rule can be saved, so only its card is signed.
        "token": sign(action, user_sub) if validation["valid"] else "",
        "kind": "create",
        "check_name": fields["name"],
        "description": fields["description"],
        "fact_ids": fact_ids,
        "category": fields["category"],
        "tier": fields["tier"],
        "applies_to": validation.get("kinds", []),
        "new_rule": json.dumps(rule, indent=2),
        "valid": validation["valid"],
        "problems": validation["problems"],
    }


# The problem an edit card shows when the saved check changed since the chat
# opened: no rule rewrite fixes it (see check_authoring.stream_turn).
CHECK_CHANGED_PROBLEM = (
    "This check changed since you opened it, so this edit could undo that change. "
    "Reopen the check and ask again."
)


def _changed_since_opened(saved: dict[str, Any], scope: dict[str, Any]) -> bool:
    """Whether the saved rule differs from the one the chat opened with (the
    rule the edit was made against)."""
    try:
        opened = unwrap(json.loads(scope.get("current_rule") or ""))
    except TypeError, ValueError:
        return False
    return not same_rules(unwrap(saved.get("rule")), opened)


async def _edit_card(
    message: str,
    scope: dict[str, Any],
    rule: Any,
    user_sub: str,
    backstage_token: str,
) -> dict[str, Any] | None:
    check_id = scope["check_id"]
    try:
        tools = await backstage_tools_as_user(BearerTokenAuth(backstage_token))
        saved = (await call_backstage_tool(tools, GET_CHECK, {"id": check_id}))["check"]
        conditions = rule
        # The saved retrievers, and any the edited rule's new facts need.
        fact_ids = await fact_ids_for_rule(tools, rule, saved["factIds"])
        # The check's id: validated with its entity filter, as the save will be.
        validation = await call_backstage_tool(
            tools,
            VALIDATE_CHECK,
            {"id": check_id, "factIds": fact_ids, "rule": {"conditions": conditions}},
        )
    except Exception as exc:  # noqa: BLE001 — no card; the reply still arrives
        logger.warning("No confirm card for check %s: %s", check_id, exc)
        return None

    if _changed_since_opened(saved, scope):
        # The edit was made against the rule the chat opened with: saving it
        # would drop what changed since. No signed card; reopen the check.
        logger.info("No signed card for check %s: it changed since the chat opened", check_id)
        validation = {"valid": False, "problems": [CHECK_CHANGED_PROBLEM]}
    text_changes = _text_changes(message, saved)
    action = {
        "kind": "update",
        "check_id": check_id,
        "status": saved["status"],
        # update-check's input as saved, with the rule changed (and the
        # retrievers its facts need); the user chooses on the card whether
        # the texts below are applied.
        "check": {
            "name": saved["name"],
            "description": saved.get("description") or "",
            "factIds": fact_ids,
            "rule": {"conditions": conditions},
        },
        "texts": {change["field"]: change["new"] for change in text_changes},
        # The check as this card saw it: the save refuses if it changed since.
        "baseline": check_baseline(saved),
    }
    logger.info(
        "Confirm card for check %s: valid=%s, %d text change(s)",
        check_id,
        validation["valid"],
        len(text_changes),
    )
    return {
        "type": "confirmation_required",
        # Only a valid rule can be saved, so only its card is signed.
        "token": sign(action, user_sub) if validation["valid"] else "",
        "kind": "update",
        "check_id": check_id,
        "check_name": saved["name"],
        "fact_ids": fact_ids,
        "status": saved["status"],
        "current_rule": json.dumps(unwrap(saved["rule"]), indent=2),
        "new_rule": json.dumps(rule, indent=2),
        "text_changes": text_changes,
        "valid": validation["valid"],
        "problems": validation["problems"],
    }
