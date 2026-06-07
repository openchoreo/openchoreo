# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the auth-bound MCP tool cache."""

from dataclasses import dataclass

from src.auth.bearer import BearerTokenAuth
from src.clients import mcp as mcp_module


@dataclass
class _FakeTool:
    name: str


class _FakeMCPClient:
    calls: list[str] = []

    def __init__(self, auth):
        self._auth = auth

    async def get_tools(self):
        token = self._auth._token
        type(self).calls.append(token)
        return [_FakeTool(name=f"tool-for-{token}")]


async def test_same_user_and_token_hits_cache(monkeypatch):
    await mcp_module.invalidate_tools_cache()
    _FakeMCPClient.calls.clear()
    monkeypatch.setattr(mcp_module, "MCPClient", _FakeMCPClient)

    first = await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-1"))
    second = await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-1"))

    assert [t.name for t in first] == ["tool-for-tok-1"]
    assert second is first
    assert _FakeMCPClient.calls == ["tok-1"]


async def test_token_refresh_bypasses_old_cached_tools(monkeypatch):
    await mcp_module.invalidate_tools_cache()
    _FakeMCPClient.calls.clear()
    monkeypatch.setattr(mcp_module, "MCPClient", _FakeMCPClient)

    first = await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-1"))
    second = await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-2"))

    assert [t.name for t in first] == ["tool-for-tok-1"]
    assert [t.name for t in second] == ["tool-for-tok-2"]
    assert _FakeMCPClient.calls == ["tok-1", "tok-2"]


async def test_invalidate_tools_cache_clears_all_tokens_for_user(monkeypatch):
    await mcp_module.invalidate_tools_cache()
    _FakeMCPClient.calls.clear()
    monkeypatch.setattr(mcp_module, "MCPClient", _FakeMCPClient)

    await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-1"))
    await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-2"))

    await mcp_module.invalidate_tools_cache("user-a")

    await mcp_module.get_tools_for_user("user-a", BearerTokenAuth("tok-1"))

    assert _FakeMCPClient.calls == ["tok-1", "tok-2", "tok-1"]
