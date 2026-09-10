# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from dataclasses import dataclass
from datetime import datetime
from typing import Any

import httpx
from fastapi import HTTPException

from src.auth import get_oauth2_auth
from src.clients.openchoreo_api import get

logger = logging.getLogger(__name__)


@dataclass
class AlertScope:
    namespace: str
    project: str
    project_uid: str
    environment: str
    environment_uid: str
    component: str | None = None
    component_uid: str | None = None


def validate_time_range(start_time: str, end_time: str) -> tuple[str, str]:
    start_dt = datetime.fromisoformat(start_time.replace("Z", "+00:00"))
    end_dt = datetime.fromisoformat(end_time.replace("Z", "+00:00"))
    if start_dt > end_dt:
        raise ValueError("startTime must be before endTime")
    return start_dt.isoformat(), end_dt.isoformat()


async def resolve_component_scope(
    namespace: str,
    project: str,
    component: str,
    environment: str,
) -> AlertScope:
    auth = get_oauth2_auth()
    project_data = await get(f"/namespaces/{namespace}/projects/{project}", auth)
    component_data = await get(f"/namespaces/{namespace}/components/{component}", auth)
    environment_data = await get(f"/namespaces/{namespace}/environments/{environment}", auth)

    return AlertScope(
        namespace=namespace,
        project=project,
        project_uid=project_data["metadata"]["uid"],
        environment=environment,
        environment_uid=environment_data["metadata"]["uid"],
        component=component,
        component_uid=component_data["metadata"]["uid"],
    )


async def resolve_project_scope(
    namespace: str,
    project: str,
    environment: str,
) -> AlertScope:
    auth = get_oauth2_auth()
    project_data = await get(f"/namespaces/{namespace}/projects/{project}", auth)
    environment_data = await get(f"/namespaces/{namespace}/environments/{environment}", auth)

    return AlertScope(
        namespace=namespace,
        project=project,
        project_uid=project_data["metadata"]["uid"],
        environment=environment,
        environment_uid=environment_data["metadata"]["uid"],
    )


async def resolve_project_uid(namespace: str, project: str) -> str:
    auth = get_oauth2_auth()
    project_data = await get(f"/namespaces/{namespace}/projects/{project}", auth)
    return project_data["metadata"]["uid"]


async def verify_report_project(namespace: str, project: str, result: dict[str, Any]) -> None:
    try:
        claimed_uid = await resolve_project_uid(namespace, project)
    except httpx.HTTPError:
        raise HTTPException(status_code=404, detail="Report not found") from None
    if claimed_uid != result.get("projectUid"):
        raise HTTPException(status_code=404, detail="Report not found")
