# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import hashlib
import json
import logging
import time
from typing import Any, Literal

import httpx
from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.clients.backstage_tools import agent_name, strict_for_model
from src.config import settings
from src.logging_config import request_id_context

logger = logging.getLogger(__name__)


# Per-user-per-token MCP tool cache. Listing tools across the openchoreo +
# observer + rca MCP servers is the single biggest per-turn latency
# (≈6-9 s for ~100 tools). The returned BaseTool objects are bound to the
# auth object used when the client is constructed, so the cache key MUST
# include the bearer token identity — otherwise a token refresh (or a user
# whose permissions narrowed on refresh) would keep invoking tools through
# the stale credential. The TTL still bounds staleness for authz changes
# that happen *without* a token change. Pod restart evicts everything; that's fine.
_TOOLS_TTL_SECONDS = 300  # 5 min
_TOOLS_CACHE: dict[str, tuple[float, list[BaseTool]]] = {}
# Single-flight: when N concurrent turns hit a cache miss for the same
# (user, token) pair, coalesce them onto one in-flight fetch instead of
# fanning out N copies.
_TOOLS_FETCH_LOCKS: dict[str, asyncio.Lock] = {}
# Coordinates every read/write/sweep of the two dicts above so that
# expiry-aware lookups and lock allocation can't interleave with a
# sweep mid-await. Held only across cheap sync dict ops — NOT across
# the slow MCPClient.get_tools fetch (that runs under the per-key
# fetch lock instead, so concurrent users on different keys don't
# serialize on this one lock).
_CACHE_LOCK: asyncio.Lock = asyncio.Lock()


def _sweep_expired_cache(now: float) -> None:
    """Drop expired _TOOLS_CACHE entries.

    Caller must hold ``_CACHE_LOCK``. Intentionally does NOT prune
    ``_TOOLS_FETCH_LOCKS``: popping a lock between another coroutine's
    ``setdefault`` and its ``async with`` would let a subsequent caller
    install a *different* lock under the same key, breaking single-flight
    (both coroutines would then race to fetch). Locks are tiny; let them
    accumulate over a pod's lifetime. ``invalidate_tools_cache`` is the
    explicit clean-up path.
    """
    expired = [
        k for k, (ts, _) in _TOOLS_CACHE.items() if (now - ts) >= _TOOLS_TTL_SECONDS
    ]
    for k in expired:
        _TOOLS_CACHE.pop(k, None)


async def _inject_request_id(request: httpx.Request) -> None:
    """Stamp X-Request-Id on every outbound MCP HTTP call.

    Reads ``request_id_context`` at request-send time (not client-
    construction time) so a cached MCP client that's reused across
    turns picks up each turn's own request id. If the caller already
    supplied the header explicitly, leave it alone.
    """
    rid = request_id_context.get()
    if not rid:
        return
    if not any(k.lower() == "x-request-id" for k in request.headers.keys()):
        request.headers["X-Request-Id"] = rid


def _httpx_client_factory(
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        verify=not settings.uid_resolver_tls_insecure_skip_verify,
        event_hooks={"request": [_inject_request_id]},
    )


class MCPClient:
    """Wraps langchain_mcp_adapters' MultiServerMCPClient with the user's bearer token.

    Connects to the openchoreo control-plane MCP (CRUD over OpenChoreo resources)
    and the observer MCP (logs / metrics / traces) so the agent can answer both
    platform-shape and observability questions in a single chat session.

    Tool listing across the backing servers is fanned out with
    ``asyncio.gather`` — one ``MultiServerMCPClient`` per server. The
    library's built-in multi-server ``get_tools`` walks servers
    sequentially, so a single shared client serializes the (slow) cold
    list_tools roundtrip. Splitting into per-server clients lets the
    cold path run in ≈ max(server latency) instead of ≈ sum(server latency).

    Mutation classification (which tools require user confirmation) is performed
    *client-side* in src.agent.tool_registry — there is no server-side flag. See
    the proposal section "MCP changes: dryRun plumbing only" for the rationale.
    """

    def __init__(
        self,
        auth: httpx.Auth,
        *,
        backstage_auth: httpx.Auth | None = None,
        backstage: Literal["include", "exclude", "only"] = "include",
    ) -> None:
        # One MultiServerMCPClient per backing server. Each holds a config
        # with a single entry so ``get_tools`` on the instance hits exactly
        # one server — we drive parallelism at this layer instead.
        servers: dict[str, StreamableHttpConnection] = {
            "openchoreo": {
                "transport": "streamable_http",
                "url": settings.openchoreo_mcp_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            },
            "observability": {
                "transport": "streamable_http",
                "url": settings.observer_mcp_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            },
        }
        # Optional: register the rca-agent MCP server when an URL is
        # configured. Exposes list_rca_reports / get_rca_report /
        # analyze_runtime_state — used by the runtime_debug case as the
        # Tier 2 escalation path.
        if settings.rca_agent_mcp_url:
            servers["rca"] = {
                "transport": "streamable_http",
                "url": settings.rca_agent_mcp_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            }
        # Optional: register Backstage's mcp-actions server when an URL is
        # configured. Exposes the check-authoring tools used by the
        # check_authoring case. Backstage doesn't accept OpenChoreo tokens,
        # so it gets ``backstage_auth`` — a token Backstage issued on the
        # signed-in user's behalf. Without one there are no Backstage tools:
        # every call runs as the user.
        if settings.backstage_mcp_url and backstage_auth is not None:
            servers["backstage"] = {
                "transport": "streamable_http",
                "url": settings.backstage_mcp_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": backstage_auth,
            }
        if backstage == "exclude":
            servers.pop("backstage", None)
        elif backstage == "only":
            servers = {k: v for k, v in servers.items() if k == "backstage"}
        self._clients: dict[str, MultiServerMCPClient] = {
            name: MultiServerMCPClient({name: conn}) for name, conn in servers.items()
        }

    async def _get_tools_for_server(
        self, name: str, client: MultiServerMCPClient,
    ) -> tuple[str, list[BaseTool] | Exception]:
        started = time.monotonic()
        try:
            tools = await client.get_tools()
        except Exception as e:  # noqa: BLE001 — surface per-server, decide upstream
            elapsed = time.monotonic() - started
            logger.warning(
                "MCP get_tools failed for server=%s after %.2fs: %s",
                name, elapsed, e,
            )
            return name, e
        elapsed = time.monotonic() - started
        if name == "backstage":
            # tech-insights.list-checks → tech-insights__list-checks: model
            # tool names can't hold a dot. Calls still use the MCP name.
            for tool in tools:
                tool.name = agent_name(tool.name)
                strict_for_model(tool)
        logger.debug(
            "MCP get_tools succeeded for server=%s in %.2fs (%d tools)",
            name, elapsed, len(tools),
        )
        return name, tools

    async def get_tools(self) -> list[BaseTool]:
        results = await asyncio.gather(
            *(self._get_tools_for_server(name, c) for name, c in self._clients.items())
        )
        merged: list[BaseTool] = []
        failed: list[str] = []
        for name, result in results:
            if isinstance(result, BaseException):
                failed.append(name)
            else:
                merged.extend(result)
        if not merged and failed:
            # All servers failed — preserve the original raise-on-error
            # behaviour so a fully broken catalog surfaces hard at the
            # lifespan probe and at chat time, instead of letting the
            # agent run with no tools at all.
            raise RuntimeError(
                f"Failed to fetch tools from any MCP server (failed={failed})"
            )
        if failed:
            # Partial success: degrade rather than fail the turn. A missing
            # optional rca server, for example, should not break chat.
            logger.warning(
                "MCP get_tools partial success: failed=%s — proceeding with "
                "tools from healthy servers only",
                failed,
            )
        return merged


def _sub_fp(user_sub: str) -> str:
    """Irreversible truncated fingerprint of a user_sub for log correlation.

    Same sha256(...)[:16] pattern as the token fingerprint below — gives ops
    cross-line correlation without retaining the raw OIDC sub in logs.
    """
    if not user_sub:
        return "<none>"
    return hashlib.sha256(user_sub.encode()).hexdigest()[:16]


def _cache_key_for(user_sub: str, auth: httpx.Auth) -> str | None:
    """Return a stable cache key for auth-bound tool objects.

    The tool instances produced by MultiServerMCPClient capture the auth object
    they were created with, so reusing them across distinct bearer tokens is
    incorrect. Use a token fingerprint rather than the raw token to avoid
    retaining the secret in cache keys or logs.

    When the auth object doesn't expose a bearer token, bypass caching rather
    than risk binding the wrong credential to a later request.
    """
    if not user_sub:
        return None

    token = getattr(auth, "_token", None)
    if not isinstance(token, str):
        return None

    digest = hashlib.sha256(token.encode()).hexdigest()[:16]
    return f"{user_sub}:{digest}"


async def get_tools_for_user(
    user_sub: str,
    auth: httpx.Auth,
    backstage_auth: httpx.Auth | None = None,
) -> list[BaseTool]:
    """Return MCP tools for ``user_sub``, hitting an in-process cache.

    Cache hit for the same (user, bearer token) within TTL → returns
    immediately, skipping the 3 list_tools round-trips entirely. Cache miss
    → fetches once, fills the cache, and coalesces concurrent miss requests
    onto a single asyncio.Lock so we don't stampede the MCP servers on first turn.

    Pass an empty user_sub (or an auth object without an inspectable bearer)
    to bypass the cache — useful for the lifespan probe where no real user
    exists.

    ``backstage_auth`` is a token Backstage issued on the user's behalf for
    this one request. Tools bound to it can't be reused on a later turn, so
    the Backstage tools are fetched fresh with it while the other servers'
    tools still come from the cache. Without it, or if that fetch fails, the
    turn has no Backstage tools. If the other servers fail, the Backstage
    tools are still returned; only when both fail does this raise, as when
    every server fails.
    """
    if backstage_auth is None or not settings.backstage_mcp_url:
        return await _get_cached_tools(user_sub, auth, backstage="include")

    others, backstage_tools = await asyncio.gather(
        _get_other_tools(user_sub, auth),
        _get_user_backstage_tools(user_sub, auth, backstage_auth),
    )
    if others is None and backstage_tools is None:
        raise RuntimeError("Failed to fetch tools from any MCP server")
    return [*(others or []), *(backstage_tools or [])]


async def _get_other_tools(user_sub: str, auth: httpx.Auth) -> list[BaseTool] | None:
    """The cached tools of every server but Backstage; None if none can be listed."""
    try:
        return await _get_cached_tools(user_sub, auth, backstage="exclude")
    except Exception:  # noqa: BLE001 — the Backstage tools may still serve the turn
        logger.warning(
            "MCP tools of the other servers failed for sub_fp=%s",
            _sub_fp(user_sub),
            exc_info=True,
        )
        return None


async def _get_user_backstage_tools(
    user_sub: str, auth: httpx.Auth, backstage_auth: httpx.Auth,
) -> list[BaseTool] | None:
    """Backstage tools bound to the user's token; None if they can't be listed."""
    try:
        tools = await MCPClient(
            auth=auth, backstage_auth=backstage_auth, backstage="only",
        ).get_tools()
    except Exception:  # noqa: BLE001 — no Backstage tools this turn, don't fail it
        logger.warning(
            "Backstage MCP tools with the user's token failed for sub_fp=%s — "
            "no Backstage tools this turn",
            _sub_fp(user_sub),
            exc_info=True,
        )
        return None
    logger.info(
        "Backstage MCP tools bound to the user's token for sub_fp=%s (%d tools)",
        _sub_fp(user_sub),
        len(tools),
    )
    return tools


async def _get_cached_tools(
    user_sub: str,
    auth: httpx.Auth,
    *,
    backstage: Literal["include", "exclude"],
) -> list[BaseTool]:
    cache_key = _cache_key_for(user_sub, auth)
    if cache_key and backstage == "exclude":
        cache_key = f"{cache_key}:no-backstage"

    def new_client() -> MCPClient:
        # Keep the default call shape: the cache tests swap MCPClient out.
        if backstage == "include":
            return MCPClient(auth=auth)
        return MCPClient(auth=auth, backstage=backstage)

    if not cache_key:
        return await new_client().get_tools()

    # Fast path: sweep expired entries and check the cache under the
    # coordinating lock. If we miss, claim (or reuse) the per-key fetch
    # lock here too so a concurrent sweep can't orphan it between this
    # assignment and the ``async with fetch_lock`` below.
    async with _CACHE_LOCK:
        _sweep_expired_cache(time.monotonic())
        cached = _TOOLS_CACHE.get(cache_key)
        if cached and (time.monotonic() - cached[0]) < _TOOLS_TTL_SECONDS:
            return cached[1]
        fetch_lock = _TOOLS_FETCH_LOCKS.setdefault(cache_key, asyncio.Lock())

    async with fetch_lock:
        # Re-check inside the fetch lock so the second concurrent caller
        # uses the result the first just populated. The re-check itself
        # must coordinate with sweep/write, so re-acquire _CACHE_LOCK.
        async with _CACHE_LOCK:
            cached = _TOOLS_CACHE.get(cache_key)
            if cached and (time.monotonic() - cached[0]) < _TOOLS_TTL_SECONDS:
                return cached[1]

        started = time.monotonic()
        tools = await new_client().get_tools()
        elapsed = time.monotonic() - started

        async with _CACHE_LOCK:
            _TOOLS_CACHE[cache_key] = (time.monotonic(), tools)

        names = sorted(t.name for t in tools)
        sub_fp = _sub_fp(user_sub)
        logger.info(
            "MCP tools cache miss for sub_fp=%s token_fp=%s — fetched %d tools in %.2fs",
            sub_fp,
            _cache_key_for(user_sub, auth).rsplit(":", 1)[-1],
            len(tools),
            elapsed,
        )
        # Surfaces which exact tool names made it through the per-server
        # (langchain-mcp-adapters) and per-user (openchoreo PDP filter)
        # gates. If a tool that exists in the source isn't listed here,
        # the gap is server-side; if it IS listed here but the agent
        # doesn't see it, the gap is in the case-type filter.
        logger.debug("MCP tool names for sub_fp=%s: %s", sub_fp, names)
        return tools


class BackstageToolError(Exception):
    """A Backstage check tool refused a call; the message is Backstage's."""


async def backstage_tools_as_user(backstage_auth: httpx.Auth) -> dict[str, BaseTool]:
    """The Backstage check tools bound to the user's token, by name.

    For code that calls the tools itself — building and confirming the
    confirm card — rather than handing them to the model.
    """
    tools = await MCPClient(
        auth=backstage_auth, backstage_auth=backstage_auth, backstage="only",
    ).get_tools()
    return {tool.name: tool for tool in tools}


async def call_backstage_tool(
    tools: dict[str, BaseTool], name: str, args: dict[str, Any],
) -> dict[str, Any]:
    """Run one Backstage check tool and return its output object.

    Raises BackstageToolError with Backstage's message when the tool is
    missing or refuses the call (permission, validation, unknown id).
    """
    tool = tools.get(name)
    if tool is None:
        raise BackstageToolError(f"The {name} tool isn't available.")
    # Invoked as a tool call so the result is a ToolMessage: its status tells
    # an error from a result, and the artifact holds the structured output.
    result = await tool.ainvoke({"type": "tool_call", "id": name, "name": name, "args": args})
    content = result.content
    text = content if isinstance(content, str) else "".join(
        block.get("text", "") for block in content if isinstance(block, dict)
    )
    if getattr(result, "status", "success") == "error":
        raise BackstageToolError(text or f"{name} failed.")
    artifact = getattr(result, "artifact", None) or {}
    output = artifact.get("structured_content")
    if output is None:
        output = json.loads(text)
    return output


async def invalidate_tools_cache(user_sub: str | None = None) -> None:
    """Drop cached tools for one user (or all if ``user_sub`` is None).

    Useful when authz changes are known to have happened (admin role grant,
    role binding update, etc.) — the next turn re-fetches.

    Holds ``_CACHE_LOCK`` for the full mutation so the clear/pop operations
    on ``_TOOLS_CACHE`` and ``_TOOLS_FETCH_LOCKS`` are atomic with respect to
    the cache-miss path in ``get_tools_for_user`` (which does its sweep,
    lookup, and ``setdefault`` on ``_TOOLS_FETCH_LOCKS`` under the same lock).
    """
    async with _CACHE_LOCK:
        if user_sub is None:
            _TOOLS_CACHE.clear()
            _TOOLS_FETCH_LOCKS.clear()
            return
        prefix = f"{user_sub}:"
        for key in [k for k in _TOOLS_CACHE if k.startswith(prefix)]:
            _TOOLS_CACHE.pop(key, None)
        for key in [k for k in _TOOLS_FETCH_LOCKS if k.startswith(prefix)]:
            _TOOLS_FETCH_LOCKS.pop(key, None)
