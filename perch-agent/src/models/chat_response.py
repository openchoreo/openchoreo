# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from src.models.base import BaseModel


class ChatResponse(BaseModel):
    """Return your final answer to the user. Call this once you have everything you
    need (or have hit the per-tool cap from the system prompt) — do not keep calling
    other tools after this. Set ``message`` to the user-facing reply and ``actions``
    to any proposed write actions (each gathered from a dry-run preview)."""

    message: str
    actions: list[Any] = []
