# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Domain exceptions for the AuthzClient.

The client used to raise ``fastapi.HTTPException`` directly, which made
``AuthzClient`` depend on the web framework and forced any non-FastAPI
caller (tests, future workers, scripts) to import HTTPException to
handle authz failures. These domain exceptions keep the client
framework-agnostic; ``src/auth/dependencies.py`` maps them to HTTP
responses at the FastAPI seam.
"""


class AuthzError(Exception):
    """Base class for every failure raised by ``AuthzClient.evaluate``."""


class AuthzServiceUnavailable(AuthzError):
    """The authz service is unreachable, timed out, or returned an
    unparseable / unexpected response. Maps to HTTP 503 at the FastAPI
    boundary.
    """


class AuthzUnauthorized(AuthzError):
    """The authz service rejected our token. Maps to HTTP 401."""


class AuthzForbidden(AuthzError):
    """The authz service explicitly forbade the request. Maps to HTTP 403."""
