# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import os
from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from common.llm_auth import get_llm_api_key
from src.config import settings


def _is_openai_model(model_name: str) -> bool:
    provider = model_name.split(":", 1)[0].lower() if ":" in model_name else "openai"
    return provider == "openai"


def _has_base_url_override(kwargs: dict[str, Any]) -> bool:
    # ChatOpenAI itself falls back to OPENAI_API_BASE/OPENAI_BASE_URL when no
    # base_url kwarg is given, so those env vars can route elsewhere too.
    return bool(
        kwargs.get("base_url")
        or os.environ.get("OPENAI_API_BASE")
        or os.environ.get("OPENAI_BASE_URL")
    )


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
    if settings.finops_agent_llm_base_url and "base_url" not in kwargs:
        kwargs["base_url"] = settings.finops_agent_llm_base_url
    # Force /v1/responses for direct OpenAI traffic only; a base_url override may point elsewhere.
    if (
        not _has_base_url_override(kwargs)
        and _is_openai_model(model_name)
        and "use_responses_api" not in kwargs
    ):
        kwargs["use_responses_api"] = True
    return init_chat_model(
        model=model_name,
        api_key=get_llm_api_key(settings, api_key),
        max_tokens=settings.finops_llm_max_tokens,
        **kwargs,
    )
