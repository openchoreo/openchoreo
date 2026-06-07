# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import ssl
import threading
import time
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
    ):
        self.jwks_url = jwks_url
        self.issuer = issuer
        self.audience = audience
        self.refresh_interval = refresh_interval
        self.verify_ssl = verify_ssl

        self._jwks_client: PyJWKClient | None = None
        self._last_refresh: float = 0
        self._lock = threading.Lock()

        logger.info(
            "JWT validator initialized",
            extra={
                "jwks_url": jwks_url,
                "issuer": issuer or "(not validated)",
                "audience": audience or "(not validated)",
                "refresh_interval": refresh_interval,
            },
        )

    def _get_jwks_client(self) -> PyJWKClient:
        now = time.time()

        with self._lock:
            if self._jwks_client is None or (now - self._last_refresh) > self.refresh_interval:
                logger.debug("Refreshing JWKS client", extra={"jwks_url": self.jwks_url})

                ssl_context = None
                if self.verify_ssl is False:
                    ssl_context = ssl.create_default_context()
                    ssl_context.check_hostname = False
                    ssl_context.verify_mode = ssl.CERT_NONE
                    logger.debug("SSL verification disabled for JWKS client")

                # PyJWKClient handles caching internally, but we recreate it
                # periodically to pick up key rotations
                self._jwks_client = PyJWKClient(
                    self.jwks_url,
                    cache_keys=True,
                    lifespan=self.refresh_interval,
                    headers={"User-Agent": "rca-agent/1.0"},
                    ssl_context=ssl_context,
                )
                self._last_refresh = now

            return self._jwks_client

    def validate(self, token: str) -> dict[str, Any]:
        try:
            jwks_client = self._get_jwks_client()
            signing_key = jwks_client.get_signing_key_from_jwt(token)

            # Build decode options
            options = {
                "verify_signature": True,
                "verify_exp": True,
                "verify_iat": True,
                "require": ["exp", "iat", "sub"],
            }

            # Build validation kwargs
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


class DisabledJWTValidator:
    def validate(self, _token: str) -> dict[str, Any]:
        logger.debug("JWT validation disabled, skipping")
        return {}


_jwt_validator: JWTValidator | DisabledJWTValidator | None = None


def get_jwt_validator() -> JWTValidator | DisabledJWTValidator:
    global _jwt_validator

    if _jwt_validator is None:
        if not settings.jwt_jwks_url:
            logger.warning(
                "JWT JWKS URL not configured - JWT authentication disabled. "
                "Set JWT_JWKS_URL to enable token validation."
            )
            _jwt_validator = DisabledJWTValidator()
        else:
            _jwt_validator = JWTValidator(
                jwks_url=settings.jwt_jwks_url,
                issuer=settings.jwt_issuer,
                audience=settings.jwt_audience,
                refresh_interval=settings.jwt_jwks_refresh_interval,
                verify_ssl=not settings.tls_insecure_skip_verify,
            )

    return _jwt_validator
