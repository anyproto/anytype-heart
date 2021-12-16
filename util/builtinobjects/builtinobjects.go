package builtinobjects

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "builtinobjects"

//go:embed data/bundled_objects.zip
var objectsZip []byte

var log = logging.Logger("anytype-mw-builtinobjects")

const (
	analyticsContext = "get-started"
)

func New() BuiltinObjects {
	return new(builtinObjects)
}

type BuiltinObjects interface {
	app.ComponentRunnable
}

type builtinObjects struct {
	cancel     func()
	l          sync.Mutex
	source     source.Service
	service    block.Service
	newAccount bool
	idsMap     map[string]string
}

func (b *builtinObjects) Init(a *app.App) (err error) {
	b.source = a.MustComponent(source.CName).(source.Service)
	b.service = a.MustComponent(block.CName).(block.Service)
	b.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount

	b.cancel = func() {}
	return
}

func (b *builtinObjects) Name() (name string) {
	return CName
}

func (b *builtinObjects) Run() (err error) {
	if !b.newAccount {
		// import only for new accounts
		return
	}

	var ctx context.Context
	ctx, b.cancel = context.WithCancel(context.Background())
	go func() {
		err = b.inject(ctx)
		if err != nil {
			log.Errorf("failed to import builtinObjects: %s", err.Error())
		}
	}()

	return
}

func (b *builtinObjects) inject(ctx context.Context) (err error) {
	zr, err := zip.NewReader(bytes.NewReader(objectsZip), int64(len(objectsZip)))
	if err != nil {
		return
	}
	b.idsMap = make(map[string]string, len(zr.File))
	for _, zf := range zr.File {
		id := strings.TrimSuffix(zf.Name, filepath.Ext(zf.Name))
		sbt, err := smartblock.SmartBlockTypeFromID(id)
		if err != nil {
			return err
		}
		tid, err := threads.ThreadCreateID(thread.AccessControlled, sbt)
		if err != nil {
			return err
		}
		b.idsMap[id] = tid.String()
	}

	for _, zf := range zr.File {
		rd, e := zf.Open()
		if e != nil {
			return e
		}
		if err = b.createObject(ctx, rd); err != nil {
			return
		}
	}
	return nil
}

func (b *builtinObjects) createObject(ctx context.Context, rd io.ReadCloser) (err error) {
	defer rd.Close()
	data, err := ioutil.ReadAll(rd)
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return
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
	f := m.GetFields().GetFields()
	if f == nil {
		f = make(map[string]*types.Value)
	}
	m.Fields = &types.Struct{Fields: f}
	f["analyticsContext"] = pbtypes.String(analyticsContext)
	f["analyticsOriginalId"] = pbtypes.String(oldId)

	st.Set(simple.New(m))

	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
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
				log.Errorf("cant find target id for link: %s", a.Model().GetLink().TargetBlockId)
				return true
			}

			a.Model().GetLink().TargetBlockId = newTarget
			st.Set(simple.New(a.Model()))
		case text.Block:
			for i, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				newTarget := b.idsMap[mark.Param]
				if newTarget == "" {
					log.Errorf("cant find target id for mentrion: %s", mark.Param)
					continue
				}

				a.Model().GetText().GetMarks().GetMarks()[i].Param = newTarget
			}
			st.Set(simple.New(a.Model()))
		}
		return true
	})

	for k, v := range st.Details().GetFields() {
		rel, err := bundle.GetRelation(bundle.RelationKey(k))
		if err != nil {
			log.Errorf("failed to find relation %s: %s", k, err.Error())
			continue
		}
		if rel.Format != model.RelationFormat_object {
			continue
		}

		vals := pbtypes.GetStringListValue(v)
		for i, val := range vals {
			newTarget, _ := b.idsMap[val]
			if newTarget == "" {
				log.Errorf("cant find target id for relation %s: %s", k, val)
				continue
			}
			vals[i] = newTarget

		}
		st.SetDetail(k, pbtypes.StringList(vals))
	}

	sbt, err := smartblock.SmartBlockTypeFromID(newId)
	if err != nil {
		return err
	}

	_, _, err = b.service.CreateSmartBlockFromState(ctx, sbt, nil, nil, st)
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
	for _, rel := range st.ExtraRelations() {
		if !bundle.HasRelation(rel.Key) {
			// todo: temporarily, make this as error
			log.Errorf("builtin objects should not contain custom relations, got %s in %s(%s)", rel.Name, st.RootId(), pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()))
			//return fmt.Errorf("builtin objects should not contain custom relations, got %s in %s(%s)", rel.Name, st.RootId(), pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()))
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

func (b *builtinObjects) Close() (err error) {
	if b.cancel != nil {
		b.cancel()
	}
	return
}
