package validator

import (
	"strings"
	"testing"

	"github.com/shellkjell/langspace/pkg/ast"
)

// TestValidator_ValidateMicrostepEntity tests microstep entity validation
func TestValidator_ValidateMicrostepEntity(t *testing.T) {
	v := New()

	tests := []struct {
		name        string
		entity      ast.Entity
		wantError   bool
		errContains string
	}{
		{
			name: "valid_microstep",
			entity: func() ast.Entity {
				e := ast.NewMicrostepEntity("step1")
				e.SetProperty("use", ast.ReferenceValue{Type: "agent", Name: "solver"})
				return e
			}(),
			wantError: false,
		},
		{
			name:        "missing_name",
			entity:      ast.NewMicrostepEntity(""),
			wantError:   true,
			errContains: "must have a name",
		},
		{
			name: "missing_use",
			entity: func() ast.Entity {
				e := ast.NewMicrostepEntity("step1")
				// no use property
				return e
			}(),
			wantError:   true,
			errContains: "must have 'use' property",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateEntity(tt.entity)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateEntity() error = %v, wantError %v", err, tt.wantError)
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

// TestValidator_ValidateMDAPConfigEntity tests MDAP config entity validation
func TestValidator_ValidateMDAPConfigEntity(t *testing.T) {
	v := New()

	tests := []struct {
		name        string
		entity      ast.Entity
		wantError   bool
		errContains string
	}{
		{
			name: "valid_mdap_config",
			entity: func() ast.Entity {
				e := ast.NewMDAPConfigEntity()
				e.SetProperty("k", ast.NumberValue{Value: 3})
				e.SetProperty("voting_strategy", ast.StringValue{Value: "first-to-ahead-by-k"})
				return e
			}(),
			wantError: false,
		},
		{
			name: "valid_majority_strategy",
			entity: func() ast.Entity {
				e := ast.NewMDAPConfigEntity()
				e.SetProperty("voting_strategy", ast.StringValue{Value: "majority"})
				return e
			}(),
			wantError: false,
		},
		{
			name: "invalid_k_value",
			entity: func() ast.Entity {
				e := ast.NewMDAPConfigEntity()
				e.SetProperty("k", ast.NumberValue{Value: 0})
				return e
			}(),
			wantError:   true,
			errContains: "must be >= 1",
		},
		{
			name: "negative_k_value",
			entity: func() ast.Entity {
				e := ast.NewMDAPConfigEntity()
				e.SetProperty("k", ast.NumberValue{Value: -1})
				return e
			}(),
			wantError:   true,
			errContains: "must be >= 1",
		},
		{
			name: "invalid_voting_strategy",
			entity: func() ast.Entity {
				e := ast.NewMDAPConfigEntity()
				e.SetProperty("voting_strategy", ast.StringValue{Value: "invalid"})
				return e
			}(),
			wantError:   true,
			errContains: "must be 'first-to-ahead-by-k' or 'majority'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateEntity(tt.entity)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateEntity() error = %v, wantError %v", err, tt.wantError)
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

// TestValidator_ValidateMDAPPipelineEntity tests MDAP pipeline entity validation
func TestValidator_ValidateMDAPPipelineEntity(t *testing.T) {
	v := New()

	tests := []struct {
		name        string
		entity      ast.Entity
		wantError   bool
		errContains string
	}{
		{
			name: "valid_mdap_pipeline",
			entity: func() ast.Entity {
				e := ast.NewMDAPPipelineEntity("solver")
				e.SetProperty("strategy", ast.StringValue{Value: "solve optimally"})
				return e
			}(),
			wantError: false,
		},
		{
			name:        "missing_name",
			entity:      ast.NewMDAPPipelineEntity(""),
			wantError:   true,
			errContains: "must have a name",
		},
		{
			name: "missing_strategy",
			entity: func() ast.Entity {
				e := ast.NewMDAPPipelineEntity("solver")
				// no strategy property
				return e
			}(),
			wantError:   true,
			errContains: "should have 'strategy' property",
		},
		{
			name: "valid_with_microsteps",
			entity: func() ast.Entity {
				e := ast.NewMDAPPipelineEntity("solver")
				e.SetProperty("strategy", ast.StringValue{Value: "solve optimally"})
				step := ast.NewMicrostepEntity("step1")
				step.SetProperty("use", ast.ReferenceValue{Type: "agent", Name: "solver"})
				e.AddMicrostep(step)
				return e
			}(),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateEntity(tt.entity)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateEntity() error = %v, wantError %v", err, tt.wantError)
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
