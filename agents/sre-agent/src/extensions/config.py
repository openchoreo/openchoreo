# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

logger = logging.getLogger(__name__)

_ENV_REF = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}")
_FRONTMATTER = re.compile(r"\A---\n(.*?)\n---\n?(.*)\Z", re.DOTALL)
_CONTEXT_WARN_BYTES = 8 * 1024


class ExtensionConfigError(Exception):
    pass


@dataclass(frozen=True)
class ExternalServer:
    name: str
    connection: dict[str, Any]


@dataclass(frozen=True)
class Skill:
    name: str
    description: str
    body: str


@dataclass(frozen=True)
class Extensions:
    servers: tuple[ExternalServer, ...] = ()
    skills: tuple[Skill, ...] = ()
    context: str = ""


def _resolve_env_refs(value: Any) -> Any:
    if isinstance(value, dict):
        return {k: _resolve_env_refs(v) for k, v in value.items()}
    if not isinstance(value, str):
        return value

    missing = [m.group(1) for m in _ENV_REF.finditer(value) if m.group(1) not in os.environ]
    if missing:
        raise ExtensionConfigError(f"references unset environment variables: {', '.join(missing)}")
    return _ENV_REF.sub(lambda m: os.environ[m.group(1)], value)


def _load_servers(path: Path) -> tuple[ExternalServer, ...]:
    if not path.is_file():
        return ()
    try:
        document = json.loads(path.read_text())
    except json.JSONDecodeError as e:
        raise ExtensionConfigError(f"{path} is not valid JSON: {e}") from e
    if not isinstance(document, dict):
        raise ExtensionConfigError(f"{path} must contain a JSON object")

    specs = document.get("mcpServers") or {}
    if not isinstance(specs, dict):
        raise ExtensionConfigError(f"{path} has a non-object 'mcpServers'")

    servers = []
    for name, spec in specs.items():
        try:
            servers.append(ExternalServer(name=name, connection=_connection(spec)))
        except ExtensionConfigError as e:
            raise ExtensionConfigError(f"mcp server {name!r} {e}") from e
    return tuple(servers)


def _connection(spec: dict[str, Any]) -> dict[str, Any]:
    if not isinstance(spec, dict):
        raise ExtensionConfigError("must be an object")
    transport = str(spec.get("type") or spec.get("transport") or "http").lower()
    if transport not in ("http", "streamable_http"):
        raise ExtensionConfigError(f"has unsupported transport {transport!r}")
    if not spec.get("url"):
        raise ExtensionConfigError("is missing 'url'")

    url = str(spec["url"])
    connection: dict[str, Any] = {"transport": "streamable_http", "url": url}
    headers = spec.get("headers")
    if headers:
        if not isinstance(headers, dict):
            raise ExtensionConfigError("has non-object 'headers'")
        if not url.lower().startswith("https://"):
            raise ExtensionConfigError(
                "sends headers over a plaintext URL: use https so credentials are not "
                "transmitted in the clear"
            )
        connection["headers"] = _resolve_env_refs(headers)
    return connection


def _load_skills(path: Path) -> tuple[Skill, ...]:
    if not path.is_dir():
        return ()

    skills = []
    for directory in sorted(p for p in path.iterdir() if p.is_dir() and not p.name.startswith(".")):
        try:
            skills.append(_read_skill(directory))
        except ExtensionConfigError as e:
            raise ExtensionConfigError(f"skill {directory.name!r} {e}") from e
    return tuple(skills)


def _read_skill(directory: Path) -> Skill:
    unexpected = sorted(
        p.name
        for p in directory.iterdir()
        if p.name != "SKILL.md" and not p.name.startswith(".")
    )
    if unexpected:
        raise ExtensionConfigError(
            f"contains {', '.join(unexpected)}: only a single SKILL.md file is supported as of now"
        )
    if not (directory / "SKILL.md").is_file():
        raise ExtensionConfigError("has no SKILL.md")

    match = _FRONTMATTER.match((directory / "SKILL.md").read_text())
    if match is None:
        raise ExtensionConfigError("has no YAML frontmatter")
    try:
        meta = yaml.safe_load(match.group(1)) or {}
    except yaml.YAMLError as e:
        raise ExtensionConfigError(f"has invalid frontmatter: {e}") from e
    if not isinstance(meta, dict):
        raise ExtensionConfigError("has non-object frontmatter")

    name, description = meta.get("name"), meta.get("description")
    if not name or not description:
        raise ExtensionConfigError("frontmatter needs both 'name' and 'description'")
    if name != directory.name:
        raise ExtensionConfigError(f"declares name {name!r}, which is not its directory")

    return Skill(name=name, description=str(description), body=match.group(2).strip())


def _load_context(path: Path) -> str:
    if not path.is_file():
        return ""
    context = path.read_text().strip()
    size = len(context.encode())
    if size > _CONTEXT_WARN_BYTES:
        logger.warning(
            "%s is %d bytes and is sent with every request; consider moving parts of it "
            "into a skill",
            path,
            size,
        )
    return context


def load_extensions(root: str | Path, agent_name: str) -> Extensions:
    directory = Path(root) / agent_name
    if not directory.is_dir():
        logger.debug("No extensions directory at %s", directory)
        return Extensions()

    extensions = Extensions(
        servers=_load_servers(directory / "mcp.json"),
        skills=_load_skills(directory / "skills"),
        context=_load_context(directory / "CONTEXT.md"),
    )
    if extensions == Extensions():
        logger.debug("Extensions directory %s defines nothing", directory)
    else:
        logger.info(
            "Loaded extensions for %s: servers=%s skills=%s context=%s",
            agent_name,
            [s.name for s in extensions.servers],
            [s.name for s in extensions.skills],
            f"{len(extensions.context)} chars" if extensions.context else "none",
        )
    return extensions
