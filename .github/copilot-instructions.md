# LangSpace AI Agent Instructions

LangSpace is a declarative DSL for composing AI agent workflows. The codebase is a Go 1.23+ project implementing a lexer → parser → AST → runtime pipeline.

## Architecture & Philosophy

**Big Picture:**
```
Input (.ls file) → Tokenizer → Parser → AST Entities → Workspace → Runtime/Execution
```

**Core Philosophy: Script-First vs Tool-Heavy**
LangSpace prioritizes "Script-first" agent actions to minimize context window bloat. Instead of multiple tool round-trips, agents write executable code (Python, JS, etc.) that performs complex operations in a single execution.

**Core packages:**
- [pkg/tokenizer](../pkg/tokenizer/tokenizer.go) - Lexical analysis, produces tokens with line/column tracking
- [pkg/parser](../pkg/parser/parser.go) - Recursive descent parser, builds AST from tokens
- [pkg/ast](../pkg/ast/entity.go) - Entity types (`agent`, `file`, `tool`, `intent`, `pipeline`, `script`, `mdap_pipeline`, `microstep`, etc.)
- [pkg/workspace](../pkg/workspace/workspace.go) - Entity storage with hooks, events, relationships, and snapshots
- [pkg/validator](../pkg/validator/validator.go) - Type-specific validation rules
- [pkg/runtime](../pkg/runtime/runtime.go) - LLM integration and workflow execution (Intent/Pipeline)
- [pkg/slices](../pkg/slices/slices.go) - Generic slice utilities (Filter, Map, Find, etc.)

## Key Patterns

### Entity System (pkg/ast)
- All entities implement `ast.Entity` interface (Type, Name, Properties, Metadata, Location)
- Use factory functions: `ast.NewAgentEntity("name")`, `ast.NewFileEntity("name")`, etc.
- Extensible via `ast.RegisterEntityType()` for custom entity types
- Value types are sealed: `StringValue`, `NumberValue`, `BoolValue`, `ArrayValue`, `ObjectValue`, `ReferenceValue`, `VariableValue`, `TypedParameterValue`, `PropertyAccessValue`, etc.

### Parser Error Recovery
```go
// Use ParseWithRecovery() for graceful error handling
p := parser.New(input).WithErrorRecovery()
result := p.ParseWithRecovery()
// result.Entities contains successfully parsed entities
// result.Errors contains ParseError with Line/Column/Message
```

### Workspace Hooks, Events & Validation
```go
ws := workspace.New()
// Lifecycle hooks
ws.OnEntityEvent(workspace.HookBeforeAdd, func(e ast.Entity) error {
    return nil // Return error to cancel
})
// Custom validators
ws.RegisterEntityValidator("agent", func(e ast.Entity) error {
    if _, ok := e.GetProperty("model"); !ok {
        return fmt.Errorf("missing model")
    }
    return nil
})
// Global events
ws.OnEvent(func(event workspace.Event) {
    // React to EventEntityAdded, etc.
})
```

### Runtime Execution (pkg/runtime)
- `Runtime` coordinates `LLMProvider`s and `ExecutionContext`.
- `Resolver` handles variable interpolation and property access during execution.
- `StreamHandler` manages real-time output (chunks and progress events).
- Execution flow: `Execute` → `executeIntent` or `executePipeline` → `LLMProvider.Complete`.

### Generic Slice Utilities (pkg/slices)
**CRITICAL:** Prefer these over manual loops for readability and consistency:
```go
slices.Filter(entities, func(e ast.Entity) bool { return e.Type() == "agent" })
slices.Find(entities, predicate)
slices.Map(entities, transform)
slices.Any(entities, predicate)
slices.GroupBy(entities, keyFunc)
```

## Development Commands

```bash
make test          # Run all tests with race detector
make lint          # Run golangci-lint
make coverage      # Generate coverage report (coverage.out)
make benchmark     # Run benchmarks with memory stats
go test -v ./pkg/parser/...  # Test specific package
```

## Testing Patterns

- Table-driven tests with `checkFirst` callback pattern (see [parser_test.go](../pkg/parser/parser_test.go))
- Benchmark functions for performance-critical paths
- Test names: `TestParser_Parse_BlockSyntax`, `TestParser_TypedParameters`
- Error cases test specific error substrings

```go
{
    name:       "simple_agent",
    input:      `agent "reviewer" { model: "gpt-4o" }`,
    wantCount:  1,
    checkFirst: func(t *testing.T, e ast.Entity) {
        // Validate parsed entity
    },
}
```

## LangSpace Syntax Quick Reference

```langspace
# Agents - LLM-powered actors
agent "name" {
  model: "claude-sonnet-4-20250514"
  temperature: 0.7
  instruction: ```multiline text```
  tools: [tool_a, tool_b]
}

# Files - static data
file "name" {
  path: "./path/to/file"   # OR
  contents: ```inline content```
}

# Pipelines - multi-step workflows
pipeline "name" {
  step "step1" { use: agent("name") input: $input }
  step "step2" { use: agent("other") input: step("step1").output }
  output: step("step2").output
}

# Scripts - code-first actions (context efficient)
script "db-update" {
  language: "python"
  code: ```python ... ```
  capabilities: [database]
}

# MDAP Pipelines - reliable long-horizon tasks
mdap_pipeline "solve-hanoi" {
  strategy: file("strategy")
  mdap_config {
    voting_strategy: "first-to-ahead-by-k"
    k: 3
  }
  microstep "move" {
    use: agent("solver")
  }
}

# References: agent("x"), file("y"), step("z").output
# Variables: $input, $code
# Property access: params.location, config.defaults.timeout
```

## Entity Type Properties

| Type | Required Properties |
|------|---------------------|
| `agent` | `model` |
| `file` | `path` OR `contents` |
| `tool` | `command` OR `function` |
| `intent` | `use` (agent reference) |
| `step` | `use` |
| `trigger` | `event` OR `schedule` |
| `mcp` | `command` |
| `script` | `language`, `code` OR `path` |
| `mdap_pipeline` | `strategy` |
| `microstep` | `use` (agent reference) |
| `mdap_config` | (no required properties) |

## Code Style

- Document all exported functions/types
- Use functional options pattern: `WithConfig()`, `WithValidator()`
- Errors include context: `fmt.Errorf("entity not found: %s %q", entityType, entityName)`
- Concurrent-safe workspace operations use `sync.RWMutex`
- Return copies from getters to prevent external mutation

## Current Limitations (Not Yet Implemented)

- TypeScript compilation target (Python works)
- Cloud-hosted execution
- Advanced debugging visualization

See [examples/](../examples/) for syntax demos and [ROADMAP.md](../ROADMAP.md) for planned features.
