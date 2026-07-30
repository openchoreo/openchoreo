# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Shared authn/authz plumbing, settings-free: each agent's dependencies
shim passes in its validator, authz client, and auth-config path."""

import logging
from collections.abc import Callable
from pathlib import Path
from typing import Any

import yaml
from fastapi import HTTPException, Request

from common.auth.authz_client import AuthzClient
from common.auth.authz_errors import (
    AuthzForbidden,
    AuthzServiceUnavailable,
    AuthzUnauthorized,
)
from common.auth.authz_models import (
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from common.auth.jwt import DisabledJWTValidator, JWTValidationError, JWTValidator

logger = logging.getLogger(__name__)


def load_auth_config(config_path: str | None) -> dict[str, Any]:
    if not config_path or not Path(config_path).is_file():
        logger.warning("Auth config not found at %s, using defaults", config_path)
        return {}

    with open(config_path, encoding="utf-8") as f:
        config = yaml.safe_load(f) or {}

    if not isinstance(config, dict):
        raise ValueError(f"Auth config at {config_path} must be a YAML mapping")
    subject_types = config.get("auth", {}).get("subject_types", [])
    if not isinstance(subject_types, list) or not all(isinstance(t, dict) for t in subject_types):
        raise ValueError(f"auth.subject_types in {config_path} must be a list of mappings")

    logger.info("Loaded auth config from %s", config_path)
    return config


def get_subject_types(auth_config: dict[str, Any]) -> list[dict[str, Any]]:
    types = list(auth_config.get("auth", {}).get("subject_types", []))
    types.sort(key=lambda t: t.get("priority", 0))
    return types


def get_jwt_claim(subject_type_config: dict[str, Any]) -> str | None:
    for mech in subject_type_config.get("auth_mechanisms", []):
        if mech.get("type") == "jwt":
            return mech.get("entitlement", {}).get("claim")
    return None


def extract_entitlements(claims: dict[str, Any], claim: str) -> list[str] | None:
    if claim not in claims:
        return None
    value = claims[claim]
    if isinstance(value, list):
        return [str(v) for v in value if v]
    if value:
        return [str(value)]
    return []


def extract_bearer_token(request: Request) -> str | None:
    auth_header = request.headers.get("Authorization")
    if not auth_header:
        return None

    parts = auth_header.split(" ", 1)
    if len(parts) != 2 or parts[0].lower() != "bearer":
        return None

    return parts[1]


def extract_subject_context(
    claims: dict[str, Any], subject_types: list[dict[str, Any]]
) -> SubjectContext:
    for st in subject_types:
        claim = get_jwt_claim(st)
        if claim is None:
            continue
        entitlements = extract_entitlements(claims, claim)
        if entitlements is None:
            continue
        return SubjectContext(
            type=st.get("type", "unknown"),
            entitlementClaim=claim,
            entitlementValues=entitlements,
        )

    sub = claims.get("sub", "")
    return SubjectContext(
        type="user",
        entitlementClaim="sub",
        entitlementValues=[sub] if sub else [],
    )


async def require_authn_with(
    request: Request,
    *,
    validator: JWTValidator | DisabledJWTValidator,
    extract_subject: Callable[[dict[str, Any]], SubjectContext],
) -> SubjectContext:
    """Validate the JWT; stash bearer token and sub on request.state."""
    if isinstance(validator, DisabledJWTValidator):
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
        claims = await validator.validate(token)
        request.state.bearer_token = token
        request.state.user_sub = claims.get("sub", "")
        logger.debug("Authentication successful")
        return extract_subject(claims)
    except JWTValidationError as e:
        logger.warning("JWT validation failed", extra={"error": str(e)})
        raise HTTPException(
            status_code=401,
            detail={"error": "INVALID_TOKEN", "message": str(e)},
        ) from e


async def enforce_authz(
    *,
    client: AuthzClient,
    subject: SubjectContext,
    token: str | None,
    action: str,
    resource_type: str,
    hierarchy: ResourceHierarchy,
) -> SubjectContext:
    """Evaluate one authz decision; map domain errors and denies to HTTP."""
    logger.info(
        "Authorization check: action=%s, resource_type=%s, subject_type=%s",
        action,
        resource_type,
        subject.type,
    )

    authz_request = EvaluateRequest(
        subjectContext=subject,
        resource=Resource(
            type=resource_type,
            id="",
            hierarchy=hierarchy,
        ),
        action=action,
        context={},
    )

    try:
        decision = await client.evaluate(authz_request, token)
    except AuthzUnauthorized as e:
        raise HTTPException(
            status_code=401,
            detail={"error": "UNAUTHORIZED", "message": str(e)},
        ) from e
    except AuthzForbidden as e:
        raise HTTPException(
            status_code=403,
            detail={"error": "FORBIDDEN", "message": str(e)},
        ) from e
    except AuthzServiceUnavailable as e:
        raise HTTPException(
            status_code=503,
            detail={"error": "SERVICE_UNAVAILABLE", "message": str(e)},
        ) from e

    logger.info("Authz decision: allowed=%s", decision.decision)

    if not decision.decision:
        logger.warning(
            "Access denied: action=%s, resource_type=%s",
            action,
            resource_type,
        )
        raise HTTPException(
            status_code=403,
            detail={"error": "FORBIDDEN", "message": "Access denied"},
        )

    return subject
