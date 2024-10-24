package detailservice

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
)

func newStruct() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"tag":     domain.StringList([]string{"red", "black"}),
		"author":  domain.String("William Shakespeare"),
		"haters":  domain.StringList([]string{}),
		"year":    domain.Int64(1564),
		"numbers": domain.Int64List(8, 13, 21, 34),
	})
}

func TestAddValueToListDetail(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      domain.RelationKey
		s        *domain.Details
		toAdd    domain.Value
		expected domain.Value
	}{
		{"string list + string list", "tag", newStruct(), domain.StringList([]string{"blue", "green"}), domain.StringList([]string{"red", "black", "blue", "green"})},
		{"string list + string list (intersect)", "tag", newStruct(), domain.StringList([]string{"blue", "black"}), domain.StringList([]string{"red", "black", "blue"})},
		{"string + string list", "author", newStruct(), domain.StringList([]string{"Victor Hugo"}), domain.StringList([]string{"William Shakespeare", "Victor Hugo"})},
		{"string list + string", "tag", newStruct(), domain.String("orange"), domain.StringList([]string{"red", "black", "orange"})},
		{"int list + int list", "numbers", newStruct(), domain.Int64List(55, 89), domain.Int64List(8, 13, 21, 34, 55, 89)},
		{"int list + int list (intersect)", "numbers", newStruct(), domain.Int64List(13, 8, 55), domain.Int64List(8, 13, 21, 34, 55)},
		{"int + int list", "year", newStruct(), domain.Int64List(1666, 2025), domain.Int64List(1564, 1666, 2025)},
		{"int list + int", "numbers", newStruct(), domain.Int64(55), domain.Int64List(8, 13, 21, 34, 55)},
		{"string list + empty", "haters", newStruct(), domain.StringList([]string{"Tomas River", "Leo Tolstoy"}), domain.StringList([]string{"Tomas River", "Leo Tolstoy"})},
		{"string list + no such key", "plays", newStruct(), domain.StringList([]string{"Falstaff", "Romeo and Juliet", "Macbeth"}), domain.StringList([]string{"Falstaff", "Romeo and Juliet", "Macbeth"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addValueToListDetail(tc.s, tc.key, tc.toAdd)
			assert.True(t, tc.s.Get(tc.key).Equal(tc.expected))
		})
	}
}

func TestRemoveValueFromListDetail(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      domain.RelationKey
		s        *domain.Details
		toRemove domain.Value
		expected domain.Value
	}{
		{"string list - string list", "tag", newStruct(), domain.StringList([]string{"red", "black"}), domain.Invalid()},
		{"string list - string list (some are not presented)", "tag", newStruct(), domain.StringList([]string{"blue", "black"}), domain.StringList([]string{"red"})},
		{"string list - string", "tag", newStruct(), domain.String("red"), domain.StringList([]string{"black"})},
		{"string - string list", "author", newStruct(), domain.StringList([]string{"William Shakespeare"}), domain.StringList([]string{})},
		{"int list - int list", "numbers", newStruct(), domain.Int64List(13, 34), domain.Int64List(8, 21)},
		{"int list - int list (some are not presented)", "numbers", newStruct(), domain.Int64List(2020, 5), domain.Int64List(8, 13, 21, 34)},
		{"int - int list", "year", newStruct(), domain.Int64List(1380, 1564), domain.Invalid()},
		{"int list - int", "numbers", newStruct(), domain.Int64(21), domain.Int64List(8, 13, 34)},
		{"empty - string list", "haters", newStruct(), domain.StringList([]string{"Tomas River", "Leo Tolstoy"}), domain.StringList([]string{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removeValueFromListDetail(tc.s, tc.key, tc.toRemove)
		})
	}
}
