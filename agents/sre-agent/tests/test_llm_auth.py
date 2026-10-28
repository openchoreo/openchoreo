# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the LLM credential: a static API key or an Entra ID token provider."""

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest
from azure.core.credentials import AccessToken
from langchain.chat_models import init_chat_model
from pydantic import ValidationError

import common.llm_auth as llm_auth
from common.config import CommonSettings

DEFAULT_SCOPE = "https://cognitiveservices.azure.com/.default"


class FakeCredential:
    """Stands in for DefaultAzureCredential: issues numbered tokens and records scopes."""

    def __init__(self):
        self.lifetime_seconds = 3600
        self.scopes = []
        self.options = None

    def get_token(self, *scopes, **kwargs):
        self.scopes.append(scopes)
        return AccessToken(f"tok-{len(self.scopes)}", int(time.time()) + self.lifetime_seconds)


@pytest.fixture
def credential(monkeypatch):
    cred = FakeCredential()

    def build(**kwargs):
        cred.options = kwargs
        return cred

    monkeypatch.setattr(llm_auth, "DefaultAzureCredential", build)
    llm_auth._azure_token_provider.cache_clear()
    yield cred
    llm_auth._azure_token_provider.cache_clear()


@pytest.fixture
def chat_endpoint():
    """Serve an OpenAI-compatible chat completions endpoint that records Authorization headers."""
    authorizations: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            self.rfile.read(int(self.headers["Content-Length"]))
            authorizations.append(self.headers["Authorization"])
            body = json.dumps(
                {
                    "id": "chatcmpl-test",
                    "object": "chat.completion",
                    "created": 0,
                    "model": "gpt-4o-mini",
                    "choices": [
                        {
                            "index": 0,
                            "message": {"role": "assistant", "content": "Pong"},
                            "finish_reason": "stop",
                        }
                    ],
                    "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
                }
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
        yield f"http://127.0.0.1:{server.server_port}/openai/v1", authorizations
    finally:
        server.shutdown()
        server.server_close()


def test_defaults_to_api_key_auth_with_the_cognitive_services_scope():
    settings = CommonSettings()
    assert settings.llm_auth == "api_key"
    assert settings.llm_token_scope == DEFAULT_SCOPE


def test_rejects_an_unknown_auth_mode():
    with pytest.raises(ValidationError):
        CommonSettings(llm_auth="managed_identity")


def test_api_key_auth_returns_the_static_key(credential):
    settings = CommonSettings(llm_auth="api_key")
    assert llm_auth.get_llm_api_key(settings, "sk-test") == "sk-test"
    assert credential.scopes == []


def test_azure_auth_returns_a_token_provider_for_the_default_scope(credential):
    provider = llm_auth.get_llm_api_key(CommonSettings(llm_auth="azure"), "ignored")
    assert provider() == "tok-1"
    assert credential.scopes == [(DEFAULT_SCOPE,)]


def test_azure_auth_uses_the_configured_scope(credential):
    settings = CommonSettings(llm_auth="azure", llm_token_scope="api://llm-gateway/.default")
    llm_auth.get_llm_api_key(settings, "")()
    assert credential.scopes == [("api://llm-gateway/.default",)]


def test_azure_auth_shares_one_provider_that_caches_the_token(credential):
    settings = CommonSettings(llm_auth="azure")
    first = llm_auth.get_llm_api_key(settings, "")
    second = llm_auth.get_llm_api_key(settings, "")
    assert first is second
    assert [first(), second(), first()] == ["tok-1", "tok-1", "tok-1"]
    assert len(credential.scopes) == 1


def test_azure_auth_bounds_token_requests(credential):
    llm_auth.get_llm_api_key(CommonSettings(llm_auth="azure"), "")
    assert credential.options == {"connection_timeout": 10, "read_timeout": 10, "retry_total": 2}


def test_azure_auth_refreshes_a_token_close_to_expiry(credential):
    # Tokens inside the provider's refresh window (5 minutes) are replaced on the next call.
    credential.lifetime_seconds = 60
    provider = llm_auth.get_llm_api_key(CommonSettings(llm_auth="azure"), "")
    assert [provider(), provider()] == ["tok-1", "tok-2"]


async def test_chat_model_sends_the_current_token_on_every_request(credential, chat_endpoint):
    url, authorizations = chat_endpoint
    credential.lifetime_seconds = 60
    model = init_chat_model(
        model="openai:gpt-4o-mini",
        api_key=llm_auth.get_llm_api_key(CommonSettings(llm_auth="azure"), ""),
        base_url=url,
        max_retries=0,
    )

    assert model.invoke("ping").content == "Pong"
    assert (await model.ainvoke("ping")).content == "Pong"
    assert authorizations == ["Bearer tok-1", "Bearer tok-2"]
