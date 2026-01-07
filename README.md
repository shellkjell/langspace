# LangSpace

[![Go Report Card](https://goreportcard.com/badge/github.com/shellkjell/langspace)](https://goreportcard.com/report/github.com/shellkjell/langspace)
[![GoDoc](https://pkg.go.dev/badge/github.com/shellkjell/langspace.svg)](https://pkg.go.dev/github.com/shellkjell/langspace)
[![License](https://img.shields.io/badge/License-GPL%20v2-blue.svg)](LICENSE.md)

LangSpace is a declarative language for composing AI agent workflows. It provides a readable, versionable format for defining agents, tools, and multi-step pipelines that can be executed directly or compiled to other targets.

## Overview

LangSpace sits between writing raw Python/TypeScript and using no-code builders. It captures the full specification of an AI workflow in a single file that can be version-controlled, shared, and executed.

````langspace
agent "code-reviewer" {
  model: "claude-sonnet-4-20250514"
  temperature: 0.3

  instruction: ```
    You are a senior code reviewer. Analyze code for security
    vulnerabilities, performance issues, and best practices.
  ```

  tools: [read_file, search_codebase]
}

intent "review-changes" {
  use: agent("code-reviewer")
  input: git.diff(base: "main")
  output: file("review.md")
}
````

## Installation

```bash
go get github.com/shellkjell/langspace
```

## Getting Started

1. **Define your workflow**: Create a `.ls` file defining agents, tools, and pipelines.
2. **Execute**: Run `langspace run -file workflow.ls -name my-intent`.
3. **Advanced Reliability**: For complex, long-horizon tasks requiring zero-error execution, use the [Advanced: MDAP Pipelines](#advanced-mdap-pipelines) mode.

## Language Syntax

LangSpace uses block-based declarations with key-value properties and supports modular imports for large projects.

### Imports

Import other LangSpace files to share definitions.

```langspace
import "common/agents.ls"
import "prompts/reviewer.md"
```

### Files

Files represent static data: prompts, configuration, or output destinations.

````langspace
# Inline contents
file "prompt.md" {
  contents: ```
    You are a helpful assistant.
  ```
}

# Reference to external file
file "config.json" {
  path: "./config/app.json"
}
````

### Agents

Agents are LLM-powered actors with specific roles and capabilities.

````langspace
agent "analyst" {
  model: "claude-sonnet-4-20250514"
  temperature: 0.5

  instruction: ```
    Analyze the provided data and generate insights.
  ```

  tools: [read_file, query_database]
}
````

### Tools

Tools extend agent capabilities by connecting to external systems.

```langspace
tool "search_codebase" {
  description: "Search for patterns in source code"

  parameters: {
    query: string required
    file_pattern: string optional
  }

  handler: mcp("filesystem-server")
}
```

### Intentions

Intentions express what you want to accomplish.

```langspace
intent "summarize-docs" {
  use: agent("summarizer")
  input: file("docs/**/*.md")
  output: file("summary.md")
}
```

### Pipelines

Pipelines chain multiple agents together with data flowing between steps.

```langspace
pipeline "analyze-and-report" {
  step "analyze" {
    use: agent("analyzer")
    input: $input
  }

  step "report" {
    use: agent("reporter")
    input: step("analyze").output
  }

  output: step("report").output
}
```

### MCP Integration

Connect to Model Context Protocol servers for tool access.

```langspace
mcp "filesystem" {
  command: "npx"
  args: ["-y", "@anthropic/mcp-filesystem", "/workspace"]
}

agent "file-manager" {
  tools: [
    mcp("filesystem").read_file,
    mcp("filesystem").write_file,
  ]
}
```

### Scripts

Scripts enable code-first agent actions — a more efficient alternative to multiple tool calls. Instead of loading full data into the context window through repeated tool invocations, agents write executable code that performs complex operations in a single execution.

````langspace
# Define a reusable script template
script "db-update" {
  language: "python"
  runtime: "python3"

  capabilities: [database]

  parameters: {
    table: string required
    id: string required
    updates: object required
  }

  code: ```python
    import db

    # Find, modify, and save in one execution
    record = db.find(table, {"id": id})
    record.update(updates)
    db.save(table, record)

    print(f"Updated {table}/{id}")
  ```

  timeout: "30s"
}

# Agent that generates and executes scripts
agent "efficient-data-manager" {
  model: "claude-sonnet-4-20250514"

  instruction: ```
    Perform database operations by writing Python scripts
    rather than making individual tool calls. This is more
    efficient and keeps the context window small.
  ```

  scripts: [script("db-update")]
}
````

**Why Scripts over Tools?**

| Approach | Context Usage | Round Trips |
|----------|---------------|-------------|
| Multiple tool calls | High (full data loaded each time) | Many |
| Single script execution | Low (only results returned) | One |

See [examples/09-scripts.ls](examples/09-scripts.ls) for more patterns.

### Configuration

Set global defaults for providers and models.

```langspace
config {
  default_model: "claude-sonnet-4-20250514"

  providers: {
    anthropic: {
      api_key: env("ANTHROPIC_API_KEY")
    }
  }
}
```

### Advanced: MDAP Pipelines

MDAP (Massively Decomposed Agentic Processes) is an **advanced execution mode** that enables reliable execution of long-horizon tasks through voting and rejection sampling. Based on the MAKER framework from "Solving a Million-Step LLM Task with Zero Errors", it ensures near-zero error rates by breaking tasks into atomic microsteps.

```langspace
mdap_pipeline "solve-hanoi" {
  strategy: file("hanoi-strategy")

  mdap_config {
    voting_strategy: "first-to-ahead-by-k"
    k: 3
    parallel_samples: 3
    temperature_first: 0.0
    temperature_subsequent: 0.1
    max_output_tokens: 500
    require_format: true
  }

  total_steps: 31

  microstep "move" {
    use: agent("solver")
    context: {
      state: $current_state
      previous_move: $last_action
    }
  }
}
```

**Key MDAP Features:**
- **Maximal Decomposition**: Each microstep handles exactly ONE atomic action
- **First-to-ahead-by-k Voting**: Multiple samples vote until k-margin consensus
- **Red-Flagging**: Reject suspicious responses (too long, wrong format)
- **Checkpointing**: Automatic state saves for recovery

See [examples/advanced/09-tower-of-hanoi-mdap.ls](examples/advanced/09-tower-of-hanoi-mdap.ls) for a complete example.

### Comments

Single-line comments start with `#`:

```langspace
# This is a comment
agent "example" { }  # Inline comment
```

## Usage

### As a Library

```go
import (
    "github.com/shellkjell/langspace/pkg/parser"
    "github.com/shellkjell/langspace/pkg/workspace"
)

// Parse LangSpace definitions
input := `
agent "reviewer" {
  model: "claude-sonnet-4-20250514"
  instruction: "Review code for issues"
}
`

p := parser.New(input)
entities, err := p.Parse()
if err != nil {
    log.Fatal(err)
}

// Add to workspace
ws := workspace.New()
for _, entity := range entities {
    ws.AddEntity(entity)
}
```

### Command Line

```bash
# Parse a file and show statistics
langspace parse -file workflow.ls

# Execute a workflow
langspace run -file workflow.ls -name my-intent

# Start a server for triggers (HTTP/SSE)
langspace serve -file triggers.ls -port 8080

# Compile to Python/LangGraph
langspace compile --target python -file workflow.ls -output ./out

# Validate syntax and rules
langspace validate -file workflow.ls

# Start Language Server (LSP) for IDE support
langspace lsp
```

## VS Code Extension

A VS Code extension for LangSpace is available in the `vscode-langspace/` directory. It provides:
- Syntax highlighting for `.ls` files
- Intelligent IDE support (Go to Definition) via LSP
- Language configuration and snippets

To install manually:
```bash
code --install-extension vscode-langspace/langspace-0.1.0.vsix
```

## Project Status

**Current Phase: Integration & Compilation**

### - Implemented & Working
- **Block-based Syntax**: Full DSL support with nested blocks and typed parameters
- **Expression Parser**: Method calls (`str.upper()`), comparisons (`x == y`), and control flow (`branch`, `loop`)
- **Direct Execution**: Built-in runtime with Anthropic/OpenAI/Ollama support
- **Tool Orchestration**: Auto-management of tool loops and MCP server integration
- **Scripting**: Sandboxed Python/Shell execution for context-efficient actions
- **MDAP Pipelines**: Massively Decomposed Agentic Processes with voting and rejection sampling
- **Compilation**: Python/LangGraph target generation via `langspace compile`
- **Automation**: Trigger engine for scheduled and event-driven workflows
- **Workspace**: Full persistence, snapshoting, and versioning system
- **CLI**: Comprehensive toolset (`parse`, `run`, `validate`, `serve`, `compile`)
- **Modular Imports**: Multi-file support with `import` statements and recursive loading
- **Intelligent IDE Support**: Full "Go to Definition" support across files via LSP server
- **Test Coverage**: 160+ tests covering core logic, imports, and LSP features

### - In Progress / Planned
- **TypeScript Compilation**: Target generation for Node.js/Deno
- **Cloud Hosting**: Managed deployment for LangSpace workflows
- **Advanced Debugging**: Step-by-step execution visualization
- **Security**: Granular RBAC and secret management

See [ROADMAP.md](ROADMAP.md) for the full development plan and [PRD.md](PRD.md) for the product specification.

## Contributing

## Development

### Prerequisites

- Go 1.23+
- Docker Desktop (for local CI)
- [act](https://github.com/nektos/act) (for local CI)

### Setup Local CI

To run GitHub Actions locally and ensure quality before pushing, run:

```bash
make setup-local-ci
```

This will configure a git pre-push hook that runs the CI suite (`test` and `build` jobs) locally using `act`.

### Running Tests

```bash
make test
```

### Local CI Manual Run

You can also run the local CI manually:

```bash
make local-ci
```

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[GNU GPL v2](LICENSE.md)