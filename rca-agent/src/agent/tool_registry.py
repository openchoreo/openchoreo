# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json

import httpx
from langchain_core.tools import StructuredTool
from pydantic import BaseModel, Field

from src.clients.openchoreo_api import get


class Tool(str):
    active_form: str | None
    server: str

    def __new__(cls, name: str, *, server: str, active_form: str | None = None):
        instance = super().__new__(cls, name)
        instance.active_form = active_form
        instance.server = server
        return instance


OBSERVABILITY = "observability"
OPENCHOREO = "openchoreo"


class TOOLS:
    QUERY_COMPONENT_LOGS = Tool(
        "query_component_logs", server=OBSERVABILITY, active_form="Fetching component logs..."
    )
    QUERY_WORKFLOW_LOGS = Tool(
        "query_workflow_logs", server=OBSERVABILITY, active_form="Fetching workflow logs..."
    )
    QUERY_RESOURCE_METRICS = Tool(
        "query_resource_metrics",
        server=OBSERVABILITY,
        active_form="Gathering resource metrics...",
    )
    QUERY_HTTP_METRICS = Tool(
        "query_http_metrics",
        server=OBSERVABILITY,
        active_form="Gathering HTTP metrics...",
    )
    QUERY_TRACES = Tool("query_traces", server=OBSERVABILITY, active_form="Retrieving traces...")
    QUERY_TRACE_SPANS = Tool(
        "query_trace_spans", server=OBSERVABILITY, active_form="Retrieving trace spans..."
    )
    GET_SPAN_DETAILS = Tool(
        "get_span_details", server=OBSERVABILITY, active_form="Fetching span details..."
    )
    LIST_ENVIRONMENTS = Tool(
        "list_environments", server=OPENCHOREO, active_form="Loading environments..."
    )
    LIST_NAMESPACES = Tool(
        "list_namespaces", server=OPENCHOREO, active_form="Loading namespaces..."
    )
    LIST_PROJECTS = Tool("list_projects", server=OPENCHOREO, active_form="Loading projects...")
    LIST_COMPONENTS = Tool(
        "list_components", server=OPENCHOREO, active_form="Loading components..."
    )
    PATCH_RELEASEBINDING = Tool(
        "patch_releasebinding", server=OPENCHOREO, active_form="Patching release binding..."
    )
    GET_RESOURCE = Tool("get_resource", server=OPENCHOREO, active_form="Fetching resource...")
    GET_COMPONENT_RELEASE_SCHEMA = Tool(
        "get_component_release_schema",
        server=OPENCHOREO,
        active_form="Fetching release schema...",
    )
    CREATE_WORKLOAD = Tool("create_workload", server=OPENCHOREO, active_form="Creating workload...")
    GET_COMPONENT_WORKLOADS = Tool(
        "get_component_workloads",
        server=OPENCHOREO,
        active_form="Fetching component workloads...",
    )
    LIST_RELEASE_BINDINGS = Tool(
        "list_release_bindings",
        server=OPENCHOREO,
        active_form="Loading release bindings...",
    )
    LIST_COMPONENT_TRAITS = Tool(
        "list_component_traits",
        server=OPENCHOREO,
        active_form="Loading component traits...",
    )
    GET_TRAIT_SCHEMA = Tool(
        "get_trait_schema",
        server=OPENCHOREO,
        active_form="Fetching trait schema...",
    )


_ALL = [v for v in vars(TOOLS).values() if isinstance(v, Tool)]

# Tool names grouped by server
OBSERVABILITY_TOOLS = {t for t in _ALL if t.server == OBSERVABILITY}
OPENCHOREO_TOOLS = {t for t in _ALL if t.server == OPENCHOREO}

# Active forms for streaming UI
TOOL_ACTIVE_FORMS: dict[str, str] = {
    v: v.active_form
    for v in vars(TOOLS).values()
    if isinstance(v, Tool) and v.active_form is not None
}


class _ListReleaseBindingsInput(BaseModel):
    namespace: str = Field(..., description="Namespace name")
    component: str = Field(..., description="Component name to filter by")


class _GetComponentWorkloadsInput(BaseModel):
    namespace: str = Field(..., description="Namespace name")
    project: str = Field(..., description="Project name")
    component: str = Field(..., description="Component name")


class _GetComponentReleaseSchemaInput(BaseModel):
    namespace: str = Field(..., description="Namespace name")
    component: str = Field(..., description="Component name")


class _ListComponentTraitsInput(BaseModel):
    namespace: str = Field(..., description="Namespace name")
    component: str = Field(..., description="Component name")


class _ListComponentsInput(BaseModel):
    namespace: str = Field(..., description="Namespace name")
    project: str = Field(..., description="Project name")


def create_list_release_bindings_tool(auth: httpx.Auth) -> StructuredTool:
    async def _run(namespace: str, component: str) -> str:
        result = await get(
            f"/namespaces/{namespace}/releasebindings",
            auth,
            params={"component": component},
        )
        return json.dumps(result)

    return StructuredTool.from_function(
        coroutine=_run,
        name="list_release_bindings",
        description=(
            "List release bindings for a component. Returns the full binding spec "
            "including current workloadOverrides, traitOverrides, and "
            "componentTypeEnvOverrides."
        ),
        args_schema=_ListReleaseBindingsInput,
    )


def create_get_component_workloads_tool(auth: httpx.Auth) -> StructuredTool:
    async def _run(namespace: str, project: str, component: str) -> str:
        result = await get(
            f"/namespaces/{namespace}/workloads",
            auth,
            params={"project": project, "component": component},
        )
        return json.dumps(result)

    return StructuredTool.from_function(
        coroutine=_run,
        name="get_component_workloads",
        description="Get workloads for a component including container specs, env vars, and endpoints.",
        args_schema=_GetComponentWorkloadsInput,
    )


def create_get_component_release_schema_tool(auth: httpx.Auth) -> StructuredTool:
    async def _run(namespace: str, component: str) -> str:
        result = await get(
            f"/namespaces/{namespace}/components/{component}/schema",
            auth,
        )
        return json.dumps(result)

    return StructuredTool.from_function(
        coroutine=_run,
        name="get_component_release_schema",
        description=(
            "Get the JSON Schema for a component's trait and componentType overrides. "
            "Source of truth for valid override fields."
        ),
        args_schema=_GetComponentReleaseSchemaInput,
    )


def create_list_component_traits_tool(auth: httpx.Auth) -> StructuredTool:
    async def _run(namespace: str, component: str) -> str:
        result = await get(
            f"/namespaces/{namespace}/components/{component}",
            auth,
        )
        traits = result.get("spec", {}).get("traits", [])
        return json.dumps(traits)

    return StructuredTool.from_function(
        coroutine=_run,
        name="list_component_traits",
        description="List traits attached to a component with their base parameter values.",
        args_schema=_ListComponentTraitsInput,
    )


def create_list_components_tool(auth: httpx.Auth) -> StructuredTool:
    async def _run(namespace: str, project: str) -> str:
        result = await get(
            f"/namespaces/{namespace}/components",
            auth,
            params={"project": project},
        )
        return json.dumps(result)

    return StructuredTool.from_function(
        coroutine=_run,
        name="list_components",
        description="List components in a project.",
        args_schema=_ListComponentsInput,
    )


ALL_TOOL_FACTORIES = [
    create_list_release_bindings_tool,
    create_get_component_workloads_tool,
    create_get_component_release_schema_tool,
    create_list_component_traits_tool,
    create_list_components_tool,
]
