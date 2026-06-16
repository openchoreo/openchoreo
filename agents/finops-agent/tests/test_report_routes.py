# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""HTTP-level tests for the user-facing report routes.

Unlike the analyze route, every report route has auth dependencies
(``require_authn`` + a ``ReportAuthorizationChecker`` instance), so the test app
overrides them via ``dependency_overrides``.

The update tests use an ``AsyncMock`` backend whose ``update_report_actions_atomic``
*invokes the mutate callback* it is handed — exactly as the real SQL backend does
— so the route's index-validation and state-transition logic is exercised.
"""

from unittest.mock import AsyncMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from src.api.report_routes import router as report_router
from src.auth import require_authn, require_reports_authz, require_reports_update_authz


@pytest.fixture
def app():
    application = FastAPI()
    application.include_router(report_router)
    # Bypass authn/authz — these handlers never read the returned subject.
    application.dependency_overrides[require_authn] = lambda: None
    application.dependency_overrides[require_reports_authz] = lambda: None
    application.dependency_overrides[require_reports_update_authz] = lambda: None
    return application


# --------------------------------------------------------------------------- list


def test_list_reports_returns_aliased_envelope(app):
    backend = AsyncMock()
    backend.list_reports.return_value = {
        "reports": [
            {
                "reportId": "r1",
                "namespace": "ns",
                "project": "proj",
                "environment": "dev",
                "component": "comp",
                "timestamp": "2026-06-16T00:00:00Z",
                "summary": "s",
                "status": "completed",
            }
        ],
        "totalCount": 1,
    }

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get("/api/v1alpha1/reports?namespace=ns&project=proj")

    assert resp.status_code == 200
    body = resp.json()
    assert body["totalCount"] == 1
    assert body["reports"][0]["reportId"] == "r1"


def test_list_reports_empty(app):
    backend = AsyncMock()
    backend.list_reports.return_value = {"reports": [], "totalCount": 0}

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get("/api/v1alpha1/reports?namespace=ns&project=proj")

    assert resp.status_code == 200
    assert resp.json() == {"reports": [], "totalCount": 0}


# ---------------------------------------------------------------------------- get


def test_get_report_200(app):
    backend = AsyncMock()
    backend.get_report.return_value = {
        "reportId": "r1",
        "namespace": "ns",
        "project": "proj",
        "environment": "dev",
        "component": "comp",
        "@timestamp": "2026-06-16T00:00:00Z",
        "status": "completed",
        "report": {"summary": "s"},
    }

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get("/api/v1alpha1/reports/r1")

    assert resp.status_code == 200
    body = resp.json()
    assert body["reportId"] == "r1"
    assert body["report"] == {"summary": "s"}


def test_get_report_404(app):
    backend = AsyncMock()
    backend.get_report.return_value = None

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).get("/api/v1alpha1/reports/missing")

    assert resp.status_code == 404


# ------------------------------------------------------------------------- update


def _backend_invoking_mutate(actions, *, found=True):
    """AsyncMock backend whose atomic update runs the route-supplied mutate_fn."""
    backend = AsyncMock()
    state = {"actions": actions}

    async def atomic(report_id, mutate_fn):
        if not found:
            return None
        new_actions, _changed = mutate_fn(state["actions"])
        state["actions"] = new_actions
        return {"reportId": report_id, "report": {"recommended_actions": new_actions}}

    backend.update_report_actions_atomic.side_effect = atomic
    backend._state = state
    return backend


def test_update_applies_valid_index(app):
    backend = _backend_invoking_mutate([{"status": "revised", "description": "x"}])

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put("/api/v1alpha1/reports/r1", json={"appliedIndices": [0]})

    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}
    assert backend._state["actions"][0]["status"] == "applied"


def test_update_dismisses_valid_index(app):
    backend = _backend_invoking_mutate([{"status": "revised", "description": "x"}])

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put("/api/v1alpha1/reports/r1", json={"dismissedIndices": [0]})

    assert resp.status_code == 200
    assert backend._state["actions"][0]["status"] == "dismissed"


def test_update_overlapping_indices_is_422(app):
    backend = _backend_invoking_mutate([{"status": "revised"}])

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put(
            "/api/v1alpha1/reports/r1",
            json={"appliedIndices": [0], "dismissedIndices": [0]},
        )

    assert resp.status_code == 422
    backend.update_report_actions_atomic.assert_not_called()


def test_update_out_of_range_index_is_400(app):
    backend = _backend_invoking_mutate([{"status": "revised"}])

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put("/api/v1alpha1/reports/r1", json={"appliedIndices": [5]})

    assert resp.status_code == 400


def test_update_no_actions_is_400(app):
    backend = _backend_invoking_mutate([])

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put("/api/v1alpha1/reports/r1", json={"appliedIndices": [0]})

    assert resp.status_code == 400


def test_update_missing_report_is_404(app):
    backend = _backend_invoking_mutate([], found=False)

    with patch("src.api.report_routes.get_report_backend", return_value=backend):
        resp = TestClient(app).put("/api/v1alpha1/reports/r1", json={"appliedIndices": [0]})

    assert resp.status_code == 404
