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

    def __bool__(self) -> bool:
        return bool(self.servers or self.skills or self.context)


def _expand(value: Any, *, where: str) -> Any:
    if isinstance(value, dict):
        return {k: _expand(v, where=where) for k, v in value.items()}
    if not isinstance(value, str):
        return value

    def replace(match: re.Match[str]) -> str:
        var = match.group(1)
        resolved = os.environ.get(var)
        if resolved is None:
            raise ExtensionConfigError(f"{where} references ${{{var}}}, which is not set")
        return resolved

    return _ENV_REF.sub(replace, value)


def _parse_server(name: str, spec: dict[str, Any]) -> ExternalServer:
    where = f"mcp server {name!r}"
    if not isinstance(spec, dict):
        raise ExtensionConfigError(f"{where} must be an object")

    if "command" in spec:
        raise ExtensionConfigError(
            f"{where} declares 'command': only HTTP transports are supported, "
            "since a stdio server's binary is not present in this image"
        )

    declared = str(spec.get("type") or spec.get("transport") or "http").lower()
    if declared not in ("http", "streamable_http"):
        raise ExtensionConfigError(f"{where} has unsupported transport {declared!r}")

    url = spec.get("url")
    if not url:
        raise ExtensionConfigError(f"{where} is missing 'url'")

    connection: dict[str, Any] = {"transport": "streamable_http", "url": url}
    headers = spec.get("headers")
    if headers:
        if not isinstance(headers, dict):
            raise ExtensionConfigError(f"{where} has non-object 'headers'")
        connection["headers"] = _expand(headers, where=where)
    return ExternalServer(name=name, connection=connection)


def _load_servers(path: Path) -> tuple[ExternalServer, ...]:
    if not path.is_file():
        return ()
    try:
        document = json.loads(path.read_text())
    except json.JSONDecodeError as e:
        raise ExtensionConfigError(f"{path} is not valid JSON: {e}") from e

    servers = document.get("mcpServers") or {}
    if not isinstance(servers, dict):
        raise ExtensionConfigError(f"{path} has a non-object 'mcpServers'")
    return tuple(_parse_server(name, spec) for name, spec in servers.items())


def _load_skill(directory: Path) -> Skill:
    where = f"skill {directory.name!r}"
    unexpected = sorted(p.name for p in directory.iterdir() if p.name != "SKILL.md")
    if unexpected:
        raise ExtensionConfigError(
            f"{where} contains {', '.join(unexpected)}: only a single SKILL.md is supported"
        )

    match = _FRONTMATTER.match((directory / "SKILL.md").read_text())
    if match is None:
        raise ExtensionConfigError(f"{where} has no YAML frontmatter")

    try:
        meta = yaml.safe_load(match.group(1)) or {}
    except yaml.YAMLError as e:
        raise ExtensionConfigError(f"{where} has invalid frontmatter: {e}") from e
    if not isinstance(meta, dict):
        raise ExtensionConfigError(f"{where} has non-object frontmatter")

    name = meta.get("name")
    description = meta.get("description")
    if not name or not description:
        raise ExtensionConfigError(f"{where} frontmatter needs both 'name' and 'description'")
    if name != directory.name:
        raise ExtensionConfigError(f"{where} declares name {name!r}, which is not its directory")

    return Skill(name=name, description=str(description), body=match.group(2).strip())


def _load_skills(path: Path) -> tuple[Skill, ...]:
    if not path.is_dir():
        return ()
    skills = []
    for directory in sorted(p for p in path.iterdir() if p.is_dir()):
        if not (directory / "SKILL.md").is_file():
            raise ExtensionConfigError(f"skill {directory.name!r} has no SKILL.md")
        skills.append(_load_skill(directory))
    return tuple(skills)


def _load_context(path: Path) -> str:
    if not path.is_file():
        return ""
    context = path.read_text().strip()
    if len(context.encode()) > _CONTEXT_WARN_BYTES:
        logger.warning(
            "%s is %d bytes and is sent with every request; consider moving parts of it "
            "into a skill so it loads only when relevant",
            path,
            len(context.encode()),
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
    if not extensions:
        logger.debug("Extensions directory %s defines no servers or skills", directory)
    else:
        logger.info(
            "Loaded extensions for %s: servers=%s skills=%s context=%s",
            agent_name,
            [s.name for s in extensions.servers],
            [s.name for s in extensions.skills],
            f"{len(extensions.context)} chars" if extensions.context else "none",
        )
    return extensions
