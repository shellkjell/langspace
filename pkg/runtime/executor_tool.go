package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// executeShellCommand executes a shell command with arguments.
func (r *Runtime) executeShellCommand(ctx *ExecutionContext, command string, args map[string]interface{}) (string, error) {
	// Replace placeholders in command string: {{arg}}
	for k, v := range args {
		placeholder := fmt.Sprintf("{{%s}}", k)
		command = strings.ReplaceAll(command, placeholder, toString(v))
	}

	// Execute the command
	cmd := exec.CommandContext(ctx.Context, "sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if err != nil {
		return output, fmt.Errorf("command failed: %w\nStderr: %s", err, stderr.String())
	}

	return output, nil
}

// executeFunction executes a built-in function.
func (r *Runtime) executeFunction(ctx *ExecutionContext, name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "http", "http_request":
		return r.executeHTTPTool(ctx, args)
	case "read_file":
		if path, ok := args["path"].(string); ok {
			return r.executeShellCommand(ctx, "cat {{path}}", map[string]interface{}{"path": path})
		}
	case "write_file":
		if path, ok := args["path"].(string); ok {
			if content, ok := args["content"].(string); ok {
				// Use a safer way in production, but for now:
				return r.executeShellCommand(ctx, "printf '%s' {{content}} > {{path}}", map[string]interface{}{
					"path":    path,
					"content": content,
				})
			}
		}
	}

	return nil, fmt.Errorf("unknown function tool: %s", name)
}

// executeHTTPTool performs an HTTP request.
func (r *Runtime) executeHTTPTool(ctx *ExecutionContext, args map[string]interface{}) (interface{}, error) {
	method := "GET"
	if m, ok := args["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("http tool requires 'url' argument")
	}

	var bodyReader io.Reader
	if body, ok := args["body"]; ok {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx.Context, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers if provided
	if headers, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, toString(v))
		}
	} else if headers, ok := args["headers"].(map[string]string); ok {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	result := map[string]interface{}{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     resp.Header,
		"body":        string(respBody),
	}

	// Try to parse body as JSON
	var jsonBody interface{}
	if err := json.Unmarshal(respBody, &jsonBody); err == nil {
		result["json"] = jsonBody
	}

	return result, nil
}
