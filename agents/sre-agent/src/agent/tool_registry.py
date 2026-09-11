# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

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
    QUERY_COMPONENT_EVENTS = Tool(
        "query_component_events", server=OBSERVABILITY, active_form="Fetching component events..."
    )
    QUERY_WORKFLOW_LOGS = Tool(
        "query_workflow_logs", server=OBSERVABILITY, active_form="Fetching workflow logs..."
    )
    QUERY_WORKFLOW_EVENTS = Tool(
        "query_workflow_events", server=OBSERVABILITY, active_form="Fetching workflow events..."
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
    GET_COMPONENT = Tool(
        "get_component", server=OPENCHOREO, active_form="Fetching component..."
    )
    PATCH_RELEASEBINDING = Tool(
        "patch_releasebinding", server=OPENCHOREO, active_form="Patching release binding..."
    )
    GET_RESOURCE = Tool("get_resource", server=OPENCHOREO, active_form="Fetching resource...")
    GET_COMPONENT_RELEASE = Tool(
        "get_component_release",
        server=OPENCHOREO,
        active_form="Fetching component release...",
    )
    GET_COMPONENT_RELEASE_SCHEMA = Tool(
        "get_component_release_schema",
        server=OPENCHOREO,
        active_form="Fetching release schema...",
    )
    CREATE_WORKLOAD = Tool("create_workload", server=OPENCHOREO, active_form="Creating workload...")
    LIST_WORKLOADS = Tool(
        "list_workloads", server=OPENCHOREO, active_form="Loading workloads..."
    )
    GET_WORKLOAD = Tool(
        "get_workload", server=OPENCHOREO, active_form="Fetching workload..."
    )
    LIST_RELEASE_BINDINGS = Tool(
        "list_release_bindings",
        server=OPENCHOREO,
        active_form="Loading release bindings...",
    )
    GET_RELEASE_BINDING = Tool(
        "get_release_binding",
        server=OPENCHOREO,
        active_form="Fetching release binding...",
    )
    LIST_RESOURCE_RELEASE_BINDINGS = Tool(
        "list_resource_release_bindings",
        server=OPENCHOREO,
        active_form="Loading resource release bindings...",
    )
    GET_RESOURCE_RELEASE_BINDING = Tool(
        "get_resource_release_binding",
        server=OPENCHOREO,
        active_form="Fetching resource release binding...",
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
