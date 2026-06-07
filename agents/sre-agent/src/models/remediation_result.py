# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field


class ActionStatus(StrEnum):
    SUGGESTED = "suggested"
    REVISED = "revised"
    APPLIED = "applied"
    DISMISSED = "dismissed"


class EnvVarChange(BaseModel):
    """A single environment variable change, identified by key name"""

    key: str = Field(..., description="Environment variable name (e.g. 'POSTGRES_DSN')")
    value: str = Field(..., description="New value for the environment variable")


class FileChange(BaseModel):
    """A content-only change to an existing file mount, identified by key + mountPath"""

    model_config = ConfigDict(populate_by_name=True)

    key: str = Field(..., description="File key of the existing mount (e.g. 'config.yaml')")
    mount_path: str = Field(
        ...,
        alias="mountPath",
        description="Mount path that identifies the file mount (e.g. '/app/frontend/'). Must match an existing mount.",
    )
    value: str = Field(..., description="New file content")


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
    value: str | int | float | bool = Field(
        ...,
        description=(
            "Scalar value to set at the JSON Pointer location. "
            "For nested objects, flatten into separate FieldChange entries with leaf-level pointers."
        ),
    )


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
        description="File content changes for existing mounts, identified by key + mountPath. Only the content (value) is changed.",
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
    change: ResourceChange | None = Field(
        default=None,
        description="The specific changes to apply to a single ReleaseBinding. None when status is 'suggested'",
    )


class RemediationResult(BaseModel):
    """Structured output from the remediation agent"""

    recommended_actions: list[RemediationAction] = Field(
        ...,
        description="Recommended actions to resolve the identified root causes",
    )
