# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the tool registry: tool metadata and OpenChoreo tool factories."""

import json
from unittest.mock import AsyncMock, patch

import httpx
import pytest

from src.agent.tool_registry import (
    ALL_TOOL_FACTORIES,
    OBSERVABILITY,
    OBSERVABILITY_TOOLS,
    OPENCHOREO,
    OPENCHOREO_TOOLS,
    TOOL_ACTIVE_FORMS,
    TOOLS,
    create_list_components_tool,
    create_list_release_bindings_tool,
)

AUTH = httpx.BasicAuth("user", "pass")


def test_tool_is_str_with_metadata():
    assert TOOLS.QUERY_TRACES == "query_traces"
    assert TOOLS.QUERY_TRACES.server == OBSERVABILITY
    assert TOOLS.QUERY_TRACES.active_form == "Retrieving traces..."


def test_tools_are_grouped_by_server():
    assert TOOLS.QUERY_TRACES in OBSERVABILITY_TOOLS
    assert TOOLS.LIST_COMPONENTS in OPENCHOREO_TOOLS
    assert all(t.server == OBSERVABILITY for t in OBSERVABILITY_TOOLS)
    assert all(t.server == OPENCHOREO for t in OPENCHOREO_TOOLS)


def test_active_forms_only_include_tools_with_forms():
    assert TOOL_ACTIVE_FORMS[TOOLS.QUERY_RESOURCE_METRICS] == "Gathering resource metrics..."
    assert all(v is not None for v in TOOL_ACTIVE_FORMS.values())


def test_all_tool_factories_are_callables():
    assert create_list_components_tool in ALL_TOOL_FACTORIES
    assert len(ALL_TOOL_FACTORIES) == 5


@pytest.mark.asyncio
async def test_list_components_factory_builds_tool_and_calls_api():
    tool = create_list_components_tool(AUTH)
    assert tool.name == "list_components"

    get_mock = AsyncMock(return_value={"items": ["a"]})
    with patch("src.agent.tool_registry.get", get_mock):
        out = await tool.coroutine(namespace="ns", project="p")

    assert json.loads(out) == {"items": ["a"]}
    assert get_mock.await_args.args[0] == "/namespaces/ns/components"
    assert get_mock.await_args.kwargs["params"] == {"project": "p"}


@pytest.mark.asyncio
async def test_list_release_bindings_factory_filters_by_component():
    tool = create_list_release_bindings_tool(AUTH)
    assert tool.name == "list_release_bindings"

    get_mock = AsyncMock(return_value={"items": []})
    with patch("src.agent.tool_registry.get", get_mock):
        await tool.coroutine(namespace="ns", component="c")

    assert get_mock.await_args.args[0] == "/namespaces/ns/releasebindings"
    assert get_mock.await_args.kwargs["params"] == {"component": "c"}
