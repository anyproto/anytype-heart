package anystorehelper

import (
	"context"
	"errors"
	"fmt"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/stretchr/testify/assert"
	"zombiezen.com/go/sqlite"
)

func TestIsCorruptedError(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"v2 corrupt umbrella":         {fmt.Errorf("open: %w", anystore.ErrCorrupt), true},
		"v2 header corrupt":           {fmt.Errorf("open: %w", anystore.ErrHeaderCorrupt), true},
		"v2 wal corrupt":              {fmt.Errorf("open: %w", anystore.ErrWALCorrupt), true},
		"v2 quick check failed":       {fmt.Errorf("open: %w", anystore.ErrQuickCheckFailed), true},
		"v2 incompatible version":     {anystore.ErrIncompatibleVersion, true},
		"v1 file":                     {anystore.ErrV1Database, false},
		"nil":                         {nil, false},
		"cancelled context":           {fmt.Errorf("open: %w", context.Canceled), false},
		"expired context":             {fmt.Errorf("open: %w", context.DeadlineExceeded), false},
		"cancelled joined to corrupt": {errors.Join(context.Canceled, anystore.ErrCorrupt), false},
		"other io error":              {errors.New("disk on fire"), false},
	} {
		t.Run(name, func(t *testing.T) {
			_, got := IsCorruptedError(tc.err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("sqlite interrupt", func(t *testing.T) {
		_, got := IsCorruptedError(sqlite.ResultInterrupt.ToError())
		assert.False(t, got)
	})
	t.Run("sqlite corrupt", func(t *testing.T) {
		_, got := IsCorruptedError(sqlite.ResultCorrupt.ToError())
		assert.True(t, got)
	})
}
