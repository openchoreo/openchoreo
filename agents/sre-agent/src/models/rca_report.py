# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import StrEnum
from typing import Annotated, Literal

from pydantic import BaseModel, Discriminator, Field


class ConfidenceLevel(StrEnum):
    """Confidence level in root cause determination"""

    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


class LogLevel(StrEnum):
    """Log severity levels"""

    ERROR = "ERROR"
    WARN = "WARN"
    INFO = "INFO"
    DEBUG = "DEBUG"
    UNDEFINED = "UNDEFINED"


class NoRootCauseOutcome(StrEnum):
    """Categorized outcomes when no root cause is identified"""

    NO_ANOMALY_DETECTED = "no_anomaly_detected"  # System appears healthy
    INSUFFICIENT_DATA = "insufficient_data"  # Missing telemetry to conclude
    TRANSIENT = "transient"  # Issue self-resolved before analysis
    EXTERNAL_DEPENDENCY = "external_dependency"  # Issue in external system


class AlertCondition(BaseModel):
    """Structured alert condition that triggered the alert"""

    window: str = Field(..., description="Time window for the condition")
    interval: str = Field(..., description="Evaluation interval")
    operator: str = Field(..., description="Comparison operator")
    threshold: int = Field(..., description="Threshold value that was exceeded")


class ReportAlertContext(BaseModel):
    """Alert context echoed in the RCA report for reference"""

    alert_id: str = Field(..., description="Unique identifier of the alert")
    alert_name: str = Field(..., description="Name of the alert rule that triggered")
    alert_description: str | None = Field(default=None, description="Description of the alert rule")
    severity: str | None = Field(default=None, description="Alert severity level")
    triggered_at: str = Field(..., description="ISO 8601 timestamp when alert fired")
    trigger_value: float = Field(..., description="The value that triggered the alert")
    source_type: str | None = Field(
        default=None, description="Alert source type (e.g., 'log', 'metric')"
    )
    source_query: str | None = Field(
        default=None, description="The query used to detect this alert if source type is log"
    )
    source_metric: str | None = Field(
        default=None, description="The metric name if source type is metric"
    )
    condition: AlertCondition = Field(..., description="The condition that triggered the alert")
    component: str = Field(..., description="Component name the alert was configured on")
    project: str = Field(..., description="Project name of the relevant component")
    environment: str = Field(..., description="Environment name of the relevant project")


class TimeRange(BaseModel):
    """Time range for observations"""

    start: str = Field(..., description="ISO 8601 timestamp for range start")
    end: str = Field(..., description="ISO 8601 timestamp for range end")


class LogLine(BaseModel):
    """A single log line with valuable information for RCA"""

    timestamp: str = Field(..., description="ISO 8601 timestamp when the log was emitted")
    level: LogLevel = Field(..., description="Log severity level")
    log: str = Field(..., description="The log message content")


class LogEvidence(BaseModel):
    """Evidence from application logs"""

    type: Literal["log"] = "log"
    log_lines: list[LogLine] = Field(
        ...,
        min_length=1,
        description="Relevant log lines (can be 1 or multiple related lines)",
    )
    repetition: str | None = Field(
        default=None,
        description="One sentence explaining repetition pattern if applicable (e.g., 'This error repeated 47 times over 5 minutes')",
    )


class MetricEvidence(BaseModel):
    """Evidence from metrics"""

    type: Literal["metric"] = "metric"
    summary: str = Field(
        ...,
        description="Summary of the metric behavior. Use backticks to highlight key info (e.g., 'Avg `85%`, peaked at `99%`')",
    )


class TraceEvidence(BaseModel):
    """Evidence from distributed traces"""

    type: Literal["trace"] = "trace"
    trace_id: str = Field(..., description="Trace ID for linking to trace viewer")
    span_id: str | None = Field(default=None, description="Span ID for linking to specific span")
    summary: str = Field(
        ...,
        description="Summary of the trace issue. Use backticks to highlight key info (e.g., 'db.query took `4,800ms`')",
    )
    is_error: bool = Field(default=False, description="Whether this span had an error")
    error_message: str | None = Field(default=None, description="Error message if is_error is True")
    repetition: str | None = Field(
        default=None,
        description="One sentence explaining repetition pattern if applicable (e.g., 'Similar slow spans seen in 23 traces')",
    )


# Discriminated union for evidence types
Evidence = Annotated[LogEvidence | MetricEvidence | TraceEvidence, Discriminator("type")]


class Finding(BaseModel):
    """A single observation that supports a root cause"""

    observation: str = Field(..., description="Human-readable summary of the finding")
    component: str = Field(..., description="Component name this finding relates to")
    time_range: TimeRange = Field(..., description="Time range for deep-dive linking")
    evidence: Evidence = Field(..., description="The supporting evidence")


class RootCause(BaseModel):
    """An identified root cause with its supporting findings"""

    summary: str = Field(
        ...,
        description="One sentence summary of the root cause",
    )
    confidence: ConfidenceLevel = Field(
        ..., description="Confidence level in this root cause determination"
    )
    analysis: str = Field(
        ...,
        description="2-3 sentence concise explanation of how findings correlate to support this root cause. Do not repeat the summary. Use backticks to highlight key values",
    )
    supporting_findings: list[Finding] = Field(
        ...,
        min_length=1,
        description="Include only evidence-backed observations directly supporting this root cause. If a finding has multiple insights within a rootcause, do not split it into multiple findings",
    )


class TimelineEvent(BaseModel):
    """A significant system event - when, where, what"""

    timestamp: str = Field(..., description="ISO 8601 timestamp when the event occurred")
    component: str | None = Field(
        default=None,
        description="Which component (None for alert/system-level events)",
    )
    event: str = Field(
        ...,
        description="What happened - include only significant system-level events in the causal chain. Use backticks to highlight key info",
    )


class InvestigationStep(BaseModel):
    """A significant step the agent took during investigation"""

    action: str = Field(
        ...,
        description="What the agent investigated (e.g., 'Analyzed error logs from analytics-service')",
    )
    outcome: str = Field(
        ..., description="What the agent found or concluded. Use backticks to highlight key info"
    )
    rationale: str | None = Field(
        default=None,
        description="Why the agent took this step",
    )


class ExcludedCause(BaseModel):
    """A potential cause that was investigated and ruled out"""

    description: str = Field(
        ..., description="The potential cause that was investigated and excluded"
    )
    rationale: str = Field(
        ...,
        description="Why this was ruled out as a root cause based on evidence. Use backticks to highlight key info",
    )


class Action(BaseModel):
    """An actionable recommendation"""

    description: str = Field(..., description="Description of the action to take")
    rationale: str | None = Field(
        default=None,
        description="Why this action is recommended. Use backticks to highlight key info",
    )


class Recommendations(BaseModel):
    """Actionable recommendations to prevent recurrence"""

    recommended_actions: list[Action] = Field(
        default_factory=list,
        max_length=3,
        description="Top 2-3 prioritized actions to mitigate and prevent recurrence. Only include more than 2 if absolutely necessary",
    )
    observability_recommendations: list[Action] = Field(
        default_factory=list,
        max_length=2,
        description="Up to 2 suggestions for improving telemetry/monitoring for better future RCA",
    )


class RootCauseIdentified(BaseModel):
    """RCA was performed and root causes were identified"""

    type: Literal["root_cause_identified"] = "root_cause_identified"
    root_causes: list[RootCause] = Field(
        ...,
        min_length=1,
        description="Identified root causes in order of significance",
    )
    timeline: list[TimelineEvent] = Field(
        ...,
        min_length=1,
        description="Chronological sequence of significant system events. Include only system(project) level events",
    )
    excluded_causes: list[ExcludedCause] = Field(
        default_factory=list,
        description="Potential causes that were investigated and ruled out",
    )
    recommendations: Recommendations = Field(
        ...,
        description="Actionable practical recommendations to prevent recurrence",
    )


class NoRootCauseIdentified(BaseModel):
    """RCA was performed but no root cause could be identified"""

    type: Literal["no_root_cause_identified"] = "no_root_cause_identified"
    outcome: NoRootCauseOutcome = Field(
        ..., description="Categorized reason why no root cause was identified"
    )
    explanation: str = Field(
        ...,
        description="Detailed explanation of why no root cause was identified",
    )
    recommendations: Recommendations | None = Field(
        default=None,
        description="Recommendations for improving observability if applicable",
    )


# Discriminated union for RCA result
RCAResult = Annotated[RootCauseIdentified | NoRootCauseIdentified, Discriminator("type")]


class RCAReport(BaseModel):
    """Complete Root Cause Analysis Report OpenChoreo incidents"""

    alert_context: ReportAlertContext = Field(..., description="The alert that triggered this RCA")

    summary: str = Field(
        ...,
        description="Concise summary of the investigation outcome (1 sentence)",
    )

    result: RCAResult = Field(
        ...,
        description="The RCA result - either root causes identified, or explanation of why not",
    )

    investigation_path: list[InvestigationStep] = Field(
        ...,
        min_length=1,
        description="Sequential steps the agent took during investigation",
    )
