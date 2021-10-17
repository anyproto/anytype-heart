package threads

import (
	"context"
	"fmt"
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
		if !strings.HasPrefix(action.Collection, CreatorCollectionName) {
			return
		}
		WorkspaceLogger.
			With("device id", action.ID.String()).
			Debug("processing creator info")
		var err error
		defer func() {
			if err != nil {
				WorkspaceLogger.
					With("device id", action.ID.String()).
					Errorf("error processing creator info: %v", err)
			} else {
				WorkspaceLogger.
					With("device id", action.ID.String()).
					Debug("successfully processed creator info")
			}
		}()

		result, err := collection.FindByID(action.ID)
		if err != nil {
			return
		}
		creatorInfo := CreatorInfo{}
		threadsUtil.InstanceFromJSON(result, &creatorInfo)

		profileId, err := ProfileThreadIDFromAccountAddress(creatorInfo.AccountPubKey)
		if err != nil {
			return
		}

		sk, rk, err := ProfileThreadKeysFromAccountAddress(creatorInfo.AccountPubKey)
		if err != nil {
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
			err = fmt.Errorf("can't load profile: %w", err)
			return
		}
	}
}
