package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHookRunner_RegisterHook(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		hr := hookRunner{}

		// when
		hook := func(ctx Context) error {
			return nil
		}
		hr.RegisterHook(hook)

		// then
		assert.Len(t, hr.onNewSessionHooks, 1)
	})
}

func TestHookRunner_RunHooks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		hr := hookRunner{}

		// when
		var hookCalled bool
		hook := func(ctx Context) error {
			hookCalled = true
			return nil
		}
		hr.RegisterHook(hook)
		hr.RunHooks(nil)

		// then
		assert.True(t, hookCalled)
	})
}
