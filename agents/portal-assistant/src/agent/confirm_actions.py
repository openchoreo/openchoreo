# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Save the check change on a confirm card the user clicked.

The card's token carries the signed change (see confirm_card and
pending_actions). The user's click picks only the status to end in and
whether the suggested texts are applied. Everything else comes from the
token, and every call runs through Backstage's MCP actions as the user,
so Backstage's permissions and validator decide. No model is involved.
"""

from __future__ import annotations

import logging
from typing import Any, Literal

from common.auth.bearer import BearerTokenAuth
from src.agent.confirm_card import check_baseline, same_baseline
from src.agent.pending_actions import consume, release
from src.clients.backstage_tools import (
    CREATE_CHECK,
    GET_CHECK,
    PUBLISH_CHECK,
    UNPUBLISH_CHECK,
    UPDATE_CHECK,
)
from src.clients.mcp import BackstageToolError, backstage_tools_as_user, call_backstage_tool

logger = logging.getLogger(__name__)

Status = Literal["draft", "published"]

# create-check / update-check metadata that leaves every field as saved.
_KEEP_METADATA = {"category": "", "rank": 0, "solution": ""}


class ConfirmError(Exception):
    """The change wasn't saved, or it's unknown whether it was; the message says which."""


def _save_args(action: dict[str, Any], apply_texts: bool) -> dict[str, Any]:
    """The update-check / create-check input for the signed change."""
    check = dict(action["check"])
    if action["kind"] == "create":
        metadata = {**_KEEP_METADATA, **action.get("metadata", {})}
        # The card's id: a repeat of this create is refused, not saved twice.
        return {**check, "id": action["check_id"], "metadata": metadata}
    texts = action.get("texts", {}) if apply_texts else {}
    return {
        **check,
        "id": action["check_id"],
        "name": texts.get("name", check["name"]),
        "description": texts.get("description", check["description"]),
        "metadata": {**_KEEP_METADATA},
    }


async def _refuse_if_changed(tools: Any, action: dict[str, Any], token: str) -> None:
    """Raise ConfirmError when the check's rule or retrievers changed since the
    edit card was built: update-check replaces the whole rule, so saving
    would undo someone else's change without a word. The check is read just
    before the save; a change in between those two calls isn't caught."""
    try:
        now = await call_backstage_tool(tools, GET_CHECK, {"id": action["check_id"]})
        current = check_baseline(now["check"])
    # Nothing was sent, so the same card can be tried again.
    except BackstageToolError as exc:
        release(token)
        raise ConfirmError(f"The check wasn't saved: {exc}") from exc
    except Exception as exc:  # noqa: BLE001 — transport failure: say so plainly
        release(token)
        logger.warning("Confirm: reading check %s failed", action["check_id"], exc_info=True)
        raise ConfirmError("The check wasn't saved: Backstage couldn't be reached.") from exc
    if not same_baseline(current, action["baseline"]):
        # The card can't become right again, so it stays used.
        logger.info("Confirm: check %s changed since its card; not saved", action["check_id"])
        raise ConfirmError(
            "This check changed since this card was made, so nothing was saved. "
            "Ask the assistant again for a card based on the check as it is now."
        )


async def _status_after_unclear_change(
    tools: Any, check_id: str, current: str, saved_as: str, doing: str,
) -> dict[str, Any]:
    """The result of a save whose publish or unpublish gave no clear answer:
    the check's status as Backstage reads it now, or, when that read fails
    too, the save and that its final status isn't known."""
    logger.warning("Confirm: %s check %s gave no clear answer; reading it back", doing, check_id)
    try:
        now = (await call_backstage_tool(tools, GET_CHECK, {"id": check_id}))["check"]["status"]
    except Exception:  # noqa: BLE001 — say what is known
        logger.warning("Confirm: reading check %s back failed", check_id, exc_info=True)
        return {
            "check_id": check_id,
            "status": current,
            "message": f"{saved_as}, but Backstage didn't say whether {doing} worked. "
            "Look at the check's status in the checks list.",
        }
    state = "published" if now == "published" else "a draft"
    return {"check_id": check_id, "status": now, "message": f"{saved_as} as {state}."}


async def confirm_check_change(
    token: str,
    *,
    user_sub: str,
    backstage_token: str | None,
    status: Status,
    apply_texts: bool,
) -> dict[str, Any]:
    """Save the change in ``token`` as ``user_sub`` and end in ``status``.

    Returns ``{check_id, status, message}``. Raises InvalidAction for a bad
    token and ConfirmError when nothing was saved, or when Backstage gave no
    clear answer (the card is then used up). When the save works
    but the status change doesn't, returns the saved check's real status
    and a message saying what failed.
    """
    if not backstage_token:
        # Checked before the token is used up, so a retry can still work.
        raise ConfirmError(
            "Your Backstage sign-in didn't reach the assistant. Reload the page and try again."
        )
    action = consume(token, user_sub)
    try:
        tools = await backstage_tools_as_user(BearerTokenAuth(backstage_token))
    # No save was sent, so the same card can be tried again.
    except Exception as exc:  # noqa: BLE001 — transport failure: say so plainly
        release(token)
        logger.warning("Confirm: listing the Backstage tools failed", exc_info=True)
        raise ConfirmError("The check wasn't saved: Backstage couldn't be reached.") from exc
    if action["kind"] == "update":
        await _refuse_if_changed(tools, action, token)
    try:
        if action["kind"] == "create":
            saved = await call_backstage_tool(
                tools, CREATE_CHECK, _save_args(action, apply_texts)
            )
            check_id, current = saved["id"], "draft"
        else:
            saved = await call_backstage_tool(
                tools, UPDATE_CHECK, _save_args(action, apply_texts)
            )
            # The status the check has now, which may have changed since the card was built.
            check_id, current = action["check_id"], saved.get("status", action["status"])
    # Backstage refused, so nothing was saved and the same card can be tried again.
    except BackstageToolError as exc:
        release(token)
        raise ConfirmError(f"The check wasn't saved: {exc}") from exc
    # The save was sent and may have worked (a timeout, or a reply that can't
    # be read): the card stays used, so a retry on this replica can't create
    # the check twice. Used cards are kept per process (see pending_actions),
    # so another replica could still accept it; the README says so.
    except Exception as exc:  # noqa: BLE001 — unknown outcome: say so plainly
        logger.warning("Confirm: saving check gave no clear answer", exc_info=True)
        raise ConfirmError(
            "Backstage didn't say whether the check was saved. "
            "Look for it in the checks list before asking the assistant again."
        ) from exc

    saved_as = "Created" if action["kind"] == "create" else "Saved"
    logger.info("Confirm: %s check %s (kind=%s)", saved_as.lower(), check_id, action["kind"])
    if status == current:
        state = "published" if current == "published" else "a draft"
        return {"check_id": check_id, "status": current, "message": f"{saved_as} as {state}."}

    change = PUBLISH_CHECK if status == "published" else UNPUBLISH_CHECK
    try:
        await call_backstage_tool(tools, change, {"id": check_id})
    except Exception as exc:  # noqa: BLE001 — the save stands; report the rest
        doing = "publishing" if status == "published" else "unpublishing"
        if not isinstance(exc, BackstageToolError):
            # No clear answer: the change may have gone through. Read the
            # check's status as the user instead of guessing it.
            return await _status_after_unclear_change(tools, check_id, current, saved_as, doing)
        reason = str(exc)
        state = "published" if current == "published" else "a draft"
        logger.warning("Confirm: %s check %s failed: %s", doing, check_id, reason)
        return {
            "check_id": check_id,
            "status": current,
            "message": f"{saved_as} as {state}, but {doing} failed: {reason}",
        }
    return {
        "check_id": check_id,
        "status": status,
        "message": f"{saved_as} and published."
        if status == "published"
        else f"{saved_as} as a draft.",
    }
