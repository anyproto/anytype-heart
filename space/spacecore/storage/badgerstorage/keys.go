package badgerstorage

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
)

type aclKeys struct {
	spaceId string
	rootKey []byte
	headKey []byte
}

func newAclKeys(spaceId string) aclKeys {
	return aclKeys{
		spaceId: spaceId,
		rootKey: treestorage.JoinStringsToBytes("space", spaceId, "a", "rootId"),
		headKey: treestorage.JoinStringsToBytes("space", spaceId, "a", "headId"),
	}
}

func (a aclKeys) HeadIdKey() []byte {
	return a.headKey
}

func (a aclKeys) RootIdKey() []byte {
	return a.rootKey
}

func (a aclKeys) RawRecordKey(id string) []byte {
	return treestorage.JoinStringsToBytes("space", a.spaceId, "a", id)
}

type treeKeys struct {
	id              string
	spaceId         string
	headsKey        []byte
	rootKey         []byte
	rawChangePrefix []byte
}

func newTreeKeys(spaceId, id string) treeKeys {
	return treeKeys{
		id:              id,
		spaceId:         spaceId,
		headsKey:        treestorage.JoinStringsToBytes("space", spaceId, "t", id, "heads"),
		rootKey:         treestorage.JoinStringsToBytes("space", spaceId, "t", "rootId", id),
		rawChangePrefix: treestorage.JoinStringsToBytes("space", spaceId, "t", id),
	}
}

func (t treeKeys) HeadsKey() []byte {
	return t.headsKey
}

func (t treeKeys) RootIdKey() []byte {
	return t.rootKey
}

func (t treeKeys) RawChangeKey(id string) []byte {
	return treestorage.JoinStringsToBytes("space", t.spaceId, "t", t.id, id)
}

func (t treeKeys) RawChangePrefix() []byte {
	return t.rawChangePrefix
}

type spaceKeys struct {
	spaceId            string
	headerKey          []byte
	treePrefixKey      []byte
	treeRootPrefixKey  []byte
	spaceSettingsIdKey []byte
	spaceHash          []byte
	oldSpaceHash       []byte
	spaceDeletedKey    []byte
}

func newSpaceKeys(spaceId string) spaceKeys {
	return spaceKeys{
		spaceId:            spaceId,
		headerKey:          treestorage.JoinStringsToBytes("space", "header", spaceId),
		treeRootPrefixKey:  treestorage.JoinStringsToBytes("space", spaceId, "t", "rootId"),
		treePrefixKey:      treestorage.JoinStringsToBytes("space", spaceId, "t"),
		spaceSettingsIdKey: treestorage.JoinStringsToBytes("space", spaceId, "spaceSettingsId"),
		spaceHash:          treestorage.JoinStringsToBytes("space", spaceId, "spaceHash"),
		oldSpaceHash:       treestorage.JoinStringsToBytes("space", spaceId, "oldSpaceHash"),
		spaceDeletedKey:    treestorage.JoinStringsToBytes("space", spaceId, "spaceDeleted"),
	}
}

func (s spaceKeys) HeaderKey() []byte {
	return s.headerKey
}

func (s spaceKeys) TreeRootPrefix() []byte {
	return s.treeRootPrefixKey
}

func (s spaceKeys) TreePrefix() []byte {
	return s.treePrefixKey
}

func (s spaceKeys) SpaceSettingsId() []byte {
	return s.spaceSettingsIdKey
}

func (s spaceKeys) TreeDeletedKey(id string) []byte {
	return treestorage.JoinStringsToBytes("space", s.spaceId, "deleted", id)
}

func (s spaceKeys) SpaceDeletedKey() []byte {
	return s.spaceDeletedKey
}

func (s spaceKeys) SpaceHash() []byte {
	return s.spaceHash
}

func (s spaceKeys) OldSpaceHash() []byte {
	return s.oldSpaceHash
}

type storageServiceKeys struct {
	spacePrefix []byte
}

func newStorageServiceKeys() storageServiceKeys {
	return storageServiceKeys{
		spacePrefix: []byte("space/header"),
	}
}

func (s storageServiceKeys) SpacePrefix() []byte {
	return s.spacePrefix
}

func (s storageServiceKeys) SpaceCreatedKey(id string) []byte {
	return treestorage.JoinStringsToBytes("space/created", id)
}

func (s storageServiceKeys) BindObjectIDKey(objectID string) []byte {
	return treestorage.JoinStringsToBytes("bind", objectID)
}
