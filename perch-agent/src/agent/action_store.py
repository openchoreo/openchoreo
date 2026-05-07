# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
import copy
import hashlib
import logging
import time
import uuid
from dataclasses import dataclass
from typing import Any

from src.config import settings

logger = logging.getLogger(__name__)


def _hash_user_subject(user_sub: str) -> str:
    """Return a short, non-reversible fingerprint of a user subject for logs."""
    if not user_sub:
        return "<empty>"
    return "sub_" + hashlib.sha256(user_sub.encode()).hexdigest()[:12]


@dataclass
class _Item:
    """One pending mutating action awaiting user confirmation."""

    action_id: str
    tool: str
    args: dict[str, Any]
    user_sub: str
    expires_at: float
    summary: str = ""
    current_state: Any = None
    proposed_state: Any = None


class ActionStore:
    """In-memory action map keyed by action_id, with TTL eviction.

    Invariants:
    - take() is one-shot — replay returns None.
    - user_sub at execute-time must match user_sub at propose-time, otherwise None.
    - Pod restart drops everything in flight (graceful UX: "session expired,
      please re-ask"). HA migration to a shared store is a separate proposal.
    """

    def __init__(self, ttl_seconds: int):
        self._items: dict[str, _Item] = {}
        self._lock = asyncio.Lock()
        self._ttl = ttl_seconds
        self._gc_task: asyncio.Task | None = None

    async def start(self) -> None:
        """Start the background TTL eviction loop. Idempotent."""
        if self._gc_task is None or self._gc_task.done():
            self._gc_task = asyncio.create_task(self._gc_loop())

    async def stop(self) -> None:
        if self._gc_task is not None:
            self._gc_task.cancel()
            try:
                await self._gc_task
            except asyncio.CancelledError:
                pass
            except Exception:
                logger.exception("Error while awaiting _gc_task in stop()")
            self._gc_task = None

    async def put(
        self,
        *,
        tool: str,
        args: dict[str, Any],
        user_sub: str,
        summary: str = "",
        current_state: Any = None,
        proposed_state: Any = None,
    ) -> str:
        """Store a proposed action and return its unique action_id.

        The caller's bearer token is intentionally NOT persisted — execute_action
        re-resolves auth from the live request token at execute-time.
        """
        action_id = f"act_{uuid.uuid4().hex[:16]}"
        # Deep-copy mutable inputs so post-store mutations by the caller can't
        # change what we eventually execute.
        item = _Item(
            action_id=action_id,
            tool=tool,
            args=copy.deepcopy(args),
            user_sub=user_sub,
            expires_at=time.time() + self._ttl,
            summary=summary,
            current_state=copy.deepcopy(current_state),
            proposed_state=copy.deepcopy(proposed_state),
        )
        async with self._lock:
            self._items[action_id] = item
        return action_id

    async def take(self, *, action_id: str, user_sub: str) -> _Item | None:
        """Pop an item by id, validating the requesting user matches.

        Returns None when the action is missing, expired, or owned by another user.
        Callers should map None → 404 (or 403 if they want to distinguish).
        """
        async with self._lock:
            item = self._items.get(action_id)
            if item is None:
                return None
            if item.expires_at < time.time():
                # Expired entries are safe to drop on any caller's behalf.
                self._items.pop(action_id, None)
                logger.info("action %s expired before execute", action_id)
                return None
            if item.user_sub != user_sub:
                # Do NOT pop — leaving it lets the rightful owner still consume it.
                logger.warning(
                    "action %s execute denied: subject mismatch (proposed by %s, executed by %s)",
                    action_id,
                    _hash_user_subject(item.user_sub),
                    _hash_user_subject(user_sub),
                )
                return None
            self._items.pop(action_id, None)
            return item

    async def _gc_loop(self) -> None:
        # Sweep at 1/4 of TTL so an action lives at most TTL + sweep_interval.
        interval = max(self._ttl / 4, 5)
        while True:
            try:
                await asyncio.sleep(interval)
                await self._gc_once()
            except asyncio.CancelledError:
                return
            except Exception as e:  # noqa: BLE001 — never let GC die silently
                logger.error("action store gc error: %s", e, exc_info=True)

    async def _gc_once(self) -> None:
        now = time.time()
        async with self._lock:
            expired = [aid for aid, it in self._items.items() if it.expires_at < now]
            for aid in expired:
                del self._items[aid]
        if expired:
            logger.debug("action store gc: evicted %d expired entries", len(expired))


_store: ActionStore | None = None


def get_action_store() -> ActionStore:
    global _store
    if _store is None:
        _store = ActionStore(ttl_seconds=settings.action_ttl_seconds)
    return _store
