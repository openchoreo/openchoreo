# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Compiles the LangChain agent for one chat turn.

Owns:
- per-case recursion-limit caps + tool allowlists,
- the cached system-prompt render (which also computes per-tool
  required-field maps and the rca-available flag),
- the agent assembly itself (model + tools + middleware + structured
  output).

Pulled out of the old ``agent.py`` so the streaming orchestrator can
import a single, well-scoped factory. No behaviour change vs. the
pre-split implementation — this is a file-move only.
"""

from __future__ import annotations

import json
import logging
from collections import OrderedDict
from typing import Any

from langchain.agents import create_agent
from langchain.agents.middleware import SummarizationMiddleware
from langchain.agents.structured_output import ToolStrategy
from langchain_core.runnables import Runnable, RunnableConfig

from src.agent.middleware import (
    EmptyResultGuardMiddleware,
    LoggingMiddleware,
    LoopGuardMiddleware,
    OutputTransformerMiddleware,
    ToolErrorHandlerMiddleware,
    WriteGuardMiddleware,
)
from src.agent.tool_registry import is_mutating
from src.auth.bearer import BearerTokenAuth
from src.clients import get_model, get_tools_for_user
from src.models import ChatResponse
from src.template_manager import render

logger = logging.getLogger(__name__)


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
# middleware nodes (Logging / LoopGuard / EmptyResultGuard /
# WriteGuard / ToolErrorHandler) + agent node + tool node, one tool
# cycle is roughly 4-6 supersteps. So the per-case caps below
# translate to:
#   build_failure (20)  ≈ 3-4 tool cycles + final reply
#   runtime_debug (20)  ≈ 3-4 tool cycles + final reply — bumped from
#                         the old logs_debug (14) to account for the
#                         cross-correlation pivot (logs → trace, or
#                         trace → logs) that the unified branch does
#                         per turn.
# (EmptyResultGuard short-circuits the empty-data loop separately, so
#  this cap is the safety net for non-empty-but-unproductive iteration.)
_RECURSION_LIMIT_FOR_CASE: dict[str, int] = {
    "build_failure": 20,
    "runtime_debug": 20,
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
    "runtime_debug": {
        # Tier 1 — direct observer queries (logs side)
        "query_component_logs",
        "query_resource_metrics",
        "query_http_metrics",
        "query_alerts",
        "query_incidents",
        # Tier 1 — direct observer queries (traces side). The unified
        # branch pivots between the two via trace_id correlation, so
        # both halves of the tool catalogue must be present.
        "query_traces",
        "query_trace_spans",
        "get_span_details",
        # Tier 2 — rca escalation
        "list_rca_reports",
        "get_rca_report",
        "analyze_runtime_state",
    },
}


def _filter_tools_for_case(tools: list[Any], case_type: str | None) -> list[Any]:
    """Restrict the LLM's tool catalog.

    Two layers:
      1. The agent is read-only — drop every mutating tool unconditionally.
         The catalog the LLM sees never contains a write tool, so it cannot
         even attempt to call one. ``WriteGuardMiddleware`` is the backstop
         in case a misclassification slips one through.
      2. Per-case allowlists narrow the catalog further so the prompt
         stays focused. No-op when ``case_type`` is missing or unknown.
    """
    tools = [t for t in tools if not is_mutating(t)]
    if not case_type:
        return tools
    allowed = _TOOLS_FOR_CASE.get(case_type)
    if allowed is None:
        return tools
    return [t for t in tools if t.name in allowed]


# Bounded LRU cache for the rendered system prompt. Keyed by
# (case_type, scope_fingerprint, tools_fingerprint). Rendering the
# perch_prompt.j2 template plus the ``_required_fields_for`` schema
# walk is ~20-50 ms per turn; on the same chat session (stable scope
# + tools) the second turn onwards hits this cache and pays ~0 ms.
#
# Safe to share across users because the value (a string) carries no
# auth: tool *names* and the user's *scope hints* are not secrets, and
# the per-user auth is enforced separately when the actual tools are
# invoked. The tools_fingerprint also varies per-user (because the MCP
# tools cache is per-user-per-token), so users with different tool sets
# naturally key into different cache entries.
_PROMPT_CACHE_MAX = 32
_prompt_cache: OrderedDict[tuple[str, str, tuple[str, ...]], str] = OrderedDict()


def _scope_fingerprint(scope: dict[str, Any] | None) -> str:
    if not scope:
        return ""
    # ``default=str`` so non-string leaves (list[str] for log_levels,
    # numbers, etc.) hash deterministically. sort_keys for stability.
    return json.dumps(scope, sort_keys=True, default=str)


def _tools_fingerprint(tools: list[Any]) -> tuple[str, ...]:
    return tuple(sorted(t.name for t in tools))


def _get_or_render_prompt(
    *,
    case_type: str | None,
    scope: dict[str, Any] | None,
    tools: list[Any],
    tools_by_name: dict[str, Any],
) -> str:
    key = (case_type or "", _scope_fingerprint(scope), _tools_fingerprint(tools))
    cached = _prompt_cache.get(key)
    if cached is not None:
        _prompt_cache.move_to_end(key)
        return cached

    # Catalog is read-only after _filter_tools_for_case, so every tool here
    # is a read tool.
    read_tool_names = sorted(t.name for t in tools)
    rca_tools_available = any(
        name in tools_by_name
        for name in ("analyze_runtime_state", "list_rca_reports", "get_rca_report")
    )

    template_context: dict[str, Any] = {
        "read_tools": read_tool_names,
        "rca_tools_available": rca_tools_available,
    }
    if scope:
        template_context["scope"] = scope

    rendered = render("prompts/perch_prompt.j2", template_context)
    _prompt_cache[key] = rendered
    while len(_prompt_cache) > _PROMPT_CACHE_MAX:
        _prompt_cache.popitem(last=False)
    return rendered


async def _build_agent(
    token: str,
    scope: dict[str, Any] | None,
    *,
    user_sub: str = "",
) -> tuple[Runnable, list[Any], dict[str, Any]]:
    """Construct a LangChain agent bound to all openchoreo MCP tools the user is
    authorized for. Returns (runnable, tools, tools_by_name) — both
    ``tools`` and the ``tools_by_name`` index are returned so the caller
    can look up is_mutating by name without rebuilding the dict.

    ``user_sub`` keys an in-process tool-schema cache (5-min TTL). On a warm
    cache, this skips ~6-9 s of list_tools round-trips against the 3 MCP
    servers. Pass an empty string to bypass the cache (e.g. lifespan probe).
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
        # queries (the dominant cost on runtime_debug turns when the
        # target has no recent activity). Sits before WriteGuard /
        # ToolError so the synthetic refusal lands in the model's next
        # prompt unmodified.
        EmptyResultGuardMiddleware(),
        # OutputTransformer compresses observability tool responses
        # (logs/traces/spans) into compact markdown tables before the
        # LLM ever sees them — benchmarked at ~60% token reduction
        # across the three shapes. Sits AFTER EmptyResultGuard so empty
        # short-circuits don't pay the render cost, and BEFORE
        # WriteGuard so refusals land on raw input unmodified.
        OutputTransformerMiddleware(),
        # WriteGuard runs before ToolErrorHandler so blocked writes surface as
        # errors the model can recover from, instead of being swallowed.
        WriteGuardMiddleware(tools_by_name=tools_by_name),
        ToolErrorHandlerMiddleware(),
    ]
    model = get_model()
    middleware.append(SummarizationMiddleware(model=model, trigger=("fraction", 0.8)))

    # Render (or look up) the system prompt and the derived tool
    # annotations as one cached unit. Tool names + scope + case_type
    # fully determine the rendered string; on the same chat session the
    # second turn onwards hits the cache and skips both the Jinja2
    # render and the ``_required_fields_for`` schema walk.
    system_prompt = _get_or_render_prompt(
        case_type=case_type,
        scope=scope,
        tools=tools,
        tools_by_name=tools_by_name,
    )

    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=system_prompt,
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
    return agent.with_config(runnable_config), tools, tools_by_name
