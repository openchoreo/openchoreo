# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


@lru_cache
def get_model(
    model_name: str = settings.llm_name,
    api_key: str = settings.llm_api_key,
    **kwargs: Any,
) -> BaseChatModel:
    # Route through an OpenAI-compatible proxy (the ai-gateway-agentgateway
    # module) when configured; the real provider key then lives at the gateway
    # so api_key may be a placeholder. Forward base_url only when set to leave
    # the direct-to-provider path unchanged.
    if settings.llm_base_url and "base_url" not in kwargs:
        kwargs["base_url"] = settings.llm_base_url
    return init_chat_model(
        model=model_name,
        api_key=api_key,
        max_tokens=settings.finops_llm_max_tokens,
        **kwargs,
    )
