# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

logger = logging.getLogger(__name__)

# Smaller models (notably gpt-4o-mini) routinely ignore "do not call the same tool
# more than N times" prompt directives and loop until langgraph's recursion limit
# kills the turn. This middleware short-circuits repeated identical calls with a
# synthetic ToolMessage that nudges the agent to call ChatResponse next, and once
# the abuse becomes obvious it refuses ALL further tool calls (including ones the
# model has never tried) so langgraph drops out the next turn instead of burning
# the recursion budget in 2-second model→tool ping-pongs.
_PER_KEY_LIMIT = 2
_TOTAL_LIMIT = 12
_HARD_STOP_LIMIT = 18


class LoopGuardMiddleware(AgentMiddleware):
    def __init__(self) -> None:
        super().__init__()
        self._counts: dict[str, int] = {}
        self._total = 0

    @staticmethod
    def _key(name: str, args: object) -> str:
        try:
            args_str = json.dumps(args, sort_keys=True, default=str)
        except Exception:
            args_str = repr(args)
        return f"{name}::{args_str}"

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        name = request.tool_call.get("name", "")
        args = request.tool_call.get("args", {})
        # ChatResponse is the structured-output terminator — never block it.
        if name == "ChatResponse":
            return await handler(request)

        key = self._key(name, args)
        self._counts[key] = self._counts.get(key, 0) + 1
        self._total += 1

        if self._total > _HARD_STOP_LIMIT:
            logger.warning(
                "LoopGuard: HARD-STOP — '%s' refused after %d total tool calls",
                name,
                self._total,
            )
            return ToolMessage(
                content=(
                    "LoopGuard: hard stop. You have called too many tools this "
                    "turn without producing a final ChatResponse. STOP calling "
                    "tools entirely. Your VERY NEXT action MUST be ChatResponse "
                    "with a short message explaining what you were trying to do "
                    "and what specific information you still need from the user."
                ),
                tool_call_id=request.tool_call.get("id", ""),
                name=name,
                status="error",
            )

        if self._counts[key] > _PER_KEY_LIMIT or self._total > _TOTAL_LIMIT:
            logger.warning(
                "LoopGuard: short-circuiting %s (per-key=%d, total=%d)",
                name,
                self._counts[key],
                self._total,
            )
            return ToolMessage(
                content=(
                    f"LoopGuard: '{name}' has already been called with these arguments. "
                    "Do not call any read tool again. Call `ChatResponse` now with a "
                    "summary of what you have so far."
                ),
                tool_call_id=request.tool_call.get("id", ""),
                name=name,
                status="error",
            )

        return await handler(request)
