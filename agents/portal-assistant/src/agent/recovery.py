# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Last-resort fallback path for the chat loop.

The main agent loop in the orchestrator runs the user's request through
langgraph with the read-only MCP tool catalog and the middleware stack.
When that loop hits ``recursion_limit`` (the model thrashes between
read tools without converging — e.g. retries the same query with
slightly different windows over and over), we recover by running a
*fresh* model call with **no tools**. With the loop gone, the model
can only emit plain text, which we surface as the user's reply.

The function is split out from the orchestrator so the recovery logic
can be tested and adjusted independently of the streaming pipeline. It
has no side effects beyond invoking the LLM.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any, Literal

from src.clients import get_model

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class RecoveryResult:
    """Outcome of ``recover_with_fallback``.

    ``source`` distinguishes:
      - ``"model"`` — we reached the fallback LLM and have its reply.
      - ``"stub"`` — the LLM call itself failed; the user is being
        served the canned apology. Ops dashboards should treat this as
        a systemic-failure signal, not a recovery success.
    """

    text: str
    source: Literal["model", "stub"]


# Cap context fed to the recovery model. Long chats can overshoot the
# context window of cheaper fallback-tier models; logging the trim
# explains a "you didn't tell me X" reply when the user actually did,
# 13 turns ago.
_RECOVERY_TAIL = 12

_FALLBACK_SYSTEM_PROMPT = (
    "You are Perch, a read-only OpenChoreo assistant. The previous turn "
    "ran out of tool-call budget before reaching a final answer — most "
    "likely the investigation looped on observability queries that kept "
    "returning empty results, or kept fanning out lookups without "
    "converging. Read the conversation below and write ONE concise "
    "reply for the user that:\n"
    "  • states what you were investigating in their words,\n"
    "  • summarises anything concrete you found before the budget ran "
    "out (run names, error strings, status fields, timestamps),\n"
    "  • names one specific thing the user can check or share to "
    "narrow the next attempt (a component name, a time window, "
    "whether logging is enabled on the component).\n"
    "Do NOT apologise more than once. Do NOT promise to retry. Output "
    "plain text only, no JSON, no tool calls."
)

_FALLBACK_STATIC_REPLY = (
    "I ran out of tool-call budget before reaching a clear answer. If "
    "you can narrow what to look at — a specific component, a time "
    "window, or the exact error you saw — I can try again with a "
    "tighter scope."
)


async def recover_with_fallback(
    *,
    messages: list[dict[str, Any]],
    scope: dict[str, Any] | None,
) -> RecoveryResult:
    """Run the recovery prompt against a tools-less LLM and return text.

    Always returns a non-empty ``RecoveryResult.text``: either the
    model's answer (``source="model"``) or a static apology
    (``source="stub"``) if the LLM call itself fails. Never raises.

    The structured return lets the caller tag the wire event so ops
    dashboards can distinguish a recovered chat from a stubbed-out one
    (the latter is a real outage signal, not just a slow turn).
    """
    try:
        model = get_model()
        system = _FALLBACK_SYSTEM_PROMPT
        if scope:
            system += "\n\nUser scope: " + ", ".join(
                f"{k}={v}" for k, v in scope.items() if v
            )

        history: list[dict[str, str]] = [{"role": "system", "content": system}]
        recent = messages[-_RECOVERY_TAIL:]
        if len(messages) > _RECOVERY_TAIL:
            logger.info(
                "Recovery fallback truncated history: %d → %d messages",
                len(messages),
                _RECOVERY_TAIL,
            )
        skipped = 0
        for m in recent:
            role = m.get("role")
            content = m.get("content", "")
            if role in {"user", "assistant"} and isinstance(content, str):
                history.append({"role": role, "content": content})
            else:
                skipped += 1
        if skipped:
            logger.info(
                "Recovery fallback skipped %d malformed message(s) "
                "(non-user/assistant role or non-string content)",
                skipped,
            )

        response = await model.ainvoke(history)
        text = getattr(response, "content", "") or ""
        if isinstance(text, list):
            text = "".join(
                part.get("text", "") if isinstance(part, dict) else str(part)
                for part in text
            )
        text = text.strip()
        if text:
            return RecoveryResult(text=text, source="model")
        # Empty model reply — treat as a stub so dashboards flag it.
        logger.warning("Recovery fallback returned empty content; using stub reply")
    except Exception as fallback_err:  # pragma: no cover — last-ditch
        # Keep exc_info so the systemic class (auth failure, API down,
        # token exhaustion) shows up in logs instead of a bare message.
        logger.error(
            "Recovery fallback failed: %s",
            fallback_err.__class__.__name__,
            exc_info=True,
        )

    return RecoveryResult(text=_FALLBACK_STATIC_REPLY, source="stub")
