package v2service

// servedexamples_test.go answers one question for every create kind: does the
// endpoint accept the example we publish beside its schema?
//
// Nothing else asked it. The schema/example agreement is checked in
// schemas_test.go, and both halves can agree on a body the runtime refuses —
// which is what happened: the type example carried `"api_key":"task"` and
// `task` is a bundled type, so GET /v2/schemas/type published a body that
// POST /types answered with "type key is reserved". It shipped that way
// because validating an example against its own schema cannot see the space.
//
// Dry runs, so this asserts acceptance without writing anything.
//
// Every flag is sent at its DOCUMENTED DEFAULT. An earlier version of this
// file passed createMissingOptions=true, and that hid the very class of bug it
// was written for: the `type` example declared select options, which need
// ?create_missing_options=true, so a caller copying it at default settings got
// "option does not exist". An example that only works with a non-default query
// param is not a working example.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestV2ServedExamplesAreAcceptedByTheirEndpoints(t *testing.T) {
	// decodeInto sends an example that the endpoint takes as a typed request
	// rather than as a raw document body.
	decodeInto := func(t *testing.T, example []byte, req any) {
		t.Helper()
		require.NoError(t, json.Unmarshal(example, req),
			"the published example does not fit the request type")
	}

	// prepare supplies what a real space supplies and the bare fixture does
	// not: an installed bundled type, an object to point at. It returns the
	// body to send, so a case whose published example necessarily names
	// space-specific ids can substitute real ones.
	cases := []struct {
		kind    string
		prepare func(t *testing.T, fx *v2Fixture, example []byte) []byte
		send    func(t *testing.T, fx *v2Fixture, ctx context.Context, example []byte) error
	}{
		{"object", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			_, err := fx.CreateObject(ctx, testSpaceId, b, true, false)
			return err
		}},
		{"shortcut", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			_, err := fx.CreateObject(ctx, testSpaceId, b, true, false)
			return err
		}},
		{"type", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			_, err := fx.CreateType(ctx, testSpaceId, b, true, false)
			return err
		}},
		{"type_document", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			_, err := fx.CreateType(ctx, testSpaceId, b, true, false)
			return err
		}},
		{"template", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			_, err := fx.CreateTemplate(ctx, testSpaceId, b, true, false)
			return err
		}},
		{"property", nil, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			var req v2model.CreatePropertyRequest
			decodeInto(t, b, &req)
			_, err := fx.CreateProperty(ctx, testSpaceId, req, true)
			return err
		}},
		{"query", func(t *testing.T, fx *v2Fixture, example []byte) []byte {
			// the example queries `task`, a BUNDLED type: present in every real
			// space, absent from the bare fixture. Installing it is what makes
			// this a test of the example rather than of the fixture.
			// a top-level type narrows the filterable keys to that type's
			// recommended set, so the type has to recommend what the example
			// filters and sorts on, exactly as the bundled Task does.
			fx.addType(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String("type-task"),
				bundle.RelationKeyUniqueKey:    domain.String("ot-task"),
				bundle.RelationKeyApiObjectKey: domain.String("task"),
				bundle.RelationKeyName:         domain.String("Task"),
				bundle.RelationKeyRecommendedRelations: domain.StringList(
					[]string{"rel-done", "rel-due-date"}),
			})
			// and the properties it filters and sorts on, both bundled too.
			// `due_date` is the served slug for the stored key `dueDate`.
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String("rel-done"),
				bundle.RelationKeyRelationKey:  domain.String("done"),
				bundle.RelationKeyApiObjectKey: domain.String("done"),
				bundle.RelationKeyName:         domain.String("Done"),
				bundle.RelationKeyRelationFormat: domain.Int64(
					int64(model.RelationFormat_checkbox)),
			})
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String("rel-due-date"),
				bundle.RelationKeyRelationKey:  domain.String("dueDate"),
				bundle.RelationKeyApiObjectKey: domain.String("due_date"),
				bundle.RelationKeyName:         domain.String("Due date"),
				bundle.RelationKeyRelationFormat: domain.Int64(
					int64(model.RelationFormat_date)),
			})
			return example
		}, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			var req v2model.CreateQueryRequest
			decodeInto(t, b, &req)
			_, err := fx.CreateQuery(ctx, testSpaceId, req, true, false)
			return err
		}},
		{"collection", func(t *testing.T, fx *v2Fixture, example []byte) []byte {
			// the published items are elided placeholders, not ids anyone can
			// send — no example can name real object ids. Assert they are still
			// placeholders, then substitute ids this space has, so the rest of
			// the body is covered.
			var published struct {
				Items []string `json:"items"`
			}
			require.NoError(t, json.Unmarshal(example, &published))
			require.NotEmpty(t, published.Items)
			for _, id := range published.Items {
				assert.True(t, strings.ContainsRune(id, '…'),
					"%q reads as a real id: either it is one, and this case should send it, or the ellipsis was dropped", id)
			}
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("member1"),
				bundle.RelationKeyName:           domain.String("First"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}, {
				bundle.RelationKeyId:             domain.String("member2"),
				bundle.RelationKeyName:           domain.String("Second"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}})
			var body map[string]any
			require.NoError(t, json.Unmarshal(example, &body))
			body["items"] = []string{"member1", "member2"}
			rewritten, err := json.Marshal(body)
			require.NoError(t, err)
			return rewritten
		}, func(t *testing.T, fx *v2Fixture, ctx context.Context, b []byte) error {
			var req v2model.CreateCollectionRequest
			decodeInto(t, b, &req)
			_, err := fx.CreateCollection(ctx, testSpaceId, req, true)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			// given: exactly what GET /v2/schemas/<kind> publishes
			fx := newV2Fixture(t)
			entry, err := fx.SchemaKind(tc.kind)
			require.NoError(t, err)
			require.NotEmpty(t, entry.Example, "%s publishes no example", tc.kind)

			body := entry.Example
			if tc.prepare != nil {
				body = tc.prepare(t, fx, body)
			}

			// when: sent to the endpoint that entry names
			err = tc.send(t, fx, context.Background(), body)

			// then
			if err != nil {
				for _, iss := range v2Err(t, err).Issues {
					t.Logf("issue path=%q message=%q hint=%q", iss.Path, iss.Message, iss.Hint)
				}
			}
			require.NoErrorf(t, err,
				"GET /v2/schemas/%s publishes an example that %s refuses", tc.kind, entry.Endpoint)
		})
	}
}
