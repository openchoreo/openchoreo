# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Integration-style tests for ``SQLReportBackend`` against a real SQLite DB.

A temp-file database (not ``:memory:``) is used so the schema persists across
the separate connections that ``initialize`` / ``get`` / ``list`` open.
"""

from datetime import UTC, datetime

import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import create_async_engine

from src.clients.backend.sql_backend import SQLReportBackend


@pytest_asyncio.fixture
async def backend(tmp_path):
    engine = create_async_engine(f"sqlite+aiosqlite:///{tmp_path}/finops.db")
    be = SQLReportBackend(engine)
    await be.initialize()
    yield be
    await be.close()


def _ts(day: int) -> datetime:
    return datetime(2026, 6, day, tzinfo=UTC)


@pytest.mark.asyncio
async def test_get_missing_report_returns_none(backend):
    assert await backend.get_report("does-not-exist") is None


@pytest.mark.asyncio
async def test_upsert_pending_then_completed_updates_fields(backend):
    await backend.upsert_report(
        report_id="r1",
        status="pending",
        timestamp=_ts(1),
        namespace="ns",
        project="proj",
        component="comp",
        environment="dev",
    )

    pending = await backend.get_report("r1")
    assert pending["status"] == "pending"
    assert pending["reportId"] == "r1"
    assert pending["namespace"] == "ns"
    assert pending["component"] == "comp"
    assert pending["environment"] == "dev"
    assert "report" not in pending  # no report payload yet

    await backend.upsert_report(
        report_id="r1",
        status="completed",
        report={"summary": "all good", "recommended_actions": []},
        namespace="ns",
        project="proj",
        component="comp",
        environment="dev",
    )

    completed = await backend.get_report("r1")
    assert completed["status"] == "completed"
    assert completed["summary"] == "all good"  # summary derived from report payload
    assert completed["report"]["summary"] == "all good"


@pytest.mark.asyncio
async def test_list_filters_by_scope_status_and_time(backend):
    await backend.upsert_report(
        report_id="a",
        status="completed",
        timestamp=_ts(1),
        namespace="ns",
        project="proj",
        component="comp-1",
    )
    await backend.upsert_report(
        report_id="b",
        status="failed",
        timestamp=_ts(2),
        namespace="ns",
        project="proj",
        component="comp-2",
    )
    await backend.upsert_report(
        report_id="c",
        status="completed",
        timestamp=_ts(3),
        namespace="other",
        project="proj",
        component="comp-1",
    )

    # namespace/project scope excludes the "other" namespace.
    result = await backend.list_reports(namespace="ns", project="proj")
    assert result["totalCount"] == 2
    assert {r["reportId"] for r in result["reports"]} == {"a", "b"}

    # component filter.
    by_comp = await backend.list_reports(namespace="ns", project="proj", component="comp-1")
    assert [r["reportId"] for r in by_comp["reports"]] == ["a"]

    # status filter.
    failed = await backend.list_reports(namespace="ns", project="proj", status="failed")
    assert [r["reportId"] for r in failed["reports"]] == ["b"]

    # time-range filter (start inclusive).
    windowed = await backend.list_reports(
        namespace="ns", project="proj", start_time=_ts(2).isoformat()
    )
    assert [r["reportId"] for r in windowed["reports"]] == ["b"]


@pytest.mark.asyncio
async def test_list_respects_limit_but_total_count_is_unbounded(backend):
    for day in range(1, 6):
        await backend.upsert_report(
            report_id=f"r{day}", timestamp=_ts(day), namespace="ns", project="proj"
        )

    result = await backend.list_reports(namespace="ns", project="proj", limit=2)

    assert len(result["reports"]) == 2
    # totalCount counts all matches, independent of the page limit.
    assert result["totalCount"] == 5


@pytest.mark.asyncio
async def test_list_end_time_filter_is_inclusive_upper_bound(backend):
    await backend.upsert_report(report_id="a", timestamp=_ts(1), namespace="ns", project="proj")
    await backend.upsert_report(report_id="b", timestamp=_ts(3), namespace="ns", project="proj")
    await backend.upsert_report(report_id="c", timestamp=_ts(5), namespace="ns", project="proj")

    result = await backend.list_reports(namespace="ns", project="proj", end_time=_ts(3).isoformat())

    assert {r["reportId"] for r in result["reports"]} == {"a", "b"}


@pytest.mark.asyncio
async def test_completed_upsert_preserves_original_timestamp(backend):
    # Pending row carries an explicit timestamp; the later "completed" upsert
    # supplies none, so the update path (which omits timestamp) must keep the
    # original rather than overwriting it with "now".
    await backend.upsert_report(
        report_id="r1", status="pending", timestamp=_ts(1), namespace="ns", project="proj"
    )

    await backend.upsert_report(
        report_id="r1",
        status="completed",
        report={"summary": "done"},
        namespace="ns",
        project="proj",
    )

    stored = await backend.get_report("r1")
    assert stored["status"] == "completed"
    assert stored["@timestamp"] == _ts(1).isoformat()


@pytest.mark.asyncio
async def test_list_sort_order(backend):
    await backend.upsert_report(report_id="a", timestamp=_ts(1), namespace="ns", project="proj")
    await backend.upsert_report(report_id="b", timestamp=_ts(2), namespace="ns", project="proj")

    desc = await backend.list_reports(namespace="ns", project="proj", sort="desc")
    assert [r["reportId"] for r in desc["reports"]] == ["b", "a"]

    asc = await backend.list_reports(namespace="ns", project="proj", sort="asc")
    assert [r["reportId"] for r in asc["reports"]] == ["a", "b"]


@pytest.mark.asyncio
async def test_update_actions_atomic_missing_report_returns_none(backend):
    result = await backend.update_report_actions_atomic("nope", lambda actions: (actions, True))
    assert result is None


@pytest.mark.asyncio
async def test_update_actions_atomic_mutates_only_recommended_actions(backend):
    await backend.upsert_report(
        report_id="r1",
        status="completed",
        report={
            "summary": "keep me",
            "actual_cost": {"total_cost": 9.0},
            "recommended_actions": [{"status": "revised", "description": "x"}],
        },
        namespace="ns",
        project="proj",
    )

    def apply_first(actions):
        actions[0]["status"] = "applied"
        return actions, True

    returned = await backend.update_report_actions_atomic("r1", apply_first)
    assert returned is not None

    stored = await backend.get_report("r1")
    # Only recommended_actions changed; sibling report fields are preserved.
    assert stored["report"]["recommended_actions"][0]["status"] == "applied"
    assert stored["report"]["summary"] == "keep me"
    assert stored["report"]["actual_cost"] == {"total_cost": 9.0}


@pytest.mark.asyncio
async def test_update_actions_atomic_noop_does_not_rewrite(backend):
    original_actions = [{"status": "revised", "description": "x"}]
    await backend.upsert_report(
        report_id="r1",
        status="completed",
        report={"summary": "keep me", "recommended_actions": original_actions},
        namespace="ns",
        project="proj",
    )

    def noop(actions):
        return actions, False  # signal "no change"

    await backend.update_report_actions_atomic("r1", noop)

    stored = await backend.get_report("r1")
    assert stored["report"]["recommended_actions"] == original_actions
