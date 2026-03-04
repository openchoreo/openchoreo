# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any

from fastapi import APIRouter, BackgroundTasks, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import Field

from src.agent import run_analysis, stream_chat
from src.auth import require_authn, require_chat_authz
from src.auth.authz_models import SubjectContext
from src.clients import get_report_backend
from src.helpers import resolve_component_scope, resolve_project_scope
from src.models import BaseModel, get_current_utc

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1/rca-agent", tags=["RCA Agent"])


class AlertRuleSource(BaseModel):
    type: str
    query: str | None = None
    metric: str | None = None


class AlertRuleCondition(BaseModel):
    window: str
    interval: str
    operator: str
    threshold: int


class AlertRuleInfo(BaseModel):
    name: str
    description: str | None = None
    severity: str | None = None
    source: AlertRuleSource | None = None
    condition: AlertRuleCondition | None = None


class AlertContext(BaseModel):
    id: str
    value: int | str
    timestamp: str
    rule: AlertRuleInfo


class AnalyzeRequest(BaseModel):
    namespace: str
    project: str
    component: str
    environment: str
    alert: AlertContext
    meta: dict[str, Any] | None = None


class ChatMessage(BaseModel):
    model_config = {"extra": "forbid"}

    role: str
    content: str = Field(max_length=10000)


class ChatRequest(BaseModel):
    report_id: str = Field(alias="reportId")
    namespace: str
    project: str
    environment: str
    messages: list[ChatMessage] = Field(min_length=1, max_length=50)


@router.post("/analyze")
async def analyze(
    request: AnalyzeRequest,
    background_tasks: BackgroundTasks,
):
    if logger.isEnabledFor(logging.DEBUG):
        body = request.model_dump_json(by_alias=True)
        logger.debug("Received analyze request: %s", body)

    scope = await resolve_component_scope(
        namespace=request.namespace,
        project=request.project,
        component=request.component,
        environment=request.environment,
    )

    timestamp = get_current_utc()
    report_id = f"{request.alert.id}_{int(timestamp.timestamp())}"
    report_backend = get_report_backend()

    try:
        await report_backend.upsert_rca_report(
            report_id=report_id,
            alert_id=request.alert.id,
            status="pending",
            timestamp=timestamp,
            environment_uid=scope.environment_uid,
            project_uid=scope.project_uid,
        )
    except Exception as e:
        logger.error("Failed to create RCA report: %s", e, exc_info=True)
        raise HTTPException(
            status_code=500, detail=f"Failed to create analysis task: {str(e)}"
        ) from e

    background_tasks.add_task(
        run_analysis,
        report_id=report_id,
        alert_id=request.alert.id,
        alert=request.alert,
        scope=scope,
        meta=request.meta,
    )

    return {"report_id": report_id, "status": "pending"}


@router.post("/chat")
async def chat(
    request: ChatRequest,
    http_request: Request,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_chat_authz)],
):
    if logger.isEnabledFor(logging.DEBUG):
        body = request.model_dump_json(by_alias=True)
        logger.debug("Received chat request: %s", body)

    token = http_request.state.bearer_token

    # Fetch report context for the chat
    report_backend = get_report_backend()
    report_context = await report_backend.get_rca_report(
        report_id=request.report_id,
    )
    if not report_context:
        raise HTTPException(status_code=404, detail="Report not found")

    scope = await resolve_project_scope(
        namespace=request.namespace,
        project=request.project,
        environment=request.environment,
    )

    return StreamingResponse(
        stream_chat(
            messages=[m.model_dump() for m in request.messages],
            token=token,
            report_context=report_context,
            scope=scope,
        ),
        media_type="application/x-ndjson",
        headers={"Cache-Control": "no-cache"},
    )
