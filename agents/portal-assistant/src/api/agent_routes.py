# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import hashlib
import logging
import re
from typing import Annotated, Any, Literal

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse
from pydantic import Field, field_validator, model_validator

from common.auth.authz_models import SubjectContext
from common.auth.bearer import BearerTokenAuth
from src.agent import stream_chat
from src.agent.confirm_actions import ConfirmError, confirm_check_change
from src.agent.pending_actions import InvalidAction
from src.agent.user_action import CONTEXT_VALUE_MAX
from src.auth import require_authn, require_invoke_authz
from src.clients import get_tools_for_user
from src.models import BaseModel

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/api/v1alpha1/portal-assistant", tags=["Portal Assistant"])

BACKSTAGE_USER_TOKEN_HEADER = "x-backstage-user-token"

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


class ChatAction(BaseModel):
    """The drawer button a user message came from: its A2UI event's
    ``name`` and ``context`` (see src/agent/user_action.py)."""

    model_config = {"extra": "forbid"}

    name: str = Field(max_length=64, pattern=r"^[a-z_]+$")
    context: dict[str, str | int] = Field(default_factory=dict, max_length=8)

    @model_validator(mode="after")
    def _small_context(self) -> ChatAction:
        for key, value in self.context.items():
            if len(key) > 64 or (isinstance(value, str) and len(value) > CONTEXT_VALUE_MAX):
                raise ValueError("action context values are too long")
        return self


class ChatMessage(BaseModel):
    model_config = {"extra": "forbid"}

    role: Literal["user", "assistant"]
    content: str = Field(max_length=10000)
    # Set on a user message sent by a drawer button; None when typed.
    action: ChatAction | None = None


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


class PendingDependency(BaseModel):
    """An unresolved outbound endpoint dependency the Deploy tab read off the
    binding's ``pendingConnections``.

    Forwarded on ``dependency_pending`` chats so the agent names *which*
    dependency is blocking without a discovery tool call and spends its
    first call inspecting *why* it's down. Each field is size-capped so a
    client can't blow the per-request content budget (counted by the
    ChatRequest total-content validator below, which walks list-of-dict
    string values).
    """

    model_config = {"populate_by_name": True, "extra": "ignore"}

    project: str | None = Field(default=None, max_length=253)
    component: str = Field(max_length=253)
    endpoint: str | None = Field(default=None, max_length=253)
    reason: str | None = Field(default=None, max_length=512)


class FactRetrieverField(BaseModel):
    """One fact exposed by a registered fact retriever.

    Mirrors the shape the host tech-insights plugin's ``/fact-schemas``
    endpoint returns per fact, trimmed to what the check_authoring prompt
    branch actually renders.
    """

    model_config = {"populate_by_name": True, "extra": "ignore"}

    name: str = Field(max_length=253)
    type: str = Field(max_length=64)
    description: str | None = Field(default=None, max_length=512)


class FactRetriever(BaseModel):
    """One registered fact retriever and the facts it exposes.

    Forwarded on ``check_authoring`` chats so the agent drafts checks
    against facts that actually exist on this backend instead of
    hallucinating retriever/fact names — see the "Available facts" table
    in perch_prompt.j2's CHECK AUTHORING block.
    """

    model_config = {"populate_by_name": True, "extra": "ignore"}

    id: str = Field(max_length=253)
    # Entity kinds it collects facts for; None when not limited to any.
    kinds: list[str] | None = Field(default=None, max_length=50)
    facts: list[FactRetrieverField] = Field(default_factory=list, max_length=100)


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
    # When set, the agent can go straight to get_workflow (with the matching
    # scope) without
    # first calling list_*.
    workflow_name: str | None = Field(default=None, alias="workflowName", max_length=253)
    workflow_kind: str | None = Field(default=None, alias="workflowKind", max_length=64)
    # Git repository URL extracted from the component's workflow schema by
    # the Backstage launcher. Forwarded to the system prompt so the model
    # can include it in the build_failure ``fix_prompt`` without an extra
    # tool call. Optional — components without a workflow ``repository.url``
    # parameter omit it, and the prompt template degrades gracefully.
    repo_url: str | None = Field(default=None, alias="repoUrl", max_length=2000)
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

    # ── Dependency-pending scope fields ────────────────────────────
    # Set by the Deploy-tab "Investigate" button when a component is
    # stuck NotReady because an outbound endpoint connection can't
    # resolve. The dependency_pending prompt branch names these directly
    # so the agent skips rediscovering them and inspects why each is down.
    # Length-capped (per-row strings count toward _TOTAL_CONTENT_LIMIT
    # via the validator below).
    pending_dependencies: list[PendingDependency] | None = Field(
        default=None, alias="pendingDependencies", max_length=50,
    )

    # ── Check-authoring scope fields ───────────────────────────────
    # Set by the "Generate with AI" and "Edit with AI" launchers. The
    # check_authoring prompt branch renders these as the live
    # "Available facts" table so the agent only references facts that
    # actually exist on this backend, instead of a fixed example set.
    available_fact_retrievers: list[FactRetriever] | None = Field(
        default=None, alias="availableFactRetrievers", max_length=50,
    )
    # factIds already picked in the check-editor form for this specific
    # check. The prompt branch treats these as a preference, not a hard
    # restriction — the agent may still reach for a different retriever
    # when the request needs a fact none of the selected ones provide.
    selected_fact_ids: list[str] | None = Field(
        default=None, alias="selectedFactIds", max_length=50,
    )
    # Set only when the launcher is opened from the edit dialog of an
    # existing check. The prompt branch then edits ``current_rule`` (the
    # check's ``rule.conditions`` as a JSON string) instead of drafting a
    # new check, and uses the name/description to suggest
    # related text changes the user may want to apply too.
    check_id: str | None = Field(default=None, alias="checkId", max_length=253)
    check_name: str | None = Field(default=None, alias="checkName", max_length=253)
    check_description: str | None = Field(
        default=None, alias="checkDescription", max_length=2000,
    )
    current_rule: str | None = Field(
        default=None, alias="currentRule", max_length=8000,
    )
    # Set by the drawer on the level's message in a new check authoring chat:
    # the reply is the welcome for that level (see stream_chat).
    welcome: bool | None = None
    # How much the user knows about checks, picked in the check authoring
    # welcome: the prompt suits each reply to it.
    experience: Literal["beginner", "experienced"] | None = None

    @model_validator(mode="after")
    def _validate_edit_has_its_rule(self) -> ChatScope:
        """An edit (``check_id``) must carry the saved rule: the guards that
        make the model ask before changing a saved check need it (see
        orchestrator._edits_saved_rule). The edit dialog always sends it."""
        if self.check_id and not (self.current_rule or "").strip():
            raise ValueError("checkId needs currentRule, the check's saved rule")
        return self

    @field_validator("repo_url")
    @classmethod
    def _validate_repo_url(cls, v: str | None) -> str | None:
        """Reject control chars / non-http(s) schemes in ``repo_url``.

        ``repo_url`` is interpolated into the system prompt and into the
        agent-generated ``fix_prompt`` payload. Without validation a
        client could send newlines, NUL, or other control bytes and
        break out of the URL context to inject instructions into the
        prompt template ("Repo: https://x.com\\n\\nIGNORE PRIOR …").
        We strip surrounding whitespace, reject any C0 / DEL control
        character (including newlines and tabs), and require an http
        or https scheme — anything else is rejected with a 422.
        """
        if v is None:
            return None
        v = v.strip()
        if not v:
            return None
        if re.search(r"[\x00-\x1F\x7F]", v):
            raise ValueError("repo_url contains control characters")
        if not re.match(r"^https?://[^\s]+$", v):
            raise ValueError("repo_url must be an http(s) URL")
        return v


# Per-request total bytes of message content. Each ChatMessage.content
# is already capped at 10,000 chars individually, but a 50-message list
# at the cap is 500k chars per request — enough to push the LLM into a
# slow/expensive turn or DOS one pod's CPU/memory at the prompt-template
# render. Reject early with 422 when the sum exceeds this.
_TOTAL_CONTENT_LIMIT = 60_000


def _sum_string_lengths(value: Any) -> int:
    """Recursively sum the length of every string reachable from ``value``.

    Walks dicts/lists/tuples of arbitrary depth so a ChatScope field with
    nested structured sub-objects (e.g. ``available_fact_retrievers``,
    whose ``facts`` is itself a list of dicts) is covered by the
    per-request content budget without a bespoke branch per shape.
    """
    if isinstance(value, str):
        return len(value)
    if isinstance(value, dict):
        return sum(_sum_string_lengths(v) for v in value.values())
    if isinstance(value, (list, tuple)):
        return sum(_sum_string_lengths(v) for v in value)
    return 0


class ChatRequest(BaseModel):
    messages: list[ChatMessage] = Field(min_length=1, max_length=50)
    scope: ChatScope | None = None

    @model_validator(mode="after")
    def _validate_total_content(self) -> "ChatRequest":
        total = sum(len(m.content) for m in self.messages)
        # A button's action context counts too (a change card's button
        # carries its rule).
        total += sum(
            _sum_string_lengths(m.action.model_dump()) for m in self.messages if m.action
        )
        if self.scope is not None:
            total += _sum_string_lengths(self.scope.model_dump(exclude_none=True))
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
    # Set by the Backstage forwarder: a short-lived token Backstage issued on
    # the signed-in user's behalf, valid only for its mcp-actions plugin.
    # Backstage verifies it on every MCP call; never log it.
    backstage_token = http_request.headers.get(BACKSTAGE_USER_TOKEN_HEADER) or None

    return StreamingResponse(
        stream_chat(
            messages=[m.model_dump(exclude_none=True) for m in request.messages],
            token=token,
            user_sub=user_sub,
            backstage_token=backstage_token,
            scope=request.scope.model_dump(exclude_none=True) if request.scope else None,
        ),
        media_type="application/x-ndjson",
        headers={"Cache-Control": "no-cache"},
    )


class ConfirmRequest(BaseModel):
    """The user's click on a check confirm card."""

    model_config = {"populate_by_name": True, "extra": "forbid"}

    # Room for the card's check and baseline, each with the description, in
    # any language (see test_a_long_non_english_card_round_trips).
    token: str = Field(max_length=64_000)
    # The status the check ends in; the card offers only these two.
    status: Literal["draft", "published"]
    # Whether the card's suggested Name / Description are saved.
    apply_texts: bool = Field(default=True, alias="applyTexts")


@router.post("/chat/confirm")
async def confirm(
    request: ConfirmRequest,
    http_request: Request,
    _auth: Annotated[SubjectContext, Depends(require_authn)],
    _authz: Annotated[SubjectContext, Depends(require_invoke_authz)],
):
    """Save the check change on a confirm card, as the signed-in user.

    Runs exactly the change signed into the card's token, through Backstage
    MCP with the user's Backstage token. No model is involved.
    """
    try:
        return await confirm_check_change(
            request.token,
            user_sub=http_request.state.user_sub,
            backstage_token=http_request.headers.get(BACKSTAGE_USER_TOKEN_HEADER) or None,
            status=request.status,
            apply_texts=request.apply_texts,
        )
    except (InvalidAction, ConfirmError) as exc:
        raise HTTPException(
            status_code=400,
            detail={"error": "CONFIRM_FAILED", "message": str(exc)},
        ) from exc


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
