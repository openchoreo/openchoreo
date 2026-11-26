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
from src.agent.pending_actions import consume, release
from src.clients.mcp import BackstageToolError, backstage_tools_as_user, call_backstage_tool

logger = logging.getLogger(__name__)

Status = Literal["draft", "published"]

# create_check / update_check metadata that leaves every field as saved.
_KEEP_METADATA = {"category": "", "rank": 0, "solution": ""}


class ConfirmError(Exception):
    """The change wasn't saved, or it's unknown whether it was; the message says which."""


def _save_args(action: dict[str, Any], apply_texts: bool) -> dict[str, Any]:
    """The update_check / create_check input for the signed change."""
    check = dict(action["check"])
    if action["kind"] == "create":
        metadata = {**_KEEP_METADATA, **action.get("metadata", {})}
        return {**check, "metadata": metadata}
    texts = action.get("texts", {}) if apply_texts else {}
    return {
        **check,
        "id": action["check_id"],
        "name": texts.get("name", check["name"]),
        "description": texts.get("description", check["description"]),
        "metadata": {**_KEEP_METADATA},
    }


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
    try:
        if action["kind"] == "create":
            saved = await call_backstage_tool(
                tools, "create_check", _save_args(action, apply_texts)
            )
            check_id, current = saved["id"], "draft"
        else:
            saved = await call_backstage_tool(
                tools, "update_check", _save_args(action, apply_texts)
            )
            # The status the check has now, which may have changed since the card was built.
            check_id, current = action["check_id"], saved.get("status", action["status"])
    # Backstage refused, so nothing was saved and the same card can be tried again.
    except BackstageToolError as exc:
        release(token)
        raise ConfirmError(f"The check wasn't saved: {exc}") from exc
    # The save was sent and may have worked (a timeout, or a reply that can't
    # be read): the card stays used, so a retry can't create the check twice.
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

    change = "publish_check" if status == "published" else "unpublish_check"
    try:
        await call_backstage_tool(tools, change, {"id": check_id})
    except Exception as exc:  # noqa: BLE001 — the save stands; report the rest
        reason = (
            str(exc) if isinstance(exc, BackstageToolError) else "Backstage couldn't be reached."
        )
        doing = "publishing" if status == "published" else "unpublishing"
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
