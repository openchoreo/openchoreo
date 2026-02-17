# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any

from fastapi import Depends, HTTPException, Request

from src.auth.authz_client import AuthzClient
from src.auth.authz_models import (
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from src.auth.jwt import DisabledJWTValidator, JWTValidationError, get_jwt_validator
from src.config import settings

logger = logging.getLogger(__name__)

_authz_client: AuthzClient | None = None


def get_authz_client() -> AuthzClient:
    global _authz_client
    if _authz_client is None:
        _authz_client = AuthzClient(
            base_url=settings.authz_service_url,
            timeout=settings.authz_timeout_seconds,
            verify_ssl=not settings.tls_insecure_skip_verify,
        )
    return _authz_client


def extract_bearer_token(request: Request) -> str | None:
    auth_header = request.headers.get("Authorization")
    if not auth_header:
        return None

    parts = auth_header.split(" ", 1)
    if len(parts) != 2 or parts[0].lower() != "bearer":
        return None

    return parts[1]


def extract_subject_context_from_claims(claims: dict[str, Any]) -> SubjectContext:
    # Service account: has client_id but no sub
    if "client_id" in claims and "sub" not in claims:
        subject_type = "service"
        entitlement_claim = "client_id"
        entitlement_values = [claims.get("client_id", "")]
    else:
        # Regular user
        subject_type = "user"
        if "groups" in claims:
            entitlement_claim = "groups"
            groups = claims.get("groups", [])
            entitlement_values = groups if isinstance(groups, list) else [groups]
        else:
            entitlement_claim = "sub"
            entitlement_values = [claims.get("sub", "")]

    return SubjectContext(
        type=subject_type,
        entitlementClaim=entitlement_claim,
        entitlementValues=entitlement_values,
    )


async def require_authn(request: Request) -> SubjectContext | None:
    # Allow bypass if JWT is explicitly disabled (for local development)
    if settings.jwt_disabled:
        logger.debug("JWT authentication disabled, skipping validation")
        return None

    validator = get_jwt_validator()

    if isinstance(validator, DisabledJWTValidator):
        # JWKS URL not configured but JWT not explicitly disabled
        logger.error("JWT authentication not configured - JWT_JWKS_URL is required")
        raise HTTPException(
            status_code=500,
            detail={
                "error": "AUTH_NOT_CONFIGURED",
                "message": "JWT authentication is not configured. Set JWT_JWKS_URL.",
            },
        )

    token = extract_bearer_token(request)
    if not token:
        raise HTTPException(
            status_code=401,
            detail={"error": "MISSING_TOKEN", "message": "Authorization header required"},
        )

    try:
        claims = validator.validate(token)
        logger.debug("Authentication successful", extra={"sub": claims.get("sub")})
        return extract_subject_context_from_claims(claims)
    except JWTValidationError as e:
        logger.warning("JWT validation failed", extra={"error": str(e)})
        raise HTTPException(
            status_code=401,
            detail={"error": "INVALID_TOKEN", "message": str(e)},
        )


async def extract_request_body(request: Request) -> dict[str, Any]:
    if hasattr(request.state, "_parsed_body"):
        return request.state._parsed_body

    try:
        body = await request.json()
        request.state._parsed_body = body
        return body
    except Exception:
        return {}


class AuthorizationChecker:
    def __init__(self, action: str, resource_type: str):
        self.action = action
        self.resource_type = resource_type

    async def __call__(
        self,
        request: Request,
        subject: Annotated[SubjectContext | None, Depends(require_authn)],
    ) -> SubjectContext | None:
        # If authn returned None (JWT disabled for local dev), skip authz too
        if subject is None:
            return None

        client = get_authz_client()

        logger.info(
            "Authorization check: action=%s, resource_type=%s, subject_type=%s",
            self.action,
            self.resource_type,
            subject.type,
        )

        token = extract_bearer_token(request)

        hierarchy = await self._extract_hierarchy(request)
        logger.debug(
            "Resource hierarchy: project=%s, component=%s",
            hierarchy.project,
            hierarchy.component,
        )

        authz_request = EvaluateRequest(
            subjectContext=subject,
            resource=Resource(
                type=self.resource_type,
                id="",
                hierarchy=hierarchy,
            ),
            action=self.action,
            context={},
        )

        decision = await client.evaluate(authz_request, token)
        logger.info("Authz decision: allowed=%s", decision.decision)

        if not decision.decision:
            logger.warning(
                "Access denied: action=%s, resource_type=%s",
                self.action,
                self.resource_type,
            )
            raise HTTPException(
                status_code=403,
                detail={"error": "FORBIDDEN", "message": "Access denied"},
            )

        logger.info("Authorization successful")
        return subject

    async def _extract_hierarchy(self, request: Request) -> ResourceHierarchy:
        body = await extract_request_body(request)
        return ResourceHierarchy(
            project=body.get("projectUid"),
            component=body.get("componentUid"),
        )


class ReportAuthorizationChecker(AuthorizationChecker):
    async def _extract_hierarchy(self, request: Request) -> ResourceHierarchy:
        project = request.path_params.get("project_id")
        return ResourceHierarchy(
            project=str(project) if project else None,
        )


require_chat_authz = AuthorizationChecker(action="rcareport:view", resource_type="rcareport")
require_reports_authz = ReportAuthorizationChecker(
    action="rcareport:view", resource_type="rcareport"
)
