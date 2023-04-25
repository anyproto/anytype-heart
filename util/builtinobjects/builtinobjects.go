package builtinobjects

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/widget"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "builtinobjects"

//go:embed data/bundled_objects.zip
var objectsZip []byte

//go:embed data/migration_dashboard.zip
var migrationDashboardZip []byte

var log = logging.Logger("anytype-mw-builtinobjects")

const (
	analyticsContext         = "get-started"
	builtInDashboardObjectID = "bafyreiffhfg6rxuerttu2uhlvd6hlvhh4w3cd3iu6d7pge6eypwhw6mlsa"
	bookmarkSetObjectID      = "bafyecaocfyfix22kzhfqt42r5xk2ngzwwmxchnhzcsz6akx4pqd6evez"
	everythingSetObjectID    = "bafyedkxh25dd4ij7nlyogawdc6bsmj7skiqcjiwbckovqzqnacxx57mo"
	videosViewID             = "c9e117ea-f101-491c-9622-524ec2a4d529"
	mediaViewID              = "e83e70ab-0601-4ab7-abd9-d4dc09b9e703"

	injectionTimeout = 30 * time.Second
)

type BuiltinObjects interface {
	app.ComponentRunnable

	InjectMigrationDashboard() error
}

type builtinObjects struct {
	cancel      func()
	source      source.Service
	service     *block.Service
	relService  relation2.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
	coreService core.Service

	createBuiltinObjects bool
	idsMap               map[string]string
}

func New(sbtProvider typeprovider.SmartBlockTypeProvider) BuiltinObjects {
	return &builtinObjects{
		sbtProvider: sbtProvider,
	}
}

func (b *builtinObjects) Init(a *app.App) (err error) {
	b.source = a.MustComponent(source.CName).(source.Service)
	b.service = a.MustComponent(block.CName).(*block.Service)
	b.createBuiltinObjects = a.MustComponent(config.CName).(*config.Config).CreateBuiltinObjects
	b.relService = a.MustComponent(relation2.CName).(relation2.Service)
	b.coreService = a.MustComponent(core.CName).(core.Service)
	b.cancel = func() {}
	return
}

func (b *builtinObjects) Name() (name string) {
	return CName
}

func (b *builtinObjects) Run(context.Context) (err error) {
	if !b.createBuiltinObjects {
		// import only for new accounts
		return
	}

	// todo: return fixed builtin objects with correct sbtype
	var ctx context.Context
	ctx, b.cancel = context.WithCancel(context.Background())
	start := time.Now()

	err = b.inject(ctx)
	if err != nil {
		log.Errorf("failed to import builtinObjects: %s", err.Error())
	}

	spent := time.Now().Sub(start)
	if spent > injectionTimeout {
		log.Debugf("built-in objects injection time exceeded timeout of %s and is %s", injectionTimeout.String(), spent.String())
	}

	return
}

func (b *builtinObjects) InjectMigrationDashboard() (err error) {
	zr, err := zip.NewReader(bytes.NewReader(migrationDashboardZip), int64(len(migrationDashboardZip)))
	if err != nil {
		return
	}
	if len(zr.File) != 1 {
		return fmt.Errorf("one file expected in migration_dashboard.zip. Found %d instead", len(zr.File))
	}
	ctx := context.Background()
	obj, err := b.service.CreateTreeObject(ctx, coresb.SmartBlockTypePage, func(id string) *sb.InitContext {
		return &sb.InitContext{
			IsNewObject: true,
			Ctx:         ctx,
		}
	})
	if err != nil {
		return
	}

	zf := zr.File[0]
	id := strings.TrimSuffix(zf.Name, filepath.Ext(zf.Name))
	newId := obj.Id()
	b.idsMap[id] = newId

	b.handleSpaceDashboard(obj.Id())

	rd, err := zf.Open()
	if err != nil {
		return
	}
	if err = b.createObject(rd); err != nil {
		return
	}
	return nil
}

func (b *builtinObjects) inject(ctx context.Context) (err error) {
	zr, err := zip.NewReader(bytes.NewReader(objectsZip), int64(len(objectsZip)))
	if err != nil {
		return
	}
	b.idsMap = make(map[string]string, len(zr.File))
	isSpaceDashboardIDFound := false
	for _, zf := range zr.File {
		id := strings.TrimSuffix(zf.Name, filepath.Ext(zf.Name))
		var sbt smartblock.SmartBlockType
		if id == builtInDashboardObjectID {
			isSpaceDashboardIDFound = true
			sbt = smartblock.SmartBlockTypePage
		} else {
			sbt, err = b.sbtProvider.Type(id)
		}
		if err != nil {
			sbt, err = SmartBlockTypeFromThreadID(id)
			if err != nil {

				return err
			}
		}
		if err := sbt.Valid(); err != nil {
			sbt = smartblock.SmartBlockTypePage
			log.Errorf("failed to get smartblock type for %s: %s; fallback to page", id, err.Error())
			// todo: this this TEMP! we shouldn't get incorrect ids here, need to fix zip file
			//return fmt.Errorf("invalid smartblock type: %v %v", sbt, err)
		}
		if sbt == smartblock.SmartBlockTypeSubObject {
			// todo: probably subobjects are broken here
			// preserve original id for subobjects, it makes no sense to replace them and also it breaks the grouping
			b.idsMap[id] = id
			continue
		}

		// create object
		obj, err := b.service.CreateTreeObject(ctx, sbt, func(id string) *sb.InitContext {
			return &sb.InitContext{
				IsNewObject: true,
				Ctx:         ctx,
			}
		})
		if err != nil {
			return err
		}
		newId := obj.Id()
		if id == builtInDashboardObjectID {
			b.handleSpaceDashboard(newId)
		}
		b.idsMap[id] = newId
	}

	if !isSpaceDashboardIDFound {
		panic("Workspace dashboard object file was not found in built-in objects")
	}

	for _, zf := range zr.File {
		rd, e := zf.Open()
		if e != nil {
			return e
		}
		if err = b.createObject(rd); err != nil {
			return
		}
	}
	return nil
}

func (b *builtinObjects) handleSpaceDashboard(id string) {
	if err := b.service.SetDetails(nil, pb.RpcObjectSetDetailsRequest{
		ContextId: b.coreService.PredefinedBlocks().Account,
		Details: []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeySpaceDashboardId.String(),
				Value: pbtypes.String(id),
			},
		},
	}); err != nil {
		log.Errorf("Failed to set SpaceDashboardId relation to Account object: %s", err.Error())
	}
	b.createSpaceDashboardWidget(id)
}

func (b *builtinObjects) createSpaceDashboardWidget(id string) {
	targetID, err := b.getFirstWidgetBlockId()
	if err != nil {
		log.Errorf(err.Error())
		return
	}

	if _, err := b.service.CreateWidgetBlock(nil, &pb.RpcBlockCreateWidgetRequest{
		ContextId:    b.coreService.PredefinedBlocks().Widgets,
		TargetId:     targetID,
		Position:     model.Block_Top,
		WidgetLayout: widget.LayoutLink,
		Block: &model.Block{
			Id:          "",
			ChildrenIds: nil,
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: id,
					Style:         model.BlockContentLink_Page,
					IconSize:      model.BlockContentLink_SizeNone,
					CardStyle:     model.BlockContentLink_Inline,
					Description:   model.BlockContentLink_None,
				},
			},
		},
	}); err != nil {
		log.Errorf("Failed to link SpaceDashboard to Widget object: %s", err.Error())
	}
}

func (b *builtinObjects) getFirstWidgetBlockId() (string, error) {
	w, err := b.service.GetObject(context.Background(), b.coreService.PredefinedBlocks().Account, b.coreService.PredefinedBlocks().Widgets)
	if err != nil {
		return "", fmt.Errorf("failed to get Widget object: %s", err.Error())
	}
	if len(w.Blocks()) < 2 {
		return "", fmt.Errorf("failed to get first block of Widget object: %s", err.Error())
	}
	if w.Blocks()[1] == nil {
		return "", fmt.Errorf("failed to get id of first block of Widget object: %s", err.Error())
	}
	return w.Blocks()[1].Id, nil
}

func (b *builtinObjects) createObject(rd io.ReadCloser) (err error) {
	defer rd.Close()
	data, err := ioutil.ReadAll(rd)
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		snapshotWithType := &pb.SnapshotWithType{}
		if err = snapshotWithType.Unmarshal(data); err != nil {
			return
		}
		snapshot = snapshotWithType.Snapshot
	}

	isFavorite := pbtypes.GetBool(snapshot.Data.Details, bundle.RelationKeyIsFavorite.String())
	isArchived := pbtypes.GetBool(snapshot.Data.Details, bundle.RelationKeyIsArchived.String())
	if isArchived {
		return fmt.Errorf("object has isarchived == true")
	}
	st := state.NewDocFromSnapshot("", snapshot).(*state.State)
	oldId := st.RootId()
	newId, exists := b.idsMap[oldId]
	if !exists {
		return fmt.Errorf("new id not found for '%s'", st.RootId())
	}

	st.SetRootId(newId)
	a := st.Get(newId)
	m := a.Model()
	sbt, err := b.sbtProvider.Type(newId)
	if sbt == smartblock.SmartBlockTypeSubObject {
		ot, err := bundle.TypeKeyFromUrl(pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyType.String()))
		if err != nil {
			return err
		}
		_, _, err = b.service.CreateObject(&pb.RpcObjectCreateRequest{Details: st.CombinedDetails()}, ot)
		if err != nil {
			return err
		}
	}

	f := m.GetFields().GetFields()
	if f == nil {
		f = make(map[string]*types.Value)
	}
	m.Fields = &types.Struct{Fields: f}
	f["analyticsContext"] = pbtypes.String(analyticsContext)
	if f["analyticsOriginalId"] == nil {
		// in case we already have analyticsOriginalId do not update it
		f["analyticsOriginalId"] = pbtypes.String(oldId)
	}

	st.Set(simple.New(m))
	rels := relationutils.MigrateRelationModels(st.OldExtraRelations())
	st.AddRelationLinks(rels...)

	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String(), bundle.RelationKeyLastOpenedDate.String(), bundle.RelationKeyLinks.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyWorkspaceId.String(), pbtypes.String(b.service.Anytype().PredefinedBlocks().Account))
	st.InjectDerivedDetails()
	if err = b.validate(st); err != nil {
		return
	}

	st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch a := bl.(type) {
		case link.Block:
			newTarget := b.idsMap[a.Model().GetLink().TargetBlockId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", st.RootId()).Errorf("cant find target id for link: %s", a.Model().GetLink().TargetBlockId)
				return true
			}

			a.Model().GetLink().TargetBlockId = newTarget
			st.Set(simple.New(a.Model()))
		case bookmark.Block:
			newTarget := b.idsMap[a.Model().GetBookmark().TargetObjectId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", oldId).Errorf("cant find target id for bookmark: %s", a.Model().GetBookmark().TargetObjectId)
				return true
			}

			a.Model().GetBookmark().TargetObjectId = newTarget
			st.Set(simple.New(a.Model()))
		case text.Block:
			for i, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				newTarget := b.idsMap[mark.Param]
				if newTarget == "" {
					log.With("object", oldId).Errorf("cant find target id for mention: %s", mark.Param)
					continue
				}

				a.Model().GetText().GetMarks().GetMarks()[i].Param = newTarget
			}
			st.Set(simple.New(a.Model()))
		case dataview.Block:
			// TODO: temporary solution of GO-1034, will be removed after GO-948 merge
			b.putFirstRelationInViews(st, oldId, &a)
			oldTarget := a.Model().GetDataview().TargetObjectId
			if oldTarget == "" {
				return true
			}
			newTarget := b.idsMap[oldTarget]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", oldId).Errorf("cant find target id for dataview: %s", oldTarget)
				return true
			}

			a.Model().GetDataview().TargetObjectId = newTarget
			st.Set(simple.New(a.Model()))
		}
		return true
	})

	for k, v := range st.Details().GetFields() {
		rel, err := bundle.GetRelation(bundle.RelationKey(k))
		if err != nil {
			log.With("object", oldId).Errorf("failed to find relation %s: %s", k, err.Error())
			continue
		}
		if rel.Format != model.RelationFormat_object && rel.Format != model.RelationFormat_tag && rel.Format != model.RelationFormat_status {
			continue
		}

		vals := pbtypes.GetStringListValue(v)
		for i, val := range vals {
			if bundle.HasRelation(val) {
				continue
			}
			newTarget, _ := b.idsMap[val]
			if newTarget == "" {
				log.With("object", oldId).Errorf("cant find target id for relation %s: %s", k, val)
				continue
			}
			vals[i] = newTarget

		}
		st.SetDetail(k, pbtypes.StringList(vals))
	}
	start := time.Now()
	err = b.service.Do(newId, func(sb sb.SmartBlock) error {
		return history.ResetToVersion(sb, st)
	})
	if err != nil {
		return err
	}
	err = b.service.Do(newId, func(b sb.SmartBlock) error {
		return nil
	})

	log.With("timeMs", time.Now().Sub(start).Milliseconds()).Info("creating debug obj")

	if isFavorite {
		err = b.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newId, IsFavorite: true})
		if err != nil {
			log.Errorf("failed to set isFavorite when importing object %s(originally %s): %s", newId, oldId, err.Error())
		}
	}
	return err
}

func (b *builtinObjects) validate(st *state.State) (err error) {
	var relKeys []string
	for _, rel := range st.PickRelationLinks() {
		if !bundle.HasRelation(rel.Key) {
			// todo: temporarily, make this as error
			log.Errorf("builtin objects should not contain custom relations, got %s in %s(%s)", rel.Key, st.RootId(), pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()))
		}
	}

	st.Iterate(func(b simple.Block) (isContinue bool) {
		if rb, ok := b.(relation.Block); ok {
			relKeys = append(relKeys, rb.Model().GetRelation().Key)
		}
		return true
	})
	for _, rk := range relKeys {
		if !st.HasRelation(rk) {
			return fmt.Errorf("bundled template validation: relation '%v' exists in block but not in extra relations", rk)
		}
	}
	return nil
}

func (b *builtinObjects) Close(ctx context.Context) (err error) {
	if b.cancel != nil {
		b.cancel()
	}
	return
}

func (b *builtinObjects) putFirstRelationInViews(st *state.State, oldID string, bl *dataview.Block) {
	var viewID string

	switch oldID {
	case bookmarkSetObjectID:
		viewID = videosViewID
	case everythingSetObjectID:
		viewID = mediaViewID
		(*bl).AddRelation(&model.RelationLink{
			Key:    bundle.RelationKeySource.String(),
			Format: model.RelationFormat_url,
		})
	default:
		return
	}

	if err := (*bl).RemoveViewRelations(viewID, []string{bundle.RelationKeySource.String()}); err != nil {
		return
	}
	v, err := (*bl).GetView(viewID)
	if err != nil {
		return
	}

	v.Relations = append([]*model.BlockContentDataviewRelation{{
		Key:       bundle.RelationKeySource.String(),
		IsVisible: true,
	}}, v.Relations...)
	st.Set(simple.New((*bl).Model()))
}

func SmartBlockTypeFromThreadID(id string) (coresb.SmartBlockType, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return coresb.SmartBlockTypePage, err
	}

	rawid := tid.KeyString()
	// skip version
	_, n := uvarint(rawid)
	// skip variant
	_, n2 := uvarint(rawid[n:])
	blockType, _ := uvarint(rawid[n+n2:])

	// checks in order to detect invalid sb type
	if err := coresb.SmartBlockType(blockType).Valid(); err != nil {
		return coresb.SmartBlockTypePage, nil
		return 0, err
	}

	return coresb.SmartBlockType(blockType), nil
}

func uvarint(buf string) (uint64, int) {
	var x uint64
	var s uint
	// we have a binary string so we can't use a range loope
	for i := 0; i < len(buf); i++ {
		b := buf[i]
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				return 0, -(i + 1) // overflow
			}
			return x | uint64(b)<<s, i + 1
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, 0
}
