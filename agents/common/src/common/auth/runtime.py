# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import inspect
import logging
from collections.abc import Awaitable, Callable
from typing import Annotated, Any

from fastapi import Depends, Request

from common.auth import dependencies as deps
from common.auth.authz_client import AuthzClient
from common.auth.authz_models import ResourceHierarchy, SubjectContext
from common.auth.jwt import DisabledJWTValidator, JWTValidator, create_jwt_validator
from common.auth.oauth_client import OAuth2ClientCredentialsAuth
from common.auth.oauth_client import check_oauth2_connection as _check_oauth2_connection
from common.config import CommonSettings

logger = logging.getLogger(__name__)

HierarchyExtractor = Callable[[Request], ResourceHierarchy | Awaitable[ResourceHierarchy]]


def hierarchy_from_query(**fields: str) -> HierarchyExtractor:
    def extract(request: Request) -> ResourceHierarchy:
        return ResourceHierarchy(
            **{name: request.query_params.get(param) for name, param in fields.items()}
        )

    return extract


def hierarchy_from_path(**fields: str) -> HierarchyExtractor:
    def extract(request: Request) -> ResourceHierarchy:
        values = {name: request.path_params.get(param) for name, param in fields.items()}
        return ResourceHierarchy(
            **{k: str(v) if v is not None else None for k, v in values.items()}
        )

    return extract


async def read_cached_body(request: Request) -> dict[str, Any]:
    if hasattr(request.state, "_parsed_body"):
        return request.state._parsed_body

    try:
        body = await request.json()
        request.state._parsed_body = body
        return body
    except Exception as e:
        logger.warning(
            "Failed to parse request body for hierarchy extraction: %s", type(e).__name__
        )
        return {}


def hierarchy_from_body(**fields: str) -> HierarchyExtractor:
    async def extract(request: Request) -> ResourceHierarchy:
        body = await read_cached_body(request)
        return ResourceHierarchy(
            **{name: body.get(param) for name, param in fields.items()}
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

    def checker(
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
