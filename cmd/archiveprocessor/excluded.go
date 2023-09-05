package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	_ "embed"
)

//go:embed excluded.json
var excludedJson []byte

const (
	objectTypeIdPattern = "ot-"
	relationIdPattern   = "rel-"
)

var (
	objectsToExcludeSlice []string
	objectsToExclude      map[string]struct{}

	shouldObjectTypesBeExcluded = false
	shouldRelationsBeExcluded   = false

	sbTypesToBeExcluded = map[model.SmartBlockType]struct{}{
		model.SmartBlockType_Workspace:   {},
		model.SmartBlockType_Widget:      {},
		model.SmartBlockType_ProfilePage: {},
		model.SmartBlockType_Template:    {},
	}
)

func init() {
	err := json.Unmarshal(excludedJson, &objectsToExcludeSlice)
	if err != nil {
		panic(fmt.Errorf("failed to unmarshal excluded.json: %v", err))
	}
	objectsToExclude = make(map[string]struct{})
	for _, id := range objectsToExcludeSlice {
		switch id {
		case objectTypeIdPattern:
			shouldObjectTypesBeExcluded = true
		case relationIdPattern:
			shouldRelationsBeExcluded = true
		default:
			objectsToExclude[id] = struct{}{}
		}
	}
}

func shouldBeExcluded(id string, sbType model.SmartBlockType) bool {
	if _, found := sbTypesToBeExcluded[sbType]; found {
		fmt.Printf("Smartblock '%s' is excluded as has type %s\n", id, sbType.String())
		return true
	}

	if shouldObjectTypesBeExcluded && strings.HasPrefix(id, objectTypeIdPattern) {
		fmt.Printf("Smartblock '%s' is excluded as it is object type\n", id)
		return true
	}

	if shouldRelationsBeExcluded && strings.HasPrefix(id, relationIdPattern) {
		fmt.Printf("Smartblock '%s' is excluded as it is relation\n", id)
		return true
	}

	if _, found := objectsToExclude[id]; found {
		fmt.Printf("Smartblock '%s' is excluded as it is listed in 'excluded.json'\n", id)
		return true
	}

	return false
}
