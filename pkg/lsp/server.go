package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/shellkjell/langspace/pkg/ast"
	"github.com/shellkjell/langspace/pkg/parser"
	"github.com/shellkjell/langspace/pkg/workspace"
)

// Server handles LSP requests.
type Server struct {
	workspace *workspace.Workspace
	files     map[string]string
	mu        sync.RWMutex
}

// NewServer creates a new LSP server.
func NewServer() *Server {
	return &Server{
		workspace: workspace.New(),
		files:     make(map[string]string),
	}
}

// Start starts the LSP server on stdin/stdout.
func (s *Server) Start() error {
	log.Println("LangSpace LSP starting...")
	dec := json.NewDecoder(os.Stdin)
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		go s.handleRequest(req)
	}
	return nil
}

type Request struct {
	ID     interface{}     `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

func (s *Server) handleRequest(req Request) {
	var result interface{}
	var err error

	switch req.Method {
	case "initialize":
		result = map[string]interface{}{
			"capabilities": map[string]interface{}{
				"textDocumentSync":   1, // Full sync
				"definitionProvider": true,
			},
		}
	case "textDocument/didOpen":
		err = s.handleDidOpen(req.Params)
	case "textDocument/didChange":
		err = s.handleDidChange(req.Params)
	case "textDocument/definition":
		result, err = s.handleDefinition(req.Params)
	}

	if req.ID != nil {
		s.sendResponse(req.ID, result, err)
	}
}

func (s *Server) sendResponse(id interface{}, result interface{}, err error) {
	resp := Response{ID: id, Result: result}
	if err != nil {
		resp.Error = map[string]string{"message": err.Error()}
	}
	data, _ := json.Marshal(resp)
	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (s *Server) handleDidOpen(params json.RawMessage) error {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}
	s.mu.Lock()
	s.files[p.TextDocument.URI] = p.TextDocument.Text
	s.mu.Unlock()
	return s.reindex()
}

func (s *Server) handleDidChange(params json.RawMessage) error {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}
	if len(p.ContentChanges) > 0 {
		s.mu.Lock()
		s.files[p.TextDocument.URI] = p.ContentChanges[0].Text
		s.mu.Unlock()
	}
	return s.reindex()
}

func (s *Server) reindex() error {
	// Re-build workspace from all open files
	s.mu.RLock()
	defer s.mu.RUnlock()

	newWS := workspace.New()
	for uri, content := range s.files {
		p := parser.New(content)
		entities, err := p.Parse()
		if err != nil {
			log.Printf("indexing error for %s: %v", uri, err)
			continue
		}
		for _, e := range entities {
			// Note: We need to store URI in metadata to allow "Go to Definition" to return correct file
			e.SetMetadata("uri", uri)
			if err := newWS.AddEntity(e); err != nil {
				log.Printf("failed to add entity %s: %v", e.Name(), err)
			}
		}
	}
	s.workspace = newWS
	return nil
}

func (s *Server) handleDefinition(params json.RawMessage) (interface{}, error) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	s.mu.RLock()
	content, ok := s.files[p.TextDocument.URI]
	ws := s.workspace
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("file not found: %s", p.TextDocument.URI)
	}

	// Simple heuristic: find the word at the position
	lines := strings.Split(content, "\n")
	if p.Position.Line >= len(lines) {
		return nil, nil
	}
	line := lines[p.Position.Line]
	if p.Position.Character >= len(line) {
		return nil, nil
	}

	// Extract identifier around position
	start := p.Position.Character
	for start > 0 && isIdentChar(line[start-1]) {
		start--
	}
	end := p.Position.Character
	for end < len(line) && isIdentChar(line[end]) {
		end++
	}
	symbol := line[start:end]
	if symbol == "" {
		return nil, nil
	}

	// Look for entity with this name
	// This is a broad search; could be refined by checking surrounding context (agent(...), step(...))
	var targetEntity ast.Entity
	for _, e := range ws.GetEntities() {
		if e.Name() == symbol {
			targetEntity = e
			break
		}
	}

	if targetEntity == nil {
		return nil, nil
	}

	uri, _ := targetEntity.GetMetadata("uri")
	if uri == "" {
		return nil, nil
	}

	return map[string]interface{}{
		"uri": uri,
		"range": map[string]interface{}{
			"start": map[string]int{"line": targetEntity.Line() - 1, "character": targetEntity.Column() - 1},
			"end":   map[string]int{"line": targetEntity.Line() - 1, "character": targetEntity.Column() + 10},
		},
	}, nil
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
}
