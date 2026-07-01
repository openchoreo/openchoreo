# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""HTTP-level tests for the RCA report list / get / update endpoints."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from src.api.report_routes import router as report_router
from src.auth import (
    require_authn,
    require_reports_authz,
    require_reports_update_authz,
)
from src.auth.authz_models import SubjectContext
from src.helpers import AlertScope

SCOPE = AlertScope(
    namespace="ns",
    project="p",
    project_uid="proj-uid",
    environment="dev",
    environment_uid="env-uid",
)

BASE = "/api/v1/rca-agent/reports"
LIST_QUERY = {
    "project": "p",
    "environment": "dev",
    "namespace": "ns",
    "startTime": "2026-06-01T00:00:00Z",
    "endTime": "2026-06-30T00:00:00Z",
}


def _subject():
    return SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])


@pytest.fixture
def app():
    application = FastAPI()
    application.include_router(report_router)
    application.dependency_overrides[require_authn] = _subject
    application.dependency_overrides[require_reports_authz] = _subject
    application.dependency_overrides[require_reports_update_authz] = _subject
    return application


def test_list_returns_aliased_envelope(app):
    backend = MagicMock()
    backend.list_rca_reports = AsyncMock(
        return_value={
            "reports": [
                {
                    "alertId": "a1",
                    "reportId": "r1",
                    "timestamp": "2026-06-10T00:00:00+00:00",
                    "summary": "ok",
                    "status": "completed",
                }
            ],
            "totalCount": 1,
        }
    )

    with (
        patch("src.api.report_routes.resolve_project_scope", AsyncMock(return_value=SCOPE)),
        patch("src.api.report_routes.get_report_backend", return_value=backend),
    ):
        resp = TestClient(app).get(BASE, params=LIST_QUERY)

    assert resp.status_code == 200
    body = resp.json()
    assert body["totalCount"] == 1
    assert body["reports"][0]["reportId"] == "r1"

    call = backend.list_rca_reports.await_args.kwargs
    assert call["project_uid"] == "proj-uid"
    assert call["environment_uid"] == "env-uid"


def test_get_returns_report(app):
    backend = MagicMock()
    backend.get_rca_report = AsyncMock(
        return_value={
            "alertId": "a1",
            "reportId": "r1",
            "@timestamp": "2026-06-10T00:00:00+00:00",
            "status": "completed",
            "report": {"summary": "done"},
        }
    )
    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get(f"{BASE}/r1")

    assert resp.status_code == 200
    assert resp.json()["report"] == {"summary": "done"}


def test_get_returns_404_when_missing(app):
    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value=None)
    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get(f"{BASE}/nope")
    assert resp.status_code == 404


def test_update_rejects_overlapping_indices(app):
    resp = TestClient(app).put(f"{BASE}/r1", json={"appliedIndices": [0], "dismissedIndices": [0]})
    assert resp.status_code == 400


def test_update_applies_revised_action(app):
    stored = {
        "reportId": "r1",
        "alertId": "a1",
        "status": "completed",
        "resource": {
            "openchoreo.dev/environment-uid": "env-uid",
            "openchoreo.dev/project-uid": "proj-uid",
        },
        "report": {
            "result": {
                "recommendations": {
                    "recommended_actions": [{"status": "revised", "description": "x"}]
                }
            }
        },
    }
    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value=stored)
    backend.upsert_rca_report = AsyncMock()

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put(f"{BASE}/r1", json={"appliedIndices": [0]})

    assert resp.status_code == 200
    backend.upsert_rca_report.assert_awaited_once()
    saved = backend.upsert_rca_report.await_args.kwargs
    actions = saved["report"]["result"]["recommendations"]["recommended_actions"]
    assert actions[0]["status"] == "applied"
    assert saved["project_uid"] == "proj-uid"


def test_update_returns_404_when_report_missing(app):
    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value=None)
    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put(f"{BASE}/r1", json={"appliedIndices": [0]})
    assert resp.status_code == 404


def test_update_noop_does_not_upsert(app):
    stored = {
        "reportId": "r1",
        "alertId": "a1",
        "status": "completed",
        "resource": {},
        "report": {
            "result": {
                "recommendations": {
                    "recommended_actions": [{"status": "applied", "description": "x"}]
                }
            }
        },
    }
    backend = MagicMock()
    backend.get_rca_report = AsyncMock(return_value=stored)
    backend.upsert_rca_report = AsyncMock()

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put(f"{BASE}/r1", json={"appliedIndices": [0]})

    assert resp.status_code == 200
    backend.upsert_rca_report.assert_not_awaited()
