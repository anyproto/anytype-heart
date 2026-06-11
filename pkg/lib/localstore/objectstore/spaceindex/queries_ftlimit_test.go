package spaceindex

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func TestFtCandidatesLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		offset int
		want   int
	}{
		{"no limit means everything, use the cap", 0, 0, ftCandidatesMax},
		{"small page is padded to the minimum", 10, 0, ftCandidatesMin},
		{"offset counts towards the budget", 50, 80, 130},
		{"budget is capped", 900, 500, ftCandidatesMax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ftCandidatesLimit(database.Query{Limit: tc.limit, Offset: tc.offset})
			assert.Equal(t, tc.want, got)
		})
	}
}
