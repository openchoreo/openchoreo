# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Top-level streaming entry point.

Owns:
- ``stream_chat`` — runs one chat turn end-to-end and yields NDJSON
  StreamEvents.
- the streaming-loop helpers (``_yield_chunks``,
  ``_recover_from_structured_response``, ``_handle_stream_error``).
- the per-pod concurrency semaphore.

A check authoring chat's turn lives in ``check_authoring.py``
(``stream_chat`` calls it once), on these same streaming helpers.

The agent build + middleware assembly live in ``builder.py``. The
agent is read-only: there is no action proposal / approval / execute
flow. ``WriteGuardMiddleware`` hard-refuses any mutating tool that
slips into the catalog so the agent cannot mutate state even if the
LLM tries.
"""

from __future__ import annotations

import asyncio
import logging
import re
import uuid
from collections.abc import AsyncIterator
from typing import Any

from langchain.agents.structured_output import StructuredOutputValidationError
from langchain_core.runnables import Runnable
from pydantic import ValidationError

from src.agent import check_authoring
from src.agent.a2ui import (
    dry_run_result_event,
)
from src.agent.builder import _build_agent
from src.agent.dry_run_view import DryRun, entity_name, read_dry_run
from src.agent.problem_view import ToolError
from src.agent.recovery import recover_with_fallback
from src.agent.stream_events import emit as _emit_event
from src.agent.stream_parser import ChatResponseParser
from src.agent.user_action import (
    for_model,
)
from src.clients.backstage_tools import (
    DRY_RUN_CHECK,
    GET_CHECK,
    GET_ENTITY_FACTS,
    LIST_CHECKS,
    LIST_FACT_SCHEMAS,
    VALIDATE_CHECK,
)
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


async def stream_chat(
    *,
    messages: list[dict[str, Any]],
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
    user's behalf for this request (see ``_build_agent``). A check
    authoring chat's turn is check_authoring.stream_turn.
    """
    request_id_context.set(f"msg_{uuid.uuid4().hex[:12]}")
    if (scope or {}).get("case_type") == "check_authoring":
        async for event in check_authoring.stream_turn(
            messages=messages,
            token=token,
            user_sub=user_sub,
            scope=scope,
            backstage_token=backstage_token,
        ):
            yield event
        return

    # Stamp the server clock onto the latest user turn ONCE and share it with
    # both the normal streaming path and the recovery fallback, so they anchor
    # on the same "now" (see _stamp_current_time).
    # The model gets each message's words only, not the button it came from.
    stamped_messages = _stamp_current_time(for_model(messages))

    async with _get_semaphore():
        try:
            agent, _tools, _tools_by_name = await _build_agent(
                token,
                scope,
                user_sub=user_sub,
                backstage_token=backstage_token,
            )
            parser = ChatResponseParser()

            async for ev in _yield_chunks(agent, stamped_messages, parser, scope=scope):
                yield ev

            # Final delta flush — when the model emits the structured
            # response as a single text block (some tool-using turns), the
            # parser's ``pop_delta`` inside ``_yield_chunks`` already
            # fired; this trailing call is idempotent and returns ""
            # in that case. Kept as a safety net so a never-fired-mid-
            # stream model still gets one ``message_chunk`` before
            # ``done`` lands.
            trailing = parser.pop_delta()
            if trailing:
                yield _emit_event(
                    {"type": "message_chunk", "content": trailing},
                )

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

            # Post-stream recovery: ProviderStrategy is expected to fill
            # ``parser.message`` from streamed text (or, on rare fallback,
            # from ChatResponse tool-call args we also fed in above). If
            # both paths produced nothing, the model finished without an
            # answer — emit an explicit ``error`` event rather than a
            # ``done`` with empty ``message``. The drawer drops empty
            # done.message silently, which would leave the user staring
            # at a cleared streaming buffer with no signal at all.
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

            done_payload: dict[str, Any] = {
                "type": "done",
                "message": parser.message,
            }
            if fix_prompt:
                done_payload["fix_prompt"] = fix_prompt
            yield _emit_event(done_payload)

        except Exception as e:
            async for err_event in _handle_stream_error(e, stamped_messages, scope):
                yield err_event


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

    Each dry-run-check result is added to ``dry_runs``, with the component
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
    # dry-run-check result can be read with the component and rule it ran.
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
                if dry_runs is not None and getattr(chunk, "name", None) == DRY_RUN_CHECK:
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
                if tool_name != DRY_RUN_CHECK or call_id in named_tests:
                    continue
                named = _ENTITY_REF_ARG_RE.search(call_args.get(call_id, ""))
                if named:
                    named_tests.add(call_id)
                    yield _emit_event({
                        "type": "tool_call",
                        "tool": DRY_RUN_CHECK,
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
    VALIDATE_CHECK: "Checking the rule",
    DRY_RUN_CHECK: "Testing the check",
    GET_ENTITY_FACTS: "Reading the component's facts",
    LIST_FACT_SCHEMAS: "Reading the facts you can use",
    LIST_CHECKS: "Looking at existing checks",
    GET_CHECK: "Looking at existing checks",
}
# Between a tool's result and the model's next step.
_THINKING_STEP = {"type": "tool_call", "tool": "think", "activeForm": "Thinking"}
# The component in a dry-run-check call's streamed arguments, once complete.
_ENTITY_REF_ARG_RE = re.compile(r'"entityRef"\s*:\s*"([^"]+)"')


def _testing_label(entity_ref: str | None) -> str:
    """The test step, naming the component when it is known:
    "Testing the check on order-api"."""
    if not entity_ref:
        return _active_form(DRY_RUN_CHECK)
    name, _kind = entity_name(entity_ref)
    return f"{_active_form(DRY_RUN_CHECK)} on {name}"
_WRITING_LABEL = "Writing the reply"
# A new check's reply that drafts the rule; shown only when the turn checked
# a rule (_RULE_TOOLS), so a question never says it drafts a check.
_DRAFTING_LABEL = "Drafting the check"
_RULE_TOOLS = frozenset({VALIDATE_CHECK, DRY_RUN_CHECK})
# The last step of a held-back reply, by what the request is, so the drawer
# shows the step the user asked for (see _writing_label).
_WRITING_LABELS = {
    "propose": "Working out the change",
    "apply": "Writing the change",
    "test": "Writing up the test",
    "discuss": "Writing the answer",
    "welcome": "Getting ready",
}


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
