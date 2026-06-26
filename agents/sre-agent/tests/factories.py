# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Builders for valid domain objects used across tests."""

from typing import Any

from src.models import RCAReport, RemediationResult
from src.models.rca_report import (
    AlertCondition,
    Finding,
    InvestigationStep,
    LogEvidence,
    LogLevel,
    LogLine,
    Recommendations,
    ReportAlertContext,
    RootCause,
    RootCauseIdentified,
    TimelineEvent,
    TimeRange,
)
from src.models.remediation_result import (
    ActionStatus,
    FieldChange,
    RemediationAction,
    ResourceChange,
)


def make_alert_condition(**overrides: Any) -> AlertCondition:
    defaults: dict[str, Any] = {
        "window": "5m",
        "interval": "1m",
        "operator": ">",
        "threshold": 90,
    }
    defaults.update(overrides)
    return AlertCondition(**defaults)


def make_alert_context(**overrides: Any) -> ReportAlertContext:
    defaults: dict[str, Any] = {
        "alert_id": "alert-1",
        "alert_name": "High CPU",
        "triggered_at": "2026-06-16T00:00:00+00:00",
        "trigger_value": 95.0,
        "condition": make_alert_condition(),
        "component": "my-service",
        "project": "my-project",
        "environment": "development",
    }
    defaults.update(overrides)
    return ReportAlertContext(**defaults)


def make_finding(**overrides: Any) -> Finding:
    defaults: dict[str, Any] = {
        "observation": "Repeated OOM kills",
        "component": "my-service",
        "time_range": TimeRange(
            start="2026-06-16T00:00:00+00:00",
            end="2026-06-16T00:05:00+00:00",
        ),
        "evidence": LogEvidence(
            log_lines=[
                LogLine(
                    timestamp="2026-06-16T00:01:00+00:00",
                    level=LogLevel.ERROR,
                    log="OOMKilled",
                )
            ]
        ),
    }
    defaults.update(overrides)
    return Finding(**defaults)


def make_root_cause(**overrides: Any) -> RootCause:
    defaults: dict[str, Any] = {
        "summary": "Memory limit too low",
        "confidence": "high",
        "analysis": "Usage consistently exceeds the configured limit.",
        "supporting_findings": [make_finding()],
    }
    defaults.update(overrides)
    return RootCause(**defaults)


def make_root_cause_identified(**overrides: Any) -> RootCauseIdentified:
    defaults: dict[str, Any] = {
        "root_causes": [make_root_cause()],
        "timeline": [TimelineEvent(timestamp="2026-06-16T00:00:00+00:00", event="Alert fired")],
        "recommendations": Recommendations(),
    }
    defaults.update(overrides)
    return RootCauseIdentified(**defaults)


def make_rca_report(**overrides: Any) -> RCAReport:
    defaults: dict[str, Any] = {
        "alert_context": make_alert_context(),
        "summary": "Service was OOM-killed due to an undersized memory limit.",
        "result": make_root_cause_identified(),
        "investigation_path": [
            InvestigationStep(action="Queried logs", outcome="Found OOM kills"),
        ],
    }
    defaults.update(overrides)
    return RCAReport(**defaults)


def make_field_change(**overrides: Any) -> FieldChange:
    defaults: dict[str, Any] = {
        "json_pointer": "/spec/componentTypeEnvironmentConfigs/memoryLimit",
        "value": "512Mi",
    }
    defaults.update(overrides)
    return FieldChange(**defaults)


def make_remediation_action(**overrides: Any) -> RemediationAction:
    defaults: dict[str, Any] = {
        "description": "Raise the memory limit to 512Mi",
        "rationale": "Usage peaks above the current limit.",
        "status": ActionStatus.REVISED,
        "change": ResourceChange(
            release_binding="my-service-development",
            fields=[make_field_change()],
        ),
    }
    defaults.update(overrides)
    return RemediationAction(**defaults)


def make_remediation_result(**overrides: Any) -> RemediationResult:
    defaults: dict[str, Any] = {
        "recommended_actions": [make_remediation_action()],
    }
    defaults.update(overrides)
    return RemediationResult(**defaults)
