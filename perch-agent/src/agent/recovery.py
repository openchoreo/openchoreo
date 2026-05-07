# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Last-resort fallback path for the chat loop.

The main agent loop in ``agent.py`` runs the user's request through
langgraph with the full tool catalog and middleware stack. When that
loop hits ``recursion_limit`` (typically because the model loops on a
refused write tool or thrashes between tool calls without converging),
we recover by running a *fresh* model call with **no tools**. With the
loop gone, the model can only emit plain text, which we surface as the
user's reply.

The function is split out from ``agent.py`` so the recovery logic can
be tested and adjusted independently of the streaming pipeline. It has
no side effects beyond invoking the LLM.
"""

from __future__ import annotations

import logging
from typing import Any

from src.clients import get_model

logger = logging.getLogger(__name__)


# Cap context fed to the recovery model. Long chats can overshoot the
# context window of cheaper fallback-tier models; logging the trim
# explains a "you didn't tell me X" reply when the user actually did,
# 13 turns ago.
_RECOVERY_TAIL = 12

_FALLBACK_SYSTEM_PROMPT = (
    "You are Perch, an OpenChoreo assistant. The previous turn timed out "
    "before producing a response, most likely because it was missing a "
    "required field for a write action. Read the conversation below "
    "and write ONE concise reply for the user that:\n"
    "  • acknowledges what they have already supplied,\n"
    "  • lists the specific fields you still need (by name), and\n"
    "  • does NOT propose any actions — just ask.\n"
    "Output plain text only, no JSON, no tool calls, no apologies."
)

_FALLBACK_STATIC_REPLY = (
    "I got stuck before I could give you a clean answer. If you were "
    "creating something, please reply with the missing details (name, "
    "data plane, etc.) explicitly — for example: `name: uat, data "
    "plane: default, not production`."
)


async def recover_with_fallback(
    *,
    messages: list[dict[str, Any]],
    scope: dict[str, Any] | None,
) -> str:
    """Run the recovery prompt against a tools-less LLM and return text.

    Always returns a non-empty string: either the model's answer or a
    static apology if the LLM call itself fails. Never raises.
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
            return text
    except Exception as fallback_err:  # pragma: no cover — last-ditch
        logger.warning("Recovery fallback failed: %s", fallback_err)

    return _FALLBACK_STATIC_REPLY
