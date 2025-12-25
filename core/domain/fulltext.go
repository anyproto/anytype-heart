package domain

import (
	"fmt"
	"strings"
)

const (
	// ObjectPathSeparator is the separator between object id and block id or relation key
	ObjectPathSeparator = "/"
	blockPrefix         = "b"
	relationPrefix      = "r"
	messagePrefix       = "m"
)

type ObjectPath struct {
	ObjectId    string
	BlockId     string
	RelationKey string
	MessageId   string
}

// String returns the full path, e.g. "objectId-b-blockId" or "objectId-r-relationKey"
func (o ObjectPath) String() string {
	if o.HasBlock() {
		return strings.Join([]string{o.ObjectId, blockPrefix, o.BlockId}, ObjectPathSeparator)
	}
	if o.HasRelation() {
		return strings.Join([]string{o.ObjectId, relationPrefix, o.RelationKey}, ObjectPathSeparator)
	}
	if o.HasMessage() {
		return strings.Join([]string{o.ObjectId, messagePrefix, o.MessageId}, ObjectPathSeparator)
	}
	return o.ObjectId
}

// ObjectRelativePath returns the relative path of the object without the object id prefix
func (o ObjectPath) ObjectRelativePath() string {
	if o.HasBlock() {
		return strings.Join([]string{blockPrefix, o.BlockId}, ObjectPathSeparator)
	}
	if o.HasRelation() {
		return strings.Join([]string{relationPrefix, o.RelationKey}, ObjectPathSeparator)
	}
	if o.HasMessage() {
		return strings.Join([]string{messagePrefix, o.MessageId}, ObjectPathSeparator)
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

func (o ObjectPath) HasMessage() bool {
	return o.MessageId != ""
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

func NewObjectPathWithMessage(objectId, messageId string) ObjectPath {
	return ObjectPath{
		ObjectId:  objectId,
		MessageId: messageId,
	}
}

func NewFromPath(path string) (ObjectPath, error) {
	parts := strings.Split(path, ObjectPathSeparator)
	if len(parts) == 3 && parts[1] == blockPrefix {
		return NewObjectPathWithBlock(parts[0], parts[2]), nil
	}
	if len(parts) == 3 && parts[1] == relationPrefix {
		return NewObjectPathWithRelation(parts[0], parts[2]), nil
	}
	if len(parts) == 3 && parts[1] == messagePrefix {
		return NewObjectPathWithMessage(parts[0], parts[2]), nil
	}
	return ObjectPath{ObjectId: path}, fmt.Errorf("fts invalid path: %s", path)
}
