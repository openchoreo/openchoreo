# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

import httpx
from fastapi import HTTPException

from src.auth.authz_models import AuthzResponse, Decision, EvaluateRequest

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
        url = f"{self.base_url}/api/v1/authz/evaluate"
        headers = {"Content-Type": "application/json"}

        if auth_token:
            headers["Authorization"] = f"Bearer {auth_token}"

        body = request.model_dump()

        logger.debug("Authz request", extra={"url": url, "body": body})

        client = await self._get_client()

        try:
            response = await client.post(url, json=body, headers=headers)
        except (httpx.ConnectError, httpx.TimeoutException) as e:
            logger.error("Authz service unavailable", extra={"url": url, "error": str(e)})
            raise HTTPException(
                status_code=503,
                detail={
                    "error": "SERVICE_UNAVAILABLE",
                    "message": "Authorization service unavailable",
                },
            ) from e

        if response.status_code == 401:
            logger.warning("Authz service returned unauthorized", extra={"status": 401})
            raise HTTPException(
                status_code=401,
                detail={"error": "UNAUTHORIZED", "message": "Token validation failed"},
            )

        if response.status_code == 403:
            logger.debug("Authz service returned forbidden")
            return Decision(decision=False)

        if response.status_code != 200:
            body_text = response.text
            logger.error(
                "Authz service error",
                extra={"status": response.status_code, "response_body": body_text},
            )
            raise HTTPException(
                status_code=503,
                detail={
                    "error": "SERVICE_UNAVAILABLE",
                    "message": f"Authorization service error: {response.status_code}",
                },
            )

        try:
            authz_response = AuthzResponse.model_validate(response.json())
        except Exception as e:
            logger.error("Failed to parse authz response", extra={"error": str(e)})
            raise HTTPException(
                status_code=503,
                detail={
                    "error": "SERVICE_UNAVAILABLE",
                    "message": "Invalid response from authorization service",
                },
            ) from e

        logger.debug(
            "Authorization evaluated",
            extra={
                "action": request.action,
                "resource_type": request.resource.type,
                "resource_id": request.resource.id,
                "decision": authz_response.data.decision,
            },
        )

        return authz_response.data
