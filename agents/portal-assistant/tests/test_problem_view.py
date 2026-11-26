# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for reading a failed check tool for the problem card."""

import pytest

from src.agent.problem_view import ToolError, failed_test_note, problem_view


@pytest.mark.parametrize(
    ("error", "title", "summary"),
    [
        (
            ToolError(
                "dry_run_check", "Error: No entity component:default/order-apx in the catalog."
            ),
            "Couldn't test the check",
            "order-apx isn't in the catalog. Check the component's name.",
        ),
        (
            ToolError(
                "validate_check", "Error: The signed-in user is not allowed to manage checks."
            ),
            "Couldn't check the rule",
            "You don't have permission to do this. Ask an admin for access to manage checks.",
        ),
        (
            ToolError("list_checks", "Error: Request failed with status 503"),
            "Couldn't list the checks",
            "The check service isn't available right now. Try again in a moment.",
        ),
        (
            ToolError(
                "dry_run_check",
                "Error executing tool dry_run_check: The check could not be evaluated: "
                "Unknown operator 'between'.",
            ),
            "Couldn't test the check",
            "The rule couldn't be run: Unknown operator 'between'.",
        ),
        (
            ToolError("get_check", "Error: Something else broke."),
            "Couldn't load the check",
            "Something else broke.",
        ),
        (ToolError("some_new_tool", "Error: "), "Something went wrong", "The tool gave no answer."),
    ],
)
def test_a_failed_tool_reads_in_plain_words(error, title, summary):
    view = problem_view(error)

    assert (view.tone, view.title, view.summary) == ("warn", title, summary)


def test_a_failed_test_reads_as_the_save_cards_note():
    error = ToolError(
        "dry_run_check", "Error: No entity component:default/no-such-service in the catalog."
    )

    assert failed_test_note(error) == (
        "Couldn't test the check: no-such-service isn't in the catalog. Check the component's name."
    )


def test_only_a_failed_test_is_the_save_cards_note():
    assert (
        failed_test_note(ToolError("validate_check", "Error: Request failed with status 503"))
        is None
    )


def test_an_unknown_error_is_shown_without_secrets_or_internal_addresses():
    # A test run by code gets Backstage's error straight, not through the
    # tool error middleware.
    view = problem_view(
        ToolError(
            "dry_run_check",
            "Fetch failed for http://backstage.openchoreo.svc:7007/api "
            "with Bearer eyJhbGciOi.abc from 10.0.0.5:8080",
        )
    )

    assert "eyJhbGciOi" not in view.summary
    assert ".svc" not in view.summary
    assert "10.0.0.5" not in view.summary
    assert "[REDACTED]" in view.summary
