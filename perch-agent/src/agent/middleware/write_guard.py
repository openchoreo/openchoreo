# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
import re
from collections.abc import Awaitable, Callable

from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.agent.tool_registry import is_mutating

logger = logging.getLogger(__name__)


def _user_supplied_value(value: str, corpus_lower: str) -> bool:
    """Return True when ``value`` appears in the user corpus as a discrete
    token, not just any substring.

    The previous implementation used a plain ``substring in corpus`` check
    which had two false-positive modes:
      - The literal field name appearing in the user's message ("the
        component's name…") would mark a value of ``"name"`` as supplied.
      - A short value like ``"prod"`` would match against "the production
        database" because ``"prod"`` is a substring of "production".

    Word-boundary matching against the lowercased corpus closes both —
    the value is required to appear as a standalone token (or at least
    bounded by a non-alphanumeric on each side). We escape the value
    because it may contain regex metacharacters (e.g. ``.`` in a domain).
    """
    if not value:
        return False
    pattern = re.compile(rf"(?<![A-Za-z0-9_]){re.escape(value)}(?![A-Za-z0-9_])")
    return bool(pattern.search(corpus_lower))


class WriteGuardMiddleware(AgentMiddleware):
    """Block every mutating tool call from the agent loop.

    The system prompt instructs the agent to dry-run every write tool first and
    surface the result via the structured ``actions`` field so the user can
    confirm before anything runs for real. Two problems make a prompt-only
    contract unsafe:

    1. Smaller models (e.g. gpt-4o-mini) routinely ignore the directive and
       call write tools directly.
    2. Most openchoreo MCP write tools (e.g. ``create_environment``) do not
       accept a ``dry_run`` argument — the Go handler just executes regardless,
       so passing ``dry_run: true`` would not have prevented a real write.

    This guard intercepts every mutating call before it leaves the agent and
    returns a synthetic preview ToolMessage echoing the args. The model is then
    instructed to place the proposal into ``ChatResponse.actions``; the actual
    write happens later via ``/execute`` once the user confirms in the UI.
    """

    # Verbs / phrases that signal the user is asking for a mutation in the
    # current turn. Conservative-on-purpose: missing a verb means the model
    # must ask the user to confirm intent before any write tool is approved.
    _WRITE_INTENT_TOKENS: tuple[str, ...] = (
        "create", "make", "add", "new",
        "update", "change", "modify", "edit", "set", "rename", "patch",
        "delete", "remove", "drop", "destroy", "uninstall",
        "apply", "deploy", "promote", "trigger", "run",
        "register", "provision", "scale",
    )

    # Required-arg fields whose values are inferred from system context rather
    # than supplied by the user. Excluded from the "user must mention this"
    # check below so the agent isn't forced to ask for things like the kind
    # discriminator.
    _CONTEXT_FIELDS: frozenset[str] = frozenset(
        {"data_plane_ref_kind", "namespace_name"},
    )

    def __init__(
        self,
        tools_by_name: dict[str, object],
        *,
        last_user_message: str = "",
        user_corpus: str = "",
        is_follow_up_reply: bool = False,
    ) -> None:
        super().__init__()
        self._tools_by_name = tools_by_name
        # The current user message is a write-intent turn if it contains a
        # write verb, OR if it's a continuation of an unresolved write request
        # (the prior assistant turn asked the user for info and the user just
        # supplied it — see ``is_follow_up_reply`` computed in agent.py).
        self._has_write_intent = (
            self._detect_write_intent(last_user_message)
            or (is_follow_up_reply and self._detect_write_intent(user_corpus))
        )
        self._user_corpus_lower = (user_corpus or last_user_message).lower()

    @classmethod
    def _detect_write_intent(cls, text: str) -> bool:
        if not text:
            return False
        lowered = text.lower()
        for tok in cls._WRITE_INTENT_TOKENS:
            pattern = rf"(?<![A-Za-z0-9_]){re.escape(tok.lower())}(?![A-Za-z0-9_])"
            if re.search(pattern, lowered):
                return True
        return False

    def _required_fields(self, tool: object) -> list[str]:
        schema = getattr(tool, "args_schema", None)
        if schema is None:
            return []
        # Pydantic v2 model
        model_fields = getattr(schema, "model_fields", None)
        if isinstance(model_fields, dict):
            return [
                name
                for name, field in model_fields.items()
                if getattr(field, "is_required", lambda: False)()
            ]
        # JSON-schema dict (langchain-mcp-adapters returns this for raw tools)
        if isinstance(schema, dict):
            req = schema.get("required") or []
            return [r for r in req if isinstance(r, str)]
        return []

    def _missing_user_supplied_fields(
        self, args: dict[str, object], required: list[str]
    ) -> list[str]:
        """Return required fields whose values weren't mentioned by the user.

        Only considers string args — booleans / numbers / nested objects are
        too noisy to keyword-match. Skips ``_CONTEXT_FIELDS`` like
        ``namespace_name`` since the UI provides that via scope.
        """
        missing: list[str] = []
        for field in required:
            if field in self._CONTEXT_FIELDS:
                continue
            value = args.get(field)
            if value is None:
                missing.append(field)
                continue
            if isinstance(value, str) and not value.strip():
                missing.append(field)
                continue
            if not isinstance(value, str):
                # Non-string present (int / bool / etc.) — accept as supplied.
                continue
            if not _user_supplied_value(value.lower(), self._user_corpus_lower):
                missing.append(field)
        return missing

    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        name = request.tool_call.get("name", "")
        args = request.tool_call.get("args") or {}

        tool = self._tools_by_name.get(name)
        if tool is None or not is_mutating(tool):
            return await handler(request)

        # Hard refusal #1: the user's current turn doesn't ask for a mutation.
        if not self._has_write_intent:
            logger.info(
                "WriteGuard: refusing '%s' — current user message has no write intent",
                name,
            )
            return ToolMessage(
                content=(
                    f"WriteGuard: tool '{name}' is mutating, but the current user "
                    "message does not request a mutation. Do NOT propose an action "
                    "and do NOT add anything to `actions`. Answer the user's "
                    "actual question with read tools only, then call ChatResponse."
                ),
                tool_call_id=request.tool_call.get("id", ""),
                name=name,
                status="error",
            )

        # Hard refusal #2: required string fields whose values the user never
        # mentioned. Smaller models invent plausible-sounding names ("testing",
        # "demo", "default-env") and slip them into the proposal. Refuse and
        # force the agent to ask the user instead.
        if isinstance(args, dict):
            required = self._required_fields(tool)
            missing = self._missing_user_supplied_fields(args, required)
            if missing:
                logger.info(
                    "WriteGuard: refusing '%s' — user did not supply required field(s) %s "
                    "(args=%s)",
                    name,
                    missing,
                    list(args.keys()),
                )
                friendly = ", ".join(f"`{m}`" for m in missing)
                return ToolMessage(
                    content=(
                        f"WriteGuard: tool '{name}' was called with required field(s) "
                        f"the user has NOT supplied: {friendly}. Do NOT invent values, "
                        "do NOT pick defaults, do NOT add anything to `actions`. "
                        "Call ChatResponse now and ask the user for the missing "
                        "information in plain language. Only retry the tool after the "
                        "user replies with the value(s)."
                    ),
                    tool_call_id=request.tool_call.get("id", ""),
                    name=name,
                    status="error",
                )

        try:
            args_preview = json.dumps(args, default=str)
        except Exception:
            args_preview = repr(args)

        logger.info(
            "WriteGuard: intercepting mutating tool '%s'; routing to actions proposal",
            name,
        )
        return ToolMessage(
            content=(
                f"WriteGuard: tool '{name}' is a mutating operation and will not "
                f"run from inside this agent loop. Treat the args you sent as the "
                f"preview. Now call ChatResponse and put exactly one entry in "
                f"`actions` with this shape: "
                f'{{"tool": "{name}", "args": {args_preview}, "summary": '
                f'"<one-line description for the user>"}}. The user will confirm '
                f"in the UI; the server will execute it after approval. Do not "
                f"call this tool again."
            ),
            tool_call_id=request.tool_call.get("id", ""),
            name=name,
            status="error",
        )
