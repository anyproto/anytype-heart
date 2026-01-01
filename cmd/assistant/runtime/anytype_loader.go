package runtime

import (
	"context"
	"fmt"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	apiservice "github.com/anyproto/anytype-heart/core/api/service"
)

// AnytypeModuleLoader loads JS modules from Anytype objects
// Objects must have __anytype_program_name and __anytype_program_version properties
// Import syntax: import { fn } from "program_name@version"
type AnytypeModuleLoader struct {
	ctx            context.Context
	apiService     *apiservice.Service
	currentSpaceId string
	builtins       map[string]string // built-in modules like "openai"
}

// NewAnytypeModuleLoader creates a new Anytype-based module loader
func NewAnytypeModuleLoader(ctx context.Context, apiService *apiservice.Service, currentSpaceId string) *AnytypeModuleLoader {
	return &AnytypeModuleLoader{
		ctx:            ctx,
		apiService:     apiService,
		currentSpaceId: currentSpaceId,
		builtins:       make(map[string]string),
	}
}

// Register allows pre-registering built-in modules (like openai)
func (l *AnytypeModuleLoader) Register(name, source string) {
	l.builtins[name] = source
}

// Load implements ModuleLoader - loads a module by name@version specifier
// Built-in modules are loaded from memory, Anytype programs are always fetched fresh
func (l *AnytypeModuleLoader) Load(specifier string) (string, error) {
	// Check built-in modules first
	if source, ok := l.builtins[specifier]; ok {
		return source, nil
	}

	// Parse specifier: "name@version" or just "name"
	name, version := parseSpecifier(specifier)

	// Search for the program object (always fetch fresh, no caching)
	object, err := l.findProgramObject(name, version)
	if err != nil {
		return "", err
	}

	// Extract JS code from the object's markdown body
	source := extractJSFromMarkdown(object.Markdown)
	if source == "" {
		return "", fmt.Errorf("no valid JS source found in %q (must have // __main_source marker)", specifier)
	}

	return source, nil
}

// Names implements ModuleLoader - returns all registered built-in module names
func (l *AnytypeModuleLoader) Names() []string {
	names := make([]string, 0, len(l.builtins))
	for name := range l.builtins {
		names = append(names, name)
	}
	return names
}

// parseSpecifier splits "name@version" into name and version
// If no version specified, version is empty (matches any)
func parseSpecifier(specifier string) (name, version string) {
	parts := strings.SplitN(specifier, "@", 2)
	name = parts[0]
	if len(parts) > 1 {
		version = parts[1]
	}
	return
}

// getPropertyTextValue extracts the text value from a property with the given name
func getPropertyTextValue(props []apimodel.PropertyWithValue, propName string) string {
	for _, prop := range props {
		// Properties are wrapped types, we need to check if it's a TextPropertyValue
		if textProp, ok := prop.WrappedPropertyWithValue.(apimodel.TextPropertyValue); ok {
			if textProp.Name == propName {
				return textProp.Text
			}
		}
	}
	return ""
}

// findProgramObject searches for an object with matching program name and version
// Uses iteration over all objects since filter queries with custom properties are not reliable
func (l *AnytypeModuleLoader) findProgramObject(name, version string) (*apimodel.ObjectWithBody, error) {
	// List all objects and find the matching one by checking properties
	allObjects, _, _, err := l.apiService.ListObjects(l.ctx, l.currentSpaceId, nil, 0, 100)
	if err != nil {
		return nil, fmt.Errorf("list objects failed: %w", err)
	}

	for _, obj := range allObjects {
		// Get full object with properties
		fullObj, err := l.apiService.GetObject(l.ctx, l.currentSpaceId, obj.Id)
		if err != nil {
			continue
		}

		// Check if program name matches
		progName := getPropertyTextValue(fullObj.Properties, "__anytype_program_name")
		if progName != name {
			continue
		}

		// Check version if specified
		if version != "" {
			progVersion := getPropertyTextValue(fullObj.Properties, "__anytype_program_version")
			if progVersion != version {
				continue
			}
		}

		// Found it!
		return fullObj, nil
	}

	if version != "" {
		return nil, fmt.Errorf("program not found: %s@%s", name, version)
	}
	return nil, fmt.Errorf("program not found: %s", name)
}

// extractJSFromMarkdown extracts JavaScript code from markdown code blocks
// Looks for the code block that starts with "// __main_source" marker
// Supports: ```javascript, ```js, ``` (plain)
func extractJSFromMarkdown(markdown string) string {
	const mainSourceMarker = "// __main_source"
	const codeBlockEnd = "```"

	remaining := markdown
	for {
		// Find any code block start
		start := strings.Index(remaining, "```")
		if start == -1 {
			break
		}

		// Find the newline after the opening ```
		codeStart := start + 3
		// Skip optional language specifier until newline
		for codeStart < len(remaining) && remaining[codeStart] != '\n' {
			codeStart++
		}
		if codeStart < len(remaining) && remaining[codeStart] == '\n' {
			codeStart++
		}

		// Find the closing ```
		end := strings.Index(remaining[codeStart:], codeBlockEnd)
		if end == -1 {
			break
		}

		codeBlock := remaining[codeStart : codeStart+end]

		// Check if this block starts with the __main_source marker
		trimmed := strings.TrimSpace(codeBlock)
		if strings.HasPrefix(trimmed, mainSourceMarker) {
			// Remove the marker line and return the rest
			lines := strings.SplitN(trimmed, "\n", 2)
			if len(lines) > 1 {
				return strings.TrimSpace(lines[1])
			}
			return ""
		}

		// Move past this code block
		remaining = remaining[codeStart+end+3:]
	}

	return ""
}
