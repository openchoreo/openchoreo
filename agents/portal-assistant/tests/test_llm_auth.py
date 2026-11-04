# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the LLM credential in the model factory and the startup config check."""

import pytest

import common.llm_auth as llm_auth
import src.clients.llm as llm
import src.main as main


@pytest.fixture
def init_calls(monkeypatch):
    calls = []

    def fake_init(**kwargs):
        calls.append(kwargs["api_key"])
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)
    return calls


def test_get_model_passes_the_static_key_by_default(init_calls, monkeypatch):
    monkeypatch.setattr(llm.settings, "portal_assistant_llm_api_key", "static-key")

    llm.get_model("openai:gpt-4o-mini")

    assert init_calls == ["static-key"]


def test_get_model_passes_a_token_provider_with_azure_auth(init_calls, monkeypatch):
    monkeypatch.setattr(llm.settings, "llm_auth", "azure")
    monkeypatch.setattr(llm.settings, "portal_assistant_llm_api_key", "")
    monkeypatch.setattr(llm_auth, "_azure_token_provider", lambda scope: f"provider:{scope}")

    llm.get_model("openai:gpt-4o-mini")

    assert init_calls == ["provider:https://cognitiveservices.azure.com/.default"]


async def test_startup_requires_the_api_key_with_api_key_auth(monkeypatch):
    monkeypatch.setattr(main.settings, "llm_auth", "api_key")
    monkeypatch.setattr(main.settings, "portal_assistant_llm_api_key", "")

    with pytest.raises(RuntimeError, match="PORTAL_ASSISTANT_LLM_API_KEY is not set"):
        async with main.lifespan(main.app):
            pass


async def test_startup_does_not_require_the_api_key_with_azure_auth(monkeypatch):
    monkeypatch.setattr(main.settings, "llm_auth", "azure")
    monkeypatch.setattr(main.settings, "portal_assistant_llm_api_key", "")
    monkeypatch.setattr(main.settings, "portal_assistant_model_name", "")

    # Startup gets past the key check and stops at the next one, the model name.
    with pytest.raises(RuntimeError, match="PORTAL_ASSISTANT_MODEL_NAME is not set"):
        async with main.lifespan(main.app):
            pass
