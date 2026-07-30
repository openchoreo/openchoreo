# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Annotated, Any

from fastapi import Depends, Request

from common.auth import dependencies as common_deps
from common.auth.authz_client import AuthzClient
from common.auth.authz_models import ResourceHierarchy, SubjectContext
from common.auth.dependencies import extract_bearer_token  # noqa: F401
from src.auth.jwt import get_jwt_validator
from src.config import settings

logger = logging.getLogger(__name__)

_authz_client: AuthzClient | None = None
_auth_config: dict[str, Any] | None = None


def _load_auth_config() -> dict[str, Any]:
    global _auth_config
    if _auth_config is None:
        _auth_config = common_deps.load_auth_config(settings.auth_config_path)
    return _auth_config


def _get_subject_types() -> list[dict[str, Any]]:
    return common_deps.get_subject_types(_load_auth_config())


def _get_jwt_claim(subject_type_config: dict[str, Any]) -> str | None:
    return common_deps.get_jwt_claim(subject_type_config)


def _extract_entitlements(claims: dict[str, Any], claim: str) -> list[str] | None:
    return common_deps.extract_entitlements(claims, claim)


def get_authz_client() -> AuthzClient:
    global _authz_client
    if _authz_client is None:
        _authz_client = AuthzClient(
            base_url=settings.authz_service_url,
            timeout=settings.authz_timeout_seconds,
            verify_ssl=not settings.tls_insecure_skip_verify,
        )
    return _authz_client


def extract_subject_context_from_claims(claims: dict[str, Any]) -> SubjectContext:
    return common_deps.extract_subject_context(claims, _get_subject_types())


async def require_authn(request: Request) -> SubjectContext:
    return await common_deps.require_authn_with(
        request,
        validator=get_jwt_validator(),
        extract_subject=extract_subject_context_from_claims,
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
        token = getattr(request.state, "bearer_token", None) or extract_bearer_token(request)
        return await common_deps.enforce_authz(
            client=get_authz_client(),
            subject=subject,
            token=token,
            action=self.action,
            resource_type=self.resource_type,
            hierarchy=await self._extract_hierarchy(request),
        )

    async def _extract_hierarchy(self, request: Request) -> ResourceHierarchy:
        return ResourceHierarchy()


async def extract_request_body(request: Request) -> dict[str, Any]:
    if hasattr(request.state, "_parsed_body"):
        return request.state._parsed_body

    try:
        body = await request.json()
        request.state._parsed_body = body
        return body
    except Exception:
        return {}


class BodyHierarchyAuthorizationChecker(AuthorizationChecker):
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


require_chat_authz = BodyHierarchyAuthorizationChecker(
    action="rcareport:view", resource_type="rcareport"
)
require_reports_authz = ReportAuthorizationChecker(
    action="rcareport:view", resource_type="rcareport"
)
require_reports_update_authz = ReportAuthorizationChecker(
    action="rcareport:update", resource_type="rcareport"
)
