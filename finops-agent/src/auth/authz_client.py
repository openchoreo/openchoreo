# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

import httpx
from fastapi import HTTPException

from src.auth.authz_models import Decision, EvaluateRequest

logger = logging.getLogger(__name__)


class AuthzClient:
    def __init__(self, base_url: str, timeout: float, verify_ssl: bool = True):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.verify_ssl = verify_ssl
        self._client: httpx.AsyncClient | None = None

        logger.info(
            "Authorization client initialized",
            extra={"service_url": base_url, "timeout": timeout},
        )

    async def _get_client(self) -> httpx.AsyncClient:
        if self._client is None:
            self._client = httpx.AsyncClient(
                timeout=httpx.Timeout(self.timeout),
                verify=self.verify_ssl,
            )
        return self._client

    async def close(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def evaluate(self, request: EvaluateRequest, auth_token: str | None = None) -> Decision:
        url = f"{self.base_url}/api/v1/authz/evaluates"
        headers = {"Content-Type": "application/json"}

        if auth_token:
            headers["Authorization"] = f"Bearer {auth_token}"

        body = [request.model_dump(by_alias=True)]

        logger.debug("Authz request", extra={"url": url})

        client = await self._get_client()

        try:
            response = await client.post(url, json=body, headers=headers)
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Authz service unavailable", extra={"url": url, "error_type": type(e).__name__})
            raise HTTPException(
                status_code=500,
                detail="Internal server error",
            ) from e

        if response.status_code == 401:
            logger.warning("Authz service returned unauthorized", extra={"status": 401})
            raise HTTPException(
                status_code=401,
                detail="Token validation failed",
            )

        if response.status_code == 403:
            logger.debug("Authz service returned forbidden")
            raise HTTPException(
                status_code=403,
                detail="Access denied",
            )

        if response.status_code != 200:
            logger.error(
                "Authz service error",
                extra={"status": response.status_code},
            )
            raise HTTPException(
                status_code=500,
                detail="Internal server error",
            )

        try:
            decisions = [Decision.model_validate(d) for d in response.json()]
        except Exception as e:
            logger.error("Failed to parse authz response", extra={"error_type": type(e).__name__})
            raise HTTPException(
                status_code=500,
                detail="Internal server error",
            ) from e

        if not decisions:
            logger.error("Authz service returned empty decisions array")
            raise HTTPException(
                status_code=500,
                detail="Internal server error",
            )

        decision = decisions[0]

        logger.debug(
            "Authorization evaluated",
            extra={
                "action": request.action,
                "resource_type": request.resource.type,
                "resource_id": request.resource.id,
                "decision": decision.decision,
            },
        )

        return decision
