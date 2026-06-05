# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import httpx


class BearerTokenAuth(httpx.Auth):
    def __init__(self, token: str) -> None:
        self._token = token

    def sync_auth_flow(self, request: httpx.Request):
        request.headers["Authorization"] = f"Bearer {self._token}"
        yield request

    async def async_auth_flow(self, request: httpx.Request):
        request.headers["Authorization"] = f"Bearer {self._token}"
        yield request
