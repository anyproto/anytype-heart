package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Template struct {
	basic.PlaceholdersOwner
	*Page
}

func (f *ObjectFactory) newTemplate(spaceId string, sb smartblock.SmartBlock) *Template {
	return &Template{
		Page: f.newPage(spaceId, sb),
	}
}

func (t *Template) Init(ctx *smartblock.InitContext) (err error) {
	if err = t.Page.Init(ctx); err != nil {
		return
	}

	if !ctx.IsNewObject {
		migrateFilesToObjects(t, t.fileObjectService)(ctx.State)
	}

	return
}

func (t *Template) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	parent := t.Page.CreationStateMigration(ctx)

	return migration.Compose(parent, migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			if t.Type() == coresb.SmartBlockTypeTemplate && (len(t.ObjectTypeKeys()) != 2) {
				targetObjectTypeId := s.Details().GetString(bundle.RelationKeyTargetObjectType)
				if targetObjectTypeId != "" {
					uniqueKey, err := t.objectStore.GetUniqueKeyById(targetObjectTypeId)
					if err == nil && uniqueKey.SmartblockType() != coresb.SmartBlockTypeObjectType {
						err = fmt.Errorf("unique key %s has wrong smartblock type %d", uniqueKey.InternalKey(), uniqueKey.SmartblockType())
					}
					if err != nil {
						log.Errorf("get target object type %s: %s", targetObjectTypeId, err)
						return
					}
					s.SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTemplate, domain.TypeKey(uniqueKey.InternalKey())})
				}
			}
		},
	})
}

func (t *Template) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (t *Template) SetPlaceholders(ctx session.Context, placeholders []domain.TemplatePlaceholder) error {
	s := t.NewStateCtx(ctx)

	// Get existing placeholders from store
	placeholdersStruct := s.GetSubObjectCollection(template.PlaceholdersStoreKey)
	existing := make(map[string]string)
	if placeholdersStruct != nil && placeholdersStruct.Fields != nil {
		for k, v := range placeholdersStruct.Fields {
			existing[k] = v.GetStringValue()
		}
	}

	// Update placeholders
	for _, p := range placeholders {
		key := p.RelationKey.String()
		if p.Type == model.TemplatePlaceholderType_TemplatePlaceholderNone {
			delete(existing, key)
			s.RemoveFromStore([]string{template.PlaceholdersStoreKey, key})
		} else {
			existing[key] = domain.PlaceholderTypeToString(p.Type)
			s.SetInStore([]string{template.PlaceholdersStoreKey, key}, pbtypes.String(domain.PlaceholderTypeToString(p.Type)))
		}
	}

	return t.Apply(s)
}

func (t *Template) GetPlaceholders() []domain.TemplatePlaceholder {
	st := t.NewState()
	placeholdersStruct := st.GetSubObjectCollection(template.PlaceholdersStoreKey)
	if placeholdersStruct == nil || placeholdersStruct.Fields == nil {
		return nil
	}

	var placeholders []domain.TemplatePlaceholder
	for relKey, typeValue := range placeholdersStruct.Fields {
		placeholderType, err := domain.ParsePlaceholderType(typeValue.GetStringValue())
		if err != nil {
			continue
		}
		placeholders = append(placeholders, domain.TemplatePlaceholder{
			RelationKey: domain.RelationKey(relKey),
			Type:        placeholderType,
		})
	}
	return placeholders
}
