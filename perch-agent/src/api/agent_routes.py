# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import hashlib
import logging
from typing import Annotated, Literal

from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import Field, model_validator

from src.agent import stream_chat
from src.auth import SubjectContext, require_authn, require_invoke_authz
from src.auth.bearer import BearerTokenAuth
from src.clients import get_tools_for_user
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1/perch-agent", tags=["Perch Agent"])

# Strong references for fire-and-forget background tasks. Without this set,
# asyncio only weakly references the task and may garbage-collect it mid-flight.
_warmup_tasks: set[asyncio.Task] = set()


def _sub_fp(user_sub: str) -> str:
    """Irreversible truncated fingerprint of a user_sub for log correlation.

    Logging the raw OIDC sub retains a stable user identifier across log lines.
    A truncated sha256 keeps cross-line correlation while not leaking the
    identifier itself. Matches the token-fingerprint pattern in clients/mcp.py.
    """
    if not user_sub:
        return "<none>"
    return hashlib.sha256(user_sub.encode()).hexdigest()[:16]


class ChatMessage(BaseModel):
    model_config = {"extra": "forbid"}

    role: Literal["user", "assistant"]
    content: str = Field(max_length=10000)


class PrefetchedLogEntry(BaseModel):
    """A single log row the frontend captured from the rendered Logs tab.

    Forwarded so the agent can skip its first ``query_component_logs``
    call when launching from runtime_debug. The fields mirror the
    subset of ``ComponentLogEntry`` the prompt actually consumes —
    keeping the shape tight so a malicious / oversized client cannot
    blow the per-request content budget. Each row is sized-capped here
    AND the list is length-capped on ``ChatScope.prefetched_logs``;
    the per-request total-content validator below adds them up.
    """

    model_config = {"populate_by_name": True, "extra": "ignore"}

    # 64 chars covers RFC3339 with fractional seconds and any tz suffix.
    timestamp: str | None = Field(default=None, max_length=64)
    # ERROR / WARN / INFO / DEBUG — generous cap so non-standard
    # severities ("NOTICE", "TRACE") still parse without surprising
    # the user.
    level: str | None = Field(default=None, max_length=32)
    # The frontend already trims each line; this is a defensive
    # ceiling, not the intended length. A row hitting it is a hint
    # the frontend trimmer broke.
    message: str = Field(max_length=2000)
    component_name: str | None = Field(
        default=None, alias="componentName", max_length=253,
    )
    environment_name: str | None = Field(
        default=None, alias="environmentName", max_length=253,
    )


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
    # blocks. Allowed values today: "build_failure", "runtime_debug".
    # Unknown values fall back to the base prompt (no extra guidance).
    case_type: str | None = Field(default=None, alias="caseType", max_length=64)

    # ── Runtime-debug scope fields ─────────────────────────────────
    # These describe what the user has visible on the Logs / Traces
    # tab so the agent's first tool call matches the rendered table
    # instead of synthesising its own narrower default.

    # Severity filter the user has selected on the Logs tab
    # (e.g. ["ERROR","WARN","INFO"]). When set, the agent uses these
    # verbatim as ``log_levels`` for ``query_component_logs``.
    log_levels: list[str] | None = Field(default=None, alias="logLevels")
    # RFC3339 start/end already computed in the browser using the
    # user's clock. Passed verbatim to query_component_logs /
    # query_traces so the window matches the rendered table exactly.
    # We pre-compute frontend-side because LLMs hallucinate
    # timestamps with depressing consistency.
    logs_start_time: str | None = Field(
        default=None, alias="logsStartTime", max_length=64,
    )
    logs_end_time: str | None = Field(
        default=None, alias="logsEndTime", max_length=64,
    )

    # Marks the chat as launched from the Logs tab. The runtime_debug
    # prompt branch checks this to decide whether to take the
    # log-anchored sub-flow; today only the literal "log" is emitted.
    # Kept as a free-form string (not enum) so a future caller can
    # introduce a new anchor without breaking validation.
    runtime_anchor: str | None = Field(
        default=None, alias="runtimeAnchor", max_length=16,
    )

    # Log-side anchors (set when the user clicked a specific log
    # row). pinnedLogTraceId is the shortest path to a trace —
    # when present the agent fans out a parallel
    # query_trace_spans + query_traces on turn 1.
    pinned_log_timestamp: str | None = Field(
        default=None, alias="pinnedLogTimestamp", max_length=64,
    )
    pinned_log_message: str | None = Field(
        default=None, alias="pinnedLogMessage", max_length=2000,
    )
    pinned_log_trace_id: str | None = Field(
        default=None, alias="pinnedLogTraceId", max_length=128,
    )

    # Snapshot of the rows the user has rendered on the Logs tab.
    # When set, the runtime_debug prompt feeds these directly to the
    # model and tells it to SKIP query_component_logs on turn 1 —
    # eliminating a ~5-15 s tool roundtrip. List length capped at 50
    # so a misbehaving client can't push the request past the
    # _TOTAL_CONTENT_LIMIT (the per-row strings count toward it via
    # the validator below).
    prefetched_logs: list[PrefetchedLogEntry] | None = Field(
        default=None, alias="prefetchedLogs", max_length=50,
    )


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
            for v in self.scope.model_dump(exclude_none=True).values():
                if isinstance(v, str):
                    total += len(v)
                elif isinstance(v, (list, tuple)):
                    for item in v:
                        if isinstance(item, str):
                            total += len(item)
                        elif isinstance(item, dict):
                            # Lists of structured items (e.g. prefetched_logs
                            # rendered via model_dump). Count every string
                            # value so the per-request budget enforcement
                            # covers them too.
                            total += sum(
                                len(field_value)
                                for field_value in item.values()
                                if isinstance(field_value, str)
                            )
        if total > _TOTAL_CONTENT_LIMIT:
            raise ValueError(
                f"request total content {total} chars exceeds limit "
                f"{_TOTAL_CONTENT_LIMIT}"
            )
        return self


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
                "MCP tools warmup complete sub_fp=%s tool_count=%d",
                _sub_fp(user_sub),
                len(tools),
            )
        except Exception:  # noqa: BLE001
            logger.warning(
                "MCP tools warmup failed sub_fp=%s — first chat will pay "
                "the cache miss",
                _sub_fp(user_sub),
                exc_info=True,
            )

    task = asyncio.create_task(_prewarm())
    _warmup_tasks.add(task)
    task.add_done_callback(_warmup_tasks.discard)
    return {"status": "warming"}
