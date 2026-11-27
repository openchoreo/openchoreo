# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Signed check changes waiting for the user's confirmation.

After an edit's Step 2 the agent shows a confirm card. The change the card
describes travels to the browser as a signed token and comes back with the
user's click, so the confirm endpoint saves exactly what was shown:

- ``sign`` wraps an action for one user, valid for ``ttl_s`` seconds.
- ``consume`` checks the signature, expiry and user, and refuses a token
  that was already used. Its result is the action to run.
- ``release`` lets a token be confirmed again when its confirm saved nothing.

No model is involved in either step. Used ids are kept in memory, per
replica; a token confirmed again elsewhere saves nothing twice: a new check
is saved under the card's id, which Backstage refuses to save again, and an
edit is refused once the check changed (see confirm_actions). Every save is
still checked by Backstage's permissions and validator.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import logging
import secrets
import time
import uuid
from typing import Any

from src.config import settings

logger = logging.getLogger(__name__)

DEFAULT_TTL_S = 300

_FALLBACK_SECRET = secrets.token_bytes(32)
_warned_fallback = False

# id -> expiry, for tokens already confirmed. Pruned as tokens expire.
_USED: dict[str, float] = {}


class InvalidAction(Exception):
    """The confirm token can't be used; the message says why."""


def _secret() -> bytes:
    global _warned_fallback
    if settings.portal_assistant_action_secret:
        return settings.portal_assistant_action_secret.encode()
    if not _warned_fallback:
        logger.warning(
            "PORTAL_ASSISTANT_ACTION_SECRET is not set — confirm cards are "
            "signed with a per-process secret and only work on this replica"
        )
        _warned_fallback = True
    return _FALLBACK_SECRET


def _b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def _unb64(text: str) -> bytes:
    return base64.urlsafe_b64decode(text + "=" * (-len(text) % 4))


def _signature(body: str) -> str:
    return _b64(hmac.new(_secret(), body.encode(), hashlib.sha256).digest())


def sign(action: dict[str, Any], user_sub: str, ttl_s: int = DEFAULT_TTL_S) -> str:
    """A token carrying ``action`` that only ``user_sub`` can confirm."""
    payload = {
        "id": uuid.uuid4().hex,
        "sub": user_sub,
        "exp": time.time() + ttl_s,
        "action": action,
    }
    # UTF-8, not \u escapes: a non-English description stays about half the size.
    body = _b64(
        json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode(),
    )
    return f"{body}.{_signature(body)}"


def _verify(token: str, user_sub: str) -> dict[str, Any]:
    body, _, signature = token.partition(".")
    if not body or not signature or not hmac.compare_digest(signature, _signature(body)):
        raise InvalidAction("This confirmation isn't valid. Ask the assistant again.")
    payload = json.loads(_unb64(body))
    if payload["exp"] <= time.time():
        raise InvalidAction("This confirmation expired. Ask the assistant again.")
    if not user_sub or payload["sub"] != user_sub:
        raise InvalidAction("This confirmation belongs to another user.")
    return payload


def consume(token: str, user_sub: str) -> dict[str, Any]:
    """The action in ``token``, marking it used; raises InvalidAction."""
    payload = _verify(token, user_sub)
    now = time.time()
    for used_id in [i for i, exp in _USED.items() if exp <= now]:
        del _USED[used_id]
    if payload["id"] in _USED:
        raise InvalidAction("This change was already confirmed.")
    _USED[payload["id"]] = payload["exp"]
    return payload["action"]


def release(token: str) -> None:
    """Let ``token`` be confirmed again, after a confirm that saved nothing."""
    body, _, _ = token.partition(".")
    _USED.pop(json.loads(_unb64(body))["id"], None)
