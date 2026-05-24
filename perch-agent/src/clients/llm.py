# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


def _requires_responses_api(model_name: str) -> bool:
    # gpt-5.x-mini / gpt-5.x-nano / o-series-mini reject ``reasoning_effort``
    # on /v1/chat/completions when function tools are bound and require
    # /v1/responses instead. langchain-openai routes to the responses
    # endpoint when ``use_responses_api=True``. Match on the model segment
    # after the optional ``provider:`` prefix.
    base = model_name.split(":", 1)[-1].lower()
    return base.endswith("-mini") or base.endswith("-nano")


def get_model(
    model_name: str | None = None,
    model_provider: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    model_name = model_name or settings.portal_assistant_model_name
    model_provider = model_provider or settings.portal_assistant_model_provider
    api_key = api_key or settings.portal_assistant_llm_api_key
    # OpenAI gpt-5 / o-series reasoning_effort. ``init_chat_model`` forwards
    # unknown kwargs to the provider class (langchain-openai's ChatOpenAI),
    # which accepts ``reasoning_effort`` as a first-class field. Only pass
    # when explicitly set so legacy / non-reasoning models that don't
    # support the param aren't surprised by it. Caller-supplied kwargs win
    # over the settings value so per-call probes (e.g. main.py's startup
    # ping) can override without touching configuration.
    model_name_lower = str(model_name).lower()
    is_openai = (
        "openai" in model_name_lower
        or (model_provider and "openai" in str(model_provider).lower())
        or model_name_lower.startswith(("gpt-", "o1-", "o3-"))
    )

    if (
        is_openai
        and settings.portal_assistant_reasoning_effort
        and "reasoning_effort" not in kwargs
    ):
        kwargs["reasoning_effort"] = settings.portal_assistant_reasoning_effort
        if _requires_responses_api(model_name) and "use_responses_api" not in kwargs:
            kwargs["use_responses_api"] = True

    return init_chat_model(
        model=model_name,
        model_provider=model_provider or None,
        api_key=api_key or None,
        **kwargs,
    )
