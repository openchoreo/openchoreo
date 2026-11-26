# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the auth-bound MCP tool cache."""

from dataclasses import dataclass

import pytest

from common.auth.bearer import BearerTokenAuth
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


# ── Backstage tools bound to a token issued for the user ───────────────


class _FakeSplitMCPClient:
    """Records each fetch as (backstage mode, token the tools are bound to)."""

    calls: list[tuple[str, str]] = []
    fail_backstage = False
    fail_others = False

    def __init__(self, auth, *, backstage_auth=None, backstage="include"):
        self._auth = auth
        self._backstage_auth = backstage_auth
        self._backstage = backstage

    async def get_tools(self):
        if self._backstage == "only":
            if type(self).fail_backstage:
                raise RuntimeError("Failed to fetch tools from any MCP server")
            token = self._backstage_auth._token
            type(self).calls.append(("only", token))
            return [_FakeTool(name=f"backstage-for-{token}")]
        token = self._auth._token
        type(self).calls.append((self._backstage, token))
        if type(self).fail_others:
            raise RuntimeError("Failed to fetch tools from any MCP server")
        return [_FakeTool(name=f"{self._backstage}-for-{token}")]


async def _split_setup(monkeypatch, *, fail_backstage=False, fail_others=False):
    await mcp_module.invalidate_tools_cache()
    _FakeSplitMCPClient.calls = []
    _FakeSplitMCPClient.fail_backstage = fail_backstage
    _FakeSplitMCPClient.fail_others = fail_others
    monkeypatch.setattr(mcp_module, "MCPClient", _FakeSplitMCPClient)
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", "http://backstage/mcp")


async def test_user_backstage_tools_are_fresh_while_the_rest_stay_cached(monkeypatch):
    await _split_setup(monkeypatch)

    first = await mcp_module.get_tools_for_user(
        "user-a",
        BearerTokenAuth("tok-1"),
        BearerTokenAuth("issued-1"),
    )
    # The forwarder issues a new Backstage token for every chat.
    second = await mcp_module.get_tools_for_user(
        "user-a",
        BearerTokenAuth("tok-1"),
        BearerTokenAuth("issued-2"),
    )

    assert [t.name for t in first] == ["exclude-for-tok-1", "backstage-for-issued-1"]
    assert [t.name for t in second] == ["exclude-for-tok-1", "backstage-for-issued-2"]
    assert _FakeSplitMCPClient.calls == [
        ("exclude", "tok-1"),  # cached after the first turn
        ("only", "issued-1"),
        ("only", "issued-2"),
    ]


async def test_no_backstage_tools_when_the_users_token_fails(monkeypatch):
    # The static token reads as a service: it must not stand in for a user
    # whose own token Backstage refused.
    await _split_setup(monkeypatch, fail_backstage=True)

    tools = await mcp_module.get_tools_for_user(
        "user-a",
        BearerTokenAuth("tok-1"),
        BearerTokenAuth("issued-1"),
    )

    assert [t.name for t in tools] == ["exclude-for-tok-1"]
    assert ("include", "tok-1") not in _FakeSplitMCPClient.calls


async def test_the_users_backstage_tools_survive_the_other_servers_failing(monkeypatch):
    await _split_setup(monkeypatch, fail_others=True)

    tools = await mcp_module.get_tools_for_user(
        "user-a",
        BearerTokenAuth("tok-1"),
        BearerTokenAuth("issued-1"),
    )

    assert [t.name for t in tools] == ["backstage-for-issued-1"]


async def test_raises_when_every_server_fails(monkeypatch):
    await _split_setup(monkeypatch, fail_backstage=True, fail_others=True)

    with pytest.raises(RuntimeError, match="any MCP server"):
        await mcp_module.get_tools_for_user(
            "user-a",
            BearerTokenAuth("tok-1"),
            BearerTokenAuth("issued-1"),
        )


async def test_user_token_is_ignored_without_a_backstage_server(monkeypatch):
    await _split_setup(monkeypatch)
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", "")

    tools = await mcp_module.get_tools_for_user(
        "user-a",
        BearerTokenAuth("tok-1"),
        BearerTokenAuth("issued-1"),
    )

    assert [t.name for t in tools] == ["include-for-tok-1"]
    assert _FakeSplitMCPClient.calls == [("include", "tok-1")]
