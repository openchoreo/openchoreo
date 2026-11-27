# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for showing a check edit question in plain words."""

import json

import pytest

from src.agent.rule_view import (
    ChangeRow,
    Option,
    change_view,
    numbered_choices,
    proposed_conditions,
    rule_lines,
    saved_change_view,
    understood,
    value_words,
)

# The check from the 2026-11-12 live test: "make it 3".
_SCOPE = {
    "case_type": "check_authoring",
    "check_id": "c1",
    "check_name": "Main branch must be protected and require 2 approvals",
    "current_rule": json.dumps(
        {
            "all": [
                {"fact": "githubBranchProtected", "operator": "equal", "value": True},
                {
                    "fact": "githubRequiredApprovingReviewCount",
                    "operator": "greaterThanInclusive",
                    "value": 2,
                },
            ]
        }
    ),
    "available_fact_retrievers": [
        {
            "id": "githubRepositoryFactRetriever",
            "facts": [
                {
                    "name": "githubRequiredApprovingReviewCount",
                    "type": "integer",
                    "description": "Approving reviews required before merging",
                },
                {
                    "name": "githubBranchProtected",
                    "type": "boolean",
                    "description": "Whether the branch has classic branch protection or an active "
                    "ruleset rule of type pull_request, required_status_checks",
                },
                {
                    "name": "githubVisibility",
                    "type": "string",
                    "description": "Repository visibility: public, private, or internal",
                },
            ],
        }
    ],
}
_PROTECTED = "`githubBranchProtected` `equal` `true`"
_REVIEWS = "`githubRequiredApprovingReviewCount`"
_SAME = ChangeRow("Branch protected", "same", "Yes")

# The two-choice reply from the live test, word for word.
_TWO_CHOICES = f"""**Current rule:** {_PROTECTED} AND {_REVIEWS} `greaterThanInclusive` `2` — passes …

**What I understood:** You want the required approving review count changed from 2 to 3.

Please confirm:
1. **Proposed rule:** {_PROTECTED} AND {_REVIEWS} `greaterThanInclusive` `3` — 3 or more approvals required (3 passes).
2. **Proposed rule:** {_PROTECTED} AND {_REVIEWS} `greaterThan` `3` — more than 3 approvals required (3 does not pass).

Reply with 1 or 2."""  # noqa: E501


def test_one_proposal_shows_what_changes_and_what_stays():
    reply = (
        f"**Proposed rule:** {_PROTECTED} AND {_REVIEWS} `greaterThanInclusive` `3` — 3 or more\n\n"
        "Shall I change it to that?"
    )

    view = change_view(reply, _SCOPE)

    assert view.rows == [
        _SAME,
        ChangeRow("Approving reviews required before merging", "changed", "2 or more", "3 or more"),
    ]
    assert view.options == []
    assert view.current == [
        ("Branch protected", "Yes"),
        ("Approving reviews required before merging", "2 or more"),
    ]
    assert view.changes == [view.rows[1]]


def test_two_choices_become_option_tiles():
    view = change_view(_TWO_CHOICES, _SCOPE)

    assert view.rows == [
        _SAME,
        ChangeRow("Approving reviews required before merging", "pick", "2 or more"),
    ]
    assert view.options == [Option(1, "3 or more"), Option(2, "more than 3")]
    assert view.summary == ("You want the required approving review count changed from 2 to 3.")


def test_a_removed_and_an_added_condition():
    reply = f'**Proposed rule:** {_PROTECTED} AND `githubVisibility` `equal` `"private"` — …'

    view = change_view(reply, _SCOPE)

    assert view.rows == [
        _SAME,
        ChangeRow("Approving reviews required before merging", "removed", "2 or more"),
        ChangeRow("Repository visibility", "added", None, "private"),
    ]


@pytest.mark.parametrize(
    ("operator", "value", "words"),
    [
        ("equal", True, "Yes"),
        ("equal", False, "No"),
        ("notEqual", True, "No"),
        ("equal", "private", "private"),
        ("notEqual", "public", "not public"),
        ("greaterThanInclusive", 3, "3 or more"),
        ("greaterThan", 3, "more than 3"),
        ("lessThanInclusive", 60, "60 or less"),
        ("lessThan", 60, "less than 60"),
        ("in", ["private", "internal"], "one of private, internal"),
        ("notIn", ["public"], "none of public"),
    ],
)
def test_value_words(operator, value, words):
    assert value_words(operator, value) == words


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param("Shall I change the limit to 3?", id="no proposal line"),
        pytest.param(f"**Proposed rule:** NOT ({_PROTECTED})", id="not"),
        pytest.param(f"**Proposed rule:** {_PROTECTED} AND {_PROTECTED}", id="same fact twice"),
        pytest.param(
            f"**Proposed rule:** {_PROTECTED} AND ({_REVIEWS} `greaterThan` `3` OR "
            f"{_REVIEWS} `equal` `0`)",
            id="nested group",
        ),
        pytest.param(
            f"1. **Proposed rule:** {_PROTECTED}\n2. **Proposed rule:** {_PROTECTED}",
            id="identical choices",
        ),
    ],
)
def test_rules_it_can_not_show_exactly_give_none(reply):
    assert change_view(reply, _SCOPE) is None


def test_a_nested_current_rule_gives_none():
    scope = {**_SCOPE, "current_rule": json.dumps({"all": [{"any": []}]})}

    assert change_view(f"**Proposed rule:** {_PROTECTED}", scope) is None


def test_a_fact_without_a_short_description_gets_a_readable_name():
    view = change_view(
        f"**Proposed rule:** {_PROTECTED}", {**_SCOPE, "available_fact_retrievers": []}
    )

    assert view.rows[0].label == "Branch protected"
    assert view.rows[1].label == "Required approving review count"


@pytest.mark.parametrize(
    ("line", "sentence"),
    [
        ("**What I understood:** You want 3.", "You want 3."),
        ("What I understood: You want `3`.", "You want 3."),
        ("**What I understood**: You want 3. Should 3 pass?", "You want 3. Should 3 pass?"),
    ],
)
def test_understood_reads_the_sentence_without_markdown(line, sentence):
    assert understood(f"**Current rule:** …\n\n{line}\n\n**Proposed rule:** …") == sentence


def test_understood_is_none_without_the_sentence():
    assert understood("Shall I change it?") is None


def test_numbered_choices_are_named_by_the_words_after_their_dash():
    reply = (
        "**Please confirm:**\n"
        "1. **Proposed rule:** `a` `lessThanInclusive` `60` AND (`b` `equal` `1` OR `c` `equal` `2`)"
        " — 60 or less, if b is 1 or c is 2\n"
        "2. **Proposed rule:** `a` `lessThan` `60` - less than 60\n\n"
        "Reply with 1 or 2."
    )

    assert numbered_choices(reply) == [
        (1, "60 or less, if b is 1 or c is 2"),
        (2, "Less than 60"),
    ]


def test_a_numbered_choice_without_words_is_named_by_its_rule():
    reply = "1. **Proposed rule:** `a` `lessThan` `60`\n2. `a` `lessThan` `-5`"

    # A dash inside a backticked value doesn't start the words.
    assert numbered_choices(reply) == [(1, "a lessThan 60"), (2, "a lessThan -5")]


def test_saved_change_view_reads_the_signed_rules():
    new = json.dumps(
        {
            "all": [
                {"fact": "githubBranchProtected", "operator": "equal", "value": True},
                {
                    "fact": "githubRequiredApprovingReviewCount",
                    "operator": "greaterThanInclusive",
                    "value": 3,
                },
            ]
        },
        indent=2,
    )

    view = saved_change_view(_SCOPE["current_rule"], new, _SCOPE)

    assert view.changes == [
        ChangeRow("Approving reviews required before merging", "changed", "2 or more", "3 or more"),
    ]


def test_rule_lines_of_a_new_check():
    rule = json.dumps(
        {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]}
    )

    assert rule_lines(rule, _SCOPE) == [("Repository visibility", "private")]


@pytest.mark.parametrize(
    "rule",
    [
        pytest.param(
            {
                "any": [
                    {"fact": "githubVisibility", "operator": "equal", "value": "private"},
                    {"fact": "githubVisibility", "operator": "equal", "value": "internal"},
                ]
            },
            id="any with two conditions",
        ),
        pytest.param(
            {"all": [{"not": {"fact": "a", "operator": "equal", "value": 1}}]}, id="nested"
        ),
        pytest.param(
            {
                "all": [
                    {
                        "fact": "githubTopics",
                        "path": "$.length",
                        "operator": "greaterThan",
                        "value": 0,
                    }
                ]
            },
            id="a value read through a path",
        ),
    ],
)
def test_rules_that_dont_read_as_all_of_these_have_no_plain_words(rule):
    assert rule_lines(json.dumps(rule), _SCOPE) is None
    assert saved_change_view(_SCOPE["current_rule"], json.dumps(rule), _SCOPE) is None


def test_an_or_proposal_is_not_shown_as_rows():
    reply = f"**Proposed rule:** {_PROTECTED} OR {_REVIEWS} `greaterThanInclusive` `3`"

    assert change_view(reply, _SCOPE) is None


def test_an_exact_number_reads_exactly():
    assert value_words("equal", 5) == "exactly 5"
    assert value_words("equal", "private") == "private"
    assert value_words("equal", True) == "Yes"


@pytest.mark.parametrize(
    ("line", "said"),
    [
        (
            "You want 60 instead of 90. Should exactly 60 still pass?",
            "You want 60 instead of 90. Should exactly 60 still pass?",
        ),
        (
            "your “no” rejected both. Tell me which you want (reply with the number):",
            "Your “no” rejected both.",
        ),
    ],
)
def test_the_understood_sentence_is_tidied(line, said):
    assert understood(f"**What I understood:** {line}") == said


def test_a_condition_in_one_pair_of_backticks_is_read_too():
    # 2026-11-25: the model wrote `githubVisibility in ["private","internal"]`
    # and the change showed as plain text instead of the card.
    scope = {
        **_SCOPE,
        "current_rule": json.dumps(
            {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]},
        ),
    }
    one = '**Proposed rule:** `githubVisibility in ["private","internal"]` — private or internal'
    bare = (
        '**Proposed rule:** `githubVisibility` `in` ["private", "internal"] — private or internal'
    )
    three = '**Proposed rule:** `githubVisibility` `in` `["private","internal"]`'

    assert change_view(one, scope) is not None
    assert change_view(one, scope).changes == change_view(three, scope).changes
    assert change_view(bare, scope).changes == change_view(three, scope).changes
    # Words in backticks that aren't a condition are not read as one.
    assert change_view("**Proposed rule:** `make it stricter`", scope) is None


_VISIBILITY = {
    **_SCOPE,
    "current_rule": json.dumps(
        {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]},
    ),
}


def test_choices_with_list_values_become_option_tiles():
    reply = """Please confirm:
1. **Proposed rule:** `githubVisibility` `in` `["private","internal"]` — private or internal
2. **Proposed rule:** `githubVisibility` `in` `["private"]` — private only

Reply with 1 or 2."""

    view = change_view(reply, _VISIBILITY)

    assert [row.state for row in view.rows] == ["pick"]
    assert [option.number for option in view.options] == [1, 2]


def test_a_list_value_both_choices_keep_beside_one_they_differ_in():
    reply = f"""Please confirm:
1. **Proposed rule:** `githubVisibility` `in` `["private","internal"]` AND {_REVIEWS} `greaterThanInclusive` `2` — 2 or more
2. **Proposed rule:** `githubVisibility` `in` `["private","internal"]` AND {_REVIEWS} `greaterThanInclusive` `3` — 3 or more

Reply with 1 or 2."""  # noqa: E501

    view = change_view(reply, _VISIBILITY)

    assert [row.state for row in view.rows] == ["changed", "pick"]
    assert view.options == [Option(1, "2 or more"), Option(2, "3 or more")]


def test_a_choice_without_a_condition_the_other_adds_says_not_added():
    # Live test 2026-11-26: option 1 read "removed" for githubCodeqlScanned,
    # a condition the rule never had; only option 2 adds it.
    scope = {
        **_SCOPE,
        "current_rule": json.dumps(
            {"all": [{"fact": "githubVisibility", "operator": "equal", "value": "private"}]},
        ),
    }
    private = '`githubVisibility` `equal` `"private"`'
    reply = f"""Please confirm:
1. **Proposed rule:** {private} AND `githubCodeqlOpenAlertCount` `equal` `0` — no open alerts
2. **Proposed rule:** {private} AND `githubCodeqlOpenAlertCount` `equal` `0` AND `githubCodeqlScanned` `equal` `true` — no alerts, and scanned

Reply with 1 or 2."""  # noqa: E501

    view = change_view(reply, scope)

    assert view.options[0].value == "not added"
    assert view.options[1].value == "Yes"


def test_a_choice_that_drops_a_current_condition_still_says_removed():
    reply = f"""Please confirm:
1. **Proposed rule:** {_PROTECTED} — protection only
2. **Proposed rule:** {_PROTECTED} AND {_REVIEWS} `greaterThanInclusive` `3` — 3 or more

Reply with 1 or 2."""

    view = change_view(reply, _SCOPE)

    assert view.options[0].value == "removed"


@pytest.mark.parametrize(
    ("message", "proposals"),
    [
        pytest.param(
            "**Current rule:** `a` `lessThanInclusive` `30`\n\nShall I make it `a` `lessThanInclusive` `60`?",
            {1: [("a", "lessThanInclusive", 60)]},
            id="in a question",
        ),
        pytest.param(
            "Current rule: `a lessThanInclusive 30`\n\nI'd change it to `a lessThan 60`. OK?",
            {1: [("a", "lessThan", 60)]},
            id="in one pair of backticks",
        ),
        pytest.param(
            "Shall I change it to `a` `equal` `1` AND `b` `equal` `true`?",
            {1: [("a", "equal", 1), ("b", "equal", True)]},
            id="two conditions",
        ),
        pytest.param(
            "Which?\n1. `a` `lessThanInclusive` `60` — 60 or less\n2. `a` `lessThan` `60` — less than 60",
            {1: [("a", "lessThanInclusive", 60)], 2: [("a", "lessThan", 60)]},
            id="numbered choices",
        ),
    ],
)
def test_a_proposal_without_its_label_is_read(message, proposals):
    # CodeRabbit: approved like a labeled one, so it must be compared like one.
    # As JSON: == holds for True and 1, but the rules engine tells them apart.
    assert json.dumps(proposed_conditions(message)) == json.dumps(proposals)


@pytest.mark.parametrize(
    "message",
    [
        # Which line is the proposal can't be told.
        "Maybe `a` `equal` `1`.\nOr `a` `equal` `2`?",
        # Not a flat AND.
        "Shall I make it `a` `equal` `1` OR `b` `equal` `2`?",
        # Labeled ones decide: one that can't be read isn't read around.
        "**Proposed rule:** `a` `equal` `1` OR `b` `equal` `2`\n\nAlso `c` `equal` `3`?",
        # Only the current rule.
        "**Current rule:** `a` `equal` `1`\n\nWhat should it be instead?",
    ],
)
def test_a_proposal_that_cant_be_told_is_not_read(message):
    assert proposed_conditions(message) is None


@pytest.mark.parametrize(
    "message",
    [
        pytest.param("Shall I use NOT (`a` `equal` `true`)?", id="not around one"),
        pytest.param("Shall I use not `a` `equal` `true`?", id="lowercase not"),
        pytest.param("Shall I make it `a` `equal` `true` OR `b` `equal` `1`?", id="or between"),
        pytest.param(
            "1. NOT (`a` `equal` `true`)\n2. `a` `equal` `false`", id="numbered with not",
        ),
    ],
)
def test_an_unlabeled_proposal_with_logic_around_it_isnt_read(message):
    # Read without its NOT, it would be the opposite condition.
    assert proposed_conditions(message) is None


def test_prose_around_an_unlabeled_proposal_still_reads():
    assert proposed_conditions(
        "Shall I make it `a` `lessThan` `60` and leave the rest as it is?",
    ) == {1: [("a", "lessThan", 60)]}
