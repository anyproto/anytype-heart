package runtime

import (
	"fmt"

	"github.com/buke/quickjs-go"
)

// ModuleLoader is an interface for resolving and loading JS modules
type ModuleLoader interface {
	// Load takes a module name and returns the module source code
	Load(name string) (string, error)

	// Names returns all registered module names
	Names() []string
}

// PreloadModules loads all modules from the loader into the context
func PreloadModules(ctx *quickjs.Context, loader ModuleLoader) error {
	for _, name := range loader.Names() {
		source, err := loader.Load(name)
		if err != nil {
			return fmt.Errorf("load module %q: %w", name, err)
		}

		// Load module with EvalLoadOnly to register it without executing
		result := ctx.LoadModule(source, name, quickjs.EvalLoadOnly(true))
		if result.IsException() {
			result.Free()
			return fmt.Errorf("preload module %q: %w", name, ctx.Exception())
		}
		result.Free()
	}
	return nil
}

// MapModuleLoader is a simple in-memory module loader using a map
type MapModuleLoader struct {
	modules map[string]string
	order   []string // preserve registration order
}

// NewMapModuleLoader creates a new map-based module loader
func NewMapModuleLoader() *MapModuleLoader {
	return &MapModuleLoader{
		modules: make(map[string]string),
		order:   []string{},
	}
}

// Register adds a module to the loader
func (m *MapModuleLoader) Register(name, source string) {
	if _, exists := m.modules[name]; !exists {
		m.order = append(m.order, name)
	}
	m.modules[name] = source
}

// Load implements ModuleLoader
func (m *MapModuleLoader) Load(name string) (string, error) {
	source, ok := m.modules[name]
	if !ok {
		return "", fmt.Errorf("module not found: %s", name)
	}
	return source, nil
}

// Names implements ModuleLoader
func (m *MapModuleLoader) Names() []string {
	return m.order
}
