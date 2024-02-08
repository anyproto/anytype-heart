package files

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageAdd(t *testing.T) {
	t.Run("add image", func(t *testing.T) {
		fx := newFixture(t)
		got := testAddImage(t, fx)

		assert.NotEmpty(t, got.MIME)
		assert.True(t, got.Size > 0)
		assert.False(t, got.IsExisting)

		t.Run("add same image again", func(t *testing.T) {
			got2 := testAddImage(t, fx)

			assert.NotEmpty(t, got2.MIME)
			assert.True(t, got2.Size > 0)

			assert.Equal(t, got.FileId, got2.FileId)
			assert.Equal(t, got.EncryptionKeys, got2.EncryptionKeys)
			assert.Equal(t, got.MIME, got2.MIME)
			assert.Equal(t, got.Size, got2.Size)
			assert.True(t, got2.IsExisting)
		})
	})

	// We had a problem with concurrent adding of the same image, so test it
	t.Run("concurrent adding of the same image", func(t *testing.T) {
		testAddConcurrently(t, func(t *testing.T, fx *fixture) *AddResult {
			return testAddImage(t, fx)
		})
	})
}

func testAddImage(t *testing.T, fx *fixture) *AddResult {
	f, err := os.Open("../../pkg/lib/mill/testdata/image.jpeg")
	require.NoError(t, err)
	defer f.Close()

	fileName := "myFile"
	lastModifiedDate := time.Now()
	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(f),
	}
	got, err := fx.ImageAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()
	return got
}
