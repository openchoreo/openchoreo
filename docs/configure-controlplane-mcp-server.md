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
- `ToolsetCore` (`core`) - Core operations (organizations, projects, components, etc.)

## Configuring Enabled Toolsets

Toolsets can be configured via the `MCP_TOOLSETS` environment variable. This allows you to enable/disable toolsets without code changes.

### Configuration

Set the `MCP_TOOLSETS` environment variable to a comma-separated list of toolsets:

```bash
# Enable only core toolset (default)
export MCP_TOOLSETS="core"
```

### Default Behavior

If `MCP_TOOLSETS` is not set, the system defaults to enabling the `core` toolset.

### Kubernetes/Helm Configuration

In production deployments, configure toolsets via Helm values:

```yaml
openchoreoApi:
  mcp:
    toolsets: "core"
```
