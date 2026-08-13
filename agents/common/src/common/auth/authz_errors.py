# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

class AuthzError(Exception):
    pass


class AuthzServiceUnavailable(AuthzError):
    """Authz service unreachable or returned an unusable response (503)."""


class AuthzUnauthorized(AuthzError):
    """Authz service rejected the token (401)."""


class AuthzForbidden(AuthzError):
    """Authz service forbade the request (403)."""
