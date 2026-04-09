package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSpaceId(t *testing.T) {
	tests := []struct {
		participantId   string
		expectedSpaceId string
		expectedId      string
		expectError     bool
	}{
		{"prefix_space_123", "", "", true},
		{"_participant_space.participant_456", "", "", true},
		{"invalid_format", "", "", true},
		{"_participant_spacepref_spacesuf_participantid", "spacepref.spacesuf", "participantid", false},
		{"_participant_spacepref_spacesuf", "", "", true},
	}

	for _, test := range tests {
		spaceId, id, err := ParseParticipantId(test.participantId)
		if test.expectError {
			assert.Error(t, err, "Expected error for input %s", test.participantId)
		} else {
			assert.NoError(t, err, "Unexpected error for input %s", test.participantId)
			assert.Equal(t, test.expectedSpaceId, spaceId, "For input space %s", test.participantId)
			assert.Equal(t, test.expectedId, id, "For input id %s", test.participantId)
		}
	}
}

func TestNewPersonalWidgetsId_roundtrip(t *testing.T) {
	t.Run("typical spaceId", func(t *testing.T) {
		// given
		const spaceId = "bafya4spacepayload.yt7efooa"
		want := "_personalWidgets_bafya4spacepayload_yt7efooa"

		// when
		got := NewPersonalWidgetsId(spaceId)
		parsed, err := ParsePersonalWidgetsId(got)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, spaceId, parsed)
	})

	t.Run("single-character halves", func(t *testing.T) {
		got := NewPersonalWidgetsId("a.b")
		parsed, err := ParsePersonalWidgetsId(got)
		require.NoError(t, err)
		assert.Equal(t, "_personalWidgets_a_b", got)
		assert.Equal(t, "a.b", parsed)
	})
}

func TestParsePersonalWidgetsId_malformed(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"missing prefix", "personalWidgets_space_id"},
		{"no underscore", "_personalWidgets_bafyaspacepayload"},
		{"empty tail", "_personalWidgets_"},
		{"leading underscore only", "_personalWidgets__yt7efooa"},
		{"trailing underscore only", "_personalWidgets_bafya_"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spaceId, err := ParsePersonalWidgetsId(tc.id)
			assert.Error(t, err, "expected error for %q", tc.id)
			assert.Empty(t, spaceId)
		})
	}
}
