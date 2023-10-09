package logging

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanupArgs(t *testing.T) {
	t.Run("with nil arg", func(t *testing.T) {
		in := []interface{}{
			nil,
		}
		cleanupArgs(in)

		got := fmt.Sprintf("error: %s", in...)

		want := "error: %!s(<nil>)"
		assert.Equal(t, want, got)
	})

	t.Run("with just error", func(t *testing.T) {
		in := []interface{}{
			fmt.Errorf("some error"),
		}
		cleanupArgs(in)

		got := fmt.Sprintf("error: %s", in...)

		want := "error: some error"
		assert.Equal(t, want, got)
	})

	t.Run("with os.PathError", func(t *testing.T) {
		in := []interface{}{
			123,
			&os.PathError{
				Op:   "open",
				Path: "/home/user/secret folder/secret file.txt",
				Err:  fmt.Errorf("severe error"),
			},
		}
		cleanupArgs(in)

		got := fmt.Sprintf("trial %d: uploading file: %s", in...)

		want := "trial 123: uploading file: open <masked file path>: severe error"
		assert.Equal(t, want, got)
	})

	t.Run("wrapped os.PathError", func(t *testing.T) {
		err := &os.PathError{
			Op:   "open",
			Path: "/home/user/secret folder/secret file.txt",
			Err:  fmt.Errorf("severe error"),
		}
		in := []interface{}{
			123,
			fmt.Errorf("wrapped: %w", err),
		}
		cleanupArgs(in)

		got := fmt.Sprintf("trial %d: uploading file: %s", in...)

		want := "trial 123: uploading file: wrapped: open <masked file path>: severe error"
		assert.Equal(t, want, got)
	})

	t.Run("nested wrapped errors", func(t *testing.T) {
		err := &url.Error{
			Op:  "get",
			URL: "secretaffairs.com/foo/bar",
			Err: &net.DNSError{
				Name:   "secretaffairs.com",
				Err:    "resolve",
				Server: "1.1.1.1",
			},
		}
		in := []interface{}{
			fmt.Errorf("wrapped: %w", err),
		}
		cleanupArgs(in)

		got := fmt.Sprintf("error %s", in...)

		want := "error wrapped: get \"<masked url>\": lookup <masked host name> on <masked dns server>: resolve"
		assert.Equal(t, want, got)
	})
}
