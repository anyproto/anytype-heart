package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
