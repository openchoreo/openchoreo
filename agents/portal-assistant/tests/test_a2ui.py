# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the A2UI answer buttons of a check edit question."""

import json

import pytest

from src.agent.a2ui import (
    CATALOG_ID,
    OWN_ANSWER,
    ask_card_surface,
    change_card_surface,
    choice_card_surface,
    dry_run_card_surface,
    dry_run_note,
    dry_run_result_event,
    edit_question_surface,
    message_card_surface,
    notice_card_surface,
    reply_card_surface,
    save_card_surface,
)
from src.agent.ask_view import AskOption, AskView, Editor
from src.agent.dry_run_view import DryRun
from src.agent.reply_view import NoticeView, Pick
from src.agent.rule_view import ChangeRow, ChangeView, Option
from src.agent.stream_events import emit


def _components(surface):
    return {c["id"]: c for c in surface["createSurface"]["components"]}


def _buttons(surface):
    """(label, userMessage) of each button, in the order the Row shows them."""
    components = _components(surface)
    buttons = [components[i] for i in components["root"]["children"]]
    return [(components[b["child"]]["text"], b["action"]["event"]["userMessage"]) for b in buttons]


def test_one_proposal_gets_yes_and_no():
    surface = edit_question_surface()

    assert surface["version"] == "1.0"
    assert _buttons(surface) == [("Yes, change it", "yes"), ("No", "no")]
    assert _components(surface)["yes"]["variant"] == "primary"
    # The chat box is there for anything else.
    assert not any(c["component"] == "AskField" for c in _components(surface).values())


def test_every_child_is_a_component():
    components = _components(edit_question_surface())

    children = [*components["root"]["children"]]
    children += [c["child"] for c in components.values() if "child" in c]
    assert all(child in components for child in children)


def test_the_surface_names_the_drawer_catalog():
    # A2UI's processor rejects a surface without one.
    assert edit_question_surface()["createSurface"]["catalogId"] == CATALOG_ID


def test_each_surface_has_its_own_id():
    assert (
        edit_question_surface()["createSurface"]["surfaceId"]
        != edit_question_surface()["createSurface"]["surfaceId"]
    )


def test_the_event_passes_the_wire_validator():
    assert '"type":"a2ui"' in emit({"type": "a2ui", "messages": [edit_question_surface()]})


_CURRENT_SCOPE = {
    "current_rule": json.dumps(
        {
            "all": [
                {"fact": "githubDaysSinceLastPush", "operator": "lessThanInclusive", "value": 90},
            ]
        }
    ),
    "available_fact_retrievers": [
        {
            "id": "r",
            "facts": [
                {"name": "githubDaysSinceLastPush", "description": "Days since the last push"},
            ],
        }
    ],
}


def test_numbered_choices_the_change_card_can_not_show_get_a_card_of_their_own():
    surface = choice_card_surface(
        [(1, "60 or less, if b is 1 or c is 2"), (2, "Less than 60")],
        "You want the limit to be 60. Should exactly 60 pass?",
        _CURRENT_SCOPE,
        "Recent push",
    )

    components = _components(surface)
    root = components["root"]
    assert (root["component"], root["title"], root["subtitle"], root["summary"]) == (
        "AskCard",
        "Proposed change",
        "Recent push",
        "You want the limit to be 60. Should exactly 60 pass?",
    )
    assert [(components[i]["label"], components[i]["value"]) for i in root["current"]] == [
        ("Days since the last push", "90 or less"),
    ]
    options = [components[i] for i in root["options"]]
    # Each choice says what it is, and sends only its number.
    assert [(o["component"], o["badge"], o["label"]) for o in options] == [
        ("AskOption", "1", "60 or less, if b is 1 or c is 2"),
        ("AskOption", "2", "Less than 60"),
    ]
    assert [o["action"]["event"]["userMessage"] for o in options] == ["1", "2"]
    (answer,) = [components[i] for i in root["answer"]]
    assert (answer["component"], answer["placeholder"]) == ("AskField", OWN_ANSWER)
    (keep,) = [components[i] for i in root["footer"]]
    assert components[keep["child"]]["text"] == "Keep the current rule"
    assert keep["action"]["event"]["userMessage"] == "no"


def test_the_choice_card_without_a_plain_rule_or_a_summary():
    nested = {
        "current_rule": json.dumps(
            {
                "all": [
                    {
                        "any": [
                            {"fact": "a", "operator": "equal", "value": 1},
                        ]
                    }
                ]
            }
        )
    }

    surface = choice_card_surface([(1, "x"), (2, "y")], None, nested, None)

    components = _components(surface)
    root = components["root"]
    assert "subtitle" not in root
    assert root["current"] == []
    assert root["summary"] == "Which of these do you want?"
    refs = root["current"] + root["options"] + root["answer"] + root["footer"]
    refs += [c["child"] for c in components.values() if "child" in c]
    assert all(ref in components for ref in refs)
    assert len(components) == len(surface["createSurface"]["components"])


_ROWS = [
    ChangeRow("Branch protected", "same", "Yes"),
    ChangeRow("Approving reviews required before merging", "pick", "2 or more"),
]


def test_the_card_for_two_choices():
    view = ChangeView(
        _ROWS,
        [Option(1, "3 or more"), Option(2, "more than 3")],
        "You want 3. Should 3 pass?",
    )

    components = _components(change_card_surface(view, "Main branch"))

    root = components["root"]
    assert (root["component"], root["title"], root["subtitle"], root["summary"]) == (
        "ChangeCard",
        "Proposed change",
        "Main branch",
        "You want 3. Should 3 pass?",
    )
    assert [(components[i]["label"], components[i]["value"]) for i in root["current"]] == [
        ("Branch protected", "Yes"),
        ("Approving reviews required before merging", "2 or more"),
    ]
    (change,) = [components[i] for i in root["changes"]]
    assert (change["state"], change["before"]) == ("pick", "2 or more")
    *tiles, no = [components[i] for i in root["footer"]]
    assert [(t["component"], t["badge"], t["value"]) for t in tiles] == [
        ("OptionTile", "1", "3 or more"),
        ("OptionTile", "2", "More than 3"),
    ]
    assert [t["action"]["event"]["userMessage"] for t in tiles] == ["1", "2"]
    assert (no["variant"], no["action"]["event"]["userMessage"]) == ("borderless", "no")
    assert components[no["child"]]["text"] == "Keep the current rule"
    (answer,) = [components[i] for i in root["answer"]]
    assert (answer["component"], answer["placeholder"]) == ("AskField", OWN_ANSWER)
    assert answer["value"] == answer["action"]["event"]["userMessage"] == {"@path": "/reply/text"}


def test_the_card_for_one_proposal_has_yes_and_no():
    view = ChangeView(
        [
            ChangeRow("Branch protected", "same", "Yes"),
            ChangeRow("Approving reviews", "changed", "2 or more", "3 or more"),
        ]
    )

    components = _components(change_card_surface(view, None))

    root = components["root"]
    assert "subtitle" not in root and "summary" not in root
    assert len(root["current"]) == 2
    (change,) = [components[i] for i in root["changes"]]
    assert (change["before"], change["after"]) == ("2 or more", "3 or more")
    assert [components[b]["action"]["event"]["userMessage"] for b in root["footer"]] == [
        "yes",
        "no",
    ]
    # Yes / No gets no box: the chat box is there for anything else.
    assert "answer" not in root
    assert not any(c["component"] == "AskField" for c in components.values())


def test_every_card_reference_is_a_component():
    view = ChangeView(_ROWS, [Option(1, "3 or more"), Option(2, "more than 3")])
    components = _components(change_card_surface(view, "x"))

    root = components["root"]
    refs = root["current"] + root["changes"] + root["footer"] + root["answer"]
    refs += [c["child"] for c in components.values() if "child" in c]
    assert all(ref in components for ref in refs)


_EDIT_CONFIRMATION = {
    "type": "confirmation_required",
    "token": "signed-token",
    "kind": "update",
    "check_id": "c1",
    "check_name": "Main branch",
    "status": "published",
    "current_rule": json.dumps(
        {
            "all": [
                {
                    "fact": "githubRequiredApprovingReviewCount",
                    "operator": "greaterThanInclusive",
                    "value": 2,
                },
            ]
        },
        indent=2,
    ),
    "new_rule": json.dumps(
        {
            "all": [
                {
                    "fact": "githubRequiredApprovingReviewCount",
                    "operator": "greaterThanInclusive",
                    "value": 3,
                },
            ]
        },
        indent=2,
    ),
    "text_changes": [{"field": "name", "label": "Name", "old": "Main branch", "new": "Main 3"}],
    "valid": True,
    "problems": [],
}
_FACTS = {
    "available_fact_retrievers": [
        {
            "id": "r",
            "facts": [
                {
                    "name": "githubRequiredApprovingReviewCount",
                    "description": "Approving reviews required",
                },
            ],
        }
    ]
}


def test_the_save_card_carries_the_confirmation_unchanged_plus_plain_words():
    surface = save_card_surface(_EDIT_CONFIRMATION, _FACTS)

    assert surface["createSurface"]["surfaceId"].startswith("save-card-")
    components = _components(surface)
    root = components["root"]
    assert root["component"] == "SaveCard"
    assert root["card"] == {k: v for k, v in _EDIT_CONFIRMATION.items() if k != "type"}
    (change,) = [components[i] for i in root["plain"]]
    assert (change["component"], change["label"], change["before"], change["after"]) == (
        "ChangeRow",
        "Approving reviews required",
        "2 or more",
        "3 or more",
    )


def test_the_save_card_of_a_new_check_lists_its_rule():
    confirmation = {
        "type": "confirmation_required",
        "token": "t",
        "kind": "create",
        "check_name": "Private",
        "new_rule": json.dumps(
            {
                "all": [
                    {"fact": "githubVisibility", "operator": "equal", "value": "private"},
                ]
            }
        ),
        "valid": True,
    }

    components = _components(save_card_surface(confirmation, {}))

    (line,) = [components[i] for i in components["root"]["plain"]]
    assert (line["component"], line["label"], line["value"]) == (
        "RuleLine",
        "Visibility",
        "private",
    )


def test_a_rule_without_plain_words_still_gets_the_whole_card():
    nested = json.dumps({"all": [{"any": [{"fact": "a", "operator": "equal", "value": 1}]}]})
    confirmation = {**_EDIT_CONFIRMATION, "new_rule": nested}

    root = _components(save_card_surface(confirmation, _FACTS))["root"]

    assert root["plain"] == []
    assert root["card"]["new_rule"] == nested


def test_the_notice_card():
    view = NoticeView(
        tone="warn",
        title="Can't build this check yet",
        summary="There's no fact for x.",
        subtitle="No fact measures this",
        picks_label="You could check instead",
        picks=[Pick("A CODEOWNERS file exists", "Make a check on this instead: a CODEOWNERS")],
        chips_label="Or did you mean",
        chips=[Pick("Must have a README", "I mean: a README")],
        ask="Or describe a different check",
    )

    surface = notice_card_surface(view)

    assert surface["createSurface"]["surfaceId"].startswith("notice-")
    components = _components(surface)
    root = components["root"]
    assert (root["component"], root["tone"], root["picksLabel"], root["chipsLabel"]) == (
        "NoticeCard",
        "warn",
        "You could check instead",
        "Or did you mean",
    )
    (pick,) = [components[i] for i in root["picks"]]
    (chip,) = [components[i] for i in root["chips"]]
    assert (pick["component"], pick["label"]) == ("PickRow", "A CODEOWNERS file exists")
    assert pick["action"]["event"]["userMessage"] == "Make a check on this instead: a CODEOWNERS"
    assert (chip["component"], chip["action"]["event"]["userMessage"]) == (
        "PickChip",
        "I mean: a README",
    )
    (answer,) = [components[i] for i in root["answer"]]
    assert (answer["component"], answer["placeholder"]) == (
        "AskField",
        "Or describe a different check",
    )
    assert "ideas" not in root and "question" not in root


def test_the_notice_card_with_ideas_to_read():
    view = NoticeView(
        tone="warn",
        title="Can't build this check yet",
        summary="There's no fact for x.",
        ideas_label="Checks you could write instead",
        ideas=["The repo must be private"],
        question="Would you like to write one of these, or something else?",
    )

    components = _components(notice_card_surface(view))
    root = components["root"]

    assert root["ideasLabel"] == "Checks you could write instead"
    assert root["ideas"] == ["The repo must be private"]
    assert root["question"] == "Would you like to write one of these, or something else?"
    assert (root["picks"], root["answer"], root["footer"]) == ([], [], [])
    assert set(components) == {"root"}
    assert root["footer"] == []
    assert "note" not in root


def test_the_notice_card_buttons():
    view = NoticeView(
        tone="warn",
        title="t",
        summary="s",
        buttons=[
            Pick("Check Components instead", "Yes, check Components instead"),
            Pick("No", "No"),
        ],
    )

    components = _components(notice_card_surface(view))

    first, second = [components[i] for i in components["root"]["footer"]]
    assert (first["variant"], first["action"]["event"]["userMessage"]) == (
        "primary",
        "Yes, check Components instead",
    )
    assert second["variant"] == "borderless"
    # A yes / no question gets no box.
    assert components["root"]["answer"] == []


_ASK = AskView(
    title="What should change?",
    summary="You rejected changing the required approvals from 3 to 2.",
    current=[("Branch protected", "Yes"), ("Approving reviews", "3 or more")],
    editors=[
        Editor(
            "Branch protected",
            "choice",
            "Yes",
            "Change branch protected to {value}",
            remove="Remove the condition: branch protected",
        ),
        Editor("Approving reviews", "number", 3, "Change approving reviews to {value}", "at least"),
    ],
    placeholder='Or type it, e.g. "make it 4"',
)


def test_the_ask_card():
    surface = ask_card_surface(_ASK, "Main branch protected")

    assert surface["createSurface"]["surfaceId"].startswith("ask-")
    components = _components(surface)
    root = components["root"]
    assert (root["component"], root["title"], root["subtitle"], root["summary"]) == (
        "AskCard",
        "What should change?",
        "Main branch protected",
        "You rejected changing the required approvals from 3 to 2.",
    )
    assert [components[i]["value"] for i in root["current"]] == ["Yes", "3 or more"]
    assert root["options"] == []
    (keep,) = [components[i] for i in root["footer"]]
    assert (keep["variant"], keep["action"]["event"]["userMessage"]) == (
        "borderless",
        "Keep the rule as it is",
    )


def test_each_editor_sends_the_sentence_it_writes():
    components = _components(ask_card_surface(_ASK, None))

    choice, number = [components[i] for i in components["root"]["editors"]]
    for editor in (choice, number):
        # The editor writes to ``message``; its button sends that same path.
        assert editor["message"] == editor["action"]["event"]["userMessage"]
        assert editor["message"]["@path"].startswith("/reply/")
    assert choice["message"] != number["message"]
    assert (choice["kind"], choice["value"], choice["choices"]) == ("choice", "Yes", ["Yes", "No"])
    assert choice["remove"]["event"]["userMessage"] == "Remove the condition: branch protected"
    assert (number["kind"], number["value"], number["comparison"]) == ("number", 3, "at least")
    assert number["comparisons"] == ["at least", "more than", "exactly", "at most", "less than"]
    assert number["template"] == "Change approving reviews to {value}"
    assert "remove" not in number


def test_the_text_box_sends_what_is_typed():
    components = _components(ask_card_surface(_ASK, None))

    (answer,) = [components[i] for i in components["root"]["answer"]]
    assert answer["component"] == "AskField"
    assert answer["value"] == answer["action"]["event"]["userMessage"] == {"@path": "/reply/text"}
    assert answer["placeholder"] == 'Or type it, e.g. "make it 4"'


def test_the_options_card():
    view = AskView(
        title="Which change do you mean?",
        summary="I'm not sure which option you mean.",
        options=[
            AskOption(1, "Require more than 2 approvals", "Require more than 2 approvals"),
            AskOption(2, "A different number", "A different number: {value}", 3),
        ],
        keep="Keep the current rule",
    )

    components = _components(ask_card_surface(view, None))

    root = components["root"]
    assert "subtitle" not in root
    assert (root["current"], root["editors"]) == ([], [])
    plain, number = [components[i] for i in root["options"]]
    assert (plain["component"], plain["badge"], plain["label"]) == (
        "AskOption",
        "1",
        "Require more than 2 approvals",
    )
    assert plain["action"]["event"]["userMessage"] == "Require more than 2 approvals"
    assert "template" not in plain
    assert (number["value"], number["template"]) == (3, "A different number: {value}")
    assert number["message"] == number["action"]["event"]["userMessage"]
    keep = components[root["footer"][0]]
    assert components[keep["child"]]["text"] == "Keep the current rule"
    assert keep["action"]["event"]["userMessage"] == "Keep the rule as it is"


def test_every_ask_card_reference_is_a_component_with_its_own_id():
    surface = ask_card_surface(_ASK, "x")
    components = _components(surface)

    root = components["root"]
    refs = root["current"] + root["editors"] + root["options"] + root["answer"] + root["footer"]
    refs += [c["child"] for c in components.values() if "child" in c]
    assert all(ref in components for ref in refs)
    assert len(components) == len(surface["createSurface"]["components"])


def test_the_message_card_carries_the_reply_text():
    surface = message_card_surface("Which entity kind should **this** check run on?")

    assert surface["createSurface"]["surfaceId"].startswith("message-")
    assert surface["createSurface"]["catalogId"] == CATALOG_ID
    assert _components(surface) == {
        "root": {
            "id": "root",
            "component": "MessageCard",
            "text": "Which entity kind should **this** check run on?",
        }
    }


def test_the_save_card_shows_the_reply_s_own_words():
    root = _components(save_card_surface(_EDIT_CONFIRMATION, {}, "**Tested on** `x`"))["root"]

    assert root["note"] == "**Tested on** `x`"
    assert "note" not in _components(save_card_surface(_EDIT_CONFIRMATION, {}))["root"]


_CARD = {
    "title": "What this check does",
    "summary": "It passes when the main branch is protected and needs `2` approvals.",
    "sections": [
        {
            "label": "What it checks",
            "lines": [
                {"label": "Branch protected", "value": "Yes"},
                {"label": "Approving reviews required", "value": "2 or more"},
                {"label": "", "value": "dropped: no label"},
            ],
        }
    ],
    "choices": [
        {"label": "Make it 3", "message": "Make the required approvals 3 or more"},
        {"label": "Dropped", "message": ""},
    ],
    "ask": "Or type a change",
}


def test_the_reply_card():
    surface = reply_card_surface(_CARD, "Main branch protected")

    assert surface["createSurface"]["surfaceId"].startswith("reply-")
    components = _components(surface)
    root = components["root"]
    assert (root["component"], root["title"], root["subtitle"]) == (
        "ReplyCard",
        "What this check does",
        "Main branch protected",
    )
    # Plain words: Markdown marks are taken out.
    assert root["summary"] == "It passes when the main branch is protected and needs 2 approvals."
    (section,) = [components[i] for i in root["sections"]]
    assert (section["component"], section["label"]) == ("ReplySection", "What it checks")
    assert [(components[i]["label"], components[i]["value"]) for i in section["lines"]] == [
        ("Branch protected", "Yes"),
        ("Approving reviews required", "2 or more"),
    ]
    (choice,) = [components[i] for i in root["choices"]]
    # It shows exactly the words it sends.
    assert (choice["component"], choice["label"]) == (
        "PickRow",
        "Make the required approvals 3 or more",
    )
    assert choice["action"]["event"]["userMessage"] == "Make the required approvals 3 or more"
    (answer,) = [components[i] for i in root["answer"]]
    assert answer["component"] == "AskField"
    assert answer["value"] == answer["action"]["event"]["userMessage"] == {"@path": "/reply/text"}


def test_every_reply_card_reference_is_a_component():
    components = _components(reply_card_surface(_CARD, None))

    root = components["root"]
    refs = root["sections"] + root["choices"] + root["answer"]
    refs += [i for c in components.values() for i in c.get("lines", [])]
    assert all(ref in components for ref in refs)


def test_the_reply_card_keeps_within_limits():
    card = {
        "title": "t" * 200,
        "summary": "s",
        "sections": [
            {"label": f"S{i}", "lines": [{"label": "a", "value": "b"}] * 20} for i in range(7)
        ],
        "choices": [{"label": f"C{i}", "message": "m"} for i in range(9)],
    }

    components = _components(reply_card_surface(card, None))

    root = components["root"]
    assert len(root["title"]) == 80 and root["title"].endswith("…")
    assert len(root["sections"]) == 5
    assert all(len(components[i]["lines"]) == 8 for i in root["sections"])
    # Two suggestions at most, with a box to type your own.
    assert len(root["choices"]) == 2
    (answer,) = [components[i] for i in root["answer"]]
    assert answer["placeholder"] == OWN_ANSWER


def test_starting_questions_set_by_code_come_with_type_your_own():
    card = {"title": "t", "summary": "s", "choices": [{"label": "x", "message": "Ignored"}]}

    components = _components(reply_card_surface(card, None, ["Explain this rule"]))

    root = components["root"]
    assert [components[i]["label"] for i in root["choices"]] == ["Explain this rule"]
    (answer,) = [components[i] for i in root["answer"]]
    assert answer["placeholder"] == OWN_ANSWER


@pytest.mark.parametrize(
    "raw",
    [
        None,
        {},
        {"title": "Only a title"},
        {"summary": "Only a summary"},
        {"title": "t", "summary": "s", "sections": "not a list"},
    ],
)
def test_a_card_without_a_title_and_summary_is_not_shown(raw):
    assert reply_card_surface(raw, None) is None


def test_reply_card_lines_wrap_instead_of_squeezing():
    card = {
        "title": "t",
        "summary": "s",
        "sections": [
            {
                "label": "Closest facts",
                "lines": [
                    {
                        "label": "CODEOWNERS",
                        "value": "Whether the evaluated branch has a CODEOWNERS file",
                    },
                ],
            }
        ],
    }
    components = _components(reply_card_surface(card, None))

    (line,) = [c for c in components.values() if c.get("label") == "CODEOWNERS"]
    # Not the change card's RuleLine, which keeps a value on one line.
    assert line["component"] == "ReplyLine"
    assert line["value"] == "Whether the evaluated branch has a CODEOWNERS file"


@pytest.mark.parametrize(
    ("run", "summary"),
    [
        (
            DryRun(status="passed", entity_ref="component:default/order-api"),
            "order-api meets this rule.",
        ),
        (
            DryRun(
                status="not-applicable",
                entity_ref="component:default/order-api",
                reasons=[("evaluation-error", "The repository is not on GitHub.")],
            ),
            "This check doesn't apply to order-api. The repository is not on GitHub.",
        ),
        (DryRun(status="failed"), "the component doesn't meet this rule."),
    ],
)
def test_the_test_card_says_the_result_in_plain_words(run, summary):
    root = _components(dry_run_card_surface(run, {}))["root"]

    assert (root["component"], root["status"], root["summary"]) == (
        "TestCard",
        run.status,
        summary,
    )


def test_a_fact_that_was_not_collected_says_so():
    run = DryRun(
        status="failed",
        entity_ref="component:default/order-api",
        fact_values={"githubTrivyOpenCriticalAlertCount": None},
    )

    components = _components(dry_run_card_surface(run, {}))

    (has,) = [components[i] for i in components["root"]["sections"]]
    (line,) = [components[i] for i in has["lines"]]
    assert (has["label"], line["label"], line["value"]) == (
        "What order-api has",
        "Trivy open critical alert count",
        "not collected",
    )


def test_the_live_order_api_result_reads_in_plain_words():
    # 2026-11-16: "test it on order-api" on the branch-protection check.
    run = DryRun(
        status="not-applicable",
        entity_ref="component:default/order-api",
        reasons=[
            (
                "missing-fact-values",
                "githubRepositoryFactRetriever collected facts for component:default/order-api "
                "but didn't report githubRequiredApprovingReviewCount.",
            )
        ],
    )
    scope = {
        "available_fact_retrievers": [
            {
                "id": "r",
                "facts": [
                    {
                        "name": "githubRequiredApprovingReviewCount",
                        "description": "Approving reviews required before merging",
                    },
                ],
            }
        ]
    }

    components = _components(dry_run_card_surface(run, scope))

    root = components["root"]
    assert root["summary"] == (
        "This check doesn't apply to order-api. The facts collected for order-api have no "
        "value for approving reviews required before merging."
    )
    (has,) = [components[i] for i in root["sections"]]
    assert [(components[i]["label"], components[i]["value"]) for i in has["lines"]] == [
        ("Approving reviews required before merging", "not reported"),
    ]


def test_the_live_result_says_what_its_test_card_says():
    run = DryRun(
        status="failed",
        entity_ref="component:default/order-api",
        fact_values={"githubVisibility": "public"},
        rule='{"all":[{"fact":"githubVisibility","operator":"equal","value":"private"}]}',
    )

    event = json.loads(emit(dry_run_result_event(run, {})))

    components = _components(dry_run_card_surface(run, {}))
    root = components["root"]
    assert (
        event["type"],
        event["status"],
        event["title"],
        event["subtitle"],
        event["summary"],
    ) == (
        "dry_run_result",
        "failed",
        root["title"],
        root["subtitle"],
        root["summary"],
    )
    assert event["sections"] == [
        {
            "label": components[s]["label"],
            "lines": [
                {"label": components[i]["label"], "value": components[i]["value"]}
                for i in components[s]["lines"]
            ],
        }
        for s in root["sections"]
    ]
    assert [s["label"] for s in event["sections"]] == ["What the rule needs", "What order-api has"]


def test_a_live_result_leaves_out_what_it_does_not_know():
    event = json.loads(emit(dry_run_result_event(DryRun(status="passed"), {})))

    assert event == {
        "type": "dry_run_result",
        "status": "passed",
        "title": "the component",
        "summary": "the component meets this rule.",
        "sections": [],
    }


def test_the_save_card_after_a_test_shows_each_fact_with_a_mark():
    run = DryRun(
        status="failed",
        entity_ref="component:default/order-api",
        fact_values={"githubVisibility": "public", "githubArchived": False},
        rule=json.dumps(
            {
                "all": [
                    {"fact": "githubVisibility", "operator": "equal", "value": "private"},
                    {"fact": "githubArchived", "operator": "equal", "value": False},
                ]
            }
        ),
    )

    components = _components(save_card_surface(_EDIT_CONFIRMATION, {}, None, run))

    root = components["root"]
    # The status apart from the heading, so the card can highlight it
    # (2026-11-25).
    assert (root["resultTitle"], root["resultStatus"]) == ("Tested on order-api", "failed")
    lines = [components[i] for i in root["result"]]
    assert [(line["label"], line["value"], line["mark"], line.get("hint")) for line in lines] == [
        ("Visibility", "public", "fail", "the rule wants private"),
        ("Archived", "No", "pass", None),
    ]


def test_a_fact_that_cant_be_compared_has_no_mark():
    run = DryRun(
        status="passed",
        entity_ref="component:default/order-api",
        fact_values={"githubVisibility": "private"},
        rule=json.dumps({"fact": "githubVisibility", "operator": "custom", "value": "x"}),
    )

    components = _components(save_card_surface(_EDIT_CONFIRMATION, {}, None, run))

    (line,) = [components[i] for i in components["root"]["result"]]
    assert "mark" not in line and "hint" not in line


def test_a_save_card_without_a_test_has_no_result():
    root = _components(save_card_surface(_EDIT_CONFIRMATION, {}))["root"]

    assert "resultTitle" not in root and "result" not in root
    assert "resultStatus" not in root


@pytest.mark.parametrize(
    "status, words",
    [
        ("passed", "Good news: order-api already follows this rule"),
        ("failed", "order-api doesn't follow this rule yet"),
    ],
)
def test_a_beginners_test_result_says_what_it_means_and_what_comes_next(status, words):
    run = DryRun(status=status, entity_ref="component:default/order-api")

    note = dry_run_note(run, {"experience": "beginner"})

    assert note.startswith(words)
    assert "draft" in note and "publish" in note


@pytest.mark.parametrize(
    "status, words",
    [
        ("passed", "Good news: order-api already follows this rule, so it passes."),
        ("failed", "order-api doesn't follow this rule yet, so it fails."),
    ],
)
def test_the_examples_test_result_says_what_it_means_without_saving(status, words):
    run = DryRun(status=status, entity_ref="component:default/order-api")

    note = dry_run_note(run, {"experience": "beginner"}, example=True)

    assert words in note
    assert "draft" not in note and "publish" not in note


@pytest.mark.parametrize(
    "scope",
    [
        {"experience": "experienced"},
        {},
        {"experience": "beginner", "check_id": "c1"},  # an edit
    ],
)
def test_other_test_results_leave_it_to_the_rows(scope):
    run = DryRun(status="passed", entity_ref="component:default/order-api")

    assert dry_run_note(run, scope) == ""


def test_a_check_that_doesnt_apply_says_why():
    run = DryRun(
        status="not-applicable",
        entity_ref="component:default/order-api",
        reasons=[("no-result", "No result.")],
    )

    assert dry_run_note(run, {}) == (
        "This check doesn't apply to order-api. The check gave no result for order-api."
    )


def test_a_reply_card_with_null_parts_is_still_shown():
    # The model often writes null for a list it leaves empty.
    card = {
        "title": "Start a security check",
        "summary": "Which area?",
        "sections": None,
        "choices": [{"label": "Code access", "message": "I want a check about code access"}],
        "ask": None,
    }

    components = _components(reply_card_surface(card, None))

    assert components["root"]["component"] == "ReplyCard"
    assert components["root"]["sections"] == []
    assert [components[i]["label"] for i in components["root"]["choices"]] == [
        "I want a check about code access",
    ]


def test_a_reply_without_suggestions_has_no_type_your_own():
    components = _components(reply_card_surface({"title": "t", "summary": "s?"}, None))
    assert components["root"]["choices"] == [] and components["root"]["answer"] == []
