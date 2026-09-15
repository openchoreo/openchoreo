# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging

import pytest

from src.extensions import ExtensionConfigError, load_extensions


def _write_skill(root, agent, name, frontmatter, body="Do the thing."):
    directory = root / agent / "skills" / name
    directory.mkdir(parents=True)
    (directory / "SKILL.md").write_text(f"---\n{frontmatter}\n---\n{body}\n")
    return directory


def _write_mcp(root, agent, servers):
    directory = root / agent
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "mcp.json").write_text(json.dumps({"mcpServers": servers}))


def test_missing_directory_is_not_an_error(tmp_path):
    assert not load_extensions(tmp_path, "rca")


def test_each_agent_reads_only_its_own_directory(tmp_path):
    _write_mcp(tmp_path, "rca", {"pagerduty": {"url": "https://pd.example/mcp"}})
    _write_mcp(tmp_path, "remediation", {"grafana": {"url": "https://gf.example/mcp"}})

    assert [s.name for s in load_extensions(tmp_path, "rca").servers] == ["pagerduty"]
    assert [s.name for s in load_extensions(tmp_path, "remediation").servers] == ["grafana"]


def test_server_defaults_to_streamable_http(tmp_path):
    _write_mcp(tmp_path, "rca", {"pagerduty": {"url": "https://pd.example/mcp"}})

    connection = load_extensions(tmp_path, "rca").servers[0].connection

    assert connection == {"transport": "streamable_http", "url": "https://pd.example/mcp"}


def test_header_secrets_resolve_from_the_environment(tmp_path, monkeypatch):
    monkeypatch.setenv("PD_TOKEN", "s3cret")
    _write_mcp(
        tmp_path,
        "rca",
        {"pagerduty": {"url": "https://pd.example/mcp", "headers": {"Authorization": "Bearer ${PD_TOKEN}"}}},
    )

    connection = load_extensions(tmp_path, "rca").servers[0].connection

    assert connection["headers"] == {"Authorization": "Bearer s3cret"}


def test_unset_header_secret_is_an_error(tmp_path):
    _write_mcp(
        tmp_path,
        "rca",
        {"pagerduty": {"url": "https://pd.example/mcp", "headers": {"Authorization": "Bearer ${NOPE}"}}},
    )

    with pytest.raises(ExtensionConfigError, match="NOPE"):
        load_extensions(tmp_path, "rca")


def test_stdio_server_is_rejected(tmp_path):
    _write_mcp(tmp_path, "rca", {"local": {"command": "npx", "args": ["-y", "some-server"]}})

    with pytest.raises(ExtensionConfigError, match="only HTTP transports"):
        load_extensions(tmp_path, "rca")


def test_skill_frontmatter_is_parsed(tmp_path):
    _write_skill(tmp_path, "rca", "runbook", "name: runbook\ndescription: Payment triage.")

    skill = load_extensions(tmp_path, "rca").skills[0]

    assert (skill.name, skill.description, skill.body) == ("runbook", "Payment triage.", "Do the thing.")


def test_skill_name_must_match_its_directory(tmp_path):
    _write_skill(tmp_path, "rca", "runbook", "name: other\ndescription: Payment triage.")

    with pytest.raises(ExtensionConfigError, match="not its directory"):
        load_extensions(tmp_path, "rca")


def test_skill_with_supporting_files_is_rejected(tmp_path):
    directory = _write_skill(tmp_path, "rca", "runbook", "name: runbook\ndescription: Triage.")
    (directory / "scripts").mkdir()

    with pytest.raises(ExtensionConfigError, match="only a single SKILL.md"):
        load_extensions(tmp_path, "rca")


def test_context_is_loaded_and_stripped(tmp_path):
    (tmp_path / "rca").mkdir(parents=True)
    (tmp_path / "rca" / "CONTEXT.md").write_text("\n- Staging DNS is flaky.\n\n")

    assert load_extensions(tmp_path, "rca").context == "- Staging DNS is flaky."


def test_absent_context_is_empty(tmp_path):
    _write_mcp(tmp_path, "rca", {"pagerduty": {"url": "https://pd.example/mcp"}})

    assert load_extensions(tmp_path, "rca").context == ""


def test_oversized_context_warns_but_loads(tmp_path, caplog):
    (tmp_path / "rca").mkdir(parents=True)
    (tmp_path / "rca" / "CONTEXT.md").write_text("x" * 9000)

    with caplog.at_level(logging.WARNING, logger="src.extensions.config"):
        extensions = load_extensions(tmp_path, "rca")

    assert len(extensions.context) == 9000
    assert "sent with every request" in caplog.text
