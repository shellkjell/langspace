# LangSpace Advanced Examples

This folder contains comprehensive, production-ready examples that demonstrate the full power of LangSpace for building sophisticated AI agent workflows.

## Examples Overview

| Example | Description | Key Features |
|---------|-------------|--------------|
| [01-autonomous-research-agent](01-autonomous-research-agent.ls) | Self-directed research system that explores topics, synthesizes findings, and generates reports | Iterative loops, web scraping, citation management |
| [02-multi-agent-software-factory](02-multi-agent-software-factory.ls) | Complete software development lifecycle with specialized agents | Full SDLC, parallel reviews, automated testing |
| [03-intelligent-data-pipeline](03-intelligent-data-pipeline.ls) | AI-powered ETL with anomaly detection and auto-remediation | Scripts, database ops, alerting |
| [04-conversational-assistant](04-conversational-assistant.ls) | Stateful multi-turn assistant with memory and context management | Session state, RAG, tool routing |
| [05-continuous-monitoring](05-continuous-monitoring.ls) | 24/7 system monitoring with intelligent incident response | Triggers, escalation, runbooks |
| [06-content-generation-studio](06-content-generation-studio.ls) | Multi-format content creation with quality assurance | Parallel generation, A/B testing, publishing |
| [07-autonomous-coding-agent](07-autonomous-coding-agent.ls) | Self-improving coding agent with test-driven development | TDD, self-debugging, git integration |
| [08-enterprise-workflow-orchestration](08-enterprise-workflow-orchestration.ls) | Complex business process automation with approvals | Human-in-loop, compliance, audit trails |
| [09-tower-of-hanoi-mdap](09-tower-of-hanoi-mdap.ls) | Advanced execution mode for Tower of Hanoi (5 disks, 31 steps) | Voting, rejection sampling, checkpointing |
| [10-massive-refactoring-mdap](10-massive-refactoring-mdap.ls) | Large-scale API migration across monorepos | Consensus-based refactoring, rejection sampling |
| [11-security-audit-mdap](11-security-audit-mdap.ls) | Global security audit for large codebases | High-precision vulnerability scanning, noise reduction |

## Prerequisites

Before running these examples, ensure you have:

1. **API Keys** configured as environment variables:
   ```bash
   export ANTHROPIC_API_KEY="your-key"
   export OPENAI_API_KEY="your-key"  # Optional, for multi-model examples
   ```

2. **MCP Servers** installed (for tool-using examples):
   ```bash
   npm install -g @anthropic/mcp-git
   npm install -g @anthropic/mcp-filesystem
   ```

3. **LangSpace CLI** installed:
   ```bash
   go install github.com/shellkjell/langspace/cmd/langspace@latest
   ```

## Running Examples

```bash
# Run a specific intent
langspace run examples/advanced/01-autonomous-research-agent.ls research "quantum computing applications"

# Execute a pipeline
langspace run examples/advanced/02-multi-agent-software-factory.ls --pipeline develop-feature --input spec.md

# Start trigger listeners (for event-driven examples)
langspace serve examples/advanced/05-continuous-monitoring.ls

# Compile to Python for integration
langspace compile --target python examples/advanced/03-intelligent-data-pipeline.ls
```

## Architecture Patterns Demonstrated

### 1. Agent Specialization
Each example demonstrates agents with focused responsibilities, enabling:
- Better performance through specialized instructions
- Easier debugging and iteration
- Modular composition

### 2. Pipeline Orchestration
Complex workflows are broken into discrete steps with:
- Clear data flow between stages
- Parallel execution where dependencies allow
- Conditional branching based on intermediate results

### 3. Script-First Efficiency
Heavy data operations use scripts instead of tool calls:
- Minimal context window usage
- Atomic multi-step operations
- Sandboxed execution

### 4. Event-Driven Architecture
Triggers enable:
- Real-time response to external events
- Scheduled batch processing
- Integration with existing systems

### 5. Advanced Execution: MDAP
The MDAP (Massively Decomposed Agentic Processes) pattern enables:
- Near-zero error rates on long-horizon tasks
- Consensus through voting (MAKER framework)
- Quality control via rejection sampling (Red-Flagging)

## Best Practices

1. **Start with the simplest example** that matches your use case
2. **Customize the agents** with domain-specific instructions
3. **Add tools gradually** — start with built-ins, then add MCP
4. **Use scripts for data-heavy operations** to minimize costs
5. **Implement proper error handling** with on_error handlers
6. **Test incrementally** using the individual intent invocations

## Contributing

Found a bug or have an improvement? We welcome contributions!
See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.
