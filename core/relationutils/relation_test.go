package relationutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestValidateRelationFormat(t *testing.T) {
	t.Run("null value is allowed for any format", func(t *testing.T) {
		for _, format := range []model.RelationFormat{
			model.RelationFormat_longtext,
			model.RelationFormat_number,
			model.RelationFormat_date,
			model.RelationFormat_checkbox,
			model.RelationFormat_object,
		} {
			err := ValidateRelationFormat("key", domain.Null(), format, -1)
			assert.NoError(t, err, "null should be accepted for format %s", format.String())
		}
	})

	t.Run("invalid value is rejected", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Value{}, model.RelationFormat_longtext, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid value")
	})

	t.Run("longtext accepts string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("hello"), model.RelationFormat_longtext, -1)
		assert.NoError(t, err)
	})

	t.Run("longtext rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(42), model.RelationFormat_longtext, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("shorttext accepts string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("hello"), model.RelationFormat_shorttext, -1)
		assert.NoError(t, err)
	})

	t.Run("shorttext rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(42), model.RelationFormat_shorttext, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("number accepts float64", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(3.14), model.RelationFormat_number, -1)
		assert.NoError(t, err)
	})

	t.Run("number rejects string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("abc"), model.RelationFormat_number, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of number")
	})

	t.Run("date accepts float64", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(1700000000), model.RelationFormat_date, -1)
		assert.NoError(t, err)
	})

	t.Run("date rejects string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("2024-01-01"), model.RelationFormat_date, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of number")
	})

	t.Run("checkbox accepts bool", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Bool(true), model.RelationFormat_checkbox, -1)
		assert.NoError(t, err)
	})

	t.Run("checkbox rejects string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("true"), model.RelationFormat_checkbox, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of bool")
	})

	t.Run("status accepts single string list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"opt1"}), model.RelationFormat_status, -1)
		assert.NoError(t, err)
	})

	t.Run("status rejects more than one value", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"opt1", "opt2"}), model.RelationFormat_status, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should not contain more than one value")
	})

	t.Run("status rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(1), model.RelationFormat_status, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string list")
	})

	t.Run("tag accepts string list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"t1", "t2"}), model.RelationFormat_tag, -1)
		assert.NoError(t, err)
	})

	t.Run("tag respects maxCount", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"t1", "t2", "t3"}), model.RelationFormat_tag, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maxCount exceeded")
	})

	t.Run("tag allows within maxCount", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"t1", "t2"}), model.RelationFormat_tag, 3)
		assert.NoError(t, err)
	})

	t.Run("object accepts string list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"obj1", "obj2"}), model.RelationFormat_object, -1)
		assert.NoError(t, err)
	})

	t.Run("object rejects empty string in list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"obj1", ""}), model.RelationFormat_object, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty option")
	})

	t.Run("object respects maxCount", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"o1", "o2", "o3"}), model.RelationFormat_object, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maxCount exceeded")
	})

	t.Run("file accepts string list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{"file1"}), model.RelationFormat_file, -1)
		assert.NoError(t, err)
	})

	t.Run("file rejects empty string in list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.StringList([]string{""}), model.RelationFormat_file, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty option")
	})

	t.Run("url accepts valid URL string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("https://example.com"), model.RelationFormat_url, -1)
		assert.NoError(t, err)
	})

	t.Run("url accepts empty string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String(""), model.RelationFormat_url, -1)
		assert.NoError(t, err)
	})

	t.Run("url rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(42), model.RelationFormat_url, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("email accepts string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("test@example.com"), model.RelationFormat_email, -1)
		assert.NoError(t, err)
	})

	t.Run("email rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(42), model.RelationFormat_email, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("phone accepts string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("+1234567890"), model.RelationFormat_phone, -1)
		assert.NoError(t, err)
	})

	t.Run("phone rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(123), model.RelationFormat_phone, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("emoji accepts string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("\U0001f389"), model.RelationFormat_emoji, -1)
		assert.NoError(t, err)
	})

	t.Run("emoji rejects number", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.Float64(42), model.RelationFormat_emoji, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of string")
	})

	t.Run("map accepts map value", func(t *testing.T) {
		mapVal := domain.NewValueMap(map[string]domain.Value{"k": domain.String("v")})
		err := ValidateRelationFormat("key", mapVal, model.RelationFormat_map, -1)
		assert.NoError(t, err)
	})

	t.Run("map rejects string", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("notmap"), model.RelationFormat_map, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instead of map")
	})

	t.Run("object accepts single string wrapped to list", func(t *testing.T) {
		// A single string value should be wrappable to string list
		err := ValidateRelationFormat("key", domain.String("obj1"), model.RelationFormat_object, -1)
		assert.NoError(t, err)
	})

	t.Run("status accepts single string wrapped to list", func(t *testing.T) {
		err := ValidateRelationFormat("key", domain.String("opt1"), model.RelationFormat_status, -1)
		assert.NoError(t, err)
	})
}
