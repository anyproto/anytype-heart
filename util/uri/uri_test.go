package uri

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURI_NormalizeURI(t *testing.T) {
	t.Run("should process mailto uri", func(t *testing.T) {
		uri := "john@doe.com"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "mailto:"+uri, processedURI)
	})

	t.Run("should process tel uri", func(t *testing.T) {
		uri := "+491234567"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "tel:"+uri, processedURI)
	})

	t.Run("should process url", func(t *testing.T) {
		uri := "website.com"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "http://"+uri, processedURI)
	})

	t.Run("should process url with additional content 1", func(t *testing.T) {
		uri := "website.com/123/456"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "http://"+uri, processedURI)
	})

	t.Run("should process url with additional content 2", func(t *testing.T) {
		uri := "website.com?content=11"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "http://"+uri, processedURI)
	})

	t.Run("should process url with additional content and numbers", func(t *testing.T) {
		uri := "webs1te.com/123/456"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, "http://"+uri, processedURI)
	})

	t.Run("should not modify url with http://", func(t *testing.T) {
		uri := "http://website.com"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, uri, processedURI)
	})

	t.Run("should not modify url with https://", func(t *testing.T) {
		uri := "https://website.com"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, uri, processedURI)
	})

	t.Run("should not modify non url/tel/mailto uri", func(t *testing.T) {
		uri := "type:content"
		processedURI, err := NormalizeURI(uri)
		assert.NoError(t, err)
		assert.Equal(t, uri, processedURI)
	})
}

func TestURI_ValidateURI(t *testing.T) {
	t.Run("should return error on empty string", func(t *testing.T) {
		uri := ""
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, errURLEmpty)
	})

	t.Run("should return error on win filepath", func(t *testing.T) {
		uri := "D://folder//file.txt"
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, errFilepathNotSupported)
	})

	t.Run("should return error on unix abs filepath", func(t *testing.T) {
		uri := "/folder/file.txt"
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, errFilepathNotSupported)
	})

	t.Run("should return error on unix rel filepath", func(t *testing.T) {
		uri := "../folder/file.txt"
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, errFilepathNotSupported)
	})

	t.Run("should not return error if url is surrounded by whitespaces", func(t *testing.T) {
		uri := " \t\n\v\r\f https://brutal-site.org \t\n\v\r\f "
		err := ValidateURI(uri)
		assert.NoError(t, err)
	})

	t.Run("should not return error if url has spaces inside", func(t *testing.T) {
		uri := "I do love enough space.org"
		err := ValidateURI(uri)
		assert.NoError(t, err)
	})

	t.Run("should not return error if url contains emojis", func(t *testing.T) {
		uri := "merry 🎄 and a happy 🎁.kevin.blog"
		err := ValidateURI(uri)
		assert.NoError(t, err)
	})
}
