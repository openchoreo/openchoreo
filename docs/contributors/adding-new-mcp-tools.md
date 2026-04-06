# Adding New MCP Tools

This guide explains how to add new tools to the OpenChoreo MCP server implementation in `pkg/mcp/tools/`.

## How the MCP server is structured

- **Tool definitions + registration**: `pkg/mcp/tools/`
  - Toolset handler interfaces and `Toolsets` struct: `pkg/mcp/tools/types.go`
  - Tool registration lists and `Toolsets.Register(...)`: `pkg/mcp/tools/register.go`
  - Schema/result helpers: `pkg/mcp/tools/helpers.go`
  - Tool implementations are grouped by domain in files like `namespace.go`, `project.go`, `component.go`, `deployment.go`, `build.go`, `pe.go`
- **Tool implementations (handlers)**: `internal/openchoreo-api/mcphandlers/`
  - A single handler type implements one or more toolset handler interfaces (e.g. `NamespaceToolsetHandler`, `ComponentToolsetHandler`, `PEToolsetHandler`).
- **Wiring enabled toolsets**: `cmd/openchoreo-api/main.go` (`buildMCPToolsets`)
- **Toolset configuration/validation**: `internal/openchoreo-api/config/mcp.go`

## Adding a new tool to an existing toolset

### 1. Add a method to the toolset handler interface

In `pkg/mcp/tools/types.go`, add a method to the relevant handler interface.

- **Signature**: handler methods return `(any, error)`
- **Pagination**: list tools should accept a `ListOpts` argument

Example (add to `ComponentToolsetHandler`):

```go
type ComponentToolsetHandler interface {
    // ... existing methods ...
    YourNewMethod(ctx context.Context, namespaceName string, someID string) (any, error)
}
```

### 2. Implement the handler method in `mcphandlers`

Add the method to the MCP handler implementation in `internal/openchoreo-api/mcphandlers/` (pick the file that matches the domain, or create a new one).

- **Return value**: return a Go value/struct/map/slice; the tool layer will JSON-encode it.
- **List results**: when returning arrays, wrap them as an object (record) using `wrapList(...)` so structured MCP responses are valid, and include `next_cursor` when paginating.

### 3. Register the tool in `pkg/mcp/tools/`

Add a `Register...` function to the correct tool file (e.g. `component.go`, `pe.go`, etc). Follow the existing pattern:

- Define `Name`, `Description`, and `InputSchema`
- Use `createSchema(...)`, `stringProperty(...)`, `intProperty(...)`, and `addPaginationProperties(...)`
- In the handler callback, call the handler method and return via `handleToolResult(result, err)`

Example:

```go
func (t *Toolsets) RegisterYourNewTool(s *mcp.Server) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "your_new_tool",
        Description: "What this tool does.",
        InputSchema: createSchema(map[string]any{
            "namespace_name": defaultStringProperty(),
            "some_id":        stringProperty("Identifier of the thing to operate on"),
        }, []string{"namespace_name", "some_id"}),
    }, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
        NamespaceName string `json:"namespace_name"`
        SomeID        string `json:"some_id"`
    }) (*mcp.CallToolResult, any, error) {
        result, err := t.ComponentToolset.YourNewMethod(ctx, args.NamespaceName, args.SomeID)
        return handleToolResult(result, err)
    })
}
```

### 4. Add the registration function to the toolset registration list

In `pkg/mcp/tools/register.go`, add your `Register...` function to the appropriate registration list so it becomes part of `Toolsets.Register(...)`.

Example (component toolset):

```go
func (t *Toolsets) componentToolRegistrations() []RegisterFunc {
    return []RegisterFunc{
        // ... existing registrations ...
        t.RegisterYourNewTool,
    }
}
```

## Creating a new toolset (new domain)

If your tool doesn’t fit an existing toolset, create a new one.

### 1. Add a new `ToolsetType` constant

In `pkg/mcp/tools/types.go`:

```go
const (
    // ... existing ...
    ToolsetYourNew ToolsetType = "yournew"
)
```

### 2. Add a handler interface and wire it into `Toolsets`

In `pkg/mcp/tools/types.go`:

```go
type YourNewToolsetHandler interface {
    Method1(ctx context.Context, namespaceName string) (any, error)
}

type Toolsets struct {
    // ... existing ...
    YourNewToolset YourNewToolsetHandler
}
```

### 3. Add a registration list and invoke it from `Toolsets.Register`

In `pkg/mcp/tools/register.go`, add:

```go
func (t *Toolsets) yourNewToolRegistrations() []RegisterFunc {
    return []RegisterFunc{
        t.RegisterMethod1,
    }
}
```

And extend `Register(...)`:

```go
if t.YourNewToolset != nil {
    for _, registerFunc := range t.yourNewToolRegistrations() {
        registerFunc(s)
    }
}
```

### 4. Wire the toolset into the API server

- **Enable in toolset switch**: update `cmd/openchoreo-api/main.go` (`buildMCPToolsets`) to set `toolsets.YourNewToolset = handler` when enabled.
- **Allow in config validation**: update `internal/openchoreo-api/config/mcp.go` (`validToolsets`) to include the new toolset string.

## Conventions and gotchas

- **Tool names**: use `snake_case` (e.g., `list_environments`, `create_component_release`).
- **Inputs**:
  - JSON tags in the args struct must match the schema property names exactly.
  - Use `defaultStringProperty()` for common identifiers (like `namespace_name`) unless you need a specific description.
  - For list tools, include pagination fields via `addPaginationProperties(...)` and accept `limit` and `cursor` in args.
- **Outputs**:
  - Tool handlers return `(any, error)`.
  - The registration layer uses `handleToolResult(...)` to JSON-marshal and return `mcp.CallToolResult`.
  - When returning arrays from list operations, ensure the handler wraps them as an object (record) and includes `next_cursor` when present (see `internal/openchoreo-api/mcphandlers/helpers.go`).
