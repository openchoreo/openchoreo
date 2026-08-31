# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging

import httpx

from common.auth.authz_errors import (
    AuthzForbidden,
    AuthzServiceUnavailable,
    AuthzUnauthorized,
)
from common.auth.authz_models import Decision, EvaluateRequest
from common.logging_config import request_id_context

logger = logging.getLogger(__name__)


async def _inject_request_id(request: httpx.Request) -> None:
    rid = request_id_context.get()
    if not rid:
        return
    if not any(k.lower() == "x-request-id" for k in request.headers.keys()):
        request.headers["X-Request-Id"] = rid


_RETRY_ATTEMPTS = 3
_RETRY_BACKOFF_SECONDS = 0.1


class AuthzClient:
    def __init__(self, base_url: str, timeout: float, verify_ssl: bool = True):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.verify_ssl = verify_ssl
        # Eager construction: no race to build the pool on first use.
        self._client: httpx.AsyncClient | None = httpx.AsyncClient(
            timeout=httpx.Timeout(timeout),
            verify=verify_ssl,
            event_hooks={"request": [_inject_request_id]},
        )

        logger.info(
            "Authorization client initialized",
            extra={"service_url": base_url, "timeout": timeout},
        )

    async def close(self) -> None:
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def evaluate(self, request: EvaluateRequest, auth_token: str | None = None) -> Decision:
        url = f"{self.base_url}/api/v1/authz/evaluates"
        headers = {"Content-Type": "application/json"}

        if auth_token:
            headers["Authorization"] = f"Bearer {auth_token}"

        body = [request.model_dump()]

        resource = getattr(request, "resource", None)
        logger.debug(
            "Authz request",
            extra={
                "url": url,
                "action": request.action,
                "resource_type": getattr(resource, "type", None),
                "resource_id": getattr(resource, "id", None),
            },
        )

        if self._client is None:
            raise RuntimeError("AuthzClient has been closed")
        client = self._client

        # evaluate is a read-only check, so transient connection failures are
        # safe to retry; HTTP-level errors are not retried.
        last_error: httpx.RequestError | None = None
        response = None
        for attempt in range(_RETRY_ATTEMPTS):
            try:
                response = await client.post(url, json=body, headers=headers)
                break
            except httpx.RequestError as e:
                last_error = e
                if attempt < _RETRY_ATTEMPTS - 1:
                    await asyncio.sleep(_RETRY_BACKOFF_SECONDS * (2**attempt))
        if response is None:
            logger.error(
                "Authz service unavailable after %d attempts",
                _RETRY_ATTEMPTS,
                extra={"url": url, "error": str(last_error)},
            )
            raise AuthzServiceUnavailable(
                "Authorization service unavailable",
            ) from last_error

        if response.status_code == 401:
            logger.warning("Authz service returned unauthorized", extra={"status": 401})
            raise AuthzUnauthorized("Token validation failed")

        if response.status_code == 403:
            logger.debug("Authz service returned forbidden")
            raise AuthzForbidden("Access denied")

        if response.status_code != 200:
            body_text = response.text
            logger.error(
                "Authz service error",
                extra={"status": response.status_code, "response_body": body_text},
            )
            raise AuthzServiceUnavailable(
                f"Authorization service error: {response.status_code}",
            )

        try:
            decisions = [Decision.model_validate(d) for d in response.json()]
        except Exception as e:
            logger.error("Failed to parse authz response", extra={"error": str(e)})
            raise AuthzServiceUnavailable(
                "Invalid response from authorization service",
            ) from e

        if not decisions:
            logger.error("Authz service returned empty decisions array")
            raise AuthzServiceUnavailable(
                "Invalid response from authorization service",
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
