package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	testTemplateTypeId = "type-template"
	testMemoTypeId     = "type-memo"
)

// blockCountPtr is the pointer form a known count takes on the wire.
func blockCountPtr(n int) *int { return &n }

// addTemplateType registers the space's template type, without which no row
// can BE a template (its `type` names this object).
func (fx *v2Fixture) addTemplateType(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(testTemplateTypeId),
		bundle.RelationKeyName:           domain.String("Template"),
		bundle.RelationKeyUniqueKey:      domain.String(bundle.TypeKeyTemplate.URL()),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}})
}

// addMemoType registers a custom type `memo`, optionally carrying a
// default_template. The detail is written as a LIST, which is the spelling
// this API's own type writes take.
func (fx *v2Fixture) addMemoType(t *testing.T, defaultTemplate ...string) {
	memo := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(testMemoTypeId),
		bundle.RelationKeyName:           domain.String("Memo"),
		bundle.RelationKeyUniqueKey:      domain.String("ot-memo"),
		bundle.RelationKeyApiObjectKey:   domain.String("memo"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	if len(defaultTemplate) > 0 {
		memo[bundle.RelationKeyDefaultTemplateId] = domain.StringList(defaultTemplate)
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{memo})
}

// addTemplate registers one template object of targetTypeId.
func (fx *v2Fixture) addTemplate(t *testing.T, id, name, targetTypeId string, extra ...objectstore.TestObject) {
	row := objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyName:             domain.String(name),
		bundle.RelationKeyType:             domain.String(testTemplateTypeId),
		bundle.RelationKeyTargetObjectType: domain.String(targetTypeId),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
	}
	for _, more := range extra {
		for key, value := range more {
			row[key] = value
		}
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{row})
}

func TestCreateObjectTemplate(t *testing.T) {
	t.Run("a template the body names is applied and named back", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")
		want := &v2model.AppliedTemplate{Id: "tpl-weekly", Name: "Weekly memo", Source: "request", BlocksAdded: blockCountPtr(0)}

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-weekly"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Template)
		assert.Equal(t, "tpl-weekly", *applied, "the create itself must start from the template the result names")
		assert.Empty(t, result.Warnings)
	})

	t.Run("the type's default template applies when the body names none", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")
		want := &v2model.AppliedTemplate{Id: "tpl-weekly", Name: "Weekly memo", Source: "type_default", BlocksAdded: blockCountPtr(0)}

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Template, "the caller did not choose it, so the result has to say who did")
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("a default stored as a bare string applies too", func(t *testing.T) {
		// given — the spelling the clients write; this API writes a list
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(testMemoTypeId),
			bundle.RelationKeyName:              domain.String("Memo"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-memo"),
			bundle.RelationKeyApiObjectKey:      domain.String("memo"),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyDefaultTemplateId: domain.String("tpl-weekly"),
		}})
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("none starts from nothing even when the type has a default", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"none"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
	})

	t.Run("an empty template member reads as absent, not as none", func(t *testing.T) {
		// given — a body generated against the schema carries every member it
		// can see; an empty one must not switch the type's default off
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":""}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "type_default", result.Template.Source)
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("a type with no default template applies none", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a stale default warns, and the object is still created", func(t *testing.T) {
		// given — the default outlives its template: deleting one clears the
		// type only when the detail was stored as a bare string
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-gone")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id, "a type nobody repaired must not block every create of that type")
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-gone")
		assert.Contains(t, result.Warnings[0].Message, "default template")
		assert.Contains(t, result.Warnings[0].Hint, "default_template")
	})

	t.Run("a default in the bin warns like a missing one, whichever way it went", func(t *testing.T) {
		// the three flags a template can end its life under. The API's word
		// for all of them is deleted: its own DELETE archives, and an
		// archived object is what a caller of this surface cannot address
		for _, gone := range []domain.RelationKey{
			bundle.RelationKeyIsArchived,
			bundle.RelationKeyIsDeleted,
			bundle.RelationKeyIsUninstalled,
		} {
			t.Run(gone.String(), func(t *testing.T) {
				// given
				fx := newV2Fixture(t)
				fx.addTemplateType(t)
				fx.addMemoType(t, "tpl-weekly")
				fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId, objectstore.TestObject{
					gone: domain.Bool(true),
				})
				applied := fx.expectCreateWithTemplate("newObj")
				fx.expectEtagRead("newObj")

				// when
				result, err := fx.CreateObject(context.Background(), testSpaceId,
					[]byte(`{"type":"memo","name":"Monday"}`), false, false)

				// then
				require.NoError(t, err)
				assert.Equal(t, "newObj", result.Id, "the object is created either way")
				assert.Nil(t, result.Template)
				assert.Empty(t, *applied, "and it starts from nothing")
				require.Len(t, result.Warnings, 1)
				assert.Contains(t, result.Warnings[0].Message, "is deleted")
				assert.Contains(t, result.Warnings[0].Hint, "default_template")
			})
		}
	})

	t.Run("a template in the bin that the body names is refused, not warned about", func(t *testing.T) {
		// given — the same state, the other chooser
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsArchived: domain.Bool(true),
		})

		// when — no create expectation: nothing may be written
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-weekly"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "is deleted")
	})

	t.Run("a template the body names and the space does not hold is refused", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when — no create expectation: nothing may be written
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-nope"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "tpl-nope")
		assert.Contains(t, apiErr.Issues[0].Hint, "none")
	})

	t.Run("a template of another type is refused, and the refusal names that type", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-task"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "task")
	})

	t.Run("an object that is not a template is refused", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("obj-plain"),
			bundle.RelationKeyName:           domain.String("Just a page"),
			bundle.RelationKeyType:           domain.String(testMemoTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"obj-plain"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "not a template")
	})

	t.Run("a non-string template member is refused with the member named", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","template":{"id":"tpl-weekly"}}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("a dry run names the template it would apply and creates nothing", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when — no create expectation: a dry run writes nothing
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), true, false)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.NotNil(t, result.Template, "a dry run that does not name the template says nothing about the outcome")
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		assert.Equal(t, "type_default", result.Template.Source)
	})

	t.Run("the full document form takes the same member", func(t *testing.T) {
		// given — the member is lifted before the format validation, which
		// would otherwise refuse it as an unknown envelope member
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","template":"tpl-weekly","properties":{"name":"Monday"},"blocks":[{"type":"paragraph","text":"body"}]}`),
			false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		for _, b := range snapshot.Blocks {
			assert.NotContains(t, b.String(), "tpl-weekly", "the directive must not reach the document")
		}
	})

	t.Run("a template document cannot itself start from a template", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","kind":"template","type":"template","template_for":"memo","template":"tpl-weekly","properties":{"name":"New"}}`),
			false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("POST templates refuses the member as an unknown document member", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateTemplate(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"memo","template":"tpl-weekly","properties":{"name":"New"}}`),
			false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})
}

func TestListTemplates(t *testing.T) {
	t.Run("templates list with their type and the default marked", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(200),
		})
		fx.addTemplate(t, "tpl-daily", "Daily memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
		})
		want := []v2model.TemplateRow{
			{Id: "tpl-weekly", Name: "Weekly memo", TemplateFor: "memo", Default: true},
			{Id: "tpl-daily", Name: "Daily memo", TemplateFor: "memo"},
		}

		// when
		rows, total, hasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("the type filter narrows to one type's templates", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "memo", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "tpl-weekly", rows[0].Id)
		assert.Equal(t, 1, total)
	})

	t.Run("an object that is not a template does not list", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("obj-plain"),
			bundle.RelationKeyName:           domain.String("Just a page"),
			bundle.RelationKeyType:           domain.String(testMemoTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Equal(t, 0, total)
	})

	t.Run("an unknown type key is refused with the parameter named", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, "nosuchtype", 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "type", apiErr.Issues[0].Path)
	})
}

func TestTemplateSchemaMembers(t *testing.T) {
	t.Run("the shortcut kind declares template", func(t *testing.T) {
		// given
		var schema map[string]any

		// when
		require.NoError(t, json.Unmarshal([]byte(v2SchemaKinds["shortcut"].schema), &schema))

		// then
		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "template")
	})

	t.Run("the object kind's narrowed document schema declares template, and the template kind does not", func(t *testing.T) {
		// when
		object := apiV2KindSchema("object")
		template := apiV2KindSchema("template")

		// then
		var objectRoot, templateRoot map[string]any
		require.NoError(t, json.Unmarshal(object, &objectRoot))
		require.NoError(t, json.Unmarshal(template, &templateRoot))
		assert.Contains(t, objectRoot["properties"].(map[string]any), "template")
		assert.NotContains(t, templateRoot["properties"].(map[string]any), "template",
			"a template does not start from a template")
	})
}

func TestCreateObjectTemplateHardening(t *testing.T) {
	t.Run("a refused template mints nothing on the way", func(t *testing.T) {
		// given — an option that would be created, and a template that will
		// not resolve: the refusal has to come first, or the caller's space
		// keeps an option for an object that was never created
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addSelectProperty(t)

		// when — no ObjectCreateRelationOption expectation: minting one fails the test
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-nope","properties":{"severity":"Brand new"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("a template of another type is refused for a type this space has not installed", func(t *testing.T) {
		// given — `task` is bundled, so it resolves as a type key while
		// holding no store row; its id is still derivable, and it is what any
		// template of that type carries as its target
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-memo", "Weekly memo", testMemoTypeId)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","name":"Monday","template":"tpl-memo"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "memo")
	})

	t.Run("a template of the derived type applies for a type this space has not installed", func(t *testing.T) {
		// given — the fixture derives type ids as drv-ot-<key>
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addTemplate(t, "tpl-task", "Weekly task", "drv-ot-task")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","name":"Monday","template":"tpl-task"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-task", *applied)
	})

	t.Run("a default that points at another type's template warns and is not applied", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-task")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-task")
	})

	t.Run("a create with no type takes the page type's default", func(t *testing.T) {
		// given — an absent type defaults to page, and the default template
		// must follow the type the object is actually created as
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String("type-page"),
			bundle.RelationKeyName:              domain.String("Page"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-page"),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyDefaultTemplateId: domain.StringList([]string{"tpl-page"}),
		}})
		fx.addTemplate(t, "tpl-page", "Blank-ish page", "type-page")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","properties":{"name":"Monday"}}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-page", result.Template.Id)
		assert.Equal(t, "tpl-page", *applied)
	})

	t.Run("the full document form passes the template to the create, not only to the result", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","template":"tpl-weekly","properties":{"name":"Monday"},"blocks":[{"type":"paragraph","text":"body"}]}`),
			false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "tpl-weekly", *applied)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
	})

	t.Run("a template that vanishes mid-request refuses when the caller named it", func(t *testing.T) {
		// given — it passed the index check, then failed to load
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "tpl-weekly").
			Return(apicore.CreateOutcome{}, apicore.ErrTemplateUnavailable).Once()

		// when — the Once above is the assertion that a refusal does not fall
		// back to creating the object anyway
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-weekly"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "deleted")
	})

	t.Run("a default that vanishes mid-request warns, and the object is created without it", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "tpl-weekly").
			Return(apicore.CreateOutcome{}, apicore.ErrTemplateUnavailable).Once()
		// exactly one retry, and it must carry NO template
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "").
			Return(apicore.CreateOutcome{Id: "newObj"}, nil).Once()
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id)
		assert.Nil(t, result.Template, "the result must not name a template the object did not get")
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-weekly")
	})

	t.Run("a stored type key beside the api key is refused, not silently preferred", func(t *testing.T) {
		// given — the format lets a document carry type_internal_key, and on
		// import it wins over `type`: every check this endpoint makes, the
		// template included, reads `type`
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","type_internal_key":"task","properties":{"name":"X"}}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type_internal_key", apiErr.Issues[0].Path)
	})
}

func TestListTemplatesHardening(t *testing.T) {
	t.Run("an uninstalled or hidden template does not list", func(t *testing.T) {
		// given — the create path refuses both, so offering them here would
		// hand the caller ids that cannot be used
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-live", "Weekly memo", testMemoTypeId)
		fx.addTemplate(t, "tpl-uninstalled", "Removed", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		fx.addTemplate(t, "tpl-hidden", "Hidden", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsHidden: domain.Bool(true),
		})

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "tpl-live", rows[0].Id)
		assert.Equal(t, 1, total)
	})

	t.Run("pages carry has_more and a stable total", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-a", "A", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(300),
		})
		fx.addTemplate(t, "tpl-b", "B", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(200),
		})
		fx.addTemplate(t, "tpl-c", "C", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
		})

		// when
		first, total, hasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 2)
		require.NoError(t, err)
		second, secondTotal, secondHasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 2, 2)
		require.NoError(t, err)
		beyond, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 3, 2)
		require.NoError(t, err)

		// then
		assert.Equal(t, []string{"tpl-a", "tpl-b"}, []string{first[0].Id, first[1].Id})
		assert.True(t, hasMore)
		assert.Equal(t, 3, total)
		require.Len(t, second, 1)
		assert.Equal(t, "tpl-c", second[0].Id)
		assert.False(t, secondHasMore)
		assert.Equal(t, 3, secondTotal)
		assert.Empty(t, beyond)
	})

	t.Run("the type filter takes every spelling the type routes take", func(t *testing.T) {
		// given — a stored key that is not the served spelling
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-custom"),
			bundle.RelationKeyName:           domain.String("Field note"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-6a941a2861fab2a6d6059813"),
			bundle.RelationKeyApiObjectKey:   domain.String("field_note"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-note", "Weekly note", "type-custom")

		for _, term := range []string{"field_note", "6a941a2861fab2a6d6059813", "Field note"} {
			// when
			rows, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, term, 0, 25)

			// then
			require.NoError(t, err, term)
			require.Len(t, rows, 1, term)
			assert.Equal(t, "tpl-note", rows[0].Id, term)
			assert.Equal(t, "field_note", rows[0].TemplateFor, term, "the row spells the type the way every other response does")
		}
	})
}

func TestCreateObjectTemplateReportsComposition(t *testing.T) {
	t.Run("a request that sent content is told its body holds the template's too", func(t *testing.T) {
		// given — the object's blocks are the template's followed by these,
		// which is the one thing a caller cannot infer from its own request
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj", 4)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes\n\nfirst"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "tpl-weekly", *applied, "the composition describes a template the create actually started from")
		require.NotNil(t, result.Template)
		assert.True(t, result.Template.Combined)
		require.NotNil(t, result.Template.BlocksAdded)
		assert.Equal(t, 4, *result.Template.BlocksAdded)
	})

	t.Run("a request that sent no content is not told its body was combined", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj", 4)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "tpl-weekly", *applied)
		require.NotNil(t, result.Template)
		assert.False(t, result.Template.Combined, "nothing of the caller's went after the template's blocks")
		require.NotNil(t, result.Template.BlocksAdded)
		assert.Equal(t, 4, *result.Template.BlocksAdded)
	})

	t.Run("a dry run says the body would be combined and counts nothing", func(t *testing.T) {
		// given — no template is built on a dry run, so the count is unknown;
		// what the request itself carries is not
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes"}`), true, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.True(t, result.Template.Combined)
		assert.Nil(t, result.Template.BlocksAdded, "a dry run does not build the template, so the count is not known")
	})

	t.Run("an object created without a template carries no composition at all", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes"}`), false, false)

		require.NoError(t, err)
		assert.Nil(t, result.Template)
	})
}

func TestCreateObjectTemplateZeroContribution(t *testing.T) {
	t.Run("a template that adds nothing reports zero, not nothing", func(t *testing.T) {
		// given — a template can carry only its header; absent has to keep
		// meaning "not known", which is what a dry run leaves behind
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-empty")
		fx.addTemplate(t, "tpl-empty", "Empty", testMemoTypeId)
		fx.expectCreateWithTemplate("newObj", 0)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		require.NotNil(t, result.Template.BlocksAdded, "zero is an answer this create knows")
		assert.Equal(t, 0, *result.Template.BlocksAdded)

		// and the wire shape carries it
		encoded, err := json.Marshal(result.Template)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"blocks_added":0`)
	})

	t.Run("a dry run leaves the count out of the wire shape entirely", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-empty")
		fx.addTemplate(t, "tpl-empty", "Empty", testMemoTypeId)

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), true, false)

		require.NoError(t, err)
		require.NotNil(t, result.Template)
		encoded, err := json.Marshal(result.Template)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "blocks_added")
	})
}

func TestCreateObjectMarkdownTitle(t *testing.T) {
	// the object renders its name as its title, so a body that opens by
	// restating it shows the same words twice — the shape a small model
	// reaches for by default
	t.Run("a leading heading that repeats the name is dropped", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Tegeler Forst","markdown":"# Tegeler Forst\n\n## Overview\n\nA forest."}`), false, false)

		// then
		require.NoError(t, err)
		snapshot := *captured
		require.NotNil(t, snapshot)
		assert.Equal(t, []string{"Overview", "A forest."}, snapshotTexts(snapshot))
		assert.Equal(t, "Tegeler Forst", pbtypes.GetString(snapshot.Details, "name"))
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/markdown[0]", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, "repeated")
	})

	t.Run("a leading heading becomes the name when the request set none", func(t *testing.T) {
		// given — what the markdown importer does with every file
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","markdown":"# Tegeler Forst\n\nA forest."}`), false, false)

		// then
		require.NoError(t, err)
		snapshot := *captured
		assert.Equal(t, "Tegeler Forst", pbtypes.GetString(snapshot.Details, "name"))
		assert.Equal(t, []string{"A forest."}, snapshotTexts(snapshot))
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "became the object's name")
	})

	t.Run("a name in properties counts as the name", func(t *testing.T) {
		// given — the shortcut takes the name either way
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"name":"Tegeler Forst"},"markdown":"# Tegeler Forst\n\nA forest."}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"A forest."}, snapshotTexts(*captured))
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "repeated", "dropped as a duplicate, not promoted")
	})

	t.Run("a heading that is not the name is body content", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Tegeler Forst","markdown":"# Overview\n\nA forest."}`), false, false)

		require.NoError(t, err)
		assert.Equal(t, []string{"Overview", "A forest."}, snapshotTexts(*captured))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a subheading may repeat the name but may not become it", func(t *testing.T) {
		// given — a model restating the name does not always pick h1, but
		// promoting a section heading would invent a title out of a section
		fx := newV2Fixture(t)
		dropped := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Tegeler Forst","markdown":"## Tegeler Forst\n\nA forest."}`), false, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"A forest."}, snapshotTexts(*dropped))

		// when — the same heading with no name to match
		fx2 := newV2Fixture(t)
		kept := fx2.expectCreate("newObj2")
		fx2.expectEtagRead("newObj2")
		_, err = fx2.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","markdown":"## Tegeler Forst\n\nA forest."}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Tegeler Forst", "A forest."}, snapshotTexts(*kept))
		assert.Empty(t, pbtypes.GetString((*kept).Details, "name"))
	})

	t.Run("markdown that is only the title leaves a named object with no body", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Tegeler Forst","markdown":"# Tegeler Forst"}`), false, false)

		require.NoError(t, err)
		assert.Empty(t, snapshotTexts(*captured))
		assert.Equal(t, "Tegeler Forst", pbtypes.GetString((*captured).Details, "name"))
	})

	t.Run("a full document is left alone", func(t *testing.T) {
		// given — an authored block tree is a deliberate choice, and the
		// markdown convention does not reach it
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Tegeler Forst"},"blocks":[{"type":"heading_1","text":"Tegeler Forst"},{"type":"paragraph","text":"A forest."}]}`),
			false, false)

		require.NoError(t, err)
		assert.Equal(t, []string{"Tegeler Forst", "A forest."}, snapshotTexts(*captured))
		assert.Empty(t, result.Warnings)
	})
}

// snapshotTexts is the text of a snapshot's blocks in document order, root aside.
func snapshotTexts(snapshot *model.SmartBlockSnapshotBase) []string {
	var texts []string
	for _, block := range snapshot.Blocks {
		if block.GetSmartblock() != nil {
			continue
		}
		texts = append(texts, block.GetText().GetText())
	}
	return texts
}

func TestCreateObjectMarkdownTitleEdges(t *testing.T) {
	// the cases a third review round found: the heading's text is markdown,
	// a heading can own the blocks under it, a name the caller sent is never
	// replaced, and a note has no title to duplicate
	t.Run("emphasis in the heading matches the name and never reaches it", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Tegeler Forst","markdown":"# **Tegeler Forst**\n\nA forest."}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"A forest."}, snapshotTexts(*captured))
	})

	t.Run("a promoted name is the rendered text, not the markdown", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","markdown":"# **Tegeler** Forst\n\nA forest."}`), false, false)

		require.NoError(t, err)
		assert.Equal(t, "Tegeler Forst", pbtypes.GetString((*captured).Details, "name"))
	})

	t.Run("a heading with nested content under it is left whole", func(t *testing.T) {
		// given — removing it would leave its children indented under nothing,
		// and the document would be refused: a create that worked before
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Title","markdown":"# Title\n\n  child"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Title", "child"}, snapshotTexts(*captured))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a name the caller sent is never replaced by a heading", func(t *testing.T) {
		// given — a value this layer cannot read is still a value the caller
		// chose
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"name":123},"markdown":"# Section\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Section", "body"}, snapshotTexts(*captured), "the heading stayed body content")
	})

	t.Run("a display-name spelling of the name property counts as supplied", func(t *testing.T) {
		// given — properties take display names too, and promoting would add a
		// second spelling of the same property
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"Name":"Chosen"},"markdown":"# Section\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "Chosen", pbtypes.GetString((*captured).Details, "name"))
		assert.Equal(t, []string{"Section", "body"}, snapshotTexts(*captured))
	})

	t.Run("a note has no title, so its heading is not promoted", func(t *testing.T) {
		// given — a note turns its name back into the first block of its body,
		// so a promoted heading would lose its style, move below any template
		// content and leave the object unnamed while the response claimed a name
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String("type-note"),
			bundle.RelationKeyName:              domain.String("Jotting"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-jotting"),
			bundle.RelationKeyApiObjectKey:      domain.String("jotting"),
			bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
		}})
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"jotting","markdown":"# Monday\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Monday", "body"}, snapshotTexts(*captured))
		assert.Empty(t, pbtypes.GetString((*captured).Details, "name"))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a note still drops a heading that repeats its name", func(t *testing.T) {
		// given — the name becomes the first block there, so the heading would
		// be the duplicate this rule exists to remove
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String("type-note"),
			bundle.RelationKeyName:              domain.String("Jotting"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-jotting"),
			bundle.RelationKeyApiObjectKey:      domain.String("jotting"),
			bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
		}})
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"jotting","name":"Monday","markdown":"# Monday\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"body"}, snapshotTexts(*captured))
	})
}

func TestCreateObjectMarkdownTitleContracts(t *testing.T) {
	t.Run("a dry run reports the transformation it would make", func(t *testing.T) {
		// given — a dry run that silently changed the body would be a worse
		// answer than no dry run at all
		fx := newV2Fixture(t)

		// when — no create expectation
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Title","markdown":"# Title\n\nbody"}`), true, false)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/markdown[0]", result.Warnings[0].Path)
	})

	t.Run("every heading level can be the duplicate", func(t *testing.T) {
		for markdown, kept := range map[string][]string{
			"# Title\n\nbody":   {"body"},
			"## Title\n\nbody":  {"body"},
			"### Title\n\nbody": {"body"},
		} {
			fx := newV2Fixture(t)
			captured := fx.expectCreate("newObj")
			fx.expectEtagRead("newObj")

			_, err := fx.CreateObject(context.Background(), testSpaceId,
				[]byte(`{"type":"page","name":"Title","markdown":`+strconv.Quote(markdown)+`}`), false, false)

			require.NoError(t, err, markdown)
			assert.Equal(t, kept, snapshotTexts(*captured), markdown)
		}
	})
}

func TestCreateObjectTemplateNamesTheTypeAsTheCallerDoes(t *testing.T) {
	// a space-minted type's STORED key is a bson id the caller never sent;
	// the fixtures elsewhere use `memo` for both spellings and so cannot see
	// the difference. A live run could, and did.
	const storedKey = "6ab13f0f877a91054dd66c3e"

	addMintedType := func(t *testing.T, fx *v2Fixture, defaultTemplate ...string) {
		row := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("type-minted"),
			bundle.RelationKeyName:           domain.String("Bike trip"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-" + storedKey),
			bundle.RelationKeyApiObjectKey:   domain.String("bike_trip"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		if len(defaultTemplate) > 0 {
			row[bundle.RelationKeyDefaultTemplateId] = domain.StringList(defaultTemplate)
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{row})
	}

	t.Run("a stale default warns in the api spelling, never the stored key", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		addMintedType(t, fx, "tpl-gone")
		fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"bike_trip","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, `"bike_trip"`)
		assert.NotContains(t, result.Warnings[0].Message, storedKey)
		assert.NotContains(t, result.Warnings[0].Hint, storedKey)
		require.Len(t, result.Warnings[0].SeeAlso, 1)
		assert.Equal(t, "bike_trip", result.Warnings[0].SeeAlso[0].Params["type"])
	})

	t.Run("a refused template points at the list in the api spelling", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		addMintedType(t, fx)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"bike_trip","name":"Monday","template":"tpl-nope"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.NotContains(t, apiErr.Issues[0].Hint, storedKey)
		require.Len(t, apiErr.Issues[0].SeeAlso, 1)
		assert.Equal(t, "bike_trip", apiErr.Issues[0].SeeAlso[0].Query["type"])
	})
}

func TestCreateObjectMarkdownTitleRoundFour(t *testing.T) {
	t.Run("a heading carrying a link keeps its target and its place", func(t *testing.T) {
		// given — the rendering matches the name, the content does not: the
		// link has a destination a name could never hold
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Title","markdown":"# [Title](https://example.com)\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Title", "body"}, snapshotTexts(*captured))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a heading that is only emphasis is still a duplicate", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Title","markdown":"# *Title*\n\nbody"}`), false, false)

		require.NoError(t, err)
		assert.Equal(t, []string{"body"}, snapshotTexts(*captured))
	})

	t.Run("an empty name reads as absent in either spelling", func(t *testing.T) {
		for _, body := range []string{
			`{"type":"page","name":"","markdown":"# Title\n\nbody"}`,
			`{"type":"page","properties":{"name":""},"markdown":"# Title\n\nbody"}`,
		} {
			fx := newV2Fixture(t)
			captured := fx.expectCreate("newObj")
			fx.expectEtagRead("newObj")

			_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(body), false, false)

			require.NoError(t, err, body)
			assert.Equal(t, "Title", pbtypes.GetString((*captured).Details, "name"), body)
			assert.Equal(t, []string{"body"}, snapshotTexts(*captured), body)
		}
	})

	t.Run("a caller-chosen layout keeps the heading where it is", func(t *testing.T) {
		// given — the type's recommended layout is not the one this object
		// will have, so it cannot answer whether the name becomes a title
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"layout":"note"},"markdown":"# Monday\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Monday", "body"}, snapshotTexts(*captured))
		assert.Empty(t, pbtypes.GetString((*captured).Details, "name"))
		assert.Empty(t, result.Warnings)
	})

	t.Run("a bundled note this space has not installed is exempt too", func(t *testing.T) {
		// given — no store row for `note`, so the layout comes from the
		// bundle the create is about to install
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"note","markdown":"# Monday\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Monday", "body"}, snapshotTexts(*captured))
		assert.Empty(t, pbtypes.GetString((*captured).Details, "name"))
	})

	t.Run("a key the format folds onto name supplies the name, whatever the space keys that way", func(t *testing.T) {
		// given — a space-local relation whose STORED key is `Name`. It
		// changes nothing: the format folds the spelling onto the name
		// property and refuses a document carrying both ("Name and name both
		// address property name"), so promoting beside it would produce a
		// document the format rejects
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-shadow"),
			bundle.RelationKeyRelationKey:    domain.String("Name"),
			bundle.RelationKeyName:           domain.String("Vendor name"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"Name":"Acme"},"markdown":"# Section\n\nbody"}`), false, false)

		// then — the value lands on the space's own property (an exact stored
		// key wins the resolution), so the object has no name and no title
		// was lifted. That is the only outcome that neither refuses the
		// create nor moves the caller's content: promoting would add `name`
		// beside `Name`, and the format rejects a document carrying both
		require.NoError(t, err, "the create must not be refused for a name the server added")
		assert.Empty(t, pbtypes.GetString((*captured).Details, "name"))
		assert.Equal(t, "Acme", pbtypes.GetString((*captured).Details, "Name"))
		assert.Equal(t, []string{"Section", "body"}, snapshotTexts(*captured))
		assert.Empty(t, result.Warnings)
	})
}

func TestCreateObjectMarkdownTitleRoundFive(t *testing.T) {
	// the finding all three fifth-round lenses reached independently: the
	// question "may a promotion add `name`" and the question "what is this
	// object called" are not the same question, and answering both with a
	// folded key deleted a heading that repeated nothing
	addShadow := func(t *testing.T, fx *v2Fixture, storedKey string) {
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("rel-" + storedKey),
			bundle.RelationKeyRelationKey:    domain.String(storedKey),
			bundle.RelationKeyName:           domain.String("Vendor " + storedKey),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}})
	}

	t.Run("a shadowed spelling never supplies the name a heading is compared against", func(t *testing.T) {
		// given — `Name` is this space's own property, so "Acme" is not what
		// the object is called and the heading repeats nothing
		fx := newV2Fixture(t)
		addShadow(t, fx, "Name")
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"Name":"Acme"},"markdown":"# Acme\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Acme", "body"}, snapshotTexts(*captured), "the heading was never a duplicate")
		assert.Empty(t, result.Warnings, "and nothing may claim it was")
	})

	t.Run("a shadowed spelling still blocks a promotion", func(t *testing.T) {
		// given — adding `name` beside `Name` is a document the format refuses
		fx := newV2Fixture(t)
		addShadow(t, fx, "Name")
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"Name":"Acme"},"markdown":"# Section\n\nbody"}`), false, false)

		// then
		require.NoError(t, err, "the create must not be refused for a name the server added")
		assert.Equal(t, []string{"Section", "body"}, snapshotTexts(*captured))
	})

	t.Run("a key the format folds past separators blocks a promotion too", func(t *testing.T) {
		// given — the format folds `n_ame` onto `name`, so adding `name`
		// beside it collides; a lowercase test would have missed it
		fx := newV2Fixture(t)
		addShadow(t, fx, "n_ame")
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"n_ame":"Acme"},"markdown":"# Section\n\nbody"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"Section", "body"}, snapshotTexts(*captured))
	})

	t.Run("a promoted name takes the caller's own spelling of the property", func(t *testing.T) {
		// given — an empty `Name` with a new `name` beside it is two
		// spellings of one property, which the format refuses
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","properties":{"Name":""},"markdown":"# Tegel loop\n\nbody"}`), false, false)

		// then
		require.NoError(t, err, "the promotion must not build a document the format refuses")
		assert.Equal(t, "Tegel loop", pbtypes.GetString((*captured).Details, "name"))
		assert.Equal(t, []string{"body"}, snapshotTexts(*captured))
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "became the object's name")
	})
}

func TestListTemplatesAcceptsTheTypesCreateAccepts(t *testing.T) {
	t.Run("a bundled type this space has not installed lists its templates", func(t *testing.T) {
		// given — the create path derives the id; a listing that refused
		// would hide templates a create would then accept
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addTemplate(t, "tpl-task", "Weekly task", "drv-ot-task")

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "task", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "tpl-task", rows[0].Id)
		assert.Equal(t, 1, total)
	})
}
