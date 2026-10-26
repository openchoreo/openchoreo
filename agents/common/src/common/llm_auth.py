# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Credential for the agents' LLM calls: a static API key or an Entra ID token."""

from collections.abc import Callable
from functools import lru_cache

from azure.identity import DefaultAzureCredential, get_bearer_token_provider

from common.config import CommonSettings

LLM_AUTH_AZURE = "azure"


@lru_cache
def _azure_token_provider(scope: str) -> Callable[[], str]:
    # One provider per scope, shared by every model the process builds, so the
    # token is cached and refreshed in one place.
    return get_bearer_token_provider(DefaultAzureCredential(), scope)


def get_llm_api_key(settings: CommonSettings, api_key: str) -> str | Callable[[], str]:
    """Return the value to pass to the chat model as ``api_key``.

    With ``llm_auth=azure`` this is a callable returning an Entra ID access token
    for ``llm_token_scope``. The OpenAI client calls it before every request, and
    the provider refreshes the token before it expires, so a long-running pod
    never needs a restart. Otherwise ``api_key`` is returned unchanged.
    """
    if settings.llm_auth == LLM_AUTH_AZURE:
        return _azure_token_provider(settings.llm_token_scope)
    return api_key
