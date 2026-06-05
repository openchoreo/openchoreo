# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from datetime import UTC, datetime

from pydantic import BaseModel as PydanticBaseModel


class BaseModel(PydanticBaseModel):
    def __str__(self):
        fields = [f"{field}={repr(getattr(self, field))}" for field in type(self).model_fields]
        return "\n".join(fields)


def get_current_utc() -> datetime:
    return datetime.now(UTC)
