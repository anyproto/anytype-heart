package notion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type issueRecorder struct {
	issues []importv2.Issue
}

func (r *issueRecorder) Object(ctx context.Context, o *importv2.Object) error { return nil }
func (r *issueRecorder) Issue(i importv2.Issue)                               { r.issues = append(r.issues, i) }
func (r *issueRecorder) Progress(delta int64)                                 {}

func TestSuggestPageType(t *testing.T) {
	t.Run("database title types its rows via both parent forms", func(t *testing.T) {
		// given
		c := New(nil, nil, nil, t.TempDir())
		sink := &issueRecorder{}
		database := &databaseObject{Name: "Tasks", Properties: map[string]propertySchema{}}

		// when
		c.suggestPageType("entity-1", "ds-1", database, nil, sink)

		// then — rows resolve through data_source_id and database_id parents
		byDataSource := Entity{Parent: Parent{Type: "data_source_id", DataSourceId: "ds-1"}}
		byDatabase := Entity{Parent: Parent{Type: "database_id", DatabaseId: "entity-1"}}
		assert.Equal(t, bundle.TypeKeyTask.String(), c.pageTypeKey(byDataSource))
		assert.Equal(t, bundle.TypeKeyTask.String(), c.pageTypeKey(byDatabase))

		require.Len(t, sink.issues, 1)
		assert.Equal(t, importv2.IssueTypeSuggested, sink.issues[0].Code)
		assert.Equal(t, importv2.SeverityInfo, sink.issues[0].Severity)
	})

	t.Run("schema shape types rows when the title says nothing", func(t *testing.T) {
		// given — email + phone properties on a neutrally named database
		c := New(nil, nil, nil, t.TempDir())
		database := &databaseObject{
			Name: "CRM",
			Properties: map[string]propertySchema{
				"Email": {Name: "Email", Type: "email"},
				"Phone": {Name: "Phone", Type: "phone_number"},
			},
		}

		// when
		c.suggestPageType("entity-1", "ds-1", database, []string{"Email", "Phone"}, &issueRecorder{})

		// then
		stub := Entity{Parent: Parent{Type: "data_source_id", DataSourceId: "ds-1"}}
		assert.Equal(t, bundle.TypeKeyContact.String(), c.pageTypeKey(stub))
	})

	t.Run("unsuggested databases keep Page rows", func(t *testing.T) {
		// given / when
		c := New(nil, nil, nil, t.TempDir())
		sink := &issueRecorder{}
		c.suggestPageType("entity-1", "ds-1", &databaseObject{Name: "Stuff"}, nil, sink)

		// then
		stub := Entity{Parent: Parent{Type: "data_source_id", DataSourceId: "ds-1"}}
		assert.Equal(t, bundle.TypeKeyPage.String(), c.pageTypeKey(stub))
		assert.Empty(t, sink.issues)
	})

	t.Run("workspace-parented pages stay Pages", func(t *testing.T) {
		// given / when
		c := New(nil, nil, nil, t.TempDir())

		// then
		stub := Entity{Parent: Parent{Type: "workspace"}}
		assert.Equal(t, bundle.TypeKeyPage.String(), c.pageTypeKey(stub))
	})
}
