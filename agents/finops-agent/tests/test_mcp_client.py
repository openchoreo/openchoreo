# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ``MCPClient`` — the underlying MCP SDK is mocked at the boundary."""

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from src.clients.mcp import MCPClient

AUTH = httpx.BasicAuth("user", "pass")


@pytest.mark.asyncio
async def test_get_tools_returns_tools_on_success():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.get_tools = AsyncMock(return_value=["tool-a", "tool-b"])
        client = MCPClient(auth=AUTH)
        tools = await client.get_tools()

    assert tools == ["tool-a", "tool-b"]


@pytest.mark.asyncio
async def test_get_tools_names_unreachable_servers_in_error():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.get_tools = AsyncMock(side_effect=Exception("boom"))
        client = MCPClient(auth=AUTH)

        with (
            patch.object(
                client, "_identify_unreachable_servers", AsyncMock(return_value=["observability"])
            ),
            pytest.raises(RuntimeError, match="observability"),
        ):
            await client.get_tools()


@pytest.mark.asyncio
async def test_get_tools_generic_error_when_all_reachable():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.get_tools = AsyncMock(side_effect=Exception("boom"))
        client = MCPClient(auth=AUTH)

        with (
            patch.object(client, "_identify_unreachable_servers", AsyncMock(return_value=[])),
            pytest.raises(RuntimeError, match="Failed to fetch tools"),
        ):
            await client.get_tools()


@pytest.mark.asyncio
async def test_close_swallows_cleanup_errors():
    with patch("src.clients.mcp.MultiServerMCPClient") as mock_cls:
        mock_cls.return_value.cleanup = AsyncMock(side_effect=Exception("boom"))
        client = MCPClient(auth=AUTH)
        # Must not raise.
        await client.close()


@pytest.mark.asyncio
async def test_identify_unreachable_servers_lists_failures():
    with patch("src.clients.mcp.MultiServerMCPClient"):
        client = MCPClient(auth=AUTH)

    # First server connects, second raises -> only the second is unreachable.
    async def fake_get(_url):
        return MagicMock()

    fake_async_client = MagicMock()
    fake_async_client.__aenter__ = AsyncMock(return_value=fake_async_client)
    fake_async_client.__aexit__ = AsyncMock(return_value=False)
    fake_async_client.get = AsyncMock(side_effect=[MagicMock(), Exception("down")])

    with patch("src.clients.mcp.httpx.AsyncClient", return_value=fake_async_client):
        unreachable = await client._identify_unreachable_servers()

    assert unreachable == ["opencost"]
