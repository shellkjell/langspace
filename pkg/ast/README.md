# Abstract Syntax Tree (AST) Package

The `ast` package provides the core data structures and interfaces for representing LangSpace entities in memory. It serves as the backbone of the language's type system and entity management.

## Overview

The AST package defines the following key components:

- `Entity` interface: The fundamental building block of LangSpace
- `BaseEntity`: Common implementation shared across entity types
- `FileEntity`: Represents file system resources
- `AgentEntity`: Represents AI-powered agents
- `ToolEntity`: Represents external tool integrations
- `IntentEntity`: Represents desired outcomes
- `PipelineEntity`: Represents multi-step workflows
- `StepEntity`: Represents pipeline steps
- `TriggerEntity`: Represents event-driven execution
- `ConfigEntity`: Represents global configuration
- `MCPEntity`: Represents MCP server connections
- `ScriptEntity`: Represents code-first agent actions
- `MicrostepEntity`: Represents atomic steps in MDAP pipelines
- `MDAPConfigEntity`: Represents MDAP execution configuration
- `MDAPPipelineEntity`: Represents MDAP pipelines with voting and rejection sampling

## Usage

```go
import "github.com/shellkjell/langspace/pkg/ast"

// Create a new file entity
entity, err := ast.NewEntity("file", "config.json")
if err != nil {
    log.Fatal(err)
}

// Add properties
entity.SetProperty("path", ast.StringValue{Value: "/etc/config.json"})
entity.SetProperty("contents", ast.StringValue{Value: "{}"})

// Add metadata
entity.SetMetadata("author", "john.doe")
entity.SetMetadata("version", "1.0")

// Retrieve metadata
author, ok := entity.GetMetadata("author")
if ok {
    fmt.Printf("Author: %s\n", author)
}

// Get all metadata
allMeta := entity.AllMetadata()
for key, value := range allMeta {
    fmt.Printf("%s: %s\n", key, value)
}
```

## Entity Types

### File Entity
- **Purpose**: Represents file system resources
- **Properties**:
  - `path`: File system path (optional)
  - `contents`: Inline file contents (optional)

### Agent Entity
- **Purpose**: Represents AI-powered agents
- **Properties**:
  - `model`: LLM model identifier
  - `temperature`: Sampling temperature
  - `instruction`: System instructions
  - `tools`: List of available tools
  - `scripts`: List of available scripts

### Tool Entity
- **Purpose**: Represents external tool integrations
- **Properties**:
  - `description`: Tool description
  - `parameters`: Input parameter definitions
  - `handler`: Execution handler (mcp, http, shell, builtin)

### Intent Entity
- **Purpose**: Represents desired outcomes
- **Properties**:
  - `use`: Agent or pipeline reference
  - `input`: Input data or file reference
  - `output`: Output destination
  - `context`: Additional context files

### Pipeline Entity
- **Purpose**: Represents multi-step workflows
- **Properties**:
  - `output`: Final output definition
- **Additional**: Contains ordered list of StepEntity

### Script Entity
- **Purpose**: Represents code-first agent actions for context-efficient operations
- **Properties**:
  - `language`: Programming language (python, javascript, bash, sql)
  - `runtime`: Runtime/interpreter (python3, node, bash, postgresql)
  - `code`: Script source code (inline or variable reference)
  - `parameters`: Input parameters passed to the script
  - `capabilities`: What the script can access (database, filesystem, network)
  - `timeout`: Maximum execution time
  - `limits`: Resource constraints (memory, cpu)
  - `sandbox`: Security restrictions (allowed_modules, network access)

**Why Scripts?** Scripts solve the context window problem with MCP/tool-heavy approaches. Instead of loading full data into the context through multiple tool calls, agents write executable code that performs complex operations in a single execution, returning only the results.

```go
// Create a script entity
script := ast.NewScriptEntity("update-record")
script.SetProperty("language", ast.StringValue{Value: "python"})
script.SetProperty("runtime", ast.StringValue{Value: "python3"})
script.SetProperty("capabilities", ast.ArrayValue{Elements: []ast.Value{
    ast.StringValue{Value: "database"},
}})
script.SetProperty("code", ast.StringValue{Value: `
import db
record = db.find("users", {"id": user_id})
record["description"] = new_description
db.save("users", record)
print(f"Updated user {user_id}")
`})
```

### MCP Entity
- **Purpose**: Represents MCP server connections
- **Properties**:
  - `transport`: Connection type (stdio, sse)
  - `command`: Command to spawn the server
  - `args`: Command arguments
  - `url`: SSE endpoint URL

### Microstep Entity
- **Purpose**: Represents atomic steps in MDAP (Massively Decomposed Agentic Processes) pipelines
- **Properties**:
  - `use`: Agent reference for execution
  - `context`: Minimal context for this step (state, strategy, previous_action)
  - `output_schema`: Expected output format for validation
  - `red_flags`: Conditions that trigger rejection sampling

### MDAP Config Entity
- **Purpose**: Configures MDAP execution parameters for voting and rejection sampling
- **Properties**:
  - `voting_strategy`: "first-to-ahead-by-k" (default) or "majority"
  - `k`: Vote margin required for consensus (default: 3)
  - `parallel_samples`: Number of parallel samples per round (default: k)
  - `temperature_first`: Temperature for first sample (default: 0.0)
  - `temperature_subsequent`: Temperature for subsequent samples (default: 0.1)
  - `max_output_tokens`: Red-flag threshold for response length
  - `require_format`: Whether to reject format errors (default: true)
  - `checkpoint_interval`: Steps between checkpoints (default: 1000)

### MDAP Pipeline Entity
- **Purpose**: Orchestrates massive parallel step execution with voting for reliable long-horizon tasks
- **Properties**:
  - `strategy`: Fixed strategy provided to all microsteps
  - `total_steps`: Total number of steps in the pipeline
  - `input`: Initial state for the pipeline
- **Additional**: Contains MDAPConfigEntity and list of MicrostepEntity

**Why MDAP?** MDAP solves the reliability problem for long-horizon tasks. Based on the MAKER framework from "Solving a Million-Step LLM Task with Zero Errors", it uses voting and rejection sampling to ensure each step is executed correctly before proceeding.

```go
// Create an MDAP pipeline
pipeline := ast.NewMDAPPipelineEntity("solve-hanoi")
pipeline.SetProperty("strategy", ast.StringValue{Value: "recursive tower of hanoi"})
pipeline.SetProperty("total_steps", ast.NumberValue{Value: 31})

// Add config
config := ast.NewMDAPConfigEntity()
config.SetProperty("k", ast.NumberValue{Value: 3})
config.SetProperty("voting_strategy", ast.StringValue{Value: "first-to-ahead-by-k"})
pipeline.Config = config

// Add microstep
microstep := ast.NewMicrostepEntity("move")
microstep.SetProperty("use", ast.ReferenceValue{Type: "agent", Name: "solver"})
pipeline.AddMicrostep(microstep)
```

### Config Entity
- **Purpose**: Represents global configuration
- **Properties**:
  - `default_model`: Default LLM model
  - `providers`: Provider configurations

## Entity Metadata

All entities support key-value metadata for storing additional information:

```go
// Set metadata
entity.SetMetadata("key", "value")

// Get metadata (returns value and existence flag)
value, exists := entity.GetMetadata("key")

// Get all metadata as a map (returns a copy)
allMetadata := entity.AllMetadata()
```

**Use cases for metadata:**
- Tracking entity creation time or author
- Storing version information
- Adding custom tags or labels
- Associating external identifiers

## Value Types

The AST package supports various value types:

- `StringValue`: String literals and multiline strings
- `NumberValue`: Numeric values (float64)
- `BoolValue`: Boolean values (true/false)
- `ArrayValue`: Arrays of values
- `ObjectValue`: Key-value object maps
- `ReferenceValue`: References to other entities (e.g., `agent("name")`)
- `VariableValue`: Variable references (e.g., `$input`)

## Extension

To add new entity types:

1. Create a new struct embedding `*BaseEntity`
2. Add a constructor function `NewXxxEntity(name string)`
3. Add the new type to `NewEntity` factory function
4. Implement type-specific validation rules in the validator package
5. Update parser if special syntax is needed
6. Update relevant documentation

## Best Practices

- Always validate entities after creation
- Use the `NewEntity` factory function instead of direct struct initialization
- Handle all error cases when adding properties
- Consider using composition with `BaseEntity` for new entity types
- Use metadata for extensible, non-core properties
- Remember that `AllMetadata()` returns a copy to prevent unintended modifications
