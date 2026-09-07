# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the tool registry metadata."""

from src.agent.tool_registry import (
    OBSERVABILITY,
    OBSERVABILITY_TOOLS,
    OPENCHOREO,
    OPENCHOREO_TOOLS,
    TOOL_ACTIVE_FORMS,
    TOOLS,
)


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


def test_native_mcp_discovery_tools_are_grouped_as_openchoreo():
    expected = {
        TOOLS.LIST_COMPONENTS,
        TOOLS.GET_COMPONENT,
        TOOLS.LIST_WORKLOADS,
        TOOLS.GET_WORKLOAD,
        TOOLS.LIST_RELEASE_BINDINGS,
        TOOLS.GET_RELEASE_BINDING,
        TOOLS.GET_COMPONENT_RELEASE,
        TOOLS.GET_COMPONENT_RELEASE_SCHEMA,
        TOOLS.LIST_RESOURCE_RELEASE_BINDINGS,
        TOOLS.GET_RESOURCE_RELEASE_BINDING,
    }

    assert expected <= OPENCHOREO_TOOLS
