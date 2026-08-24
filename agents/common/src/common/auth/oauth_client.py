# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import threading
import time
from urllib.parse import urlparse, urlunparse

import httpx
from authlib.integrations.httpx_client import AsyncOAuth2Client, OAuth2Client

logger = logging.getLogger(__name__)

# RFC 7523 assertion type for JWT client authentication.
CLIENT_ASSERTION_TYPE = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"


def _read_client_assertion(path: str) -> str:
    """Read a JWT client assertion from disk.

    Read on every token request rather than cached: platforms that project an identity
    token, such as Azure Workload Identity and EKS IRSA, rotate the file in place well
    before the pod restarts, so a cached copy would expire.
    """
    with open(path, encoding="utf-8") as f:
        assertion = f.read().strip()
    if not assertion:
        raise RuntimeError(f"Client assertion file '{path}' is empty")
    return assertion


def _auth_method(client_assertion_file: str) -> str:
    """Client authentication method for the token endpoint.

    A client assertion is passed as grant parameters, so the client itself authenticates
    with "none" and only contributes client_id to the request body.
    """
    return "none" if client_assertion_file else "client_secret_post"


def _grant_kwargs(scope: str, client_assertion_file: str) -> dict:
    """Build the client credentials grant parameters for a token request."""
    kwargs = {"grant_type": "client_credentials"}
    if scope:
        kwargs["scope"] = scope
    if client_assertion_file:
        kwargs["client_assertion_type"] = CLIENT_ASSERTION_TYPE
        kwargs["client_assertion"] = _read_client_assertion(client_assertion_file)
    return kwargs


def _sanitize_url(url: str) -> str:
    """Remove credentials and query parameters from URL for safe logging."""
    parsed = urlparse(url)
    # Remove username, password, query params, and fragment
    sanitized = urlunparse((parsed.scheme, parsed.netloc, parsed.path, "", "", ""))
    return sanitized


class OAuth2ClientCredentialsAuth(httpx.Auth):
    def __init__(
        self,
        token_url: str,
        client_id: str,
        client_secret: str,
        scope: str = "",
        verify_ssl: bool = True,
        client_assertion_file: str = "",
    ):
        self.token_url = token_url
        self.client_id = client_id
        self.client_secret = client_secret
        self.scope = scope
        self.client_assertion_file = client_assertion_file
        self._token: dict | None = None
        self.verify_ssl = verify_ssl
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
            kwargs = _grant_kwargs(self.scope, self.client_assertion_file)
            self._token = client.fetch_token(self.token_url, **kwargs)
            if self._token is None:
                raise RuntimeError("Failed to fetch OAuth2 token")
            logger.debug("Fetched OAuth2 token, expires in %s", self._token.get("expires_in"))
        return self._token

    async def _async_ensure_token(self, client: AsyncOAuth2Client) -> dict:
        if self._token is None or client.token.is_expired():
            kwargs = _grant_kwargs(self.scope, self.client_assertion_file)
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
                with OAuth2Client(
                    client_id=self.client_id,
                    client_secret=self.client_secret,
                    token_endpoint_auth_method=_auth_method(self.client_assertion_file),
                    verify=self.verify_ssl,
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
                async with AsyncOAuth2Client(
                    client_id=self.client_id,
                    client_secret=self.client_secret,
                    token_endpoint_auth_method=_auth_method(self.client_assertion_file),
                    verify=self.verify_ssl,
                ) as client:
                    client.token = self._token
                    token = await self._async_ensure_token(client)

        # Set header and yield outside the lock and context manager
        request.headers["Authorization"] = f"Bearer {token['access_token']}"
        yield request


async def check_oauth2_connection(
    *,
    token_url: str,
    client_id: str,
    client_secret: str,
    scope: str = "",
    verify_ssl: bool = True,
    client_assertion_file: str = "",
) -> bool:
    client = AsyncOAuth2Client(
        client_id=client_id,
        client_secret=client_secret,
        token_endpoint_auth_method=_auth_method(client_assertion_file),
        verify=verify_ssl,
    )

    try:
        kwargs = _grant_kwargs(scope, client_assertion_file)
        token = await client.fetch_token(token_url, **kwargs)
        logger.debug("OAuth2 token fetch successful, expires in %s", token.get("expires_in"))
        return True
    except Exception as e:
        logger.error("Failed to fetch OAuth2 token from %s", _sanitize_url(token_url))
        raise RuntimeError(
            "Failed to fetch OAuth2 token. Check credentials and token endpoint configuration."
        ) from e
    finally:
        await client.aclose()
