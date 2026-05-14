# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any, Literal

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import ConfigDict, Field, model_validator

from src.auth import require_authn, require_reports_authz, require_reports_update_authz
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


class ReportUpdateRequest(BaseModel):
    applied_indices: list[int] = Field(default_factory=list, alias="appliedIndices")
    dismissed_indices: list[int] = Field(default_factory=list, alias="dismissedIndices")

    model_config = ConfigDict(populate_by_name=True)

    @model_validator(mode="after")
    def _no_overlap(self) -> "ReportUpdateRequest":
        overlap = set(self.applied_indices) & set(self.dismissed_indices)
        if overlap:
            raise ValueError(
                f"Indices cannot appear in both appliedIndices and dismissedIndices: {sorted(overlap)}"
            )
        return self


@router.put("/{report_id}")
async def update_report(
    report_id: str,
    body: ReportUpdateRequest,
    _auth: Annotated[SubjectContext, Depends(require_authn)] = None,
    _authz: Annotated[SubjectContext, Depends(require_reports_update_authz)] = None,
):
    logger.info(
        "Updating report %s: applied=%s dismissed=%s",
        report_id,
        body.applied_indices,
        body.dismissed_indices,
    )
    await _update_action_statuses(
        report_id,
        applied=set(body.applied_indices),
        dismissed=set(body.dismissed_indices),
    )
    return {"status": "ok"}


async def _update_action_statuses(
    report_id: str,
    applied: set[int],
    dismissed: set[int],
) -> None:
    report_backend = get_report_backend()

    validation_error: HTTPException | None = None

    def mutate(actions: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], bool]:
        nonlocal validation_error
        invalid = sorted(i for i in applied | dismissed if not (0 <= i < len(actions)))
        if invalid:
            if not actions:
                detail = f"No available actions; cannot apply or dismiss indices: {invalid}"
            else:
                detail = f"Invalid action indices (out of range 0–{len(actions) - 1}): {invalid}"
            validation_error = HTTPException(status_code=400, detail=detail)
            return actions, False

        changed = False
        for i, action in enumerate(actions):
            current = action.get("status")
            if i in applied and current == "revised":
                action["status"] = "applied"
                changed = True
            elif i in dismissed and current == "revised":
                action["status"] = "dismissed"
                changed = True
        return actions, changed

    stored = await report_backend.update_report_actions_atomic(report_id, mutate)
    if stored is None:
        raise HTTPException(status_code=404, detail="Report not found")
    if validation_error:
        raise validation_error
