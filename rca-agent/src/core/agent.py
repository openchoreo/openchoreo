# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from typing import Any
from uuid import UUID

from langchain.agents import create_agent
from langchain.agents.middleware import AgentMiddleware, TodoListMiddleware
from langchain.agents.structured_output import ToolStrategy
from langchain_core.callbacks import BaseCallbackHandler, UsageMetadataCallbackHandler
from langchain_core.language_models import BaseChatModel
from langchain_core.runnables import Runnable, RunnableConfig

from src.core.config import settings
from src.core.llm import get_model
from src.core.mcp import MCPClient
from src.core.middleware import LoggingMiddleware, OutputProcessorMiddleware, TimingMiddleware
from src.core.models.rca_report import RCAReport
from src.core.opensearch import get_opensearch_client
from src.core.prompts.system_prompt import get_system_prompt
from src.core.template_manager import render
from src.core.utils import get_semaphore
from src.logging_config import report_id_context

logger = logging.getLogger(__name__)


async def create_rca_agent(
    model: BaseChatModel, usage_callback: BaseCallbackHandler | None = None
) -> Runnable:
    mcp_client = MCPClient()
    tools = await mcp_client.get_tools()

    prompt = get_system_prompt(tools)

    middleware: list[AgentMiddleware] = [
        TimingMiddleware(),
        OutputProcessorMiddleware(),
        TodoListMiddleware(),
    ]

    if logger.isEnabledFor(logging.DEBUG):
        middleware.append(LoggingMiddleware())

    config: RunnableConfig = {"recursion_limit": 200}
    if usage_callback is not None:
        config["callbacks"] = [usage_callback]

    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=prompt,
        response_format=ToolStrategy(RCAReport),
        middleware=middleware,
    ).with_config(config)

    logger.info("Created RCA agent with %d tools: %s", len(tools), [tool.name for tool in tools])

    return agent


async def run_analysis(
    report_id: str,
    alert_id: str,
    alert: Any,
    component_uid: UUID,
    project_uid: UUID,
    environment_uid: UUID,
    meta: dict[str, Any] | None = None,
) -> None:
    # Set report_id in context for logging
    report_id_context.set(report_id)

    semaphore = get_semaphore()
    opensearch_client = get_opensearch_client()

    logger.info("Analysis task queued")

    async with semaphore:
        logger.info("Analysis task started")

        try:
            usage_callback = UsageMetadataCallbackHandler()
            model = get_model()
            agent = await create_rca_agent(model, usage_callback=usage_callback)

            content = render(
                "api/rca_request.j2",
                {
                    "component_uid": component_uid,
                    "project_uid": project_uid,
                    "environment_uid": environment_uid,
                    "alert": alert,
                    "meta": meta,
                },
            )

            result = await asyncio.wait_for(
                agent.ainvoke(
                    {
                        "messages": [
                            {
                                "role": "user",
                                "content": content,
                            }
                        ],
                    }
                ),
                timeout=settings.analysis_timeout_seconds,
            )

            logger.info("Analysis completed: usage=%s", usage_callback.usage_metadata)

            rca_report = result["structured_response"]

            response = await opensearch_client.upsert_rca_report(
                report_id=report_id,
                alert_id=alert_id,
                status="completed",
                report=rca_report,
                environment_uid=str(environment_uid),
                project_uid=str(project_uid),
                component_uids=[str(component_uid)],
            )
            logger.info(
                "Updated RCA report to completed: index=%s, status=%s",
                response.get("_index"),
                response.get("result"),
            )

        except TimeoutError:
            logger.error(
                "Analysis timed out after %d seconds",
                settings.analysis_timeout_seconds,
            )
            await _update_failed_status(
                opensearch_client,
                report_id,
                alert_id,
                environment_uid,
                project_uid,
                component_uid,
                f"Analysis timed out after {settings.analysis_timeout_seconds} seconds",
            )

        except Exception as e:
            logger.error("Analysis failed: error=%s", e, exc_info=True)
            await _update_failed_status(
                opensearch_client,
                report_id,
                alert_id,
                environment_uid,
                project_uid,
                component_uid,
                f"Analysis failed: {str(e)}",
            )


async def _update_failed_status(
    opensearch_client: Any,
    report_id: str,
    alert_id: str,
    environment_uid: UUID,
    project_uid: UUID,
    component_uid: UUID,
    summary: str,
) -> None:
    """Update OpenSearch with failed status."""
    try:
        await opensearch_client.upsert_rca_report(
            report_id=report_id,
            alert_id=alert_id,
            status="failed",
            summary=summary,
            environment_uid=str(environment_uid),
            project_uid=str(project_uid),
            component_uids=[str(component_uid)],
        )
        logger.info("Updated RCA report status to failed")
    except Exception as update_error:
        logger.error(
            "Failed to update failed status to OpenSearch: error=%s",
            update_error,
            exc_info=True,
        )
