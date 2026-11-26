# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for reading the parts of an edit reply."""

import json

import pytest

from src.agent.check_reply import (
    RULE_FENCE_RE,
    parse_suggestions,
    reply_words,
    strip_step_labels,
    without_failed_test_lines,
    without_test_summary,
    without_unrequested_guard,
)

_OLD = "Fails if the repository has not had a push in the last 30 days"
_NEW = "Fails if the repository has not had a push in the last 60 days"


@pytest.mark.parametrize(
    "line",
    [
        pytest.param(f"- Description: `{_OLD}` → `{_NEW}`", id="arrow"),
        pytest.param(f"- Description: `{_OLD}` -> `{_NEW}`", id="ascii arrow"),
        pytest.param(f"- Description: `{_OLD}` → `{_NEW}`.", id="period after"),
        pytest.param(f"- Description: `{_OLD}` → **`{_NEW}`**", id="bold"),
        pytest.param(f"- Description: `{_OLD}` `{_NEW}`", id="no arrow"),
        pytest.param(f"- Description: `{_OLD}` ⟶ `{_NEW}`", id="long arrow"),
        pytest.param(f"- Description: `{_OLD}`→`{_NEW}`", id="no spaces"),
        pytest.param(f"- Description: `{_OLD}` => `{_NEW}`", id="fat arrow"),
        pytest.param(f"- Description: `{_OLD}` to `{_NEW}`", id="the word to"),
        pytest.param(f"- Description: `{_OLD}` → `{_NEW}`. (matches the rule)", id="note after"),
        pytest.param(f"- Description: `{_OLD}` → `{_NEW}` (was `30`)", id="note with backticks"),
        pytest.param(f"- **Description:** `{_OLD}` → `{_NEW}`", id="bold label"),
    ],
)
def test_the_suggested_text_is_read_however_the_line_is_written(line):
    assert parse_suggestions(line) == {"Description": (_OLD, _NEW)}


def test_a_single_backticked_text_is_the_suggestion():
    assert parse_suggestions(f"- Description: `{_NEW}`") == {"Description": (None, _NEW)}


def test_plain_text_still_works():
    assert parse_suggestions(f"- Description: {_OLD} → {_NEW}") == {"Description": (_OLD, _NEW)}


@pytest.mark.parametrize(
    "line",
    [
        pytest.param(
            "- Description: Fails if `githubVisibility` isn't private", id="backtick in plain text"
        ),
        pytest.param(f"- Description: {_OLD} => {_NEW}", id="unknown arrow in plain text"),
    ],
)
def test_a_suggestion_that_cant_be_read_is_skipped(line, caplog):
    with caplog.at_level("WARNING"):
        assert parse_suggestions(line) == {}
    assert "Suggestion for Description skipped" in caplog.text


def test_name_and_description_together():
    tail = (
        "Suggested related changes:\n"
        "- Name: `Repository must have a recent push` `Repository must have a push within 60 days`\n"
        f"- Description: `{_OLD}` → `{_NEW}`\n"
    )
    assert parse_suggestions(tail) == {
        "Name": (
            "Repository must have a recent push",
            "Repository must have a push within 60 days",
        ),
        "Description": (_OLD, _NEW),
    }


_QUESTION = "Proposed rule: `a` `equal` `true`. Shall I change it to that?"


@pytest.mark.parametrize(
    "message",
    [
        pytest.param(f"{_QUESTION}\n\nStep 1 — confirm the change", id="at the end"),
        pytest.param(f"Step 1 — confirm the change\n\n{_QUESTION}", id="at the start"),
        pytest.param(f"{_QUESTION}\n\n**Step 1 - confirm the change**", id="bold"),
        pytest.param(f"#### Step 1 — confirm the change\n{_QUESTION}", id="heading"),
        pytest.param(f"Step 2 — the edited rule\n{_QUESTION}", id="step 2"),
    ],
)
def test_step_headings_are_dropped(message):
    assert strip_step_labels(message) == _QUESTION


def test_a_sentence_about_a_step_is_kept():
    message = "In step 1 you picked 3. Shall I change it to that?"
    assert strip_step_labels(message) == message


_RULE = '```json\n{ "conditions": { "all": [] } }\n```'
_TESTED = (
    "**Tested on** `component:default/order-api`\n"
    "- Actual facts: `githubRequiredApprovingReviewCount` = `1`\n"
    "- Dry-run result: **Failed** — it requires 1 approving review, the rule needs 2."
)


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param(
            f"Edited rule:\n\n{_RULE}\n\nSuggested related changes:\n"
            "- Name: `Require 3 approvals` → `Require 2 approvals`\n"
            "- Description: `Fails below 3` → `Fails below 2`",
            id="edit",
        ),
        pytest.param(
            f"Edited rule:\n\n{_RULE}\n\n**Suggested related changes:**\n"
            "- None — the name and description still match the new rule.",
            id="edit, nothing to rename",
        ),
        pytest.param(
            "Name: Require 2 approvals\n\nDescription: Fails below 2.\n\n"
            "Suggested Fact Retriever: githubRepositoryFactRetriever\n\n"
            "Suggested Category: Security\n\nSuggested Maturity Tier: Silver\n\n"
            f"Rule (json-rules-engine conditions)\n{_RULE}",
            id="new check",
        ),
    ],
)
def test_a_drafted_reply_has_no_words_of_its_own(reply):
    assert reply_words(reply) is None


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param(
            f"{_TESTED}\n\nEdited rule:\n\n{_RULE}\n\nSuggested related changes:\n"
            "- None — the name and description still match the new rule.",
            id="edit",
        ),
        pytest.param(
            f"{_TESTED}\n\nName: Require 2 approvals\n\nDescription: Fails below 2.\n\n"
            f"Rule (json-rules-engine conditions)\n{_RULE}",
            id="new check",
        ),
    ],
)
def test_the_test_summary_is_kept(reply):
    assert reply_words(reply) == _TESTED


def test_a_sentence_the_model_added_is_kept():
    reply = f"Edited rule:\n\n{_RULE}\n\nWant me to test it on a component?"

    assert reply_words(reply) == "Want me to test it on a component?"


@pytest.mark.parametrize("lead_in", ["The edited rule:", "Here's the change:", "**New rule:**"])
def test_the_line_leading_into_the_rule_goes_with_it(lead_in):
    reply = f"{lead_in}\n\n{_RULE}\n\nWant me to test it on a component?"

    assert reply_words(reply) == "Want me to test it on a component?"


def test_without_test_summary_drops_the_tested_on_block():
    text = (
        "**Tested on** `component:default/order-api`\n"
        "- Actual facts: `githubVisibility` = `public`\n"
        "- Dry-run result: **Failed** — it is public\n\n"
        "I used the visibility fact."
    )

    assert without_test_summary(text) == "I used the visibility fact."
    assert without_test_summary(text.split("\n\n")[0]) is None
    assert without_test_summary(None) is None


def test_without_failed_test_lines_drops_the_models_error_lines():
    text = (
        "Tested on `component:default/no-such-service`\n"
        "Dry-run error: InputError: No entity component:default/no-such-service in the catalog.\n"
        "**Dry-run result:** Error — not found\n\n"
        "I used the visibility fact."
    )

    assert without_failed_test_lines(text) == "I used the visibility fact."
    assert without_failed_test_lines("Dry-run error: No entity x in the catalog.") is None
    assert without_failed_test_lines(None) is None


_GUARDED = (
    "Name: Recent push\n\nRule (json-rules-engine conditions)\n```json\n"
    '{"conditions": {"all": [{"fact": "githubRepositoryResolved", "operator": "equal", '
    '"value": true}, {"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", '
    '"value": 30}]}}\n```\n\nSuggested Category: Reliability'
)


def test_the_repository_resolved_guard_is_taken_out_of_a_new_checks_rule():
    fixed = without_unrequested_guard(_GUARDED, ["Repos must have a push in the last 30 days"])

    assert "githubRepositoryResolved" not in fixed
    assert '"githubDaysSinceLastPush"' in fixed
    assert fixed.startswith("Name: Recent push") and fixed.endswith(
        "Suggested Category: Reliability"
    )
    # Still one rule block that reads as before.
    assert json.loads(RULE_FENCE_RE.search(fixed).group(1)) == {
        "conditions": {
            "all": [
                {"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", "value": 30},
            ]
        },
    }


def test_an_unwrapped_rule_is_fixed_too():
    bare = _GUARDED.replace('{"conditions": ', "", 1).replace("}}\n```", "}\n```")

    fixed = without_unrequested_guard(bare, ["push in the last 30 days"])

    assert "githubRepositoryResolved" not in fixed
    assert (
        json.loads(RULE_FENCE_RE.search(fixed).group(1))["all"][0]["fact"]
        == "githubDaysSinceLastPush"
    )


@pytest.mark.parametrize(
    "requests",
    [
        ["Repos must be resolved and pushed in the last 30 days"],
        ["Check githubRepositoryResolved too"],
    ],
)
def test_the_guard_stays_when_the_user_asked_for_it(requests):
    assert without_unrequested_guard(_GUARDED, requests) == _GUARDED


def test_other_rules_are_left_as_they_are():
    only_guard = _GUARDED.replace(
        ', {"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", "value": 30}',
        "",
    )
    other_value = _GUARDED.replace(
        '"value": true}, {"fact": "githubDays', '"value": false}, {"fact": "githubDays'
    )
    any_rule = _GUARDED.replace('"all"', '"any"')

    for message in (only_guard, other_value, any_rule, "No rule here", "```json\nnot json\n```"):
        assert without_unrequested_guard(message, ["a request"]) == message
