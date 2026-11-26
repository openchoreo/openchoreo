# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the per-case tool filter in the agent builder."""

import contextlib
import json
from dataclasses import dataclass

import pytest

from src.agent.builder import (
    _current_rule_summary,
    _filter_tools_for_case,
    _response_model,
)
from src.models import ChatResponse, CheckAuthoringResponse


@dataclass
class _FakeTool:
    name: str
    description: str = ""


def _names(tools):
    return sorted(t.name for t in tools)


class _StopAfterTools(Exception):
    pass


async def _backstage_auth_for(monkeypatch, case_type, backstage_token):
    """What _build_agent passes to get_tools_for_user as backstage_auth."""
    from src.agent import builder

    seen = {}

    async def fake_get_tools_for_user(_user_sub, _auth, backstage_auth=None):
        seen["backstage_auth"] = backstage_auth
        raise _StopAfterTools

    monkeypatch.setattr(builder, "get_tools_for_user", fake_get_tools_for_user)
    with contextlib.suppress(_StopAfterTools):
        await builder._build_agent(
            "openchoreo-token",
            {"case_type": case_type},
            user_sub="u",
            backstage_token=backstage_token,
        )
    return seen["backstage_auth"]


async def test_check_authoring_binds_backstage_tools_to_the_user(monkeypatch):
    auth = await _backstage_auth_for(monkeypatch, "check_authoring", "issued-for-user")
    assert auth._token == "issued-for-user"


async def test_other_cases_keep_the_read_only_backstage_token(monkeypatch):
    assert await _backstage_auth_for(monkeypatch, "build_failure", "issued-for-user") is None


async def test_no_user_token_keeps_the_read_only_backstage_token(monkeypatch):
    assert await _backstage_auth_for(monkeypatch, "check_authoring", None) is None


_CHECK_WRITE_TOOLS = ("create_check", "update_check", "publish_check", "unpublish_check")


@pytest.mark.parametrize(
    "scope",
    [
        pytest.param({"case_type": "check_authoring"}, id="new check"),
        pytest.param(
            {"case_type": "check_authoring", "check_id": "c1", "current_rule": "{}"},
            id="edit",
        ),
        pytest.param({"case_type": "check_authoring", "check_id": "c1"}, id="edit without rule"),
    ],
)
def test_check_writes_never_reach_the_model(scope):
    """Saving is the user's click on the confirm card, run by code. The model
    gets no write tool in check authoring, whoever the tools are bound to:
    the filters below don't look at the token at all. It does get
    validate_check, which saves nothing, to check every rule it drafts."""
    tools = [
        _FakeTool(name=n)
        for n in (
            *_CHECK_WRITE_TOOLS,
            "validate_check",
            "dry_run_check",
            "get_check",
            "list_checks",
        )
    ]

    kept = {t.name for t in _filter_tools_for_case(tools, scope["case_type"])}

    assert not kept & set(_CHECK_WRITE_TOOLS)
    assert "validate_check" in kept


def test_check_authoring_keeps_only_backstage_read_tools():
    tools = [
        _FakeTool(name=n)
        for n in (
            # Backstage check tools.
            "list_checks",
            "get_check",
            "list_fact_schemas",
            "get_entity_facts",
            "dry_run_check",
            "validate_check",
            "create_check",
            # OpenChoreo / observability tools the case doesn't need.
            "list_components",
            "query_component_logs",
            "create_component",
        )
    ]

    filtered = _filter_tools_for_case(tools, "check_authoring")

    assert _names(filtered) == [
        "dry_run_check",
        "get_check",
        "get_entity_facts",
        "list_checks",
        "list_fact_schemas",
        "validate_check",
    ]


@pytest.mark.parametrize("case_type", [None, "unknown_case"])
def test_other_chats_get_no_backstage_check_tools(case_type):
    tools = [
        _FakeTool(name=n)
        for n in (
            "list_checks",
            "get_check",
            "list_fact_schemas",
            "get_entity_facts",
            "dry_run_check",
            "validate_check",
            "list_components",
            "query_component_logs",
        )
    ]

    filtered = _filter_tools_for_case(tools, case_type)

    assert _names(filtered) == ["list_components", "query_component_logs"]


def test_check_authoring_drops_create_check_as_a_write():
    filtered = _filter_tools_for_case([_FakeTool(name="create_check")], "check_authoring")

    assert filtered == []


# ── _current_rule_summary ──────────────────────────────────────────────


def _edit_scope(rule, **text):
    return {"check_id": "abc", "current_rule": json.dumps(rule), **text}


def test_summary_of_a_single_condition():
    scope = _edit_scope(
        {"all": [{"fact": "githubAllowsDeletions", "operator": "equal", "value": False}]}
    )
    assert _current_rule_summary(scope) == "`githubAllowsDeletions` `equal` `false`"


def test_summary_joins_groups_and_parenthesises_nested_ones():
    rule = {
        "all": [
            {"fact": "githubRequiresPullRequest", "operator": "equal", "value": True},
            {
                "any": [
                    {"fact": "githubVisibility", "operator": "equal", "value": "private"},
                    {"fact": "githubVisibility", "operator": "in", "value": ["internal"]},
                ]
            },
        ]
    }
    assert _current_rule_summary(_edit_scope(rule)) == (
        "`githubRequiresPullRequest` `equal` `true` AND "
        '(`githubVisibility` `equal` `private` OR `githubVisibility` `in` `["internal"]`)'
    )


def test_summary_handles_not_and_a_conditions_wrapper():
    rule = {
        "conditions": {
            "not": {"any": [{"fact": "githubVisibility", "operator": "equal", "value": "public"}]}
        }
    }
    assert _current_rule_summary(_edit_scope(rule)) == "NOT (`githubVisibility` `equal` `public`)"


def test_summary_is_none_without_a_parseable_rule():
    assert _current_rule_summary({"check_id": "abc"}) is None


@pytest.mark.parametrize(
    ("case_type", "model"),
    [
        ("check_authoring", CheckAuthoringResponse),
        ("build_failure", ChatResponse),
        ("runtime_debug", ChatResponse),
        (None, ChatResponse),
    ],
)
def test_only_check_authoring_answers_with_a_card(case_type, model):
    assert _response_model(case_type) is model
