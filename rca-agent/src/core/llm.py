# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from functools import lru_cache

from langchain.chat_models import init_chat_model

from src.core.config import settings


@lru_cache
def get_model(
    model_name: str = settings.rca_model_name, api_key: str = settings.rca_llm_api_key, **kwargs
):
    return init_chat_model(model=model_name, api_key=api_key, **kwargs)
