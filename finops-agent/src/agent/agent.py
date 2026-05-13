# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from collections.abc import Callable
from typing import TYPE_CHECKING, Any
from uuid import UUID

import httpx
from langchain.agents import create_agent
from langchain.agents.structured_output import ProviderStrategy
from langchain_core.callbacks import BaseCallbackHandler, UsageMetadataCallbackHandler
from langchain_core.runnables import Runnable, RunnableConfig
from langchain_core.tools import BaseTool
from pydantic import BaseModel

from src.agent.middleware import (
    LoggingMiddleware,
    ToolErrorHandlerMiddleware,
    ToolResultTruncationMiddleware,
)
from src.auth import get_oauth2_auth
from src.clients import MCPClient, get_model, get_report_backend
from src.config import settings
from src.logging_config import request_id_context
from src.models import FinOpsReport, FieldChange, RemediationAction, ResourceChange
from src.template_manager import render

if TYPE_CHECKING:
    from src.api.agent_routes import SearchScope

logger = logging.getLogger(__name__)


class CloseMCPCallback(BaseCallbackHandler):
    """Callback handler to properly close MCPClient when the agent completes."""

    def __init__(self, mcp_client: "MCPClient") -> None:
        super().__init__()
        self._mcp_client = mcp_client
        self._closed = False

    async def _cleanup(self) -> None:
        """Close the MCP client if not already closed."""
        if not self._closed:
            self._closed = True
            try:
                await self._mcp_client.close()
                logger.debug("MCPClient closed successfully")
            except Exception as e:
                logger.warning("Error closing MCPClient: %s", type(e).__name__)

    async def on_chain_end(
        self,
        outputs: dict[str, Any],
        *,
        run_id: UUID,
        parent_run_id: UUID | None = None,
        **kwargs: Any,
    ) -> None:
        """Called when the chain completes successfully."""
        await self._cleanup()

    async def on_chain_error(
        self,
        error: BaseException,
        *,
        run_id: UUID,
        parent_run_id: UUID | None = None,
        **kwargs: Any,
    ) -> None:
        """Called when the chain encounters an error."""
        await self._cleanup()


class Agent:
    def __init__(
        self,
        *,
        template: str,
        middleware: list[type],
        response_format: type[BaseModel],
        recursion_limit: int,
        tool_factories: list[Callable[..., BaseTool]] | None = None,
        allowed_mcp_tools: set[str] | None = None,
    ):
        self.template = template
        self.response_format = response_format
        self.recursion_limit = recursion_limit
        self.model = get_model()
        self._middleware_classes = middleware
        self._tool_factories = tool_factories or []
        self._allowed_mcp_tools = allowed_mcp_tools

    async def create(
        self,
        auth: httpx.Auth,
        usage_callback: BaseCallbackHandler | None = None,
        context: dict[str, Any] | None = None,
    ) -> tuple[Runnable, LoggingMiddleware | None]:
        tools: list[BaseTool] = []

        mcp_client = MCPClient(auth=auth)
        try:
            all_tools = await mcp_client.get_tools()
            if self._allowed_mcp_tools is not None:
                tools = [t for t in all_tools if t.name in self._allowed_mcp_tools]
            else:
                tools = list(all_tools)
            logger.debug("Loaded %d MCP tools: %s", len(tools), [t.name for t in tools])

            for factory in self._tool_factories:
                tools.append(factory(auth))

            logger.debug("Total tools: %d — %s", len(tools), [t.name for t in tools])

            template_context: dict[str, Any] = {
                "tools": tools,
            }
            if context:
                template_context.update(context)

            middleware = [m() for m in self._middleware_classes]
            logging_mw = next((m for m in middleware if isinstance(m, LoggingMiddleware)), None)

            agent = create_agent(
                model=self.model,
                tools=tools,
                system_prompt=render(self.template, template_context),
                middleware=middleware,
                response_format=ProviderStrategy(self.response_format),
            )

            # Keep mcp_client alive and ensure proper cleanup
            cleanup_callback = CloseMCPCallback(mcp_client)
            callbacks: list[BaseCallbackHandler] = [cleanup_callback]
            if usage_callback is not None:
                callbacks.append(usage_callback)

            runnable_config: RunnableConfig = {
                "recursion_limit": self.recursion_limit,
                "callbacks": callbacks,
            }

            logger.info("Created agent with %d tools: %s", len(tools), [t.name for t in tools])
            return agent.with_config(runnable_config), logging_mw
        except Exception:
            # Ensure MCPClient is properly closed if agent creation fails
            await mcp_client.close()
            raise


FINOPS_AGENT = Agent(
    template="prompts/finops_agent_prompt.j2",
    middleware=[
        LoggingMiddleware,
        ToolErrorHandlerMiddleware,
        ToolResultTruncationMiddleware,
    ],
    response_format=FinOpsReport,
    recursion_limit=10,
    allowed_mcp_tools={
        "query_resource_metrics",
        "get_allocation_costs",
        "get_asset_costs",
        "get_cloud_costs",
        "get_efficiency",
    },
)


def _synthesize_remediation_actions(report: FinOpsReport) -> list[RemediationAction]:
    rec = report.overprovisioning.recommendation
    if not rec or not rec.release_binding:
        return []

    return [
        RemediationAction(
            description=(
                f"Right-size ReleaseBinding `{rec.release_binding}` "
                "CPU and memory requests based on actual usage"
            ),
            rationale=rec.rationale,
            change=ResourceChange(
                release_binding=rec.release_binding,
                fields=[
                    FieldChange(
                        json_pointer="/spec/componentTypeEnvironmentConfigs/resources/requests/cpu",
                        value=rec.cpu_request,
                    ),
                    FieldChange(
                        json_pointer="/spec/componentTypeEnvironmentConfigs/resources/requests/memory",
                        value=rec.memory_request,
                    ),
                ],
            ),
        )
    ]


# Module-level semaphore for limiting concurrent analyses
_semaphore: asyncio.Semaphore | None = None


def _get_semaphore() -> asyncio.Semaphore:
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_analyses)
    return _semaphore


async def run_analysis(
    report_id: str,
    search_scope: "SearchScope",
    request_context: dict[str, Any],
) -> None:
    # Set request_id in context for logging (use report_id as it's unique per request)
    request_id_context.set(report_id)

    semaphore = _get_semaphore()
    report_backend = get_report_backend()

    logger.info("Analysis task queued")

    async with semaphore:
        logger.info("Analysis task started")

        try:
            usage_callback = UsageMetadataCallbackHandler()

            agent, agent_logging = await FINOPS_AGENT.create(
                auth=get_oauth2_auth(),
                usage_callback=usage_callback,
            )

            content = render(
                "api/finops_request.j2",
                request_context,
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

            finops_report: FinOpsReport = result["structured_response"]
            if agent_logging and (summary := agent_logging.tool_call_summary()):
                logger.debug("FinOps tool calls: %s", summary)
            logger.info("FinOps analysis completed: usage=%s", usage_callback.usage_metadata)

            if settings.remediation_enabled:
                finops_report.recommended_actions = _synthesize_remediation_actions(finops_report)

            report_data = finops_report.model_dump()

            await report_backend.upsert_report(
                report_id=report_id,
                status="completed",
                report=report_data,
                namespace=search_scope.namespace,
                project=search_scope.project,
                component=search_scope.component,
                environment=search_scope.environment,
            )
            logger.info("Updated FinOps report to completed: report_id=%s", report_id)

        except TimeoutError:
            logger.error(
                "Analysis timed out after %d seconds",
                settings.analysis_timeout_seconds,
            )
            try:
                await report_backend.upsert_report(
                    report_id=report_id,
                    status="failed",
                    summary=f"Analysis timed out (report_id: {report_id})",
                    namespace=search_scope.namespace,
                    project=search_scope.project,
                    component=search_scope.component,
                    environment=search_scope.environment,
                )
            except Exception as update_error:
                logger.error("Failed to update status: %s", type(update_error).__name__)

        except Exception as e:
            logger.error("Analysis failed: %s", type(e).__name__)
            try:
                await report_backend.upsert_report(
                    report_id=report_id,
                    status="failed",
                    summary=f"Analysis failed (report_id: {report_id})",
                    namespace=search_scope.namespace,
                    project=search_scope.project,
                    component=search_scope.component,
                    environment=search_scope.environment,
                )
            except Exception as update_error:
                logger.error("Failed to update status: %s", type(update_error).__name__)
