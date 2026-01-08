package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/shellkjell/langspace/pkg/parser"
	"github.com/shellkjell/langspace/pkg/runtime"
	"github.com/shellkjell/langspace/pkg/workspace"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// run contains the main application logic, separated from main() for testability.
// This follows the pattern recommended by Mat Ryer and others for Go CLI apps.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return showHelp(stdout)
	}

	command := args[0]
	commandArgs := args[1:]

	switch command {
	case "parse":
		return runParse(commandArgs, stdin, stdout)
	case "run":
		return runExecute(commandArgs, stdin, stdout, stderr)
	case "serve":
		return runServe(commandArgs, stdin, stdout, stderr)
	case "validate":
		return runValidate(commandArgs, stdin, stdout)
	case "help", "-h", "--help":
		return showHelp(stdout)
	case "version", "-v", "--version":
		return showVersion(stdout)
	default:
		// If no subcommand, treat as parse (backward compatibility)
		return runParse(args, stdin, stdout)
	}
}

func showHelp(w io.Writer) error {
	help := `LangSpace - A declarative language for composing AI workflows

Usage:
  langspace <command> [options]

Commands:
  parse     Parse a LangSpace file and display entities
  run       Execute an intent or pipeline
  validate  Validate a LangSpace file without executing

Options:
  -h, --help     Show this help message
  -v, --version  Show version information

Examples:
  langspace parse -file workflow.ls
  langspace run -file workflow.ls -name my-intent
  langspace run -file workflow.ls -name my-pipeline -input "Review this code"
  langspace validate -file workflow.ls

For more information, visit: https://github.com/shellkjell/langspace
`
	_, _ = fmt.Fprint(w, help)
	return nil
}

func showVersion(w io.Writer) error {
	_, _ = fmt.Fprintln(w, "langspace version 0.1.0")
	return nil
}

// runParse handles the parse command
func runParse(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	inputFile := fs.String("file", "", "Input file to parse")
	showJSON := fs.Bool("json", false, "Output as JSON")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	input, err := readInput(*inputFile, stdin)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// Create a new workspace
	ws := workspace.New()

	// Parse input
	p := parser.New(input)
	entities, err := p.Parse()
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Add entities to workspace
	for _, entity := range entities {
		if err := ws.AddEntity(entity); err != nil {
			return fmt.Errorf("adding entity %q: %w", entity.Name(), err)
		}
	}

	if *showJSON {
		return outputJSON(stdout, ws)
	}

	// Print statistics
	printStats(stdout, ws.Stat(), len(entities))
	return nil
}

// runExecute handles the run command
func runExecute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	inputFile := fs.String("file", "", "LangSpace file to execute")
	entityName := fs.String("name", "", "Name of the intent or pipeline to execute")
	entityType := fs.String("type", "", "Entity type (intent or pipeline, auto-detected if not specified)")
	inputData := fs.String("input", "", "Input data for the execution")
	inputFile2 := fs.String("input-file", "", "File containing input data")
	timeout := fs.Duration("timeout", 5*time.Minute, "Execution timeout")
	noStream := fs.Bool("no-stream", false, "Disable streaming output")
	verbose := fs.Bool("verbose", false, "Show verbose output")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *inputFile == "" {
		return fmt.Errorf("required flag -file not provided")
	}

	if *entityName == "" {
		return fmt.Errorf("required flag -name not provided")
	}

	// Read and parse the file
	content, err := readInput(*inputFile, stdin)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	ws := workspace.New()
	p := parser.New(content)
	entities, err := p.Parse()
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	for _, entity := range entities {
		if err := ws.AddEntity(entity); err != nil {
			return fmt.Errorf("adding entity %q: %w", entity.Name(), err)
		}
	}

	// Determine entity type if not specified
	if *entityType == "" {
		*entityType = detectEntityType(ws, *entityName)
		if *entityType == "" {
			return fmt.Errorf("entity %q not found. Specify -type to search by type", *entityName)
		}
	}

	// Get input data
	var input interface{}
	if *inputData != "" {
		input = *inputData
	} else if *inputFile2 != "" {
		data, err := os.ReadFile(*inputFile2)
		if err != nil {
			return fmt.Errorf("reading input file: %w", err)
		}
		input = string(data)
	}

	// Create runtime
	rt := runtime.New(ws, runtime.WithConfig(&runtime.Config{
		DefaultModel:    "claude-sonnet-4-20250514",
		DefaultProvider: "anthropic",
		Timeout:         *timeout,
		EnableStreaming: !*noStream,
	}))

	// Register providers
	rt.RegisterProvider("anthropic", runtime.NewAnthropicProvider())
	rt.RegisterProvider("openai", runtime.NewOpenAIProvider())

	// Create stream handler for output
	var handler runtime.StreamHandler
	if !*noStream {
		handler = &CLIStreamHandler{
			stdout:  stdout,
			stderr:  stderr,
			verbose: *verbose,
		}
	}

	// Execute
	ctx := context.Background()
	var opts []runtime.ExecuteOption
	if input != nil {
		opts = append(opts, runtime.WithInput(input))
	}
	if handler != nil {
		opts = append(opts, runtime.WithStreamHandler(handler))
	}
	opts = append(opts, runtime.WithTimeout(*timeout))

	result, err := rt.ExecuteByName(ctx, *entityType, *entityName, opts...)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Print result
	if !*noStream {
		_, _ = fmt.Fprintln(stdout) // Newline after streaming
	}

	if *verbose {
		printExecutionResult(stdout, result)
	} else if result.Output != nil && !*noStream {
		// If not streaming, print the output now
	} else if result.Output != nil {
		_, _ = fmt.Fprintf(stdout, "%v\n", result.Output)
	}

	if !result.Success {
		return fmt.Errorf("execution failed: %v", result.Error)
	}

	return nil
}

// runValidate handles the validate command
func runValidate(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	inputFile := fs.String("file", "", "LangSpace file to validate")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	input, err := readInput(*inputFile, stdin)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// Parse with error recovery
	p := parser.New(input)
	result := p.ParseWithRecovery()

	if result.HasErrors() {
		_, _ = fmt.Fprintf(stdout, "Validation failed with %d error(s):\n", len(result.Errors))
		for i, e := range result.Errors {
			_, _ = fmt.Fprintf(stdout, "  %d. %s\n", i+1, e.Error())
		}
		return fmt.Errorf("validation failed")
	}

	_, _ = fmt.Fprintf(stdout, "Validation successful: %d entities parsed\n", len(result.Entities))
	for _, entity := range result.Entities {
		_, _ = fmt.Fprintf(stdout, "  - %s %q\n", entity.Type(), entity.Name())
	}

	return nil
}

// runServe handles the serve command
func runServe(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	inputFile := fs.String("file", "", "LangSpace file to serve")
	port := fs.Int("port", 8080, "Port to listen on")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *inputFile == "" {
		return fmt.Errorf("required flag -file not provided")
	}

	// Read and parse the file
	content, err := os.ReadFile(*inputFile)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	ws := workspace.New()
	p := parser.New(string(content))
	entities, err := p.Parse()
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	for _, entity := range entities {
		if err := ws.AddEntity(entity); err != nil {
			return fmt.Errorf("adding entity %q: %w", entity.Name(), err)
		}
	}

	// Create runtime
	rt := runtime.New(ws)
	rt.RegisterProvider("anthropic", runtime.NewAnthropicProvider())
	rt.RegisterProvider("openai", runtime.NewOpenAIProvider())

	// Start trigger engine
	engine := runtime.NewTriggerEngine(rt)
	if err := engine.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start trigger engine: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "LangSpace server listening on port %d...\n", *port)
	_, _ = fmt.Fprintf(stdout, "Trigger engine active with %d triggers\n", len(ws.GetEntitiesByType("trigger")))

	// Keep running until interrupted
	select {}
}

// detectEntityType tries to find an entity by name and returns its type
func detectEntityType(ws *workspace.Workspace, name string) string {
	// Try common types in order of likelihood
	for _, typ := range []string{"intent", "pipeline", "agent", "tool"} {
		if _, found := ws.GetEntityByName(typ, name); found {
			return typ
		}
	}
	return ""
}

// readInput reads input from a file or stdin
func readInput(filePath string, stdin io.Reader) (string, error) {
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading file %s: %w", filePath, err)
		}
		return string(data), nil
	}

	reader := bufio.NewReader(stdin)
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	return string(data), nil
}

// printStats outputs workspace statistics
func printStats(w io.Writer, stats workspace.WorkspaceStats, entityCount int) {
	_, _ = fmt.Fprintln(w, "Workspace statistics:")
	_, _ = fmt.Fprintf(w, "  Number of entities: %d\n", stats.TotalEntities)
	_, _ = fmt.Fprintf(w, "  Number of file entities: %d\n", stats.FileEntities)
	_, _ = fmt.Fprintf(w, "  Number of agent entities: %d\n", stats.AgentEntities)
	_, _ = fmt.Fprintf(w, "  Number of tool entities: %d\n", stats.ToolEntities)
	_, _ = fmt.Fprintf(w, "  Number of intent entities: %d\n", stats.IntentEntities)
	_, _ = fmt.Fprintf(w, "  Number of pipeline entities: %d\n", stats.PipelineEntities)
	_, _ = fmt.Fprintf(w, "  Number of script entities: %d\n", stats.ScriptEntities)
	_, _ = fmt.Fprintf(w, "  Number of MDAP pipeline entities: %d\n", stats.MDAPPipelineEntities)
	_, _ = fmt.Fprintf(w, "  Number of relationships: %d\n", stats.TotalRelationships)
	_, _ = fmt.Fprintf(w, "  Number of hooks: %d\n", stats.TotalHooks)
	_, _ = fmt.Fprintf(w, "Successfully processed entities: %d\n", entityCount)
}

// outputJSON outputs the workspace as JSON
func outputJSON(w io.Writer, ws *workspace.Workspace) error {
	return ws.SaveTo(w)
}

// printExecutionResult prints detailed execution result
func printExecutionResult(w io.Writer, result *runtime.ExecutionResult) {
	_, _ = fmt.Fprintln(w, "\n--- Execution Result ---")
	_, _ = fmt.Fprintf(w, "Success: %v\n", result.Success)
	_, _ = fmt.Fprintf(w, "Duration: %s\n", result.Duration)
	_, _ = fmt.Fprintf(w, "Tokens Used: %d (input: %d, output: %d)\n",
		result.TokensUsed.TotalTokens,
		result.TokensUsed.InputTokens,
		result.TokensUsed.OutputTokens)

	if len(result.StepResults) > 0 {
		_, _ = fmt.Fprintln(w, "\nStep Results:")
		for name, step := range result.StepResults {
			_, _ = fmt.Fprintf(w, "  %s: success=%v, duration=%s\n", name, step.Success, step.Duration)
		}
	}

	if result.Error != nil {
		_, _ = fmt.Fprintf(w, "\nError: %v\n", result.Error)
	}

	if result.Output != nil {
		_, _ = fmt.Fprintln(w, "\n--- Output ---")
		_, _ = fmt.Fprintf(w, "%v\n", result.Output)
	}
}

// CLIStreamHandler handles streaming output for the CLI
type CLIStreamHandler struct {
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
}

func (h *CLIStreamHandler) OnChunk(chunk runtime.StreamChunk) {
	if chunk.Type == runtime.ChunkTypeContent {
		_, _ = fmt.Fprint(h.stdout, chunk.Content)
	}
}

func (h *CLIStreamHandler) OnProgress(event runtime.ProgressEvent) {
	if h.verbose {
		switch event.Type {
		case runtime.ProgressTypeStart:
			_, _ = fmt.Fprintf(h.stderr, "🚀 %s\n", event.Message)
		case runtime.ProgressTypeStep:
			_, _ = fmt.Fprintf(h.stderr, "📍 [%d%%] %s\n", event.Progress, event.Message)
		case runtime.ProgressTypeComplete:
			_, _ = fmt.Fprintf(h.stderr, "✅ %s\n", event.Message)
		case runtime.ProgressTypeError:
			_, _ = fmt.Fprintf(h.stderr, "❌ %s\n", event.Message)
		}
	}
}

func (h *CLIStreamHandler) OnComplete(response *runtime.CompletionResponse) {
	// Already handled by OnChunk
}

func (h *CLIStreamHandler) OnError(err error) {
	_, _ = fmt.Fprintf(h.stderr, "Error: %v\n", err)
}
