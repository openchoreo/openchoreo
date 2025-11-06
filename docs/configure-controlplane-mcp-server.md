# Openchoreo MCP Server Configuration

This guide explains the OpenChoreo MCP (Model Context Protocol) server concepts, implementation and configuration.

## Architecture Overview

The MCP server implementation consists of three main components:

1. **Toolsets & Registration** (`pkg/mcp/tools.go`) - Defines tool handler interfaces organized by toolsets and registers them with the MCP server
2. **Server Setup** (`pkg/mcp/server.go`) - Creates HTTP and STDIO server instances
3. **Handler Implementation** (`internal/openchoreo-api/mcphandlers/`) - Implements the actual business logic

## Toolset Concept

Tools are organized into **Toolsets** - logical groupings of related functionality. Each toolset has its own handler interface.

**Available Toolsets:**
- `ToolsetOrganization` (`organization`) - Organization operations (get organization details)
- `ToolsetProject` (`project`) - Project operations (list, get, create projects)
- `ToolsetComponent` (`component`) - Component operations (list, get, create components, bindings, workloads)
- `ToolsetBuild` (`build`) - Build operations (trigger builds, list builds, build templates, build planes)
- `ToolsetDeployment` (`deployment`) - Deployment operations (deployment pipelines, observer URLs)
- `ToolsetInfrastructure` (`infrastructure`) - Infrastructure operations (environments, data planes)

## Configuring Enabled Toolsets

Toolsets can be configured via the `MCP_TOOLSETS` environment variable. This allows you to enable/disable toolsets without code changes.

### Configuration

Set the `MCP_TOOLSETS` environment variable to a comma-separated list of toolsets:

```bash
# Enable only organization and project toolsets
export MCP_TOOLSETS="organization,project"

# Enable all toolsets (default)
export MCP_TOOLSETS="organization,project,component,build,deployment,infrastructure"

# Enable specific toolsets for your use case
export MCP_TOOLSETS="organization,project,component"
```

### Default Behavior

If `MCP_TOOLSETS` is not set, the system defaults to enabling all toolsets:
- `organization`
- `project`
- `component`
- `build`
- `deployment`
- `infrastructure`

### Kubernetes/Helm Configuration

In production deployments, configure toolsets via Helm values:

```yaml
openchoreoApi:
  mcp:
    # Enable all toolsets (default)
    toolsets: "organization,project,component,build,deployment,infrastructure"
    
    # Or enable specific toolsets based on your requirements
    # toolsets: "organization,project,component"
```
