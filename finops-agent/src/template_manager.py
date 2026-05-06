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
