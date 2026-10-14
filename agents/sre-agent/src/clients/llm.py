# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


def _uses_adc(model_provider: str) -> bool:
    # Vertex AI reads application default credentials from the environment
    # (GOOGLE_APPLICATION_CREDENTIALS, or the workload identity metadata
    # server in-cluster) and accepts no API key.
    return "vertex" in model_provider.lower()


@lru_cache
def get_model(
    model_name: str | None = None,
    model_provider: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    model_name = model_name or settings.rca_model_name
    model_provider = model_provider or settings.rca_model_provider
    api_key = api_key or settings.rca_llm_api_key
    # Route through an OpenAI-compatible proxy (the ai-gateway-agentgateway
    # module) when configured; the real provider key then lives at the gateway
    # so api_key may be a placeholder. Forward base_url only when set to leave
    # the direct-to-provider path unchanged.
    if settings.rca_llm_base_url and "base_url" not in kwargs:
        kwargs["base_url"] = settings.rca_llm_base_url
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
