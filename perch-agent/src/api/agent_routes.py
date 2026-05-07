# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from typing import Annotated, Literal

from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import Field, model_validator

from src.agent import execute_action, stream_chat
from src.auth import SubjectContext, require_authn, require_invoke_authz
from src.auth.bearer import BearerTokenAuth
from src.clients import get_tools_for_user
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1/perch-agent", tags=["Perch Agent"])

# Strong references for fire-and-forget background tasks. Without this set,
# asyncio only weakly references the task and may garbage-collect it mid-flight.
_warmup_tasks: set[asyncio.Task] = set()


class ChatMessage(BaseModel):
    model_config = {"extra": "forbid"}

    role: Literal["user", "assistant"]
    content: str = Field(max_length=10000)


class ChatScope(BaseModel):
    """Optional default scope hints derived from the Backstage entity context.

    Soft-scoped (Q7): the agent prefers these for tool calls but will answer
    cross-tenant queries when explicitly asked, bounded by per-tool MCP authz.

    ``run_name`` is set by external triggers (e.g. the failed-build snackbar
    on a component overview page) so the agent has an unambiguous handle on
    the workflow run being discussed without having to infer it.
    """

    model_config = {"populate_by_name": True}

    # K8s identifiers are bounded by the 253-char DNS-subdomain limit; pad
    # generously to absorb annotation-style values without enabling abuse.
    namespace: str | None = Field(default=None, max_length=253)
    project: str | None = Field(default=None, max_length=253)
    component: str | None = Field(default=None, max_length=253)
    environment: str | None = Field(default=None, max_length=253)
    # Frontend serializes camelCase; accept either form so the contract is
    # compatible with the existing ChatScope.runName TypeScript type.
    run_name: str | None = Field(default=None, alias="runName", max_length=253)
    # Status of the pinned run if known (e.g. "Failed"). Lets the prompt
    # confirm the failure premise without an extra tool call.
    run_status: str | None = Field(default=None, alias="runStatus", max_length=64)
    # Bound workflow CRD details — name + kind ("Workflow" | "ClusterWorkflow").
    # When set, the agent can go straight to get_(cluster_)workflow without
    # first calling list_*.
    workflow_name: str | None = Field(default=None, alias="workflowName", max_length=253)
    workflow_kind: str | None = Field(default=None, alias="workflowKind", max_length=64)
    # Optional case discriminator set by purpose-built launchers
    # (e.g. failed-build snackbar). The base prompt branches on this to
    # layer in case-specific guidance — see perch_prompt.j2 case_type
    # blocks. Allowed values today: "build_failure", "logs_debug".
    # Unknown values fall back to base.
    case_type: str | None = Field(default=None, alias="caseType", max_length=64)


# Per-request total bytes of message content. Each ChatMessage.content
# is already capped at 10,000 chars individually, but a 50-message list
# at the cap is 500k chars per request — enough to push the LLM into a
# slow/expensive turn or DOS one pod's CPU/memory at the prompt-template
# render. Reject early with 422 when the sum exceeds this.
_TOTAL_CONTENT_LIMIT = 60_000


class ChatRequest(BaseModel):
    messages: list[ChatMessage] = Field(min_length=1, max_length=50)
    scope: ChatScope | None = None

    @model_validator(mode="after")
    def _validate_total_content(self) -> "ChatRequest":
        total = sum(len(m.content) for m in self.messages)
        if self.scope is not None:
            total += sum(
                len(v) for v in self.scope.model_dump(exclude_none=True).values()
                if isinstance(v, str)
            )
        if total > _TOTAL_CONTENT_LIMIT:
            raise ValueError(
                f"request total content {total} chars exceeds limit "
                f"{_TOTAL_CONTENT_LIMIT}"
            )
        return self


class ExecuteRequest(BaseModel):
    action_id: str = Field(min_length=1, max_length=64)


@router.post("/chat")
async def chat(
    request: ChatRequest,
    http_request: Request,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_invoke_authz)],
):
    token = http_request.state.bearer_token
    user_sub = http_request.state.user_sub

    return StreamingResponse(
        stream_chat(
            messages=[m.model_dump() for m in request.messages],
            token=token,
            user_sub=user_sub,
            scope=request.scope.model_dump(exclude_none=True) if request.scope else None,
        ),
        media_type="application/x-ndjson",
        headers={"Cache-Control": "no-cache"},
    )


@router.post("/execute")
async def execute(
    request: ExecuteRequest,
    http_request: Request,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_invoke_authz)],
):
    token = http_request.state.bearer_token
    user_sub = http_request.state.user_sub

    result = await execute_action(action_id=request.action_id, user_sub=user_sub, token=token)
    return result


@router.post("/warmup", status_code=202)
async def warmup(
    http_request: Request,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_invoke_authz)],
):
    """Pre-populate the per-user MCP tools cache so the user's first chat is fast.

    Frontend calls this once after sign-in (when the Perch feature is on).
    The fetch runs in the background; the response returns immediately so the
    UI doesn't block on the 6-9s tool-listing roundtrip.
    """
    token = http_request.state.bearer_token
    user_sub = http_request.state.user_sub

    async def _prewarm() -> None:
        try:
            tools = await get_tools_for_user(user_sub, BearerTokenAuth(token))
            logger.info(
                "MCP tools warmup complete user_sub=%s tool_count=%d",
                user_sub,
                len(tools),
            )
        except Exception:  # noqa: BLE001
            logger.warning(
                "MCP tools warmup failed user_sub=%s — first chat will pay "
                "the cache miss",
                user_sub,
                exc_info=True,
            )

    task = asyncio.create_task(_prewarm())
    _warmup_tasks.add(task)
    task.add_done_callback(_warmup_tasks.discard)
    return {"status": "warming"}
