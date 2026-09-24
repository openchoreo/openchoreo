# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest
import pytest_asyncio
from sqlalchemy import text
from sqlalchemy.ext.asyncio import create_async_engine

from src.clients.backend.sql_backend import metadata, rca_reports
from src.report_migration import _list_all, _project_names, migrate_rca_reports


@pytest_asyncio.fixture
async def engine(tmp_path):
    engine = create_async_engine(f"sqlite+aiosqlite:///{tmp_path}/rca.db")
    async with engine.begin() as conn:
        await conn.run_sync(metadata.create_all)
    yield engine
    await engine.dispose()


async def _insert(engine, report_id: str, project_uid: str | None) -> None:
    async with engine.begin() as conn:
        await conn.execute(
            text(
                "INSERT INTO rca_reports (report_id, alert_id, status, timestamp, "
                "environment_uid, project_uid) VALUES (:report_id, 'a1', 'completed', "
                "'2026-06-01T00:00:00+00:00', 'env-uid', :project_uid)"
            ),
            {"report_id": report_id, "project_uid": project_uid},
        )


async def _names(engine, report_id: str) -> tuple[str | None, str | None]:
    async with engine.connect() as conn:
        result = await conn.execute(
            text("SELECT namespace, project FROM rca_reports WHERE report_id = :report_id"),
            {"report_id": report_id},
        )
        return result.fetchone()


def _namespace_page(*names: str, cursor: str | None = None) -> dict:
    return {
        "items": [{"metadata": {"name": name}} for name in names],
        "pagination": {"nextCursor": cursor} if cursor else {},
    }


def _project_page(*projects: tuple[str, str]) -> dict:
    return {
        "items": [{"metadata": {"name": name, "uid": uid}} for name, uid in projects],
        "pagination": {},
    }


@pytest.mark.asyncio
async def test_names_legacy_rows_from_the_control_plane(engine):
    await _insert(engine, "legacy-1", "uid-1")
    await _insert(engine, "legacy-2", "uid-1")
    await _insert(engine, "legacy-3", "uid-2")
    get_mock = AsyncMock(
        side_effect=[
            _namespace_page("team-a"),
            _project_page(("greeter", "uid-1"), ("webapp", "uid-2")),
        ]
    )

    with (
        patch("src.report_migration.get", get_mock),
        patch("src.report_migration.get_oauth2_auth", return_value=object()),
    ):
        await migrate_rca_reports(engine, rca_reports)

    assert await _names(engine, "legacy-1") == ("team-a", "greeter")
    assert await _names(engine, "legacy-2") == ("team-a", "greeter")
    assert await _names(engine, "legacy-3") == ("team-a", "webapp")
    assert get_mock.call_args_list[0].args[0] == "/namespaces"
    assert get_mock.call_args_list[1].args[0] == "/namespaces/team-a/projects"


@pytest.mark.asyncio
async def test_leaves_rows_for_a_deleted_project_alone(engine):
    await _insert(engine, "legacy-1", "uid-1")
    await _insert(engine, "legacy-2", "uid-gone")
    get_mock = AsyncMock(
        side_effect=[
            _namespace_page("team-a"),
            _project_page(("greeter", "uid-1")),
        ]
    )

    with (
        patch("src.report_migration.get", get_mock),
        patch("src.report_migration.get_oauth2_auth", return_value=object()),
    ):
        await migrate_rca_reports(engine, rca_reports)

    assert await _names(engine, "legacy-1") == ("team-a", "greeter")
    assert await _names(engine, "legacy-2") == (None, None)


@pytest.mark.asyncio
async def test_only_fills_the_half_of_a_scope_that_is_missing(engine):
    await _insert(engine, "half-named", "uid-1")
    async with engine.begin() as conn:
        await conn.execute(
            text(
                "UPDATE rca_reports SET namespace = 'recorded-at-write-time' "
                "WHERE report_id = 'half-named'"
            )
        )
    get_mock = AsyncMock(
        side_effect=[
            _namespace_page("team-a"),
            _project_page(("greeter", "uid-1")),
        ]
    )

    with (
        patch("src.report_migration.get", get_mock),
        patch("src.report_migration.get_oauth2_auth", return_value=object()),
    ):
        await migrate_rca_reports(engine, rca_reports)

    assert await _names(engine, "half-named") == ("recorded-at-write-time", "greeter")


@pytest.mark.asyncio
async def test_does_not_touch_the_control_plane_when_every_row_is_named(engine):
    await _insert(engine, "current", "uid-1")
    async with engine.begin() as conn:
        await conn.execute(text("UPDATE rca_reports SET namespace = 'team-a', project = 'greeter'"))
    get_mock = MagicMock()

    with patch("src.report_migration.get", get_mock):
        await migrate_rca_reports(engine, rca_reports)

    get_mock.assert_not_called()
    assert await _names(engine, "current") == ("team-a", "greeter")


@pytest.mark.asyncio
async def test_does_not_touch_the_control_plane_for_rows_without_a_project_uid(engine):
    await _insert(engine, "manual", None)
    get_mock = MagicMock()

    with patch("src.report_migration.get", get_mock):
        await migrate_rca_reports(engine, rca_reports)

    get_mock.assert_not_called()


@pytest.mark.asyncio
async def test_keeps_serving_when_the_control_plane_is_unreachable(engine):
    await _insert(engine, "legacy", "uid-1")
    get_mock = AsyncMock(side_effect=httpx.ConnectError("connection refused"))

    with (
        patch("src.report_migration.get", get_mock),
        patch("src.report_migration.get_oauth2_auth", return_value=object()),
    ):
        await migrate_rca_reports(engine, rca_reports)

    assert await _names(engine, "legacy") == (None, None)


@pytest.mark.asyncio
async def test_adds_missing_columns_once(tmp_path):
    engine = create_async_engine(f"sqlite+aiosqlite:///{tmp_path}/partial.db")
    async with engine.begin() as conn:
        await conn.execute(
            text(
                "CREATE TABLE rca_reports (report_id VARCHAR PRIMARY KEY, "
                "alert_id VARCHAR NOT NULL, status VARCHAR NOT NULL, timestamp VARCHAR NOT NULL)"
            )
        )

    await migrate_rca_reports(engine, rca_reports)
    await migrate_rca_reports(engine, rca_reports)

    async with engine.connect() as conn:
        result = await conn.execute(text("SELECT name FROM pragma_table_info('rca_reports')"))
        assert {row[0] for row in result.fetchall()} == {c.name for c in rca_reports.columns}
    await engine.dispose()


@pytest.mark.asyncio
async def test_project_names_pages_and_stops_once_uids_are_found():
    get_mock = AsyncMock(
        side_effect=[
            _namespace_page("team-a", cursor="next"),
            _namespace_page("team-b"),
            _project_page(("greeter", "uid-1")),
        ]
    )
    with (
        patch("src.report_migration.get", get_mock),
        patch("src.report_migration.get_oauth2_auth", return_value=object()),
    ):
        names = await _project_names({"uid-1"})

    assert names == {"uid-1": ("team-a", "greeter")}
    assert [call.args[0] for call in get_mock.call_args_list] == [
        "/namespaces",
        "/namespaces",
        "/namespaces/team-a/projects",
    ]
    assert get_mock.call_args_list[1].args[2] == {"limit": "100", "cursor": "next"}


@pytest.mark.asyncio
async def test_stops_paging_when_the_control_plane_repeats_a_cursor():
    get_mock = AsyncMock(return_value=_namespace_page("team-a", cursor="stuck"))

    with (
        patch("src.report_migration.get", get_mock),
        pytest.raises(RuntimeError, match="pagination cursor repeated"),
    ):
        await _list_all("/namespaces", object())

    assert get_mock.await_count == 2
