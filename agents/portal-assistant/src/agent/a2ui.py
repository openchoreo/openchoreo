# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""A2UI surfaces the drawer renders under a reply.

Built by code, not the model, in the A2UI v1.0 message format
(https://a2ui.org/concepts/messages/). A button's ``userMessage`` is sent
as the user's next chat message, so the usual approval check still
decides what happens next — the buttons add no new way to approve.
"""

from __future__ import annotations

import json
import re
import uuid
from typing import Any

from pydantic import ValidationError

from src.agent.ask_view import COMPARISONS, KEEP_MESSAGE, AskOption, AskView, Editor
from src.agent.check_reply import unwrap
from src.agent.dry_run_view import (
    DryRun,
    actual_lines,
    entity_name,
    fact_results,
    needed_lines,
    reason_words,
)
from src.agent.existing_check import ExistingCheckView
from src.agent.reply_view import NoticeView, Pick
from src.agent.rule_view import ChangeView, rule_lines, saved_change_view
from src.agent.words import first_upper
from src.models import ReplyCard

A2UI_VERSION = "1.0"
# The drawer's own catalog: the basic Row / Text / Button APIs plus the
# ChangeCard / RuleLine / ChangeRow / OptionTile of an edit question, the
# AskCard / ValueEditor / AskOption / AskField of an edit reply that asks
# instead of proposing, the SaveCard of a check change, the NoticeCard /
# PickRow / PickChip of a reply that drafts no rule, the ReplyCard /
# ReplySection / ReplyLine the model fills for its other replies, the
# ExistingCheckCard of a new check like a saved one, the
# TestCard of a dry-run and the MessageCard
# when it doesn't, drawn with the portal's components.
# A2UI's processor needs a catalog on the surface even though the spec
# marks ``catalogId`` optional.
CATALOG_ID = "openchoreo.dev:portal-assistant/v1"


def _button(
    button_id: str,
    label: str,
    action: str,
    user_message: str,
    *,
    variant: str | None = None,
) -> list[dict[str, Any]]:
    """A Button and its Text label, as two components."""
    button: dict[str, Any] = {
        "id": button_id,
        "component": "Button",
        "child": f"{button_id}_label",
        "action": {"event": {"name": action, "userMessage": user_message}},
    }
    if variant:
        button["variant"] = variant
    return [button, {"id": f"{button_id}_label", "component": "Text", "text": label}]


# The box under a card's choices when nothing more specific fits.
OWN_ANSWER = "Or type your own answer"


def _answer_field(placeholder: str) -> dict[str, Any]:
    """The box to type your own answer. It writes what is typed to its path,
    and its action sends that path as the next message."""
    text = {"@path": "/reply/text"}
    return {
        "id": "answer",
        "component": "AskField",
        "placeholder": placeholder,
        "value": text,
        "action": _sends("type_answer", text),
    }


def edit_question_surface() -> dict[str, Any]:
    """Yes / No for an edit's one proposal that the change card can't show.

    No box to type in: the chat box is there for anything else. Numbered
    choices get choice_card_surface instead — "yes" alone doesn't say which.
    """
    buttons = _button("yes", "Yes, change it", "confirm_edit", "yes", variant="primary")
    buttons += _button("no", "No", "reject_edit", "no")
    button_ids = [c["id"] for c in buttons if c["component"] == "Button"]
    return _surface([{"id": "root", "component": "Row", "children": button_ids}, *buttons])


def choice_card_surface(
    choices: list[tuple[int, str]],
    summary: str | None,
    scope: dict[str, Any],
    check_name: str | None,
) -> dict[str, Any]:
    """Numbered choices the change card can't show exactly, as a card: the
    reply's question, the rule as it is now when it reads in plain words,
    one row per choice with the words naming it, a box to type your own
    answer and "Keep the current rule".

    A row sends its number and Keep sends "no" — the same answers the
    approval check reads when the user types them.
    """
    current = rule_lines(scope.get("current_rule") or "", scope) or []
    lines = [_rule_line(i, label, value) for i, (label, value) in enumerate(current)]
    options = [
        {
            "id": f"option_{n}",
            "component": "AskOption",
            "badge": str(n),
            "label": _words(label, 120),
            "action": _sends("pick_choice", str(n)),
        }
        for n, label in choices
    ]
    answer = _answer_field(OWN_ANSWER)
    keep = _button("no", "Keep the current rule", "reject_edit", "no", variant="borderless")
    card: dict[str, Any] = {
        "id": "root",
        "component": "AskCard",
        "title": "Proposed change",
        "summary": summary or "Which of these do you want?",
        "current": [line["id"] for line in lines],
        "editors": [],
        "options": [option["id"] for option in options],
        "answer": [answer["id"]],
        "footer": [keep[0]["id"]],
    }
    if check_name:
        card["subtitle"] = check_name
    return _surface([card, *lines, *options, answer, *keep])


def _surface(components: list[dict[str, Any]], kind: str = "edit-question") -> dict[str, Any]:
    return {
        "version": A2UI_VERSION,
        "createSurface": {
            "surfaceId": f"{kind}-{uuid.uuid4().hex[:12]}",
            "catalogId": CATALOG_ID,
            "components": components,
        },
    }


def change_card_surface(view: ChangeView, check_name: str | None) -> dict[str, Any]:
    """The edit question as a card: the reply's sentence, the rule as it is
    now, and only the conditions that change, in plain words.

    One proposal gets Yes / No. Numbered choices get a small OptionTile
    each, sending its number, next to "Keep the current rule" (sends "no")
    — the same answers the approval check reads when the user types them —
    and a box under them to type your own answer.
    Only clear choices come here (``ChangeView.clear``).
    """
    lines = [_rule_line(i, label, value) for i, (label, value) in enumerate(view.current)]
    changes = [_change_row(i, row) for i, row in enumerate(view.changes)]

    tiles = [
        {
            "id": f"option_{option.number}",
            "component": "OptionTile",
            "badge": str(option.number),
            "value": first_upper(option.value),
            "action": {
                "event": {"name": "pick_choice", "userMessage": str(option.number)},
            },
        }
        for option in view.options
    ]
    answer: list[dict[str, Any]] = []
    if tiles:
        buttons = _button(
            "no",
            "Keep the current rule",
            "reject_edit",
            "no",
            variant="borderless",
        )
        answer = [_answer_field(OWN_ANSWER)]
    else:
        buttons = _button("yes", "Yes, change it", "confirm_edit", "yes", variant="primary")
        buttons += _button("no", "No", "reject_edit", "no")

    card: dict[str, Any] = {
        "id": "root",
        "component": "ChangeCard",
        "title": "Proposed change",
        "current": [line["id"] for line in lines],
        "changes": [change["id"] for change in changes],
        "footer": [t["id"] for t in tiles]
        + [c["id"] for c in buttons if c["component"] == "Button"],
    }
    if answer:
        card["answer"] = [field["id"] for field in answer]
    if check_name:
        card["subtitle"] = check_name
    if view.summary:
        card["summary"] = view.summary
    return _surface([card, *lines, *changes, *tiles, *buttons, *answer])


def _rule_line(i: int, label: str, value: str) -> dict[str, Any]:
    return {"id": f"line_{i}", "component": "RuleLine", "label": label, "value": value}


def _change_row(i: int, row: Any) -> dict[str, Any]:
    change: dict[str, Any] = {
        "id": f"change_{i}",
        "component": "ChangeRow",
        "label": row.label,
        "state": row.state,
    }
    if row.before is not None:
        change["before"] = row.before
    if row.after is not None:
        change["after"] = row.after
    return change


def _result_line(
    i: int, label: str, value: str, follows: bool | None, wants: str | None
) -> dict[str, Any]:
    """One fact of a test's result: the component's value, ticked when it
    follows the rule, crossed with what the rule wants when it doesn't."""
    line: dict[str, Any] = {
        "id": f"result_{i}",
        "component": "RuleLine",
        "label": label,
        "value": value,
    }
    if follows is not None:
        line["mark"] = "pass" if follows else "fail"
    if wants:
        line["hint"] = f"the rule wants {wants}"
    return line


def save_card_surface(
    confirmation: dict[str, Any],
    scope: dict[str, Any],
    note: str | None = None,
    run: DryRun | None = None,
) -> dict[str, Any]:
    """The confirm card as a SaveCard: the same signed change, in the change card's style.

    ``card`` is every field of the ``confirmation_required`` event, unchanged
    — the drawer's card saves with its token through /chat/confirm exactly as
    before. Code adds the rule in plain words when it can be shown exactly:
    what changes for an edit, the conditions for a new check. ``note`` is
    the reply's own words (Markdown), shown at the top of the card, since the
    drawer shows the card instead of the reply's text. ``run``: the test it
    was tried in, shown above the note, one row per fact.
    """
    rows: list[dict[str, Any]] = []
    if confirmation.get("kind") == "update":
        view = saved_change_view(
            confirmation.get("current_rule"),
            confirmation["new_rule"],
            scope,
        )
        rows = [_change_row(i, row) for i, row in enumerate(view.changes if view else [])]
    else:
        lines = rule_lines(confirmation["new_rule"], scope) or []
        rows = [_rule_line(i, label, value) for i, (label, value) in enumerate(lines)]
    root = {
        "id": "root",
        "component": "SaveCard",
        "card": {k: v for k, v in confirmation.items() if k != "type"},
        "plain": [row["id"] for row in rows],
    }
    if note:
        root["note"] = note
    results: list[dict[str, Any]] = []
    if run:
        name, _ = entity_name(run.entity_ref)
        # The result itself is the status, which the card shows highlighted
        # like the test card's (2026-11-25), next to this heading.
        root["resultTitle"] = f"Tested on {name}"
        root["resultStatus"] = run.status
        results = [_result_line(i, *row) for i, row in enumerate(fact_results(run, scope))]
        root["result"] = [line["id"] for line in results]
    return _surface([root, *rows, *results], kind="save-card")


def _pick(component: str, i: int, pick: Pick, kind: str) -> dict[str, Any]:
    return {
        "id": f"{kind}_{i}",
        "component": component,
        "label": pick.label,
        # What a click sends, also for the portal to put in the message box
        # for the user to change before sending.
        "text": pick.message,
        "action": {"event": {"name": f"pick_{kind}", "userMessage": pick.message}},
    }


def notice_card_surface(view: NoticeView) -> dict[str, Any]:
    """A reply that drafts no rule, as a card: what's wrong in one sentence,
    ideas to read and a question, or clear answers to click, with a box to
    type your own when the view asks for one. A click or a typed answer is
    sent as the next message.
    """
    picks = [_pick("PickRow", i, p, "pick") for i, p in enumerate(view.picks)]
    chips = [_pick("PickChip", i, p, "chip") for i, p in enumerate(view.chips)]
    answer = [_answer_field(view.ask)] if view.ask else []
    buttons: list[dict[str, Any]] = []
    for i, pick in enumerate(view.buttons):
        buttons += _button(
            f"button_{i}",
            pick.label,
            "pick_button",
            pick.message,
            variant="primary" if i == 0 else "borderless",
        )
    card: dict[str, Any] = {
        "id": "root",
        "component": "NoticeCard",
        "tone": view.tone,
        "title": view.title,
        "summary": view.summary,
        "picks": [p["id"] for p in picks],
        "chips": [c["id"] for c in chips],
        "answer": [a["id"] for a in answer],
        "footer": [b["id"] for b in buttons if b["component"] == "Button"],
    }
    for key, value in (
        ("subtitle", view.subtitle),
        ("picksLabel", view.picks_label),
        ("ideasLabel", view.ideas_label),
        ("question", view.question),
        ("chipsLabel", view.chips_label),
        ("note", view.note),
    ):
        if value:
            card[key] = value
    if view.ideas:
        card["ideas"] = view.ideas
    return _surface([card, *picks, *chips, *answer, *buttons], kind="notice")


def _sends(name: str, user_message: str | dict[str, str]) -> dict[str, Any]:
    return {"event": {"name": name, "userMessage": user_message}}


def _editor(i: int, editor: Editor) -> dict[str, Any]:
    # The editor writes its sentence to ``message`` as the user changes the
    # value; the button sends what is there.
    path = {"@path": f"/reply/edit_{i}"}
    component: dict[str, Any] = {
        "id": f"edit_{i}",
        "component": "ValueEditor",
        "label": editor.label,
        "kind": editor.kind,
        "value": editor.value,
        "template": editor.template,
        "message": path,
        "action": _sends("change_value", path),
    }
    if editor.kind == "number":
        component["comparison"] = editor.comparison
        component["comparisons"] = list(COMPARISONS.values())
    else:
        component["choices"] = ["Yes", "No"]
    if editor.remove:
        component["remove"] = _sends("remove_condition", editor.remove)
    return component


def _ask_option(option: AskOption) -> dict[str, Any]:
    component: dict[str, Any] = {
        "id": f"option_{option.number}",
        "component": "AskOption",
        "badge": str(option.number),
        "label": option.label,
    }
    if option.value is None:
        component["action"] = _sends("pick_option", option.message)
    else:
        path = {"@path": f"/reply/option_{option.number}"}
        component |= {
            "value": option.value,
            "template": option.message,
            "message": path,
            "action": _sends("pick_option", path),
        }
    return component


def ask_card_surface(view: AskView, check_name: str | None) -> dict[str, Any]:
    """An edit reply that asks instead of proposing, as a card: the rule in
    plain words with an editor per number or yes/no condition, or the
    reply's options, plus a text box and a button that keeps the rule.

    Every answer is sent as the next message, so _edit_phase still decides.
    A typed or edited answer goes through the surface's data model (A2UI
    two-way binding): the field writes the sentence to its path and its
    button's ``userMessage`` reads that path.
    """
    lines = [_rule_line(i, label, value) for i, (label, value) in enumerate(view.current)]
    editors = [_editor(i, editor) for i, editor in enumerate(view.editors)]
    options = [_ask_option(option) for option in view.options]
    answer = _answer_field(view.placeholder)
    keep = _button("keep", view.keep, "keep_rule", KEEP_MESSAGE, variant="borderless")
    card: dict[str, Any] = {
        "id": "root",
        "component": "AskCard",
        "title": view.title,
        "current": [line["id"] for line in lines],
        "editors": [editor["id"] for editor in editors],
        "options": [option["id"] for option in options],
        "answer": [answer["id"]],
        "footer": [keep[0]["id"]],
    }
    if check_name:
        card["subtitle"] = check_name
    if view.summary:
        card["summary"] = view.summary
    return _surface([card, *lines, *editors, *options, answer, *keep], kind="ask")


def message_card_surface(text: str) -> dict[str, Any]:
    """A check authoring reply that no other card shows, as a card with its
    text (Markdown), so every reply in the chat reads the same way."""
    return _surface([{"id": "root", "component": "MessageCard", "text": text}], kind="message")


def existing_check_surface(view: ExistingCheckView) -> dict[str, Any]:
    """A drafted new check like a saved one, as a card in place of its save
    card: the check found and why editing it is recommended, its rule (and
    the new one when they differ) in plain words, and two buttons.

    The first moves the chat to that check in the drawer (``check``), as a
    save does, and sends ``editMessage`` when there is one. "Create a
    separate check" sends its message and gets the draft's save card.
    Nothing is saved from this card.
    """
    rule = [_rule_line(i, label, value) for i, (label, value) in enumerate(view.rule)]
    draft = [
        {**_rule_line(i, label, value), "id": f"draft_{i}"}
        for i, (label, value) in enumerate(view.draft)
    ]
    separate = _button(
        "separate",
        "Create a separate check",
        "create_separate",
        view.separate_message,
        variant="borderless",
    )
    root: dict[str, Any] = {
        "id": "root",
        "component": "ExistingCheckCard",
        "title": view.title,
        "subtitle": view.subtitle,
        "summary": view.summary,
        "question": view.question,
        "ruleLabel": view.rule_label,
        "rule": [line["id"] for line in rule],
        "draft": [line["id"] for line in draft],
        "editLabel": view.edit_label,
        "nextStep": view.next_step,
        "check": view.check,
        "footer": [separate[0]["id"]],
    }
    if view.edit_message:
        root["editMessage"] = view.edit_message
    return _surface([root, *rule, *draft, *separate], kind="existing-check")


# How much of the model's card is shown: enough for a reply, never a wall.
# Two suggestions at most: the card always adds "Tell me something else" for the rest.
_MAX_SECTIONS, _MAX_LINES, _MAX_CHOICES = 5, 8, 2


def _words(text: str | None, limit: int) -> str:
    """Plain text for the card: no Markdown marks, one line, at most ``limit``."""
    text = " ".join(re.sub(r"`|\*\*", "", text or "").split())
    return text if len(text) <= limit else text[: limit - 1].rstrip() + "…"


def reply_card_surface(
    raw: dict[str, Any] | None,
    check_name: str | None,
    fixed_choices: list[str] | None = None,
) -> dict[str, Any] | None:
    """The card the model filled for a check authoring reply, or None when
    it has no title or summary (the reply then gets the message card).

    The model never writes A2UI: it fills ``ReplyCard`` and code builds the
    components, in plain text and within limits. A choice or a typed answer
    is only sent as the next message, so _edit_phase still decides what it
    does.
    """
    try:
        card = ReplyCard.model_validate(raw or {})
    except ValidationError:
        return None
    title, summary = _words(card.title, 80), _words(card.summary, 600)
    if not title or not summary:
        return None

    sections: list[dict[str, Any]] = []
    lines: list[dict[str, Any]] = []
    for i, section in enumerate(card.sections[:_MAX_SECTIONS]):
        label = _words(section.label, 60)
        # ReplyLine, not the change card's RuleLine: a value here can be a
        # few words longer, and it wraps instead of squeezing its label.
        rows = [
            {
                "id": f"line_{i}_{j}",
                "component": "ReplyLine",
                "label": _words(line.label, 80),
                "value": _words(line.value, 120),
            }
            for j, line in enumerate(section.lines[:_MAX_LINES])
            if line.label.strip() and line.value.strip()
        ]
        if label and rows:
            sections.append(
                {
                    "id": f"section_{i}",
                    "component": "ReplySection",
                    "label": label,
                    "lines": [row["id"] for row in rows],
                }
            )
            lines += rows
    # ``fixed_choices`` set by code (a welcome's starting questions) replace
    # the model's. The model offers suggestions only when they help (most
    # replies end with a question instead), at most two. Each shows exactly
    # the words it sends.
    texts = (
        fixed_choices
        if fixed_choices is not None
        else [_words(c.message, 160) for c in card.choices if c.message.strip()][:_MAX_CHOICES]
    )
    choices = [_pick("PickRow", i, Pick(text, text), "choice") for i, text in enumerate(texts)]
    # Suggestions come with "Tell me something else", which points to the box.
    answer = [_answer_field(OWN_ANSWER)] if choices else []

    root: dict[str, Any] = {
        "id": "root",
        "component": "ReplyCard",
        "title": title,
        "summary": summary,
        "sections": [section["id"] for section in sections],
        "choices": [choice["id"] for choice in choices],
        "answer": [field["id"] for field in answer],
    }
    if check_name:
        root["subtitle"] = check_name
    return _surface([root, *sections, *lines, *choices, *answer], kind="reply")


def areas_card_surface(title: str, summary: str, areas: list[str]) -> dict[str, Any]:
    """A familiar user's areas as a plain answer: the area names listed, with
    nothing to click, and the summary's question under them (a ReplyCard
    asks after its sections). The user answers in the message box."""
    lines = [
        {"id": f"line_0_{i}", "component": "ReplyLine", "label": "", "value": _words(area, 60)}
        for i, area in enumerate(areas)
    ]
    section = {
        "id": "section_0",
        "component": "ReplySection",
        "label": "Areas",
        "lines": [line["id"] for line in lines],
    }
    root = {
        "id": "root",
        "component": "ReplyCard",
        "title": title,
        "summary": summary,
        "sections": [section["id"]],
        "choices": [],
        "answer": [],
    }
    return _surface([root, section, *lines], kind="reply")


_TEST_SUMMARY = {
    "passed": "{name} meets this rule.",
    "failed": "{name} doesn't meet this rule.",
    "not-applicable": "This check doesn't apply to {name}.",
}


def _ran_saved_rule(run: DryRun, scope: dict[str, Any]) -> bool:
    """Whether the test ran the check's saved rule. The model writes the
    test's rule, so a test meant for the saved rule may have run another."""

    def _rule(text: str | None) -> str | None:
        try:
            return json.dumps(unwrap(json.loads(text or "")), sort_keys=True)
        except ValueError:
            return None

    ran = _rule(run.rule)
    return ran is not None and ran == _rule(scope.get("current_rule"))


def _test_words(
    run: DryRun,
    scope: dict[str, Any],
) -> tuple[str, str | None, str, list[tuple[str, list[tuple[str, str]]]]]:
    """What a dry-run's result says, in plain words: the component's name and
    kind, the summary, and each section's label and lines (empty when it has
    none). Shared by the test card and the live result, so they match."""
    name, kind = entity_name(run.entity_ref)
    summary = _TEST_SUMMARY[run.status].format(name=name)
    if run.status == "not-applicable" and run.reasons:
        summary = f"{summary} {_words(' '.join(reason_words(run, scope)), 400)}"
    if scope.get("tests_saved_rule") and _ran_saved_rule(run, scope):
        # The edit in the chat isn't approved, so the saved rule ran: say so,
        # or the result reads as the edit's.
        summary = f"{summary} This ran the saved rule, as the change isn't approved yet."
    sections = [
        ("What the rule needs", needed_lines(run, scope)[:_MAX_LINES]),
        (f"What {name} has", actual_lines(run, scope)[:_MAX_LINES]),
    ]
    return name, kind, summary, sections


def dry_run_result_event(run: DryRun, scope: dict[str, Any]) -> dict[str, Any]:
    """A dry-run's result as soon as it comes back, for the drawer to show
    while the reply is being written: the words its test card shows later."""
    name, kind, summary, sections = _test_words(run, scope)
    event: dict[str, Any] = {
        "type": "dry_run_result",
        "status": run.status,
        "title": name,
        "summary": summary,
        "sections": [
            {"label": label, "lines": [{"label": row[0], "value": row[1]} for row in rows]}
            for label, rows in sections
            if rows
        ],
    }
    if kind:
        event["subtitle"] = kind
    return event


# After the test of a beginner's practice check (the welcome's example): what
# the result means. Its card then offers what to do next, not saving.
_EXAMPLE_NEXT = {
    "passed": (
        "Good news: {name} already follows this rule, so it passes. Every check works "
        "like this: it looks at each component's facts and shows who follows the rule."
    ),
    "failed": (
        "{name} doesn't follow this rule yet, so it fails. That's what a check is for: "
        "it shows which components need to fix something."
    ),
}

# For someone new to checks, after a new check's test: what the result means
# and what saving does next.
_BEGINNER_NEXT = {
    "passed": (
        "Good news: {name} already follows this rule, so the check works. Save it as a "
        "draft to look at it again later (scores don't change yet), or publish it and "
        "every component gets scored on it."
    ),
    "failed": (
        "{name} doesn't follow this rule yet, so it would fail once the check is "
        "published. That's what a check is for: it shows who needs to fix something. "
        "Save it as a draft or publish it, or tell me what to change."
    ),
}


def dry_run_note(run: DryRun, scope: dict[str, Any], *, example: bool = False) -> str:
    """What a dry-run's result means, under its rows at the top of the save
    card that follows it (save_card_surface): why the check doesn't apply,
    and for a beginner what passing or failing means (``example``: their
    practice check, which isn't saved). Empty when the rows say it all."""
    name, _kind, summary, _sections = _test_words(run, scope)
    if run.status == "not-applicable":
        return summary
    if example:
        return _EXAMPLE_NEXT.get(run.status, "").format(name=name)
    if scope.get("experience") == "beginner" and not scope.get("check_id"):
        return _BEGINNER_NEXT.get(run.status, "").format(name=name)
    return ""


def dry_run_card_surface(run: DryRun, scope: dict[str, Any]) -> dict[str, Any]:
    """A dry-run's result as a card, built from dry_run_check's own call and
    result: the component, passed / failed / doesn't apply, what the rule
    needs and what the component has, in plain words."""
    name, kind, summary, parts = _test_words(run, scope)

    sections: list[dict[str, Any]] = []
    lines: list[dict[str, Any]] = []
    for i, (label, rows) in enumerate(parts):
        rows = [
            {"id": f"line_{i}_{j}", "component": "ReplyLine", "label": row[0], "value": row[1]}
            for j, row in enumerate(rows)
        ]
        if rows:
            sections.append(
                {
                    "id": f"section_{i}",
                    "component": "ReplySection",
                    "label": label,
                    "lines": [row["id"] for row in rows],
                }
            )
            lines += rows

    root: dict[str, Any] = {
        "id": "root",
        "component": "TestCard",
        "status": run.status,
        "title": name,
        "summary": summary,
        "sections": [section["id"] for section in sections],
    }
    if kind:
        root["subtitle"] = kind
    return _surface([root, *sections, *lines], kind="test")
