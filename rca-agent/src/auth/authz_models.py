# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic import BaseModel, Field


class SubjectContext(BaseModel):
    type: str
    entitlement_claim: str = Field(alias="entitlementClaim")
    entitlement_values: list[str] = Field(default_factory=list, alias="entitlementValues")

    model_config = {"populate_by_name": True}


class ResourceHierarchy(BaseModel):
    namespace: str | None = None
    project: str | None = None
    component: str | None = None


class Resource(BaseModel):
    type: str
    id: str = ""
    hierarchy: ResourceHierarchy


class EvaluateRequest(BaseModel):
    subject_context: SubjectContext = Field(alias="subjectContext")
    resource: Resource
    action: str
    context: dict = Field(default_factory=dict)

    model_config = {"populate_by_name": True}


class DecisionContext(BaseModel):
    reason: str | None = None


class Decision(BaseModel):
    decision: bool
    context: DecisionContext | None = None
