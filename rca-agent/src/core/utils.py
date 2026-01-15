# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import asyncio
from datetime import UTC, datetime

from pydantic import BaseModel as PydanticBaseModel

from src.core.config import settings


class BaseModel(PydanticBaseModel):
    def __str__(self):
        fields = [f"{field}={repr(getattr(self, field))}" for field in type(self).model_fields]
        return "\n".join(fields)


def get_current_utc() -> datetime:
    return datetime.now(UTC)


_semaphore: asyncio.Semaphore | None = None


def get_semaphore() -> asyncio.Semaphore:
    """Get the singleton semaphore for limiting concurrent analyses."""
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(settings.max_concurrent_analyses)
    return _semaphore
