package mill

import (
	"testing"
)

func TestSchema_Mill(t *testing.T) {
	m := &Schema{}

	person := `
{
  "pin": true,
  "mill": "/json",
  "json_schema": {
    "$id": "https://example.com/person.schema.json",
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "Person",
    "type": "object",
    "properties": {
      "firstName": {
        "type": "string",
        "description": "The person's first name."
      },
      "lastName": {
        "type": "string",
        "description": "The person's last name."
      },
      "age": {
        "description": "Age in years which must be equal to or greater than zero.",
        "type": "integer",
        "minimum": 0
      }
    }
  }
}
`

	if _, err := m.Mill([]byte(person), "test"); err != nil {
		t.Fatal(err)
	}
}
