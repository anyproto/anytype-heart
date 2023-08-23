// TODO move to another package?
package uniquekey

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const separator = "-"

var smartBlockTypeToKey = map[model.SmartBlockType]string{
	model.SmartBlockType_STType:      "ot",
	model.SmartBlockType_STRelation:  "rel",
	model.SmartBlockType_Workspace:   "ws",
	model.SmartBlockType_Home:        "home",
	model.SmartBlockType_Archive:     "archive",
	model.SmartBlockType_ProfilePage: "profile",
	model.SmartBlockType_Widget:      "widget",
}

// UniqueKey is unique key composed of two parts: smartblock type and internal key.
// It may not have a second component. This means that unique key represents unique object in a space, i.e. Workspace object.
type UniqueKey interface {
	SmartblockType() model.SmartBlockType
	// InternalKey is underlying key, that unique within smartblock type.
	// For example: in unique key "ot-page", "page" is internal key.
	InternalKey() string
	// Marshal returns string representation of unique key. For example: "ot-page"
	Marshal() string
}

type uniqueKey struct {
	sbt model.SmartBlockType
	key string
}

func New(sbt model.SmartBlockType, key string) (UniqueKey, error) {
	if _, exists := smartBlockTypeToKey[sbt]; !exists {
		return nil, fmt.Errorf("smartblocktype %s not supported", sbt.String())
	}
	return &uniqueKey{
		sbt: sbt,
		key: key,
	}, nil
}

func MustUniqueKey(sbt model.SmartBlockType, key string) UniqueKey {
	uk, err := New(sbt, key)
	if err != nil {
		panic(err)
	}
	return uk
}

func UnmarshalFromString(raw string) (UniqueKey, error) {
	parts := strings.Split(raw, separator)
	if raw == "" || len(parts) > 2 {
		return nil, errors.New("invalid key format")
	}

	// UniqueKey can be without second component, for example, unique key for Workspace object
	var key string
	if len(parts) == 2 {
		key = parts[1]
	}
	for sbt, sbtString := range smartBlockTypeToKey {
		if sbtString == parts[0] {
			return &uniqueKey{
				sbt: sbt,
				key: key,
			}, nil
		}
	}
	return nil, fmt.Errorf("smartblocktype %s not supported", parts[0])
}

func GetTypeKeyFromRawUniqueKey(raw string) (bundle.TypeKey, error) {
	uk, err := UnmarshalFromString(raw)
	if err != nil {
		return "", err
	}
	if uk.SmartblockType() != model.SmartBlockType_STType {
		return "", fmt.Errorf("wrong type of unique key %s", uk.SmartblockType().String())
	}
	return bundle.TypeKey(uk.InternalKey()), nil
}

func (uk *uniqueKey) Marshal() string {
	if uk.key == "" {
		return smartBlockTypeToKey[uk.sbt]
	}
	return smartBlockTypeToKey[uk.sbt] + separator + uk.key
}

func (uk *uniqueKey) SmartblockType() model.SmartBlockType {
	return uk.sbt
}

func (uk *uniqueKey) InternalKey() string {
	return uk.key
}
