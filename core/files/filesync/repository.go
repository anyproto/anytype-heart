package filesync

import (
	"context"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

var errNoRows = fmt.Errorf("no rows")

type anystoreFileRepository struct {
	arenaPool *anyenc.ArenaPool
	coll      anystore.Collection

	ctx       context.Context
	ctxCancel context.CancelFunc

	updateCh      chan FileInfo
	subscriptions map[chan FileInfo]struct{}

	subscribeCh   chan chan FileInfo
	unsubscribeCh chan chan FileInfo
}

func newAnystoreFileRepository(coll anystore.Collection) *anystoreFileRepository {
	ctx, cancel := context.WithCancel(context.Background())
	return &anystoreFileRepository{
		arenaPool:     &anyenc.ArenaPool{},
		coll:          coll,
		ctx:           ctx,
		ctxCancel:     cancel,
		updateCh:      make(chan FileInfo),
		subscriptions: make(map[chan FileInfo]struct{}),
		subscribeCh:   make(chan chan FileInfo),
		unsubscribeCh: make(chan chan FileInfo),
	}
}

func (r *anystoreFileRepository) runSubscriptions() {
	go func() {
		for {
			select {
			case ch := <-r.subscribeCh:
				r.subscriptions[ch] = struct{}{}
			case ch := <-r.unsubscribeCh:
				if _, ok := r.subscriptions[ch]; ok {
					delete(r.subscriptions, ch)
					close(ch)
				}
			case it := <-r.updateCh:
				for sub := range r.subscriptions {
					sub <- it
				}
			case <-r.ctx.Done():
				return
			}
		}
	}()
}

func (r *anystoreFileRepository) subscribe() chan FileInfo {
	ch := make(chan FileInfo)
	r.subscribeCh <- ch
	return ch
}

func (r *anystoreFileRepository) unsubscribe(ch chan FileInfo) {
	// Drain the channel
	select {
	case <-ch:
	default:
	}

	r.unsubscribeCh <- ch
}

func (r *anystoreFileRepository) upsert(file FileInfo) error {
	arena := r.arenaPool.Get()
	defer r.arenaPool.Put(arena)

	doc := marshalFileInfo(file, arena)
	err := r.coll.UpsertOne(r.ctx, doc)
	if err != nil {
		return err
	}
	r.updateCh <- file
	return nil
}

func (r *anystoreFileRepository) queryOne(filter query.Filter, sorts []query.Sort) (*FileInfo, error) {
	sortsArgs := make([]any, 0, len(sorts))
	for _, sort := range sorts {
		sortsArgs = append(sortsArgs, sort)
	}
	iter, err := r.coll.Find(filter).Sort(sortsArgs...).Limit(1).Iter(r.ctx)
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	ok := iter.Next()
	if !ok {
		return nil, errNoRows
	}
	err = iter.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	doc, err := iter.Doc()
	if err != nil {
		return nil, fmt.Errorf("get doc: %w", err)
	}

	fi, err := unmarshalFileInfo(doc.Value())
	if err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &fi, nil
}

func (r *anystoreFileRepository) Close() {
	if r.ctxCancel != nil {
		r.ctxCancel()
	}
}

func marshalFileInfo(info FileInfo, arena *anyenc.Arena) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("spaceId", arena.NewString(info.SpaceId))
	obj.Set("id", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("addedAt", arena.NewNumberInt(int(info.AddedAt.UTC().Unix())))
	obj.Set("handledAt", arena.NewNumberInt(int(info.HandledAt.UTC().Unix())))
	variants := arena.NewArray()
	for i, variant := range info.Variants {
		variants.SetArrayItem(i, arena.NewString(variant.String()))
	}
	obj.Set("variants", variants)
	obj.Set("addedByUser", newBool(arena, info.AddedByUser))
	obj.Set("imported", newBool(arena, info.Imported))
	obj.Set("bytesToUpload", arena.NewNumberInt(info.BytesToUpload))
	cidsToUpload := arena.NewArray()
	var i int
	for c := range info.CidsToUpload {
		cidsToUpload.SetArrayItem(i, arena.NewString(c.String()))
	}
	obj.Set("cidsToUpload", cidsToUpload)
	return obj
}

func newBool(arena *anyenc.Arena, val bool) *anyenc.Value {
	if val {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

func unmarshalFileInfo(doc *anyenc.Value) (FileInfo, error) {
	rawVariants := doc.GetArray("variants")
	variants := make([]domain.FileId, 0, len(rawVariants))
	for _, v := range rawVariants {
		variants = append(variants, domain.FileId(v.GetString()))
	}
	cidsToUpload := map[cid.Cid]struct{}{}
	for _, raw := range doc.GetArray("cidsToUpload") {
		c, err := cid.Parse(raw.GetString())
		if err != nil {
			return FileInfo{}, fmt.Errorf("parse cid: %w", err)
		}
		cidsToUpload[c] = struct{}{}
	}
	fileId := domain.FileId(doc.GetString("fileId"))
	if !fileId.Valid() {
		return FileInfo{}, fmt.Errorf("invalid file id")
	}
	return FileInfo{
		FileId:        fileId,
		SpaceId:       doc.GetString("spaceId"),
		ObjectId:      doc.GetString("id"),
		State:         FileState(doc.GetInt("state")),
		AddedAt:       time.Unix(int64(doc.GetInt("addedAt")), 0).UTC(),
		HandledAt:     time.Unix(int64(doc.GetInt("handledAt")), 0).UTC(),
		Variants:      variants,
		AddedByUser:   doc.GetBool("addedByUser"),
		Imported:      doc.GetBool("imported"),
		BytesToUpload: doc.GetInt("bytesToUpload"),
		CidsToUpload:  cidsToUpload,
	}, nil
}

/*
queue behavior:
	- get next item, handle it OR start timer to wait for it
	- subscribe for all changes
	- wait for next item's timer OR for subscription change
*/
