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
			if omit, ok := op.Extensions["x-ai-omit"]; ok {
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
				prop := map[string]interface{}{
					"type": ps.Type,
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

				params["properties"].(map[string]interface{})[p.Value.Name] = prop
				if p.Value.Required {
					params["required"] = append(params["required"].([]string), p.Value.Name)
				}
			}

			// If there is a requestBody, include it as a `body` property
			if op.RequestBody != nil {
				params["properties"].(map[string]interface{})["body"] = map[string]interface{}{
					"type": "object",
				}
				params["required"] = append(params["required"].([]string), "body")
			}

			// Use x-ai-description if available, otherwise use description
			description := op.Description
			if aiDesc, ok := op.Extensions["x-ai-description"]; ok {
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
	outputDir := "cmd/assistant"
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
	fmt.Fprintln(file, "package assistant")
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
