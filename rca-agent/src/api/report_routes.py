# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from datetime import datetime
from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import ConfigDict, Field

from src.auth import require_authn, require_reports_authz
from src.auth.authz_models import SubjectContext
from src.clients import get_opensearch_client
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1/rca-reports", tags=["RCA Reports"])


class RCAReportSummary(BaseModel):
    alert_id: str = Field(alias="alertId")
    project_uid: str = Field(alias="projectUid")
    report_id: str = Field(alias="reportId")
    timestamp: str
    summary: str | None = None
    status: str

    model_config = ConfigDict(populate_by_name=True)


class RCAReportsResponse(BaseModel):
    reports: list[RCAReportSummary]
    total_count: int = Field(alias="totalCount")
    took_ms: int = Field(alias="tookMs")

    model_config = ConfigDict(populate_by_name=True)


class RCAReportDetailed(BaseModel):
    alert_id: str = Field(alias="alertId")
    project_uid: str = Field(alias="projectUid")
    report_version: int = Field(alias="reportVersion")
    report_id: str = Field(alias="reportId")
    timestamp: str
    status: str
    available_versions: list[int] = Field(alias="availableVersions")
    report: dict[str, Any] | None = None

    model_config = ConfigDict(populate_by_name=True)


def validate_time_range(start_time: str, end_time: str) -> tuple[datetime, datetime]:
    try:
        start_dt = datetime.fromisoformat(start_time.replace("Z", "+00:00"))
    except ValueError as e:
        raise HTTPException(
            status_code=400,
            detail=f"Invalid startTime format: {e}. Expected RFC3339 format.",
        ) from e

    try:
        end_dt = datetime.fromisoformat(end_time.replace("Z", "+00:00"))
    except ValueError as e:
        raise HTTPException(
            status_code=400,
            detail=f"Invalid endTime format: {e}. Expected RFC3339 format.",
        ) from e

    if start_dt > end_dt:
        raise HTTPException(
            status_code=400,
            detail="startTime must be before endTime",
        )

    return start_dt, end_dt


@router.get(
    "/projects/{project_id}",
    response_model=RCAReportsResponse,
    response_model_by_alias=True,
)
async def get_rca_reports_by_project(
    project_id: UUID,
    environment_uid: Annotated[UUID, Query(alias="environmentUid")],
    start_time: Annotated[str, Query(alias="startTime")],
    end_time: Annotated[str, Query(alias="endTime")],
    _auth: Annotated[SubjectContext | None, Depends(require_authn)],
    _authz: Annotated[SubjectContext | None, Depends(require_reports_authz)],
    component_uids: Annotated[list[UUID] | None, Query(alias="componentUids")] = None,
    status: str | None = None,
    limit: Annotated[int, Query(ge=1, le=10000)] = 100,
):
    validate_time_range(start_time, end_time)

    opensearch_client = get_opensearch_client()
    result = await opensearch_client.get_rca_reports_by_project(
        project_uid=str(project_id),
        environment_uid=str(environment_uid),
        start_time=start_time,
        end_time=end_time,
        component_uids=[str(uid) for uid in component_uids] if component_uids else None,
        status=status,
        limit=limit,
    )

    return RCAReportsResponse(
        reports=[RCAReportSummary(**r) for r in result["reports"]],
        totalCount=result["totalCount"],
        tookMs=result["tookMs"],
    )


@router.get(
    "/alerts/{alert_id}",
    response_model=RCAReportDetailed,
    response_model_by_alias=True,
)
async def get_rca_report_by_alert(
    alert_id: str,
    _auth: Annotated[SubjectContext | None, Depends(require_authn)],
    _authz: Annotated[SubjectContext | None, Depends(require_reports_authz)],
    version: Annotated[int | None, Query(ge=1)] = None,
):
    opensearch_client = get_opensearch_client()
    result = await opensearch_client.get_rca_report_by_alert(alert_id, version)

    if not result:
        raise HTTPException(status_code=404, detail="Report not found")

    return RCAReportDetailed(
        alertId=result["alertId"],
        projectUid=result["projectUid"],
        reportVersion=result["reportVersion"],
        reportId=result["reportId"],
        timestamp=result["timestamp"],
        status=result["status"],
        availableVersions=result["availableVersions"],
        report=result["report"],
    )
