# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import ssl
from typing import Any

import jwt
from jwt import PyJWKClient, PyJWKClientError

logger = logging.getLogger(__name__)


class JWTValidationError(Exception):
    pass


class JWTValidator:
    def __init__(
        self,
        jwks_url: str,
        issuer: str = "",
        audience: str = "",
        refresh_interval: int = 3600,
        verify_ssl: bool = True,
        allow_unverified: bool = False,
        service_name: str = "openchoreo-agent",
    ):
        if not allow_unverified and not jwks_url:
            raise ValueError(
                "JWTValidator misconfigured: jwks_url is required. Set JWT_JWKS_URL, "
                "or enable JWT_INSECURE_ALLOW_UNVERIFIED for dev-only use."
            )
        self.jwks_url = jwks_url
        self.issuer = issuer
        self.audience = audience
        self.refresh_interval = refresh_interval
        self.verify_ssl = verify_ssl

        ssl_context = None
        if self.verify_ssl is False:
            ssl_context = ssl.create_default_context()
            ssl_context.check_hostname = False
            ssl_context.verify_mode = ssl.CERT_NONE
            logger.debug("SSL verification disabled for JWKS client")

        # One long-lived client; PyJWKClient TTLs its own key cache via lifespan.
        self._jwks_client = PyJWKClient(
            self.jwks_url,
            cache_keys=True,
            lifespan=self.refresh_interval,
            headers={"User-Agent": f"{service_name}/1.0"},
            ssl_context=ssl_context,
        )

        logger.info(
            "JWT validator initialized",
            extra={
                "jwks_url": jwks_url,
                "issuer": issuer or "(not validated)",
                "audience": audience or "(not validated)",
                "refresh_interval": refresh_interval,
            },
        )

    def _validate_sync(self, token: str) -> dict[str, Any]:
        try:
            signing_key = self._jwks_client.get_signing_key_from_jwt(token)

            options = {
                "verify_signature": True,
                "verify_exp": True,
                "verify_iat": True,
                "require": ["exp", "iat", "sub"],
            }

            decode_kwargs: dict[str, Any] = {
                "algorithms": ["RS256", "RS384", "RS512", "ES256", "ES384", "ES512"],
            }

            if self.issuer:
                options["verify_iss"] = True
                decode_kwargs["issuer"] = self.issuer
            else:
                options["verify_iss"] = False

            if self.audience:
                options["verify_aud"] = True
                decode_kwargs["audience"] = self.audience
            else:
                options["verify_aud"] = False

            claims = jwt.decode(
                token,
                signing_key.key,
                options=options,
                **decode_kwargs,
            )

            logger.debug("JWT validation successful", extra={"iss": claims.get("iss")})

            return claims

        except PyJWKClientError as e:
            logger.warning("Failed to fetch signing key from JWKS", extra={"error": str(e)})
            raise JWTValidationError(f"Failed to fetch signing key: {e}") from e
        except jwt.ExpiredSignatureError as e:
            logger.debug("Token has expired")
            raise JWTValidationError("Token has expired") from e
        except jwt.InvalidIssuerError as e:
            logger.debug("Invalid token issuer")
            raise JWTValidationError("Invalid token issuer") from e
        except jwt.InvalidAudienceError as e:
            logger.debug("Invalid token audience")
            raise JWTValidationError("Invalid token audience") from e
        except jwt.InvalidTokenError as e:
            logger.warning("Invalid token", extra={"error": str(e)})
            raise JWTValidationError(f"Invalid token: {e}") from e

    async def validate(self, token: str) -> dict[str, Any]:
        # JWKS fetch + signature verify are blocking; keep them off the event loop.
        return await asyncio.to_thread(self._validate_sync, token)


class DisabledJWTValidator:
    async def validate(self, _token: str) -> dict[str, Any]:
        logger.debug("JWT validation disabled, skipping")
        return {}


def create_jwt_validator(
    *,
    jwks_url: str,
    issuer: str = "",
    audience: str = "",
    refresh_interval: int = 3600,
    verify_ssl: bool = True,
    allow_unverified: bool = False,
    service_name: str = "openchoreo-agent",
) -> JWTValidator | DisabledJWTValidator:
    """Fail closed: a missing JWKS URL aborts startup unless allow_unverified
    (dev-only) is set, which disables validation but keeps the pod booting."""
    if allow_unverified and not jwks_url:
        logger.warning(
            "JWT_INSECURE_ALLOW_UNVERIFIED is set and JWT_JWKS_URL is empty — "
            "JWT validation is disabled. Do not use this in production."
        )
        return DisabledJWTValidator()

    if not jwks_url:
        raise RuntimeError(
            "JWT_JWKS_URL is required. Set it, or enable "
            "JWT_INSECURE_ALLOW_UNVERIFIED for dev-only use."
        )

    return JWTValidator(
        jwks_url=jwks_url,
        issuer=issuer,
        audience=audience,
        refresh_interval=refresh_interval,
        verify_ssl=verify_ssl,
        allow_unverified=allow_unverified,
        service_name=service_name,
    )
