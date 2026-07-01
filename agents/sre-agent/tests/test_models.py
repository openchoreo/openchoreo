# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Schema-validation tests for the RCA / remediation / chat models."""

import pytest
from pydantic import ValidationError

from src.models import RCAReport, get_current_utc
from src.models.chat_response import ChatResponse
from src.models.rca_report import (
    LogEvidence,
    LogLevel,
    LogLine,
    MetricEvidence,
    NoRootCauseIdentified,
    Recommendations,
    RootCauseIdentified,
    TraceEvidence,
)
from src.models.remediation_result import ActionStatus, RemediationAction
from tests.factories import (
    make_finding,
    make_rca_report,
    make_root_cause,
    make_root_cause_identified,
)


def test_get_current_utc_is_timezone_aware():
    now = get_current_utc()
    assert now.tzinfo is not None
    assert now.utcoffset().total_seconds() == 0


def test_base_model_str_lists_fields():
    cr = ChatResponse(message="hi")
    rendered = str(cr)
    assert "message=" in rendered
    assert "actions=" in rendered


def test_chat_response_defaults_actions_to_empty_list():
    assert ChatResponse(message="hi").actions == []


def test_rca_report_round_trips_through_json():
    report = make_rca_report()
    dumped = report.model_dump(mode="json")
    restored = RCAReport.model_validate(dumped)
    assert restored.summary == report.summary
    assert isinstance(restored.result, RootCauseIdentified)


def test_rca_result_discriminator_resolves_no_root_cause():
    report = make_rca_report(
        result={
            "type": "no_root_cause_identified",
            "outcome": "insufficient_data",
            "explanation": "Not enough telemetry to conclude.",
        }
    )
    assert isinstance(report.result, NoRootCauseIdentified)


def test_evidence_discriminator_resolves_each_type():
    log = make_finding(
        evidence=LogEvidence(log_lines=[LogLine(timestamp="t", level=LogLevel.WARN, log="x")])
    )
    metric = make_finding(evidence=MetricEvidence(summary="Avg `85%`"))
    trace = make_finding(evidence=TraceEvidence(trace_id="abc", summary="slow"))
    assert log.evidence.type == "log"
    assert metric.evidence.type == "metric"
    assert trace.evidence.type == "trace"


def test_log_evidence_requires_at_least_one_line():
    with pytest.raises(ValidationError):
        LogEvidence(log_lines=[])


def test_root_cause_requires_supporting_findings():
    with pytest.raises(ValidationError):
        make_root_cause(supporting_findings=[])


def test_root_cause_identified_requires_a_root_cause():
    with pytest.raises(ValidationError):
        make_root_cause_identified(root_causes=[])


def test_recommendations_cap_recommended_actions_at_three():
    from src.models.rca_report import Action

    with pytest.raises(ValidationError):
        Recommendations(recommended_actions=[Action(description=f"a{i}") for i in range(4)])


def test_investigation_path_must_be_non_empty():
    with pytest.raises(ValidationError):
        make_rca_report(investigation_path=[])


def test_remediation_action_change_optional_for_suggested():
    action = RemediationAction(description="just monitor", status=ActionStatus.SUGGESTED)
    assert action.change is None


def test_field_change_accepts_camel_case_alias():
    from src.models.remediation_result import FieldChange

    fc = FieldChange.model_validate({"jsonPointer": "/spec/x", "value": 3})
    assert fc.json_pointer == "/spec/x"
    assert fc.value == 3


def test_file_change_mount_path_alias_round_trips():
    from src.models.remediation_result import FileChange

    fc = FileChange.model_validate({"key": "config.yaml", "mountPath": "/app/", "value": "data"})
    assert fc.mount_path == "/app/"
    assert fc.model_dump(by_alias=True)["mountPath"] == "/app/"
