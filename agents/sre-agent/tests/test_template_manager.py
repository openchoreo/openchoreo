# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the Jinja2 template manager."""

from jinja2 import DictLoader, Environment

import src.template_manager as tm
from src.template_manager import _match_test


def test_match_test_matches_prefix():
    assert _match_test("ERROR", "ERR") is True
    assert _match_test("INFO", "ERR") is False


def test_render_substitutes_context(monkeypatch):
    env = Environment(loader=DictLoader({"t.j2": "Hello {{ name }}"}))
    env.tests["match"] = _match_test
    monkeypatch.setattr(tm, "_env", env)
    assert tm.render("t.j2", {"name": "world"}) == "Hello world"


def test_render_exposes_match_test(monkeypatch):
    env = Environment(
        loader=DictLoader({"t.j2": "{% if v is match('E') %}yes{% else %}no{% endif %}"})
    )
    env.tests["match"] = _match_test
    monkeypatch.setattr(tm, "_env", env)
    assert tm.render("t.j2", {"v": "ERROR"}) == "yes"
    assert tm.render("t.j2", {"v": "INFO"}) == "no"


def test_get_env_registers_match_test(monkeypatch):
    monkeypatch.setattr(tm, "_env", None)
    env = tm._get_env()
    assert "match" in env.tests
