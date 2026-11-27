# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for src.agent.orchestrator helpers.

``_stamp_current_time`` hands the model a trustworthy "now" by prefixing the
latest *user* turn with the server clock. These tests pin the contract: it
finds the latest user turn wherever it sits (not just the final message),
leaves later turns untouched, and never mutates the caller's input.
"""
import copy
import json
from datetime import UTC, datetime
from types import SimpleNamespace

import pytest

from src.agent import confirm_card, existing_check, orchestrator
from src.agent.dry_run_view import DryRun
from src.agent.orchestrator import _stamp_current_time
from src.agent.rule_view import flat_conditions
from src.agent.stream_parser import ChatResponseParser
from src.clients.mcp import BackstageToolError


def _evaluated(values):
    """A dry-run's conditions as Backstage returns them: each fact's value as factResult."""
    return {"all": [{"fact": fact, "factResult": value} for fact, value in values.items()]}

_FIXED = datetime(2026, 7, 14, 12, 0, 0, tzinfo=UTC)
_PREFIX = "[current server time: 2026-07-14T12:00:00+00:00]\n"


@pytest.fixture(autouse=True)
def _freeze_clock(monkeypatch):
    """Pin the server clock so the stamped prefix is deterministic."""
    monkeypatch.setattr(orchestrator, "get_current_utc", lambda: _FIXED)


@pytest.fixture(autouse=True)
def _no_saved_check_like_the_draft(monkeypatch):
    """No saved check like a drafted new one, unless a test says so: the
    lookup would call Backstage."""
    async def _none(*_args, **_kwargs):
        return None

    monkeypatch.setattr(orchestrator, "find_existing_check", _none)


def test_empty_input_returns_unchanged():
    assert _stamp_current_time([]) == []


def test_stamps_trailing_user_turn():
    out = _stamp_current_time([{"role": "user", "content": "is it fixed?"}])
    assert out == [{"role": "user", "content": f"{_PREFIX}is it fixed?"}]


def test_stamps_latest_user_turn_when_assistant_trails():
    # Latest user turn is "second" (index 1); the trailing assistant turn must
    # NOT suppress the stamp, and both other turns stay byte-for-byte identical.
    msgs = [
        {"role": "user", "content": "first"},
        {"role": "user", "content": "second"},
        {"role": "assistant", "content": "answer"},
    ]
    assert _stamp_current_time(msgs) == [
        {"role": "user", "content": "first"},
        {"role": "user", "content": f"{_PREFIX}second"},
        {"role": "assistant", "content": "answer"},
    ]


def test_no_user_turn_returns_unchanged():
    msgs = [{"role": "assistant", "content": "hi"}]
    assert _stamp_current_time(msgs) == msgs


def test_does_not_mutate_input():
    # Trailing assistant turn ensures the stamped user turn is not the last
    # element, exercising the copy-not-mutate path on an interior message.
    msgs = [
        {"role": "user", "content": "second"},
        {"role": "assistant", "content": "answer"},
    ]
    before = copy.deepcopy(msgs)
    _stamp_current_time(msgs)
    assert msgs == before


async def test_recovery_fallback_receives_stamped_user_turn(monkeypatch):
    """On a recursion-limit failure the fallback must get the SAME stamped
    "now" anchor as the primary path, not the raw unstamped messages."""
    captured: dict[str, object] = {}

    async def _fake_build_agent(token, scope, *, user_sub="", backstage_token=None, edit_phase=None):
        return object(), [], {}

    async def _boom(agent, messages, parser, **_kwargs):
        raise RuntimeError("Recursion limit of 50 reached")
        yield  # unreachable; makes this an async generator

    class _Recovery:
        text = "recovered answer"
        source = "model"  # DoneEvent.recovery is Literal["model", "stub"]

    async def _fake_recover(*, messages, scope):
        captured["messages"] = messages
        return _Recovery()

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _boom)
    monkeypatch.setattr(orchestrator, "recover_with_fallback", _fake_recover)

    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": "is it fixed?"}],
            token="t",
            user_sub="u",
            scope=None,
        )
    ]

    # The fallback received the stamped latest user turn — same anchor the
    # primary streaming path would have used.
    assert captured["messages"][-1]["content"] == f"{_PREFIX}is it fixed?"
    # And the recovered answer still reached the client.
    assert any("recovered answer" in ev for ev in events)


# ── Confirm card only for an approved edit ──────────────────────────

_EDIT_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "current_rule": '{"all":[{"fact":"githubDaysSinceLastPush","operator":"lessThanInclusive","value":30}]}',
}
_STEP1 = "Proposed rule: `githubDaysSinceLastPush lessThanInclusive 60`. Shall I change it to that?"
_STEP2 = (
    "The edited rule:\n```json\n"
    '{"all":[{"fact":"githubDaysSinceLastPush","operator":"lessThanInclusive","value":60}]}\n```'
)
_FIRST = [{"role": "user", "content": "Change it to 60 and test it on order-api"}]
_CARD = {
    "type": "confirmation_required",
    "token": "signed",
    "kind": "update",
    "check_name": "Repository must have a recent push",
    "new_rule": "{}",
    "valid": True,
}


async def _turn(
    monkeypatch, messages, reply, scope=_EDIT_SCOPE, agent=None, seen=None, card=None,
    dry_run=None, confirmation=None,
):
    """Runs stream_chat with a model that replies ``reply``; returns (events, card calls).

    ``seen`` gets the edit phase the agent was built with and the messages the
    model was sent. ``confirmation`` builds the save card instead of the fake
    one that returns _CARD for any chat (e.g. the real build_confirmation).
    """
    built = []
    seen = {} if seen is None else seen

    async def _fake_build_agent(_token, _scope, *, edit_phase=None, **_kwargs):
        seen["edit_phase"] = edit_phase
        return agent or object(), [], {}

    async def _fake_chunks(_agent, messages, parser, **_kwargs):
        seen["messages"] = messages
        if dry_run is not None:
            _kwargs["dry_runs"].append(dry_run)
        if card is None:
            parser.set_message(reply)
        else:
            parser.push(json.dumps({"message": reply, "card": card}))
        return
        yield  # makes this an async generator

    async def _fake_confirmation(message, _scope, **_kwargs):
        built.append(message)
        return _CARD if confirmation is None else await confirmation(message, _scope, **_kwargs)

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _fake_chunks)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=messages, token="t", user_sub="u", scope=scope, backstage_token="b",
        )
    ]
    return events, built


def _has_card(events):
    return any('"confirmation_required"' in ev for ev in events)


async def test_no_card_on_the_first_message_of_an_edit_even_with_a_rule_block(monkeypatch):
    # The model skipped Step 1 and wrote the edited rule.
    events, built = await _turn(
        monkeypatch, [{"role": "user", "content": "Change it to 60 and test it on order-api"}], _STEP2,
    )

    assert built == []
    assert not _has_card(events)
    assert '"done"' in events[-1]  # the reply itself still arrives


async def test_an_approved_step1_gets_the_card(monkeypatch):
    events, built = await _turn(
        monkeypatch,
        [
            {"role": "user", "content": "Change it to 60 and test it on order-api"},
            {"role": "assistant", "content": _STEP1},
            {"role": "user", "content": "yes"},
        ],
        _STEP2,
    )

    assert built == [_STEP2]
    assert _has_card(events)


async def test_a_new_request_after_step1_gets_no_card(monkeypatch):
    events, built = await _turn(
        monkeypatch,
        [
            {"role": "user", "content": "Change it to 60"},
            {"role": "assistant", "content": _STEP1},
            {"role": "user", "content": "make it 90 instead"},
        ],
        _STEP2,
    )

    assert built == []
    assert not _has_card(events)


async def test_a_new_check_still_gets_its_card_on_the_first_message(monkeypatch):
    events, built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "Repositories must be private"}],
        _STEP2,
        scope={"case_type": "check_authoring"},
    )

    assert built == [_STEP2]
    assert _has_card(events)


async def test_save_this_still_returns_the_card_without_a_model_call(monkeypatch):
    from src.agent import confirm_card

    async def _fake_confirmation(message, _scope, **_kwargs):
        return _CARD if "```" in message else None

    async def _no_agent(*_args, **_kwargs):
        raise AssertionError("save this must not call the model")

    monkeypatch.setattr(confirm_card, "build_confirmation", _fake_confirmation)
    monkeypatch.setattr(orchestrator, "_build_agent", _no_agent)
    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=[
                {"role": "user", "content": "Change it to 60"},
                {"role": "assistant", "content": _STEP1},
                {"role": "user", "content": "yes"},
                {"role": "assistant", "content": _STEP2},
                {"role": "user", "content": "save it"},
            ],
            token="t", user_sub="u", scope=_EDIT_SCOPE, backstage_token="b",
        )
    ]

    assert _has_card(events)


_TESTED = "Tested on component:default/order-api\n\nOrder-api doesn't meet this rule."


async def _save_it_after(monkeypatch, history, scope):
    """Runs "save it" after ``history`` with no model; returns (events, rebuilt replies)."""
    built = []

    async def _fake_confirmation(message, _scope, **_kwargs):
        built.append(message)
        return _CARD if "```" in message else None

    async def _no_agent(*_args, **_kwargs):
        raise AssertionError("save it must not call the model")

    monkeypatch.setattr(confirm_card, "build_confirmation", _fake_confirmation)
    monkeypatch.setattr(orchestrator, "_build_agent", _no_agent)
    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=[*history, {"role": "user", "content": "save it"}],
            token="t",
            user_sub="u",
            scope=scope,
            backstage_token="b",
        )
    ]
    return events, built


@pytest.mark.parametrize(
    ("history", "scope"),
    [
        pytest.param(
            [
                {"role": "user", "content": "Repositories must be private"},
                {"role": "assistant", "content": _STEP2},
            ],
            {"case_type": "check_authoring"},
            id="a drafted check",
        ),
        pytest.param(
            [
                {"role": "user", "content": "Change it to 60"},
                {"role": "assistant", "content": _STEP1},
                {"role": "user", "content": "yes"},
                {"role": "assistant", "content": _STEP2},
            ],
            _EDIT_SCOPE,
            id="an approved edit",
        ),
    ],
)
async def test_save_it_after_a_test_result_saves_the_rule_before_it(monkeypatch, history, scope):
    # CodeRabbit: a test of another rule ends in its result alone, with no rule
    # in the reply, so "save it" found nothing to save.
    tested = [
        *history,
        {"role": "user", "content": "test it on order-api"},
        {"role": "assistant", "content": _TESTED},
    ]

    events, built = await _save_it_after(monkeypatch, tested, scope)

    assert built == [_STEP2]
    assert _has_card(events)
    assert json.loads(events[-1])["message"] == confirm_card.SAVE_REQUEST_REPLY


async def test_save_it_after_the_real_result_of_a_mismatched_test(monkeypatch):
    # CodeRabbit: both turns together, with the reply the test turn wrote.
    other = {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "internal"}]}
    tested = await _ended_with_test(monkeypatch, other)
    _assert_ends_in_the_test_result(tested)
    history = [
        {"role": "user", "content": "Repositories must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        {"role": "user", "content": _PICKED},
        {"role": "assistant", "content": tested[-1]["message"]},
    ]

    events, built = await _save_it_after(monkeypatch, history, {"case_type": "check_authoring"})

    assert built == [_PRIVATE_DRAFT]
    (card,) = [json.loads(ev) for ev in events if '"confirmation_required"' in ev]
    assert card["test_skipped"] is True and "tested" not in card


def test_save_it_reaches_back_only_over_test_results():
    draft = [
        {"role": "user", "content": "Repositories must be private"},
        {"role": "assistant", "content": _STEP2},
    ]
    scope = {"case_type": "check_authoring"}
    save = {"role": "user", "content": "save it"}
    test = {"role": "user", "content": "test it on order-api"}

    def reply(*after):
        return orchestrator._reply_to_save([*draft, *after, save], scope)

    assert reply(test, {"role": "assistant", "content": _TESTED}) == _STEP2
    assert (
        reply(
            test,
            {"role": "assistant", "content": _TESTED},
            test,
            {"role": "assistant", "content": f"**{_TESTED}"},
        )
        == _STEP2
    )
    # A test turn is the reply to a test request, whatever the reply says:
    # the Test button's action, or a typed request.
    clicked = {
        "role": "user",
        "content": "Test it",
        "action": {"name": "test", "context": {"entityRef": "component:default/order-api"}},
    }
    assert reply(clicked, {"role": "assistant", "content": "Order-api fails this rule."}) == _STEP2
    # Words that look like a result don't make one.
    assert (
        reply({"role": "user", "content": "hmm"}, {"role": "assistant", "content": _TESTED})
        is None
    )
    # The previous reply is the rule: card_for_save_request reads it itself.
    assert reply() is None
    # Anything else in between: "save it" isn't about that draft any more.
    assert (
        reply(
            {"role": "user", "content": "what does it check?"},
            {"role": "assistant", "content": "It checks that the repository is private."},
        )
        is None
    )
    # An edit the user didn't approve isn't saved, tested or not.
    unapproved = [
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": _STEP2},
        test,
        {"role": "assistant", "content": _TESTED},
        save,
    ]
    assert orchestrator._reply_to_save(unapproved, _EDIT_SCOPE) is None


async def test_a_failing_shortcut_falls_back_to_the_model(monkeypatch):
    async def _broken(*_args, **_kwargs):
        raise KeyError("valid")

    built = []

    async def _fake_build_agent(_token, scope, **_kwargs):
        built.append(scope)
        return object(), [], {}

    async def _reply(_agent, _messages, parser, **_kwargs):
        parser.set_message("Here is the check again.")
        return
        yield  # makes this an async generator

    monkeypatch.setattr(orchestrator, "card_for_save_request", _broken)
    monkeypatch.setattr(orchestrator, "card_for_separate_check", _broken)
    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _reply)
    events = [
        json.loads(ev)
        async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": "save it"}],
            token="t", user_sub="u", scope=None, backstage_token="b",
        )
    ]

    assert len(built) == 1
    assert events[-1] == {"type": "done", "message": "Here is the check again."}


_TWO_CHOICES = (
    "**Please confirm:**\n"
    "1. **Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60` — 60 or less\n"
    "2. **Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60` — less than 60\n\n"
    "Reply with 1 or 2."
)


def _phase(step1, answer):
    return orchestrator._edit_phase([
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": step1},
        {"role": "user", "content": answer},
    ], _EDIT_SCOPE)


@pytest.mark.parametrize("answer", ["yes", "Yes, go ahead!", "ok", "yes please", "sounds good", "1"])
def test_approvals_of_a_single_proposal(answer):
    assert _phase(_STEP1, answer) == "apply"


@pytest.mark.parametrize("answer", [
    "yes change to 70", "make it 90", "2", "must be private", "60",
    "yesterday's pushes", "no", "",
])
def test_non_approvals_of_a_single_proposal(answer):
    assert _phase(_STEP1, answer) == "propose"


@pytest.mark.parametrize("answer", [
    "what does this check do?", "Suggest ways to improve this check",
    "Explain what this check tests", "any ideas to make it better?", "why 60?",
    "How can I improve this to require more reviews?",
    "Suggest how to improve it to make it stricter",
])
def test_a_request_for_ideas_or_an_explanation_is_no_change(answer):
    assert _phase(_STEP1, answer) == "discuss"


@pytest.mark.parametrize("answer", [
    "Make this check stricter: at least 3 reviews", "add repo must be private too",
])
def test_a_change_request_is_still_a_change(answer):
    assert _phase(_STEP1, answer) == "propose"


@pytest.mark.parametrize("answer", ["1", "2.", "option 2", "the first one", "yes, 1", "1 please", "(2)"])
def test_picks_of_a_listed_choice(answer):
    assert _phase(_TWO_CHOICES, answer) == "apply"


@pytest.mark.parametrize("answer", ["yes", "ok", "3", "1 and make it 90", "60", "the third one"])
def test_non_picks_of_a_listed_choice(answer):
    # "yes" doesn't say which choice; 3 wasn't offered.
    assert _phase(_TWO_CHOICES, answer) == "propose"


# The 2026-11-13 live test: after "No, keep it" the reply asked an open
# question, got Yes / No buttons, and the user's "yes" skipped to Step 2.
_OPEN_QUESTION = (
    "**Current rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredApprovingReviewCount` `greaterThanInclusive` `3` — passes when the branch "
    "is protected and requires 3 or more approving reviews.\n\n"
    "**What I understood:** You rejected changing the required approvals from 3 to 2. Tell me "
    "exactly what change you want instead: a different number, more than N, or removing the "
    "approvals condition?"
)
# Options in words, with no "Proposed rule:" line.
_WORDED_OPTIONS = (
    "I'm not sure which option you're confirming. Do you mean:\n"
    "1. Change the required approving reviews from 3 to 2 (so 2 or more approvals pass),\n"
    "2. Require more than 2 approvals (so 3 or more approvals pass), or\n"
    "3. Remove the approvals requirement entirely?\n\n"
    "Reply with 1, 2 or 3."
)


@pytest.mark.parametrize("step1", [_OPEN_QUESTION, _WORDED_OPTIONS], ids=["open", "worded"])
@pytest.mark.parametrize("answer", ["yes", "ok", "1"])
def test_an_approval_of_a_reply_that_proposes_no_rule_is_not_step2(step1, answer):
    assert _phase(step1, answer) == "propose"


@pytest.mark.parametrize("step1", [
    "Shall I change it to `githubVisibility in [\"private\",\"internal\"]`?",
    "You want 60. Shall I make it `githubDaysSinceLastPush` `lessThanInclusive` `60`?",
], ids=["one span", "three spans"])
def test_a_proposal_without_the_label_can_still_be_approved(step1):
    # Without this, "yes" to such a reply would be asked again and again.
    assert _phase(step1, "yes") == "apply"


def test_an_approval_after_a_step2_reply_is_not_a_new_confirmation():
    # "ok" after the card was shown answers no Step 1 question.
    messages = [
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": _STEP1},
        {"role": "user", "content": "yes"},
        {"role": "assistant", "content": _STEP2},
        {"role": "user", "content": "ok"},
    ]
    assert orchestrator._edit_phase(messages, _EDIT_SCOPE) == "propose"


def test_the_first_message_of_an_edit_is_propose():
    assert orchestrator._edit_phase(_FIRST, _EDIT_SCOPE) == "propose"


def test_a_new_check_has_no_edit_phase():
    assert orchestrator._edit_phase(_FIRST, {"case_type": "check_authoring"}) is None


async def test_a_changed_value_gets_no_tools_no_card_and_the_note(monkeypatch):
    # "yes change to 70": the model wrote a rule for 70, which wasn't proposed.
    seen = {}
    agent = _RewriteAgent(reply="Proposed rule: `githubDaysSinceLastPush` ≤ `70`. Shall I change it to that?")

    events, built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes change to 70"}],
        _STEP2,
        agent=agent,
        seen=seen,
    )

    assert seen["edit_phase"] == "propose"  # DryRunStop: no tools
    assert seen["messages"][-1]["content"].endswith(orchestrator._NOT_APPROVED_NOTE)
    assert "yes change to 70" in seen["messages"][-1]["content"]
    assert built == []
    assert not _has_card(events)
    assert len(agent.calls) == 1  # the rule reply was rewritten as a question
    assert "70" in _done_message(events) and "```" not in _done_message(events)


async def test_the_first_message_gets_no_note(monkeypatch):
    seen = {}

    await _turn(monkeypatch, _FIRST, _STEP1, seen=seen)

    assert seen["edit_phase"] == "propose"
    assert orchestrator._NOT_APPROVED_NOTE not in seen["messages"][-1]["content"]


async def test_an_approval_builds_the_agent_with_tools(monkeypatch):
    seen = {}

    await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes"}],
        _STEP2,
        seen=seen,
    )

    assert seen["edit_phase"] == "apply"
    assert orchestrator._NOT_APPROVED_NOTE not in seen["messages"][-1]["content"]


# ── A first edit reply that skipped Step 1 is rewritten ─────────────

class _RewriteAgent:
    """The agent's second, non-streamed run: returns ``reply`` or raises ``error``."""

    def __init__(self, reply=None, error=None):
        # A list of replies is answered one by one, the last one again after that.
        self.reply, self.error, self.calls = reply, error, []

    async def ainvoke(self, payload):
        self.calls.append(payload["messages"])
        if self.error:
            raise self.error
        reply = self.reply
        if isinstance(reply, list):
            reply = reply[min(len(self.calls), len(reply)) - 1]
        return {"structured_response": SimpleNamespace(message=reply)}


def _done_message(events):
    return json.loads(events[-1])["message"]


async def test_a_first_edit_reply_with_a_rule_is_rewritten_as_the_step1_question(monkeypatch):
    agent = _RewriteAgent(reply=_STEP1)

    events, built = await _turn(monkeypatch, _FIRST, _STEP2, agent=agent)

    assert len(agent.calls) == 1
    history = agent.calls[0]
    assert history[-2] == {"role": "assistant", "content": _STEP2}
    assert history[-1]["content"] == orchestrator._STEP1_NOTE
    assert _done_message(events) == _STEP1
    assert built == []  # no card, so no validation by the card either
    assert not _has_card(events)
    assert any('"ask_first"' in ev for ev in events)


async def test_a_rewrite_that_still_has_a_rule_keeps_the_first_reply(monkeypatch):
    still_a_rule = _STEP2 + "\nShall I save it?"
    agent = _RewriteAgent(reply=still_a_rule)

    events, built = await _turn(monkeypatch, _FIRST, _STEP2, agent=agent)

    assert len(agent.calls) == 1
    assert _done_message(events) == _STEP2
    assert built == []
    assert not _has_card(events)


async def test_a_failed_rewrite_keeps_the_first_reply(monkeypatch):
    agent = _RewriteAgent(error=RuntimeError("model unavailable"))

    events, built = await _turn(monkeypatch, _FIRST, _STEP2, agent=agent)

    assert len(agent.calls) == 1
    assert _done_message(events) == _STEP2
    assert built == []
    assert not _has_card(events)


async def test_a_first_edit_reply_without_a_rule_is_not_rewritten(monkeypatch):
    agent = _RewriteAgent(reply="unused")

    events, _built = await _turn(monkeypatch, _FIRST, _STEP1, agent=agent)

    assert agent.calls == []
    assert _done_message(events) == _STEP1


async def test_an_approved_step2_reply_is_not_rewritten(monkeypatch):
    agent = _RewriteAgent(reply="unused")

    events, built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "1"}],
        _STEP2,
        agent=agent,
    )

    assert agent.calls == []
    assert built == [_STEP2]
    assert _has_card(events)


async def test_a_new_check_reply_with_a_rule_is_not_rewritten(monkeypatch):
    agent = _RewriteAgent(reply="unused")

    events, built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "Repositories must be private"}],
        _STEP2,
        scope={"case_type": "check_authoring"},
        agent=agent,
    )

    assert agent.calls == []
    assert built == [_STEP2]
    assert _has_card(events)


async def test_the_step1_heading_is_dropped_from_an_edit_reply(monkeypatch):
    events, _built = await _turn(
        monkeypatch, _FIRST, f"{_STEP1}\n\nStep 1 — confirm the change",
    )

    assert _done_message(events) == _STEP1


async def test_a_new_check_reply_keeps_its_text(monkeypatch):
    reply = f"{_STEP2}\n\nStep 1 — confirm the change"

    events, _built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "Repositories must be private"}],
        reply,
        scope={"case_type": "check_authoring"},
    )

    assert _done_message(events) == reply


# ── Streaming of check edit replies ─────────────────────────────────


class _StreamAgent:
    """Streams ``message`` as ChatResponse JSON text in a few chunks."""

    def __init__(self, message):
        self._json = json.dumps({"message": message})

    async def astream(self, _input, **_kwargs):
        step = max(1, len(self._json) // 4)
        for i in range(0, len(self._json), step):
            block = {"type": "text", "text": self._json[i:i + step]}
            yield SimpleNamespace(type="AIMessageChunk", content_blocks=[block]), {}


async def _streamed_turn(monkeypatch, messages, reply, scope=_EDIT_SCOPE):
    """Runs stream_chat through the real _yield_chunks; returns the events."""

    async def _fake_build_agent(_token, _scope, **_kwargs):
        return _StreamAgent(reply), [], {}

    async def _fake_confirmation(_message, _scope, **_kwargs):
        return None

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    return [
        ev
        async for ev in orchestrator.stream_chat(
            messages=messages, token="t", user_sub="u", scope=scope, backstage_token="b",
        )
    ]


def _chunks(events):
    return [ev for ev in events if '"message_chunk"' in ev]


async def test_an_edit_reply_waiting_for_yes_is_not_streamed(monkeypatch):
    events = await _streamed_turn(monkeypatch, _FIRST, _STEP1)

    assert _chunks(events) == []
    assert _done_message(events) == _STEP1


async def test_a_rewritten_edit_reply_never_shows_the_first_draft(monkeypatch):
    # The model wrote the rule; only the rewritten question may reach the user.
    agent = _StreamAgent(_STEP2)
    rewrite = _RewriteAgent(reply=_STEP1)
    agent.ainvoke = rewrite.ainvoke

    async def _fake_build_agent(_token, _scope, **_kwargs):
        return agent, [], {}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    events = [
        ev
        async for ev in orchestrator.stream_chat(
            messages=_FIRST, token="t", user_sub="u", scope=_EDIT_SCOPE, backstage_token="b",
        )
    ]

    assert _chunks(events) == []
    assert "```" not in "".join(events)
    assert _done_message(events) == _STEP1


async def test_an_approved_edit_reply_is_sent_whole(monkeypatch):
    # Every check authoring reply is shown as a card once complete.
    events = await _streamed_turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes"}],
        _STEP2,
    )

    assert _chunks(events) == []
    assert _done_message(events) == _STEP2


async def test_a_new_check_reply_is_sent_whole(monkeypatch):
    reply = "Which entity kind should this check run on?"

    events = await _streamed_turn(
        monkeypatch, [{"role": "user", "content": "Why?"}], reply,
        scope={"case_type": "check_authoring"},
    )

    assert _chunks(events) == []
    assert _done_message(events) == reply


@pytest.mark.parametrize("scope", [
    pytest.param({"case_type": "build_failure"}, id="build failure"),
    pytest.param({"case_type": "runtime_debug"}, id="runtime debug"),
    pytest.param(None, id="no scope"),
])
async def test_other_chats_still_stream(monkeypatch, scope):
    reply = "The build failed because a dependency is missing."

    events = await _streamed_turn(monkeypatch, [{"role": "user", "content": "Why?"}], reply, scope=scope)

    assert "".join(json.loads(ev)["content"] for ev in _chunks(events)) == reply
    assert _done_message(events) == reply


# ── A2UI answer buttons ─────────────────────────────────────────────


def _a2ui(events):
    return [json.loads(ev) for ev in events if '"type":"a2ui"' in ev]


def _components(event):
    return event["messages"][0]["createSurface"]["components"]


def _questions(events):
    """The a2ui events that answer an edit question (not the save card)."""
    return [e for e in _a2ui(events) if _components(e)[0]["component"] != "SaveCard"]


def _save_cards(events):
    return [e for e in _a2ui(events) if _components(e)[0]["component"] == "SaveCard"]


def _button_messages(event):
    """What each clickable part sends: option tiles first, then buttons."""
    components = _components(event)
    return [
        c["action"]["event"]["userMessage"]
        for kind in ("OptionTile", "Button")
        for c in components
        if c["component"] == kind
    ]


async def test_an_edit_question_gets_yes_and_no_buttons_before_done(monkeypatch):
    events, _built = await _turn(monkeypatch, _FIRST, _STEP1)

    (event,) = _a2ui(events)
    assert _button_messages(event) == ["yes", "no"]
    assert '"done"' in events[-1]


_TWO_CHOICES = (
    "**Please confirm:**\n"
    "1. **Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60` — 60 passes\n"
    "2. **Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60` — 60 fails\n\n"
    "Reply with 1 or 2."
)


async def test_two_choices_get_a_change_card_with_an_option_tile_each(monkeypatch):
    # No "?" in the reply: the 2026-11-12 live test showed no buttons for it.
    events, _built = await _turn(monkeypatch, _FIRST, _TWO_CHOICES)

    (event,) = _a2ui(events)
    root = _components(event)[0]
    assert root["component"] == "ChangeCard"
    assert root["footer"] == ["option_1", "option_2", "no"]
    assert _button_messages(event) == ["1", "2", "no"]


async def test_one_proposal_gets_a_change_card_with_yes_and_no(monkeypatch):
    reply = (
        "**Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60` — 60 or less\n\n"
        "Shall I change it to that?"
    )

    events, _built = await _turn(
        monkeypatch, _FIRST, reply, scope={**_EDIT_SCOPE, "check_name": "Recent push"},
    )

    (event,) = _a2ui(events)
    by_id = {c["id"]: c for c in _components(event)}
    root = by_id["root"]
    assert (root["component"], root["subtitle"]) == ("ChangeCard", "Recent push")
    (change,) = [by_id[i] for i in root["changes"]]
    assert (change["before"], change["after"]) == ("30 or less", "60 or less")
    assert _button_messages(event) == ["yes", "no"]


# ── Step 2 writes the rule the user approved ────────────────────────

_AT_MOST_60 = {
    "all": [{"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", "value": 60}]
}
_UNDER_60 = {"all": [{"fact": "githubDaysSinceLastPush", "operator": "lessThan", "value": 60}]}


def _edited(rule):
    return f"Edited rule:\n\n```json\n{json.dumps(rule)}\n```"


async def _card_of_reply(message, _scope, **_kwargs):
    """A save card for the rule in ``message``, as build_confirmation makes it."""
    rule = json.loads(message.split("```json\n", 1)[1].split("\n```", 1)[0])
    return {**_CARD, "new_rule": json.dumps(rule, indent=2), "problems": []}


@pytest.mark.parametrize(
    ("answer", "written", "card"),
    [
        pytest.param("1", _AT_MOST_60, True, id="the picked choice"),
        pytest.param("2", _UNDER_60, True, id="the other picked choice"),
        pytest.param("1", _UNDER_60, False, id="the other choice than the picked one"),
        pytest.param("2", _AT_MOST_60, False, id="the other choice than the picked two"),
        pytest.param("1, then test it on order-api", _UNDER_60, False, id="picked with a test"),
    ],
)
async def test_step2_gets_the_card_only_for_the_picked_choice(monkeypatch, answer, written, card):
    events, _built = await _turn(
        monkeypatch,
        [
            *_FIRST,
            {"role": "assistant", "content": _TWO_CHOICES},
            {"role": "user", "content": answer},
        ],
        _edited(written),
        confirmation=_card_of_reply,
    )

    assert _has_card(events) is card


async def test_another_rule_than_the_picked_one_is_asked_about_again(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _TWO_CHOICES}, {"role": "user", "content": "1"}],
        _edited(_UNDER_60),
        confirmation=_card_of_reply,
    )

    message = _done_message(events)
    assert "```" not in message
    assert "**Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60`" in message
    assert orchestrator._REPROPOSED in message
    (event,) = _a2ui(events)
    assert _components(event)[0]["component"] == "ChangeCard"
    assert _button_messages(event) == ["yes", "no"]


async def test_a_yes_to_the_question_asked_again_gets_the_card(monkeypatch):
    asked = orchestrator._reproposal(
        [("githubDaysSinceLastPush", "lessThan", 60)],
        _EDIT_SCOPE,
    )

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": asked}, {"role": "user", "content": "yes"}],
        _edited(_UNDER_60),
        confirmation=_card_of_reply,
    )

    assert _has_card(events)


async def test_an_unlabeled_proposal_is_compared_too(monkeypatch):
    # CodeRabbit: approved without a "Proposed rule:" label, then another rule written.
    step1 = (
        "**Current rule:** `githubDaysSinceLastPush` `lessThanInclusive` `30`\n\n"
        "Shall I make it `githubDaysSinceLastPush` `lessThanInclusive` `60` (60 or less)?"
    )
    history = [*_FIRST, {"role": "assistant", "content": step1}, {"role": "user", "content": "yes"}]

    events, _built = await _turn(
        monkeypatch,
        history,
        _edited(_UNDER_60),
        confirmation=_card_of_reply,
    )
    assert not _has_card(events)
    assert "`lessThan` `60`" in _done_message(events)

    events, _built = await _turn(
        monkeypatch,
        history,
        _edited(_AT_MOST_60),
        confirmation=_card_of_reply,
    )
    assert _has_card(events)


@pytest.mark.parametrize(
    ("step1", "phase"),
    [
        # CodeRabbit: conditions on two loose lines, no label: which one is
        # proposed can't be told, so a yes to it couldn't be compared.
        pytest.param(
            "I could make it `githubDaysSinceLastPush` `lessThanInclusive` `60`.\n"
            "Or `githubDaysSinceLastPush` `lessThan` `60`. Shall I?",
            "propose",
            id="unlabeled on two lines",
        ),
        pytest.param(
            "Shall I make it `githubDaysSinceLastPush` `lessThanInclusive` `60`?",
            "apply",
            id="unlabeled on one line",
        ),
        # Labeled ones are approved as before, read or not (an OR isn't compared).
        pytest.param(
            "**Proposed rule:** `a` `equal` `1` OR `b` `equal` `2`\n\nShall I change it to that?",
            "apply",
            id="labeled OR",
        ),
    ],
)
def test_a_yes_approves_an_unlabeled_proposal_only_when_it_can_be_read(step1, phase):
    assert _phase(step1, "yes") == phase


async def test_a_rewrite_after_validation_is_compared_too(monkeypatch):
    async def _invalid_until_rewritten(message, scope, **kwargs):
        built = await _card_of_reply(message, scope, **kwargs)
        if "lessThanInclusive" in message:
            return {**built, "valid": False, "problems": ["Something is wrong"]}
        return built

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _TWO_CHOICES}, {"role": "user", "content": "1"}],
        _edited(_AT_MOST_60),
        agent=_RewriteAgent(reply=_edited(_UNDER_60)),
        confirmation=_invalid_until_rewritten,
    )

    assert not _has_card(events)
    assert "`lessThan` `60`" in _done_message(events)


async def test_a_test_in_a_step2_asked_about_again_shows_the_question(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [
            *_FIRST,
            {"role": "assistant", "content": _TWO_CHOICES},
            {"role": "user", "content": "1, then test it on order-api"},
        ],
        _edited(_UNDER_60),
        confirmation=_card_of_reply,
        dry_run=DryRun(status="passed", entity_ref="component:default/order-api"),
    )

    (event,) = _a2ui(events)
    assert _components(event)[0]["component"] == "ChangeCard"


async def test_the_same_conditions_in_another_order_get_the_card(monkeypatch):
    step1 = (
        '**Proposed rule:** `visibility` `in` `["private", "internal"]` AND `archived` `equal` `false`'
        "\n\nShall I change it to that?"
    )
    written = {
        "all": [
            {"fact": "archived", "operator": "equal", "value": False},
            {"fact": "visibility", "operator": "in", "value": ["internal", "private"]},
        ]
    }

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": step1}, {"role": "user", "content": "yes"}],
        _edited(written),
        confirmation=_card_of_reply,
    )

    assert _has_card(events)


async def test_a_proposal_code_cant_read_isnt_compared(monkeypatch):
    # An OR proposal: the card shows the rule it saves, as before.
    step1 = "**Proposed rule:** `a` `equal` `1` OR `b` `equal` `2`\n\nShall I change it to that?"

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": step1}, {"role": "user", "content": "yes"}],
        _edited({"all": [{"fact": "a", "operator": "equal", "value": 3}]}),
        confirmation=_card_of_reply,
    )

    assert _has_card(events)


@pytest.mark.parametrize(
    "rule",
    [
        pytest.param({"all": [{"not": {"fact": "a", "operator": "equal", "value": 1}}]}, id="NOT"),
        pytest.param(
            {"all": [{"all": [{"fact": "a", "operator": "equal", "value": 1}]}]}, id="nested"
        ),
        pytest.param(
            {
                "any": [
                    {"fact": "a", "operator": "equal", "value": 1},
                    {"fact": "b", "operator": "equal", "value": 2},
                ]
            },
            id="any of two",
        ),
        pytest.param(
            {"all": [{"fact": "a", "path": "$.length", "operator": "greaterThan", "value": 0}]},
            id="a path",
        ),
        pytest.param({"all": []}, id="empty"),
        pytest.param("not json", id="not json"),
    ],
)
def test_rules_a_proposal_cant_write_arent_compared(rule):
    conditions = flat_conditions(rule if isinstance(rule, str) else json.dumps(rule))
    assert not conditions


def test_plain_conditions_read_a_wrapped_rule_and_a_single_any():
    wrapped = json.dumps({"conditions": {"any": [{"fact": "a", "operator": "equal", "value": 1}]}})

    assert flat_conditions(wrapped) == [("a", "equal", 1)]


@pytest.mark.parametrize(
    ("first", "second", "same"),
    [
        pytest.param([("a", "equal", True)], [("a", "equal", 1)], False, id="true is not 1"),
        pytest.param([("a", "in", ["x", "y"])], [("a", "in", ["y", "x"])], True, id="list order"),
        pytest.param(
            [("a", "equal", 1)],
            [("a", "equal", 1), ("a", "equal", 1)],
            False,
            id="a condition more",
        ),
        pytest.param([("a", "equal", "1")], [("a", "equal", 1)], False, id="text is not a number"),
    ],
)
def test_same_conditions(first, second, same):
    assert orchestrator._same_conditions(first, second) is same


async def test_choices_the_change_card_can_not_show_get_a_choice_card(monkeypatch):
    reply = (
        "1. **Proposed rule:** `a` `lessThanInclusive` `60` AND (`b` `equal` `1` OR `c` `equal` `2`)"
        " — 60 or less, if b is 1 or c is 2\n"
        "2. **Proposed rule:** `a` `lessThan` `60` — less than 60\n\n"
        "Reply with 1 or 2."
    )

    events, _built = await _turn(monkeypatch, _FIRST, reply)

    (event,) = _a2ui(events)
    components = _components(event)
    assert components[0]["component"] == "AskCard"
    options = [c for c in components if c["component"] == "AskOption"]
    assert [(o["label"], o["action"]["event"]["userMessage"]) for o in options] == [
        ("60 or less, if b is 1 or c is 2", "1"), ("Less than 60", "2"),
    ]
    assert _button_messages(event) == ["no"]
    assert any(c["component"] == "AskField" for c in components)


@pytest.mark.parametrize(("reply", "title"), [
    (_OPEN_QUESTION, "What should change?"),
    (_WORDED_OPTIONS, "Which change do you mean?"),
], ids=["open", "worded"])
async def test_an_edit_reply_that_proposes_no_rule_gets_an_ask_card(monkeypatch, reply, title):
    events, _built = await _turn(
        monkeypatch, _FIRST, reply, scope={**_EDIT_SCOPE, "check_name": "Recent push"},
    )

    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["title"], root["subtitle"]) == ("AskCard", title, "Recent push")
    # Nothing to say yes to: no Yes / No, only "keep the rule".
    assert _button_messages(event) == ["Keep the rule as it is"]


_BRANCH_SCOPE = {
    **_EDIT_SCOPE,
    "current_rule": json.dumps({"all": [
        {"fact": "githubBranchProtected", "operator": "equal", "value": True},
        {"fact": "githubRequiredApprovingReviewCount", "operator": "greaterThanInclusive",
         "value": 3},
    ]}),
}
_MAKE_IT_2 = (
    "**What I understood:** You want the required approvals changed from 3 to 2. Should exactly "
    "2 approvals still pass?\n\n"
    "1. **Proposed rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredApprovingReviewCount` `greaterThanInclusive` `2` — 2 or more\n"
    "2. **Proposed rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredApprovingReviewCount` `greaterThan` `2` — more than 2"
)


async def test_no_keep_it_then_yes_as_in_the_live_test(monkeypatch):
    # 2026-11-13: "make it 2" → change card → "No, keep it" (sends "no").
    chat = [
        {"role": "user", "content": "make it 2"},
        {"role": "assistant", "content": _MAKE_IT_2},
        {"role": "user", "content": "no"},
    ]
    seen = {}

    events, built = await _turn(monkeypatch, chat, _OPEN_QUESTION, scope=_BRANCH_SCOPE, seen=seen)

    assert seen["edit_phase"] == "propose"  # no tools, nothing saved
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    by_id = {c["id"]: c for c in _components(event)}
    assert by_id["root"]["title"] == "What should change?"
    assert [by_id[i]["value"] for i in by_id["root"]["current"]] == ["Yes", "3 or more"]
    assert [by_id[i]["kind"] for i in by_id["root"]["editors"]] == ["choice", "number"]
    assert _button_messages(event) == ["Keep the rule as it is"]  # no "Yes, change it"

    # Then the user typed "yes": before the fix this went to the save step.
    chat += [{"role": "assistant", "content": _OPEN_QUESTION}, {"role": "user", "content": "yes"}]

    events, built = await _turn(monkeypatch, chat, _WORDED_OPTIONS, scope=_BRANCH_SCOPE, seen=seen)

    assert seen["edit_phase"] == "propose"
    assert seen["messages"][-1]["content"].endswith(orchestrator._NOT_APPROVED_NOTE)
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    assert _components(event)[0]["title"] == "Which change do you mean?"


_BRANCH = "`githubBranchProtected` `equal` `true` AND "
# The reply after "make it 5" → "no", 2026-11-13: option 4 also removes the
# branch protection, so the choices differ in two conditions.
_UNCLEAR_CHOICES = (
    "**What I understood:** your earlier “no” rejected my two readings of “make it 5”. Please "
    "tell me which of these you actually want (reply with the number):\n\n"
    f"1. **Proposed rule:** {_BRANCH}`githubRequiredApprovingReviewCount` "
    "`greaterThanInclusive` `5` — 5 or more\n"
    f"2. **Proposed rule:** {_BRANCH}`githubRequiredApprovingReviewCount` `greaterThan` `5` — "
    "more than 5\n"
    f"3. **Proposed rule:** {_BRANCH}`githubRequiredApprovingReviewCount` `equal` `5` — exactly 5\n"
    "4. **Proposed rule:** `githubRequiredApprovingReviewCount` `greaterThanInclusive` `5` — "
    "no branch protection"
)


async def test_unclear_choices_get_the_input_card_instead(monkeypatch):
    chat = [{"role": "user", "content": "make it 5"}]

    events, _built = await _turn(monkeypatch, chat, _UNCLEAR_CHOICES, scope=_BRANCH_SCOPE)

    (event,) = _questions(events)
    by_id = {c["id"]: c for c in _components(event)}
    root = by_id["root"]
    assert (root["component"], root["title"]) == ("AskCard", "What should change?")
    # The lead-in to the hidden list is left out, and it starts with a capital.
    assert root["summary"] == "Your earlier “no” rejected my two readings of “make it 5”."
    assert [by_id[i]["kind"] for i in root["editors"]] == ["choice", "number"]
    assert _button_messages(event) == ["Keep the rule as it is"]


@pytest.mark.parametrize(("step1", "phase"), [(_UNCLEAR_CHOICES, "propose"), (_MAKE_IT_2, "apply")])
def test_a_number_picks_a_choice_only_when_the_choices_were_shown(step1, phase):
    # After _UNCLEAR_CHOICES the user saw the input card, not the choices.
    messages = [
        {"role": "user", "content": "make it 5"},
        {"role": "assistant", "content": step1},
        {"role": "user", "content": "2"},
    ]
    assert orchestrator._edit_phase(messages, _BRANCH_SCOPE) == phase


async def test_clear_choices_still_get_tiles_and_keep_the_current_rule(monkeypatch):
    chat = [{"role": "user", "content": "make it 2"}]

    events, _built = await _turn(monkeypatch, chat, _MAKE_IT_2, scope=_BRANCH_SCOPE)

    (event,) = _questions(events)
    by_id = {c["id"]: c for c in _components(event)}
    assert by_id["root"]["component"] == "ChangeCard"
    assert [by_id[i]["value"] for i in by_id["root"]["footer"][:2]] == ["2 or more", "More than 2"]
    assert by_id["no_label"]["text"] == "Keep the current rule"
    assert _button_messages(event) == ["1", "2", "no"]


async def test_an_ask_card_only_comes_with_an_edit(monkeypatch):
    # A new check has no saved rule to edit: its reply gets the message card.
    events, _built = await _turn(
        monkeypatch, _FIRST, _WORDED_OPTIONS, scope={"case_type": "check_authoring"},
    )

    assert [_components(e)[0]["component"] for e in _questions(events)] == ["MessageCard"]


async def test_a_closing_reply_gets_its_text_in_a_message_card(monkeypatch):
    reply = "**What I understood:** You want to keep the rule as it is. Nothing changes."

    events, _built = await _turn(monkeypatch, _FIRST, reply)

    (event,) = _questions(events)
    (card,) = _components(event)
    assert (card["component"], card["text"]) == ("MessageCard", reply)


async def test_an_edit_reply_that_still_has_a_rule_gets_no_buttons(monkeypatch):
    # The rewrite failed, so the first reply (with a rule) is kept.
    events, _built = await _turn(
        monkeypatch, _FIRST, _STEP2, agent=_RewriteAgent(error=RuntimeError("down")),
    )

    assert _a2ui(events) == []


async def test_an_approved_edit_gets_the_card_and_no_buttons(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes"}],
        _STEP2,
    )

    assert _has_card(events)
    assert _questions(events) == []
    # The same card again as an A2UI SaveCard, right after it, unchanged.
    (save,) = _save_cards(events)
    types = [json.loads(ev)["type"] for ev in events]
    assert types[types.index("confirmation_required") + 1] == "a2ui"
    root = _components(save)[0]
    card = next(json.loads(ev) for ev in events if '"confirmation_required"' in ev)
    assert root["card"] == {k: v for k, v in card.items() if k != "type"}
    # The reply has no words besides the rule: nothing to show above it.
    assert "note" not in root


async def test_a_test_the_model_only_wrote_about_is_not_on_the_card(monkeypatch):
    # No dry-run ran: the model's "Tested on" text would show a test that never ran.
    tested = "**Tested on** `component:default/order-api`\n- Dry-run result: **Passed**"

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes"}],
        f"{tested}\n\n{_STEP2}",
    )

    (save,) = _save_cards(events)
    root = _components(save)[0]
    assert "note" not in root and "resultTitle" not in root


@pytest.mark.parametrize("scope", [
    pytest.param({"case_type": "build_failure"}, id="build failure"),
    pytest.param({"case_type": "runtime_debug"}, id="runtime debug"),
    pytest.param(None, id="no scope"),
])
async def test_other_chats_get_no_cards(monkeypatch, scope):
    # The real save step, a Backstage that would validate the rule, and a
    # reply with a rule in it: only the save step's scope gate stands between
    # these chats and a save card, whatever the model writes.
    from src.agent import confirm_card
    from src.agent.confirm_card import build_confirmation

    async def _tools(_auth):
        return {}

    async def _call(_tools, _name, _args):
        return {"valid": True, "problems": [], "kinds": []}

    monkeypatch.setattr(confirm_card, "backstage_tools_as_user", _tools)
    monkeypatch.setattr(confirm_card, "call_backstage_tool", _call)
    events, _built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "Is it OK?"}],
        _GUARDED_DRAFT,
        scope=scope,
        confirmation=build_confirmation,
    )

    assert not [ev for ev in events if '"confirmation_required"' in ev]
    assert _a2ui(events) == []
    # And the chat itself still worked: no error, and the reply arrived whole.
    assert not [ev for ev in events if json.loads(ev)["type"] == "error"]
    assert json.loads(events[-1])["type"] == "done"
    assert "Repository must be private" in _done_message(events)


_MODEL_CARD = {
    "title": "Which entity kind?",
    "summary": "Which kind of entity should this check run on?",
    "choices": [
        {"label": "Components", "message": "Run it on Components"},
        {"label": "APIs", "message": "Run it on APIs"},
    ],
}


async def test_a_check_authoring_reply_is_shown_as_the_card_the_model_filled(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "check the readme"}],
        "Which entity kind should this check run on? Components or APIs?",
        scope={"case_type": "check_authoring"},
        card=_MODEL_CARD,
    )

    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["title"]) == ("ReplyCard", "Which entity kind?")
    pick_messages = [
        c["action"]["event"]["userMessage"] for c in _components(event) if c["component"] == "PickRow"
    ]
    assert pick_messages == ["Run it on Components", "Run it on APIs"]


@pytest.mark.parametrize(("reply", "card", "logged"), [
    ("Which entity kind should this check run on?", _MODEL_CARD,
     "Reply shown as ReplyCard (the model's card: no code card read the reply)"),
    ("Which entity kind should this check run on?", None,
     "Reply shown as MessageCard (no code card read the reply, the model's card unusable)"),
    ("No available fact measures README content, so I can't build this check yet.", _MODEL_CARD,
     "Reply shown as NoticeCard"),
], ids=["model card", "message card", "notice card"])
async def test_the_log_names_the_card_each_reply_got(monkeypatch, caplog, reply, card, logged):
    caplog.set_level("INFO", logger="src.agent.orchestrator")

    await _turn(
        monkeypatch, [{"role": "user", "content": "check the readme"}], reply,
        scope={"case_type": "check_authoring"}, card=card,
    )

    assert logged in [r.getMessage() for r in caplog.records]


async def test_without_the_model_s_card_the_reply_gets_the_message_card(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "check the readme"}],
        "Which entity kind should this check run on?",
        scope={"case_type": "check_authoring"},
        card={"title": "", "summary": ""},
    )

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "MessageCard"


async def test_cards_built_by_code_come_before_the_model_s_card(monkeypatch):
    # An edit question: the change card, built from the exact rules, wins.
    events, _built = await _turn(monkeypatch, _FIRST, _MAKE_IT_2, scope=_BRANCH_SCOPE, card=_MODEL_CARD)

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "ChangeCard"


async def test_a_rewritten_reply_drops_the_card_of_the_first_draft(monkeypatch):
    # The first draft had a rule; the rewrite is a reply no code card reads,
    # so only dropping the draft's card keeps it from showing.
    events, _built = await _turn(
        monkeypatch, _FIRST, _STEP2,
        agent=_RewriteAgent(reply="Which value did you mean for the approvals?"),
        scope=_BRANCH_SCOPE, card=_MODEL_CARD,
    )

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "MessageCard"


@pytest.mark.parametrize("scope", [
    pytest.param({"case_type": "build_failure"}, id="build failure"),
    pytest.param(None, id="no scope"),
])
async def test_other_chats_never_show_a_card_the_model_filled(monkeypatch, scope):
    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "Why?"}], "Because.", scope=scope,
        card=_MODEL_CARD,
    )

    assert _questions(events) == []


async def test_a_new_check_question_gets_a_message_card_without_buttons(monkeypatch):
    events, _built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "Is it OK?"}],
        "Shall I look at the logs?",
        scope={"case_type": "check_authoring"},
    )

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "MessageCard"
    assert _button_messages(event) == []


async def test_a_no_fact_reply_in_a_check_chat_gets_a_notice_card(monkeypatch):
    reply = (
        "No available fact measures whether a README file exists, so I can't build this check "
        "yet. The closest facts are `githubHasCodeowners` (whether a CODEOWNERS file exists)."
    )
    scope = {
        "case_type": "check_authoring",
        "available_fact_retrievers": [{"id": "r", "facts": [{"name": "githubHasCodeowners"}]}],
    }

    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "check the readme"}], reply, scope=scope,
    )

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "NoticeCard"
    assert _done_message(events) == reply


async def test_the_same_reply_outside_check_authoring_gets_no_card(monkeypatch):
    reply = "No available fact measures x, so I can't build this check yet."

    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "?"}], reply, scope={"case_type": "build_failure"},
    )

    assert _questions(events) == []


# ── Testing an edit on a component ───────────────────────────────────


@pytest.mark.parametrize("answer", [
    "test it on order-api",
    "dry run on component:default/order-api",
    "Can you try it on order-api?",
    "run it against payments-api please",
    "test the rule on `order-api`",
    # What the portal's component picker sends.
    "Test it on component:default/order-api",
])
def test_a_test_request_is_its_own_step(answer):
    assert _phase(_STEP2, answer) == "test"


def test_a_test_request_as_the_first_message_tests_the_saved_rule():
    assert orchestrator._edit_phase(
        [{"role": "user", "content": "test it on order-api"}], _EDIT_SCOPE,
    ) == "test"


_DRAFTED = (
    "Name: Repository must have a recent push\n\n```json\n"
    '{"all":[{"fact":"githubDaysSinceLastPush","operator":"lessThanInclusive","value":30}]}\n```'
)


def test_a_test_request_only_tests_once_a_check_is_drafted():
    # Unsaved, or saved — then the portal sends the saved check in the
    # scope and the chat is an edit chat for it from then on.
    chat = [
        {"role": "user", "content": "Repos must have a push in the last 30 days"},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ]

    assert orchestrator._edit_phase(chat, {"case_type": "check_authoring"}) == "test"
    assert orchestrator._edit_phase(chat, _EDIT_SCOPE) == "test"
    # Nothing drafted yet: a new check's chat as before.
    assert orchestrator._edit_phase(chat[2:], {"case_type": "check_authoring"}) is None


async def test_a_saved_new_check_gets_no_second_save_card_when_tested(monkeypatch):
    agent = _DryRunAgent("order-api meets the rule.", {
        "status": "passed", "passed": True, "summary": "…", "reasons": [],
        "conditions": _evaluated({"githubDaysSinceLastPush": 3}),
    })
    built = []

    async def _fake_build_agent(_token, _scope, **_kwargs):
        return agent, [], {}

    async def _fake_confirmation(message, _scope, **_kwargs):
        built.append(message)
        return {**_CARD, "kind": "create"}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    chat = [
        {"role": "user", "content": "Repos must have a push in the last 30 days"},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ]

    events = [
        ev async for ev in orchestrator.stream_chat(
            messages=chat, token="t", user_sub="u", scope=_EDIT_SCOPE, backstage_token="b",
        )
    ]

    # Saved already: a second create card would save the check twice.
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "TestCard"


@pytest.mark.parametrize("answer", [
    "test it with 70 instead",
    "make it 70 and test it on order-api",
    "test it on order-api and make it 70",
    "test it",
])
def test_a_change_with_a_test_is_confirmed_first(answer):
    assert _phase(_STEP2, answer) == "propose"


_BRANCH_RULE = _BRANCH_SCOPE["current_rule"]


class _DryRunAgent:
    """Calls dry-run-check — its arguments streamed in two pieces, as the
    model sends them — gets ``result``, then answers ``message``."""

    def __init__(self, message, result):
        self._message = message
        self._result = result
        self.inputs = []

    async def astream(self, agent_input, **_kwargs):
        self.inputs.append(agent_input)
        args = json.dumps({
            "entityRef": "component:default/order-api",
            "check": {"name": "n", "description": "", "factIds": ["r"],
                      "rule": {"conditions": _BRANCH_RULE}},
        })
        half = len(args) // 2
        for piece, call_id, name in ((args[:half], "call_1", "tech-insights__dry-run-check"), (args[half:], None, None)):
            yield SimpleNamespace(type="AIMessageChunk", content_blocks=[], tool_call_chunks=[
                {"index": 0, "id": call_id, "name": name, "args": piece},
            ]), {}
        yield SimpleNamespace(
            type="tool", name="tech-insights__dry-run-check", tool_call_id="call_1",
            content=[{"type": "text", "text": json.dumps(self._result)}],
        ), {}
        yield SimpleNamespace(type="AIMessageChunk", content_blocks=[
            {"type": "text", "text": json.dumps({"message": self._message})},
        ]), {}


async def _tested_turn(
    monkeypatch, chat, scope=_BRANCH_SCOPE, message="It doesn't meet the rule.",
    card_rule=_BRANCH_RULE,
):
    """A turn whose model runs one dry-run of _BRANCH_RULE on order-api, which
    fails; the card built saves ``card_rule``. Returns (events, the phase the
    agent was built with, the replies cards were built from)."""
    agent = _DryRunAgent(message, {
        "status": "failed", "passed": False, "summary": "…", "reasons": [],
        "conditions": _evaluated({"githubBranchProtected": True, "githubRequiredApprovingReviewCount": 1}),
    })
    seen, built = {}, []

    async def _fake_build_agent(_token, _scope, *, edit_phase=None, **_kwargs):
        seen["edit_phase"] = edit_phase
        return agent, [], {}

    async def _fake_confirmation(message, _scope, **_kwargs):
        built.append(message)
        return {**_CARD, "new_rule": card_rule}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    events = [
        ev async for ev in orchestrator.stream_chat(
            messages=chat, token="t", user_sub="u", scope=scope, backstage_token="b",
        )
    ]
    seen["note"] = agent.inputs[0]["messages"][-1]["content"]
    return events, seen, built


def _confirmation(events):
    (event,) = [json.loads(ev) for ev in events if '"confirmation_required"' in ev]
    return event


async def test_a_test_of_an_unsaved_edit_ends_in_its_save_card(monkeypatch):
    chat = [
        {"role": "user", "content": "make it 2"},
        {"role": "assistant", "content": _MAKE_IT_2},
        {"role": "user", "content": "1"},
        {"role": "assistant", "content": _STEP2},
        {"role": "user", "content": "test it on order-api"},
    ]

    events, seen, built = await _tested_turn(monkeypatch, chat)

    assert seen["edit_phase"] == "test"
    assert seen["note"].endswith(orchestrator._TEST_NOTE)
    # The change's card again — what's left is to save it — and no test card.
    assert built == [_STEP2]
    assert _confirmation(events)["tested"] is True
    (card,) = _save_cards(events)
    assert _components(card)[0]["resultTitle"] == "Tested on order-api"
    assert _questions(events) == []
    assert "tested" not in _CARD  # the card it was built from is unchanged


async def test_a_test_of_an_unsaved_draft_ends_in_its_save_card(monkeypatch):
    chat = [
        {"role": "user", "content": "Repos must have a push in the last 30 days"},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ]

    events, seen, built = await _tested_turn(
        monkeypatch, chat, scope={"case_type": "check_authoring"},
    )

    assert seen["edit_phase"] == "test"
    assert built == [_DRAFTED]
    assert _confirmation(events)["tested"] is True
    assert len(_save_cards(events)) == 1 and _questions(events) == []
    steps = [json.loads(ev) for ev in events if '"tool_call"' in ev]
    assert [ev["activeForm"] for ev in steps] == [
        "Testing the check on order-api", "Thinking", "Writing up the test",
        "Preparing the save card",
    ]


_EXAMPLE = orchestrator._NEW_WELCOME_CHOICES["beginner"][0]


async def test_the_beginners_example_is_practice_once_tested(monkeypatch):
    # The welcome's example: tested, its card offers what to do next, not saving.
    chat = [
        {"role": "user", "content": _EXAMPLE},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ]

    events, _seen, _built = await _tested_turn(
        monkeypatch, chat, scope={"case_type": "check_authoring", "experience": "beginner"},
    )

    confirmation = _confirmation(events)
    assert confirmation["tested"] is True
    assert confirmation["example"] is True
    note = _components(_save_cards(events)[0])[0]["note"]
    assert "That's what a check is for" in note
    assert "draft" not in note and "publish" not in note


@pytest.mark.parametrize("chat, scope", [
    # Their own rule after the example: a real check.
    ([
        {"role": "user", "content": _EXAMPLE},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Repos must have a push in the last 30 days"},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ], {"case_type": "check_authoring", "experience": "beginner"}),
    # The same words from a user familiar with checks.
    ([
        {"role": "user", "content": _EXAMPLE},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ], {"case_type": "check_authoring", "experience": "experienced"}),
])
async def test_other_tested_cards_are_not_the_example(monkeypatch, chat, scope):
    events, _seen, _built = await _tested_turn(monkeypatch, chat, scope=scope)

    confirmation = _confirmation(events)
    assert confirmation["tested"] is True
    assert "example" not in confirmation


def test_the_example_is_found_only_in_a_beginners_new_check():
    chat = [{"role": "user", "content": _EXAMPLE}]
    beginner = {"case_type": "check_authoring", "experience": "beginner"}

    assert orchestrator._is_example(chat, beginner)
    assert not orchestrator._is_example(chat, {**beginner, "check_id": "c1"})
    assert not orchestrator._is_example([{"role": "user", "content": "save it"}], beginner)


_EXAMPLE_SCOPE = {
    "case_type": "check_authoring",
    "experience": "beginner",
    "available_fact_retrievers": [
        {"id": "githubRepositoryFactRetriever", "facts": [{"name": "githubBranchProtected"}]},
    ],
}


async def test_the_beginners_example_is_drafted_by_code_with_no_model_call(monkeypatch):
    # 2026-11-24: the same check every time, so the model isn't asked.
    seen = {}

    events, built = await _turn(
        monkeypatch, [{"role": "user", "content": _EXAMPLE}], "unused",
        scope=_EXAMPLE_SCOPE, seen=seen,
    )

    assert seen == {}  # the model was never asked
    (draft,) = built
    assert draft.startswith(
        "Name: Default branch must be protected\n\n"
        "Description: Fails if the default branch is not protected\n\n"
        "Suggested Fact Retriever: githubRepositoryFactRetriever\n\n"
        "Suggested Category: Security\n\n"
        "Suggested Maturity Tier: Bronze\n\n",
    )
    assert _confirmation(events)["example"] is True
    assert len(_save_cards(events)) == 1
    done = json.loads(events[-1])
    assert done["type"] == "done" and done["message"] == draft


async def test_without_the_examples_fact_the_model_drafts_it(monkeypatch):
    seen = {}
    scope = {
        **_EXAMPLE_SCOPE,
        "available_fact_retrievers": [{"id": "r", "facts": [{"name": "githubVisibility"}]}],
    }

    await _turn(monkeypatch, [{"role": "user", "content": _EXAMPLE}], _DRAFTED, scope=scope, seen=seen)

    assert "messages" in seen  # the model drafted it


async def test_a_failing_example_card_falls_back_to_the_model(monkeypatch):
    calls = []

    async def _fails_first(message, _scope, **_kwargs):
        calls.append(message)
        if len(calls) == 1:
            raise KeyError("valid")
        return None

    asked = []

    async def _fake_build_agent(_token, _scope, **_kwargs):
        return object(), [], {}

    async def _reply(_agent, messages, parser, **_kwargs):
        asked.append(messages)
        parser.set_message(_DRAFTED)
        return
        yield  # makes this an async generator

    monkeypatch.setattr(orchestrator, "build_confirmation", _fails_first)
    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _reply)
    events = [
        json.loads(ev)
        async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": _EXAMPLE}],
            token="t", user_sub="u", scope=_EXAMPLE_SCOPE, backstage_token="b",
        )
    ]

    assert len(asked) == 1  # the model drafted it
    assert events[-1] == {"type": "done", "message": _DRAFTED}


def test_only_the_examples_own_words_are_drafted_by_code():
    asked = [{"role": "user", "content": _EXAMPLE}]
    tested = [
        *asked,
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
    ]

    assert orchestrator._example_draft(asked, _EXAMPLE_SCOPE)
    assert orchestrator._example_draft(tested, _EXAMPLE_SCOPE) is None
    assert orchestrator._example_draft(asked, {**_EXAMPLE_SCOPE, "experience": "experienced"}) is None
    assert orchestrator._example_draft(asked, {**_EXAMPLE_SCOPE, "check_id": "c1"}) is None


# ── The test picker's test, run by code ──────────────────────────────

_PICKED = "Test it on component:default/order-api"
_PRIVATE = {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]}
_PRIVATE_DRAFT = (
    "Name: Repo must be private\n\n"
    "Description: Fails if the repo is public\n\n"
    "Suggested Fact Retriever: githubRepositoryFactRetriever\n\n"
    "Rule (json-rules-engine conditions)\n```json\n"
    + json.dumps({"conditions": _PRIVATE}) + "\n```"
)
_FAILED_RUN = {
    "status": "failed", "passed": False, "summary": "…", "reasons": [],
    "conditions": _evaluated({"githubVisibility": "public"}),
}
_SAVED_SCOPE = {
    **_EDIT_SCOPE,
    "check_name": "Repository must have a recent push",
    "check_description": "Fails if there was no push in 30 days",
    "selected_fact_ids": ["githubRepositoryFactRetriever"],
}


async def _picked_turn(monkeypatch, chat, scope, result=_FAILED_RUN, error=None):
    """A turn whose dry-run-check code runs; returns (events, the tool calls,
    the replies cards were built from, whether the model was asked)."""
    calls, built, model = [], [], []

    async def _fake_build_agent(*_args, **_kwargs):
        model.append(True)
        return object(), [], {}

    async def _fake_tools(_auth):
        return {}

    async def _fake_call(_tools, name, args):
        calls.append((name, args))
        if error:
            raise BackstageToolError(error)
        return result

    async def _fake_confirmation(message, _scope, **_kwargs):
        built.append(message)
        # Like the real card: it saves the reply's rule, the rule code tested.
        rule = confirm_card._edited_rule(message)
        return {**_CARD, "kind": "create", "new_rule": json.dumps(rule)}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "backstage_tools_as_user", _fake_tools)
    monkeypatch.setattr(orchestrator, "call_backstage_tool", _fake_call)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    events = [
        ev async for ev in orchestrator.stream_chat(
            messages=chat, token="t", user_sub="u", scope=scope, backstage_token="b",
        )
    ]
    return events, calls, built, bool(model)


async def test_the_pickers_test_of_a_draft_runs_by_code_with_no_model_call(monkeypatch):
    # 2026-11-24: the rule and the component are known; the model only ran the tool.
    chat = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        {"role": "user", "content": _PICKED},
    ]

    events, calls, built, model = await _picked_turn(
        monkeypatch, chat, {"case_type": "check_authoring"},
    )

    assert not model
    assert calls == [("tech-insights__dry-run-check", {
        "entityRef": "component:default/order-api",
        "check": {
            "name": "Repo must be private",
            "description": "Fails if the repo is public",
            "factIds": ["githubRepositoryFactRetriever"],
            "rule": {"conditions": _PRIVATE},
        },
    })]
    steps = [json.loads(ev) for ev in events if '"tool_call"' in ev]
    assert [ev["activeForm"] for ev in steps] == [
        "Testing the check on order-api", "Preparing the save card",
    ]
    assert any('"dry_run_result"' in ev for ev in events)
    # The draft's save card again, tested: what's left is to save it.
    assert built == [_PRIVATE_DRAFT]
    assert _confirmation(events)["tested"] is True
    (card,) = _save_cards(events)
    assert _components(card)[0]["resultTitle"] == "Tested on order-api"
    assert _questions(events) == []
    done = json.loads(events[-1])
    assert done["type"] == "done"
    assert done["message"].startswith(
        "Tested on component:default/order-api\n\norder-api doesn't meet this rule.",
    )
    assert "```" not in done["message"]  # so the draft is still the last rule


async def test_the_pickers_test_of_the_saved_check_shows_the_test_card(monkeypatch):
    passed = {
        "status": "passed", "passed": True, "summary": "…", "reasons": [],
        "conditions": _evaluated({"githubDaysSinceLastPush": 3}),
    }

    events, calls, built, model = await _picked_turn(
        monkeypatch, [{"role": "user", "content": _PICKED}], _SAVED_SCOPE, result=passed,
    )

    assert not model
    ((_name, args),) = calls
    assert args["check"]["name"] == "Repository must have a recent push"
    assert args["check"]["factIds"] == ["githubRepositoryFactRetriever"]
    assert args["check"]["rule"] == {"conditions": json.loads(_EDIT_SCOPE["current_rule"])}
    assert built == [] and not _has_card(events)  # a test saves nothing
    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["status"]) == ("TestCard", "passed")


async def test_the_pickers_test_of_the_example_stays_practice(monkeypatch):
    draft = orchestrator.example_draft(_EXAMPLE_SCOPE)
    chat = [
        {"role": "user", "content": _EXAMPLE},
        {"role": "assistant", "content": draft},
        {"role": "user", "content": _PICKED},
    ]

    events, _calls, built, model = await _picked_turn(monkeypatch, chat, _EXAMPLE_SCOPE)

    assert not model and built == [draft]
    confirmation = _confirmation(events)
    assert confirmation["tested"] is True and confirmation["example"] is True
    note = _components(_save_cards(events)[0])[0]["note"]
    assert "That's what a check is for" in note


async def test_the_pickers_test_that_fails_shows_the_problem_card_at_once(monkeypatch):
    chat = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        {"role": "user", "content": "Test it on component:default/nope"},
    ]

    events, calls, built, model = await _picked_turn(
        monkeypatch, chat, {"case_type": "check_authoring"},
        error="No entity component:default/nope in the catalog",
    )

    assert not model and len(calls) == 1  # no retry: there's no input to fix
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["title"]) == ("NoticeCard", "Couldn't test the check")
    assert root["summary"] == "nope isn't in the catalog. Check the component's name."
    done = json.loads(events[-1])
    assert done["message"] == (
        "Couldn't test the check: nope isn't in the catalog. Check the component's name."
    )


_NO_KIND_SCOPE = {
    "case_type": "check_authoring",
    # Not limited to any kind: a bare name is a component.
    "available_fact_retrievers": [{"id": "githubRepositoryFactRetriever", "facts": []}],
}


def test_a_test_request_code_can_read_is_run_by_code():
    # 2026-11-25: typed tests too, so every result is a card.
    drafted = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
    ]

    def ref(text, scope=_NO_KIND_SCOPE, chat=drafted):
        test = orchestrator._code_test([*chat, {"role": "user", "content": text}], scope)
        return test and test[0]

    # The picker's full ref, and a test typed in your own words.
    assert ref(_PICKED) == "component:default/order-api"
    assert ref("test it on order-api") == "component:default/order-api"
    assert ref("Try it on payments/order-api.") == "component:payments/order-api"
    assert ref("dry run on api:default/orders") == "api:default/orders"
    # Anything more than a test, or no name to test on: the model reads it.
    assert ref(f"{_PICKED} and make it public") is None
    assert ref("try it for 60") is None
    # A bare name is a component only when the check's facts are for components.
    def kinds(*names):
        return {**_NO_KIND_SCOPE, "available_fact_retrievers": [
            {"id": "githubRepositoryFactRetriever", "kinds": list(names), "facts": []},
        ]}
    assert ref("test it on order-api", kinds("Component")) == "component:default/order-api"
    assert ref("test it on order-api", kinds("System")) is None
    assert ref("Test it on system:default/shop", kinds("System")) == "system:default/shop"
    # A draft without a retriever, or nothing to test: the model reads it too.
    no_retriever = [drafted[0], {"role": "assistant", "content": _DRAFTED}]
    assert ref(_PICKED, chat=no_retriever) is None
    assert ref(_PICKED, chat=[]) is None


async def test_a_typed_test_runs_by_code_with_no_model_call(monkeypatch):
    chat = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        {"role": "user", "content": "test it on order-api"},
    ]

    events, calls, built, model = await _picked_turn(monkeypatch, chat, _NO_KIND_SCOPE)

    assert not model
    ((_name, args),) = calls
    assert args["entityRef"] == "component:default/order-api"
    assert built == [_PRIVATE_DRAFT]
    assert _confirmation(events)["tested"] is True
    assert len(_save_cards(events)) == 1


class _NoReplyAfterTestAgent(_DryRunAgent):
    """Runs the dry-run like _DryRunAgent, then the reply fails (``fail``)
    or never comes."""

    def __init__(self, result, fail):
        super().__init__("", result)
        self._fail = fail

    async def astream(self, agent_input, **kwargs):
        async for chunk in super().astream(agent_input, **kwargs):
            if chunk[0].type == "AIMessageChunk" and chunk[0].content_blocks:
                if self._fail:
                    raise RuntimeError("Recursion limit of 25 reached")
                return
            yield chunk


@pytest.mark.parametrize("fail", [True, False], ids=["the reply fails", "no reply comes"])
async def test_a_test_result_is_still_a_card_when_the_reply_fails(monkeypatch, fail):
    # 2026-11-25: a test that ran is never lost to an error or plain text.
    agent = _NoReplyAfterTestAgent({
        "status": "failed", "passed": False, "summary": "…", "reasons": [],
        "conditions": _evaluated({"githubBranchProtected": True, "githubRequiredApprovingReviewCount": 1}),
    }, fail)

    async def _fake_build_agent(*_args, **_kwargs):
        return agent, [], {}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    # An edit chat without its facts in the scope: the model runs the test.
    events = [
        ev async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": "test it on order-api"}],
            token="t", user_sub="u", scope=_BRANCH_SCOPE, backstage_token="b",
        )
    ]

    assert agent.inputs  # the model ran the test
    assert not any('"type": "error"' in ev for ev in events)
    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["status"]) == ("TestCard", "failed")
    done = json.loads(events[-1])
    assert done["type"] == "done"
    assert done["message"].startswith("Tested on component:default/order-api")


async def test_a_check_tested_as_it_is_drafted_gets_a_tested_save_card(monkeypatch):
    drafted = (
        "**Tested on** `component:default/order-api`\n"
        "- Actual facts: `githubBranchProtected` = `true`\n"
        "- Dry-run result: **Failed** — it has 1 approval\n\n" + _DRAFTED
    )

    events, seen, built = await _tested_turn(
        monkeypatch,
        [{"role": "user", "content": "Repos need a push in 30 days, test it on order-api"}],
        scope={"case_type": "check_authoring"},
        message=drafted,
    )

    assert seen["edit_phase"] is None
    assert built == [drafted]
    assert _confirmation(events)["tested"] is True
    # The result once, as the card's rows, not in the model's words as well.
    root = _components(_save_cards(events)[0])[0]
    assert root["resultTitle"] == "Tested on order-api"
    assert "Dry-run result" not in root.get("note", "")


async def test_a_card_whose_rule_differs_from_the_tested_one_is_not_tested(monkeypatch):
    # The model tested one rule, then drafted another: the result isn't this
    # rule's, so the card offers to test it instead of showing that result.
    drafted = (
        "**Tested on** `component:default/order-api`\n"
        "- Dry-run result: **Failed** — it has 1 approval\n\n" + _DRAFTED
    )
    other = json.dumps({"all": [{"fact": "githubBranchProtected", "operator": "equal", "value": False}]})

    events, _seen, _built = await _tested_turn(
        monkeypatch,
        [{"role": "user", "content": "Repos need a push in 30 days, test it on order-api"}],
        scope={"case_type": "check_authoring"},
        message=drafted,
        card_rule=other,
    )

    assert "tested" not in _confirmation(events)
    root = _components(_save_cards(events)[0])[0]
    assert "resultTitle" not in root
    assert "Dry-run result" not in root.get("note", "")


def test_a_run_matches_the_card_whatever_the_rule_s_layout():
    run = DryRun(status="passed", rule=json.dumps({"conditions": json.loads(_BRANCH_RULE)}))

    assert orchestrator._run_of_card_rule(
        [run], {"new_rule": json.dumps(json.loads(_BRANCH_RULE), indent=2)},
    ) is run
    assert orchestrator._run_of_card_rule([run], {"new_rule": "{}"}) is None
    assert orchestrator._run_of_card_rule([DryRun(status="passed")], {"new_rule": "{}"}) is None
    assert orchestrator._run_of_card_rule([], {"new_rule": "{}"}) is None


async def test_a_test_with_nothing_unsaved_shows_the_test_card(monkeypatch):
    events, seen, built = await _tested_turn(
        monkeypatch, [{"role": "user", "content": "test it on order-api"}],
    )

    assert seen["edit_phase"] == "test"
    assert built == [] and not _has_card(events)  # a test saves nothing
    (event,) = _questions(events)
    by_id = {c["id"]: c for c in _components(event)}
    root = by_id["root"]
    assert (root["component"], root["status"], root["title"], root["subtitle"]) == (
        "TestCard", "failed", "order-api", "Component",
    )
    assert root["summary"] == "order-api doesn't meet this rule."
    needs, has = [by_id[i] for i in root["sections"]]
    assert needs["label"] == "What the rule needs"
    assert [(by_id[i]["label"], by_id[i]["value"]) for i in has["lines"]] == [
        ("Branch protected", "Yes"), ("Required approving review count", "1"),
    ]


async def test_a_tested_edit_keeps_its_save_card(monkeypatch):
    # A second card would replace the save card in the drawer.
    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, {"role": "user", "content": "yes"}],
        _STEP2,
        dry_run=DryRun(status="passed", entity_ref="component:default/order-api"),
    )

    assert len(_save_cards(events)) == 1
    assert _questions(events) == []


# ── A check tool that failed ─────────────────────────────────────────


class _ToolsAgent:
    """Gets each (tool, status, text) result in turn, then answers ``message``."""

    def __init__(self, message, results):
        self._message = message
        self._results = results

    async def astream(self, _agent_input, **_kwargs):
        for i, (tool, status, text) in enumerate(self._results):
            yield SimpleNamespace(
                type="tool", name=tool, status=status, tool_call_id=f"call_{i}",
                content=[{"type": "text", "text": text}],
            ), {}
        yield SimpleNamespace(type="AIMessageChunk", content_blocks=[
            {"type": "text", "text": json.dumps({"message": self._message})},
        ]), {}


async def _tools_turn(monkeypatch, results, reply="I couldn't test it.", confirmation=None):
    async def _fake_build_agent(_token, _scope, **_kwargs):
        return _ToolsAgent(reply, results), [], {}

    async def _fake_confirmation(_message, _scope, **_kwargs):
        return confirmation

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    return [
        ev async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": "check it on order-apx"}], token="t",
            user_sub="u", scope={"case_type": "check_authoring"}, backstage_token="b",
        )
    ]


_NO_ENTITY = ("tech-insights__dry-run-check", "error", "Error: No entity component:default/order-apx in the catalog.")


async def test_a_turn_whose_last_tool_failed_gets_the_problem_card(monkeypatch):
    events = await _tools_turn(monkeypatch, [_NO_ENTITY])

    (event,) = _questions(events)
    root = _components(event)[0]
    assert (root["component"], root["tone"], root["title"], root["summary"]) == (
        "NoticeCard", "warn", "Couldn't test the check",
        "order-apx isn't in the catalog. Check the component's name.",
    )


async def test_an_error_the_model_got_past_gets_no_problem_card(monkeypatch):
    events = await _tools_turn(monkeypatch, [_NO_ENTITY, ("tech-insights__list-checks", "success", "[]")])

    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "MessageCard"


async def test_a_drafted_check_keeps_its_save_card_after_a_failed_test(monkeypatch):
    events = await _tools_turn(
        monkeypatch, [_NO_ENTITY], reply=_STEP2, confirmation=_CARD,
    )

    assert len(_save_cards(events)) == 1
    assert _questions(events) == []


class _StepsAgent:
    """Runs each ("tool", name) call and sends each ("text", piece) of its answer, in turn."""

    def __init__(self, steps):
        self._steps = steps

    async def astream(self, _input, **_kwargs):
        for i, (kind, value) in enumerate(self._steps):
            if kind == "tool":
                yield SimpleNamespace(type="AIMessageChunk", content_blocks=[
                    {"type": "tool_call_chunk", "name": value, "args": "", "index": 0, "id": f"call_{i}"},
                ]), {}
                yield SimpleNamespace(
                    type="tool", name=value, status="success", tool_call_id=f"call_{i}",
                    content=[{"type": "text", "text": "{}"}],
                ), {}
            else:
                yield SimpleNamespace(type="AIMessageChunk", content_blocks=[
                    {"type": "text", "text": value},
                ]), {}


async def _labels(steps, *, stream_text, writing_label=None):
    """The working labels a turn sends, in order."""
    events = [
        json.loads(ev)
        async for ev in orchestrator._yield_chunks(
            _StepsAgent(steps), [], ChatResponseParser(), stream_text=stream_text,
            writing_label=writing_label,
        )
    ]
    return [ev["activeForm"] for ev in events if ev["type"] == "tool_call"]


_ANSWER = json.dumps({"message": "Done."})


def test_check_tools_get_plain_labels():
    assert orchestrator._active_form("tech-insights__validate-check") == "Checking the rule"
    assert orchestrator._active_form("tech-insights__dry-run-check") == "Testing the check"
    # Other tools are still named from their own names.
    assert orchestrator._active_form("list_components") == "Listing components"


async def test_a_held_back_reply_says_when_the_model_starts_writing():
    steps = [("tool", "tech-insights__validate-check"), ("text", _ANSWER[:6]), ("text", _ANSWER[6:])]

    # Between the tool's result and the writing, the model is thinking.
    assert await _labels(steps, stream_text=False) == [
        "Checking the rule", "Thinking", "Writing the reply",
    ]


async def test_writing_is_said_again_after_a_tool_in_between():
    steps = [("text", "{"), ("tool", "tech-insights__dry-run-check"), ("text", _ANSWER[1:])]

    assert await _labels(steps, stream_text=False) == [
        "Writing the reply", "Testing the check", "Thinking", "Writing the reply",
    ]


async def test_a_test_step_names_its_component_once_the_model_says_which():
    # 2026-11-25: the step starts before the arguments come, then names it.
    args = json.dumps({"entityRef": "component:default/order-api", "check": {}})

    class _Agent:
        async def astream(self, _input, **_kwargs):
            yield SimpleNamespace(
                type="AIMessageChunk",
                content_blocks=[{
                    "type": "tool_call_chunk", "name": "tech-insights__dry-run-check", "args": "",
                    "index": 0, "id": "call_1",
                }],
                tool_call_chunks=[{"index": 0, "id": "call_1", "name": "tech-insights__dry-run-check", "args": ""}],
            ), {}
            for piece in (args[:20], args[20:]):
                yield SimpleNamespace(
                    type="AIMessageChunk", content_blocks=[],
                    tool_call_chunks=[{"index": 0, "id": None, "name": None, "args": piece}],
                ), {}

    events = [
        json.loads(ev) async for ev in orchestrator._yield_chunks(
            _Agent(), [], ChatResponseParser(), stream_text=False,
        )
    ]

    assert [ev["activeForm"] for ev in events if ev["type"] == "tool_call"] == [
        "Testing the check", "Testing the check on order-api",
    ]


def test_a_test_step_names_the_component_when_known():
    assert orchestrator._testing_label("component:default/order-api") == (
        "Testing the check on order-api"
    )
    assert orchestrator._testing_label(None) == "Testing the check"


async def test_a_streamed_reply_gets_no_writing_label():
    # The drawer shows the streamed text itself.
    steps = [("tool", "tech-insights__validate-check"), ("text", _ANSWER)]

    assert await _labels(steps, stream_text=True) == ["Checking the rule"]


async def test_a_dry_run_result_is_sent_as_soon_as_it_comes_back(monkeypatch):
    result = json.dumps({"status": "failed", "reasons": [], "conditions": _evaluated({"githubBranchProtected": False})})

    events = [json.loads(ev) for ev in await _tools_turn(monkeypatch, [("tech-insights__dry-run-check", "success", result)])]

    kinds = [ev.get("tool", ev["type"]) for ev in events]
    # Before the model even starts writing the reply.
    assert kinds.index("dry_run_result") < kinds.index("write_reply") < kinds.index("done")
    assert events[kinds.index("dry_run_result")]["status"] == "failed"


@pytest.mark.parametrize(("edit_phase", "scope", "label"), [
    (None, {"case_type": "check_authoring"}, "Drafting the check"),
    ("propose", _EDIT_SCOPE, "Working out the change"),
    ("apply", _EDIT_SCOPE, "Writing the change"),
    ("test", {"case_type": "check_authoring"}, "Writing up the test"),
    (None, {"case_type": "build_failure"}, "Writing the reply"),
])
def test_the_last_step_is_named_by_the_request(edit_phase, scope, label):
    assert orchestrator._writing_label(edit_phase, scope) == label


async def test_a_new_check_shows_only_its_own_steps(monkeypatch):
    # Nothing was tested, so no test step: drafting the check, then code's
    # own step for its save card.
    events = [
        json.loads(ev) for ev in await _tools_turn(
            monkeypatch, [("tech-insights__validate-check", "success", '{"valid": true}')], reply=_DRAFTED,
        )
    ]

    assert [ev["activeForm"] for ev in events if ev["type"] == "tool_call"] == [
        "Thinking",
        "Drafting the check",
        "Preparing the save card",
    ]
    assert not any(ev["type"] == "dry_run_result" for ev in events)


# ── "add … too": the proposal keeps the current conditions ───────────

_PROTECTED_SCOPE = {
    **_EDIT_SCOPE,
    "current_rule": json.dumps({"all": [
        {"fact": "githubBranchProtected", "operator": "equal", "value": True},
    ]}),
}
_ADD_PRIVATE = [
    {"role": "user", "content": "Make this check stricter"},
    {"role": "assistant", "content": "What should change? The rule checks that the branch is protected."},
    {"role": "user", "content": "add repo must be private too"},
]
_ONLY_PRIVATE = (
    "**What I understood:** You want the repository to be private.\n\n"
    "**Proposed rule:** `githubVisibility` `equal` `private`\n\nShall I change it to that?"
)
_BOTH = (
    "**What I understood:** You want to add a private-repository requirement.\n\n"
    "**Proposed rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubVisibility` `equal` `private`\n\n"
    "Shall I add it, so the default branch must be protected and the repository must be private?"
)


@pytest.mark.parametrize(("text", "adds"), [
    ("add repo must be private too", True),
    ("also check that the repo is private", True),
    ("make it private as well", True),
    ("make it private instead", False),
    ("remove branch protection and add visibility", False),
    ("make it stricter", False),
])
def test_asks_to_add(text, adds):
    assert orchestrator._asks_to_add(text) is adds


async def test_an_added_condition_keeps_the_current_ones(monkeypatch):
    # The model dropped branch protection: one round to keep it and ask.
    agent = _RewriteAgent(reply=_BOTH)

    events, _built = await _turn(
        monkeypatch, _ADD_PRIVATE, _ONLY_PRIVATE, scope=_PROTECTED_SCOPE, agent=agent,
    )

    (history,) = agent.calls
    assert history[-2] == {"role": "assistant", "content": _ONLY_PRIVATE}
    assert history[-1]["content"] == orchestrator._KEEP_CONDITIONS_NOTE
    assert _done_message(events) == _BOTH


async def test_a_rewrite_that_still_drops_a_condition_is_not_used(monkeypatch):
    agent = _RewriteAgent(reply=_ONLY_PRIVATE.replace("Shall I", "Should I"))

    events, _built = await _turn(
        monkeypatch, _ADD_PRIVATE, _ONLY_PRIVATE, scope=_PROTECTED_SCOPE, agent=agent,
    )

    assert len(agent.calls) == 1
    assert _done_message(events) == _ONLY_PRIVATE


async def test_a_replacement_the_user_asked_for_is_left_alone(monkeypatch):
    agent = _RewriteAgent(reply=_BOTH)
    chat = [*_ADD_PRIVATE[:2], {"role": "user", "content": "make it private instead"}]

    events, _built = await _turn(
        monkeypatch, chat, _ONLY_PRIVATE, scope=_PROTECTED_SCOPE, agent=agent,
    )

    assert agent.calls == []
    assert _done_message(events) == _ONLY_PRIVATE


# ── Ideas or an explanation in an edit chat ──────────────────────────

_IDEAS = (
    "This check only asks for branch protection, so a branch with no reviews still passes. "
    "Two ways to make it stronger:\n"
    "1. **Proposed rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredApprovingReviewCount` `greaterThanInclusive` `2`\n"
    "2. **Proposed rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredStatusChecks` `equal` `true`\n\nWhich would you like?"
)
_IDEAS_CARD = {
    "title": "Ways to improve this check",
    "summary": "It only asks for branch protection, so a branch with no reviews still passes.",
    "choices": [
        {"label": "Require 2 approvals", "message": "Also require 2 approving reviews"},
        {"label": "Require status checks", "message": "Also require status checks"},
    ],
}


async def test_suggestions_for_an_edit_keep_the_models_own_card(monkeypatch):
    # The case from the live test: the suggestions were hidden behind the
    # generic "What should change?" form.
    seen = {}
    chat = [{"role": "user", "content": "Suggest ways to improve this check"}]

    events, built = await _turn(
        monkeypatch, chat, _IDEAS, scope=_PROTECTED_SCOPE, seen=seen, card=_IDEAS_CARD,
    )

    assert seen["edit_phase"] == "discuss"
    assert seen["messages"][-1]["content"].endswith(orchestrator._DISCUSS_NOTE)
    assert built == [] and not _has_card(events)  # nothing to save yet
    (event,) = _questions(events)
    assert _components(event)[0]["component"] == "ReplyCard"
    assert [
        c["action"]["event"]["userMessage"] for c in _components(event) if c["component"] == "PickRow"
    ] == ["Also require 2 approving reviews", "Also require status checks"]


def test_an_ideas_turn_writes_the_answer():
    assert orchestrator._writing_label("discuss", _EDIT_SCOPE) == "Writing the answer"


@pytest.mark.parametrize("case_type", ["build_failure", "runtime_debug", None])
def test_other_chats_get_no_ideas_phase(case_type):
    chat = [{"role": "user", "content": "Why did it fail? Suggest a fix"}]
    assert orchestrator._edit_phase(chat, {"case_type": case_type}) is None


@pytest.mark.parametrize(("latest", "label"), [
    ("Help me write a security check", "Writing the answer"),
    ("Which facts can I use?", "Writing the answer"),
    ("Repos must be private", "Drafting the check"),
])
def test_a_new_check_turn_is_named_by_what_it_asks(latest, label):
    assert orchestrator._writing_label(None, {"case_type": "check_authoring"}, latest) == label


# ── The welcome of a check authoring chat ────────────────────────────

_WELCOME_CARD = {
    "title": "I can help you write checks",
    "summary": "Tell me what to enforce in plain words; I'll turn it into a rule.",
    "choices": [
        {"label": "A security check", "message": "Help me write a security check"},
        {"label": "Repos private", "message": "Repos must be private"},
    ],
}


_AREAS_CARD = {
    "title": "What you can check",
    "summary": "Pick an area.",
    "choices": [
        {"label": area, "message": area}
        for area in ("Code access", "Repository hygiene", "Code scanning")
    ],
}


async def test_a_familiar_users_welcome_lists_only_the_areas(monkeypatch):
    # 2026-11-25: area names only (no facts or descriptions), as a plain
    # answer with nothing to click, in fixed words, the question last.
    seen = {}

    events, built = await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "Welcome!",
        scope={"case_type": "check_authoring", "welcome": True, "experience": "experienced"},
        seen=seen, card=_AREAS_CARD,
    )

    assert seen["edit_phase"] == "welcome"  # no tools
    (note,) = seen["messages"]
    assert "just opened the check assistant and picked how they want to work" in note["content"]
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    components = _components(event)
    root = components[0]
    assert root["title"] == "Areas you can check"
    assert root["summary"] == orchestrator._AREAS_SUMMARY
    assert root["summary"].startswith("Here are some different areas you can write checks for.")
    # The question at the end doesn't mention facts.
    assert root["summary"].endswith(
        "Would you like to pick one of these areas, or do you have another check in mind?",
    )
    (section,) = [c for c in components if c["component"] == "ReplySection"]
    assert section["label"] == "Areas"
    assert [(c["label"], c["value"]) for c in components if c["component"] == "ReplyLine"] == [
        ("", "Code access"), ("", "Repository hygiene"), ("", "Code scanning"),
    ]
    # Nothing to click: the user answers in the message box.
    assert root["choices"] == [] and root["answer"] == []
    assert not any(c["component"] in ("PickRow", "AskField") for c in components)
    done = json.loads(events[-1])
    assert done["message"].startswith("Areas you can check")
    assert "- Code access\n" in done["message"]


def test_the_welcomes_areas_are_short_and_each_named_once():
    many = {"choices": [
        {"label": name, "message": name}
        for name in ("Code access", "code access", "A" * 41, "B", "C", "D", "E", "F", "G")
    ]}
    assert orchestrator._welcome_areas(many) == ["Code access", "B", "C", "D", "E", "F"]
    # No choices: the section labels the model used instead.
    assert orchestrator._welcome_areas({"sections": [{"label": "Code scanning"}]}) == [
        "Code scanning",
    ]
    assert orchestrator._welcome_areas({"title": "x"}) == []


@pytest.mark.parametrize(("said", "noted"), [
    ("Code access", True),
    ("code access.", True),
    ("Show me the code access facts", True),
    ("show me code access", True),
    ("code access area", True),
    ("Repos must be private", False),
    # A rule that mentions the area is drafted as usual.
    ("code access must need 2 reviews", False),
])
async def test_an_area_named_after_the_welcome_gets_its_facts(monkeypatch, said, noted):
    seen = {}
    chat = [
        {"role": "user", "content": "I'm familiar with checks. Show me the available facts."},
        {"role": "assistant", "content": orchestrator._areas_text(["Code access", "Code scanning"])},
        {"role": "user", "content": said},
    ]

    await _turn(
        monkeypatch, chat, "…", scope={"case_type": "check_authoring", "experience": "experienced"},
        seen=seen,
    )

    note = orchestrator._AREA_NOTE.format(area="Code access")
    assert seen["messages"][-1]["content"].endswith(note) is noted


async def test_a_beginners_welcome_is_fixed_words_with_no_model_call(monkeypatch):
    # 2026-11-24: the prompt fixed nearly every word, and the model still
    # changed some, so code sends the card at once.
    seen = {}

    events, built = await _turn(
        monkeypatch,
        [{"role": "user", "content": "I'm new to checks. Walk me through it, with examples."}],
        "Welcome!",
        scope={"case_type": "check_authoring", "welcome": True, "experience": "beginner"},
        seen=seen, card=_WELCOME_CARD,
    )

    assert seen == {}  # the model was never asked
    assert built == [] and not _has_card(events)
    (event,) = _questions(events)
    components = _components(event)
    assert components[0]["title"] == "Let's write your first check"
    assert components[0]["summary"] == (
        "A check is a rule every service has to pass. For example: the default branch must be "
        "protected. We'll go one step at a time. Need any clarification, or tell me what you'd "
        "like your check to make sure of?"
    )
    assert [(c["label"], c["value"]) for c in components if c["component"] == "ReplyLine"] == [
        ("Describe it", "In your own words"),
        ("I draft the rule", "You see it before anything is saved"),
        ("Test it", "On one of your components"),
        ("Save it", "As a draft, or publish it"),
    ]
    assert [
        c["action"]["event"]["userMessage"] for c in components if c["component"] == "PickRow"
    ] == [_EXAMPLE]
    done = json.loads(events[-1])
    assert done["type"] == "done" and done["message"].startswith("Let's write your first check")


@pytest.mark.parametrize("scope", [
    {"case_type": "check_authoring", "experience": "experienced"},
    {**_EDIT_SCOPE, "experience": "experienced"},
])
def test_the_welcome_note_asks_for_plain_words(scope):
    # Words like "creative" and "lively" made the openings read as generated.
    note = orchestrator._welcome_note(scope)
    assert 'see "How to write"' in note
    for word in ("creative", "lively", "warm", "inviting", "plain-language", "runnable"):
        assert word not in note


def test_an_edit_chat_welcome_is_about_the_check():
    note = orchestrator._welcome_note({**_EDIT_SCOPE, "experience": "experienced"})
    assert "to change this check" in note
    assert "what this check checks now" in note


async def test_other_chats_ignore_the_welcome_flag(monkeypatch):
    seen = {}

    await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "Hello",
        scope={"case_type": "build_failure", "welcome": True}, seen=seen,
    )

    assert seen["edit_phase"] is None
    assert seen["messages"][-1]["content"].endswith("Hi")


async def test_a_welcome_written_only_in_its_card_is_shown(monkeypatch):
    # The model often leaves `message` empty when the note says "in `card`".
    # With no areas named, its own card is shown.
    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "",
        scope={"case_type": "check_authoring", "welcome": True, "experience": "experienced"},
        card={"title": _WELCOME_CARD["title"], "summary": _WELCOME_CARD["summary"]},
    )

    assert not any('"type": "error"' in ev for ev in events)
    (event,) = _questions(events)
    assert _components(event)[0]["title"] == "I can help you write checks"
    done = json.loads(events[-1])
    assert done["type"] == "done" and done["message"].startswith("I can help you write checks")


@pytest.mark.parametrize(("experience", "words"), [
    ("experienced", [
        "`choices`: 5 or 6 different areas",
        "just the area's name",
        "No facts, fact names, descriptions or example checks",
    ]),
])
def test_the_welcome_suits_the_users_experience(experience, words):
    # Each level gets its own card, not the same one in other words.
    scope = {"case_type": "check_authoring", "experience": experience}
    note = orchestrator._welcome_note(scope)
    assert all(w in note for w in words)


@pytest.mark.parametrize("latest", [
    "Add another condition",
    "add a condition",
    "Make this check stricter",
    "Make this check stricter or add a condition",
    "make it stricter please.",
    "Tighten the rule",
])
def test_a_change_that_names_none_asks_for_ideas(latest):
    chat = [{"role": "user", "content": latest}]
    assert orchestrator._edit_phase(chat, _EDIT_SCOPE) == "discuss"


@pytest.mark.parametrize("latest", [
    "Add another condition: require 2 approving reviews",
    "Make it stricter by requiring status checks",
    "Also require the repo to be private",
    "Improve this check by requiring two approving reviews",
    "Make it better: improve it to require code owner reviews",
])
def test_a_change_that_names_one_is_proposed(latest):
    chat = [{"role": "user", "content": latest}]
    assert orchestrator._edit_phase(chat, _EDIT_SCOPE) == "propose"


def test_ideas_come_with_a_reason_and_an_example():
    assert '"Why"' in orchestrator._DISCUSS_NOTE
    assert '"Example"' in orchestrator._DISCUSS_NOTE


def test_a_new_check_has_no_open_change_phase():
    chat = [{"role": "user", "content": "Add another condition"}]
    assert orchestrator._edit_phase(chat, {"case_type": "check_authoring"}) != "discuss"


async def test_an_edit_chat_welcome_offers_the_drawers_options(monkeypatch):
    # Whatever the model suggests: the same as the drawer's own welcome.
    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "Welcome!",
        scope={**_EDIT_SCOPE, "welcome": True, "experience": "experienced"}, card=_WELCOME_CARD,
    )

    (event,) = _questions(events)
    assert [
        c["action"]["event"]["userMessage"] for c in _components(event) if c["component"] == "PickRow"
    ] == ["Explain this rule"]
    assert "Leave `choices` empty" in orchestrator._welcome_note(
        {**_EDIT_SCOPE, "experience": "experienced"},
    )


async def test_a_welcome_flag_without_a_level_is_an_ordinary_turn(monkeypatch):
    # The opening before a level is the drawer's own: no model welcome.
    seen = {}

    await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "Hello",
        scope={"case_type": "check_authoring", "welcome": True}, seen=seen,
    )

    assert seen["edit_phase"] != "welcome"
    assert seen["messages"][-1]["content"].endswith("Hi")



@pytest.mark.parametrize(("experience", "options"), [
    ("beginner", ["Try an example: the default branch must be protected"]),
    # A familiar user's areas are listed, with nothing to click.
    ("experienced", []),
])
async def test_a_new_check_welcome_offers_the_same_options_for_each_level(
    monkeypatch, experience, options,
):
    # Whatever the model suggests: fixed per level, and "Type your own" too.
    events, _built = await _turn(
        monkeypatch, [{"role": "user", "content": "Hi"}], "Welcome!",
        scope={"case_type": "check_authoring", "welcome": True, "experience": experience},
        card=_WELCOME_CARD,
    )

    (event,) = _questions(events)
    components = _components(event)
    assert [
        c["action"]["event"]["userMessage"] for c in components if c["component"] == "PickRow"
    ] == options
    # "Type your own" only with starting questions; a familiar user types.
    assert any(c["component"] == "AskField" for c in components) == bool(options)


async def test_the_level_message_reaches_the_welcome_note(monkeypatch):
    seen = {}
    level = "I'm familiar with checks. Show me the available facts."

    await _turn(
        monkeypatch, [{"role": "user", "content": level}], "Welcome!",
        scope={"case_type": "check_authoring", "welcome": True, "experience": "experienced"},
        seen=seen, card=_WELCOME_CARD,
    )

    (note,) = seen["messages"]
    assert note["content"].endswith(f'They said: "{level}"')



async def test_a_reply_says_it_drafts_a_check_only_after_checking_a_rule():
    # A question in a new check chat ("Help me create a check using specific
    # facts") is answered, not drafted: no rule was checked.
    answered = [("text", _ANSWER)]
    drafted = [("tool", "tech-insights__validate-check"), ("text", _ANSWER)]

    assert await _labels(answered, stream_text=False, writing_label="Drafting the check") == [
        "Writing the answer",
    ]
    assert await _labels(drafted, stream_text=False, writing_label="Drafting the check") == [
        "Checking the rule", "Thinking", "Drafting the check",
    ]


@pytest.mark.parametrize("answer", ["Explain this rule", "How can I improve this?"])
def test_the_edit_intro_questions_ask_for_ideas_not_a_change(answer):
    chat = [{"role": "user", "content": answer}]
    assert orchestrator._edit_phase(chat, _EDIT_SCOPE) == "discuss"


async def test_explain_this_rule_gets_an_explanation_without_changes(monkeypatch):
    seen = {}
    await _turn(monkeypatch, [{"role": "user", "content": "Explain this rule"}], "It checks …",
                seen=seen, card=_WELCOME_CARD)
    assert seen["edit_phase"] == "discuss"
    assert seen["messages"][-1]["content"].endswith(orchestrator._EXPLAIN_NOTE)


async def test_how_can_i_improve_this_gets_ideas(monkeypatch):
    seen = {}
    await _turn(monkeypatch, [{"role": "user", "content": "How can I improve this?"}], "Ideas …",
                seen=seen, card=_WELCOME_CARD)
    assert seen["messages"][-1]["content"].endswith(orchestrator._DISCUSS_NOTE)


# ── "no fact fits" with nothing to pick instead ──────────────────────

_NO_README_SCOPE = {
    "case_type": "check_authoring",
    "available_fact_retrievers": [{"id": "githubRepositoryFactRetriever", "facts": [
        {"name": "githubHasCodeowners", "description": "Whether the branch has a CODEOWNERS file"},
        {"name": "githubHasSecurityPolicy", "description": "Whether the branch has a SECURITY.md"},
    ]}],
}
_README_CHAT = [{"role": "user", "content": "Repos must have a README"}]
_BARE_NO_FACT = (
    "No available fact measures whether a repo has a README, so I can't build this check yet. "
    "What would you like to check instead?"
)
_NO_FACT_WITH_IDEAS = (
    "No available fact measures whether a repo has a README, so I can't build this check yet. "
    "Checks you could write instead: `githubHasCodeowners` (the repo must have a CODEOWNERS "
    "file) and `githubHasSecurityPolicy` (the repo must have a SECURITY.md file). Would you "
    "like to write one of these, or something else?"
)


async def test_a_no_fact_reply_without_alternatives_is_written_again(monkeypatch):
    agent = _RewriteAgent(reply=_NO_FACT_WITH_IDEAS)

    events, _built = await _turn(
        monkeypatch, _README_CHAT, _BARE_NO_FACT, scope=_NO_README_SCOPE, agent=agent,
    )

    (history,) = agent.calls
    assert history[-2] == {"role": "assistant", "content": _BARE_NO_FACT}
    assert history[-1]["content"] == orchestrator._ALTERNATIVES_NOTE
    assert _done_message(events) == _NO_FACT_WITH_IDEAS


async def test_a_no_fact_reply_with_alternatives_is_left_alone(monkeypatch):
    agent = _RewriteAgent(reply=_BARE_NO_FACT)

    events, _built = await _turn(
        monkeypatch, _README_CHAT, _NO_FACT_WITH_IDEAS, scope=_NO_README_SCOPE, agent=agent,
    )

    assert agent.calls == []
    assert _done_message(events) == _NO_FACT_WITH_IDEAS


async def test_rewrites_that_still_have_no_alternatives_are_not_used(monkeypatch, caplog):
    # Two rounds at most: then the first reply is kept and the card asks what to check.
    invented = _NO_FACT_WITH_IDEAS.replace("githubHasCodeowners", "githubHasReadme").replace(
        "githubHasSecurityPolicy", "githubHasLicense",
    )
    agent = _RewriteAgent(reply=invented)

    events, _built = await _turn(
        monkeypatch, _README_CHAT, _BARE_NO_FACT, scope=_NO_README_SCOPE, agent=agent,
    )

    assert len(agent.calls) == orchestrator._ALTERNATIVES_TRIES == 2
    assert _done_message(events) == _BARE_NO_FACT
    # Each rejected rewrite is logged, by its length: replies aren't logged.
    rejected = [r.getMessage() for r in caplog.records if "no usable alternatives" in r.getMessage()]
    assert len(rejected) == 2 and "githubHasReadme" not in rejected[0]


async def test_a_second_rewrite_can_give_the_alternatives(monkeypatch):
    # The first rewrite names a fact that doesn't exist; the second is right.
    invented = _NO_FACT_WITH_IDEAS.replace("githubHasCodeowners", "githubHasReadme").replace(
        "githubHasSecurityPolicy", "githubHasLicense",
    )
    agent = _RewriteAgent(reply=[invented, _NO_FACT_WITH_IDEAS])

    events, _built = await _turn(
        monkeypatch, _README_CHAT, _BARE_NO_FACT, scope=_NO_README_SCOPE, agent=agent,
    )

    assert len(agent.calls) == 2
    assert _done_message(events) == _NO_FACT_WITH_IDEAS


async def test_a_first_rewrite_that_works_is_the_only_one(monkeypatch):
    agent = _RewriteAgent(reply=_NO_FACT_WITH_IDEAS)

    await _turn(monkeypatch, _README_CHAT, _BARE_NO_FACT, scope=_NO_README_SCOPE, agent=agent)

    assert len(agent.calls) == 1


# ── a drafted new check like a saved one ─────────────────────────────

_NEW_SCOPE = {"case_type": "check_authoring"}
_DRAFT_CHAT = [{"role": "user", "content": "Repos must have a push in the last 30 days"}]
_FOUND = existing_check.ExistingCheck(
    match="same", reason="rule", id="c1", name="Recent push", description="",
    fact_ids=["githubRepositoryFactRetriever"], status="draft",
    rule='{"all":[{"fact":"githubDaysSinceLastPush","operator":"lessThanInclusive","value":30}]}',
)


async def _new_check_turn(monkeypatch, messages, found, scope=_NEW_SCOPE, reply=_DRAFTED):
    """A new check's turn: the model replies ``reply``, a draft gets a create
    card, and ``found`` is the saved check like it. Returns (events, the
    lookups' messages, whether the model ran)."""
    lookups, ran = [], []

    async def _fake_find(_confirmation, messages, **_kwargs):
        lookups.append(messages)
        return found

    async def _fake_build_agent(_token, _scope, **_kwargs):
        return object(), [], {}

    async def _fake_chunks(_agent, _messages, parser, **_kwargs):
        ran.append(True)
        parser.set_message(reply)
        return
        yield  # makes this an async generator

    async def _fake_confirmation(_message, _scope, **_kwargs):
        return {**_CARD, "kind": "create"}

    monkeypatch.setattr(orchestrator, "find_existing_check", _fake_find)
    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    monkeypatch.setattr(orchestrator, "_yield_chunks", _fake_chunks)
    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    monkeypatch.setattr(existing_check, "build_confirmation", _fake_confirmation)
    events = [
        ev async for ev in orchestrator.stream_chat(
            messages=messages, token="t", user_sub="u", scope=scope, backstage_token="b",
        )
    ]
    return events, lookups, bool(ran)


async def test_a_draft_like_a_saved_check_gets_its_card_instead_of_the_save_card(monkeypatch):
    events, lookups, _ran = await _new_check_turn(monkeypatch, _DRAFT_CHAT, _FOUND)

    assert lookups == [_DRAFT_CHAT]
    assert not _has_card(events)
    (event,) = _a2ui(events)
    root = _components(event)[0]
    assert root["component"] == "ExistingCheckCard"
    assert root["title"] == "This check is already a draft"
    assert root["check"]["checkId"] == "c1"
    # The reply is still the draft, for the turns after it.
    assert _done_message(events) == _DRAFTED


async def test_a_draft_like_no_saved_check_gets_the_save_card_as_before(monkeypatch):
    events, lookups, _ran = await _new_check_turn(monkeypatch, _DRAFT_CHAT, None)

    assert len(lookups) == 1
    assert _has_card(events)
    (event,) = _a2ui(events)
    assert _components(event)[0]["component"] == "SaveCard"


@pytest.mark.parametrize("scope, messages", [
    # The welcome's practice example.
    ({**_NEW_SCOPE, "experience": "beginner"}, [{"role": "user", "content": _EXAMPLE}]),
    # An edit chat.
    (_EDIT_SCOPE, [
        {"role": "user", "content": "Make it 60"},
        {"role": "assistant", "content": _STEP1},
        {"role": "user", "content": "yes"},
    ]),
])
async def test_no_lookup_outside_a_new_checks_draft(monkeypatch, scope, messages):
    _events, lookups, _ran = await _new_check_turn(monkeypatch, messages, _FOUND, scope=scope)

    assert lookups == []


async def test_create_a_separate_check_gets_the_save_card_without_the_model(monkeypatch):
    chat = [
        *_DRAFT_CHAT,
        {"role": "assistant", "content": _DRAFTED},
        {
            "role": "user",
            "content": "Create a separate check instead of editing Recent push",
            "action": {"name": "create_separate", "context": {"check": "Recent push"}},
        },
    ]

    events, lookups, ran = await _new_check_turn(monkeypatch, chat, _FOUND)

    assert not ran and lookups == []
    assert _has_card(events)
    (event,) = _a2ui(events)
    assert _components(event)[0]["component"] == "SaveCard"
    assert _done_message(events) == _DRAFTED


async def test_the_save_card_says_the_test_failed_from_the_tools_own_error(monkeypatch):
    # The model's reply, with its own "Tested on" line, doesn't decide it.
    reply = (
        "Tested on component:default/order-apx\n"
        "Dry-run error: InputError: No entity component:default/order-apx in the catalog.\n\n"
        + _STEP2
    )

    events = await _tools_turn(monkeypatch, [_NO_ENTITY], reply=reply, confirmation=_CARD)

    note = _components(_save_cards(events)[0])[0]["note"]
    assert note.startswith(
        "Couldn't test the check: order-apx isn't in the catalog. Check the component's name.",
    )
    assert "Tested on" not in note and "Dry-run" not in note


async def test_a_failed_test_the_model_got_past_adds_no_note(monkeypatch):
    events = await _tools_turn(
        monkeypatch, [_NO_ENTITY, ("tech-insights__list-checks", "success", "[]")], reply=_STEP2, confirmation=_CARD,
    )

    note = _components(_save_cards(events)[0])[0].get("note") or ""
    assert "Couldn't test the check" not in note


def test_save_it_after_the_example_is_still_the_example_only_for_its_save_card():
    beginner = {"case_type": "check_authoring", "experience": "beginner"}
    chat = [
        {"role": "user", "content": _EXAMPLE},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "Test it on component:default/order-api"},
        {"role": "assistant", "content": _DRAFTED},
        {"role": "user", "content": "save it"},
    ]

    assert orchestrator._is_example(chat, beginner, past_save=True)
    # Everywhere else "save it" is not the example's words, as before.
    assert not orchestrator._is_example(chat, beginner)
    assert not orchestrator._is_example(
        [{"role": "user", "content": "Repos must be private"}, *chat[-1:]], beginner, past_save=True,
    )


def test_the_beginner_welcome_has_no_dash_or_ill_sentence():
    # Its exact words: test_a_beginners_welcome_is_fixed_words_with_no_model_call.
    words = orchestrator._card_text(orchestrator._BEGINNER_WELCOME)

    assert "—" not in words and "walk you through" not in words


_GUARDED_DRAFT = (
    "Name: Repository must be private\n\nSuggested Fact Retriever: githubRepositoryFactRetriever\n\n"
    "Rule (json-rules-engine conditions)\n```json\n"
    '{"conditions": {"all": [{"fact": "githubRepositoryResolved", "operator": "equal", '
    '"value": true}, {"fact": "githubVisibility", "operator": "equal", "value": "private"}]}}\n```'
)


async def test_a_new_checks_card_is_built_without_the_guard_nobody_asked_for(monkeypatch):
    chat = [{"role": "user", "content": "Repositories must be private"}]

    events, built = await _turn(
        monkeypatch, chat, _GUARDED_DRAFT, scope={"case_type": "check_authoring"},
    )

    assert "githubRepositoryResolved" not in built[0]
    assert "githubRepositoryResolved" not in _done_message(events)
    assert "githubVisibility" in built[0]


async def test_an_edit_keeps_every_condition_of_the_saved_rule(monkeypatch):
    # Only a new check loses the guard: an edit starts from the saved rule,
    # here one with the guard, changed from public to private and approved.
    scope = {
        **_EDIT_SCOPE,
        "current_rule": json.dumps({"all": [
            {"fact": "githubRepositoryResolved", "operator": "equal", "value": True},
            {"fact": "githubVisibility", "operator": "equal", "value": "public"},
        ]}),
    }
    step1 = (
        "Proposed rule: `githubRepositoryResolved equal true` AND "
        '`githubVisibility equal "private"`. Shall I change it to that?'
    )
    chat = [
        {"role": "user", "content": "Repositories must be private"},
        {"role": "assistant", "content": step1},
        {"role": "user", "content": "yes"},
    ]
    seen = {}

    _events, built = await _turn(monkeypatch, chat, _GUARDED_DRAFT, scope=scope, seen=seen)

    assert seen["edit_phase"] == "apply"
    assert len(built) == 1
    assert "githubRepositoryResolved" in built[0]
    assert '"private"' in built[0]



def test_an_explanation_never_names_a_real_component():
    # 2026-11-25: it named order-api as passing, while its test failed.
    assert "never name a real component or service" in orchestrator._EXPLAIN_NOTE
    assert "one entity that passes" not in orchestrator._EXPLAIN_NOTE


async def test_a_reply_that_fails_validation_is_not_logged(monkeypatch, caplog):
    from langchain.agents.structured_output import StructuredOutputValidationError
    from langchain_core.messages import AIMessage
    from pydantic import BaseModel, ValidationError

    secret = "KEY=hunter2"

    class _Reply(BaseModel):
        message: str

    try:
        _Reply.model_validate({"message": {"text": secret}})
    except ValidationError as exc:
        source = exc

    class _BadAgent:
        async def astream(self, _input, **_kwargs):
            yield SimpleNamespace(type="AIMessageChunk", content_blocks=[]), {}
            raise StructuredOutputValidationError(
                "ChatResponse",
                source,
                AIMessage(content=f'{{"message": "{secret}"}}'),
            )

    async def _fake_build_agent(*_args, **_kwargs):
        return _BadAgent(), [], {}

    monkeypatch.setattr(orchestrator, "_build_agent", _fake_build_agent)
    with caplog.at_level("WARNING"):
        _ = [
            ev
            async for ev in orchestrator.stream_chat(
                messages=[{"role": "user", "content": "hi"}],
                token="t",
                user_sub="u",
            )
        ]

    assert "Structured output validation failed" in caplog.text
    assert "string_type" in caplog.text
    assert secret not in caplog.text


async def test_a_test_run_by_code_takes_a_chat_slot(monkeypatch):
    # It calls Backstage like a model turn, so max_concurrent_chats holds for it.
    import asyncio

    slot = asyncio.Semaphore(1)
    held = []
    monkeypatch.setattr(orchestrator, "_semaphore", slot)

    async def _fake_run(*_args, **_kwargs):
        held.append(slot.locked())
        yield orchestrator._emit_event({"type": "done", "message": "ok"})

    monkeypatch.setattr(orchestrator, "_run_code_test", _fake_run)
    chat = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        {"role": "user", "content": _PICKED},
    ]
    _ = [
        ev async for ev in orchestrator.stream_chat(
            messages=chat, token="t", user_sub="u", scope={"case_type": "check_authoring"},
            backstage_token="b",
        )
    ]

    assert held == [True]
    assert not slot.locked()


async def test_the_beginners_example_takes_a_chat_slot(monkeypatch):
    import asyncio

    slot = asyncio.Semaphore(1)
    held = []
    monkeypatch.setattr(orchestrator, "_semaphore", slot)

    async def _fake_confirmation(_message, _scope, **_kwargs):
        held.append(slot.locked())
        return {**_CARD, "kind": "create"}

    monkeypatch.setattr(orchestrator, "build_confirmation", _fake_confirmation)
    _ = [
        ev async for ev in orchestrator.stream_chat(
            messages=[{"role": "user", "content": _EXAMPLE}], token="t", user_sub="u",
            scope=_EXAMPLE_SCOPE, backstage_token="b",
        )
    ]

    assert held == [True]
    assert not slot.locked()


async def _slot_seen_by(monkeypatch, name, latest):
    """Whether a chat slot was held while ``name`` built its card for ``latest``."""
    import asyncio

    slot = asyncio.Semaphore(1)
    held = []
    monkeypatch.setattr(orchestrator, "_semaphore", slot)

    async def _shortcut(*_args, **_kwargs):
        held.append(slot.locked())
        return None

    async def _no_model(*_args, **_kwargs):
        raise RuntimeError("stop after the shortcuts")

    monkeypatch.setattr(orchestrator, name, _shortcut)
    monkeypatch.setattr(orchestrator, "_build_agent", _no_model)
    chat = [
        {"role": "user", "content": "Repos must be private"},
        {"role": "assistant", "content": _PRIVATE_DRAFT},
        latest if isinstance(latest, dict) else {"role": "user", "content": latest},
    ]
    _ = [
        ev async for ev in orchestrator.stream_chat(
            messages=chat, token="t", user_sub="u", scope={"case_type": "check_authoring"},
            backstage_token="b",
        )
    ]
    return held


@pytest.mark.parametrize(("name", "latest"), [
    ("card_for_save_request", "save it"),
    ("card_for_separate_check", {
        "role": "user",
        "content": "Create a separate check instead of editing Repo is private",
        "action": {"name": "create_separate", "context": {"check": "Repo is private"}},
    }),
])
async def test_a_shortcut_that_calls_backstage_takes_a_chat_slot(monkeypatch, name, latest):
    # Like a model turn, so max_concurrent_chats holds for it.
    assert await _slot_seen_by(monkeypatch, name, latest) == [True]


@pytest.mark.parametrize("name", ["card_for_save_request", "card_for_separate_check"])
async def test_any_other_message_takes_no_slot_for_the_shortcuts(monkeypatch, name):
    # The shortcut returns at once: it mustn't wait for a slot it doesn't need.
    assert await _slot_seen_by(monkeypatch, name, "make it stricter") == [False]


async def _ended_with_test(monkeypatch, card_rule):
    """How _tested_reply ends a turn whose test ran _PRIVATE and whose card saves card_rule."""
    async def _card(_message, _scope, **_kwargs):
        return {**_CARD, "kind": "create", "new_rule": json.dumps(card_rule)}

    monkeypatch.setattr(orchestrator, "build_confirmation", _card)
    run = DryRun(status="failed", entity_ref="component:default/order-api",
                 fact_values={"githubVisibility": "public"}, rule=json.dumps(_PRIVATE))
    return [
        json.loads(ev) async for ev in orchestrator._tested_reply(
            run, _PRIVATE_DRAFT, [{"role": "user", "content": _PICKED}],
            {"case_type": "check_authoring"}, user_sub="u", backstage_token="b",
        )
    ]


async def test_a_test_of_the_cards_rule_ends_in_the_tested_save_card(monkeypatch):
    events = await _ended_with_test(monkeypatch, _PRIVATE)

    (card,) = [e for e in events if e["type"] == "confirmation_required"]
    assert card["tested"] is True


async def test_a_test_of_another_rule_ends_in_the_test_card_not_a_tested_save_card(monkeypatch):
    # The model's test ran one rule and its reply failed: the card of another
    # rule must not show that result as its own.
    other = {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "internal"}]}

    events = await _ended_with_test(monkeypatch, other)

    _assert_ends_in_the_test_result(events)


def _assert_ends_in_the_test_result(events):
    """The turn ends in order-api's own failed result: its test card, no
    save card, and a reply that says it."""
    assert not [e for e in events if e["type"] == "confirmation_required"]
    (ui,) = [e for e in events if e["type"] == "a2ui"]
    root = {c["id"]: c for c in ui["messages"][0]["createSurface"]["components"]}["root"]
    assert (root["component"], root["status"], root["title"]) == ("TestCard", "failed", "order-api")
    assert events[-1]["type"] == "done"
    assert events[-1]["message"].startswith("Tested on component:default/order-api")


def test_a_test_with_other_retrievers_than_the_cards_is_not_its_result():
    # The drawer's copy of the check can be stale: the test ran with its
    # retrievers while the card signs the saved check's.
    rule = json.dumps(_PRIVATE)
    run = DryRun(status="passed", rule=rule, fact_ids=["githubRepositoryFactRetriever"])
    card = {"new_rule": rule, "fact_ids": ["githubRepositoryFactRetriever"]}

    assert orchestrator._run_of_card_rule([run], card) is run
    assert orchestrator._run_of_card_rule([run], {**card, "fact_ids": ["other"]}) is None
    # Order doesn't matter, and a card with none can't differ.
    two = DryRun(status="passed", rule=rule, fact_ids=["a", "b"])
    assert orchestrator._run_of_card_rule([two], {**card, "fact_ids": ["b", "a"]}) is two
    assert orchestrator._run_of_card_rule([run], {"new_rule": rule}) is run
    # A run that doesn't say which retrievers it used isn't the card's result.
    unknown = DryRun(status="passed", rule=rule)
    assert orchestrator._run_of_card_rule([unknown], card) is None


def test_a_test_of_true_is_not_the_result_of_a_card_with_1():
    # Python says True == 1; the rules engine compares strictly.
    tested = {"all": [{"fact": "githubIsPrivate", "operator": "equal", "value": True}]}
    signed = {"all": [{"fact": "githubIsPrivate", "operator": "equal", "value": 1}]}
    run = DryRun(status="passed", rule=json.dumps({"conditions": tested}))

    assert orchestrator._run_of_card_rule([run], {"new_rule": json.dumps(signed)}) is None
    # Key order and spacing don't matter.
    same = json.dumps({"all": [{"value": True, "operator": "equal", "fact": "githubIsPrivate"}]})
    assert orchestrator._run_of_card_rule([run], {"new_rule": same}) is run


def _yes_events(event):
    return {
        c["id"]: c["action"]["event"]
        for c in _components(event)
        if c["component"] in ("Button", "OptionTile")
    }


@pytest.mark.parametrize(
    "value",
    ["Platform — Core", "a – b", "uses `code`", ["x — y", "z"]],
)
async def test_a_question_asked_again_carries_the_written_rule_on_its_yes(monkeypatch, value):
    # The card is built from the rule Step 2 wrote, and its Yes carries it:
    # no words are written for code to read back, whatever the value holds.
    written = {"all": [{"fact": "team", "operator": "equal", "value": value}]}
    scope = {**_EDIT_SCOPE, "current_rule": json.dumps(
        {"all": [{"fact": "team", "operator": "equal", "value": "Platform"}]},
    )}

    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _TWO_CHOICES}, {"role": "user", "content": "1"}],
        _edited(written),
        scope=scope,
        confirmation=_card_of_reply,
    )

    (event,) = _a2ui(events)
    assert _components(event)[0]["component"] == "ChangeCard"
    yes = _yes_events(event)["yes"]
    assert yes["name"] == "confirm_edit"
    assert json.loads(yes["context"]["rule"]) == written


def _clicked_yes(rule):
    return {
        "role": "user",
        "content": "yes",
        "action": {"name": "confirm_edit", "context": {"rule": json.dumps(rule)}},
    }


@pytest.mark.parametrize(("written", "card"), [(_UNDER_60, True), (_AT_MOST_60, False)])
async def test_a_click_is_compared_with_the_rule_it_carries(monkeypatch, written, card):
    # The question's words say "60 or less"; the clicked Yes says "less than
    # 60". The click decides, so no words are read back.
    events, _built = await _turn(
        monkeypatch,
        [*_FIRST, {"role": "assistant", "content": _STEP1}, _clicked_yes(_UNDER_60)],
        _edited(written),
        confirmation=_card_of_reply,
    )

    assert _has_card(events) is card


def test_a_click_with_its_rule_approves_whatever_the_words_say():
    messages = [
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": "Shall I change it? It's hard to say in words."},
        _clicked_yes(_UNDER_60),
    ]

    assert orchestrator._edit_phase(messages, _EDIT_SCOPE) == "apply"


async def test_the_change_cards_buttons_carry_their_rules(monkeypatch):
    events, _built = await _turn(monkeypatch, _FIRST, _TWO_CHOICES)

    (event,) = _a2ui(events)
    tiles = _yes_events(event)
    assert json.loads(tiles["option_1"]["context"]["rule"]) == _AT_MOST_60
    assert json.loads(tiles["option_2"]["context"]["rule"]) == _UNDER_60
    assert "context" not in tiles["no"]


_UNREADABLE_STEP1 = (
    "I could make it `githubDaysSinceLastPush` `lessThanInclusive` `60`.\n"
    "Or `githubDaysSinceLastPush` `lessThan` `60`. Shall I?"
)


async def test_a_yes_to_an_unreadable_proposal_is_told_so(monkeypatch):
    # The model isn't told the user didn't approve: it's told why code asks again.
    seen = {}

    await _turn(
        monkeypatch,
        [
            *_FIRST,
            {"role": "assistant", "content": _UNREADABLE_STEP1},
            {"role": "user", "content": "yes"},
        ],
        _STEP1,
        seen=seen,
    )

    assert seen["edit_phase"] == "propose"
    note = seen["messages"][-1]["content"]
    assert note.endswith(orchestrator._UNREADABLE_NOTE)
    assert orchestrator._NOT_APPROVED_NOTE not in note


def test_the_same_unreadable_proposal_is_asked_about_only_once():
    asked = [
        {"role": "user", "content": "Change it to 60"},
        {"role": "assistant", "content": _UNREADABLE_STEP1},
        {"role": "user", "content": "yes"},
    ]
    assert orchestrator._edit_phase(asked, _EDIT_SCOPE) == "propose"

    # The model wrote it unreadably again: the yes approves it, and the save
    # card shows that rule for the user to check before saving.
    again = [
        *asked,
        {"role": "assistant", "content": _UNREADABLE_STEP1},
        {"role": "user", "content": "yes"},
    ]
    assert orchestrator._edit_phase(again, _EDIT_SCOPE) == "apply"
    assert orchestrator._picked_proposal(again) is None


def test_a_no_to_an_unreadable_proposal_is_not_an_approval():
    assert orchestrator._not_approved_reason(
        [
            {"role": "user", "content": "Change it to 60"},
            {"role": "assistant", "content": _UNREADABLE_STEP1},
            {"role": "user", "content": "no, make it 90"},
        ],
        _EDIT_SCOPE,
    ) == "not an approval"


_TIGHT_CHOICES = (
    "1.**Proposed rule:** `githubDaysSinceLastPush` `lessThanInclusive` `60` — 60 or less\n"
    "2.**Proposed rule:** `githubDaysSinceLastPush` `lessThan` `60` — less than 60\n\n"
    "Reply with 1 or 2."
)


@pytest.mark.parametrize(("answer", "picked"), [("yes", None), ("1", 1), ("2", 2)])
def test_choices_with_no_space_after_the_number_still_need_a_number(answer, picked):
    assert orchestrator._picked_choice(answer, _TIGHT_CHOICES) == picked


async def test_a_test_with_other_retrievers_ends_in_the_test_card(monkeypatch):
    async def _card(_message, _scope, **_kwargs):
        return {**_CARD, "kind": "create", "new_rule": json.dumps(_PRIVATE), "fact_ids": ["saved"]}

    monkeypatch.setattr(orchestrator, "build_confirmation", _card)
    run = DryRun(status="failed", entity_ref="component:default/order-api",
                 fact_values={"githubVisibility": "public"}, rule=json.dumps(_PRIVATE),
                 fact_ids=["drawer-copy"])

    events = [
        json.loads(ev) async for ev in orchestrator._tested_reply(
            run, _PRIVATE_DRAFT, [{"role": "user", "content": _PICKED}],
            {"case_type": "check_authoring"}, user_sub="u", backstage_token="b",
        )
    ]

    _assert_ends_in_the_test_result(events)


@pytest.mark.parametrize("answer", [
    "yes, test it on order-api",
    "Yes. Test it on component:default/order-api",
    "ok and test it on order-api",
    "yes please, then run it on order-api",
])
def test_an_approval_that_also_asks_for_a_test_approves(answer):
    # CodeRabbit: the prompt runs a test asked for "in their confirmation",
    # but such an answer went back to Step 1 with no tools.
    assert _phase(_STEP1, answer) == "apply"


@pytest.mark.parametrize("answer", ["1, then test it on order-api", "the second and test it on order-api"])
def test_a_picked_choice_that_also_asks_for_a_test_approves(answer):
    assert _phase(_TWO_CHOICES, answer) == "apply"


@pytest.mark.parametrize("answer", [
    "yes, but make it 70 and test it on order-api",
    "yes, test it",  # no component named: not a test request
    "test it on order-api",  # a test alone is the test phase, not an approval
    "yes to both, test it on order-api",
])
def test_a_test_request_does_not_turn_other_answers_into_approvals(answer):
    assert _phase(_STEP1, answer) != "apply"
