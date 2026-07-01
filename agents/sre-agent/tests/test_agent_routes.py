# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""HTTP-level tests for the analyze / chat endpoints.

A minimal app mounts only the agent router (avoiding the lifespan LLM/MCP
checks). Scope resolution, the report backend, and the background
``run_analysis`` / ``stream_chat`` flows are mocked.
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import FastAPI, Request
from fastapi.testclient import TestClient

from src.api.agent_routes import router as agent_router
from src.auth import require_authn, require_chat_authz
from src.auth.authz_models import SubjectContext
from src.helpers import AlertScope

SCOPE = AlertScope(
    namespace="ns",
    project="p",
    project_uid="proj-uid",
    environment="dev",
    environment_uid="env-uid",
    component="c",
    component_uid="comp-uid",
)

ANALYZE_BODY = {
    "namespace": "ns",
    "project": "p",
    "component": "c",
    "environment": "dev",
    "alert": {
        "id": "alert-7",
        "value": 95,
        "timestamp": "2026-06-16T00:00:00Z",
        "rule": {"name": "High CPU"},
    },
}

CHAT_BODY = {
    "reportId": "r1",
    "namespace": "ns",
    "project": "p",
    "environment": "dev",
    "messages": [{"role": "user", "content": "what happened?"}],
}


@pytest.fixture
def app():
    application = FastAPI()
    application.include_router(agent_router)
    return application


def _subject():
    return SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])


def test_analyze_returns_pending_and_schedules_task(app):
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock()

    with (
        patch("src.api.agent_routes.resolve_component_scope", AsyncMock(return_value=SCOPE)),
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.run_analysis") as run,
    ):
        resp = TestClient(app).post("/api/v1alpha1/rca-agent/analyze", json=ANALYZE_BODY)

    assert resp.status_code == 200
    payload = resp.json()
    assert payload["status"] == "pending"
    assert payload["report_id"].startswith("alert-7_")

    backend.upsert_rca_report.assert_awaited_once()
    pending = backend.upsert_rca_report.await_args.kwargs
    assert pending["status"] == "pending"
    assert pending["project_uid"] == "proj-uid"
    assert pending["environment_uid"] == "env-uid"

    run.assert_called_once()
    task_kwargs = run.call_args.kwargs
    assert task_kwargs["report_id"] == payload["report_id"]
    assert task_kwargs["scope"] is SCOPE


def test_analyze_returns_500_when_backend_fails(app):
    backend = MagicMock()
    backend.upsert_rca_report = AsyncMock(side_effect=RuntimeError("db down"))

    with (
        patch("src.api.agent_routes.resolve_component_scope", AsyncMock(return_value=SCOPE)),
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.run_analysis") as run,
    ):
        resp = TestClient(app).post("/api/v1alpha1/rca-agent/analyze", json=ANALYZE_BODY)

    assert resp.status_code == 500
    run.assert_not_called()


def test_analyze_rejects_malformed_body(app):
    resp = TestClient(app).post("/api/v1alpha1/rca-agent/analyze", json={"namespace": "ns"})
    assert resp.status_code == 422


def test_chat_streams_when_report_exists(app):
    async def _fake_authn(request: Request):
        request.state.bearer_token = "tok"
        return _subject()

    app.dependency_overrides[require_authn] = _fake_authn
    app.dependency_overrides[require_chat_authz] = _subject

    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value={"reportId": "r1"})

    async def fake_stream(**kwargs):
        yield '{"type": "done", "message": "hi"}\n'

    with (
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.resolve_project_scope", AsyncMock(return_value=SCOPE)),
        patch("src.api.agent_routes.stream_chat", fake_stream),
    ):
        resp = TestClient(app).post("/api/v1alpha1/rca-agent/chat", json=CHAT_BODY)

    assert resp.status_code == 200
    assert '"type": "done"' in resp.text


def test_chat_returns_404_when_report_missing(app):
    async def _fake_authn(request: Request):
        request.state.bearer_token = "tok"
        return _subject()

    app.dependency_overrides[require_authn] = _fake_authn
    app.dependency_overrides[require_chat_authz] = _subject

    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value=None)

    with (
        patch("src.api.agent_routes.get_report_backend", return_value=backend),
        patch("src.api.agent_routes.resolve_project_scope", AsyncMock(return_value=SCOPE)),
    ):
        resp = TestClient(app).post("/api/v1alpha1/rca-agent/chat", json=CHAT_BODY)

    assert resp.status_code == 404
