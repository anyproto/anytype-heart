package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestExtractSpaceId(t *testing.T) {
	x := strings.Split("_participant_spaceIdprefix_spaceIdsuffix_identity", "_")
	fmt.Println(x)
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
			if err == nil {
				t.Errorf("Expected error for input %s, but got none", test.participantId)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", test.participantId, err)
			}
			if spaceId != test.expectedSpaceId {
				t.Errorf("For input space %s, expected %s but got %s", test.participantId, test.expectedSpaceId, spaceId)
			}
			if id != test.expectedId {
				t.Errorf("For input id %s, expected %s but got %s", test.participantId, test.expectedId, id)
			}
		}
	}
}
