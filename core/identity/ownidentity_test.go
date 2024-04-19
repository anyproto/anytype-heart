package identity

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

type ownSubscriptionFixture struct {
	*ownProfileSubscription

	spaceService      *mock_space.MockService
	coordinatorClient *inMemoryIdentityRepo
	testObserver      *testObserver
}

type testObserver struct {
	lock     sync.Mutex
	profiles []*model.IdentityProfile
}

func (t *testObserver) broadcastMyIdentityProfile(identityProfile *model.IdentityProfile) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.profiles = append(t.profiles, identityProfile)
}

func newOwnSubscriptionFixture(t *testing.T) *ownSubscriptionFixture {
	accountService := mock_account.NewMockService(t)
	spaceService := mock_space.NewMockService(t)
	objectStore := objectstore.NewStoreFixture(t)
	coordinatorClient := newInMemoryIdentityRepo()
	fileAclService := mock_fileacl.NewMockService(t)
	testObserver := &testObserver{}

	accountService.EXPECT().AccountID().Return("identity1")

	sub := newOwnProfileSubscription(spaceService, objectStore, accountService, coordinatorClient, fileAclService, testObserver, time.Second)

	return &ownSubscriptionFixture{
		ownProfileSubscription: sub,
		spaceService:           spaceService,
		coordinatorClient:      coordinatorClient,
		testObserver:           testObserver,
	}
}

func TestOwnProfileSubscription(t *testing.T) {
	t.Run("do not rewrite global name from profile details", func(t *testing.T) {

	})

	t.Run("rewrite global name from channel signal", func(t *testing.T) {

	})
}

func TestStartWithError(t *testing.T) {
	fx := newOwnSubscriptionFixture(t)
	fx.spaceService.EXPECT().GetPersonalSpace(mock.Anything).Return(nil, fmt.Errorf("space error"))

	t.Run("GetMyProfileDetails before run with cancelled input context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		identity, key, details := fx.getDetails(ctx)
		assert.Empty(t, identity)
		assert.Nil(t, key)
		assert.Nil(t, details)
	})

	err := fx.run(context.Background())
	require.Error(t, err)

	fx.close()

	done := make(chan struct{})

	go func() {
		_, _, _ = fx.getDetails(context.Background())
		close(done)
	}()

	select {
	case <-time.After(time.Second):
		t.Fatal("GetMyProfileDetails should not block")
	case <-done:
	}
}
