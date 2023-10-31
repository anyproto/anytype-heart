package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type Template struct {
	*Page

	picker getblock.ObjectGetter
}

func (f *ObjectFactory) newTemplate(sb smartblock.SmartBlock) *Template {
	return &Template{
		Page:   f.newPage(sb),
		picker: f.picker,
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

// GetNewPageState returns state that can be safely used to create the new document
// it has not localDetails set
func (t *Template) GetNewPageState(name string) (st *state.State, err error) {
	st = t.NewState().Copy()

	if err = t.updateTypeKey(st); err != nil {
		return nil, err
	}

	st.RemoveDetail(bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyTemplateIsBundled.String())
	st.SetDetailAndBundledRelation(bundle.RelationKeySourceObject, pbtypes.String(t.Id()))
	// clean-up local details from the template state
	st.SetLocalDetails(nil)

	if name != "" {
		st.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(name))
		if title := st.Get(template.TitleBlockId); title != nil {
			title.Model().GetText().Text = ""
		}
	}
	return
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

func (t *Template) updateTypeKey(st *state.State) error {
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
