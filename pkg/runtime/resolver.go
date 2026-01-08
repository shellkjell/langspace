package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shellkjell/langspace/pkg/ast"
)

// Resolver handles variable resolution and value interpolation.
// It resolves references like $input, step("x").output, file("path"), env("VAR").
type Resolver struct {
	ctx       *ExecutionContext
	workspace *WorkspaceResolver
}

// NewResolver creates a new resolver with the given execution context.
func NewResolver(ctx *ExecutionContext) *Resolver {
	return &Resolver{
		ctx:       ctx,
		workspace: &WorkspaceResolver{ws: ctx.Workspace},
	}
}

// Resolve resolves an AST value to a concrete Go value.
func (r *Resolver) Resolve(value ast.Value) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case ast.StringValue:
		// Interpolate template variables in strings
		return r.interpolateString(v.Value)

	case ast.NumberValue:
		return v.Value, nil

	case ast.BoolValue:
		return v.Value, nil

	case ast.ArrayValue:
		return r.resolveArray(v)

	case ast.ObjectValue:
		return r.resolveObject(v)

	case ast.VariableValue:
		return r.resolveVariable(v.Name)

	case ast.ReferenceValue:
		return r.resolveReference(v)

	case ast.PropertyAccessValue:
		return r.resolvePropertyAccess(v)

	case ast.MethodCallValue:
		return r.resolveMethodCall(v)

	case ast.FunctionCallValue:
		return r.resolveFunctionCall(v)

	case ast.NestedEntityValue:
		return v.Entity, nil

	case ast.TypedParameterValue:
		// Return as parameter definition, not resolved value
		return v, nil

	case ast.ComparisonValue:
		return r.resolveComparison(v)

	case ast.BranchValue:
		// Branch values are control flow, return as-is
		return v, nil

	case ast.LoopValue:
		// Loop values are control flow, return as-is
		return v, nil

	default:
		return nil, fmt.Errorf("unsupported value type: %T", value)
	}
}

// ResolveString resolves a value to a string.
func (r *Resolver) ResolveString(value ast.Value) (string, error) {
	resolved, err := r.Resolve(value)
	if err != nil {
		return "", err
	}
	return toString(resolved), nil
}

// toString converts a value to string.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// interpolateString handles template interpolation in strings.
// Supports {{variable}} and {{expression}} syntax.
func (r *Resolver) interpolateString(s string) (string, error) {
	result := s

	// Find and replace {{...}} patterns
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}

		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		end += start + 2

		expr := strings.TrimSpace(result[start+2 : end-2])
		value, err := r.resolveExpression(expr)
		if err != nil {
			return "", fmt.Errorf("failed to interpolate {{%s}}: %w", expr, err)
		}

		result = result[:start] + toString(value) + result[end:]
	}

	return result, nil
}

// resolveExpression parses and resolves a string expression.
func (r *Resolver) resolveExpression(expr string) (interface{}, error) {
	expr = strings.TrimSpace(expr)

	// Handle variable references: $var or var
	if strings.HasPrefix(expr, "$") {
		return r.resolveVariable(expr[1:])
	}

	// Handle property access: params.field, step.output
	if strings.Contains(expr, ".") {
		parts := strings.SplitN(expr, ".", 2)
		base := parts[0]
		path := strings.Split(parts[1], ".")

		// Check if base is a variable
		if baseVal, ok := r.ctx.GetVariable(base); ok {
			return getNestedValue(baseVal, path)
		}

		// Check if base is a special reference
		switch base {
		case "params":
			if params, ok := r.ctx.GetVariable("params"); ok {
				return getNestedValue(params, path)
			}
		case "step":
			if len(path) >= 1 {
				stepName := path[0]
				remainder := path[1:]
				if output, ok := r.ctx.GetStepOutput(stepName); ok {
					if len(remainder) == 0 {
						return output, nil
					}
					return getNestedValue(output, remainder)
				}
			}
		case "env":
			if len(path) == 1 {
				return os.Getenv(path[0]), nil
			}
		case "date":
			return formatDate(path[0])
		}

		return nil, fmt.Errorf("cannot resolve expression: %s", expr)
	}

	// Simple variable name
	if val, ok := r.ctx.GetVariable(expr); ok {
		return val, nil
	}

	// Literal value
	return expr, nil
}

// resolveVariable resolves a variable by name.
func (r *Resolver) resolveVariable(name string) (interface{}, error) {
	// Check execution context variables
	if val, ok := r.ctx.GetVariable(name); ok {
		return val, nil
	}

	// Check environment variables
	if envVal := os.Getenv(name); envVal != "" {
		return envVal, nil
	}

	// Check runtime config environment
	if r.ctx.Runtime.config.Environment != nil {
		if val, ok := r.ctx.Runtime.config.Environment[name]; ok {
			return val, nil
		}
	}

	return nil, fmt.Errorf("undefined variable: $%s", name)
}

// resolveReference resolves an entity reference.
func (r *Resolver) resolveReference(ref ast.ReferenceValue) (interface{}, error) {
	switch ref.Type {
	case "agent":
		return r.workspace.GetAgent(ref.Name)

	case "file":
		return r.resolveFileReference(ref.Name)

	case "step":
		// Handle step references with optional path
		// step("name") returns the step output directly
		// step("name").output returns the step output
		// step("name").tokens returns token usage info
		if len(ref.Path) == 0 {
			output, ok := r.ctx.GetStepOutput(ref.Name)
			if !ok {
				return nil, fmt.Errorf("step output not found: %s", ref.Name)
			}
			return output, nil
		}

		// Handle known step accessors
		if ref.Path[0] == "output" {
			output, ok := r.ctx.GetStepOutput(ref.Name)
			if !ok {
				return nil, fmt.Errorf("step output not found: %s", ref.Name)
			}
			// If there are more path elements after "output", access nested properties
			if len(ref.Path) > 1 {
				return getNestedValue(output, ref.Path[1:])
			}
			return output, nil
		}

		if ref.Path[0] == "tokens" {
			tokens, ok := r.ctx.GetStepOutput(ref.Name + ".tokens")
			if !ok {
				return nil, fmt.Errorf("step tokens not found: %s", ref.Name)
			}
			if len(ref.Path) > 1 {
				return getNestedValue(tokens, ref.Path[1:])
			}
			return tokens, nil
		}

		// For other paths, try to get the output and access properties on it
		output, ok := r.ctx.GetStepOutput(ref.Name)
		if !ok {
			return nil, fmt.Errorf("step output not found: %s", ref.Name)
		}
		return getNestedValue(output, ref.Path)

	case "pipeline":
		return r.workspace.GetPipeline(ref.Name)

	case "tool":
		return r.workspace.GetTool(ref.Name)

	case "intent":
		return r.workspace.GetIntent(ref.Name)

	case "env":
		// env("VAR_NAME")
		return os.Getenv(ref.Name), nil

	case "mcp", "mcp_server":
		return r.workspace.GetMCP(ref.Name)

	case "script":
		return r.workspace.GetScript(ref.Name)

	case "config":
		return r.workspace.GetConfig()

	default:
		return nil, fmt.Errorf("unknown reference type: %s", ref.Type)
	}
}

// resolveFileReference resolves a file reference.
func (r *Resolver) resolveFileReference(path string) (interface{}, error) {
	// Check if it's a glob pattern
	if strings.Contains(path, "*") {
		return r.resolveGlobPattern(path)
	}

	// Single file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return string(content), nil
}

// resolveGlobPattern resolves a glob pattern to file contents.
func (r *Resolver) resolveGlobPattern(pattern string) (interface{}, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %s: %w", pattern, err)
	}

	files := make([]FileContent, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		files = append(files, FileContent{
			Path:    path,
			Content: string(content),
		})
	}

	return files, nil
}

// FileContent represents the content of a file.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// resolvePropertyAccess resolves a property access chain.
func (r *Resolver) resolvePropertyAccess(pa ast.PropertyAccessValue) (interface{}, error) {
	// Handle variable prefix ($varname.property)
	base := pa.Base
	if strings.HasPrefix(base, "$") {
		varName := base[1:]
		varVal, err := r.resolveVariable(varName)
		if err != nil {
			return nil, err
		}
		return getNestedValue(varVal, pa.Path)
	}

	// Resolve base as a variable or special reference
	switch base {
	case "git":
		return r.resolveGitProperty(pa.Path)
	case "github":
		return r.resolveGitHubProperty(pa.Path)
	case "params":
		if params, ok := r.ctx.GetVariable("params"); ok {
			return getNestedValue(params, pa.Path)
		}
		return nil, fmt.Errorf("params not defined")
	case "step":
		if len(pa.Path) > 0 {
			stepName := pa.Path[0]
			output, ok := r.ctx.GetStepOutput(stepName)
			if !ok {
				return nil, fmt.Errorf("step output not found: %s", stepName)
			}
			if len(pa.Path) > 1 {
				return getNestedValue(output, pa.Path[1:])
			}
			return output, nil
		}
	}

	// Try as a variable
	if val, ok := r.ctx.GetVariable(base); ok {
		return getNestedValue(val, pa.Path)
	}

	return nil, fmt.Errorf("cannot resolve property access: %s.%s", base, strings.Join(pa.Path, "."))
}

// resolveMethodCall resolves a method call.
func (r *Resolver) resolveMethodCall(mc ast.MethodCallValue) (interface{}, error) {
	// Resolve arguments
	args := make([]interface{}, len(mc.Arguments))
	for i, arg := range mc.Arguments {
		resolved, err := r.Resolve(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve argument %d: %w", i, err)
		}
		args[i] = resolved
	}

	// Get the object
	obj, err := r.Resolve(mc.Object)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve method object: %w", err)
	}

	// Handle map property access (e.g., step("first").output)
	if objMap, ok := obj.(map[string]interface{}); ok {
		if val, exists := objMap[mc.Method]; exists {
			return val, nil
		}
	}

	// Handle string methods
	if s, ok := obj.(string); ok {
		switch mc.Method {
		case "upper":
			return strings.ToUpper(s), nil
		case "lower":
			return strings.ToLower(s), nil
		case "trim":
			return strings.TrimSpace(s), nil
		case "split":
			sep := " "
			if len(args) > 0 {
				sep = toString(args[0])
			}
			return strings.Split(s, sep), nil
		case "contains":
			if len(args) > 0 {
				return strings.Contains(s, toString(args[0])), nil
			}
			return false, nil
		case "replace":
			if len(args) >= 2 {
				return strings.ReplaceAll(s, toString(args[0]), toString(args[1])), nil
			}
			return s, nil
		case "starts_with":
			if len(args) > 0 {
				return strings.HasPrefix(s, toString(args[0])), nil
			}
			return false, nil
		case "ends_with":
			if len(args) > 0 {
				return strings.HasSuffix(s, toString(args[0])), nil
			}
			return false, nil
		case "len":
			return float64(len(s)), nil
		}
	}

	// Handle array methods
	if arr, ok := obj.([]interface{}); ok {
		switch mc.Method {
		case "join":
			sep := ","
			if len(args) > 0 {
				sep = toString(args[0])
			}
			strParts := make([]string, len(arr))
			for i, v := range arr {
				strParts[i] = toString(v)
			}
			return strings.Join(strParts, sep), nil
		case "len":
			return float64(len(arr)), nil
		case "at":
			if len(args) > 0 {
				if idx, ok := toInt(args[0]); ok {
					if idx >= 0 && idx < len(arr) {
						return arr[idx], nil
					}
				}
			}
			return nil, nil
		case "first":
			if len(arr) > 0 {
				return arr[0], nil
			}
			return nil, nil
		case "last":
			if len(arr) > 0 {
				return arr[len(arr)-1], nil
			}
			return nil, nil
		case "contains":
			if len(args) > 0 {
				target := args[0]
				for _, v := range arr {
					if v == target {
						return true, nil
					}
				}
			}
			return false, nil
		}
	}

	// Handle object methods
	if objMap, ok := obj.(map[string]interface{}); ok {
		switch mc.Method {
		case "keys":
			keys := make([]interface{}, 0, len(objMap))
			for k := range objMap {
				keys = append(keys, k)
			}
			return keys, nil
		case "values":
			values := make([]interface{}, 0, len(objMap))
			for _, v := range objMap {
				values = append(values, v)
			}
			return values, nil
		case "has":
			if len(args) > 0 {
				_, exists := objMap[toString(args[0])]
				return exists, nil
			}
			return false, nil
		}
	}

	// Handle special method calls
	objStr := toString(obj)
	switch objStr {
	case "git":
		return r.callGitMethod(mc.Method, args)
	case "github":
		return r.callGitHubMethod(mc.Method, args)
	case "env":
		if len(args) > 0 {
			return os.Getenv(toString(args[0])), nil
		}
	}

	return nil, fmt.Errorf("unknown method call: %s.%s", objStr, mc.Method)
}

// toInt converts a value to int if possible.
func toInt(v interface{}) (int, bool) {
	if f, ok := toFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

// resolveFunctionCall resolves a function call.
func (r *Resolver) resolveFunctionCall(fc ast.FunctionCallValue) (interface{}, error) {
	// Resolve arguments
	args := make([]interface{}, len(fc.Arguments))
	for i, arg := range fc.Arguments {
		resolved, err := r.Resolve(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve argument %d: %w", i, err)
		}
		args[i] = resolved
	}

	// Built-in functions
	switch fc.Function {
	case "env":
		if len(args) > 0 {
			return os.Getenv(toString(args[0])), nil
		}
		return "", nil

	case "file":
		if len(args) > 0 {
			return r.resolveFileReference(toString(args[0]))
		}
		return nil, fmt.Errorf("file() requires a path argument")

	case "read_file":
		if len(args) > 0 {
			content, err := os.ReadFile(toString(args[0]))
			if err != nil {
				return nil, err
			}
			return string(content), nil
		}
		return nil, fmt.Errorf("read_file() requires a path argument")

	case "write_file":
		if len(args) >= 2 {
			path := toString(args[0])
			content := toString(args[1])
			return nil, os.WriteFile(path, []byte(content), 0644)
		}
		return nil, fmt.Errorf("write_file() requires path and content arguments")

	case "print":
		for _, arg := range args {
			fmt.Print(toString(arg))
		}
		fmt.Println()
		return nil, nil

	case "concat":
		var result strings.Builder
		for _, arg := range args {
			result.WriteString(toString(arg))
		}
		return result.String(), nil

	case "len":
		if len(args) > 0 {
			switch v := args[0].(type) {
			case string:
				return float64(len(v)), nil
			case []interface{}:
				return float64(len(v)), nil
			case []FileContent:
				return float64(len(v)), nil
			case map[string]interface{}:
				return float64(len(v)), nil
			}
		}
		return 0.0, nil

	case "step":
		// step("name") returns a step result object with output, tokens, etc.
		if len(args) > 0 {
			stepName := toString(args[0])
			// Return a map with the step's data for property access
			output, hasOutput := r.ctx.GetStepOutput(stepName)
			tokens, hasTokens := r.ctx.GetStepOutput(stepName + ".tokens")
			result := map[string]interface{}{}
			if hasOutput {
				result["output"] = output
			}
			if hasTokens {
				result["tokens"] = tokens
			}
			return result, nil
		}
		return nil, fmt.Errorf("step() requires a step name argument")

	// Codebase analysis functions for MDAP planning
	case "list_files":
		// list_files("src/**/*.go") - returns array of file paths matching pattern
		if len(args) > 0 {
			pattern := toString(args[0])
			return r.listFiles(pattern)
		}
		return nil, fmt.Errorf("list_files() requires a pattern argument")

	case "count_matches":
		// count_matches("pattern", "src/**/*.go") - count regex matches across files
		if len(args) >= 2 {
			pattern := toString(args[0])
			filePattern := toString(args[1])
			return r.countMatches(pattern, filePattern)
		}
		return nil, fmt.Errorf("count_matches() requires pattern and file pattern arguments")

	case "grep":
		// grep("pattern", "src/**/*.go") - returns matches with context
		if len(args) >= 2 {
			pattern := toString(args[0])
			filePattern := toString(args[1])
			contextLines := 2
			if len(args) >= 3 {
				if cl, ok := toInt(args[2]); ok {
					contextLines = cl
				}
			}
			return r.grepFiles(pattern, filePattern, contextLines)
		}
		return nil, fmt.Errorf("grep() requires pattern and file pattern arguments")

	case "analyze_codebase":
		// analyze_codebase("src/**/*.go") - returns summary: file count, total lines, etc.
		if len(args) > 0 {
			pattern := toString(args[0])
			return r.analyzeCodebase(pattern)
		}
		return nil, fmt.Errorf("analyze_codebase() requires a pattern argument")

	default:
		return nil, fmt.Errorf("unknown function: %s", fc.Function)
	}
}

// resolveArray resolves an array value.
func (r *Resolver) resolveArray(arr ast.ArrayValue) (interface{}, error) {
	result := make([]interface{}, len(arr.Elements))
	for i, elem := range arr.Elements {
		resolved, err := r.Resolve(elem)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve array element %d: %w", i, err)
		}
		result[i] = resolved
	}
	return result, nil
}

// resolveObject resolves an object value.
func (r *Resolver) resolveObject(obj ast.ObjectValue) (interface{}, error) {
	result := make(map[string]interface{}, len(obj.Properties))
	for key, val := range obj.Properties {
		resolved, err := r.Resolve(val)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve object property %s: %w", key, err)
		}
		result[key] = resolved
	}
	return result, nil
}

// resolveComparison resolves a comparison expression.
func (r *Resolver) resolveComparison(cmp ast.ComparisonValue) (interface{}, error) {
	left, err := r.Resolve(cmp.Left)
	if err != nil {
		return nil, err
	}

	right, err := r.Resolve(cmp.Right)
	if err != nil {
		return nil, err
	}

	// Try numeric comparison first
	if leftNum, ok := toFloat(left); ok {
		if rightNum, ok := toFloat(right); ok {
			switch cmp.Operator {
			case "==":
				return leftNum == rightNum, nil
			case "!=":
				return leftNum != rightNum, nil
			case "<":
				return leftNum < rightNum, nil
			case ">":
				return leftNum > rightNum, nil
			case "<=":
				return leftNum <= rightNum, nil
			case ">=":
				return leftNum >= rightNum, nil
			}
		}
	}

	// Fall back to string comparison
	leftStr := toString(left)
	rightStr := toString(right)

	switch cmp.Operator {
	case "==":
		return leftStr == rightStr, nil
	case "!=":
		return leftStr != rightStr, nil
	case "<":
		return leftStr < rightStr, nil
	case ">":
		return leftStr > rightStr, nil
	case "<=":
		return leftStr <= rightStr, nil
	case ">=":
		return leftStr >= rightStr, nil
	default:
		return nil, fmt.Errorf("unknown comparison operator: %s", cmp.Operator)
	}
}

// toFloat converts a value to float64 if possible.
func toFloat(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// getNestedValue retrieves a nested value from a complex object.
func getNestedValue(obj interface{}, path []string) (interface{}, error) {
	if len(path) == 0 {
		return obj, nil
	}

	current := obj
	for _, key := range path {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[key]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", key)
			}
			current = val

		case map[string]string:
			val, ok := v[key]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", key)
			}
			current = val

		case ast.Entity:
			val, ok := v.GetProperty(key)
			if !ok {
				return nil, fmt.Errorf("property not found: %s", key)
			}
			current = val

		default:
			return nil, fmt.Errorf("cannot access property %s on type %T", key, current)
		}
	}

	return current, nil
}

// formatDate formats a date component.
func formatDate(format string) (string, error) {
	now := timeNow()

	switch format {
	case "date":
		return now.Format("2006-01-02"), nil
	case "time":
		return now.Format("15:04:05"), nil
	case "datetime":
		return now.Format("2006-01-02T15:04:05"), nil
	case "year":
		return now.Format("2006"), nil
	case "month":
		return now.Format("01"), nil
	case "day":
		return now.Format("02"), nil
	case "timestamp":
		return fmt.Sprintf("%d", now.Unix()), nil
	default:
		return now.Format(format), nil
	}
}

// Git integration helpers

func (r *Resolver) resolveGitProperty(path []string) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("git property path is empty")
	}

	switch path[0] {
	case "staged_files", "diff", "branch", "commit":
		// These would typically be method calls, return placeholder
		return nil, fmt.Errorf("use git.%s() method call syntax", path[0])
	}

	return nil, fmt.Errorf("unknown git property: %s", path[0])
}

func (r *Resolver) callGitMethod(method string, args []interface{}) (interface{}, error) {
	// Git integration is a future enhancement
	// For now, return placeholders or errors
	switch method {
	case "staged_files":
		return []string{}, nil
	case "diff":
		return "", nil
	case "branch":
		return "main", nil
	case "commit":
		return "", nil
	case "push":
		return nil, nil
	case "commits":
		return []string{}, nil
	default:
		return nil, fmt.Errorf("unknown git method: %s", method)
	}
}

// GitHub integration helpers

func (r *Resolver) resolveGitHubProperty(path []string) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("github property path is empty")
	}

	switch path[0] {
	case "pr", "pull_request":
		if len(path) > 1 {
			return r.resolveGitHubPRProperty(path[1:])
		}
		return map[string]interface{}{}, nil
	case "issue":
		return map[string]interface{}{}, nil
	}

	return nil, fmt.Errorf("unknown github property: %s", path[0])
}

func (r *Resolver) resolveGitHubPRProperty(path []string) (interface{}, error) {
	// GitHub integration is a future enhancement
	return nil, fmt.Errorf("github PR properties require API integration")
}

func (r *Resolver) callGitHubMethod(method string, args []interface{}) (interface{}, error) {
	// GitHub integration is a future enhancement
	switch method {
	case "pr_comment", "comment":
		return nil, nil
	case "create_pr":
		return nil, nil
	case "merge_pr":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown github method: %s", method)
	}
}

// WorkspaceResolver provides access to workspace entities.
type WorkspaceResolver struct {
	ws interface {
		GetEntityByName(entityType, entityName string) (ast.Entity, bool)
		GetEntitiesByType(entityType string) []ast.Entity
	}
}

func (wr *WorkspaceResolver) GetAgent(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("agent", name)
	if !found {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetFile(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("file", name)
	if !found {
		return nil, fmt.Errorf("file entity not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetTool(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("tool", name)
	if !found {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetPipeline(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("pipeline", name)
	if !found {
		return nil, fmt.Errorf("pipeline not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetIntent(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("intent", name)
	if !found {
		return nil, fmt.Errorf("intent not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetMCP(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("mcp", name)
	if !found {
		return nil, fmt.Errorf("mcp not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetScript(name string) (ast.Entity, error) {
	entity, found := wr.ws.GetEntityByName("script", name)
	if !found {
		return nil, fmt.Errorf("script not found: %s", name)
	}
	return entity, nil
}

func (wr *WorkspaceResolver) GetConfig() (ast.Entity, error) {
	entities := wr.ws.GetEntitiesByType("config")
	if len(entities) == 0 {
		return nil, fmt.Errorf("no config entity found")
	}
	return entities[0], nil
}

// =============================================================================
// Codebase Analysis Functions (for MDAP runtime inference)
// =============================================================================

// listFiles returns file paths matching a glob pattern.
func (r *Resolver) listFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %s: %w", pattern, err)
	}

	// Filter out directories
	result := make([]string, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			result = append(result, path)
		}
	}

	return result, nil
}

// GrepMatch represents a single grep match with context.
type GrepMatch struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Content    string   `json:"content"`
	Context    []string `json:"context,omitempty"`
	MatchCount int      `json:"match_count,omitempty"`
}

// countMatches counts regex matches across files matching a pattern.
func (r *Resolver) countMatches(regexPattern, filePattern string) (int, error) {
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return 0, fmt.Errorf("invalid regex pattern: %w", err)
	}

	files, err := r.listFiles(filePattern)
	if err != nil {
		return 0, err
	}

	totalCount := 0
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		matches := re.FindAllString(string(content), -1)
		totalCount += len(matches)
	}

	return totalCount, nil
}

// grepFiles searches for a pattern across files and returns matches with context.
func (r *Resolver) grepFiles(regexPattern, filePattern string, contextLines int) ([]GrepMatch, error) {
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	files, err := r.listFiles(filePattern)
	if err != nil {
		return nil, err
	}

	var results []GrepMatch
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				match := GrepMatch{
					File:    path,
					Line:    i + 1,
					Content: line,
				}

				// Add context lines
				if contextLines > 0 {
					start := i - contextLines
					if start < 0 {
						start = 0
					}
					end := i + contextLines + 1
					if end > len(lines) {
						end = len(lines)
					}
					match.Context = lines[start:end]
				}

				results = append(results, match)
			}
		}
	}

	return results, nil
}

// CodebaseAnalysis contains summary statistics about a codebase.
type CodebaseAnalysis struct {
	FileCount      int            `json:"file_count"`
	TotalLines     int            `json:"total_lines"`
	TotalBytes     int64          `json:"total_bytes"`
	FilesByExt     map[string]int `json:"files_by_ext"`
	EstimatedSteps int            `json:"estimated_steps"`
}

// analyzeCodebase returns summary statistics about files matching a pattern.
func (r *Resolver) analyzeCodebase(filePattern string) (*CodebaseAnalysis, error) {
	files, err := r.listFiles(filePattern)
	if err != nil {
		return nil, err
	}

	analysis := &CodebaseAnalysis{
		FilesByExt: make(map[string]int),
	}

	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		analysis.FileCount++
		analysis.TotalBytes += info.Size()
		analysis.TotalLines += strings.Count(string(content), "\n") + 1

		ext := filepath.Ext(path)
		if ext == "" {
			ext = "(no ext)"
		}
		analysis.FilesByExt[ext]++
	}

	// Simple heuristic: estimate ~1 step per file for transformations
	analysis.EstimatedSteps = analysis.FileCount

	return analysis, nil
}
