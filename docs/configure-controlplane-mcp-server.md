# Openchoreo MCP Server Configuration

> **DEPRECATED:** This document describes the legacy MCP server (`pkg/mcp/legacytools/`). The toolset code has been moved to `pkg/mcp/legacytools/` and the config key renamed from `mcp` to `legacy_mcp`. A new MCP server implementation is being developed.

## Before You Begin

### What is the MCP Server?

The OpenChoreo MCP (Model Context Protocol) server allows AI assistants (such as Claude, Cursor, and VS Code Copilot) to interact with your OpenChoreo control plane programmatically. Through MCP, AI assistants can list projects, create components, trigger builds, check deployment status, and more — all using natural language.

### Legacy vs. New MCP Server

> **Important:** This document describes the **legacy** MCP server. OpenChoreo is transitioning to a new MCP server implementation with improved toolset organization and handler separation.

| | Legacy MCP Server | New MCP Server |
|---|---|---|
| **Code location** | `pkg/mcp/legacytools/` | `pkg/mcp/tools/` |
| **Config key** | `legacy_mcp` | `mcp` |
| **Status** | Deprecated (still functional) | Active development |
| **Documentation** | This document | [Adding New MCP Tools](./contributors/adding-new-mcp-tools.md) |

If you are setting up MCP for the first time, refer to the [MCP samples](../samples/mcp/) for getting started with the current implementation.

### Prerequisites

- A running OpenChoreo control plane with the `openchoreo-api` server deployed
- `kubectl` access to the control plane cluster
- Helm (if configuring via Helm values)

This guide explains the OpenChoreo MCP (Model Context Protocol) server concepts, implementation and configuration.

## Architecture Overview

The MCP server implementation consists of three main components:

1. **Toolsets & Registration** (`pkg/mcp/tools.go`) - Defines tool handler interfaces organized by toolsets and registers them with the MCP server
2. **Server Setup** (`pkg/mcp/server.go`) - Creates HTTP and STDIO server instances
3. **Handler Implementation** (`internal/openchoreo-api/mcphandlers/`) - Implements the actual business logic

## Toolset Concept

Tools are organized into **Toolsets** - logical groupings of related functionality. Each toolset has its own handler interface.

**Available Toolsets:**
- `ToolsetNamespace` (`namespace`) - Namespace operations (get namespace details)
- `ToolsetProject` (`project`) - Project operations (list, get, create projects)
- `ToolsetComponent` (`component`) - Component operations (list, get, create components, bindings, workloads, releases, release bindings, deployment, promotion)
- `ToolsetBuild` (`build`) - Build operations (trigger builds, list builds, build templates, workflow planes)
- `ToolsetDeployment` (`deployment`) - Deployment operations (deployment pipelines, observer URLs)
- `ToolsetInfrastructure` (`infrastructure`) - Infrastructure operations (environments, data planes, component types, workflows, traits)
- `ToolsetSchema` (`schema`) - Schema operations (describe a given kind)
- `ToolsetResource` (`resource`) - Resource operations (kubectl-like apply/delete for OpenChoreo resources)

## Configuring Enabled Toolsets

Toolsets can be configured via the `MCP_TOOLSETS` environment variable. This allows you to enable/disable toolsets without code changes.

### Configuration

Set the `MCP_TOOLSETS` environment variable to a comma-separated list of toolsets:

```bash
# Enable only namespace and project toolsets
export MCP_TOOLSETS="namespace,project"

# Enable all toolsets (default)
export MCP_TOOLSETS="namespace,project,component,build,deployment,infrastructure,schema,resource"

# Enable specific toolsets for your use case
export MCP_TOOLSETS="namespace,project,component"
```

### Default Behavior

If `MCP_TOOLSETS` is not set, the system defaults to enabling all toolsets:
- `namespace`
- `project`
- `component`
- `build`
- `deployment`
- `infrastructure`
- `schema`
- `resource`

### Kubernetes/Helm Configuration

In production deployments, configure toolsets via Helm values:

```yaml
openchoreoApi:
  mcp:
    # Enable all toolsets (default)
    toolsets: "namespace,project,component,build,deployment,infrastructure,schema,resource"
    
    # Or enable specific toolsets based on your requirements
    # toolsets: "namespace,project,component"
```

## Troubleshooting

### MCP tools not responding

If AI assistants cannot reach the MCP server:

1. Verify the `openchoreo-api` pod is running:
   ```bash
   kubectl get pods -n openchoreo-control-plane -l app.kubernetes.io/component=openchoreo-api
   ```

2. Check the MCP server logs for errors:
   ```bash
   kubectl logs -n openchoreo-control-plane -l app.kubernetes.io/component=openchoreo-api --tail=50
   ```

3. Verify the `MCP_TOOLSETS` environment variable is set correctly:
   ```bash
   kubectl get deployment -n openchoreo-control-plane openchoreo-api -o jsonpath='{.spec.template.spec.containers[0].env}' | jq .
   ```

### Only some toolsets are available

If certain MCP tools are missing, check which toolsets are enabled:

```bash
kubectl get deployment -n openchoreo-control-plane openchoreo-api \
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="MCP_TOOLSETS")].value}'
```

Ensure the desired toolsets are included in the comma-separated list.

## Next Steps

- **Try MCP with AI assistants**: See the [MCP samples](../samples/mcp/) for guided walkthroughs
- **Add new MCP tools**: See [Adding New MCP Tools](./contributors/adding-new-mcp-tools.md) for the new server implementation
- **Understand resource types**: See the [Resource Kind Reference Guide](./resource-kind-reference-guide.md) for what MCP tools operate on
