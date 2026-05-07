# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import uuid
from collections.abc import AsyncIterator
from typing import Any

from langchain.agents import create_agent
from langchain.agents.middleware import SummarizationMiddleware
from langchain.agents.structured_output import (
    StructuredOutputValidationError,
    ToolStrategy,
)
from langchain_core.runnables import Runnable, RunnableConfig

from src.agent.action_store import get_action_store
from src.agent.middleware import (
    EmptyResultGuardMiddleware,
    LoggingMiddleware,
    LoopGuardMiddleware,
    ToolErrorHandlerMiddleware,
    WriteGuardMiddleware,
)
from src.agent.recovery import recover_with_fallback
from src.agent.stream_events import emit as _emit_event
from src.agent.stream_parser import ChatResponseParser
from src.agent.tool_registry import is_mutating
from src.auth.bearer import BearerTokenAuth
from src.clients import get_model, get_tools_for_user
from src.config import settings
from src.logging_config import request_id_context
from src.models import ChatResponse
from src.template_manager import render

logger = logging.getLogger(__name__)


# Module-level semaphore caps concurrent chats per pod. Chat is much spikier
# than rca-agent's analysis (which caps at 5), so a higher default.
_semaphore: asyncio.Semaphore | None = None


# Per-case recursion-limit cap. langgraph's ``recursion_limit`` counts graph
# supersteps — each model call + each tool node visit ticks the counter.
# A single-tool turn ≈ 3 supersteps; a 3-tool turn ≈ 7. Capping per-case
# guarantees worst-case per-turn latency: even if the model loops on a
# tool, the graph hard-stops at the cap and emits a final answer.
#
# Default 30 (the langgraph default we used everywhere before per-case caps)
# is preserved for unknown / missing case_type so the generic FAB and any
# future case keep working without code change.
_DEFAULT_RECURSION_LIMIT = 30
# langgraph counts every node visit, not every model call. With 5
# middleware nodes (Logging / LoopGuard / EmptyResultGuard / WriteGuard /
# ToolErrorHandler) + agent node + tool node, one tool cycle is roughly
# 4-6 supersteps. So the per-case caps below translate to:
#   build_failure (20) ≈ 2-3 tool cycles + final reply
#   logs_debug (14)    ≈ 1-2 tool cycles + final reply
# (EmptyResultGuard short-circuits the empty-data loop separately, so
#  this cap is the safety net for non-empty-but-unproductive iteration.)
_RECURSION_LIMIT_FOR_CASE: dict[str, int] = {
    "build_failure": 20,
    "logs_debug": 14,
}


# Per-case tool allowlists. The MCP catalog ships ~100 tools across the
# openchoreo + observability + rca servers; passing all of them in every
# turn's context bloats the LLM input by 10k+ tokens AND tempts smaller
# models to loop on unrelated read tools. Per-case filtering keeps the
# agent narrowly focused: the build-failure flow doesn't see component
# create tools, the create flow doesn't see log queries, etc.
#
# Conventions:
# - Set is the EXACT tool name (no glob). Names of tools that don't exist
#   in the current MCP catalog are silently ignored at filter time.
# - Add a tool here only if a prompt block actually instructs the agent
#   to call it.
# - Unknown / missing case_type → no filtering (full catalog), so the
#   generic FAB and any future case keep working without code change.
_TOOLS_FOR_CASE: dict[str, set[str]] = {
    "build_failure": {
        # Run enumeration — used when the user opens Perch on the
        # Build tab without a specific failed run pinned ("show me the
        # recent runs and tell me which to investigate"). For per-run
        # investigations, scope.runName is set and the agent skips the
        # list call.
        "list_workflow_runs",
        # Phase A — config inspection
        "get_workflow_run",
        "get_workflow",
        "get_cluster_workflow",
        # Phase B — log inspection (try in order: live pod logs → events →
        # OpenSearch fallback). Live logs + events come from the build plane
        # directly and work even when OpenSearch hasn't indexed anything yet.
        "get_workflow_run_logs",
        "get_workflow_run_events",
        "query_workflow_logs",
    },
    "logs_debug": {
        # Tier 1 — direct observer queries
        "query_component_logs",
        "query_resource_metrics",
        "query_http_metrics",
        "query_alerts",
        "query_incidents",
        # Tier 2 — rca escalation
        "list_rca_reports",
        "get_rca_report",
        "analyze_runtime_state",
    },
}


def _filter_tools_for_case(tools: list[Any], case_type: str | None) -> list[Any]:
    """Restrict the LLM's tool catalog to those listed for the case.

    No-op when ``case_type`` is missing or unknown — preserves the generic
    FAB (and future cases) without requiring a code change first.
    """
    if not case_type:
        return tools
    allowed = _TOOLS_FOR_CASE.get(case_type)
    if allowed is None:
        return tools
    return [t for t in tools if t.name in allowed]


def _required_fields_for(tool: Any) -> list[str]:
    """Best-effort extraction of required field names from a langchain tool's
    args schema, used to render a per-tool required-fields cheat sheet in the
    system prompt. Returns an empty list when the schema can't be introspected.
    """
    schema = getattr(tool, "args_schema", None)
    if schema is None:
        return []
    model_fields = getattr(schema, "model_fields", None)
    if isinstance(model_fields, dict):
        return sorted(
            name
            for name, field in model_fields.items()
            if getattr(field, "is_required", lambda: False)()
        )
    if isinstance(schema, dict):
        req = schema.get("required") or []
        return sorted(r for r in req if isinstance(r, str))
    return []


# Phrases that indicate the agent is still missing information (vs. just
# narrating a proposal). When one of these appears in the message AND the
# model has emitted a proposal anyway, that proposal was likely built from
# guessed values — drop it and let the user reply.
#
# Plain "please confirm" / "would you like to proceed" are intentionally
# absent: the action card itself is the confirmation, so the model adding
# "please confirm" alongside a proposal is fine — we don't need to strip it.
_INFO_GATHERING_PHRASES: tuple[str, ...] = (
    "please provide",
    "what name",
    "which name",
    "what data plane",
    "which data plane",
    "what namespace",
    "which namespace",
    "what is the",
    "could you provide",
    "can you provide",
    "could you tell me",
    "can you tell me",
    "let me know the",
    "i need the following",
    "i still need",
    "what would you like to name",
)


def _message_is_asking(message: str) -> bool:
    """Heuristic: does ``message`` ask the user for *missing information* the
    model still needs before it can build a proposal?

    Used to suppress action proposals the model emits *while* fishing for more
    fields — the proposal would carry guessed values. We only treat info-
    gathering wording as a stripping signal; bare confirmation phrasing
    ("please confirm", "ready to proceed?") is fine because the action card
    already carries Approve / Dismiss.
    """
    if not message:
        return False
    lowered = message.lower()
    return any(phrase in lowered for phrase in _INFO_GATHERING_PHRASES)


def _get_semaphore() -> asyncio.Semaphore:
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_chats)
    return _semaphore


async def _build_agent(
    token: str,
    scope: dict[str, Any] | None,
    *,
    user_sub: str = "",
    last_user_message: str = "",
    user_corpus: str = "",
    is_follow_up_reply: bool = False,
) -> tuple[Runnable, list[Any]]:
    """Construct a LangChain agent bound to all openchoreo MCP tools the user is
    authorized for. Returns (runnable, tools) — tools are returned so the caller
    can look up is_mutating by name when assembling proposed actions.

    ``user_sub`` keys an in-process tool-schema cache (5-min TTL). On a warm
    cache, this skips ~6-9 s of list_tools round-trips against the 3 MCP
    servers. Pass an empty string to bypass the cache (e.g. lifespan probe).

    ``last_user_message`` gates write tool calls when the current turn is a
    read. ``user_corpus`` is every user message in the chat concatenated, used
    to verify that values the agent fills into write-tool args were actually
    mentioned by the user instead of invented. ``is_follow_up_reply`` is True
    when the prior assistant turn asked the user for info — so a short reply
    like "name: uat" is treated as a continuation of an earlier write intent
    instead of a fresh read turn.
    """
    all_tools = await get_tools_for_user(user_sub, BearerTokenAuth(token))
    case_type = (scope or {}).get("case_type")
    tools = _filter_tools_for_case(all_tools, case_type)
    if case_type and len(tools) != len(all_tools):
        logger.info(
            "Filtered tools for case_type=%s: %d → %d tools",
            case_type, len(all_tools), len(tools),
        )
    else:
        logger.debug("MCP returned %d tools (user_sub=%s)", len(tools), user_sub or "<none>")
    tools_by_name = {t.name: t for t in tools}

    middleware: list[Any] = [
        LoggingMiddleware(),
        LoopGuardMiddleware(),
        # EmptyResultGuard short-circuits repeated empty observability
        # queries (the dominant cost on logs_debug turns when the target
        # has no recent activity). Sits before WriteGuard / ToolError so
        # the synthetic refusal lands in the model's next prompt unmodified.
        EmptyResultGuardMiddleware(),
        # WriteGuard runs before ToolErrorHandler so blocked writes surface as
        # errors the model can recover from, instead of being swallowed.
        WriteGuardMiddleware(
            tools_by_name=tools_by_name,
            last_user_message=last_user_message,
            user_corpus=user_corpus,
            is_follow_up_reply=is_follow_up_reply,
        ),
        ToolErrorHandlerMiddleware(),
    ]
    model = get_model()
    middleware.append(SummarizationMiddleware(model=model, trigger=("fraction", 0.8)))

    # Annotate tool list for the prompt template — read tools that the agent
    # may call freely vs. write tools that MUST be invoked with dry_run=true
    # and proposed via an actions event.
    write_tool_names = sorted(t.name for t in tools if is_mutating(t))
    read_tool_names = sorted(t.name for t in tools if not is_mutating(t))

    # Per-tool required-field map for write tools, so the system prompt can
    # show the model exactly which fields it must collect from the user. Without
    # this the model tends to ask for "(optional)" fields too — even after
    # the prompt forbids it — because it has to infer required-ness from each
    # tool's schema.
    write_tool_required: dict[str, list[str]] = {}
    for tool in tools:
        if not is_mutating(tool):
            continue
        write_tool_required[tool.name] = _required_fields_for(tool)

    # Detect whether the rca-agent MCP server contributed any tools so the
    # logs_debug prompt block can branch on it without hardcoding tool names.
    rca_tools_available = any(
        name in tools_by_name
        for name in ("analyze_runtime_state", "list_rca_reports", "get_rca_report")
    )

    template_context: dict[str, Any] = {
        "read_tools": read_tool_names,
        "write_tools": write_tool_names,
        "write_tool_required": write_tool_required,
        "rca_tools_available": rca_tools_available,
    }
    if scope:
        template_context["scope"] = scope

    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=render("prompts/perch_prompt.j2", template_context),
        middleware=middleware,
        response_format=ToolStrategy(ChatResponse),
    )

    # 100 was the original budget. Lowered to 30: if the model can't reach a
    # ChatResponse in 30 model↔tool hops it's almost certainly looping — the
    # WriteGuard / LoopGuard refusals make the right next step obvious. The
    # outer error handler in stream_chat translates the recursion exception
    # into a friendly user-facing message.
    recursion_limit = _RECURSION_LIMIT_FOR_CASE.get(
        case_type or "", _DEFAULT_RECURSION_LIMIT
    )
    runnable_config: RunnableConfig = {"recursion_limit": recursion_limit}
    return agent.with_config(runnable_config), tools


async def stream_chat(
    *,
    messages: list[dict[str, str]],
    token: str,
    user_sub: str,
    scope: dict[str, Any] | None = None,
) -> AsyncIterator[str]:
    """Run one chat turn and yield NDJSON StreamEvents.

    Event types match rca-agent's ChatPanelSection contract verbatim:
    tool_call, message_chunk, actions, done, error.

    Mutating tool calls are intercepted at the proposal stage: the agent emits
    an `actions` event carrying an action_id; nothing is executed until the
    caller POSTs /execute with that action_id.
    """
    request_id_context.set(f"msg_{uuid.uuid4().hex[:12]}")

    # Typed wire-format events live in src.agent.stream_events. The local
    # ``emit`` is a thin alias kept so existing call sites in this
    # function don't all churn at once — every dict that flows through
    # is now validated against the StreamEvent discriminated union, so a
    # typo in a key name fails locally instead of corrupting the stream.
    emit = _emit_event

    semaphore = _get_semaphore()

    last_user_message = next(
        (m.get("content", "") for m in reversed(messages) if m.get("role") == "user"),
        "",
    )
    user_corpus_parts = [
        m.get("content", "") for m in messages if m.get("role") == "user"
    ]
    # Scope values (namespace / project / component / environment derived from
    # the user's current Backstage URL) count as user-supplied — they're the
    # implicit context the user is operating under, even when not retyped.
    if scope:
        user_corpus_parts.extend(
            str(v) for v in scope.values() if isinstance(v, str) and v
        )
    user_corpus = "\n".join(user_corpus_parts)
    # If the immediately preceding assistant turn asked the user a question,
    # treat the current user message as a follow-up reply rather than a fresh
    # read intent — short replies like "name: uat" are continuations of an
    # unresolved write request.
    last_assistant_message = next(
        (
            m.get("content", "")
            for m in reversed(messages[:-1])
            if m.get("role") == "assistant"
        ),
        "",
    )
    is_follow_up_reply = _message_is_asking(last_assistant_message)

    async with semaphore:
        try:
            agent, tools = await _build_agent(
                token,
                scope,
                user_sub=user_sub,
                last_user_message=last_user_message,
                user_corpus=user_corpus,
                is_follow_up_reply=is_follow_up_reply,
            )
            tools_by_name = {t.name: t for t in tools}
            store = get_action_store()
            parser = ChatResponseParser()
            # Track whether the active tool_call_chunk stream belongs to
            # ChatResponse. Set when a tool_call_chunk arrives with name set;
            # subsequent chunks (without name) inherit the flag.
            in_chat_response_args = False

            # Track active ChatResponse tool calls per stream index. Subsequent
            # tool_call_chunks (which only carry args) inherit the routing.
            chat_response_indices: set[int] = set()
            structured_response: Any = None

            try:
                async for mode, payload in agent.astream(
                    {"messages": list(messages)},
                    stream_mode=["messages", "values"],
                ):
                    if mode == "values":
                        sr = payload.get("structured_response") if isinstance(payload, dict) else None
                        if sr is not None:
                            structured_response = sr
                        continue

                    chunk, _ = payload

                    for block in getattr(chunk, "content_blocks", None) or []:
                        block_type = block.get("type")
                        if block_type == "tool_call_chunk":
                            tool_name = block.get("name")
                            if tool_name:
                                yield emit(
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
                                delta = parser.push(text)
                                if delta:
                                    yield emit({"type": "message_chunk", "content": delta})

                    # Streaming args arrive on chunk.tool_call_chunks — feed any
                    # ChatResponse args into the parser so message_chunk events fire
                    # incrementally instead of waiting for the final state.
                    for tc in getattr(chunk, "tool_call_chunks", []) or []:
                        idx = tc.get("index")
                        name = tc.get("name")
                        args_str = tc.get("args") or ""
                        if name == "ChatResponse" and idx is not None:
                            chat_response_indices.add(idx)
                        elif name and idx is not None:
                            chat_response_indices.discard(idx)
                        if idx in chat_response_indices and args_str:
                            delta = parser.push(args_str)
                            if delta:
                                yield emit({"type": "message_chunk", "content": delta})
            except StructuredOutputValidationError:
                logger.warning("Structured output validation failed; using streamed content")

            # If the message wasn't recovered from streamed tool args, fall back to
            # the parsed structured_response that landed on agent state.
            if not parser.message and structured_response is not None:
                msg = getattr(structured_response, "message", None)
                if msg is None and isinstance(structured_response, dict):
                    msg = structured_response.get("message")
                actions = getattr(structured_response, "actions", None)
                if actions is None and isinstance(structured_response, dict):
                    actions = structured_response.get("actions")
                if msg:
                    parser._message = msg
                    yield emit({"type": "message_chunk", "content": msg})
                if actions:
                    parser._actions = list(actions)

            # Drop proposed actions if the agent is still asking the user for
            # confirmation in the message body. The Approve / Dismiss buttons
            # next to the action card already serve as confirmation — surfacing
            # both creates a "do you want to proceed?" question that's redundant
            # AND, worse, the model often emits the proposal with default values
            # it picked itself before the user agreed (e.g. inferring a data
            # plane, namespace, or display name). Forcing a real reply
            # disambiguates intent before anything reaches the action store.
            if parser.actions and _message_is_asking(parser.message):
                logger.info(
                    "Dropping %d proposed action(s): message contains a "
                    "confirmation question, so the user has not yet agreed",
                    len(parser.actions),
                )
                parser._actions = []

            # Promote any LLM-emitted "actions" entries into action_store entries
            # so /execute can find them. The agent's actions output schema is
            # free-form (List[Any]) — we treat each entry as {tool, args, summary, ...}
            # and reject anything that doesn't conform.
            promoted = await _promote_actions(
                parser.actions, store=store, tools_by_name=tools_by_name,
                user_sub=user_sub,
            )
            if promoted:
                yield emit({"type": "actions", "actions": promoted})

            yield emit({"type": "done", "message": parser.message})

        except Exception as e:
            logger.error("Chat stream error: %s", e, exc_info=True)
            err_str = str(e)
            # Recursion-limit blow-ups happen when the model loops on a refused
            # tool. Recover by running a tool-less fallback that's only allowed
            # to produce ChatResponse text — the langgraph loop is gone, so the
            # model can write a clean follow-up question for the user.
            if "Recursion limit" in err_str or "GRAPH_RECURSION_LIMIT" in err_str:
                try:
                    fallback = await recover_with_fallback(
                        messages=list(messages), scope=scope,
                    )
                except Exception as fallback_err:
                    logger.error(
                        "recover_with_fallback failed: %s", fallback_err, exc_info=True
                    )
                    yield emit(
                        {
                            "type": "error",
                            "message": (
                                f"An internal error occurred (request_id: {request_id_context.get()})."
                            ),
                        }
                    )
                    yield emit({"type": "done", "message": ""})
                else:
                    yield emit({"type": "message_chunk", "content": fallback})
                    yield emit({"type": "done", "message": fallback})
            else:
                yield emit(
                    {
                        "type": "error",
                        "message": (
                            f"An internal error occurred (request_id: {request_id_context.get()})."
                        ),
                    }
                )



async def _promote_actions(
    raw_actions: list[Any],
    *,
    store: Any,
    tools_by_name: dict[str, Any],
    user_sub: str,
) -> list[dict[str, Any]]:
    """Validate, store, and decorate each LLM-proposed action.

    Drops any malformed entry. Returns the list to surface to the client.
    """
    out: list[dict[str, Any]] = []
    for raw in raw_actions or []:
        if not isinstance(raw, dict):
            logger.warning("dropping malformed action (not a dict): %r", raw)
            continue
        tool = raw.get("tool")
        args = raw.get("args") or {}
        summary = raw.get("summary") or ""
        if not isinstance(tool, str) or tool not in tools_by_name:
            logger.warning("dropping action for unknown tool: %r", tool)
            continue
        if not is_mutating(tools_by_name[tool]):
            logger.warning("dropping proposed action for non-mutating tool: %s", tool)
            continue
        if not isinstance(args, dict):
            logger.warning("dropping action with non-dict args for tool %s", tool)
            continue

        action_id = await store.put(
            tool=tool,
            args=args,
            user_sub=user_sub,
            summary=summary,
            current_state=raw.get("currentState"),
            proposed_state=raw.get("proposedState"),
        )
        out.append(
            {
                "action_id": action_id,
                "tool": tool,
                "args": args,
                "summary": summary,
                "mutating": True,
                "currentState": raw.get("currentState"),
                "proposedState": raw.get("proposedState"),
            }
        )
    return out


async def execute_action(*, action_id: str, user_sub: str, token: str) -> dict[str, Any]:
    """Execute a previously-proposed action.

    Returns {"success": bool, "result": ..., "error": ...}. Caller maps to HTTP.
    """
    store = get_action_store()
    item = await store.take(action_id=action_id, user_sub=user_sub)
    if item is None:
        return {
            "success": False,
            "error": "action not found, expired, or owned by another user",
        }

    # Re-resolve the tool against the cached tool list (still bound to the
    # user's JWT). On a warm cache this is free; on a miss we pay the same
    # one-time fetch as a chat turn would.
    tools = await get_tools_for_user(user_sub, BearerTokenAuth(token))
    tool = next((t for t in tools if t.name == item.tool), None)
    if tool is None:
        return {
            "success": False,
            "error": f"tool {item.tool!r} no longer available to this user",
        }
    if not is_mutating(tool):
        logger.warning("refusing to execute non-mutating tool via approval flow: %s", item.tool)
        return {
            "success": False,
            "error": "tool is not a mutating operation",
        }

    # Strip dry_run if present — execute is the real thing.
    args = {k: v for k, v in item.args.items() if k != "dry_run"}
    try:
        result = await tool.ainvoke(args)
        return {"success": True, "result": result}
    except Exception as e:
        logger.error("execute %s failed: %s", item.tool, e, exc_info=True)
        return {"success": False, "error": "action execution failed"}


def _active_form(tool_name: str) -> str:
    """Best-effort human-readable progress label rendered while a tool runs."""
    name = tool_name.replace("_", " ")
    if tool_name.startswith("list_"):
        return f"Listing {name[5:]}"
    if tool_name.startswith("get_"):
        return f"Looking up {name[4:]}"
    if tool_name.startswith("create_"):
        return f"Previewing creation of {name[7:]}"
    if tool_name.startswith("update_") or tool_name.startswith("patch_"):
        return f"Previewing update of {name.split(' ', 1)[1]}"
    if tool_name.startswith("delete_"):
        return f"Previewing deletion of {name[7:]}"
    return f"Running {name}"
