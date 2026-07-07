package markdown

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
)

// enumerate runs pass 1 only and returns the converter with its flavour
// resolved.
func enumerate(t *testing.T, files map[string]string, params Params) (*Converter, error) {
	t.Helper()
	src, err := source.Open(writeTree(t, files))
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	converter := New(src, params, stubFactory{})
	enumErr := converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil })
	return converter, enumErr
}

func TestFlavourDetection(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "plain markdown is generic",
			files: map[string]string{"a.md": "# A\n", "b.md": "# B\n", "data.csv": "x\n"},
			want:  FlavourGeneric,
		},
		{
			name: "x-app schemas mean anytype-export",
			files: map[string]string{
				"work.md":          "# W\n",
				"task.schema.json": taskSchema,
			},
			want: FlavourAnytypeExport,
		},
		{
			name: "an .obsidian config dir means obsidian",
			files: map[string]string{
				"Note.md":            "# N\n",
				".obsidian/app.json": "{}",
			},
			want: FlavourObsidian,
		},
		{
			name: "notion id suffixes mean notion-export",
			files: map[string]string{
				"Page one 0123456789abcdef0123456789abcdef.md": "# One\n",
				"Page two fedcba9876543210fedcba9876543210.md": "# Two\n",
			},
			want: FlavourNotionExport,
		},
		{
			name: "a csv with sibling-directory pages means notion-export",
			files: map[string]string{
				"Tasks.csv":  "Name\n",
				"Tasks/a.md": "# A\n",
			},
			want: FlavourNotionExport,
		},
		{
			name: "one id-suffixed file among plain ones stays generic",
			files: map[string]string{
				"Page 0123456789abcdef0123456789abcdef.md": "# P\n",
				"a.md": "# A\n",
				"b.md": "# B\n",
			},
			want: FlavourGeneric,
		},
		{
			name: "schemas outrank an .obsidian dir",
			files: map[string]string{
				"work.md":            "# W\n",
				"task.schema.json":   taskSchema,
				".obsidian/app.json": "{}",
			},
			want: FlavourAnytypeExport,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// given / when
			converter, err := enumerate(t, tc.files, Params{})

			// then
			require.NoError(t, err)
			assert.Equal(t, tc.want, converter.flavour.Name)
			assert.False(t, converter.flavourForced)
		})
	}
}

func TestFlavourForced(t *testing.T) {
	t.Run("request override beats detection", func(t *testing.T) {
		// given — a source that would detect as notion-export
		files := map[string]string{"Tasks.csv": "Name\n", "Tasks/a.md": "# A\n"}

		// when
		converter, err := enumerate(t, files, Params{Flavour: FlavourObsidian})

		// then
		require.NoError(t, err)
		assert.Equal(t, FlavourObsidian, converter.flavour.Name)
		assert.True(t, converter.flavourForced)
	})

	t.Run("unknown flavour name is fatal", func(t *testing.T) {
		// given / when
		_, err := enumerate(t, map[string]string{"a.md": "# A\n"}, Params{Flavour: "roam"})

		// then
		require.Error(t, err)
		issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueSourceInvalid, issue.Code)
	})
}

func TestFlavourIssue(t *testing.T) {
	t.Run("non-generic detection is reported as info", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{
			"Tasks.csv":  "Name\n",
			"Tasks/a.md": "# A\n",
		}, Params{})

		// then
		require.NotEmpty(t, sink.issues)
		want := importv2.Info(importv2.IssueFlavourDetected, "markdown source detected as notion-export")
		assert.Equal(t, want, sink.issues[0])
	})

	t.Run("forced flavour is reported as requested", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{"a.md": "# A\n"}, Params{Flavour: FlavourObsidian})

		// then
		require.NotEmpty(t, sink.issues)
		want := importv2.Info(importv2.IssueFlavourDetected, "markdown source requested as obsidian")
		assert.Equal(t, want, sink.issues[0])
	})

	t.Run("generic detection stays silent", func(t *testing.T) {
		// given / when
		sink, _ := runConverterWithParams(t, map[string]string{"a.md": "# A\n"}, Params{})

		// then
		assert.Empty(t, sink.issues)
	})
}
