package chatobject

import (
	"context"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const detailsDocumentId = "details"

type detailsComponent struct {
	componentCtx          context.Context
	collectionName        string
	deniedRelationKeys    []domain.RelationKey
	deniedRelationKeysSet map[domain.RelationKey]struct{}
	storeSource           source.Store
	storeState            *storestate.StoreState
	sb                    smartblock.SmartBlock
}

func (c *detailsComponent) init(st *state.State) error {
	c.deniedRelationKeysSet = map[domain.RelationKey]struct{}{}
	for _, key := range c.deniedRelationKeys {
		c.deniedRelationKeysSet[key] = struct{}{}
	}

	err := c.setDetailsFromAnystore(c.componentCtx, st, true)
	if err != nil {
		return fmt.Errorf("set details from anystore: %w", err)
	}
	return nil
}

func (c *detailsComponent) onPushOrdinaryChange(params source.PushChangeParams) (id string, err error) {
	builder := &storestate.Builder{}
	arena := &anyenc.Arena{}
	for _, ch := range params.Changes {
		set := ch.GetDetailsSet()
		if set != nil && set.Key != "" {
			key := domain.RelationKey(set.Key)
			if _, ok := c.deniedRelationKeysSet[key]; ok {
				continue
			}
			if slices.Contains(bundle.LocalAndDerivedRelationKeys, key) {
				continue
			}
			val := domain.ValueFromProto(set.Value)
			if !val.Ok() {
				continue
			}
			err := builder.Modify(c.collectionName, detailsDocumentId, []string{set.Key}, pb.ModifyOp_Set, val.ToAnyEnc(arena))
			if err != nil {
				return "", fmt.Errorf("modify content: %w", err)
			}
		}
	}
	if builder.StoreChange == nil {
		return "", nil
	}
	return c.storeSource.PushStoreChange(c.componentCtx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   c.storeState,
		Time:    time.Now(),
	})
}

func (c *detailsComponent) setDetailsFromAnystore(ctx context.Context, st *state.State, applyToParentState bool) error {
	coll, err := c.storeState.Collection(ctx, c.collectionName)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	doc, err := coll.FindId(ctx, detailsDocumentId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find id: %w", err)
	}

	details, err := domain.NewDetailsFromAnyEnc(doc.Value())
	if err != nil {
		return fmt.Errorf("parse details: %w", err)
	}
	for key, v := range details.Iterate() {
		// Ignore orders key
		if key == "_o" {
			continue
		}
		if _, ok := c.deniedRelationKeysSet[key]; ok {
			continue
		}
		if slices.Contains(bundle.LocalAndDerivedRelationKeys, key) {
			continue
		}
		if applyToParentState {
			st.ParentState().SetDetail(key, v)
		} else {
			st.SetDetail(key, v)
		}
	}
	return nil
}

func (c *detailsComponent) onAnystoreUpdated(ctx context.Context) error {
	c.sb.(source.ChangeReceiver).StateAppend(func(d state.Doc) (*state.State, []*pb.ChangeContent, error) {
		st := d.NewState()
		err := c.setDetailsFromAnystore(ctx, st, false)
		if err != nil {
			return nil, nil, fmt.Errorf("set details from anystore: %w", err)
		}
		return st, nil, nil
	})
	return nil
}
