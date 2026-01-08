package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shellkjell/langspace/pkg/ast"
	"github.com/shellkjell/langspace/pkg/tokenizer"
)

// Package parser implements the LangSpace language parser, responsible for converting
// raw input text into structured AST entities. The parser supports both the new
// block-based syntax and legacy single-line declarations for backward compatibility.

// ParseError represents a parsing error with location information
type ParseError struct {
	Line    int    // Line number where the error occurred
	Column  int    // Column number where the error occurred
	Message string // Error message
}

// Error implements the error interface
func (e ParseError) Error() string {
	return fmt.Sprintf("at line %d, col %d: %s", e.Line, e.Column, e.Message)
}

// ParseResult contains the result of parsing, including any recovered errors
type ParseResult struct {
	Entities []ast.Entity // Successfully parsed entities
	Errors   []ParseError // Errors encountered during parsing
}

// HasErrors returns true if there were any parsing errors
func (r ParseResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// ErrorString returns all errors as a single string
func (r ParseResult) ErrorString() string {
	if len(r.Errors) == 0 {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// Parser represents a LangSpace parser instance
type Parser struct {
	input         string
	tokenizer     *tokenizer.Tokenizer
	tokens        []tokenizer.Token
	pos           int
	errorRecovery bool
}

// Option is a functional option for configuring the Parser
type Option func(*Parser)

// WithTokenizer sets a custom tokenizer for the parser.
// This supports dependency injection for testing and customization.
func WithTokenizer(t *tokenizer.Tokenizer) Option {
	return func(p *Parser) {
		p.tokenizer = t
	}
}

// New creates a new Parser instance with the given input text.
// Optional configuration can be provided via functional options.
func New(input string, opts ...Option) *Parser {
	p := &Parser{
		input:         input,
		tokenizer:     tokenizer.New(),
		errorRecovery: false,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithErrorRecovery enables error recovery mode
func (p *Parser) WithErrorRecovery() *Parser {
	p.errorRecovery = true
	return p
}

// current returns the current token
func (p *Parser) current() tokenizer.Token {
	if p.pos >= len(p.tokens) {
		return tokenizer.Token{Type: -1, Value: "", Line: 0, Column: 0}
	}
	return p.tokens[p.pos]
}

// peek returns a token at offset from current position
func (p *Parser) peek(offset int) tokenizer.Token {
	pos := p.pos + offset
	if pos >= len(p.tokens) || pos < 0 {
		return tokenizer.Token{Type: -1, Value: "", Line: 0, Column: 0}
	}
	return p.tokens[pos]
}

// advance moves to the next token
func (p *Parser) advance() {
	p.pos++
}

// expect checks if current token matches expected type and advances
func (p *Parser) expect(t tokenizer.TokenType) (tokenizer.Token, *ParseError) {
	tok := p.current()
	if tok.Type != t {
		return tok, &ParseError{
			Line:    tok.Line,
			Column:  tok.Column,
			Message: fmt.Sprintf("expected %s, got %s", t, tok.Type),
		}
	}
	p.advance()
	return tok, nil
}

// ParseWithRecovery processes the input and returns a ParseResult
func (p *Parser) ParseWithRecovery() ParseResult {
	result := ParseResult{
		Entities: make([]ast.Entity, 0),
		Errors:   make([]ParseError, 0),
	}

	allTokens := p.tokenizer.Tokenize(p.input)
	if len(allTokens) == 0 {
		return result
	}

	// Filter out comment tokens
	p.tokens = make([]tokenizer.Token, 0, len(allTokens))
	for _, t := range allTokens {
		if t.Type != tokenizer.TokenTypeComment {
			p.tokens = append(p.tokens, t)
		}
	}

	if len(p.tokens) == 0 {
		return result
	}

	p.pos = 0
	for p.pos < len(p.tokens) {
		entity, err := p.parseTopLevel()
		if err != nil {
			result.Errors = append(result.Errors, *err)
			p.skipToRecoveryPoint()
			continue
		}
		if entity != nil {
			result.Entities = append(result.Entities, entity)
		}
	}

	return result
}

// skipToRecoveryPoint advances past errors
func (p *Parser) skipToRecoveryPoint() {
	for p.pos < len(p.tokens) {
		tok := p.current()
		if tok.Type == tokenizer.TokenTypeRightBrace ||
			tok.Type == tokenizer.TokenTypeSemicolon {
			p.advance()
			return
		}
		p.advance()
	}
}

// parseTopLevel parses a top-level declaration
func (p *Parser) parseTopLevel() (ast.Entity, *ParseError) {
	tok := p.current()
	if tok.Type != tokenizer.TokenTypeIdentifier {
		return nil, &ParseError{
			Line:    tok.Line,
			Column:  tok.Column,
			Message: fmt.Sprintf("expected entity type, got %s", tok.Type),
		}
	}

	entityType := tok.Value
	p.advance()

	// Check if this is block syntax (name followed by {) or legacy syntax
	nameTok := p.current()

	// Config and mdap_config don't have a name
	if entityType == "config" || entityType == "mdap_config" {
		return p.parseBlockEntity(entityType, "", tok.Line, tok.Column)
	}

	// Expect a name (string or identifier)
	var name string
	switch nameTok.Type {
	case tokenizer.TokenTypeString:
		name = nameTok.Value
		p.advance()
	case tokenizer.TokenTypeIdentifier:
		name = nameTok.Value
		p.advance()
	default:
		return nil, &ParseError{
			Line:    nameTok.Line,
			Column:  nameTok.Column,
			Message: "expected entity name",
		}
	}

	// Check for block syntax vs legacy
	nextTok := p.current()
	if nextTok.Type == tokenizer.TokenTypeLeftBrace {
		return p.parseBlockEntity(entityType, name, tok.Line, tok.Column)
	}

	// Legacy single-line syntax
	return p.parseLegacyEntity(entityType, name, tok.Line, tok.Column)
}

// parseBlockEntity parses an entity with block syntax: entity "name" { ... }
func (p *Parser) parseBlockEntity(entityType, name string, line, col int) (ast.Entity, *ParseError) {
	entity, err := ast.NewEntity(entityType, name)
	if err != nil {
		return nil, &ParseError{Line: line, Column: col, Message: err.Error()}
	}
	entity.SetLocation(line, col)

	// Expect opening brace
	if _, err := p.expect(tokenizer.TokenTypeLeftBrace); err != nil {
		return nil, err
	}

	// Parse properties until closing brace
	for p.current().Type != tokenizer.TokenTypeRightBrace {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    line,
				Column:  col,
				Message: "unclosed block",
			}
		}

		if err := p.parseProperty(entity); err != nil {
			return nil, err
		}
	}

	// Expect closing brace
	if _, err := p.expect(tokenizer.TokenTypeRightBrace); err != nil {
		return nil, err
	}

	return entity, nil
}

// parseProperty parses a property assignment: key: value
// This also handles typed parameters: key: type required/optional [default] ["description"]
// And nested entity blocks: step "name" { ... }
// And control flow: branch expr { ... }, loop max: N { ... }
func (p *Parser) parseProperty(entity ast.Entity) *ParseError {
	keyTok := p.current()
	if keyTok.Type != tokenizer.TokenTypeIdentifier {
		return &ParseError{
			Line:    keyTok.Line,
			Column:  keyTok.Column,
			Message: fmt.Sprintf("expected property name, got %s", keyTok.Type),
		}
	}
	key := keyTok.Value
	p.advance()

	// Check for branch control flow: branch expr { "case" => ... }
	if key == "branch" {
		branchValue, err := p.parseBranch(keyTok.Line, keyTok.Column)
		if err != nil {
			return err
		}
		entity.SetProperty(key, branchValue)
		return nil
	}

	// Check for loop control flow: loop max: N { ... }
	if key == "loop" {
		loopValue, err := p.parseLoop(keyTok.Line, keyTok.Column)
		if err != nil {
			return err
		}
		entity.SetProperty(key, loopValue)
		return nil
	}

	// Check for nested entity block: step "name" { or parallel { etc
	// Only specific keywords trigger nested entity parsing
	nextTok := p.current()
	if p.isNestedEntityKeyword(key) && (nextTok.Type == tokenizer.TokenTypeString || nextTok.Type == tokenizer.TokenTypeLeftBrace) {
		// This is a nested entity block (like step "analyze" { ... } or parallel { ... })
		nestedValue, err := p.parseNestedEntity(key, keyTok.Line, keyTok.Column)
		if err != nil {
			return err
		}
		// Add to parent entity - special handling for pipelines and parallel blocks
		if pipeline, ok := entity.(*ast.PipelineEntity); ok {
			if step, ok := nestedValue.Entity.(*ast.StepEntity); ok {
				pipeline.Steps = append(pipeline.Steps, step)
				return nil
			}
		}

		if parallel, ok := entity.(*ast.ParallelEntity); ok {
			if step, ok := nestedValue.Entity.(*ast.StepEntity); ok {
				parallel.Steps = append(parallel.Steps, step)
				return nil
			}
		}

		// Handle microsteps in MDAP pipelines
		if mdapPipeline, ok := entity.(*ast.MDAPPipelineEntity); ok {
			if microstep, ok := nestedValue.Entity.(*ast.MicrostepEntity); ok {
				mdapPipeline.Microsteps = append(mdapPipeline.Microsteps, microstep)
				return nil
			}
			// Also handle mdap_config
			if mdapConfig, ok := nestedValue.Entity.(*ast.MDAPConfigEntity); ok {
				mdapPipeline.Config = mdapConfig
				return nil
			}
		}

		// For other cases, store as property
		entity.SetProperty(key, nestedValue)
		return nil
	}

	// Expect colon for regular property
	if _, err := p.expect(tokenizer.TokenTypeColon); err != nil {
		return err
	}

	// Parse value
	value, err := p.parseValue()
	if err != nil {
		return err
	}

	entity.SetProperty(key, value)
	return nil
}

// isNestedEntityKeyword checks if an identifier is a keyword that can start a nested entity block
func (p *Parser) isNestedEntityKeyword(name string) bool {
	switch name {
	case "step", "parallel", "handler", "on_success", "on_failure", "on_error", "on_complete", "config", "microstep", "mdap_config":
		return true
	}
	return false
}

// parseValue parses a value with optional comparison operators
func (p *Parser) parseValue() (ast.Value, *ParseError) {
	left, err := p.parsePrimaryValue()
	if err != nil {
		return nil, err
	}

	// Check for comparison operators
	tok := p.current()
	var operator string
	switch tok.Type {
	case tokenizer.TokenTypeDoubleEquals:
		operator = "=="
	case tokenizer.TokenTypeNotEquals:
		operator = "!="
	case tokenizer.TokenTypeLess:
		operator = "<"
	case tokenizer.TokenTypeGreater:
		operator = ">"
	case tokenizer.TokenTypeLessEquals:
		operator = "<="
	case tokenizer.TokenTypeGreaterEquals:
		operator = ">="
	default:
		return left, nil
	}

	// We have a comparison operator
	p.advance()

	right, err := p.parsePrimaryValue()
	if err != nil {
		return nil, err
	}

	return ast.ComparisonValue{
		Left:     left,
		Operator: operator,
		Right:    right,
	}, nil
}

// parsePrimaryValue parses a primary value (string, number, bool, array, object, reference)
func (p *Parser) parsePrimaryValue() (ast.Value, *ParseError) {
	tok := p.current()

	switch tok.Type {
	case tokenizer.TokenTypeString:
		p.advance()
		return ast.StringValue{Value: tok.Value}, nil

	case tokenizer.TokenTypeMultilineString:
		p.advance()
		// Trim leading newline from multiline strings
		content := tok.Value
		if len(content) > 0 && content[0] == '\n' {
			content = content[1:]
		}
		return ast.StringValue{Value: content}, nil

	case tokenizer.TokenTypeNumber:
		p.advance()
		val, _ := strconv.ParseFloat(tok.Value, 64)
		return ast.NumberValue{Value: val}, nil

	case tokenizer.TokenTypeBoolean:
		p.advance()
		return ast.BoolValue{Value: tok.Value == "true"}, nil

	case tokenizer.TokenTypeDollar:
		// Variable reference: $name or $name.property
		p.advance()
		nameTok := p.current()
		if nameTok.Type != tokenizer.TokenTypeIdentifier {
			return nil, &ParseError{
				Line:    nameTok.Line,
				Column:  nameTok.Column,
				Message: "expected variable name after $",
			}
		}
		varName := nameTok.Value
		p.advance()

		// Check for property access: $name.property.subproperty
		if p.current().Type == tokenizer.TokenTypeDot {
			path := make([]string, 0)
			for p.current().Type == tokenizer.TokenTypeDot {
				p.advance() // consume dot
				propTok := p.current()
				if propTok.Type != tokenizer.TokenTypeIdentifier {
					return nil, &ParseError{
						Line:    propTok.Line,
						Column:  propTok.Column,
						Message: "expected property name after .",
					}
				}
				path = append(path, propTok.Value)
				p.advance()
			}
			// Return as PropertyAccessValue with $ prefix to indicate variable
			return ast.PropertyAccessValue{Base: "$" + varName, Path: path}, nil
		}

		return ast.VariableValue{Name: varName}, nil

	case tokenizer.TokenTypeIdentifier:
		// Could be:
		// 1. A reference like agent("name") or file("path")
		// 2. A typed parameter: string required "desc"
		// 3. An inline type definition: enum ["a", "b"]
		// 4. A typed block: http { ... } or shell { ... }
		// 5. A simple identifier used as a value
		// 6. A function call: write_file("path", data), print("msg")
		// 7. auto/infer keywords: auto, auto(min: 2, max: 5), infer
		nextTok := p.peek(1)

		// Check for auto/infer keywords FIRST (before function call check)
		// because auto(min: 2, max: 5) looks like a function call but isn't
		if tok.Value == "auto" || tok.Value == "infer" {
			return p.parseInferredValue(tok.Value)
		}

		if nextTok.Type == tokenizer.TokenTypeLeftParen {
			// Check if this is a known entity type (reference) or a general function call
			if p.isEntityType(tok.Value) {
				return p.parseReference()
			}
			return p.parseFunctionCall()
		}
		// Check if this is a typed parameter: type required/optional [default] ["description"]
		// It must have required/optional after the type name
		if p.isTypeName(tok.Value) && nextTok.Type == tokenizer.TokenTypeIdentifier {
			nextVal := nextTok.Value
			if nextVal == "required" || nextVal == "optional" {
				return p.parseTypedParameter()
			}
		}
		// Check for inline enum type: enum ["a", "b", "c"]
		if tok.Value == "enum" && nextTok.Type == tokenizer.TokenTypeLeftBracket {
			return p.parseInlineEnum()
		}
		// Check for typed block: identifier { ... } (e.g., http { }, shell { }, builtin("..."))
		if nextTok.Type == tokenizer.TokenTypeLeftBrace {
			return p.parseTypedBlock()
		}
		// Check for property access chain: identifier.property.property
		if nextTok.Type == tokenizer.TokenTypeDot {
			return p.parsePropertyAccess()
		}
		// Simple identifier as value (e.g., tool names in array)
		p.advance()
		return ast.StringValue{Value: tok.Value}, nil

	case tokenizer.TokenTypeLeftBracket:
		return p.parseArray()

	case tokenizer.TokenTypeLeftBrace:
		return p.parseObject()

	default:
		return nil, &ParseError{
			Line:    tok.Line,
			Column:  tok.Column,
			Message: fmt.Sprintf("unexpected token in value: %s", tok.Type),
		}
	}
}

// isTypeName checks if an identifier is a type name for typed parameters
func (p *Parser) isTypeName(name string) bool {
	switch name {
	case "string", "number", "bool", "boolean", "array", "object", "enum":
		return true
	}
	return false
}

// isEntityType checks if an identifier is a known entity type that takes a reference (single string arg)
func (p *Parser) isEntityType(name string) bool {
	switch name {
	case "agent", "file", "pipeline", "step", "tool", "handler", "intent", "config", "env", "mcp_server", "mcp", "script":
		return true
	}
	return false
}

// parseFunctionCall parses a function call: identifier(args...)
// Also handles property access after function calls: func().property
func (p *Parser) parseFunctionCall() (ast.Value, *ParseError) {
	funcTok := p.current()
	p.advance() // consume identifier

	args, err := p.parseArgumentList()
	if err != nil {
		return nil, err
	}

	result := ast.FunctionCallValue{
		Function:  funcTok.Value,
		Arguments: args,
	}

	// Check for property access after function call: func().property
	if p.current().Type == tokenizer.TokenTypeDot {
		p.advance() // consume dot
		propTok := p.current()
		if propTok.Type != tokenizer.TokenTypeIdentifier {
			return nil, &ParseError{
				Line:    propTok.Line,
				Column:  propTok.Column,
				Message: "expected property name after .",
			}
		}
		propName := propTok.Value
		p.advance()

		// Check if this is a method call: func().method()
		if p.current().Type == tokenizer.TokenTypeLeftParen {
			methodArgs, err := p.parseArgumentList()
			if err != nil {
				return nil, err
			}
			return ast.MethodCallValue{
				Object:    result,
				Method:    propName,
				Arguments: methodArgs,
			}, nil
		}

		// Property access: func().property
		return ast.MethodCallValue{
			Object:    result,
			Method:    propName,
			Arguments: []ast.Value{},
		}, nil
	}

	return result, nil
}

// parseTypedParameter parses a typed parameter declaration:
// type required/optional [default] ["description"]
// e.g., "string required \"description\"" or "string optional \"default\" \"description\""
func (p *Parser) parseTypedParameter() (ast.Value, *ParseError) {
	typeTok := p.current()
	paramType := typeTok.Value
	p.advance()

	// Next should be "required" or "optional"
	reqTok := p.current()
	if reqTok.Type != tokenizer.TokenTypeIdentifier {
		return nil, &ParseError{
			Line:    reqTok.Line,
			Column:  reqTok.Column,
			Message: "expected 'required' or 'optional' after type",
		}
	}

	var required bool
	switch reqTok.Value {
	case "required":
		required = true
	case "optional":
		required = false
	default:
		return nil, &ParseError{
			Line:    reqTok.Line,
			Column:  reqTok.Column,
			Message: fmt.Sprintf("expected 'required' or 'optional', got '%s'", reqTok.Value),
		}
	}
	p.advance()

	result := ast.TypedParameterValue{
		ParamType: paramType,
		Required:  required,
	}

	// Parse optional default value and/or description
	// For required: string required "description"
	// For optional: string optional "default" "description" OR string optional false "description"
	for p.current().Type == tokenizer.TokenTypeString ||
		p.current().Type == tokenizer.TokenTypeNumber ||
		p.current().Type == tokenizer.TokenTypeBoolean ||
		p.current().Type == tokenizer.TokenTypeLeftBracket {

		tok := p.current()
		switch tok.Type {
		case tokenizer.TokenTypeString:
			p.advance()
			// If we already have a default, this is the description
			// If required is true, first string is description
			// If required is false, first string is default, second is description
			if required {
				result.Description = tok.Value
			} else if result.Default == nil {
				result.Default = ast.StringValue{Value: tok.Value}
			} else {
				result.Description = tok.Value
			}
		case tokenizer.TokenTypeNumber:
			p.advance()
			val, _ := strconv.ParseFloat(tok.Value, 64)
			result.Default = ast.NumberValue{Value: val}
		case tokenizer.TokenTypeBoolean:
			p.advance()
			result.Default = ast.BoolValue{Value: tok.Value == "true"}
		case tokenizer.TokenTypeLeftBracket:
			// Array default or enum values
			if paramType == "enum" {
				enumVals, err := p.parseEnumValues()
				if err != nil {
					return nil, err
				}
				result.EnumValues = enumVals
			} else {
				arrVal, err := p.parseArray()
				if err != nil {
					return nil, err
				}
				result.Default = arrVal
			}
		}
	}

	return result, nil
}

// parseEnumValues parses enum values: ["value1", "value2", ...]
func (p *Parser) parseEnumValues() ([]string, *ParseError) {
	if _, err := p.expect(tokenizer.TokenTypeLeftBracket); err != nil {
		return nil, err
	}

	values := make([]string, 0)

	for p.current().Type != tokenizer.TokenTypeRightBracket {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    0,
				Column:  0,
				Message: "unclosed enum values array",
			}
		}

		tok := p.current()
		if tok.Type != tokenizer.TokenTypeString {
			return nil, &ParseError{
				Line:    tok.Line,
				Column:  tok.Column,
				Message: "expected string in enum values",
			}
		}
		values = append(values, tok.Value)
		p.advance()

		// Optional comma
		if p.current().Type == tokenizer.TokenTypeComma {
			p.advance()
		}
	}

	if _, err := p.expect(tokenizer.TokenTypeRightBracket); err != nil {
		return nil, err
	}

	return values, nil
}

// parseInlineEnum parses an inline enum type: enum ["value1", "value2", ...]
func (p *Parser) parseInlineEnum() (ast.Value, *ParseError) {
	// Consume "enum"
	p.advance()

	values, err := p.parseEnumValues()
	if err != nil {
		return nil, err
	}

	return ast.TypedParameterValue{
		ParamType:  "enum",
		EnumValues: values,
	}, nil
}

// parseInferredValue parses an inferred value: auto, auto(min: 2, max: 5), infer
// This enables runtime inference of MDAP configuration parameters.
func (p *Parser) parseInferredValue(inferType string) (ast.Value, *ParseError) {
	p.advance() // consume "auto" or "infer"

	result := ast.InferredValue{
		InferenceType: inferType,
		Constraints:   make(map[string]ast.Value),
	}

	// Check for optional constraints: auto(min: 2, max: 5)
	if p.current().Type == tokenizer.TokenTypeLeftParen {
		p.advance() // consume (

		for p.current().Type != tokenizer.TokenTypeRightParen {
			if p.pos >= len(p.tokens) {
				return nil, &ParseError{
					Line:    0,
					Column:  0,
					Message: "unclosed auto() constraints",
				}
			}

			// Parse constraint: key: value
			keyTok := p.current()
			if keyTok.Type != tokenizer.TokenTypeIdentifier {
				return nil, &ParseError{
					Line:    keyTok.Line,
					Column:  keyTok.Column,
					Message: "expected constraint name in auto()",
				}
			}
			key := keyTok.Value
			p.advance()

			if _, err := p.expect(tokenizer.TokenTypeColon); err != nil {
				return nil, err
			}

			val, err := p.parsePrimaryValue()
			if err != nil {
				return nil, err
			}

			result.Constraints[key] = val

			// Optional comma
			if p.current().Type == tokenizer.TokenTypeComma {
				p.advance()
			}
		}

		if _, err := p.expect(tokenizer.TokenTypeRightParen); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// parseTypedBlock parses a typed block: identifier { ... } (e.g., http { method: "GET" })
func (p *Parser) parseTypedBlock() (ast.Value, *ParseError) {
	typeTok := p.current()
	blockType := typeTok.Value
	line, col := typeTok.Line, typeTok.Column
	p.advance()

	// Create a nested entity for this block
	nestedValue, err := p.parseNestedEntity(blockType, line, col)
	if err != nil {
		return nil, err
	}

	return nestedValue, nil
}

// parsePropertyAccess parses a property access chain with optional method calls:
// identifier.property.property or identifier.method() or identifier.prop.method(args)
// e.g., params.location, git.staged_files(), github.pr.comment(output)
func (p *Parser) parsePropertyAccess() (ast.Value, *ParseError) {
	baseTok := p.current()
	base := baseTok.Value
	p.advance()

	var result ast.Value
	result = ast.StringValue{Value: base}

	// Parse the property/method chain
	for p.current().Type == tokenizer.TokenTypeDot {
		p.advance() // consume dot
		propTok := p.current()
		if propTok.Type != tokenizer.TokenTypeIdentifier {
			return nil, &ParseError{
				Line:    propTok.Line,
				Column:  propTok.Column,
				Message: "expected property name after .",
			}
		}
		propName := propTok.Value
		p.advance()

		// Check if this is a method call: .method()
		if p.current().Type == tokenizer.TokenTypeLeftParen {
			args, err := p.parseArgumentList()
			if err != nil {
				return nil, err
			}

			mc := ast.MethodCallValue{
				Object:    result,
				Method:    propName,
				Arguments: args,
			}

			// Check for inline block after method call: method() { ... }
			if p.current().Type == tokenizer.TokenTypeLeftBrace {
				nested, nestedErr := p.parseNestedEntity("", 0, 0)
				if nestedErr != nil {
					return nil, nestedErr
				}
				mc.InlineBody = nested.Entity
			}

			result = mc
		} else {
			// Property access
			if pa, ok := result.(ast.PropertyAccessValue); ok {
				pa.Path = append(pa.Path, propName)
				result = pa
			} else if sv, ok := result.(ast.StringValue); ok && sv.Value == base {
				result = ast.PropertyAccessValue{Base: base, Path: []string{propName}}
			} else {
				// Chained property access after method call or other value
				result = ast.MethodCallValue{
					Object:    result,
					Method:    propName,
					Arguments: []ast.Value{},
				}
			}
		}
	}

	// Check for inline block after property access: github.pull_request { ... }
	if p.current().Type == tokenizer.TokenTypeLeftBrace {
		var typeName string
		var object ast.Value

		if pa, ok := result.(ast.PropertyAccessValue); ok {
			typeName = pa.Path[len(pa.Path)-1]
			if len(pa.Path) == 1 {
				object = ast.StringValue{Value: pa.Base}
			} else {
				object = ast.PropertyAccessValue{Base: pa.Base, Path: pa.Path[:len(pa.Path)-1]}
			}
		} else if sv, ok := result.(ast.StringValue); ok {
			typeName = sv.Value
			object = ast.StringValue{Value: ""} // Or some other indicator
		} else if mc, ok := result.(ast.MethodCallValue); ok {
			typeName = mc.Method
			object = mc.Object
		}

		nested, err := p.parseNestedEntity(typeName, baseTok.Line, baseTok.Column)
		if err != nil {
			return nil, err
		}

		if mc, ok := result.(ast.MethodCallValue); ok {
			mc.InlineBody = nested.Entity
			result = mc
		} else {
			result = ast.MethodCallValue{
				Object:     object,
				Method:     typeName,
				Arguments:  []ast.Value{},
				InlineBody: nested.Entity,
			}
		}
	}

	return result, nil
}

// parseArgumentList parses a function argument list: (arg1, arg2, ...)
func (p *Parser) parseArgumentList() ([]ast.Value, *ParseError) {
	if _, err := p.expect(tokenizer.TokenTypeLeftParen); err != nil {
		return nil, err
	}

	args := make([]ast.Value, 0)

	for p.current().Type != tokenizer.TokenTypeRightParen {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    0,
				Column:  0,
				Message: "unclosed argument list",
			}
		}

		arg, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)

		// Optional comma
		if p.current().Type == tokenizer.TokenTypeComma {
			p.advance()
		}
	}

	if _, err := p.expect(tokenizer.TokenTypeRightParen); err != nil {
		return nil, err
	}

	return args, nil
}

// parseNestedEntity parses a nested entity block like: step "name" { ... } or parallel { ... }
func (p *Parser) parseNestedEntity(entityType string, line, col int) (ast.NestedEntityValue, *ParseError) {
	var name string

	// Check if there's a name (string) before the brace
	if p.current().Type == tokenizer.TokenTypeString {
		name = p.current().Value
		p.advance()
	}

	// Create the nested entity
	entity, err := ast.NewEntity(entityType, name)
	if err != nil {
		// If the entity type isn't registered, create a generic entity
		entity = ast.NewBaseEntity(entityType, name)
	}
	entity.SetLocation(line, col)

	// Expect opening brace
	if _, astErr := p.expect(tokenizer.TokenTypeLeftBrace); astErr != nil {
		return ast.NestedEntityValue{}, astErr
	}

	// Parse properties until closing brace
	for p.current().Type != tokenizer.TokenTypeRightBrace {
		if p.pos >= len(p.tokens) {
			return ast.NestedEntityValue{}, &ParseError{
				Line:    line,
				Column:  col,
				Message: "unclosed nested block",
			}
		}

		if propErr := p.parseProperty(entity); propErr != nil {
			return ast.NestedEntityValue{}, propErr
		}
	}

	// Expect closing brace
	if _, astErr := p.expect(tokenizer.TokenTypeRightBrace); astErr != nil {
		return ast.NestedEntityValue{}, astErr
	}

	return ast.NestedEntityValue{Entity: entity}, nil
}

// parseBranch parses a branch control flow: branch expr { "case" => step "name" { ... } }
func (p *Parser) parseBranch(line, col int) (ast.Value, *ParseError) {
	// Parse the condition expression
	condition, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// Expect opening brace
	if _, err := p.expect(tokenizer.TokenTypeLeftBrace); err != nil {
		return nil, err
	}

	cases := make(map[string]ast.NestedEntityValue)

	// Parse case blocks: "value" => step "name" { ... }
	for p.current().Type != tokenizer.TokenTypeRightBrace {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    line,
				Column:  col,
				Message: "unclosed branch block",
			}
		}

		// Expect case value (string)
		caseTok := p.current()
		if caseTok.Type != tokenizer.TokenTypeString {
			return nil, &ParseError{
				Line:    caseTok.Line,
				Column:  caseTok.Column,
				Message: "expected string case value in branch",
			}
		}
		caseValue := caseTok.Value
		p.advance()

		// Expect arrow =>
		if _, err := p.expect(tokenizer.TokenTypeArrow); err != nil {
			return nil, err
		}

		// Parse the nested entity (step "name" { ... })
		entityTok := p.current()
		if entityTok.Type != tokenizer.TokenTypeIdentifier {
			return nil, &ParseError{
				Line:    entityTok.Line,
				Column:  entityTok.Column,
				Message: "expected entity type after => in branch",
			}
		}
		entityType := entityTok.Value
		p.advance()

		nestedEntity, err := p.parseNestedEntity(entityType, entityTok.Line, entityTok.Column)
		if err != nil {
			return nil, err
		}
		cases[caseValue] = nestedEntity
	}

	// Expect closing brace
	if _, err := p.expect(tokenizer.TokenTypeRightBrace); err != nil {
		return nil, err
	}

	return ast.BranchValue{
		Condition: condition,
		Cases:     cases,
	}, nil
}

// parseLoop parses a loop control flow: loop max: N { ... }
func (p *Parser) parseLoop(line, col int) (ast.Value, *ParseError) {
	maxIterations := 0

	// Check for max: N attribute
	if p.current().Type == tokenizer.TokenTypeIdentifier && p.current().Value == "max" {
		p.advance()
		if _, err := p.expect(tokenizer.TokenTypeColon); err != nil {
			return nil, err
		}
		numTok := p.current()
		if numTok.Type != tokenizer.TokenTypeNumber {
			return nil, &ParseError{
				Line:    numTok.Line,
				Column:  numTok.Column,
				Message: "expected number after max:",
			}
		}
		val, _ := strconv.ParseFloat(numTok.Value, 64)
		maxIterations = int(val)
		p.advance()
	}

	// Expect opening brace
	if _, err := p.expect(tokenizer.TokenTypeLeftBrace); err != nil {
		return nil, err
	}

	body := make([]ast.NestedEntityValue, 0)
	var breakCondition ast.Value

	// Parse loop body
	for p.current().Type != tokenizer.TokenTypeRightBrace {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    line,
				Column:  col,
				Message: "unclosed loop block",
			}
		}

		// Check for special loop keywords
		tok := p.current()
		if tok.Type == tokenizer.TokenTypeIdentifier {
			switch tok.Value {
			case "break_if":
				p.advance()
				if _, err := p.expect(tokenizer.TokenTypeColon); err != nil {
					return nil, err
				}
				cond, err := p.parseValue()
				if err != nil {
					return nil, err
				}
				breakCondition = cond
				continue
			case "set":
				// Parse set statements: set $varname: value
				p.advance()
				// Expect $ for variable
				if p.current().Type == tokenizer.TokenTypeDollar {
					p.advance()
				}
				// Skip variable name
				if p.current().Type == tokenizer.TokenTypeIdentifier {
					p.advance()
				}
				// Expect colon
				if p.current().Type == tokenizer.TokenTypeColon {
					p.advance()
				}
				// Parse the value (this consumes the whole expression)
				_, err := p.parseValue()
				if err != nil {
					return nil, err
				}
				continue
			case "step":
				p.advance()
				nestedEntity, err := p.parseNestedEntity("step", tok.Line, tok.Column)
				if err != nil {
					return nil, err
				}
				body = append(body, nestedEntity)
				continue
			}
		}

		// If not a special keyword, skip unknown tokens
		p.advance()
	}

	// Expect closing brace
	if _, err := p.expect(tokenizer.TokenTypeRightBrace); err != nil {
		return nil, err
	}

	return ast.LoopValue{
		MaxIterations:  maxIterations,
		Body:           body,
		BreakCondition: breakCondition,
	}, nil
}

// parseReference parses a reference like agent("name") or step("x").output
func (p *Parser) parseReference() (ast.Value, *ParseError) {
	typeTok := p.current()
	p.advance() // consume identifier

	if _, err := p.expect(tokenizer.TokenTypeLeftParen); err != nil {
		return nil, err
	}

	nameTok := p.current()
	if nameTok.Type != tokenizer.TokenTypeString {
		return nil, &ParseError{
			Line:    nameTok.Line,
			Column:  nameTok.Column,
			Message: "expected string in reference",
		}
	}
	p.advance()

	if _, err := p.expect(tokenizer.TokenTypeRightParen); err != nil {
		return nil, err
	}

	ref := ast.ReferenceValue{
		Type: typeTok.Value,
		Name: nameTok.Value,
		Path: []string{},
	}

	// Check for dot access: .output, .files, etc.
	for p.current().Type == tokenizer.TokenTypeDot {
		p.advance()
		pathTok := p.current()
		if pathTok.Type != tokenizer.TokenTypeIdentifier {
			return nil, &ParseError{
				Line:    pathTok.Line,
				Column:  pathTok.Column,
				Message: "expected property name after .",
			}
		}
		ref.Path = append(ref.Path, pathTok.Value)
		p.advance()
	}

	// Check for inline block: reference("name") { ... }
	// Only allow inline blocks when there's no path (e.g., pipeline("name") { ... })
	// Not for step("x").output { ... } which doesn't make sense
	if len(ref.Path) == 0 && p.current().Type == tokenizer.TokenTypeLeftBrace {
		nested, err := p.parseNestedEntity(ref.Type, typeTok.Line, typeTok.Column)
		if err != nil {
			return nil, err
		}
		// Return as a MethodCallValue with inline body
		return ast.MethodCallValue{
			Object:     ast.StringValue{Value: ref.Type},
			Method:     ref.Name,
			Arguments:  []ast.Value{},
			InlineBody: nested.Entity,
		}, nil
	}

	return ref, nil
}

// parseArray parses an array: [val1, val2, ...]
func (p *Parser) parseArray() (ast.Value, *ParseError) {
	if _, err := p.expect(tokenizer.TokenTypeLeftBracket); err != nil {
		return nil, err
	}

	elements := make([]ast.Value, 0)

	for p.current().Type != tokenizer.TokenTypeRightBracket {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    0,
				Column:  0,
				Message: "unclosed array",
			}
		}

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		elements = append(elements, val)

		// Optional comma
		if p.current().Type == tokenizer.TokenTypeComma {
			p.advance()
		}
	}

	if _, err := p.expect(tokenizer.TokenTypeRightBracket); err != nil {
		return nil, err
	}

	return ast.ArrayValue{Elements: elements}, nil
}

// parseObject parses an object: { key: value, ... }
func (p *Parser) parseObject() (ast.Value, *ParseError) {
	if _, err := p.expect(tokenizer.TokenTypeLeftBrace); err != nil {
		return nil, err
	}

	props := make(map[string]ast.Value)
	var statements []ast.Value

	for p.current().Type != tokenizer.TokenTypeRightBrace {
		if p.pos >= len(p.tokens) {
			return nil, &ParseError{
				Line:    0,
				Column:  0,
				Message: "unclosed object",
			}
		}

		keyTok := p.current()
		if keyTok.Type != tokenizer.TokenTypeIdentifier && keyTok.Type != tokenizer.TokenTypeString {
			return nil, &ParseError{
				Line:    keyTok.Line,
				Column:  keyTok.Column,
				Message: "expected property name in object",
			}
		}

		// Peek ahead to determine if this is a statement expression (identifier.method() or identifier())
		// vs a key-value pair (key: value)
		nextTok := p.peek(1)
		if nextTok.Type == tokenizer.TokenTypeDot || nextTok.Type == tokenizer.TokenTypeLeftParen {
			// This is a statement expression like github.pr.comment(output) or write_file("path", data)
			stmtValue, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			statements = append(statements, stmtValue)
			continue
		}

		key := keyTok.Value
		p.advance()

		if _, err := p.expect(tokenizer.TokenTypeColon); err != nil {
			return nil, err
		}

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		props[key] = val

		// Optional comma
		if p.current().Type == tokenizer.TokenTypeComma {
			p.advance()
		}
	}

	if _, err := p.expect(tokenizer.TokenTypeRightBrace); err != nil {
		return nil, err
	}

	// If we have statements, store them in a special property
	if len(statements) > 0 {
		props["_statements"] = ast.ArrayValue{Elements: statements}
	}

	return ast.ObjectValue{Properties: props}, nil
}

// parseLegacyEntity handles the old single-line syntax for backward compatibility
func (p *Parser) parseLegacyEntity(entityType, name string, line, col int) (ast.Entity, *ParseError) {
	entity, err := ast.NewEntity(entityType, name)
	if err != nil {
		return nil, &ParseError{Line: line, Column: col, Message: err.Error()}
	}
	entity.SetLocation(line, col)

	// For legacy syntax, the name becomes a property
	entity.SetProperty("name", ast.StringValue{Value: name})

	// Check for property identifier
	tok := p.current()
	if tok.Type == tokenizer.TokenTypeIdentifier {
		propName := tok.Value
		p.advance()

		// Check for multiline string
		nextTok := p.current()
		if nextTok.Type == tokenizer.TokenTypeMultilineString {
			content := nextTok.Value
			if len(content) > 0 && content[0] == '\n' {
				content = content[1:]
			}
			entity.SetProperty(propName, ast.StringValue{Value: content})
			p.advance()
		} else {
			entity.SetProperty("property", ast.StringValue{Value: propName})
		}
	}

	// Expect semicolon
	if p.current().Type == tokenizer.TokenTypeSemicolon {
		p.advance()
	}

	return entity, nil
}

// Parse processes the input text and returns a slice of parsed entities.
func (p *Parser) Parse() ([]ast.Entity, error) {
	result := p.ParseWithRecovery()
	if result.HasErrors() {
		return nil, result.Errors[0]
	}
	return result.Entities, nil
}
