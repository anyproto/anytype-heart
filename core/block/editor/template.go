package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Template struct {
	*Page

	picker getblock.ObjectGetter
}

func NewTemplate(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	fileBlockService file.BlockService,
	picker getblock.ObjectGetter,
	bookmarkService bookmark.BookmarkService,
	systemObjectService system_object.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.Service,
	eventSender event.Sender,
) *Template {
	return &Template{
		Page: NewPage(
			sb,
			objectStore,
			anytype,
			fileBlockService,
			picker,
			bookmarkService,
			systemObjectService,
			tempDirProvider,
			sbtProvider,
			layoutConverter,
			fileService,
			eventSender,
		),
		picker: picker,
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

func (t *Template) getTypeKeyById(typeId string) (domain.TypeKey, error) {
	var typeKey domain.TypeKey
	err := getblock.Do(t.picker, typeId, func(sb smartblock.SmartBlock) error {
		typeKey = domain.TypeKey(sb.UniqueKeyInternal())
		return nil
	})
	return typeKey, err
}

// GetNewPageState returns state that can be safely used to create the new document
// it has not localDetails set
func (t *Template) GetNewPageState(name string) (st *state.State, err error) {
	st = t.NewState().Copy()
	objectTypeID := pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String())
	if objectTypeID != "" {
		typeKey, err := t.getTypeKeyById(objectTypeID)
		if err != nil {
			return nil, fmt.Errorf("get target object type %s: %s", objectTypeID, err)
		}
		st.SetObjectTypeKey(typeKey)
	}
	st.RemoveDetail(bundle.RelationKeyTargetObjectType.String(), bundle.RelationKeyTemplateIsBundled.String())
	st.SetDetail(bundle.RelationKeySourceObject.String(), pbtypes.String(t.Id()))
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
