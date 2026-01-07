# LangSpace Development Roadmap

This document outlines the planned development roadmap for the LangSpace project.

## Current Status (v0.1.0)

### Core Features
- [x] Basic entity system with File, Agent, and Task types
- [x] Efficient parser with error reporting
- [x] Workspace management for entities
- [x] Comprehensive test coverage
- [x] Validator interface exported for extensibility
- [x] Line/column error reporting

### Recent Improvements
- [x] Enhanced parser performance
- [x] Improved error messages with line/position info
- [x] Multi-line string support
- [x] Entity type registry
- [x] Block syntax parsing (full DSL support)
- [x] Flexible agent property validation
- [x] EntityValidator interface for custom validators
- [x] Single-line comment support (# comments)
- [x] Entity relationships (assigned, depends, produces, consumes)
- [x] Updated performance benchmarks
- [x] Script entity type for code-first agent actions (context-efficient)
- [x] Generic slice utilities package for type-safe collection operations
- [x] All core entity types: file, agent, tool, intent, pipeline, step, trigger, config, mcp, script
- [x] Tool execution loop for agents
- [x] Shell and HTTP tool handlers
- [x] Sandboxed Python and Shell script execution
- [x] Trigger engine for automated workflows
- [x] LangSpace CLI with `serve` command
- [x] Model Context Protocol (MCP) client interface
- [x] Advanced Execution: MDAP (Massively Decomposed Agentic Processes) for high-reliability tasks
- [x] First-to-ahead-by-k voting and rejection sampling for reliable long-horizon tasks


### Known Issues / Limitations
- [x] ~~Nested block parsing not yet supported~~ — Now supports nested entities (step inside pipeline, handler blocks)
- [x] ~~Typed parameters not yet supported~~ — Now parses `name: string required "desc"` syntax
- [x] Advanced expression syntax supported — Method calls (`git.func()`), comparisons (`x == y`), control flow (`branch`, `loop`)
- [ ] Some PRD features still pending (triggers, automation, execution)
- [x] All 9 examples parse fully (01-09); advanced expressions supported
- [x] MDAP Tower of Hanoi example (5-disk problem with voting)
- [x] Performance claims verified with current benchmarks
- [x] Entity relationships implemented

## Immediate Priorities (v0.1.1)

### 1. Documentation and Consistency
- [x] Update API documentation to match implementation
- [x] Align repository naming across all documentation
- [x] Verify and update performance claims

### 2. Core Implementation Alignment
- [x] Block syntax parsing with full DSL support
- [x] Move validator interface to validator package
- [x] Implement comprehensive error reporting with line/column info
- [ ] Add proper authentication/authorization mechanisms
- [x] Implement entity relationships (file-agent-task)

## Short-term Goals (v0.2.0)

### 1. Entity System Enhancements
- [x] Add support for entity relationships
- [x] Implement entity validation hooks
- [x] Implement pluggable entity type system (RegisterEntityType, RegisterValidator)
- [x] Add entity metadata support
- [x] Create entity event system
- [x] Support entity versioning (WithVersioning, GetEntityVersion, GetEntityHistory)

### 2. Script Execution Runtime
- [x] Implement sandboxed Python script execution
- [x] Add JavaScript/Node.js runtime support (via Shell/Node)
- [x] Implement capability-based security model (basic)
- [x] Add resource limits (timeout, memory, CPU)
- [x] Support agent-generated code execution

### 3. Parser Improvements
- [x] Add support for comments
- [x] ~~Implement nested block parsing~~ — Supports step/parallel/handler/on_success/on_failure/on_error inside parent blocks
- [ ] Implement syntax highlighting
- [ ] Add source map support
- [x] Improve error recovery
- [ ] Support for custom entity types
- [x] ~~Handle typed parameters~~ — Supports `query: string required "desc"` and `type: enum ["a", "b"]`
- [x] **Expression parser** — Method calls, comparisons, control flow (branch, loop, break_if)

### 4. Workspace Features
> **Note:** Many workspace features are implemented as infrastructure for future phases.
> They are tested and working but not yet exposed via CLI or used by execution runtime.

- [x] Add workspace persistence (SaveTo/LoadFrom, SaveToFile/LoadFromFile)
- [x] Implement workspace snapshots (CreateSnapshot, RestoreSnapshot, SnapshotStore)
- [x] Add entity search/query capabilities
- [x] Support workspace configuration (Config, WithConfig, limits, constraints)
- [x] Add workspace events
- [x] Entity transformation pipeline (for batch processing)
- [x] Concurrent batch operations (AddEntitiesBatch, etc.)
- [x] Dependency graph with topological sort

### 5. Architecture Improvements
- [ ] Reorganize package structure
- [ ] Implement proper entity/ package
- [ ] Add clean extension points
- [ ] Improve error handling system
- [ ] Add workspace configuration support

### 6. Security and Validation
- [ ] Implement robust access control
- [ ] Add comprehensive entity validation
- [ ] Implement secure entity storage
- [ ] Add audit logging
- [ ] Add security documentation

## Medium-term Goals (v0.3.0)

### 1. Advanced Features
- [x] Entity dependency tracking
- [x] Concurrent entity processing
- [x] Entity lifecycle hooks
- [x] Custom entity validators
- [x] Entity transformation pipeline

### 2. Developer Experience
- [ ] CLI tools for entity management
- [ ] Interactive debugging support
- [ ] Documentation generator
- [ ] Entity visualization tools
- [ ] Integration examples

### 3. Performance Optimizations
- [ ] Parallel parsing for large inputs
- [ ] Entity caching system
- [ ] Memory usage optimizations
- [ ] Parser streaming support
- [ ] Lazy entity loading

## Long-term Goals (v1.0.0)

### 1. Enterprise Features
- [ ] Role-based access control
- [ ] Entity audit logging
- [ ] Distributed workspace support
- [ ] Entity encryption
- [ ] Compliance features

### 2. Integration Support
- [ ] REST API
- [ ] GraphQL support
- [ ] WebSocket events
- [ ] Message queue integration
- [ ] Plugin system

### 3. Advanced Use Cases
- [ ] Large-scale entity management
- [ ] Real-time collaboration
- [ ] Entity conflict resolution
- [ ] Custom storage backends
- [ ] Advanced query language

## Release Schedule

- **v0.1.1**:
  - Focus on documentation fixes
  - Core implementation alignment
  - Security fundamentals

- **v0.2.0**:
  - Entity system enhancements
  - Security and validation
  - Architecture improvements

- **v0.3.0**:
  - Advanced features implementation
  - Developer tools
  - Performance optimizations

- **v1.0.0**:
  - Enterprise features
  - Integration support
  - Production readiness

## Contributing

We welcome contributions in the following areas:
1. Documentation improvements and consistency fixes
2. Core functionality alignment with PRD
3. Security implementations
4. Testing and benchmarking
5. Feature requests and bug reports

## Feedback

We value community feedback in shaping this roadmap. Please feel free to:
- Open issues for feature requests
- Submit pull requests for improvements
- Join discussions about future directions
- Share use cases and requirements
