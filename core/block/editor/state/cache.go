package state

import (
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/hash"
)

const Name = "statecache"

type Cache interface {
	SaveState(heads []string, st *State, filesKeys []*pb.ChangeFileKeys) error
	GetState(objectID string, heads []string) (Doc, error)
	DeleteState(heads []string) error
	app.Component
}

type Store interface {
	SaveState(hash string, csh *pb.ChangeSnapshot) error
	GetState(hash string) (*pb.ChangeSnapshot, error)
	DeleteState(hash string) error
}

type cache struct {
	store Store
}

func (c *cache) Name() (name string) {
	return Name
}

func NewCache() Cache {
	return &cache{}
}

func (c *cache) Init(a *app.App) (err error) {
	c.store = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (c *cache) SaveState(heads []string, st *State, filesKeys []*pb.ChangeFileKeys) error {
	storedState, err := c.store.GetState(hash.HeadsHash(heads))
	if err != nil {
		log.Warnf("failed to get state in cache %s", err.Error())
	}

	if storedState != nil {
		return nil
	}
	sn := &model.SmartBlockSnapshotBase{
		Blocks:         st.Blocks(),
		Details:        st.Details(),
		ExtraRelations: st.OldExtraRelations(),
		ObjectTypes:    st.ObjectTypes(),
		RelationLinks:  st.GetRelationLinks(),
	}
	chs := &pb.ChangeSnapshot{
		Data:     sn,
		FileKeys: filesKeys,
	}
	return c.store.SaveState(hash.HeadsHash(heads), chs)
}

func (c *cache) GetState(objectID string, heads []string) (Doc, error) {
	csn, sErr := c.store.GetState(hash.HeadsHash(heads))
	if sErr == nil && csn != nil {
		doc := NewDocFromSnapshot(objectID, csn)
		return doc, nil
	}
	return nil, sErr
}

func (c *cache) DeleteState(heads []string) error {
	return c.store.DeleteState(hash.HeadsHash(heads))
}
