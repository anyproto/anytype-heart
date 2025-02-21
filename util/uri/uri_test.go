package uri

import (
	"net/url"
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
		assert.Equal(t, err, ErrFilepathNotSupported)
	})

	t.Run("should return error on unix abs filepath", func(t *testing.T) {
		uri := "/folder/file.txt"
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, ErrFilepathNotSupported)
	})

	t.Run("should return error on unix rel filepath", func(t *testing.T) {
		uri := "../folder/file.txt"
		err := ValidateURI(uri)
		assert.Error(t, err)
		assert.Equal(t, err, ErrFilepathNotSupported)
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

func TestGetFileNameFromURLWithContentTypeAndMime(t *testing.T) {
	mustParseURL := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatalf("url.Parse(%q) failed: %v", s, err)
		}
		return u
	}

	tests := []struct {
		name        string
		url         *url.URL
		contentType string
		expected    string
	}{
		{
			name:        "URL with explicit filename and extension",
			url:         mustParseURL("https://example.com/image.jpg"),
			contentType: "image/jpeg",
			expected:    "image.jpg",
		},
		{
			name:        "URL with explicit filename and extension, but wrong content type",
			url:         mustParseURL("https://example.com/image.jpg"),
			contentType: "image/png",
			expected:    "image.jpg",
		},
		{
			name:        "URL with explicit filename and extension, and empty content type",
			url:         mustParseURL("https://example.com/image.jpg"),
			contentType: "",
			expected:    "image.jpg",
		},
		{
			name:        "URL with query and fragment, explicit filename",
			url:         mustParseURL("https://example.com/file.jpeg?query=1#111"),
			contentType: "image/jpeg",
			expected:    "file.jpeg",
		},
		{
			name:        "No filename in URL, fallback to host and image/jpeg",
			url:         mustParseURL("https://www.example.com/path/to/"),
			contentType: "image/jpeg",
			// host -> example_com
			// image/jpeg typically corresponds to .jpeg or .jpg (mime usually returns .jpeg)
			expected: "example_com_image.jpeg",
		},
		{
			name:        "Host-only URL, fallback with image/png",
			url:         mustParseURL("https://www.example.com"),
			contentType: "image/png",
			expected:    "example_com_image.png",
		},
		{
			name:        "Filename present with video/mp4",
			url:         mustParseURL("https://www.sub.example.co.uk/folder/video.mp4"),
			contentType: "video/mp4",
			expected:    "video.mp4",
		},
		{
			name:        "No extension but filename present",
			url:         mustParseURL("https://example.com/filename"),
			contentType: "image/gif",
			expected:    "example_com_image.gif",
		},
		{
			name:        "Invalid URL returns empty",
			url:         nil,
			contentType: "image/jpeg",
			expected:    "image.jpeg",
		},
		{
			name:        "No filename, video/unknown fallback to .bin",
			url:         mustParseURL("https://www.subdomain.example.com/folder/"),
			contentType: "video/unknown",
			// no known extension for "video/unknown", fallback .bin
			expected: "subdomain_example_com_video.bin",
		},
		{
			name:        "Hidden file as filename",
			url:         mustParseURL("https://example.com/.htaccess"),
			contentType: "text/plain",
			expected:    ".htaccess",
		},
		{
			name:        "URL with query but no filename extension, fallback audio/mpeg",
			url:         mustParseURL("https://example.com/path?version=2"),
			contentType: "audio/mpeg",
			// audio/mpeg known extension: .mp3
			expected: "example_com_audio.mp3",
		},
		{
			name:        "Unknown type entirely",
			url:         mustParseURL("https://example.net/"),
			contentType: "application/x-something-strange",
			// no filename, fallback host: example_net
			// unknown type -> .bin
			expected: "example_net_file.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFileNameFromURLAndContentType(tt.url, tt.contentType)
			if got != tt.expected {
				t.Errorf("GetFileNameFromURL(%q, %q) = %q; want %q", tt.url, tt.contentType, got, tt.expected)
			}
		})
	}
}
