package validator

import (
	"fmt"

	"github.com/shellkjell/langspace/pkg/ast"
)

// Package validator provides entity validation functionality for LangSpace.
// It ensures that entities conform to their type-specific rules and constraints,
// providing detailed error messages for validation failures.

// EntityValidator is the interface that defines entity validation behavior.
// This interface can be implemented by custom validators.
type EntityValidator interface {
	ValidateEntity(entity ast.Entity) error
}

// ValidationFunc is a function that validates a specific entity type.
type ValidationFunc func(entity ast.Entity) error

// Validator performs entity validation according to LangSpace's type system rules.
// It can be extended with custom validation rules and error formatting.
// Validator implements the EntityValidator interface.
type Validator struct {
	// customValidators holds additional validators registered at runtime
	customValidators map[string]ValidationFunc
}

// New creates a new Validator instance configured with default validation rules.
// In the future, this constructor could accept options for customizing validation
// behavior.
//
// Returns:
//   - *Validator: A new validator instance ready to validate entities
func New() *Validator {
	return &Validator{
		customValidators: make(map[string]ValidationFunc),
	}
}

// RegisterValidator registers a custom validation function for a specific entity type.
// This supports the Open/Closed Principle by allowing extensions without modification.
//
// Example:
//
//	v := validator.New()
//	v.RegisterValidator("custom", func(e ast.Entity) error {
//	    // custom validation logic
//	    return nil
//	})
func (v *Validator) RegisterValidator(entityType string, fn ValidationFunc) {
	v.customValidators[entityType] = fn
}

// ValidateEntity validates an entity according to its type-specific rules.
// This is the main entry point for entity validation.
//
// Parameters:
//   - entity: The entity to validate
//
// Returns:
//   - error: Detailed validation error if the entity is invalid
//
// Validation includes:
//   - Entity type verification
//   - Required property checks
//   - Property value validation
//   - Cross-property consistency checks
func (v *Validator) ValidateEntity(entity ast.Entity) error {
	if entity == nil {
		return fmt.Errorf("entity cannot be nil")
	}

	// Check for custom validator first
	if fn, ok := v.customValidators[entity.Type()]; ok {
		return fn(entity)
	}

	// Fall back to built-in validators
	switch entity.Type() {
	case "file":
		return v.validateFileEntity(entity)
	case "agent":
		return v.validateAgentEntity(entity)
	case "tool":
		return v.validateToolEntity(entity)
	case "intent":
		return v.validateIntentEntity(entity)
	case "pipeline":
		return v.validatePipelineEntity(entity)
	case "step":
		return v.validateStepEntity(entity)
	case "trigger":
		return v.validateTriggerEntity(entity)
	case "config":
		return v.validateConfigEntity(entity)
	case "mcp":
		return v.validateMCPEntity(entity)
	case "script":
		return v.validateScriptEntity(entity)
	case "microstep":
		return v.validateMicrostepEntity(entity)
	case "mdap_config":
		return v.validateMDAPConfigEntity(entity)
	case "mdap_pipeline":
		return v.validateMDAPPipelineEntity(entity)
	default:
		return fmt.Errorf("unknown entity type: %s", entity.Type())
	}
}

// validateFileEntity performs file-specific validation rules.
func (v *Validator) validateFileEntity(entity ast.Entity) error {
	// File entities should have a name
	if entity.Name() == "" {
		return fmt.Errorf("file entity must have a name")
	}

	// Check for either path or contents property
	_, hasPath := entity.GetProperty("path")
	_, hasContents := entity.GetProperty("contents")

	if !hasPath && !hasContents {
		return fmt.Errorf("file entity must have either 'path' or 'contents' property")
	}

	return nil
}

// validateAgentEntity validates an agent entity
func (v *Validator) validateAgentEntity(entity ast.Entity) error {
	// Agent entities should have a name
	if entity.Name() == "" {
		return fmt.Errorf("agent entity must have a name")
	}

	// Check for required model property
	_, hasModel := entity.GetProperty("model")
	if !hasModel {
		return fmt.Errorf("agent entity must have 'model' property")
	}

	return nil
}

// validateToolEntity validates a tool entity
func (v *Validator) validateToolEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("tool entity must have a name")
	}

	// Tool should have either command or function property
	_, hasCommand := entity.GetProperty("command")
	_, hasFunction := entity.GetProperty("function")

	if !hasCommand && !hasFunction {
		return fmt.Errorf("tool entity must have either 'command' or 'function' property")
	}

	return nil
}

// validateIntentEntity validates an intent entity
func (v *Validator) validateIntentEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("intent entity must have a name")
	}

	// Intent must have a 'use' property referencing an agent
	_, hasUse := entity.GetProperty("use")
	if !hasUse {
		return fmt.Errorf("intent entity must have 'use' property referencing an agent")
	}

	return nil
}

// validatePipelineEntity validates a pipeline entity
func (v *Validator) validatePipelineEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("pipeline entity must have a name")
	}

	return nil
}

// validateStepEntity validates a step entity
func (v *Validator) validateStepEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("step entity must have a name")
	}

	// Step must have a 'use' property
	_, hasUse := entity.GetProperty("use")
	if !hasUse {
		return fmt.Errorf("step entity must have 'use' property")
	}

	return nil
}

// validateTriggerEntity validates a trigger entity
func (v *Validator) validateTriggerEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("trigger entity must have a name")
	}

	// Trigger should have event or schedule property
	_, hasEvent := entity.GetProperty("event")
	_, hasSchedule := entity.GetProperty("schedule")

	if !hasEvent && !hasSchedule {
		return fmt.Errorf("trigger entity must have 'event' or 'schedule' property")
	}

	return nil
}

// validateConfigEntity validates a config entity
func (v *Validator) validateConfigEntity(entity ast.Entity) error {
	// Config entities don't require a name
	// They should have at least one property
	if len(entity.Properties()) == 0 {
		return fmt.Errorf("config entity must have at least one property")
	}

	return nil
}

// validateMCPEntity validates an MCP entity
func (v *Validator) validateMCPEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("mcp entity must have a name")
	}

	// MCP entities should have command property
	_, hasCommand := entity.GetProperty("command")
	if !hasCommand {
		return fmt.Errorf("mcp entity must have 'command' property")
	}

	return nil
}

// validateScriptEntity validates a script entity
func (v *Validator) validateScriptEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("script entity must have a name")
	}

	// Script entities must have a language property
	_, hasLanguage := entity.GetProperty("language")
	if !hasLanguage {
		return fmt.Errorf("script entity must have 'language' property")
	}

	// Script entities should have either code or a file reference
	_, hasCode := entity.GetProperty("code")
	_, hasPath := entity.GetProperty("path")
	if !hasCode && !hasPath {
		return fmt.Errorf("script entity must have 'code' or 'path' property")
	}

	return nil
}

// validateMicrostepEntity validates a microstep entity for MDAP pipelines.
// Microsteps require minimal context and output schema for reliable atomic execution.
func (v *Validator) validateMicrostepEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("microstep entity must have a name")
	}

	// Microstep must have an agent to use
	_, hasUse := entity.GetProperty("use")
	if !hasUse {
		return fmt.Errorf("microstep entity must have 'use' property referencing an agent")
	}

	return nil
}

// validateMDAPConfigEntity validates an MDAP configuration entity.
// MDAP config controls voting, sampling, and red-flagging behavior.
func (v *Validator) validateMDAPConfigEntity(entity ast.Entity) error {
	// Config entities don't require a name

	// Validate k if provided (must be >= 1)
	if kProp, hasK := entity.GetProperty("k"); hasK {
		if kVal, ok := kProp.(ast.NumberValue); ok {
			if kVal.Value < 1 {
				return fmt.Errorf("mdap_config 'k' must be >= 1, got %v", kVal.Value)
			}
		}
	}

	// Validate voting_strategy if provided
	if strategyProp, hasStrategy := entity.GetProperty("voting_strategy"); hasStrategy {
		if strategyVal, ok := strategyProp.(ast.StringValue); ok {
			switch strategyVal.Value {
			case "first-to-ahead-by-k", "majority":
				// Valid strategies
			default:
				return fmt.Errorf("mdap_config 'voting_strategy' must be 'first-to-ahead-by-k' or 'majority', got %q", strategyVal.Value)
			}
		}
	}

	return nil
}

// validateMDAPPipelineEntity validates an MDAP pipeline entity.
// MDAP pipelines coordinate massive parallel step execution with voting.
func (v *Validator) validateMDAPPipelineEntity(entity ast.Entity) error {
	if entity.Name() == "" {
		return fmt.Errorf("mdap_pipeline entity must have a name")
	}

	// MDAP pipeline should have a strategy for agents to follow
	_, hasStrategy := entity.GetProperty("strategy")
	if !hasStrategy {
		return fmt.Errorf("mdap_pipeline entity should have 'strategy' property defining the solving approach")
	}

	return nil
}
