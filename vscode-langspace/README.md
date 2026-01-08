# LangSpace VSCode Extension

Syntax highlighting and language support for [LangSpace](https://github.com/shellkjell/langspace) files.

## Features

- Syntax highlighting for `.ls` files
- **Intelligent IDE support**: "Go to Definition" across imported files
- Bracket matching and auto-closing
- Comment toggling (`Cmd+/` or `Ctrl+/`)
- Code folding

## Installation

### From Source

1. Clone the repository
2. Open the `vscode-langspace` folder in VSCode
3. Press `F5` to launch Extension Development Host
4. Open a `.ls` file to see syntax highlighting

### Manual Installation

```bash
cd vscode-langspace
# Requires Node.js and npm
npm install
npm run compile
npm install -g @vscode/vsce
vsce package
code --install-extension langspace-0.1.0.vsix
```

> **Note**: For "Go to Definition" support, make sure the `langspace` CLI is installed and available in your PATH.

## Supported Syntax

### Entity Types
- **Core entities**: `agent`, `file`, `tool`, `intent`, `config`, `trigger`
- **Pipelines**: `pipeline`, `step`, `parallel`
- **MDAP pipelines**: `mdap_pipeline`, `microstep`, `mdap_config`
- **Integrations**: `mcp`, `script`

### Control Flow
- **Branching**: `branch` with pattern matching
- **Loops**: `loop` with `max` iterations and `break_if` conditions
- **Conditional execution**: `if`/`else` patterns

### Data Types
- **Primitives**: `string`, `number`, `bool`
- **Collections**: `array`, `object`
- **Special**: `enum` for constrained values

### Advanced Features
- **References**: `agent("name")`, `step("x").output`, `file("path")`
- **Variables**: `$input`, `$current`, `$output`
- **Property access**: `step("x").output.field`, `params.location`
- **Multi-line strings**: Triple backticks with optional language tag
- **Comments**: `# single line comments`

## Examples

### Basic Agent

```langspace
agent "reviewer" {
  model: "claude-sonnet-4-20250514"
  temperature: 0.3
  
  instruction: ```
    Review the code for best practices.
  ```
  
  tools: [read_file, search_codebase]
}
```

### MDAP Pipeline (High-Reliability Tasks)

```langspace
mdap_pipeline "solve-task" {
  strategy: file("strategy.md")
  
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
  
  microstep "step" {
    use: agent("solver")
    context: {
      state: $current_state
    }
  }
}
```

### Control Flow

```langspace
pipeline "adaptive-workflow" {
  step "classify" {
    use: agent("classifier")
    input: $input
  }
  
  # Branch based on classification
  branch step("classify").output.type {
    "urgent" => step "priority-handler" {
      use: agent("urgency-handler")
      input: $input
    }
    
    "normal" => step "standard-handler" {
      use: agent("standard-processor")
      input: $input
    }
  }
  
  output: $branch.output
}
```

## License

GNU GPL v2
