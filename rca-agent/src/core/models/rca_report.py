# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import Enum
from typing import Annotated, Literal

from pydantic import BaseModel, Discriminator, Field


class ConfidenceLevel(str, Enum):
    """Confidence level in root cause determination"""

    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


class LogLevel(str, Enum):
    """Log severity levels"""

    ERROR = "ERROR"
    WARN = "WARN"
    INFO = "INFO"
    DEBUG = "DEBUG"


class TimeRange(BaseModel):
    """Time range for metric observations"""

    start: str = Field(..., description="ISO 8601 timestamp for range start")
    end: str = Field(..., description="ISO 8601 timestamp for range end")


class MetricSeverity(str, Enum):
    """Severity level for metric values - used for color coding in UI"""

    CRITICAL = "critical"
    WARNING = "warning"
    NORMAL = "normal"


class EvidenceType(str, Enum):
    """Types of evidence that can support a root cause"""

    LOG = "log"
    METRIC = "metric"
    TRACE = "trace"


class BaseEvidenceItem(BaseModel):
    """Base class for all evidence items"""

    type: EvidenceType
    component_uid: str = Field(..., description="Component UID this evidence relates to")
    project_uid: str = Field(..., description="Project UID this evidence relates to")


class LogEvidenceItem(BaseEvidenceItem):
    """Evidence from application logs showing significant issues

    Contains the actual log message that implies a root cause. Do not include unrelated or duplicate log entries.
    """

    type: Literal[EvidenceType.LOG] = EvidenceType.LOG
    log_level: LogLevel = Field(..., description="Severity level of the log")
    timestamp: str = Field(..., description="ISO 8601 timestamp when log occurred")
    log_message: str = Field(..., description="The significant log message implying root cause")


class MetricEvidenceItem(BaseEvidenceItem):
    """Evidence from metrics - rendered as a styled value

    Example- metric_name="RAM usage", value="95.5%", severity=CRITICAL
    renders as "95.5% RAM usage" with value in red
    """

    type: Literal[EvidenceType.METRIC] = EvidenceType.METRIC

    metric_name: str = Field(
        ...,
        description="Human-readable name of the metric (e.g., 'RAM usage', 'CPU utilization', 'Request latency p99')",
    )

    value: str = Field(
        ...,
        description="Short formatted metric value with unit (e.g., '95.5%', '1.2 cores', '64 req/s', '2.5 GB/s'). Do not include timestamps and long text here.",
    )

    description: str = Field(
        ...,
        description="Self-contained explanation of the anomaly and its RCA relevance (e.g., 'RAM usage at 95.5% exceeds 90% threshold, causing OOM kills')",
    )

    severity: MetricSeverity = Field(
        ..., description="Severity level: CRITICAL (red), WARNING (yellow), NORMAL (green)"
    )

    time_range: TimeRange = Field(
        ..., description="Time range for aggregated metrics. Use timestamp OR time_range, not both."
    )


class SpanInfo(BaseModel):
    """Information about a significant span within a trace"""

    span_id: str = Field(..., description="Unique span identifier")
    name: str = Field(
        ...,
        description="Span operation name (e.g., 'GET /api/users', 'db.query', 'redis.get')",
    )
    component_uid: str = Field(..., description="Component that executed this span")
    duration_ms: float = Field(..., description="Span duration in milliseconds")
    start_time: str = Field(..., description="ISO 8601 timestamp when span started")
    end_time: str = Field(..., description="ISO 8601 timestamp when span ended")
    is_error: bool | None = Field(default=None, description="Whether this span had an error")
    error_message: str | None = Field(default=None, description="Error message if is_error is True")
    parent_span_id: str | None = Field(
        default=None, description="Parent span ID to show request flow hierarchy"
    )


class TraceEvidenceItem(BaseEvidenceItem):
    """Evidence from distributed traces showing request flow patterns

    Context and analysis provided by root cause description
    """

    type: Literal[EvidenceType.TRACE] = EvidenceType.TRACE
    trace_id: str = Field(..., description="Unique trace identifier for reference")
    total_duration_ms: float = Field(..., description="Total trace duration in milliseconds")
    significant_spans: list[SpanInfo] = Field(
        default_factory=list,
        description="Key spans that are significant for RCA (e.g., slowest spans, error spans, bottlenecks). Include 3-5 most relevant spans.",
    )


# Discriminated union type for evidence
EvidenceItem = Annotated[
    LogEvidenceItem | MetricEvidenceItem | TraceEvidenceItem, Discriminator("type")
]


class RootCause(BaseModel):
    """An identified root cause with its supporting evidence"""

    description: str = Field(
        ...,
        description="Concise description of the root cause, in one sentence.",
    )
    confidence: ConfidenceLevel = Field(
        ..., description="AI confidence level in this root cause determination"
    )
    evidences: list[EvidenceItem] = Field(
        default_factory=list,
        description="Evidence supporting this root cause from logs, metrics, or traces. DO NOT include unrelated evidences or duplicate evidences(eg: repeating redundant log entries).",
    )
    analysis: str = Field(
        ...,
        description="Explain the root cause, and how the evidences correlate with each other and support this root cause determination",
    )


class TimelineEvent(BaseModel):
    """A significant system event observed in telemetry data. Represents actual system behavior, not agent investigation actions."""

    timestamp: str = Field(..., description="ISO 8601 timestamp when the event occurred")
    description: str = Field(
        ...,
        description="Description of what happened in the system (e.g., 'analytics-service started returning 500 errors')",
    )
    source_type: EvidenceType = Field(..., description="Which telemetry type revealed this event")
    aggregated_count: int | None = Field(
        default=None,
        description="If this event represents multiple similar occurrences, how many times did it occur",
    )


class InvestigationStep(BaseModel):
    """A significant step the agent took during investigation"""

    action: str = Field(
        ...,
        description="What the agent investigated (e.g., 'Analyzed error logs from analytics-service')",
    )
    outcome: str = Field(..., description="What the agent found or concluded from this step")
    rationale: str | None = Field(
        default=None,
        description="Why the agent took this step (e.g., 'Previous step showed high error rate')",
    )


class ExcludedCause(BaseModel):
    """A potential cause that was investigated and ruled out"""

    description: str = Field(
        ..., description="The potential cause that was investigated and excluded"
    )
    reason: str = Field(..., description="Why this was ruled out as a root cause based on evidence")


class Action(BaseModel):
    """An actionable recommendation"""

    description: str = Field(..., description="Description of the action to take")
    rationale: str | None = Field(default=None, description="Why this action is recommended")


class Recommendations(BaseModel):
    """Actionable recommendations to prevent recurrence"""

    actions: list[Action] = Field(
        default_factory=list,
        description="Prioritized actions sorted by priority.",
    )
    monitoring_improvements: list[str] = Field(
        default_factory=list,
        description="Suggestions for additional monitoring, alerting, or observability improvements",
    )


class IssueIdentified(BaseModel):
    """RCA was performed and issues were identified"""

    type: Literal["issue_identified"] = "issue_identified"
    root_causes: list[RootCause] = Field(
        ...,
        min_length=1,
        description="Identified root causes in order of significance. Each contains its own supporting evidence.",
    )
    timeline: list[TimelineEvent] = Field(
        ...,
        min_length=1,
        description="Chronological sequence of significant system events discovered through analysis",
    )
    excluded_causes: list[ExcludedCause] = Field(
        default_factory=list,
        description="Potential causes that were investigated and ruled out with reasoning, helping narrow down the actual root cause.",
    )
    recommendations: Recommendations = Field(
        ...,
        description="Actionable recommendations to prevent recurrence of this issue. Do not include vague or non-actionable suggestions. Provide at most two actions each unless absolutely necessary.",
    )


class RCANotPerformed(BaseModel):
    """RCA was not performed due to insufficient data or other reasons"""

    type: Literal["rca_not_performed"] = "rca_not_performed"
    reason: str = Field(
        ...,
        description="Explanation of why RCA was not performed (e.g., insufficient telemetry data, false positive, no issue detected)",
    )
    recommendations: Recommendations | None = Field(
        default=None,
        description="Recommendations for improving tracing, observability, monitoring, or alerting to enable better RCA in the future. Do not include vague or non-actionable suggestions. Provide at most two actions unless absolutely necessary.",
    )


# Discriminated union for RCA result
RCAResult = Annotated[IssueIdentified | RCANotPerformed, Discriminator("type")]


class RCAReport(BaseModel):
    """Complete Root Cause Analysis Report for OpenChoreo incidents"""

    summary: str = Field(
        ...,
        description="Concise summary of the investigation and outcome. Maximum 1 short sentence on what happened and what went wrong.",
    )

    result: RCAResult = Field(
        ...,
        description="The result of the RCA - either issues identified with root causes, or explanation of why RCA was not performed",
    )

    investigation_path: list[InvestigationStep] = Field(
        ...,
        min_length=1,
        description="Sequential steps the agent took during investigation. Include only significant investigative actions",
    )
