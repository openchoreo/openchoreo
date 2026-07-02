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
        if (
            _requires_responses_api(model_name)
            and "use_responses_api" not in kwargs
        ):
            kwargs["use_responses_api"] = True
    return init_chat_model(model=model_name, api_key=api_key, **kwargs)
