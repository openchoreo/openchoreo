# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ActionStore — single-shot semantics and authz invariants."""
import asyncio
import time

import pytest

from src.agent.action_store import ActionStore


@pytest.fixture
async def store():
    s = ActionStore(ttl_seconds=60)
    yield s
    await s.stop()


async def _put(store, *, user_sub="user-a", tool="create_x", args=None):
    return await store.put(
        tool=tool,
        args=args or {"x": 1},
        user_sub=user_sub,
        jwt="jwt-stub",
        summary="proposed",
    )


async def test_put_then_take_returns_item(store):
    aid = await _put(store)
    item = await store.take(action_id=aid, user_sub="user-a")
    assert item is not None
    assert item.tool == "create_x"
    assert item.args == {"x": 1}


async def test_take_is_one_shot(store):
    aid = await _put(store)
    first = await store.take(action_id=aid, user_sub="user-a")
    second = await store.take(action_id=aid, user_sub="user-a")
    assert first is not None
    assert second is None


async def test_take_with_wrong_user_returns_none(store):
    aid = await _put(store, user_sub="user-a")
    item = await store.take(action_id=aid, user_sub="user-b")
    assert item is None


async def test_take_with_unknown_id_returns_none(store):
    item = await store.take(action_id="act_nope", user_sub="user-a")
    assert item is None


async def test_expired_item_returns_none():
    store = ActionStore(ttl_seconds=60)
    aid = await store.put(
        tool="x", args={}, user_sub="u", jwt="j", summary="",
    )
    # Force expiry by mutating the stored item directly.
    store._items[aid].expires_at = time.time() - 1
    item = await store.take(action_id=aid, user_sub="u")
    assert item is None


async def test_concurrent_takes_only_one_wins(store):
    aid = await _put(store)
    results = await asyncio.gather(
        store.take(action_id=aid, user_sub="user-a"),
        store.take(action_id=aid, user_sub="user-a"),
        store.take(action_id=aid, user_sub="user-a"),
    )
    successes = [r for r in results if r is not None]
    assert len(successes) == 1
