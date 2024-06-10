package linkresolver

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateLink(t *testing.T) {
	for _, tc := range []struct {
		name, resource string
		pars           map[string]string
		link           string
		shouldError    bool
	}{
		{"generate object link", ResourceObject, map[string]string{ParameterSpaceId: "space1", ParameterObjectId: "obj1"},
			"object/spaceId=space1&objectId=obj1", false},
		{"generate block link", ResourceBlock, map[string]string{ParameterSpaceId: "space1", ParameterObjectId: "obj1", ParameterBlockId: "dataview"},
			"block/spaceId=space1&objectId=obj1&blockId=dataview", false},
		{"invalid resource", "invalid", map[string]string{ParameterSpaceId: "space1", ParameterObjectId: "obj1"}, "", true},
		{"parameter is missing", ResourceObject, map[string]string{ParameterObjectId: "obj1"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, err := generateLink(tc.resource, tc.pars)
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.link, link)
		})
	}
}

func TestParseLink(t *testing.T) {
	for _, tc := range []struct {
		name, link, resource string
		params               map[string]string
		shouldError          bool
	}{
		{"parse object link", "object/spaceId=space1&objectId=obj1", ResourceObject,
			map[string]string{ParameterSpaceId: "space1", ParameterObjectId: "obj1"}, false},
		{"parse block link", "block/spaceId=space1&objectId=obj1&blockId=title", ResourceBlock,
			map[string]string{ParameterSpaceId: "space1", ParameterObjectId: "obj1", ParameterBlockId: "title"}, false},
		{"invalid string", "absolutely invalid string", "", nil, true},
		{"invalid resource", "image/size=100&format=png", "image", nil, true},
		{"wrong parameters order", "object/objectId=o&spaceId=s", ResourceObject, nil, true},
		{"redundant parameter", "object/spaceId=s&objectId=o&creator=k", ResourceObject, nil, true},
		{"wrong parameter format", "block/spaceId=s&objectId=o&blockId==description", ResourceBlock, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource, params, err := parseLink(tc.link)
			if tc.shouldError {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrLinkParsing))
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.resource, resource)
			assert.Equal(t, tc.params, params)
		})
	}
}
