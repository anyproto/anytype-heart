package runtime

import (
	"fmt"
	"regexp"
	"strings"

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

		// Prepend "export {};" to ensure QuickJS detects this as a module
		source = "export {};\n" + source

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

// importRegex matches ES6 import statements: import ... from "module" or import "module"
var importRegex = regexp.MustCompile(`import\s+(?:[^"']+\s+from\s+)?["']([^"']+)["']`)

// stripComments removes JS comments from source code to avoid matching imports in comments
func stripComments(source string) string {
	result := make([]byte, 0, len(source))
	i := 0
	for i < len(source) {
		// Check for single-line comment
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '/' {
			// Skip until end of line
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		// Check for multi-line comment
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '*' {
			i += 2
			// Skip until */
			for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
				i++
			}
			i += 2 // skip */
			continue
		}
		// Check for string literals (to avoid matching // or /* inside strings)
		if source[i] == '"' || source[i] == '\'' || source[i] == '`' {
			quote := source[i]
			result = append(result, source[i])
			i++
			for i < len(source) && source[i] != quote {
				if source[i] == '\\' && i+1 < len(source) {
					result = append(result, source[i], source[i+1])
					i += 2
				} else {
					result = append(result, source[i])
					i++
				}
			}
			if i < len(source) {
				result = append(result, source[i])
				i++
			}
			continue
		}
		result = append(result, source[i])
		i++
	}
	return string(result)
}

// parseImports extracts all module specifiers from import statements in JS code
// It strips comments first to avoid matching imports inside comments
func parseImports(source string) []string {
	// Strip comments to avoid matching import examples in JSDoc
	stripped := stripComments(source)
	matches := importRegex.FindAllStringSubmatch(stripped, -1)
	seen := make(map[string]bool)
	var imports []string
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			seen[match[1]] = true
			imports = append(imports, match[1])
		}
	}
	return imports
}

// PreloadModuleRecursively loads a module and all its dependencies into the context
// It tracks already loaded modules to avoid duplicates
func PreloadModuleRecursively(ctx *quickjs.Context, loader ModuleLoader, specifier string, loaded map[string]bool) error {
	// Skip if already loaded
	if loaded[specifier] {
		return nil
	}

	// Load the module source
	source, err := loader.Load(specifier)
	if err != nil {
		return fmt.Errorf("load module %q: %w", specifier, err)
	}

	// Mark as loaded before processing dependencies to handle circular imports
	loaded[specifier] = true

	// Parse and recursively load all imports
	imports := parseImports(source)
	for _, imp := range imports {
		if err := PreloadModuleRecursively(ctx, loader, imp, loaded); err != nil {
			return err
		}
	}

	// Clean the source: strip comments and trim whitespace
	// This is necessary because QuickJS has issues with certain source formats
	source = strings.TrimSpace(source)

	// Prepend "export {};" to ensure QuickJS detects this as a module
	// This is needed because JS_DetectModule looks for import/export at the start
	source = "export {};\n" + source

	// Now load this module into QuickJS context
	result := ctx.LoadModule(source, specifier, quickjs.EvalLoadOnly(true))
	if result.IsException() {
		result.Free()
		return fmt.Errorf("preload module %q: %w", specifier, ctx.Exception())
	}
	result.Free()

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
