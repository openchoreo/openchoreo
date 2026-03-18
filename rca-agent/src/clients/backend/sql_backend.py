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

rca_reports = Table(
    "rca_reports",
    metadata,
    Column("report_id", String, primary_key=True),
    Column("alert_id", String, nullable=False),
    Column("status", String, nullable=False, server_default="pending"),
    Column("summary", Text, nullable=True),
    Column("timestamp", String, nullable=False),
    Column("environment_uid", String, nullable=True),
    Column("project_uid", String, nullable=True),
    Column("report", Text, nullable=True),
    Index("idx_alert_id", "alert_id"),
    Index("idx_project_env", "project_uid", "environment_uid"),
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
        async with self.engine.connect() as conn:
            await conn.execute(text("SELECT 1"))
        logger.info("SQL report backend initialized")

    async def upsert_rca_report(
        self,
        report_id: str,
        alert_id: str,
        status: str = "pending",
        report: dict[str, Any] | None = None,
        summary: str | None = None,
        timestamp: datetime | None = None,
        environment_uid: str | None = None,
        project_uid: str | None = None,
    ) -> dict[str, Any]:
        doc_timestamp = timestamp or datetime.now(UTC)
        ts_str = doc_timestamp.isoformat()

        report_json: str | None = None
        if report is not None:
            report_json = json.dumps(report)
            summary = report.get("summary", summary)

        values = {
            "report_id": report_id,
            "alert_id": alert_id,
            "status": status,
            "summary": summary,
            "timestamp": ts_str,
            "environment_uid": environment_uid,
            "project_uid": project_uid,
            "report": report_json,
        }

        update_values: dict[str, Any] = {
            "status": status,
            "summary": summary,
            "environment_uid": environment_uid,
            "project_uid": project_uid,
        }
        if report_json is not None:
            update_values["report"] = report_json

        if self._is_sqlite:
            stmt = sqlite_insert(rca_reports).values(**values)
            stmt = stmt.on_conflict_do_update(
                index_elements=["report_id"],
                set_=update_values,
            )
        else:
            stmt = pg_insert(rca_reports).values(**values)
            stmt = stmt.on_conflict_do_update(
                index_elements=["report_id"],
                set_=update_values,
            )

        async with self.engine.begin() as conn:
            await conn.execute(stmt)

        logger.info(f"Successfully upserted RCA report {report_id} with status={status}")
        return {"result": "created", "_id": report_id}

    async def get_rca_report(
        self,
        report_id: str,
    ) -> dict[str, Any] | None:
        stmt = select(rca_reports).where(rca_reports.c.report_id == report_id)
        async with self.engine.connect() as conn:
            row = (await conn.execute(stmt)).mappings().fetchone()
        if row is None:
            return None
        return _row_to_doc(row)

    async def list_rca_reports(
        self,
        project_uid: str,
        environment_uid: str,
        start_time: str,
        end_time: str,
        status: str | None = None,
        limit: int = 100,
        sort: str = "desc",
    ) -> dict[str, Any]:
        conditions = [
            rca_reports.c.project_uid == project_uid,
            rca_reports.c.environment_uid == environment_uid,
            rca_reports.c.timestamp >= start_time,
            rca_reports.c.timestamp <= end_time,
        ]
        if status is not None:
            conditions.append(rca_reports.c.status == status)

        count_stmt = select(func.count()).select_from(rca_reports).where(*conditions)
        order_col = (
            rca_reports.c.timestamp.desc() if sort == "desc" else rca_reports.c.timestamp.asc()
        )
        query_stmt = select(rca_reports).where(*conditions).order_by(order_col).limit(limit)

        async with self.engine.connect() as conn:
            total_count = (await conn.execute(count_stmt)).scalar() or 0
            rows = (await conn.execute(query_stmt)).mappings().fetchall()

        reports = [
            {
                "alertId": row["alert_id"],
                "projectUid": row["project_uid"],
                "reportId": row["report_id"],
                "timestamp": row["timestamp"],
                "summary": row["summary"],
                "status": row["status"],
            }
            for row in rows
        ]

        return {
            "reports": reports,
            "totalCount": total_count,
            "tookMs": 0,
        }

    async def close(self) -> None:
        await self.engine.dispose()


def _row_to_doc(row: Any) -> dict[str, Any]:
    doc: dict[str, Any] = {
        "@timestamp": row["timestamp"],
        "reportId": row["report_id"],
        "alertId": row["alert_id"],
        "status": row["status"],
        "summary": row["summary"],
        "resource": {
            "openchoreo.dev/environment-uid": row["environment_uid"],
            "openchoreo.dev/project-uid": row["project_uid"],
        },
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
