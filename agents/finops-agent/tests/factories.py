# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Builders for valid domain objects used across tests.

Each helper returns a fully-populated, schema-valid object with sensible
defaults, while allowing any field to be overridden via keyword arguments.
"""

from typing import Any

from src.models import CostBreakdown, FinOpsReport
from src.models.finops_report import (
    InvestigationStep,
    OverprovisioningAssessment,
    ResourceMetrics,
    ResourceRecommendation,
)


def make_cost_breakdown(**overrides: Any) -> CostBreakdown:
    defaults: dict[str, Any] = {
        "cpu_cost": 1.0,
        "memory_cost": 2.0,
        "total_cost": 3.0,
        "currency": "USD",
        "is_estimated": True,
        "breakdown_source": "estimated_from_usage_ratio",
    }
    defaults.update(overrides)
    return CostBreakdown(**defaults)


def make_recommendation(**overrides: Any) -> ResourceRecommendation:
    defaults: dict[str, Any] = {
        "cpu_request": "200m",
        "cpu_limit": "500m",
        "memory_request": "256Mi",
        "memory_limit": "512Mi",
        "estimated_savings": 12.5,
        "currency": "USD",
        "rationale": "CPU and memory are consistently underutilized.",
        "release_binding": "my-service-development",
    }
    defaults.update(overrides)
    return ResourceRecommendation(**defaults)


def make_report(
    *,
    recommendation: ResourceRecommendation | None = None,
    is_overprovisioned: bool = True,
    **overrides: Any,
) -> FinOpsReport:
    defaults: dict[str, Any] = {
        "component": "my-service",
        "namespace": "my-org",
        "project": "my-project",
        "analysis_period": "5d",
        "budgeted_cost": make_cost_breakdown(total_cost=10.0),
        "actual_cost": make_cost_breakdown(total_cost=25.0),
        "resource_metrics": ResourceMetrics(),
        "overprovisioning": OverprovisioningAssessment(
            is_overprovisioned=is_overprovisioned,
            analysis="Average utilization is well below requests.",
            recommendation=recommendation,
        ),
        "summary": "Component is overprovisioned and can be right-sized.",
        "investigation_path": [
            InvestigationStep(action="Queried metrics", outcome="Low CPU/memory usage"),
        ],
    }
    defaults.update(overrides)
    return FinOpsReport(**defaults)


def make_request_context(**overrides: Any) -> dict[str, Any]:
    """Build the ``request_context`` dict that the route passes to run_analysis."""
    ctx: dict[str, Any] = {
        "search_scope": {
            "component": "my-service",
            "namespace": "my-org",
            "project": "my-project",
            "environment": "development",
        },
        "budgeted_cost": {"amount": 10.0, "period": "5d", "currency": "USD"},
        "actual_cost": {"amount": 25.0, "currency": "USD"},
        "budget_alert_triggered_at": "2026-06-16T00:00:00Z",
    }
    ctx.update(overrides)
    return ctx
