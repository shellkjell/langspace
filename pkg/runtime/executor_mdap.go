// Package runtime provides the execution runtime for LangSpace.
// This file implements MDAP (Massively Decomposed Agentic Processes) execution
// based on the MAKER framework from "Solving a Million-Step LLM Task with Zero Errors".
//
// Key mechanisms:
//   - Maximal Agentic Decomposition: Each microstep handles ONE atomic action
//   - First-to-ahead-by-k Voting: Multiple samples vote until consensus
//   - Red-Flagging: Reject suspicious responses (long outputs, format errors)
package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shellkjell/langspace/pkg/ast"
)

// MDAPConfig holds configuration for MDAP execution.
type MDAPConfig struct {
	// VotingStrategy: "first-to-ahead-by-k" (default) or "majority"
	VotingStrategy string

	// K is the vote margin required for consensus (default: 3)
	K int

	// ParallelSamples is the number of parallel samples per round (default: K)
	ParallelSamples int

	// TemperatureFirst is the temperature for the first sample (default: 0.0)
	TemperatureFirst float64

	// TemperatureSubsequent is the temperature for subsequent samples (default: 0.1)
	TemperatureSubsequent float64

	// MaxOutputTokens is the red-flag threshold for response length (default: 750)
	MaxOutputTokens int

	// RequireFormat enables format validation (default: true)
	RequireFormat bool

	// CheckpointInterval is the number of steps between checkpoints (default: 1000)
	CheckpointInterval int

	// MaxRetries is the maximum retries for a single step before failing (default: 100)
	MaxRetries int

	// OutputPattern is a regex pattern that valid outputs must match
	OutputPattern *regexp.Regexp
}

// DefaultMDAPConfig returns sensible defaults for MDAP execution.
func DefaultMDAPConfig() *MDAPConfig {
	return &MDAPConfig{
		VotingStrategy:        "first-to-ahead-by-k",
		K:                     3,
		ParallelSamples:       3,
		TemperatureFirst:      0.0,
		TemperatureSubsequent: 0.1,
		MaxOutputTokens:       750,
		RequireFormat:         true,
		CheckpointInterval:    1000,
		MaxRetries:            100,
	}
}

// MDAPExecutionResult extends ExecutionResult with MDAP-specific metrics.
type MDAPExecutionResult struct {
	*ExecutionResult

	// TotalMicrosteps is the number of microsteps executed
	TotalMicrosteps int

	// TotalSamples is the total number of LLM samples taken
	TotalSamples int

	// RejectedSamples is the number of red-flagged responses
	RejectedSamples int

	// VotingRounds is the total voting rounds across all microsteps
	VotingRounds int

	// Checkpoints created during execution
	Checkpoints []MDAPCheckpoint
}

// MDAPCheckpoint represents a saved state during MDAP execution.
type MDAPCheckpoint struct {
	StepIndex int
	State     interface{}
	Timestamp time.Time
}

// MDAPSample represents a single sample from an LLM for voting.
type MDAPSample struct {
	// Content is the raw response content
	Content string

	// Action is the extracted action (for voting comparison)
	Action string

	// NextState is the extracted next state
	NextState interface{}

	// TokenCount is the number of tokens in the response
	TokenCount int

	// RedFlagged indicates if this sample was rejected
	RedFlagged bool

	// RedFlagReason explains why it was red-flagged
	RedFlagReason string
}

// executeMDAPPipeline executes an MDAP pipeline with voting and rejection sampling.
func (r *Runtime) executeMDAPPipeline(ctx *ExecutionContext, entity ast.Entity) (*ExecutionResult, error) {
	pipeline, ok := entity.(*ast.MDAPPipelineEntity)
	if !ok {
		return nil, fmt.Errorf("entity is not an MDAP pipeline")
	}

	result := &MDAPExecutionResult{
		ExecutionResult: &ExecutionResult{
			Metadata:    make(map[string]string),
			StepResults: make(map[string]*StepResult),
		},
		Checkpoints: make([]MDAPCheckpoint, 0),
	}
	startTime := time.Now()

	// Load MDAP config
	config := r.loadMDAPConfig(pipeline)

	// Get the strategy from the pipeline
	strategy := r.resolveStrategy(pipeline)

	// Initialize state from input
	state := ctx.Variables["input"]
	if state == nil {
		state = make(map[string]interface{})
	}

	// Emit start event
	ctx.EmitProgress(ProgressEvent{
		Type:    ProgressTypeStart,
		Message: fmt.Sprintf("Executing MDAP pipeline: %s with %d microsteps", pipeline.Name(), len(pipeline.Microsteps)),
	})

	resolver := NewResolver(ctx)

	// Check for generate_steps function
	totalSteps := len(pipeline.Microsteps)
	if genProp, ok := pipeline.GetProperty("total_steps"); ok {
		if numVal, ok := genProp.(ast.NumberValue); ok {
			totalSteps = int(numVal.Value)
		}
	}

	// If no explicit microsteps, we execute dynamically
	isDynamic := len(pipeline.Microsteps) == 0 && totalSteps > 0

	var lastAction string
	for stepIdx := 0; stepIdx < totalSteps; stepIdx++ {
		// Checkpoint at intervals
		if config.CheckpointInterval > 0 && stepIdx > 0 && stepIdx%config.CheckpointInterval == 0 {
			result.Checkpoints = append(result.Checkpoints, MDAPCheckpoint{
				StepIndex: stepIdx,
				State:     state,
				Timestamp: time.Now(),
			})
			ctx.EmitProgress(ProgressEvent{
				Type:    ProgressTypeStep,
				Message: fmt.Sprintf("Checkpoint at step %d", stepIdx),
				Step:    fmt.Sprintf("checkpoint-%d", stepIdx),
			})
		}

		// Get or generate the microstep
		var microstep *ast.MicrostepEntity
		if isDynamic {
			microstep = r.generateMicrostep(stepIdx, state, strategy)
		} else {
			microstep = pipeline.Microsteps[stepIdx]
		}

		// Execute with MDAP voting
		stepResult, action, newState, err := r.executeMicrostepWithVoting(
			ctx, microstep, config, state, lastAction, strategy, resolver, stepIdx, totalSteps,
		)

		result.TotalMicrosteps++
		result.StepResults[microstep.Name()] = stepResult

		if err != nil {
			result.ExecutionResult.Error = fmt.Errorf("microstep %q failed: %w", microstep.Name(), err)
			ctx.EmitProgress(ProgressEvent{
				Type:    ProgressTypeError,
				Message: err.Error(),
				Step:    microstep.Name(),
			})
			return result.ExecutionResult, result.Error
		}

		// Update state for next iteration
		state = newState
		lastAction = action
		ctx.SetStepOutput(microstep.Name(), stepResult.Output)
	}

	result.ExecutionResult.Success = true
	result.ExecutionResult.Duration = time.Since(startTime)
	result.ExecutionResult.Output = state

	// Emit completion
	ctx.EmitProgress(ProgressEvent{
		Type:     ProgressTypeComplete,
		Message:  fmt.Sprintf("MDAP pipeline completed: %d steps, %d samples, %d rejected", result.TotalMicrosteps, result.TotalSamples, result.RejectedSamples),
		Progress: 100,
		Metadata: map[string]string{
			"total_steps":      fmt.Sprintf("%d", result.TotalMicrosteps),
			"total_samples":    fmt.Sprintf("%d", result.TotalSamples),
			"rejected_samples": fmt.Sprintf("%d", result.RejectedSamples),
			"duration":         result.Duration.String(),
		},
	})

	return result.ExecutionResult, nil
}

// executeMicrostepWithVoting executes a single microstep using first-to-ahead-by-k voting.
func (r *Runtime) executeMicrostepWithVoting(
	ctx *ExecutionContext,
	step *ast.MicrostepEntity,
	config *MDAPConfig,
	currentState interface{},
	lastAction string,
	strategy string,
	resolver *Resolver,
	stepIdx, totalSteps int,
) (*StepResult, string, interface{}, error) {
	stepResult := &StepResult{
		Name:      step.Name(),
		StartTime: time.Now(),
	}

	// Emit step progress
	progress := (stepIdx * 100) / totalSteps
	if stepIdx%100 == 0 || stepIdx < 10 { // Don't spam for every step
		ctx.EmitProgress(ProgressEvent{
			Type:     ProgressTypeStep,
			Message:  fmt.Sprintf("Step %d/%d: %s", stepIdx+1, totalSteps, step.Name()),
			Step:     step.Name(),
			Progress: progress,
		})
	}

	// Get the agent for this microstep
	agent, err := r.resolveMicrostepAgent(ctx, step, resolver)
	if err != nil {
		stepResult.Error = err
		stepResult.EndTime = time.Now()
		stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)
		return stepResult, "", nil, err
	}

	// Build minimal context prompt
	prompt := r.buildMDAPPrompt(step, currentState, lastAction, strategy)

	// Get system prompt
	systemPrompt, err := r.getAgentSystemPrompt(agent, resolver)
	if err != nil {
		stepResult.Error = err
		stepResult.EndTime = time.Now()
		stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)
		return stepResult, "", nil, err
	}

	// Add MDAP-specific instruction
	systemPrompt += `

CRITICAL INSTRUCTIONS:
1. Output EXACTLY the required format - no explanations, no extra text
2. Your response must be parseable - format errors will be rejected
3. Think carefully before answering - wrong format indicates confusion
4. Keep your response concise - overly long responses will be rejected`

	model := r.getAgentModel(agent)
	provider, err := r.getProviderForModel(model)
	if err != nil {
		stepResult.Error = err
		stepResult.EndTime = time.Now()
		stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)
		return stepResult, "", nil, err
	}

	// Voting loop
	votes := make(map[string]int)
	samples := make(map[string]*MDAPSample)
	totalSamples := 0
	rejectedSamples := 0

	for round := 0; round < config.MaxRetries; round++ {
		// Parallel sampling
		roundSamples := r.parallelSample(ctx.Context, provider, model, systemPrompt, prompt, config, round)
		totalSamples += len(roundSamples)

		for _, sample := range roundSamples {
			// Red-flag check
			if r.isRedFlagged(sample, config) {
				rejectedSamples++
				continue
			}

			// Extract action for voting
			action := sample.Action
			if action == "" {
				action = sample.Content // Fallback to full content
			}

			votes[action]++
			samples[action] = sample

			// Check for winner (first-to-ahead-by-k)
			if config.VotingStrategy == "first-to-ahead-by-k" {
				if r.hasWinner(votes, config.K) {
					winner := r.getWinner(votes)
					winnerSample := samples[winner]

					stepResult.Success = true
					stepResult.Output = winnerSample.Content
					stepResult.EndTime = time.Now()
					stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)

					return stepResult, winner, winnerSample.NextState, nil
				}
			}
		}

		// For majority voting, check after each round
		if config.VotingStrategy == "majority" && len(votes) > 0 {
			if totalSamples >= config.K*3 { // Enough samples for majority
				winner := r.getWinner(votes)
				winnerSample := samples[winner]

				stepResult.Success = true
				stepResult.Output = winnerSample.Content
				stepResult.EndTime = time.Now()
				stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)

				return stepResult, winner, winnerSample.NextState, nil
			}
		}
	}

	// No consensus reached
	stepResult.Error = fmt.Errorf("failed to reach consensus after %d samples (%d rejected)", totalSamples, rejectedSamples)
	stepResult.EndTime = time.Now()
	stepResult.Duration = stepResult.EndTime.Sub(stepResult.StartTime)
	return stepResult, "", nil, stepResult.Error
}

// parallelSample generates multiple samples in parallel.
func (r *Runtime) parallelSample(
	ctx context.Context,
	provider LLMProvider,
	model, systemPrompt, prompt string,
	config *MDAPConfig,
	round int,
) []*MDAPSample {
	numSamples := config.ParallelSamples
	if numSamples <= 0 {
		numSamples = config.K
	}

	samples := make([]*MDAPSample, numSamples)
	var wg sync.WaitGroup

	for i := 0; i < numSamples; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Temperature varies: first sample at 0, subsequent at 0.1
			temperature := config.TemperatureSubsequent
			if round == 0 && idx == 0 {
				temperature = config.TemperatureFirst
			}

			req := &CompletionRequest{
				Model:        model,
				SystemPrompt: systemPrompt,
				Messages: []Message{
					{Role: RoleUser, Content: prompt},
				},
				Temperature: temperature,
				MaxTokens:   config.MaxOutputTokens,
			}

			resp, err := provider.Complete(ctx, req)
			if err != nil {
				samples[idx] = &MDAPSample{
					RedFlagged:    true,
					RedFlagReason: fmt.Sprintf("LLM error: %v", err),
				}
				return
			}

			sample := &MDAPSample{
				Content:    resp.Content,
				TokenCount: resp.Usage.OutputTokens,
			}

			// Parse the response to extract action and next_state
			sample.Action, sample.NextState = r.parseHanoiResponse(resp.Content)

			samples[idx] = sample
		}(i)
	}

	wg.Wait()
	return samples
}

// isRedFlagged checks if a sample should be rejected.
func (r *Runtime) isRedFlagged(sample *MDAPSample, config *MDAPConfig) bool {
	if sample.RedFlagged {
		return true
	}

	// Check token length
	if config.MaxOutputTokens > 0 && sample.TokenCount > config.MaxOutputTokens {
		sample.RedFlagged = true
		sample.RedFlagReason = fmt.Sprintf("response too long: %d tokens > %d", sample.TokenCount, config.MaxOutputTokens)
		return true
	}

	// Check format if required
	if config.RequireFormat && config.OutputPattern != nil {
		if !config.OutputPattern.MatchString(sample.Content) {
			sample.RedFlagged = true
			sample.RedFlagReason = "response does not match required format"
			return true
		}
	}

	// For Hanoi, check that we have valid move and next_state
	if sample.Action == "" {
		sample.RedFlagged = true
		sample.RedFlagReason = "could not extract action from response"
		return true
	}

	return false
}

// hasWinner checks if any action has a k-vote lead over all others.
func (r *Runtime) hasWinner(votes map[string]int, k int) bool {
	if len(votes) == 0 {
		return false
	}

	maxVotes := 0
	secondMax := 0
	for _, v := range votes {
		if v > maxVotes {
			secondMax = maxVotes
			maxVotes = v
		} else if v > secondMax {
			secondMax = v
		}
	}

	return maxVotes >= secondMax+k
}

// getWinner returns the action with the most votes.
func (r *Runtime) getWinner(votes map[string]int) string {
	maxVotes := 0
	winner := ""
	for action, v := range votes {
		if v > maxVotes {
			maxVotes = v
			winner = action
		}
	}
	return winner
}

// buildMDAPPrompt creates a minimal context prompt for a microstep.
func (r *Runtime) buildMDAPPrompt(step *ast.MicrostepEntity, state interface{}, lastAction, strategy string) string {
	var parts []string

	// Add strategy if provided
	if strategy != "" {
		parts = append(parts, "## Strategy\n"+strategy)
	}

	// Add current state
	parts = append(parts, fmt.Sprintf("## Current State\n%v", state))

	// Add last action if exists
	if lastAction != "" {
		parts = append(parts, fmt.Sprintf("## Previous Action\n%s", lastAction))
	}

	// Add step-specific prompt if any
	if promptProp, ok := step.GetProperty("prompt"); ok {
		if sv, ok := promptProp.(ast.StringValue); ok {
			parts = append(parts, "## Task\n"+sv.Value)
		}
	}

	// Add output format requirement
	parts = append(parts, `## Required Output Format
Respond with exactly:
move = <your move>
next_state = <resulting state>

No explanations, no extra text.`)

	return strings.Join(parts, "\n\n")
}

// parseHanoiResponse extracts action and next_state from Tower of Hanoi response.
func (r *Runtime) parseHanoiResponse(content string) (action string, nextState interface{}) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "move") || strings.HasPrefix(line, "Move") {
			// Extract move = disk X from A to B
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				action = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "next_state") || strings.HasPrefix(line, "Next_state") {
			// Extract next_state = {...}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				nextState = strings.TrimSpace(parts[1])
			}
		}
	}

	return action, nextState
}

// loadMDAPConfig extracts MDAP configuration from pipeline entity.
func (r *Runtime) loadMDAPConfig(pipeline *ast.MDAPPipelineEntity) *MDAPConfig {
	config := DefaultMDAPConfig()

	if pipeline.Config == nil {
		return config
	}

	cfg := pipeline.Config

	if strategyProp, ok := cfg.GetProperty("voting_strategy"); ok {
		if sv, ok := strategyProp.(ast.StringValue); ok {
			config.VotingStrategy = sv.Value
		}
	}

	if kProp, ok := cfg.GetProperty("k"); ok {
		if nv, ok := kProp.(ast.NumberValue); ok {
			config.K = int(nv.Value)
			if config.ParallelSamples == 0 {
				config.ParallelSamples = config.K
			}
		}
	}

	if parallelProp, ok := cfg.GetProperty("parallel_samples"); ok {
		if nv, ok := parallelProp.(ast.NumberValue); ok {
			config.ParallelSamples = int(nv.Value)
		}
	}

	if tempFirstProp, ok := cfg.GetProperty("temperature_first"); ok {
		if nv, ok := tempFirstProp.(ast.NumberValue); ok {
			config.TemperatureFirst = nv.Value
		}
	}

	if tempSubProp, ok := cfg.GetProperty("temperature_subsequent"); ok {
		if nv, ok := tempSubProp.(ast.NumberValue); ok {
			config.TemperatureSubsequent = nv.Value
		}
	}

	if maxTokensProp, ok := cfg.GetProperty("max_output_tokens"); ok {
		if nv, ok := maxTokensProp.(ast.NumberValue); ok {
			config.MaxOutputTokens = int(nv.Value)
		}
	}

	if requireFormatProp, ok := cfg.GetProperty("require_format"); ok {
		if bv, ok := requireFormatProp.(ast.BoolValue); ok {
			config.RequireFormat = bv.Value
		}
	}

	if checkpointProp, ok := cfg.GetProperty("checkpoint_interval"); ok {
		if nv, ok := checkpointProp.(ast.NumberValue); ok {
			config.CheckpointInterval = int(nv.Value)
		}
	}

	return config
}

// resolveStrategy extracts the strategy from the pipeline.
func (r *Runtime) resolveStrategy(pipeline *ast.MDAPPipelineEntity) string {
	strategyProp, ok := pipeline.GetProperty("strategy")
	if !ok {
		return ""
	}

	switch v := strategyProp.(type) {
	case ast.StringValue:
		return v.Value
	case ast.ReferenceValue:
		// Load from file
		if v.Type == "file" {
			if fileEntity, found := r.workspace.GetEntityByName("file", v.Name); found {
				if contentsProp, ok := fileEntity.GetProperty("contents"); ok {
					if sv, ok := contentsProp.(ast.StringValue); ok {
						return sv.Value
					}
				}
			}
		}
	}

	return ""
}

// resolveMicrostepAgent resolves the agent for a microstep.
func (r *Runtime) resolveMicrostepAgent(ctx *ExecutionContext, step *ast.MicrostepEntity, resolver *Resolver) (ast.Entity, error) {
	useProp, ok := step.GetProperty("use")
	if !ok {
		return nil, fmt.Errorf("microstep %q has no 'use' property", step.Name())
	}

	switch v := useProp.(type) {
	case ast.ReferenceValue:
		if v.Type != "agent" {
			return nil, fmt.Errorf("expected agent reference, got %s", v.Type)
		}
		return resolver.workspace.GetAgent(v.Name)

	case ast.StringValue:
		return resolver.workspace.GetAgent(v.Value)

	default:
		resolved, err := resolver.Resolve(useProp)
		if err != nil {
			return nil, err
		}
		if agent, ok := resolved.(ast.Entity); ok && agent.Type() == "agent" {
			return agent, nil
		}
		return nil, fmt.Errorf("cannot resolve agent from %T", useProp)
	}
}

// generateMicrostep creates a dynamic microstep for step N.
func (r *Runtime) generateMicrostep(stepIdx int, state interface{}, strategy string) *ast.MicrostepEntity {
	step := ast.NewMicrostepEntity(fmt.Sprintf("step-%d", stepIdx))
	step.SetProperty("prompt", ast.StringValue{Value: "Determine and execute the next move."})
	return step
}

// Add MDAP pipeline execution to the main Execute dispatch
func (r *Runtime) executeEntityDispatch(ctx *ExecutionContext, entity ast.Entity) (*ExecutionResult, error) {
	switch entity.Type() {
	case "mdap_pipeline":
		return r.executeMDAPPipeline(ctx, entity)
	default:
		return nil, fmt.Errorf("executor_mdap: unknown entity type %q", entity.Type())
	}
}
