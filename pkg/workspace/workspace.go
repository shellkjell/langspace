// Package workspace provides a virtual workspace for managing LangSpace entities.
// It handles entity storage, relationship tracking, lifecycle hooks, and versioning.
package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/shellkjell/langspace/pkg/ast"
	"github.com/shellkjell/langspace/pkg/slices"
	"github.com/shellkjell/langspace/pkg/validator"
)

// HookType defines the type of lifecycle hook
type HookType string

const (
	// HookBeforeAdd is called before an entity is added
	HookBeforeAdd HookType = "before_add"
	// HookAfterAdd is called after an entity is added
	HookAfterAdd HookType = "after_add"
	// HookBeforeRemove is called before an entity is removed
	HookBeforeRemove HookType = "before_remove"
	// HookAfterRemove is called after an entity is removed
	HookAfterRemove HookType = "after_remove"
	// HookBeforeUpdate is called before an entity is updated
	HookBeforeUpdate HookType = "before_update"
	// HookAfterUpdate is called after an entity is updated
	HookAfterUpdate HookType = "after_update"
)

// EntityHook is a function called during entity lifecycle events
// Return an error to cancel the operation (for "before" hooks)
type EntityHook func(entity ast.Entity) error

// RelationType defines the type of relationship between entities
type RelationType string

const (
	// RelationTypeAssigned indicates an agent is assigned to work on a file or task
	RelationTypeAssigned RelationType = "assigned"
	// RelationTypeDepends indicates a task depends on another entity
	RelationTypeDepends RelationType = "depends"
	// RelationTypeProduces indicates an entity produces another entity
	RelationTypeProduces RelationType = "produces"
	// RelationTypeConsumes indicates an entity consumes/reads another entity
	RelationTypeConsumes RelationType = "consumes"
)

// Relationship represents a connection between two entities
type Relationship struct {
	SourceType string       // Type of the source entity (file, agent, task)
	SourceName string       // Name/identifier of the source entity
	TargetType string       // Type of the target entity
	TargetName string       // Name/identifier of the target entity
	Type       RelationType // Type of relationship
}

// EventType defines the type of workspace event
type EventType string

const (
	// EventEntityAdded is emitted when an entity is added
	EventEntityAdded EventType = "entity_added"
	// EventEntityRemoved is emitted when an entity is removed
	EventEntityRemoved EventType = "entity_removed"
	// EventEntityUpdated is emitted when an entity is updated
	EventEntityUpdated EventType = "entity_updated"
	// EventRelationshipAdded is emitted when a relationship is added
	EventRelationshipAdded EventType = "relationship_added"
	// EventRelationshipRemoved is emitted when a relationship is removed
	EventRelationshipRemoved EventType = "relationship_removed"
	// EventWorkspaceCleared is emitted when the workspace is cleared
	EventWorkspaceCleared EventType = "workspace_cleared"
)

// Event represents a workspace event
type Event struct {
	Type         EventType     // The type of event
	Entity       ast.Entity    // The entity involved (for entity events)
	Relationship *Relationship // The relationship involved (for relationship events)
}

// EventHandler is a function that handles workspace events
type EventHandler func(event Event)

// EntityVersion represents a historical version of an entity
type EntityVersion struct {
	Version   int        // Version number (starts at 1)
	Entity    ast.Entity // The entity at this version
	Timestamp int64      // Unix timestamp when this version was created
}

// entityKey creates a unique key for an entity based on type and name
func entityKey(entityType, entityName string) string {
	return entityType + ":" + entityName
}

// Config holds workspace configuration options.
type Config struct {
	// MaxEntities limits the maximum number of entities (0 = unlimited)
	MaxEntities int `json:"max_entities,omitempty"`
	// MaxRelationships limits the maximum number of relationships (0 = unlimited)
	MaxRelationships int `json:"max_relationships,omitempty"`
	// MaxVersions limits the number of versions kept per entity (0 = unlimited)
	MaxVersions int `json:"max_versions,omitempty"`
	// AllowDuplicateNames allows entities of the same type with duplicate names
	AllowDuplicateNames bool `json:"allow_duplicate_names,omitempty"`
	// StrictValidation requires all entities to pass validation
	StrictValidation bool `json:"strict_validation,omitempty"`
	// EnableVersioning enables entity version tracking
	EnableVersioning bool `json:"enable_versioning,omitempty"`
	// AllowedEntityTypes restricts which entity types can be added (empty = all allowed)
	AllowedEntityTypes []string `json:"allowed_entity_types,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxEntities:         0,     // unlimited
		MaxRelationships:    0,     // unlimited
		MaxVersions:         100,   // keep last 100 versions
		AllowDuplicateNames: false, // no duplicates
		StrictValidation:    true,  // require validation
		EnableVersioning:    false, // disabled by default
		AllowedEntityTypes:  nil,   // all types allowed
	}
}

// EntityValidatorFunc is a function that validates an entity.
// It returns an error if the entity is invalid.
type EntityValidatorFunc func(entity ast.Entity) error

// Workspace represents a virtual workspace containing entities
type Workspace struct {
	entities          []ast.Entity
	relationships     []Relationship
	hooks             map[HookType][]EntityHook
	eventHandlers     []EventHandler
	entityVersions    map[string][]EntityVersion // Maps entity key to version history
	versioningEnabled bool
	config            *Config
	customValidators  map[string][]EntityValidatorFunc // Maps entity type to validators
	mu                sync.RWMutex
	validator         validator.EntityValidator
}

// New creates a new Workspace instance
func New() *Workspace {
	return &Workspace{
		entities:          make([]ast.Entity, 0),
		relationships:     make([]Relationship, 0),
		hooks:             make(map[HookType][]EntityHook),
		eventHandlers:     make([]EventHandler, 0),
		entityVersions:    make(map[string][]EntityVersion),
		versioningEnabled: false,
		config:            DefaultConfig(),
		customValidators:  make(map[string][]EntityValidatorFunc),
	}
}

// WorkspaceStats contains statistics about the workspace
type WorkspaceStats struct {
	TotalEntities        int
	FileEntities         int
	AgentEntities        int
	ToolEntities         int
	IntentEntities       int
	PipelineEntities     int
	ScriptEntities       int
	MDAPPipelineEntities int
	TotalRelationships   int
	TotalHooks           int
	HasValidator         bool
}

// Stat returns statistics about the workspace
func (w *Workspace) Stat() WorkspaceStats {
	w.mu.RLock()
	defer w.mu.RUnlock()

	totalHooks := 0
	for _, hooks := range w.hooks {
		totalHooks += len(hooks)
	}

	stats := WorkspaceStats{
		TotalEntities:      len(w.entities),
		TotalRelationships: len(w.relationships),
		TotalHooks:         totalHooks,
		HasValidator:       w.validator != nil,
	}

	// Count entities by type
	for _, entity := range w.entities {
		switch entity.Type() {
		case "file":
			stats.FileEntities++
		case "agent":
			stats.AgentEntities++
		case "tool":
			stats.ToolEntities++
		case "intent":
			stats.IntentEntities++
		case "pipeline":
			stats.PipelineEntities++
		case "script":
			stats.ScriptEntities++
		case "mdap_pipeline":
			stats.MDAPPipelineEntities++
		}
	}

	return stats
}

// WithValidator sets a validator for the workspace
func (w *Workspace) WithValidator(v validator.EntityValidator) *Workspace {
	w.validator = v
	return w
}

// RegisterEntityValidator registers a custom validator for a specific entity type.
// Multiple validators can be registered for the same type; they will all be executed.
// Validators are run during AddEntity, UpdateEntity, and UpsertEntity operations.
func (w *Workspace) RegisterEntityValidator(entityType string, validator EntityValidatorFunc) *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.customValidators == nil {
		w.customValidators = make(map[string][]EntityValidatorFunc)
	}
	w.customValidators[entityType] = append(w.customValidators[entityType], validator)
	return w
}

// RegisterGlobalValidator registers a validator that applies to all entity types.
// This is registered under the special type "*".
func (w *Workspace) RegisterGlobalValidator(validator EntityValidatorFunc) *Workspace {
	return w.RegisterEntityValidator("*", validator)
}

// runCustomValidators runs all custom validators for an entity.
// Returns the first error encountered, or nil if all pass.
func (w *Workspace) runCustomValidators(entity ast.Entity) error {
	// Run global validators first
	globalValidators := w.customValidators["*"]
	for _, v := range globalValidators {
		if err := v(entity); err != nil {
			return err
		}
	}

	// Run type-specific validators
	typeValidators := w.customValidators[entity.Type()]
	for _, v := range typeValidators {
		if err := v(entity); err != nil {
			return err
		}
	}

	return nil
}

// ClearValidators removes all custom validators.
func (w *Workspace) ClearValidators() *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.customValidators = make(map[string][]EntityValidatorFunc)
	return w
}

// ClearValidatorsForType removes custom validators for a specific entity type.
func (w *Workspace) ClearValidatorsForType(entityType string) *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.customValidators, entityType)
	return w
}

// WithConfig applies a configuration to the workspace.
// This should be called before adding entities.
func (w *Workspace) WithConfig(cfg *Config) *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()
	if cfg != nil {
		w.config = cfg
		w.versioningEnabled = cfg.EnableVersioning
	}
	return w
}

// GetConfig returns a copy of the current workspace configuration.
func (w *Workspace) GetConfig() Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.config == nil {
		return *DefaultConfig()
	}
	// Return a copy
	cfg := *w.config
	if w.config.AllowedEntityTypes != nil {
		cfg.AllowedEntityTypes = make([]string, len(w.config.AllowedEntityTypes))
		copy(cfg.AllowedEntityTypes, w.config.AllowedEntityTypes)
	}
	return cfg
}

// WithVersioning enables entity version tracking.
// When enabled, the workspace maintains a history of all entity versions,
// allowing you to retrieve previous states of entities.
func (w *Workspace) WithVersioning() *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.versioningEnabled = true
	w.config.EnableVersioning = true
	return w
}

// recordVersion stores a new version of an entity if versioning is enabled.
// Must be called with lock held.
func (w *Workspace) recordVersion(entity ast.Entity) {
	if !w.versioningEnabled {
		return
	}
	key := entityKey(entity.Type(), entity.Name())
	versions := w.entityVersions[key]
	newVersion := EntityVersion{
		Version:   len(versions) + 1,
		Entity:    entity,
		Timestamp: time.Now().Unix(),
	}
	versions = append(versions, newVersion)

	// Trim versions if MaxVersions is set
	if w.config != nil && w.config.MaxVersions > 0 && len(versions) > w.config.MaxVersions {
		// Keep only the most recent MaxVersions versions
		versions = versions[len(versions)-w.config.MaxVersions:]
		// Renumber versions
		for i := range versions {
			versions[i].Version = i + 1
		}
	}

	w.entityVersions[key] = versions
}

// GetEntityVersion returns a specific version of an entity.
// Version numbers start at 1. Returns nil if the version doesn't exist.
func (w *Workspace) GetEntityVersion(entityType, entityName string, version int) (ast.Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	key := entityKey(entityType, entityName)
	versions, ok := w.entityVersions[key]
	if !ok || version < 1 || version > len(versions) {
		return nil, false
	}
	return versions[version-1].Entity, true
}

// GetEntityVersionCount returns the number of versions for an entity.
func (w *Workspace) GetEntityVersionCount(entityType, entityName string) int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	key := entityKey(entityType, entityName)
	return len(w.entityVersions[key])
}

// GetEntityHistory returns all versions of an entity in chronological order.
func (w *Workspace) GetEntityHistory(entityType, entityName string) []EntityVersion {
	w.mu.RLock()
	defer w.mu.RUnlock()

	key := entityKey(entityType, entityName)
	versions := w.entityVersions[key]
	if versions == nil {
		return nil
	}
	// Return a copy
	result := make([]EntityVersion, len(versions))
	copy(result, versions)
	return result
}

// OnEntityEvent registers a hook for a specific entity lifecycle event
func (w *Workspace) OnEntityEvent(hookType HookType, hook EntityHook) *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.hooks[hookType] = append(w.hooks[hookType], hook)
	return w
}

// OnEvent registers an event handler for workspace events.
// Unlike hooks, event handlers cannot block operations and are called
// after the operation is complete.
func (w *Workspace) OnEvent(handler EventHandler) *Workspace {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.eventHandlers = append(w.eventHandlers, handler)
	return w
}

// emit sends an event to all registered event handlers.
// This is called after operations complete successfully.
func (w *Workspace) emit(event Event) {
	for _, handler := range w.eventHandlers {
		handler(event)
	}
}

// runHooks executes all hooks of a given type for an entity
func (w *Workspace) runHooks(hookType HookType, entity ast.Entity) error {
	hooks := w.hooks[hookType]
	for _, hook := range hooks {
		if err := hook(entity); err != nil {
			return fmt.Errorf("hook %s failed: %w", hookType, err)
		}
	}
	return nil
}

// AddEntity adds an entity to the workspace
func (w *Workspace) AddEntity(entity ast.Entity) error {
	if entity == nil {
		return fmt.Errorf("cannot add nil entity")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check configuration constraints
	if err := w.checkAddConstraints(entity); err != nil {
		return err
	}

	// Run before-add hooks
	if err := w.runHooks(HookBeforeAdd, entity); err != nil {
		return err
	}

	if w.validator != nil {
		if err := w.validator.ValidateEntity(entity); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Run custom validators
	if err := w.runCustomValidators(entity); err != nil {
		return fmt.Errorf("custom validation failed: %w", err)
	}

	w.entities = append(w.entities, entity)

	// Record version if versioning is enabled
	w.recordVersion(entity)

	// Run after-add hooks (errors are logged but don't fail the operation)
	_ = w.runHooks(HookAfterAdd, entity)

	// Emit entity added event
	w.emit(Event{Type: EventEntityAdded, Entity: entity})

	return nil
}

// checkAddConstraints checks if adding an entity violates configuration constraints.
// Must be called with lock held.
func (w *Workspace) checkAddConstraints(entity ast.Entity) error {
	if w.config == nil {
		return nil
	}

	// Check max entities limit
	if w.config.MaxEntities > 0 && len(w.entities) >= w.config.MaxEntities {
		return fmt.Errorf("maximum entity limit reached (%d)", w.config.MaxEntities)
	}

	// Check allowed entity types
	if len(w.config.AllowedEntityTypes) > 0 {
		allowed := false
		for _, t := range w.config.AllowedEntityTypes {
			if t == entity.Type() {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("entity type %q is not allowed", entity.Type())
		}
	}

	// Check for duplicate names (unless explicitly allowed)
	if !w.config.AllowDuplicateNames {
		for _, e := range w.entities {
			if e.Type() == entity.Type() && e.Name() == entity.Name() {
				return fmt.Errorf("entity %s/%s already exists", entity.Type(), entity.Name())
			}
		}
	}

	return nil
}

// GetEntities returns all entities in the workspace
func (w *Workspace) GetEntities() []ast.Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]ast.Entity, len(w.entities))
	copy(result, w.entities)
	return result
}

// GetEntitiesByType returns all entities of a specific type
func (w *Workspace) GetEntitiesByType(entityType string) []ast.Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return slices.Filter(w.entities, func(e ast.Entity) bool {
		return e.Type() == entityType
	})
}

// GetEntityByName returns an entity by type and name
func (w *Workspace) GetEntityByName(entityType, entityName string) (ast.Entity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return slices.Find(w.entities, func(e ast.Entity) bool {
		return e.Type() == entityType && e.Name() == entityName
	})
}

// RemoveEntity removes an entity from the workspace by type and name
func (w *Workspace) RemoveEntity(entityType, entityName string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, entity := range w.entities {
		if entity.Type() == entityType && entity.Name() == entityName {
			// Run before-remove hooks
			if err := w.runHooks(HookBeforeRemove, entity); err != nil {
				return err
			}

			// Remove the entity
			w.entities = append(w.entities[:i], w.entities[i+1:]...)

			// Remove any relationships involving this entity
			w.removeRelationshipsForEntity(entityType, entityName)

			// Run after-remove hooks
			_ = w.runHooks(HookAfterRemove, entity)

			// Emit entity removed event
			w.emit(Event{Type: EventEntityRemoved, Entity: entity})

			return nil
		}
	}

	return fmt.Errorf("entity not found: %s %q", entityType, entityName)
}

// UpdateEntity replaces an existing entity with a new version.
// The entity must already exist in the workspace.
// Hooks are called before and after the update.
func (w *Workspace) UpdateEntity(entity ast.Entity) error {
	if entity == nil {
		return fmt.Errorf("cannot update with nil entity")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Find the existing entity
	idx := slices.FindIndex(w.entities, func(e ast.Entity) bool {
		return e.Type() == entity.Type() && e.Name() == entity.Name()
	})
	if idx == -1 {
		return fmt.Errorf("entity not found: %s %q", entity.Type(), entity.Name())
	}

	// Run before-update hooks
	if err := w.runHooks(HookBeforeUpdate, entity); err != nil {
		return err
	}

	// Validate the new entity if validator is set
	if w.validator != nil {
		if err := w.validator.ValidateEntity(entity); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Run custom validators
	if err := w.runCustomValidators(entity); err != nil {
		return fmt.Errorf("custom validation failed: %w", err)
	}

	// Replace the entity
	w.entities[idx] = entity

	// Record version if versioning is enabled
	w.recordVersion(entity)

	// Run after-update hooks
	_ = w.runHooks(HookAfterUpdate, entity)

	// Emit entity updated event
	w.emit(Event{Type: EventEntityUpdated, Entity: entity})

	return nil
}

// UpsertEntity adds an entity if it doesn't exist, or updates it if it does.
// This is a convenience method combining AddEntity and UpdateEntity behavior.
func (w *Workspace) UpsertEntity(entity ast.Entity) error {
	if entity == nil {
		return fmt.Errorf("cannot upsert nil entity")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if entity already exists
	idx := slices.FindIndex(w.entities, func(e ast.Entity) bool {
		return e.Type() == entity.Type() && e.Name() == entity.Name()
	})

	if idx >= 0 {
		// Update existing entity
		if err := w.runHooks(HookBeforeUpdate, entity); err != nil {
			return err
		}

		if w.validator != nil {
			if err := w.validator.ValidateEntity(entity); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
		}

		// Run custom validators
		if err := w.runCustomValidators(entity); err != nil {
			return fmt.Errorf("custom validation failed: %w", err)
		}

		w.entities[idx] = entity
		w.recordVersion(entity)
		_ = w.runHooks(HookAfterUpdate, entity)
		w.emit(Event{Type: EventEntityUpdated, Entity: entity})
	} else {
		// Add new entity
		if err := w.runHooks(HookBeforeAdd, entity); err != nil {
			return err
		}

		if w.validator != nil {
			if err := w.validator.ValidateEntity(entity); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
		}

		// Run custom validators
		if err := w.runCustomValidators(entity); err != nil {
			return fmt.Errorf("custom validation failed: %w", err)
		}

		w.entities = append(w.entities, entity)
		w.recordVersion(entity)
		_ = w.runHooks(HookAfterAdd, entity)
		w.emit(Event{Type: EventEntityAdded, Entity: entity})
	}

	return nil
}

// ProcessResult represents the result of processing a single entity.
type ProcessResult struct {
	Entity ast.Entity // The entity that was processed
	Error  error      // Any error that occurred during processing
}

// EntityProcessor is a function that processes an entity and returns a result.
type EntityProcessor func(entity ast.Entity) error

// EntityTransformer is a function that transforms an entity into a new entity.
type EntityTransformer func(entity ast.Entity) (ast.Entity, error)

// EntityPredicate is a function that tests an entity and returns true/false.
type EntityPredicate func(entity ast.Entity) bool

// PipelineStage represents a single stage in a transformation pipeline.
type PipelineStage struct {
	Name        string            // Name of the stage for debugging/logging
	Predicate   EntityPredicate   // Optional: only process entities matching this predicate
	Transformer EntityTransformer // The transformation to apply
}

// Pipeline represents a series of transformation stages.
type Pipeline struct {
	Name   string          // Name of the pipeline
	Stages []PipelineStage // The stages to execute in order
}

// NewPipeline creates a new transformation pipeline.
func NewPipeline(name string) *Pipeline {
	return &Pipeline{
		Name:   name,
		Stages: make([]PipelineStage, 0),
	}
}

// AddStage adds a transformation stage to the pipeline.
func (p *Pipeline) AddStage(name string, transformer EntityTransformer) *Pipeline {
	p.Stages = append(p.Stages, PipelineStage{
		Name:        name,
		Transformer: transformer,
	})
	return p
}

// AddConditionalStage adds a stage that only applies to entities matching the predicate.
func (p *Pipeline) AddConditionalStage(name string, predicate EntityPredicate, transformer EntityTransformer) *Pipeline {
	p.Stages = append(p.Stages, PipelineStage{
		Name:        name,
		Predicate:   predicate,
		Transformer: transformer,
	})
	return p
}

// PipelineResult represents the result of running an entity through a pipeline.
type PipelineResult struct {
	OriginalEntity  ast.Entity // The original entity before transformation
	ResultEntity    ast.Entity // The resulting entity after all stages
	StagesExecuted  []string   // Names of stages that were executed
	StagesSkipped   []string   // Names of stages that were skipped (predicate failed)
	Error           error      // First error encountered, if any
	FailedStageName string     // Name of the stage that failed, if any
}

// Execute runs a single entity through the pipeline.
func (p *Pipeline) Execute(entity ast.Entity) PipelineResult {
	result := PipelineResult{
		OriginalEntity: entity,
		ResultEntity:   entity,
		StagesExecuted: make([]string, 0),
		StagesSkipped:  make([]string, 0),
	}

	current := entity
	for _, stage := range p.Stages {
		// Check predicate if present
		if stage.Predicate != nil && !stage.Predicate(current) {
			result.StagesSkipped = append(result.StagesSkipped, stage.Name)
			continue
		}

		// Execute transformation
		transformed, err := stage.Transformer(current)
		if err != nil {
			result.Error = err
			result.FailedStageName = stage.Name
			return result
		}

		result.StagesExecuted = append(result.StagesExecuted, stage.Name)
		current = transformed
	}

	result.ResultEntity = current
	return result
}

// ExecuteAll runs multiple entities through the pipeline.
func (p *Pipeline) ExecuteAll(entities []ast.Entity) []PipelineResult {
	results := make([]PipelineResult, len(entities))
	for i, entity := range entities {
		results[i] = p.Execute(entity)
	}
	return results
}

// ExecutePipeline executes a pipeline on matching entities in the workspace.
// Returns the results for each processed entity.
func (w *Workspace) ExecutePipeline(pipeline *Pipeline, predicate EntityPredicate) []PipelineResult {
	w.mu.RLock()
	var entities []ast.Entity
	for _, e := range w.entities {
		if predicate == nil || predicate(e) {
			entities = append(entities, e)
		}
	}
	w.mu.RUnlock()

	return pipeline.ExecuteAll(entities)
}

// ExecutePipelineAndUpdate executes a pipeline and updates matching entities in the workspace.
// Only successfully transformed entities are updated.
func (w *Workspace) ExecutePipelineAndUpdate(pipeline *Pipeline, predicate EntityPredicate) ([]PipelineResult, error) {
	results := w.ExecutePipeline(pipeline, predicate)

	var updateErrors []error
	for _, result := range results {
		if result.Error != nil {
			continue // Skip failed transformations
		}

		if result.ResultEntity != nil && result.ResultEntity != result.OriginalEntity {
			if err := w.UpdateEntity(result.ResultEntity); err != nil {
				updateErrors = append(updateErrors, err)
			}
		}
	}

	if len(updateErrors) > 0 {
		return results, fmt.Errorf("failed to update %d entities", len(updateErrors))
	}

	return results, nil
}

// ProcessEntitiesConcurrently processes entities concurrently with the given processor.
// It returns a slice of results for each entity processed.
// The maxConcurrency parameter limits the number of concurrent operations (0 = no limit).
func (w *Workspace) ProcessEntitiesConcurrently(entities []ast.Entity, processor EntityProcessor, maxConcurrency int) []ProcessResult {
	if len(entities) == 0 {
		return nil
	}

	results := make([]ProcessResult, len(entities))

	// Default to unlimited concurrency if not specified
	if maxConcurrency <= 0 {
		maxConcurrency = len(entities)
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, entity := range entities {
		wg.Add(1)
		go func(idx int, e ast.Entity) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			err := processor(e)
			results[idx] = ProcessResult{Entity: e, Error: err}
		}(i, entity)
	}

	wg.Wait()
	return results
}

// AddEntitiesBatch adds multiple entities concurrently.
// Returns a slice of results indicating success or failure for each entity.
// The maxConcurrency parameter limits the number of concurrent add operations.
func (w *Workspace) AddEntitiesBatch(entities []ast.Entity, maxConcurrency int) []ProcessResult {
	return w.ProcessEntitiesConcurrently(entities, func(entity ast.Entity) error {
		return w.AddEntity(entity)
	}, maxConcurrency)
}

// UpdateEntitiesBatch updates multiple entities concurrently.
// Returns a slice of results indicating success or failure for each entity.
func (w *Workspace) UpdateEntitiesBatch(entities []ast.Entity, maxConcurrency int) []ProcessResult {
	return w.ProcessEntitiesConcurrently(entities, func(entity ast.Entity) error {
		return w.UpdateEntity(entity)
	}, maxConcurrency)
}

// UpsertEntitiesBatch upserts multiple entities concurrently.
// Returns a slice of results indicating success or failure for each entity.
func (w *Workspace) UpsertEntitiesBatch(entities []ast.Entity, maxConcurrency int) []ProcessResult {
	return w.ProcessEntitiesConcurrently(entities, func(entity ast.Entity) error {
		return w.UpsertEntity(entity)
	}, maxConcurrency)
}

// TransformEntities applies a transformation to all entities matching the predicate.
// The transformation runs concurrently with the specified concurrency limit.
// Returns the transformed entities and any errors that occurred.
func (w *Workspace) TransformEntities(predicate EntityPredicate, transformer EntityTransformer, maxConcurrency int) ([]ast.Entity, []error) {
	w.mu.RLock()
	// Find entities matching predicate
	var toTransform []ast.Entity
	for _, entity := range w.entities {
		if predicate(entity) {
			toTransform = append(toTransform, entity)
		}
	}
	w.mu.RUnlock()

	if len(toTransform) == 0 {
		return nil, nil
	}

	if maxConcurrency <= 0 {
		maxConcurrency = len(toTransform)
	}

	transformed := make([]ast.Entity, len(toTransform))
	errors := make([]error, len(toTransform))

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, entity := range toTransform {
		wg.Add(1)
		go func(idx int, e ast.Entity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := transformer(e)
			transformed[idx] = result
			errors[idx] = err
		}(i, entity)
	}

	wg.Wait()

	// Filter out nil results and collect only non-nil errors
	var validResults []ast.Entity
	var actualErrors []error
	for i, t := range transformed {
		if t != nil {
			validResults = append(validResults, t)
		}
		if errors[i] != nil {
			actualErrors = append(actualErrors, errors[i])
		}
	}

	return validResults, actualErrors
}

// FilterEntitiesConcurrently filters entities using a predicate, processed concurrently.
// This is useful when the predicate is expensive to compute.
func (w *Workspace) FilterEntitiesConcurrently(predicate EntityPredicate, maxConcurrency int) []ast.Entity {
	w.mu.RLock()
	entities := make([]ast.Entity, len(w.entities))
	copy(entities, w.entities)
	w.mu.RUnlock()

	if len(entities) == 0 {
		return nil
	}

	if maxConcurrency <= 0 {
		maxConcurrency = len(entities)
	}

	matches := make([]bool, len(entities))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, entity := range entities {
		wg.Add(1)
		go func(idx int, e ast.Entity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			matches[idx] = predicate(e)
		}(i, entity)
	}

	wg.Wait()

	var result []ast.Entity
	for i, match := range matches {
		if match {
			result = append(result, entities[i])
		}
	}

	return result
}

// ForEachEntity executes a function for each entity concurrently.
// Unlike ProcessEntitiesConcurrently, this operates on the workspace's own entities.
func (w *Workspace) ForEachEntity(fn EntityProcessor, maxConcurrency int) []ProcessResult {
	w.mu.RLock()
	entities := make([]ast.Entity, len(w.entities))
	copy(entities, w.entities)
	w.mu.RUnlock()

	return w.ProcessEntitiesConcurrently(entities, fn, maxConcurrency)
}

// ForEachEntityOfType executes a function for each entity of a specific type concurrently.
func (w *Workspace) ForEachEntityOfType(entityType string, fn EntityProcessor, maxConcurrency int) []ProcessResult {
	w.mu.RLock()
	var entities []ast.Entity
	for _, e := range w.entities {
		if e.Type() == entityType {
			entities = append(entities, e)
		}
	}
	w.mu.RUnlock()

	return w.ProcessEntitiesConcurrently(entities, fn, maxConcurrency)
}

// removeRelationshipsForEntity removes all relationships involving the specified entity
// Must be called with lock held
func (w *Workspace) removeRelationshipsForEntity(entityType, entityName string) {
	w.relationships = slices.Filter(w.relationships, func(rel Relationship) bool {
		return (rel.SourceType != entityType || rel.SourceName != entityName) &&
			(rel.TargetType != entityType || rel.TargetName != entityName)
	})
}

// Clear removes all entities from the workspace
func (w *Workspace) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.entities = make([]ast.Entity, 0)
	w.relationships = make([]Relationship, 0)

	// Emit workspace cleared event
	w.emit(Event{Type: EventWorkspaceCleared})
}

// AddRelationship creates a relationship between two entities
func (w *Workspace) AddRelationship(sourceType, sourceName, targetType, targetName string, relType RelationType) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check max relationships limit
	if w.config != nil && w.config.MaxRelationships > 0 && len(w.relationships) >= w.config.MaxRelationships {
		return fmt.Errorf("maximum relationship limit reached (%d)", w.config.MaxRelationships)
	}

	// Validate that source entity exists
	sourceExists := slices.Any(w.entities, func(e ast.Entity) bool {
		return e.Type() == sourceType && e.Name() == sourceName
	})
	if !sourceExists {
		return fmt.Errorf("source entity not found: %s %q", sourceType, sourceName)
	}

	// Validate that target entity exists
	targetExists := slices.Any(w.entities, func(e ast.Entity) bool {
		return e.Type() == targetType && e.Name() == targetName
	})
	if !targetExists {
		return fmt.Errorf("target entity not found: %s %q", targetType, targetName)
	}

	// Check for duplicate relationship
	isDuplicate := slices.Any(w.relationships, func(rel Relationship) bool {
		return rel.SourceType == sourceType && rel.SourceName == sourceName &&
			rel.TargetType == targetType && rel.TargetName == targetName &&
			rel.Type == relType
	})
	if isDuplicate {
		return fmt.Errorf("relationship already exists")
	}

	rel := Relationship{
		SourceType: sourceType,
		SourceName: sourceName,
		TargetType: targetType,
		TargetName: targetName,
		Type:       relType,
	}
	w.relationships = append(w.relationships, rel)

	// Emit relationship added event
	w.emit(Event{Type: EventRelationshipAdded, Relationship: &rel})

	return nil
}

// GetRelationships returns all relationships in the workspace
func (w *Workspace) GetRelationships() []Relationship {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]Relationship, len(w.relationships))
	copy(result, w.relationships)
	return result
}

// GetRelationshipsForEntity returns all relationships involving the specified entity
func (w *Workspace) GetRelationshipsForEntity(entityType, entityName string) []Relationship {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return slices.Filter(w.relationships, func(rel Relationship) bool {
		return (rel.SourceType == entityType && rel.SourceName == entityName) ||
			(rel.TargetType == entityType && rel.TargetName == entityName)
	})
}

// GetRelatedEntities returns entities related to the specified entity
func (w *Workspace) GetRelatedEntities(entityType, entityName string, relType RelationType) []ast.Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Find matching relationships
	matchingRels := slices.Filter(w.relationships, func(rel Relationship) bool {
		if rel.Type != relType {
			return false
		}
		return (rel.SourceType == entityType && rel.SourceName == entityName) ||
			(rel.TargetType == entityType && rel.TargetName == entityName)
	})

	// Collect related entities
	var result []ast.Entity
	for _, rel := range matchingRels {
		var targetType, targetName string
		if rel.SourceType == entityType && rel.SourceName == entityName {
			targetType = rel.TargetType
			targetName = rel.TargetName
		} else {
			targetType = rel.SourceType
			targetName = rel.SourceName
		}

		if entity, found := slices.Find(w.entities, func(e ast.Entity) bool {
			return e.Type() == targetType && e.Name() == targetName
		}); found {
			result = append(result, entity)
		}
	}

	return result
}

// RemoveRelationship removes a specific relationship
func (w *Workspace) RemoveRelationship(sourceType, sourceName, targetType, targetName string, relType RelationType) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	idx := slices.FindIndex(w.relationships, func(rel Relationship) bool {
		return rel.SourceType == sourceType && rel.SourceName == sourceName &&
			rel.TargetType == targetType && rel.TargetName == targetName &&
			rel.Type == relType
	})
	if idx == -1 {
		return fmt.Errorf("relationship not found")
	}

	removedRel := w.relationships[idx]
	w.relationships = append(w.relationships[:idx], w.relationships[idx+1:]...)

	// Emit relationship removed event
	w.emit(Event{Type: EventRelationshipRemoved, Relationship: &removedRel})

	return nil
}

// SerializedProperty represents a property for JSON serialization
type SerializedProperty struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// SerializedEntity represents an entity for JSON serialization
type SerializedEntity struct {
	Type       string                        `json:"type"`
	Name       string                        `json:"name"`
	Properties map[string]SerializedProperty `json:"properties"`
	Metadata   map[string]string             `json:"metadata,omitempty"`
	Line       int                           `json:"line,omitempty"`
	Column     int                           `json:"column,omitempty"`
}

// SerializedRelationship represents a relationship for JSON serialization
type SerializedRelationship struct {
	SourceType string       `json:"source_type"`
	SourceName string       `json:"source_name"`
	TargetType string       `json:"target_type"`
	TargetName string       `json:"target_name"`
	Type       RelationType `json:"type"`
}

// SerializedWorkspace represents a workspace for JSON serialization
type SerializedWorkspace struct {
	Version       int                      `json:"version"`
	Entities      []SerializedEntity       `json:"entities"`
	Relationships []SerializedRelationship `json:"relationships"`
}

// serializeValue converts an ast.Value to a SerializedProperty
func serializeValue(v ast.Value) (SerializedProperty, error) {
	var prop SerializedProperty
	switch val := v.(type) {
	case ast.StringValue:
		prop.Type = "string"
		data, _ := json.Marshal(val.Value)
		prop.Value = data
	case ast.NumberValue:
		prop.Type = "number"
		data, _ := json.Marshal(val.Value)
		prop.Value = data
	case ast.BoolValue:
		prop.Type = "bool"
		data, _ := json.Marshal(val.Value)
		prop.Value = data
	case ast.ArrayValue:
		prop.Type = "array"
		var elements []SerializedProperty
		for _, elem := range val.Elements {
			se, err := serializeValue(elem)
			if err != nil {
				return prop, err
			}
			elements = append(elements, se)
		}
		data, _ := json.Marshal(elements)
		prop.Value = data
	case ast.ReferenceValue:
		prop.Type = "reference"
		data, _ := json.Marshal(map[string]interface{}{
			"type": val.Type,
			"name": val.Name,
			"path": val.Path,
		})
		prop.Value = data
	case ast.VariableValue:
		prop.Type = "variable"
		data, _ := json.Marshal(val.Name)
		prop.Value = data
	default:
		return prop, fmt.Errorf("unsupported value type: %T", v)
	}
	return prop, nil
}

// deserializeValue converts a SerializedProperty back to an ast.Value
func deserializeValue(prop SerializedProperty) (ast.Value, error) {
	switch prop.Type {
	case "string":
		var s string
		if err := json.Unmarshal(prop.Value, &s); err != nil {
			return nil, err
		}
		return ast.StringValue{Value: s}, nil
	case "number":
		var n float64
		if err := json.Unmarshal(prop.Value, &n); err != nil {
			return nil, err
		}
		return ast.NumberValue{Value: n}, nil
	case "bool":
		var b bool
		if err := json.Unmarshal(prop.Value, &b); err != nil {
			return nil, err
		}
		return ast.BoolValue{Value: b}, nil
	case "array":
		var elements []SerializedProperty
		if err := json.Unmarshal(prop.Value, &elements); err != nil {
			return nil, err
		}
		values := make([]ast.Value, len(elements))
		for i, elem := range elements {
			v, err := deserializeValue(elem)
			if err != nil {
				return nil, err
			}
			values[i] = v
		}
		return ast.ArrayValue{Elements: values}, nil
	case "reference":
		var ref struct {
			Type string   `json:"type"`
			Name string   `json:"name"`
			Path []string `json:"path"`
		}
		if err := json.Unmarshal(prop.Value, &ref); err != nil {
			return nil, err
		}
		return ast.ReferenceValue{Type: ref.Type, Name: ref.Name, Path: ref.Path}, nil
	case "variable":
		var name string
		if err := json.Unmarshal(prop.Value, &name); err != nil {
			return nil, err
		}
		return ast.VariableValue{Name: name}, nil
	default:
		return nil, fmt.Errorf("unsupported value type: %s", prop.Type)
	}
}

// Serialize converts the workspace to a SerializedWorkspace for JSON export.
func (w *Workspace) Serialize() (*SerializedWorkspace, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	sw := &SerializedWorkspace{
		Version:       1,
		Entities:      make([]SerializedEntity, 0, len(w.entities)),
		Relationships: make([]SerializedRelationship, 0, len(w.relationships)),
	}

	for _, entity := range w.entities {
		se := SerializedEntity{
			Type:       entity.Type(),
			Name:       entity.Name(),
			Properties: make(map[string]SerializedProperty),
			Metadata:   entity.AllMetadata(),
			Line:       entity.Line(),
			Column:     entity.Column(),
		}

		for key, val := range entity.Properties() {
			prop, err := serializeValue(val)
			if err != nil {
				return nil, fmt.Errorf("failed to serialize property %s: %w", key, err)
			}
			se.Properties[key] = prop
		}

		sw.Entities = append(sw.Entities, se)
	}

	for _, rel := range w.relationships {
		sw.Relationships = append(sw.Relationships, SerializedRelationship(rel))
	}

	return sw, nil
}

// SaveTo writes the workspace to an io.Writer in JSON format.
func (w *Workspace) SaveTo(writer io.Writer) error {
	sw, err := w.Serialize()
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sw)
}

// SaveToFile writes the workspace to a file in JSON format.
func (w *Workspace) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}()

	return w.SaveTo(file)
}

// LoadFrom loads entities and relationships from an io.Reader containing JSON.
// This clears the existing workspace and replaces it with the loaded data.
// Hooks and event handlers are NOT loaded - they must be re-registered.
func (w *Workspace) LoadFrom(reader io.Reader) error {
	var sw SerializedWorkspace
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&sw); err != nil {
		return fmt.Errorf("failed to decode workspace: %w", err)
	}

	return w.loadFromSerialized(&sw)
}

// LoadFromFile loads entities and relationships from a JSON file.
func (w *Workspace) LoadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}()

	return w.LoadFrom(file)
}

// loadFromSerialized populates the workspace from a SerializedWorkspace.
func (w *Workspace) loadFromSerialized(sw *SerializedWorkspace) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Clear existing data (but keep hooks and event handlers)
	w.entities = make([]ast.Entity, 0, len(sw.Entities))
	w.relationships = make([]Relationship, 0, len(sw.Relationships))
	w.entityVersions = make(map[string][]EntityVersion)

	// Load entities
	for _, se := range sw.Entities {
		entity, err := ast.NewEntity(se.Type, se.Name)
		if err != nil {
			return fmt.Errorf("failed to create entity %s/%s: %w", se.Type, se.Name, err)
		}

		// Set properties
		for key, prop := range se.Properties {
			val, err := deserializeValue(prop)
			if err != nil {
				return fmt.Errorf("failed to deserialize property %s: %w", key, err)
			}
			entity.SetProperty(key, val)
		}

		// Set metadata
		for key, value := range se.Metadata {
			entity.SetMetadata(key, value)
		}

		// Set location
		entity.SetLocation(se.Line, se.Column)

		w.entities = append(w.entities, entity)

		// Record version if versioning is enabled
		if w.versioningEnabled {
			key := entityKey(entity.Type(), entity.Name())
			w.entityVersions[key] = []EntityVersion{{
				Version:   1,
				Entity:    entity,
				Timestamp: time.Now().Unix(),
			}}
		}
	}

	// Load relationships
	for _, sr := range sw.Relationships {
		w.relationships = append(w.relationships, Relationship(sr))
	}

	return nil
}

// Snapshot represents a point-in-time snapshot of the workspace state.
type Snapshot struct {
	ID        string               // Unique identifier for the snapshot
	Timestamp int64                // Unix timestamp when snapshot was created
	Data      *SerializedWorkspace // The serialized workspace state
}

// CreateSnapshot creates a point-in-time snapshot of the current workspace state.
// The snapshot can be restored later to return the workspace to this state.
func (w *Workspace) CreateSnapshot(id string) (*Snapshot, error) {
	sw, err := w.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize workspace: %w", err)
	}

	return &Snapshot{
		ID:        id,
		Timestamp: time.Now().Unix(),
		Data:      sw,
	}, nil
}

// RestoreSnapshot restores the workspace to a previous snapshot state.
// This clears the current workspace and replaces it with the snapshot data.
// Hooks and event handlers are preserved.
func (w *Workspace) RestoreSnapshot(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("cannot restore nil snapshot")
	}
	if snapshot.Data == nil {
		return fmt.Errorf("snapshot contains no data")
	}
	return w.loadFromSerialized(snapshot.Data)
}

// SnapshotStore manages multiple snapshots for a workspace.
type SnapshotStore struct {
	snapshots map[string]*Snapshot
	mu        sync.RWMutex
}

// NewSnapshotStore creates a new snapshot store.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{
		snapshots: make(map[string]*Snapshot),
	}
}

// Save stores a snapshot in the store.
func (ss *SnapshotStore) Save(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("cannot save nil snapshot")
	}
	if snapshot.ID == "" {
		return fmt.Errorf("snapshot must have an ID")
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.snapshots[snapshot.ID] = snapshot
	return nil
}

// Get retrieves a snapshot by ID.
func (ss *SnapshotStore) Get(id string) (*Snapshot, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	snapshot, ok := ss.snapshots[id]
	return snapshot, ok
}

// Delete removes a snapshot from the store.
func (ss *SnapshotStore) Delete(id string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	_, exists := ss.snapshots[id]
	if exists {
		delete(ss.snapshots, id)
	}
	return exists
}

// List returns all snapshot IDs in the store.
func (ss *SnapshotStore) List() []string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	ids := make([]string, 0, len(ss.snapshots))
	for id := range ss.snapshots {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of snapshots in the store.
func (ss *SnapshotStore) Count() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.snapshots)
}

// DependencyGraph represents a graph of entity dependencies.
// It tracks which entities depend on other entities.
type DependencyGraph struct {
	// dependencies maps entity key -> list of entity keys it depends on
	dependencies map[string][]string
	mu           sync.RWMutex
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		dependencies: make(map[string][]string),
	}
}

// AddDependency adds a dependency from one entity to another.
// Returns an error if this would create a circular dependency.
func (dg *DependencyGraph) AddDependency(fromType, fromName, toType, toName string) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	fromKey := entityKey(fromType, fromName)
	toKey := entityKey(toType, toName)

	// Check for circular dependency before adding
	if dg.wouldCreateCycle(fromKey, toKey) {
		return fmt.Errorf("circular dependency detected: %s -> %s", fromKey, toKey)
	}

	// Check if dependency already exists
	for _, dep := range dg.dependencies[fromKey] {
		if dep == toKey {
			return nil // Already exists
		}
	}

	dg.dependencies[fromKey] = append(dg.dependencies[fromKey], toKey)
	return nil
}

// wouldCreateCycle checks if adding a dependency from -> to would create a cycle.
// Must be called with lock held.
func (dg *DependencyGraph) wouldCreateCycle(from, to string) bool {
	// If to already depends on from (directly or transitively), adding from -> to creates a cycle
	visited := make(map[string]bool)
	return dg.hasPath(to, from, visited)
}

// hasPath checks if there's a path from start to end in the dependency graph.
// Must be called with lock held.
func (dg *DependencyGraph) hasPath(start, end string, visited map[string]bool) bool {
	if start == end {
		return true
	}
	if visited[start] {
		return false
	}
	visited[start] = true

	for _, dep := range dg.dependencies[start] {
		if dg.hasPath(dep, end, visited) {
			return true
		}
	}
	return false
}

// RemoveDependency removes a dependency.
func (dg *DependencyGraph) RemoveDependency(fromType, fromName, toType, toName string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	fromKey := entityKey(fromType, fromName)
	toKey := entityKey(toType, toName)

	deps := dg.dependencies[fromKey]
	for i, dep := range deps {
		if dep == toKey {
			dg.dependencies[fromKey] = append(deps[:i], deps[i+1:]...)
			break
		}
	}
}

// RemoveEntity removes all dependencies involving an entity.
func (dg *DependencyGraph) RemoveEntity(entityType, entityName string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	key := entityKey(entityType, entityName)

	// Remove as a source
	delete(dg.dependencies, key)

	// Remove as a target
	for from, deps := range dg.dependencies {
		filtered := make([]string, 0, len(deps))
		for _, dep := range deps {
			if dep != key {
				filtered = append(filtered, dep)
			}
		}
		dg.dependencies[from] = filtered
	}
}

// GetDependencies returns all entities that the specified entity depends on.
func (dg *DependencyGraph) GetDependencies(entityType, entityName string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	key := entityKey(entityType, entityName)
	deps := dg.dependencies[key]
	result := make([]string, len(deps))
	copy(result, deps)
	return result
}

// GetDependents returns all entities that depend on the specified entity.
func (dg *DependencyGraph) GetDependents(entityType, entityName string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	key := entityKey(entityType, entityName)
	var result []string

	for from, deps := range dg.dependencies {
		for _, dep := range deps {
			if dep == key {
				result = append(result, from)
				break
			}
		}
	}
	return result
}

// GetTransitiveDependencies returns all entities that the specified entity
// depends on, directly or transitively.
func (dg *DependencyGraph) GetTransitiveDependencies(entityType, entityName string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	key := entityKey(entityType, entityName)
	visited := make(map[string]bool)
	var result []string

	dg.collectTransitive(key, visited, &result)
	return result
}

// collectTransitive recursively collects all transitive dependencies.
// Must be called with lock held.
func (dg *DependencyGraph) collectTransitive(key string, visited map[string]bool, result *[]string) {
	for _, dep := range dg.dependencies[key] {
		if !visited[dep] {
			visited[dep] = true
			*result = append(*result, dep)
			dg.collectTransitive(dep, visited, result)
		}
	}
}

// TopologicalSort returns entities in topological order (dependencies first).
// Returns an error if there's a cycle in the graph.
func (dg *DependencyGraph) TopologicalSort() ([]string, error) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Collect all entities
	entities := make(map[string]bool)
	for from := range dg.dependencies {
		entities[from] = true
		for _, to := range dg.dependencies[from] {
			entities[to] = true
		}
	}

	// Build reverse adjacency (who depends on each entity)
	dependents := make(map[string][]string)
	for from, deps := range dg.dependencies {
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], from)
		}
	}

	// Kahn's algorithm - count how many dependencies each entity has
	inDegree := make(map[string]int)
	for e := range entities {
		inDegree[e] = len(dg.dependencies[e])
	}

	// Start with entities that have no dependencies (leaf nodes)
	var queue []string
	for e, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, e)
		}
	}

	var result []string
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Decrement in-degree for entities that depend on current
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(entities) {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}

	return result, nil
}

// Clear removes all dependencies.
func (dg *DependencyGraph) Clear() {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.dependencies = make(map[string][]string)
}

// Count returns the total number of dependencies.
func (dg *DependencyGraph) Count() int {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	count := 0
	for _, deps := range dg.dependencies {
		count += len(deps)
	}
	return count
}
