# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for Pydantic model constraints and Jinja template rendering."""

import pytest
from pydantic import ValidationError

from src.api.agent_routes import AnalyzeRequest
from src.templates import render

from .factories import make_cost_breakdown, make_report, make_request_context


def test_finops_report_requires_non_empty_investigation_path():
    with pytest.raises(ValidationError):
        make_report(investigation_path=[])


def test_recommended_actions_default_empty():
    assert make_report().recommended_actions == []


def test_cost_breakdown_allows_null_component_costs():
    cb = make_cost_breakdown(cpu_cost=None, memory_cost=None)
    assert cb.cpu_cost is None
    assert cb.memory_cost is None


def test_analyze_request_populates_from_aliases():
    req = AnalyzeRequest.model_validate(
        {
            "searchScope": {
                "component": "c",
                "namespace": "n",
                "project": "p",
                "environment": "dev",
            },
            "budgetedCost": {"amount": 10.0, "period": "5d", "currency": "USD"},
            "actualCost": {"amount": 25.0, "currency": "USD"},
            "budgetAlertTriggeredAt": "2026-06-16T00:00:00Z",
        }
    )
    assert req.search_scope.component == "c"
    assert req.actual_cost.amount == 25.0
    assert req.search_scope.release_line is None


def test_analyze_request_accepts_release_line():
    req = AnalyzeRequest.model_validate(
        {
            "searchScope": {
                "component": "c",
                "namespace": "n",
                "project": "p",
                "environment": "dev",
                "releaseLine": "canary",
            },
            "budgetedCost": {"amount": 10.0, "period": "5d", "currency": "USD"},
            "actualCost": {"amount": 25.0, "currency": "USD"},
            "budgetAlertTriggeredAt": "2026-06-16T00:00:00Z",
        }
    )
    assert req.search_scope.release_line == "canary"
    assert req.search_scope.model_dump()["release_line"] == "canary"


def test_request_template_renders_scope_and_costs():
    rendered = render("api/finops_request.j2", make_request_context())
    assert "my-service" in rendered
    assert "25.0 USD" in rendered
    assert "5d" in rendered


def test_request_template_names_the_release_line():
    ctx = make_request_context()
    ctx["search_scope"] = {**ctx["search_scope"], "release_line": "canary"}
    rendered = render("api/finops_request.j2", ctx)
    assert "- **Release Line**: canary" in rendered
    assert "this component's `canary` release line over the budget period" in rendered

    plain = render("api/finops_request.j2", make_request_context())
    assert "Release Line" not in plain
    assert "release line" not in plain
