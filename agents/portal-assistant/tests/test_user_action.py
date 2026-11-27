# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for branching on the drawer button a message came from."""

import json

import pytest
from pydantic import ValidationError

from src.agent import a2ui, check_authoring, orchestrator
from src.agent.reply_view import Pick
from src.agent.rule_view import change_view, flat_conditions
from src.agent.user_action import CONTEXT_VALUE_MAX, for_model
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
    assert check_authoring._picked_choice(_clicked("confirm_edit", "yes"), _ONE) == 1


@pytest.mark.parametrize("choice", [1, 2, "2"])
def test_a_numbered_proposal_is_picked_by_its_number(choice):
    assert check_authoring._picked_choice(
        _clicked("pick_proposal", "whatever it says", choice=choice), _TWO,
    ) == int(choice)


def test_a_number_that_isnt_a_choice_picks_nothing():
    assert check_authoring._picked_choice(_clicked("pick_proposal", "3", choice=3), _TWO) is None


@pytest.mark.parametrize("name", ["reject_edit", "keep_rule", "test", "skip_test"])
def test_another_button_never_approves_even_if_its_words_would(name):
    assert check_authoring._picked_choice(_clicked(name, "yes"), _ONE) is None


def test_yes_to_two_choices_doesnt_say_which():
    assert check_authoring._picked_choice(_clicked("confirm_edit", "yes"), _TWO) is None


def test_the_test_button_names_its_entity_ref():
    message = _clicked("test", "Test it", entityRef="component:shop/order-api")

    assert check_authoring._test_entity(message) == "component:shop/order-api"


def test_a_button_that_isnt_a_test_is_never_read_as_one():
    # Its words look like a test request, but the button says otherwise.
    assert check_authoring._test_entity(_clicked("reject_edit", "test it on order-api")) is None


def test_skip_the_test_asks_to_save():
    assert check_authoring.asks_to_save(_clicked("skip_test", "Skip the test"))
    assert not check_authoring.asks_to_save(_clicked("reject_edit", "save it"))


def test_explain_this_rule_is_a_discussion_in_an_edit_chat():
    messages = [_clicked("explain_rule", "Explain this rule")]

    scope = {"case_type": "check_authoring", "check_id": "c1", "current_rule": '{"all":[]}'}

    assert check_authoring._edit_phase(messages, scope) == "discuss"


def test_try_the_example_is_the_beginners_example():
    assert check_authoring._picks_example(_clicked("try_example", "Try an example"))


# ── a typed reply stands for the question's own button ───────────────

_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "current_rule": json.dumps(
        {"all": [{"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", "value": 30}]}
    ),
}


# One proposal on its own line, as the prompt asks: shown as the change card.
_CARD_ONE = (
    "**Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60`\n\n"
    "Shall I change it to that?"
)


def _typed(content, step1, action=None):
    message = {"role": "user", "content": content}
    if action:
        message["action"] = action
    return check_authoring._answer(
        [{"role": "user", "content": "Change it to 60"}, {"role": "assistant", "content": step1},
         message],
        _SCOPE,
    )


def _sent(event):
    """What a button's click puts on the message: its name and context."""
    return {k: v for k, v in event.items() if k != "userMessage"}


def _card_events(step1):
    return _events(a2ui.change_card_surface(change_view(step1, _SCOPE), None))


@pytest.mark.parametrize("typed", ["yes", "ok, do it", "Yes please."])
def test_typed_approval_is_the_cards_own_yes(typed):
    assert _typed(typed, _CARD_ONE)["action"] == _sent(_card_events(_CARD_ONE)["yes"])


@pytest.mark.parametrize(
    ("typed", "option"), [("2", "option_2"), ("the second one", "option_2"), ("option 1", "option_1")],
)
def test_a_typed_number_is_the_cards_own_choice(typed, option):
    assert _typed(typed, _TWO)["action"] == _sent(_card_events(_TWO)[option])


def test_a_typed_one_to_a_single_proposal_is_its_yes():
    # Its card has Yes and No, not numbered choices.
    assert _typed("1", _CARD_ONE)["action"] == _sent(_card_events(_CARD_ONE)["yes"])


@pytest.mark.parametrize("typed", ["no", "keep it", "Keep the current rule"])
def test_turning_it_down_is_no(typed):
    assert _typed(typed, _ONE)["action"] == {"name": "reject_edit"}


@pytest.mark.parametrize(
    ("typed", "step1"),
    [
        ("yes but make it 70", _ONE),
        ("no, make it 90", _ONE),
        ("maybe", _ONE),
        ("hmm", _ONE),
        ("yes?", _ONE),
        ("3", _TWO),
        # "yes" to two choices doesn't say which.
        ("yes", _TWO),
        # Nothing to say yes to.
        ("yes", "What should change instead?"),
    ],
)
def test_anything_else_is_left_for_the_model(typed, step1):
    answer = _typed(typed, step1)

    assert "action" not in answer
    assert check_authoring._picked_choice(answer, step1) is None


def test_choices_shown_as_the_input_card_cant_be_picked_by_typing():
    # They differ in more than one condition: no buttons were shown for them.
    step1 = (
        "1. **Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60` AND `a` `equal` `true`\n"
        "2. **Proposed rule:** `githubDaysSinceLastPush` `lessThan` `90` AND `a` `equal` `false`"
    )

    assert "action" not in _typed("1", step1)


def test_an_approval_with_a_test_keeps_the_words_for_the_model():
    answer = _typed("1, then test it on order-api", _TWO)

    assert answer["action"] == _sent(_card_events(_TWO)["option_1"])
    assert answer["content"] == "1, then test it on order-api"


def test_the_answer_box_is_read_as_typed():
    boxed = _typed("yes", _ONE, {"name": "type_answer"})

    assert boxed["action"] == _typed("yes", _ONE)["action"]


def test_a_button_keeps_its_own_action():
    assert _typed("yes", _ONE, {"name": "reject_edit"})["action"] == {"name": "reject_edit"}


def test_typed_and_clicked_answers_decide_alike():
    yes = _card_events(_CARD_ONE)["yes"]
    clicked = _clicked(yes["name"], "yes", **yes["context"])
    history = [
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": _CARD_ONE},
    ]

    for latest in (clicked, {"role": "user", "content": "yes"}):
        messages = [*history, latest]
        assert check_authoring._edit_phase(messages, _SCOPE) == "apply"
        assert check_authoring._picked_proposal(messages, _SCOPE) == [
            ("githubDaysSinceLastPush", "lessThanInclusive", 60),
        ]


def test_a_typed_yes_to_a_proposal_code_cant_read_approves_without_a_rule():
    # Two unlabeled lines: which one is proposed can't be told, so there is
    # no rule to carry, and the approval check asks once more (see the
    # orchestrator's tests).
    step1 = (
        "I could make it `githubDaysSinceLastPush` `lessThanInclusive` `60`.\n"
        "Or `githubDaysSinceLastPush` `lessThan` `60`. Shall I?"
    )

    assert _typed("yes", step1)["action"] == {"name": "confirm_edit", "context": {}}


def test_a_typed_test_request_still_names_its_entity():
    typed = {"role": "user", "content": "test it on order-api"}

    assert check_authoring._test_entity(typed) == "order-api"


def test_a_button_that_sends_a_request_in_words_is_read_by_them():
    # A card's suggestion ("pick_chip" etc.) sends words for the model.
    chip = {"role": "user", "content": "test it on order-api", "action": {"name": "pick_chip"}}

    assert check_authoring._test_entity(chip) == "order-api"


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


def test_button_actions_count_toward_the_request_limit():
    # 50 messages, each with 8 context values of the longest allowed size.
    big = {f"k{i}": "x" * CONTEXT_VALUE_MAX for i in range(8)}
    message = {"role": "user", "content": "x", "action": {"name": "pick_proposal", "context": big}}

    with pytest.raises(ValidationError, match="exceeds limit"):
        ChatRequest(messages=[message] * 50)
    # A real button's action is still well within it.
    ChatRequest(messages=[{**message, "action": {"name": "test", "context": {"entityRef": "x"}}}])


@pytest.mark.parametrize(
    "action",
    [
        {"name": "Not-A-Name"},
        {"name": "test", "extra": 1},
        {"name": "test", "context": {"entityRef": "x" * (CONTEXT_VALUE_MAX + 1)}},
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


def test_every_step1_card_carries_its_rule():
    rule = [("githubDaysSinceLastPush", "lessThan", 60)]

    plain = _events(a2ui.edit_question_surface(rule))["yes"]
    choices = _events(
        a2ui.choice_card_surface([(1, "a"), (2, "b")], None, {}, None, {2: rule}),
    )

    assert flat_conditions(plain["context"]["rule"]) == rule
    assert flat_conditions(choices["option_2"]["context"]["rule"]) == rule
    assert "rule" not in choices["option_1"]["context"]


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
