# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import json
import logging
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from sqlalchemy import Column, Index, MetaData, String, Table, Text, func, select, text
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.dialects.sqlite import insert as sqlite_insert
from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine

from src.clients.backend.report_backend import ReportBackend
from src.config import settings

logger = logging.getLogger(__name__)

metadata = MetaData()

finops_reports = Table(
    "finops_reports",
    metadata,
    Column("report_id", String, primary_key=True),
    Column("status", String, nullable=False, server_default="pending"),
    Column("summary", Text, nullable=True),
    Column("timestamp", String, nullable=False),
    Column("namespace", String, nullable=True),
    Column("project", String, nullable=True),
    Column("component", String, nullable=True),
    Column("environment", String, nullable=True),
    Column("report", Text, nullable=True),
    Index("idx_namespace_project_component", "namespace", "project", "component"),
    Index("idx_timestamp", "timestamp"),
    Index("idx_status", "status"),
)

# Module-level singleton
_client: SQLReportBackend | None = None


class SQLReportBackend(ReportBackend):
    def __init__(self, engine: AsyncEngine) -> None:
        self.engine = engine
        self._is_sqlite = engine.dialect.name == "sqlite"

    async def initialize(self) -> None:
        if self._is_sqlite:
            db_path = str(self.engine.url.database or "")
            if db_path:
                Path(db_path).parent.mkdir(parents=True, exist_ok=True)
        async with self.engine.begin() as conn:
            if self._is_sqlite:
                await conn.execute(text("PRAGMA journal_mode=WAL"))
            await conn.run_sync(metadata.create_all)
            await self._migrate_add_missing_columns(conn)
        async with self.engine.connect() as conn:
            await conn.execute(text("SELECT 1"))
        logger.info("SQL report backend initialized")

    async def _migrate_add_missing_columns(self, conn: Any) -> None:
        """Add any columns present in the schema but missing from the existing table."""
        if self._is_sqlite:
            result = await conn.execute(text("PRAGMA table_info(finops_reports)"))
            existing_cols = {row[1] for row in result.fetchall()}
        else:
            result = await conn.execute(
                text(
                    "SELECT column_name FROM information_schema.columns "
                    "WHERE table_name = 'finops_reports' AND table_schema = current_schema()"
                )
            )
            existing_cols = {row[0] for row in result.fetchall()}

        for col in finops_reports.columns:
            if col.name not in existing_cols:
                col_type = col.type.compile(dialect=self.engine.dialect)
                await conn.execute(
                    text(f"ALTER TABLE finops_reports ADD COLUMN {col.name} {col_type}")
                )
                logger.info(f"Added missing column '{col.name}' to finops_reports table")

    async def upsert_report(
        self,
        report_id: str,
        status: str = "pending",
        report: dict[str, Any] | None = None,
        summary: str | None = None,
        timestamp: datetime | None = None,
        namespace: str | None = None,
        project: str | None = None,
        component: str | None = None,
        environment: str | None = None,
    ) -> dict[str, Any]:
        doc_timestamp = timestamp or datetime.now(UTC)
        ts_str = doc_timestamp.isoformat()

        report_json: str | None = None
        if report is not None:
            report_json = json.dumps(report)
            summary = report.get("summary", summary)

        values = {
            "report_id": report_id,
            "status": status,
            "summary": summary,
            "timestamp": ts_str,
            "namespace": namespace,
            "project": project,
            "component": component,
            "environment": environment,
            "report": report_json,
        }

        update_values: dict[str, Any] = {
            "status": status,
            "summary": summary,
            "namespace": namespace,
            "project": project,
            "component": component,
            "environment": environment,
        }
        if report_json is not None:
            update_values["report"] = report_json

        if self._is_sqlite:
            stmt = sqlite_insert(finops_reports).values(**values)
            stmt = stmt.on_conflict_do_update(
                index_elements=["report_id"],
                set_=update_values,
            )
        else:
            stmt = pg_insert(finops_reports).values(**values)
            stmt = stmt.on_conflict_do_update(
                index_elements=["report_id"],
                set_=update_values,
            )

        async with self.engine.begin() as conn:
            await conn.execute(stmt)

        logger.info(f"Successfully upserted FinOps report {report_id} with status={status}")
        return {"result": "created", "_id": report_id}

    async def get_report(
        self,
        report_id: str,
    ) -> dict[str, Any] | None:
        stmt = select(finops_reports).where(finops_reports.c.report_id == report_id)
        async with self.engine.connect() as conn:
            row = (await conn.execute(stmt)).mappings().fetchone()
        if row is None:
            return None
        return _row_to_doc(row)

    async def list_reports(
        self,
        namespace: str,
        project: str,
        component: str | None = None,
        start_time: str | None = None,
        end_time: str | None = None,
        status: str | None = None,
        limit: int = 100,
        sort: str = "desc",
    ) -> dict[str, Any]:
        conditions = [
            finops_reports.c.namespace == namespace,
            finops_reports.c.project == project,
        ]
        if component is not None:
            conditions.append(finops_reports.c.component == component)
        if start_time is not None:
            conditions.append(finops_reports.c.timestamp >= start_time)
        if end_time is not None:
            conditions.append(finops_reports.c.timestamp <= end_time)
        if status is not None:
            conditions.append(finops_reports.c.status == status)

        count_stmt = select(func.count()).select_from(finops_reports).where(*conditions)
        order_col = (
            finops_reports.c.timestamp.desc()
            if sort == "desc"
            else finops_reports.c.timestamp.asc()
        )
        query_stmt = select(finops_reports).where(*conditions).order_by(order_col).limit(limit)

        async with self.engine.connect() as conn:
            total_count = (await conn.execute(count_stmt)).scalar() or 0
            rows = (await conn.execute(query_stmt)).mappings().fetchall()

        reports = [
            {
                "reportId": row["report_id"],
                "namespace": row["namespace"],
                "project": row["project"],
                "component": row["component"],
                "environment": row["environment"],
                "timestamp": row["timestamp"],
                "summary": row["summary"],
                "status": row["status"],
            }
            for row in rows
        ]

        return {
            "reports": reports,
            "totalCount": total_count,
        }

    async def close(self) -> None:
        global _client
        await self.engine.dispose()
        _client = None


def _row_to_doc(row: Any) -> dict[str, Any]:
    doc: dict[str, Any] = {
        "@timestamp": row["timestamp"],
        "reportId": row["report_id"],
        "namespace": row["namespace"],
        "project": row["project"],
        "component": row["component"],
        "environment": row["environment"],
        "status": row["status"],
        "summary": row["summary"],
    }
    if row["report"]:
        doc["report"] = json.loads(row["report"])
    return doc


def get_sql_backend() -> SQLReportBackend:
    global _client
    if _client is None:
        engine = create_async_engine(settings.sql_backend_uri)
        _client = SQLReportBackend(engine)
    return _client
