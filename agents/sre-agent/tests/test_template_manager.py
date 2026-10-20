# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the Jinja2 template manager."""

from jinja2 import DictLoader, Environment

import src.templates as tm
from common.template_manager import _match_test


def test_match_test_matches_prefix():
    assert _match_test("ERROR", "ERR") is True
    assert _match_test("INFO", "ERR") is False


def test_render_substitutes_context(monkeypatch):
    env = Environment(loader=DictLoader({"t.j2": "Hello {{ name }}"}))
    env.tests["match"] = _match_test
    monkeypatch.setattr(tm._manager, "_env", env)
    assert tm.render("t.j2", {"name": "world"}) == "Hello world"


def test_render_exposes_match_test(monkeypatch):
    env = Environment(
        loader=DictLoader({"t.j2": "{% if v is match('E') %}yes{% else %}no{% endif %}"})
    )
    env.tests["match"] = _match_test
    monkeypatch.setattr(tm._manager, "_env", env)
    assert tm.render("t.j2", {"v": "ERROR"}) == "yes"
    assert tm.render("t.j2", {"v": "INFO"}) == "no"


def test_get_env_registers_match_test(monkeypatch):
    monkeypatch.setattr(tm._manager, "_env", None)
    env = tm._manager._get_env()
    assert "match" in env.tests


def _make_scope(environment: str, release_line: str | None = None):
    from src.helpers import AlertScope

    return AlertScope(
        namespace="default",
        project="url-shortener",
        project_uid="proj-uid",
        environment=environment,
        environment_uid=f"{environment}-uid",
        component="greeter",
        component_uid="comp-uid",
        release_line=release_line,
    )


def _make_tool(name: str):
    from types import SimpleNamespace

    return SimpleNamespace(name=name)


def test_rca_prompt_scopes_resource_binding_selection():
    rendered = tm.render(
        "prompts/rca_agent_prompt.j2",
        {
            "openchoreo_tools": [_make_tool("list_resource_release_bindings")],
            "observability_tools": [],
            "scope": _make_scope("staging"),
        },
    )
    assert "`resource_name`" in rendered
    assert "staging" in rendered


def test_rca_prompt_reflects_different_scoped_environments():
    oc_tools = [_make_tool("list_resource_release_bindings")]
    dev = tm.render(
        "prompts/rca_agent_prompt.j2",
        {"openchoreo_tools": oc_tools, "observability_tools": [], "scope": _make_scope("development")},
    )
    prod = tm.render(
        "prompts/rca_agent_prompt.j2",
        {"openchoreo_tools": oc_tools, "observability_tools": [], "scope": _make_scope("production")},
    )
    assert "development" in dev
    assert "production" not in dev
    assert "production" in prod
    assert "development" not in prod


def test_remed_prompt_scopes_resource_binding_selection():
    rendered = tm.render(
        "prompts/remed_agent_prompt.j2",
        {"tools": [], "scope": _make_scope("staging")},
    )
    assert (
        "Call with `namespace_name` and the exact `resource_name` from the ComponentRelease dependency. "
        "Select the summary whose `environment` is `staging`."
    ) in rendered


def test_remed_prompt_separates_schema_source_by_target_kind():
    rendered = tm.render(
        "prompts/remed_agent_prompt.j2",
        {"tools": [], "scope": _make_scope("staging")},
    )
    fields_bullet = next(
        line for line in rendered.splitlines() if line.strip().startswith("- `fields`:")
    )
    assert "get_component_release_schema" in fields_bullet
    assert "get_resource_release_binding" in fields_bullet

    constraints_line = next(
        line for line in rendered.splitlines() if "get_component_release_schema" in line and "ReleaseBinding`" in line
    )
    assert "get_resource_release_binding" in constraints_line


def test_rca_prompt_scopes_alerting_component_to_its_release_line():
    oc_tools = [_make_tool("list_release_bindings"), _make_tool("get_release_line")]
    with_line = tm.render(
        "prompts/rca_agent_prompt.j2",
        {
            "openchoreo_tools": oc_tools,
            "observability_tools": [],
            "scope": _make_scope("dev", "canary"),
        },
    )
    without_line = tm.render(
        "prompts/rca_agent_prompt.j2",
        {"openchoreo_tools": oc_tools, "observability_tools": [], "scope": _make_scope("dev")},
    )
    assert "MUST also set `release_line` to `canary`" in with_line
    assert "`canary` for <comp:greeter>" in with_line
    assert "`get_release_line`" in with_line
    assert "MUST also set `release_line`" not in without_line
    assert (
        "for a dependency, the `releaseLine` that the consumer's dependency entry names"
        in without_line
    )


def test_rca_request_names_the_release_line():
    alert = {
        "id": "a1",
        "value": 5,
        "timestamp": "2026-10-20T00:00:00Z",
        "rule": {"name": "errors"},
    }
    rendered = tm.render(
        "api/rca_request.j2", {"alert": alert, "meta": None, "scope": _make_scope("dev", "canary")}
    )
    assert "- **Release Line:** canary" in rendered
    plain = tm.render(
        "api/rca_request.j2", {"alert": alert, "meta": None, "scope": _make_scope("dev")}
    )
    assert "Release Line" not in plain


def test_remed_prompt_targets_the_alerted_line_binding():
    rendered = tm.render(
        "prompts/remed_agent_prompt.j2",
        {"tools": [], "scope": _make_scope("dev", "canary")},
    )
    assert "`canary` for <comp:greeter>, the line the alert fired on" in rendered
    assert "Never patch another line's binding." in rendered
