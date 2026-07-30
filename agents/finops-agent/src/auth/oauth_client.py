# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from common.auth.oauth_client import (  # noqa: F401
    OAuth2ClientCredentialsAuth,
    _sanitize_url,
)
from common.auth.oauth_client import (
    check_oauth2_connection as _check_oauth2_connection,
)
from src.config import settings

logger = logging.getLogger(__name__)


def _require_oauth_settings() -> None:
    if not all(
        [settings.oauth_token_url, settings.oauth_client_id, settings.oauth_client_secret]
    ):
        raise RuntimeError(
            "OAuth2 credentials not configured. "
            "Set OAUTH_TOKEN_URL, OAUTH_CLIENT_ID, and OAUTH_CLIENT_SECRET."
        )


def get_oauth2_auth() -> OAuth2ClientCredentialsAuth:
    _require_oauth_settings()
    logger.debug("OAuth2 authentication enabled: %s", _sanitize_url(settings.oauth_token_url))
    return OAuth2ClientCredentialsAuth(
        token_url=settings.oauth_token_url,
        client_id=settings.oauth_client_id,
        client_secret=settings.oauth_client_secret,
        scope=settings.oauth_scope,
        verify_ssl=not settings.tls_insecure_skip_verify,
    )


async def check_oauth2_connection() -> bool:
    _require_oauth_settings()
    return await _check_oauth2_connection(
        token_url=settings.oauth_token_url,
        client_id=settings.oauth_client_id,
        client_secret=settings.oauth_client_secret,
        scope=settings.oauth_scope,
        verify_ssl=not settings.tls_insecure_skip_verify,
    )
