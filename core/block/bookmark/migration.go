package bookmark

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var relationBlockKeys = []string{
	bundle.RelationKeyUrl.String(),
	bundle.RelationKeyPicture.String(),
	bundle.RelationKeyCreatedDate.String(),
	bundle.RelationKeyTag.String(),
	bundle.RelationKeyNotes.String(),
	bundle.RelationKeyQuote.String(),
}

func makeRelationBlock(k string) *model.Block {
	return &model.Block{
		Id: k,
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: k,
			},
		},
	}
}

// WithBookmarkBlocks is state transformer for using in templates
func WithBookmarkBlocks(st *state.State) {
	for _, k := range relationBlockKeys {
		if b := st.Pick(k); b != nil {
			if ok := st.Unlink(b.Model().Id); !ok {
				log.Errorf("can't unlink block %s", b.Model().Id)
				return
			}
			continue
		}

		ok := st.Add(simple.New(makeRelationBlock(k)))
		if !ok {
			log.Errorf("can't add block %s", k)
			return
		}
	}

	if err := st.InsertTo(st.RootId(), model.Block_InnerFirst, relationBlockKeys...); err != nil {
		log.Errorf("insert relation blocks: %w", err)
		return
	}
}

type BlockMigrator interface {
	MigrateBlock(bm bookmark.Block) (err error)
}

func WithFixedBookmarks(bm BlockMigrator) func(st *state.State) {
	return func(st *state.State) {
		if err := migrateBlocks(bm, st); err != nil {
			log.Errorf("migrate bookmark blocks: %s", err)
		}
	}
}

func migrateBlocks(bm BlockMigrator, st *state.State) error {
	var migrateErr error
	err := st.Iterate(func(b simple.Block) bool {
		block, ok := b.(bookmark.Block)
		if !ok {
			return true
		}

		if migrateErr = bm.MigrateBlock(block); migrateErr != nil {
			return false
		}
		return true
	})
	if migrateErr != nil {
		return migrateErr
	}
	if err != nil {
		return fmt.Errorf("iterate: %w", err)
	}
	return nil
}
