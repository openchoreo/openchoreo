# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Integration-style tests for ``SQLReportBackend`` against a real SQLite DB.

A temp-file database (not ``:memory:``) is used so the schema persists across
the separate connections that ``initialize`` / ``get`` / ``list`` open.
"""

from datetime import UTC, datetime
from unittest.mock import AsyncMock, patch

import pytest
import pytest_asyncio
from sqlalchemy import text
from sqlalchemy.ext.asyncio import create_async_engine

from src.clients.backend.sql_backend import SQLReportBackend

WIDE_START = "2000-01-01T00:00:00+00:00"
WIDE_END = "2100-01-01T00:00:00+00:00"

LEGACY_SCHEMA = """
CREATE TABLE rca_reports (
    report_id VARCHAR NOT NULL,
    alert_id VARCHAR NOT NULL,
    status VARCHAR DEFAULT 'pending' NOT NULL,
    summary TEXT,
    timestamp VARCHAR NOT NULL,
    environment_uid VARCHAR,
    project_uid VARCHAR,
    report TEXT,
    PRIMARY KEY (report_id)
)
"""


@pytest_asyncio.fixture
async def backend(tmp_path):
    engine = create_async_engine(f"sqlite+aiosqlite:///{tmp_path}/rca.db")
    be = SQLReportBackend(engine)
    await be.initialize()
    yield be
    await be.close()


def _ts(day: int) -> datetime:
    return datetime(2026, 6, day, tzinfo=UTC)


async def _legacy_backend(tmp_path, rows: list[dict[str, str]]) -> SQLReportBackend:
    engine = create_async_engine(f"sqlite+aiosqlite:///{tmp_path}/legacy.db")
    async with engine.begin() as conn:
        await conn.execute(text(LEGACY_SCHEMA))
        for row in rows:
            await conn.execute(
                text(
                    "INSERT INTO rca_reports (report_id, alert_id, status, timestamp, "
                    "project_uid, environment_uid) VALUES (:report_id, :alert_id, "
                    "'completed', :timestamp, :project_uid, :environment_uid)"
                ),
                row,
            )
    backend = SQLReportBackend(engine)
    # The naming pass has its own tests; keep this one off the network.
    with (
        patch("src.report_migration.get_oauth2_auth", side_effect=RuntimeError("no credentials")),
        patch("src.report_migration.get", AsyncMock()),
    ):
        await backend.initialize()
    return backend


async def _list(backend, **overrides):
    kwargs = {
        "project_uid": "proj-uid",
        "environment_uid": "env-uid",
        "start_time": WIDE_START,
        "end_time": WIDE_END,
    }
    kwargs.update(overrides)
    return await backend.list_rca_reports(**kwargs)


@pytest.mark.asyncio
async def test_get_missing_report_returns_none(backend):
    assert await backend.get_rca_report("does-not-exist") is None


@pytest.mark.asyncio
async def test_upsert_pending_then_completed_updates_fields(backend):
    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        status="pending",
        timestamp=_ts(1),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )

    pending = await backend.get_rca_report("r1")
    assert pending["status"] == "pending"
    assert pending["reportId"] == "r1"
    assert pending["alertId"] == "a1"
    assert pending["projectUid"] == "proj-uid"
    assert pending["environmentUid"] == "env-uid"
    assert "report" not in pending

    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        status="completed",
        report={"summary": "root cause found", "result": {"type": "x"}},
        project_uid="proj-uid",
        environment_uid="env-uid",
    )

    completed = await backend.get_rca_report("r1")
    assert completed["status"] == "completed"
    assert completed["summary"] == "root cause found"  # derived from report payload
    assert completed["report"]["summary"] == "root cause found"


@pytest.mark.asyncio
async def test_row_to_doc_surfaces_resource_hierarchy(backend):
    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        timestamp=_ts(1),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    doc = await backend.get_rca_report("r1")
    assert doc["resource"] == {
        "openchoreo.dev/environment-uid": "env-uid",
        "openchoreo.dev/project-uid": "proj-uid",
    }
    assert doc["@timestamp"] == _ts(1).isoformat()


@pytest.mark.asyncio
async def test_completed_upsert_preserves_original_timestamp(backend):
    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        status="pending",
        timestamp=_ts(1),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        status="completed",
        report={"summary": "done"},
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    stored = await backend.get_rca_report("r1")
    assert stored["status"] == "completed"
    assert stored["@timestamp"] == _ts(1).isoformat()


@pytest.mark.asyncio
async def test_list_filters_by_scope_status_and_time(backend):
    await backend.upsert_rca_report(
        report_id="a",
        alert_id="a",
        status="completed",
        timestamp=_ts(1),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    await backend.upsert_rca_report(
        report_id="b",
        alert_id="b",
        status="failed",
        timestamp=_ts(2),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    await backend.upsert_rca_report(
        report_id="c",
        alert_id="c",
        status="completed",
        timestamp=_ts(3),
        project_uid="other-uid",
        environment_uid="env-uid",
    )

    scoped = await _list(backend)
    assert scoped["totalCount"] == 2
    assert {r["reportId"] for r in scoped["reports"]} == {"a", "b"}

    failed = await _list(backend, status="failed")
    assert [r["reportId"] for r in failed["reports"]] == ["b"]

    windowed = await _list(backend, start_time=_ts(2).isoformat())
    assert [r["reportId"] for r in windowed["reports"]] == ["b"]


@pytest.mark.asyncio
async def test_list_end_time_is_inclusive(backend):
    for day in (1, 3, 5):
        await backend.upsert_rca_report(
            report_id=f"r{day}",
            alert_id="a",
            timestamp=_ts(day),
            project_uid="proj-uid",
            environment_uid="env-uid",
        )
    result = await _list(backend, end_time=_ts(3).isoformat())
    assert {r["reportId"] for r in result["reports"]} == {"r1", "r3"}


@pytest.mark.asyncio
async def test_list_respects_limit_but_total_count_is_unbounded(backend):
    for day in range(1, 6):
        await backend.upsert_rca_report(
            report_id=f"r{day}",
            alert_id="a",
            timestamp=_ts(day),
            project_uid="proj-uid",
            environment_uid="env-uid",
        )
    result = await _list(backend, limit=2)
    assert len(result["reports"]) == 2
    assert result["totalCount"] == 5


@pytest.mark.asyncio
async def test_list_sort_order(backend):
    await backend.upsert_rca_report(
        report_id="a",
        alert_id="a",
        timestamp=_ts(1),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    await backend.upsert_rca_report(
        report_id="b",
        alert_id="b",
        timestamp=_ts(2),
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    desc = await _list(backend, sort="desc")
    assert [r["reportId"] for r in desc["reports"]] == ["b", "a"]
    asc = await _list(backend, sort="asc")
    assert [r["reportId"] for r in asc["reports"]] == ["a", "b"]


@pytest.mark.asyncio
async def test_upsert_stores_namespace_and_project(backend):
    await backend.upsert_rca_report(
        report_id="r1",
        alert_id="a1",
        timestamp=_ts(1),
        namespace="ns",
        project="proj",
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    doc = await backend.get_rca_report("r1")
    assert doc["namespace"] == "ns"
    assert doc["project"] == "proj"


@pytest.mark.asyncio
async def test_initialize_upgrades_a_table_from_before_namespace_project(tmp_path):
    backend = await _legacy_backend(
        tmp_path,
        [
            {
                "report_id": "legacy",
                "alert_id": "a1",
                "timestamp": _ts(1).isoformat(),
                "project_uid": "proj-uid",
                "environment_uid": "env-uid",
            }
        ],
    )

    legacy = await backend.get_rca_report("legacy")
    assert legacy["namespace"] is None
    assert legacy["project"] is None
    assert legacy["projectUid"] == "proj-uid"

    await backend.upsert_rca_report(
        report_id="legacy",
        alert_id="a1",
        timestamp=_ts(1),
        namespace="ns",
        project="proj",
        project_uid="proj-uid",
        environment_uid="env-uid",
    )
    assert (await backend.get_rca_report("legacy"))["project"] == "proj"
    await backend.close()
