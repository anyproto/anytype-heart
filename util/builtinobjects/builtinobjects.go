package builtinobjects

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
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

func New() BuiltinObjects {
	return new(builtinObjects)
}

type BuiltinObjects interface {
	Inject(ctx context.Context) error
	app.Component
}

type builtinObjects struct {
	cancel  func()
	l       sync.Mutex
	source  source.Service
	service block.Service
	idsMap  map[string]string
}

func (b *builtinObjects) Init(a *app.App) (err error) {
	b.source = a.MustComponent(source.CName).(source.Service)
	b.service = a.MustComponent(block.CName).(block.Service)
	b.cancel = func() {}
	return
}

func (b *builtinObjects) Name() (name string) {
	return CName
}

func (b *builtinObjects) Inject(ctx context.Context) (err error) {
	var ctx2 context.Context
	b.l.Lock()
	ctx2, b.cancel = context.WithCancel(ctx)
	b.l.Unlock()
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
		if err = b.createObject(ctx2, rd); err != nil {
			return
		}
	}

	return
}

func (b *builtinObjects) createObject(ctx context.Context, rd io.ReadCloser) (err error) {
	defer rd.Close()
	data, err := ioutil.ReadAll(rd)
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return
	}
	st := state.NewDocFromSnapshot("", snapshot).(*state.State)
	oldId := st.RootId()
	newId, exists := b.idsMap[oldId]
	if !exists {
		return fmt.Errorf("new id not found for '%s'", st.RootId())
	}

	st.SetRootId(newId)
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
	}

	sbt, err := smartblock.SmartBlockTypeFromID(newId)
	if err != nil {
		return err
	}

	_, _, err = b.service.CreateSmartBlockFromState(ctx, sbt, nil, nil, st)
	if pbtypes.GetBool(st.CombinedDetails(), bundle.RelationKeyIsFavorite.String()) {
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
	return
}
