# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.models.base import BaseModel


class ChatResponse(BaseModel):
    """Return your final answer to the user. Call this once you have everything
    you need (or have hit the per-tool cap from the system prompt) — do not keep
    calling other tools after this. Set ``message`` to the user-facing reply.

    ``fix_prompt`` is a SECOND, paste-ready prompt for an external coding bot
    (CodeRabbit / Cursor / Claude Code). Populate it ONLY for ``build_failure``
    turns where the diagnosis names a concrete, code-level root cause the user
    could fix in their repo (Dockerfile error, missing dep, bad import, wrong
    env var, etc.). Leave it ``None`` for everything else — infra issues
    (OOMKilled tuning, registry secret, scheduler), runs not found, empty
    logs, and any non-build_failure case. The chat drawer renders a copy
    button next to the assistant message iff this field is set; the user
    pastes the value verbatim into their bot, so it must be self-contained
    (framing line, verbatim error excerpt, context bullets, ask).
    """

    message: str
    fix_prompt: str | None = None
