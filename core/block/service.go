package block

import (
	"context"
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/ocache"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

const CName = "blockService"

var (
	ErrBlockNotFound       = errors.New("block not found")
	ErrBlockAlreadyOpen    = errors.New("block already open")
	ErrUnexpectedBlockType = errors.New("unexpected block type")
	ErrUnknownObjectType   = fmt.Errorf("unknown object type")
)

var log = logging.Logger("anytype-mw-service")

var (
	blockCacheTTL       = time.Minute
	blockCleanupTimeout = time.Second * 30
)

var (
	// quick fix for limiting file upload goroutines
	uploadFilesLimiter = make(chan struct{}, 8)
)

func init() {
	for i := 0; i < cap(uploadFilesLimiter); i++ {
		uploadFilesLimiter <- struct{}{}
	}
}

type Service interface {
	Do(id string, apply func(b smartblock.SmartBlock) error) error

	OpenBlock(ctx *state.Context, id string) error
	ShowBlock(ctx *state.Context, id string) error
	OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error)
	SetBreadcrumbs(ctx *state.Context, req pb.RpcBlockSetBreadcrumbsRequest) (err error)
	CloseBlock(id string) error
	CloseBlocks()
	CreateBlock(ctx *state.Context, req pb.RpcBlockCreateRequest) (string, error)
	CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error)
	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromTemplate(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, createState *state.State) (id string, newDetails *types.Struct, err error)
	DuplicateBlocks(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) ([]string, error)
	UnlinkBlock(ctx *state.Context, req pb.RpcBlockUnlinkRequest) error
	ReplaceBlock(ctx *state.Context, req pb.RpcBlockReplaceRequest) (newId string, err error)
	UpdateBlockContent(ctx *state.Context, req pb.RpcBlockUpdateContentRequest) (err error)

	MoveBlocks(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	MoveBlocksToNewPage(ctx *state.Context, req pb.RpcBlockListMoveToNewPageRequest) (linkId string, err error)
	ConvertChildrenToPages(req pb.RpcBlockListConvertChildrenToPagesRequest) (linkIds []string, err error)
	SetFields(ctx *state.Context, req pb.RpcBlockSetFieldsRequest) error
	SetFieldsList(ctx *state.Context, req pb.RpcBlockListSetFieldsRequest) error

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) (err error)
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) // you must copy original struct within the modifier in order to modify it

	GetRelations(objectId string) (relations []*model.Relation, err error)
	UpdateExtraRelations(ctx *state.Context, id string, relations []*model.Relation, createIfMissing bool) (err error)
	ModifyExtraRelations(ctx *state.Context, objectId string, modifier func(current []*model.Relation) ([]*model.Relation, error)) (err error)
	AddExtraRelations(ctx *state.Context, id string, relations []*model.Relation) (relationsWithKeys []*model.Relation, err error)
	RemoveExtraRelations(ctx *state.Context, id string, relationKeys []string) (err error)
	CreateSet(ctx *state.Context, req pb.RpcBlockCreateSetRequest) (linkId string, setId string, err error)

	ListAvailableRelations(objectId string) (aggregatedRelations []*model.Relation, err error)
	SetObjectTypes(ctx *state.Context, objectId string, objectTypes []string) (err error)
	AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionUpdateRequest) (err error)
	DeleteExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionDeleteRequest) (err error)

	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)

	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error)

	SplitBlock(ctx *state.Context, req pb.RpcBlockSplitRequest) (blockId string, err error)
	MergeBlock(ctx *state.Context, req pb.RpcBlockMergeRequest) error
	SetTextText(ctx *state.Context, req pb.RpcBlockSetTextTextRequest) error
	SetTextStyle(ctx *state.Context, contextId string, style model.BlockContentTextStyle, blockIds ...string) error
	SetTextChecked(ctx *state.Context, req pb.RpcBlockSetTextCheckedRequest) error
	SetTextColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetTextMark(ctx *state.Context, id string, mark *model.BlockContentTextMark, ids ...string) error
	SetBackgroundColor(ctx *state.Context, contextId string, color string, blockIds ...string) error
	SetAlign(ctx *state.Context, contextId string, align model.BlockAlign, blockIds ...string) (err error)
	SetLayout(ctx *state.Context, id string, layout model.ObjectTypeLayout) error

	FeaturedRelationAdd(ctx *state.Context, contextId string, relations ...string) error
	FeaturedRelationRemove(ctx *state.Context, contextId string, relations ...string) error

	TurnInto(ctx *state.Context, id string, style model.BlockContentTextStyle, ids ...string) error

	SetDivStyle(ctx *state.Context, contextId string, style model.BlockContentDivStyle, ids ...string) (err error)

	UploadFile(req pb.RpcUploadFileRequest) (hash string, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest, groupId string) error
	UploadBlockFileSync(ctx *state.Context, req pb.RpcBlockUploadRequest) (err error)
	CreateAndUploadFile(ctx *state.Context, req pb.RpcBlockFileCreateAndUploadRequest) (id string, err error)
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)

	Undo(ctx *state.Context, req pb.RpcBlockUndoRequest) (pb.RpcBlockUndoRedoCounter, error)
	Redo(ctx *state.Context, req pb.RpcBlockRedoRequest) (pb.RpcBlockUndoRedoCounter, error)

	SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) error
	SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) error
	SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) error

	DeleteArchivedObjects(req pb.RpcBlockListDeletePageRequest) error

	GetAggregatedRelations(req pb.RpcBlockDataviewRelationListAvailableRequest) (relations []*model.Relation, err error)
	DeleteDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewDeleteRequest) error
	UpdateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewUpdateRequest) error
	SetDataviewActiveView(ctx *state.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error
	CreateDataviewView(ctx *state.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error)
	AddDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationAddRequest) (relation *model.Relation, err error)
	UpdateDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationUpdateRequest) error
	DeleteDataviewRelation(ctx *state.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error
	AddDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionAddRequest) (opt *model.RelationOption, err error)
	UpdateDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) error
	DeleteDataviewRecordRelationOption(ctx *state.Context, req pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) error

	CreateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordCreateRequest) (*types.Struct, error)
	UpdateDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordUpdateRequest) error
	DeleteDataviewRecord(ctx *state.Context, req pb.RpcBlockDataviewRecordDeleteRequest) error

	BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) error
	BookmarkFetchSync(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) (err error)
	BookmarkCreateAndFetch(ctx *state.Context, req pb.RpcBlockBookmarkCreateAndFetchRequest) (id string, err error)

	SetRelationKey(ctx *state.Context, request pb.RpcBlockRelationSetKeyRequest) error
	AddRelationBlock(ctx *state.Context, request pb.RpcBlockRelationAddRequest) error

	Process() process.Service
	ProcessAdd(p process.Process) (err error)
	ProcessCancel(id string) error

	SimplePaste(contextId string, anySlot []*model.Block) (err error)

	GetDocInfo(ctx context.Context, id string) (info doc.DocInfo, err error)
	Wakeup(id string) (err error)

	MakeTemplate(id string) (templateId string, err error)
	MakeTemplateByObjectType(otId string) (templateId string, err error)
	CloneTemplate(id string) (templateId string, err error)
	ApplyTemplate(contextId, templateId string) error

	app.ComponentRunnable
}

func newOpenedBlock(sb smartblock.SmartBlock) *openedBlock {
	var ob = openedBlock{SmartBlock: sb}
	if sb.Type() != model.SmartBlockType_Breadcrumbs {
		// decode and store corresponding threadID for appropriate block
		if tid, err := thread.Decode(sb.Id()); err != nil {
			log.With("thread", sb.Id()).Warnf("can't restore thread ID: %v", err)
		} else {
			ob.threadId = tid
		}
	}
	return &ob
}

type openedBlock struct {
	smartblock.SmartBlock
	threadId thread.ID
}

func New() Service {
	return new(service)
}

type service struct {
	anytype     core.Service
	meta        meta.Service
	status      status.Service
	sendEvent   func(event *pb.Event)
	closed      bool
	linkPreview linkpreview.LinkPreview
	process     process.Service
	doc         doc.Service
	app         *app.App
	source      source.Service
	cache       ocache.OCache
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.meta = a.MustComponent(meta.CName).(meta.Service)
	s.status = a.MustComponent(status.CName).(status.Service)
	s.linkPreview = a.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	s.process = a.MustComponent(process.CName).(process.Service)
	s.sendEvent = a.MustComponent(event.CName).(event.Sender).Send
	s.source = a.MustComponent(source.CName).(source.Service)
	s.doc = a.MustComponent(doc.CName).(doc.Service)
	s.app = a
	s.cache = ocache.New(s.loadSmartblock)
	return
}

func (s *service) Run() (err error) {
	s.initPredefinedBlocks()
	s.testArchiveInconsistency() //// TODO: THIS IS TEMPORARY DEBUG, REMOVE ME
	return
}

func (s *service) initPredefinedBlocks() {
	ids := []string{
		// skip account because it is not a smartblock, it's a threadsDB
		s.anytype.PredefinedBlocks().Profile,
		s.anytype.PredefinedBlocks().Archive,
		s.anytype.PredefinedBlocks().Home,
		s.anytype.PredefinedBlocks().SetPages,
		s.anytype.PredefinedBlocks().MarketplaceType,
		s.anytype.PredefinedBlocks().MarketplaceRelation,
		s.anytype.PredefinedBlocks().MarketplaceTemplate,
	}
	for _, id := range ids {
		sb, err := s.newSmartBlock(id, &smartblock.InitContext{State: state.NewDoc(id, nil).(*state.State)})
		if err != nil {
			if err != smartblock.ErrCantInitExistingSmartblockWithNonEmptyState {
				log.Errorf("can't init predefined block: %v", err)
			}
		} else {
			sb.Close()
		}
	}
}

// TODO: THIS IS TEMPORARY DEBUG, REMOVE ME
func (s *service) testArchiveInconsistency() {
	var (
		archivedObjects   = make(map[string]bool)
		archivedObjectsSl []string
	)

	err := s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		for _, b := range b.Blocks() {
			if v := b.GetLink(); v != nil {
				if _, exists := archivedObjects[v.TargetBlockId]; !exists {
					archivedObjectsSl = append(archivedObjectsSl, v.TargetBlockId)
					archivedObjects[v.TargetBlockId] = false
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Errorf("ARCHIVE INCONSISTENCY: failed to open archive sb")
		return
	}

	archiveLinks, err := s.anytype.ObjectStore().GetOutboundLinksById(s.anytype.PredefinedBlocks().Archive)
	if err != nil {
		log.Errorf("ARCHIVE INCONSISTENCY: failed to get archive outbound links from localstore")
	}

	removed, added := slice.DifferenceRemovedAdded(archivedObjectsSl, archiveLinks)
	if len(added)+len(removed) > 0 {
		log.Debugf("ARCHIVE INCONSISTENCY: indexed outgoing links mismatch: added %v, removed %v", added, removed)
	}
	records, _, err := s.anytype.ObjectStore().Query(nil, database.Query{
		IncludeArchivedObjects: true,
	})
	if err != nil {
		log.Errorf("ARCHIVE INCONSISTENCY: failed to query objectstore")
		return
	}

	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, "id")
		if pbtypes.GetBool(rec.Details, bundle.RelationKeyIsArchived.String()) {
			if _, exists := archivedObjects[id]; !exists {
				log.Errorf("ARCHIVE INCONSISTENCY: object %s should not have archived flag", id)
			} else {
				archivedObjects[id] = true
			}
		} else {
			if _, exists := archivedObjects[id]; exists {
				log.Errorf("ARCHIVE INCONSISTENCY: object %s should have archived flag", id)
				archivedObjects[id] = true
			}
		}
	}

	for id, proceed := range archivedObjects {
		if !proceed {
			// this may be ok in case of recovery, so keep it debug
			log.Debugf("ARCHIVE INCONSISTENCY: object %s not found in objectsearch", id)
		}
	}
}

func (s *service) Anytype() core.Service {
	return s.anytype
}

func (s *service) OpenBlock(ctx *state.Context, id string) (err error) {
	ob, err := s.getSmartblock(context.TODO(), id)
	if err != nil {
		return
	}
	ob.Lock()
	defer ob.Unlock()
	ob.SetEventFunc(s.sendEvent)
	if v, hasOpenListner := ob.SmartBlock.(smartblock.SmartblockOpenListner); hasOpenListner {
		v.SmartblockOpened(ctx)
	}

	st := ob.NewState()
	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	if err = ob.Apply(st); err != nil {
		log.Errorf("failed to update lastOpenedDate: %s", err.Error())
	}

	if err = ob.Show(ctx); err != nil {
		return
	}

	if tid := ob.threadId; tid != thread.Undef && s.status != nil {
		var (
			fList = func() []string {
				ob.Lock()
				defer ob.Unlock()
				bs := ob.NewState()
				return bs.GetAllFileHashes(bs.FileRelationKeys())
			}
		)

		if newWatcher := s.status.Watch(tid, fList); newWatcher {
			ob.AddHook(func() { s.status.Unwatch(tid) }, smartblock.HookOnClose)
		}
	}
	return nil
}

func (s *service) ShowBlock(ctx *state.Context, id string) (err error) {
	return s.Do(id, func(b smartblock.SmartBlock) error {
		return b.Show(ctx)
	})
}

func (s *service) OpenBreadcrumbsBlock(ctx *state.Context) (blockId string, err error) {
	bs := editor.NewBreadcrumbs(s.meta)
	if err = bs.Init(&smartblock.InitContext{
		App:    s.app,
		Doc:    s.doc,
		Source: source.NewVirtual(s.anytype, model.SmartBlockType_Breadcrumbs),
	}); err != nil {
		return
	}
	bs.Lock()
	defer bs.Unlock()
	bs.SetEventFunc(s.sendEvent)
	ob := newOpenedBlock(bs)
	s.cache.Add(bs.Id(), ob)
	if err = bs.Show(ctx); err != nil {
		return
	}
	return bs.Id(), nil
}

func (s *service) CloseBlock(id string) error {
	err := s.Do(id, func(b smartblock.SmartBlock) error {
		b.BlockClose()
		return nil
	})
	if err != nil {
		return err
	}
	s.cache.Release(id)
	return nil
}

func (s *service) CloseBlocks() {
	s.cache.ForEach(func(v ocache.Object) (isContinue bool) {
		ob := v.(*openedBlock)
		ob.Lock()
		ob.BlockClose()
		ob.Unlock()
		s.cache.Reset(ob.Id())
		return true
	})
}

// SetPagesIsArchived is deprecated
func (s *service) SetPagesIsArchived(req pb.RpcBlockListSetPageIsArchivedRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		for _, blockId := range req.BlockIds {
			if req.IsArchived {
				err = archive.AddObject(blockId)
			} else {
				err = archive.RemoveObject(blockId)
			}
			if err != nil {
				log.Errorf("failed to archive %s: %s", blockId, err.Error())
			} else {
				anySucceed = true
			}
		}

		if !anySucceed {
			return err
		}

		return nil
	})
}

func (s *service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	return s.Do(collectionId, func(b smartblock.SmartBlock) error {
		coll, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unsupported sb block type: %T", b)
		}
		if value {
			return coll.AddObject(objectId)
		} else {
			return coll.RemoveObject(objectId)
		}
	})
}

func (s *service) SetPageIsFavorite(req pb.RpcObjectSetIsFavoriteRequest) (err error) {
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Home, req.ContextId, req.IsFavorite)
}

func (s *service) SetPageIsArchived(req pb.RpcObjectSetIsArchivedRequest) (err error) {
	return s.objectLinksCollectionModify(s.anytype.PredefinedBlocks().Archive, req.ContextId, req.IsArchived)
}

func (s *service) DeleteArchivedObjects(req pb.RpcBlockListDeletePageRequest) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		anySucceed := false
		for _, blockId := range req.BlockIds {
			if exists, _ := archive.HasObject(blockId); exists {
				if err = s.deleteObject(blockId); err == nil {
					archive.RemoveObject(blockId)
					anySucceed = true
				}
			}
		}

		if !anySucceed {
			return err
		}

		return nil
	})
}

func (s *service) DeleteArchivedObject(id string) (err error) {
	return s.Do(s.anytype.PredefinedBlocks().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		if exists, _ := archive.HasObject(id); exists {
			if err = s.deleteObject(id); err == nil {
				err = archive.RemoveObject(id)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *service) deleteObject(id string) (err error) {
	err = s.CloseBlock(id)
	if err != nil && err != ErrBlockNotFound {
		return err
	}

	return s.anytype.DeleteBlock(id)
}

func (s *service) CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation) (id string, newDetails *types.Struct, err error) {
	return s.CreateSmartBlockFromState(sbType, details, relations, state.NewDoc("", nil).NewState())
}

func (s *service) CreateSmartBlockFromTemplate(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, templateId string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateId != "" {
		if createState, err = s.stateFromTemplate(templateId, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return s.CreateSmartBlockFromState(sbType, details, relations, createState)
}

func (s *service) CreateSmartBlockFromState(sbType coresb.SmartBlockType, details *types.Struct, relations []*model.Relation, createState *state.State) (id string, newDetails *types.Struct, err error) {
	objectTypes := pbtypes.GetStringList(details, bundle.RelationKeyType.String())
	if objectTypes == nil {
		objectTypes = createState.ObjectTypes()
		if objectTypes == nil {
			objectTypes = pbtypes.GetStringList(createState.Details(), bundle.RelationKeyType.String())
		}
	}
	if len(objectTypes) == 0 {
		if ot, exists := bundle.DefaultObjectTypePerSmartblockType[sbType]; exists {
			objectTypes = []string{ot.URL()}
		} else {
			objectTypes = []string{bundle.TypeKeyPage.URL()}
		}
	}

	objType, err := objectstore.GetObjectType(s.anytype.ObjectStore(), objectTypes[0])
	if err != nil {
		return "", nil, fmt.Errorf("object type not found")
	}

	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			createState.SetDetail(k, v)
			if !createState.HasRelation(k) && !pbtypes.HasRelation(relations, k) {
				rel := pbtypes.GetRelation(objType.Relations, k)
				if rel == nil {
					return "", nil, fmt.Errorf("relation for detail %s not found", k)
				}
				relCopy := pbtypes.CopyRelation(rel)
				relCopy.Scope = model.Relation_object
				relations = append(relations, relCopy)
			}
		}
	}

	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(s.anytype.ProfileID()))

	csm, err := s.anytype.CreateBlock(sbType)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	id = csm.ID()
	createState.SetRootId(id)
	initCtx := &smartblock.InitContext{
		State:          createState,
		ObjectTypeUrls: objectTypes,
		Relations:      relations,
	}
	var sb smartblock.SmartBlock
	if sb, err = s.newSmartBlock(id, initCtx); err != nil {
		return id, nil, err
	}
	defer sb.Close()
	return id, sb.CombinedDetails(), nil
}

func (s *service) CreatePage(ctx *state.Context, groupId string, req pb.RpcBlockCreatePageRequest) (linkId string, pageId string, err error) {
	if req.ContextId != "" {
		var contextBlockType model.SmartBlockType
		if err = s.Do(req.ContextId, func(b smartblock.SmartBlock) error {
			contextBlockType = b.Type()
			return nil
		}); err != nil {
			return
		}

		if contextBlockType == model.SmartBlockType_Set {
			return "", "", basic.ErrNotSupported
		}
	}
	pageId, _, err = s.CreateSmartBlockFromTemplate(coresb.SmartBlockTypePage, req.Details, nil, req.TemplateId)
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	if req.ContextId == "" && req.TargetId == "" {
		// do not create a link
		return "", pageId, err
	}

	err = s.DoBasic(req.ContextId, func(b basic.Basic) error {
		linkId, err = b.Create(ctx, groupId, pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: pageId,
						Style:         model.BlockContentLink_Page,
					},
				},
			},
			Position: req.Position,
		})
		if err != nil {
			err = fmt.Errorf("link create error: %v", err)
		}
		return err
	})
	return
}

func (s *service) Process() process.Service {
	return s.process
}

func (s *service) ProcessAdd(p process.Process) (err error) {
	return s.process.Add(p)
}

func (s *service) ProcessCancel(id string) (err error) {
	return s.process.Cancel(id)
}

func (s *service) Close() error {
	return s.cache.Close()
}

// pickBlock returns opened smartBlock or opens smartBlock in silent mode
func (s *service) pickBlock(id string) (sb smartblock.SmartBlock, release func(), err error) {
	ob, err := s.getSmartblock(context.TODO(), id)
	if err != nil {
		return
	}
	return ob.SmartBlock, func() {
		s.cache.Release(id)
	}, nil
}

func (s *service) newSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := s.source.NewSource(id, false)
	if err != nil {
		return
	}
	switch sc.Type() {
	case model.SmartBlockType_Page:
		sb = editor.NewPage(s.meta, s, s, s, s.linkPreview)
	case model.SmartBlockType_Archive:
		sb = editor.NewArchive(s.meta)
	case model.SmartBlockType_Home:
		sb = editor.NewDashboard(s.meta, s)
	case model.SmartBlockType_Set:
		sb = editor.NewSet(s.meta, s)
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s.meta, s, s, s.linkPreview, s.sendEvent)
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		sb = editor.NewObjectType(s.meta, s)
	case model.SmartBlockType_BundledRelation,
		model.SmartBlockType_IndexedRelation:
		sb = editor.NewRelation(s.meta, s)
	case model.SmartBlockType_File:
		sb = editor.NewFiles(s.meta)
	case model.SmartBlockType_MarketplaceType:
		sb = editor.NewMarketplaceType(s.meta, s)
	case model.SmartBlockType_MarketplaceRelation:
		sb = editor.NewMarketplaceRelation(s.meta, s)
	case model.SmartBlockType_MarketplaceTemplate:
		sb = editor.NewMarketplaceTemplate(s.meta, s)
	case model.SmartBlockType_Template:
		sb = editor.NewTemplate(s.meta, s, s, s, s.linkPreview)
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate(s.meta, s, s, s, s.linkPreview)
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	sb.Lock()
	defer sb.Unlock()
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	if initCtx.App == nil {
		initCtx.App = s.app
	}
	if initCtx.Doc == nil {
		initCtx.Doc = s.doc
	}
	initCtx.Source = sc
	err = sb.Init(initCtx)
	return
}

func (s *service) stateFromTemplate(templateId, name string) (st *state.State, err error) {
	if err = s.Do(templateId, func(b smartblock.SmartBlock) error {
		if tmpl, ok := b.(*editor.Template); ok {
			st, err = tmpl.GetNewPageState(name)
		} else {
			return fmt.Errorf("not a template")
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("can't apply template: %v", err)
	}
	return
}

func (s *service) DoBasic(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.Basic); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *service) DoLinksCollection(id string, apply func(b basic.Basic) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.Basic); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("basic operation not available for this block type: %T", sb)
}

func (s *service) DoClipboard(id string, apply func(b clipboard.Clipboard) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(clipboard.Clipboard); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("clipboard operation not available for this block type: %T", sb)
}

func (s *service) DoText(id string, apply func(b stext.Text) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(stext.Text); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *service) DoFile(id string, apply func(b file.File) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(file.File); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("file operation not available for this block type: %T", sb)
}

func (s *service) DoBookmark(id string, apply func(b bookmark.Bookmark) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(bookmark.Bookmark); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("bookmark operation not available for this block type: %T", sb)
}

func (s *service) DoFileNonLock(id string, apply func(b file.File) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(file.File); ok {
		return apply(bb)
	}
	return fmt.Errorf("file non lock operation not available for this block type: %T", sb)
}

func (s *service) DoHistory(id string, apply func(b basic.IHistory) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(basic.IHistory); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("undo operation not available for this block type: %T", sb)
}

func (s *service) DoImport(id string, apply func(b _import.Import) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(_import.Import); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}

	return fmt.Errorf("import operation not available for this block type: %T", sb)
}

func (s *service) DoDataview(id string, apply func(b dataview.Dataview) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	if bb, ok := sb.(dataview.Dataview); ok {
		sb.Lock()
		defer sb.Unlock()
		return apply(bb)
	}
	return fmt.Errorf("text operation not available for this block type: %T", sb)
}

func (s *service) Do(id string, apply func(b smartblock.SmartBlock) error) error {
	sb, release, err := s.pickBlock(id)
	if err != nil {
		return err
	}
	defer release()
	sb.Lock()
	defer sb.Unlock()
	return apply(sb)
}

func (s *service) MakeTemplate(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st = b.NewState().Copy()
		return nil
	}); err != nil {
		return
	}
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(st.ObjectType()))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), st.ObjectType()})
	templateId, _, err = s.CreateSmartBlockFromState(coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *service) MakeTemplateByObjectType(otId string) (templateId string, err error) {
	if err = s.Do(otId, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
		return "", fmt.Errorf("can't open objectType: %v", err)
	}
	var st = state.NewDoc("", nil).(*state.State)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(otId))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), otId})
	templateId, _, err = s.CreateSmartBlockFromState(coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *service) CloneTemplate(id string) (templateId string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		return nil
	}); err != nil {
		return
	}
	templateId, _, err = s.CreateSmartBlockFromState(coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *service) ApplyTemplate(contextId, templateId string) error {
	return s.Do(contextId, func(b smartblock.SmartBlock) error {
		orig := b.NewState().ParentState()
		ts, err := s.stateFromTemplate(templateId, pbtypes.GetString(orig.Details(), bundle.RelationKeyName.String()))
		if err != nil {
			return err
		}
		ts.SetRootId(contextId)
		ts.SetParent(orig)
		ts.BlocksInit(orig)
		ts.InjectDerivedDetails()
		return b.Apply(ts, smartblock.NoRestrictions)
	})
}

func (s *service) AllDescendantIds(rootBlockId string, allBlocks map[string]*model.Block) []string {
	var (
		// traversal queue
		queue = []string{rootBlockId}
		// traversed IDs collected (including root)
		traversed = []string{rootBlockId}
	)

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		chIDs := allBlocks[next].ChildrenIds
		traversed = append(traversed, chIDs...)
		queue = append(queue, chIDs...)
	}

	return traversed
}

func (s *service) ResetToState(pageId string, state *state.State) (err error) {
	return s.Do(pageId, func(sb smartblock.SmartBlock) error {
		return sb.ResetToVersion(state)
	})
}

func (s *service) loadSmartblock(ctx context.Context, id string) (value ocache.Object, err error) {
	sb, err := s.newSmartBlock(id, nil)
	if err != nil {
		return
	}
	value = newOpenedBlock(sb)
	return
}

func (s *service) getSmartblock(ctx context.Context, id string) (ob *openedBlock, err error) {
	val, err := s.cache.Get(ctx, id)
	if err != nil {
		return
	}
	return val.(*openedBlock), nil
}
