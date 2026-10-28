# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the LLM factory's credential handling."""

import common.llm_auth as llm_auth
import src.clients.llm as llm


def capture_api_keys(monkeypatch):
    calls = []

    def fake_init(**kwargs):
        calls.append(kwargs["api_key"])
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)
    return calls


def test_get_model_passes_the_static_key_by_default(monkeypatch):
    calls = capture_api_keys(monkeypatch)

    llm.get_model("openai:gpt-4o-mini", "static-key")

    assert calls == ["static-key"]


def test_get_model_passes_a_token_provider_with_azure_auth(monkeypatch):
    calls = capture_api_keys(monkeypatch)
    monkeypatch.setattr(llm.settings, "llm_auth", "azure")
    monkeypatch.setattr(llm.settings, "llm_token_scope", "api://llm/.default")
    monkeypatch.setattr(llm_auth, "_azure_token_provider", lambda scope: f"provider:{scope}")

    llm.get_model("openai:gpt-4o-mini", "unused-key")

    assert calls == ["provider:api://llm/.default"]
