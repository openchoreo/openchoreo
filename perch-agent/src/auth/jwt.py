# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import ssl
from typing import Any

import jwt
from jwt import PyJWKClient, PyJWKClientError

from src.config import settings

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
    ):
        if not allow_unverified:
            missing = [
                name
                for name, value in (("jwks_url", jwks_url), ("issuer", issuer))
                if not value
            ]
            if missing:
                raise ValueError(
                    "JWTValidator misconfigured: missing required settings "
                    f"{missing}. Set JWT_JWKS_URL and JWT_ISSUER, "
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

        # PyJWKClient handles its own TTL'd key cache via ``lifespan``; we
        # construct it once and let it refresh signing keys in-place.
        # Previously we recreated the client on every ``refresh_interval``,
        # which discarded the cached keys and forced a fresh HTTPS handshake
        # on the next request — pure overhead.
        self._jwks_client = PyJWKClient(
            self.jwks_url,
            cache_keys=True,
            lifespan=self.refresh_interval,
            headers={"User-Agent": "perch-agent/1.0"},
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

            logger.debug(
                "JWT validation successful",
                extra={"sub": claims.get("sub"), "iss": claims.get("iss")},
            )

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
        # PyJWKClient + jwt.decode are synchronous and can block the event
        # loop for hundreds of ms on JWKS misses (network I/O + RS256
        # verify). Punt to a worker thread so the loop keeps serving other
        # requests while one validation is in flight.
        return await asyncio.to_thread(self._validate_sync, token)


class DisabledJWTValidator:
    async def validate(self, _token: str) -> dict[str, Any]:
        logger.debug("JWT validation disabled, skipping")
        return {}


_jwt_validator: JWTValidator | DisabledJWTValidator | None = None


def get_jwt_validator() -> JWTValidator | DisabledJWTValidator:
    global _jwt_validator

    if _jwt_validator is None:
        if settings.jwt_insecure_allow_unverified and not settings.jwt_jwks_url:
            logger.warning(
                "JWT_INSECURE_ALLOW_UNVERIFIED is set and JWT_JWKS_URL is empty — "
                "JWT validation is disabled. Do not use this in production."
            )
            _jwt_validator = DisabledJWTValidator()
        else:
            if not settings.jwt_jwks_url:
                raise RuntimeError(
                    "JWT_JWKS_URL is required. Set it, or enable "
                    "JWT_INSECURE_ALLOW_UNVERIFIED for dev-only use."
                )
            _jwt_validator = JWTValidator(
                jwks_url=settings.jwt_jwks_url,
                issuer=settings.jwt_issuer,
                audience=settings.jwt_audience,
                refresh_interval=settings.jwt_jwks_refresh_interval,
                verify_ssl=not settings.jwks_url_tls_insecure_skip_verify,
                allow_unverified=settings.jwt_insecure_allow_unverified,
            )

    return _jwt_validator
