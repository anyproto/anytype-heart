package planfixture

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These guard the fixture-authoring rules in fixtures/FORMAT.md. They exercise
// decode directly because the shipped fixtures are embedded and correct — a
// malformed one only ever exists mid-edit, which is exactly when the author
// needs the error to be specific.
func TestDecodeRejects(t *testing.T) {
	t.Run("unknown format name", func(t *testing.T) {
		// given
		raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[
			{"id":"db","name":"D","properties":[{"id":"p","name":"P","format":"richtext"}]}],
			"expect":{}}`)

		// when
		_, err := decode(raw)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown format "richtext"`)
	})

	t.Run("select without options", func(t *testing.T) {
		// given
		raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[
			{"id":"db","name":"D","properties":[{"id":"p","name":"P","format":"select"}]}],
			"expect":{}}`)

		// when
		_, err := decode(raw)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires options")
	})

	t.Run("options on a format that cannot carry them", func(t *testing.T) {
		// given
		raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[
			{"id":"db","name":"D","properties":[
				{"id":"p","name":"P","format":"date","options":["a","b"]}]}],
			"expect":{}}`)

		// when
		_, err := decode(raw)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not carry options")
	})

	t.Run("bundled target outside the closed set", func(t *testing.T) {
		// given — "status" is the canonical illegal target: a real select, but
		// one option pool per space (see AllowedBundledTargets' doc comment).
		raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[
			{"id":"db","name":"D","properties":[
				{"id":"p","name":"P","format":"select","options":["a","b"]}]}],
			"expect":{"bundled":{"db":{"p":"status"}}}}`)

		// when
		_, err := decode(raw)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an allowed bundled target")
	})

	t.Run("unknown field, so a typo in an expectation key is not silently ignored", func(t *testing.T) {
		// given
		raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[],
			"expect":{},"sameKnid":[["a","b"]]}`)

		// when
		_, err := decode(raw)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestDecodeAcceptsAbsentSamples(t *testing.T) {
	// given — FORMAT.md allows omitting titles to test the no-samples path
	raw := []byte(`{"id":"x","name":"x","family":"corporate","containers":[
		{"id":"db","name":"D","properties":[{"id":"p","name":"P","format":"text"}]}],
		"expect":{}}`)

	// when
	fixture, err := decode(raw)

	// then
	require.NoError(t, err)
	require.Len(t, fixture.Containers, 1)
	assert.Nil(t, fixture.Containers[0].Samples)
}
