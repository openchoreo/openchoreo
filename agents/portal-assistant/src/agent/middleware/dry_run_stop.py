# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
import re
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware, ModelRequest, ModelResponse
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.clients.backstage_tools import DRY_RUN_CHECK, VALIDATE_CHECK

logger = logging.getLogger(__name__)

# dry-run-check statuses that are a final answer. "invalid" is not here: the
# tool raises an error for it so the model can fix the rule and retry.
_RESULT_STATUSES = frozenset({"passed", "failed", "not-applicable"})

# Matches the prompt's "at most 3 dry-run-check calls"; 6 tool calls is
# more than a successful check_authoring turn needs.
_MAX_DRY_RUNS = 3
_MAX_TOOL_CALLS = 6
# Matches the prompt's "after 3 attempts" for validate-check.
_MAX_VALIDATIONS = 3

# A validate-check error that a retry can't fix.
_CANNOT_VALIDATE_RE = re.compile(
    r"permission|signed-in user|not allowed|not ready|isn't available",
    re.IGNORECASE,
)


def _text(message: object) -> str:
    content = getattr(message, "content", "")
    if isinstance(content, list):
        return " ".join(b.get("text", "") if isinstance(b, dict) else str(b) for b in content)
    return str(content)


def _payload(result: ToolMessage | Command) -> dict | None:
    """The JSON object a successful tool result carries, if any."""
    if not isinstance(result, ToolMessage) or result.status == "error":
        return None
    try:
        payload = json.loads(_text(result))
    except TypeError, ValueError:
        return None
    return payload if isinstance(payload, dict) else None


def _dry_run_status(result: ToolMessage | Command) -> str | None:
    """The `status` of a successful dry-run-check result, if any."""
    status = (_payload(result) or {}).get("status")
    # A list or object can't be looked up in the set: not a result either.
    return status if isinstance(status, str) and status in _RESULT_STATUSES else None


def _tool_name(tool: object) -> str:
    return tool if isinstance(tool, str) else getattr(tool, "name", "")


class DryRunStopMiddleware(AgentMiddleware):
    """Ends the check_authoring tool loop once a dry-run has a result.

    Smaller models (notably gpt-4o-mini) keep calling tools after
    dry-run-check has answered — re-running it, rewriting the rule until it
    passes, or listing checks — until LoopGuard and then the recursion limit
    end the turn with a fallback reply, despite prompt and tool-result
    instructions to stop. Once a dry-run returns passed / failed /
    not-applicable, every later model call this turn gets no tools, so the
    model's only move is the final ChatResponse.

    The same happens after _MAX_DRY_RUNS dry-run attempts or _MAX_TOOL_CALLS
    tool calls without a result — e.g. when no component was named and the
    model guesses entity refs that don't exist — so the turn still ends with
    a drafted check instead of the recursion-limit fallback.

    validate-check gets the same treatment. Once it says the rule is valid,
    the model keeps only dry-run-check, for a test the user asked for, and
    otherwise must reply. It is taken away after _MAX_VALIDATIONS attempts, or at once
    when it can't validate at all (no permission, unavailable), so the
    model shows its last rule with the problems instead of looping.

    Without ``tools_allowed`` (an edit message the user hasn't approved —
    see orchestrator._edit_phase), no model call gets tools, including the
    orchestrator's rewrite of the reply: the model can only answer with the
    Step 1 confirmation, which the prompt alone didn't always hold it to.
    With ``dry_run_only`` (a message that only asks for a test) the model
    gets dry-run-check and nothing else: it can test, not change or save.
    """

    def __init__(self, *, tools_allowed: bool = True, dry_run_only: bool = False) -> None:
        super().__init__()
        self._tools_allowed = tools_allowed
        self._dry_run_only = dry_run_only
        self._tools_off = False
        self._dry_runs = 0
        self._tool_calls = 0
        self._validations = 0
        self._validated = False
        self._validate_off = False

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        result = await handler(request)
        self._tool_calls += 1
        if request.tool_call.get("name") == VALIDATE_CHECK:
            self._after_validation(result)
        if request.tool_call.get("name") == DRY_RUN_CHECK:
            self._dry_runs += 1
            status = _dry_run_status(result)
            if status:
                self._stop(f"dry-run returned {status}")
        if self._dry_runs >= _MAX_DRY_RUNS:
            self._stop(f"{self._dry_runs} dry-run attempts")
        elif self._tool_calls >= _MAX_TOOL_CALLS:
            self._stop(f"{self._tool_calls} tool calls")
        return result

    def _after_validation(self, result: ToolMessage | Command) -> None:
        self._validations += 1
        payload = _payload(result)
        if payload is not None and payload.get("valid") is True:
            logger.info("DryRunStop: rule valid after %d validation(s)", self._validations)
            self._validated = True
        elif payload is None and _CANNOT_VALIDATE_RE.search(_text(result)):
            logger.info("DryRunStop: validate-check unavailable — no more validation this turn")
            self._validate_off = True
        elif self._validations >= _MAX_VALIDATIONS:
            logger.info("DryRunStop: rule still invalid after %d validations", self._validations)
            self._validate_off = True

    def _stop(self, reason: str) -> None:
        if not self._tools_off:
            logger.info("DryRunStop: %s — tools off for this turn", reason)
            self._tools_off = True

    async def awrap_model_call(
        self,
        request: ModelRequest,
        handler: Callable[[ModelRequest], Awaitable[ModelResponse]],
    ) -> ModelResponse:
        if not self._tools_allowed and request.tools:
            logger.info("DryRunStop: edit not approved — tools off for this message")
            request = request.override(tools=[])
        if self._tools_off and request.tools:
            request = request.override(tools=[])
        if request.tools and (self._validated or self._dry_run_only):
            # Valid (or a test-only message): reply now — or dry-run it, when
            # the user asked for a test.
            request = request.override(
                tools=[t for t in request.tools if _tool_name(t) == DRY_RUN_CHECK],
            )
        elif request.tools and self._validate_off:
            request = request.override(
                tools=[t for t in request.tools if _tool_name(t) != VALIDATE_CHECK],
            )
        if not request.tools:
            return await handler(request)

        # ProviderStrategy binds tools with strict=True, and OpenAI's strict
        # mode doesn't hold for parallel tool calls: gpt-4o-mini then emits
        # truncated arguments that the SDK's json.loads crashes the turn on.
        # Only set while tools are bound — OpenAI rejects the flag otherwise.
        settings = dict(request.model_settings)
        with_tools = request.override(
            model_settings={**settings, "parallel_tool_calls": False},
        )
        try:
            return await handler(with_tools)
        except json.JSONDecodeError as exc:
            # Safety net if the arguments are still malformed: retry once
            # with no tools so the model's only move is the final answer.
            logger.warning(
                "DryRunStop: model sent malformed tool arguments (%s) — "
                "retrying once without tools",
                exc,
            )
            self._stop("malformed tool arguments")
            return await handler(request.override(tools=[], model_settings=settings))
