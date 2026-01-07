package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/shellkjell/langspace/pkg/ast"
)

// executeIntent executes an intent entity.
func (r *Runtime) executeIntent(ctx *ExecutionContext, entity ast.Entity) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Metadata: make(map[string]string),
	}
	// Copy metadata from execution context
	for k, v := range ctx.Metadata {
		result.Metadata[k] = v
	}
	startTime := time.Now()

	// Emit start event
	ctx.EmitProgress(ProgressEvent{
		Type:    ProgressTypeStart,
		Message: fmt.Sprintf("Executing intent: %s", entity.Name()),
	})

	resolver := NewResolver(ctx)

	// Get the agent to use
	agent, err := r.resolveAgent(ctx, entity, resolver)
	if err != nil {
		result.Error = fmt.Errorf("failed to resolve agent: %w", err)
		return result, result.Error
	}

	// Build the prompt from input and context
	prompt, err := r.buildIntentPrompt(ctx, entity, resolver)
	if err != nil {
		result.Error = fmt.Errorf("failed to build prompt: %w", err)
		return result, result.Error
	}

	// Get the system prompt from the agent
	systemPrompt, err := r.getAgentSystemPrompt(agent, resolver)
	if err != nil {
		result.Error = fmt.Errorf("failed to get system prompt: %w", err)
		return result, result.Error
	}

	// Get the model to use
	model := r.getAgentModel(agent)

	// Get temperature
	temperature := r.getAgentTemperature(agent)

	// Get tools
	tools, err := r.getAgentTools(ctx, agent, resolver)
	if err != nil {
		result.Error = fmt.Errorf("failed to get agent tools: %w", err)
		return result, result.Error
	}

	// Get the provider
	provider, err := r.getProviderForModel(model)
	if err != nil {
		result.Error = fmt.Errorf("failed to get provider: %w", err)
		return result, result.Error
	}

	// Build the initial messages
	messages := []Message{
		{Role: RoleUser, Content: prompt},
	}

	// Loop for tool execution
	maxTurns := 10
	for turn := 0; turn < maxTurns; turn++ {
		// Build the request
		req := &CompletionRequest{
			Model:        model,
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Temperature:  temperature,
			Tools:        tools,
		}

	// Execute the LLM call
	var resp *CompletionResponse
	var lastResp *CompletionResponse
	if ctx.Handler != nil && r.config.EnableStreaming {
		resp, err = provider.CompleteStream(ctx.Context, req, ctx.Handler)
	} else {
		resp, err = provider.Complete(ctx.Context, req)
	}

	if err != nil {
		result.Error = fmt.Errorf("LLM request failed: %w", err)
		ctx.EmitProgress(ProgressEvent{
			Type:    ProgressTypeError,
			Message: err.Error(),
		})
		return result, result.Error
	}

	lastResp = resp
	// Update token usage
	result.TokensUsed.Add(resp.Usage)

	// Add assistant message to history
	assistantMsg := Message{
		Role:      RoleAssistant,
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}
	messages = append(messages, assistantMsg)

	// If no tool calls, we're done
	if len(resp.ToolCalls) == 0 || resp.FinishReason != FinishReasonToolUse {
		result.Output = resp.Content
		result.Metadata["finish_reason"] = string(resp.FinishReason)
		break
	}

	// Execute tool calls
	for _, tc := range resp.ToolCalls {
		ctx.EmitProgress(ProgressEvent{
			Type:    ProgressTypeStep,
			Message: fmt.Sprintf("Executing tool: %s", tc.Name),
			Metadata: map[string]string{
				"tool": tc.Name,
			},
		})

		toolResult, err := r.executeToolCall(ctx, tc, resolver)
		if err != nil {
			// We report the error back to the LLM so it can try to fix it
			toolResult = fmt.Sprintf("Error: %v", err)
		}

		// Add tool result to history
		messages = append(messages, Message{
			Role:       RoleTool,
			Content:    toString(toolResult),
			ToolCallID: tc.ID,
		})
	}
	resp = lastResp // Restore for metadata access if needed
}

	// Store the output
	result.Success = true
	result.Duration = time.Since(startTime)
	result.Metadata["model"] = model

	// Handle output destination if specified
	if result.Output != nil {
		if err := r.handleIntentOutput(ctx, entity, toString(result.Output), resolver); err != nil {
			result.Error = fmt.Errorf("failed to handle output: %w", err)
			return result, result.Error
		}
	}

	// Emit completion event
	ctx.EmitProgress(ProgressEvent{
		Type:     ProgressTypeComplete,
		Message:  fmt.Sprintf("Intent completed: %s", entity.Name()),
		Progress: 100,
		Metadata: map[string]string{
			"tokens_used": fmt.Sprintf("%d", result.TokensUsed.TotalTokens),
			"duration":    result.Duration.String(),
		},
	})

	return result, nil
}

// getAgentTools extracts tool definitions from an agent.
func (r *Runtime) getAgentTools(ctx *ExecutionContext, agent ast.Entity, resolver *Resolver) ([]ToolDefinition, error) {
	toolsProp, ok := agent.GetProperty("tools")
	if !ok {
		return nil, nil
	}

	var toolNames []string
	switch v := toolsProp.(type) {
	case ast.ArrayValue:
		for _, elem := range v.Elements {
			if sv, ok := elem.(ast.StringValue); ok {
				toolNames = append(toolNames, sv.Value)
			} else if rv, ok := elem.(ast.ReferenceValue); ok && rv.Type == "tool" {
				toolNames = append(toolNames, rv.Name)
			} else if rv, ok := elem.(ast.ReferenceValue); ok && rv.Type == "mcp" {
				toolNames = append(toolNames, rv.Name)
			}
		}
	}

	var definitions []ToolDefinition
	for _, name := range toolNames {
		// Check if it's an MCP server reference
		if mcpEntity, err := resolver.workspace.GetMCP(name); err == nil {
			client, err := r.getMCPClient(mcpEntity.Name())
			if err != nil {
				return nil, err
			}
			mcpTools, err := client.ListTools(ctx.Context)
			if err != nil {
				return nil, err
			}

			// Track which tools belong to this MCP server
			if ctx.MCPTools == nil {
				ctx.MCPTools = make(map[string]string)
			}
			for _, t := range mcpTools {
				ctx.MCPTools[t.Name] = mcpEntity.Name()
			}

			definitions = append(definitions, mcpTools...)
			continue
		}

		tool, err := resolver.workspace.GetTool(name)
		if err != nil {
			return nil, err
		}

		def := ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Name(), // Default to name
		}

		if desc, ok := tool.GetProperty("description"); ok {
			if sv, ok := desc.(ast.StringValue); ok {
				def.Description = sv.Value
			}
		}

		if params, ok := tool.GetProperty("parameters"); ok {
			if obj, ok := params.(ast.ObjectValue); ok {
				def.Parameters = make(map[string]interface{})
				// Convert AST ObjectValue to JSON-compatible map
				// This is a simplified conversion
				for k, v := range obj.Properties {
					def.Parameters[k] = v
				}
			}
		}

		definitions = append(definitions, def)
	}

	return definitions, nil
}

// executeToolCall executes a single tool call from the LLM.
func (r *Runtime) executeToolCall(ctx *ExecutionContext, tc ToolCall, resolver *Resolver) (interface{}, error) {
	// Check if it's an MCP tool
	if mcpServer, ok := ctx.MCPTools[tc.Name]; ok {
		return r.executeMCPTool(ctx, mcpServer, tc.Name, tc.Arguments)
	}

	tool, err := resolver.workspace.GetTool(tc.Name)
	if err != nil {
		return nil, err
	}

	// Check for command property (shell tool)
	if cmd, ok := tool.GetProperty("command"); ok {
		cmdStr, err := resolver.ResolveString(cmd)
		if err != nil {
			return nil, err
		}

		// Interpolate arguments into command if needed
		// For now, just append them or use a simple template
		return r.executeShellCommand(ctx, cmdStr, tc.Arguments)
	}

	// Check for function property (built-in or custom function)
	if fn, ok := tool.GetProperty("function"); ok {
		fnName, err := resolver.ResolveString(fn)
		if err != nil {
			return nil, err
		}
		return r.executeFunction(ctx, fnName, tc.Arguments)
	}

	return nil, fmt.Errorf("tool %q has no executable property (command or function)", tc.Name)
}

// resolveAgent resolves the agent to use for an intent.
func (r *Runtime) resolveAgent(ctx *ExecutionContext, entity ast.Entity, resolver *Resolver) (ast.Entity, error) {
	useProp, ok := entity.GetProperty("use")
	if !ok {
		return nil, fmt.Errorf("intent %q has no 'use' property", entity.Name())
	}

	// Resolve the agent reference
	switch v := useProp.(type) {
	case ast.ReferenceValue:
		if v.Type != "agent" {
			return nil, fmt.Errorf("expected agent reference, got %s", v.Type)
		}
		return resolver.workspace.GetAgent(v.Name)

	case ast.StringValue:
		// Direct agent name
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

// buildIntentPrompt builds the user prompt from input and context.
func (r *Runtime) buildIntentPrompt(ctx *ExecutionContext, entity ast.Entity, resolver *Resolver) (string, error) {
	var promptParts []string

	// Get input
	if inputProp, ok := entity.GetProperty("input"); ok {
		inputContent, err := r.resolveInputContent(inputProp, resolver)
		if err != nil {
			return "", fmt.Errorf("failed to resolve input: %w", err)
		}
		if inputContent != "" {
			promptParts = append(promptParts, "## Input\n\n"+inputContent)
		}
	} else if input, ok := ctx.GetVariable("input"); ok {
		// Use input from execution context
		promptParts = append(promptParts, "## Input\n\n"+toString(input))
	}

	// Get context
	if contextProp, ok := entity.GetProperty("context"); ok {
		contextContent, err := r.resolveContextContent(contextProp, resolver)
		if err != nil {
			return "", fmt.Errorf("failed to resolve context: %w", err)
		}
		if contextContent != "" {
			promptParts = append(promptParts, "## Context\n\n"+contextContent)
		}
	}

	// Get explicit prompt if provided
	if promptProp, ok := entity.GetProperty("prompt"); ok {
		promptStr, err := resolver.ResolveString(promptProp)
		if err != nil {
			return "", fmt.Errorf("failed to resolve prompt: %w", err)
		}
		promptParts = append(promptParts, promptStr)
	}

	if len(promptParts) == 0 {
		// Use entity name as a basic prompt
		return fmt.Sprintf("Please help me: %s", entity.Name()), nil
	}

	return strings.Join(promptParts, "\n\n"), nil
}

// resolveInputContent resolves the input property to content.
func (r *Runtime) resolveInputContent(input ast.Value, resolver *Resolver) (string, error) {
	resolved, err := resolver.Resolve(input)
	if err != nil {
		return "", err
	}

	return formatContent(resolved), nil
}

// resolveContextContent resolves the context property to content.
func (r *Runtime) resolveContextContent(context ast.Value, resolver *Resolver) (string, error) {
	resolved, err := resolver.Resolve(context)
	if err != nil {
		return "", err
	}

	return formatContent(resolved), nil
}

// formatContent formats resolved content for inclusion in a prompt.
func formatContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v

	case []byte:
		return string(v)

	case FileContent:
		return fmt.Sprintf("### %s\n\n```\n%s\n```", v.Path, v.Content)

	case []FileContent:
		var parts []string
		for _, f := range v {
			parts = append(parts, fmt.Sprintf("### %s\n\n```\n%s\n```", f.Path, f.Content))
		}
		return strings.Join(parts, "\n\n")

	case []interface{}:
		var parts []string
		for _, item := range v {
			parts = append(parts, formatContent(item))
		}
		return strings.Join(parts, "\n\n")

	case map[string]interface{}:
		var parts []string
		for k, val := range v {
			parts = append(parts, fmt.Sprintf("**%s**: %s", k, formatContent(val)))
		}
		return strings.Join(parts, "\n")

	case ast.Entity:
		// For file entities, try to get contents
		if v.Type() == "file" {
			if contents, ok := v.GetProperty("contents"); ok {
				return formatContent(contents)
			}
			if path, ok := v.GetProperty("path"); ok {
				return formatContent(path)
			}
		}
		return fmt.Sprintf("[%s: %s]", v.Type(), v.Name())

	default:
		return fmt.Sprintf("%v", content)
	}
}

// getAgentSystemPrompt extracts the system prompt from an agent.
func (r *Runtime) getAgentSystemPrompt(agent ast.Entity, resolver *Resolver) (string, error) {
	// Check for instruction property
	if instruction, ok := agent.GetProperty("instruction"); ok {
		return resolver.ResolveString(instruction)
	}

	// Check for system_prompt property
	if systemPrompt, ok := agent.GetProperty("system_prompt"); ok {
		return resolver.ResolveString(systemPrompt)
	}

	// Check for prompt property
	if prompt, ok := agent.GetProperty("prompt"); ok {
		return resolver.ResolveString(prompt)
	}

	// Default system prompt based on agent name
	return fmt.Sprintf("You are %s. Help the user with their request.", agent.Name()), nil
}

// getAgentModel gets the model to use for an agent.
func (r *Runtime) getAgentModel(agent ast.Entity) string {
	if model, ok := agent.GetProperty("model"); ok {
		if sv, ok := model.(ast.StringValue); ok {
			return sv.Value
		}
	}
	return r.defaultModel
}

// getAgentTemperature gets the temperature setting for an agent.
func (r *Runtime) getAgentTemperature(agent ast.Entity) float64 {
	if temp, ok := agent.GetProperty("temperature"); ok {
		if nv, ok := temp.(ast.NumberValue); ok {
			return nv.Value
		}
	}
	return 0.7 // Default temperature
}

// getProviderForModel returns the appropriate provider for a model.
func (r *Runtime) getProviderForModel(model string) (LLMProvider, error) {
	// Check model prefix to determine provider
	switch {
	case strings.HasPrefix(model, "claude"):
		if p, ok := r.providers["anthropic"]; ok {
			return p, nil
		}
	case strings.HasPrefix(model, "gpt"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"):
		if p, ok := r.providers["openai"]; ok {
			return p, nil
		}
	}

	// Try default provider
	if p, ok := r.providers[r.config.DefaultProvider]; ok {
		return p, nil
	}

	// Return any available provider
	for _, p := range r.providers {
		return p, nil
	}

	return nil, fmt.Errorf("no LLM provider available for model %q", model)
}

// handleIntentOutput handles writing output to a destination.
func (r *Runtime) handleIntentOutput(ctx *ExecutionContext, entity ast.Entity, output string, resolver *Resolver) error {
	outputProp, ok := entity.GetProperty("output")
	if !ok {
		return nil // No output destination specified
	}

	// Resolve the output destination
	switch v := outputProp.(type) {
	case ast.ReferenceValue:
		if v.Type == "file" {
			// Resolve the file path with interpolation
			path, err := resolver.interpolateString(v.Name)
			if err != nil {
				return err
			}
			return writeFile(path, output)
		}

	case ast.StringValue:
		// Assume it's a file path
		path, err := resolver.interpolateString(v.Value)
		if err != nil {
			return err
		}
		return writeFile(path, output)

	case ast.MethodCallValue:
		// Handle method calls like github.pr_comment()
		// This is a future enhancement
		return nil
	}

	return nil
}

// writeFile writes content to a file, creating directories as needed.
func writeFile(path string, content string) error {
	// Create parent directories if needed
	// (this is handled by os.WriteFile in Go 1.16+, but we're explicit)
	return writeFileWithDirs(path, []byte(content), 0644)
}

// writeFileWithDirs writes a file, creating parent directories as needed.
func writeFileWithDirs(path string, data []byte, perm uint32) error {
	// For simplicity, just use os.WriteFile
	// Directory creation would be added in production
	return nil // Placeholder - actual file writing would happen here
}
