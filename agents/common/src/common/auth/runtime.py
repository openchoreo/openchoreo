# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import inspect
import logging
from collections.abc import Awaitable, Callable
from typing import Annotated, Any

from fastapi import Depends, HTTPException, Request

from common.auth import dependencies as deps
from common.auth.authz_client import AuthzClient
from common.auth.authz_models import ResourceHierarchy, SubjectContext
from common.auth.jwt import DisabledJWTValidator, JWTValidator, create_jwt_validator
from common.auth.oauth_client import OAuth2ClientCredentialsAuth
from common.auth.oauth_client import check_oauth2_connection as _check_oauth2_connection
from common.config import CommonSettings

logger = logging.getLogger(__name__)

HierarchyExtractor = Callable[[Request], ResourceHierarchy | Awaitable[ResourceHierarchy]]


class MissingReportHierarchy(Exception):
    pass


def hierarchy_from_result(
    result: dict[str, Any], *, id_field: str = "reportId"
) -> ResourceHierarchy:
    namespace = result.get("namespace")
    project = result.get("project")
    if not namespace or not project:
        logger.error(
            "Resource %s has no project/namespace on record — refusing to authorize",
            result.get(id_field),
        )
        raise MissingReportHierarchy(result.get(id_field))
    return ResourceHierarchy(namespace=namespace, project=project)


def hierarchy_from_query(**fields: str) -> HierarchyExtractor:
    def extract(request: Request) -> ResourceHierarchy:
        return ResourceHierarchy(
            **{name: request.query_params.get(param) for name, param in fields.items()}
        )

    return extract


class AuthRuntime:
    def __init__(self, settings: CommonSettings, *, service_name: str):
        self._settings = settings
        self._service_name = service_name
        self._validator: JWTValidator | DisabledJWTValidator | None = None
        self._authz_client: AuthzClient | None = None
        self._auth_config: dict[str, Any] | None = None

    def get_jwt_validator(self) -> JWTValidator | DisabledJWTValidator:
        if self._validator is None:
            s = self._settings
            self._validator = create_jwt_validator(
                jwks_url=s.jwt_jwks_url,
                issuer=s.jwt_issuer,
                audience=s.jwt_audience,
                refresh_interval=s.jwt_jwks_refresh_interval,
                verify_ssl=s.jwks_verify_ssl,
                allow_unverified=s.jwt_insecure_allow_unverified,
                service_name=self._service_name,
            )
        return self._validator

    def get_authz_client(self) -> AuthzClient:
        if self._authz_client is None:
            s = self._settings
            self._authz_client = AuthzClient(
                base_url=s.authz_service_url,
                timeout=s.authz_timeout_seconds,
                verify_ssl=s.authz_verify_ssl,
            )
        return self._authz_client

    def get_auth_config(self) -> dict[str, Any]:
        if self._auth_config is None:
            self._auth_config = deps.load_auth_config(self._settings.auth_config_path)
        return self._auth_config

    def extract_subject_context(self, claims: dict[str, Any]) -> SubjectContext:
        return deps.extract_subject_context(
            claims, deps.get_subject_types(self.get_auth_config())
        )

    async def require_authn(self, request: Request) -> SubjectContext:
        return await deps.require_authn_with(
            request,
            validator=self.get_jwt_validator(),
            extract_subject=self.extract_subject_context,
        )

    def require_authz(
        self,
        action: str,
        resource_type: str,
        hierarchy: HierarchyExtractor | None = None,
    ) -> Callable[..., Awaitable[SubjectContext]]:
        extract = hierarchy or (lambda _request: ResourceHierarchy())

        async def dependency(
            request: Request,
            subject: Annotated[SubjectContext, Depends(self.require_authn)],
        ) -> SubjectContext:
            resolved = extract(request)
            if inspect.isawaitable(resolved):
                resolved = await resolved
            token = getattr(request.state, "bearer_token", None) or deps.extract_bearer_token(
                request
            )
            return await deps.enforce_authz(
                client=self.get_authz_client(),
                subject=subject,
                token=token,
                action=action,
                resource_type=resource_type,
                hierarchy=resolved,
            )

        return dependency

    async def authorize_result(
        self,
        request: Request,
        subject: SubjectContext,
        *,
        action: str,
        resource_type: str,
        result: dict[str, Any],
    ) -> SubjectContext:
        try:
            hierarchy = hierarchy_from_result(result)
        except MissingReportHierarchy as e:
            raise HTTPException(
                status_code=403, detail={"error": "FORBIDDEN", "message": "Access denied"}
            ) from e
        token = getattr(request.state, "bearer_token", None) or deps.extract_bearer_token(request)
        return await deps.enforce_authz(
            client=self.get_authz_client(),
            subject=subject,
            token=token,
            action=action,
            resource_type=resource_type,
            hierarchy=hierarchy,
        )

    def _require_oauth_settings(self) -> None:
        s = self._settings
        if not all([s.oauth_token_url, s.oauth_client_id, s.oauth_client_secret]):
            raise RuntimeError(
                "OAuth2 credentials not configured. "
                "Set OAUTH_TOKEN_URL, OAUTH_CLIENT_ID, and OAUTH_CLIENT_SECRET."
            )

    def get_oauth2_auth(self) -> OAuth2ClientCredentialsAuth:
        self._require_oauth_settings()
        s = self._settings
        return OAuth2ClientCredentialsAuth(
            token_url=s.oauth_token_url,
            client_id=s.oauth_client_id,
            client_secret=s.oauth_client_secret,
            scope=s.oauth_scope,
            verify_ssl=s.oauth_verify_ssl,
        )

    async def check_oauth2_connection(self) -> bool:
        self._require_oauth_settings()
        s = self._settings
        return await _check_oauth2_connection(
            token_url=s.oauth_token_url,
            client_id=s.oauth_client_id,
            client_secret=s.oauth_client_secret,
            scope=s.oauth_scope,
            verify_ssl=s.oauth_verify_ssl,
        )
