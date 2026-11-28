# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for reading a check edit reply that asks instead of proposing a rule."""

import json

import pytest

from src.agent.ask_view import AskOption, Editor, ask_view

_RULE = {
    "all": [
        {"fact": "githubBranchProtected", "operator": "equal", "value": True},
        {
            "fact": "githubRequiredApprovingReviewCount",
            "operator": "greaterThanInclusive",
            "value": 3,
        },
    ]
}
_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "current_rule": json.dumps(_RULE),
    "available_fact_retrievers": [
        {
            "id": "r",
            "facts": [
                {"name": "githubBranchProtected", "description": "Branch protected"},
                {
                    "name": "githubRequiredApprovingReviewCount",
                    "description": "Approving reviews required before merging",
                },
            ],
        }
    ],
}

# The replies from the 2026-11-13 live test, word for word.
_AFTER_NO = (
    "**Current rule:** `githubBranchProtected` `equal` `true` AND "
    "`githubRequiredApprovingReviewCount` `greaterThanInclusive` `3` — passes when the branch is "
    "protected and requires 3 or more approving reviews; anything requiring fewer than 3 fails.\n\n"
    "**What I understood:** You rejected changing the required approvals from 3 to 2. Tell me "
    "exactly what change you want instead: do you want a different number (which one?), to "
    "require strictly more than N instead of at-least N, to remove the approvals condition, or "
    "something else? If you mean a number, say it (e.g. “make it 1” or “make it 4”)."
)
_WHICH_OPTION = (
    "I’m not sure which option you’re confirming. Do you mean:\n\n"
    "1. Change the required approving reviews from 3 to 2 (so 2 or more approvals pass),\n"
    "2. Require more than 2 approvals (so 3 or more approvals pass),\n"
    "3. A different number (tell me the exact number), or\n"
    "4. Remove the approvals requirement entirely?\n\n"
    "Reply with 1, 2, 3 (and the number), or 4."
)


def test_an_open_question_shows_the_rule_with_an_editor_per_value():
    view = ask_view(_AFTER_NO, _SCOPE)

    assert view.title == "What should change?"
    assert view.summary == "You rejected changing the required approvals from 3 to 2."
    assert view.current == [
        ("Branch protected", "Yes"),
        ("Approving reviews required before merging", "3 or more"),
    ]
    assert view.editors == [
        Editor(
            "Branch protected",
            "choice",
            "Yes",
            "Change branch protected to {value}",
            remove="Remove the condition: branch protected",
        ),
        Editor(
            "Approving reviews required before merging",
            "number",
            3,
            "Change approving reviews required before merging to {value}",
            "at least",
            "Remove the condition: approving reviews required before merging",
        ),
    ]
    assert view.options == []
    assert view.placeholder == 'Or type it, e.g. "make it 4"'
    assert view.keep == "Keep the current rule"


def test_options_in_words_get_one_option_each():
    view = ask_view(_WHICH_OPTION, _SCOPE)

    assert view.title == "Which change do you mean?"
    assert view.summary == "I’m not sure which option you’re confirming."
    assert view.options == [
        AskOption(
            1,
            "Change the required approving reviews from 3 to 2 (so 2 or more approvals pass)",
            "Change the required approving reviews from 3 to 2 (so 2 or more approvals pass)",
        ),
        AskOption(
            2,
            "Require more than 2 approvals (so 3 or more approvals pass)",
            "Require more than 2 approvals (so 3 or more approvals pass)",
        ),
        # Needs a number: a field that starts at the rule's number.
        AskOption(3, "A different number", "A different number: {value}", 3),
        AskOption(
            4,
            "Remove the approvals requirement entirely",
            "Remove the approvals requirement entirely",
        ),
    ]
    assert (view.current, view.editors) == ([], [])
    assert view.keep == "Keep the current rule"


def test_the_only_condition_can_not_be_removed():
    scope = {**_SCOPE, "current_rule": json.dumps({"all": [_RULE["all"][1]]})}

    (editor,) = ask_view(_AFTER_NO, scope).editors

    assert editor.remove is None


def test_only_numbers_and_yes_no_values_get_an_editor():
    scope = {
        **_SCOPE,
        "current_rule": json.dumps(
            {
                "all": [
                    {
                        "fact": "githubVisibility",
                        "operator": "in",
                        "value": ["private", "internal"],
                    },
                    {"fact": "coverage", "operator": "greaterThanInclusive", "value": 0.8},
                ]
            }
        ),
    }

    view = ask_view(_AFTER_NO, scope)

    assert view.current == [("Visibility", "one of private, internal"), ("Coverage", "0.8 or more")]
    assert view.editors == []
    assert view.placeholder == "Or type the change you want"


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param(
            "**What I understood:** You want to keep the rule as it is. Nothing changes.",
            id="no question",
        ),
        pytest.param(
            "This check passes when the branch is protected. Do you want to change anything?",
            id="not the Step 1 format",
        ),
        pytest.param(
            "To change it yourself:\n1. Open the check\n2. Click Edit\n\nAnything else?",
            id="steps, not options",
        ),
        pytest.param(
            "Do you mean:\n1. a higher number\n3. a lower number",
            id="numbers out of order",
        ),
    ],
)
def test_other_replies_get_no_card(reply):
    assert ask_view(reply, _SCOPE) is None


def test_a_rule_that_can_not_be_shown_exactly_gets_no_open_question_card():
    nested = {"all": [{"any": [_RULE["all"][0], _RULE["all"][1]]}]}

    assert ask_view(_AFTER_NO, {**_SCOPE, "current_rule": json.dumps(nested)}) is None


def test_an_open_question_that_lists_ideas_keeps_its_own_card():
    # The form shows only the current rule's values: listed ideas would be
    # hidden behind it.
    ideas = (
        "**What I understood:** You want ideas to make this check stronger.\n\n"
        "- Also require status checks\n"
        "- Also require signed commits\n\nWhich would you like?"
    )

    assert ask_view(ideas, _SCOPE) is None


def test_an_option_keeps_the_indented_lines_under_it():
    # A button without its second line would send half the answer.
    reply = (
        "Which do you mean?\n\n"
        "1. Require two approving reviews\n"
        "   - Keep the administrator bypass enabled\n"
        "2. Require one approving review,\n"
        "   counted only from code owners\n\n"
        "Reply with 1 or 2."
    )

    view = ask_view(reply, _SCOPE)

    assert [option.message for option in view.options] == [
        "Require two approving reviews; Keep the administrator bypass enabled",
        "Require one approving review, counted only from code owners",
    ]


def test_an_unindented_line_between_options_gets_no_card():
    reply = (
        "Which do you mean?\n"
        "1. Require two approving reviews\n"
        "Keep the administrator bypass enabled\n"
        "2. Require one approving review"
    )

    assert ask_view(reply, _SCOPE) is None


@pytest.mark.parametrize(
    "tail",
    [
        # Unindented text after the last option is part of it.
        "counted only from code owners.",
        "counted only from code owners.\n\nReply with 1 or 2.",
    ],
)
def test_text_after_the_last_option_gets_no_buttons(tail):
    # CodeRabbit: the code-owner restriction was dropped, so the button sent half the answer.
    reply = f"Which do you mean?\n1. Require two approving reviews\n2. Require one approving review,\n{tail}"

    assert ask_view(reply, _SCOPE) is None


@pytest.mark.parametrize(
    "footer",
    [
        "Reply with 1 or 2.",
        "Reply with 1, 2 (and the number), or 3.",
        "Please reply with 1 or 2",
        "Or tell me something else.",
        "Which one do you want?",
    ],
)
def test_a_closing_line_after_the_options_is_not_part_of_them(footer):
    reply = f"Which do you mean?\n1. Require two approving reviews\n2. Require one approving review\n\n{footer}"

    view = ask_view(reply, _SCOPE)

    assert [o.message for o in view.options] == [
        "Require two approving reviews",
        "Require one approving review",
    ]


@pytest.mark.parametrize(
    "tail",
    [
        # CodeRabbit: "or tell …" starts like a closing line, but goes on with option 2.
        "or tell the repository owner to approve it when no code owner is available.",
        # Any closing-line start, after an option that isn't finished.
        "Reply with 1 or 2.",
        "Which one do you want?",
    ],
)
def test_a_line_after_an_unfinished_option_gets_no_buttons(tail):
    reply = (
        "Which do you mean?\n"
        "1. Require two approving reviews\n"
        f"2. Require one approving review,\n{tail}\n\nReply with 1 or 2."
    )

    assert ask_view(reply, _SCOPE) is None


@pytest.mark.parametrize(
    "footer",
    ["  Reply with 1 or 2.", "   Please reply with 1, 2 or 3.", "\tPick 1 or 2."],
)
def test_an_indented_closing_line_is_not_part_of_the_last_option(footer):
    # CodeRabbit: it was joined to option 2, so its button sent the instruction too.
    reply = (
        "Which do you mean?\n"
        "1. Require two approving reviews\n"
        "   - Keep the administrator bypass enabled\n"
        "2. Require one approving review\n"
        "   counted only from code owners\n"
        f"{footer}\n"
        "Which one?"
    )

    view = ask_view(reply, _SCOPE)

    assert [o.message for o in view.options] == [
        "Require two approving reviews; Keep the administrator bypass enabled",
        "Require one approving review counted only from code owners",
    ]


@pytest.mark.parametrize(
    "line",
    [
        "   or type a new value",
        "   or tell the owner first",
        # Names numbers, but goes on with the option ("… with").
        "   reply with 1 or 2 approvals",
    ],
)
def test_an_indented_line_that_goes_on_with_the_option_stays_in_it(line):
    reply = (
        "Which do you mean?\n"
        "1. Require two approving reviews\n"
        "2. Require one approving review, or\n"
        f"{line}\n\n"
        "Reply with 1 or 2."
    )

    view = ask_view(reply, _SCOPE)

    assert view.options[1].message == f"Require one approving review, or {line.strip()}"
