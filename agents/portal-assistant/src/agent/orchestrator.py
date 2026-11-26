# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Top-level streaming entry point.

Owns:
- ``stream_chat`` — runs one chat turn end-to-end and yields NDJSON
  StreamEvents.
- the streaming-loop helpers (``_yield_chunks``,
  ``_recover_from_structured_response``, ``_handle_stream_error``).
- the per-pod concurrency semaphore.

The agent build + middleware assembly live in ``builder.py``. The
agent is read-only: there is no action proposal / approval / execute
flow. ``WriteGuardMiddleware`` hard-refuses any mutating tool that
slips into the catalog so the agent cannot mutate state even if the
LLM tries.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import re
import uuid
from collections.abc import AsyncIterator
from contextlib import AbstractAsyncContextManager
from typing import Any

from langchain.agents.structured_output import StructuredOutputValidationError
from langchain_core.runnables import Runnable
from pydantic import ValidationError

from common.auth.bearer import BearerTokenAuth
from src.agent.a2ui import (
    areas_card_surface,
    ask_card_surface,
    change_card_surface,
    choice_card_surface,
    dry_run_card_surface,
    dry_run_note,
    dry_run_result_event,
    edit_question_surface,
    existing_check_surface,
    message_card_surface,
    notice_card_surface,
    reply_card_surface,
    save_card_surface,
)
from src.agent.ask_view import ask_view, edit_view
from src.agent.builder import _build_agent
from src.agent.check_reply import (
    RULE_FENCE_RE,
    reply_words,
    strip_step_labels,
    unwrap,
    without_failed_test_lines,
    without_test_summary,
    without_unrequested_guard,
)
from src.agent.confirm_card import (
    SAVE_REQUEST_REPLY,
    build_confirmation,
    card_for_save_request,
    check_to_test,
    example_draft,
    is_save_request,
    is_saved_rule,
    rule_fix_note,
    same_rule,
    with_example_description,
)
from src.agent.dry_run_view import DryRun, entity_name, read_dry_run
from src.agent.existing_check import (
    card_for_separate_check,
    existing_check_view,
    find_existing_check,
    is_separate_request,
)
from src.agent.problem_view import ToolError, failed_test_note, problem_view
from src.agent.recovery import recover_with_fallback
from src.agent.reply_view import lacks_alternatives, notice_view
from src.agent.rule_view import change_view, numbered_choices, understood
from src.agent.stream_events import ConfirmationRequiredEvent
from src.agent.stream_events import emit as _emit_event
from src.agent.stream_parser import ChatResponseParser
from src.clients.mcp import BackstageToolError, backstage_tools_as_user, call_backstage_tool
from src.config import settings
from src.logging_config import request_id_context
from src.models import RESPONSE_NAMES, get_current_utc

logger = logging.getLogger(__name__)


# Module-level semaphore caps concurrent chats per pod. Chat is much spikier
# than rca-agent's analysis (which caps at 5), so a higher default.
_semaphore: asyncio.Semaphore | None = None


def _get_semaphore() -> asyncio.Semaphore:
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_chats)
    return _semaphore


def _slot_if(needed: bool) -> AbstractAsyncContextManager[Any]:
    """A chat slot when ``needed`` (the step calls Backstage), else nothing to wait for."""
    return _get_semaphore() if needed else contextlib.nullcontext()


def _stamp_current_time(messages: list[dict[str, str]]) -> list[dict[str, str]]:
    """Prefix the latest user turn with the current server time.

    Perch has no clock and must never synthesise timestamps (it hallucinates
    dates), so the frontend normally pre-computes the log window. But the
    observer's ``query_component_logs`` requires an explicit ``end_time``, and
    on an "is it fixed now?" follow-up the frontend window is stale — captured
    when the drawer opened, before the user's fix. We hand the model a
    trustworthy "now" here — per turn, from the server clock — so the prompt
    can anchor a fresh re-fetch on it. Injected into the message rather than the
    system prompt because the rendered system prompt is cached across turns
    (see builder._get_or_render_prompt) and would freeze the timestamp at first
    render.

    The latest user turn is normally the final message, but we scan backward
    for it so a trailing assistant (or other) turn can't silently suppress the
    stamp — the contract is "the latest user turn," wherever it sits.
    """
    idx = next(
        (i for i in reversed(range(len(messages))) if messages[i].get("role") == "user"),
        None,
    )
    if idx is None:
        return messages
    now = get_current_utc().isoformat()
    target = messages[idx]
    stamped = dict(target)
    stamped["content"] = f"[current server time: {now}]\n{target.get('content', '')}"
    return [*messages[:idx], stamped, *messages[idx + 1 :]]


# Words that approve an edit's Step 1 question, and the ways to pick one of
# its numbered choices. A reply approves only when it is made of these and
# nothing else: "yes change to 70" or "make it 90" is a new request.
_APPROVAL_WORDS = (
    r"(?:yes|yeah|yep|ok|okay|sure|please|confirm(?:ed)?|approved?|go ahead|do it"
    r"|please do|sounds good|looks good|correct|that'?s (?:right|correct|it))"
)
_SEP = r"[\s,.!;:]*"
_APPROVAL_ONLY_RE = re.compile(rf"(?:{_APPROVAL_WORDS}{_SEP})+", re.IGNORECASE)
_CHOICE_RE = re.compile(
    rf"(?:{_APPROVAL_WORDS}{_SEP})*"
    r"(?:(?:option|choice|number)\s*)?#?\(?(?:([1-9])|(?:the\s+)?(first|second|third|1st|2nd|3rd)"
    r"(?:\s+(?:one|option))?)\)?"
    rf"{_SEP}(?:{_APPROVAL_WORDS}{_SEP})*",
    re.IGNORECASE,
)
_ORDINALS = {"first": 1, "1st": 1, "second": 2, "2nd": 2, "third": 3, "3rd": 3}
# A numbered choice line in a Step 1 reply: "1. **Proposed rule:** …".
_CHOICE_LINE_RE = re.compile(r"^[ \t]*([1-9])[.)][ \t]", re.MULTILINE)
# What a Step 1 reply that proposes a change has: a "Proposed rule:" line,
# or a condition in backticks (`fact` `operator` `value`, or one span like
# `fact in [...]`) outside its "Current rule:" line. An open question
# ("what should change instead?") has neither.
_PROPOSED_RULE_RE = re.compile(r"Proposed rule\**[ \t]*:", re.IGNORECASE)
_CONDITION_RE = re.compile(
    r"`[^`\n]+`[ \t]+`[^`\n]+`[ \t]+`[^`\n]+`|`[A-Za-z][\w.]*[ \t]+[^`\n]+`",
)
_CURRENT_RULE_LINE_RE = re.compile(r"^.*Current rule\**[ \t]*:.*$", re.IGNORECASE | re.MULTILINE)

_STEP1_NOTE = (
    "[Automatic check — not written by the user] The user hasn't confirmed this change "
    "yet, so don't write the edited rule. Write your reply again as a confirmation: what "
    "the current rule checks, what you understood, the proposed rule (or numbered "
    "choices) with operators and values in backticks, and a question asking the user to "
    "confirm. No code block, no headings, and don't say the rule was validated or tested."
)
# A message that only asks to test the check on a component: "test it on
# order-api", "dry run on component:default/x", "try it on x", and the test
# picker's "Test it on <ref>" (testOn in AssistantChatDrawer.tsx). Anything
# more ("test it with 70 instead") is a change and is confirmed first.
_TEST_REQUEST_RE = re.compile(
    r"(?:(?:please|can you|could you)\s+)?(?:test|dry[- ]?run|try|run)"
    r"(?:\s+(?:it|this|that|the (?:check|rule|edit|change)))?"
    r"\s+(?:on|against|for)\s+(?:the\s+)?(?:component\s+)?"
    r"[`'\"]?(?P<entity>[\w.:/-]+)[`'\"]?(?:\s+component)?(?:\s+please)?[\s.!?]*",
    re.IGNORECASE,
)
# The entity a test request names: a full ref ("component:default/x"), or a
# name with its kind or namespace if given. The name has a letter, so "try it
# for 60" is not read as an entity.
_ENTITY_RE = re.compile(
    r"(?:(?P<kind>[a-z]+):)?(?:(?P<ns>[\w.-]+)/)?(?P<name>[\w.-]*[a-z][\w.-]*)",
    re.IGNORECASE,
)
_TEST_NOTE = (
    "[Automatic note — not written by the user] This message only asks for a test. Run "
    "dry_run_check on that component with the rule from your last reply that has one (the "
    "drafted check or the edited rule), or else the current rule, and report the result in "
    "plain words. Don't change the rule and don't write it in a code block."
)
# Added to a later edit message that doesn't approve the last proposal.
_NOT_APPROVED_NOTE = (
    "[Automatic note — not written by the user] This message doesn't confirm your last "
    "proposal. If it asks for a change, reply with a confirmation question for that "
    "change. Don't write the edited rule."
)
# "add …", "also …", "… too": the user adds a condition to the rule. With
# "instead", "replace" or "remove" it's another kind of change.
_ADD_RE = re.compile(
    r"\b(?:add|adding|also|too|as well|plus|in addition|additionally)\b", re.IGNORECASE,
)
_NOT_ADD_RE = re.compile(r"\b(?:instead|replace|rather than|remove|drop|delete)\b", re.IGNORECASE)
_KEEP_CONDITIONS_NOTE = (
    "[Automatic check — not written by the user] The user asked to add a condition, not to "
    "replace the rule: keep every condition of the current rule exactly as it is and add "
    "the new one under `all`. Write your reply again as a confirmation that shows the whole "
    "proposed rule and asks the user whether to add it, so both must pass. Don't write the "
    "edited rule in a code block."
)


_ALTERNATIVES_NOTE = (
    "[Automatic check — not written by the user] You said no available fact measures what the "
    "user asked for, but you suggested no checks they could write instead. Write your reply "
    "again in the form from \"When no fact fits\": say no available fact measures it, then "
    "after \"Checks you could write instead:\" give two or three checks on facts from the "
    "available facts, each as `<fact>` (<the check in plain words>), and end by asking whether "
    "they want one of these or something else. Use only facts from the available facts."
)

# How many times the model is asked for them: a rewrite can come back with a
# fact that doesn't exist or in a form that can't be read (1 run in 5).
_ALTERNATIVES_TRIES = 2

# The welcome of a check authoring chat, asked for when it opens.
_WELCOME_NOTE = (
    "[Automatic note — not written by the user] The user just opened the check "
    "assistant{editing} and picked how they want to work. Write the welcome in `card`, and the same words in `message`, "
    "short and plain, the way a teammate would say it (see \"How to write\"). Don't "
    "list everything you can do{about}. {level} {first_steps} No rule block."
)

# The options under an edit chat's welcome, set by code: the same as the
# drawer's own intro (EDIT_CHECK_QUESTIONS in editCheckIntro.ts). No "How
# can I improve this?": asking for ideas in your own words still works.
_EDIT_WELCOME_CHOICES = [
    "Explain this rule",
]

# The model's welcome for the experience the user picked (scope.experience).
# A beginner's is fixed words, sent by code (_BEGINNER_WELCOME).
_WELCOME_LEVELS = {
    "experienced": (
        "The user is familiar with checks and asked what they can check. Name the main "
        "areas the \"Available facts\" cover in `choices`: 5 or 6 different areas when the "
        "facts cover that many, one choice each, with `label` and `message` both just the "
        "area's name in 1 to 3 plain words (e.g. \"Code access\", \"Repository hygiene\", "
        "\"Code scanning\", \"Deployment security\", \"Release and reliability\", "
        "\"Ownership\"). No facts, fact names, descriptions or example checks, and no "
        "sections: the portal lists the areas and writes the card's words around them."
    ),
}

# A familiar user's welcome: fixed words from code around the areas the model
# named (2026-11-25), as a plain answer: the area names listed, nothing to
# click, and the question under them. An area named back gets its facts
# (_AREA_NOTE).
_AREAS_TITLE = "Areas you can check"
_AREAS_SUMMARY = (
    "Here are some different areas you can write checks for. Each area groups related "
    "facts the portal collects, like who can change the code or whether code scanning "
    "ran. Would you like to pick one of these areas, or do you have another check in "
    "mind?"
)
_MAX_AREAS = 6
# An answer that names one area: "Code access", "show me code scanning",
# "code scanning facts". A rule that mentions one is drafted as usual.
_AREA_ASK_RE = re.compile(
    r"(?:show me\s+)?(?:the\s+)?(?P<area>.+?)(?:\s+(?:facts|area))?[\s.!?]*",
    re.IGNORECASE,
)
_AREA_NOTE = (
    "[Automatic note — not written by the user] They picked \"{area}\" from your list of "
    "areas: show every fact in that area, as when the user asks to see all the facts for "
    "one area."
)

# The options under a new check's welcome, set by code so they are the same
# every time. Only a beginner gets them; a user familiar with checks is asked
# to describe the rule.
_NEW_WELCOME_CHOICES = {
    "beginner": ["Try an example: the default branch must be protected"],
}

# A beginner's welcome in a new check chat: the same words every time, so
# code sends it at once, with no model call. The question ends `summary`, so
# the card shows it after "How it works".
_BEGINNER_WELCOME = {
    "title": "Let's write your first check",
    "summary": (
        "A check is a rule every service has to pass. For example: the default branch must be "
        "protected. We'll go one step at a time. Need any clarification, or tell me what you'd "
        "like your check to make sure of?"
    ),
    "sections": [{"label": "How it works", "lines": [
        {"label": "Describe it", "value": "In your own words"},
        {"label": "I draft the rule", "value": "You see it before anything is saved"},
        {"label": "Test it", "value": "On one of your components"},
        {"label": "Save it", "value": "As a draft, or publish it"},
    ]}],
}


def _beginner_welcome(scope: dict[str, Any] | None) -> bool:
    """Whether this turn is a beginner's welcome in a new check chat."""
    scope = scope or {}
    return bool(
        scope.get("welcome")
        and scope.get("case_type") == "check_authoring"
        and scope.get("experience") == "beginner"
        and not scope.get("check_id")
    )


def _card_text(card: dict[str, Any]) -> str:
    """A fixed card's words as the reply's text, for the chat history."""
    parts = [card["title"], card["summary"]]
    for section in card.get("sections", []):
        lines = "\n".join(f"- {line['label']}: {line['value']}" for line in section["lines"])
        parts.append(f"{section['label']}:\n{lines}")
    return "\n\n".join(parts)


def _is_example(
    messages: list[dict[str, str]], scope: dict[str, Any] | None, *, past_save: bool = False,
) -> bool:
    """Whether the rule in this beginner's chat is the welcome's example:
    their latest request, a test request aside, is the example's words.
    ``past_save``: a request to save is aside too, for the card of "save it".
    Its save card is practice: a test, then what to do next, no save."""
    scope = scope or {}
    if scope.get("experience") != "beginner" or scope.get("check_id"):
        return False
    for message in reversed(messages):
        if message.get("role") != "user":
            continue
        text = (message.get("content") or "").strip()
        if not _TEST_REQUEST_RE.fullmatch(text) and not (past_save and is_save_request(text)):
            return text == _NEW_WELCOME_CHOICES["beginner"][0]
    return False


def _example_draft(
    messages: list[dict[str, str]], scope: dict[str, Any] | None,
) -> str | None:
    """The practice example drafted by code when the latest message is the
    example's own words in a beginner's new check chat, or None."""
    latest = messages[-1] if messages and messages[-1].get("role") == "user" else {}
    if (latest.get("content") or "").strip() != _NEW_WELCOME_CHOICES["beginner"][0]:
        return None
    if not _is_example(messages, scope):
        return None
    return example_draft(scope or {})


def _welcome_choices(scope: dict[str, Any]) -> list[str]:
    """The fixed options under a welcome: an edit's, or a beginner's; none
    (not the model's) for a user familiar with checks."""
    if scope.get("check_id"):
        return _EDIT_WELCOME_CHOICES
    return _NEW_WELCOME_CHOICES.get(scope.get("experience") or "", [])


def _welcome_note(scope: dict[str, Any]) -> str:
    """What the model is asked for when the user picked a level
    (scope.experience); the opening before it is the drawer's own."""
    experience = scope["experience"]
    level = _WELCOME_LEVELS[experience]
    if scope.get("check_id"):
        return _WELCOME_NOTE.format(
            editing=" to change this check",
            about="; say in one sentence what this check checks now",
            level=level,
            first_steps="Leave `choices` empty: the portal adds the options.",
        )
    return _WELCOME_NOTE.format(
        editing="",
        about="",
        level=level,
        # The areas are the choices; the portal adds the user's own check.
        first_steps="",
    )


def _welcome_areas(card: dict[str, Any] | None) -> list[str]:
    """The areas a familiar user's welcome named, from its choices (or else
    its section labels), at most _MAX_AREAS, each once."""
    card = card or {}
    named = [c.get("label") or c.get("message") for c in card.get("choices") or []]
    if not any(named):
        named = [s.get("label") for s in card.get("sections") or []]
    areas: list[str] = []
    for name in named:
        name = " ".join(str(name or "").split())
        if name and len(name) <= 40 and name.lower() not in {a.lower() for a in areas}:
            areas.append(name)
    return areas[:_MAX_AREAS]


def _areas_text(areas: list[str]) -> str:
    """The areas welcome's words as the reply's text, for the chat history."""
    statement, _, question = _AREAS_SUMMARY.rpartition(". ")
    lines = "\n".join(f"- {area}" for area in areas)
    return f"{_AREAS_TITLE}\n\n{statement}.\n\n{lines}\n\n{question}"


def _picked_area(messages: list[dict[str, str]]) -> str | None:
    """The area the user named in answer to the areas welcome just before,
    or None."""
    if len(messages) < 2 or messages[-1].get("role") != "user":
        return None
    previous = messages[-2]
    content = previous.get("content") or ""
    if previous.get("role") != "assistant" or not content.startswith(_AREAS_TITLE):
        return None
    asked = _AREA_ASK_RE.fullmatch((messages[-1].get("content") or "").strip())
    if not asked:
        return None
    said = asked["area"].lower()
    areas = [line[2:].strip() for line in content.splitlines() if line.startswith("- ")]
    return next((area for area in areas if area.lower() == said), None)


# A message that asks for ideas or an explanation, not for a change.
_DISCUSS_RE = re.compile(
    r"\b(?:suggest\w*|ideas?|improve\w*|recommend\w*|explain\w*|what does|what do|"
    r"what is|what are|how does|how do|why|tell me about)\b",
    re.IGNORECASE,
)
_DISCUSS_NOTE = (
    "[Automatic note — not written by the user] This message asks for ideas, not for a "
    "change. Answer it in plain words and don't propose or write a rule yet. Analyse the "
    "current rule against the \"Available facts\": in `summary`, say in one sentence what "
    "it checks now and what it misses. Then give the two most useful improvements as "
    "`card.sections`, one each: `label` the improvement in plain words, a line \"Why\" "
    "with the risk it closes, and a line \"Example\" with an entity that passes now but "
    "would fail (e.g. \"A repo with protection but 0 required reviews\"). Offer the same "
    "two as card.choices (e.g. \"Also require 2 approving reviews\"), and end by asking "
    "which one they want to add, or what else they have in mind."
)

# A message that asks to understand the check ("Explain this rule"), not for
# ideas: the answer explains, and suggests nothing to change.
_EXPLAIN_RE = re.compile(
    r"\b(?:explain\w*|what does|what do|what is|what are|how does|how do|why|"
    r"tell me about)\b",
    re.IGNORECASE,
)
_EXPLAIN_NOTE = (
    "[Automatic note — not written by the user] This message asks to understand the "
    "check, not to change it. Explain the current rule in plain words and suggest no "
    "change: in `summary`, what it makes sure of in one sentence; then one section "
    "\"What it checks\" (each condition and the value that passes) and one section "
    "\"Examples\" (one kind of entity that passes and one that fails, and why, described by "
    "its value in plain words with no fact names, e.g. \"A private repository\": never "
    "name a real component or service, "
    "since you haven't read its facts). Write no rule and "
    "no improvements and no choices; end by asking whether they need any more "
    "clarification or want to change something."
)

# The words of a message that asks for ideas (improvements, suggestions).
_IDEAS_RE = re.compile(r"\b(?:suggest\w*|ideas?|improve\w*|recommend\w*|better)\b", re.I)

# A change request that names no change ("add another condition", "make it
# stricter"): a request for ideas, not for a change to confirm.
_OPEN_TARGET = r"(?:it|this(?: check)?|the (?:check|rule))"
_OPEN_CHANGE = (
    rf"(?:make {_OPEN_TARGET} (?:stricter|tighter|stronger|more strict)"
    r"|add (?:another |one more |an? (?:new |extra )?|more )?(?:condition|requirement|rule)s?"
    rf"|tighten {_OPEN_TARGET}|harden {_OPEN_TARGET})"
)
_OPEN_CHANGE_RE = re.compile(
    rf"(?:please )?{_OPEN_CHANGE}(?:,? or {_OPEN_CHANGE})?"
    rf"(?: (?:to|in|for) {_OPEN_TARGET})?(?: please)?",
    re.IGNORECASE,
)


# "Improve it by requiring two reviews": an improvement that says what to do
# is a change, not a request for ideas — unless it still asks for some
# ("suggest how to improve it to …") or is a question.
_NAMED_IMPROVEMENT_RE = re.compile(
    r"\b(?:improve\w*|better)\b.*\b(?:by|to|so(?: that)?)\s+"
    r"(?:requir|add|includ|exclud|chang|set|remov|mak|check|fail|pass|only)\w*",
    re.IGNORECASE,
)
_ASKS_SOME_RE = re.compile(r"\b(?:suggest\w*|ideas?|recommend\w*|how)\b", re.IGNORECASE)


def _asks_for_ideas(text: str) -> bool:
    """Whether the user's message asks for ideas or an explanation."""
    if (
        _NAMED_IMPROVEMENT_RE.search(text)
        and "?" not in text
        and not _ASKS_SOME_RE.search(text)
        and not _EXPLAIN_RE.search(text)
    ):
        return False
    return bool(_DISCUSS_RE.search(text))


def _asks_open_change(text: str) -> bool:
    """Whether the user's message asks for a change without naming one."""
    words = re.sub(r"\s+", " ", text).strip().rstrip(".!?").strip()
    return bool(_OPEN_CHANGE_RE.fullmatch(words))


def _asks_to_add(text: str) -> bool:
    """Whether the user's message adds a condition ("add …", "… too") rather
    than replacing or removing one."""
    return bool(_ADD_RE.search(text)) and not _NOT_ADD_RE.search(text)


def _drops_a_condition(reply: str, scope: dict[str, Any] | None) -> bool:
    """Whether a Step 1 proposal removes a condition of the current rule."""
    view = change_view(reply, scope or {})
    return bool(view and any(row.state == "removed" for row in view.rows))


def _edits_saved_rule(scope: dict[str, Any] | None) -> bool:
    """An edit chat that has the saved rule (the chats the Step 1 guards apply to)."""
    scope = scope or {}
    return bool(
        scope.get("case_type") == "check_authoring"
        and scope.get("check_id")
        and scope.get("current_rule")
    )


def _proposes_rule(reply: str) -> bool:
    """Whether a Step 1 reply proposes a rule the user can say yes to."""
    return bool(
        _PROPOSED_RULE_RE.search(reply)
        or _CONDITION_RE.search(_CURRENT_RULE_LINE_RE.sub("", reply))
    )


def _shown_as_input_card(reply: str, scope: dict[str, Any] | None) -> bool:
    """Whether a Step 1 reply's choices are unclear (they differ in more than
    one condition), so the drawer shows the input card instead of them."""
    view = change_view(reply, scope or {})
    return bool(view and not view.clear)


def _approves(answer: str, step1: str) -> bool:
    """Whether ``answer``, and nothing else in it, approves the ``step1`` reply."""
    answer = answer.strip()
    choices = {int(n) for n in _CHOICE_LINE_RE.findall(step1)}
    if _APPROVAL_ONLY_RE.fullmatch(answer):
        # "yes" to two choices doesn't say which one.
        return len(choices) <= 1
    match = _CHOICE_RE.fullmatch(answer)
    if not match:
        return False
    picked = int(match.group(1)) if match.group(1) else _ORDINALS[match.group(2).lower()]
    # "1" to a single proposal means yes; otherwise it must be a listed choice.
    return picked in choices if len(choices) > 1 else picked == 1


def _unsaved_rule_reply(
    messages: list[dict[str, str]], scope: dict[str, Any] | None,
) -> str | None:
    """The last reply with a rule — a drafted check or an edited rule — when
    it isn't saved yet (not the check's saved rule), or None. In an edit
    chat, also None when the user didn't approve that edit (see
    _approved_edit): a test or "save it" must not turn it into a save card."""
    if (scope or {}).get("case_type") != "check_authoring":
        return None
    index = next(
        (
            i
            for i in reversed(range(len(messages)))
            if messages[i].get("role") == "assistant"
            and RULE_FENCE_RE.search(messages[i].get("content") or "")
        ),
        None,
    )
    if index is None:
        return None
    reply = messages[index].get("content") or ""
    if is_saved_rule(reply, scope):
        return None
    if (scope or {}).get("check_id") and not _approved_edit(messages, index, scope):
        logger.info("The last edited rule wasn't approved: no save card from it")
        return None
    return reply


def _approved_edit(
    messages: list[dict[str, str]],
    index: int,
    scope: dict[str, Any] | None,
) -> bool:
    """Whether the edited rule in ``messages[index]`` (an assistant reply) is
    one the user approved: the reply to an approval of a Step 1 proposal, or
    the same rule shown again after a test or save request that followed one.
    A rule the model wrote before approval (it should only ask) never is."""
    shown = messages[index].get("content") or ""
    for i in range(index, -1, -1):
        content = messages[i].get("content") or ""
        if messages[i].get("role") != "assistant" or not RULE_FENCE_RE.search(content):
            continue
        if not same_rule(content, shown):
            return False
        asked = messages[:i]
        if _not_approved_reason(asked, scope) is None:
            return True
        said = asked[-1] if asked and asked[-1].get("role") == "user" else {}
        text = (said.get("content") or "").strip()
        if not (_TEST_REQUEST_RE.fullmatch(text) or is_save_request(text)):
            return False
    return False


def _edit_phase(messages: list[dict[str, str]], scope: dict[str, Any] | None) -> str | None:
    """``"apply"`` (Step 2), ``"propose"`` (Step 1), ``"test"``, ``"discuss"`` or None (not an edit).

    In an edit chat, a message that asks for ideas or an explanation is
    ``"discuss"``: no tools, no change proposed, and the reply is the model's
    own card with its answer and suggested changes.

    A message that only asks to test on a component is ``"test"`` in an edit
    chat, or in a new check's chat once a check is drafted: the model gets
    dry_run_check alone, and the reply is the save card of the draft or
    change again when it isn't saved yet (see stream_chat), else the test card.

    In an edit chat that has the saved rule, a message is Step 2 only when
    the previous assistant reply was a Step 1 question (no rule block) that
    proposed a rule and the user's whole reply approves it. Everything else
    — the first message, a changed request, a question, a "yes" to an open
    question, anything after a card — is Step 1: no tools, no card. Every
    other chat (a new check) is unchanged.
    """
    latest = messages[-1] if messages and messages[-1].get("role") == "user" else {}
    if _TEST_REQUEST_RE.fullmatch((latest.get("content") or "").strip()) and (
        _edits_saved_rule(scope) or _unsaved_rule_reply(messages, scope)
    ):
        logger.info("Edit phase: test")
        return "test"
    if not _edits_saved_rule(scope):
        return None
    if _asks_for_ideas(latest.get("content") or "") or _asks_open_change(
        latest.get("content") or "",
    ):
        logger.info("Edit phase: discuss")
        return "discuss"
    reason = _not_approved_reason(messages, scope)
    if reason is None:
        logger.info("Edit phase: apply")
        return "apply"
    logger.info("Edit phase: propose (%s)", reason)
    return "propose"


def _not_approved_reason(
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
) -> str | None:
    """None when the latest message approves the Step 1 proposal before it
    (Step 2), else why it doesn't. Logs nothing, so _approved_edit can ask
    it about earlier turns."""
    latest = messages[-1] if messages and messages[-1].get("role") == "user" else {}
    previous = next((m for m in reversed(messages) if m.get("role") == "assistant"), None)
    if previous is None:
        return "first message"
    if RULE_FENCE_RE.search(previous.get("content") or ""):
        return "the last reply wasn't a question"
    if not _proposes_rule(previous.get("content") or ""):
        return "the last reply proposed no rule"
    if _shown_as_input_card(previous.get("content") or "", scope):
        # Its choices weren't shown, so a number can't pick one.
        return "the last reply's choices were shown as the input card"
    if _approves(latest.get("content") or "", previous.get("content") or ""):
        return None
    return "not an approval"


def _collects_for_components(check: dict[str, Any], scope: dict[str, Any]) -> bool:
    """Whether the check's facts are collected for components: one of its
    retrievers isn't limited to any kind, or lists Component."""
    by_id = {r.get("id"): r for r in scope.get("available_fact_retrievers") or []}
    return any(
        not by_id[fact_id].get("kinds")
        or "component" in {kind.lower() for kind in by_id[fact_id]["kinds"]}
        for fact_id in check.get("factIds") or []
        if fact_id in by_id
    )


def _entity_ref(named: str, check: dict[str, Any], scope: dict[str, Any]) -> str | None:
    """The full ref of the entity a test request names, or None (the model
    then asks): a name without a kind is a component, as the prompt says,
    only when the check's facts are collected for components."""
    entity = _ENTITY_RE.fullmatch(named.rstrip("."))
    if not entity:
        return None
    kind = (entity["kind"] or "").lower()
    if not kind and not _collects_for_components(check, scope):
        return None
    return f"{kind or 'component'}:{entity['ns'] or 'default'}/{entity['name']}"


def _code_test(
    messages: list[dict[str, str]], scope: dict[str, Any] | None,
) -> tuple[str, dict[str, Any], str | None] | None:
    """What a message that only asks for a test tests, when code can run it
    ("test it on order-api", or the test picker's "Test it on <ref>"), or
    None (the model then reads the message): the entity ref, dry_run_check's
    ``check``, and the drafted check or edit it tests when that isn't saved
    yet."""
    latest = messages[-1] if messages and messages[-1].get("role") == "user" else {}
    asked = _TEST_REQUEST_RE.fullmatch((latest.get("content") or "").strip())
    if not asked:
        return None
    reply = _unsaved_rule_reply(messages, scope)
    if reply is None and not _edits_saved_rule(scope):
        return None
    check = check_to_test(reply, scope or {})
    if not check:
        return None
    entity_ref = _entity_ref(asked["entity"], check, scope or {})
    return (entity_ref, check, reply) if entity_ref else None


async def _tested_reply(
    run: DryRun,
    reply: str | None,
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
    *,
    user_sub: str,
    backstage_token: str | None,
) -> AsyncIterator[str]:
    """How a turn with a test result ends, so the result is always a card:
    the save card again, tested, for a draft or edit that isn't saved yet
    (``reply``), else the test card; and the result's words as the reply."""
    scope = scope or {}
    example = _is_example(messages, scope)
    if reply is not None:
        yield _emit_event(_PREPARING_STEP)
    confirmation = None if reply is None else await build_confirmation(
        with_example_description(reply) if example else reply,
        scope, user_sub=user_sub, backstage_token=backstage_token,
    )
    if confirmation:
        confirmation = {**confirmation, "tested": True}
        if example:
            confirmation["example"] = True
        yield _emit_event(confirmation)
        yield _emit_event(_save_card_ui(
            confirmation, scope, dry_run_note(run, scope, example=example), run,
        ))
    else:
        yield _emit_event({"type": "a2ui", "messages": [dry_run_card_surface(run, scope)]})
    result = dry_run_result_event(run, scope)
    yield _emit_event({"type": "done", "message": _card_text({
        "title": f"Tested on {run.entity_ref or 'the component'}",
        "summary": result["summary"],
        "sections": result["sections"],
    })})


def _run_of_card_rule(dry_runs: list[DryRun], confirmation: dict[str, Any]) -> DryRun | None:
    """The turn's last test when it ran the rule the card saves, else None:
    the model may change the rule after testing, or the card's rule may be
    the validator's rewrite, and the card must not show another rule's result."""
    if not dry_runs or not dry_runs[-1].rule:
        return None
    try:
        tested = unwrap(json.loads(dry_runs[-1].rule))
        saved = unwrap(json.loads(confirmation.get("new_rule") or ""))
    except ValueError:
        return None
    return dry_runs[-1] if tested == saved else None


def _shows_test_after_failure(dry_runs: list[DryRun], scope: dict[str, Any] | None) -> bool:
    """Whether a check authoring turn whose reply failed or came back empty
    ran a test, so its result is shown instead of an error."""
    return bool(dry_runs) and (scope or {}).get("case_type") == "check_authoring"


async def _run_code_test(
    entity_ref: str,
    check: dict[str, Any],
    reply: str | None,
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
    *,
    user_sub: str,
    backstage_token: str,
) -> AsyncIterator[str]:
    """A test request's test (see _code_test), run by code with no model
    call: the same events and cards as a test the model runs, and the
    result's words as the reply. A draft or edit that isn't saved yet
    (``reply``) ends in its save card again, as after the model's test."""
    scope = scope or {}
    args = {"entityRef": entity_ref, "check": check}
    yield _emit_event({
        "type": "tool_call",
        "tool": "dry_run_check",
        "activeForm": _testing_label(entity_ref),
        "args": json.dumps(args),
    })
    run: DryRun | None = None
    try:
        tools = await backstage_tools_as_user(BearerTokenAuth(backstage_token))
        output = await call_backstage_tool(tools, "dry_run_check", args)
        run = read_dry_run(args, output)
        error = ToolError("dry_run_check", json.dumps(output))
    except BackstageToolError as exc:
        error = ToolError("dry_run_check", str(exc))
    except Exception as exc:  # noqa: BLE001 — the problem card says so
        logger.warning("Test run by code failed: %s", exc)
        error = ToolError("dry_run_check", "The check service is unavailable.")
    if run is None:
        view = problem_view(error)
        yield _emit_event({"type": "a2ui", "messages": [notice_card_surface(view)]})
        yield _emit_event({"type": "done", "message": f"{view.title}: {view.summary}"})
        return

    yield _emit_event(dry_run_result_event(run, scope))
    async for event in _tested_reply(
        run, reply, messages, scope, user_sub=user_sub, backstage_token=backstage_token,
    ):
        yield event


def _save_card_ui(
    confirmation: dict[str, Any],
    scope: dict[str, Any] | None,
    note: str | None = None,
    run: DryRun | None = None,
) -> dict[str, Any]:
    """The confirm card again as an A2UI SaveCard, sent right after it.

    Same data and token — read from the event as it goes on the wire, with
    its defaults — so the drawer draws this one and keeps using the
    ``confirmation_required`` event for everything else. ``note``: the
    reply's own words, shown in the card instead of above it.
    """
    wire = ConfirmationRequiredEvent.model_validate(confirmation).model_dump(
        mode="json", exclude_none=True,
    )
    return {"type": "a2ui", "messages": [save_card_surface(wire, scope or {}, note, run)]}


def _add_note(messages: list[dict[str, str]], note: str) -> list[dict[str, str]]:
    """``messages`` with ``note`` appended to the latest user turn."""
    *head, last = messages
    return [*head, {**last, "content": f"{last.get('content', '')}\n\n{note}"}]


async def stream_chat(
    *,
    messages: list[dict[str, str]],
    token: str,
    user_sub: str,
    scope: dict[str, Any] | None = None,
    backstage_token: str | None = None,
) -> AsyncIterator[str]:
    """Run one chat turn and yield NDJSON StreamEvents.

    Event types: tool_call, message_chunk, done, error.

    The agent is read-only — mutating tools are filtered out of the
    catalog and any that slip through are hard-refused by
    ``WriteGuardMiddleware``.

    ``backstage_token`` is the token Backstage issued on the signed-in
    user's behalf for this request (see ``_build_agent``).
    """
    request_id_context.set(f"msg_{uuid.uuid4().hex[:12]}")

    # "save this" right after a drafted check or edited rule: the confirm
    # card again, built by code from that reply — no model call needed. Not
    # for an edited rule the user didn't approve: the model asks first.
    latest_text = (messages[-1].get("content") or "") if messages else ""
    unapproved_edit = (
        bool((scope or {}).get("check_id"))
        and is_save_request(latest_text)
        and _unsaved_rule_reply(messages[:-1], scope) is None
    )
    save_card = None
    try:
        # A save request calls Backstage, so it takes a chat slot like a model
        # turn; any other message returns at once and takes none.
        async with _slot_if(is_save_request(latest_text) and not unapproved_edit):
            if not unapproved_edit:
                save_card = await card_for_save_request(
                    messages, scope, user_sub=user_sub, backstage_token=backstage_token,
                    practice=_is_example(messages, scope, past_save=True),
                )
    except Exception:  # noqa: BLE001 — answer with the model instead
        logger.warning("Save card shortcut failed; answering with the model", exc_info=True)
        save_card = None
    if save_card:
        yield _emit_event(save_card)
        yield _emit_event(_save_card_ui(save_card, scope))
        yield _emit_event({"type": "done", "message": SAVE_REQUEST_REPLY})
        return

    # "Create a separate check" on the card that found a saved check like
    # the draft: the draft's save card, as the draft would have had it, built
    # by code from that reply. The reply is the draft again, so later turns
    # read it as before.
    try:
        async with _slot_if(is_separate_request(latest_text)):
            separate = await card_for_separate_check(
                messages, scope, user_sub=user_sub, backstage_token=backstage_token,
            )
    except Exception:  # noqa: BLE001 — answer with the model instead
        logger.warning("Separate check shortcut failed; answering with the model", exc_info=True)
        separate = None
    if separate:
        separate_card, draft = separate
        yield _emit_event(separate_card)
        yield _emit_event(_save_card_ui(separate_card, scope, reply_words(draft)))
        yield _emit_event({"type": "done", "message": draft})
        return

    # A beginner's welcome: fixed words, so the card at once, no model call.
    if _beginner_welcome(scope):
        logger.info("Beginner welcome sent by code (no model call)")
        yield _emit_event({"type": "a2ui", "messages": [reply_card_surface(
            _BEGINNER_WELCOME, None, _NEW_WELCOME_CHOICES["beginner"],
        )]})
        yield _emit_event({"type": "done", "message": _card_text(_BEGINNER_WELCOME)})
        return

    # The beginner's practice example: the same check every time, so code
    # drafts it, with no model call. Without its fact, or when its card
    # can't be built, the model drafts it as before.
    example_reply = _example_draft(messages, scope)
    if example_reply:
        yield _emit_event(_PREPARING_STEP)
        try:
            # Calls Backstage, so it takes a chat slot like a model turn.
            async with _get_semaphore():
                confirmation = await build_confirmation(
                    example_reply, scope, user_sub=user_sub, backstage_token=backstage_token,
                )
        except Exception:  # noqa: BLE001 — the model drafts it instead
            logger.warning("Building the practice example's card failed", exc_info=True)
            confirmation = None
        if confirmation and confirmation["valid"]:
            logger.info("Practice example drafted by code (no model call)")
            confirmation = {**confirmation, "example": True}
            yield _emit_event(confirmation)
            yield _emit_event(_save_card_ui(confirmation, scope, reply_words(example_reply)))
            yield _emit_event({"type": "done", "message": example_reply})
            return
        logger.warning("The practice example's card couldn't be built; the model drafts it")

    # A message that only asks for a test ("test it on order-api", or the test
    # picker's "Test it on <ref>"): the rule and the component are known, so
    # code runs the test, with no model call, and the result is always a
    # card. Anything code can't read goes to the model as before.
    test = _code_test(messages, scope) if backstage_token and user_sub else None
    if test:
        logger.info("Test run by code (no model call)")
        # Calls Backstage, so it takes a chat slot like a model turn.
        async with _get_semaphore():
            async for event in _run_code_test(
                *test, messages, scope, user_sub=user_sub, backstage_token=backstage_token or "",
            ):
                yield event
        return

    # Stamp the server clock onto the latest user turn ONCE and share it with
    # both the normal streaming path and the recovery fallback, so they anchor
    # on the same "now" (see _stamp_current_time).
    stamped_messages = _stamp_current_time(messages)
    edit_phase = _edit_phase(messages, scope)
    # A later edit message that doesn't approve the last proposal: tell the
    # model so, instead of leaving it to read the reply as a confirmation.
    if edit_phase == "propose" and any(m.get("role") == "assistant" for m in messages):
        stamped_messages = _add_note(stamped_messages, _NOT_APPROVED_NOTE)
    elif edit_phase == "test":
        stamped_messages = _add_note(stamped_messages, _TEST_NOTE)
    elif edit_phase == "discuss":
        latest_text = (messages[-1].get("content") or "") if messages else ""
        explains = _EXPLAIN_RE.search(latest_text) and not _IDEAS_RE.search(latest_text)
        stamped_messages = _add_note(
            stamped_messages, _EXPLAIN_NOTE if explains else _DISCUSS_NOTE,
        )
    # An area picked from a familiar user's welcome (its button sends just the
    # area's name): that area's facts, not checks to write for it.
    picked_area = _picked_area(messages)
    if picked_area:
        stamped_messages = _add_note(stamped_messages, _AREA_NOTE.format(area=picked_area))
    # The user picked a level in a new check authoring chat: the model writes
    # the welcome for it, from the scope alone, with no tools and nothing to
    # save. Without a level the flag is ignored (the opening is the drawer's);
    # a beginner's welcome was sent above.
    if (
        (scope or {}).get("welcome")
        and (scope or {}).get("case_type") == "check_authoring"
        and (scope or {}).get("experience") in _WELCOME_LEVELS
    ):
        edit_phase = "welcome"
        note = _welcome_note(scope or {})
        # The level the user picked arrives as their first message.
        said = (messages[-1].get("content") or "").strip() if messages else ""
        if said:
            note += f' They said: "{said}"'
        stamped_messages = [{"role": "user", "content": note}]

    # dry_run_check results this turn, for the test card: kept outside the
    # try, so a test that ran still shows its result if the reply fails.
    dry_runs: list[DryRun] = []

    async with _get_semaphore():
        try:
            agent, _tools, _tools_by_name = await _build_agent(
                token,
                scope,
                user_sub=user_sub,
                backstage_token=backstage_token,
                edit_phase=edit_phase,
            )
            parser = ChatResponseParser()
            # A check authoring reply is shown as a card once it is complete,
            # and an edit reply waiting for the user's "yes" may be rewritten
            # below, so its text is sent only once checked, in ``done`` —
            # the user never sees text that gets replaced. Other chats
            # stream as before.
            stream_text = (scope or {}).get("case_type") != "check_authoring"

            # Tool errors since the last tool that worked, for the problem card.
            tool_errors: list[ToolError] = []
            async for ev in _yield_chunks(
                agent, stamped_messages, parser, stream_text=stream_text, dry_runs=dry_runs,
                scope=scope,
                writing_label=_writing_label(
                    edit_phase, scope, (messages[-1].get("content") or "") if messages else "",
                ),
                tool_errors=tool_errors,
            ):
                yield ev

            # Final delta flush — when the model emits the structured
            # response as a single text block (some tool-using turns), the
            # parser's ``pop_delta`` inside ``_yield_chunks`` already
            # fired; this trailing call is idempotent and returns ""
            # in that case. Kept as a safety net so a never-fired-mid-
            # stream model still gets one ``message_chunk`` before
            # ``done`` lands.
            trailing = parser.pop_delta()
            if trailing and stream_text:
                yield _emit_event(
                    {"type": "message_chunk", "content": trailing},
                )

            # Check authoring: the card the model filled for this reply.
            card = parser.card

            # fix_prompt is a sibling field on the same ChatResponse JSON
            # the parser was reading; no second source of truth needed.
            # Cap at the wire-side backstop so a misbehaving model can't
            # ship an N-megabyte payload to the frontend.
            fix_prompt = (parser.fix_prompt or "").strip()
            if len(fix_prompt) > _FIX_PROMPT_MAX_CHARS:
                logger.warning(
                    "fix_prompt exceeded %d chars (got %d); truncating",
                    _FIX_PROMPT_MAX_CHARS, len(fix_prompt),
                )
                fix_prompt = fix_prompt[:_FIX_PROMPT_MAX_CHARS]

            # A check authoring reply the model wrote only in its card (the
            # welcome often is): the card is the reply, so its words stand in
            # for the empty message instead of failing the turn.
            if (
                not parser.message
                and card
                and (scope or {}).get("case_type") == "check_authoring"
            ):
                words = " ".join(
                    filter(None, [card.get("title"), card.get("summary")]),
                ).strip()
                if words:
                    parser.set_message(words)

            # Post-stream recovery: ProviderStrategy is expected to fill
            # ``parser.message`` from streamed text (or, on rare fallback,
            # from ChatResponse tool-call args we also fed in above). If
            # both paths produced nothing, the model finished without an
            # answer — emit an explicit ``error`` event rather than a
            # ``done`` with empty ``message``. The drawer drops empty
            # done.message silently, which would leave the user staring
            # at a cleared streaming buffer with no signal at all.
            if not parser.message and _shows_test_after_failure(dry_runs, scope):
                # No reply, but a test ran: its result is the reply.
                logger.warning("No reply after a test; showing the test's result")
                async for event in _tested_reply(
                    dry_runs[-1],
                    _unsaved_rule_reply(messages, scope) if edit_phase == "test" else None,
                    messages, scope, user_sub=user_sub, backstage_token=backstage_token,
                ):
                    yield event
                return
            if not parser.message:
                logger.warning(
                    "[%s] parser.message empty after stream — likely the model "
                    "produced no ChatResponse via either text or tool-call args; "
                    "emitting error so the drawer surfaces a failure",
                    request_id_context.get(),
                )
                yield _emit_event(
                    {
                        "type": "error",
                        "message": (
                            "The assistant didn't produce a response "
                            f"(request_id: {request_id_context.get()}). "
                            "Please try again."
                        ),
                    },
                )
                return

            message = parser.message

            # A new check's rule has only the conditions the user asked for:
            # the guard the model sometimes adds is taken out (the prompt says
            # so too; this holds when it doesn't).
            if (scope or {}).get("case_type") == "check_authoring" and not (scope or {}).get(
                "check_id",
            ):
                message = without_unrequested_guard(
                    message, [m.get("content") or "" for m in messages if m.get("role") == "user"],
                )

            # An unapproved edit message must get the Step 1 question. When
            # the model wrote the edited rule instead, it gets one round to
            # write the question; DryRunStop keeps the tools off for it. The
            # first reply is kept if the rewrite fails or still has a rule.
            if edit_phase == "propose" and RULE_FENCE_RE.search(message):
                yield _emit_event({
                    "type": "tool_call",
                    "tool": "ask_first",
                    "activeForm": _writing_label(edit_phase, scope),
                })
                rewritten = await _rewrite_reply(
                    agent, stamped_messages, message, _STEP1_NOTE, must_contain="?",
                )
                if rewritten and not RULE_FENCE_RE.search(rewritten):
                    logger.info("Edit reply rewritten as the Step 1 question")
                    message, card = rewritten, None
                else:
                    logger.info("Edit reply kept: the rewrite failed or still has a rule")

            # The user asked to add a condition, but the proposal drops one
            # of the current rule's: one round for the model to keep them all
            # and ask whether to add it.
            latest_text = (
                messages[-1].get("content") or ""
                if messages and messages[-1].get("role") == "user" else ""
            )
            if (
                edit_phase == "propose"
                and _asks_to_add(latest_text)
                and _drops_a_condition(message, scope)
            ):
                yield _emit_event({
                    "type": "tool_call",
                    "tool": "keep_conditions",
                    "activeForm": _writing_label(edit_phase, scope),
                })
                rewritten = await _rewrite_reply(
                    agent, stamped_messages, message, _KEEP_CONDITIONS_NOTE, must_contain="?",
                )
                if (
                    rewritten
                    and not RULE_FENCE_RE.search(rewritten)
                    and not _drops_a_condition(rewritten, scope)
                ):
                    logger.info("Edit reply rewritten to keep the current conditions")
                    message, card = rewritten, None
                else:
                    logger.warning(
                        "Edit reply drops a condition the user asked to keep: the rewrite "
                        "failed or still drops it",
                    )

            # No fact fits, but the reply offers nothing to pick instead: the
            # card would show only the bare question. Up to two rounds to name the
            # closest checks the user could write.
            if (
                (scope or {}).get("case_type") == "check_authoring"
                and not RULE_FENCE_RE.search(message)
                and lacks_alternatives(message, scope or {})
            ):
                yield _emit_event({
                    "type": "tool_call",
                    "tool": "suggest_alternatives",
                    "activeForm": _writing_label(edit_phase, scope),
                })
                for _ in range(_ALTERNATIVES_TRIES):
                    rewritten = await _rewrite_reply(
                        agent, stamped_messages, message, _ALTERNATIVES_NOTE,
                        must_contain="Checks you could write instead",
                    )
                    if rewritten and not lacks_alternatives(rewritten, scope or {}):
                        logger.info("No-fact reply rewritten with checks to write instead")
                        message, card = rewritten, None
                        break
                    if rewritten:
                        logger.warning(
                            "No-fact rewrite has no usable alternatives: %s",
                            _log_excerpt(rewritten),
                        )
                else:
                    logger.warning(
                        "No-fact reply has no alternatives: the rewrites failed or still have none",
                    )

            # The prompt's "Step 1 — confirm the change" style headings mean
            # nothing to the user; the model sometimes copies them anyway.
            if edit_phase:
                message = strip_step_labels(message)

            # Step 2 of an edit: the change as a confirm card, built by code
            # from the final message and saved only on the user's click.
            # Sent before ``done``; None for every other reply, and for an
            # edit the user hasn't approved yet (see _edit_phase). A test of
            # a draft or change that isn't saved yet ends in its save card
            # again, so what's left is to save it.
            tested_reply = (
                _unsaved_rule_reply(messages, scope) if edit_phase == "test" and dry_runs else None
            )
            # The welcome's practice example has fixed Description words, so a
            # fact's own wording ("evaluated branch") never gets into its card.
            practice = _is_example(messages, scope)
            # Code checks the rule again and signs the save card: the
            # progress line says so instead of sitting on the last step.
            if (scope or {}).get("case_type") == "check_authoring" and (
                tested_reply
                or (
                    edit_phase not in ("propose", "test", "discuss", "welcome")
                    and RULE_FENCE_RE.search(message)
                )
            ):
                yield _emit_event(_PREPARING_STEP)
            if edit_phase not in ("propose", "test", "discuss", "welcome"):
                confirmation = await build_confirmation(
                    with_example_description(message) if practice else message,
                    scope, user_sub=user_sub, backstage_token=backstage_token,
                )
            elif tested_reply:
                confirmation = await build_confirmation(
                    with_example_description(tested_reply) if practice else tested_reply,
                    scope, user_sub=user_sub, backstage_token=backstage_token,
                )
            else:
                confirmation = None
                if RULE_FENCE_RE.search(message):
                    logger.info(
                        "No confirm card: %s",
                        {
                            "test": "the message only asks for a test",
                            "discuss": "the message asks for ideas or an explanation",
                            "welcome": "it is the welcome",
                        }.get(edit_phase or "", "the edit isn't approved yet (Step 1)"),
                    )
            # The card's own validation found problems (e.g. the model
            # skipped validate_check): one round for the model to fix them.
            # Not after a test: the card is an earlier reply's, not this one's.
            if confirmation and not confirmation["valid"] and edit_phase != "test":
                yield _emit_event({
                    "type": "tool_call",
                    "tool": "fix_rule",
                    "activeForm": "Checking the rule again",
                })
                rewritten = await _rewrite_reply(
                    agent, stamped_messages, message,
                    rule_fix_note(confirmation["problems"]), must_contain="```",
                )
                if rewritten:
                    fixed = await build_confirmation(
                        rewritten, scope, user_sub=user_sub, backstage_token=backstage_token,
                    )
                    if fixed:
                        message, confirmation, card = rewritten, fixed, None
            # A drafted new check: first a saved check like it, read as the
            # user. When there is one, its card recommends editing that check
            # and says why, in place of the save card, and the user can still
            # create a separate check from it. Not for the welcome's practice
            # example, a test of the draft or a rule with problems.
            existing = None
            if (
                confirmation
                and confirmation.get("kind") == "create"
                and confirmation["valid"]
                and edit_phase is None
                and not _is_example(messages, scope)
            ):
                yield _emit_event({
                    "type": "tool_call",
                    "tool": "list_checks",
                    "activeForm": _active_form("list_checks"),
                })
                existing = await find_existing_check(
                    confirmation, messages, backstage_token=backstage_token,
                )
            if existing:
                yield _emit_event({
                    "type": "a2ui",
                    "messages": [existing_check_surface(
                        existing_check_view(existing, confirmation, scope or {}),
                    )],
                })
                confirmation = None
            if confirmation:
                note = reply_words(message)
                example = _is_example(messages, scope)
                if example:
                    confirmation = {**confirmation, "example": True}
                run = _run_of_card_rule(dry_runs, confirmation)
                if run:
                    # Tested in this turn: the card says the result in the
                    # test card's words and offers only to save, not to test.
                    confirmation = {**confirmation, "tested": True}
                    note = "\n\n".join(filter(None, [
                        dry_run_note(run, scope or {}, example=example),
                        None if edit_phase == "test" else without_test_summary(note),
                    ]))
                elif dry_runs:
                    # The test ran another rule: its result isn't this rule's,
                    # so the card offers to test this one.
                    logger.info("The test ran another rule than the card's; card not marked tested")
                    note = without_test_summary(note)
                elif tool_errors and (failed_test := failed_test_note(tool_errors[-1])):
                    # The test of this draft failed: say so from the tool's own
                    # error, not the model's words, and not as "Tested on".
                    note = "\n\n".join(filter(None, [failed_test, without_failed_test_lines(note)]))
                yield _emit_event(confirmation)
                yield _emit_event(_save_card_ui(confirmation, scope, note, run))

            # An edit question waiting for the user's answer: a card that
            # shows the change in plain words, with answer buttons, built by
            # code (plain buttons when the rule can't be shown exactly). A
            # click sends the button's text as the next message, so
            # _edit_phase still decides. An edit reply that proposes no rule
            # (an open question, options in words) gets no Yes / No — there
            # is nothing to say yes to — but an ask card: the rule with an
            # editor per value, or the options, and a text box.
            # A check authoring reply that drafts no rule ("no fact
            # measures …", "which value?") gets a notice card the same way.
            # A dry-run's result gets the test card, built from the tool's own
            # result — unless a save card was sent: a second card would
            # replace it in the drawer.
            surface = None
            check_name = (scope or {}).get("check_name")
            # A turn whose last tool call failed gets the problem card, built
            # from the tool's own error.
            tested = dry_runs[-1] if dry_runs and not confirmation else None
            failed = tool_errors[-1] if tool_errors and not tested and not confirmation else None
            # The save card of an earlier reply, sent after a test, is this
            # reply's only card: another would replace it in the drawer. So is
            # the card of a saved check like the draft.
            resent = bool(tested_reply and confirmation)
            if (
                (scope or {}).get("case_type") == "check_authoring"
                and not resent
                and not existing
                and (tested or failed or not RULE_FENCE_RE.search(message))
            ):
                notice = None if tested or failed else notice_view(message, scope or {})
                if tested:
                    surface = dry_run_card_surface(tested, scope or {})
                elif failed:
                    surface = notice_card_surface(problem_view(failed))
                elif notice:
                    surface = notice_card_surface(notice)
                elif edit_phase in ("discuss", "welcome"):
                    # Its answer and suggestions are the model's own card
                    # (below): a change card or form would hide them.
                    pass
                elif edit_phase == "propose" and _proposes_rule(message):
                    view = change_view(message, scope or {})
                    if view and view.clear:
                        surface = change_card_surface(view, check_name)
                    elif view and (ask := edit_view(scope or {}, understood(message))):
                        # Choices that differ in more than one condition
                        # read badly as buttons: ask for the value instead.
                        surface = ask_card_surface(ask, check_name)
                    elif view:
                        surface = change_card_surface(view, check_name)
                    elif len(choices := numbered_choices(message)) > 1:
                        surface = choice_card_surface(
                            choices, understood(message), scope or {}, check_name,
                        )
                    else:
                        surface = edit_question_surface()
                elif edit_phase and not _proposes_rule(message):
                    ask = ask_view(message, scope or {})
                    if ask:
                        surface = ask_card_surface(ask, check_name)
                # A familiar user's welcome: the areas the model named, listed
                # in fixed words, with the question under them.
                areas = (
                    _welcome_areas(card)
                    if edit_phase == "welcome" and not (scope or {}).get("check_id")
                    else []
                )
                if surface is None and areas:
                    surface = areas_card_surface(_AREAS_TITLE, _AREAS_SUMMARY, areas)
                    message = _areas_text(areas)
                # Any other check authoring reply without a rule: the card
                # the model filled, or else its text in a card, so every
                # reply looks the same.
                if surface is None:
                    surface = reply_card_surface(
                        card, check_name,
                        _welcome_choices(scope or {}) if edit_phase == "welcome" else None,
                    )
                if surface is None and message.strip():
                    surface = message_card_surface(message)
            if surface:
                shown = surface["createSurface"]["components"][0]["component"]
                logger.info(
                    "Reply shown as %s%s", shown,
                    {
                        "ReplyCard": " (the model's card: no code card read the reply)",
                        "MessageCard": " (no code card read the reply, the model's card unusable)",
                    }.get(shown, ""),
                )
                yield _emit_event({"type": "a2ui", "messages": [surface]})

            done_payload: dict[str, Any] = {
                "type": "done",
                "message": message,
            }
            if fix_prompt:
                done_payload["fix_prompt"] = fix_prompt
            yield _emit_event(done_payload)

        except Exception as e:
            if _shows_test_after_failure(dry_runs, scope):
                # The reply failed after a test: its result is still shown.
                logger.error("Reply failed after a test; showing its result: %s", e, exc_info=True)
                async for event in _tested_reply(
                    dry_runs[-1],
                    _unsaved_rule_reply(messages, scope) if edit_phase == "test" else None,
                    messages, scope, user_sub=user_sub, backstage_token=backstage_token,
                ):
                    yield event
                return
            async for err_event in _handle_stream_error(e, stamped_messages, scope):
                yield err_event


# ── Helpers used by stream_chat ────────────────────────────────────


_REWRITE_TIMEOUT_S = 30


async def _rewrite_reply(
    agent: Runnable,
    messages: list[dict[str, str]],
    draft: str,
    correction: str,
    *,
    must_contain: str,
) -> str | None:
    """The model's reply written again after a code check, or None.

    Runs the same agent once more, not streamed, with the first draft and
    the correction appended. On any failure, or a reply without
    ``must_contain`` (e.g. no proposal, no rule block), the first draft is
    kept.
    """
    history = [
        *messages,
        {"role": "assistant", "content": draft},
        {"role": "user", "content": correction},
    ]
    try:
        state = await asyncio.wait_for(
            agent.ainvoke({"messages": history}), _REWRITE_TIMEOUT_S,
        )
    except Exception as exc:  # noqa: BLE001 — keep the first draft
        logger.warning("Rewrite failed (%s); keeping the first reply", exc)
        return None
    response = state.get("structured_response") if isinstance(state, dict) else None
    text = getattr(response, "message", None)
    if not isinstance(text, str) or must_contain not in text:
        logger.warning("Rewrite didn't keep the expected shape; keeping the first reply")
        return None
    logger.info("Reply rewritten after a code check")
    return text


def _log_excerpt(text: str, limit: int = 600) -> str:
    """``text`` on one line, cut at ``limit`` characters, for a log line."""
    flat = " ".join(text.split())
    return flat if len(flat) <= limit else flat[: limit - 1] + "…"


def _tool_text(message: Any) -> str:
    content = getattr(message, "content", "")
    if isinstance(content, list):
        return " ".join(b.get("text", "") if isinstance(b, dict) else str(b) for b in content)
    return str(content)


async def _yield_chunks(
    agent: Runnable,
    messages: list[dict[str, str]],
    parser: ChatResponseParser,
    *,
    stream_text: bool = True,
    dry_runs: list[DryRun] | None = None,
    tool_errors: list[ToolError] | None = None,
    scope: dict[str, Any] | None = None,
    writing_label: str | None = None,
) -> AsyncIterator[str]:
    """Stream LangChain agent chunks and emit per-chunk NDJSON events.

    Uses ``stream_mode="messages"`` (single mode) — same as rca-agent.
    In single-mode, LangGraph routes the structured-output JSON through
    the messages channel as TEXT blocks that stream token-by-token. The
    dual ``["messages", "values"]`` form (previously used here) routes
    the structured response into the ``values`` channel as one final
    snapshot, which collapsed the final answer into a single dump on
    the wire. We pay for the simpler streaming by losing the agent's
    final ``structured_response`` snapshot — fine because the parser
    extracts both ``message`` and ``fix_prompt`` from the streamed
    JSON itself.

    Each text-block push is followed by a ``pop_delta`` call so the
    drawer paints text as it arrives. With ``stream_text=False`` the
    parser still reads every chunk but no ``message_chunk`` is sent;
    the text arrives only in ``done``, and a ``write_reply`` tool_call
    labelled ``writing_label`` (default "Writing the reply") tells the
    drawer when the model starts writing it.

    Each dry_run_check result is added to ``dry_runs``, with the component
    and rule from its call, and sent at once as a ``dry_run_result`` event
    in its test card's words (fact labels from ``scope``). ``tool_errors``
    holds the tool errors since the last tool that worked: empty when the
    turn's last tool call succeeded.
    """

    def _flush_delta() -> str | None:
        delta = parser.pop_delta()
        if not delta or not stream_text:
            return None
        return _emit_event({"type": "message_chunk", "content": delta})

    # ChatResponse tool-call indices seen so far. The model emits the
    # ``name`` once at the start of a tool call; subsequent chunks for
    # the same index carry only ``args``. Persisting this set across
    # chunks lets the parser keep reassembling JSON after the first
    # chunk reveals the name.
    chat_response_indices: set[int] = set()
    # Tool call arguments by call id, streamed in pieces by index, so a
    # dry_run_check result can be read with the component and rule it ran.
    call_ids: dict[int, str] = {}
    call_args: dict[str, str] = {}
    # Each call's tool by call id, and the tests whose step already names
    # their component.
    call_names: dict[str, str] = {}
    named_tests: set[str] = set()
    # A held-back reply shows nothing while the model writes it, so the
    # drawer is told when that starts — again if a tool ran in between.
    writing = False
    # Whether this turn checked a rule (see _DRAFTING_LABEL).
    checked_rule = False

    try:
        async for chunk, _ in agent.astream(
            {"messages": list(messages)},
            stream_mode="messages",
        ):
            # Drop ToolMessage chunks. They also carry ``content_blocks``
            # with ``type: "text"`` blocks (the tool's result text), and
            # if we let those reach ``parser.push`` they pollute the JSON
            # buffer — partial-JSON parsing breaks for the rest of the
            # turn and the final message_chunk events never fire.
            # ``ToolMessage.type == "tool"``; AIMessageChunk's class
            # attribute is "AIMessageChunk", not "ai" (don't filter on
            # equality with "ai" — older RCA pattern relied on a
            # ``content: str`` check that no longer holds in current
            # langchain-core, where AI chunks also have list content).
            if getattr(chunk, "type", None) == "tool":
                if tool_errors is not None:
                    if getattr(chunk, "status", None) == "error":
                        tool_errors.append(ToolError(
                            getattr(chunk, "name", None) or "", _tool_text(chunk),
                        ))
                    else:
                        tool_errors.clear()
                if getattr(chunk, "name", None) in _RULE_TOOLS:
                    checked_rule = True
                if dry_runs is not None and getattr(chunk, "name", None) == "dry_run_check":
                    run = read_dry_run(
                        call_args.get(getattr(chunk, "tool_call_id", None) or ""),
                        getattr(chunk, "content", None),
                    )
                    if run:
                        dry_runs.append(run)
                        yield _emit_event(dry_run_result_event(run, scope or {}))
                # The tool is done and the model thinks about what comes next:
                # a held-back reply's progress line says so, instead of still
                # naming the tool.
                if not stream_text:
                    yield _emit_event(_THINKING_STEP)
                continue

            # ChatResponse can land via either path depending on what
            # LangChain decides per turn — ProviderStrategy normally
            # streams the JSON as ``text`` blocks, but a model/runtime
            # quirk can still emit it as a final ``ChatResponse``
            # tool-call whose args carry the same JSON shape. We feed
            # both channels into the parser so the post-stream guard
            # below has something to read even on the rare fallback.
            for tc in getattr(chunk, "tool_call_chunks", None) or []:
                idx = tc.get("index")
                name = tc.get("name")
                if name in RESPONSE_NAMES and idx is not None:
                    chat_response_indices.add(idx)
                if tc.get("id") and idx is not None:
                    call_ids[idx] = tc["id"]
                    call_args[tc["id"]] = ""
                    if name:
                        call_names[tc["id"]] = name
                if idx in call_ids and tc.get("args"):
                    call_args[call_ids[idx]] += tc["args"]

            pushed = False
            for block in getattr(chunk, "content_blocks", None) or []:
                block_type = block.get("type")
                if block_type == "tool_call_chunk":
                    tool_name = block.get("name")
                    if tool_name and tool_name not in RESPONSE_NAMES:
                        # Don't surface the structured-output tool as a
                        # tool_call event — it isn't a real read tool the
                        # user should see in the working indicator.
                        writing = False
                        if tool_name in _RULE_TOOLS:
                            checked_rule = True
                        yield _emit_event(
                            {
                                "type": "tool_call",
                                "tool": tool_name,
                                "activeForm": _active_form(tool_name),
                                "args": block.get("args", ""),
                            }
                        )
                elif block_type == "text":
                    text = block.get("text", "")
                    if text:
                        parser.push(text)
                        pushed = True

            # A test's step names its component as soon as the model's
            # arguments say which one ("Testing the check on order-api").
            for call_id, tool_name in call_names.items():
                if tool_name != "dry_run_check" or call_id in named_tests:
                    continue
                named = _ENTITY_REF_ARG_RE.search(call_args.get(call_id, ""))
                if named:
                    named_tests.add(call_id)
                    yield _emit_event({
                        "type": "tool_call",
                        "tool": "dry_run_check",
                        "activeForm": _testing_label(named[1]),
                    })

            # If THIS chunk carries ChatResponse tool-call args, push
            # the partial JSON into the same parser so the message /
            # fix_prompt fields surface either way.
            for tc in getattr(chunk, "tool_call_chunks", None) or []:
                idx = tc.get("index")
                args_str = tc.get("args") or ""
                if idx in chat_response_indices and args_str:
                    parser.push(args_str)
                    pushed = True

            if pushed and not stream_text and not writing:
                writing = True
                label = writing_label or _WRITING_LABEL
                # A reply that drafts a check checks its rule first: without
                # that, it answers a question, whatever the message said.
                if label == _DRAFTING_LABEL and not checked_rule:
                    label = _WRITING_LABELS["discuss"]
                yield _emit_event({
                    "type": "tool_call", "tool": "write_reply", "activeForm": label,
                })

            # One flush per outer chunk — keeps the event rate proportional
            # to the model's token rate without firing on every micro-update.
            delta_event = _flush_delta()
            if delta_event is not None:
                yield delta_event
    except StructuredOutputValidationError as exc:
        # Log why parsing failed without what the model sent: the reply (and
        # a pydantic error's text) can echo the user's words or tool data.
        raw = getattr(exc.ai_message, "content", "")
        source = exc.source
        errors = (
            [(e["loc"], e["type"]) for e in source.errors(include_input=False, include_url=False)]
            if isinstance(source, ValidationError)
            else None
        )
        logger.warning(
            "Structured output validation failed; using streamed content "
            "(error: %s %s; raw output length: %d)",
            type(source).__name__,
            errors,
            len(str(raw)),
        )


# Hard cap on the fix_prompt payload. The system-prompt instruction asks
# the model to keep it ≤ 2000 chars (see perch_prompt.j2's BUILD-FAILURE
# block); this is the wire-side backstop that mirrors that contract in
# case the model overshoots. A pasted prompt at this size already
# stretches the CodeRabbit/Cursor input boxes — anything bigger is
# almost always log noise that dilutes the actionable excerpt.
_FIX_PROMPT_MAX_CHARS = 2_000


async def _handle_stream_error(
    err: Exception,
    messages: list[dict[str, str]],
    scope: dict[str, Any] | None,
) -> AsyncIterator[str]:
    """Translate a stream-loop exception into user-facing events."""
    logger.error("Chat stream error: %s", err, exc_info=True)
    err_str = str(err)
    is_recursion = (
        "Recursion limit" in err_str or "GRAPH_RECURSION_LIMIT" in err_str
    )
    if is_recursion:
        try:
            recovery = await recover_with_fallback(
                messages=list(messages), scope=scope,
            )
        except Exception as fallback_err:  # noqa: BLE001
            logger.error(
                "recover_with_fallback unexpectedly raised: %s",
                fallback_err,
                exc_info=True,
            )
            yield _emit_event(
                {
                    "type": "error",
                    "message": (
                        f"An internal error occurred (request_id: {request_id_context.get()})."
                    ),
                },
            )
            yield _emit_event({"type": "done", "message": ""})
        else:
            logger.warning(
                "Chat recovered via fallback path: source=%s request_id=%s",
                recovery.source,
                request_id_context.get(),
            )
            yield _emit_event({"type": "message_chunk", "content": recovery.text})
            yield _emit_event(
                {
                    "type": "done",
                    "message": recovery.text,
                    "recovery": recovery.source,
                },
            )
    else:
        yield _emit_event(
            {
                "type": "error",
                "message": (
                    f"An internal error occurred (request_id: {request_id_context.get()})."
                ),
            },
        )
        yield _emit_event({"type": "done", "message": ""})


# What the user is waiting for, by check authoring tool. No trailing "…":
# the drawer animates its own dots after the label.
_TOOL_LABELS = {
    "validate_check": "Checking the rule",
    "dry_run_check": "Testing the check",
    "get_entity_facts": "Reading the component's facts",
    "list_fact_schemas": "Reading the facts you can use",
    "list_checks": "Looking at existing checks",
    "get_check": "Looking at existing checks",
}
# Between a tool's result and the model's next step.
_THINKING_STEP = {"type": "tool_call", "tool": "think", "activeForm": "Thinking"}
# The component in a dry_run_check call's streamed arguments, once complete.
_ENTITY_REF_ARG_RE = re.compile(r'"entityRef"\s*:\s*"([^"]+)"')


def _testing_label(entity_ref: str | None) -> str:
    """The test step, naming the component when it is known:
    "Testing the check on order-api"."""
    if not entity_ref:
        return _active_form("dry_run_check")
    name, _kind = entity_name(entity_ref)
    return f"{_active_form('dry_run_check')} on {name}"
# Code's own step after the model's: checking the rule again and signing the
# save card (build_confirmation), so the progress line shows it too.
_PREPARING_STEP = {
    "type": "tool_call",
    "tool": "prepare_save_card",
    "activeForm": "Preparing the save card",
}
_WRITING_LABEL = "Writing the reply"
# A new check's reply that drafts the rule; shown only when the turn checked
# a rule (_RULE_TOOLS), so a question never says it drafts a check.
_DRAFTING_LABEL = "Drafting the check"
_RULE_TOOLS = frozenset({"validate_check", "dry_run_check"})
# The last step of a held-back reply, by what the request is, so the drawer
# shows the step the user asked for (see _writing_label).
_WRITING_LABELS = {
    "propose": "Working out the change",
    "apply": "Writing the change",
    "test": "Writing up the test",
    "discuss": "Writing the answer",
    "welcome": "Getting ready",
}


# A new check's message that asks for help or a question, not for a rule:
# the reply guides instead of drafting (see the prompt's guidance).
_GUIDE_RE = re.compile(r"\bhelp me\b|\bwhich\b|\bchoose\b|\?\s*$", re.IGNORECASE)


def _writing_label(
    edit_phase: str | None, scope: dict[str, Any] | None, latest: str = "",
) -> str:
    """What the drawer says while the model writes the reply: the step of an
    edit or a test, drafting a new check (or answering, when the ``latest``
    message asks for help), or else just writing the reply."""
    if edit_phase in _WRITING_LABELS:
        return _WRITING_LABELS[edit_phase]
    scope = scope or {}
    if scope.get("case_type") == "check_authoring" and not scope.get("check_id"):
        if _asks_for_ideas(latest) or _GUIDE_RE.search(latest):
            return _WRITING_LABELS["discuss"]
        return _DRAFTING_LABEL
    return _WRITING_LABEL


def _active_form(tool_name: str) -> str:
    """Best-effort human-readable progress label rendered while a tool runs."""
    if tool_name in _TOOL_LABELS:
        return _TOOL_LABELS[tool_name]
    name = tool_name.replace("_", " ")
    if tool_name.startswith("list_"):
        return f"Listing {name[5:]}"
    if tool_name.startswith("get_"):
        return f"Looking up {name[4:]}"
    return f"Running {name}"
