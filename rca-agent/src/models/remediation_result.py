# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class ActionStatus(StrEnum):
    SUGGESTED = "suggested"
    REVISED = "revised"
    APPLIED = "applied"


class FieldChange(BaseModel):
    """A single field-level change within a ReleaseBinding, identified by JSON Pointer"""

    model_config = ConfigDict(populate_by_name=True)

    json_pointer: str = Field(
        ...,
        alias="jsonPointer",
        description=(
            "RFC 6901 JSON Pointer starting with /spec/ followed by override category. "
            "Use numeric indices for arrays, or '-' to append. "
            "Example: '/spec/workloadOverrides/container/env/0/value'"
        ),
    )
    value: Any = Field(..., description="Value to set at the JSON Pointer location")


class ResourceChange(BaseModel):
    """A set of field changes to apply to a specific ReleaseBinding"""

    release_binding: str = Field(
        ...,
        description="Name of the ReleaseBinding to modify (e.g. 'api-service-development')",
    )
    fields: list[FieldChange] = Field(
        ...,
        description="Field-level changes to apply to this ReleaseBinding",
    )


class RemediationAction(BaseModel):
    """A recommended action — either kept as suggested or revised with concrete OpenChoreo changes"""

    description: str = Field(..., description="Description of the remediation action")
    rationale: str | None = Field(
        default=None,
        description="Why this action is recommended",
    )
    status: ActionStatus = Field(
        ...,
        description="'suggested' if kept as-is, 'revised' if translated into concrete OpenChoreo guidance",
    )
    changes: list[ResourceChange] = Field(
        default_factory=list,
        description="Specific field changes to make. Empty when status is 'suggested'",
    )


class RemediationResult(BaseModel):
    """Structured output from the remediation agent"""

    recommended_actions: list[RemediationAction] = Field(
        ...,
        description="Recommended actions to resolve the identified root causes",
    )
