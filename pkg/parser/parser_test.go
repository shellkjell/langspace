package parser

import (
	"strings"
	"testing"

	"github.com/shellkjell/langspace/pkg/ast"
)

// TestParser_Parse_BlockSyntax tests the new block-based syntax
func TestParser_Parse_BlockSyntax(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCount  int
		checkFirst func(t *testing.T, e ast.Entity)
		wantError  bool
	}{
		{
			name:      "simple_agent",
			input:     `agent "reviewer" { model: "gpt-4o" }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "agent" {
					t.Errorf("Type() = %q, want agent", e.Type())
				}
				if e.Name() != "reviewer" {
					t.Errorf("Name() = %q, want reviewer", e.Name())
				}
				model, ok := e.GetProperty("model")
				if !ok {
					t.Error("expected model property")
				}
				if sv, ok := model.(ast.StringValue); !ok || sv.Value != "gpt-4o" {
					t.Errorf("model = %v, want gpt-4o", model)
				}
			},
		},
		{
			name:      "agent_with_number",
			input:     `agent "test" { model: "gpt-4" temperature: 0.7 }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				temp, ok := e.GetProperty("temperature")
				if !ok {
					t.Error("expected temperature property")
				}
				if nv, ok := temp.(ast.NumberValue); !ok || nv.Value != 0.7 {
					t.Errorf("temperature = %v, want 0.7", temp)
				}
			},
		},
		{
			name:      "agent_with_array",
			input:     `agent "coder" { tools: [read_file, write_file, search] }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				tools, ok := e.GetProperty("tools")
				if !ok {
					t.Error("expected tools property")
				}
				arr, ok := tools.(ast.ArrayValue)
				if !ok {
					t.Errorf("tools is not ArrayValue: %T", tools)
					return
				}
				if len(arr.Elements) != 3 {
					t.Errorf("tools has %d elements, want 3", len(arr.Elements))
				}
			},
		},
		{
			name:      "agent_with_boolean",
			input:     `agent "assistant" { streaming: true verbose: false }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				streaming, ok := e.GetProperty("streaming")
				if !ok {
					t.Error("expected streaming property")
				}
				if bv, ok := streaming.(ast.BoolValue); !ok || !bv.Value {
					t.Errorf("streaming = %v, want true", streaming)
				}

				verbose, ok := e.GetProperty("verbose")
				if !ok {
					t.Error("expected verbose property")
				}
				if bv, ok := verbose.(ast.BoolValue); !ok || bv.Value {
					t.Errorf("verbose = %v, want false", verbose)
				}
			},
		},
		{
			name:      "file_entity",
			input:     `file "config.json" { path: "/etc/config.json" }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "file" {
					t.Errorf("Type() = %q, want file", e.Type())
				}
				if e.Name() != "config.json" {
					t.Errorf("Name() = %q, want config.json", e.Name())
				}
			},
		},
		{
			name:      "intent_with_reference",
			input:     `intent "review" { use: agent("reviewer") input: $code }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "intent" {
					t.Errorf("Type() = %q, want intent", e.Type())
				}

				use, ok := e.GetProperty("use")
				if !ok {
					t.Error("expected use property")
				}
				ref, ok := use.(ast.ReferenceValue)
				if !ok {
					t.Errorf("use is not ReferenceValue: %T", use)
					return
				}
				if ref.Type != "agent" || ref.Name != "reviewer" {
					t.Errorf("use = %v, want agent(reviewer)", ref)
				}

				input, ok := e.GetProperty("input")
				if !ok {
					t.Error("expected input property")
				}
				varVal, ok := input.(ast.VariableValue)
				if !ok {
					t.Errorf("input is not VariableValue: %T", input)
					return
				}
				if varVal.Name != "code" {
					t.Errorf("input = $%s, want $code", varVal.Name)
				}
			},
		},
		{
			name:      "step_with_dot_access",
			input:     `step "report" { input: step("analyze").output }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				input, ok := e.GetProperty("input")
				if !ok {
					t.Error("expected input property")
				}
				ref, ok := input.(ast.ReferenceValue)
				if !ok {
					t.Errorf("input is not ReferenceValue: %T", input)
					return
				}
				if ref.Type != "step" || ref.Name != "analyze" {
					t.Errorf("ref = %v, want step(analyze)", ref)
				}
				if len(ref.Path) != 1 || ref.Path[0] != "output" {
					t.Errorf("ref.Path = %v, want [output]", ref.Path)
				}
			},
		},
		{
			name:      "config_no_name",
			input:     `config { api_key: $OPENAI_API_KEY }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "config" {
					t.Errorf("Type() = %q, want config", e.Type())
				}
			},
		},
		{
			name:      "multiple_entities",
			input:     `agent "a1" { model: "gpt-4" } agent "a2" { model: "gpt-3.5" }`,
			wantCount: 2,
		},
		{
			name:      "empty_input",
			input:     "",
			wantCount: 0,
		},
		{
			name:      "whitespace_only",
			input:     "   \n\t  \n  ",
			wantCount: 0,
		},
		{
			name:      "comment_only",
			input:     "# This is a comment",
			wantCount: 0,
		},
		{
			name: "multiline_with_comments",
			input: `# Agent definition
agent "reviewer" {
    model: "gpt-4o"
    # Temperature setting
    temperature: 0.5
}`,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()

			if (err != nil) != tt.wantError {
				t.Errorf("Parser.Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("Parser.Parse() got %d entities, want %d", len(got), tt.wantCount)
				return
			}

			if tt.checkFirst != nil && len(got) > 0 {
				tt.checkFirst(t, got[0])
			}
		})
	}
}

// TestParser_Parse_LegacySyntax tests backward compatibility with legacy single-line syntax
func TestParser_Parse_LegacySyntax(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantError bool
	}{
		{
			name:      "legacy_file_entity",
			input:     `file "test.txt" path;`,
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "legacy_agent_entity",
			input:     `agent "gpt-4" model;`,
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "legacy_multiple_entities",
			input:     `file "test.txt" path; agent "gpt-4" model;`,
			wantCount: 2,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()

			if (err != nil) != tt.wantError {
				t.Errorf("Parser.Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("Parser.Parse() got %d entities, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// TestParser_Parse_Errors tests error handling
func TestParser_Parse_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantError   bool
		errContains string
	}{
		{
			name:        "invalid_entity_type",
			input:       `unknown "test" { }`,
			wantError:   true,
			errContains: "unknown entity type",
		},
		{
			name:        "missing_entity_name",
			input:       `agent { model: "gpt-4" }`,
			wantError:   true,
			errContains: "expected entity name",
		},
		{
			name:        "unclosed_block",
			input:       `agent "test" { model: "gpt-4"`,
			wantError:   true,
			errContains: "unclosed block",
		},
		{
			name:        "unclosed_array",
			input:       `agent "test" { tools: [a, b }`,
			wantError:   true,
			errContains: "",
		},
		{
			name:        "missing_colon",
			input:       `agent "test" { model "gpt-4" }`,
			wantError:   true,
			errContains: "expected COLON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			_, err := p.Parse()

			if (err != nil) != tt.wantError {
				t.Errorf("Parser.Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestParser_Parse_MultilineStrings tests multiline string content
func TestParser_Parse_MultilineStrings(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantError bool
	}{
		{
			name: "multiline_instruction",
			input: `agent "reviewer" {
    instruction: ` + "```" + `
You are a code reviewer.
Review the following code for bugs and style issues.
` + "```" + `
}`,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()

			if (err != nil) != tt.wantError {
				t.Errorf("Parser.Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("Parser.Parse() got %d entities, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// TestParser_ParseWithRecovery tests error recovery
func TestParser_ParseWithRecovery(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantEntities  int
		wantErrors    int
		checkFirstErr string
	}{
		{
			name:          "no_errors",
			input:         `agent "test" { model: "gpt-4" }`,
			wantEntities:  1,
			wantErrors:    0,
			checkFirstErr: "",
		},
		{
			name:          "recover_after_bad_entity",
			input:         `unknown "x" { } agent "test" { model: "gpt-4" }`,
			wantEntities:  1,
			wantErrors:    1,
			checkFirstErr: "unknown entity type",
		},
		{
			name:          "multiple_errors_with_recovery",
			input:         `unknown "a" { } agent "test" { model: "gpt-4" } badtype "b" { }`,
			wantEntities:  1,
			wantErrors:    2,
			checkFirstErr: "unknown entity type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input).WithErrorRecovery()
			result := p.ParseWithRecovery()

			if len(result.Entities) != tt.wantEntities {
				t.Errorf("ParseWithRecovery() got %d entities, want %d", len(result.Entities), tt.wantEntities)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("ParseWithRecovery() got %d errors, want %d", len(result.Errors), tt.wantErrors)
			}
			if tt.checkFirstErr != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0].Message, tt.checkFirstErr) {
					t.Errorf("First error = %q, want containing %q", result.Errors[0].Message, tt.checkFirstErr)
				}
			}
		})
	}
}

func TestParseError(t *testing.T) {
	err := ParseError{
		Line:    10,
		Column:  5,
		Message: "test error",
	}

	expected := "at line 10, col 5: test error"
	if err.Error() != expected {
		t.Errorf("ParseError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestParseResult_HasErrors(t *testing.T) {
	t.Run("no_errors", func(t *testing.T) {
		result := ParseResult{Errors: []ParseError{}}
		if result.HasErrors() {
			t.Error("HasErrors() = true, want false")
		}
	})

	t.Run("with_errors", func(t *testing.T) {
		result := ParseResult{Errors: []ParseError{{Message: "test"}}}
		if !result.HasErrors() {
			t.Error("HasErrors() = false, want true")
		}
	})
}

func TestParseResult_ErrorString(t *testing.T) {
	t.Run("no_errors", func(t *testing.T) {
		result := ParseResult{Errors: []ParseError{}}
		if result.ErrorString() != "" {
			t.Errorf("ErrorString() = %q, want empty", result.ErrorString())
		}
	})

	t.Run("single_error", func(t *testing.T) {
		result := ParseResult{Errors: []ParseError{{Line: 1, Column: 1, Message: "error1"}}}
		expected := "at line 1, col 1: error1"
		if result.ErrorString() != expected {
			t.Errorf("ErrorString() = %q, want %q", result.ErrorString(), expected)
		}
	})

	t.Run("multiple_errors", func(t *testing.T) {
		result := ParseResult{Errors: []ParseError{
			{Line: 1, Column: 1, Message: "error1"},
			{Line: 2, Column: 5, Message: "error2"},
		}}
		expected := "at line 1, col 1: error1; at line 2, col 5: error2"
		if result.ErrorString() != expected {
			t.Errorf("ErrorString() = %q, want %q", result.ErrorString(), expected)
		}
	})
}

// TestParser_Parse_ScriptEntity tests script entity parsing
func TestParser_Parse_ScriptEntity(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCount  int
		checkFirst func(t *testing.T, e ast.Entity)
		wantError  bool
	}{
		{
			name:      "simple_script",
			input:     `script "hello" { language: "python" runtime: "python3" }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "script" {
					t.Errorf("Type() = %q, want script", e.Type())
				}
				if e.Name() != "hello" {
					t.Errorf("Name() = %q, want hello", e.Name())
				}
				lang, ok := e.GetProperty("language")
				if !ok {
					t.Error("expected language property")
				}
				if sv, ok := lang.(ast.StringValue); !ok || sv.Value != "python" {
					t.Errorf("language = %v, want python", lang)
				}
			},
		},
		{
			name: "script_with_code",
			input: `script "update-record" {
    language: "python"
    runtime: "python3"
    code: ` + "```python\n" + `import db
record = db.find("users", {"id": user_id})
record["description"] = new_description
db.save("users", record)
` + "```" + `
}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "script" {
					t.Errorf("Type() = %q, want script", e.Type())
				}
				code, ok := e.GetProperty("code")
				if !ok {
					t.Error("expected code property")
				}
				if sv, ok := code.(ast.StringValue); !ok {
					t.Errorf("code is not StringValue: %T", code)
				} else {
					if !strings.Contains(sv.Value, "import db") {
						t.Errorf("code should contain 'import db', got: %s", sv.Value)
					}
				}
			},
		},
		{
			name: "script_with_capabilities",
			input: `script "db-script" {
    language: "python"
    capabilities: [database, filesystem]
}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				caps, ok := e.GetProperty("capabilities")
				if !ok {
					t.Error("expected capabilities property")
				}
				arr, ok := caps.(ast.ArrayValue)
				if !ok {
					t.Errorf("capabilities is not ArrayValue: %T", caps)
					return
				}
				if len(arr.Elements) != 2 {
					t.Errorf("capabilities has %d elements, want 2", len(arr.Elements))
				}
			},
		},
		{
			name: "script_with_parameters",
			input: `script "parameterized" {
    language: "python"
    parameters: {
        table: "string required"
        id: "string required"
    }
}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				params, ok := e.GetProperty("parameters")
				if !ok {
					t.Error("expected parameters property")
				}
				obj, ok := params.(ast.ObjectValue)
				if !ok {
					t.Errorf("parameters is not ObjectValue: %T", params)
					return
				}
				if len(obj.Properties) != 2 {
					t.Errorf("parameters has %d properties, want 2", len(obj.Properties))
				}
			},
		},
		{
			name: "script_with_sandbox",
			input: `script "safe-script" {
    language: "python"
    sandbox: {
        network: false
        filesystem: "readonly"
    }
    limits: {
        timeout: "60s"
        memory: "256MB"
    }
}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				sandbox, ok := e.GetProperty("sandbox")
				if !ok {
					t.Error("expected sandbox property")
				}
				obj, ok := sandbox.(ast.ObjectValue)
				if !ok {
					t.Errorf("sandbox is not ObjectValue: %T", sandbox)
					return
				}
				network, exists := obj.Properties["network"]
				if !exists {
					t.Error("sandbox should have network property")
				}
				if bv, ok := network.(ast.BoolValue); !ok || bv.Value != false {
					t.Errorf("sandbox.network = %v, want false", network)
				}

				limits, ok := e.GetProperty("limits")
				if !ok {
					t.Error("expected limits property")
				}
				limObj, ok := limits.(ast.ObjectValue)
				if !ok {
					t.Errorf("limits is not ObjectValue: %T", limits)
					return
				}
				if len(limObj.Properties) != 2 {
					t.Errorf("limits has %d properties, want 2", len(limObj.Properties))
				}
			},
		},
		{
			name:      "script_with_timeout",
			input:     `script "quick" { language: "bash" timeout: "10s" }`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				timeout, ok := e.GetProperty("timeout")
				if !ok {
					t.Error("expected timeout property")
				}
				if sv, ok := timeout.(ast.StringValue); !ok || sv.Value != "10s" {
					t.Errorf("timeout = %v, want 10s", timeout)
				}
			},
		},
		{
			name: "multiple_scripts",
			input: `script "s1" { language: "python" }
script "s2" { language: "bash" }`,
			wantCount: 2,
		},
		{
			name: "script_with_variable_code",
			input: `script "dynamic" {
    language: "python"
    code: $agent_generated_code
}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				code, ok := e.GetProperty("code")
				if !ok {
					t.Error("expected code property")
				}
				varVal, ok := code.(ast.VariableValue)
				if !ok {
					t.Errorf("code is not VariableValue: %T", code)
					return
				}
				if varVal.Name != "agent_generated_code" {
					t.Errorf("code = $%s, want $agent_generated_code", varVal.Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()

			if (err != nil) != tt.wantError {
				t.Errorf("Parser.Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("Parser.Parse() got %d entities, want %d", len(got), tt.wantCount)
				return
			}

			if tt.checkFirst != nil && len(got) > 0 {
				tt.checkFirst(t, got[0])
			}
		})
	}
}

func BenchmarkParser_Parse_BlockSyntax(b *testing.B) {
	input := `agent "reviewer" { model: "gpt-4o" temperature: 0.7 tools: [read_file, write_file] }`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := New(input)
		_, _ = p.Parse()
	}
}

func BenchmarkParser_Parse_Large(b *testing.B) {
	// Generate 100 entities
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(`agent "agent`)
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString(`" { model: "gpt-4" temperature: 0.7 } `)
	}
	input := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := New(input)
		_, _ = p.Parse()
	}
}

// TestParser_TypedParameters tests parsing of typed parameter declarations
func TestParser_TypedParameters(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		checkFirst func(t *testing.T, e ast.Entity)
	}{
		{
			name: "required_string_param",
			input: `tool "test" {
				parameters: {
					name: string required "The name"
				}
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				params, ok := e.GetProperty("parameters")
				if !ok {
					t.Fatal("expected parameters property")
				}
				obj, ok := params.(ast.ObjectValue)
				if !ok {
					t.Fatalf("expected ObjectValue, got %T", params)
				}
				nameParam, ok := obj.Properties["name"]
				if !ok {
					t.Fatal("expected name in parameters")
				}
				typed, ok := nameParam.(ast.TypedParameterValue)
				if !ok {
					t.Fatalf("expected TypedParameterValue, got %T", nameParam)
				}
				if typed.ParamType != "string" {
					t.Errorf("ParamType = %q, want string", typed.ParamType)
				}
				if !typed.Required {
					t.Error("expected Required = true")
				}
				if typed.Description != "The name" {
					t.Errorf("Description = %q, want 'The name'", typed.Description)
				}
			},
		},
		{
			name: "optional_with_default",
			input: `tool "test" {
				parameters: {
					count: number optional 10 "The count"
				}
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				params, _ := e.GetProperty("parameters")
				obj := params.(ast.ObjectValue)
				countParam := obj.Properties["count"].(ast.TypedParameterValue)
				if countParam.ParamType != "number" {
					t.Errorf("ParamType = %q, want number", countParam.ParamType)
				}
				if countParam.Required {
					t.Error("expected Required = false")
				}
				def, ok := countParam.Default.(ast.NumberValue)
				if !ok {
					t.Fatalf("expected NumberValue default, got %T", countParam.Default)
				}
				if def.Value != 10 {
					t.Errorf("Default = %v, want 10", def.Value)
				}
			},
		},
		{
			name: "inline_enum",
			input: `tool "test" {
				output_schema: {
					type: enum ["error", "warning", "info"]
				}
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				schema, _ := e.GetProperty("output_schema")
				obj := schema.(ast.ObjectValue)
				typeParam := obj.Properties["type"].(ast.TypedParameterValue)
				if typeParam.ParamType != "enum" {
					t.Errorf("ParamType = %q, want enum", typeParam.ParamType)
				}
				if len(typeParam.EnumValues) != 3 {
					t.Errorf("EnumValues length = %d, want 3", len(typeParam.EnumValues))
				}
				expected := []string{"error", "warning", "info"}
				for i, v := range typeParam.EnumValues {
					if v != expected[i] {
						t.Errorf("EnumValues[%d] = %q, want %q", i, v, expected[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()
			if err != nil {
				t.Fatalf("Parser.Parse() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d entities, want 1", len(got))
			}
			tt.checkFirst(t, got[0])
		})
	}
}

// TestParser_NestedBlocks tests parsing of nested entity blocks
func TestParser_NestedBlocks(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		checkFirst func(t *testing.T, e ast.Entity)
	}{
		{
			name: "pipeline_with_steps",
			input: `pipeline "review" {
				step "analyze" {
					use: agent("analyzer")
					input: $input
				}
				step "report" {
					use: agent("reporter")
					input: step("analyze").output
				}
				output: step("report").output
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				pipeline, ok := e.(*ast.PipelineEntity)
				if !ok {
					t.Fatalf("expected *ast.PipelineEntity, got %T", e)
				}
				if len(pipeline.Steps) != 2 {
					t.Fatalf("got %d steps, want 2", len(pipeline.Steps))
				}
				if pipeline.Steps[0].Name() != "analyze" {
					t.Errorf("Step[0].Name() = %q, want analyze", pipeline.Steps[0].Name())
				}
				if pipeline.Steps[1].Name() != "report" {
					t.Errorf("Step[1].Name() = %q, want report", pipeline.Steps[1].Name())
				}
			},
		},
		{
			name: "tool_with_handler_block",
			input: `tool "fetch" {
				handler: http {
					method: "GET"
					url: "https://api.example.com"
				}
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				handler, ok := e.GetProperty("handler")
				if !ok {
					t.Fatal("expected handler property")
				}
				nested, ok := handler.(ast.NestedEntityValue)
				if !ok {
					t.Fatalf("expected NestedEntityValue, got %T", handler)
				}
				if nested.Entity.Type() != "http" {
					t.Errorf("handler type = %q, want http", nested.Entity.Type())
				}
				method, ok := nested.Entity.GetProperty("method")
				if !ok {
					t.Fatal("expected method property in handler")
				}
				if sv, ok := method.(ast.StringValue); !ok || sv.Value != "GET" {
					t.Errorf("method = %v, want GET", method)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()
			if err != nil {
				t.Fatalf("Parser.Parse() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d entities, want 1", len(got))
			}
			tt.checkFirst(t, got[0])
		})
	}
}

// TestParser_PropertyAccess tests parsing of property access chains
func TestParser_PropertyAccess(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		checkFirst func(t *testing.T, e ast.Entity)
	}{
		{
			name: "simple_property_access",
			input: `tool "test" {
				query: params.location
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				query, ok := e.GetProperty("query")
				if !ok {
					t.Fatal("expected query property")
				}
				access, ok := query.(ast.PropertyAccessValue)
				if !ok {
					t.Fatalf("expected PropertyAccessValue, got %T", query)
				}
				if access.Base != "params" {
					t.Errorf("Base = %q, want params", access.Base)
				}
				if len(access.Path) != 1 || access.Path[0] != "location" {
					t.Errorf("Path = %v, want [location]", access.Path)
				}
			},
		},
		{
			name: "nested_property_access",
			input: `tool "test" {
				value: config.defaults.timeout
			}`,
			checkFirst: func(t *testing.T, e ast.Entity) {
				val, _ := e.GetProperty("value")
				access := val.(ast.PropertyAccessValue)
				if access.Base != "config" {
					t.Errorf("Base = %q, want config", access.Base)
				}
				if len(access.Path) != 2 {
					t.Fatalf("Path length = %d, want 2", len(access.Path))
				}
				if access.Path[0] != "defaults" || access.Path[1] != "timeout" {
					t.Errorf("Path = %v, want [defaults timeout]", access.Path)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.input)
			got, err := p.Parse()
			if err != nil {
				t.Fatalf("Parser.Parse() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d entities, want 1", len(got))
			}
			tt.checkFirst(t, got[0])
		})
	}
}
