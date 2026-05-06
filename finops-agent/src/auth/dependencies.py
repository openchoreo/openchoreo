# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from pathlib import Path
from typing import Annotated, Any

import yaml
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
_auth_config: dict[str, Any] | None = None


def _load_auth_config() -> dict[str, Any]:
    global _auth_config
    if _auth_config is not None:
        return _auth_config

    config_path = settings.auth_config_path
    if not config_path or not Path(config_path).is_file():
        logger.warning("Auth config not found at %s, using defaults", config_path)
        _auth_config = {}
        return _auth_config

    with open(config_path) as f:
        _auth_config = yaml.safe_load(f) or {}

    logger.info("Loaded auth config from %s", config_path)
    return _auth_config


def _get_subject_types() -> list[dict[str, Any]]:
    config = _load_auth_config()
    types = list(config.get("auth", {}).get("subject_types", []))
    types.sort(key=lambda t: t.get("priority", 0))
    return types


def _get_jwt_claim(subject_type_config: dict[str, Any]) -> str | None:
    for mech in subject_type_config.get("auth_mechanisms", []):
        if mech.get("type") == "jwt":
            return mech.get("entitlement", {}).get("claim")
    return None


def _extract_entitlements(claims: dict[str, Any], claim: str) -> list[str] | None:
    if claim not in claims:
        return None
    value = claims[claim]
    if isinstance(value, list):
        filtered = [str(v) for v in value if v]
        return filtered if filtered else None
    if value:
        return [str(value)]
    return None


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
    for st in _get_subject_types():
        claim = _get_jwt_claim(st)
        if claim is None:
            continue
        entitlements = _extract_entitlements(claims, claim)
        if entitlements is None:
            continue
        return SubjectContext(
            type=st.get("type", "unknown"),
            entitlementClaim=claim,
            entitlementValues=entitlements,
        )

    # Fallback when no config matches
    sub = claims.get("sub", "")
    return SubjectContext(
        type="user",
        entitlementClaim="sub",
        entitlementValues=[sub] if sub else [],
    )


async def require_authn(request: Request) -> SubjectContext:
    validator = get_jwt_validator()

    if isinstance(validator, DisabledJWTValidator):
        logger.error("JWT authentication not configured - JWT_JWKS_URL is required")
        raise HTTPException(
            status_code=500,
            detail="Internal server error",
        )

    token = extract_bearer_token(request)
    if not token:
        raise HTTPException(
            status_code=401,
            detail="Missing 'Authorization' header. Expected format: Authorization: Bearer <token>",
        )

    try:
        claims = validator.validate(token)
        request.state.bearer_token = token
        logger.debug("Authentication successful", extra={"sub": claims.get("sub")})
        return extract_subject_context_from_claims(claims)
    except JWTValidationError as e:
        logger.warning("JWT validation failed", extra={"error_type": type(e).__name__})
        raise HTTPException(
            status_code=401,
            detail="Token validation failed",
        )


class AuthorizationChecker:
    def __init__(self, action: str, resource_type: str):
        self.action = action
        self.resource_type = resource_type

    async def __call__(
        self,
        request: Request,
        subject: Annotated[SubjectContext, Depends(require_authn)],
    ) -> SubjectContext:
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
                detail="Access denied",
            )

        logger.info("Authorization successful")
        return subject

    async def _extract_hierarchy(self, request: Request) -> ResourceHierarchy:
        return ResourceHierarchy()


class ReportAuthorizationChecker(AuthorizationChecker):
    async def _extract_hierarchy(self, request: Request) -> ResourceHierarchy:
        project = request.query_params.get("project")
        namespace = request.query_params.get("namespace")
        return ResourceHierarchy(
            namespace=namespace,
            project=project,
        )


require_reports_authz = ReportAuthorizationChecker(
    action="finopsreport:view", resource_type="finopsreport"
)
