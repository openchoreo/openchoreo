# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the LLM factory: provider selection and the OpenAI-only kwargs gate."""

import pytest

import src.clients.llm as llm


@pytest.fixture
def captured(monkeypatch):
    """Stub init_chat_model and record the kwargs get_model builds for it."""
    calls = []

    def fake_init(**kwargs):
        calls.append(kwargs)
        return object()

    monkeypatch.setattr(llm, "init_chat_model", fake_init)
    monkeypatch.setattr(llm.settings, "portal_assistant_model_provider", "")
    monkeypatch.setattr(llm.settings, "portal_assistant_llm_base_url", "")
    monkeypatch.setattr(llm.settings, "portal_assistant_reasoning_effort", "low")
    return calls


def test_explicit_provider_is_forwarded(captured):
    llm.get_model("gemini-3.6-flash", model_provider="google_genai", api_key="k")
    assert captured[0]["model_provider"] == "google_genai"


def test_unset_provider_is_omitted(captured):
    # An unset provider must not be forwarded at all, so langchain falls
    # back to inferring it from the model name.
    llm.get_model("gpt-5.2", api_key="k")
    assert "model_provider" not in captured[0]


@pytest.mark.parametrize("provider", ["google_vertexai", "vertexai"])
def test_api_key_dropped_for_vertex(captured, provider):
    # ChatVertexAI has no api_key field, so langchain would move the key into
    # model_kwargs and send it as a generation parameter. Vertex uses ADC.
    llm.get_model("gemini-3.1-pro", model_provider=provider, api_key="secret-key")
    assert "api_key" not in captured[0]


def test_api_key_kept_for_gemini_developer_api(captured):
    # google_genai is the API-key path and must still receive the key.
    llm.get_model("gemini-3.6-flash", model_provider="google_genai", api_key="secret-key")
    assert captured[0]["api_key"] == "secret-key"


def test_unset_api_key_is_omitted(captured, monkeypatch):
    # Google reads credentials from the environment when no key is given;
    # an empty string would be taken at face value and rejected.
    monkeypatch.setattr(llm.settings, "portal_assistant_llm_api_key", "")
    llm.get_model("gemini-3.6-flash", model_provider="google_genai")
    assert "api_key" not in captured[0]


@pytest.mark.parametrize(
    "model_name, model_provider",
    [
        ("gemini-3.6-flash", "google_genai"),
        ("gemini-3.6-flash", ""),
        ("google_vertexai:gemini-3.1-pro", ""),
        ("claude-sonnet-5", "anthropic"),
    ],
)
def test_reasoning_effort_withheld_from_non_openai(captured, model_name, model_provider):
    # reasoning_effort is a langchain-openai field; forwarding it to
    # Gemini or Anthropic is a hard error at model construction.
    llm.get_model(model_name, model_provider=model_provider, api_key="k")
    assert "reasoning_effort" not in captured[0]
    assert "use_responses_api" not in captured[0]


@pytest.mark.parametrize(
    "model_name, model_provider",
    [
        ("gpt-5.2", ""),
        ("openai:gpt-5.2", ""),
        # o3 / o4-mini are retired, but _is_openai still branches on the
        # o1-/o3-/o4- prefixes, so these keep that branch covered for as
        # long as it exists. Not a recommendation of the models.
        ("o3-mini", ""),
        ("o4-mini", ""),
        ("some-proxied-name", "openai"),
    ],
)
def test_reasoning_effort_applied_to_openai(captured, model_name, model_provider):
    # Detection must survive an empty provider, so an operator who never
    # set modelProvider still gets the OpenAI options on a gpt-*/o*- model.
    llm.get_model(model_name, model_provider=model_provider, api_key="k")
    assert captured[0]["reasoning_effort"] == "low"


@pytest.mark.parametrize("model_name", ["gpt-5.2-mini", "gpt-5.2-nano", "o4-mini"])
def test_responses_api_forced_for_mini_and_nano(captured, model_name):
    llm.get_model(model_name, api_key="k")
    assert captured[0]["use_responses_api"] is True


def test_responses_api_not_forced_for_full_size_models(captured):
    llm.get_model("gpt-5.2", api_key="k")
    assert "use_responses_api" not in captured[0]


def test_caller_kwargs_win_over_settings(captured):
    llm.get_model("gpt-5.2", api_key="k", reasoning_effort="high")
    assert captured[0]["reasoning_effort"] == "high"
