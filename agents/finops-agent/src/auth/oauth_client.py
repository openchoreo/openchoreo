# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import threading
import time
from urllib.parse import urlparse, urlunparse

import httpx
from authlib.integrations.httpx_client import AsyncOAuth2Client, OAuth2Client

from src.config import settings

logger = logging.getLogger(__name__)


def _sanitize_url(url: str) -> str:
    """Remove credentials and query parameters from URL for safe logging."""
    parsed = urlparse(url)
    # Remove username, password, query params, and fragment
    sanitized = urlunparse((parsed.scheme, parsed.netloc, parsed.path, "", "", ""))
    return sanitized


class OAuth2ClientCredentialsAuth(httpx.Auth):
    def __init__(self, token_url: str, client_id: str, client_secret: str, scope: str = ""):
        self.token_url = token_url
        self.client_id = client_id
        self.client_secret = client_secret
        self.scope = scope
        self._token: dict | None = None
        self._sync_lock = threading.Lock()
        self._async_lock = asyncio.Lock()

    def _is_token_valid(self) -> bool:
        """Check if token exists and is not expired."""
        if self._token is None:
            return False
        expires_at = self._token.get("expires_at")
        if expires_at is None:
            return False
        return time.time() < expires_at

    def _ensure_token(self, client: OAuth2Client) -> dict:
        if self._token is None or client.token.is_expired():
            kwargs = {"grant_type": "client_credentials"}
            if self.scope:
                kwargs["scope"] = self.scope
            self._token = client.fetch_token(self.token_url, **kwargs)
            if self._token is None:
                raise RuntimeError("Failed to fetch OAuth2 token")
            logger.debug("Fetched OAuth2 token, expires in %s", self._token.get("expires_in"))
        return self._token

    async def _async_ensure_token(self, client: AsyncOAuth2Client) -> dict:
        if self._token is None or client.token.is_expired():
            kwargs = {"grant_type": "client_credentials"}
            if self.scope:
                kwargs["scope"] = self.scope
            self._token = await client.fetch_token(self.token_url, **kwargs)
            if self._token is None:
                raise RuntimeError("Failed to fetch OAuth2 token")
            logger.debug("Fetched OAuth2 token, expires in %s", self._token.get("expires_in"))
        return self._token

    def sync_auth_flow(self, request: httpx.Request):
        # Short-circuit if token is still valid
        if self._is_token_valid():
            request.headers["Authorization"] = f"Bearer {self._token['access_token']}"
            yield request
            return

        # Need to refresh - use lock to prevent concurrent refreshes
        with self._sync_lock:
            # Double-check after acquiring lock
            if self._is_token_valid():
                token = self._token
            else:
                # Create client in context manager so it gets closed
                verify = not settings.tls_insecure_skip_verify
                with OAuth2Client(
                    client_id=self.client_id,
                    client_secret=self.client_secret,
                    token_endpoint_auth_method="client_secret_post",
                    verify=verify,
                ) as client:
                    client.token = self._token
                    token = self._ensure_token(client)

        # Set header and yield outside the lock and context manager
        request.headers["Authorization"] = f"Bearer {token['access_token']}"
        yield request

    async def async_auth_flow(self, request: httpx.Request):
        # Short-circuit if token is still valid
        if self._is_token_valid():
            request.headers["Authorization"] = f"Bearer {self._token['access_token']}"
            yield request
            return

        # Need to refresh - use lock to prevent concurrent refreshes
        async with self._async_lock:
            # Double-check after acquiring lock
            if self._is_token_valid():
                token = self._token
            else:
                # Create client in context manager so it gets closed
                verify = not settings.tls_insecure_skip_verify
                async with AsyncOAuth2Client(
                    client_id=self.client_id,
                    client_secret=self.client_secret,
                    token_endpoint_auth_method="client_secret_post",
                    verify=verify,
                ) as client:
                    client.token = self._token
                    token = await self._async_ensure_token(client)

        # Set header and yield outside the lock and context manager
        request.headers["Authorization"] = f"Bearer {token['access_token']}"
        yield request


def get_oauth2_auth() -> OAuth2ClientCredentialsAuth:
    if not all([settings.oauth_token_url, settings.oauth_client_id, settings.oauth_client_secret]):
        raise RuntimeError(
            "OAuth2 credentials not configured. "
            "Set OAUTH_TOKEN_URL, OAUTH_CLIENT_ID, and OAUTH_CLIENT_SECRET."
        )

    logger.debug("OAuth2 authentication enabled: %s", _sanitize_url(settings.oauth_token_url))
    return OAuth2ClientCredentialsAuth(
        token_url=settings.oauth_token_url,
        client_id=settings.oauth_client_id,
        client_secret=settings.oauth_client_secret,
        scope=settings.oauth_scope,
    )


async def check_oauth2_connection() -> bool:
    if not all([settings.oauth_token_url, settings.oauth_client_id, settings.oauth_client_secret]):
        raise RuntimeError(
            "OAuth2 credentials not configured. "
            "Set OAUTH_TOKEN_URL, OAUTH_CLIENT_ID, and OAUTH_CLIENT_SECRET."
        )

    verify = not settings.tls_insecure_skip_verify
    client = AsyncOAuth2Client(
        client_id=settings.oauth_client_id,
        client_secret=settings.oauth_client_secret,
        token_endpoint_auth_method="client_secret_post",
        verify=verify,
    )

    try:
        kwargs = {"grant_type": "client_credentials"}
        if settings.oauth_scope:
            kwargs["scope"] = settings.oauth_scope
        token = await client.fetch_token(settings.oauth_token_url, **kwargs)
        logger.debug("OAuth2 token fetch successful, expires in %s", token.get("expires_in"))
        return True
    except Exception as e:
        logger.error("Failed to fetch OAuth2 token from %s", _sanitize_url(settings.oauth_token_url))
        raise RuntimeError("Failed to fetch OAuth2 token. Check credentials and token endpoint configuration.") from e
    finally:
        await client.aclose()
