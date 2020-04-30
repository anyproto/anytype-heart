package uri

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURI_ProcessURI(t *testing.T) {
	t.Run("should process mailto uri", func(t *testing.T) {
		uri := "john@doe.com"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, "mailto:"+uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should process tel uri", func(t *testing.T) {
		uri := "+491234567"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, "tel:"+uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should process url", func(t *testing.T) {
		uri := "website.com"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, "http://"+uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should process url with additional content 1", func(t *testing.T) {
		uri := "website.com/123/456"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, "http://"+uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should process url with additional content 2", func(t *testing.T) {
		uri := "website.com?content=11"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, "http://"+uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should return error if it is not a uri", func(t *testing.T) {
		uri := "website"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, uri, processedUri)
		assert.Error(t, err)
	})

	t.Run("should not process url with http://", func(t *testing.T) {
		uri := "http://website.com"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should not process url with https://", func(t *testing.T) {
		uri := "https://website.com"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, uri, processedUri)
		assert.NoError(t, err)
	})

	t.Run("should not process non url/tel/mailto uri", func(t *testing.T) {
		uri := "type:content"
		processedUri, err := ProcessURI(uri)
		assert.Equal(t, uri, processedUri)
		assert.NoError(t, err)
	})
}
