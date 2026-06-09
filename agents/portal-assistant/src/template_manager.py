# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import re
from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader

_TEMPLATES_DIR = Path(__file__).parent / "templates"
_env: Environment | None = None


def _match_test(value: Any, pattern: str) -> bool:
    return re.match(pattern, str(value)) is not None


def _get_env() -> Environment:
    global _env
    if _env is None:
        _env = Environment(
            loader=FileSystemLoader(_TEMPLATES_DIR),
            trim_blocks=True,
            lstrip_blocks=True,
        )
        _env.tests["match"] = _match_test  # type: ignore[assignment]
    return _env


def render(template_path: str, context: dict[str, Any]) -> str:
    env = _get_env()
    template = env.get_template(template_path)
    return template.render(**context)


def preload(template_paths: list[str]) -> None:
    """Resolve and parse the given templates eagerly.

    Called from the FastAPI lifespan so a missing or malformed prompt
    template fails the deploy at startup rather than emitting a 500 the
    first time a user sends a chat. ``Environment.get_template`` raises
    ``TemplateNotFound`` (or ``TemplateSyntaxError`` on parse errors) —
    let the caller log + re-raise so uvicorn aborts the process.
    """
    env = _get_env()
    for path in template_paths:
        # get_template loads, parses, and compiles the template — raises
        # TemplateNotFound / TemplateSyntaxError on any problem. The
        # compiled result is cached on the Environment, so the first
        # request-time render() doesn't pay this cost again.
        env.get_template(path)
