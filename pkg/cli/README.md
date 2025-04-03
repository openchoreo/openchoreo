# Choreo CLI Library

A framework for building consistent and user-friendly command-line interfaces in Go. This guide explains how to use the library to create a new CLI and implement commands.

## Table of Contents
- [Choreo CLI Library](#choreo-cli-library)
    - [Table of Contents](#table-of-contents)
    - [Using the Library](#using-the-library)
        - [1. Create a New CLI Project](#1-create-a-new-cli-project)
        - [2. Create Main Entry Point](#2-create-main-entry-point)
        - [3. Implement Command Interface](#3-implement-command-interface)
    - [Architecture](#architecture)
        - [Core Concepts](#core-concepts)
        - [Implementing a New Command](#implementing-a-new-command)
        - [Best Practices](#best-practices)
        - [Testing Commands](#testing-commands)
        - [Examples](#examples)

## Using the Library

### 1. Create a New CLI Project

```bash
# Initialize a new Go module
go mod init mycli

# Add the Choreo CLI library as a dependency
go get github.com/openchoreo/openchoreo/pkg/cli
```

### 2. Create Main Entry Point
Create ```main.go```:

```go
package main

import (
    "fmt"
    "os"

    "github.com/openchoreo/openchoreo/pkg/cli/common/config"
    "github.com/openchoreo/openchoreo/pkg/cli/core/root"
)

func main() {
    // Initialize config
    cfg := config.DefaultConfig()
    cfg.Name = "mycli"
    cfg.ShortDescription = "My CLI tool"
    
    // Create implementation
    impl := &CommandImplementation{}
    
    // Build root command
    rootCmd := root.BuildRootCmd(cfg, impl)

    // Execute
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### 3. Implement Command Interface
Create ```internal/mycli/impl.go```:

```go
package mycli

import "github.com/openchoreo/openchoreo/pkg/cli/types/api"

type CommandImplementation struct{}

// Implement required interfaces
var _ api.CommandImplementationInterface = &CommandImplementation{}

// Implement interface methods
func (c *CommandImplementation) MyCommand(params api.MyCommandParams) error {
    // Command implementation
    return nil
}
```

## Architecture

```
pkg/cli/
├── cmd/                    # Command implementations
│   ├── apply/             # Apply command 
│   ├── create/            # Create command
│   ├── list/              # List command
│   ├── login/             # Login command
│   └── logout/            # Logout command
├── common/                 # Shared utilities
│   ├── config/            # CLI configuration
│   ├── constants/         # Command definitions
│   ├── messages/          # User-facing messages
│   └── flags/             # Common flag definitions
├── core/                  # Core CLI functionality
│   └── root/             # Root command builder
└── types/                # Interfaces and types
    └── api/             # Command interfaces
```

### Core Concepts

1. **Command Implementation Interface**: All commands implement 

CommandImplementationInterface



2. **User Messages**: All user-facing text is centralized in messages.go

3. **Command Structure**: Commands follow a consistent pattern with:
   - Command definition
   - Flag handling
   - Input validation
   - Error handling

### Implementing a New Command

1. Create a new package under ```cmd:```

```go
package mycommand

import (
    "github.com/spf13/cobra"
    "github.com/openchoreo/openchoreo/pkg/cli/common/constants"
    "github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewMyCommand(impl api.CommandImplementationInterface) *cobra.Command {
    cmd := &cobra.Command{
        Use:   constants.MyCommand.Use,
        Short: constants.MyCommand.Short,
        RunE: func(cmd *cobra.Command, args []string) error {
            return impl.MyCommand(api.MyCommandParams{})
        },
    }
    return cmd
}
```

2. Add command definition in ```definitions.go```:

```go
var MyCommand = Command{
    Use:   "mycommand",
    Short: "My command description",
    Long:  "Detailed description of my command",
}
```

3. Add messages in ```messages.go```:

```go
const (
    ErrMyCommand = "my command failed: %v"
    SuccessMyCommand = "✓ My command completed successfully"
)
```

4. Register command in ```root.go```:

```go
rootCmd.AddCommand(
    mycommand.NewMyCommand(impl),
)
```

### Best Practices

1. **Error Handling**
- ToDo: Introduce errors.go and split the messages.go
- Use typed errors from ```messages.go```
- Include helpful error hints
- Clean up resources on error

2. **User Experience**
- Provide interactive and flag-based modes
- Add command examples and help text
- Use consistent output formatting

3. **Code Organization**
- Keep command logic in separate packages
- Reuse common flags and utilities
- Follow existing patterns

### Testing Commands

//ToDo

### Examples

See existing commands for reference implementations:
- list.go - Table output, YAML output, interactive mode
- create.go - Resource creation with validation
- apply.go - File handling and error management

For more details on the CLI architecture and implementation, see the code documentation in each package.
