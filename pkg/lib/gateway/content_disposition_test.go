package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/stretchr/testify/mock"
)

// The name comes from file metadata, which whoever shared the file controls, so
// it reaches this header as untrusted text.
func TestContentDisposition(t *testing.T) {
	t.Run("a plain name needs only the quoted form", func(t *testing.T) {
		assert.Equal(t, `inline; filename="report.pdf"`, contentDisposition("inline", "report.pdf"))
		assert.Equal(t, `attachment; filename="report.pdf"`, contentDisposition("attachment", "report.pdf"))
	})

	t.Run("a non-ascii name carries an ascii fallback and the real name", func(t *testing.T) {
		assert.Equal(t,
			`attachment; filename="Pr_fung.pdf"; filename*=UTF-8''Pr%C3%BCfung.pdf`,
			contentDisposition("attachment", "Prüfung.pdf"))
	})

	t.Run("one fallback character per rune, not per byte", func(t *testing.T) {
		// Three runes, three underscores — a per-byte fallback would emit nine.
		assert.Contains(t, contentDisposition("attachment", "日本語.txt"), `filename="___.txt"`)
	})

	t.Run("a quote cannot escape the quoted form", func(t *testing.T) {
		got := contentDisposition("attachment", `my"quoted.txt`)

		assert.Equal(t,
			`attachment; filename="my_quoted.txt"; filename*=UTF-8''my%22quoted.txt`,
			got)
		assert.Equal(t, 2, strings.Count(got, `"`), "the quoted form must have exactly one pair of quotes")
	})

	t.Run("control characters are dropped from the fallback", func(t *testing.T) {
		got := contentDisposition("attachment", "a\r\nb.txt")

		assert.Contains(t, got, `filename="ab.txt"`)
		assert.NotContains(t, got, "\r")
		assert.NotContains(t, got, "\n")
	})

	// The write path already confines the name to one path element; the header
	// must not hand a caller back something it can treat as a path either.
	// Both separators, whatever this machine's, because the name came from
	// whoever shared the file and the client saving it may be on Windows.
	t.Run("path separators do not survive into the filename", func(t *testing.T) {
		assert.Equal(t, `attachment; filename=".bashrc"`,
			contentDisposition("attachment", "../../../.bashrc"))
		assert.Equal(t, `attachment; filename="passwd"`,
			contentDisposition("attachment", "/etc/passwd"))
		assert.Equal(t, `attachment; filename="report.pdf"`,
			contentDisposition("attachment", `..\..\report.pdf`))
	})

	t.Run("a name that is only traversal gets the generic fallback", func(t *testing.T) {
		for _, name := range []string{"..", "../..", "/", "/../"} {
			got := contentDisposition("attachment", name)
			assert.Equal(t, `attachment; filename="file"`, got, "name %q", name)
		}
	})

	// A NUL reaching a client that decodes filename* is a footgun, and browsers
	// cap response headers (Chromium at 256KB) so an unbounded name fails the
	// download with a header error rather than a filename error.
	t.Run("control characters never reach filename*", func(t *testing.T) {
		got := contentDisposition("attachment", "x\x00y.txt")

		assert.NotContains(t, got, "%00")
		assert.Contains(t, got, `filename="xy.txt"`)
	})

	t.Run("an absurdly long name is capped, keeping the extension", func(t *testing.T) {
		got := contentDisposition("attachment", strings.Repeat("ä", 40000)+".pdf")

		assert.Less(t, len(got), 1024, "header must stay far below any client cap")
		assert.Contains(t, got, `.pdf"`)
		assert.True(t, strings.HasSuffix(got, ".pdf"), "filename* keeps the extension too: %s", got)
	})

	t.Run("an empty name gets a generic fallback", func(t *testing.T) {
		assert.Equal(t, `attachment; filename="file"`, contentDisposition("attachment", ""))
	})

	t.Run("a name that is only unrepresentable characters still names something", func(t *testing.T) {
		got := contentDisposition("attachment", "\r\n")

		assert.Contains(t, got, `filename="file"`)
	})
}

func TestAttachmentRequested(t *testing.T) {
	requested := func(query string) bool {
		t.Helper()
		return attachmentRequested(httptest.NewRequest(http.MethodGet, "/file/objectId?"+query, nil))
	}

	t.Run("requested", func(t *testing.T) {
		assert.True(t, requested("attachment=1"))
		assert.True(t, requested("attachment=true"))
		assert.True(t, requested("attachment"))
	})

	t.Run("not requested", func(t *testing.T) {
		assert.False(t, requested(""))
		assert.False(t, requested("attachment=0"))
		assert.False(t, requested("attachment=false"))
		assert.False(t, requested("attachment=nonsense"))
	})
}

func TestFileHandlerContentDisposition(t *testing.T) {
	serve := func(t *testing.T, name string, query string) string {
		t.Helper()
		fx := newFixture(t)

		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(mock.Anything).Return(strings.NewReader("payload"), nil)
		file.EXPECT().Meta().Return(&files.FileMeta{Media: "application/octet-stream", Name: name})
		fx.fileObjectService.EXPECT().GetFileData(mock.Anything, mock.Anything).Return(file, nil)

		resp, err := http.Get("http://" + fx.Addr() + "/file/fileObjectId?" + query)
		require.NoError(t, err)
		defer resp.Body.Close()

		return resp.Header.Get("Content-Disposition")
	}

	t.Run("inline by default, so image and preview rendering is unchanged", func(t *testing.T) {
		assert.Equal(t, `inline; filename="report.pdf"`, serve(t, "report.pdf", ""))
	})

	t.Run("attachment on request, so the save flow gets a download", func(t *testing.T) {
		assert.Equal(t, `attachment; filename="report.pdf"`, serve(t, "report.pdf", "attachment=1"))
	})

	t.Run("a non-ascii name survives the round trip", func(t *testing.T) {
		assert.Equal(t,
			`attachment; filename="Pr_fung.pdf"; filename*=UTF-8''Pr%C3%BCfung.pdf`,
			serve(t, "Prüfung.pdf", "attachment=1"))
	})
}
