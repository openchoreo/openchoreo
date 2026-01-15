# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import re
from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader

_TEMPLATES_DIR = Path(__file__).parent.parent / "templates"
_env: Environment | None = None


def _match_test(value: Any, pattern: str) -> bool:
    """Jinja2 test for regex matching."""
    return re.match(pattern, str(value)) is not None


def _get_env() -> Environment:
    """Get or create the Jinja2 environment."""
    global _env
    if _env is None:
        _env = Environment(
            loader=FileSystemLoader(_TEMPLATES_DIR),
            trim_blocks=True,
            lstrip_blocks=True,
        )
        _env.tests["match"] = _match_test
    return _env


def render(template_path: str, context: dict[str, Any]) -> str:
    """
    Render a Jinja2 template with the given context.

    Args:
        template_path: Path to template relative to templates directory (e.g., 'middleware/metrics.j2')
        context: Dictionary of variables to pass to the template

    Returns:
        Rendered template as a string
    """
    env = _get_env()
    template = env.get_template(template_path)
    return template.render(**context)
