package llmplan

import "encoding/json"

// responseSchema is the strict, deliberately non-recursive plan schema
// (docs/ImportV2LLM.md §5). Every field is required — strict structured
// output demands it — and absence is spelled "". The shape stays within the
// JSON Schema subset local servers (ollama/LM Studio/llama.cpp) compile into
// decoding grammars.
var responseSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["types", "containers"],
  "properties": {
    "types": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["key", "name", "pluralName", "icon", "layout", "typeProperties"],
        "properties": {
          "key": {"type": "string"},
          "name": {"type": "string"},
          "pluralName": {"type": "string"},
          "icon": {"type": "string"},
          "layout": {"type": "string", "enum": ["basic", "todo", "profile", "note", ""]},
          "typeProperties": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["key", "name", "format", "section"],
              "properties": {
                "key": {"type": "string"},
                "name": {"type": "string"},
                "format": {"type": "string", "enum": ["text", "select", "multiSelect", "date", "number", "checkbox", "url", "email", "phone", "files", "objects", ""]},
                "section": {"type": "string", "enum": ["featured", "regular", ""]}
              }
            }
          }
        }
      }
    },
    "containers": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "type", "properties"],
        "properties": {
          "id": {"type": "string"},
          "type": {"type": "string"},
          "properties": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["id", "key", "name", "format"],
              "properties": {
                "id": {"type": "string"},
                "key": {"type": "string"},
                "name": {"type": "string"},
                "format": {"type": "string", "enum": ["text", "select", "multiSelect", "date", "number", "checkbox", "url", "email", "phone", "files", "objects", ""]}
              }
            }
          }
        }
      }
    }
  }
}`)
