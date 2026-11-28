package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/restriction"
)

func TestLayoutWidthEditRestrictions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		refuseDetails bool
		dryRun        bool
	}{
		{"details restricted", true, false},
		{"details restricted preview", true, true},
		{"blocks restricted", false, false},
		{"blocks restricted preview", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			read := editRead(t, editEmptyDoc)
			if tc.refuseDetails {
				read.DetailsRefused = restriction.ErrRestricted
			} else {
				read.BlocksRefused = blocksRefusedProduction()
			}
			if tc.dryRun || tc.refuseDetails {
				fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)
			} else {
				fx.expectMutate(read)
			}

			result, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
				patchBody(`{"op":"set_layout_width","layout_width_percent":175}`), "", tc.dryRun, false)
			if tc.refuseDetails {
				apiErr := v2Err(t, err)
				require.Equal(t, v2model.CodeForbidden, apiErr.Code)
				require.Equal(t, "/ops/0", apiErr.Issues[0].Path)
				return
			}
			require.NoError(t, err)
			require.True(t, result.DiffStats.LayoutWidthChanged)
			if !tc.dryRun {
				fx.mutatorMock.AssertCalled(t, "MutateObject", mock.Anything, testSpaceId, "obj1", apicore.EditNeeds{Details: true}, mock.Anything)
			}
		})
	}
}
