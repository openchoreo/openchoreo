# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import re
from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader


def _match_test(value: Any, pattern: str) -> bool:
    return re.match(pattern, str(value)) is not None


class TemplateManager:
    def __init__(self, templates_dir: Path) -> None:
        self._templates_dir = templates_dir
        self._env: Environment | None = None

    def _get_env(self) -> Environment:
        if self._env is None:
            env = Environment(
                loader=FileSystemLoader(self._templates_dir),
                trim_blocks=True,
                lstrip_blocks=True,
            )
            env.tests["match"] = _match_test  # type: ignore[assignment]
            self._env = env
        return self._env

    def render(self, template_path: str, context: dict[str, Any]) -> str:
        env = self._get_env()
        template = env.get_template(template_path)
        return template.render(**context)

    def preload(self, template_paths: list[str]) -> None:
        """Resolve and parse the given templates eagerly.

        Called from the FastAPI lifespan so a missing or malformed prompt
        template fails the deploy at startup rather than emitting a 500 the
        first time a user sends a chat. ``Environment.get_template`` raises
        ``TemplateNotFound`` (or ``TemplateSyntaxError`` on parse errors) —
        let the caller log + re-raise so uvicorn aborts the process.
        """
        env = self._get_env()
        for path in template_paths:
            # get_template loads, parses, and compiles the template — raises
            # TemplateNotFound / TemplateSyntaxError on any problem. The
            # compiled result is cached on the Environment, so the first
            # request-time render() doesn't pay this cost again.
            env.get_template(path)
