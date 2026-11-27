package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// fakeSpaceLoads is apicore.SpaceLoads for the bounded-wait tests. A space
// in loading never finishes loading: WaitLoaded blocks until its context
// ends, exactly as space.Service.Get does, so a caller that puts no deadline
// on the wait blocks for as long as the test lets it.
type fakeSpaceLoads struct {
	mu      sync.Mutex
	loading map[string]bool
	errs    map[string]error
	waited  []string
}

func (f *fakeSpaceLoads) WaitLoaded(ctx context.Context, spaceId string) error {
	f.mu.Lock()
	f.waited = append(f.waited, spaceId)
	loading, err := f.loading[spaceId], f.errs[spaceId]
	f.mu.Unlock()
	if err != nil {
		return fmt.Errorf("get space %s: %w", spaceId, err)
	}
	if !loading {
		return nil
	}
	<-ctx.Done()
	return fmt.Errorf("get space %s: %w", spaceId, ctx.Err())
}

func (f *fakeSpaceLoads) waitedFor() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.waited...)
}

// withLoadingSpace makes spaceId a space whose load never ends and shortens
// the bounded wait, so a gated entry point answers within the test.
func (fx *v2Fixture) withLoadingSpace(spaceId string) *fakeSpaceLoads {
	loads := &fakeSpaceLoads{loading: map[string]bool{spaceId: true}}
	fx.spaceLoads = loads
	fx.spaceLoadWait = 30 * time.Millisecond
	return loads
}

// within fails the test when call does not return in time: an unbounded
// wait must fail the test, not hang it.
func within(t *testing.T, limit time.Duration, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("the call did not return within %s: the wait is not bounded", limit)
		return nil
	}
}

func requireSpaceLoading(t *testing.T, err error) {
	t.Helper()
	apiErr := v2Err(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
	assert.Equal(t, v2model.CodeSpaceLoading, apiErr.Code)
	assert.Equal(t, 5, apiErr.RetryAfterSeconds)
	assert.Equal(t, `space "space1" is still loading on this device`, apiErr.Message)
}

func TestEnsureSpaceLoaded(t *testing.T) {
	t.Run("a loaded space passes at once", func(t *testing.T) {
		// given: the load of ANOTHER space never ends
		fx := newV2Fixture(t)
		loads := fx.withLoadingSpace("otherSpace")
		fx.spaceLoadWait = time.Hour

		// when
		err := within(t, 5*time.Second, func() error { return fx.ensureSpaceLoaded(context.Background(), testSpaceId) })

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{testSpaceId}, loads.waitedFor())
	})

	t.Run("a space still loading at the bound is a 503 with Retry-After", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.withLoadingSpace(testSpaceId)

		// when
		err := within(t, 5*time.Second, func() error { return fx.ensureSpaceLoaded(context.Background(), testSpaceId) })

		// then
		requireSpaceLoading(t, err)
	})

	t.Run("a space this account does not hold is the 404 of the route's own lookup", func(t *testing.T) {
		for _, loadErr := range []error{space.ErrSpaceNotExists, space.ErrSpaceDeleted, space.ErrSpaceStorageMissig} {
			// given
			fx := newV2Fixture(t)
			fx.spaceLoads = &fakeSpaceLoads{errs: map[string]error{testSpaceId: loadErr}}

			// when
			err := fx.ensureSpaceLoaded(context.Background(), testSpaceId)

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusNotFound, apiErr.Status, loadErr.Error())
			assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
		}
	})

	t.Run("any other load failure stays an internal error carrying its cause", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.spaceLoads = &fakeSpaceLoads{errs: map[string]error{testSpaceId: space.ErrFailedToLoad}}

		// when
		err := fx.ensureSpaceLoaded(context.Background(), testSpaceId)

		// then
		require.ErrorIs(t, err, space.ErrFailedToLoad)
		var apiErr *v2model.Error
		assert.False(t, errors.As(err, &apiErr), "not a shaped refusal, and above all not a 503")
	})

	t.Run("a cancelled request ends with its own error, not a 503", func(t *testing.T) {
		// given: the bound is far away, so only the cancellation can end the wait
		fx := newV2Fixture(t)
		fx.withLoadingSpace(testSpaceId)
		fx.spaceLoadWait = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)

		// when
		err := within(t, 5*time.Second, func() error { return fx.ensureSpaceLoaded(ctx, testSpaceId) })

		// then
		require.ErrorIs(t, err, context.Canceled)
		var apiErr *v2model.Error
		assert.False(t, errors.As(err, &apiErr))
	})

	t.Run("a request whose own deadline passes first is not a 503 either", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.withLoadingSpace(testSpaceId)
		fx.spaceLoadWait = time.Hour
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		// when
		err := within(t, 5*time.Second, func() error { return fx.ensureSpaceLoaded(ctx, testSpaceId) })

		// then
		require.ErrorIs(t, err, context.DeadlineExceeded)
		var apiErr *v2model.Error
		assert.False(t, errors.As(err, &apiErr))
	})

	t.Run("without the port nothing waits", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		require.Nil(t, fx.spaceLoads)

		// when
		err := fx.ensureSpaceLoaded(context.Background(), testSpaceId)

		// then
		require.NoError(t, err)
	})
}

// TestWritesIntoALoadingSpaceAreBounded covers every entry point that needs
// the loaded space: on a space whose load never ends each answers 503 within
// the bound and calls nothing. The fixture's mocks are strict — an entry
// point that reached its RPC, creator, reader or widget port without the
// gate fails on the unexpected call (and the reads would block on it in
// production for as long as the client stays connected; the writes that drop
// the request context, past that).
func TestWritesIntoALoadingSpaceAreBounded(t *testing.T) {
	name := "Renamed"
	limit := 10
	calls := map[string]func(fx *v2Fixture) error{
		"update space (WorkspaceSetInfo drops the context)": func(fx *v2Fixture) error {
			_, err := fx.UpdateSpace(context.Background(), testSpaceId, v2model.UpdateSpaceRequest{Name: &name}, false)
			return err
		},
		"update type (ObjectSetDetails drops the context)": func(fx *v2Fixture) error {
			_, err := fx.UpdateType(context.Background(), testSpaceId, "page", "", []byte(`{"name":"Renamed"}`), false, false)
			return err
		},
		"update property (ObjectSetDetails drops the context)": func(fx *v2Fixture) error {
			_, err := fx.UpdateProperty(context.Background(), testSpaceId, "severity", v2model.UpdatePropertyRequest{Name: &name}, false)
			return err
		},
		"delete property": func(fx *v2Fixture) error {
			_, err := fx.DeleteProperty(context.Background(), testSpaceId, "severity", false)
			return err
		},
		"create property": func(fx *v2Fixture) error {
			_, err := fx.CreateProperty(context.Background(), testSpaceId, v2model.CreatePropertyRequest{Name: "Effort", Format: "number"}, false)
			return err
		},
		"delete type": func(fx *v2Fixture) error {
			_, err := fx.DeleteType(context.Background(), testSpaceId, "chore", false)
			return err
		},
		"create chat": func(fx *v2Fixture) error {
			_, err := fx.CreateChat(context.Background(), testSpaceId, v2model.CreateChatRequest{Name: "General"}, false)
			return err
		},
		"upload file": func(fx *v2Fixture) error {
			_, err := fx.UploadFile(context.Background(), testSpaceId, "", "https://example.com/a.png", "", false)
			return err
		},
		"create query": func(fx *v2Fixture) error {
			_, err := fx.CreateQuery(context.Background(), testSpaceId, v2model.CreateQueryRequest{Name: "Chores", Type: "chore"}, false, false)
			return err
		},
		"create collection": func(fx *v2Fixture) error {
			_, err := fx.CreateCollection(context.Background(), testSpaceId, v2model.CreateCollectionRequest{Name: "Shelf"}, false)
			return err
		},
		"create widget": func(fx *v2Fixture) error {
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "obj1", Scope: "personal"}, false)
			return err
		},
		"update widget": func(fx *v2Fixture) error {
			_, err := fx.UpdateWidget(context.Background(), testSpaceId, "widget1", "", v2model.UpdateWidgetRequest{Limit: &limit}, false)
			return err
		},
		"templates of a built-in type the space has not installed": func(fx *v2Fixture) error {
			_, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, "page", 0, 10)
			return err
		},
		"create object": func(fx *v2Fixture) error {
			_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{"type":"page","name":"New"}`), false, false)
			return err
		},
		"patch object": func(fx *v2Fixture) error {
			_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", []byte(`{"ops":[]}`), "", false, false)
			return err
		},
		"delete object": func(fx *v2Fixture) error {
			_, err := fx.DeleteObject(callerCtx(), testSpaceId, "obj1", false, false)
			return err
		},
		"chat write": func(fx *v2Fixture) error {
			_, err := fx.AddChatMessage(context.Background(), testSpaceId, "chat1", v2model.AddChatMessageRequest{Text: "hi"}, false)
			return err
		},
		"chat read": func(fx *v2Fixture) error {
			_, err := fx.GetChatMessages(context.Background(), testSpaceId, "chat1", ChatMessagesQuery{})
			return err
		},
		"object read": func(fx *v2Fixture) error {
			_, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", ObjectQuery{})
			return err
		},
		"type document read": func(fx *v2Fixture) error {
			_, _, err := fx.GetType(context.Background(), testSpaceId, "chore", ObjectQuery{})
			return err
		},
		"query objects read": func(fx *v2Fixture) error {
			_, _, _, _, err := fx.GetQueryObjects(context.Background(), testSpaceId, "query1", "", nil, 0, 10)
			return err
		},
		"widget list": func(fx *v2Fixture) error {
			_, _, _, _, err := fx.ListWidgets(context.Background(), testSpaceId, "", 0, 10)
			return err
		},
		"file content": func(fx *v2Fixture) error {
			_, err := fx.GetFileContent(context.Background(), testSpaceId, "fileObj1", 0)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			// given
			fx := newV2Fixture(t)
			// the index holds what the entry points resolve before they
			// need the space: a type and a property
			fx.addTaskType(t)
			fx.addSelectProperty(t)
			loads := fx.withLoadingSpace(testSpaceId)

			// when
			err := within(t, 5*time.Second, func() error { return call(fx) })

			// then
			requireSpaceLoading(t, err)
			assert.Equal(t, []string{testSpaceId}, loads.waitedFor())
			// the id derivation runs in the space: the fixture answers it
			// leniently, so the gate is checked to come before it by name
			fx.creatorMock.AssertNotCalled(t, "TypeIdByKey", mock.Anything, mock.Anything, mock.Anything)
			fx.creatorMock.AssertNotCalled(t, "RelationIdByKey", mock.Anything, mock.Anything, mock.Anything)
		})
	}

	t.Run("a template list that needs no derived id is a store read and does not wait", func(t *testing.T) {
		for _, typeTerm := range []string{"", "chore"} {
			// given: no filter, and a type that has a store row
			fx := newV2Fixture(t)
			fx.addTaskType(t)
			loads := fx.withLoadingSpace(testSpaceId)

			// when
			_, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, typeTerm, 0, 10)

			// then
			require.NoError(t, err, typeTerm)
			assert.Empty(t, loads.waitedFor(), typeTerm)
		}
	})

	t.Run("a dry-run space update sends nothing and does not wait", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		loads := fx.withLoadingSpace(testSpaceId)

		// when
		got, err := fx.UpdateSpace(context.Background(), testSpaceId, v2model.UpdateSpaceRequest{Name: &name}, true)

		// then
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Empty(t, loads.waitedFor())
	})

	t.Run("a store read of the same space does not wait", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		loads := fx.withLoadingSpace(testSpaceId)

		// when
		_, _, _, err := fx.ListObjects(context.Background(), testSpaceId, nil, 0, 10)

		// then
		require.NoError(t, err)
		assert.Empty(t, loads.waitedFor())
	})

	t.Run("a refused key is refused before the wait", func(t *testing.T) {
		// given: the grant names another space
		fx := newV2Fixture(t)
		loads := fx.withLoadingSpace(testSpaceId)
		ctx := grantCtx(util.GrantPermsReadWrite, "otherSpace")

		// when
		_, err := fx.CreateObject(ctx, testSpaceId, []byte(`{"type":"page"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeSpaceNotGranted, apiErr.Code)
		assert.Empty(t, loads.waitedFor())
	})
}

func TestComputeEtag(t *testing.T) {
	t.Run("deterministic and order-independent", func(t *testing.T) {
		// given
		a := ComputeEtag([]string{"headA", "headB"})
		b := ComputeEtag([]string{"headB", "headA"})

		// then
		assert.Equal(t, a, b)
		assert.Len(t, a, 8)
	})

	t.Run("different heads give different etags", func(t *testing.T) {
		assert.NotEqual(t, ComputeEtag([]string{"headA"}), ComputeEtag([]string{"headB"}))
	})

	t.Run("head concatenation is unambiguous", func(t *testing.T) {
		// "ab"+"c" must not hash like "a"+"bc"
		assert.NotEqual(t, ComputeEtag([]string{"ab", "c"}), ComputeEtag([]string{"a", "bc"}))
	})
}

func TestEtagMatches(t *testing.T) {
	heads := []string{"headA", "headB"}

	t.Run("absent If-Match is advisory pass", func(t *testing.T) {
		assert.True(t, EtagMatches("", heads))
	})

	t.Run("display form matches", func(t *testing.T) {
		assert.True(t, EtagMatches(ComputeEtag(heads), heads))
	})

	t.Run("full hash matches", func(t *testing.T) {
		assert.True(t, EtagMatches(headsHash(heads), heads))
	})

	t.Run("stale etag does not match", func(t *testing.T) {
		assert.False(t, EtagMatches(ComputeEtag([]string{"other"}), heads))
	})

	t.Run("quoted and weak header forms match (RFC 7232)", func(t *testing.T) {
		etag := ComputeEtag(heads)
		assert.Equal(t, `"`+etag+`"`, QuoteEtag(etag))
		assert.True(t, EtagMatches(QuoteEtag(etag), heads), "a client echoing the quoted ETag header must match")
		assert.True(t, EtagMatches(`W/"`+etag+`"`, heads), "a weak indicator is tolerated")
	})
}

func TestEncodeEnvelope(t *testing.T) {
	t.Run("canonical key order with unknown keys last", func(t *testing.T) {
		// given
		fields := map[string]json.RawMessage{
			"blocks":        json.RawMessage(`[]`),
			"etag":          json.RawMessage(`"abcd1234"`),
			"formatVersion": json.RawMessage(`"2.0"`),
			"id":            json.RawMessage(`"obj1"`),
			"zzz":           json.RawMessage(`true`),
		}
		want := `{"formatVersion":"2.0","etag":"abcd1234","id":"obj1","blocks":[],"zzz":true}`

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("compacts the interior of embedded values (C3)", func(t *testing.T) {
		// given — the indented bytes anyblockjson.Marshal produces (SPEC §4
		// canonical form), re-embedded verbatim by parseEnvelope
		fields := map[string]json.RawMessage{
			"id":     json.RawMessage(`"obj1"`),
			"blocks": json.RawMessage("[\n  {\n    \"id\": \"b1\",\n    \"type\": \"paragraph\",\n    \"text\": \"a  b\"\n  }\n]"),
			"refs":   json.RawMessage("{\n  \"ab12c\": \"bafyreiab12c\"\n}"),
		}
		want := `{"id":"obj1","refs":{"ab12c":"bafyreiab12c"},"blocks":[{"id":"b1","type":"paragraph","text":"a  b"}]}`

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "C3 is about the whole body, not just the envelope")
	})

	t.Run("string contents survive compaction byte for byte", func(t *testing.T) {
		// given — the format's writer escapes without HTML escaping;
		// json.Compact must neither re-escape `<`/`>`/`&` nor rewrite a RAW
		// U+2028/U+2029. The fixture carries the six-character \u2028 escape
		// (which Compact could never touch) AND the raw U+2028 and U+2029
		// characters after "raw:" — only the raw ones actually pin the claim.
		fields := map[string]json.RawMessage{
			"id": json.RawMessage("{\n  \"a\": \"<b>&amp; \\u2028 raw:   café — ünïcode\"\n}"),
		}

		// when
		got, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		want := "{\"id\":{\"a\":\"<b>&amp; \\u2028 raw:   caf\u00e9 \u2014 \u00fcn\u00efcode\"}}"
		assert.Equal(t, want, string(got))
	})

	t.Run("round-trips through parseEnvelope", func(t *testing.T) {
		// given
		doc := []byte(`{"formatVersion":"2.0","id":"x","blocks":[{"type":"paragraph","text":"hi"}]}`)

		// when
		fields, err := parseEnvelope(doc)
		require.NoError(t, err)
		out, err := encodeEnvelope(fields)

		// then
		require.NoError(t, err)
		assert.JSONEq(t, string(doc), string(out))
	})
}

func TestV2ObjectQueryValidate(t *testing.T) {
	tests := []struct {
		name     string
		query    ObjectQuery
		wantErr  string // expected error code, "" = valid
		wantPlan func(t *testing.T, plan objectReadPlan)
	}{
		{
			// the edit shape: short labels for minted block ids; object refs
			// are full inline on every shape (the legend is a measured loss —
			// TOKENS §1.2, §8.26)
			name:  "defaults: both sections, short block labels, anyblock",
			query: ObjectQuery{},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.wantProperties)
				assert.True(t, plan.wantBlocks)
				assert.True(t, plan.compactBlockLabels)
				assert.False(t, plan.markdown)
			},
		},
		{
			name:  "include=properties suppresses blocks",
			query: ObjectQuery{Include: "properties"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.wantProperties)
				assert.False(t, plan.wantBlocks)
			},
		},
		{
			// the export shape: full block ids so a GET body PUTs back as a
			// minimal diff; no legend here either
			name:  "ids=full is the export shape: full block ids",
			query: ObjectQuery{Ids: "full"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.False(t, plan.compactBlockLabels)
			},
		},
		{
			name:  "ids=compact is the explicit spelling of the default",
			query: ObjectQuery{Ids: "compact"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.compactBlockLabels)
			},
		},
		{
			// T7: the outline fixes the axis and ignores ?ids=
			name:  "outline fixes the axis and ignores ids=full",
			query: ObjectQuery{Outline: true, Ids: "full"},
			wantPlan: func(t *testing.T, plan objectReadPlan) {
				assert.True(t, plan.compactBlockLabels)
			},
		},
		{name: "outline and block conflict", query: ObjectQuery{Outline: true, Block: "b1"}, wantErr: "ambiguous_input"},
		{name: "outline and md conflict", query: ObjectQuery{Outline: true, Format: "md"}, wantErr: "ambiguous_input"},
		{name: "block and md conflict", query: ObjectQuery{Block: "b1", Format: "md"}, wantErr: "ambiguous_input"},
		{name: "unknown ids value", query: ObjectQuery{Ids: "short"}, wantErr: "validation_failed"},
		{name: "unknown format value", query: ObjectQuery{Format: "html"}, wantErr: "validation_failed"},
		{name: "unknown include value", query: ObjectQuery{Include: "blocks,everything"}, wantErr: "validation_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := tt.query.validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				tt.wantPlan(t, plan)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestDryRunsAndRefusalsDoNotWaitForALoad pins where a write that validates
// against the index waits for a space that is still loading
// (APIV2_SPACE_STREAM.md §3.4):
//
//   - a refusal of the request itself — body shape, field values — reads
//     nothing from the space and answers at once, real write or dry run;
//   - a dry run never waits: it sends nothing and answers from the index as
//     it is;
//   - a real write waits before its first read of the index, so what a
//     partial index would call "not found" or "no duplicate" is the
//     retryable 503 instead.
//
// The real writes that pass every check are in
// TestWritesIntoALoadingSpaceAreBounded.
func TestDryRunsAndRefusalsDoNotWaitForALoad(t *testing.T) {
	name := "Renamed"
	empty := ""
	bg := context.Background()
	// given, for every case: a space whose load never ends, holding one type
	// ("chore") and one property ("severity", named "Severity") in its index
	loading := func(t *testing.T, bound time.Duration) (*v2Fixture, *fakeSpaceLoads) {
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		fx.addSelectProperty(t)
		// the space view says loading too, as it does whenever the load is in
		// progress: a schema dry run reads it to leave the Bin unopened
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("spaceView_" + testSpaceId),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String(testSpaceId),
			bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(model.SpaceStatus_Loading)),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceActive)),
		}})
		loads := fx.withLoadingSpace(testSpaceId)
		fx.spaceLoadWait = bound
		return fx, loads
	}

	dryRuns := map[string]func(fx *v2Fixture) (bool, error){
		"create chat": func(fx *v2Fixture) (bool, error) {
			got, err := fx.CreateChat(bg, testSpaceId, v2model.CreateChatRequest{Name: "General"}, true)
			return got != nil && got.DryRun, err
		},
		"upload file": func(fx *v2Fixture) (bool, error) {
			got, err := fx.UploadFile(bg, testSpaceId, "", "https://example.com/a.png", "", true)
			return got != nil && got.DryRun, err
		},
		"delete type": func(fx *v2Fixture) (bool, error) {
			got, err := fx.DeleteType(bg, testSpaceId, "chore", true)
			return got != nil && got.DryRun, err
		},
		"create property": func(fx *v2Fixture) (bool, error) {
			got, err := fx.CreateProperty(bg, testSpaceId, v2model.CreatePropertyRequest{Name: "Effort", Format: "number"}, true)
			return got != nil && got.DryRun, err
		},
		"update property": func(fx *v2Fixture) (bool, error) {
			got, err := fx.UpdateProperty(bg, testSpaceId, "severity", v2model.UpdatePropertyRequest{Name: &name}, true)
			return got != nil && got.DryRun, err
		},
		"delete property": func(fx *v2Fixture) (bool, error) {
			got, err := fx.DeleteProperty(bg, testSpaceId, "severity", true)
			return got != nil && got.DryRun, err
		},
		"update space": func(fx *v2Fixture) (bool, error) {
			got, err := fx.UpdateSpace(bg, testSpaceId, v2model.UpdateSpaceRequest{Name: &name}, true)
			return got != nil && got.DryRun, err
		},
	}
	for label, call := range dryRuns {
		t.Run("a dry run does not wait: "+label, func(t *testing.T) {
			// given: a wait would hit the test's own deadline
			fx, loads := loading(t, time.Hour)

			// when
			var dryRun bool
			err := within(t, 5*time.Second, func() (err error) {
				dryRun, err = call(fx)
				return err
			})

			// then
			require.NoError(t, err)
			assert.True(t, dryRun)
			assert.Empty(t, loads.waitedFor())
		})
	}

	// refusals of the request itself: real writes
	badRequests := map[string]func(fx *v2Fixture) error{
		"create chat without a name": func(fx *v2Fixture) error {
			_, err := fx.CreateChat(bg, testSpaceId, v2model.CreateChatRequest{}, false)
			return err
		},
		"upload with neither file nor url": func(fx *v2Fixture) error {
			_, err := fx.UploadFile(bg, testSpaceId, "", "", "", false)
			return err
		},
		"create a property with an unknown format": func(fx *v2Fixture) error {
			_, err := fx.CreateProperty(bg, testSpaceId, v2model.CreatePropertyRequest{Name: "Effort", Format: "nope"}, false)
			return err
		},
		"update a property with nothing": func(fx *v2Fixture) error {
			_, err := fx.UpdateProperty(bg, testSpaceId, "severity", v2model.UpdatePropertyRequest{}, false)
			return err
		},
		"update a space with an empty name": func(fx *v2Fixture) error {
			_, err := fx.UpdateSpace(bg, testSpaceId, v2model.UpdateSpaceRequest{Name: &empty}, false)
			return err
		},
		"create a query without a name": func(fx *v2Fixture) error {
			_, err := fx.CreateQuery(bg, testSpaceId, v2model.CreateQueryRequest{Type: "chore"}, false, false)
			return err
		},
		"create a collection without a name": func(fx *v2Fixture) error {
			_, err := fx.CreateCollection(bg, testSpaceId, v2model.CreateCollectionRequest{}, false)
			return err
		},
		"create a widget with an unknown scope": func(fx *v2Fixture) error {
			_, err := fx.CreateWidget(bg, testSpaceId, v2model.CreateWidgetRequest{Target: "obj1", Scope: "nowhere"}, false)
			return err
		},
		"update a widget with nothing": func(fx *v2Fixture) error {
			_, err := fx.UpdateWidget(bg, testSpaceId, "widget1", "", v2model.UpdateWidgetRequest{}, false)
			return err
		},
	}
	for label, call := range badRequests {
		t.Run("a real write refused for its own content does not wait: "+label, func(t *testing.T) {
			// given
			fx, loads := loading(t, time.Hour)

			// when
			err := within(t, 5*time.Second, func() error { return call(fx) })

			// then: the body's own 400, at once
			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status, apiErr.Message)
			assert.Empty(t, loads.waitedFor())
		})
	}

	// refusals concluded from the index: the same request as a real write
	// and as a dry run
	indexRefusals := map[string]struct {
		call       func(fx *v2Fixture, dryRun bool) error
		wantStatus int
	}{
		"delete an unknown type": {func(fx *v2Fixture, dryRun bool) error {
			_, err := fx.DeleteType(bg, testSpaceId, "no_such_type", dryRun)
			return err
		}, http.StatusNotFound},
		"update an unknown property": {func(fx *v2Fixture, dryRun bool) error {
			_, err := fx.UpdateProperty(bg, testSpaceId, "no_such_property", v2model.UpdatePropertyRequest{Name: &name}, dryRun)
			return err
		}, http.StatusNotFound},
		"delete an unknown property": {func(fx *v2Fixture, dryRun bool) error {
			_, err := fx.DeleteProperty(bg, testSpaceId, "no_such_property", dryRun)
			return err
		}, http.StatusNotFound},
		"create a property whose key is taken": {func(fx *v2Fixture, dryRun bool) error {
			_, err := fx.CreateProperty(bg, testSpaceId, v2model.CreatePropertyRequest{Key: "severity", Name: "Severity again", Format: "select"}, dryRun)
			return err
		}, http.StatusBadRequest},
		"create a collection of an unknown object": {func(fx *v2Fixture, dryRun bool) error {
			_, err := fx.CreateCollection(bg, testSpaceId, v2model.CreateCollectionRequest{Name: "Shelf", Items: []string{"no_such_object"}}, dryRun)
			return err
		}, http.StatusBadRequest},
	}
	for label, tc := range indexRefusals {
		t.Run("a real write waits before it reads the index: "+label, func(t *testing.T) {
			// given: the index cannot be trusted yet
			fx, loads := loading(t, 30*time.Millisecond)

			// when
			err := within(t, 5*time.Second, func() error { return tc.call(fx, false) })

			// then: the retryable 503, not a 404 or a duplicate from a partial index
			requireSpaceLoading(t, err)
			assert.Equal(t, []string{testSpaceId}, loads.waitedFor())
		})

		t.Run("a dry run answers from the index without waiting: "+label, func(t *testing.T) {
			// given
			fx, loads := loading(t, time.Hour)

			// when
			err := within(t, 5*time.Second, func() error { return tc.call(fx, true) })

			// then
			apiErr := v2Err(t, err)
			assert.Equal(t, tc.wantStatus, apiErr.Status, apiErr.Message)
			assert.Empty(t, loads.waitedFor())
		})
	}
}
