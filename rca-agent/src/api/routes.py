# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, BackgroundTasks, Depends, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import Field

from src.core.agent import run_analysis, stream_chat
from src.core.authn import require_authn
from src.core.authz.dependencies import require_chat_authz
from src.core.authz.models import SubjectContext
from src.core.opensearch import get_opensearch_client
from src.core.utils import BaseModel, get_current_utc

logger = logging.getLogger(__name__)

router = APIRouter()


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
    namespace: str | None = None
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
    component_uid: UUID = Field(alias="componentUid")
    project_uid: UUID = Field(alias="projectUid")
    environment_uid: UUID = Field(alias="environmentUid")
    alert: AlertContext
    meta: dict[str, Any] | None = None


class ChatRequest(BaseModel):
    report_id: str = Field(alias="reportId")
    version: int | None = None
    project_uid: UUID = Field(alias="projectUid")
    environment_uid: UUID = Field(alias="environmentUid")
    component_uid: UUID | None = Field(default=None, alias="componentUid")
    messages: list[dict[str, str]]


@router.get("/health")
async def health():
    try:
        opensearch_client = get_opensearch_client()
        if not await opensearch_client.check_connection():
            raise Exception("OpenSearch connection check failed")

        return {"status": "healthy"}
    except Exception as e:
        logger.error("Health check failed: %s", e)
        raise HTTPException(status_code=503, detail={"status": "unhealthy", "error": str(e)})


@router.post("/analyze")
async def analyze(
    request: AnalyzeRequest,
    background_tasks: BackgroundTasks,
):
    if logger.isEnabledFor(logging.DEBUG):
        body = request.model_dump_json(by_alias=True)
        logger.debug("Received analyze request: %s", body)

    timestamp = int(get_current_utc().timestamp())
    report_id = f"{request.alert.id}_{timestamp}"
    opensearch_client = get_opensearch_client()

    try:
        await opensearch_client.upsert_rca_report(
            report_id=report_id,
            alert_id=request.alert.id,
            status="pending",
            environment_uid=str(request.environment_uid),
            project_uid=str(request.project_uid),
            component_uids=[str(request.component_uid)],
        )
        logger.info("Created pending RCA report: report_id=%s", report_id)
    except Exception as e:
        logger.error("Failed to create pending RCA report: %s", e, exc_info=True)
        raise HTTPException(
            status_code=500, detail=f"Failed to create analysis task: {str(e)}"
        ) from e

    background_tasks.add_task(
        run_analysis,
        report_id,
        request.alert.id,
        request.alert,
        request.component_uid,
        request.project_uid,
        request.environment_uid,
        request.meta,
    )

    return {"report_id": report_id, "status": "pending"}


@router.post("/chat")
async def chat(
    request: ChatRequest,
    auth: Annotated[SubjectContext | None, Depends(require_authn)],
    _authz: Annotated[SubjectContext | None, Depends(require_chat_authz)],
):
    if auth:
        logger.debug(
            "Chat request authenticated: type=%s, claim=%s, values=%s",
            auth.type,
            auth.entitlement_claim,
            auth.entitlement_values,
        )

    if logger.isEnabledFor(logging.DEBUG):
        body = request.model_dump_json(by_alias=True)
        logger.debug("Received chat request: %s", body)

    # Fetch report context for the chat
    opensearch_client = get_opensearch_client()
    report_context = await opensearch_client.get_rca_report(
        report_id=request.report_id,
        version=request.version,
    )

    return StreamingResponse(
        stream_chat(
            messages=request.messages,
            report_context=report_context,
        ),
        media_type="application/x-ndjson",
        headers={"Cache-Control": "no-cache"},
    )
