package mill

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestBlob_Mill(t *testing.T) {
	m := &Blob{}

	input := make([]byte, 512)
	rand.Read(input)

	if _, err := m.Mill(bytes.NewReader(input), "test"); err != nil {
		t.Fatal(err)
	}
}
