# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0


class OpenchoreoTools:
    LIST_ENVIRONMENTS = "list_environments"
    LIST_NAMESPACES = "list_namespaces"
    LIST_PROJECTS = "list_projects"
    LIST_COMPONENTS = "list_components"
    LIST_COMPONENT_RELEASES = "list_component_releases"


class ObservabilityTools:
    GET_TRACES = "get_traces"
    GET_NAMESPACE_LOGS = "get_namespace_logs"
    GET_PROJECT_LOGS = "get_project_logs"
    GET_COMPONENT_LOGS = "get_component_logs"
    GET_COMPONENT_RESOURCE_METRICS = "get_component_resource_metrics"


obs_tools = ObservabilityTools()
oc_tools = OpenchoreoTools()


class OpenchoreoLabels:
    COMPONENT_UID = "openchoreo.dev/component-uid"
    COMPONENT_UIDS = "openchoreo.dev/component-uids"
    ENVIRONMENT_UID = "openchoreo.dev/environment-uid"
    PROJECT_UID = "openchoreo.dev/project-uid"


oc_labels = OpenchoreoLabels()

# Default configuration values
DEFAULT_MCP_OBSERVABILITY_URL = "http://observer:8080/mcp"
DEFAULT_MCP_OPENCHOREO_URL = (
    "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080/mcp"
)
DEFAULT_RCA_AGENT_LLM = "gpt-5"
