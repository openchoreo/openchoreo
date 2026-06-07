# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from fastapi import APIRouter, BackgroundTasks, HTTPException
from pydantic import ConfigDict, Field

from src.agent import run_analysis
from src.clients import get_report_backend
from src.models import BaseModel, get_current_utc

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1", tags=["FinOps Agent"])


class SearchScope(BaseModel):
    component: str
    namespace: str
    project: str
    environment: str


class BudgetedCost(BaseModel):
    amount: float
    period: str  # e.g. "5d"
    currency: str  # e.g. "USD"


class ActualCost(BaseModel):
    amount: float
    currency: str


class AnalyzeRequest(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    search_scope: SearchScope = Field(..., alias="searchScope")
    budgeted_cost: BudgetedCost = Field(..., alias="budgetedCost")
    actual_cost: ActualCost = Field(..., alias="actualCost")
    budget_alert_triggered_at: str = Field(..., alias="budgetAlertTriggeredAt")  # ISO 8601


@router.post("/analyses")
async def analyze(
    request: AnalyzeRequest,
    background_tasks: BackgroundTasks,
):
    if logger.isEnabledFor(logging.DEBUG):
        body = request.model_dump_json(by_alias=True)
        logger.debug("Received analyze request: %s", body)

    timestamp = get_current_utc()
    report_id = f"finops_{request.search_scope.component}_{int(timestamp.timestamp())}_{timestamp.microsecond}"
    report_backend = get_report_backend()

    try:
        await report_backend.upsert_report(
            report_id=report_id,
            status="pending",
            timestamp=timestamp,
            namespace=request.search_scope.namespace,
            project=request.search_scope.project,
            component=request.search_scope.component,
            environment=request.search_scope.environment,
        )
    except Exception as e:
        logger.error("Failed to create FinOps report: %s", type(e).__name__)
        raise HTTPException(
            status_code=500, detail="Internal server error while creating analysis task"
        ) from e

    request_context = {
        "search_scope": request.search_scope.model_dump(),
        "budgeted_cost": request.budgeted_cost.model_dump(),
        "actual_cost": request.actual_cost.model_dump(),
        "budget_alert_triggered_at": request.budget_alert_triggered_at,
    }

    background_tasks.add_task(
        run_analysis,
        report_id=report_id,
        search_scope=request.search_scope,
        request_context=request_context,
    )

    return {"reportId": report_id, "status": "pending"}
