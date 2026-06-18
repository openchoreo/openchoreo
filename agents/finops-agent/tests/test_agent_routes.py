# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""HTTP-level tests for the analyze endpoint.

A minimal app mounts only the agent router (avoiding the lifespan LLM/MCP
checks). The report backend and the background ``run_analysis`` task are mocked.
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from src.api.agent_routes import router as agent_router

VALID_BODY = {
    "searchScope": {
        "component": "my-service",
        "namespace": "my-org",
        "project": "my-project",
        "environment": "development",
    },
    "budgetedCost": {"amount": 10.0, "period": "5d", "currency": "USD"},
    "actualCost": {"amount": 25.0, "currency": "USD"},
    "budgetAlertTriggeredAt": "2026-06-16T00:00:00Z",
}


@pytest.fixture
def app():
    application = FastAPI()
    application.include_router(agent_router)
    return application


def test_analyze_returns_pending_and_schedules_task(app):
    backend = MagicMock()
    backend.upsert_report = AsyncMock()

    with (
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.run_analysis") as run,
    ):
        resp = TestClient(app).post("/api/v1alpha1/analyses", json=VALID_BODY)

    assert resp.status_code == 200
    payload = resp.json()
    assert payload["status"] == "pending"
    assert payload["reportId"].startswith("finops_my-service_")

    # The pending row carries the full search scope.
    backend.upsert_report.assert_awaited_once()
    pending = backend.upsert_report.await_args.kwargs
    assert pending["status"] == "pending"
    assert pending["namespace"] == "my-org"
    assert pending["project"] == "my-project"
    assert pending["component"] == "my-service"
    assert pending["environment"] == "development"

    # Background task scheduled with the same id, the search scope, and a
    # correctly-shaped request context.
    run.assert_called_once()
    task_kwargs = run.call_args.kwargs
    assert task_kwargs["report_id"] == payload["reportId"]
    assert task_kwargs["search_scope"].component == "my-service"
    assert task_kwargs["search_scope"].environment == "development"
    ctx = task_kwargs["request_context"]
    assert ctx["actual_cost"] == {"amount": 25.0, "currency": "USD"}
    assert ctx["budgeted_cost"] == {"amount": 10.0, "period": "5d", "currency": "USD"}


def test_analyze_returns_500_when_backend_fails(app):
    backend = MagicMock()
    backend.upsert_report = AsyncMock(side_effect=RuntimeError("db down"))

    with (
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.run_analysis") as run,
    ):
        resp = TestClient(app).post("/api/v1alpha1/analyses", json=VALID_BODY)

    assert resp.status_code == 500
    run.assert_not_called()


def test_analyze_rejects_malformed_body(app):
    with patch("src.api.agent_routes.get_report_backend"):
        resp = TestClient(app).post("/api/v1alpha1/analyses", json={"searchScope": {}})
    assert resp.status_code == 422
