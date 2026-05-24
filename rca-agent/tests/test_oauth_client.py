# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for OAuth2 client configuration permutations in get_oauth2_auth and
check_oauth2_connection.

Three states are covered for both functions:
1. All credentials empty   → skip (return None / return True).
2. Credentials partially set → RuntimeError with a descriptive message.
3. All credentials present  → success path (auth object constructed / token fetch).
"""

import logging

import pytest

import src.auth.oauth_client as oauth_module
from src.auth.oauth_client import (
    OAuth2ClientCredentialsAuth,
    check_oauth2_connection,
    get_oauth2_auth,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _patch_settings(monkeypatch, *, token_url="", client_id="", client_secret="", scope=""):
    """Monkeypatch only the OAuth-related attributes on the module-level settings.

    All fields default to empty string, which represents the unconfigured state.
    """
    monkeypatch.setattr(oauth_module.settings, "oauth_token_url", token_url)
    monkeypatch.setattr(oauth_module.settings, "oauth_client_id", client_id)
    monkeypatch.setattr(oauth_module.settings, "oauth_client_secret", client_secret)
    monkeypatch.setattr(oauth_module.settings, "oauth_scope", scope)


# ===========================================================================
# get_oauth2_auth
# ===========================================================================


class TestGetOauth2Auth:
    """Tests for the get_oauth2_auth factory function."""

    def test_all_empty_returns_none(self, monkeypatch, caplog):
        """When no OAuth fields are set the function must skip auth silently."""
        _patch_settings(monkeypatch)
        with caplog.at_level(logging.DEBUG, logger="src.auth.oauth_client"):
            result = get_oauth2_auth()

        assert result is None
        assert "not configured" in caplog.text.lower()

    @pytest.mark.parametrize(
        "token_url, client_id, client_secret",
        [
            ("https://token.example.com", "", ""),
            ("", "my-client-id", ""),
            ("", "", "my-secret"),
            ("https://token.example.com", "my-client-id", ""),
            ("https://token.example.com", "", "my-secret"),
            ("", "my-client-id", "my-secret"),
        ],
    )
    def test_partial_config_raises(self, monkeypatch, token_url, client_id, client_secret):
        """Any partial combination of OAuth credentials must raise RuntimeError."""
        _patch_settings(
            monkeypatch,
            token_url=token_url,
            client_id=client_id,
            client_secret=client_secret,
        )
        with pytest.raises(RuntimeError, match="partially configured"):
            get_oauth2_auth()

    def test_all_present_returns_auth_object(self, monkeypatch):
        """When all credentials are set the function returns a valid auth object."""
        _patch_settings(
            monkeypatch,
            token_url="https://token.example.com/token",
            client_id="my-client",
            client_secret="my-secret",
            scope="openid",
        )
        result = get_oauth2_auth()

        assert isinstance(result, OAuth2ClientCredentialsAuth)
        assert result.token_url == "https://token.example.com/token"
        assert result.client_id == "my-client"
        assert result.client_secret == "my-secret"
        assert result.scope == "openid"


# ===========================================================================
# check_oauth2_connection
# ===========================================================================


class TestCheckOauth2Connection:
    """Tests for the check_oauth2_connection async function."""

    @pytest.mark.asyncio
    async def test_all_empty_skips_check(self, monkeypatch, caplog):
        """When no OAuth fields are set the connection check must be skipped."""
        _patch_settings(monkeypatch)
        with caplog.at_level(logging.DEBUG, logger="src.auth.oauth_client"):
            result = await check_oauth2_connection()

        assert result is True
        assert "not configured" in caplog.text.lower()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        "token_url, client_id, client_secret",
        [
            ("https://token.example.com", "", ""),
            ("", "my-client-id", ""),
            ("", "", "my-secret"),
            ("https://token.example.com", "my-client-id", ""),
        ],
    )
    async def test_partial_config_raises(self, monkeypatch, token_url, client_id, client_secret):
        """Any partial combination of OAuth credentials must raise RuntimeError."""
        _patch_settings(
            monkeypatch,
            token_url=token_url,
            client_id=client_id,
            client_secret=client_secret,
        )
        with pytest.raises(RuntimeError, match="partially configured"):
            await check_oauth2_connection()

    @pytest.mark.asyncio
    async def test_all_present_fetches_token(self, monkeypatch):
        """When all credentials are set the function successfully fetches a token.

        The underlying AsyncOAuth2Client.fetch_token is mocked so no real
        network call is made. Asserts that check_oauth2_connection returns True.
        """
        _patch_settings(
            monkeypatch,
            token_url="https://token.example.com/token",
            client_id="my-client",
            client_secret="my-secret",
        )

        fake_token = {"access_token": "fake-token", "expires_in": 3600}

        class _FakeAsyncOAuth2Client:
            """Minimal AsyncOAuth2Client stub that returns a pre-baked token."""

            def __init__(self, **kwargs):
                """Initialise with an empty token slot."""
                self.token = None

            async def fetch_token(self, url, **kwargs):
                """Return a fake token without making a real HTTP request."""
                return fake_token

            async def aclose(self):
                """No-op cleanup."""

        monkeypatch.setattr(oauth_module, "AsyncOAuth2Client", _FakeAsyncOAuth2Client)

        result = await check_oauth2_connection()
        assert result is True
