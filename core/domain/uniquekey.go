package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

const uniqueKeySeparator = "-"

var smartBlockTypeToKey = map[smartblock.SmartBlockType]string{
	smartblock.SmartBlockTypeObjectType:         "ot",
	smartblock.SmartBlockTypeRelation:           "rel",
	smartblock.SmartBlockTypeRelationOption:     "opt",
	smartblock.SmartBlockTypeWorkspace:          "ws",
	smartblock.SmartBlockTypeHome:               "home",
	smartblock.SmartBlockTypeArchive:            "archive",
	smartblock.SmartBlockTypeProfilePage:        "profile",
	smartblock.SmartBlockTypeWidget:             "widget",
	smartblock.SmartBlockTypeSpaceView:          "spaceview",
	smartblock.SmartBlockTypeFileObject:         "file", // For migration purposes only
	smartblock.SmartBlockTypePage:               "page", // For migration purposes only, used for old profile data migration
	smartblock.SmartBlockTypeNotificationObject: "notification",
	smartblock.SmartBlockTypeDevicesObject:      "devices",
	smartblock.SmartBlockTypeChatDerivedObject:  "chatDerived",
	smartblock.SmartBlockTypeAccountObject:      "account",
}

// UniqueKey is unique key composed of two parts: smartblock type and internal key.
// It may not have a second component. This means that unique key represents unique object in a space, i.e. Workspace object.
type UniqueKey interface {
	SmartblockType() smartblock.SmartBlockType
	// InternalKey is underlying key, that unique within smartblock type.
	// For example: in unique key "ot-page", "page" is internal key.
	InternalKey() string
	// Marshal returns string representation of unique key. For example: "ot-page"
	Marshal() string
}

type uniqueKey struct {
	sbt smartblock.SmartBlockType
	key string
}

func NewUniqueKey(sbt smartblock.SmartBlockType, key string) (UniqueKey, error) {
	if _, exists := smartBlockTypeToKey[sbt]; !exists {
		return nil, fmt.Errorf("smartblocktype %s not supported", sbt)
	}
	return &uniqueKey{
		sbt: sbt,
		key: key,
	}, nil
}

func (uk *uniqueKey) Marshal() string {
	if uk.key == "" {
		return smartBlockTypeToKey[uk.sbt]
	}
	return smartBlockTypeToKey[uk.sbt] + uniqueKeySeparator + uk.key
}

func (uk *uniqueKey) SmartblockType() smartblock.SmartBlockType {
	return uk.sbt
}

func (uk *uniqueKey) InternalKey() string {
	return uk.key
}

func MustUniqueKey(sbt smartblock.SmartBlockType, key string) UniqueKey {
	uk, err := NewUniqueKey(sbt, key)
	if err != nil {
		panic(err)
	}
	return uk
}

func UnmarshalUniqueKey(raw string) (UniqueKey, error) {
	parts := strings.Split(raw, uniqueKeySeparator)
	if raw == "" || len(parts) > 2 {
		return nil, errors.New("uniquekey: invalid key format")
	}

	// UniqueKey can be without second component, for example, unique key for Workspace object
	var key string
	if len(parts) == 2 {
		key = parts[1]
	}
	if key == "" {
		return nil, fmt.Errorf("invalid key format: empty key")
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

func GetTypeKeyFromRawUniqueKey(raw string) (TypeKey, error) {
	uk, err := UnmarshalUniqueKey(raw)
	if err != nil {
		return "", err
	}
	if uk.SmartblockType() != smartblock.SmartBlockTypeObjectType {
		return "", fmt.Errorf("wrong type of unique key %s", uk.SmartblockType().String())
	}
	return TypeKey(uk.InternalKey()), nil
}
