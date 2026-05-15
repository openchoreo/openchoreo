# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings


def get_model(
    model_name: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    model_name = model_name or settings.perch_model_name
    api_key = api_key or settings.perch_llm_api_key
    # OpenAI gpt-5 / o-series reasoning_effort. ``init_chat_model`` forwards
    # unknown kwargs to the provider class (langchain-openai's ChatOpenAI),
    # which accepts ``reasoning_effort`` as a first-class field. Only pass
    # when explicitly set so legacy / non-reasoning models that don't
    # support the param aren't surprised by it. Caller-supplied kwargs win
    # over the settings value so per-call probes (e.g. main.py's startup
    # ping) can override without touching configuration.
    if settings.perch_reasoning_effort and "reasoning_effort" not in kwargs:
        kwargs["reasoning_effort"] = settings.perch_reasoning_effort
    return init_chat_model(model=model_name, api_key=api_key, **kwargs)
