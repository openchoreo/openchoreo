# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for branching on the drawer button a message came from."""

import pytest
from pydantic import ValidationError

from src.agent import a2ui, orchestrator
from src.agent.reply_view import Pick
from src.agent.user_action import for_model
from src.api.agent_routes import ChatMessage, ChatRequest

_ONE = "Proposed rule: `githubDaysSinceLastPush lessThanInclusive 60`. Shall I change it to that?"
_TWO = (
    "1. **Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60` — 60 passes\n"
    "2. **Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60` — 60 fails\n\n"
    "Reply with 1 or 2."
)


def _clicked(name, content="", **context):
    action = {"name": name, "context": context} if context else {"name": name}
    return {"role": "user", "content": content, "action": action}


# ── a decision button decides by its action, never by its words ──────


def test_yes_approves_the_one_proposal():
    assert orchestrator._picked_choice(_clicked("confirm_edit", "yes"), _ONE) == 1


@pytest.mark.parametrize("choice", [1, 2, "2"])
def test_a_numbered_proposal_is_picked_by_its_number(choice):
    assert orchestrator._picked_choice(
        _clicked("pick_proposal", "whatever it says", choice=choice), _TWO,
    ) == int(choice)


def test_a_number_that_isnt_a_choice_picks_nothing():
    assert orchestrator._picked_choice(_clicked("pick_proposal", "3", choice=3), _TWO) is None


@pytest.mark.parametrize("name", ["reject_edit", "keep_rule", "test", "skip_test"])
def test_another_button_never_approves_even_if_its_words_would(name):
    assert orchestrator._picked_choice(_clicked(name, "yes"), _ONE) is None


def test_yes_to_two_choices_doesnt_say_which():
    assert orchestrator._picked_choice(_clicked("confirm_edit", "yes"), _TWO) is None


def test_the_test_button_names_its_entity_ref():
    message = _clicked("test", "Test it", entityRef="component:shop/order-api")

    assert orchestrator._test_entity(message) == "component:shop/order-api"


def test_a_button_that_isnt_a_test_is_never_read_as_one():
    # Its words look like a test request, but the button says otherwise.
    assert orchestrator._test_entity(_clicked("reject_edit", "test it on order-api")) is None


def test_skip_the_test_asks_to_save():
    assert orchestrator.asks_to_save(_clicked("skip_test", "Skip the test"))
    assert not orchestrator.asks_to_save(_clicked("reject_edit", "save it"))


def test_explain_this_rule_is_a_discussion_in_an_edit_chat():
    messages = [_clicked("explain_rule", "Explain this rule")]

    scope = {"case_type": "check_authoring", "check_id": "c1", "current_rule": '{"all":[]}'}

    assert orchestrator._edit_phase(messages, scope) == "discuss"


def test_try_the_example_is_the_beginners_example():
    assert orchestrator._picks_example(_clicked("try_example", "Try an example"))


# ── typed messages are read by their words, as before ────────────────


@pytest.mark.parametrize(
    ("typed", "picked"), [("yes", 1), ("ok, do it", 1), ("yes but make it 70", None)],
)
def test_typed_words_still_approve(typed, picked):
    assert orchestrator._picked_choice({"role": "user", "content": typed}, _ONE) == picked


def test_a_typed_test_request_still_names_its_entity():
    typed = {"role": "user", "content": "test it on order-api"}

    assert orchestrator._test_entity(typed) == "order-api"


def test_the_answer_box_is_typed_text():
    typed = {"role": "user", "content": "yes", "action": {"name": "type_answer"}}

    assert orchestrator._picked_choice(typed, _ONE) == 1


def test_a_button_that_sends_a_request_in_words_is_read_by_them():
    # A card's suggestion ("pick_chip" etc.) sends words for the model.
    chip = {"role": "user", "content": "test it on order-api", "action": {"name": "pick_chip"}}

    assert orchestrator._test_entity(chip) == "order-api"


# ── the model never sees the action ──────────────────────────────────


def test_the_model_gets_only_role_and_content():
    assert for_model([_clicked("confirm_edit", "yes")]) == [{"role": "user", "content": "yes"}]


async def test_the_messages_sent_to_the_model_have_no_action(monkeypatch):
    seen = {}

    async def _fake_build_agent(*_args, **_kwargs):
        return object(), [], {}

    async def _fake_chunks(_agent, messages, parser, **_kwargs):
        seen["messages"] = messages
        parser.set_message("Sure.")
        return
        yield  # makes this an async generator

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _fake_chunks)
    _ = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=[_clicked("pick_chip", "Show me the facts")],
            token="t",
            user_sub="u",
            scope={"case_type": "check_authoring"},
        )
    ]

    assert all(set(m) == {"role", "content"} for m in seen["messages"])


# ── the request ──────────────────────────────────────────────────────


def test_a_message_takes_a_buttons_action():
    message = ChatMessage(
        role="user",
        content="Test it",
        action={"name": "test", "context": {"entityRef": "component:default/x"}},
    )

    assert message.model_dump(exclude_none=True)["action"]["context"] == {
        "entityRef": "component:default/x",
    }
    # A typed message carries none.
    assert "action" not in ChatMessage(role="user", content="hi").model_dump(exclude_none=True)


@pytest.mark.parametrize(
    "action",
    [
        {"name": "Not-A-Name"},
        {"name": "test", "extra": 1},
        {"name": "test", "context": {"entityRef": "x" * 600}},
        {"name": "test", "context": {f"k{i}": "v" for i in range(9)}},
    ],
)
def test_the_request_refuses_a_bad_action(action):
    with pytest.raises(ValidationError):
        ChatRequest(messages=[{"role": "user", "content": "x", "action": action}])


# ── the cards' buttons carry their action ────────────────────────────


def _events(surface):
    return {
        c["id"]: c["action"]["event"]
        for c in surface["createSurface"]["components"]
        if "action" in c
    }


def test_numbered_proposals_send_their_number_as_context():
    surface = a2ui.choice_card_surface([(1, "60 passes"), (2, "60 fails")], None, {}, None)

    events = _events(surface)

    assert events["option_2"] == {
        "name": "pick_proposal", "userMessage": "2", "context": {"choice": 2},
    }


def test_a_welcomes_fixed_choice_is_its_decision_button():
    card = {"title": "Edit this check", "summary": "What would you like to do?"}

    surface = a2ui.reply_card_surface(
        card, None, ["Explain this rule"], {"Explain this rule": "explain_rule"},
    )

    assert _events(surface)["choice_0"]["name"] == "explain_rule"


def test_a_models_choice_still_sends_its_words():
    card = {
        "title": "Ideas",
        "summary": "Which one?",
        "choices": [{"label": "Add a README check", "message": "Add a README check"}],
    }

    surface = a2ui.reply_card_surface(card, None)

    assert _events(surface)["choice_0"] == {
        "name": "pick_choice", "userMessage": "Add a README check",
    }


def test_a_pick_without_an_action_keeps_its_kind():
    assert a2ui._pick("PickChip", 0, Pick("a", "a"), "chip")["action"]["event"]["name"] == (
        "pick_chip"
    )
