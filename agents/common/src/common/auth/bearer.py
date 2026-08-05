# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import httpx


class BearerTokenAuth(httpx.Auth):
    def __init__(self, token: str, *, allow_empty: bool = False) -> None:
        token = token.strip()
        if not token and not allow_empty:
            raise ValueError("Bearer token must not be empty or whitespace-only")
        self._token = token

    def sync_auth_flow(self, request: httpx.Request):
        request.headers["Authorization"] = f"Bearer {self._token}"
        yield request

    async def async_auth_flow(self, request: httpx.Request):
        request.headers["Authorization"] = f"Bearer {self._token}"
        yield request
