# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import os
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


def get_model(
    model_name: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    model_name = model_name or settings.portal_assistant_model_name
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
    if settings.portal_assistant_reasoning_effort and "reasoning_effort" not in kwargs:
        kwargs["reasoning_effort"] = settings.portal_assistant_reasoning_effort
    # Force /v1/responses for direct OpenAI traffic only; a base_url override may point elsewhere.
    if (
        not _has_base_url_override(kwargs)
        and _is_openai_model(model_name)
        and "use_responses_api" not in kwargs
    ):
        kwargs["use_responses_api"] = True
    return init_chat_model(model=model_name, api_key=get_llm_api_key(settings, api_key), **kwargs)
