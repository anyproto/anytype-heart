package objectstore

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func BenchmarkQuery(b *testing.B) {
	store := NewStoreFixture(b)
	objects := make([]TestObject, 1000)
	for i := range objects {
		objects[i] = generateObjectWithRandomID()
	}
	store.AddObjects(b, objects)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Query(database.Query{})
		require.NoError(b, err)
	}
}
