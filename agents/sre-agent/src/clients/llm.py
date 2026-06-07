# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from functools import lru_cache
from typing import Any

from langchain.chat_models import init_chat_model
from langchain_core.language_models import BaseChatModel

from src.config import settings

logger = logging.getLogger(__name__)


@lru_cache
def get_model(
    model_name: str | None = None,
    model_provider: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> BaseChatModel:
    m_name = model_name or settings.rca_model_name
    m_provider = model_provider or settings.rca_model_provider
    a_key = api_key or settings.rca_llm_api_key

    logger.info("Initializing LLM: name='%s', provider='%s'", m_name, m_provider)

    return init_chat_model(
        model=m_name,
        model_provider=m_provider or None,
        api_key=a_key or None,
        **kwargs,
    )
