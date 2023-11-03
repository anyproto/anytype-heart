package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Template struct {
	*Page
}

func (f *ObjectFactory) newTemplate(sb smartblock.SmartBlock) *Template {
	return &Template{
		Page: f.newPage(sb),
	}
}

func (t *Template) Init(ctx *smartblock.InitContext) (err error) {
	if err = t.Page.Init(ctx); err != nil {
		return
	}

	return
}

func (t *Template) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	parent := t.Page.CreationStateMigration(ctx)

	return migration.Compose(parent, migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			if t.Type() == coresb.SmartBlockTypeTemplate && (len(t.ObjectTypeKeys()) != 2) {
				targetObjectTypeId := pbtypes.GetString(s.Details(), bundle.RelationKeyTargetObjectType.String())
				if targetObjectTypeId != "" {
					uniqueKey, err := t.objectStore.GetUniqueKeyById(targetObjectTypeId)
					if err == nil && uniqueKey.SmartblockType() != coresb.SmartBlockTypeObjectType {
						err = fmt.Errorf("unique key %s has wrong smartblock type %d", uniqueKey.InternalKey(), uniqueKey.SmartblockType())
					}
					if err != nil {
						log.Errorf("get target object type %s: %s", targetObjectTypeId, err)
					}
					s.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, domain.TypeKey(uniqueKey.InternalKey())})
				}
			}
		},
	})
}
