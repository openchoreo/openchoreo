# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import json
import logging
import uuid
from collections.abc import AsyncIterator
from typing import Any
from uuid import UUID

from langchain.agents import create_agent
from langchain.agents.middleware import SummarizationMiddleware, TodoListMiddleware
from langchain.agents.structured_output import ProviderStrategy
from langchain_core.callbacks import BaseCallbackHandler, UsageMetadataCallbackHandler
from langchain_core.language_models import BaseChatModel
from langchain_core.runnables import Runnable, RunnableConfig
from langchain_core.tools import BaseTool

from src.agent.middleware import (
    LoggingMiddleware,
    OutputTransformerMiddleware,
    ToolErrorHandlerMiddleware,
)
from src.agent.prompts import Agent, get_prompt
from src.agent.stream_parser import ChatResponseParser
from src.clients import MCPClient, get_model, get_opensearch_client
from src.config import settings
from src.constants import CHAT_AGENT_TOOLS, RCA_AGENT_TOOLS, TOOL_ACTIVE_FORMS, Templates
from src.logging_config import request_id_context
from src.models import ChatResponse, RCAReport
from src.template_manager import render

logger = logging.getLogger(__name__)

# Module-level semaphore for limiting concurrent analyses
_semaphore: asyncio.Semaphore | None = None


def _get_semaphore() -> asyncio.Semaphore:
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_analyses)
    return _semaphore


def _filter_tools(tools: list[BaseTool], allowed: set[str]) -> list[BaseTool]:
    filtered = [tool for tool in tools if tool.name in allowed]
    logger.debug(
        "Filtered to %d tools: %s",
        len(filtered),
        [tool.name for tool in filtered],
    )
    return filtered


async def _get_tools(allowed: set[str]) -> list[BaseTool]:
    mcp_client = MCPClient()
    all_tools = await mcp_client.get_tools()
    return _filter_tools(all_tools, allowed)


def _build_config(
    recursion_limit: int, usage_callback: BaseCallbackHandler | None
) -> RunnableConfig:
    config: RunnableConfig = {"recursion_limit": recursion_limit}
    if usage_callback is not None:
        config["callbacks"] = [usage_callback]
    return config


async def create_rca_agent(
    model: BaseChatModel, usage_callback: BaseCallbackHandler | None = None
) -> Runnable:
    tools = await _get_tools(RCA_AGENT_TOOLS)
    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=get_prompt(Agent.RCA, tools),
        middleware=[
            LoggingMiddleware(),
            ToolErrorHandlerMiddleware(),
            OutputTransformerMiddleware(),
            TodoListMiddleware(),
            SummarizationMiddleware(model=model, trigger=("fraction", 0.8)),
        ],
        response_format=ProviderStrategy(RCAReport),
    ).with_config(_build_config(200, usage_callback))

    logger.info("Created RCA agent with %d tools: %s", len(tools), [tool.name for tool in tools])
    return agent


async def create_chat_agent(
    model: BaseChatModel, usage_callback: BaseCallbackHandler | None = None
) -> Runnable:
    tools = await _get_tools(CHAT_AGENT_TOOLS)
    agent = create_agent(
        model=model,
        tools=tools,
        system_prompt=get_prompt(Agent.CHAT, tools),
        middleware=[
            LoggingMiddleware(),
            ToolErrorHandlerMiddleware(),
            OutputTransformerMiddleware(),
            SummarizationMiddleware(model=model, trigger=("fraction", 0.8)),
        ],
        response_format=ProviderStrategy(ChatResponse),
    ).with_config(_build_config(50, usage_callback))

    logger.info("Created chat agent with %d tools: %s", len(tools), [tool.name for tool in tools])
    return agent


async def stream_chat(
    messages: list[dict[str, str]],
    report_context: dict[str, Any] | None = None,
) -> AsyncIterator[str]:
    request_id_context.set(f"msg_{uuid.uuid4().hex[:12]}")

    def emit(event: dict[str, Any]) -> str:
        return json.dumps(event) + "\n"  # Newline for ndjson

    try:
        model = get_model()
        agent = await create_chat_agent(model)

        # Build messages with optional report context
        agent_messages = []
        if report_context:
            context_msg = (
                f"## RCA Report Context\n\n```json\n{json.dumps(report_context, indent=2)}\n```"
            )
            agent_messages.append({"role": "system", "content": context_msg})
        agent_messages.extend(messages)

        parser = ChatResponseParser()

        async for chunk, _ in agent.astream(
            {"messages": agent_messages},
            stream_mode="messages",
        ):
            # Skip non-AI message chunks (e.g., ToolMessage has content as list)
            if not isinstance(chunk.content, str):
                continue

            for block in chunk.content_blocks:
                block_type = block.get("type")

                if block_type == "tool_call_chunk":
                    tool_name = block.get("name")
                    args = block.get("args", "")
                    if tool_name:
                        active_form = TOOL_ACTIVE_FORMS.get(tool_name)
                        yield emit(
                            {
                                "type": "tool_call",
                                "tool": tool_name,
                                "activeForm": active_form,
                                "args": args,
                            }
                        )

                elif block_type == "text":
                    text = block.get("text", "")
                    if text:
                        delta = parser.push(text)
                        if delta:
                            yield emit({"type": "message_chunk", "content": delta})

        # Emit actions event if actions exist
        if parser.actions:
            yield emit({"type": "actions", "actions": parser.actions})

        # Build done event with parsed response
        yield emit({"type": "done", "message": parser.message})

    except Exception as e:
        logger.error("Chat stream error: %s", e, exc_info=True)
        yield emit(
            {
                "type": "error",
                "message": f"An error occured (request_id: {request_id_context.get()})",
            }
        )


async def run_analysis(
    report_id: str,
    alert_id: str,
    alert: Any,
    component_uid: UUID,
    project_uid: UUID,
    environment_uid: UUID,
    meta: dict[str, Any] | None = None,
) -> None:
    # Set request_id in context for logging (use report_id as it's unique per request)
    request_id_context.set(report_id)

    semaphore = _get_semaphore()
    opensearch_client = get_opensearch_client()

    logger.info("Analysis task queued")

    async with semaphore:
        logger.info("Analysis task started")

        try:
            usage_callback = UsageMetadataCallbackHandler()
            model = get_model()
            agent = await create_rca_agent(model, usage_callback=usage_callback)

            content = render(
                Templates.RCA_REQUEST,
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
                f"Analysis timed out (report_id: {report_id})",
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
                f"Analysis failed (report_id: {report_id})",
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
