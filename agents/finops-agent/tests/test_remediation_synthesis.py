# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the deterministic remediation-action synthesizer.

This is pure logic (no LLM/IO) and encodes the JSON-pointer contract that the
downstream remediation step depends on, so it gets thorough coverage.
"""

from src.agent.agent import _synthesize_remediation_actions

from .factories import make_recommendation, make_report

EXPECTED_POINTERS = [
    "/spec/componentTypeEnvironmentConfigs/resources/requests/cpu",
    "/spec/componentTypeEnvironmentConfigs/resources/requests/memory",
    "/spec/componentTypeEnvironmentConfigs/resources/limits/cpu",
    "/spec/componentTypeEnvironmentConfigs/resources/limits/memory",
]


def test_no_recommendation_returns_empty():
    report = make_report(recommendation=None)
    assert _synthesize_remediation_actions(report) == []


def test_recommendation_without_release_binding_returns_empty():
    report = make_report(recommendation=make_recommendation(release_binding=None))
    assert _synthesize_remediation_actions(report) == []


def test_builds_single_action_with_four_field_changes():
    rec = make_recommendation(
        release_binding="svc-dev",
        cpu_request="200m",
        cpu_limit="500m",
        memory_request="256Mi",
        memory_limit="512Mi",
        rationale="Underutilized.",
    )
    actions = _synthesize_remediation_actions(make_report(recommendation=rec))

    assert len(actions) == 1
    action = actions[0]
    assert action.rationale == "Underutilized."
    assert action.change is not None
    assert action.change.release_binding == "svc-dev"
    assert [f.json_pointer for f in action.change.fields] == EXPECTED_POINTERS
    assert [f.value for f in action.change.fields] == ["200m", "256Mi", "500m", "512Mi"]


def test_action_description_references_release_binding():
    rec = make_recommendation(release_binding="payments-prod")
    [action] = _synthesize_remediation_actions(make_report(recommendation=rec))
    assert "payments-prod" in action.description
