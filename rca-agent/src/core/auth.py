# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""OAuth2 Client Credentials authentication using authlib."""

import logging

import httpx
from authlib.integrations.httpx_client import AsyncOAuth2Client, OAuth2Client

from src.core.config import settings

logger = logging.getLogger(__name__)


class OAuth2ClientCredentialsAuth(httpx.Auth):
    """httpx.Auth implementation using authlib for OAuth2 client credentials.

    Handles automatic token fetching, caching, and refresh.
    """

    def __init__(self, token_url: str, client_id: str, client_secret: str):
        self.token_url = token_url
        self.client_id = client_id
        self.client_secret = client_secret
        self._token: dict | None = None

    def _ensure_token(self, client: OAuth2Client) -> None:
        """Fetch token if not present or expired."""
        if self._token is None or client.token.is_expired():
            self._token = client.fetch_token(self.token_url, grant_type="client_credentials")
            logger.debug("Fetched OAuth2 token, expires in %s", self._token.get("expires_in"))

    async def _async_ensure_token(self, client: AsyncOAuth2Client) -> None:
        """Async version of token fetching."""
        if self._token is None or client.token.is_expired():
            self._token = await client.fetch_token(self.token_url, grant_type="client_credentials")
            logger.debug("Fetched OAuth2 token, expires in %s", self._token.get("expires_in"))

    def sync_auth_flow(self, request: httpx.Request):
        """Sync auth flow."""
        client = OAuth2Client(
            client_id=self.client_id,
            client_secret=self.client_secret,
            token_endpoint_auth_method="client_secret_post",
        )
        client.token = self._token
        self._ensure_token(client)
        assert self._token is not None
        request.headers["Authorization"] = f"Bearer {self._token['access_token']}"
        yield request

    async def async_auth_flow(self, request: httpx.Request):
        """Async auth flow."""
        client = AsyncOAuth2Client(
            client_id=self.client_id,
            client_secret=self.client_secret,
            token_endpoint_auth_method="client_secret_post",
        )
        client.token = self._token
        await self._async_ensure_token(client)
        assert self._token is not None
        request.headers["Authorization"] = f"Bearer {self._token['access_token']}"
        yield request


def get_oauth2_auth() -> OAuth2ClientCredentialsAuth | None:
    """Get OAuth2 auth if credentials are configured."""
    if not all([settings.oauth_token_url, settings.oauth_client_id, settings.oauth_client_secret]):
        logger.debug("OAuth2 credentials not configured")
        return None

    logger.debug("OAuth2 authentication enabled: %s", settings.oauth_token_url)
    return OAuth2ClientCredentialsAuth(
        token_url=settings.oauth_token_url,
        client_id=settings.oauth_client_id,
        client_secret=settings.oauth_client_secret,
    )
