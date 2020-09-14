package mill

import (
	"testing"
)

func TestJson_Mill(t *testing.T) {
	m := &Json{}

	obj := `{"firstName": "Grigori", "lastName": "Rasputin", "age": 47}`

	if _, err := m.Mill([]byte(obj), "test"); err != nil {
		t.Fatal(err)
	}
}
