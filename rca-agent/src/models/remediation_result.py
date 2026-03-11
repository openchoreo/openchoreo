# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class ActionStatus(StrEnum):
    SUGGESTED = "suggested"
    REVISED = "revised"
    APPLIED = "applied"


class EnvVarChange(BaseModel):
    """A single environment variable change, identified by key name"""

    key: str = Field(..., description="Environment variable name (e.g. 'POSTGRES_DSN')")
    value: str = Field(..., description="New value for the environment variable")


class FileChange(BaseModel):
    """A single file mount change, identified by key name"""

    model_config = ConfigDict(populate_by_name=True)

    key: str = Field(..., description="File key (e.g. 'config.yaml')")
    value: str | None = Field(default=None, description="New file content")
    mount_path: str | None = Field(
        default=None,
        alias="mountPath",
        description="New mount path (e.g. '/app/')",
    )


class FieldChange(BaseModel):
    """A single field-level change within a ReleaseBinding, identified by JSON Pointer"""

    model_config = ConfigDict(populate_by_name=True)

    json_pointer: str = Field(
        ...,
        alias="jsonPointer",
        description=(
            "RFC 6901 JSON Pointer for non-array fields. "
            "Example: '/spec/componentTypeEnvironmentConfigs/replicas', "
            "'/spec/traitEnvironmentConfigs/my-trait/enabled'"
        ),
    )
    value: Any = Field(..., description="Value to set at the JSON Pointer location")


class ResourceChange(BaseModel):
    """A set of changes to apply to a specific ReleaseBinding"""

    release_binding: str = Field(
        ...,
        description="Name of the ReleaseBinding to modify (e.g. 'api-service-development')",
    )
    env: list[EnvVarChange] = Field(
        default_factory=list,
        description="Environment variable changes, identified by key. Updates existing vars or appends new ones.",
    )
    files: list[FileChange] = Field(
        default_factory=list,
        description="File mount changes, identified by key. Updates existing files or appends new ones.",
    )
    fields: list[FieldChange] = Field(
        default_factory=list,
        description="Field-level changes for non-array paths (e.g. trait overrides, componentType overrides)",
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
