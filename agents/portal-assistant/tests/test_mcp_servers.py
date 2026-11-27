# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for which MCP servers MCPClient registers."""

from common.auth.bearer import BearerTokenAuth
from src.clients import mcp as mcp_module

_URL = "http://backstage:7007/api/mcp-actions/v1/check-authoring"


def _backstage_auth(client):
    return client._clients["backstage"].connections["backstage"]["auth"]


def test_backstage_server_skipped_when_url_empty(monkeypatch):
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", "")

    client = mcp_module.MCPClient(auth=BearerTokenAuth("user-token"))

    assert "backstage" not in client._clients


def test_no_backstage_server_without_a_token_issued_for_the_user(monkeypatch):
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", _URL)

    client = mcp_module.MCPClient(auth=BearerTokenAuth("user-token"))

    # Every Backstage call runs as the user: no token, no Backstage tools.
    assert "backstage" not in client._clients
    assert "openchoreo" in client._clients


def test_backstage_server_uses_the_token_issued_for_the_user(monkeypatch):
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", _URL)

    client = mcp_module.MCPClient(
        auth=BearerTokenAuth("user-token"),
        backstage_auth=BearerTokenAuth("issued-for-user"),
    )

    assert client._clients["backstage"].connections["backstage"]["url"] == _URL
    assert _backstage_auth(client)._token == "issued-for-user"
    # The other servers keep the user's OpenChoreo token.
    assert client._clients["openchoreo"].connections["openchoreo"]["auth"]._token == "user-token"


def test_backstage_can_be_excluded_or_selected_alone(monkeypatch):
    monkeypatch.setattr(mcp_module.settings, "backstage_mcp_url", _URL)
    auth = BearerTokenAuth("user-token")
    issued = BearerTokenAuth("issued-for-user")

    without = mcp_module.MCPClient(auth=auth, backstage_auth=issued, backstage="exclude")
    only = mcp_module.MCPClient(auth=auth, backstage_auth=issued, backstage="only")

    assert "backstage" not in without._clients and "openchoreo" in without._clients
    assert list(only._clients) == ["backstage"]


async def test_backstage_tools_are_renamed_and_made_strict_for_the_model():
    from langchain_core.tools import StructuredTool

    rule = {
        "type": "object",
        "properties": {"conditions": {"type": "object", "additionalProperties": {}}},
        "required": ["conditions"],
    }
    tool = StructuredTool(
        name="tech-insights.validate-check",
        description="d",
        args_schema={"type": "object", "properties": {"rule": rule}, "required": ["rule"]},
        coroutine=lambda **_: None,
    )

    class _Client:
        async def get_tools(self):
            return [tool]

    client = mcp_module.MCPClient(auth=BearerTokenAuth("user-token"))

    _name, (loaded,) = await client._get_tools_for_server("backstage", _Client())

    assert loaded.name == "tech-insights__validate-check"
    conditions = loaded.args_schema["properties"]["rule"]["properties"]["conditions"]
    assert conditions["type"] == "string"
