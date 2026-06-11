package space

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
)

// AddStreamable is unidirectional: it creates the space view carrying the
// guest key and waits for the watcher-registered controller, which the view's
// EncodedKey makes streamable.
func (s *service) AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) (err error) {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	exists, err := s.techSpace.SpaceViewExists(ctx, id)
	if err != nil {
		return fmt.Errorf("check space view: %w", err)
	}
	if !exists {
		if err := s.factory.CreateStreamableSpace(ctx, guestKey, id); err != nil {
			return err
		}
	}
	ctrl, err := s.waitCtrl(ctx, id)
	if err != nil {
		return err
	}
	ctrl.Demand()
	return s.waitIntentMode(ctx, ctrl, mode.ModeLoading)
}
