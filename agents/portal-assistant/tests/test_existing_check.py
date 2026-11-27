# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for finding a saved check like a drafted new one."""

import json

import pytest

from src.agent import existing_check
from src.agent.a2ui import existing_check_surface
from src.agent.existing_check import (
    card_for_separate_check,
    existing_check_view,
    find_existing_check,
)
from src.clients.mcp import BackstageToolError

_RETRIEVER = "githubRepositoryFactRetriever"
_PROTECTED = {"fact": "githubDefaultBranchProtected", "operator": "equal", "value": True}
_TWO_REVIEWS = {"fact": "githubRequiredApprovals", "operator": "greaterThanInclusive", "value": 2}
_ONE_REVIEW = {**_TWO_REVIEWS, "value": 1}
_DRAFT_RULE = {"all": [_PROTECTED, _TWO_REVIEWS]}

_SCOPE = {
    "case_type": "check_authoring",
    "available_fact_retrievers": [
        {
            "id": _RETRIEVER,
            "facts": [
                {"name": "githubDefaultBranchProtected", "description": "Default branch protected"},
                {"name": "githubRequiredApprovals", "description": "Approving reviews required"},
            ],
        }
    ],
}


def _confirmation(rule=_DRAFT_RULE, name="Protected branch with 2 reviews"):
    return {
        "type": "confirmation_required",
        "token": "signed",
        "kind": "create",
        "check_name": name,
        "fact_ids": [_RETRIEVER],
        "new_rule": json.dumps(rule, indent=2),
        "valid": True,
    }


def _saved(check_id, rule, *, name=None, status="published", fact_ids=(_RETRIEVER,)):
    return {
        "id": check_id,
        "name": name or f"Check {check_id}",
        "description": f"About {check_id}",
        "factIds": list(fact_ids),
        "status": status,
        "rule": {"conditions": rule},
    }


@pytest.fixture
def backstage(monkeypatch):
    """Fake Backstage tools serving ``state["checks"]``; records each call."""
    state = {"checks": [], "calls": [], "fail": None}

    async def fake_tools(_auth):
        return {"tech-insights__list-checks": object(), "tech-insights__get-check": object()}

    async def fake_call(_tools, name, args):
        state["calls"].append((name, args))
        if state["fail"] == name:
            raise BackstageToolError(
                "You don't have the tech-insights.dynamic-check.read permission."
            )
        if name == "tech-insights__list-checks":
            return {
                "checks": [{k: v for k, v in c.items() if k != "rule"} for c in state["checks"]]
            }
        return {"check": next(c for c in state["checks"] if c["id"] == args["id"])}

    monkeypatch.setattr(existing_check, "backstage_tools_as_user", fake_tools)
    monkeypatch.setattr(existing_check, "call_backstage_tool", fake_call)
    return state


async def _find(confirmation=None, messages=(), token="issued-for-user"):
    return await find_existing_check(
        confirmation or _confirmation(),
        list(messages),
        backstage_token=token,
    )


async def test_the_same_rule_in_another_order_is_the_same_check(backstage):
    backstage["checks"] = [_saved("c1", {"all": [_TWO_REVIEWS, _PROTECTED]})]

    found = await _find()

    assert (found.match, found.reason, found.id, found.status) == (
        "same",
        "rule",
        "c1",
        "published",
    )
    assert json.loads(found.rule) == {"all": [_TWO_REVIEWS, _PROTECTED]}


async def test_the_same_facts_with_other_values_are_a_similar_check(backstage):
    backstage["checks"] = [_saved("c1", {"all": [_PROTECTED, _ONE_REVIEW]}, status="draft")]

    found = await _find()

    assert (found.match, found.reason, found.status) == ("similar", "values", "draft")


async def test_the_same_name_with_other_facts_is_a_similar_check(backstage):
    backstage["checks"] = [
        _saved(
            "c1",
            {"all": [{"fact": "githubHasCodeowners", "operator": "equal", "value": True}]},
            name="protected branch WITH 2 reviews",
            fact_ids=["otherRetriever"],
        )
    ]

    found = await _find()

    assert (found.match, found.reason) == ("similar", "name")


async def test_the_same_rule_comes_first_then_a_published_check(backstage):
    backstage["checks"] = [
        _saved("similar", {"all": [_PROTECTED, _ONE_REVIEW]}),
        _saved("same-draft", _DRAFT_RULE, status="draft"),
        _saved("same-published", _DRAFT_RULE),
    ]

    found = await _find()

    assert found.id == "same-published"


async def test_checks_on_other_facts_or_retrievers_are_not_matches(backstage):
    backstage["checks"] = [
        _saved("other-facts", {"all": [_PROTECTED]}),
        _saved("other-retriever", _DRAFT_RULE, fact_ids=["otherRetriever"]),
    ]

    assert await _find() is None
    # A check on another retriever, with another name, isn't even read.
    assert ("tech-insights__get-check", {"id": "other-retriever"}) not in backstage["calls"]


async def test_a_check_the_user_chose_not_to_edit_is_left_out(backstage):
    backstage["checks"] = [
        _saved("c1", _DRAFT_RULE, name="Branch protection"),
        _saved("c2", {"all": [_PROTECTED, _ONE_REVIEW]}, name="Reviews"),
    ]
    chat = [
        {"role": "user", "content": "Protected branch with 2 reviews"},
        {"role": "assistant", "content": "Name: …"},
        {"role": "user", "content": "Create a separate check instead of editing Branch protection"},
    ]

    found = await _find(messages=chat)

    assert found.id == "c2"


async def test_no_lookup_when_backstage_refuses(backstage):
    backstage["fail"] = "tech-insights__list-checks"

    assert await _find() is None


async def test_a_check_that_cant_be_read_is_skipped(backstage, monkeypatch):
    backstage["checks"] = [
        _saved("gone", _DRAFT_RULE),
        _saved("c2", {"all": [_PROTECTED, _ONE_REVIEW]}),
    ]

    async def fake_call(_tools, name, args):
        if name == "tech-insights__list-checks":
            return {"checks": backstage["checks"]}
        if args["id"] == "gone":
            raise BackstageToolError('No check with id "gone"; use list-checks.')
        return {"check": backstage["checks"][1]}

    monkeypatch.setattr(existing_check, "call_backstage_tool", fake_call)

    found = await _find()

    assert found.id == "c2"


@pytest.mark.parametrize(
    "confirmation, token",
    [
        ({**_confirmation(), "kind": "update"}, "issued-for-user"),
        (_confirmation(), None),
    ],
)
async def test_only_a_new_check_with_the_users_token_is_looked_up(backstage, confirmation, token):
    assert await _find(confirmation, token=token) is None
    assert backstage["calls"] == []


async def test_the_same_published_check_is_recommended_for_editing(backstage):
    backstage["checks"] = [_saved("c1", _DRAFT_RULE, name="Branch protection")]
    found = await _find()

    view = existing_check_view(found, _confirmation(), _SCOPE)

    assert view.title == "This check already exists"
    assert view.subtitle == "Branch protection · Published"
    assert "it's in every score" in view.summary
    assert view.edit_label == "Edit this check"
    # The drawer shows the check's "Edit this check" intro: nothing to send.
    assert view.edit_message is None
    assert view.separate_message == "Create a separate check instead of editing Branch protection"
    assert view.rule == [
        ("Default branch protected", "Yes"),
        ("Approving reviews required", "2 or more"),
    ]
    assert view.draft == []
    assert view.check == {
        "checkId": "c1",
        "name": "Branch protection",
        "description": "About c1",
        "rule": json.dumps(_DRAFT_RULE, separators=(",", ":")),
        "factIds": [_RETRIEVER],
        "status": "published",
    }


async def test_the_same_draft_says_it_isnt_in_any_score(backstage):
    backstage["checks"] = [_saved("c1", _DRAFT_RULE, name="Branch protection", status="draft")]
    found = await _find()

    view = existing_check_view(found, _confirmation(), _SCOPE)

    assert view.title == "This check is already a draft"
    assert "Drafts aren't in any score yet" in view.summary
    assert view.edit_label == "Edit the draft"


async def test_a_similar_check_is_changed_to_the_users_rule(backstage):
    backstage["checks"] = [_saved("c1", {"all": [_PROTECTED, _ONE_REVIEW]}, name="Reviews")]
    found = await _find()

    view = existing_check_view(found, _confirmation(), _SCOPE)

    assert view.title == "A similar check exists"
    assert view.edit_label == "Change it to my rule"
    assert view.edit_message == (
        "Change this check to my new rule (Default branch protected: Yes, "
        "Approving reviews required: 2 or more)"
    )
    assert view.rule_label == "What it checks now"
    assert view.rule[1] == ("Approving reviews required", "1 or more")
    assert view.draft[1] == ("Approving reviews required", "2 or more")


def test_the_card_has_the_check_and_both_buttons():
    found = existing_check.ExistingCheck(
        match="similar",
        reason="values",
        id="c1",
        name="Reviews",
        description="",
        fact_ids=[_RETRIEVER],
        status="draft",
        rule=json.dumps({"all": [_PROTECTED, _ONE_REVIEW]}),
    )

    surface = existing_check_surface(existing_check_view(found, _confirmation(), _SCOPE))

    root, *rest = surface["createSurface"]["components"]
    assert root["component"] == "ExistingCheckCard"
    assert root["check"]["checkId"] == "c1"
    assert root["editMessage"].startswith("Change this check to my new rule")
    by_id = {c["id"]: c for c in rest}
    assert [by_id[i]["value"] for i in root["rule"]] == ["Yes", "1 or more"]
    assert [by_id[i]["value"] for i in root["draft"]] == ["Yes", "2 or more"]
    (button,) = [by_id[i] for i in root["footer"]]
    assert button["action"]["event"]["userMessage"] == (
        "Create a separate check instead of editing Reviews"
    )


_DRAFT_REPLY = (
    "Name: Protected branch with 2 reviews\n\n```json\n" + json.dumps(_DRAFT_RULE) + "\n```"
)
_SEPARATE_CHAT = [
    {"role": "user", "content": "Protected branch with 2 reviews"},
    {"role": "assistant", "content": _DRAFT_REPLY},
    {"role": "user", "content": "Create a separate check instead of editing Branch protection"},
]


@pytest.fixture
def built(monkeypatch):
    calls = []

    async def fake_confirmation(message, _scope, **_kwargs):
        calls.append(message)
        return _confirmation()

    monkeypatch.setattr(existing_check, "build_confirmation", fake_confirmation)
    return calls


async def test_create_a_separate_check_gets_the_drafts_save_card(built):
    found = await card_for_separate_check(
        _SEPARATE_CHAT,
        _SCOPE,
        user_sub="u",
        backstage_token="b",
    )

    assert found == (_confirmation(), _DRAFT_REPLY)
    assert built == [_DRAFT_REPLY]
    # The card the draft would have had: a test first.
    assert "test_skipped" not in found[0]


@pytest.mark.parametrize(
    "chat, scope",
    [
        (_SEPARATE_CHAT[:2] + [{"role": "user", "content": "Create a separate check"}], _SCOPE),
        (_SEPARATE_CHAT[:2] + [{"role": "user", "content": "Make it 3 reviews"}], _SCOPE),
        (_SEPARATE_CHAT, {**_SCOPE, "check_id": "c1"}),
        (_SEPARATE_CHAT[2:], _SCOPE),
    ],
)
async def test_other_messages_get_no_separate_card(built, chat, scope):
    assert await card_for_separate_check(chat, scope, user_sub="u", backstage_token="b") is None
    assert built == []
