package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestService_setCreatedInContext_SetsFields(t *testing.T) {
	// given
	detailsSvc := mock_detailservice.NewMockService(t)
	svc := &Service{detailsService: detailsSvc}
	sctx := session.NewContext()

	var capturedDetails *domain.Details
	detailsSvc.EXPECT().
		ModifyDetails(mock.Anything, "obj1", mock.Anything).
		RunAndReturn(func(_ session.Context, _ string, modifier func(*domain.Details) (*domain.Details, error)) error {
			d := domain.NewDetails()
			result, err := modifier(d)
			capturedDetails = result
			return err
		})

	// when
	svc.setCreatedInContext(sctx, "obj1", "ctx1", "link1")

	// then
	require.NotNil(t, capturedDetails)
	assert.Equal(t, "ctx1", capturedDetails.GetString(bundle.RelationKeyCreatedInContext))
	assert.Equal(t, "link1", capturedDetails.GetString(bundle.RelationKeyCreatedInContextRef))
}

func TestService_setCreatedInContext_NoOpWhenContextIdEmpty(t *testing.T) {
	// given
	detailsSvc := mock_detailservice.NewMockService(t)
	svc := &Service{detailsService: detailsSvc}
	// detailsSvc.ModifyDetails must NOT be called

	// when
	svc.setCreatedInContext(nil, "obj1", "", "link1")

	// then: no mock calls expected — testify will fail the test if ModifyDetails is called unexpectedly
}
