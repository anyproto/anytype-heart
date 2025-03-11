package common

import (
	"encoding/csv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestExtractHeaders(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		storeFixture := spaceindex.NewStoreFixture(t)
		keys := []string{"key1", "key2"}

		storeFixture.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:          domain.String("id1"),
				bundle.RelationKeyRelationKey: domain.String("key1"),
				bundle.RelationKeyName:        domain.String("Name1"),
			},
			{
				bundle.RelationKeyId:          domain.String("id2"),
				bundle.RelationKeyRelationKey: domain.String("key2"),
				bundle.RelationKeyName:        domain.String("Name2"),
			},
		})

		// when
		headers, err := ExtractHeaders(storeFixture, keys)

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"Name1", "Name2"}, headers)
	})
	t.Run("empty", func(t *testing.T) {
		// given
		storeFixture := spaceindex.NewStoreFixture(t)
		keys := []string{"key1", "key2"}

		// when
		headers, err := ExtractHeaders(storeFixture, keys)

		// then
		assert.Error(t, err)
		assert.Empty(t, headers)
	})
}

func TestWriteCSV(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		csvRows := [][]string{{"header1", "header2"}, {"value1", "value2"}}

		// when
		buffer, err := WriteCSV(csvRows)

		// then
		assert.NoError(t, err)

		reader := csv.NewReader(buffer)
		rows, err := reader.ReadAll()
		assert.NoError(t, err)
		assert.Equal(t, csvRows, rows)
	})
}
