# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the signed check changes behind the confirm card."""

import base64
import json

import pytest

from src.agent import pending_actions
from src.agent.pending_actions import InvalidAction, consume, sign

_ACTION = {"kind": "update", "check_id": "c1", "rule": '{"all":[]}'}


@pytest.fixture(autouse=True)
def _fixed_secret(monkeypatch):
    monkeypatch.setattr(pending_actions.settings, "portal_assistant_action_secret", "test-secret")
    pending_actions._USED.clear()


def test_the_signed_action_comes_back_unchanged():
    assert consume(sign(_ACTION, "user-a"), "user-a") == _ACTION


def test_a_token_works_only_once():
    token = sign(_ACTION, "user-a")
    consume(token, "user-a")

    with pytest.raises(InvalidAction, match="already confirmed"):
        consume(token, "user-a")


def test_another_user_cannot_confirm_it():
    with pytest.raises(InvalidAction, match="another user"):
        consume(sign(_ACTION, "user-a"), "user-b")


def test_an_expired_token_is_refused():
    with pytest.raises(InvalidAction, match="expired"):
        consume(sign(_ACTION, "user-a", ttl_s=-1), "user-a")


def test_a_changed_action_breaks_the_signature():
    body, _, signature = sign(_ACTION, "user-a").partition(".")
    payload = json.loads(base64.urlsafe_b64decode(body + "=" * (-len(body) % 4)))
    payload["action"]["check_id"] = "someone-elses-check"
    forged = base64.urlsafe_b64encode(json.dumps(payload).encode()).rstrip(b"=").decode()

    with pytest.raises(InvalidAction, match="isn't valid"):
        consume(f"{forged}.{signature}", "user-a")


def test_a_token_signed_with_another_secret_is_refused(monkeypatch):
    token = sign(_ACTION, "user-a")
    monkeypatch.setattr(pending_actions.settings, "portal_assistant_action_secret", "other-secret")

    with pytest.raises(InvalidAction, match="isn't valid"):
        consume(token, "user-a")


@pytest.mark.parametrize("token", ["", "no-dot", "a.b", "....."])
def test_malformed_tokens_are_refused(token):
    with pytest.raises(InvalidAction):
        consume(token, "user-a")


def test_a_failed_attempt_does_not_use_up_the_token():
    token = sign(_ACTION, "user-a")
    with pytest.raises(InvalidAction):
        consume(token, "user-b")

    assert consume(token, "user-a") == _ACTION


def test_without_a_configured_secret_tokens_still_work_on_this_process(monkeypatch):
    monkeypatch.setattr(pending_actions.settings, "portal_assistant_action_secret", "")

    assert consume(sign(_ACTION, "user-a"), "user-a") == _ACTION
