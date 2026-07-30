# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.jwt import (  # noqa: F401
    DisabledJWTValidator,
    JWTValidationError,
    JWTValidator,
    create_jwt_validator,
)

from src.config import settings

_jwt_validator: JWTValidator | DisabledJWTValidator | None = None


def get_jwt_validator() -> JWTValidator | DisabledJWTValidator:
    global _jwt_validator

    if _jwt_validator is None:
        _jwt_validator = create_jwt_validator(
            jwks_url=settings.jwt_jwks_url,
            issuer=settings.jwt_issuer,
            audience=settings.jwt_audience,
            refresh_interval=settings.jwt_jwks_refresh_interval,
            verify_ssl=not settings.jwks_url_tls_insecure_skip_verify,
            allow_unverified=settings.jwt_insecure_allow_unverified,
            service_name="portal-assistant",
        )

    return _jwt_validator
