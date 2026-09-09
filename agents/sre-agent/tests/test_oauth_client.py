# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for OAuth2 client authentication: client secret and JWT client assertion."""

import contextlib
import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs

import httpx
import pytest

from common.auth.oauth_client import (
    CLIENT_ASSERTION_TYPE,
    OAuth2ClientCredentialsAuth,
    _grant_kwargs,
    _read_client_assertion,
)


@pytest.fixture
def token_endpoint():
    """Serve a token endpoint that records the form parameters of each request."""
    received: list[dict[str, str]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers["Content-Length"])
            params = parse_qs(self.rfile.read(length).decode())
            received.append({k: v[0] for k, v in params.items()})
            body = json.dumps(
                {"access_token": "tok", "token_type": "Bearer", "expires_in": 3600}
            ).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    server = HTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}/token", received
    finally:
        server.shutdown()
        server.server_close()


def write_assertion(tmp_path, contents):
    path = tmp_path / "azure-identity-token"
    path.write_text(contents)
    return str(path)


async def run_auth_flow(auth):
    """Drive the httpx auth flow once and return the prepared request."""
    flow = auth.async_auth_flow(httpx.Request("GET", "http://openchoreo.invalid/api"))
    request = await flow.__anext__()
    with contextlib.suppress(StopAsyncIteration):
        await flow.__anext__()
    return request


def test_grant_kwargs_uses_client_secret_when_no_assertion_file():
    kwargs = _grant_kwargs("api:read", "")

    assert kwargs == {"grant_type": "client_credentials", "scope": "api:read"}


def test_grant_kwargs_includes_trimmed_client_assertion(tmp_path):
    path = write_assertion(tmp_path, "header.payload.signature\n")

    kwargs = _grant_kwargs("", path)

    assert kwargs["grant_type"] == "client_credentials"
    assert kwargs["client_assertion_type"] == CLIENT_ASSERTION_TYPE
    assert kwargs["client_assertion"] == "header.payload.signature"


def test_grant_kwargs_rereads_assertion_file(tmp_path):
    path = write_assertion(tmp_path, "first-assertion")
    assert _grant_kwargs("", path)["client_assertion"] == "first-assertion"

    # Simulate the platform rotating the projected token in place.
    write_assertion(tmp_path, "second-assertion")

    assert _grant_kwargs("", path)["client_assertion"] == "second-assertion"


def test_read_client_assertion_rejects_empty_file(tmp_path):
    path = write_assertion(tmp_path, "  \n")

    with pytest.raises(RuntimeError, match="is empty"):
        _read_client_assertion(path)


def test_read_client_assertion_rejects_missing_file(tmp_path):
    with pytest.raises(FileNotFoundError):
        _read_client_assertion(str(tmp_path / "does-not-exist"))


async def test_auth_flow_sends_client_assertion(token_endpoint, tmp_path):
    token_url, received = token_endpoint
    auth = OAuth2ClientCredentialsAuth(
        token_url=token_url,
        client_id="my-client",
        client_secret="unused-secret",
        scope="api://openchoreo-api/.default",
        client_assertion_file=write_assertion(tmp_path, "header.payload.signature"),
    )

    request = await run_auth_flow(auth)

    assert request.headers["Authorization"] == "Bearer tok"
    assert len(received) == 1
    body = received[0]
    assert body["client_id"] == "my-client"
    assert body["grant_type"] == "client_credentials"
    assert body["scope"] == "api://openchoreo-api/.default"
    assert body["client_assertion_type"] == CLIENT_ASSERTION_TYPE
    assert body["client_assertion"] == "header.payload.signature"
    assert "client_secret" not in body


async def test_auth_flow_sends_client_secret_without_assertion_file(token_endpoint):
    token_url, received = token_endpoint
    auth = OAuth2ClientCredentialsAuth(
        token_url=token_url,
        client_id="my-client",
        client_secret="my-secret",
    )

    request = await run_auth_flow(auth)

    assert request.headers["Authorization"] == "Bearer tok"
    body = received[0]
    assert body["client_secret"] == "my-secret"
    assert "client_assertion" not in body
    assert "client_assertion_type" not in body


async def test_auth_flow_caches_token_across_requests(token_endpoint, tmp_path):
    token_url, received = token_endpoint
    auth = OAuth2ClientCredentialsAuth(
        token_url=token_url,
        client_id="my-client",
        client_secret="",
        client_assertion_file=write_assertion(tmp_path, "header.payload.signature"),
    )

    await run_auth_flow(auth)
    await run_auth_flow(auth)

    assert len(received) == 1
