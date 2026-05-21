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
import logging
import uuid
from collections.abc import AsyncIterator
from typing import Any

from langchain.agents.structured_output import StructuredOutputValidationError
from langchain_core.runnables import Runnable

from src.agent.builder import _build_agent
from src.agent.recovery import recover_with_fallback
from src.agent.stream_events import emit as _emit_event
from src.agent.stream_parser import ChatResponseParser
from src.config import settings
from src.logging_config import request_id_context

logger = logging.getLogger(__name__)


# Module-level semaphore caps concurrent chats per pod. Chat is much spikier
# than rca-agent's analysis (which caps at 5), so a higher default.
_semaphore: asyncio.Semaphore | None = None


def _get_semaphore() -> asyncio.Semaphore:
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_chats)
    return _semaphore


async def stream_chat(
    *,
    messages: list[dict[str, str]],
    token: str,
    user_sub: str,
    scope: dict[str, Any] | None = None,
) -> AsyncIterator[str]:
    """Run one chat turn and yield NDJSON StreamEvents.

    Event types: tool_call, message_chunk, done, error.

    The agent is read-only — mutating tools are filtered out of the
    catalog and any that slip through are hard-refused by
    ``WriteGuardMiddleware``.
    """
    request_id_context.set(f"msg_{uuid.uuid4().hex[:12]}")

    async with _get_semaphore():
        try:
            agent, _tools, _tools_by_name = await _build_agent(
                token,
                scope,
                user_sub=user_sub,
            )
            parser = ChatResponseParser()

            async for ev in _yield_chunks(agent, messages, parser):
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

            done_payload: dict[str, Any] = {
                "type": "done",
                "message": parser.message,
            }
            if fix_prompt:
                done_payload["fix_prompt"] = fix_prompt
            yield _emit_event(done_payload)

        except Exception as e:
            async for err_event in _handle_stream_error(e, messages, scope):
                yield err_event


# ── Helpers used by stream_chat ────────────────────────────────────


async def _yield_chunks(
    agent: Runnable,
    messages: list[dict[str, str]],
    parser: ChatResponseParser,
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
    drawer paints text as it arrives.
    """

    def _flush_delta() -> str | None:
        delta = parser.pop_delta()
        if not delta:
            return None
        return _emit_event({"type": "message_chunk", "content": delta})

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
                continue

            for block in getattr(chunk, "content_blocks", None) or []:
                block_type = block.get("type")
                if block_type == "tool_call_chunk":
                    tool_name = block.get("name")
                    if tool_name:
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

            # One flush per outer chunk — keeps the event rate proportional
            # to the model's token rate without firing on every micro-update.
            delta_event = _flush_delta()
            if delta_event is not None:
                yield delta_event
    except StructuredOutputValidationError:
        logger.warning(
            "Structured output validation failed; using streamed content",
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


def _active_form(tool_name: str) -> str:
    """Best-effort human-readable progress label rendered while a tool runs."""
    name = tool_name.replace("_", " ")
    if tool_name.startswith("list_"):
        return f"Listing {name[5:]}"
    if tool_name.startswith("get_"):
        return f"Looking up {name[4:]}"
    return f"Running {name}"
