package generator

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sashabaranov/go-openai"
)

// Properties to ignore when generating tools
var ignoredProperties = map[string]bool{
	"Anytype-Version": true, // Anytype version header
	"space_id":        true, // Space ID parameter
	"template_id":     true, // Template ID parameter
}

// SanitizeName converts a string into a valid function name by replacing non-alphanumeric chars with underscores
func SanitizeName(name string) string {
	// replace non-alphanum with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	sanitized := re.ReplaceAllString(name, "_")

	// Trim leading/trailing underscores and convert to lowercase
	sanitized = strings.Trim(sanitized, "_")
	return strings.ToLower(sanitized)
}

// EndpointInfo contains information about the API endpoint
type EndpointInfo struct {
	Method          string
	Path            string
	OriginalSummary string
	OperationId     string
	Schema          map[string]interface{} // Added to store the schema
}

// ResolveSchemaRef resolves a reference to a schema in the OpenAPI spec
func ResolveSchemaRef(ref string, spec *openapi3.T) (*openapi3.Schema, error) {
	// Extract the component path from the reference
	// Expecting format like "#/components/schemas/object.CreateObjectRequest"
	if !strings.HasPrefix(ref, "#/components/schemas/") {
		return nil, fmt.Errorf("unsupported reference format: %s", ref)
	}

	// Get the schema name from the reference
	schemaName := strings.TrimPrefix(ref, "#/components/schemas/")

	// Look up the schema in the components section
	schema, exists := spec.Components.Schemas[schemaName]
	if !exists {
		return nil, fmt.Errorf("schema not found: %s", schemaName)
	}

	return schema.Value, nil
}

// SchemaToMap converts an OpenAPI schema to a map for tool parameters
func SchemaToMap(schema *openapi3.Schema, spec *openapi3.T) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Set basic properties
	if schema.Type != nil && len(*schema.Type) > 0 {
		// Handle the type properly based on the OpenAPI schema
		typeValue := (*schema.Type)[0] // Get the first type (OpenAPI can have multiple types)
		result["type"] = typeValue

		// For arrays, add the items property
		if typeValue == "array" && schema.Items != nil {
			if schema.Items.Value != nil {
				// Convert items schema
				itemsMap, err := SchemaToMap(schema.Items.Value, spec)
				if err != nil {
					// Fallback to simple type
					result["items"] = map[string]interface{}{
						"type": "string", // Default fallback for items
					}
				} else {
					result["items"] = itemsMap
				}
			} else if schema.Items.Ref != "" {
				// Handle reference for items
				refSchema, err := ResolveSchemaRef(schema.Items.Ref, spec)
				if err != nil {
					// Fallback to simple type
					result["items"] = map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("Reference to %s", schema.Items.Ref),
					}
				} else {
					itemsMap, err := SchemaToMap(refSchema, spec)
					if err != nil {
						result["items"] = map[string]interface{}{
							"type": "string",
						}
					} else {
						result["items"] = itemsMap
					}
				}
			} else {
				// Default items type if nothing specified
				result["items"] = map[string]interface{}{
					"type": "string",
				}
			}
		}
	} else {
		// Default to object if type is not specified
		result["type"] = "object"
	}

	if schema.Description != "" {
		result["description"] = schema.Description
	}

	// Handle properties
	if len(schema.Properties) > 0 {
		properties := make(map[string]interface{})
		for propName, propSchema := range schema.Properties {
			// Handle property reference
			if propSchema.Ref != "" {
				refSchema, err := ResolveSchemaRef(propSchema.Ref, spec)
				if err != nil {
					// If reference resolution fails, use a simple type
					properties[propName] = map[string]interface{}{
						"type":        "object",
						"description": fmt.Sprintf("Reference to %s", propSchema.Ref),
					}
				} else {
					propMap, err := SchemaToMap(refSchema, spec)
					if err != nil {
						// If conversion fails, use a simple type
						properties[propName] = map[string]interface{}{
							"type": "object",
						}
					} else {
						properties[propName] = propMap
					}
				}
			} else if propSchema.Value != nil {
				// Direct schema
				propMap, err := SchemaToMap(propSchema.Value, spec)
				if err != nil {
					// If conversion fails, use a simple type with the correct type format
					propType := "object"
					if propSchema.Value.Type != nil && len(*propSchema.Value.Type) > 0 {
						propType = (*propSchema.Value.Type)[0]
					}
					properties[propName] = map[string]interface{}{
						"type": propType,
					}
				} else {
					properties[propName] = propMap
				}
			}
		}
		result["properties"] = properties
	}

	// Handle required fields
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Handle enums
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	return result, nil
}

// Globals to store endpoint information and OpenAPI spec
var (
	// EndpointRegistry maps tool names to endpoint info
	EndpointRegistry map[string]*EndpointInfo

	// OpenAPISpec is the loaded OpenAPI specification
	OpenAPISpec *openapi3.T
)

// LoadSpec loads the OpenAPI specification from a file
func LoadSpec(filePath string) (*openapi3.T, error) {
	spec, err := openapi3.NewLoader().LoadFromFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load spec: %v", err)
	}
	OpenAPISpec = spec
	return spec, nil
}

// GenerateToolParameters processes an endpoint and returns the tool parameters
func GenerateToolParameters(method string, path string, op *openapi3.Operation, spec *openapi3.T) (map[string]interface{}, error) {
	// Build JSON Schema parameters
	params := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}

	// Add path and query parameters
	for _, p := range op.Parameters {
		// Skip ignored properties
		if ignoredProperties[p.Value.Name] {
			continue
		}

		ps := p.Value.Schema.Value
		prop := map[string]interface{}{}

		// Handle schema type properly
		if ps.Type != nil && len(*ps.Type) > 0 {
			typeValue := (*ps.Type)[0] // Get the first type
			prop["type"] = typeValue

			// Handle array type with items
			if typeValue == "array" && ps.Items != nil {
				if ps.Items.Value != nil {
					// Simple case - item has a type
					if ps.Items.Value.Type != nil && len(*ps.Items.Value.Type) > 0 {
						prop["items"] = map[string]interface{}{
							"type": (*ps.Items.Value.Type)[0],
						}
					} else {
						// Default item type
						prop["items"] = map[string]interface{}{
							"type": "string",
						}
					}
				} else if ps.Items.Ref != "" {
					// Reference to another schema
					prop["items"] = map[string]interface{}{
						"type":        "object",
						"description": fmt.Sprintf("Reference to %s", ps.Items.Ref),
					}
				} else {
					// Default item type if not specified
					prop["items"] = map[string]interface{}{
						"type": "string",
					}
				}
			}
		} else {
			prop["type"] = "object"
		}

		// Use x-ai-description if available, otherwise use description
		description := p.Value.Description
		if aiDesc, ok := p.Value.Extensions["x-ai-description"]; ok {
			if strDesc, ok := aiDesc.(string); ok {
				description = strDesc
			}
		}

		if description != "" {
			prop["description"] = description
		}

		// Handle enum values
		if ps.Enum != nil && len(ps.Enum) > 0 {
			prop["enum"] = ps.Enum
		}

		params["properties"].(map[string]interface{})[p.Value.Name] = prop
		if p.Value.Required {
			params["required"] = append(params["required"].([]string), p.Value.Name)
		}
	}

	// If there is a requestBody, include it as a `body` property
	if op.RequestBody != nil {
		// Extract the actual schema from the request body
		for contentType, mediaType := range op.RequestBody.Value.Content {
			// Prefer application/json if available
			if contentType == "application/json" && mediaType.Schema != nil {
				// Extract and process the schema
				var bodyProp map[string]interface{}

				// If it's a reference, resolve it
				if mediaType.Schema.Ref != "" {
					refSchema, err := ResolveSchemaRef(mediaType.Schema.Ref, spec)
					if err != nil {
						log.Printf("Warning: Could not resolve schema reference %s: %v", mediaType.Schema.Ref, err)
						// If we can't resolve, create a simple object placeholder
						bodyProp = map[string]interface{}{
							"type":        "object",
							"description": fmt.Sprintf("Reference to %s", mediaType.Schema.Ref),
						}
					} else {
						// Convert the resolved schema to a map
						bodyProp, err = SchemaToMap(refSchema, spec)
						if err != nil {
							log.Printf("Warning: Could not convert schema to map: %v", err)
							bodyProp = map[string]interface{}{
								"type": "object",
							}
						}
					}
				} else if mediaType.Schema.Value != nil {
					// Direct schema
					var err error
					bodyProp, err = SchemaToMap(mediaType.Schema.Value, spec)
					if err != nil {
						log.Printf("Warning: Could not convert schema to map: %v", err)

						// Create a fallback type
						bodyProp = map[string]interface{}{
							"type": "object",
						}

						// Try to use the schema type if available
						if mediaType.Schema.Value.Type != nil && len(*mediaType.Schema.Value.Type) > 0 {
							bodyProp["type"] = (*mediaType.Schema.Value.Type)[0]

							// Handle array type
							typeValue := (*mediaType.Schema.Value.Type)[0]
							if typeValue == "array" && mediaType.Schema.Value.Items != nil {
								// Add items definition
								bodyProp["items"] = map[string]interface{}{
									"type": "string", // Default fallback
								}
							}
						}
					}
				}

				// Instead of adding a 'body' field, merge the body properties into the top level
				if bodyProp != nil && bodyProp["type"] == "object" && bodyProp["properties"] != nil {
					// Extract body properties
					bodyProperties, ok := bodyProp["properties"].(map[string]interface{})
					if ok {
						// Merge body properties into top-level properties
						properties := params["properties"].(map[string]interface{})
						for propName, propValue := range bodyProperties {
							properties[propName] = propValue
						}

						// If the body schema has required fields, add them to the top level
						if bodyRequired, ok := bodyProp["required"].([]string); ok && len(bodyRequired) > 0 {
							params["required"] = append(params["required"].([]string), bodyRequired...)
						}
					} else {
						log.Printf("Warning: Could not extract properties from body schema, schema structure unexpected")
					}
				} else {
					// For non-object body types (like arrays), add a special field indicating the request body
					log.Printf("Warning: Request body is not an object type, adding as requestBody field")
					params["properties"].(map[string]interface{})["requestBody"] = bodyProp
					params["required"] = append(params["required"].([]string), "requestBody")
				}

				break
			}
		}

		// If we couldn't extract any properties, add a generic requestBody field
		// Check if we added any properties
		if len(params["properties"].(map[string]interface{})) == 0 {
			params["properties"].(map[string]interface{})["requestBody"] = map[string]interface{}{
				"type":        "object",
				"description": "Request body for this endpoint",
			}
			params["required"] = append(params["required"].([]string), "requestBody")
		}
	}

	return params, nil
}

// GenerateTools generates OpenAI tools from the OpenAPI spec
func GenerateTools(spec *openapi3.T) ([]*struct {
	Tool   openai.Tool
	Method string
	Path   string
}, error) {
	// Prepare tools slice and name registry to check for uniqueness
	var toolsInfo []*struct {
		Tool   openai.Tool
		Method string
		Path   string
	}
	EndpointRegistry = make(map[string]*EndpointInfo)

	// Iterate over all paths and methods
	for path, item := range spec.Paths.Map() {
		operations := item.Operations()
		for method, op := range operations {
			// Skip if x-ai-omit is set to true
			if omit, ok := op.Responses.Extensions["x-ai-omit"]; ok {
				delete(op.Responses.Extensions, "x-ai-omit")
				if omitBool, ok := omit.(bool); ok && omitBool {
					log.Printf("Skipping %s %s because x-ai-omit is set to true", method, path)
					continue
				}
			}

			// Check if operationId is available and use it, otherwise fall back to summary
			var nameSource string
			var nameOrigin string

			if op.OperationID != "" {
				nameSource = op.OperationID
				nameOrigin = "operationId"
			} else if op.Summary != "" {
				nameSource = op.Summary
				nameOrigin = "summary"
			} else {
				log.Printf("Warning: Skipping %s %s because it has neither operationId nor summary", method, path)
				continue
			}

			// Use operationId or summary for the tool name
			toolName := SanitizeName(nameSource)
			if toolName == "" {
				log.Printf("Warning: Skipping %s %s because sanitized %s is empty", method, path, nameOrigin)
				continue
			}

			// Check for uniqueness
			if existing, exists := EndpointRegistry[toolName]; exists {
				var existingSource, newSource string

				if existing.OperationId != "" {
					existingSource = fmt.Sprintf("operationId: %s", existing.OperationId)
				} else {
					existingSource = fmt.Sprintf("summary: %s", existing.OriginalSummary)
				}

				if op.OperationID != "" {
					newSource = fmt.Sprintf("operationId: %s", op.OperationID)
				} else {
					newSource = fmt.Sprintf("summary: %s", op.Summary)
				}

				log.Fatalf("Error: Duplicate tool name '%s' generated from endpoints:\n  1. %s %s (%s)\n  2. %s %s (%s)\nPlease make operationIds/summaries more unique in the OpenAPI spec.",
					toolName,
					existing.Method, existing.Path, existingSource,
					method, path, newSource)
			}

			// Generate the tool parameters
			params, err := GenerateToolParameters(method, path, op, spec)
			if err != nil {
				log.Printf("Warning: Error generating parameters for %s %s: %v", method, path, err)
				continue
			}

			// Use x-ai-description if available, otherwise use description
			description := op.Description
			if aiDesc, ok := op.Responses.Extensions["x-ai-description"]; ok {
				delete(op.Responses.Extensions, "x-ai-description")

				if strDesc, ok := aiDesc.(string); ok {
					description = strDesc
				}
			}

			// Register the tool name with its endpoint info
			EndpointRegistry[toolName] = &EndpointInfo{
				Method:          method,
				Path:            path,
				OriginalSummary: op.Summary,
				OperationId:     op.OperationID,
				Schema:          params,
			}

			// Create the function definition
			f := openai.FunctionDefinition{
				Name:        toolName,
				Description: description,
				Parameters:  params,
			}

			// Create a tool and add it to our slice with its endpoint info
			toolsInfo = append(toolsInfo, &struct {
				Tool   openai.Tool
				Method string
				Path   string
			}{
				Tool: openai.Tool{
					Type:     openai.ToolTypeFunction,
					Function: &f,
				},
				Method: method,
				Path:   path,
			})
		}
	}

	return toolsInfo, nil
}

// GetEndpointSchema returns the schema for a specific endpoint by name
func GetEndpointSchema(name string) (map[string]interface{}, error) {
	if EndpointRegistry == nil {
		return nil, fmt.Errorf("endpoints not loaded, call LoadSpec and GenerateTools first")
	}

	endpoint, exists := EndpointRegistry[name]
	if !exists {
		return nil, fmt.Errorf("endpoint %s not found", name)
	}

	return endpoint.Schema, nil
}

// CreateTool creates an OpenAI tool from a schema
func CreateTool(name string, description string, schema map[string]interface{}) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  schema,
		},
	}
}

// GetTool returns an OpenAI tool for a specific endpoint by name
func GetTool(name string) (openai.Tool, error) {
	if EndpointRegistry == nil {
		return openai.Tool{}, fmt.Errorf("endpoints not loaded, call LoadSpec and GenerateTools first")
	}

	endpoint, exists := EndpointRegistry[name]
	if !exists {
		return openai.Tool{}, fmt.Errorf("endpoint %s not found", name)
	}

	// Check if OpenAPISpec is loaded
	if OpenAPISpec == nil {
		return openai.Tool{}, fmt.Errorf("OpenAPI spec not loaded, call LoadSpec first")
	}

	// Get description for this endpoint from the OpenAPI spec
	path := OpenAPISpec.Paths.Find(endpoint.Path)
	if path == nil {
		return openai.Tool{}, fmt.Errorf("path %s not found in spec", endpoint.Path)
	}

	var description string
	switch strings.ToUpper(endpoint.Method) {
	case "GET":
		if path.Get != nil {
			description = path.Get.Description
		}
	case "POST":
		if path.Post != nil {
			description = path.Post.Description
		}
	case "PUT":
		if path.Put != nil {
			description = path.Put.Description
		}
	case "DELETE":
		if path.Delete != nil {
			description = path.Delete.Description
		}
	case "PATCH":
		if path.Patch != nil {
			description = path.Patch.Description
		}
	}

	return CreateTool(name, description, endpoint.Schema), nil
}
