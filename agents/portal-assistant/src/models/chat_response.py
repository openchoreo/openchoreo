# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.models.base import BaseModel


class ChatResponse(BaseModel):
    """Return your final answer to the user. Call once when you have enough;
    do not call other tools after. Set ``message`` to the user-facing reply.

    ``fix_prompt`` is an optional SECOND, paste-ready prompt the drawer
    surfaces behind a Copy button. Recipient depends on ``case_type``:
    ``build_failure`` → coding bot on the user's repo (no kubectl/MCP);
    ``runtime_debug`` → AI agent with OpenChoreo MCP + kubectl. The
    per-case-type sections of the system prompt carry the decision rules
    and the exact shape. Leave ``None`` whenever those rules say so
    (empty logs, not-found, no actionable handoff).
    """

    message: str
    fix_prompt: str | None = None
