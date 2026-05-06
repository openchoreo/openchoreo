# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""MCP server exposing rca-agent capabilities to upstream LLM agents.

Mounted as a sub-app at ``/mcp`` on the FastAPI app in main.py. Speaks the
streamable HTTP MCP transport — same wire protocol the openchoreo-api
control-plane and the observer plane already expose, so the existing
``langchain-mcp-adapters`` client used by assistant-agent connects without
any new code.

Auth flow:
    1. ASGI middleware validates ``Authorization: Bearer <JWT>`` on every
       request (Initialize / tools/list / tools/call). Same JWKS validation
       and claim mapping as the FastAPI REST routes (`require_authn`).
    2. The middleware stashes the validated ``SubjectContext`` and bearer
       token in contextvars so each tool can:
         - re-authorize against the platform PDP (rcareport:view / :update)
           with the user's own bearer, AND
         - attribute background work (analyze_runtime_state's run_analysis
           coroutine) back to the caller in logs.
    3. Each tool calls ``_authorize`` with the action it needs and the
       project hierarchy it is about to touch — the SAME action constants
       the REST routes use, so a deny here matches a deny on /reports/*.

Streaming: this module uses a *pure ASGI* middleware (not
``BaseHTTPMiddleware``) because the MCP streamable-HTTP transport sends
SSE-style chunked responses; ``BaseHTTPMiddleware`` buffers the entire
body before forwarding and would either deadlock or block the stream.
"""

import asyncio
import contextvars
import logging
from datetime import datetime, timezone
from typing import Annotated, Any, Literal
from uuid import uuid4

from fastapi import HTTPException
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings
from pydantic import Field
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.types import ASGIApp, Receive, Scope, Send

from src.agent import run_analysis
from src.auth.authz_models import (
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from src.auth.dependencies import get_authz_client, require_authn
from src.clients import get_report_backend
from src.helpers import resolve_component_scope, resolve_project_scope, validate_time_range

logger = logging.getLogger(__name__)


# Per-request context. The ASGI middleware sets these once auth has passed;
# tool implementations read them to authorize and to attribute logs.
_subject_ctx: contextvars.ContextVar[SubjectContext | None] = contextvars.ContextVar(
    "rca_mcp_subject", default=None
)
_token_ctx: contextvars.ContextVar[str | None] = contextvars.ContextVar(
    "rca_mcp_token", default=None
)
# Optional X-Request-ID propagated end-to-end (Backstage → assistant-agent →
# rca-agent MCP). When the caller doesn't set one, the middleware synthesizes
# a short uuid so a single MCP call still has a stable correlation id.
_request_id_ctx: contextvars.ContextVar[str | None] = contextvars.ContextVar(
    "rca_mcp_request_id", default=None
)

# Track in-flight analyze_runtime_state background tasks so shutdown can
# wait for them (or at least surface the count) rather than silently
# killing a half-written RCA report.
_background_tasks: set[asyncio.Task[Any]] = set()


mcp_server = FastMCP(
    name="rca-agent",
    instructions=(
        "Root-cause analysis tools for OpenChoreo runtime errors. "
        "Use list_rca_reports first to check for an existing analysis in the "
        "relevant time window before triggering analyze_runtime_state."
    ),
    # The streamable HTTP app's internal mount path. We mount the whole
    # sub-app at /mcp on the outer FastAPI in main.py, which strips /mcp
    # from the path before forwarding — so the inner app must serve at
    # the root "/" or the request 404s.
    streamable_http_path="/",
    # FastMCP's default DNS-rebinding protection rejects any Host header
    # not on a tiny allowlist (localhost). In-cluster traffic from the
    # assistant-agent uses the Service DNS host (e.g.
    # ai-rca-agent.openchoreo-observability-plane.svc.cluster.local),
    # which would 421. We disable the protection because (a) ingress is
    # already gated by JWT auth and (b) the protection guards browsers,
    # not service-to-service callers.
    #
    # Required deployment invariants (the security argument depends on
    # ALL of these — verify when changing the chart or networking):
    #   1. ``mcp>=1.23.0`` is pinned in pyproject.toml. Pre-1.23 the SDK
    #      had a different default for this flag and a different set of
    #      transport-layer mitigations.
    #   2. The ``/mcp`` endpoint is NOT exposed via the cluster's
    #      external HTTPRoute. Only the in-cluster Service is reachable.
    #      See install/helm/.../templates/rca-agent/httproute.yaml — it
    #      must NOT include a /mcp path match for the public gateway.
    #   3. NetworkPolicy (or equivalent) restricts ingress to ``/mcp`` to
    #      the assistant-agent ServiceAccount / Pod selector.
    # If any of (1)–(3) cannot be guaranteed in a target environment,
    # flip this to True and supply ``allowed_hosts`` for the in-cluster
    # Service DNS instead of disabling protection wholesale.
    # TODO: revisit once FastMCP exposes a clean ``allowed_hosts`` API
    # that can be combined with ``enable_dns_rebinding_protection=True``
    # for a defense-in-depth setup.
    transport_security=TransportSecuritySettings(
        enable_dns_rebinding_protection=False,
    ),
)


# ---------------------------------------------------------------------------
# Authz helpers
# ---------------------------------------------------------------------------


class _MCPAuthzError(Exception):
    """Raised inside a tool to surface a 403 Forbidden distinctly from a
    generic backend failure. The MCP transport flattens this to a tool
    error; the upstream agent sees a clear "FORBIDDEN" string and can
    decide to degrade gracefully instead of retrying."""


class _MCPNotFoundError(Exception):
    """Raised inside a tool when the requested resource doesn't exist —
    distinct from FORBIDDEN so the calling agent doesn't loop on 'try
    fetching it again'."""


async def _authorize(
    action: str,
    resource_type: str,
    hierarchy: ResourceHierarchy,
) -> SubjectContext:
    """Re-evaluate the user's entitlement for ``action`` on the given
    resource hierarchy. Mirrors the REST route's
    ``require_*_authz`` dependency exactly — same PDP, same actions —
    so MCP and REST authz outcomes are guaranteed consistent."""
    subject = _subject_ctx.get()
    token = _token_ctx.get()
    if subject is None or token is None:
        # Unreachable if the middleware has run. This is a server-side
        # bug (someone called a tool outside the request flow), NOT an
        # authorization denial — raise a distinct exception so the
        # upstream agent doesn't conflate it with a real 403.
        raise RuntimeError("MCP authn context missing — tool invoked outside request flow")

    logger.info(
        "MCP authz check rid=%s subject_type=%s action=%s resource=%s project=%s",
        _request_id_ctx.get(),
        subject.type,
        action,
        resource_type,
        hierarchy.project,
    )
    client = get_authz_client()
    decision = await client.evaluate(
        EvaluateRequest(
            subjectContext=subject,
            resource=Resource(type=resource_type, id="", hierarchy=hierarchy),
            action=action,
            context={},
        ),
        token,
    )
    if not decision.decision:
        logger.warning(
            "MCP authz denied rid=%s subject_type=%s action=%s resource=%s project=%s",
            _request_id_ctx.get(),
            subject.type,
            action,
            resource_type,
            hierarchy.project,
        )
        raise _MCPAuthzError(f"FORBIDDEN: missing entitlement for action '{action}'")
    return subject


# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------


@mcp_server.tool()
async def list_rca_reports(
    namespace: Annotated[str, Field(description="OpenChoreo namespace.")],
    project: Annotated[str, Field(description="Project name within the namespace.")],
    environment: Annotated[
        str,
        Field(description="Environment name (e.g. dev, staging, production)."),
    ],
    start_time: Annotated[
        str, Field(description="RFC3339 timestamp for the window start.")
    ],
    end_time: Annotated[
        str, Field(description="RFC3339 timestamp for the window end.")
    ],
    limit: Annotated[
        int,
        Field(
            ge=1,
            le=10_000,
            description="Max results to return (1-10000).",
        ),
    ] = 100,
    sort: Annotated[
        Literal["asc", "desc"],
        Field(description="Sort by report timestamp. Default newest-first."),
    ] = "desc",
    status: Annotated[
        Literal["pending", "completed", "failed"] | None,
        Field(description="Optional filter on report status."),
    ] = None,
) -> dict[str, Any]:
    """List existing RCA reports for a project/environment in a time window.

    Use this before triggering a fresh analysis — if an RCA report already
    exists for the relevant window, fetch it via get_rca_report instead.
    """
    norm_start, norm_end = validate_time_range(start_time, end_time)
    scope = await resolve_project_scope(
        namespace=namespace, project=project, environment=environment
    )
    await _authorize(
        "rcareport:view",
        "rcareport",
        ResourceHierarchy(project=scope.project_uid),
    )
    report_backend = get_report_backend()
    result = await report_backend.list_rca_reports(
        project_uid=scope.project_uid,
        environment_uid=scope.environment_uid,
        start_time=norm_start,
        end_time=norm_end,
        status=status,
        limit=limit,
        sort=sort,
    )
    return {
        "reports": result.get("reports", []),
        "totalCount": result.get("totalCount", 0),
    }


@mcp_server.tool()
async def get_rca_report(
    report_id: Annotated[
        str,
        Field(
            min_length=1,
            description=(
                "Report identifier returned by analyze_runtime_state or "
                "surfaced by list_rca_reports."
            ),
        ),
    ],
) -> dict[str, Any]:
    """Fetch a specific RCA report by ID."""
    report_backend = get_report_backend()
    result = await report_backend.get_rca_report(report_id)
    if not result:
        raise _MCPNotFoundError(f"RCA report not found: {report_id}")
    # Re-authorize against the report's own project — the user might be
    # entitled to one project's reports but not another's, and we don't
    # want list_rca_reports to be the only gate. Single-key contract:
    # backends MUST emit ``projectUid`` at the top level of the doc
    # (see sql_backend._row_to_doc). A missing/empty value would mean
    # we can't make an authz decision — fail closed with FORBIDDEN
    # rather than degrading to "authorize against project=None" or
    # leaking the report's existence via NOT_FOUND.
    project_uid = result.get("projectUid")
    if not project_uid:
        logger.error(
            "RCA report %s has no projectUid — refusing to authorize",
            report_id,
        )
        raise _MCPAuthzError(
            f"FORBIDDEN: report {report_id} has no project hierarchy"
        )
    await _authorize(
        "rcareport:view",
        "rcareport",
        ResourceHierarchy(project=project_uid),
    )
    return {
        "alertId": result.get("alertId"),
        "reportId": result.get("reportId"),
        "timestamp": result.get("@timestamp"),
        "status": result.get("status"),
        "report": result.get("report"),
    }


@mcp_server.tool()
async def analyze_runtime_state(
    namespace: Annotated[str, Field(description="OpenChoreo namespace.")],
    project: Annotated[str, Field(description="Project name.")],
    component: Annotated[
        str,
        Field(description="Component name (must already exist)."),
    ],
    environment: Annotated[
        str,
        Field(description="Environment name (e.g. dev, staging, production)."),
    ],
    summary: Annotated[
        str | None,
        Field(
            max_length=500,
            description=(
                "Optional one-line description of what the user is seeing; "
                "stored on the synthetic alert as user context for the "
                "analysis prompt."
            ),
        ),
    ] = None,
) -> dict[str, Any]:
    """Trigger a fresh RCA analysis for a component's runtime state.

    Synthesises a manual "alert" so the standard run_analysis pipeline can
    pull logs / metrics / traces and produce a persistent RCA report. The
    analysis runs asynchronously in the background; poll get_rca_report
    with the returned report_id.
    """
    scope = await resolve_component_scope(
        namespace=namespace,
        project=project,
        component=component,
        environment=environment,
    )
    # Triggering an analysis CREATES a report row — gate on :update, not
    # :view (matches the REST /analyze authz once that route gains one).
    await _authorize(
        "rcareport:update",
        "rcareport",
        ResourceHierarchy(project=scope.project_uid, component=scope.component_uid),
    )

    timestamp = datetime.now(timezone.utc)
    alert_id = f"manual-{uuid4().hex[:12]}"
    report_id = f"{alert_id}_{int(timestamp.timestamp())}"

    alert = {
        "id": alert_id,
        "value": 0,
        "timestamp": timestamp.isoformat().replace("+00:00", "Z"),
        "rule": {
            "name": "manual-rca-trigger",
            "description": summary
            or "Manual RCA triggered from the assistant — no specific alert.",
            "severity": "info",
            "source": None,
            "condition": None,
        },
    }

    # Bound the stub-upsert so a slow/wedged backend can't keep the MCP
    # tool call open indefinitely. The streamable-HTTP client side may
    # not have its own timeout configured, so the cap has to live here.
    # Distinct exception path → distinct error string the caller can
    # surface ("backend timed out") vs a generic upsert failure.
    _UPSERT_TIMEOUT = 5.0
    report_backend = get_report_backend()
    try:
        await asyncio.wait_for(
            report_backend.upsert_rca_report(
                report_id=report_id,
                alert_id=alert_id,
                status="pending",
                timestamp=timestamp,
                environment_uid=scope.environment_uid,
                project_uid=scope.project_uid,
            ),
            timeout=_UPSERT_TIMEOUT,
        )
    except asyncio.TimeoutError as e:
        logger.error(
            "Report backend upsert exceeded %.1fs for report_id=%s",
            _UPSERT_TIMEOUT,
            report_id,
        )
        raise RuntimeError(
            f"Report backend timed out after {_UPSERT_TIMEOUT:.0f}s; "
            "no analysis was started — please retry"
        ) from e
    except Exception as e:
        logger.error("Failed to create RCA report stub: %s", e, exc_info=True)
        raise RuntimeError(f"Failed to create analysis task: {e}") from e

    # Capture the subject for the queued-log line BEFORE we launch the
    # background task — the task is intentionally spawned in an empty
    # context (see below) so its own _subject_ctx will be None.
    subject = _subject_ctx.get()

    # Track the task so shutdown can drain in-flight analyses instead of
    # killing them mid-write. add_done_callback removes it once complete.
    #
    # Spawn the task inside an *empty* contextvars.Context so the user's
    # bearer / SubjectContext / request-id (all currently set in the
    # request-handling context) are NOT captured into the long-running
    # background task. The analysis pipeline runs as the rca-agent's
    # service-account by design (see this tool's docstring); leaking
    # the request-scoped bearer would be a privilege-attribution bug
    # if any code in run_analysis ever read _token_ctx.
    empty_ctx = contextvars.Context()
    task = empty_ctx.run(
        asyncio.create_task,
        run_analysis(
            report_id=report_id,
            alert_id=alert_id,
            alert=alert,
            scope=scope,
            meta=None,
        ),
    )
    _background_tasks.add(task)
    task.add_done_callback(_background_tasks.discard)
    logger.info(
        "MCP analyze_runtime_state queued rid=%s report_id=%s subject_type=%s entitlements=%s",
        _request_id_ctx.get(),
        report_id,
        subject.type if subject else "?",
        subject.entitlement_values if subject else [],
    )

    return {
        "report_id": report_id,
        "status": "pending",
        "message": (
            "Analysis triggered. Poll get_rca_report("
            f"report_id='{report_id}') in 10-60 seconds for results."
        ),
    }


# ---------------------------------------------------------------------------
# ASGI middleware (auth + per-request context)
# ---------------------------------------------------------------------------


class _MCPAuthMiddleware:
    """Pure ASGI middleware: validates the JWT, stashes the SubjectContext
    and bearer token in contextvars for the duration of the request, then
    forwards.

    Pure ASGI (vs. starlette.middleware.base.BaseHTTPMiddleware) is a
    deliberate choice: the MCP streamable-HTTP transport sends chunked
    SSE-style responses, and BaseHTTPMiddleware buffers the entire response
    body in memory before forwarding. That would either delay first byte
    or, on long-running tool calls, deadlock entirely.

    For non-HTTP scopes (lifespan, websocket) we forward unchanged.
    """

    def __init__(self, app: ASGIApp) -> None:
        self._app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self._app(scope, receive, send)
            return

        # Build a Starlette Request just to reuse require_authn (which
        # reads headers and writes request.state.bearer_token). We don't
        # consume the body, so the inner app still receives it.
        request = Request(scope, receive)

        # Correlation id: prefer caller-supplied X-Request-ID so a single
        # id flows Backstage → assistant-agent → rca-agent. Otherwise
        # synthesize a short uuid so every MCP call has *some* id.
        request_id = request.headers.get("x-request-id") or uuid4().hex[:12]
        rid_token = _request_id_ctx.set(request_id)
        try:
            try:
                subject = await require_authn(request)
            except HTTPException as exc:
                detail = exc.detail
                if not isinstance(detail, dict):
                    detail = {"error": "AUTH_FAILED", "message": str(detail)}
                logger.info(
                    "MCP auth rejected rid=%s status=%s reason=%s",
                    request_id,
                    exc.status_code,
                    detail.get("error"),
                )
                response = JSONResponse(
                    status_code=exc.status_code,
                    content={"detail": detail},
                    headers={"X-Request-ID": request_id},
                )
                await response(scope, receive, send)
                return
            except Exception as exc:  # noqa: BLE001
                # Any other failure here (JWT lib bug, JWKS down) should NOT
                # leak a stack to the caller — return a clean 500.
                logger.exception("MCP auth middleware crashed rid=%s: %s", request_id, exc)
                response = JSONResponse(
                    status_code=500,
                    content={"detail": {"error": "AUTH_INTERNAL", "message": "auth failure"}},
                    headers={"X-Request-ID": request_id},
                )
                await response(scope, receive, send)
                return

            # Pin subject + token on this request's task so tools can see them.
            token_token = _token_ctx.set(getattr(request.state, "bearer_token", None))
            subject_token = _subject_ctx.set(subject)
            try:
                await self._app(scope, receive, send)
            finally:
                _subject_ctx.reset(subject_token)
                _token_ctx.reset(token_token)
        finally:
            _request_id_ctx.reset(rid_token)


def make_mcp_app() -> ASGIApp:
    """Build the ASGI app to mount under ``/mcp`` on the FastAPI server."""
    return _MCPAuthMiddleware(mcp_server.streamable_http_app())


async def drain_background_tasks(
    timeout: float = 30.0, cancel_wait: float = 5.0
) -> None:
    """Best-effort wait for in-flight analyze_runtime_state tasks to
    finish before the process exits. Called from the FastAPI lifespan
    on shutdown so we don't kill an analysis mid-write.

    Two-phase bound:
      - ``timeout`` — graceful window for tasks to complete on their own.
      - ``cancel_wait`` — hard cap on how long we wait for *cancellation*
        to take effect after the graceful window expires. Necessary
        because a task that swallows ``CancelledError`` would otherwise
        wedge ``asyncio.gather`` indefinitely and block the whole
        shutdown past Kubernetes' grace period.
    """
    if not _background_tasks:
        return
    pending = list(_background_tasks)
    logger.info("Draining %d in-flight RCA analysis task(s)…", len(pending))
    _, still_pending = await asyncio.wait(pending, timeout=timeout)
    if not still_pending:
        return
    logger.warning(
        "Shutdown timeout: %d RCA analysis task(s) still running; cancelling",
        len(still_pending),
    )
    for t in still_pending:
        t.cancel()
    # Bounded post-cancel wait. NOTE: do NOT use
    # ``asyncio.wait_for(asyncio.gather(...), timeout=cancel_wait)`` here
    # — in Python 3.11+ wait_for awaits the inner task's cancellation to
    # complete before raising TimeoutError, so a task that swallows
    # CancelledError hangs wait_for indefinitely. ``asyncio.wait`` with
    # a timeout is genuinely bounded: it returns after ``timeout`` no
    # matter what the children are doing, leaving misbehaving tasks to
    # be reaped at process exit.
    await asyncio.wait(still_pending, timeout=cancel_wait)
    abandoned = sum(1 for t in still_pending if not t.done())
    if abandoned:
        logger.warning(
            "Cancellation wait exceeded %.1fs; %d task(s) ignored CancelledError "
            "and will be abandoned at process exit",
            cancel_wait,
            abandoned,
        )
