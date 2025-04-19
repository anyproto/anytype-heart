package main

// openAI tool generator from openAPI yaml
import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// sanitizeName converts a string into a valid function name by replacing non-alphanumeric chars with underscores
func sanitizeName(name string) string {
	// replace non-alphanum with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	sanitized := re.ReplaceAllString(name, "_")

	// Trim leading/trailing underscores and convert to lowercase
	sanitized = strings.Trim(sanitized, "_")
	return strings.ToLower(sanitized)
}

// endpointInfo contains information about the API endpoint
type endpointInfo struct {
	Method          string
	Path            string
	OriginalSummary string
	OperationId     string
}

// Add a new helper function to resolve schema references
func resolveSchemaRef(ref string, spec *openapi3.T) (*openapi3.Schema, error) {
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

// Helper function to convert openapi3.Schema to a map representation for the tool parameters
func schemaToMap(schema *openapi3.Schema, spec *openapi3.T) (map[string]interface{}, error) {
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
				itemsMap, err := schemaToMap(schema.Items.Value, spec)
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
				refSchema, err := resolveSchemaRef(schema.Items.Ref, spec)
				if err != nil {
					// Fallback to simple type
					result["items"] = map[string]interface{}{
						"type":        "string",
						"description": fmt.Sprintf("Reference to %s", schema.Items.Ref),
					}
				} else {
					itemsMap, err := schemaToMap(refSchema, spec)
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
				refSchema, err := resolveSchemaRef(propSchema.Ref, spec)
				if err != nil {
					// If reference resolution fails, use a simple type
					properties[propName] = map[string]interface{}{
						"type":        "object",
						"description": fmt.Sprintf("Reference to %s", propSchema.Ref),
					}
				} else {
					propMap, err := schemaToMap(refSchema, spec)
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
				propMap, err := schemaToMap(propSchema.Value, spec)
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

func main() {
	// Load your local Swagger/OpenAPI spec
	// run from the root of the project
	spec, err := openapi3.NewLoader().LoadFromFile("core/api/docs/swagger.yaml")
	if err != nil {
		log.Fatalf("failed to load spec: %v", err)
	}

	// Prepare tools slice and name registry to check for uniqueness
	var toolsInfo []*struct {
		Tool   openai.Tool
		Method string
		Path   string
	}
	nameRegistry := make(map[string]*endpointInfo)

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
			toolName := sanitizeName(nameSource)
			if toolName == "" {
				log.Printf("Warning: Skipping %s %s because sanitized %s is empty", method, path, nameOrigin)
				continue
			}

			// Check for uniqueness
			if existing, exists := nameRegistry[toolName]; exists {
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

			// Register the tool name with its endpoint info
			nameRegistry[toolName] = &endpointInfo{
				Method:          method,
				Path:            path,
				OriginalSummary: op.Summary,
				OperationId:     op.OperationID,
			}

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
							refSchema, err := resolveSchemaRef(mediaType.Schema.Ref, spec)
							if err != nil {
								log.Printf("Warning: Could not resolve schema reference %s: %v", mediaType.Schema.Ref, err)
								bodyProp = map[string]interface{}{
									"type":        "object",
									"description": fmt.Sprintf("Reference to %s", mediaType.Schema.Ref),
								}
							} else {
								// Convert the resolved schema to a map
								bodyProp, err = schemaToMap(refSchema, spec)
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
							bodyProp, err = schemaToMap(mediaType.Schema.Value, spec)
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

						params["properties"].(map[string]interface{})["body"] = bodyProp
						params["required"] = append(params["required"].([]string), "body")
						break
					}
				}

				// Fallback to a generic object if we couldn't extract a schema
				if _, exists := params["properties"].(map[string]interface{})["body"]; !exists {
					params["properties"].(map[string]interface{})["body"] = map[string]interface{}{
						"type": "object",
					}
					params["required"] = append(params["required"].([]string), "body")
				}
			}

			// Use x-ai-description if available, otherwise use description
			description := op.Description
			if aiDesc, ok := op.Responses.Extensions["x-ai-description"]; ok {
				delete(op.Responses.Extensions, "x-ai-description")

				if strDesc, ok := aiDesc.(string); ok {
					description = strDesc
				}
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

	// Ensure the directory exists
	outputDir := "cmd/assistant/api"
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	// Open the output file
	outputPath := filepath.Join(outputDir, "tools.gen.go")
	file, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Write package declaration and imports
	fmt.Fprintln(file, "// Code generated by generator; DO NOT EDIT.")
	fmt.Fprintln(file, "package api")
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "import (")
	fmt.Fprintln(file, "\t\"encoding/json\"")
	fmt.Fprintln(file, "\t\"github.com/sashabaranov/go-openai\"")
	fmt.Fprintln(file, ")")
	fmt.Fprintln(file, "")

	// Define ApiTool struct
	fmt.Fprintln(file, "// ApiTool wraps an OpenAI tool with API endpoint information")
	fmt.Fprintln(file, "type ApiTool struct {")
	fmt.Fprintln(file, "\topenai.Tool")
	fmt.Fprintln(file, "\tMethod string")
	fmt.Fprintln(file, "\tPath   string")
	fmt.Fprintln(file, "}")
	fmt.Fprintln(file, "")

	// Write tools variable declaration
	fmt.Fprintln(file, "// AnytypeTools is a generated list of tools based on the OpenAPI spec")
	fmt.Fprintln(file, "var AnytypeTools = []ApiTool{")

	// Write each tool with its endpoint info
	for i, toolInfo := range toolsInfo {
		fmt.Fprintf(file, "\t{\n\t\tTool: openai.Tool{\n\t\t\tType: openai.ToolTypeFunction,\n\t\t\tFunction: &openai.FunctionDefinition{\n\t\t\t\tName: %q,\n\t\t\t\tDescription: %q,\n",
			toolInfo.Tool.Function.Name,
			toolInfo.Tool.Function.Description)

		// Handle parameters as JSON string
		paramBytes, _ := json.Marshal(toolInfo.Tool.Function.Parameters)
		fmt.Fprintf(file, "\t\t\t\tParameters: json.RawMessage(`%s`),\n\t\t\t},\n\t\t},\n", string(paramBytes))

		// Add endpoint info
		fmt.Fprintf(file, "\t\tMethod: %q,\n\t\tPath: %q,\n\t}", toolInfo.Method, toolInfo.Path)

		if i < len(toolsInfo)-1 {
			fmt.Fprintln(file, ",")
		} else {
			fmt.Fprintln(file, "")
		}
	}
	fmt.Fprintln(file, "}")

	// Add helper to get just the OpenAI tools
	fmt.Fprintln(file, "")
	fmt.Fprintln(file, "// GetOpenAITools returns just the OpenAI tool definitions without the API info")
	fmt.Fprintln(file, "func GetOpenAITools() []openai.Tool {")
	fmt.Fprintln(file, "\ttools := make([]openai.Tool, len(AnytypeTools))")
	fmt.Fprintln(file, "\tfor i, t := range AnytypeTools {")
	fmt.Fprintln(file, "\t\ttools[i] = t.Tool")
	fmt.Fprintln(file, "\t}")
	fmt.Fprintln(file, "\treturn tools")
	fmt.Fprintln(file, "}")
	fmt.Fprintln(file, "")

	// Add helper to get tool by name
	fmt.Fprintln(file, "// GetToolByName returns an ApiTool by its name")
	fmt.Fprintln(file, "func GetToolByName(name string) *ApiTool {")
	fmt.Fprintln(file, "\tfor i := range AnytypeTools {")
	fmt.Fprintln(file, "\t\tif AnytypeTools[i].Function.Name == name {")
	fmt.Fprintln(file, "\t\t\treturn &AnytypeTools[i]")
	fmt.Fprintln(file, "\t\t}")
	fmt.Fprintln(file, "\t}")
	fmt.Fprintln(file, "\treturn nil")
	fmt.Fprintln(file, "}")

	fmt.Printf("Generated tools with endpoint mapping written to %s\n", outputPath)
}
