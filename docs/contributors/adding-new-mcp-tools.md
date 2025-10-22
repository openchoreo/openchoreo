# Adding New MCP Tools

This guide explains how to add new tools to the OpenChoreo MCP server.

## Adding a New Tool

### Option A: Add to Existing Toolset

Follow these steps to add a tool to an existing toolset (e.g., CoreToolset):

#### 1. Update the Toolset Handler Interface

In `pkg/mcp/tools.go`, add your method to the appropriate handler interface:

```go
type CoreToolsetHandler interface {
    GetOrganization(name string) (string, error)
    YourNewMethod(param1 string, param2 int) (string, error)  // Add here
}
```

**Conventions:**
- Methods should return `(string, error)` - the string contains JSON-serialized response
- Keep method names descriptive and follow Go naming conventions

#### 2. Register the Tool

In the `Register` method of `pkg/mcp/tools.go`, add your tool registration:

```go
func (t *Toolsets) Register(s *mcp.Server) {
    if t.CoreToolset != nil {
        // ... existing tools ...
        
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
            result, err := t.CoreToolset.YourNewMethod(args.Param1, args.Param2)
            if err != nil {
                return nil, nil, err
            }
            
            contentBytes, err := json.Marshal(result)
            if err != nil {
                return nil, nil, err
            }
            
            return &mcp.CallToolResult{
                Content: []mcp.Content{
                    &mcp.TextContent{Text: string(contentBytes)},
                },
            }, map[string]string{"message": string(contentBytes)}, nil
        })
    }
}
```

**Key Points:**
- Tool name should be lowercase with underscores (snake_case)
- Always check if the handler is nil before adding tools
- The args struct must have JSON tags matching the schema property names
- Always marshal the result as JSON

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
    ToolsetCore    ToolsetType = "core"
    ToolsetYourNew ToolsetType = "yournew"  // Add your toolset
)
```

#### 2. Create Handler Interface

Define a new handler interface:

```go
type YourNewToolsetHandler interface {
    Method1(param string) (string, error)
    Method2(id int) (string, error)
}
```

#### 3. Add to Toolsets Struct

```go
type Toolsets struct {
    CoreToolset    CoreToolsetHandler
    YourNewToolset YourNewToolsetHandler  // Add your toolset
}
```

#### 4. Register Tools

Add your toolset's tools in the `Register` method:

```go
func (t *Toolsets) Register(s *mcp.Server) {
    // ... existing toolsets ...
    
    if t.YourNewToolset != nil {
        mcp.AddTool(s, &mcp.Tool{
            Name:        "method1",
            Description: "Description",
            InputSchema: createSchema(map[string]any{
                "param": stringProperty("Parameter description"),
            }, []string{"param"}),
        }, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
            Param string `json:"param"`
        }) (*mcp.CallToolResult, map[string]string, error) {
            // Implementation
        })
        
        // Add more tools for this toolset...
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

func (h *MCPHandler) Method1(param string) (string, error) {
    ctx := context.Background()
    
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
        case mcp.ToolsetCore:
            toolsets.CoreToolset = &mcphandlers.MCPHandler{Services: h.services}
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
export MCP_TOOLSETS="core,yournew"
```

## Schema Helper Functions

Available helper functions for defining input schemas:

### `stringProperty(description string)`
Creates a string property:
```go
"name": stringProperty("User's name")
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
    "age":  stringProperty("Optional age parameter"),
}, []string{"name"}) // Only "name" is required
```

**Note:** Add helper functions for other types as needed (e.g., `numberProperty`, `booleanProperty`, `arrayProperty`).

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
