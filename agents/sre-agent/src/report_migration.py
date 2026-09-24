# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Any

import httpx
from sqlalchemy import Table, func, or_, select, text
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine

from src.auth import get_oauth2_auth
from src.clients.openchoreo_api import get

logger = logging.getLogger(__name__)

_PAGE_SIZE = 100


async def migrate_rca_reports(engine: AsyncEngine, table: Table) -> None:
    async with engine.begin() as conn:
        await _add_missing_columns(engine, conn, table)
    try:
        await _name_legacy_rows(engine, table)
    except Exception as e:  # noqa: BLE001
        # Best effort: must not fail startup.
        logger.warning("Could not name legacy RCA reports: %s", e, exc_info=True)


async def _add_missing_columns(engine: AsyncEngine, conn: AsyncConnection, table: Table) -> None:
    if engine.dialect.name == "sqlite":
        result = await conn.execute(text(f"PRAGMA table_info({table.name})"))
        existing = {row[1] for row in result.fetchall()}
    else:
        # Read through the same name resolution the ALTER TABLE below uses.
        result = await conn.execute(
            text(
                "SELECT attname FROM pg_catalog.pg_attribute "
                "WHERE attrelid = to_regclass(:table_name) AND attnum > 0 AND NOT attisdropped"
            ),
            {"table_name": table.name},
        )
        existing = {row[0] for row in result.fetchall()}

    # Several replicas can boot at once; the loser of a column race must not fail.
    if_not_exists = "" if engine.dialect.name == "sqlite" else " IF NOT EXISTS"

    for column in table.columns:
        if column.name in existing:
            continue
        column_type = column.type.compile(dialect=engine.dialect)
        clause = f"ALTER TABLE {table.name} ADD COLUMN{if_not_exists} {column.name} {column_type}"
        await conn.execute(text(clause))
        logger.info("Added missing column '%s' to %s", column.name, table.name)


async def _name_legacy_rows(engine: AsyncEngine, table: Table) -> None:
    unnamed = (
        table.c.project_uid.is_not(None),
        or_(table.c.namespace.is_(None), table.c.project.is_(None)),
    )

    async with engine.connect() as conn:
        result = await conn.execute(select(table.c.project_uid).where(*unnamed).distinct())
        project_uids = {row[0] for row in result.fetchall()}

    if not project_uids:
        return

    names = await _project_names(project_uids)
    if not names:
        logger.warning("No project found for %d unnamed RCA report scope(s)", len(project_uids))
        return

    named = 0
    async with engine.begin() as conn:
        for project_uid, (namespace, project) in names.items():
            result = await conn.execute(
                table.update()
                .where(table.c.project_uid == project_uid, *unnamed)
                .values(
                    namespace=func.coalesce(table.c.namespace, namespace),
                    project=func.coalesce(table.c.project, project),
                )
            )
            named += result.rowcount
    logger.info("Named namespace/project on %d legacy RCA report(s)", named)


async def _project_names(project_uids: set[str]) -> dict[str, tuple[str, str]]:
    auth = get_oauth2_auth()
    names: dict[str, tuple[str, str]] = {}
    for namespace in await _list_all("/namespaces", auth):
        namespace_name = namespace["metadata"]["name"]
        for project_name, project_uid in await _list_projects(auth, namespace_name):
            if project_uid in project_uids:
                names[project_uid] = (namespace_name, project_name)
        if len(names) == len(project_uids):
            break
    return names


async def _list_projects(auth: httpx.Auth, namespace: str) -> list[tuple[str, str]]:
    projects = await _list_all(f"/namespaces/{namespace}/projects", auth)
    return [(p["metadata"]["name"], p["metadata"]["uid"]) for p in projects]


async def _list_all(path: str, auth: httpx.Auth) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    seen: set[str] = set()
    cursor: str | None = None
    while True:
        params = {"limit": str(_PAGE_SIZE)}
        if cursor:
            params["cursor"] = cursor
        body = await get(path, auth, params)
        items.extend(body.get("items") or [])
        cursor = (body.get("pagination") or {}).get("nextCursor")
        if not cursor:
            return items
        if cursor in seen:
            raise RuntimeError(f"pagination cursor repeated for {path}")
        seen.add(cursor)
