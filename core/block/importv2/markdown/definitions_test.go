package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// TestFrontMatterNamingAnytypeFields covers a vault whose front matter uses
// Anytype's own relation keys — what our own markdown export writes, and what
// a hand-written vault picks up from it. Before this, every one of them minted
// a custom relation named after the key: the page's icon became a text
// property called "iconEmoji", and every object carried a junk "id" column.
func TestFrontMatterNamingAnytypeFields(t *testing.T) {
	files := map[string]string{
		"note.md": "---\niconEmoji: 🛠️\ndescription: from front matter\nid: bafyreioldobject\ncreator: bafyreisomeidentity\nMy Column: v\n---\n# Note\n\nBody.\n",
	}

	// when
	sink, _ := runConverterWithParams(t, files, Params{})

	// then — the icon is the page's icon, not a property
	page := sink.byKey("note.md")
	require.NotNil(t, page)
	assert.Equal(t, "🛠️", page.Payload.Details.GetString(bundle.RelationKeyIconEmoji))
	assert.Equal(t, "from front matter", page.Payload.Details.GetString(bundle.RelationKeyDescription))

	// and nothing was minted for the keys Anytype already owns
	var minted []string
	for _, object := range sink.objects {
		if object.SbType == coresb.SmartBlockTypeRelation {
			minted = append(minted, object.Payload.Details.GetString(bundle.RelationKeyName))
		}
	}
	assert.Equal(t, []string{"My Column"}, minted, "only the user's own column becomes a relation")

	// and the fields that identify the object are not transplanted from a file
	assert.NotEqual(t, "bafyreioldobject", page.Payload.Details.GetString(bundle.RelationKeyId))
	assert.Empty(t, page.Payload.Details.GetString(bundle.RelationKeyCreator))

	// and the run says so once, not once per file
	var ignored []string
	for _, issue := range sink.issues {
		if issue.Code == importv2.IssueDataLoss && issue.Subject != "" {
			ignored = append(ignored, issue.Subject)
		}
	}
	assert.ElementsMatch(t, []string{"id", "creator"}, ignored)
}

func TestFrontMatterOwnedKeysAreReportedOncePerRun(t *testing.T) {
	// given — a vault where every file carries the exported id
	files := map[string]string{}
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		files[name] = "---\nid: bafyrei" + name + "\n---\n# " + name + "\n"
	}

	// when
	sink, _ := runConverterWithParams(t, files, Params{})

	// then
	var ignored int
	for _, issue := range sink.issues {
		if issue.Subject == "id" {
			ignored++
		}
	}
	assert.Equal(t, 1, ignored, "one line for the run, not one per file")
}
