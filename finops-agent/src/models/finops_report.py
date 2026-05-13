# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic import BaseModel, Field

from src.models.remediation_action import RemediationAction


class ResourceMetrics(BaseModel):
    """Current resource configuration and actual usage metrics"""

    cpu_request: str | None = Field(
        default=None, description="CPU request (e.g. '500m'). Null if unavailable."
    )
    cpu_limit: str | None = Field(
        default=None, description="CPU limit (e.g. '1000m'). Null if unavailable."
    )
    cpu_actual_avg: str | None = Field(
        default=None, description="Average CPU usage (e.g. '120m'). Null if unavailable."
    )
    cpu_actual_peak: str | None = Field(
        default=None, description="Peak CPU usage (e.g. '350m'). Null if unavailable."
    )
    memory_request: str | None = Field(
        default=None, description="Memory request (e.g. '512Mi'). Null if unavailable."
    )
    memory_limit: str | None = Field(
        default=None, description="Memory limit (e.g. '1Gi'). Null if unavailable."
    )
    memory_actual_avg: str | None = Field(
        default=None, description="Average memory usage (e.g. '200Mi'). Null if unavailable."
    )
    memory_actual_peak: str | None = Field(
        default=None, description="Peak memory usage (e.g. '400Mi'). Null if unavailable."
    )
    data_available: bool = Field(
        default=True,
        description="Whether resource metrics data was available from the monitoring tools",
    )


class CostBreakdown(BaseModel):
    """Cost breakdown by resource type"""

    cpu_cost: float | None = Field(
        ..., description="CPU cost. Null if breakdown unavailable."
    )
    memory_cost: float | None = Field(
        ..., description="Memory cost. Null if breakdown unavailable."
    )
    total_cost: float = Field(..., description="Total cost")
    currency: str = Field(..., description="Currency code (e.g. 'USD')")
    is_estimated: bool = Field(
        ...,
        description="Whether cpu_cost/memory_cost were estimated (true) or provided by tools (false)",
    )
    breakdown_source: str = Field(
        ...,
        description="How breakdown was obtained (e.g. 'opencost_allocation', 'estimated_from_usage_ratio', 'unavailable')",
    )


class ResourceRecommendation(BaseModel):
    """Recommended resource configuration"""

    cpu_request: str = Field(..., description="Recommended CPU request (e.g. '200m')")
    cpu_limit: str = Field(..., description="Recommended CPU limit (e.g. '500m')")
    memory_request: str = Field(..., description="Recommended memory request (e.g. '256Mi')")
    memory_limit: str = Field(..., description="Recommended memory limit (e.g. '512Mi')")
    estimated_savings: float = Field(..., description="Estimated cost savings")
    currency: str = Field(..., description="Currency code (e.g. 'USD')")
    rationale: str = Field(..., description="Explanation of the recommendation")
    release_binding: str | None = Field(
        default=None,
        description="Name of the ReleaseBinding to patch (e.g. 'my-service-development'). Required for remediation.",
    )


class OverprovisioningAssessment(BaseModel):
    """Assessment of whether resources are overprovisioned"""

    is_overprovisioned: bool = Field(..., description="Whether the component is overprovisioned")
    cpu_utilization_pct: float | None = Field(
        default=None,
        description="CPU utilization percentage (actual_avg / request). Null if metrics are unavailable.",
    )
    memory_utilization_pct: float | None = Field(
        default=None,
        description="Memory utilization percentage (actual_avg / request). Null if metrics are unavailable.",
    )
    analysis: str = Field(..., description="Explanation of the overprovisioning assessment")
    recommendation: ResourceRecommendation | None = Field(
        default=None, description="Resource recommendation (present when overprovisioned)"
    )


class InvestigationStep(BaseModel):
    """A step taken during the FinOps analysis"""

    action: str = Field(..., description="What the agent investigated")
    outcome: str = Field(..., description="What the agent found or concluded")
    rationale: str | None = Field(default=None, description="Why the agent took this step")


class FinOpsReport(BaseModel):
    """Complete FinOps analysis report for a component exceeding its budget"""

    component: str = Field(..., description="Component name")
    namespace: str = Field(..., description="Namespace")
    project: str = Field(..., description="Project name")
    analysis_period: str = Field(..., description="Analysis period (e.g. '5d')")
    budgeted_cost: CostBreakdown = Field(..., description="Budgeted cost breakdown")
    actual_cost: CostBreakdown = Field(..., description="Actual cost breakdown")
    resource_metrics: ResourceMetrics = Field(..., description="Resource usage metrics")
    overprovisioning: OverprovisioningAssessment = Field(
        ..., description="Overprovisioning assessment"
    )
    summary: str = Field(..., description="1-2 sentence summary of the analysis")
    investigation_path: list[InvestigationStep] = Field(
        ..., min_length=1, description="Steps taken during the analysis"
    )
    recommended_actions: list[RemediationAction] = Field(
        default_factory=list,
        description="Remediation actions synthesized from the overprovisioning recommendation",
    )
