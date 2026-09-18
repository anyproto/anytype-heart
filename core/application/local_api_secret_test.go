package application

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalAPISecret(t *testing.T) {
	t.Run("enforcement is off until a secret is registered", func(t *testing.T) {
		// given
		s := New()

		// then
		assert.False(t, s.LocalAPISecretEnforced())
	})

	t.Run("registering a secret turns enforcement on", func(t *testing.T) {
		// given
		s := New()

		// when
		s.SetLocalAPISecret("s3cret")

		// then
		assert.True(t, s.LocalAPISecretEnforced())
	})

	t.Run("matching secret passes", func(t *testing.T) {
		// given
		s := New()
		s.SetLocalAPISecret("s3cret")

		// then
		assert.True(t, s.CheckLocalAPISecret("s3cret"))
	})

	t.Run("wrong secret is rejected", func(t *testing.T) {
		// given
		s := New()
		s.SetLocalAPISecret("s3cret")

		// then
		assert.False(t, s.CheckLocalAPISecret("s3crey"))
		assert.False(t, s.CheckLocalAPISecret("s3cret-longer"))
		assert.False(t, s.CheckLocalAPISecret("s3cre"))
		assert.False(t, s.CheckLocalAPISecret(""))
	})

	t.Run("empty secret leaves enforcement off", func(t *testing.T) {
		// given
		s := New()

		// when
		s.SetLocalAPISecret("")

		// then
		assert.False(t, s.LocalAPISecretEnforced())
	})

	t.Run("the secret is write-once: a later set does not replace it", func(t *testing.T) {
		// given
		s := New()
		s.SetLocalAPISecret("first")

		// when
		s.SetLocalAPISecret("second")

		// then
		assert.True(t, s.CheckLocalAPISecret("first"))
		assert.False(t, s.CheckLocalAPISecret("second"))
	})

	t.Run("an empty set does not clear a registered secret", func(t *testing.T) {
		// given
		s := New()
		s.SetLocalAPISecret("first")

		// when
		s.SetLocalAPISecret("")

		// then
		assert.True(t, s.LocalAPISecretEnforced())
		assert.True(t, s.CheckLocalAPISecret("first"))
	})

	t.Run("check fails when nothing is registered", func(t *testing.T) {
		// given
		s := New()

		// then
		assert.False(t, s.CheckLocalAPISecret(""))
		assert.False(t, s.CheckLocalAPISecret("anything"))
	})
}
