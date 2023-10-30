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
	"github.com/anyproto/anytype-heart/util/slice"
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
				targetObjectTypeID := pbtypes.GetString(s.Details(), bundle.RelationKeyTargetObjectType.String())
				if targetObjectTypeID != "" {
					typeKey, err := t.getTypeKeyById(targetObjectTypeID)
					if err != nil {
						log.Errorf("get target object type %s: %s", targetObjectTypeID, err)
					}
					s.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, typeKey})
				}
			}
		},
	})
}

func (t *Template) UpdateTypeKey(st *state.State) error {
	objectTypeID := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
	if objectTypeID != "" {
		typeKey, err := t.getTypeKeyById(objectTypeID)
		if err != nil {
			return fmt.Errorf("get target object type %s: %w", objectTypeID, err)
		}
		st.SetObjectTypeKey(typeKey)
		return nil
	}
	updatedTypeKeys := slice.Remove(t.ObjectTypeKeys(), bundle.TypeKeyTemplate)
	st.SetObjectTypeKeys(updatedTypeKeys)
	return nil
}

func (t *Template) getTypeKeyById(typeId string) (domain.TypeKey, error) {
	obj, err := t.objectStore.GetDetails(typeId)
	if err != nil {
		return "", err
	}
	rawUniqueKey := pbtypes.GetString(obj.Details, bundle.RelationKeyUniqueKey.String())
	uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
	if err != nil {
		return "", err
	}
	return domain.TypeKey(uniqueKey.InternalKey()), nil
}
