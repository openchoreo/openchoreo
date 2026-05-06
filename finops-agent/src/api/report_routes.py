# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any, Literal

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import ConfigDict, Field

from src.auth import require_authn, require_reports_authz
from src.auth.authz_models import SubjectContext
from src.clients import get_report_backend
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1/reports", tags=["FinOps Reports"])


class FinOpsReportSummary(BaseModel):
    report_id: str = Field(alias="reportId")
    namespace: str
    project: str
    environment: str | None = None
    component: str | None = None
    timestamp: str
    summary: str | None = None
    status: str

    model_config = ConfigDict(populate_by_name=True)


class FinOpsReportsResponse(BaseModel):
    reports: list[FinOpsReportSummary]
    total_count: int = Field(alias="totalCount")

    model_config = ConfigDict(populate_by_name=True)


class FinOpsReportDetailed(BaseModel):
    report_id: str = Field(alias="reportId")
    namespace: str
    project: str
    environment: str | None = None
    component: str | None = None
    timestamp: str
    status: str
    report: dict[str, Any] | None = None

    model_config = ConfigDict(populate_by_name=True)


@router.get(
    "",
    response_model=FinOpsReportsResponse,
    response_model_by_alias=True,
)
async def list_finops_reports(
    namespace: str,
    project: str,
    component: str | None = None,
    start_time: Annotated[str | None, Query(alias="startTime")] = None,
    end_time: Annotated[str | None, Query(alias="endTime")] = None,
    limit: Annotated[int, Query(ge=1, le=10000)] = 100,
    sort: Literal["asc", "desc"] = "desc",
    status: Literal["pending", "completed", "failed"] | None = None,
    _auth: Annotated[SubjectContext, Depends(require_authn)] = None,
    _authz: Annotated[SubjectContext, Depends(require_reports_authz)] = None,
):
    report_backend = get_report_backend()
    result = await report_backend.list_reports(
        namespace=namespace,
        project=project,
        component=component,
        start_time=start_time,
        end_time=end_time,
        status=status,
        limit=limit,
        sort=sort,
    )

    return FinOpsReportsResponse(
        reports=[FinOpsReportSummary(**r) for r in result["reports"]],
        totalCount=result["totalCount"],
    )


@router.get(
    "/{report_id}",
    response_model=FinOpsReportDetailed,
    response_model_by_alias=True,
)
async def get_finops_report(
    report_id: str,
    _auth: Annotated[SubjectContext, Depends(require_authn)] = None,
    _authz: Annotated[SubjectContext, Depends(require_reports_authz)] = None,
):
    report_backend = get_report_backend()
    result = await report_backend.get_report(report_id)

    if not result:
        raise HTTPException(status_code=404, detail="Report not found")

    return FinOpsReportDetailed(
        reportId=result["reportId"],
        namespace=result["namespace"],
        project=result["project"],
        environment=result.get("environment"),
        component=result["component"],
        timestamp=result["@timestamp"],
        status=result["status"],
        report=result.get("report"),
    )
