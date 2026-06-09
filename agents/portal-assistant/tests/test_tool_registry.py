# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the mutating-vs-read classification heuristic.

Critical because misclassification = WriteGuard bypass.
"""
from dataclasses import dataclass

from src.agent.tool_registry import is_mutating, log_classification_summary


@dataclass
class _FakeTool:
    name: str
    description: str = ""


def test_create_prefix_is_mutating():
    assert is_mutating(_FakeTool(name="create_component"))


def test_update_prefix_is_mutating():
    assert is_mutating(_FakeTool(name="update_environment"))


def test_delete_prefix_is_mutating():
    assert is_mutating(_FakeTool(name="delete_workflow_run"))


def test_pe_create_substring_is_mutating():
    # pe-toolset prefixes the verb: "pe_create_workflow"
    assert is_mutating(_FakeTool(name="pe_create_workflow"))


def test_list_prefix_is_read():
    assert not is_mutating(_FakeTool(name="list_components"))


def test_get_prefix_is_read():
    assert not is_mutating(_FakeTool(name="get_workflow_run"))


def test_query_prefix_is_read():
    # Regression: every observer MCP tool is named query_*. Without
    # query_ in READ_PREFIXES they fall through to the description-
    # keyword fallback, which misclassifies any whose description
    # mentions a write verb ("applies filtering", "triggers a
    # search"). WriteGuard then refuses them as mutations, burning
    # two ~30 s LLM round-trips per turn.
    for name in (
        "query_component_logs",
        "query_workflow_logs",
        "query_resource_metrics",
        "query_http_metrics",
        "query_alerts",
        "query_traces",
        "query_trace_spans",
        "query_incidents",
    ):
        assert not is_mutating(_FakeTool(name=name)), name


def test_query_prefix_beats_description_keyword():
    # The read-prefix check must run BEFORE the description-keyword
    # fallback. Otherwise a query_* tool whose description includes
    # any write verb gets misclassified.
    assert not is_mutating(
        _FakeTool(
            name="query_traces",
            description="Query distributed traces. Triggers a search across the indexed span store.",
        ),
    )


def test_creation_schema_override_is_read():
    # READ_OVERRIDES — the name has 'create' but it's a schema reader.
    assert not is_mutating(_FakeTool(name="get_component_type_creation_schema"))
    assert not is_mutating(_FakeTool(name="get_cluster_component_type_creation_schema"))
    assert not is_mutating(_FakeTool(name="get_trait_creation_schema"))


def test_unknown_name_with_write_keyword_in_description():
    assert is_mutating(
        _FakeTool(name="provision_dataplane", description="Provisions a new data plane")
    )


def test_unknown_name_with_read_keyword():
    assert not is_mutating(
        _FakeTool(name="describe_thing", description="Returns details about thing")
    )


def test_genuinely_unknown_defaults_to_mutating():
    # Ambiguity must err toward "show the user a confirm". Failure mode
    # of false positive (read prompts confirm) is benign; false negative
    # (write skips confirm) is dangerous.
    assert is_mutating(_FakeTool(name="zzz_unfamiliar_op"))


def test_classification_summary_logs_both_buckets(caplog):
    import logging
    caplog.set_level(logging.INFO, logger="src.agent.tool_registry")
    tools = [
        _FakeTool(name="create_component"),
        _FakeTool(name="list_components"),
        _FakeTool(name="get_component"),
    ]
    log_classification_summary(tools)
    text = "\n".join(r.getMessage() for r in caplog.records)
    assert "1 mutating, 2 read-only" in text
    assert "create_component" in text
    assert "list_components" in text
    assert "get_component" in text
