# OpenAPI to OpenAI Tools Generator

This generator converts an OpenAPI specification (swagger.yaml) into OpenAI function tools.

## Usage

Run the generator with:

```bash
go run main.go
```

This will:
1. Read the `swagger.yaml` file in the current directory
2. Convert each API endpoint into an OpenAI function tool
3. Create a file at `pkg/assistant/tools.gen.go` with the generated tools

## Configuration

You can customize the generator by modifying:

- `ignoredProperties`: Properties to exclude from the generated tools
- Output path: Change the output directory and filename in the code
- Function naming: Modify the `sanitizeName` function to change how functions are named

## Generated Output

The generated output is a Go file containing:

1. A package declaration: `package assistant`
2. Required imports
3. A variable `AnytypeTools` containing an array of OpenAI tools

## Integration

After generating the tools, you can import them in your Go code:

```go
import "github.com/anyproto/anytype-heart/pkg/assistant"

// Use the tools
tools := assistant.AnytypeTools
```

Then use these tools with the OpenAI API or with the included tool handlers. 