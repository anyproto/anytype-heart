package files

import (
	"context"
	"os"
	"sync"
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

		t.Run("add same image again", func(t *testing.T) {
			got := testAddImage(t, fx)

			assert.NotEmpty(t, got.MIME)
			assert.True(t, got.Size > 0)
		})
	})

	// We had a problem with concurrent adding of the same image, so test it
	t.Run("concurrent adding of the same image", func(t *testing.T) {
		fx := newFixture(t)
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				testAddImage(t, fx)
			}()
		}
		wg.Wait()
	})
}

func testAddImage(t *testing.T, fx *fixture) *ImageAddResult {
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

	return got
}
