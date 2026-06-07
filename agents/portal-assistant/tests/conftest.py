# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Shared pytest config.

``pyproject.toml`` already sets ``asyncio_mode = "auto"`` so async test
functions don't need explicit decorators. This file is a placeholder
for future fixtures (e.g. a sandboxed Settings, a stub LLM, an in-mem
MCP) that multiple test modules want to share.
"""
