# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from typing import Literal

from pydantic import BaseModel, Field


class FieldChange(BaseModel):
    json_pointer: str = Field(..., description="RFC 6901 JSON Pointer starting with /spec/")
    value: str | int | float | bool = Field(..., description="Scalar value to set at this path")


class ResourceChange(BaseModel):
    release_binding: str = Field(..., description="Name of the ReleaseBinding to patch")
    fields: list[FieldChange] = Field(default_factory=list)


class RemediationAction(BaseModel):
    description: str = Field(..., description="Short description of the remediation action")
    rationale: str = Field(..., description="Explanation of why this change is recommended")
    status: Literal["revised", "applied", "dismissed"] = "revised"
    change: ResourceChange | None = None
