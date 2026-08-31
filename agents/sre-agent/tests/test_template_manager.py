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


def _make_scope(environment: str):
    from src.helpers import AlertScope

    return AlertScope(
        namespace="default",
        project="url-shortener",
        project_uid="proj-uid",
        environment=environment,
        environment_uid=f"{environment}-uid",
    )


def _make_tool(name: str):
    from types import SimpleNamespace

    return SimpleNamespace(name=name)


def test_rca_prompt_requires_environment_match_for_resource_binding_selection():
    rendered = tm.render(
        "prompts/rca_agent_prompt.j2",
        {
            "openchoreo_tools": [_make_tool("list_resource_release_bindings")],
            "observability_tools": [],
            "scope": _make_scope("staging"),
        },
    )
    assert "spec.environment" in rendered
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


def test_remed_prompt_requires_environment_match_for_resource_binding_selection():
    rendered = tm.render(
        "prompts/remed_agent_prompt.j2",
        {"tools": [], "scope": _make_scope("staging")},
    )
    assert "spec.environment" in rendered
    assert "staging" in rendered
