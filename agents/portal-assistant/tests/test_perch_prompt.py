# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for case-specific Perch investigation instructions."""

from src.template_manager import render


def _render_build_failure_prompt() -> str:
    return render(
        "prompts/perch_prompt.j2",
        {
            "read_tools": [],
            "rca_tools_available": False,
            "scope": {
                "case_type": "build_failure",
                "namespace": "default",
                "run_name": "sample-run",
                "workflow_name": "sample-workflow",
                "workflow_kind": "Workflow",
            },
        },
    )


def test_build_failure_prompt_allows_complete_fallback_chain():
    prompt = _render_build_failure_prompt()

    assert "≤6 read-tool calls total" in prompt
    assert "Phase B may call up to all four log/event tools" in prompt
    assert (
        "`get_workflow_run_logs` → `get_workflow_run_events` → "
        "`query_workflow_events` → `query_workflow_logs`"
    ) in prompt


def test_build_failure_prompt_continues_after_non_diagnostic_events():
    prompt = _render_build_failure_prompt()

    assert "no live or historical event source yields a decisive error" in prompt
    assert "even when events contain non-diagnostic rows" in prompt
    assert "only if all four sources are empty" in prompt


def test_build_failure_prompt_continues_after_non_diagnostic_live_logs():
    prompt = _render_build_failure_prompt()

    assert "`get_workflow_run_logs` yields no decisive error" in prompt
    assert "whether empty or containing only routine output" in prompt
    assert "try events (`get_workflow_run_events`" in prompt
