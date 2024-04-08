package domain

import (
	"fmt"
	"strings"
)

const (
	// ObjectPathSeparator is the separator between object id and block id or relation key
	objectPathSeparator = "/"
	blockPrefix         = "b"
	relationPrefix      = "r"
)

type ObjectPath struct {
	ObjectId    string
	BlockId     string
	RelationKey string
}

// String returns the full path, e.g. "objectId-b-blockId" or "objectId-r-relationKey"
func (o ObjectPath) String() string {
	if o.HasBlock() {
		return strings.Join([]string{o.ObjectId, blockPrefix, o.BlockId}, objectPathSeparator)
	}
	if o.HasRelation() {
		return strings.Join([]string{o.ObjectId, relationPrefix, o.RelationKey}, objectPathSeparator)
	}
	return o.ObjectId
}

// ObjectRelativePath returns the relative path of the object without the object id prefix
func (o ObjectPath) ObjectRelativePath() string {
	if o.HasBlock() {
		return strings.Join([]string{blockPrefix, o.BlockId}, objectPathSeparator)
	}
	if o.HasRelation() {
		return strings.Join([]string{relationPrefix, o.RelationKey}, objectPathSeparator)
	}
	return ""
}

func (o ObjectPath) IsEmpty() bool {
	return o.ObjectId == ""
}

func (o ObjectPath) HasRelation() bool {
	return o.RelationKey != ""
}

func (o ObjectPath) HasBlock() bool {
	return o.BlockId != ""
}

func NewObjectPathWithBlock(objectId, blockId string) ObjectPath {
	return ObjectPath{
		ObjectId: objectId,
		BlockId:  blockId,
	}
}

func NewObjectPathWithRelation(objectId, relationKey string) ObjectPath {
	return ObjectPath{
		ObjectId:    objectId,
		RelationKey: relationKey,
	}
}

func NewFromPath(path string) (ObjectPath, error) {
	parts := strings.Split(path, objectPathSeparator)
	if len(parts) == 3 && parts[1] == blockPrefix {
		return NewObjectPathWithBlock(parts[0], parts[2]), nil
	}
	if len(parts) == 3 && parts[1] == relationPrefix {
		return NewObjectPathWithRelation(parts[0], parts[2]), nil
	}
	return ObjectPath{ObjectId: path}, fmt.Errorf("fts invalid path: %s", path)
}
