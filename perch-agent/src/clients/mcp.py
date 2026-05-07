# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import hashlib
import logging
import time

import httpx
from langchain_core.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient, StreamableHttpConnection

from src.config import settings

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


def _sweep_expired_cache(now: float) -> None:
    """Drop expired _TOOLS_CACHE entries and their orphaned fetch locks.

    Synchronous and lock-free: safe under single-threaded asyncio because we
    never ``await`` between reads and writes. Skips locks that are currently
    held so we don't yank a lock out from under an in-flight refresh.
    """
    expired = [
        k for k, (ts, _) in _TOOLS_CACHE.items() if (now - ts) >= _TOOLS_TTL_SECONDS
    ]
    for k in expired:
        _TOOLS_CACHE.pop(k, None)
        lock = _TOOLS_FETCH_LOCKS.get(k)
        if lock is not None and not lock.locked():
            _TOOLS_FETCH_LOCKS.pop(k, None)


def _httpx_client_factory(
    headers: dict[str, str] | None = None,
    timeout: httpx.Timeout | None = None,
    auth: httpx.Auth | None = None,
) -> httpx.AsyncClient:
    return httpx.AsyncClient(
        headers=headers,
        timeout=timeout,
        auth=auth,
        verify=not settings.tls_insecure_skip_verify,
    )


class MCPClient:
    """Wraps langchain_mcp_adapters' MultiServerMCPClient with the user's bearer token.

    Connects to the openchoreo control-plane MCP (CRUD over OpenChoreo resources)
    and the observer MCP (logs / metrics / traces) so the agent can answer both
    platform-shape and observability questions in a single chat session.

    Mutation classification (which tools require user confirmation) is performed
    *client-side* in src.agent.tool_registry — there is no server-side flag. See
    the proposal section "MCP changes: dryRun plumbing only" for the rationale.
    """

    def __init__(self, auth: httpx.Auth) -> None:
        oc_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.openchoreo_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }
        obs_connection: StreamableHttpConnection = {
            "transport": "streamable_http",
            "url": settings.observer_mcp_url,
            "httpx_client_factory": _httpx_client_factory,
            "auth": auth,
        }
        servers: dict[str, StreamableHttpConnection] = {
            "openchoreo": oc_connection,
            "observability": obs_connection,
        }
        # Optional: register the rca-agent MCP server when an URL is
        # configured. Exposes list_rca_reports / get_rca_report /
        # analyze_runtime_state — used by the logs_debug case as the
        # Tier 2 escalation path.
        if settings.rca_agent_mcp_url:
            servers["rca"] = {
                "transport": "streamable_http",
                "url": settings.rca_agent_mcp_url,
                "httpx_client_factory": _httpx_client_factory,
                "auth": auth,
            }
        self._client = MultiServerMCPClient(servers)

    async def get_tools(self) -> list[BaseTool]:
        try:
            tools = await self._client.get_tools()
        except Exception as e:
            logger.error("Failed to fetch tools from MCP client: %s", e, exc_info=True)
            raise RuntimeError(f"Failed to fetch tools from MCP client: {e}") from e
        return tools


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


async def get_tools_for_user(user_sub: str, auth: httpx.Auth) -> list[BaseTool]:
    """Return MCP tools for ``user_sub``, hitting an in-process cache.

    Cache hit for the same (user, bearer token) within TTL → returns
    immediately, skipping the 3 list_tools round-trips entirely. Cache miss
    → fetches once, fills the cache, and coalesces concurrent miss requests
    onto a single asyncio.Lock so we don't stampede the MCP servers on first turn.

    Pass an empty user_sub (or an auth object without an inspectable bearer)
    to bypass the cache — useful for the lifespan probe where no real user
    exists.
    """
    cache_key = _cache_key_for(user_sub, auth)
    now = time.monotonic()
    _sweep_expired_cache(now)
    if cache_key:
        cached = _TOOLS_CACHE.get(cache_key)
        if cached and (now - cached[0]) < _TOOLS_TTL_SECONDS:
            return cached[1]

    if not cache_key:
        return await MCPClient(auth=auth).get_tools()

    lock = _TOOLS_FETCH_LOCKS.setdefault(cache_key, asyncio.Lock())
    async with lock:
        # Re-check inside the lock so that the second concurrent caller
        # uses the result the first just populated.
        cached = _TOOLS_CACHE.get(cache_key)
        now = time.monotonic()
        if cached and (now - cached[0]) < _TOOLS_TTL_SECONDS:
            return cached[1]
        started = time.monotonic()
        tools = await MCPClient(auth=auth).get_tools()
        elapsed = time.monotonic() - started
        _TOOLS_CACHE[cache_key] = (time.monotonic(), tools)
        names = sorted(t.name for t in tools)
        logger.info(
            "MCP tools cache miss for user_sub=%s token_fp=%s — fetched %d tools in %.2fs",
            user_sub,
            cache_key.rsplit(":", 1)[-1],
            len(tools),
            elapsed,
        )
        # One-shot debug: surfaces which exact tool names made it through
        # the per-server (langchain-mcp-adapters) and per-user (openchoreo
        # PDP filter) gates. If a tool that exists in the source isn't
        # listed here, the gap is server-side; if it IS listed here but
        # the agent doesn't see it, the gap is in the case-type filter.
        logger.info("MCP tool names for user_sub=%s: %s", user_sub, names)
        return tools


def invalidate_tools_cache(user_sub: str | None = None) -> None:
    """Drop cached tools for one user (or all if ``user_sub`` is None).

    Useful when authz changes are known to have happened (admin role grant,
    role binding update, etc.) — the next turn re-fetches.
    """
    if user_sub is None:
        _TOOLS_CACHE.clear()
        _TOOLS_FETCH_LOCKS.clear()
        return
    prefix = f"{user_sub}:"
    for key in [k for k in _TOOLS_CACHE if k.startswith(prefix)]:
        _TOOLS_CACHE.pop(key, None)
    for key in [k for k in _TOOLS_FETCH_LOCKS if k.startswith(prefix)]:
        _TOOLS_FETCH_LOCKS.pop(key, None)
