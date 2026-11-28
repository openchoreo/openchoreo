# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for reading check authoring replies that draft no rule."""

import pytest

from src.agent.reply_view import Pick, lacks_alternatives, notice_view

_SCOPE = {
    "case_type": "check_authoring",
    "available_fact_retrievers": [
        {
            "id": "githubRepositoryFactRetriever",
            "facts": [
                {
                    "name": "githubHasCodeowners",
                    "description": "Whether the evaluated branch has a "
                    "CODEOWNERS file in .github/, the root or docs/",
                },
                {
                    "name": "githubHasSecurityPolicy",
                    "description": "Whether the evaluated branch has a "
                    "SECURITY.md file in .github/, the root or docs/",
                },
                {
                    "name": "githubVisibility",
                    "description": "Repository visibility: public, private, or internal",
                },
            ],
        }
    ],
}

# The reply from the 2026-11-13 live test, word for word.
_README = (
    "No available fact measures whether a README file exists, so I can't build this check yet. "
    "The closest facts are `githubHasCodeowners` (whether a CODEOWNERS file exists) and "
    "`githubHasSecurityPolicy` (whether a SECURITY.md file exists). Would you like a check on "
    "one of those instead, or do you mean a different condition (for example: must have a "
    "README, must have README in a specific path, or README must contain certain content)?"
)


def test_no_fact_reply_offers_the_close_facts_and_what_else_was_meant():
    view = notice_view(_README, _SCOPE)

    assert (view.tone, view.title) == ("warn", "Can't build this check yet")
    assert view.summary == "There's no fact for whether a README file exists."
    assert view.picks == []
    assert view.ideas == ["A CODEOWNERS file exists", "A SECURITY.md file exists"]
    assert [c.label for c in view.chips] == [
        "Must have a README",
        "Must have README in a specific path",
        "README must contain certain content",
    ]
    assert view.chips[1].message == "I mean: must have README in a specific path"
    assert view.ask == "Or describe a different check"


def test_no_fact_ideas_are_words_to_read_with_a_question_and_no_box():
    """2026-11-19: the checks to write instead are part of the reply, not
    answers to click, and with nothing to click there is no "Tell me
    something else": the question asks the user to type."""
    reply = (
        "No available fact measures whether a repo has a README file, so I can't build this "
        "check yet. Checks you could write instead: `githubHasCodeowners` (the repo must have "
        "a CODEOWNERS file) and `githubVisibility` (the repo must be private). Would you like "
        "to write one of these, or something else?"
    )

    view = notice_view(reply, _SCOPE)

    assert view.ideas_label == "Checks you could write instead"
    assert view.ideas == ["The repo must have a CODEOWNERS file", "The repo must be private"]
    assert view.question == "Would you like to write one of these, or something else?"
    assert (view.picks, view.chips, view.ask) == ([], [], None)


def test_no_fact_without_ideas_still_asks():
    reply = "No available fact measures README content, so I can't build this check yet."

    view = notice_view(reply, _SCOPE)

    assert (view.ideas, view.ideas_label) == ([], None)
    assert view.question == "What would you like to check instead?"
    assert view.ask is None


def test_an_other_options_line_is_read_too():
    reply = (
        "No available fact measures README content, so I can't build this check yet.\n\n"
        "Other options: `Must have a README`, `README in docs/`"
    )

    assert [c.label for c in notice_view(reply, _SCOPE).chips] == [
        "Must have a README",
        "README in docs/",
    ]


def test_a_fact_that_does_not_exist_is_not_offered():
    reply = (
        "No available fact measures whether a README file exists, so I can't build this check "
        "yet. The closest facts are `githubHasReadme` (whether a README exists) and "
        "`githubHasCodeowners` (whether a CODEOWNERS file exists)."
    )

    assert notice_view(reply, _SCOPE).ideas == ["A CODEOWNERS file exists"]


def test_no_retriever_for_the_kind():
    reply = (
        "No fact retriever collects `githubVisibility` for `API`, so I can't build this check. "
        "It can be checked on `Component`. Would you like a check on that instead?"
    )

    view = notice_view(reply, _SCOPE)

    assert view.title == "Can't check this for APIs"
    assert view.subtitle == "Repository visibility"
    assert "only collected for Components" in view.summary
    assert view.buttons == [
        Pick("Check Components instead", "Yes, check Components instead"),
        Pick("No", "No"),
    ]


def test_which_value_reply():
    reply = (
        "Did you mean it must be `private`, must not be `public`, or something else? "
        "`githubVisibility` can be `public`, `private` or `internal`."
    )

    view = notice_view(reply, _SCOPE)

    assert (view.tone, view.title, view.subtitle) == (
        "ask",
        "Which value should pass?",
        "Repository visibility",
    )
    assert view.summary == "Repository visibility can be public, private or internal."
    assert view.chips == [
        Pick("Must be private", "It must be private"),
        Pick("Must not be public", "It must not be public"),
    ]
    assert view.chips_label is None  # no "Pick one" (2026-11-25)


def test_which_value_with_one_value_has_no_pick_one():
    """2026-11-25: no "Pick one" above the values, whatever their number."""
    reply = (
        "Did you mean it must be `private`, or something else? "
        "`githubVisibility` can be `public`, `private` or `internal`."
    )

    view = notice_view(reply, _SCOPE)

    assert view.chips == [Pick("Must be private", "It must be private")]
    assert view.chips_label is None


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param("Name: Repo must be private\n\nRule …", id="a drafted check"),
        pytest.param("Shall I change it to that?", id="an edit question"),
        pytest.param(
            "No fact retriever collects `githubMadeUp` for `API`, so I can't build this check. "
            "It can be checked on `Component`.",
            id="an unknown fact",
        ),
    ],
)
def test_other_replies_get_no_notice(reply):
    assert notice_view(reply, _SCOPE) is None


def test_a_no_fact_reply_with_nothing_to_pick_lacks_alternatives():
    bare = (
        "No available fact measures whether a repo has a README, so I can't build this check "
        "yet. What would you like to check instead?"
    )
    assert lacks_alternatives(bare, _SCOPE)
    assert not lacks_alternatives(_README, _SCOPE)  # names facts and other options
    assert not lacks_alternatives("Here is your check.", _SCOPE)
    assert not lacks_alternatives(bare, {"case_type": "check_authoring"})  # no facts at all


# The reply the model gave twice on 2026-11-22: real facts, no backticks.
_NO_BACKTICKS = (
    "No available fact measures whether a repository has a README, so I can't build this "
    "check yet. Checks you could write instead: githubHasCodeowners (the repo must have a "
    "CODEOWNERS file), githubHasSecurityPolicy (the repo must have a SECURITY.md file), "
    "githubVisibility (the repo must be private). Would you like one of these, or something else?"
)


def test_facts_written_without_backticks_are_still_read():
    view = notice_view(_NO_BACKTICKS, _SCOPE)

    assert view.ideas == [
        "The repo must have a CODEOWNERS file",
        "The repo must have a SECURITY.md file",
        "The repo must be private",
    ]
    assert not lacks_alternatives(_NO_BACKTICKS, _SCOPE)


def test_a_fact_that_does_not_exist_is_dropped_with_or_without_backticks():
    reply = (
        _NO_BACKTICKS.replace("githubHasCodeowners", "githubHasReadme")
        .replace(
            "githubHasSecurityPolicy",
            "`githubHasLicense`",
        )
        .replace("githubVisibility", "githubIsPublic")
    )

    assert notice_view(reply, _SCOPE).ideas == []
    assert lacks_alternatives(reply, _SCOPE)


def test_words_before_a_bracket_are_not_taken_for_facts():
    reply = (
        "No available fact measures whether a README file exists, so I can't build this check "
        "yet. Do you mean a different condition (for example: must have a README)?"
    )

    assert notice_view(reply, _SCOPE).ideas == []
