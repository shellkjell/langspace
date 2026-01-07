package parser

import (
	"strings"
	"testing"

	"github.com/shellkjell/langspace/pkg/ast"
)

// TestParser_Parse_MDAPPipeline tests parsing of MDAP pipeline entities
func TestParser_Parse_MDAPPipeline(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCount  int
		checkFirst func(t *testing.T, e ast.Entity)
		wantError  bool
	}{
		{
			name: "simple_mdap_pipeline",
			input: `mdap_pipeline "test" {
				strategy: "simple strategy"
				total_steps: 10
			}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				if e.Type() != "mdap_pipeline" {
					t.Errorf("Type() = %q, want mdap_pipeline", e.Type())
				}
				if e.Name() != "test" {
					t.Errorf("Name() = %q, want test", e.Name())
				}
				strategy, ok := e.GetProperty("strategy")
				if !ok {
					t.Error("expected strategy property")
				}
				if sv, ok := strategy.(ast.StringValue); !ok || sv.Value != "simple strategy" {
					t.Errorf("strategy = %v, want 'simple strategy'", strategy)
				}
				totalSteps, ok := e.GetProperty("total_steps")
				if !ok {
					t.Error("expected total_steps property")
				}
				if nv, ok := totalSteps.(ast.NumberValue); !ok || nv.Value != 10 {
					t.Errorf("total_steps = %v, want 10", totalSteps)
				}
			},
		},
		{
			name: "mdap_pipeline_with_config",
			input: `mdap_pipeline "hanoi" {
				strategy: "recursive solution"

				mdap_config {
					voting_strategy: "first-to-ahead-by-k"
					k: 3
					parallel_samples: 3
					temperature_first: 0.0
					temperature_subsequent: 0.1
					max_output_tokens: 500
					require_format: true
				}

				total_steps: 7
			}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				pipeline, ok := e.(*ast.MDAPPipelineEntity)
				if !ok {
					t.Fatalf("expected *ast.MDAPPipelineEntity, got %T", e)
				}
				if pipeline.Config == nil {
					t.Fatal("expected non-nil Config")
				}

				// Check voting strategy
				votingStrategy, ok := pipeline.Config.GetProperty("voting_strategy")
				if !ok {
					t.Error("expected voting_strategy in config")
				}
				if sv, ok := votingStrategy.(ast.StringValue); !ok || sv.Value != "first-to-ahead-by-k" {
					t.Errorf("voting_strategy = %v, want 'first-to-ahead-by-k'", votingStrategy)
				}

				// Check k
				kVal, ok := pipeline.Config.GetProperty("k")
				if !ok {
					t.Error("expected k in config")
				}
				if nv, ok := kVal.(ast.NumberValue); !ok || nv.Value != 3 {
					t.Errorf("k = %v, want 3", kVal)
				}
			},
		},
		{
			name: "mdap_pipeline_with_microstep",
			input: `mdap_pipeline "solver" {
				strategy: "do the thing"

				microstep "step1" {
					use: agent("solver-agent")
					prompt: "execute the next action"
				}

				total_steps: 100
			}`,
			wantCount: 1,
			checkFirst: func(t *testing.T, e ast.Entity) {
				pipeline, ok := e.(*ast.MDAPPipelineEntity)
				if !ok {
					t.Fatalf("expected *ast.MDAPPipelineEntity, got %T", e)
				}
				if len(pipeline.Microsteps) != 1 {
					t.Fatalf("expected 1 microstep, got %d", len(pipeline.Microsteps))
				}
				microstep := pipeline.Microsteps[0]
				if microstep.Name() != "step1" {
					t.Errorf("microstep name = %q, want step1", microstep.Name())
				}

				// Check use property
				useProp, ok := microstep.GetProperty("use")
				if !ok {
					t.Error("expected use property in microstep")
				}
				ref, ok := useProp.(ast.ReferenceValue)
				if !ok {
					t.Fatalf("expected ReferenceValue, got %T", useProp)
				}
				if ref.Type != "agent" || ref.Name != "solver-agent" {
					t.Errorf("use = %v, want agent(solver-agent)", ref)
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

// TestParser_Parse_Microstep tests parsing of standalone microstep entities
func TestParser_Parse_Microstep(t *testing.T) {
	input := `microstep "move-disk" {
		use: agent("hanoi-solver")
		prompt: "determine the next move"
		context: {
			state: $current_state
			previous_move: $last_action
		}
		output_schema: {
			move: "disk N from A to B"
			next_state: "state representation"
		}
	}`

	p := New(input)
	got, err := p.Parse()
	if err != nil {
		t.Fatalf("Parser.Parse() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(got))
	}

	microstep := got[0]
	if microstep.Type() != "microstep" {
		t.Errorf("Type() = %q, want microstep", microstep.Type())
	}
	if microstep.Name() != "move-disk" {
		t.Errorf("Name() = %q, want move-disk", microstep.Name())
	}

	// Check use property
	useProp, ok := microstep.GetProperty("use")
	if !ok {
		t.Error("expected use property")
	}
	ref, ok := useProp.(ast.ReferenceValue)
	if !ok {
		t.Fatalf("expected ReferenceValue, got %T", useProp)
	}
	if ref.Type != "agent" || ref.Name != "hanoi-solver" {
		t.Errorf("use = %v, want agent(hanoi-solver)", ref)
	}

	// Check output_schema
	outputSchema, ok := microstep.GetProperty("output_schema")
	if !ok {
		t.Error("expected output_schema property")
	}
	obj, ok := outputSchema.(ast.ObjectValue)
	if !ok {
		t.Fatalf("expected ObjectValue, got %T", outputSchema)
	}
	if len(obj.Properties) != 2 {
		t.Errorf("output_schema has %d properties, want 2", len(obj.Properties))
	}
}

// TestParser_Parse_MDAPConfig tests parsing of MDAP config entities
func TestParser_Parse_MDAPConfig(t *testing.T) {
	input := `mdap_config {
		voting_strategy: "first-to-ahead-by-k"
		k: 5
		parallel_samples: 5
		temperature_first: 0.0
		temperature_subsequent: 0.2
		max_output_tokens: 750
		require_format: true
		checkpoint_interval: 1000
	}`

	p := New(input)
	got, err := p.Parse()
	if err != nil {
		t.Fatalf("Parser.Parse() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(got))
	}

	config := got[0]
	if config.Type() != "mdap_config" {
		t.Errorf("Type() = %q, want mdap_config", config.Type())
	}

	// Check k
	kVal, ok := config.GetProperty("k")
	if !ok {
		t.Error("expected k property")
	}
	if nv, ok := kVal.(ast.NumberValue); !ok || nv.Value != 5 {
		t.Errorf("k = %v, want 5", kVal)
	}

	// Check require_format
	requireFormat, ok := config.GetProperty("require_format")
	if !ok {
		t.Error("expected require_format property")
	}
	if bv, ok := requireFormat.(ast.BoolValue); !ok || !bv.Value {
		t.Errorf("require_format = %v, want true", requireFormat)
	}
}

// TestParser_Parse_TowerOfHanoiExample tests that the full Tower of Hanoi example parses
func TestParser_Parse_TowerOfHanoiExample(t *testing.T) {
	input := `
agent "hanoi-solver" {
    model: "gpt-4.1-mini"
    instruction: "You are a Tower of Hanoi solver."
    temperature: 0.0
}

file "hanoi-strategy" {
    contents: "Move disks optimally."
}

mdap_pipeline "solve-hanoi" {
    strategy: file("hanoi-strategy")

    mdap_config {
        voting_strategy: "first-to-ahead-by-k"
        k: 3
        parallel_samples: 3
        temperature_first: 0.0
        temperature_subsequent: 0.1
        max_output_tokens: 500
        require_format: true
        checkpoint_interval: 10000
    }

    total_steps: 7

    input: {
        num_disks: 3
        pegs: {
            A: [1, 2, 3]
            B: []
            C: []
        }
    }

    microstep "move" {
        use: agent("hanoi-solver")
        prompt: "Determine and execute the next move."
    }
}

intent "test-hanoi" {
    use: mdap_pipeline("solve-hanoi")
    input: {
        num_disks: 3
    }
}
`

	p := New(input)
	got, err := p.Parse()
	if err != nil {
		t.Fatalf("Parser.Parse() error = %v", err)
	}

	// Should have: agent, file, mdap_pipeline, intent = 4 entities
	if len(got) != 4 {
		t.Errorf("expected 4 entities, got %d", len(got))
		for _, e := range got {
			t.Logf("  - %s %q", e.Type(), e.Name())
		}
	}

	// Find the MDAP pipeline
	var pipeline *ast.MDAPPipelineEntity
	for _, e := range got {
		if mp, ok := e.(*ast.MDAPPipelineEntity); ok {
			pipeline = mp
			break
		}
	}

	if pipeline == nil {
		t.Fatal("expected to find MDAPPipelineEntity")
	}

	if pipeline.Name() != "solve-hanoi" {
		t.Errorf("pipeline name = %q, want solve-hanoi", pipeline.Name())
	}

	// Check config is parsed
	if pipeline.Config == nil {
		t.Error("expected pipeline.Config to be non-nil")
	} else {
		k, ok := pipeline.Config.GetProperty("k")
		if !ok {
			t.Error("expected k in config")
		} else if nv, ok := k.(ast.NumberValue); !ok || nv.Value != 3 {
			t.Errorf("config.k = %v, want 3", k)
		}
	}

	// Check microstep is parsed
	if len(pipeline.Microsteps) != 1 {
		t.Errorf("expected 1 microstep, got %d", len(pipeline.Microsteps))
	} else {
		if pipeline.Microsteps[0].Name() != "move" {
			t.Errorf("microstep name = %q, want move", pipeline.Microsteps[0].Name())
		}
	}
}

func TestParser_Parse_MDAPErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantError   bool
		errContains string
	}{
		{
			name: "unclosed_mdap_config",
			input: `mdap_pipeline "test" {
				strategy: "test"
				mdap_config {
					k: 3
			}`,
			wantError:   true,
			errContains: "unclosed",
		},
		{
			name: "missing_strategy",
			// This is valid syntax but may fail validation
			input: `mdap_pipeline "test" {
				total_steps: 10
			}`,
			wantError: false, // Syntax is valid, validation would catch missing strategy
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
