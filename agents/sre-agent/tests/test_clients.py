# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the LLM factory, bearer auth, MCP client, and OpenChoreo API client."""

from unittest.mock import AsyncMock, patch

import httpx
import pytest

import src.clients.llm as llm
from common.auth.bearer import BearerTokenAuth
from src.clients import openchoreo_api
from src.clients.mcp import MCPClient

AUTH = httpx.BasicAuth("user", "pass")


def test_get_model_caches_by_args(monkeypatch):
    calls = []

    def fake_init(model, api_key, **kwargs):
        calls.append((model, api_key))
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)

    a = llm.get_model("openai:gpt-4o-mini", api_key="k1")
    b = llm.get_model("openai:gpt-4o-mini", api_key="k1")
    c = llm.get_model("openai:gpt-4o-mini", api_key="k2")

    assert a is b
    assert a is not c
    assert calls == [("openai:gpt-4o-mini", "k1"), ("openai:gpt-4o-mini", "k2")]


def test_get_model_forwards_explicit_provider(monkeypatch):
    calls = []

    def fake_init(model, api_key, **kwargs):
        calls.append((model, api_key, kwargs.get("model_provider")))
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)

    llm.get_model("gemini-3.6-flash", model_provider="google_genai", api_key="k1")

    assert calls == [("gemini-3.6-flash", "k1", "google_genai")]


def test_get_model_drops_api_key_for_vertex(monkeypatch):
    # ChatVertexAI has no api_key field, so langchain would move the key into
    # model_kwargs and send it as a generation parameter. Vertex uses ADC.
    calls = []

    def fake_init(model, **kwargs):
        calls.append("api_key" in kwargs)
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)

    llm.get_model("gemini-3.1-pro", model_provider="google_vertexai", api_key="secret-key")

    assert calls == [False]


def test_get_model_omits_provider_when_unset(monkeypatch):
    # An unset provider must not be forwarded at all, so langchain falls
    # back to inferring it from the model name.
    calls = []

    def fake_init(model, api_key, **kwargs):
        calls.append("model_provider" in kwargs)
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)
    monkeypatch.setattr(llm.settings, "rca_model_provider", "")

    llm.get_model("openai:gpt-4o-mini", api_key="k-unset-provider")

    assert calls == [False]


def test_sync_auth_flow_sets_bearer_header():
    out = next(BearerTokenAuth("tok").sync_auth_flow(httpx.Request("GET", "http://x")))
    assert out.headers["Authorization"] == "Bearer tok"


@pytest.mark.asyncio
async def test_async_auth_flow_sets_bearer_header():
    gen = BearerTokenAuth("tok").async_auth_flow(httpx.Request("GET", "http://x"))
    out = await gen.__anext__()
    assert out.headers["Authorization"] == "Bearer tok"


@pytest.mark.asyncio
async def test_mcp_get_tools_returns_tools_on_success():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.get_tools = AsyncMock(return_value=["tool-a", "tool-b"])
        client = MCPClient(auth=AUTH)
        assert await client.get_tools() == ["tool-a", "tool-b"]


@pytest.mark.asyncio
async def test_mcp_get_tools_wraps_failure_in_runtime_error():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.get_tools = AsyncMock(side_effect=Exception("boom"))
        client = MCPClient(auth=AUTH)
        with pytest.raises(RuntimeError, match="Failed to fetch tools"):
            await client.get_tools()


@pytest.mark.asyncio
async def test_openchoreo_api_get_returns_json_with_base_url_and_headers():
    captured = {}

    class FakeResponse:
        def raise_for_status(self):
            pass

        def json(self):
            return {"ok": True}

    class FakeClient:
        def __init__(self, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def get(self, url, headers=None, params=None, auth=None):
            captured["url"] = url
            captured["headers"] = headers
            return FakeResponse()

    with patch("src.clients.openchoreo_api.httpx.AsyncClient", FakeClient):
        result = await openchoreo_api.get("/namespaces/ns/projects/p", AUTH)

    assert result == {"ok": True}
    assert captured["url"].endswith("/api/v1/namespaces/ns/projects/p")
    assert captured["headers"] == {"X-Use-OpenAPI": "true"}


@pytest.mark.asyncio
async def test_openchoreo_api_get_propagates_http_errors():
    class FakeResponse:
        def raise_for_status(self):
            raise httpx.HTTPStatusError("boom", request=None, response=None)

        def json(self):
            return {}

    class FakeClient:
        def __init__(self, *args, **kwargs):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return False

        async def get(self, *args, **kwargs):
            return FakeResponse()

    with (
        patch("src.clients.openchoreo_api.httpx.AsyncClient", FakeClient),
        pytest.raises(httpx.HTTPStatusError),
    ):
        await openchoreo_api.get("/x", AUTH)
