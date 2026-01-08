# LangSpace AI Agent Instructions

LangSpace is a declarative DSL for composing AI agent workflows. This Go 1.23+ project implements a complete pipeline: **Tokenizer → Parser → AST → Workspace → Runtime**.

## Architecture Overview

```
.ls file → pkg/tokenizer → pkg/parser → pkg/ast (Entities) → pkg/workspace → pkg/runtime
                                                                    ↓
                                                            LLM Providers (Anthropic/OpenAI)
```

**Core Packages:**
| Package | Purpose | Key File |
|---------|---------|----------|
| `pkg/tokenizer` | Lexical analysis with line/column tracking | [tokenizer.go](../pkg/tokenizer/tokenizer.go) |
| `pkg/parser` | Recursive descent parser, builds AST | [parser.go](../pkg/parser/parser.go) |
| `pkg/ast` | Entity types & sealed Value interface | [entity.go](../pkg/ast/entity.go) |
| `pkg/workspace` | Entity storage, hooks, events, snapshots | [workspace.go](../pkg/workspace/workspace.go) |
| `pkg/runtime` | LLM integration, execution orchestration | [runtime.go](../pkg/runtime/runtime.go) |
| `pkg/validator` | Type-specific validation rules | [validator.go](../pkg/validator/validator.go) |
| `pkg/slices` | Generic slice utilities (prefer over manual loops) | [slices.go](../pkg/slices/slices.go) |

## Development Commands

```bash
make test          # Run all tests with race detector
make lint          # Run golangci-lint
make coverage      # Generate coverage.out report
make benchmark     # Run benchmarks with memory stats
make local-ci      # Run CI suite locally via `act`
go test -v ./pkg/parser/...  # Test specific package
```

## Key Patterns & Conventions

### Entity System
All entities implement `ast.Entity` interface. Use factory functions:
```go
agent := ast.NewAgentEntity("reviewer")      // Creates *AgentEntity
file := ast.NewFileEntity("config")          // Creates *FileEntity
pipeline := ast.NewPipelineEntity("review")  // Creates *PipelineEntity with Steps slice
```

Extend via registry: `ast.RegisterEntityType("custom", factory)`.

### Sealed Value Types
The `ast.Value` interface is sealed via unexported marker method. Valid types:
`StringValue`, `NumberValue`, `BoolValue`, `ArrayValue`, `ObjectValue`, `ReferenceValue`, `VariableValue`, `TypedParameterValue`, `PropertyAccessValue`, `MethodCallValue`, `FunctionCallValue`, `InferredValue`, `BranchValue`, `LoopValue`, `NestedEntityValue`.

### Parser Error Recovery
```go
p := parser.New(input).WithErrorRecovery()
result := p.ParseWithRecovery()
// result.Entities - successfully parsed
// result.Errors - []ParseError with Line/Column/Message
```

### Workspace Lifecycle Hooks
```go
ws := workspace.New()
ws.OnEntityEvent(workspace.HookBeforeAdd, func(e ast.Entity) error {
    return nil // Return error to cancel operation
})
ws.RegisterEntityValidator("agent", func(e ast.Entity) error {
    if _, ok := e.GetProperty("model"); !ok {
        return fmt.Errorf("agent requires 'model' property")
    }
    return nil
})
```

### Use pkg/slices for Collection Operations
**CRITICAL:** Prefer these over manual loops:
```go
slices.Filter(entities, func(e ast.Entity) bool { return e.Type() == "agent" })
slices.Find(entities, predicate)   // Returns (entity, bool)
slices.FindIndex(entities, pred)   // Returns int (-1 if not found)
slices.Map(entities, transform)
slices.GroupBy(entities, keyFunc)
```

### Runtime Execution Flow
`Runtime.Execute()` → dispatches by entity type → `executeIntent`/`executePipeline`/`executeMDAPPipeline` → `LLMProvider.Complete()` with tool loop.

Providers implement `LLMProvider` interface: `Complete()`, `CompleteStream()`, `ListModels()`.

## Testing Conventions

**Table-driven tests with `checkFirst` callback pattern:**
```go
{
    name:      "pipeline_with_steps",
    input:     `pipeline "review" { step "analyze" { use: agent("a") } }`,
    wantCount: 1,
    checkFirst: func(t *testing.T, e ast.Entity) {
        pipeline := e.(*ast.PipelineEntity)
        if len(pipeline.Steps) != 1 { t.Error("expected 1 step") }
    },
}
```

- Test file naming: `*_test.go` alongside source
- Test function naming: `TestParser_Parse_BlockSyntax`, `TestWorkspace_AddEntity`
- Error tests: check specific substrings with `strings.Contains`
- Benchmarks: `BenchmarkParser_Parse_Large` for performance-critical paths

## LangSpace Syntax Reference

```langspace
# Agents - LLM-powered actors (required: model)
agent "reviewer" {
  model: "claude-sonnet-4-20250514"
  temperature: 0.7
  instruction: ```multiline prompt```
  tools: [read_file, write_file]
}

# Files - static data (required: path OR contents)
file "prompt" { contents: ```inline content``` }
file "config" { path: "./config.json" }

# Pipelines - multi-step workflows
pipeline "analyze" {
  step "s1" { use: agent("a") input: $input }
  step "s2" { use: agent("b") input: step("s1").output }
  output: step("s2").output
}

# MDAP Pipelines - reliable long-horizon tasks with voting
mdap_pipeline "solve-task" {
  strategy: file("strategy")
  mdap_config { voting_strategy: "first-to-ahead-by-k" k: 3 }
  microstep "step" { use: agent("solver") }
}

# References: agent("x"), file("y"), step("z").output
# Variables: $input, $current_state
# Property access: params.location, config.defaults.timeout
# Typed params: query: string required "description"
```

## Entity Validation Rules

| Type | Required Properties |
|------|---------------------|
| `agent` | `model` |
| `file` | `path` OR `contents` |
| `tool` | `command` OR `function` |
| `intent` | `use` (agent ref) |
| `step` | `use` |
| `mcp` | `command` |
| `script` | `language`, `code` OR `path` |
| `mdap_pipeline` | `strategy` |
| `microstep` | `use` (agent ref) |

## Code Style Requirements

- Document all exported functions/types with godoc comments
- Use functional options: `WithConfig()`, `WithValidator()`, `WithVersioning()`
- Errors include context: `fmt.Errorf("entity not found: %s %q", typ, name)`
- Thread safety: Workspace uses `sync.RWMutex`, return copies from getters
- CLI pattern: Separate `run()` function from `main()` for testability (see [cmd/langspace/main.go](../cmd/langspace/main.go))

## Examples

- Basic examples: [examples/](../examples/) (01-09 cover core features)
- Advanced MDAP: [examples/advanced/09-tower-of-hanoi-mdap.ls](../examples/advanced/09-tower-of-hanoi-mdap.ls)
- See [ROADMAP.md](../ROADMAP.md) for implementation status
