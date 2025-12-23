# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Any
from uuid import UUID

from fastapi import APIRouter, HTTPException
from langchain_core.callbacks import UsageMetadataCallbackHandler
from pydantic import Field

from src.core.agent import create_rca_agent
from src.core.llm import get_model
from src.core.mcp import MCPClient
from src.core.opensearch import get_opensearch_client
from src.core.template_manager import render
from src.core.utils import BaseModel, get_current_utc

logger = logging.getLogger(__name__)

router = APIRouter()


class AnalyzeRequest(BaseModel):
    rule_name: str = Field(alias="ruleName")
    component_uid: UUID = Field(alias="componentUid")
    project_uid: UUID = Field(alias="projectUid")
    environment_uid: UUID = Field(alias="environmentUid")
    alert_value: int = Field(alias="alertValue")
    timestamp: str
    alert_id: str = Field(alias="alertId")
    meta: dict[str, Any] | None = None


@router.get("/health")
async def health():
    try:
        opensearch_client = get_opensearch_client()
        if not opensearch_client.check_connection():
            raise Exception("OpenSearch connection check failed")

        mcp_client = MCPClient()
        tools = await mcp_client.get_tools()
        logger.debug("MCP health check successful: loaded %d tools", len(tools))

        return {"status": "healthy"}
    except Exception as e:
        logger.error("Health check failed: %s", e)
        raise HTTPException(status_code=503, detail={"status": "unhealthy", "error": str(e)})


@router.post("/analyze")
async def analyze(request: AnalyzeRequest):
    timestamp = int(get_current_utc().timestamp())
    report_id = f"{request.alert_id}_{timestamp}"
    opensearch_client = get_opensearch_client()

    try:
        # Create initial pending record
        opensearch_client.upsert_rca_report(
            report_id=report_id,
            alert_id=request.alert_id,
            status="pending",
            environment_uid=str(request.environment_uid),
            project_uid=str(request.project_uid),
            component_uids=[str(request.component_uid)],
        )
        logger.info("Created pending RCA report: report_id=%s", report_id)

        usage_callback = UsageMetadataCallbackHandler()
        model = get_model()
        agent = await create_rca_agent(model, usage_callback=usage_callback)

        # TODO: Preprocessing step to resolve id's etc.

        content = render(
            "api/rca_request.j2",
            {
                "rule_name": request.rule_name,
                "component_uid": request.component_uid,
                "project_uid": request.project_uid,
                "environment_uid": request.environment_uid,
                "alert_value": request.alert_value,
                "timestamp": request.timestamp,
                "alert_id": request.alert_id,
                "meta": request.meta,
            },
        )

        result = await agent.ainvoke(
            {
                "messages": [
                    {
                        "role": "user",
                        "content": content,
                    }
                ],
            }
        )

        logger.info("Analysis completed. Usage metadata: %s", usage_callback.usage_metadata)

        rca_report = result["structured_response"]

        # Update with completed report
        try:
            response = opensearch_client.upsert_rca_report(
                report_id=report_id,
                alert_id=request.alert_id,
                status="completed",
                report=rca_report,
                environment_uid=str(request.environment_uid),
                project_uid=str(request.project_uid),
                component_uids=[str(request.component_uid)],
            )
            logger.info(
                "Updated RCA report to completed: index=%s, report_id=%s, status=%s",
                response.get("_index"),
                report_id,
                response.get("result"),
            )
        except Exception as e:
            logger.error("Failed to update RCA report to OpenSearch: %s", e, exc_info=True)

        return {"result": rca_report, "report_id": report_id}
    except Exception as e:
        logger.error("Analysis failed: %s", e, exc_info=True)

        # Update status to failed
        try:
            opensearch_client.upsert_rca_report(
                report_id=report_id,
                alert_id=request.alert_id,
                status="failed",
                environment_uid=str(request.environment_uid),
                project_uid=str(request.project_uid),
                component_uids=[str(request.component_uid)],
            )
            logger.info("Updated RCA report status to failed: report_id=%s", report_id)
        except Exception as update_error:
            logger.error(
                "Failed to update failed status to OpenSearch: %s", update_error, exc_info=True
            )

        raise HTTPException(status_code=500, detail=f"Analysis failed: {str(e)}") from e
