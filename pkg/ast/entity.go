package ast

import (
	"fmt"
)

// Package ast provides the Abstract Syntax Tree (AST) components for LangSpace.
// It defines the core entity types and their behaviors, supporting the language's
// type system and validation rules.

// Value represents any value in the AST.
// This interface uses a sealed pattern via an unexported marker method,
// ensuring that only value types defined in this package can implement it.
// This provides type safety and prevents external packages from creating
// invalid value types.
type Value interface {
	// isValue is an unexported marker method that seals this interface.
	// Only types in this package can implement Value.
	isValue()
}

// StringValue represents a string literal
type StringValue struct {
	Value string
}

func (s StringValue) isValue() {}

// NumberValue represents a numeric literal
type NumberValue struct {
	Value float64
}

func (n NumberValue) isValue() {}

// BoolValue represents a boolean literal
type BoolValue struct {
	Value bool
}

func (b BoolValue) isValue() {}

// ArrayValue represents an array of values
type ArrayValue struct {
	Elements []Value
}

func (a ArrayValue) isValue() {}

// ObjectValue represents an object/map of key-value pairs
type ObjectValue struct {
	Properties map[string]Value
}

func (o ObjectValue) isValue() {}

// ReferenceValue represents a reference to another entity (e.g., agent("name"))
type ReferenceValue struct {
	Type string // "agent", "file", "tool", "step", etc.
	Name string
	Path []string // For dot access, e.g., step("x").output
}

func (r ReferenceValue) isValue() {}

// VariableValue represents a variable reference (e.g., $input)
type VariableValue struct {
	Name string
}

func (v VariableValue) isValue() {}

// TypedParameterValue represents a typed parameter declaration
// e.g., "query: string required" or "name: string optional \"default\" \"description\""
type TypedParameterValue struct {
	ParamType   string   // "string", "number", "bool", "array", "object", "enum"
	Required    bool     // true if "required", false if "optional"
	Default     Value    // Default value (optional)
	Description string   // Documentation string (optional)
	EnumValues  []string // For enum types, the allowed values
}

func (t TypedParameterValue) isValue() {}

// NestedEntityValue represents a nested entity block (e.g., step inside pipeline)
type NestedEntityValue struct {
	Entity Entity
}

func (n NestedEntityValue) isValue() {}

// PropertyAccessValue represents a property access chain (e.g., params.location or step("x").output)
type PropertyAccessValue struct {
	Base string   // The base identifier (e.g., "params")
	Path []string // The property path (e.g., ["location"])
}

func (p PropertyAccessValue) isValue() {}

// MethodCallValue represents a method call on an object (e.g., git.staged_files(), github.pr.comment(output))
type MethodCallValue struct {
	Object     Value   // The object being called on (can be PropertyAccessValue or another value)
	Method     string  // The method name
	Arguments  []Value // The arguments to the method
	InlineBody Entity  // Optional inline block (for patterns like pipeline("name") { ... })
}

func (m MethodCallValue) isValue() {}

// FunctionCallValue represents a function call (e.g., write_file("path", data), print("msg"))
type FunctionCallValue struct {
	Function  string  // The function name
	Arguments []Value // The arguments to the function
}

func (f FunctionCallValue) isValue() {}

// ComparisonValue represents a comparison expression (e.g., env("DEBUG") == "true")
type ComparisonValue struct {
	Left     Value  // Left operand
	Operator string // "==", "!=", "<", ">", "<=", ">="
	Right    Value  // Right operand
}

func (c ComparisonValue) isValue() {}

// BranchValue represents a branch control flow construct
// e.g., branch step("classify").output.type { "bug" => step "fix" { ... } }
type BranchValue struct {
	Condition Value                        // The expression to branch on
	Cases     map[string]NestedEntityValue // Map of case value to entity
}

func (b BranchValue) isValue() {}

// LoopValue represents a loop control flow construct
// e.g., loop max: 3 { ... }
type LoopValue struct {
	MaxIterations  int                 // Maximum iterations (from max: N)
	Body           []NestedEntityValue // The entities inside the loop
	BreakCondition Value               // Optional break_if condition
}

func (l LoopValue) isValue() {}

// Entity represents a LangSpace entity, which is the fundamental building block
// of the language. Each entity has a type and a set of properties that define
// its behavior and characteristics.
type Entity interface {
	// Type returns the entity's type identifier (e.g., "file", "agent")
	Type() string

	// Name returns the entity's name/identifier
	Name() string

	// Properties returns the entity's property map
	Properties() map[string]Value

	// GetProperty returns a property value by key
	GetProperty(key string) (Value, bool)

	// SetProperty sets a property value
	SetProperty(key string, value Value)

	// GetMetadata returns the value for a metadata key, and whether it exists
	GetMetadata(key string) (string, bool)

	// SetMetadata sets a metadata key-value pair on the entity
	SetMetadata(key, value string)

	// AllMetadata returns a copy of all metadata key-value pairs
	AllMetadata() map[string]string

	// Line returns the source line where this entity was defined
	Line() int

	// Column returns the source column where this entity was defined
	Column() int

	// SetLocation sets the source location
	SetLocation(line, column int)
}

// BaseEntity provides a base implementation of Entity with common functionality
type BaseEntity struct {
	entityType string
	name       string
	properties map[string]Value
	metadata   map[string]string
	line       int
	column     int
}

// NewBaseEntity creates a new BaseEntity
func NewBaseEntity(entityType, name string) *BaseEntity {
	return &BaseEntity{
		entityType: entityType,
		name:       name,
		properties: make(map[string]Value),
		metadata:   make(map[string]string),
	}
}

func (e *BaseEntity) Type() string { return e.entityType }
func (e *BaseEntity) Name() string { return e.name }

// Properties returns a copy of the entity's property map to prevent external mutation.
func (e *BaseEntity) Properties() map[string]Value {
	result := make(map[string]Value, len(e.properties))
	for k, v := range e.properties {
		result[k] = v
	}
	return result
}
func (e *BaseEntity) Line() int                         { return e.line }
func (e *BaseEntity) Column() int                       { return e.column }
func (e *BaseEntity) SetLocation(line, column int)      { e.line = line; e.column = column }
func (e *BaseEntity) SetProperty(key string, val Value) { e.properties[key] = val }

func (e *BaseEntity) GetProperty(key string) (Value, bool) {
	v, ok := e.properties[key]
	return v, ok
}

func (e *BaseEntity) GetMetadata(key string) (string, bool) {
	if e.metadata == nil {
		return "", false
	}
	val, ok := e.metadata[key]
	return val, ok
}

func (e *BaseEntity) SetMetadata(key, value string) {
	if e.metadata == nil {
		e.metadata = make(map[string]string)
	}
	e.metadata[key] = value
}

func (e *BaseEntity) AllMetadata() map[string]string {
	result := make(map[string]string)
	for k, v := range e.metadata {
		result[k] = v
	}
	return result
}

// FileEntity represents a file entity in LangSpace
type FileEntity struct {
	*BaseEntity
}

// NewFileEntity creates a new file entity
func NewFileEntity(name string) *FileEntity {
	return &FileEntity{BaseEntity: NewBaseEntity("file", name)}
}

// AgentEntity represents an agent entity in LangSpace
type AgentEntity struct {
	*BaseEntity
}

// NewAgentEntity creates a new agent entity
func NewAgentEntity(name string) *AgentEntity {
	return &AgentEntity{BaseEntity: NewBaseEntity("agent", name)}
}

// ToolEntity represents a tool entity in LangSpace
type ToolEntity struct {
	*BaseEntity
}

// NewToolEntity creates a new tool entity
func NewToolEntity(name string) *ToolEntity {
	return &ToolEntity{BaseEntity: NewBaseEntity("tool", name)}
}

// IntentEntity represents an intent entity in LangSpace
type IntentEntity struct {
	*BaseEntity
}

// NewIntentEntity creates a new intent entity
func NewIntentEntity(name string) *IntentEntity {
	return &IntentEntity{BaseEntity: NewBaseEntity("intent", name)}
}

// PipelineEntity represents a pipeline entity in LangSpace
type PipelineEntity struct {
	*BaseEntity
	Steps []*StepEntity
}

// NewPipelineEntity creates a new pipeline entity
func NewPipelineEntity(name string) *PipelineEntity {
	return &PipelineEntity{
		BaseEntity: NewBaseEntity("pipeline", name),
		Steps:      make([]*StepEntity, 0),
	}
}

// AddStep adds a step to the pipeline
func (p *PipelineEntity) AddStep(step *StepEntity) {
	p.Steps = append(p.Steps, step)
}

// ParallelEntity represents a parallel execution block
type ParallelEntity struct {
	*BaseEntity
	Steps []*StepEntity
}

// NewParallelEntity creates a new parallel entity
func NewParallelEntity(name string) *ParallelEntity {
	return &ParallelEntity{
		BaseEntity: NewBaseEntity("parallel", name),
		Steps:      make([]*StepEntity, 0),
	}
}

// AddStep adds a step to the parallel block
func (p *ParallelEntity) AddStep(step *StepEntity) {
	p.Steps = append(p.Steps, step)
}

// StepEntity represents a step within a pipeline
type StepEntity struct {
	*BaseEntity
}

// NewStepEntity creates a new step entity
func NewStepEntity(name string) *StepEntity {
	return &StepEntity{BaseEntity: NewBaseEntity("step", name)}
}

// TriggerEntity represents a trigger entity in LangSpace
type TriggerEntity struct {
	*BaseEntity
}

// NewTriggerEntity creates a new trigger entity
func NewTriggerEntity(name string) *TriggerEntity {
	return &TriggerEntity{BaseEntity: NewBaseEntity("trigger", name)}
}

// ConfigEntity represents a config block in LangSpace
type ConfigEntity struct {
	*BaseEntity
}

// NewConfigEntity creates a new config entity
func NewConfigEntity() *ConfigEntity {
	return &ConfigEntity{BaseEntity: NewBaseEntity("config", "")}
}

// MCPEntity represents an MCP server connection
type MCPEntity struct {
	*BaseEntity
}

// NewMCPEntity creates a new MCP entity
func NewMCPEntity(name string) *MCPEntity {
	return &MCPEntity{BaseEntity: NewBaseEntity("mcp", name)}
}

// ScriptEntity represents a script entity in LangSpace.
// Scripts enable code-first agent actions — a more efficient alternative to
// multiple tool calls. Instead of loading full data into the context window
// through repeated tool invocations, agents write executable code that performs
// complex operations in a single execution, returning only the results.
//
// Key properties:
//   - language: The programming language (python, javascript, bash, sql)
//   - runtime: The runtime/interpreter to use (python3, node, bash, postgresql)
//   - code: The script source code (inline or file reference)
//   - parameters: Input parameters passed to the script
//   - capabilities: What the script can access (database, filesystem, network)
//   - timeout: Maximum execution time
//   - limits: Resource constraints (memory, cpu)
//   - sandbox: Security restrictions (allowed_modules, network access)
type ScriptEntity struct {
	*BaseEntity
}

// NewScriptEntity creates a new script entity
func NewScriptEntity(name string) *ScriptEntity {
	return &ScriptEntity{BaseEntity: NewBaseEntity("script", name)}
}

// MicrostepEntity represents an atomic step in an MDAP (Massively Decomposed
// Agentic Process). Unlike regular steps, microsteps are designed for
// million-step tasks with minimal context per step.
//
// Key properties:
//   - context: Minimal context for this step (state, strategy, previous_action)
//   - output_schema: Expected output format for validation
//   - red_flags: Conditions that trigger rejection sampling
//   - depends_on: Explicit dependencies for DAG ordering
//
// MDAP ensures reliability through:
//   - Maximal decomposition (one atomic action per step)
//   - First-to-ahead-by-k voting for error correction
//   - Red-flagging to reject suspicious outputs
type MicrostepEntity struct {
	*BaseEntity
}

// NewMicrostepEntity creates a new microstep entity
func NewMicrostepEntity(name string) *MicrostepEntity {
	return &MicrostepEntity{BaseEntity: NewBaseEntity("microstep", name)}
}

// MDAPConfigEntity configures MDAP execution parameters for a pipeline.
// This enables the MAKER-style voting and rejection sampling mechanisms.
//
// Key properties:
//   - voting_strategy: "first-to-ahead-by-k" (default) or "majority"
//   - k: Vote margin required for consensus (default: 3)
//   - parallel_samples: Number of parallel samples per round (default: k)
//   - temperature_first: Temperature for first sample (default: 0.0)
//   - temperature_subsequent: Temperature for subsequent samples (default: 0.1)
//   - max_output_tokens: Red-flag threshold for response length
//   - require_format: Whether to reject format errors (default: true)
//   - checkpoint_interval: Steps between checkpoints (default: 1000)
type MDAPConfigEntity struct {
	*BaseEntity
}

// NewMDAPConfigEntity creates a new MDAP configuration entity
func NewMDAPConfigEntity() *MDAPConfigEntity {
	return &MDAPConfigEntity{BaseEntity: NewBaseEntity("mdap_config", "")}
}

// MDAPPipelineEntity represents a pipeline designed for MDAP execution.
// It extends PipelineEntity with MDAP-specific configuration and step handling.
//
// Key properties:
//   - mode: "mdap" to enable MDAP execution
//   - mdap_config: Configuration for voting, sampling, and red-flagging
//   - strategy: Fixed strategy provided to all microsteps
//   - state_schema: Schema for the state passed between microsteps
//   - generate_steps: Optional function to dynamically generate steps
type MDAPPipelineEntity struct {
	*BaseEntity
	Microsteps []*MicrostepEntity
	Config     *MDAPConfigEntity
}

// NewMDAPPipelineEntity creates a new MDAP pipeline entity
func NewMDAPPipelineEntity(name string) *MDAPPipelineEntity {
	return &MDAPPipelineEntity{
		BaseEntity: NewBaseEntity("mdap_pipeline", name),
		Microsteps: make([]*MicrostepEntity, 0),
	}
}

// AddMicrostep adds a microstep to the MDAP pipeline
func (p *MDAPPipelineEntity) AddMicrostep(step *MicrostepEntity) {
	p.Microsteps = append(p.Microsteps, step)
}

// EntityFactory is a function that creates a new entity of a specific type
type EntityFactory func(name string) Entity

// entityRegistry holds registered entity types and their factories
var entityRegistry = map[string]EntityFactory{
	"file":          func(name string) Entity { return NewFileEntity(name) },
	"agent":         func(name string) Entity { return NewAgentEntity(name) },
	"tool":          func(name string) Entity { return NewToolEntity(name) },
	"intent":        func(name string) Entity { return NewIntentEntity(name) },
	"pipeline":      func(name string) Entity { return NewPipelineEntity(name) },
	"parallel":      func(name string) Entity { return NewParallelEntity(name) },
	"step":          func(name string) Entity { return NewStepEntity(name) },
	"trigger":       func(name string) Entity { return NewTriggerEntity(name) },
	"config":        func(name string) Entity { return NewConfigEntity() },
	"mcp":           func(name string) Entity { return NewMCPEntity(name) },
	"script":        func(name string) Entity { return NewScriptEntity(name) },
	"env":           func(name string) Entity { return NewBaseEntity("env", name) },
	"microstep":     func(name string) Entity { return NewMicrostepEntity(name) },
	"mdap_config":   func(name string) Entity { return NewMDAPConfigEntity() },
	"mdap_pipeline": func(name string) Entity { return NewMDAPPipelineEntity(name) },
}

// RegisterEntityType registers a new entity type with its factory function.
// This allows extending the AST with custom entity types without modifying
// the core package, supporting the Open/Closed Principle.
//
// Example:
//
//	ast.RegisterEntityType("custom", func(name string) ast.Entity {
//	    return NewCustomEntity(name)
//	})
func RegisterEntityType(entityType string, factory EntityFactory) {
	entityRegistry[entityType] = factory
}

// RegisteredEntityTypes returns a list of all registered entity type names.
func RegisteredEntityTypes() []string {
	types := make([]string, 0, len(entityRegistry))
	for t := range entityRegistry {
		types = append(types, t)
	}
	return types
}

// NewEntity creates a new Entity based on the provided type identifier.
// It uses the entity registry to support extensibility.
func NewEntity(entityType string, name string) (Entity, error) {
	factory, ok := entityRegistry[entityType]
	if !ok {
		return nil, fmt.Errorf("unknown entity type: %s", entityType)
	}
	return factory(name), nil
}
