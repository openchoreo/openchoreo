# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""What a button the user clicked in the drawer asks for.

Every button sends its A2UI event, its ``name`` and ``context``, with its
message as the message's ``action`` (see ChatAction in api/agent_routes.py),
and it stays on that message in the chat history. The check authoring flow
branches on the action of a decision button below, and never reads its words.
Only a message the user typed (no action, or the card's own answer box) is
read from its text, so typing "yes" still works like clicking Yes.
"""

from __future__ import annotations

from typing import Any

# Decision buttons: the flow follows their action alone.
CONFIRM_EDIT = "confirm_edit"  # Yes to an edit's one proposal
REJECT_EDIT = "reject_edit"  # No, or Keep the current rule
KEEP_RULE = "keep_rule"  # Keep the rule as it is
PICK_PROPOSAL = "pick_proposal"  # one of numbered proposals: context.choice
TEST = "test"  # Test on an entity: context.entityRef
SKIP_TEST = "skip_test"  # a save card's Skip the test
CREATE_SEPARATE = "create_separate"  # Create a separate check: context.check
EXPLAIN_RULE = "explain_rule"  # an edit chat's Explain this rule
TRY_EXAMPLE = "try_example"  # a beginner's practice example

DECISIONS = frozenset(
    {
        CONFIRM_EDIT,
        REJECT_EDIT,
        KEEP_RULE,
        PICK_PROPOSAL,
        TEST,
        SKIP_TEST,
        CREATE_SEPARATE,
        EXPLAIN_RULE,
        TRY_EXAMPLE,
    }
)

# The answer box on a card: what the user typed there.
TYPE_ANSWER = "type_answer"

# The longest value a button's context may carry: a change card's button
# carries the rule it picks (see a2ui._rule_context).
CONTEXT_VALUE_MAX = 2000


def action_of(message: dict[str, Any] | None) -> dict[str, Any] | None:
    """The message's action, or None when the user typed it."""
    action = (message or {}).get("action")
    if not isinstance(action, dict) or action.get("name") == TYPE_ANSWER:
        return None
    return action


def decision_of(message: dict[str, Any] | None) -> str | None:
    """The decision button the message came from, or None."""
    action = action_of(message)
    name = action.get("name") if action else None
    return name if name in DECISIONS else None


def is_read_as_text(message: dict[str, Any] | None) -> bool:
    """Whether the flow reads the message's words: anything but a decision
    button (a typed message, or a button that sends a request in words)."""
    return decision_of(message) is None


def context_of(message: dict[str, Any] | None) -> dict[str, Any]:
    action = action_of(message)
    context = action.get("context") if action else None
    return context if isinstance(context, dict) else {}


def for_model(messages: list[dict[str, Any]]) -> list[dict[str, str]]:
    """The messages as the model gets them: role and content only."""
    return [{"role": m.get("role", ""), "content": m.get("content") or ""} for m in messages]
