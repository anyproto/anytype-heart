package threads

import (
	"context"
	"strings"
	"time"

	"github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/thread"
	threadsDb "github.com/textileio/go-threads/db"
	threadsUtil "github.com/textileio/go-threads/util"
)

func newCreatorInfoActionProcessor(s *service) CollectionActionProcessor {
	return func(action threadsDb.Action, collection *threadsDb.Collection) {
		WorkspaceLogger.
			With("device id", action.ID.String()).
			Info("processing creator info")
		if !strings.HasPrefix(action.Collection, CreatorCollectionName) {
			return
		}
		result, err := collection.FindByID(action.ID)
		if err != nil {
			WorkspaceLogger.
				With("device id", action.ID.String()).
				Error("can't find instance in database")
			return
		}
		creatorInfo := CreatorInfo{}
		threadsUtil.InstanceFromJSON(result, &creatorInfo)

		profileId, err := ProfileThreadIDFromAccountAddress(creatorInfo.AccountPubKey)
		if err != nil {
			WorkspaceLogger.
				With("device id", action.ID.String()).
				Error("can't create profile id from address")
			return
		}

		sk, rk, err := ProfileThreadKeysFromAccountAddress(creatorInfo.AccountPubKey)
		if err != nil {
			WorkspaceLogger.
				With("device id", action.ID.String()).
				Error("can't create keys from address")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		info, err := s.t.GetThread(ctx, profileId)
		cancel()
		if err != nil && err != logstore.ErrThreadNotFound {
			return
		}
		if info.ID != thread.Undef {
			return
		}

		ti := threadInfo{
			ID:    db.InstanceID(profileId.String()),
			Key:   thread.NewKey(sk, rk).String(),
			Addrs: creatorInfo.Addrs,
		}

		err = s.processNewExternalThreadUntilSuccess(profileId, ti)
		if err != nil {
			WorkspaceLogger.
				With("device id", action.ID.String()).
				With("profile id", profileId.String()).
				Error("can't load profile")
			return
		}
	}
}
