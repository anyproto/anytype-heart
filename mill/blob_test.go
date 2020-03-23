package mill

import (
	"math/rand"
	"testing"
)

func TestBlob_Mill(t *testing.T) {
	m := &Blob{}

	input := make([]byte, 512)
	rand.Read(input)

	if _, err := m.Mill(input, "test"); err != nil {
		t.Fatal(err)
	}
}
