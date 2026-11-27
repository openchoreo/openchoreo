# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for saving the change on a confirm card the user clicked."""

import asyncio
from types import SimpleNamespace

import pytest
from fastapi import HTTPException
from pydantic import ValidationError

from src.agent import confirm_actions, pending_actions
from src.agent.confirm_actions import ConfirmError, confirm_check_change
from src.agent.pending_actions import InvalidAction, sign
from src.api import agent_routes
from src.api.agent_routes import ConfirmRequest
from src.clients.mcp import BackstageToolError

_RULE = {"conditions": {"all": []}}
# The check as an edit card saw it; the save compares it with the check now.
_BASELINE = {
    "rule": {"all": []},
    "factIds": ["r"],
    "name": "Old name",
    "description": "Old description",
}
_EDIT = {
    "kind": "update",
    "check_id": "c1",
    "status": "draft",
    "check": {
        "name": "Old name",
        "description": "Old description",
        "factIds": ["r"],
        "rule": _RULE,
    },
    "texts": {"name": "New name", "how_to_pass": "Do this to pass."},
    "baseline": _BASELINE,
}
_NEW = {
    "kind": "create",
    "check_id": "new-id",
    "check": {"name": "Repo not archived", "description": "d", "factIds": ["r"], "rule": _RULE},
    "metadata": {"category": "Reliability", "rank": 1},
}


@pytest.fixture(autouse=True)
def backstage(monkeypatch):
    """Fake Backstage tools; records (tool, args) per call."""
    monkeypatch.setattr(pending_actions.settings, "portal_assistant_action_secret", "s")
    pending_actions._USED.clear()
    state = {"calls": [], "fail": None, "auth": None, "outputs": {}}

    async def fake_tools(auth):
        state["auth"] = auth
        return {}

    async def fake_call(_tools, name, args):
        state["calls"].append((name, args))
        if state["fail"] == name:
            raise BackstageToolError("You don't have the manage permission.")
        if name in state["outputs"]:
            return state["outputs"][name]
        if name == "tech-insights__get-check":
            return _saved_now()
        return {"id": args["id"]} if name == "tech-insights__create-check" else {}

    monkeypatch.setattr(confirm_actions, "backstage_tools_as_user", fake_tools)
    monkeypatch.setattr(confirm_actions, "call_backstage_tool", fake_call)
    return state


async def _confirm(action, status, *, apply_texts=True, user="user-a", token="issued"):
    return await confirm_check_change(
        sign(action, "user-a"),
        user_sub=user,
        backstage_token=token,
        status=status,
        apply_texts=apply_texts,
    )


def _saves(state):
    """The calls that change a check; an edit first reads it (see the guard below)."""
    return [(name, args) for name, args in state["calls"] if name != "tech-insights__get-check"]


def _names(state):
    return [name for name, _ in _saves(state)]


def _all_names(state):
    return [name for name, _ in state["calls"]]


# ── an edit ────────────────────────────────────────────────────────────


async def test_save_as_draft_applies_the_suggested_texts(backstage):
    result = await _confirm(_EDIT, "draft")

    assert result == {"check_id": "c1", "status": "draft", "message": "Saved as a draft."}
    assert _saves(backstage) == [
        (
            "tech-insights__update-check",
            {
                "id": "c1",
                "name": "New name",
                "description": "Old description",
                "factIds": ["r"],
                "rule": _RULE,
                # A how_to_pass text is ignored: the saved How to pass is kept.
                "metadata": {"category": "", "rank": 0, "solution": ""},
            },
        )
    ]
    assert backstage["auth"]._token == "issued"


async def test_without_the_texts_only_the_rule_changes(backstage):
    await _confirm(_EDIT, "draft", apply_texts=False)

    args = _saves(backstage)[0][1]
    assert (args["name"], args["description"]) == ("Old name", "Old description")
    assert args["metadata"] == {"category": "", "rank": 0, "solution": ""}


async def test_save_and_publish_a_draft(backstage):
    result = await _confirm(_EDIT, "published")

    assert _names(backstage) == ["tech-insights__update-check", "tech-insights__publish-check"]
    assert _saves(backstage)[1][1] == {"id": "c1"}
    assert result["status"] == "published" and result["message"] == "Saved and published."


async def test_keep_a_published_check_published(backstage):
    result = await _confirm({**_EDIT, "status": "published"}, "published")

    assert _names(backstage) == ["tech-insights__update-check"]
    assert result["message"] == "Saved as published."


async def test_save_a_published_check_as_a_draft(backstage):
    result = await _confirm({**_EDIT, "status": "published"}, "draft")

    assert _names(backstage) == ["tech-insights__update-check", "tech-insights__unpublish-check"]
    assert result["status"] == "draft"


async def test_a_check_published_since_the_card_is_reported_as_published(backstage):
    # The card was built for a draft; someone published the check before the click.
    backstage["outputs"]["tech-insights__update-check"] = {"id": "c1", "name": "New name", "status": "published"}

    result = await _confirm(_EDIT, "published")

    assert _names(backstage) == ["tech-insights__update-check"]
    assert result == {"check_id": "c1", "status": "published", "message": "Saved as published."}


# ── a new check ────────────────────────────────────────────────────────


async def test_create_a_draft_with_its_category_and_tier(backstage):
    result = await _confirm(_NEW, "draft")

    assert backstage["calls"] == [
        (
            "tech-insights__create-check",
            {
                **_NEW["check"],
                # The card's id: a repeat of this create is refused, not saved twice.
                "id": "new-id",
                "metadata": {"category": "Reliability", "rank": 1, "solution": ""},
            },
        )
    ]
    assert result == {"check_id": "new-id", "status": "draft", "message": "Created as a draft."}


async def test_create_and_publish_uses_the_new_id(backstage):
    result = await _confirm(_NEW, "published")

    assert _names(backstage) == ["tech-insights__create-check", "tech-insights__publish-check"]
    assert backstage["calls"][1][1] == {"id": "new-id"}
    assert result["message"] == "Created and published."


async def test_a_card_confirmed_again_after_a_restart_saves_no_second_check(backstage):
    token = sign(_NEW, "user-a")
    await confirm_check_change(
        token, user_sub="user-a", backstage_token="t", status="draft", apply_texts=True
    )
    pending_actions._USED.clear()  # a restart, or another replica
    # Backstage refuses an id it already has.
    backstage["fail"] = "tech-insights__create-check"

    with pytest.raises(ConfirmError, match="wasn't saved"):
        await confirm_check_change(
            token, user_sub="user-a", backstage_token="t", status="draft", apply_texts=True
        )
    assert [args["id"] for _, args in _saves(backstage)] == ["new-id", "new-id"]


# ── failures ───────────────────────────────────────────────────────────


async def test_a_failed_publish_keeps_the_save_and_says_so(backstage):
    backstage["fail"] = "tech-insights__publish-check"

    result = await _confirm(_NEW, "published")

    assert result["status"] == "draft"
    assert result["message"] == (
        "Created as a draft, but publishing failed: You don't have the manage permission."
    )


def _publish_times_out(monkeypatch, read_back):
    """A publish whose reply never comes; ``read_back`` answers get-check."""
    calls = []

    async def fake_call(_tools, name, args):
        calls.append(name)
        if name == "tech-insights__publish-check":
            raise TimeoutError("read timed out")
        if name == "tech-insights__get-check":
            return read_back()
        return {"id": args["id"]} if name == "tech-insights__create-check" else {}

    monkeypatch.setattr(confirm_actions, "call_backstage_tool", fake_call)
    return calls


async def test_a_publish_with_no_clear_answer_reports_the_status_read_back(monkeypatch):
    # It went through, but the reply timed out: the check is published.
    calls = _publish_times_out(monkeypatch, lambda: {"check": {"status": "published"}})

    result = await _confirm(_NEW, "published")

    assert result == {"check_id": "new-id", "status": "published", "message": "Created as published."}
    assert calls[-1] == "tech-insights__get-check"


async def test_a_publish_with_no_clear_answer_and_no_read_says_so(monkeypatch):
    def _unreachable():
        raise TimeoutError("read timed out")

    _publish_times_out(monkeypatch, _unreachable)

    result = await _confirm(_NEW, "published")

    assert result["message"] == (
        "Created, but Backstage didn't say whether publishing worked. "
        "Look at the check's status in the checks list."
    )


async def test_a_refused_save_saves_nothing(backstage):
    backstage["fail"] = "tech-insights__update-check"

    with pytest.raises(ConfirmError, match="wasn't saved: You don't have"):
        await _confirm(_EDIT, "published")
    assert _names(backstage) == ["tech-insights__update-check"]


async def test_after_a_failed_save_the_card_can_be_tried_again(backstage):
    token = sign(_EDIT, "user-a")
    backstage["fail"] = "tech-insights__update-check"
    with pytest.raises(ConfirmError, match="wasn't saved"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )

    backstage["fail"] = None
    result = await confirm_check_change(
        token,
        user_sub="user-a",
        backstage_token="t",
        status="draft",
        apply_texts=True,
    )

    assert result["message"] == "Saved as a draft."
    assert _names(backstage) == ["tech-insights__update-check", "tech-insights__update-check"]


async def test_when_backstage_cant_be_reached_the_card_can_be_tried_again(
    backstage,
    monkeypatch,
):
    token = sign(_NEW, "user-a")

    async def unreachable(_auth):
        raise ConnectionError("refused")

    monkeypatch.setattr(confirm_actions, "backstage_tools_as_user", unreachable)
    with pytest.raises(ConfirmError, match="wasn't saved: Backstage couldn't be reached"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    assert backstage["calls"] == []

    pending_actions.consume(token, "user-a")  # still usable


@pytest.mark.parametrize("output", [{}, {"name": "no id"}])
async def test_a_create_with_no_clear_answer_uses_up_the_card(backstage, output):
    """The check may be saved: a retry of the same card could create it twice."""
    token = sign(_NEW, "user-a")
    backstage["outputs"]["tech-insights__create-check"] = output

    with pytest.raises(ConfirmError, match="didn't say whether the check was saved"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    with pytest.raises(InvalidAction, match="already confirmed"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    assert _names(backstage) == ["tech-insights__create-check"]


async def test_after_a_failed_publish_the_saved_card_is_used_up(backstage):
    token = sign(_NEW, "user-a")
    backstage["fail"] = "tech-insights__publish-check"
    await confirm_check_change(
        token,
        user_sub="user-a",
        backstage_token="t",
        status="published",
        apply_texts=True,
    )

    with pytest.raises(InvalidAction, match="already confirmed"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="published",
            apply_texts=True,
        )
    assert _names(backstage) == ["tech-insights__create-check", "tech-insights__publish-check"]


async def test_without_a_backstage_token_the_card_can_be_tried_again(backstage):
    token = sign(_EDIT, "user-a")

    with pytest.raises(ConfirmError, match="sign-in"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token=None,
            status="draft",
            apply_texts=True,
        )
    assert backstage["calls"] == []

    result = await confirm_check_change(
        token,
        user_sub="user-a",
        backstage_token="issued",
        status="draft",
        apply_texts=True,
    )
    assert result["status"] == "draft"


async def test_a_card_is_saved_only_once(backstage):
    token = sign(_EDIT, "user-a")
    await confirm_check_change(
        token,
        user_sub="user-a",
        backstage_token="t",
        status="draft",
        apply_texts=True,
    )

    with pytest.raises(InvalidAction, match="already confirmed"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    assert _names(backstage) == ["tech-insights__update-check"]


async def test_another_user_cannot_save_the_card(backstage):
    with pytest.raises(InvalidAction, match="another user"):
        await _confirm(_EDIT, "draft", user="user-b")
    assert backstage["calls"] == []


# ── the route ──────────────────────────────────────────────────────────


def _post(body, headers, *, user_sub="user-a"):
    http_request = SimpleNamespace(state=SimpleNamespace(user_sub=user_sub), headers=headers)
    return asyncio.run(agent_routes.confirm(ConfirmRequest(**body), http_request, None, None))


def test_the_route_saves_with_the_users_backstage_token(backstage):
    body = {"token": sign(_EDIT, "user-a"), "status": "draft", "applyTexts": False}

    result = _post(body, {"x-backstage-user-token": "issued"})

    assert result["message"] == "Saved as a draft."
    assert backstage["auth"]._token == "issued"
    assert _saves(backstage)[0][1]["name"] == "Old name"


def test_the_route_answers_400_with_the_reason():
    body = {"token": "forged.token", "status": "draft"}

    with pytest.raises(HTTPException) as caught:
        _post(body, {"x-backstage-user-token": "issued"})

    assert caught.value.status_code == 400
    assert caught.value.detail == {
        "error": "CONFIRM_FAILED",
        "message": "This confirmation isn't valid. Ask the assistant again.",
    }


@pytest.mark.parametrize(
    "body",
    [
        {"token": "t", "status": "deleted"},
        {"token": "t", "status": "draft", "checkId": "someone-elses"},
        {"status": "draft"},
    ],
)
def test_the_request_accepts_only_the_cards_choices(body):
    with pytest.raises(ValidationError):
        ConfirmRequest(**body)


# ── an edit card made from a check that changed since ──────────────────

_GUARDED_EDIT = _EDIT


def _saved_now(rule=None, fact_ids=None, name="Old name", description="Old description"):
    return {
        "check": {
            "id": "c1",
            "status": "draft",
            "name": name,
            "description": description,
            "factIds": fact_ids or ["r"],
            "rule": {"conditions": rule if rule is not None else {"all": []}},
        }
    }


async def test_an_edit_saves_when_the_check_is_as_the_card_saw_it(backstage):
    backstage["outputs"]["tech-insights__get-check"] = _saved_now()

    result = await _confirm(_GUARDED_EDIT, "draft")

    assert result["message"] == "Saved as a draft."
    assert _all_names(backstage) == ["tech-insights__get-check", "tech-insights__update-check"]


@pytest.mark.parametrize(
    "now",
    [
        # Someone else changed the rule, or the retrievers, since the card.
        _saved_now(rule={"all": [{"fact": "x", "operator": "equal", "value": 1}]}),
        _saved_now(fact_ids=["r", "other"]),
        # Or renamed it, or changed its description: update-check sends the
        # card's texts, so saving would put the old ones back.
        _saved_now(name="Renamed by someone else"),
        _saved_now(description="Changed by someone else"),
    ],
)
async def test_an_edit_of_a_check_that_changed_since_saves_nothing(backstage, now):
    # Saving would replace the other change with the card's rule.
    backstage["outputs"]["tech-insights__get-check"] = now
    token = sign(_GUARDED_EDIT, "user-a")

    with pytest.raises(ConfirmError, match="changed since this card was made"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    assert _all_names(backstage) == ["tech-insights__get-check"]
    # The card can't become right again: it stays used.
    with pytest.raises(InvalidAction, match="already confirmed"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )


async def test_the_comparison_ignores_retriever_order_and_rule_encoding(backstage):
    # Backstage may return the conditions as a JSON string, the retrievers in any order.
    action = {**_EDIT, "baseline": {**_BASELINE, "factIds": ["a", "b"]}}
    backstage["outputs"]["tech-insights__get-check"] = _saved_now(rule='{"all": []}', fact_ids=["b", "a"])

    result = await _confirm(action, "draft")

    assert result["message"] == "Saved as a draft."


async def test_when_the_check_cant_be_read_nothing_is_saved_and_the_card_can_retry(backstage):
    backstage["fail"] = "tech-insights__get-check"
    token = sign(_GUARDED_EDIT, "user-a")

    with pytest.raises(ConfirmError, match="wasn't saved"):
        await confirm_check_change(
            token,
            user_sub="user-a",
            backstage_token="t",
            status="draft",
            apply_texts=True,
        )
    assert _all_names(backstage) == ["tech-insights__get-check"]

    backstage["fail"] = None
    backstage["outputs"]["tech-insights__get-check"] = _saved_now()
    result = await confirm_check_change(
        token,
        user_sub="user-a",
        backstage_token="t",
        status="draft",
        apply_texts=True,
    )
    assert result["message"] == "Saved as a draft."
