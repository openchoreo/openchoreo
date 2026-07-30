# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pathlib import Path

from common.template_manager import TemplateManager

_manager = TemplateManager(Path(__file__).parent)
render = _manager.render
preload = _manager.preload
