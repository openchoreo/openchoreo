# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


def _base_model_name(model_name: str) -> str:
    # Strip the optional ``provider:`` prefix that ``init_chat_model``
    # accepts, so name-based checks see just the model segment.
    return model_name.split(":", 1)[-1].lower()


def _requires_responses_api(model_name: str) -> bool:
    # gpt-5.x-mini / gpt-5.x-nano / o-series-mini reject ``reasoning_effort``
    # on /v1/chat/completions when function tools are bound and require
    # /v1/responses instead. langchain-openai routes to the responses
    # endpoint when ``use_responses_api=True``.
    base = _base_model_name(model_name)
    return base.endswith("-mini") or base.endswith("-nano")


def _uses_adc(model_provider: str) -> bool:
    # Vertex AI reads application default credentials from the environment
    # (GOOGLE_APPLICATION_CREDENTIALS, or the workload identity metadata
    # server in-cluster) and accepts no API key.
    return "vertex" in model_provider.lower()


def _is_openai(model_name: str, model_provider: str) -> bool:
    # ``reasoning_effort`` / ``use_responses_api`` are langchain-openai
    # fields; forwarding them to another provider's class is a hard error
    # (Gemini rejects both). Trust an explicit provider when set, and
    # otherwise fall back to the same naming conventions langchain itself
    # infers from, so an operator who leaves modelProvider empty on a
    # ``gpt-*`` / ``o*-`` model still gets the OpenAI options.
    if model_provider:
        return "openai" in model_provider.lower()
    base = _base_model_name(model_name)
    return "openai" in model_name.lower() or base.startswith(("gpt-", "o1-", "o3-", "o4-"))


def get_model(
    model_name: str | None = None,
    model_provider: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    model_name = model_name or settings.portal_assistant_model_name
    model_provider = model_provider or settings.portal_assistant_model_provider
    api_key = api_key or settings.portal_assistant_llm_api_key
    # Route through an OpenAI-compatible proxy (the ai-gateway-agentgateway
    # module) when configured; the real provider key then lives at the gateway
    # so api_key may be a placeholder. Forward base_url only when set to leave
    # the direct-to-provider path unchanged.
    if settings.portal_assistant_llm_base_url and "base_url" not in kwargs:
        kwargs["base_url"] = settings.portal_assistant_llm_base_url
    # OpenAI gpt-5 / o-series reasoning_effort. ``init_chat_model`` forwards
    # unknown kwargs to the provider class (langchain-openai's ChatOpenAI),
    # which accepts ``reasoning_effort`` as a first-class field. Only pass
    # when explicitly set so legacy / non-reasoning models that don't
    # support the param aren't surprised by it. Caller-supplied kwargs win
    # over the settings value so per-call probes (e.g. main.py's startup
    # ping) can override without touching configuration.
    if (
        _is_openai(model_name, model_provider)
        and settings.portal_assistant_reasoning_effort
        and "reasoning_effort" not in kwargs
    ):
        kwargs["reasoning_effort"] = settings.portal_assistant_reasoning_effort
        if _requires_responses_api(model_name) and "use_responses_api" not in kwargs:
            kwargs["use_responses_api"] = True
    # Vertex AI authenticates with application default credentials and has no
    # api_key field, so the chart's LLM secret is meaningless on this path.
    if _uses_adc(model_provider):
        api_key = ""
    # Forward only what is actually set. Omitting is not the same as passing
    # an explicit None: for a provider class with no api_key field (ChatVertexAI)
    # langchain moves the argument into model_kwargs and sends it as a
    # generation parameter, None included. An empty string is worse still, since
    # the Google classes read it as a supplied-but-empty key and reject it
    # rather than falling back to their own environment variable.
    if model_provider:
        kwargs["model_provider"] = model_provider
    if api_key:
        kwargs["api_key"] = api_key
    return init_chat_model(model=model_name, **kwargs)
