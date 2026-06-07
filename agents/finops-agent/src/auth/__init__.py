# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.auth.authz_client import AuthzClient
from src.auth.authz_models import (
    Decision,
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from src.auth.bearer import BearerTokenAuth
from src.auth.dependencies import require_authn, require_reports_authz, require_reports_update_authz
from src.auth.jwt import JWTValidationError, JWTValidator, get_jwt_validator
from src.auth.oauth_client import OAuth2ClientCredentialsAuth, check_oauth2_connection, get_oauth2_auth

__all__ = [
    # JWT
    "JWTValidator",
    "JWTValidationError",
    "get_jwt_validator",
    # OAuth2 / Auth strategies
    "BearerTokenAuth",
    "OAuth2ClientCredentialsAuth",
    "get_oauth2_auth",
    "check_oauth2_connection",
    # Authz
    "AuthzClient",
    "Decision",
    "EvaluateRequest",
    "Resource",
    "ResourceHierarchy",
    "SubjectContext",
    # Dependencies
    "require_authn",
    "require_reports_authz",
    "require_reports_update_authz",
]
