package detailservice

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func newStruct() *types.Struct {
	return &types.Struct{Fields: map[string]*types.Value{
		"tag":     pbtypes.StringList([]string{"red", "black"}),
		"author":  pbtypes.String("William Shakespeare"),
		"haters":  pbtypes.StringList([]string{}),
		"year":    pbtypes.Int64(1564),
		"numbers": pbtypes.IntList(8, 13, 21, 34),
	}}
}

func TestAddValueToListDetail(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		s        *types.Struct
		toAdd    *types.Value
		expected *types.Value
	}{
		{"string list + string list", "tag", newStruct(), pbtypes.StringList([]string{"blue", "green"}), pbtypes.StringList([]string{"red", "black", "blue", "green"})},
		{"string list + string list (intersect)", "tag", newStruct(), pbtypes.StringList([]string{"blue", "black"}), pbtypes.StringList([]string{"red", "black", "blue"})},
		{"string + string list", "author", newStruct(), pbtypes.StringList([]string{"Victor Hugo"}), pbtypes.StringList([]string{"William Shakespeare", "Victor Hugo"})},
		{"string list + string", "tag", newStruct(), pbtypes.String("orange"), pbtypes.StringList([]string{"red", "black", "orange"})},
		{"int list + int list", "numbers", newStruct(), pbtypes.IntList(55, 89), pbtypes.IntList(8, 13, 21, 34, 55, 89)},
		{"int list + int list (intersect)", "numbers", newStruct(), pbtypes.IntList(13, 8, 55), pbtypes.IntList(8, 13, 21, 34, 55)},
		{"int + int list", "year", newStruct(), pbtypes.IntList(1666, 2025), pbtypes.IntList(1564, 1666, 2025)},
		{"int list + int", "numbers", newStruct(), pbtypes.Int64(55), pbtypes.IntList(8, 13, 21, 34, 55)},
		{"string list + empty", "haters", newStruct(), pbtypes.StringList([]string{"Tomas River", "Leo Tolstoy"}), pbtypes.StringList([]string{"Tomas River", "Leo Tolstoy"})},
		{"string list + no such key", "plays", newStruct(), pbtypes.StringList([]string{"Falstaff", "Romeo and Juliet", "Macbeth"}), pbtypes.StringList([]string{"Falstaff", "Romeo and Juliet", "Macbeth"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addValueToListDetail(tc.s, tc.key, tc.toAdd)
			assert.True(t, pbtypes.Get(tc.s, tc.key).Equal(tc.expected))
		})
	}
}

func TestRemoveValueFromListDetail(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		s        *types.Struct
		toRemove *types.Value
		expected *types.Value
	}{
		{"string list - string list", "tag", newStruct(), pbtypes.StringList([]string{"red", "black"}), nil},
		{"string list - string list (some are not presented)", "tag", newStruct(), pbtypes.StringList([]string{"blue", "black"}), pbtypes.StringList([]string{"red"})},
		{"string list - string", "tag", newStruct(), pbtypes.String("red"), pbtypes.StringList([]string{"black"})},
		{"string - string list", "author", newStruct(), pbtypes.StringList([]string{"William Shakespeare"}), pbtypes.StringList([]string{})},
		{"int list - int list", "numbers", newStruct(), pbtypes.IntList(13, 34), pbtypes.IntList(8, 21)},
		{"int list - int list (some are not presented)", "numbers", newStruct(), pbtypes.IntList(2020, 5), pbtypes.IntList(8, 13, 21, 34)},
		{"int - int list", "year", newStruct(), pbtypes.IntList(1380, 1564), pbtypes.IntList()},
		{"int list - int", "numbers", newStruct(), pbtypes.Int64(21), pbtypes.IntList(8, 13, 34)},
		{"empty - string list", "haters", newStruct(), pbtypes.StringList([]string{"Tomas River", "Leo Tolstoy"}), pbtypes.StringList([]string{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removeValueFromListDetail(tc.s, tc.key, tc.toRemove)
			assert.True(t, pbtypes.Get(tc.s, tc.key).Equal(tc.expected))
		})
	}
}
