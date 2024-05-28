package objectstore

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestViewsMapToString(t *testing.T) {
	assert.Contains(t, []string{"block1:view1,block2:view2", "block2:view2,block1:view1"}, viewsMapToString(map[string]string{"block1": "view1", "block2": "view2"}))
	assert.Equal(t, "", viewsMapToString(nil))
	assert.Equal(t, "", viewsMapToString(map[string]string{}))
	assert.Contains(t, []string{":view,block:", "block:,:view"}, viewsMapToString(map[string]string{"": "view", "block": ""}))
}

func TestParseViewsMap(t *testing.T) {
	for _, tc := range []struct {
		name, str   string
		expectedErr error
		expectedMap map[string]string
	}{
		{"success", "block1:view1,block2:view2", nil, map[string]string{"block1": "view1", "block2": "view2"}},
		{"empty", "", nil, nil},
		{"invalid", "invalid", ErrParseView, nil},
		{"empty ids", ":view,block:", nil, map[string]string{"": "view", "block": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			views, err := parseViewsMap(tc.str)
			if tc.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedErr))
			} else {
				assert.True(t, reflect.DeepEqual(tc.expectedMap, views))
			}
		})
	}
}
