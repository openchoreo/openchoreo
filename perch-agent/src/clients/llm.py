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
    return init_chat_model(model=model_name, api_key=api_key, **kwargs)
