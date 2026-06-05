# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Any

import httpx

from src.config import settings

logger = logging.getLogger(__name__)

_API_BASE = f"{settings.openchoreo_api_url.rstrip('/')}/api/v1"


_HEADERS = {"X-Use-OpenAPI": "true"}


async def get(
    path: str,
    auth: httpx.Auth,
    params: dict[str, str] | None = None,
) -> dict[str, Any]:
    async with httpx.AsyncClient(
        verify=not settings.tls_insecure_skip_verify,
        timeout=httpx.Timeout(10.0, connect=5.0),
    ) as client:
        response = await client.get(
            f"{_API_BASE}{path}", headers=_HEADERS, params=params, auth=auth
        )
        response.raise_for_status()
        return response.json()
