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

            structured_response: Any = None
            async for event in _yield_chunks(agent, messages, parser):
                if event[0] == "structured":
                    structured_response = event[1]
                else:
                    yield event[1]

            # If the message wasn't recovered from streamed tool args,
            # fall back to the parsed structured_response that landed
            # on agent state.
            async for recovery_event in _recover_from_structured_response(
                parser, structured_response,
            ):
                yield recovery_event

            # Single message_chunk flush right before done. The
            # streaming loop deliberately accumulated text into the
            # parser without yielding mid-turn message_chunks (see the
            # _yield_chunks docstring) — emit the assembled final
            # message here so the drawer paints it in one go instead
            # of flashing the model's early placeholder.
            if parser.message:
                yield _emit_event(
                    {"type": "message_chunk", "content": parser.message},
                )

            yield _emit_event({"type": "done", "message": parser.message})

        except Exception as e:
            async for err_event in _handle_stream_error(e, messages, scope):
                yield err_event


# ── Helpers used by stream_chat ────────────────────────────────────


async def _yield_chunks(
    agent: Runnable,
    messages: list[dict[str, str]],
    parser: ChatResponseParser,
) -> AsyncIterator[tuple[str, Any]]:
    """Stream LangChain agent chunks and emit per-chunk NDJSON events.

    Yields one of two tuple shapes:
        ("event",      <ndjson string>)        — to forward to the client
        ("structured", <structured_response>)  — final state snapshot

    Wrapping every yield in a tuple keeps the streaming function's
    contract one-call/one-event, while still letting the caller pick up
    the agent's terminal ``structured_response`` value (which arrives
    on the ``values`` stream mode, not on a single chunk).

    **No mid-stream message_chunk emission.** We feed every text /
    ChatResponse-args block into the parser so the final message
    accumulates internally, but we do NOT yield ``message_chunk``
    events while the agent is still iterating. The model has a habit
    of emitting a parallel placeholder ChatResponse alongside the
    first tool call ("Workflow has completed with failure" before the
    real diagnosis is composed); streaming that placeholder would
    flash it on screen before the real answer arrives. The final
    parsed message is emitted as a single ``message_chunk`` right
    before ``done`` in the orchestrator. The user still sees
    ``tool_call`` pills in real time so the drawer doesn't go silent.
    """
    # Track active ChatResponse tool calls per stream index. Subsequent
    # tool_call_chunks (which only carry args) inherit the routing.
    chat_response_indices: set[int] = set()

    try:
        async for mode, payload in agent.astream(
            {"messages": list(messages)},
            stream_mode=["messages", "values"],
        ):
            if mode == "values":
                sr = payload.get("structured_response") if isinstance(payload, dict) else None
                if sr is not None:
                    yield ("structured", sr)
                continue

            chunk, _ = payload

            for block in getattr(chunk, "content_blocks", None) or []:
                block_type = block.get("type")
                if block_type == "tool_call_chunk":
                    tool_name = block.get("name")
                    if tool_name:
                        yield (
                            "event",
                            _emit_event(
                                {
                                    "type": "tool_call",
                                    "tool": tool_name,
                                    "activeForm": _active_form(tool_name),
                                    "args": block.get("args", ""),
                                }
                            ),
                        )
                elif block_type == "text":
                    text = block.get("text", "")
                    if text:
                        # Accumulate into the parser; do not stream.
                        parser.push(text)

            # Feed ChatResponse args into the parser so the final
            # message lands in parser.message — but never stream them
            # mid-turn. See the function docstring for why.
            for tc in getattr(chunk, "tool_call_chunks", []) or []:
                idx = tc.get("index")
                name = tc.get("name")
                args_str = tc.get("args") or ""
                if name == "ChatResponse" and idx is not None:
                    chat_response_indices.add(idx)
                elif name and idx is not None:
                    chat_response_indices.discard(idx)
                if idx in chat_response_indices and args_str:
                    parser.push(args_str)
    except StructuredOutputValidationError:
        logger.warning(
            "Structured output validation failed; using streamed content",
        )


async def _recover_from_structured_response(
    parser: ChatResponseParser,
    structured_response: Any,
) -> AsyncIterator[str]:
    """Pull the final message off the agent's final-state snapshot.

    Two cases this handles:

    1. The streaming pass surfaced no message at all (rare — usually
       when the LLM emits a terminal ChatResponse without prose).
       Adopt the final message verbatim.
    2. The streaming pass already surfaced a *placeholder* message
       (e.g. a parallel ChatResponse the model emitted alongside a
       tool call before reading the tool result), and the agent's
       final state has the real, longer diagnosis. Replace the
       placeholder with the final answer.

    The "longer is better" heuristic is intentionally simple: if the
    final structured_response message is meaningfully longer (≥2× the
    streamed message AND at least 80 chars longer) it's the model's
    completed thought, not a status echo.
    """
    if structured_response is None:
        return
    final_msg = getattr(structured_response, "message", None)
    if final_msg is None and isinstance(structured_response, dict):
        final_msg = structured_response.get("message")

    streamed_msg = parser.message or ""
    if final_msg and isinstance(final_msg, str):
        should_replace = not streamed_msg or (
            len(final_msg) >= 2 * len(streamed_msg)
            and len(final_msg) - len(streamed_msg) >= 80
        )
        if should_replace:
            parser.set_message(final_msg)
    # ``yield`` keeps the function an async generator so callers can
    # still ``async for`` over it; we have nothing to surface here.
    return
    yield  # pragma: no cover — unreachable, makes this an async gen


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
