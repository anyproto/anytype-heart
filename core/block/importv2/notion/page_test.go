package notion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestBinEmptyRow(t *testing.T) {
	row := func(parent string, mutate func(*importv2.Object)) (*Converter, *importv2.Object, *recordingSink) {
		object := &importv2.Object{
			SourceKey: "row1",
			SbType:    coresb.SmartBlockTypePage,
			Payload: &importv2.Snapshot{Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeySourceFilePath:   domain.String("row1"),
				bundle.RelationKeyCreatedDate:      domain.Int64(1700000000),
				bundle.RelationKeyLastModifiedDate: domain.Int64(1700000000),
				bundle.RelationKeyName:             domain.String(""),
			})},
		}
		if mutate != nil {
			mutate(object)
		}
		converter := New(nil, nil, stubFactory{}, t.TempDir())
		stub := Entity{Id: "row1", Parent: Parent{Type: parent, DataSourceId: "ds1"}}
		converter.entityById["ds1"] = Entity{Id: "ds1", Title: "Contacts"}
		sink := &recordingSink{}
		converter.binEmptyRow(stub, object, sink)
		return converter, object, sink
	}

	t.Run("a row with nothing on it goes to the bin, named by its database", func(t *testing.T) {
		// when
		_, object, sink := row("data_source_id", nil)

		// then
		assert.True(t, object.Archived)
		require.Len(t, sink.issues, 1)
		assert.Equal(t, importv2.SeverityInfo, sink.issues[0].Severity)
		assert.Equal(t, "row1", sink.issues[0].SourceKey, "the report needs the row to tally it")
		assert.Equal(t, "Contacts", sink.issues[0].Subject)
	})

	t.Run("anything a person put there keeps the row", func(t *testing.T) {
		cases := map[string]func(*importv2.Object){
			"a name":           func(o *importv2.Object) { o.Payload.Details.SetString(bundle.RelationKeyName, "Bob") },
			"a checked box":    func(o *importv2.Object) { o.Payload.Details.SetBool("nprop1", true) },
			"a number, even 0": func(o *importv2.Object) { o.Payload.Details.SetFloat64("nprop1", 0) },
			"a date":           func(o *importv2.Object) { o.Payload.Details.SetInt64("nprop1", 1700000000) },
			"a select value":   func(o *importv2.Object) { o.Payload.Details.SetStringList("nprop1", []string{"opt1"}) },
			"an icon":          func(o *importv2.Object) { o.Payload.Details.SetString(bundle.RelationKeyIconEmoji, "🚀") },
			"a cover":          func(o *importv2.Object) { o.Payload.Details.SetString(bundle.RelationKeyCoverId, "file1") },
			"content of its own": func(o *importv2.Object) {
				o.Payload.Blocks = []*model.Block{{Id: "b1"}}
			},
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				// when
				_, object, sink := row("data_source_id", mutate)

				// then
				assert.False(t, object.Archived)
				assert.Empty(t, sink.issues)
			})
		}
	})

	t.Run("an empty page that is not a row is left alone", func(t *testing.T) {
		// given — someone made this page and can find it again
		_, object, sink := row("workspace", nil)

		// then
		assert.False(t, object.Archived)
		assert.Empty(t, sink.issues)
	})

	t.Run("a row already in Notion's bin is not reported twice", func(t *testing.T) {
		// when
		_, object, sink := row("data_source_id", func(o *importv2.Object) { o.Archived = true })

		// then
		assert.True(t, object.Archived)
		assert.Empty(t, sink.issues)
	})
}
