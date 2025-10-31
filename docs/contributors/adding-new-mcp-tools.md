# Adding New MCP Tools

This guide explains how to add new tools to the OpenChoreo MCP server.

## Adding a New Tool

### Option A: Add to Existing Toolset

Follow these steps to add a tool to an existing toolset (e.g., ComponentToolset):

#### 1. Update the Toolset Handler Interface

In `pkg/mcp/tools.go`, add your method to the appropriate handler interface:

```go
type ComponentToolsetHandler interface {
    CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (string, error)
    YourNewMethod(ctx context.Context, param1 string, param2 int) (string, error)  // Add here
}
```

**Conventions:**
- Methods should return `(string, error)` - the string contains JSON-serialized response
- Keep method names descriptive and follow Go naming conventions
- First parameter should be `ctx context.Context`

#### 2. Register the Tool

In the `Register` method of `pkg/mcp/tools.go`, create a new registration function:

```go
func (t *Toolsets) RegisterYourNewTool(s *mcp.Server) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "your_tool_name",
        Description: "Clear description of what the tool does",
        InputSchema: createSchema(map[string]any{
            "param1": stringProperty("Description of param1"),
            "param2": numberProperty("Description of param2"),
        }, []string{"param1"}), // List required fields
    }, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
        Param1 string `json:"param1"`
        Param2 int    `json:"param2"`
    }) (*mcp.CallToolResult, map[string]string, error) {
        result, err := t.ComponentToolset.YourNewMethod(ctx, args.Param1, args.Param2)
        return handleToolResult(result, err)
    })
}
```

Then add it to the appropriate toolset registration list (e.g., `componentToolRegistrations`):

```go
func (t *Toolsets) componentToolRegistrations() []RegisterFunc {
    return []RegisterFunc{
        t.RegisterListComponents,
        t.RegisterGetComponent,
        t.RegisterYourNewTool,  // Add here
    }
}
```

**Key Points:**
- Tool name should be lowercase with underscores (snake_case)
- Always check if the handler is nil before adding tools
- The args struct must have JSON tags matching the schema property names
- Always marshal the result as JSON
- Use `handleToolResult` helper function for consistent error handling

#### 3. Implement the Handler

In `internal/openchoreo-api/mcphandlers/`, implement your handler method in the appropriate file:

```go
func (h *MCPHandler) YourNewMethod(param1 string, param2 int) (string, error) {
    ctx := context.Background()
    
    // Call your service layer
    res, err := h.Services.YourService.DoSomething(ctx, param1, param2)
    if err != nil {
        return "", err
    }
    
    // Marshal to JSON string
    data, err := json.Marshal(res)
    if err != nil {
        return "", err
    }
    
    return string(data), nil
}
```

### Option B: Create a New Toolset

If you're adding a new category of tools, create a new toolset:

#### 1. Define the Toolset Type

In `pkg/mcp/tools.go`, add a new constant:

```go
const (
    ToolsetOrganization   ToolsetType = "organization"
    ToolsetProject        ToolsetType = "project"
    ToolsetComponent      ToolsetType = "component"
    ToolsetBuild          ToolsetType = "build"
    ToolsetDeployment     ToolsetType = "deployment"
    ToolsetInfrastructure ToolsetType = "infrastructure"
    ToolsetYourNew        ToolsetType = "yournew"  // Add your toolset
)
```

#### 2. Create Handler Interface

Define a new handler interface:

```go
type YourNewToolsetHandler interface {
    Method1(ctx context.Context, param string) (string, error)
    Method2(ctx context.Context, id int) (string, error)
}
```

#### 3. Add to Toolsets Struct

```go
type Toolsets struct {
    OrganizationToolset   OrganizationToolsetHandler
    ProjectToolset        ProjectToolsetHandler
    ComponentToolset      ComponentToolsetHandler
    BuildToolset          BuildToolsetHandler
    DeploymentToolset     DeploymentToolsetHandler
    InfrastructureToolset InfrastructureToolsetHandler
    YourNewToolset        YourNewToolsetHandler  // Add your toolset
}
```

#### 4. Register Tools

Create registration functions and add them to a new toolset registration list:

```go
func (t *Toolsets) RegisterMethod1(s *mcp.Server) {
    mcp.AddTool(s, &mcp.Tool{
        Name:        "method1",
        Description: "Description",
        InputSchema: createSchema(map[string]any{
            "param": stringProperty("Parameter description"),
        }, []string{"param"}),
    }, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
        Param string `json:"param"`
    }) (*mcp.CallToolResult, map[string]string, error) {
        result, err := t.YourNewToolset.Method1(ctx, args.Param)
        return handleToolResult(result, err)
    })
}

// Add more registration functions...

// Create a registration list function
func (t *Toolsets) yourNewToolRegistrations() []RegisterFunc {
    return []RegisterFunc{
        t.RegisterMethod1,
        // Add more tools...
    }
}
```

Then in the `Register` method, add your toolset registration:

```go
func (t *Toolsets) Register(s *mcp.Server) {
    // ... existing toolsets ...
    
    if t.YourNewToolset != nil {
        for _, registerFunc := range t.yourNewToolRegistrations() {
            registerFunc(s)
        }
    }
}
```

#### 5. Create Handler File

Create a new file in `internal/openchoreo-api/mcphandlers/` (e.g., `yournew.go`):

```go
package mcphandlers

import (
    "context"
    "encoding/json"
)

func (h *MCPHandler) Method1(ctx context.Context, param string) (string, error) {
    // Implementation
    res, err := h.Services.YourService.Method1(ctx, param)
    if err != nil {
        return "", err
    }
    
    data, err := json.Marshal(res)
    if err != nil {
        return "", err
    }
    
    return string(data), nil
}
```

#### 6. Update Toolset Initialization

Add your new toolset to the initialization logic in `internal/openchoreo-api/handlers/handlers.go`:

```go
func getMCPServerToolsets(h *Handler) *mcp.Toolsets {
    // ... existing code ...
    
    for toolsetType := range toolsetsMap {
        switch toolsetType {
        case mcp.ToolsetOrganization:
            toolsets.OrganizationToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetProject:
            toolsets.ProjectToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetComponent:
            toolsets.ComponentToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetBuild:
            toolsets.BuildToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetDeployment:
            toolsets.DeploymentToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetInfrastructure:
            toolsets.InfrastructureToolset = &mcphandlers.MCPHandler{Services: h.services}
        case mcp.ToolsetYourNew:  // Add your new toolset
            toolsets.YourNewToolset = &mcphandlers.MCPHandler{Services: h.services}
        default:
            h.logger.Warn("Unknown toolset type", slog.String("toolset", string(toolsetType)))
        }
    }
    return toolsets
}
```

Now users can enable your toolset by setting:
```bash
export MCP_TOOLSETS="organization,project,yournew"
```

## Schema Helper Functions

Available helper functions for defining input schemas:

### `stringProperty(description string)`
Creates a string property:
```go
"name": stringProperty("User's name")
```

### `numberProperty(description string)`
Creates a number property:
```go
"age": numberProperty("User's age")
```

### `booleanProperty(description string)`
Creates a boolean property:
```go
"enabled": booleanProperty("Whether the feature is enabled")
```

### `arrayProperty(description string, itemType string)`
Creates an array property:
```go
"tags": arrayProperty("List of tags", "string")
```

### `enumProperty(description string, values []string)`
Creates an enum property with fixed allowed values:
```go
"format": enumProperty("Output format", []string{"json", "table", "yaml"})
```

### `createSchema(properties map[string]any, required []string)`
Creates the complete input schema:
```go
InputSchema: createSchema(map[string]any{
    "name": stringProperty("Required name parameter"),
    "age":  numberProperty("Optional age parameter"),
}, []string{"name"}) // Only "name" is required
```

**Note:** Some helper functions may be marked as unused by linters if they're not currently used in the codebase. This is expected for functions provided for future extensibility.

## Data Type Conventions

### Return Types
All handler methods **must** return `(string, error)`:
- The string should contain JSON-serialized data
- Use `json.Marshal()` to serialize responses
- Return empty string and error on failure

### Input Parameters
- Keep parameters simple (string, int, bool, etc.)
- Complex inputs should use structs with JSON tags
- JSON tags must match schema property names exactly

### Schema Types
- `string` - text values
- `number` / `integer` - numeric values
- `boolean` - true/false values
- `object` - nested structures
- `array` - lists of values

### Enums
Use `enumProperty()` for parameters with a fixed set of allowed values.
