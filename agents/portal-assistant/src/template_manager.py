# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pathlib import Path

from common.template_manager import TemplateManager, _match_test  # noqa: F401

_manager = TemplateManager(Path(__file__).parent / "templates")
render = _manager.render
preload = _manager.preload
