package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shellkjell/langspace/pkg/ast"
)

// executeScript executes a script entity.
func (r *Runtime) executeScript(ctx *ExecutionContext, entity ast.Entity) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Metadata: make(map[string]string),
	}
	startTime := timeNow()

	resolver := NewResolver(ctx)

	// Get language
	langProp, ok := entity.GetProperty("language")
	if !ok {
		return nil, fmt.Errorf("script %q missing 'language' property", entity.Name())
	}
	lang, _ := resolver.ResolveString(langProp)

	// Get code
	var code string
	if codeProp, ok := entity.GetProperty("code"); ok {
		code, _ = resolver.ResolveString(codeProp)
	} else if pathProp, ok := entity.GetProperty("path"); ok {
		path, _ := resolver.ResolveString(pathProp)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read script file %q: %w", path, err)
		}
		code = string(content)
	}

	if code == "" {
		return nil, fmt.Errorf("script %q has no code or path", entity.Name())
	}

	// Execute based on language
	var output string
	var err error

	// Resolve parameters
	params := make(map[string]interface{})
	if paramsProp, ok := entity.GetProperty("parameters"); ok {
		resolvedParams, err := resolver.Resolve(paramsProp)
		if err == nil {
			if m, ok := resolvedParams.(map[string]interface{}); ok {
				params = m
			}
		}
	}

	switch lang {
	case "python", "python3":
		output, err = r.executePythonScript(ctx, code, params, resolver)
	case "bash", "sh":
		output, err = r.executeShellScript(ctx, code, params, resolver)
	default:
		return nil, fmt.Errorf("unsupported script language: %s", lang)
	}

	if err != nil {
		result.Error = err
		return result, err
	}

	result.Success = true
	result.Output = output
	result.Duration = time.Since(startTime)

	return result, nil
}

// executePythonScript executes Python code in a temporary file.
func (r *Runtime) executePythonScript(ctx *ExecutionContext, code string, params map[string]interface{}, resolver *Resolver) (string, error) {
	// Create temporary file
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("ls_script_%d.py", timeNow().UnixNano()))

	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
		return "", fmt.Errorf("failed to create temporary script file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile) }()

	// Execute python
	cmd := exec.CommandContext(ctx.Context, "python3", tmpFile)

	// Pass parameters as environment variables
	cmd.Env = os.Environ()
	for k, v := range params {
		cmd.Env = append(cmd.Env, fmt.Sprintf("LS_PARAM_%s=%s", strings.ToUpper(k), toString(v)))
	}

	// Also pass as a single JSON env var
	if len(params) > 0 {
		if data, err := json.Marshal(params); err == nil {
			cmd.Env = append(cmd.Env, fmt.Sprintf("LS_PARAMS=%s", string(data)))
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("python script failed: %w\nStderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// executeShellScript executes shell code.
func (r *Runtime) executeShellScript(ctx *ExecutionContext, code string, params map[string]interface{}, resolver *Resolver) (string, error) {
	cmd := exec.CommandContext(ctx.Context, "sh", "-c", code)

	// Pass parameters as environment variables
	cmd.Env = os.Environ()
	for k, v := range params {
		cmd.Env = append(cmd.Env, fmt.Sprintf("LS_PARAM_%s=%s", strings.ToUpper(k), toString(v)))
	}

	// Also pass as a single JSON env var
	if len(params) > 0 {
		if data, err := json.Marshal(params); err == nil {
			cmd.Env = append(cmd.Env, fmt.Sprintf("LS_PARAMS=%s", string(data)))
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("shell script failed: %w\nStderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
