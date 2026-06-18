# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ``run_analysis`` — the orchestration entry point.

The LLM is mocked at the *runnable* boundary: ``FINOPS_AGENT.create`` is patched
to return a fake agent whose ``ainvoke`` yields a canned structured response.
This exercises the surrounding logic (cost overwrite, action synthesis, status
updates, error handling) without running a real ``create_agent`` graph.
"""

from contextlib import contextmanager
from unittest.mock import AsyncMock, patch

import pytest

import src.agent.agent as agent_mod
from src.api.agent_routes import SearchScope
from src.models import RemediationAction, ResourceChange

from .factories import make_recommendation, make_report, make_request_context

# A bogus action as if the model had populated recommended_actions itself; the
# deterministic synthesizer must always discard/replace it.
HALLUCINATED_ACTION = RemediationAction(
    description="HALLUCINATED",
    rationale="model made this up",
    change=ResourceChange(release_binding="bogus", fields=[]),
)

SEARCH_SCOPE = SearchScope(
    component="my-service",
    namespace="my-org",
    project="my-project",
    environment="development",
)


@contextmanager
def patched_analysis(report, *, ainvoke_side_effect=None):
    """Patch the LLM/auth/backend boundaries for a run_analysis call.

    Yields the mock report backend so tests can assert on ``upsert_report``.
    """
    fake_agent = AsyncMock()
    if ainvoke_side_effect is not None:
        fake_agent.ainvoke = AsyncMock(side_effect=ainvoke_side_effect)
    else:
        fake_agent.ainvoke = AsyncMock(return_value={"structured_response": report})

    backend = AsyncMock()

    with (
        patch.object(
            agent_mod.FINOPS_AGENT,
            "create",
            AsyncMock(return_value=(fake_agent, None)),
        ),
        patch.object(agent_mod, "get_report_backend", return_value=backend),
        patch.object(agent_mod, "get_oauth2_auth"),
    ):
        yield backend


@pytest.mark.asyncio
async def test_completed_overwrites_actual_cost_from_request():
    report = make_report(recommendation=None)

    with patched_analysis(report) as backend:
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(actual_cost={"amount": 42.5, "currency": "EUR"}),
        )

    backend.upsert_report.assert_awaited_once()
    kwargs = backend.upsert_report.await_args.kwargs
    assert kwargs["status"] == "completed"
    assert kwargs["report_id"] == "r1"
    assert kwargs["namespace"] == "my-org"

    actual = kwargs["report"]["actual_cost"]
    assert actual["total_cost"] == 42.5
    assert actual["currency"] == "EUR"
    assert actual["is_estimated"] is False
    assert actual["breakdown_source"] == "observer_alert_value"
    assert actual["cpu_cost"] is None
    assert actual["memory_cost"] is None


@pytest.mark.asyncio
async def test_completed_clears_actions_when_remediation_disabled():
    report = make_report(recommendation=make_recommendation())

    with (
        patch.object(agent_mod.settings, "remediation_enabled", False),
        patched_analysis(report) as backend,
    ):
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    kwargs = backend.upsert_report.await_args.kwargs
    assert kwargs["report"]["recommended_actions"] == []


@pytest.mark.asyncio
async def test_completed_synthesizes_actions_when_remediation_enabled():
    report = make_report(recommendation=make_recommendation(release_binding="svc-dev"))

    with (
        patch.object(agent_mod.settings, "remediation_enabled", True),
        patched_analysis(report) as backend,
    ):
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    actions = backend.upsert_report.await_args.kwargs["report"]["recommended_actions"]
    assert len(actions) == 1
    assert actions[0]["change"]["release_binding"] == "svc-dev"


@pytest.mark.asyncio
async def test_model_supplied_actions_discarded_when_remediation_disabled():
    # The model returns its own (hallucinated) recommended_actions.
    report = make_report(recommendation=None, recommended_actions=[HALLUCINATED_ACTION])

    with (
        patch.object(agent_mod.settings, "remediation_enabled", False),
        patched_analysis(report) as backend,
    ):
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    assert backend.upsert_report.await_args.kwargs["report"]["recommended_actions"] == []


@pytest.mark.asyncio
async def test_model_supplied_actions_replaced_when_remediation_enabled():
    # Model supplies a bogus action AND there is a real recommendation; the
    # synthesized action must replace the model's, not append to it.
    report = make_report(
        recommendation=make_recommendation(release_binding="svc-dev"),
        recommended_actions=[HALLUCINATED_ACTION],
    )

    with (
        patch.object(agent_mod.settings, "remediation_enabled", True),
        patched_analysis(report) as backend,
    ):
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    actions = backend.upsert_report.await_args.kwargs["report"]["recommended_actions"]
    assert len(actions) == 1
    assert actions[0]["description"] != "HALLUCINATED"
    assert actions[0]["change"]["release_binding"] == "svc-dev"


@pytest.mark.asyncio
async def test_timeout_marks_report_failed():
    report = make_report()

    with patched_analysis(report, ainvoke_side_effect=TimeoutError()) as backend:
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    backend.upsert_report.assert_awaited_once()
    kwargs = backend.upsert_report.await_args.kwargs
    assert kwargs["status"] == "failed"
    assert "timed out" in kwargs["summary"]


@pytest.mark.asyncio
async def test_generic_failure_marks_report_failed():
    report = make_report()

    with patched_analysis(report, ainvoke_side_effect=RuntimeError("boom")) as backend:
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )

    kwargs = backend.upsert_report.await_args.kwargs
    assert kwargs["status"] == "failed"
    assert "failed" in kwargs["summary"]


@pytest.mark.asyncio
async def test_failure_during_status_update_is_swallowed():
    """If the fallback upsert also fails, run_analysis must not raise."""
    fake_agent = AsyncMock()
    fake_agent.ainvoke = AsyncMock(side_effect=RuntimeError("boom"))
    backend = AsyncMock()
    backend.upsert_report = AsyncMock(side_effect=RuntimeError("db down"))

    with (
        patch.object(agent_mod.FINOPS_AGENT, "create", AsyncMock(return_value=(fake_agent, None))),
        patch.object(agent_mod, "get_report_backend", return_value=backend),
        patch.object(agent_mod, "get_oauth2_auth"),
    ):
        # Should complete without propagating the exception.
        await agent_mod.run_analysis(
            report_id="r1",
            search_scope=SEARCH_SCOPE,
            request_context=make_request_context(),
        )
