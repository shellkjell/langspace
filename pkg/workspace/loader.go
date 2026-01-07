package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shellkjell/langspace/pkg/parser"
)

// Loader handles loading LangSpace files and their dependencies into a workspace.
type Loader struct {
	workspace *Workspace
	loaded    map[string]bool
	baseDir   string
}

// NewLoader creates a new Loader instance for the given workspace.
func NewLoader(ws *Workspace) *Loader {
	return &Loader{
		workspace: ws,
		loaded:    make(map[string]bool),
	}
}

// Load loads a LangSpace file and all its imported dependencies.
func (l *Loader) Load(filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}

	if l.loaded[absPath] {
		return nil
	}

	l.loaded[absPath] = true

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", absPath, err)
	}

	l.baseDir = filepath.Dir(absPath)

	p := parser.New(string(content))
	entities, err := p.Parse()
	if err != nil {
		return fmt.Errorf("parse error in %s: %w", absPath, err)
	}

	// Add entities to workspace
	for _, entity := range entities {
		if err := l.workspace.AddEntity(entity); err != nil {
			return fmt.Errorf("failed to add entity %q from %s: %w", entity.Name(), absPath, err)
		}
	}

	// TODO: Implement import resolution when import syntax is added
	// Currently imports are not supported by the parser

	return nil
}
