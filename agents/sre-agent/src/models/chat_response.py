# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Any

from src.models.base import BaseModel


class ChatResponse(BaseModel):
    """Structured output for chat agent."""

    message: str
    actions: list[Any] = []
