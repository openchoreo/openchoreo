# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the confirm card built after an edit's Step 2."""

import json

import pytest
from langchain_core.messages import ToolMessage

from src.agent import confirm_card, orchestrator, pending_actions
from src.agent.confirm_card import (
    build_confirmation,
    check_to_test,
    example_draft,
    is_saved_rule,
    with_example_description,
)
from src.agent.pending_actions import consume
from src.agent.stream_events import emit
from src.clients import mcp as mcp_module
from src.clients.mcp import BackstageToolError, call_backstage_tool

_SAVED_RULE = {
    "all": [
        {"fact": "githubDefaultBranchProtected", "operator": "equal", "value": True},
        {"fact": "githubRequiresPullRequest", "operator": "equal", "value": True},
    ]
}
_NEW_RULE = {
    "all": [
        {"fact": "githubDefaultBranchProtected", "operator": "equal", "value": True},
        {"fact": "githubRequiredApprovals", "operator": "greaterThanInclusive", "value": 2},
    ]
}
_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "check_name": "Default branch protection with pull requests",
    "check_description": "Fails if the default branch is not protected.",
    "current_rule": json.dumps(_SAVED_RULE),
}
_STEP2 = (
    "Edited rule:\n\n```json\n"
    + json.dumps({"conditions": _NEW_RULE}, indent=2)
    + "\n```\n\nSuggested related changes:\n"
    "- Name: `Default branch protection with pull requests` → `Default branch protection with 2 approvals`\n"
    "- How to pass (new): `Require 2 approving reviews on the default branch.`"
)


@pytest.fixture(autouse=True)
def _backstage(monkeypatch):
    """Fake Backstage tools; records each call."""
    monkeypatch.setattr(pending_actions.settings, "portal_assistant_action_secret", "s")
    pending_actions._USED.clear()
    state = {"calls": [], "problems": [], "fail": None, "auth": None}

    async def fake_tools(auth):
        state["auth"] = auth
        return {"get_check": object(), "validate_check": object()}

    async def fake_call(_tools, name, args):
        state["calls"].append((name, args))
        if state["fail"] == name:
            raise BackstageToolError(
                "You don't have the tech-insights.dynamic-check.manage permission."
            )
        if name == "get_check":
            return {
                "check": {
                    "id": "c1",
                    "name": _SCOPE["check_name"],
                    "description": _SCOPE["check_description"],
                    "factIds": ["githubRepositoryFactRetriever"],
                    "status": "published",
                    "rule": {"conditions": _SAVED_RULE},
                }
            }
        return {"valid": not state["problems"], "problems": state["problems"]}

    monkeypatch.setattr(confirm_card, "backstage_tools_as_user", fake_tools)
    monkeypatch.setattr(confirm_card, "call_backstage_tool", fake_call)
    return state


async def _card(message=_STEP2, scope=_SCOPE, token="issued-for-user", user_sub="user-a"):
    return await build_confirmation(message, scope, user_sub=user_sub, backstage_token=token)


async def test_builds_a_signed_card_from_the_step2_reply(_backstage):
    card = await _card()

    assert card["kind"] == "update" and card["check_id"] == "c1"
    assert card["status"] == "published" and card["valid"] is True
    assert json.loads(card["current_rule"]) == _SAVED_RULE
    assert json.loads(card["new_rule"]) == _NEW_RULE
    assert card["text_changes"] == [
        {
            "field": "name",
            "label": "Name",
            "old": _SCOPE["check_name"],
            "new": "Default branch protection with 2 approvals",
        },
    ]  # The reply's "How to pass (new)" line is not offered.
    # The tools ran as the user, and the card is a valid stream event.
    assert _backstage["auth"]._token == "issued-for-user"
    assert json.loads(emit(card))["type"] == "confirmation_required"


async def test_the_token_carries_exactly_the_change_on_the_card():
    card = await _card()

    action = consume(card["token"], "user-a")

    assert action == {
        "kind": "update",
        "check_id": "c1",
        "status": "published",
        "check": {
            "name": _SCOPE["check_name"],
            "description": _SCOPE["check_description"],
            "factIds": ["githubRepositoryFactRetriever"],
            "rule": {"conditions": json.dumps(_NEW_RULE, separators=(",", ":"))},
        },
        "texts": {"name": "Default branch protection with 2 approvals"},
    }


async def test_validates_the_new_rule_with_the_saved_retrievers(_backstage):
    await _card()

    name, args = _backstage["calls"][1]
    assert name == "validate_check"
    assert args["factIds"] == ["githubRepositoryFactRetriever"]
    assert json.loads(args["rule"]["conditions"]) == _NEW_RULE


async def test_a_rule_with_problems_is_marked_invalid(_backstage):
    _backstage["problems"] = [
        "No registered fact retriever provides a fact named githubRequiredApprovals."
    ]

    card = await _card()

    assert card["valid"] is False
    assert card["problems"] == _backstage["problems"]


async def test_a_bare_rule_without_the_conditions_wrapper_works():
    message = "Edited rule:\n\n```json\n" + json.dumps(_NEW_RULE) + "\n```"

    card = await _card(message)

    assert json.loads(card["new_rule"]) == _NEW_RULE
    assert card["text_changes"] == []


@pytest.mark.parametrize(
    ("message", "scope", "token", "user_sub"),
    [
        pytest.param("Proposed rule: … Shall I change it to that?", _SCOPE, "t", "u", id="step 1"),
        pytest.param(_STEP2, {**_SCOPE, "check_id": None}, "t", "u", id="new check without a name"),
        pytest.param(_STEP2, {**_SCOPE, "case_type": "build_failure"}, "t", "u", id="other case"),
        pytest.param(_STEP2, None, "t", "u", id="no scope"),
        pytest.param(_STEP2, _SCOPE, None, "u", id="no Backstage token"),
        pytest.param(_STEP2, _SCOPE, "t", "", id="no user"),
        pytest.param("```json\n{not json\n```", _SCOPE, "t", "u", id="broken json"),
        pytest.param('```json\n{"fact":"x"}\n```', _SCOPE, "t", "u", id="not a rule"),
        pytest.param('```json\n{"all":[]}\n```', _SCOPE, "t", "u", id="empty rule"),
        pytest.param(
            '```json\n{"conditions":{"all":[{"any":[]}]}}\n```',
            _SCOPE,
            "t",
            "u",
            id="empty nested rule",
        ),
    ],
)
async def test_no_card(_backstage, message, scope, token, user_sub):
    assert await _card(message, scope, token, user_sub) is None
    assert _backstage["calls"] == []


@pytest.mark.parametrize("failing", ["get_check", "validate_check"])
async def test_no_card_when_backstage_refuses(_backstage, failing):
    _backstage["fail"] = failing

    assert await _card() is None


# ── a new check ────────────────────────────────────────────────────────

_NEW_SCOPE = {
    "case_type": "check_authoring",
    "available_fact_retrievers": [{"id": "githubRepositoryFactRetriever"}],
}
_ARCHIVED = {"all": [{"fact": "githubIsArchived", "operator": "equal", "value": False}]}
_NEW_REPLY = (
    "Name: Repo not archived\n\n"
    "Description: Fails if the component's GitHub repository has been archived\n\n"
    "Suggested Fact Retriever: githubRepositoryFactRetriever\n\n"
    "Suggested Category: Reliability\n\n"
    "Suggested Maturity Tier: Bronze\n\n"
    "Rule (json-rules-engine conditions)\n```json\n"
    + json.dumps({"conditions": _ARCHIVED}, indent=2)
    + "\n```"
)


async def test_builds_a_signed_card_for_a_new_check(_backstage):
    card = await _card(_NEW_REPLY, _NEW_SCOPE)

    assert card["kind"] == "create" and card["valid"] is True
    assert card["check_name"] == "Repo not archived"
    assert card["description"] == "Fails if the component's GitHub repository has been archived"
    assert card["fact_ids"] == ["githubRepositoryFactRetriever"]
    assert (card["category"], card["tier"]) == ("Reliability", "Bronze")
    assert json.loads(card["new_rule"]) == _ARCHIVED
    assert "check_id" not in card and "status" not in card
    assert json.loads(emit(card))["kind"] == "create"
    # Only validated: nothing to read for a check that doesn't exist yet.
    assert [name for name, _ in _backstage["calls"]] == ["validate_check"]


async def test_new_check_card_shows_the_kinds_validate_check_returns(monkeypatch):
    async def fake_call(_tools, _name, _args):
        return {"valid": True, "problems": [], "kinds": ["component"]}

    monkeypatch.setattr(confirm_card, "call_backstage_tool", fake_call)

    card = await _card(_NEW_REPLY, _NEW_SCOPE)

    assert card["applies_to"] == ["component"]
    assert json.loads(emit(card))["applies_to"] == ["component"]


async def test_the_new_check_token_carries_the_check_and_its_metadata():
    card = await _card(_NEW_REPLY, _NEW_SCOPE)

    assert consume(card["token"], "user-a") == {
        "kind": "create",
        "check": {
            "name": "Repo not archived",
            "description": "Fails if the component's GitHub repository has been archived",
            "factIds": ["githubRepositoryFactRetriever"],
            "rule": {"conditions": json.dumps(_ARCHIVED, separators=(",", ":"))},
        },
        "metadata": {"category": "Reliability", "rank": 1},
    }


async def test_bold_labels_and_a_selected_retriever_are_read():
    reply = (
        "**Name:** Repo not archived\n\n**Description:** Fails if archived\n\n"
        "**Fact Retriever:** `githubRepositoryFactRetriever`\n\n"
        "**Suggested Category:** Reliability\n\n**Suggested Maturity Tier:** Gold\n\n"
        "```json\n" + json.dumps({"conditions": _ARCHIVED}) + "\n```"
    )

    card = await _card(
        reply, {**_NEW_SCOPE, "selected_fact_ids": ["githubRepositoryFactRetriever"]}
    )

    assert card["fact_ids"] == ["githubRepositoryFactRetriever"]
    assert card["tier"] == "Gold"
    assert consume(card["token"], "user-a")["metadata"] == {"category": "Reliability", "rank": 3}


async def test_the_selected_retriever_fills_in_when_none_is_suggested():
    reply = _NEW_REPLY.replace(
        "Suggested Fact Retriever: githubRepositoryFactRetriever",
        "Suggested Fact Retriever: No relevant fact retriever available.",
    )

    card = await _card(
        reply, {**_NEW_SCOPE, "selected_fact_ids": ["githubRepositoryFactRetriever"]}
    )

    assert card["fact_ids"] == ["githubRepositoryFactRetriever"]


async def test_an_unknown_tier_is_left_out():
    card = await _card(_NEW_REPLY.replace("Bronze", "Platinum"), _NEW_SCOPE)

    assert card["tier"] is None
    assert consume(card["token"], "user-a")["metadata"] == {"category": "Reliability"}


async def test_no_new_check_card_without_a_retriever(_backstage):
    reply = _NEW_REPLY.replace(
        "Suggested Fact Retriever: githubRepositoryFactRetriever",
        "Suggested Fact Retriever: No relevant fact retriever available.",
    )

    assert await _card(reply, _NEW_SCOPE) is None
    assert _backstage["calls"] == []


async def test_no_new_check_card_for_a_rule_that_tests_no_fact(_backstage):
    """Asked for the full list of facts, the model wrote a check named "All
    available facts" with an empty rule; validate_check passed it, so the
    card offered to save it."""
    reply = _NEW_REPLY.replace(
        json.dumps({"conditions": _ARCHIVED}, indent=2),
        '{"conditions": {"all": []}}',
    ).replace("Name: Repo not archived", "Name: All available facts")

    assert await _card(reply, _NEW_SCOPE) is None
    assert _backstage["calls"] == []


async def test_a_not_rule_still_gets_a_card(_backstage):
    reply = _NEW_REPLY.replace(
        json.dumps({"conditions": _ARCHIVED}, indent=2),
        json.dumps(
            {
                "conditions": {
                    "not": {"fact": "githubIsArchived", "operator": "equal", "value": True}
                }
            }
        ),
    )

    assert (await _card(reply, _NEW_SCOPE))["kind"] == "create"


async def test_no_new_check_card_when_validation_is_refused(_backstage):
    _backstage["fail"] = "validate_check"

    assert await _card(_NEW_REPLY, _NEW_SCOPE) is None


# ── call_backstage_tool ────────────────────────────────────────────────


class _Tool:
    def __init__(self, message):
        self.message = message
        self.called_with = None

    async def ainvoke(self, call):
        self.called_with = call
        return self.message


async def test_call_returns_the_structured_output():
    tool = _Tool(
        ToolMessage(
            content=[{"type": "text", "text": '{"valid": true}'}],
            tool_call_id="x",
            artifact={"structured_content": {"valid": True, "problems": []}},
        )
    )

    output = await call_backstage_tool({"validate_check": tool}, "validate_check", {"a": 1})

    assert output == {"valid": True, "problems": []}
    assert tool.called_with["args"] == {"a": 1}


async def test_call_falls_back_to_the_text_output():
    tool = _Tool(ToolMessage(content='{"check": {"id": "c1"}}', tool_call_id="x"))

    assert await call_backstage_tool({"get_check": tool}, "get_check", {}) == {
        "check": {"id": "c1"}
    }


async def test_call_raises_backstages_message_on_an_error():
    tool = _Tool(
        ToolMessage(
            content=[{"type": "text", "text": 'No check with id "nope"; use list_checks.'}],
            tool_call_id="x",
            status="error",
        )
    )

    with pytest.raises(BackstageToolError, match="use list_checks"):
        await call_backstage_tool({"get_check": tool}, "get_check", {"id": "nope"})


async def test_call_raises_when_the_tool_is_missing():
    with pytest.raises(BackstageToolError, match="isn't available"):
        await call_backstage_tool({}, "validate_check", {})


async def test_backstage_tools_are_listed_with_the_users_token(monkeypatch):
    seen = {}

    class _FakeClient:
        def __init__(self, auth, *, backstage_auth=None, backstage="include"):
            seen.update(auth=auth, backstage_auth=backstage_auth, backstage=backstage)

        async def get_tools(self):
            return [type("T", (), {"name": "get_check"})()]

    monkeypatch.setattr(mcp_module, "MCPClient", _FakeClient)
    auth = object()

    tools = await mcp_module.backstage_tools_as_user(auth)

    assert list(tools) == ["get_check"]
    assert seen == {"auth": auth, "backstage_auth": auth, "backstage": "only"}


# ── the event in the chat stream ───────────────────────────────────────


async def test_the_card_is_sent_before_done(monkeypatch):
    async def fake_build_agent(_token, _scope, **_kwargs):
        return object(), [], {}

    async def fake_chunks(_agent, _messages, parser, **_kwargs):
        parser.set_message(_STEP2)
        return
        yield

    monkeypatch.setattr(orchestrator, "_build_agent", fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", fake_chunks)

    events = [
        json.loads(line)
        async for line in orchestrator.stream_chat(
            messages=[
                {"role": "user", "content": "Allow internal too"},
                {
                    "role": "assistant",
                    "content": 'Shall I change it to `githubVisibility in ["private","internal"]`?',
                },
                {"role": "user", "content": "yes"},
            ],
            token="t",
            user_sub="user-a",
            scope=_SCOPE,
            backstage_token="issued-for-user",
        )
    ]

    kinds = [e["type"] for e in events if e["type"] != "message_chunk"]
    # The progress step "Preparing the save card", then the card, then done.
    assert kinds == ["tool_call", "confirmation_required", "a2ui", "done"]
    _assert_same_save_card(events)
    assert events[-1]["message"] == _STEP2


# ── "save this" brings the card back ───────────────────────────────────

from src.agent.confirm_card import SAVE_REQUEST_REPLY, card_for_save_request  # noqa: E402


def _history(last_user, previous_reply=_STEP2):
    return [
        {"role": "user", "content": "require 2 approvals"},
        {"role": "assistant", "content": previous_reply},
        {"role": "user", "content": last_user},
    ]


async def _save_card(history, scope=_SCOPE, token="issued-for-user"):
    return await card_for_save_request(history, scope, user_sub="user-a", backstage_token=token)


@pytest.mark.parametrize(
    "request_text",
    [
        "save this",
        "Save it.",
        "publish it",
        "save and publish",
        "Save as draft",
        "apply",
        "please save the change",
        "yes, save it",
        "ok save it now",
        "go ahead and publish",
    ],
)
async def test_a_plain_save_request_gets_the_card_from_the_previous_reply(_backstage, request_text):
    card = await _save_card(_history(request_text))

    assert card["kind"] == "update"
    assert json.loads(card["new_rule"]) == _NEW_RULE
    assert [name for name, _ in _backstage["calls"]] == ["get_check", "validate_check"]
    # Asked to save: the card offers the save buttons, not the test first.
    assert card["test_skipped"] is True


@pytest.mark.parametrize(
    "request_text",
    [
        "Skip the test",
        "skip testing",
        "Save it without testing",
        "continue without a test",
        "no test",
    ],
)
async def test_skipping_the_test_gets_the_card_ready_to_save(_backstage, request_text):
    card = await _save_card(_history(request_text))

    assert card["kind"] == "update" and card["test_skipped"] is True


async def test_a_new_check_can_be_saved_the_same_way():
    card = await _save_card(_history("save this", _NEW_REPLY), _NEW_SCOPE)

    assert card["kind"] == "create" and card["check_name"] == "Repo not archived"


@pytest.mark.parametrize(
    "request_text",
    [
        "save it but rename it to Repo is private",
        "change it to 3 and save",
        "yes",
        "can you save checks?",
        "test it on component:default/order-api",
    ],
)
async def test_anything_more_than_a_save_request_goes_to_the_model(_backstage, request_text):
    assert await _save_card(_history(request_text)) is None
    assert _backstage["calls"] == []


async def test_no_card_when_the_previous_reply_has_no_rule(_backstage):
    step1 = (
        "**Proposed rule:** `githubRequiredApprovingReviewCount` `≥` `2` Shall I change it to that?"
    )

    assert await _save_card(_history("save this", step1)) is None
    assert _backstage["calls"] == []


async def test_no_card_without_the_users_backstage_token(_backstage):
    assert await _save_card(_history("save this"), token=None) is None


def _assert_same_save_card(events):
    """The a2ui SaveCard carries the confirm card's data and token unchanged."""
    card = next(e for e in events if e["type"] == "confirmation_required")
    ui = next(e for e in events if e["type"] == "a2ui")
    root = ui["messages"][0]["createSurface"]["components"][0]
    assert root["component"] == "SaveCard"
    assert root["card"] == {k: v for k, v in card.items() if k != "type"}


async def test_a_save_request_skips_the_model(monkeypatch):
    async def must_not_build(*_args, **_kwargs):
        raise AssertionError("the model must not run for a plain save request")

    monkeypatch.setattr(orchestrator, "_build_agent", must_not_build)

    events = [
        json.loads(line)
        async for line in orchestrator.stream_chat(
            messages=_history("save this"),
            token="t",
            user_sub="user-a",
            scope=_SCOPE,
            backstage_token="issued-for-user",
        )
    ]

    assert [e["type"] for e in events] == ["confirmation_required", "a2ui", "done"]
    assert events[2]["message"] == SAVE_REQUEST_REPLY
    _assert_same_save_card(events)


# ── V2b: a card whose rule has problems gets one fix round ─────────────

from types import SimpleNamespace  # noqa: E402

from src.agent.confirm_card import rule_fix_note  # noqa: E402

_BROKEN = _STEP2.replace("greaterThanInclusive", "atLeast")


def test_the_fix_note_lists_the_problems():
    note = rule_fix_note(["Unknown operator: atLeast (at /conditions/all/1/operator)"])

    assert note.startswith("[Automatic check — not written by the user]")
    assert "- Unknown operator: atLeast (at /conditions/all/1/operator)" in note
    assert "keep the rule's meaning" in note


class _FixingAgent:
    def __init__(self, reply=None, error=None):
        self.reply, self.error, self.calls = reply, error, []

    async def ainvoke(self, payload):
        self.calls.append(payload["messages"])
        if self.error:
            raise self.error
        return {"structured_response": SimpleNamespace(message=self.reply)}


async def _chat_with_cards(monkeypatch, agent, first_reply, cards):
    """A Step 2 turn whose confirm cards come from ``cards`` in order."""

    async def fake_build_agent(_token, _scope, **_kwargs):
        return agent, [], {}

    async def fake_chunks(_agent, _messages, parser, **_kwargs):
        parser.set_message(first_reply)
        return
        yield

    built = []

    async def fake_card(message, _scope, **_kwargs):
        built.append(message)
        return cards[len(built) - 1] if len(built) <= len(cards) else None

    monkeypatch.setattr(orchestrator, "_build_agent", fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", fake_chunks)
    monkeypatch.setattr(orchestrator, "build_confirmation", fake_card)
    events = [
        json.loads(line)
        async for line in orchestrator.stream_chat(
            messages=[
                {"role": "user", "content": "Allow internal too"},
                {
                    "role": "assistant",
                    "content": 'Shall I change it to `githubVisibility in ["private","internal"]`?',
                },
                {"role": "user", "content": "yes"},
            ],
            token="t",
            user_sub="u",
            scope=_SCOPE,
            backstage_token="issued",
        )
    ]
    return events, built


def _card_event(valid, problems=()):
    return {
        "type": "confirmation_required",
        "token": "t",
        "kind": "update",
        "check_id": "c1",
        "check_name": "n",
        "status": "draft",
        "current_rule": "{}",
        "new_rule": "{}",
        "valid": valid,
        "problems": list(problems),
    }


async def test_an_invalid_rule_is_fixed_once_and_the_card_rebuilt(monkeypatch):
    agent = _FixingAgent(reply=_STEP2)
    problem = "Unknown operator: atLeast (at /conditions/all/1/operator)"

    events, built = await _chat_with_cards(
        monkeypatch,
        agent,
        _BROKEN,
        [_card_event(False, [problem]), _card_event(True)],
    )

    assert built == [_BROKEN, _STEP2]
    card = next(e for e in events if e["type"] == "confirmation_required")
    assert card["valid"] is True
    assert events[-1]["message"] == _STEP2
    assert len(agent.calls) == 1
    assert agent.calls[0][-1]["content"].startswith("[Automatic check")
    assert problem in agent.calls[0][-1]["content"]
    assert any(e.get("tool") == "fix_rule" for e in events)


async def test_still_invalid_after_the_fix_round_shows_the_problems(monkeypatch):
    agent = _FixingAgent(reply=_BROKEN)

    events, built = await _chat_with_cards(
        monkeypatch,
        agent,
        _BROKEN,
        [_card_event(False, ["p1"]), _card_event(False, ["p2"])],
    )

    assert len(agent.calls) == 1  # one round only
    card = next(e for e in events if e["type"] == "confirmation_required")
    assert card["valid"] is False and card["problems"] == ["p2"]


async def test_a_failed_fix_round_keeps_the_first_card(monkeypatch):
    agent = _FixingAgent(error=RuntimeError("model down"))

    events, built = await _chat_with_cards(
        monkeypatch,
        agent,
        _BROKEN,
        [_card_event(False, ["p1"])],
    )

    assert built == [_BROKEN]
    card = next(e for e in events if e["type"] == "confirmation_required")
    assert card["problems"] == ["p1"]
    assert events[-1]["message"] == _BROKEN


async def test_a_valid_card_needs_no_fix_round(monkeypatch):
    agent = _FixingAgent(reply="unused")

    events, _built = await _chat_with_cards(monkeypatch, agent, _STEP2, [_card_event(True)])

    assert agent.calls == []
    assert not any(e.get("tool") == "fix_rule" for e in events)


def test_is_saved_rule_compares_the_reply_rule_with_the_saved_one():
    saved = {"current_rule": '{"conditions":{"all":[{"fact":"f","operator":"equal","value":1}]}}'}
    same = '```json\n{"all":[{"fact":"f","operator":"equal","value":1}]}\n```'
    other = '```json\n{"all":[{"fact":"f","operator":"equal","value":2}]}\n```'

    assert is_saved_rule(same, saved)
    assert not is_saved_rule(other, saved)
    assert not is_saved_rule(same, {})  # nothing saved yet


_EXAMPLE_REPLY = (
    "Name: Default branch must be protected\n\n"
    "Description: Fails if the component's evaluated branch is not protected\n\n"
    "Fact Retriever: githubRepositoryFactRetriever\n\n```json\n"
    '{"all":[{"fact":"githubBranchProtected","operator":"equal","value":true}]}\n```'
)


def test_the_example_gets_its_own_description_words():
    fixed = with_example_description(_EXAMPLE_REPLY)

    assert "Description: Fails if the default branch is not protected\n" in fixed
    assert "evaluated branch" not in fixed
    assert fixed.count("Name: Default branch must be protected") == 1
    assert fixed.endswith("```")


def test_another_rule_keeps_its_description():
    other = _EXAMPLE_REPLY.replace("githubBranchProtected", "githubIsArchived")

    assert with_example_description(other) == other
    assert with_example_description(_NEW_REPLY) == _NEW_REPLY
    assert with_example_description("Which branch do you mean?") == "Which branch do you mean?"


async def test_the_example_card_and_its_token_carry_the_fixed_description(_backstage):
    card = await _card(with_example_description(_EXAMPLE_REPLY), _NEW_SCOPE)

    assert card["description"] == "Fails if the default branch is not protected"
    assert consume(card["token"], "user-a")["check"]["description"] == card["description"]


async def test_save_it_after_the_example_keeps_its_fixed_description(_backstage):
    history = [
        {"role": "user", "content": "Try an example: the default branch must be protected"},
        {"role": "assistant", "content": _EXAMPLE_REPLY},
        {"role": "user", "content": "save it"},
    ]

    practice = await card_for_save_request(
        history,
        _NEW_SCOPE,
        user_sub="user-a",
        backstage_token="t",
        practice=True,
    )
    other = await card_for_save_request(history, _NEW_SCOPE, user_sub="user-a", backstage_token="t")

    assert practice["description"] == "Fails if the default branch is not protected"
    assert consume(practice["token"], "user-a")["check"]["description"] == practice["description"]
    assert practice["test_skipped"] is True
    assert practice["example"] is True
    # Not the example: the reply's own words stay, and it isn't marked one.
    assert "evaluated branch" in other["description"]
    assert other["test_skipped"] is True
    assert "example" not in other


_BRANCH_SCOPE = {
    "case_type": "check_authoring",
    "available_fact_retrievers": [
        {"id": "githubCodeScanningFactRetriever", "facts": [{"name": "githubOpenAlerts"}]},
        {"id": "githubRepositoryFactRetriever", "facts": [{"name": "githubBranchProtected"}]},
    ],
}


async def test_the_example_drafted_by_code_is_a_valid_new_check(_backstage):
    card = await _card(example_draft(_BRANCH_SCOPE), _BRANCH_SCOPE)

    assert card["kind"] == "create" and card["valid"] is True
    assert card["check_name"] == "Default branch must be protected"
    assert card["description"] == "Fails if the default branch is not protected"
    # The retriever that has the example's fact, not the first one.
    assert card["fact_ids"] == ["githubRepositoryFactRetriever"]
    assert (card["category"], card["tier"]) == ("Security", "Bronze")
    assert json.loads(card["new_rule"]) == {
        "all": [{"fact": "githubBranchProtected", "operator": "equal", "value": True}],
    }


def test_no_example_draft_without_its_fact():
    assert example_draft(_NEW_SCOPE) is None
    assert example_draft({"case_type": "check_authoring"}) is None


_EDIT_TEST_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "check_name": "Repo not archived",
    "check_description": "Fails if the repo is archived",
    "selected_fact_ids": ["githubRepositoryFactRetriever"],
    "current_rule": json.dumps({"conditions": _ARCHIVED}),
}


def test_the_check_to_test_is_the_unsaved_rule_or_else_the_saved_one():
    private = {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]}
    edited = "The edited rule:\n```json\n" + json.dumps(private) + "\n```"

    assert check_to_test(None, _EDIT_TEST_SCOPE) == {
        "name": "Repo not archived",
        "description": "Fails if the repo is archived",
        "factIds": ["githubRepositoryFactRetriever"],
        "rule": {"conditions": json.dumps(_ARCHIVED, separators=(",", ":"))},
    }
    # An edit that isn't saved yet: the saved check with the edited rule.
    assert check_to_test(edited, _EDIT_TEST_SCOPE)["rule"] == {
        "conditions": json.dumps(private, separators=(",", ":")),
    }
    # A new check's draft: its own fields.
    drafted = check_to_test(_NEW_REPLY, _NEW_SCOPE)
    assert (drafted["name"], drafted["factIds"]) == (
        "Repo not archived",
        ["githubRepositoryFactRetriever"],
    )
    # Nothing that can be read: the model runs the test.
    assert check_to_test(None, _NEW_SCOPE) is None
    no_retriever = {k: v for k, v in _EDIT_TEST_SCOPE.items() if k != "selected_fact_ids"}
    assert check_to_test(edited, no_retriever) is None
