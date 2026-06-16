# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for Pydantic model constraints and Jinja template rendering."""

import pytest
from pydantic import ValidationError

from src.api.agent_routes import AnalyzeRequest
from src.template_manager import render

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


def test_request_template_renders_scope_and_costs():
    rendered = render("api/finops_request.j2", make_request_context())
    assert "my-service" in rendered
    assert "25.0 USD" in rendered
    assert "5d" in rendered
